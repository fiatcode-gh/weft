package views

import "strings"

const (
	listInnerWidthMin  = 30
	listInnerWidthMax  = 80
	listVisibleRowsMin = 6
)

// listBox holds the geometry and selection state shared by the scrollable list
// overlays (picker, search, backlinks, todos, agenda). Concrete overlays embed it.
type listBox struct {
	width, height int
	sel           int
	drawn         rowMap // body lines of the selectable items the last View drew
}

// selected is the selected item.
func (b *listBox) selected() int { return b.sel }

// drawnRows is the map of the rows the last View drew.
func (b *listBox) drawnRows() *rowMap { return &b.drawn }

// rowSpan is the body lines [from, to) one selectable item occupies.
type rowSpan struct{ from, to, item int }

// rowMap records which body line of a View belongs to which item, so a mouse
// click on a screen row can find its item. View resets it, then records each
// selectable item as it writes it; chrome, headers and blank lines stay
// unmapped. The slice is reused across renders.
type rowMap struct {
	spans      []rowSpan
	seen, line int
}

func (m *rowMap) reset() {
	m.spans = m.spans[:0]
	m.seen, m.line = 0, 0
}

// span records the lines written to b since from (a builder offset at the
// start of the item's first line; every line ends in "\n") as item's.
func (m *rowMap) span(b *strings.Builder, from, item int) {
	s := b.String()
	m.line += strings.Count(s[m.seen:from], "\n")
	start := m.line
	m.line += strings.Count(s[from:], "\n")
	m.seen = len(s)
	m.spans = append(m.spans, rowSpan{start, m.line, item})
}

// itemAt is the item drawn on body line, if any.
func (m *rowMap) itemAt(line int) (int, bool) {
	for _, s := range m.spans {
		if line >= s.from && line < s.to {
			return s.item, true
		}
	}
	return 0, false
}

// SetSize updates the cached terminal dimensions.
func (b *listBox) SetSize(w, h int) { b.width, b.height = w, h }

// innerWidth is the content-column budget: terminal width minus border (2),
// padding (4), and a 4-cell safety margin, clamped to [30, 80].
func (b *listBox) innerWidth() int {
	return clampInt(b.width-2-4-4, listInnerWidthMin, listInnerWidthMax)
}

// moveUp moves the selection up one row, stopping at the top.
func (b *listBox) moveUp() {
	if b.sel > 0 {
		b.sel--
	}
}

// moveDown moves the selection down one row, clamped to [0, count).
func (b *listBox) moveDown(count int) {
	if b.sel < count-1 {
		b.sel++
	}
}

// groupedWindow chooses a [start, end) slice of n rows to display so that sel
// is always inside it, where every row costs 1 terminal row and a row whose
// group differs from its predecessor's costs 1 more for its header, plus 1 for
// the blank line before it when it is not the first group shown. budget is the
// terminal-row allowance.
func groupedWindow(n, sel, budget int, group func(int) string) (start, end int) {
	if n == 0 || budget <= 0 {
		return 0, 0
	}

	// Rows the i-th row adds when i follows prevGroup at i-1.
	cost := func(i int, prevGroup string) int {
		c := 1
		if group(i) != prevGroup {
			c++
			if prevGroup != "" {
				c++
			}
		}
		return c
	}

	walkForward := func(s int) int {
		used := 0
		prev := ""
		e := s
		for i := s; i < n; i++ {
			c := cost(i, prev)
			if used+c > budget {
				break
			}
			used += c
			prev = group(i)
			e = i + 1
		}
		return e
	}

	// Centre the selection in the window. If sel ends up past the rendered end
	// (because the chosen start left too little budget), nudge start forward
	// until sel fits — guaranteed to terminate because start can rise to sel.
	start = max(0, sel-budget/2)
	end = walkForward(start)
	for end <= sel && start < sel {
		start++
		end = walkForward(start)
	}
	return start, end
}
