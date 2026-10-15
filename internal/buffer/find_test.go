package buffer

import (
	"strings"
	"testing"
)

func TestFindSmartCase(t *testing.T) {
	b := New("alpha Alpha ALPHA")
	for q, want := range map[string]int{"alpha": 3, "Alpha": 1, "ALPHA": 1} {
		if got := len(b.Find(q)); got != want {
			t.Errorf("Find(%q) = %d matches, want %d", q, got, want)
		}
	}
}

func TestFindUnicodeFold(t *testing.T) {
	b := New("Straße STRASSE")
	rs := b.Find("straße")
	if len(rs) != 1 || rs[0] != (Range{Pos{0, 0}, Pos{0, len("Straße")}}) {
		t.Fatalf("straße: %v", rs)
	}
	b = New("Ölfeld öl")
	rs = b.Find("öl")
	if len(rs) != 2 {
		t.Fatalf("öl: %v", rs)
	}
	if rs[0].Start.Col != 0 || rs[0].End.Col != len("Öl") {
		t.Errorf("first match bytes: %v", rs[0])
	}
	if rs[1].Start.Col != len("Ölfeld ") || rs[1].End.Col != len("Ölfeld öl") {
		t.Errorf("second match bytes: %v", rs[1])
	}
}

func TestFindNoMatch(t *testing.T) {
	if rs := New("hello").Find("xyz"); len(rs) != 0 {
		t.Fatalf("got %v", rs)
	}
}

func TestFindEmptyQuery(t *testing.T) {
	if rs := New("hello").Find(""); rs != nil {
		t.Fatalf("got %v", rs)
	}
	if rs := New("a\nb").Find("a\nb"); rs != nil {
		t.Fatalf("newline query: %v", rs)
	}
}

func TestFindNeverCrossesLines(t *testing.T) {
	b := New("ab\ncd")
	if rs := b.Find("b\nc"); rs != nil {
		t.Errorf("b\\nc: %v", rs)
	}
	if rs := b.Find("bc"); rs != nil {
		t.Errorf("bc: %v", rs)
	}
}

func TestFindNonOverlapping(t *testing.T) {
	rs := New("aaaa").Find("aa")
	if len(rs) != 2 || rs[1].Start.Col != 2 {
		t.Fatalf("got %v", rs)
	}
	rs = New("AAAA").Find("aa")
	if len(rs) != 2 {
		t.Fatalf("folded: %v", rs)
	}
}

func TestFindCRLFOffsetsExcludeCR(t *testing.T) {
	b := New("foo\r\nbar foo\r\n")
	rs := b.Find("foo")
	want := []Range{{Pos{0, 0}, Pos{0, 3}}, {Pos{1, 4}, Pos{1, 7}}}
	if len(rs) != 2 || rs[0] != want[0] || rs[1] != want[1] {
		t.Fatalf("got %v want %v", rs, want)
	}
}

func TestFindDocumentOrder(t *testing.T) {
	b := New("x\nx x\nx")
	rs := b.Find("x")
	if len(rs) != 4 {
		t.Fatalf("got %v", rs)
	}
	for i := 1; i < len(rs); i++ {
		if !rs[i-1].Start.Less(rs[i].Start) {
			t.Fatalf("not in order: %v", rs)
		}
	}
}

func TestReplaceRangesOneUndo(t *testing.T) {
	for _, repl := range []string{"LONGER", "x", ""} {
		const src = "foo a\nb foo foo\nc"
		b := New(src)
		rs := b.Find("foo")
		if n := b.ReplaceRanges(rs, repl); n != 3 {
			t.Fatalf("repl %q: count %d", repl, n)
		}
		want := strings.ReplaceAll(src, "foo", repl)
		if b.String() != want {
			t.Fatalf("repl %q: got %q want %q", repl, b.String(), want)
		}
		if !b.Undo() || b.String() != src {
			t.Fatalf("repl %q: undo got %q", repl, b.String())
		}
		if b.Undo() {
			t.Fatalf("repl %q: second undo should be empty", repl)
		}
		if !b.Redo() || b.String() != want {
			t.Fatalf("repl %q: redo got %q", repl, b.String())
		}
	}
}

func TestReplaceRangesCursorAfterLast(t *testing.T) {
	b := New("foo foo\nfoo")
	b.ReplaceRanges(b.Find("foo"), "ab")
	if b.Cursor() != (Pos{1, 2}) {
		t.Fatalf("cursor %v", b.Cursor())
	}
	b = New("foo foo")
	b.ReplaceRanges(b.Find("foo"), "longer")
	if b.Cursor() != (Pos{0, len("longer longer")}) {
		t.Fatalf("same-line cursor %v", b.Cursor())
	}
	b = New("foo foo")
	b.ReplaceRanges(b.Find("foo"), "a\nb")
	if b.Cursor() != (Pos{2, 1}) || b.String() != "a\nb a\nb" {
		t.Fatalf("multiline repl: cursor %v text %q", b.Cursor(), b.String())
	}
}

func TestReplaceRangesNone(t *testing.T) {
	b := New("abc")
	if n := b.ReplaceRanges(nil, "x"); n != 0 {
		t.Fatalf("count %d", n)
	}
	if b.Undo() {
		t.Fatal("no group expected")
	}
}

func TestFindLarge(t *testing.T) {
	var sb strings.Builder
	for i := 0; i < 10000; i++ {
		sb.WriteString("line needle number\n")
	}
	b := New(sb.String())
	if n := len(b.Find("needle")); n != 10000 {
		t.Fatalf("got %d", n)
	}
}

// Find's allocations are the result slice's growth, not a function of the line
// count: no per-line query conversion or result slice.
func TestFindAllocationsDoNotGrowWithLines(t *testing.T) {
	doc := func(lines int) *Buffer {
		var sb strings.Builder
		for i := range lines {
			if i%10 == 0 {
				sb.WriteString("a needle in a line of words\n")
			} else {
				sb.WriteString("some other words here\n")
			}
		}
		return New(sb.String())
	}
	for _, query := range []string{"needle", "Needle", "neédle"} {
		small, large := doc(1000), doc(10000)
		a := testing.AllocsPerRun(5, func() { small.Find(query) })
		b := testing.AllocsPerRun(5, func() { large.Find(query) })
		// Ten times the lines and matches: only the result slice's doubling adds.
		if b > a+8 {
			t.Errorf("Find(%q): %.0f allocs for 1000 lines, %.0f for 10000; want it independent of the line count", query, a, b)
		}
		if b > 40 {
			t.Errorf("Find(%q): %.0f allocs for 10000 lines, want a handful", query, b)
		}
	}
}

func TestSearch(t *testing.T) {
	type res struct {
		r       Range
		wrapped bool
		ok      bool
	}
	at := func(l, c, n int) Range { return Range{Pos{l, c}, Pos{l, c + n}} }
	for _, tc := range []struct {
		name    string
		content string
		query   string
		from    Pos
		back    bool
		want    res
	}{
		{"forward mid-line", "ab ab ab", "ab", Pos{0, 1}, false, res{at(0, 3, 2), false, true}},
		{"forward skips match at from", "ab ab ab", "ab", Pos{0, 3}, false, res{at(0, 6, 2), false, true}},
		{"forward next line", "ab\nxx\nab", "ab", Pos{0, 0}, false, res{at(2, 0, 2), false, true}},
		{"forward wraps", "ab xx\nxx ab", "ab", Pos{1, 3}, false, res{at(0, 0, 2), true, true}},
		{"forward wraps onto from itself", "xx ab", "ab", Pos{0, 3}, false, res{at(0, 3, 2), true, true}},
		{"back mid-line", "ab ab ab", "ab", Pos{0, 7}, true, res{at(0, 6, 2), false, true}},
		{"back skips match at from", "ab ab ab", "ab", Pos{0, 6}, true, res{at(0, 3, 2), false, true}},
		{"back previous line", "ab\nxx\nab", "ab", Pos{2, 0}, true, res{at(0, 0, 2), false, true}},
		{"back last match of a line", "ab ab\nxx", "ab", Pos{1, 1}, true, res{at(0, 3, 2), false, true}},
		{"back wraps", "ab xx\nxx ab", "ab", Pos{0, 0}, true, res{at(1, 3, 2), true, true}},
		{"back wraps onto from itself", "ab xx", "ab", Pos{0, 0}, true, res{at(0, 0, 2), true, true}},
		{"smart case folds", "xx Beta", "beta", Pos{0, 0}, false, res{at(0, 3, 4), false, true}},
		{"smart case exact", "beta Beta", "Beta", Pos{0, 0}, false, res{at(0, 5, 4), false, true}},
		{"folded multi-byte", "x éa\nÉa", "Éa", Pos{0, 0}, false, res{at(1, 0, len("Éa")), false, true}},
		{"folded multi-byte lower", "xx Éa", "éa", Pos{0, 0}, false, res{at(0, 3, len("Éa")), false, true}},
		{"grapheme start only", "e\u0301a", "\u0301", Pos{0, 0}, false, res{}},
		{"crlf", "one\r\ntwo\r\none", "one", Pos{0, 0}, false, res{at(2, 0, 3), false, true}},
		{"crlf back", "one\r\ntwo\r\none", "one", Pos{2, 0}, true, res{at(0, 0, 3), false, true}},
		{"no match", "hello", "zzz", Pos{0, 0}, false, res{}},
		{"no match back", "hello", "zzz", Pos{0, 2}, true, res{}},
		{"empty query", "hello", "", Pos{0, 0}, false, res{}},
		{"newline query", "a\nb", "a\nb", Pos{0, 0}, false, res{}},
		{"one line only match", "xx ab xx", "ab", Pos{0, 0}, false, res{at(0, 3, 2), false, true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r, wrapped, ok := New(tc.content).Search(tc.query, tc.from, tc.back)
			if got := (res{r, wrapped, ok}); got != tc.want {
				t.Fatalf("Search(%q, %v, back=%v) = %+v, want %+v", tc.query, tc.from, tc.back, got, tc.want)
			}
		})
	}
}
