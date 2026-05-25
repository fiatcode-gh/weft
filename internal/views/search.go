package views

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/fiatcode/peekseq/internal/graph"
)

// SearchHit is one rg result row.
type SearchHit struct {
	FilePath string
	Line     int
	Context  string
	// Matches are byte offsets within Context that the query matched, so the
	// view can emphasise them in the rendered list.
	Matches []SearchSpan
}

// SearchSpan is a [Start, End) byte range inside SearchHit.Context.
type SearchSpan struct {
	Start, End int
}

type SearchView struct {
	idx        *graph.Index
	query      string
	hits       []SearchHit
	sel        int
	running    bool
	err        error
	width      int               // terminal width snapshot, for layout
	height     int               // terminal height snapshot, for scroll-window sizing
	pathToName map[string]string // file path → logical page name, for tidy row prefixes
}

func NewSearchView(idx *graph.Index, width, height int) *SearchView {
	s := &SearchView{idx: idx, width: width, height: height}
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

// SetSize updates the cached terminal dimensions.
func (s *SearchView) SetSize(w, h int) { s.width, s.height = w, h }

func (s *SearchView) Query() string { return s.query }

func (s *SearchView) SetQuery(q string) { s.query = q }

// SearchCmd returns a tea.Cmd that runs rg and returns a searchDoneMsg.
func (s *SearchView) SearchCmd(graphPath string) tea.Cmd {
	q := s.query
	return func() tea.Msg {
		out, err := runRipgrep(graphPath, q)
		if err != nil {
			return searchDoneMsg{err: err}
		}
		return searchDoneMsg{hits: parseRipgrepJSON(out)}
	}
}

type searchDoneMsg struct {
	hits []SearchHit
	err  error
}

func (s *SearchView) Apply(msg searchDoneMsg) {
	s.running = false
	s.err = msg.err
	s.hits = msg.hits
	if s.sel >= len(s.hits) {
		s.sel = 0
	}
}

// Update handles a key. Returns (selected hit, accept, cancel, cmd to run).
func (s *SearchView) Update(key string, graphPath string) (hit *SearchHit, accept, cancel bool, cmd tea.Cmd) {
	switch key {
	case "esc":
		return nil, false, true, nil
	case "enter":
		if s.running {
			return nil, false, false, nil
		}
		if s.query == "" {
			return nil, false, false, nil
		}
		if len(s.hits) == 0 {
			s.running = true
			return nil, false, false, s.SearchCmd(graphPath)
		}
		if s.sel >= 0 && s.sel < len(s.hits) {
			h := s.hits[s.sel]
			return &h, true, false, nil
		}
	case "up", "ctrl+k":
		if s.sel > 0 {
			s.sel--
		}
	case "down", "ctrl+j":
		if s.sel < len(s.hits)-1 {
			s.sel++
		}
	case "backspace":
		if len(s.query) > 0 {
			s.query = s.query[:len(s.query)-1]
			s.hits = nil
		}
	case " ", "space":
		// Both forms covered in case bubbletea reports the space key as
		// the literal " " (default in v1) or the named "space" elsewhere.
		s.query += " "
		s.hits = nil
	default:
		if len(key) == 1 {
			s.query += key
			s.hits = nil
		}
	}
	return nil, false, false, nil
}

var (
	searchBorder = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(1, 2)
	searchTitle  = lipgloss.NewStyle().Bold(true)
	searchPrompt = lipgloss.NewStyle().Faint(true)
	searchSel    = lipgloss.NewStyle().Foreground(lipgloss.Color("0")).Background(lipgloss.Color("12")).Bold(true)
	searchFaint  = lipgloss.NewStyle().Faint(true)
	searchHitPos = lipgloss.NewStyle().Foreground(lipgloss.Color("12"))
	searchMatch  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("11"))
)

// matchesWithin returns the subset of h.Matches that still fits inside the
// first ctxBytes bytes of the (possibly truncated) context.
func (s *SearchView) matchesWithin(h SearchHit, ctxBytes int) []SearchSpan {
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
		out = append(out, SearchSpan{Start: m.Start, End: end})
	}
	return out
}

// highlightMatches emphasises each [Start,End) span of ctx with searchMatch.
// Overlapping or out-of-range spans are skipped defensively.
func highlightMatches(ctx string, spans []SearchSpan) string {
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

const (
	searchInnerWidthMax  = 80
	searchInnerWidthMin  = 30
	searchVisibleRowsMax = 14
	searchVisibleRowsMin = 6
)

// searchInnerWidth returns the column width to budget for content lines,
// shrinking to fit narrower terminals so the right edge of the overlay
// never falls off-screen.
func (s *SearchView) innerWidth() int {
	// Border (2) + padding (4) + a 2-col safety margin on each side.
	w := s.width - 2 - 4 - 4
	if w > searchInnerWidthMax {
		w = searchInnerWidthMax
	}
	if w < searchInnerWidthMin {
		w = searchInnerWidthMin
	}
	return w
}

// visibleRows returns how many hit rows the search overlay renders at once.
// Hits scroll within this window when there are more of them.
func (s *SearchView) visibleRows() int {
	// Chrome: 2 (border) + 2 (padding) + 1 (title) + 1 (blank) + 1 (prompt)
	// + 1 (divider) + 1 (blank) + 1 (hint) ≈ 10 lines.
	const chrome = 10
	r := s.height - chrome
	if r > searchVisibleRowsMax {
		r = searchVisibleRowsMax
	}
	if r < searchVisibleRowsMin {
		r = searchVisibleRowsMin
	}
	return r
}

// scrollWindow returns the [start, end) slice indices of hits to render
// such that s.sel is always visible.
func (s *SearchView) scrollWindow() (start, end int) {
	rows := s.visibleRows()
	if rows >= len(s.hits) {
		return 0, len(s.hits)
	}
	half := rows / 2
	start = s.sel - half
	if start < 0 {
		start = 0
	}
	end = start + rows
	if end > len(s.hits) {
		end = len(s.hits)
		start = end - rows
	}
	return start, end
}

func (s *SearchView) View() string {
	inner := s.innerWidth()
	var b strings.Builder
	b.WriteString(searchTitle.Render("Search the graph"))
	b.WriteString("\n\n")
	b.WriteString(searchPrompt.Render("/ "))
	b.WriteString(clamp(s.query, inner-3))
	switch {
	case s.err != nil:
		b.WriteString(searchFaint.Render(clamp(fmt.Sprintf("   error: %v", s.err), inner)))
	case s.running:
		b.WriteString(searchFaint.Render("   searching…"))
	case len(s.hits) == 0 && s.query != "":
		b.WriteString(searchFaint.Render("   press enter to search"))
	}
	b.WriteString("\n")
	b.WriteString(searchFaint.Render(strings.Repeat("─", inner)))
	b.WriteString("\n")
	if len(s.hits) == 0 && s.query == "" {
		b.WriteString(searchFaint.Render("  type a query and press enter"))
		b.WriteString("\n")
	}
	// rowBudget leaves room for the 3-cell marker and keeps one cell of
	// headroom so a wide-character edge case can't push the line past the
	// inner content width and force lipgloss to wrap it.
	rowBudget := inner - 4
	const posCol = 22 // width reserved for "{page}:{line}" so context columns line up
	sep := searchFaint.Render(" · ")
	sepW := lipgloss.Width(sep)
	ctxBudget := rowBudget - posCol - sepW
	if ctxBudget < 8 {
		ctxBudget = 8
	}
	start, end := s.scrollWindow()
	if start > 0 {
		b.WriteString(searchFaint.Render(fmt.Sprintf("   ↑ %d more above", start)))
		b.WriteString("\n")
	}
	for i := start; i < end; i++ {
		h := s.hits[i]
		label := s.hitLabel(h.FilePath)
		posStr := fmt.Sprintf("%s:%d", label, h.Line)
		posStr = padTo(clamp(posStr, posCol), posCol)
		ctx := clamp(h.Context, ctxBudget)

		marker := "   " // 3-cell so unselected rows align with " ▶ " width
		var line string
		if i == s.sel {
			// Selected rows render in one blue-bg pass — applying match
			// emphasis on top would inject nested SGR resets that clobber
			// the selection background.
			marker = searchSel.Render(" ▶ ")
			line = searchSel.Render(posStr + " · " + ctx)
		} else {
			pos := searchHitPos.Render(posStr)
			line = pos + sep + highlightMatches(ctx, s.matchesWithin(h, len(ctx)))
		}
		b.WriteString(marker)
		b.WriteString(line)
		b.WriteString("\n")
	}
	if end < len(s.hits) {
		b.WriteString(searchFaint.Render(fmt.Sprintf("   ↓ %d more below", len(s.hits)-end)))
		b.WriteString("\n")
	}
	b.WriteString("\n")
	b.WriteString(searchFaint.Render("↑/↓ select · enter search/open · esc cancel"))
	// Width includes horizontal padding (2 cells each side) but excludes the
	// border, so adding 4 keeps the text area at exactly `inner` cells.
	return searchBorder.Width(inner + 4).Render(b.String())
}

func shortPath(p string) string {
	parts := strings.Split(p, "/")
	if len(parts) < 2 {
		return p
	}
	return strings.Join(parts[len(parts)-2:], "/")
}

func runRipgrep(graphPath, query string) ([]byte, error) {
	// --smart-case keeps the search case-insensitive unless the pattern
	// contains an uppercase letter, which matches what most interactive
	// users expect from a quick search.
	args := []string{"--json", "--smart-case", "--", query}
	baseArgs := len(args)
	for _, sub := range []string{"pages", "journals"} {
		p := filepath.Join(graphPath, sub)
		if _, err := os.Stat(p); err == nil {
			args = append(args, p)
		}
	}
	if len(args) == baseArgs {
		// No pages/ or journals/ dir — nothing to search.
		return nil, nil
	}
	cmd := exec.Command("rg", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
		return stdout.Bytes(), nil
	}
	if err != nil {
		return nil, fmt.Errorf("rg failed: %w (%s)", err, stderr.String())
	}
	return stdout.Bytes(), nil
}

func parseRipgrepJSON(b []byte) []SearchHit {
	var out []SearchHit
	sc := bufio.NewScanner(bytes.NewReader(b))
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		var env struct {
			Type string `json:"type"`
			Data struct {
				Path       struct{ Text string } `json:"path"`
				Lines      struct{ Text string } `json:"lines"`
				LineNumber int                   `json:"line_number"`
				Submatches []struct {
					Start int `json:"start"`
					End   int `json:"end"`
				} `json:"submatches"`
			} `json:"data"`
		}
		if err := json.Unmarshal(sc.Bytes(), &env); err != nil {
			continue
		}
		if env.Type != "match" {
			continue
		}
		ctx := strings.TrimRight(env.Data.Lines.Text, "\n")
		// Strip leading whitespace from indented bullets so list rows line
		// up. Match spans are byte offsets into the *original* line, so we
		// also shift them by the number of bytes we trimmed.
		trimmed := strings.TrimLeft(ctx, " \t")
		shift := len(ctx) - len(trimmed)
		ctx = trimmed
		ctxLen := len(ctx)
		spans := make([]SearchSpan, 0, len(env.Data.Submatches))
		for _, sm := range env.Data.Submatches {
			start := sm.Start - shift
			end := sm.End - shift
			if end <= 0 || start >= ctxLen || start >= end {
				continue // span fell entirely inside the trimmed indent, or is degenerate
			}
			if start < 0 {
				start = 0
			}
			if end > ctxLen {
				end = ctxLen
			}
			spans = append(spans, SearchSpan{Start: start, End: end})
		}
		out = append(out, SearchHit{
			FilePath: env.Data.Path.Text,
			Line:     env.Data.LineNumber,
			Context:  ctx,
			Matches:  spans,
		})
	}
	return out
}
