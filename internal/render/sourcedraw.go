package render

import (
	"strings"

	uv "github.com/charmbracelet/ultraviolet"
)

// Span is the style of the bytes [Start, End) of one source line.
type Span struct {
	Start, End int
	Style      uv.Style
}

// Overlay returns spans with st replacing every style on [start, end),
// gaps included. spans must be sorted and non-overlapping, as Paint returns
// them; the input is not modified.
func Overlay(spans []Span, start, end int, st uv.Style) []Span {
	if start >= end {
		return spans
	}
	out := make([]Span, 0, len(spans)+2)
	i := 0
	for ; i < len(spans) && spans[i].End <= start; i++ {
		out = append(out, spans[i])
	}
	if i < len(spans) && spans[i].Start < start {
		out = append(out, Span{spans[i].Start, start, spans[i].Style})
	}
	out = append(out, Span{start, end, st})
	for ; i < len(spans) && spans[i].End <= end; i++ {
	}
	if i < len(spans) && spans[i].Start < end {
		out = append(out, Span{end, spans[i].End, spans[i].Style})
		i++
	}
	return append(out, spans[i:]...)
}

// DrawRow returns the display cells of the bytes [row.Start, row.End) of line
// as SGR runs (without row.Col's leading padding). Spans give the styles;
// runs of equal style are merged. The line goes through displayCells, so the
// result holds no control byte from the buffer: only the SGR sequences DrawRow
// generates.
func DrawRow(line string, info LineInfo, row Row, spans []Span) string {
	var out, run strings.Builder
	var cur uv.Style
	flush := func() {
		if run.Len() > 0 {
			out.WriteString(cur.Styled(run.String()))
			run.Reset()
		}
	}
	si := 0
	for _, c := range displayCells(line) {
		if c.src < row.Start || c.src >= row.End {
			continue
		}
		for si < len(spans) && spans[si].End <= c.src {
			si++
		}
		var st uv.Style
		if si < len(spans) && spans[si].Start <= c.src {
			st = spans[si].Style
		}
		if run.Len() > 0 && !st.Equal(&cur) {
			flush()
		}
		cur = st
		run.WriteString(c.text)
	}
	flush()
	return out.String()
}
