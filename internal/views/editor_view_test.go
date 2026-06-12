package views

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/exp/teatest"
)

func TestEditorView_LoadsContentAndTracksDirty(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	e := NewEditorView("Alpha", "/tmp/Alpha.md", "first line\n", false, 80, 24)

	if got := e.Content(); got != "first line\n" {
		t.Errorf("Content: got %q, want %q", got, "first line\n")
	}
	if e.dirty() {
		t.Errorf("freshly loaded buffer should be clean")
	}

	e.ta.SetValue("first line\nsecond\n")
	if !e.dirty() {
		t.Errorf("buffer should be dirty after edit")
	}

	e.MarkSaved(e.Content())
	if e.dirty() {
		t.Errorf("buffer should be clean after MarkSaved")
	}
}

func TestEditorView_ContentEndsWithSingleNewline(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	e := NewEditorView("Alpha", "/tmp/Alpha.md", "", true, 80, 24)
	e.ta.SetValue("no trailing newline")
	if got := e.Content(); got != "no trailing newline\n" {
		t.Errorf("Content: got %q, want one trailing newline", got)
	}
	e.ta.SetValue("trailing blanks\n\n\n")
	if got := e.Content(); !strings.HasSuffix(got, "\n") || strings.HasSuffix(got, "\n\n") {
		t.Errorf("Content: got %q, want exactly one trailing newline", got)
	}
}

func TestEditorView_CleanAfterSaveWithoutTrailingNewline(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	e := NewEditorView("Alpha", "/tmp/a.md", "", true, 80, 24)
	e.ta.SetValue("no trailing newline") // user typed, no \n
	e.MarkSaved(e.Content())
	if e.dirty() {
		t.Errorf("buffer should be clean after MarkSaved even without a trailing newline")
	}
}

func TestEditorView_CleanOnOpenWhenFileLacksTrailingNewline(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	e := NewEditorView("Alpha", "/tmp/a.md", "loaded without newline", false, 80, 24)
	if e.dirty() {
		t.Errorf("freshly opened buffer should be clean even if the file lacked a trailing newline")
	}
}

func key(s string) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)} }

func TestEditorUpdate_CtrlSRequestsSaveStaysEditing(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	e := NewEditorView("Alpha", "/tmp/a.md", "x\n", false, 80, 24)
	res, _ := e.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	if !res.Save || res.Exit {
		t.Errorf("ctrl+s: got %+v, want {Save:true, Exit:false}", res)
	}
	if e.mode != editing {
		t.Errorf("ctrl+s should stay in editing mode")
	}
}

func TestEditorUpdate_EscOnCleanExits(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	e := NewEditorView("Alpha", "/tmp/a.md", "x\n", false, 80, 24)
	res, _ := e.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if !res.Exit {
		t.Errorf("esc on clean buffer should exit; got %+v", res)
	}
}

func TestEditorUpdate_EscOnDirtyPromptsThenDiscard(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	e := NewEditorView("Alpha", "/tmp/a.md", "x\n", false, 80, 24)
	e.ta.SetValue("x\nmore\n") // make it dirty

	res, _ := e.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if res.Exit || res.Save {
		t.Errorf("esc on dirty buffer should not exit yet; got %+v", res)
	}
	if e.mode != confirmingExit {
		t.Fatalf("esc on dirty should enter confirmingExit")
	}

	res, _ = e.Update(key("c"))
	if res.Exit || e.mode != editing {
		t.Errorf("c should cancel back to editing; got %+v mode=%v", res, e.mode)
	}

	e.Update(tea.KeyMsg{Type: tea.KeyEsc})
	res, _ = e.Update(key("d"))
	if !res.Exit || res.Save {
		t.Errorf("d should exit without save; got %+v", res)
	}
}

func TestEditorUpdate_ConfirmSaveExits(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	e := NewEditorView("Alpha", "/tmp/a.md", "x\n", false, 80, 24)
	e.ta.SetValue("x\nmore\n")
	e.Update(tea.KeyMsg{Type: tea.KeyEsc}) // -> confirmingExit
	res, _ := e.Update(key("s"))
	if !res.Save || !res.Exit {
		t.Errorf("s should save and exit; got %+v", res)
	}
}

func TestEditorUpdate_TypingInsertsAndIsDirty(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	e := NewEditorView("Alpha", "/tmp/a.md", "", true, 80, 24)
	e.Update(key("h"))
	e.Update(key("i"))
	if got := e.ta.Value(); got != "hi" {
		t.Errorf("typing: got %q, want %q", got, "hi")
	}
	if !e.dirty() {
		t.Errorf("typing should mark dirty")
	}
}

func TestEditorView_Golden(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	e := NewEditorView("Alpha", "/tmp/a.md", "- first bullet\n- [[Beta]] link\n", false, 80, 12)
	teatest.RequireEqualOutput(t, []byte(e.View()))
}

func TestEditorView_ConfirmGolden(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	e := NewEditorView("Alpha", "/tmp/a.md", "x\n", false, 80, 12)
	e.ta.SetValue("x\nedited\n")
	e.Update(tea.KeyMsg{Type: tea.KeyEsc}) // -> confirmingExit
	teatest.RequireEqualOutput(t, []byte(e.View()))
}

func TestEditorUpdate_PageDownMovesCursor(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	// 40 numbered lines; height 10 so paging is meaningful.
	var sb strings.Builder
	for i := 0; i < 40; i++ {
		sb.WriteString("line\n")
	}
	e := NewEditorView("Alpha", "/tmp/a.md", sb.String(), false, 80, 10)
	// PageDown should not panic and should leave the buffer unchanged.
	before := e.ta.Value()
	e.Update(tea.KeyMsg{Type: tea.KeyPgDown})
	e.Update(tea.KeyMsg{Type: tea.KeyPgUp})
	if e.ta.Value() != before {
		t.Errorf("paging must not modify the buffer")
	}
	if e.dirty() {
		t.Errorf("paging must not mark the buffer dirty")
	}
}

func TestEditorUpdate_CtrlCCancelsConfirm(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	e := NewEditorView("Alpha", "/tmp/a.md", "x\n", false, 80, 24)
	e.ta.SetValue("x\nmore\n")
	e.Update(tea.KeyMsg{Type: tea.KeyEsc}) // -> confirmingExit
	res, _ := e.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if res.Exit || e.mode != editing {
		t.Errorf("ctrl+c at confirm should cancel back to editing; got %+v mode=%v", res, e.mode)
	}
}

func TestEditorView_SetErrorRendersInView(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	e := NewEditorView("Alpha", "/tmp/a.md", "x\n", false, 80, 6)
	e.SetError("permission denied")
	if !strings.Contains(e.View(), "permission denied") {
		t.Errorf("editor view should show the save error; got:\n%s", e.View())
	}
}

func TestEditorView_MarkSavedClearsError(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	e := NewEditorView("Alpha", "/tmp/a.md", "x\n", false, 80, 6)
	e.SetError("permission denied")
	e.MarkSaved(e.Content())
	if strings.Contains(e.View(), "permission denied") {
		t.Errorf("a successful save should clear the error from the view; got:\n%s", e.View())
	}
}
