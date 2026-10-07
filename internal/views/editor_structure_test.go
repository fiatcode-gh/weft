package views

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

func press(e *EditorView, msgs ...tea.KeyPressMsg) {
	for _, m := range msgs {
		e.Update(m)
	}
}

var (
	tabKey      = tea.KeyPressMsg{Code: tea.KeyTab}
	shiftTabKey = tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift}
	altUpKey    = tea.KeyPressMsg{Code: tea.KeyUp, Mod: tea.ModAlt}
	altDownKey  = tea.KeyPressMsg{Code: tea.KeyDown, Mod: tea.ModAlt}
)

// structureCase presses key at (line, col) of content and checks the text,
// then that one undo restores the original.
func structureCase(t *testing.T, content string, line, col int, k tea.KeyPressMsg, want string) {
	t.Helper()
	quietTerm(t)
	e := editorAt(nil, "P", "/tmp/p.md", content, false, 60, 12, 0)
	setCursor(e, line, col)
	e.Update(k)
	if got := text(e); got != want {
		t.Errorf("text = %q, want %q", got, want)
	}
	if want != content {
		if !e.buf.Undo() || text(e) != content {
			t.Errorf("one undo must restore %q, got %q", content, text(e))
		}
	}
}

func TestEditorTabCarriesChildren(t *testing.T) {
	structureCase(t, "- a\n  - b\n- c\n", 0, 1, tabKey, "  - a\n    - b\n- c\n")
}

func TestEditorShiftTabCarriesChildren(t *testing.T) {
	structureCase(t, "  - a\n    - b\n- c\n", 0, 3, shiftTabKey, "- a\n  - b\n- c\n")
}

func TestEditorAltUpDownAtListEdgesAreNoOps(t *testing.T) {
	structureCase(t, "- a\n- b\n", 0, 0, altUpKey, "- a\n- b\n")
	structureCase(t, "- a\n- b\n", 1, 0, altDownKey, "- a\n- b\n")
}

func TestEditorAltDownMovesBulletWithChildren(t *testing.T) {
	structureCase(t, "- a\n  - a1\n- b\n- c\n", 0, 0, altDownKey, "- b\n- a\n  - a1\n- c\n")
}

func TestEditorAltUpMovesBulletWithChildren(t *testing.T) {
	structureCase(t, "- a\n- b\n  - b1\n- c\n", 1, 0, altUpKey, "- b\n  - b1\n- a\n- c\n")
}

func TestEditorAltUpMovesPlainLine(t *testing.T) {
	structureCase(t, "one\ntwo\n", 1, 0, altUpKey, "two\none\n")
}
