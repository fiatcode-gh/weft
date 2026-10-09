package buffer

import "testing"

func TestBulletPrefix(t *testing.T) {
	cases := []struct {
		line       string
		wantPrefix string
		wantOK     bool
	}{
		{"- foo", "- ", true},
		{"  - nested", "  - ", true},
		{"- TODO task", "- ", true},
		{"# heading", "", false},
		{"plain text", "", false},
		{"-no space", "", false},
	}
	for _, c := range cases {
		gotPrefix, gotOK := bulletPrefix(c.line)
		if gotPrefix != c.wantPrefix || gotOK != c.wantOK {
			t.Errorf("bulletPrefix(%q) = (%q,%v), want (%q,%v)", c.line, gotPrefix, gotOK, c.wantPrefix, c.wantOK)
		}
	}
}

func TestIsEmptyBullet(t *testing.T) {
	cases := []struct {
		line string
		want bool
	}{
		{"- ", true},
		{"  - ", true},
		{"-", true},
		{"  -", true},
		{"- x", false},
		{"  - foo", false},
		{"plain", false},
	}
	for _, c := range cases {
		if got := isEmptyBullet(c.line); got != c.want {
			t.Errorf("isEmptyBullet(%q) = %v, want %v", c.line, got, c.want)
		}
	}
}

func TestCycleMarkerEveryMarker(t *testing.T) {
	// col is one past the line end so deltas apply; before-marker col stays put.
	cases := []struct {
		name, line string
		col        int
		want       string
		wantCol    int
	}{
		{"plain to TODO", "- foo", 5, "- TODO foo", 10},
		{"TODO to DONE", "- TODO foo", 10, "- DONE foo", 10},
		{"LATER to DONE", "- LATER x", 9, "- DONE x", 8},
		{"DOING to DONE", "- DOING x", 9, "- DONE x", 8},
		{"WAITING to DONE", "- WAITING x", 11, "- DONE x", 8},
		{"NOW to DONE", "- NOW x", 7, "- DONE x", 8},
		{"DONE to plain", "- DONE foo", 10, "- foo", 5},
		{"CANCELED to plain", "- CANCELED x", 12, "- x", 3},
		{"CANCELLED to plain", "- CANCELLED x", 13, "- x", 3},
		{"priority kept on done", "- LATER [#A] x", 14, "- DONE [#A] x", 13},
		{"priority kept on plain", "- DONE [#A] x", 13, "- [#A] x", 8},
		{"priority kept on todo", "- [#A] x", 8, "- TODO [#A] x", 13},
		{"nested plain to TODO", "  - bar", 7, "  - TODO bar", 12},
		{"cursor before marker unaffected", "- foo", 1, "- TODO foo", 1},
		{"cursor inside marker clamps to marker column", "- DONE foo", 4, "- foo", 2},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			b := New(c.line)
			at(b, 0, c.col)
			if !b.CycleMarker() {
				t.Fatal("CycleMarker returned false")
			}
			if b.String() != c.want || b.Cursor() != (Pos{0, c.wantCol}) {
				t.Fatalf("got %q cur %v, want %q col %d", b.String(), b.Cursor(), c.want, c.wantCol)
			}
			oneUndo(t, b, c.line)
		})
	}
}

func TestCycleMarkerKeepsPriorityThroughTheCycle(t *testing.T) {
	b := New("- LATER [#A] x")
	at(b, 0, 14)
	for _, want := range []string{"- DONE [#A] x", "- [#A] x", "- TODO [#A] x"} {
		if !b.CycleMarker() || b.String() != want {
			t.Fatalf("got %q, want %q", b.String(), want)
		}
	}
}

func TestIndentLine(t *testing.T) {
	cases := []struct{ line, want string }{
		{"- foo", "  - foo"},
		{"  - bar", "    - bar"},
		{"\t- tab", "\t\t- tab"},
		{"", ""},
		{"   ", "   "},
	}
	for _, c := range cases {
		if got := indentLine(c.line); got != c.want {
			t.Errorf("indentLine(%q) = %q, want %q", c.line, got, c.want)
		}
	}
}

func TestDedentLine(t *testing.T) {
	cases := []struct {
		line        string
		wantLine    string
		wantRemoved int
	}{
		{"  - foo", "- foo", 2},
		{" - foo", "- foo", 1},
		{"- foo", "- foo", 0},
		{"    x", "  x", 2},
		{"\t\tx", "\tx", 1},
		{"\t  x", "  x", 1},
	}
	for _, c := range cases {
		gotLine, gotRemoved := dedentLine(c.line)
		if gotLine != c.wantLine || gotRemoved != c.wantRemoved {
			t.Errorf("dedentLine(%q) = (%q,%d), want (%q,%d)", c.line, gotLine, gotRemoved, c.wantLine, c.wantRemoved)
		}
	}
}

func TestNewline(t *testing.T) {
	cases := []struct {
		name    string
		content string
		at      Pos
		want    string
		wantCur Pos
	}{
		{"plain break", "abc", Pos{0, 1}, "a\nbc", Pos{1, 0}},
		{"bullet continues", "- foo", Pos{0, 5}, "- foo\n- ", Pos{1, 2}},
		{"nested bullet keeps indent", "  - foo", Pos{0, 7}, "  - foo\n  - ", Pos{1, 4}},
		{"bullet split mid-text", "- foobar", Pos{0, 5}, "- foo\n- bar", Pos{1, 2}},
		{"empty bullet is cleared", "- ", Pos{0, 2}, "", Pos{0, 0}},
		{"empty nested bullet cleared", "x\n  -", Pos{1, 3}, "x\n", Pos{1, 0}},
		{"cursor inside the marker breaks plainly", "- foo", Pos{0, 1}, "-\n foo", Pos{1, 0}},
		{"star bullet is not continued", "* foo", Pos{0, 5}, "* foo\n", Pos{1, 0}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			b := New(c.content)
			at(b, c.at.Line, c.at.Col)
			b.Newline()
			if b.String() != c.want || b.Cursor() != c.wantCur {
				t.Fatalf("got %q cur %v, want %q cur %v", b.String(), b.Cursor(), c.want, c.wantCur)
			}
			oneUndo(t, b, c.content)
		})
	}
}

func TestNewlineUsesEOLAndReplacesSelectionInOneStep(t *testing.T) {
	const src = "hello world\r\nnext"
	b := New(src)
	b.MoveTo(Pos{0, 5}, false)
	b.MoveTo(Pos{0, 6}, true)
	b.Newline()
	if b.String() != "hello\r\nworld\r\nnext" || b.Cursor() != (Pos{1, 0}) {
		t.Fatalf("got %q cur %v", b.String(), b.Cursor())
	}
	if _, ok := b.Selection(); ok {
		t.Fatal("selection must be gone")
	}
	oneUndo(t, b, src)
}

func TestCycleMarker(t *testing.T) {
	b := New("- foo")
	at(b, 0, 5)
	steps := []struct {
		want string
		col  int
	}{{"- TODO foo", 10}, {"- DONE foo", 10}, {"- foo", 5}}
	for _, s := range steps {
		if !b.CycleMarker() {
			t.Fatal("CycleMarker returned false on a bullet")
		}
		if b.String() != s.want || b.Cursor() != (Pos{0, s.col}) {
			t.Fatalf("got %q cur %v, want %q col %d", b.String(), b.Cursor(), s.want, s.col)
		}
	}
	for _, want := range []string{"- DONE foo", "- TODO foo", "- foo"} {
		if !b.Undo() || b.String() != want {
			t.Fatalf("undo: got %q, want %q", b.String(), want)
		}
	}
}

func TestCycleMarkerNonBulletOpensNoGroup(t *testing.T) {
	b := New("# heading")
	if b.CycleMarker() {
		t.Fatal("expected false")
	}
	if b.Undo() {
		t.Fatal("no group expected")
	}
}

func TestIndentSubtree(t *testing.T) {
	const src = "- p\n  - c1\n\n  - c2\n- q"
	b := New(src)
	at(b, 0, 2)
	b.Indent()
	if want := "  - p\n    - c1\n\n    - c2\n- q"; b.String() != want {
		t.Fatalf("got %q, want %q", b.String(), want)
	}
	if b.Cursor() != (Pos{0, 4}) {
		t.Fatalf("cursor %v", b.Cursor())
	}
	oneUndo(t, b, src)
}

func TestIndentNonBulletLineAloneAndBlankNoop(t *testing.T) {
	b := New("text\n  - x")
	at(b, 0, 0)
	b.Indent()
	if b.String() != "  text\n  - x" || b.Cursor() != (Pos{0, 2}) {
		t.Fatalf("got %q cur %v", b.String(), b.Cursor())
	}
	b = New("a\n\nb")
	at(b, 1, 0)
	b.Indent()
	if b.String() != "a\n\nb" || b.Undo() {
		t.Fatal("blank line must not gain whitespace nor open a group")
	}
}

func TestIndentSelectionLines(t *testing.T) {
	const src = "a\nb\nc"
	b := New(src)
	b.MoveTo(Pos{0, 1}, false)
	b.MoveTo(Pos{2, 0}, true) // end col 0: line 2 excluded
	b.Indent()
	if b.String() != "  a\n  b\nc" {
		t.Fatalf("got %q", b.String())
	}
	sel, ok := b.Selection()
	if !ok || sel.Start != (Pos{0, 3}) || sel.End != (Pos{2, 0}) {
		t.Fatalf("selection %v ok=%v", sel, ok)
	}
	oneUndo(t, b, src)
}

func TestIndentSelectionIncludesEndLineWithColumn(t *testing.T) {
	b := New("a\nb\nc")
	b.MoveTo(Pos{0, 0}, false)
	b.MoveTo(Pos{2, 1}, true)
	b.Indent()
	if b.String() != "  a\n  b\n  c" || b.Cursor() != (Pos{2, 3}) {
		t.Fatalf("got %q cur %v", b.String(), b.Cursor())
	}
}

func TestOutdent(t *testing.T) {
	const src = "- a\n  - b\n    - c\n- d"
	b := New(src)
	at(b, 1, 4)
	b.Outdent()
	if want := "- a\n- b\n  - c\n- d"; b.String() != want {
		t.Fatalf("got %q, want %q", b.String(), want)
	}
	if b.Cursor() != (Pos{1, 2}) {
		t.Fatalf("cursor %v", b.Cursor())
	}
	oneUndo(t, b, src)
}

func TestOutdentAtZeroNoop(t *testing.T) {
	b := New("- a\n  - b")
	at(b, 0, 2)
	b.Outdent()
	if b.String() != "- a\n  - b" || b.Cursor() != (Pos{0, 2}) || b.Undo() {
		t.Fatalf("expected no-op without group, got %q", b.String())
	}
}

func TestOutdentSelectionAndCursorClamp(t *testing.T) {
	b := New("  a\n    b")
	b.MoveTo(Pos{0, 1}, false)
	b.MoveTo(Pos{1, 3}, true)
	b.Outdent()
	if b.String() != "a\n  b" {
		t.Fatalf("got %q", b.String())
	}
	sel, ok := b.Selection()
	if !ok || sel.Start != (Pos{0, 0}) || sel.End != (Pos{1, 1}) {
		t.Fatalf("selection %v ok=%v", sel, ok)
	}
}

func TestIndentTabUnit(t *testing.T) {
	const src = "\t- a\n\t\t- b"
	b := New(src)
	at(b, 0, 1)
	b.Indent()
	if want := "\t\t- a\n\t\t\t- b"; b.String() != want || b.Cursor() != (Pos{0, 2}) {
		t.Fatalf("got %q cur %v, want %q", b.String(), b.Cursor(), want)
	}
	b.Outdent()
	if b.String() != src || b.Cursor() != (Pos{0, 1}) {
		t.Fatalf("outdent got %q cur %v", b.String(), b.Cursor())
	}
}

func TestMoveBlock(t *testing.T) {
	cases := []struct {
		name    string
		content string
		line    int
		col     int
		dir     int
		ok      bool
		want    string
		wantCur Pos
	}{
		{"first sibling up is a no-op", "- a\n- b\n- c\n", 0, 0, -1, false, "", Pos{}},
		{"last sibling down is a no-op", "- a\n- b\n- c\n", 2, 0, +1, false, "", Pos{}},
		{"sibling up", "- a\n- b\n- c\n", 1, 2, -1, true, "- b\n- a\n- c\n", Pos{0, 2}},
		{"children travel up", "- a\n  - a1\n- b\n  - b1\n", 2, 1, -1, true, "- b\n  - b1\n- a\n  - a1\n", Pos{0, 1}},
		{"move down across a sibling with children", "- a\n  - a1\n- b\n  - b1\n", 0, 3, +1, true, "- b\n  - b1\n- a\n  - a1\n", Pos{2, 3}},
		{"first child cannot leave its parent", "- p\n  - a\n  - b", 1, 0, -1, false, "", Pos{}},
		{"last child cannot leave its parent", "- p\n  - a\n- q", 1, 0, +1, false, "", Pos{}},
		{"blank lines between siblings move with the block above", "- a\n\n- b", 2, 1, -1, true, "- b\n- a\n", Pos{0, 1}},
		{"non-bullet up", "x\ny\nz\n", 1, 1, -1, true, "y\nx\nz\n", Pos{0, 1}},
		{"non-bullet down", "x\ny\nz\n", 1, 0, +1, true, "x\nz\ny\n", Pos{2, 0}},
		{"non-bullet at top", "x\ny\n", 0, 0, -1, false, "", Pos{}},
		{"final empty line is never swapped", "x\ny\n", 1, 0, +1, false, "", Pos{}},
		{"the final empty line itself does not move", "x\ny\n", 2, 0, -1, false, "", Pos{}},
		{"last line without final newline moves up", "a\nb", 1, 1, -1, true, "b\na", Pos{0, 1}},
		{"last line without final newline moves up (CRLF)", "a\r\nb", 1, 0, -1, true, "b\r\na", Pos{0, 0}},
		{"line moved off the end gains the terminator", "a\nb", 0, 0, +1, true, "b\na", Pos{1, 0}},
		{"line moved off the end gains the terminator (CRLF)", "a\r\nb\r\nc", 1, 0, +1, true, "a\r\nc\r\nb", Pos{2, 0}},
		{"mixed terminators travel with their lines", "a\r\nb\nc\n", 0, 0, +1, true, "b\na\r\nc\n", Pos{1, 0}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			b := New(c.content)
			at(b, c.line, c.col)
			ok := b.MoveBlock(c.dir)
			if ok != c.ok {
				t.Fatalf("MoveBlock = %v, want %v (buffer %q)", ok, c.ok, b.String())
			}
			if !c.ok {
				if b.String() != c.content || b.Undo() {
					t.Fatalf("no-op must leave bytes and open no group, got %q", b.String())
				}
				return
			}
			if b.String() != c.want || b.Cursor() != c.wantCur {
				t.Fatalf("got %q cur %v, want %q cur %v", b.String(), b.Cursor(), c.want, c.wantCur)
			}
			oneUndo(t, b, c.content)
		})
	}
}

func TestMoveBlockClearsSelection(t *testing.T) {
	b := New("- a\n- b")
	b.MoveTo(Pos{1, 0}, false)
	b.MoveTo(Pos{1, 2}, true)
	if !b.MoveBlock(-1) {
		t.Fatal("expected a move")
	}
	if _, ok := b.Selection(); ok {
		t.Fatal("selection must be cleared")
	}
	if b.String() != "- b\n- a" {
		t.Fatalf("got %q", b.String())
	}
}
