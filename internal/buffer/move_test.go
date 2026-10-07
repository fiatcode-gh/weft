package buffer

import "testing"

func TestGraphemeMoves(t *testing.T) {
	cases := []struct {
		name string
		text string
		stop []int // byte offsets of grapheme boundaries
	}{
		{"ascii", "abc", []int{0, 1, 2, 3}},
		{"combining", "e\u0301x", []int{0, 3, 4}},
		{"emoji zwj", "a👨‍👩‍👧b", []int{0, 1, len("a👨‍👩‍👧"), len("a👨‍👩‍👧b")}},
		{"cjk", "日本", []int{0, 3, 6}},
		{"invalid byte", "a\xffb", []int{0, 1, 2, 3}},
		{"invalid before combining", "\xff\u0301", []int{0, 1, 3}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			b := New(c.text)
			p := Pos{0, 0}
			for i := 1; i < len(c.stop); i++ {
				p = b.Right(p)
				if p.Col != c.stop[i] {
					t.Fatalf("Right #%d = %d, want %d", i, p.Col, c.stop[i])
				}
			}
			for i := len(c.stop) - 2; i >= 0; i-- {
				p = b.Left(p)
				if p.Col != c.stop[i] {
					t.Fatalf("Left #%d = %d, want %d", i, p.Col, c.stop[i])
				}
			}
		})
	}
}

func TestLeftRightCrossLines(t *testing.T) {
	b := New("ab\r\ncd")
	if got := b.Right(Pos{0, 2}); got != (Pos{1, 0}) {
		t.Errorf("Right at line end = %v", got)
	}
	if got := b.Left(Pos{1, 0}); got != (Pos{0, 2}) {
		t.Errorf("Left at line start = %v", got)
	}
	if got := b.Left(Pos{0, 0}); got != (Pos{0, 0}) {
		t.Errorf("Left at buffer start = %v", got)
	}
	if got := b.Right(b.End()); got != b.End() {
		t.Errorf("Right at buffer end = %v", got)
	}
}

func TestBackspaceDeletesWholeGrapheme(t *testing.T) {
	b := New("xe\u0301")
	at(b, 0, len(b.Line(0)))
	b.Backspace()
	if b.String() != "x" {
		t.Fatalf("got %q", b.String())
	}
	c := New("e\u0301x")
	at(c, 0, 0)
	c.Delete()
	if c.String() != "x" {
		t.Fatalf("got %q", c.String())
	}
}

func TestWordMoves(t *testing.T) {
	b := New("foo  bar\n  baz qux\n")
	right := []Pos{{0, 3}, {0, 8}, {1, 0}, {1, 5}, {1, 9}, {2, 0}}
	p := Pos{0, 0}
	for i, want := range right {
		p = b.WordRight(p)
		if p != want {
			t.Fatalf("WordRight #%d = %v, want %v", i, p, want)
		}
	}
	if got := b.WordRight(p); got != p {
		t.Fatalf("WordRight at end = %v", got)
	}
	left := []Pos{{1, 9}, {1, 6}, {1, 2}, {1, 0}, {0, 8}, {0, 5}, {0, 0}}
	p = Pos{2, 0}
	// from (2,0) at column 0: previous line's end.
	p = b.WordLeft(p)
	if p != (Pos{1, 9}) {
		t.Fatalf("WordLeft at col 0 = %v", p)
	}
	for i, want := range left[1:] {
		p = b.WordLeft(p)
		if p != want {
			t.Fatalf("WordLeft #%d = %v, want %v", i, p, want)
		}
	}
}

func TestWordRightAtLineEndGoesToNextLineStart(t *testing.T) {
	b := New("ab\ncd")
	if got := b.WordRight(Pos{0, 2}); got != (Pos{1, 0}) {
		t.Fatalf("got %v", got)
	}
}

func TestLineStartEnd(t *testing.T) {
	b := New("héllo\nx")
	if got := b.LineStart(Pos{0, 3}); got != (Pos{0, 0}) {
		t.Errorf("LineStart = %v", got)
	}
	if got := b.LineEnd(Pos{0, 1}); got != (Pos{0, len("héllo")}) {
		t.Errorf("LineEnd = %v", got)
	}
}

func TestSelection(t *testing.T) {
	b := New("one\r\ntwo\nthree")
	if _, ok := b.Selection(); ok {
		t.Fatal("fresh buffer has no selection")
	}
	b.MoveTo(Pos{0, 1}, false)
	b.MoveTo(Pos{0, 1}, true)
	if _, ok := b.Selection(); ok {
		t.Fatal("anchor equal to cursor is not a selection")
	}
	b.MoveTo(Pos{2, 2}, true)
	r, ok := b.Selection()
	if !ok || r != (Range{Pos{0, 1}, Pos{2, 2}}) {
		t.Fatalf("selection %v %v", r, ok)
	}
	if got := b.Text(r); got != "ne\ntwo\nth" {
		t.Fatalf("Text = %q", got)
	}
	// Backward selection is ordered.
	b.MoveTo(Pos{2, 2}, false)
	b.MoveTo(Pos{0, 1}, true)
	if r, ok = b.Selection(); !ok || r != (Range{Pos{0, 1}, Pos{2, 2}}) {
		t.Fatalf("backward selection %v %v", r, ok)
	}
	b.MoveTo(Pos{1, 0}, false)
	if _, ok := b.Selection(); ok {
		t.Fatal("non-extending move must clear the selection")
	}
}

func TestSelectAll(t *testing.T) {
	e := New("")
	e.SelectAll()
	if _, ok := e.Selection(); ok {
		t.Fatal("SelectAll on an empty buffer selects nothing")
	}
	b := New("ab\ncd")
	b.SelectAll()
	r, ok := b.Selection()
	if !ok || r != (Range{Pos{0, 0}, Pos{1, 2}}) || b.Cursor() != (Pos{1, 2}) {
		t.Fatalf("selection %v %v cursor %v", r, ok, b.Cursor())
	}
	if b.Text(r) != "ab\ncd" {
		t.Fatalf("Text = %q", b.Text(r))
	}
}

func TestMoveToClamps(t *testing.T) {
	b := New("ab\nc")
	b.MoveTo(Pos{9, 9}, false)
	if b.Cursor() != (Pos{1, 1}) {
		t.Fatalf("cursor %v", b.Cursor())
	}
	b.MoveTo(Pos{-1, -4}, false)
	if b.Cursor() != (Pos{0, 0}) {
		t.Fatalf("cursor %v", b.Cursor())
	}
	b.MoveTo(Pos{0, 99}, false)
	if b.Cursor() != (Pos{0, 2}) {
		t.Fatalf("cursor %v", b.Cursor())
	}
}

func TestPosLess(t *testing.T) {
	if !(Pos{0, 5}).Less(Pos{1, 0}) || (Pos{1, 0}).Less(Pos{0, 5}) || (Pos{1, 1}).Less(Pos{1, 1}) || !(Pos{1, 0}).Less(Pos{1, 1}) {
		t.Fatal("Pos.Less ordering wrong")
	}
}
