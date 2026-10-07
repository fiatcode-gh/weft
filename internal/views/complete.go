package views

import (
	"fmt"
	"strings"

	"github.com/sahilm/fuzzy"

	"github.com/fiatcode-gh/weft/v2/internal/graph"
)

// maxCompleterRows caps how many candidate rows the strip shows at once; it
// scrolls within this window when the list is longer.
const maxCompleterRows = 8

// linkCandidate is one row in the completion strip. create marks the synthetic
// "＋ Create" row offered when the partial matches no existing page.
type linkCandidate struct {
	name   string
	create bool
}

// linkCompleter is the pure logic behind the editor's live [[ completion. It
// holds the page-name corpus and derives its active/partial/candidate state
// from the text before the cursor (refresh). It never touches the buffer;
// EditorView translates between the two.
type linkCompleter struct {
	choices    []pickerChoice // deduped, mtime-sorted page names (reused from picker)
	names      []string       // lockstep with choices, for fuzzy.Find
	active     bool
	dismissed  bool   // user pressed esc; stays closed until the partial changes
	partial    string // runes between the open [[ and the cursor
	cands      []linkCandidate
	sel        int
	maxVisible int // EditorView lowers this on short terminals; default maxCompleterRows
}

func newLinkCompleter(idx *graph.Index) *linkCompleter {
	c := &linkCompleter{maxVisible: maxCompleterRows}
	if idx != nil {
		c.choices = pickerChoices(idx)
		c.names = make([]string, len(c.choices))
		for i, ch := range c.choices {
			c.names[i] = ch.name
		}
	}
	return c
}

// visible is the number of candidate rows the strip may show, clamped to the
// [1, maxCompleterRows] range. EditorView lowers maxVisible on short terminals.
func (c *linkCompleter) visible() int {
	return clampInt(c.maxVisible, 1, maxCompleterRows)
}

// extractPartial finds the active [[ completion query in the text before the
// cursor: the runes after the nearest "[[" with no intervening bracket. It
// returns ("", false) when there is no open link to complete.
func extractPartial(before string) (string, bool) {
	i := strings.LastIndex(before, "[[")
	if i < 0 {
		return "", false
	}
	partial := before[i+2:]
	if strings.ContainsAny(partial, "[]") {
		return "", false
	}
	return partial, true
}

// closingAhead reports whether the text after the cursor closes a wiki-link
// (a "]]" not preceded by another "[[") before any new link opens. When true,
// the cursor sits inside an already-closed [[link]] — out of scope, so the
// completer must not activate (completing there would corrupt the link).
func closingAhead(after string) bool {
	closeAt := strings.Index(after, "]]")
	if closeAt < 0 {
		return false
	}
	openAt := strings.Index(after, "[[")
	return openAt < 0 || closeAt < openAt
}

// refresh re-derives the completer state from the text before and after the
// cursor on the current logical row. It runs after every key forwarded to the
// buffer. allowOpen reports whether that key edited the buffer: completion is
// a typing affordance, so only an edit may transition the strip from closed to
// open. A bare caret move (or file open, or page scroll) passes allowOpen=false
// and therefore cannot pop the strip just because the cursor landed to the
// right of an unclosed "[[" — though once open, navigation still updates it or
// closes it (e.g. when the cursor leaves the link).
func (c *linkCompleter) refresh(before, after string, allowOpen bool) {
	partial, ok := extractPartial(before)
	if !ok || closingAhead(after) {
		c.active = false
		c.dismissed = false
		c.partial = ""
		c.cands = nil
		return
	}
	if !allowOpen && !c.active {
		return
	}
	if partial != c.partial {
		c.partial = partial
		c.dismissed = false
		c.sel = 0
	}
	if c.dismissed {
		c.active = false
		return
	}
	c.cands = c.buildCands(partial)
	c.active = len(c.cands) > 0
	if c.sel >= len(c.cands) {
		c.sel = 0
	}
}

// buildCands returns the candidate rows for a partial: fuzzy matches over the
// page corpus, or — when nothing matches a non-empty partial — a single
// create row. An empty partial lists the recent (mtime-sorted) pages.
func (c *linkCompleter) buildCands(partial string) []linkCandidate {
	var out []linkCandidate
	if partial == "" {
		for _, ch := range c.choices {
			out = append(out, linkCandidate{name: ch.name})
		}
		return out
	}
	for _, m := range fuzzy.Find(partial, c.names) {
		out = append(out, linkCandidate{name: m.Str})
	}
	if len(out) == 0 {
		// Intentionally differs from picker's refreshCreate (which also checks
		// idx.Resolve): fuzzy.Find already returns exact matches, so an existing
		// page never reaches here; dates are intentionally not suppressed.
		out = append(out, linkCandidate{name: partial, create: true})
	}
	return out
}

func (c *linkCompleter) selected() (linkCandidate, bool) {
	if !c.active || c.sel < 0 || c.sel >= len(c.cands) {
		return linkCandidate{}, false
	}
	return c.cands[c.sel], true
}

func (c *linkCompleter) moveUp() {
	if c.sel > 0 {
		c.sel--
	}
}

func (c *linkCompleter) moveDown() {
	if c.sel < len(c.cands)-1 {
		c.sel++
	}
}

// dismiss closes the strip until the partial changes (esc).
func (c *linkCompleter) dismiss() {
	c.dismissed = true
	c.active = false
}

// rows is how many terminal rows the rendered strip occupies (candidates,
// capped, plus the box border and vertical padding), or 0 when inactive.
// EditorView uses this to shrink the text window so the strip fits.
func (c *linkCompleter) rows() int {
	if !c.active || len(c.cands) == 0 {
		return 0
	}
	n := len(c.cands)
	if n > c.visible() {
		n = c.visible()
	}
	return n + 4 // 2 border rows + 2 vertical-padding rows (styleBorder has Padding(1, 2))
}

// View renders the candidate strip, or "" when inactive.
func (c *linkCompleter) View(width int) string {
	if !c.active || len(c.cands) == 0 {
		return ""
	}
	inner := width - 4
	if inner < 10 {
		inner = 10
	}
	start, end := scrollWindow(c.sel, len(c.cands), c.visible())
	var b strings.Builder
	for i := start; i < end; i++ {
		cand := c.cands[i]
		label := cand.name
		if cand.create {
			label = fmt.Sprintf("＋ Create %q", cand.name)
		}
		// box content is inner-2 (padding 1,2) and the marker takes 3 cells
		label = clamp(label, inner-5)
		marker := "   "
		if i == c.sel {
			marker = styleSel.Render(" ▶ ")
			label = styleSel.Render(label)
		} else {
			label = styleFaint.Render(label)
		}
		b.WriteString(marker)
		b.WriteString(label)
		if i < end-1 {
			b.WriteString("\n")
		}
	}
	return renderBordered(inner+2, b.String())
}
