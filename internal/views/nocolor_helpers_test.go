package views

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/charmbracelet/colorprofile"
	uv "github.com/charmbracelet/ultraviolet"
)

// frameCells parses an SGR frame into screen cells and converts every style
// to profile p, as Bubble Tea's renderer does before it writes a cell.
func frameCells(s string, width int, p colorprofile.Profile) [][]uv.Cell {
	ss := uv.NewStyledString(s)
	h := ss.Height()
	buf := uv.NewScreenBuffer(width, h)
	ss.Draw(buf, uv.Rect(0, 0, width, h))
	rows := make([][]uv.Cell, h)
	for y := range h {
		for x := range width {
			c := buf.CellAt(x, y)
			if c == nil {
				c = &uv.EmptyCell
			}
			cell := *c
			cell.Style = uv.ConvertStyle(cell.Style, p)
			rows[y] = append(rows[y], cell)
		}
	}
	return rows
}

// cellsShowing returns the cells of the first row that shows text, spanning
// text. Every cell here holds one rune.
func cellsShowing(t *testing.T, rows [][]uv.Cell, text string) []uv.Cell {
	t.Helper()
	for _, row := range rows {
		var sb strings.Builder
		for _, c := range row {
			sb.WriteString(c.Content)
		}
		line := sb.String()
		if i := strings.Index(line, text); i >= 0 {
			start := utf8.RuneCountInString(line[:i])
			return row[start : start+utf8.RuneCountInString(text)]
		}
	}
	t.Fatalf("no row shows %q", text)
	return nil
}

// assertNoColour fails on any cell that keeps a foreground, background or
// underline colour.
func assertNoColour(t *testing.T, rows [][]uv.Cell, what string) {
	t.Helper()
	for y, row := range rows {
		for x, c := range row {
			if c.Style.Fg != nil || c.Style.Bg != nil || c.Style.UnderlineColor != nil {
				t.Fatalf("%s: cell (%d,%d) %q keeps a colour: %q", what, x, y, c.Content, c.Style.String())
			}
		}
	}
}

// assertAnyBold fails when no cell of the frame is bold.
func assertAnyBold(t *testing.T, rows [][]uv.Cell, what string) {
	t.Helper()
	for _, row := range rows {
		for _, c := range row {
			if c.Style.Attrs&uv.AttrBold != 0 {
				return
			}
		}
	}
	t.Fatalf("%s: no bold cell under NO_COLOR", what)
}
