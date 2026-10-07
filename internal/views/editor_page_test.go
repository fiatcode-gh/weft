package views

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
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

// TestEditorPagingMovesLikeV1 pins PgUp/PgDn to what Bubbles v1 did: h-1
// visual rows per page, the column remembered across rows of other lengths,
// and the viewport moved only as far as the cursor needs. The expected traces
// were recorded from the v1 build (19c11c3) of the same scripts. Each step is
// the cursor's line, absolute column and row inside a wrapped line, and the
// window's top row, after one page key (D = PgDn, U = PgUp).
func TestEditorPagingMovesLikeV1(t *testing.T) {
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
  D line=28 col=83 ro=2 top="- 21 s"
  D line=36 col=49 ro=1 top="editor is narrow enough ok"
  D line=44 col=12 ro=0 top="over several visual rows when the"
  D line=51 col=12 ro=0 top="- 44 a fairly long bullet that wraps"
  D line=58 col=0 ro=0 top="- 51 medium length line of words"
  D line=65 col=6 ro=0 top=""
  U line=58 col=0 ro=0 top=""
  U line=51 col=12 ro=0 top="- 51 medium length line of words"
  U line=44 col=12 ro=0 top="- 44 a fairly long bullet that wraps"
  U line=36 col=49 ro=1 top="over several visual rows when the"
  U line=28 col=83 ro=2 top="editor is narrow enough ok"
  U line=21 col=6 ro=0 top="- 21 s"
  U line=14 col=0 ro=0 top=""
  U line=7 col=12 ro=0 top="- 7 medium length line of words"
  U line=0 col=12 ro=0 top="- 0 a fairly long bullet that wraps"
  U line=0 col=12 ro=0 top="- 0 a fairly long bullet that wraps"
  D line=7 col=12 ro=0 top="- 0 a fairly long bullet that wraps"
  D line=14 col=0 ro=0 top="- 7 medium length line of words"
  U line=7 col=12 ro=0 top="- 7 medium length line of words"
  U line=0 col=12 ro=0 top="- 0 a fairly long bullet that wraps"
`},
		{"mixed-col2", mixedLines(90), 40, 11, 40, 20, "DDDUUUUDDDDDDDDDDDDDDDDDDDDDDD",
			`h=9 start (40,20)
  D line=44 col=91 ro=2 top="- 40 a fairly long bullet that wraps"
  D line=50 col=0 ro=0 top="editor is narrow enough ok"
  D line=56 col=20 ro=0 top=""
  U line=50 col=0 ro=0 top=""
  U line=44 col=91 ro=2 top="editor is narrow enough ok"
  U line=40 col=20 ro=0 top="- 40 a fairly long bullet that wraps"
  U line=34 col=0 ro=0 top=""
  D line=40 col=20 ro=0 top=""
  D line=44 col=91 ro=2 top="- 40 a fairly long bullet that wraps"
  D line=50 col=0 ro=0 top="editor is narrow enough ok"
  D line=56 col=20 ro=0 top=""
  D line=60 col=91 ro=2 top="- 56 a fairly long bullet that wraps"
  D line=66 col=0 ro=0 top="editor is narrow enough ok"
  D line=72 col=20 ro=0 top=""
  D line=76 col=91 ro=2 top="- 72 a fairly long bullet that wraps"
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
  D line=1 col=56 ro=2 top="- 0 word word word word"
  D line=2 col=81 ro=3 top="word word word word word"
  D line=3 col=56 ro=2 top="word word word word word"
  D line=3 col=281 ro=11 top="word word word word word"
  D line=4 col=206 ro=8 top="word word word"
  D line=5 col=81 ro=3 top="word word word word word"
  D line=5 col=306 ro=12 top="word word word word word"
  D line=6 col=131 ro=5 top="word word word word word"
  D line=6 col=356 ro=14 top="word word word word word"
  D line=7 col=154 ro=6 top="word word word word word"
  D line=9 col=7 ro=0 top="word"
  D line=9 col=231 ro=9 top="- 9 word word word word"
  D line=10 col=207 ro=8 top="word word word word"
  D line=11 col=132 ro=5 top="word word word word word"
  U line=10 col=207 ro=8 top="word word word word word"
  U line=9 col=231 ro=9 top="word word word word"
  U line=9 col=7 ro=0 top="- 9 word word word word"
  U line=7 col=154 ro=6 top="word"
  U line=6 col=356 ro=14 top="word word word word word"
  U line=6 col=131 ro=5 top="word word word word word"
  U line=5 col=306 ro=12 top="word word word word word"
  U line=5 col=81 ro=3 top="word word word word word"
  U line=4 col=206 ro=8 top="word word word word word"
  U line=3 col=281 ro=11 top="word word word"
  U line=3 col=56 ro=2 top="word word word word word"
  U line=2 col=81 ro=3 top="word word word word word"
  U line=1 col=56 ro=2 top="word word word word word"
  U line=0 col=7 ro=0 top="- 0 word word word word"
  U line=0 col=7 ro=0 top="- 0 word word word word"
  U line=0 col=7 ro=0 top="- 0 word word word word"
  U line=0 col=7 ro=0 top="- 0 word word word word"
  U line=0 col=7 ro=0 top="- 0 word word word word"
`},
		{"wrapped-end", wrappedLines(40), 30, 12, 38, 4, "DDDUUUDDD",
			`h=10 start (38,4)
  D line=38 col=229 ro=9 top="- 38 word word word word"
  D line=39 col=154 ro=6 top="word word word word word"
  D line=40 col=0 ro=0 top="word word word word word"
  U line=39 col=129 ro=5 top="word word word word word"
  U line=38 col=204 ro=8 top="word word word word word"
  U line=37 col=229 ro=9 top="word word word word"
  D line=38 col=204 ro=8 top="word word word word"
  D line=39 col=129 ro=5 top="word word word word word"
  D line=40 col=0 ro=0 top="word word word word word"
`},
		{"tiny-h5", plainLines(120), 60, 5, 0, 0, "DDDUUU",
			`h=3 start (0,0)
  D line=2 col=0 ro=0 top="- line 0"
  D line=4 col=0 ro=0 top="- line 2"
  D line=6 col=0 ro=0 top="- line 4"
  U line=4 col=0 ro=0 top="- line 4"
  U line=2 col=0 ro=0 top="- line 2"
  U line=0 col=0 ro=0 top="- line 0"
`},
		{"short-buf", "- a\n- b\n- c\n", 60, 12, 0, 0, "DDUUDU",
			`h=10 start (0,0)
  D line=3 col=0 ro=0 top="- a"
  D line=3 col=0 ro=0 top="- a"
  U line=0 col=0 ro=0 top="- a"
  U line=0 col=0 ro=0 top="- a"
  D line=3 col=0 ro=0 top="- a"
  U line=0 col=0 ro=0 top="- a"
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
			e := NewEditorView(nil, "P", "/tmp/p.md", tc.content, false, tc.w, tc.h, tc.anchor)
			_ = e.View()
			for range tc.right {
				e.Update(tea.KeyPressMsg{Code: tea.KeyRight})
			}
			var got strings.Builder
			trace := func(prefix string) {
				_, col := e.cursorRowCol()
				rows := strings.Split(plain(e.View()), "\n")
				fmt.Fprintf(&got, "%sline=%d col=%d ro=%d top=%q\n", prefix, e.ta.Line(), col, e.ta.LineInfo().RowOffset, strings.TrimSpace(rows[1]))
			}
			fmt.Fprintf(&got, "h=%d start (%d,%d)\n", e.ta.Height(), e.ta.Line(), func() int { _, c := e.cursorRowCol(); return c }())
			for _, k := range tc.keys {
				code := tea.KeyPgUp
				if k == 'D' {
					code = tea.KeyPgDown
				}
				e.Update(tea.KeyPressMsg{Code: code})
				trace(fmt.Sprintf("  %c ", k))
			}
			if got.String() != tc.want {
				t.Errorf("paging trace differs from v1\n got:\n%s\nwant:\n%s", got.String(), tc.want)
			}
		})
	}
}

// TestEditorPagingIsLinear: Bubbles v2 renders the whole buffer on every
// textarea Update, so paging by feeding the textarea h-1 key messages cost
// about 5 s per page at 10000 lines (v1: 4 ms). Only scrollPage is timed: the
// per-key layout() render around it is the typing cost the CHANGELOG lists.
func TestEditorPagingIsLinear(t *testing.T) {
	quietTerm(t)
	content := strings.Repeat("- some line of text\n", 10000)
	e := NewEditorView(nil, "P", "/tmp/p.md", content, false, 80, 45, 0)

	t0 := time.Now()
	e.scrollPage(+1)
	page := time.Since(t0)

	if row, _ := e.cursorRowCol(); row != e.ta.Height()-1 {
		t.Fatalf("PgDn left the cursor on row %d, want %d", row, e.ta.Height()-1)
	}
	const bound = time.Second
	if page > bound {
		t.Fatalf("one page down took %v at 10000 lines (bound %v)", page, bound)
	}
}
