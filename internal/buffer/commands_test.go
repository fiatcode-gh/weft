package buffer

import "testing"

// oneUndo asserts a single Undo restores want exactly and that nothing is left to undo.
func oneUndo(t *testing.T, b *Buffer, want string) {
	t.Helper()
	if !b.Undo() {
		t.Fatal("Undo returned false")
	}
	if got := b.String(); got != want {
		t.Fatalf("after one Undo got %q, want %q", got, want)
	}
	if b.Undo() {
		t.Fatal("expected exactly one undo group")
	}
}

func TestDeleteWordBackward(t *testing.T) {
	cases := []struct {
		name     string
		content  string
		at       Pos
		want     string
		wantCur  Pos
		selected *Range
	}{
		{"word", "alpha beta", Pos{0, 10}, "alpha ", Pos{0, 6}, nil},
		{"trailing spaces", "alpha beta  ", Pos{0, 12}, "alpha ", Pos{0, 6}, nil},
		{"col 0 deletes break", "ab\ncd", Pos{1, 0}, "abcd", Pos{0, 2}, nil},
		{"start of buffer no-op", "ab", Pos{0, 0}, "ab", Pos{0, 0}, nil},
		{"selection wins", "alpha beta", Pos{0, 10}, "alphaeta", Pos{0, 5}, &Range{Pos{0, 5}, Pos{0, 7}}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			b := New(c.content)
			at(b, c.at.Line, c.at.Col)
			if c.selected != nil {
				b.MoveTo(c.selected.Start, false)
				b.MoveTo(c.selected.End, true)
			}
			b.DeleteWordBackward()
			if b.String() != c.want || b.Cursor() != c.wantCur {
				t.Fatalf("got %q cur %v, want %q cur %v", b.String(), b.Cursor(), c.want, c.wantCur)
			}
			if c.want != c.content {
				oneUndo(t, b, c.content)
			}
		})
	}
}

func TestDeleteWordForward(t *testing.T) {
	cases := []struct {
		name    string
		content string
		at      Pos
		want    string
	}{
		{"word", "alpha beta", Pos{0, 0}, " beta"},
		{"leading spaces", "alpha  beta", Pos{0, 5}, "alpha"},
		{"line end deletes break", "ab\ncd", Pos{0, 2}, "abcd"},
		{"end of buffer no-op", "ab", Pos{0, 2}, "ab"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			b := New(c.content)
			at(b, c.at.Line, c.at.Col)
			b.DeleteWordForward()
			if b.String() != c.want || b.Cursor() != c.at {
				t.Fatalf("got %q cur %v, want %q cur %v", b.String(), b.Cursor(), c.want, c.at)
			}
			if c.want != c.content {
				oneUndo(t, b, c.content)
			}
		})
	}
}

func TestDeleteWordForwardSelection(t *testing.T) {
	b := New("alpha beta")
	b.MoveTo(Pos{0, 1}, false)
	b.MoveTo(Pos{0, 3}, true)
	b.DeleteWordForward()
	if b.String() != "aha beta" {
		t.Fatalf("got %q", b.String())
	}
}

func TestDeleteToLineEnd(t *testing.T) {
	b := New("alpha beta\nnext")
	at(b, 0, 5)
	b.DeleteToLineEnd()
	if b.String() != "alpha\nnext" {
		t.Fatalf("got %q", b.String())
	}
	b.DeleteToLineEnd() // at line end: joins
	if b.String() != "alphanext" || b.Cursor() != (Pos{0, 5}) {
		t.Fatalf("join: got %q cur %v", b.String(), b.Cursor())
	}
	b.Undo()
	b.Undo()
	if b.String() != "alpha beta\nnext" {
		t.Fatalf("undo: %q", b.String())
	}
}

func TestDeleteToLineEndAtBufferEnd(t *testing.T) {
	b := New("ab")
	at(b, 0, 2)
	b.DeleteToLineEnd()
	if b.String() != "ab" || b.Undo() {
		t.Fatal("expected no-op without group")
	}
}

func TestDeleteToLineStart(t *testing.T) {
	b := New("alpha beta")
	at(b, 0, 6)
	b.DeleteToLineStart()
	if b.String() != "beta" || b.Cursor() != (Pos{0, 0}) {
		t.Fatalf("got %q cur %v", b.String(), b.Cursor())
	}
	oneUndo(t, b, "alpha beta")

	b = New("a\nb")
	at(b, 1, 0)
	b.DeleteToLineStart()
	if b.String() != "a\nb" || b.Undo() {
		t.Fatal("col 0 must be a no-op without group")
	}
}

func TestCaseWord(t *testing.T) {
	cases := []struct {
		mode CaseMode
		want string
	}{
		{CaseCapitalize, "Alpha beta"},
		{CaseUpper, "ALPHA beta"},
		{CaseLower, "alpha beta"},
	}
	for _, c := range cases {
		b := New("aLPHA beta")
		b.CaseWord(c.mode)
		if b.String() != c.want || b.Cursor() != (Pos{0, 5}) {
			t.Errorf("mode %d: got %q cur %v, want %q cur 0,5", c.mode, b.String(), b.Cursor(), c.want)
		}
		oneUndo(t, b, "aLPHA beta")
	}
}

func TestCaseWordSkipsSpacesAndHandlesMultibyte(t *testing.T) {
	b := New("x   élan z")
	at(b, 0, 1)
	b.CaseWord(CaseUpper)
	if b.String() != "x   ÉLAN z" || b.Cursor() != (Pos{0, len("x   élan")}) {
		t.Fatalf("got %q cur %v", b.String(), b.Cursor())
	}
}

func TestCaseWordNothingChanged(t *testing.T) {
	b := New("ALPHA")
	b.CaseWord(CaseUpper)
	if b.Undo() {
		t.Fatal("unchanged text must not create a group")
	}
	if b.Cursor() != (Pos{0, 5}) {
		t.Fatalf("cursor %v", b.Cursor())
	}
}

func TestCopyCutSelectionCRLF(t *testing.T) {
	const src = "one\r\ntwo\r\nthree"
	b := New(src)
	b.MoveTo(Pos{0, 1}, false)
	b.MoveTo(Pos{2, 2}, true)
	text, linewise := b.Copy()
	if text != "ne\ntwo\nth" || linewise {
		t.Fatalf("Copy = %q,%v", text, linewise)
	}
	if b.String() != src {
		t.Fatal("Copy must not modify")
	}
	text, linewise = b.Cut()
	if text != "ne\ntwo\nth" || linewise || b.String() != "oree" {
		t.Fatalf("Cut = %q,%v buffer %q", text, linewise, b.String())
	}
	oneUndo(t, b, src)

	b.MoveTo(Pos{0, 3}, false)
	b.Paste("X\nY", false)
	if b.String() != "oneX\r\nY\r\ntwo\r\nthree" {
		t.Fatalf("paste got %q", b.String())
	}
	oneUndo(t, b, src)
}

func TestCopyLinewise(t *testing.T) {
	b := New("one\ntwo")
	at(b, 1, 1)
	text, linewise := b.Copy()
	if text != "two\n" || !linewise {
		t.Fatalf("Copy = %q,%v", text, linewise)
	}
}

func TestCutLinewise(t *testing.T) {
	cases := []struct {
		name    string
		content string
		at      Pos
		text    string
		want    string
		wantCur Pos
	}{
		{"middle", "a\nb\nc", Pos{1, 0}, "b\n", "a\nc", Pos{1, 0}},
		{"first", "a\nb", Pos{0, 1}, "a\n", "b", Pos{0, 1}},
		{"last without final newline removes preceding break", "a\nbc", Pos{1, 1}, "bc\n", "a", Pos{0, 1}},
		{"only line clears", "abc", Pos{0, 2}, "abc\n", "", Pos{0, 0}},
		{"crlf middle", "a\r\nb\r\nc", Pos{1, 0}, "b\n", "a\r\nc", Pos{1, 0}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			b := New(c.content)
			at(b, c.at.Line, c.at.Col)
			text, linewise := b.Cut()
			if text != c.text || !linewise || b.String() != c.want || b.Cursor() != c.wantCur {
				t.Fatalf("Cut = %q,%v buffer %q cur %v; want %q buffer %q cur %v", text, linewise, b.String(), b.Cursor(), c.text, c.want, c.wantCur)
			}
			oneUndo(t, b, c.content)
		})
	}
}

func TestPasteLinewiseLandsAboveCursorLine(t *testing.T) {
	b := New("a\nbcd\ne")
	at(b, 1, 2)
	b.Paste("X\n", true)
	if b.String() != "a\nX\nbcd\ne" || b.Cursor() != (Pos{2, 2}) {
		t.Fatalf("got %q cur %v", b.String(), b.Cursor())
	}
	oneUndo(t, b, "a\nbcd\ne")
}

func TestPasteLinewiseOnLastLineUsesEOL(t *testing.T) {
	b := New("a\r\nb\r\nc")
	at(b, 2, 1)
	b.Paste("X\n", true)
	if b.String() != "a\r\nb\r\nX\r\nc" || b.Cursor() != (Pos{3, 1}) {
		t.Fatalf("got %q cur %v", b.String(), b.Cursor())
	}
}

func TestPasteReplacesSelection(t *testing.T) {
	b := New("hello world")
	b.MoveTo(Pos{0, 0}, false)
	b.MoveTo(Pos{0, 5}, true)
	b.Paste("bye", true) // a selection wins over linewise
	if b.String() != "bye world" || b.Cursor() != (Pos{0, 3}) {
		t.Fatalf("got %q cur %v", b.String(), b.Cursor())
	}
	oneUndo(t, b, "hello world")
}

func TestPasteCharacterwise(t *testing.T) {
	b := New("ac")
	at(b, 0, 1)
	b.Paste("b", false)
	if b.String() != "abc" || b.Cursor() != (Pos{0, 2}) {
		t.Fatalf("got %q cur %v", b.String(), b.Cursor())
	}
	oneUndo(t, b, "ac")
}
