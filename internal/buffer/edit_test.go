package buffer

import (
	"strings"
	"testing"
)

func at(b *Buffer, line, col int) *Buffer {
	b.MoveTo(Pos{line, col}, false)
	return b
}

func TestTypeCoalesces(t *testing.T) {
	b := New("")
	b.Type("a")
	b.Type("b")
	b.Type("c")
	if b.String() != "abc" {
		t.Fatalf("got %q", b.String())
	}
	if !b.Undo() {
		t.Fatal("Undo returned false")
	}
	if b.String() != "" {
		t.Fatalf("one Undo must restore empty, got %q", b.String())
	}
	if b.Undo() {
		t.Fatal("second Undo must report false")
	}
}

func TestTypeNonAdjacentIsNewGroup(t *testing.T) {
	b := New("xy")
	b.Type("a") // at (0,0)
	b.Break()
	b.Type("b")
	b.Undo()
	if b.String() != "axy" {
		t.Fatalf("got %q", b.String())
	}
}

func TestMoveBreaksGroup(t *testing.T) {
	b := New("")
	b.Type("ab")
	b.MoveTo(b.End(), false)
	b.Type("cd")
	b.Undo()
	if b.String() != "ab" {
		t.Fatalf("got %q, want ab", b.String())
	}
	b.Undo()
	if b.String() != "" {
		t.Fatalf("got %q", b.String())
	}
}

func TestBackspaceRunCoalesces(t *testing.T) {
	b := New("abcd")
	at(b, 0, 4)
	b.Backspace()
	b.Backspace()
	b.Backspace()
	if b.String() != "a" {
		t.Fatalf("got %q", b.String())
	}
	b.Undo()
	if b.String() != "abcd" {
		t.Fatalf("one Undo must restore, got %q", b.String())
	}
	if b.Cursor() != (Pos{0, 4}) {
		t.Fatalf("cursor %v", b.Cursor())
	}
}

func TestDeleteRunCoalesces(t *testing.T) {
	b := New("abcd")
	at(b, 0, 0)
	b.Delete()
	b.Delete()
	b.Delete()
	if b.String() != "d" {
		t.Fatalf("got %q", b.String())
	}
	b.Undo()
	if b.String() != "abcd" {
		t.Fatalf("got %q", b.String())
	}
}

func TestBackspaceThenDeleteAreSeparateGroups(t *testing.T) {
	b := New("abcd")
	at(b, 0, 2)
	b.Backspace()
	b.Delete()
	if b.String() != "ad" {
		t.Fatalf("got %q", b.String())
	}
	b.Undo()
	if b.String() != "acd" {
		t.Fatalf("got %q", b.String())
	}
}

func TestTypeOverSelectionIsNewGroup(t *testing.T) {
	b := New("")
	b.Type("abc")
	b.MoveTo(Pos{0, 0}, false)
	b.MoveTo(Pos{0, 2}, true)
	b.Type("X")
	b.Type("Y")
	if b.String() != "XYc" {
		t.Fatalf("got %q", b.String())
	}
	b.Undo()
	if b.String() != "abc" {
		t.Fatalf("typing over a selection and after it is one group, got %q", b.String())
	}
	b.Undo()
	if b.String() != "" {
		t.Fatalf("got %q", b.String())
	}
}

func TestBackspaceWithSelectionDeletesIt(t *testing.T) {
	b := New("hello")
	b.MoveTo(Pos{0, 1}, false)
	b.MoveTo(Pos{0, 4}, true)
	b.Backspace()
	if b.String() != "ho" || b.Cursor() != (Pos{0, 1}) {
		t.Fatalf("got %q at %v", b.String(), b.Cursor())
	}
	if _, ok := b.Selection(); ok {
		t.Fatal("selection must be gone")
	}
}

func TestInsertUsesEOL(t *testing.T) {
	b := New("a\r\nb\r\n")
	at(b, 0, 1)
	b.Insert("x\ny")
	if got := b.String(); got != "ax\r\ny\r\nb\r\n" {
		t.Fatalf("got %q", got)
	}

	for _, in := range []string{"1\r\n2", "1\r2"} {
		c := New("")
		c.Insert(in)
		if c.Len() != 2 || c.Line(0) != "1" || c.Line(1) != "2" {
			t.Errorf("Insert(%q) → %q", in, c.String())
		}
	}
	d := New("")
	d.Insert("a\r\n\rb")
	if d.Len() != 3 {
		t.Errorf("CRLF then CR is two breaks, got %q", d.String())
	}
}

func TestInsertIsOneGroup(t *testing.T) {
	b := New("")
	b.Insert("a\nb\nc")
	b.Undo()
	if b.String() != "" {
		t.Fatalf("got %q", b.String())
	}
	if b.Cursor() != (Pos{0, 0}) {
		t.Fatalf("cursor %v", b.Cursor())
	}
}

func TestTypeWithBreakRoutesToInsert(t *testing.T) {
	b := New("")
	b.Type("a\nb")
	if b.Len() != 2 {
		t.Fatalf("got %q", b.String())
	}
}

func TestBackspaceJoinsLinesKeepsTerminators(t *testing.T) {
	// CRLF line joined onto an LF line: remaining terminator is the second line's own.
	b := New("a\r\nb\nc")
	at(b, 1, 0)
	b.Backspace()
	if got := b.String(); got != "ab\nc" {
		t.Fatalf("got %q", got)
	}
	// LF line joined onto a CRLF line.
	c := New("a\nb\r\nc")
	at(c, 1, 0)
	c.Backspace()
	if got := c.String(); got != "ab\r\nc" {
		t.Fatalf("got %q", got)
	}
	// Joining the last line: the document's last line keeps having no terminator.
	d := New("a\r\nb")
	at(d, 1, 0)
	d.Backspace()
	if got := d.String(); got != "ab" {
		t.Fatalf("got %q", got)
	}
	d.Undo()
	if got := d.String(); got != "a\r\nb" {
		t.Fatalf("undo got %q", got)
	}
}

func TestDeleteJoinsNextLine(t *testing.T) {
	b := New("a\nb")
	at(b, 0, 1)
	b.Delete()
	if b.String() != "ab" {
		t.Fatalf("got %q", b.String())
	}
}

func TestBackspaceAtStartAndDeleteAtEndAreNoOps(t *testing.T) {
	b := New("ab")
	v := b.Version()
	b.Backspace()
	at(b, 0, 2)
	b.Delete()
	if b.String() != "ab" || b.Version() != v {
		t.Fatalf("got %q version %d→%d", b.String(), v, b.Version())
	}
	if b.Undo() {
		t.Fatal("nothing to undo")
	}
}

func TestUndoRedoRestoresCursorAndSelection(t *testing.T) {
	b := New("hello world")
	b.MoveTo(Pos{0, 6}, false)
	b.MoveTo(Pos{0, 11}, true)
	b.Type("there")
	if b.String() != "hello there" || b.Cursor() != (Pos{0, 11}) {
		t.Fatalf("got %q at %v", b.String(), b.Cursor())
	}
	b.Undo()
	if b.String() != "hello world" {
		t.Fatalf("got %q", b.String())
	}
	r, ok := b.Selection()
	if !ok || r != (Range{Pos{0, 6}, Pos{0, 11}}) || b.Cursor() != (Pos{0, 11}) {
		t.Fatalf("selection %v %v cursor %v", r, ok, b.Cursor())
	}
	if !b.Redo() {
		t.Fatal("Redo false")
	}
	if b.String() != "hello there" || b.Cursor() != (Pos{0, 11}) {
		t.Fatalf("got %q at %v", b.String(), b.Cursor())
	}
	if _, ok := b.Selection(); ok {
		t.Fatal("redo state has no selection")
	}
}

func TestRedoClearedByEdit(t *testing.T) {
	b := New("")
	b.Type("a")
	b.Undo()
	b.Type("b")
	if b.Redo() {
		t.Fatal("redo stack must be cleared by a new edit")
	}
	if b.String() != "b" {
		t.Fatalf("got %q", b.String())
	}
}

func TestUndoRedoMultiLine(t *testing.T) {
	b := New("a\r\nb\r\nc")
	at(b, 0, 1)
	b.MoveTo(Pos{2, 0}, true)
	b.Insert("Z\nW")
	if got := b.String(); got != "aZ\r\nWc" {
		t.Fatalf("got %q", got)
	}
	b.Undo()
	if got := b.String(); got != "a\r\nb\r\nc" {
		t.Fatalf("undo got %q", got)
	}
	b.Redo()
	if got := b.String(); got != "aZ\r\nWc" {
		t.Fatalf("redo got %q", got)
	}
}

func TestDeleteRangeAndReplaceRange(t *testing.T) {
	b := New("one\ntwo\nthree")
	b.DeleteRange(Range{Pos{0, 2}, Pos{2, 2}})
	if b.String() != "onree" || b.Cursor() != (Pos{0, 2}) {
		t.Fatalf("got %q at %v", b.String(), b.Cursor())
	}
	b.Undo()
	if b.String() != "one\ntwo\nthree" {
		t.Fatalf("got %q", b.String())
	}
	b.ReplaceRange(Range{Pos{1, 0}, Pos{1, 3}}, "2\n2")
	if b.String() != "one\n2\n2\nthree" || b.Cursor() != (Pos{2, 1}) {
		t.Fatalf("got %q at %v", b.String(), b.Cursor())
	}
	b.Undo()
	if b.String() != "one\ntwo\nthree" {
		t.Fatalf("got %q", b.String())
	}
}

func lastGroup(b *Buffer) *group { return b.undo[len(b.undo)-1] }

func TestReplaceAllOneStepMinimal(t *testing.T) {
	old := manyLines(10000, true)
	lines := strings.Split(old, "\n")
	orig := lines[5000]
	lines[5000] = "CHANGED"
	next := strings.Join(lines, "\n")

	b := New(old)
	b.ReplaceAll(next)
	if !b.EqualString(next) {
		t.Fatal("ReplaceAll content mismatch")
	}
	g := lastGroup(b)
	if len(g.edits) != 1 {
		t.Fatalf("edits = %d", len(g.edits))
	}
	e := g.edits[0]
	if e.removed != orig+"\n" || e.inserted != "CHANGED\n" {
		t.Fatalf("recorded %q → %q", e.removed, e.inserted)
	}
	if !b.Undo() || !b.EqualString(old) {
		t.Fatal("Undo must restore")
	}
	if !b.Redo() || !b.EqualString(next) {
		t.Fatal("Redo must reapply")
	}
}

func TestReplaceAllVariants(t *testing.T) {
	cases := []struct{ old, new string }{
		{"a\nb\nc", "a\nB\nc"},
		{"a\nb\nc", "a\nb\nc\n"},
		{"a\nb\nc\n", "a\nb\nc"},
		{"a\nb", "x"},
		{"x", "a\nb\nc"},
		{"a\nb\n", ""},
		{"", "a\nb\n"},
		{"a\r\nb\r\nc", "a\nb\r\nc"},
		{"same\n", "same\n"},
		{"a\nb\nb\nb\nc", "a\nb\nc"},
	}
	for _, c := range cases {
		b := New(c.old)
		b.ReplaceAll(c.new)
		if b.String() != c.new {
			t.Errorf("%q → %q: got %q", c.old, c.new, b.String())
		}
		if c.old == c.new {
			if b.Undo() {
				t.Errorf("identical content must not record a group")
			}
			continue
		}
		b.Undo()
		if b.String() != c.old {
			t.Errorf("%q → %q: undo got %q", c.old, c.new, b.String())
		}
	}
}

func TestReplaceAllClampsCursor(t *testing.T) {
	b := New("a\nbbbb\nc")
	at(b, 2, 1)
	b.ReplaceAll("a")
	if b.Cursor() != (Pos{0, 1}) {
		t.Fatalf("cursor %v", b.Cursor())
	}
}

func TestUndoEmptyStack(t *testing.T) {
	b := New("abc")
	if b.Undo() || b.Redo() {
		t.Fatal("empty stacks must report false")
	}
	if b.String() != "abc" {
		t.Fatalf("got %q", b.String())
	}
}

func TestChangedReportsFirstLine(t *testing.T) {
	b := New("a\nb\nc\nd")
	if _, ok := b.Changed(); ok {
		t.Fatal("fresh buffer has no change")
	}
	at(b, 3, 1)
	b.Type("x")
	at(b, 1, 0)
	b.Type("y")
	from, ok := b.Changed()
	if !ok || from != 1 {
		t.Fatalf("Changed = %d,%v", from, ok)
	}
	if _, ok := b.Changed(); ok {
		t.Fatal("Changed must reset")
	}
	b.Undo()
	from, ok = b.Changed()
	if !ok || from != 1 {
		t.Fatalf("undo Changed = %d,%v", from, ok)
	}
	b.Redo()
	if from, ok = b.Changed(); !ok || from != 1 {
		t.Fatalf("redo Changed = %d,%v", from, ok)
	}
}

func TestEditsKeepFinalLineWithoutTerminator(t *testing.T) {
	b := New("a")
	at(b, 0, 1)
	b.Insert("\n")
	if b.String() != "a\n" {
		t.Fatalf("got %q", b.String())
	}
	b.Backspace()
	if b.String() != "a" {
		t.Fatalf("got %q", b.String())
	}
}
