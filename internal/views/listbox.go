package views

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
