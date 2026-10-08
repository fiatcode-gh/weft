package render

import (
	"strings"

	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
)

// RestyleRow draws row with st on every cell covered by a match. match gets the
// row's plain text (each cell's content, a wide cell once) and returns byte
// ranges of it. With no match the row is returned unchanged; otherwise the row is
// re-emitted as SGR runs, unmatched cells keeping their decoded style.
func RestyleRow(row string, match func(plain string) [][2]int, st uv.Style) string {
	w := ansi.StringWidth(row)
	if w == 0 {
		return row
	}
	buf := uv.NewScreenBuffer(w, 1)
	uv.NewStyledString(row).Draw(buf, buf.Bounds())

	var plain strings.Builder
	type cellAt struct {
		x, start int // screen column, byte offset in plain
	}
	var cells []cellAt
	for x := 0; x < w; {
		c := buf.CellAt(x, 0)
		if c == nil || c.Width == 0 {
			x++
			continue
		}
		cells = append(cells, cellAt{x, plain.Len()})
		plain.WriteString(c.Content)
		x += max(c.Width, 1)
	}
	ranges := match(plain.String())
	if len(ranges) == 0 {
		return row
	}

	var out, run strings.Builder
	var cur uv.Style
	flush := func() {
		if run.Len() > 0 {
			out.WriteString(cur.Styled(run.String()))
			run.Reset()
		}
	}
	ri := 0
	for i, ca := range cells {
		end := plain.Len()
		if i+1 < len(cells) {
			end = cells[i+1].start
		}
		for ri < len(ranges) && ranges[ri][1] <= ca.start {
			ri++
		}
		c := buf.CellAt(ca.x, 0)
		style := c.Style
		if ri < len(ranges) && ranges[ri][0] < end && ranges[ri][1] > ca.start {
			style = st
		}
		if run.Len() > 0 && !style.Equal(&cur) {
			flush()
		}
		cur = style
		run.WriteString(c.Content)
	}
	flush()
	return out.String()
}
