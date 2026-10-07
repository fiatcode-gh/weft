package buffer

import (
	"strings"
	"testing"
)

func matchTexts(b *Buffer, rs []Range) []string {
	var out []string
	for _, r := range rs {
		out = append(out, b.Text(r))
	}
	return out
}

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
