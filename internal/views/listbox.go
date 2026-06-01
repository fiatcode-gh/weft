package views

const (
	listInnerWidthMin  = 30
	listInnerWidthMax  = 80
	listVisibleRowsMin = 6
)

// listBox holds the geometry and selection state shared by the scrollable list
// overlays (picker, search, backlinks, todos). Concrete overlays embed it.
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
