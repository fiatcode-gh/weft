package views

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/fiatcode-gh/weft/v2/internal/render"
)

func plainLines(n int) string {
	var b strings.Builder
	for i := range n {
		fmt.Fprintf(&b, "- line %d\n", i)
	}
	return b.String()
}

// mixedLines alternates long (wrapping), short, empty and medium lines so the
// remembered column has to survive rows of very different lengths.
func mixedLines(n int) string {
	var b strings.Builder
	for i := range n {
		switch i % 4 {
		case 0:
			fmt.Fprintf(&b, "- %d a fairly long bullet that wraps over several visual rows when the editor is narrow enough ok\n", i)
		case 1:
			fmt.Fprintf(&b, "- %d s\n", i)
		case 2:
			b.WriteString("\n")
		default:
			fmt.Fprintf(&b, "- %d medium length line of words\n", i)
		}
	}
	return b.String()
}

func wrappedLines(n int) string {
	var b strings.Builder
	for i := range n {
		fmt.Fprintf(&b, "- %d %s\n", i, strings.Repeat("word ", 30+i%7*9))
	}
	return b.String()
}

// pagingModel is an independent description of the document's display rows,
// built from the render package only, that the paging invariants are checked
// against: index 0 is line 0's margin row, then every row of every line.
type pagingModel struct {
	rows []viewPos
	x    func(p viewPos, off int) int
}

func newPagingModel(e *EditorView) pagingModel {
	sc := render.NewScanner()
	m := pagingModel{rows: []viewPos{{0, -1}}}
	type lc struct {
		line string
		info render.LineInfo
		rows []render.Row
	}
	lines := make([]lc, e.buf.Len())
	for i := range lines {
		l := e.buf.Line(i)
		info := sc.Info(e.buf, i)
		lines[i] = lc{l, info, e.geo.Rows(l, info)}
		for r := range lines[i].rows {
			m.rows = append(m.rows, viewPos{i, r})
		}
	}
	m.x = func(p viewPos, off int) int {
		c := lines[p.line]
		_, x := e.geo.CellX(c.line, c.info, c.rows, off)
		return x
	}
	return m
}

func (m pagingModel) index(p viewPos) int {
	for i, r := range m.rows {
		if r == p {
			return i
		}
	}
	return -1
}

// TestEditorPagingMoves pins PgUp/PgDn: h-1 display rows per page, the screen
// column remembered across rows of other lengths, and the viewport moved only
// as far as the cursor needs. Every step is checked against invariants of an
// independent row model, and against a literal trace of the cursor's line,
// absolute column and row inside a wrapped line, and the window's top row
// (D = PgDn, U = PgUp). The traces are the v1 scripts replayed on the buffer
// editor and reviewed: the viewport now has the top margin as a display row,
// and rows hold 2 columns of left inset, so wrapped positions differ from v1.
func TestEditorPagingMoves(t *testing.T) {
	quietTerm(t)
	tests := []struct {
		name         string
		content      string
		w, h, anchor int
		right        int // KeyRight presses before the first page key
		keys         string
		want         string
	}{
		{"plain-h12-end", plainLines(120), 60, 12, 110, 0, "DDDDDUUU",
			`h=10 start (110,0)
  D line=119 col=0 ro=0 top="- line 110"
  D line=120 col=0 ro=0 top="- line 111"
  D line=120 col=0 ro=0 top="- line 111"
  D line=120 col=0 ro=0 top="- line 111"
  D line=120 col=0 ro=0 top="- line 111"
  U line=111 col=0 ro=0 top="- line 111"
  U line=102 col=0 ro=0 top="- line 102"
  U line=93 col=0 ro=0 top="- line 93"
`},
		{"mixed-col", mixedLines(90), 40, 14, 0, 12, "DDDDDDDDDUUUUUUUUUUDDUU",
			`h=12 start (0,12)
  D line=7 col=12 ro=0 top="- 0 a fairly long bullet that wraps"
  D line=14 col=0 ro=0 top="- 7 medium length line of words"
  D line=21 col=6 ro=0 top=""
  D line=28 col=81 ro=2 top="- 21 s"
  D line=36 col=47 ro=1 top="editor is narrow enough ok"
  D line=44 col=12 ro=0 top="over several visual rows when the"
  D line=51 col=12 ro=0 top="- 44 a fairly long bullet that wraps"
  D line=58 col=0 ro=0 top="- 51 medium length line of words"
  D line=65 col=6 ro=0 top=""
  U line=58 col=0 ro=0 top=""
  U line=51 col=12 ro=0 top="- 51 medium length line of words"
  U line=44 col=12 ro=0 top="- 44 a fairly long bullet that wraps"
  U line=36 col=47 ro=1 top="over several visual rows when the"
  U line=28 col=81 ro=2 top="editor is narrow enough ok"
  U line=21 col=6 ro=0 top="- 21 s"
  U line=14 col=0 ro=0 top=""
  U line=7 col=12 ro=0 top="- 7 medium length line of words"
  U line=0 col=12 ro=0 top=""
  U line=0 col=12 ro=0 top=""
  D line=7 col=12 ro=0 top="- 0 a fairly long bullet that wraps"
  D line=14 col=0 ro=0 top="- 7 medium length line of words"
  U line=7 col=12 ro=0 top="- 7 medium length line of words"
  U line=0 col=12 ro=0 top=""
`},
		{"mixed-col2", mixedLines(90), 40, 11, 40, 20, "DDDUUUUDDDDDDDDDDDDDDDDDDDDDDD",
			`h=9 start (40,20)
  D line=44 col=89 ro=2 top="- 40 a fairly long bullet that wraps"
  D line=50 col=0 ro=0 top="editor is narrow enough ok"
  D line=56 col=20 ro=0 top=""
  U line=50 col=0 ro=0 top=""
  U line=44 col=89 ro=2 top="editor is narrow enough ok"
  U line=40 col=20 ro=0 top="- 40 a fairly long bullet that wraps"
  U line=34 col=0 ro=0 top=""
  D line=40 col=20 ro=0 top=""
  D line=44 col=89 ro=2 top="- 40 a fairly long bullet that wraps"
  D line=50 col=0 ro=0 top="editor is narrow enough ok"
  D line=56 col=20 ro=0 top=""
  D line=60 col=89 ro=2 top="- 56 a fairly long bullet that wraps"
  D line=66 col=0 ro=0 top="editor is narrow enough ok"
  D line=72 col=20 ro=0 top=""
  D line=76 col=89 ro=2 top="- 72 a fairly long bullet that wraps"
  D line=82 col=0 ro=0 top="editor is narrow enough ok"
  D line=88 col=20 ro=0 top=""
  D line=90 col=0 ro=0 top="editor is narrow enough ok"
  D line=90 col=0 ro=0 top="editor is narrow enough ok"
  D line=90 col=0 ro=0 top="editor is narrow enough ok"
  D line=90 col=0 ro=0 top="editor is narrow enough ok"
  D line=90 col=0 ro=0 top="editor is narrow enough ok"
  D line=90 col=0 ro=0 top="editor is narrow enough ok"
  D line=90 col=0 ro=0 top="editor is narrow enough ok"
  D line=90 col=0 ro=0 top="editor is narrow enough ok"
  D line=90 col=0 ro=0 top="editor is narrow enough ok"
  D line=90 col=0 ro=0 top="editor is narrow enough ok"
  D line=90 col=0 ro=0 top="editor is narrow enough ok"
  D line=90 col=0 ro=0 top="editor is narrow enough ok"
  D line=90 col=0 ro=0 top="editor is narrow enough ok"
`},
		{"wrapped-w30", wrappedLines(40), 30, 12, 0, 7, "DDDDDDDDDDDDDDUUUUUUUUUUUUUUUUUU",
			`h=10 start (0,7)
  D line=1 col=54 ro=2 top="- 0 word word word word"
  D line=2 col=79 ro=3 top="word word word word word"
  D line=3 col=54 ro=2 top="word word word word word"
  D line=3 col=279 ro=11 top="word word word word word"
  D line=4 col=204 ro=8 top="word word word"
  D line=5 col=79 ro=3 top="word word word word word"
  D line=5 col=304 ro=12 top="word word word word word"
  D line=6 col=129 ro=5 top="word word word word word"
  D line=6 col=354 ro=14 top="word word word word word"
  D line=7 col=154 ro=6 top="word word word word word"
  D line=9 col=7 ro=0 top="word"
  D line=9 col=229 ro=9 top="- 9 word word word word"
  D line=10 col=205 ro=8 top="word word word word"
  D line=11 col=130 ro=5 top="word word word word word"
  U line=10 col=205 ro=8 top="word word word word word"
  U line=9 col=229 ro=9 top="word word word word"
  U line=9 col=7 ro=0 top="- 9 word word word word"
  U line=7 col=154 ro=6 top="word"
  U line=6 col=354 ro=14 top="word word word word word"
  U line=6 col=129 ro=5 top="word word word word word"
  U line=5 col=304 ro=12 top="word word word word word"
  U line=5 col=79 ro=3 top="word word word word word"
  U line=4 col=204 ro=8 top="word word word word word"
  U line=3 col=279 ro=11 top="word word word"
  U line=3 col=54 ro=2 top="word word word word word"
  U line=2 col=79 ro=3 top="word word word word word"
  U line=1 col=54 ro=2 top="word word word word word"
  U line=0 col=7 ro=0 top=""
  U line=0 col=7 ro=0 top=""
  U line=0 col=7 ro=0 top=""
  U line=0 col=7 ro=0 top=""
  U line=0 col=7 ro=0 top=""
`},
		{"wrapped-end", wrappedLines(40), 30, 12, 38, 4, "DDDUUUDDD",
			`h=10 start (38,4)
  D line=38 col=227 ro=9 top="- 38 word word word word"
  D line=39 col=152 ro=6 top="word word word word word"
  D line=40 col=0 ro=0 top="word word word word word"
  U line=39 col=127 ro=5 top="word word word word word"
  U line=38 col=202 ro=8 top="word word word word word"
  U line=37 col=227 ro=9 top="word word word word"
  D line=38 col=202 ro=8 top="word word word word"
  D line=39 col=127 ro=5 top="word word word word word"
  D line=40 col=0 ro=0 top="word word word word word"
`},
		{"tiny-h5", plainLines(120), 60, 5, 0, 0, "DDDUUU",
			`h=3 start (0,0)
  D line=2 col=0 ro=0 top="- line 0"
  D line=4 col=0 ro=0 top="- line 2"
  D line=6 col=0 ro=0 top="- line 4"
  U line=4 col=0 ro=0 top="- line 4"
  U line=2 col=0 ro=0 top="- line 2"
  U line=0 col=0 ro=0 top=""
`},
		{"short-buf", "- a\n- b\n- c\n", 60, 12, 0, 0, "DDUUDU",
			`h=10 start (0,0)
  D line=3 col=0 ro=0 top=""
  D line=3 col=0 ro=0 top=""
  U line=0 col=0 ro=0 top=""
  U line=0 col=0 ro=0 top=""
  D line=3 col=0 ro=0 top=""
  U line=0 col=0 ro=0 top=""
`},
		{"empty", "", 60, 12, 0, 0, "DUDU",
			`h=10 start (0,0)
  D line=0 col=0 ro=0 top=""
  U line=0 col=0 ro=0 top=""
  D line=0 col=0 ro=0 top=""
  U line=0 col=0 ro=0 top=""
`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e := editorAt(nil, "P", "/tmp/p.md", tc.content, false, tc.w, tc.h-1, tc.anchor) // -1: the margin row is part of the window now, so h matches v1
			_ = e.View()
			for range tc.right {
				e.Update(tea.KeyPressMsg{Code: tea.KeyRight})
			}
			m := newPagingModel(e)
			h := e.textHeight()
			curIndex := func() (int, int) {
				p, x := e.cursorRow()
				return m.index(p), x
			}
			topIndex := func() int { return m.index(e.top) }
			_, goal := curIndex()

			var got strings.Builder
			fmt.Fprintf(&got, "h=%d start (%d,%d)\n", h, cursorPos(e).Line, func() int { _, c := cursorRowCol(e); return c }())
			for _, k := range tc.keys {
				code, dir := tea.KeyPgUp, -1
				if k == 'D' {
					code, dir = tea.KeyPgDown, +1
				}
				before, _ := curIndex()
				topBefore := topIndex()
				e.Update(tea.KeyPressMsg{Code: code})
				after, x := curIndex()
				top := topIndex()

				want := clampInt(before+dir*(h-1), 1, len(m.rows)-1) // row 0 is the margin
				if after != want {
					t.Fatalf("%c: cursor on display row %d, want %d (from %d, page %d)", k, after, want, before, h-1)
				}
				p := m.rows[after]
				c := e.line(p.line)
				maxOff := e.geo.OffsetAt(c.text, c.info, c.rows, p.row, 1<<30)
				if wantX := min(goal, m.x(p, maxOff)); x != wantX {
					t.Fatalf("%c: cursor at screen column %d, want min(goal %d, row end) = %d", k, x, goal, wantX)
				}
				if after < top || after > top+h-1 {
					t.Fatalf("%c: cursor row %d outside the window [%d,%d]", k, after, top, top+h-1)
				}
				if before >= topBefore && before <= topBefore+h-1 && after >= topBefore && after <= topBefore+h-1 && top != topBefore {
					t.Fatalf("%c: window moved %d -> %d though the cursor stayed on screen", k, topBefore, top)
				}
				if top != topBefore && top != after && top != after-(h-1) && top != 0 {
					t.Fatalf("%c: window top %d is not minimal for the cursor at %d", k, top, after)
				}
				_, col := cursorRowCol(e)
				ro := p.row
				rows := strings.Split(plain(e.View()), "\n")
				fmt.Fprintf(&got, "  %c line=%d col=%d ro=%d top=%q\n", k, cursorPos(e).Line, col, ro, strings.TrimSpace(rows[0]))
			}
			if got.String() != tc.want {
				t.Errorf("paging trace differs\n got:\n%s\nwant:\n%s", got.String(), tc.want)
			}
		})
	}
}
