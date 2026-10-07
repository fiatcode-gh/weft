package views

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/fiatcode-gh/weft/v2/internal/graph"
	"github.com/fiatcode-gh/weft/v2/internal/search"
)

type SearchView struct {
	listBox
	idx        *graph.Index
	input      textinput.Model
	gen        int // bumped on every query mutation; tags in-flight searches
	hits       []search.Hit
	running    bool
	searched   bool // a search has completed for the current query (vs. never run)
	err        error
	pathToName map[string]string // file path → logical page name, for tidy row prefixes
}

func NewSearchView(idx *graph.Index, width, height int) *SearchView {
	ti := textinput.New()
	ti.Focus()
	s := &SearchView{listBox: listBox{width: width, height: height}, idx: idx, input: ti}
	s.pathToName = make(map[string]string, len(idx.Pages))
	for _, p := range idx.Pages {
		s.pathToName[p.Path] = p.Name
	}
	return s
}

// hitLabel returns the column-left label for a hit row: the page name when
// the file is in the index, falling back to a short path otherwise.
func (s *SearchView) hitLabel(filePath string) string {
	if name, ok := s.pathToName[filePath]; ok {
		return name
	}
	return shortPath(filePath)
}

func (s *SearchView) Query() string { return s.input.Value() }

// SetQuery sets the query directly; test-only — it does not bump gen, so
// production code must mutate the query through Update to keep the
// staleness guard correct.
func (s *SearchView) SetQuery(q string) { s.input.SetValue(q) }

// SearchCmd returns a tea.Cmd that runs rg and returns a searchDoneMsg tagged
// with this view instance and the query's generation at launch time, so a
// result for an edited-away query (or a since-replaced overlay) can be
// dropped instead of landing under whatever query is current now.
func (s *SearchView) SearchCmd(graphPath string) tea.Cmd {
	q := s.input.Value()
	gen := s.gen
	return func() tea.Msg {
		hits, err := search.Run(graphPath, q)
		return searchDoneMsg{view: s, gen: gen, hits: hits, err: err}
	}
}

type searchDoneMsg struct {
	view *SearchView // which overlay instance ran the search
	gen  int         // s.gen at launch; stale generations are dropped
	hits []search.Hit
	err  error
}

// Apply installs a finished search's results. Results from an edited-away
// query (stale gen) are dropped. A failed search leaves searched=false so
// enter retries instead of dead-ending the query.
func (s *SearchView) Apply(msg searchDoneMsg) {
	if msg.gen != s.gen {
		return
	}
	s.running = false
	s.searched = msg.err == nil
	s.err = msg.err
	s.hits = msg.hits
	if s.sel >= len(s.hits) {
		s.sel = 0
	}
}

// pageName returns the logical page name for a hit's file path, or "" when the
// file isn't in the index (e.g. a result outside pages/ and journals/).
func (s *SearchView) pageName(filePath string) string {
	return s.pathToName[filePath]
}

// Update handles a key and reports the result to the App.
func (s *SearchView) Update(key string) OverlayResult {
	switch key {
	case keyEsc:
		return overlayCancel()
	case keyEnter:
		if s.running || s.input.Value() == "" {
			return OverlayResult{}
		}
		if len(s.hits) == 0 {
			if s.searched {
				return OverlayResult{} // already searched, no matches — don't re-run
			}
			s.running = true
			return overlayCommand(s.SearchCmd(s.idx.GraphPath))
		}
		if s.sel >= 0 && s.sel < len(s.hits) {
			return overlayOpen(s.pageName(s.hits[s.sel].FilePath))
		}
		return OverlayResult{}
	case keyUp, keyCtrlK:
		s.moveUp()
	case keyDown, keyCtrlJ:
		s.moveDown(len(s.hits))
	default:
		before := s.input.Value()
		ti, handled := consumeKey(s.input, key)
		if !handled {
			return OverlayResult{}
		}
		s.input = ti
		if s.input.Value() != before {
			// Query changed: previous results and any in-flight search
			// no longer apply (gen guard drops late arrivals).
			s.gen++
			s.hits = nil
			s.searched = false
			s.running = false
			s.err = nil
		}
	}
	return OverlayResult{}
}

var (
	searchHitPos = lipgloss.NewStyle().Foreground(colorHighlight)
	searchMatch  = lipgloss.NewStyle().Bold(true).Foreground(colorCursor)
)

// matchesWithin returns the subset of h.Matches that still fits inside the
// first ctxBytes bytes of the (possibly truncated) context.
func (s *SearchView) matchesWithin(h search.Hit, ctxBytes int) []search.Span {
	if len(h.Matches) == 0 {
		return nil
	}
	out := h.Matches[:0:0]
	for _, m := range h.Matches {
		if m.Start >= ctxBytes {
			break
		}
		end := m.End
		if end > ctxBytes {
			end = ctxBytes
		}
		if m.Start >= end {
			continue
		}
		out = append(out, search.Span{Start: m.Start, End: end})
	}
	return out
}

// highlightMatches emphasises each [Start,End) span of ctx with searchMatch.
// Overlapping or out-of-range spans are skipped defensively.
func highlightMatches(ctx string, spans []search.Span) string {
	if len(spans) == 0 {
		return ctx
	}
	var b strings.Builder
	last := 0
	for _, m := range spans {
		if m.Start < last || m.End > len(ctx) || m.Start >= m.End {
			continue
		}
		b.WriteString(ctx[last:m.Start])
		b.WriteString(searchMatch.Render(ctx[m.Start:m.End]))
		last = m.End
	}
	b.WriteString(ctx[last:])
	return b.String()
}

const searchVisibleRowsMax = 14

// visibleRows returns how many hit rows the search overlay renders at once.
// Hits scroll within this window when there are more of them.
func (s *SearchView) visibleRows() int {
	// Chrome: 2 (border) + 2 (padding) + 1 (title) + 1 (blank) + 1 (prompt)
	// + 1 (divider) + 1 (blank) + 1 (hint) ≈ 10 lines.
	const chrome = 10
	return clampInt(s.height-chrome, listVisibleRowsMin, searchVisibleRowsMax)
}

func (s *SearchView) View() string {
	inner := s.innerWidth()
	var b strings.Builder
	b.WriteString(styleTitle.Render("Search the graph"))
	b.WriteString("\n\n")
	b.WriteString(styleFaint.Render("/ "))
	query := s.input.Value()
	b.WriteString(clamp(query, inner-3))
	switch {
	case s.err != nil:
		b.WriteString(styleFaint.Render(clamp(fmt.Sprintf("   error: %v", s.err), inner)))
	case s.running:
		b.WriteString(styleFaint.Render("   searching…"))
	case s.searched && len(s.hits) == 0 && query != "":
		b.WriteString(styleFaint.Render("   no matches"))
	case len(s.hits) == 0 && query != "":
		b.WriteString(styleFaint.Render("   press enter to search"))
	}
	b.WriteString("\n")
	b.WriteString(styleFaint.Render(strings.Repeat("─", inner)))
	b.WriteString("\n")
	if len(s.hits) == 0 && query == "" {
		b.WriteString(styleFaint.Render("  type a query and press enter"))
		b.WriteString("\n")
	}
	// rowBudget leaves room for the 3-cell marker and keeps one cell of
	// headroom so a wide-character edge case can't push the line past the
	// inner content width and force lipgloss to wrap it.
	rowBudget := inner - 4
	const posCol = 22 // width reserved for "{page}:{line}" so context columns line up
	sep := styleFaint.Render(" · ")
	sepW := lipgloss.Width(sep)
	ctxBudget := rowBudget - posCol - sepW
	if ctxBudget < 8 {
		ctxBudget = 8
	}
	start, end := scrollWindow(s.sel, len(s.hits), s.visibleRows())
	if start > 0 {
		b.WriteString(styleFaint.Render(fmt.Sprintf("   ↑ %d more above", start)))
		b.WriteString("\n")
	}
	for i := start; i < end; i++ {
		h := s.hits[i]
		label := s.hitLabel(h.FilePath)
		posStr := fmt.Sprintf("%s:%d", label, h.Line)
		posStr = padTo(clamp(posStr, posCol), posCol)
		ctx := clamp(h.Context, ctxBudget)
		// clamp appends a 1-column "…" when it truncates; bound match
		// highlighting to the kept prefix so a span can't colour the ellipsis.
		ctxLimit := len(ctx)
		if lipgloss.Width(h.Context) > ctxBudget {
			ctxLimit -= len("…")
		}

		marker := "   " // 3-cell so unselected rows align with " ▶ " width
		var line string
		if i == s.sel {
			// Selected rows render in one blue-bg pass — applying match
			// emphasis on top would inject nested SGR resets that clobber
			// the selection background.
			marker = styleSel.Render(" ▶ ")
			line = styleSel.Render(posStr + " · " + ctx)
		} else {
			pos := searchHitPos.Render(posStr)
			line = pos + sep + highlightMatches(ctx, s.matchesWithin(h, ctxLimit))
		}
		b.WriteString(marker)
		b.WriteString(line)
		b.WriteString("\n")
	}
	if end < len(s.hits) {
		b.WriteString(styleFaint.Render(fmt.Sprintf("   ↓ %d more below", len(s.hits)-end)))
		b.WriteString("\n")
	}
	b.WriteString("\n")
	b.WriteString(styleFaint.Render(clamp("↑/↓ select · enter search/open · esc cancel", inner)))
	// Width includes horizontal padding (2 cells each side) but excludes the
	// border, so adding 4 keeps the text area at exactly `inner` cells.
	return renderBordered(inner+4, b.String())
}

func shortPath(p string) string {
	parts := strings.Split(p, "/")
	if len(parts) < 2 {
		return p
	}
	return strings.Join(parts[len(parts)-2:], "/")
}
