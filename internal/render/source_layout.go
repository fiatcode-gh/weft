package render

import (
	"strings"
	"unicode/utf8"

	"github.com/charmbracelet/x/ansi"
	"github.com/rivo/uniseg"
)

// cell is one grapheme cluster of a source line as the editor draws it: the
// source bytes [src, end), the text that stands for them on screen and its
// width in terminal cells.
type cell struct {
	src, end int
	text     string
	w        int
}

// displayCells splits line into display cells. Tabs expand to the next
// multiple of 4 from line column 0; C0 controls and DEL show as ^X, C1 code
// points and invalid UTF-8 bytes as U+FFFD, so the buffer never reaches the
// terminal as raw control bytes.
func displayCells(line string) []cell {
	cells := make([]cell, 0, len(line))
	col := 0
	for pos := 0; pos < len(line); {
		var c cell
		r, size := utf8.DecodeRuneInString(line[pos:])
		switch {
		case r == utf8.RuneError && size == 1:
			c = cell{pos, pos + 1, "\uFFFD", 1}
		case r == '\t':
			w := 4 - col%4
			c = cell{pos, pos + 1, strings.Repeat(" ", w), w}
		case r < 0x20:
			c = cell{pos, pos + 1, "^" + string(rune('@'+r)), 2}
		case r == 0x7f:
			c = cell{pos, pos + 1, "^?", 2}
		case r >= 0x80 && r < 0xa0:
			c = cell{pos, pos + size, "\uFFFD", 1}
		default:
			cluster, _, _, _ := uniseg.FirstGraphemeClusterInString(line[pos:], -1)
			c = cell{pos, pos + len(cluster), cluster, ansi.StringWidth(cluster)}
		}
		cells = append(cells, c)
		col += c.w
		pos = c.end
	}
	return cells
}

// Geometry is how a theme and a width lay source lines out on screen.
type Geometry struct {
	Width       int
	Margin      int
	Text        int // wrap width of body text: Width less both margins, at least 1
	LevelIndent int
	QuoteIndent int
	CodePad     int // code block margin + indent
}

// NewGeometry reads the layout numbers Glamour renders with from t.
func NewGeometry(t Theme, width int) Geometry {
	g := Geometry{Width: width, LevelIndent: int(t.Config.List.LevelIndent)}
	if m := t.Config.Document.Margin; m != nil {
		g.Margin = int(*m)
	}
	g.Text = max(1, width-2*g.Margin)
	if q := t.Config.BlockQuote.Indent; q != nil {
		g.QuoteIndent = int(*q)
	}
	if m := t.Config.CodeBlock.Margin; m != nil {
		g.CodePad += int(*m)
	}
	if i := t.Config.CodeBlock.Indent; i != nil {
		g.CodePad += int(*i)
	}
	return g
}

// Row is the source bytes [Start, End) of a line drawn from screen column Col.
type Row struct{ Start, End, Col int }

// Rows lays line out in screen rows that start at the read view's columns and
// wrap where the read view wraps. There is always at least one row; row 0
// starts at byte 0 so leading whitespace is drawn.
func (g Geometry) Rows(line string, info LineInfo) []Row {
	m, s, w := g.Margin, info.Indent, g.Text
	_, ws := leadingSpace(line)
	from, pad, budget := ws, 0, w-s
	col0, colN := m, m+s
	switch info.Kind {
	case KindBullet:
		budget, colN = w-info.Level*g.LevelIndent, m+s+2
	case KindOrdered:
		budget = w - info.Level*g.LevelIndent
	case KindQuote:
		from, budget, colN = info.Marker, w-g.QuoteIndent, m+s+2
	case KindFence, KindCode:
		from, pad, budget = 0, g.CodePad, w
		col0, colN = m+g.CodePad, m
	}
	budget = max(1, budget)

	cells := displayCells(line)
	first := len(cells)
	for i, c := range cells {
		if c.src >= from {
			first = i
			break
		}
	}
	cellStart := func(k int) int {
		if k < len(cells) {
			return cells[k].src
		}
		return len(line)
	}
	rows := []Row{{Start: 0, Col: col0}}
	addRow := func(k, col int) {
		rows[len(rows)-1].End = cellStart(k)
		rows = append(rows, Row{Start: cellStart(k), Col: col})
	}

	breaks := wrapCells(cells[first:], pad, budget)
	if info.Kind == KindQuote && budget > 1 {
		// The read view draws each wrapped quote row after margin and bar,
		// which is one cell wider than Glamour's wrap width allows, and
		// re-wraps rows that overflow: an inner row exactly `budget` wide
		// loses its last word to an orphan row at the margin.
		var all []int
		start := 0
		for _, end := range append(append([]int(nil), breaks...), len(cells)-first) {
			row := cells[first+start : first+end]
			all = append(all, start)
			if textWidth(row) > budget-1 {
				for _, k := range wrapCells(row, 0, budget-1) {
					all = append(all, -(start + k)) // negative: orphan row
				}
			}
			start = end
		}
		for _, k := range all[1:] {
			if k < 0 {
				addRow(first-k, m)
			} else {
				addRow(first+k, colN)
			}
		}
	} else {
		for _, k := range breaks {
			addRow(first+k, colN)
		}
	}
	rows[len(rows)-1].End = len(line)
	return rows
}

// textWidth is the width of cells without their trailing spaces.
func textWidth(cells []cell) int {
	for len(cells) > 0 && cells[len(cells)-1].text == " " {
		cells = cells[:len(cells)-1]
	}
	w := 0
	for _, c := range cells {
		w += c.w
	}
	return w
}

// wrapCells wraps pad spaces followed by cells at budget with ansi.Wrap, the
// wrapper Glamour renders with, and returns the index of the first cell of every
// row after the first. A break inside the pad belongs to the first cell.
func wrapCells(cells []cell, pad, budget int) []int {
	var text strings.Builder
	text.WriteString(strings.Repeat(" ", pad))
	disp := make([]int, len(cells))
	for i, c := range cells {
		disp[i] = text.Len()
		text.WriteString(c.text)
	}
	in := text.String()
	offsets, ok := wrapBreaks(in, ansi.Wrap(in, budget, ""))
	if !ok {
		return nil
	}
	breaks := make([]int, len(offsets))
	k := 0
	for n, d := range offsets {
		for k < len(cells) && disp[k] < d {
			k++
		}
		breaks[n] = k
	}
	return breaks
}

// wrapBreaks aligns ansi.Wrap's output with its input and returns the input
// offsets where a row ends. Wrap only inserts newlines and drops spaces, but
// which spaces it drops depends on the line: a run of spaces at a break is
// sometimes dropped, sometimes carried to the start of the next row. A run
// keeps as many spaces on the earlier row as the next row does not start with.
// ok is false when the output is anything else.
func wrapBreaks(in, out string) (breaks []int, ok bool) {
	i, j := 0, 0
	for j < len(out) {
		switch {
		case i < len(in) && in[i] == out[j]:
			i++
			j++
		case out[j] == '\n':
			dropped := spaceRun(in[i:]) - spaceRun(out[j+1:])
			if dropped < 0 {
				return nil, false
			}
			i += dropped
			breaks = append(breaks, i)
			j++
		default:
			return nil, false
		}
	}
	return breaks, true
}

// spaceRun is the number of leading spaces in s.
func spaceRun(s string) int {
	n := 0
	for n < len(s) && s[n] == ' ' {
		n++
	}
	return n
}

// rowAtOffset returns the row that owns byte offset off: a boundary belongs to the
// next row, the line end to the last.
func rowAtOffset(rows []Row, off int) int {
	r := 0
	for r+1 < len(rows) && rows[r+1].Start <= off {
		r++
	}
	return r
}

// CellX returns the row and screen column of byte offset off.
func (g Geometry) CellX(line string, info LineInfo, rows []Row, off int) (row, x int) {
	row = rowAtOffset(rows, off)
	x = rows[row].Col
	for _, c := range displayCells(line) {
		if c.src >= off {
			break
		}
		if c.src >= rows[row].Start {
			x += c.w
		}
	}
	return row, x
}

// OffsetAt returns the last grapheme boundary of row whose column is at most x,
// never the End of a row that wraps onto another one.
func (g Geometry) OffsetAt(line string, info LineInfo, rows []Row, row, x int) int {
	row = max(0, min(row, len(rows)-1))
	r := rows[row]
	best, col := r.Start, r.Col
	for _, c := range displayCells(line) {
		if c.src < r.Start {
			continue
		}
		if c.src >= r.End || col > x {
			break
		}
		best = c.src
		col += c.w
	}
	if row == len(rows)-1 && col <= x {
		best = r.End
	}
	return best
}
