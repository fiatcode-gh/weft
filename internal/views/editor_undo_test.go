package views

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/fiatcode-gh/weft/v2/internal/buffer"
)

var (
	ctrlZ = tea.KeyPressMsg{Code: 'z', Mod: tea.ModCtrl}
	ctrlY = tea.KeyPressMsg{Code: 'y', Mod: tea.ModCtrl}
)

func shiftKey(c rune) tea.KeyPressMsg { return tea.KeyPressMsg{Code: c, Mod: tea.ModShift} }

type editState struct {
	text string
	cur  buffer.Pos
}

func stateOf(e *EditorView) editState { return editState{text(e), cursorPos(e)} }

func TestUndoRedoEachEditKind(t *testing.T) {
	quietTerm(t)
	bs, del := named(tea.KeyBackspace), named(tea.KeyDelete)
	cases := []struct {
		name    string
		content string
		line    int
		col     int
		setup   func(e *EditorView)
		do      func(e *EditorView)
		wantDo  string // text after do, to prove the edit happened
	}{
		{"typing run", "", 0, 0, nil, func(e *EditorView) { typeRunes(e, "hello") }, "hello"},
		{"backspace run", "hello world\n", 0, 11, nil, func(e *EditorView) { press(e, bs, bs, bs) }, "hello wo\n"},
		{"delete run", "hello world\n", 0, 0, nil, func(e *EditorView) { press(e, del, del, del) }, "lo world\n"},
		{"word delete", "alpha beta\n", 0, 10, nil, func(e *EditorView) { press(e, ctrl('w')) }, "alpha \n"},
		{"ctrl+k", "alpha beta\n", 0, 5, nil, func(e *EditorView) { press(e, ctrl('k')) }, "alpha\n"},
		{"bracketed paste", "ab\n", 0, 1, nil, func(e *EditorView) { e.Paste(tea.PasteMsg{Content: "X\nY"}) }, "aX\nYb\n"},
		{"cut", "alpha beta\n", 0, 0,
			func(e *EditorView) { press(e, shiftKey(tea.KeyRight), shiftKey(tea.KeyRight)) },
			func(e *EditorView) { press(e, ctrl('x')) }, "pha beta\n"},
		{"register paste", "ab\n", 0, 0,
			func(e *EditorView) { *e.reg = register{text: "ZZ"} },
			func(e *EditorView) { press(e, ctrl('v')) }, "ZZab\n"},
		{"completion accept", "", 0, 0,
			func(e *EditorView) { typeRunes(e, "see [[Alp") },
			func(e *EditorView) { press(e, named(tea.KeyEnter)) }, "see [[Alpha]]"},
		{"ctrl+t", "- item\n", 0, 3, nil, func(e *EditorView) { press(e, ctrl('t')) }, "- TODO item\n"},
		{"tab on a bullet with children", "- a\n  - b\n- c\n", 0, 2, nil,
			func(e *EditorView) { press(e, named(tea.KeyTab)) }, "  - a\n    - b\n- c\n"},
		{"alt+down", "- a\n- b\n", 0, 2, nil,
			func(e *EditorView) { press(e, tea.KeyPressMsg{Code: tea.KeyDown, Mod: tea.ModAlt}) }, "- b\n- a\n"},
		{"enter continuation", "- item\n", 0, 6, nil,
			func(e *EditorView) { press(e, named(tea.KeyEnter)) }, "- item\n- \n"},
		{"case word", "alpha beta\n", 0, 0, nil, func(e *EditorView) { press(e, altKey('u')) }, "ALPHA beta\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := editorAt(loadFixture(t), "P", "/tmp/p.md", tc.content, false, 60, 12, 0)
			setCursor(e, tc.line, tc.col)
			if tc.setup != nil {
				tc.setup(e)
			}
			e.buf.Break()
			before := stateOf(e)
			tc.do(e)
			after := stateOf(e)
			if after.text != tc.wantDo {
				t.Fatalf("edit produced %q, want %q", after.text, tc.wantDo)
			}
			press(e, ctrlZ)
			if got := stateOf(e); got != before {
				t.Errorf("after ctrl+z %+v, want %+v", got, before)
			}
			press(e, ctrlY)
			if got := stateOf(e); got != after {
				t.Errorf("after ctrl+y %+v, want %+v", got, after)
			}
		})
	}
}

func TestUndoAfterSaveKeepsHistory(t *testing.T) {
	a, _ := openJournal(t)
	typeApp(a, "X")
	a.Update(ctrlS)
	if a.editor.dirty() {
		t.Fatal("setup: dirty after save")
	}
	a.Update(ctrlZ)
	if got := a.editor.Content(); got != journalBody {
		t.Errorf("content %q, want %q", got, journalBody)
	}
	if !a.editor.dirty() {
		t.Error("undoing past the save must leave the buffer dirty")
	}
}

func TestUndoMergeOnSave(t *testing.T) {
	a, path := openJournal(t)
	typeApp(a, "X")
	appendTo(t, path, "- agent\n")
	a.Update(ctrlS)
	merged := "X- one\n- two\n- three\n- agent\n"
	if a.editor.Content() != merged || a.editor.dirty() {
		t.Fatalf("setup: content %q dirty=%v", a.editor.Content(), a.editor.dirty())
	}
	a.Update(ctrlZ)
	if got := a.editor.Content(); got != "X- one\n- two\n- three\n" {
		t.Errorf("undo of the merge: content %q, want the pre-merge buffer", got)
	}
	if !a.editor.dirty() {
		t.Error("the pre-merge buffer is dirty against the merged disk")
	}
}

func TestUndoReload(t *testing.T) {
	a, _ := openClash(t)
	a.Update(key("r"))
	if a.editor.Content() != clashTheirs {
		t.Fatalf("setup: content %q", a.editor.Content())
	}
	a.Update(ctrlZ)
	if got := a.editor.Content(); got != "X- one\n- two\n- three\n" {
		t.Errorf("undo of the reload: content %q, want the pre-reload buffer", got)
	}
}

func TestNothingToUndoNotice(t *testing.T) {
	quietTerm(t)
	e := editorAt(nil, "P", "/tmp/p.md", "x\n", false, 60, 8, 0)
	press(e, ctrlZ)
	if !strings.Contains(plain(e.View()), "nothing to undo") {
		t.Errorf("no undo notice:\n%s", plain(e.View()))
	}
	press(e, ctrlY)
	if !strings.Contains(plain(e.View()), "nothing to redo") {
		t.Errorf("no redo notice:\n%s", plain(e.View()))
	}
	typeRunes(e, "a")
	press(e, ctrlZ)
	if strings.Contains(plain(e.View()), "nothing to") {
		t.Errorf("notice shown for a successful undo:\n%s", plain(e.View()))
	}
}

// After an undo the cursor is on screen and the vertical goal column is
// reset (the next up/down starts from the restored cursor).
func TestUndoRevealsCursorAndResetsGoal(t *testing.T) {
	quietTerm(t)
	e := editorAt(nil, "P", "/tmp/p.md", strings.Repeat("line\n", 40), false, 40, 8, 0)
	setCursor(e, 0, 2)
	press(e, named(tea.KeyDown), named(tea.KeyDown))
	typeRunes(e, "x")
	for range 30 {
		press(e, named(tea.KeyDown))
	}
	e.View()
	press(e, ctrlZ)
	if got := cursorPos(e); got.Line != 2 {
		t.Fatalf("cursor line %d, want 2", got.Line)
	}
	if e.goalOK {
		t.Error("goal column kept after undo")
	}
	if e.Cursor() == nil {
		t.Error("cursor not on screen after undo")
	}
}
