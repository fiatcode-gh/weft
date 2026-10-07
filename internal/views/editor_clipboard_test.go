package views

import (
	"reflect"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

var (
	ctrlC = tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}
	ctrlX = tea.KeyPressMsg{Code: 'x', Mod: tea.ModCtrl}
	ctrlV = tea.KeyPressMsg{Code: 'v', Mod: tea.ModCtrl}
)

// copyKey presses a key and returns the editor's cmd.
func copyKey(e *EditorView, m tea.KeyPressMsg) tea.Cmd {
	_, cmd := e.Update(m)
	return cmd
}

func TestCopySendsOSC52(t *testing.T) {
	quietTerm(t)
	e := editorAt(nil, "P", "/tmp/p.md", "alpha beta\n", false, 60, 8, 0)
	setCursor(e, 0, 2)
	press(e, shiftKey(tea.KeyRight), shiftKey(tea.KeyRight), shiftKey(tea.KeyRight))
	cmd := copyKey(e, ctrlC)
	if cmd == nil {
		t.Fatal("copy returned no cmd")
	}
	want := tea.SetClipboard("pha")()
	if got := cmd(); !reflect.DeepEqual(got, want) {
		t.Errorf("cmd message %#v, want %#v", got, want)
	}
	if *e.reg != (register{text: "pha"}) {
		t.Errorf("register %+v", *e.reg)
	}
	if text(e) != "alpha beta\n" {
		t.Errorf("copy changed the text: %q", text(e))
	}
	if !strings.Contains(plain(e.View()), "copied") {
		t.Errorf("no copied notice:\n%s", plain(e.View()))
	}
}

func TestCopyWithoutSelectionCopiesLine(t *testing.T) {
	quietTerm(t)
	e := editorAt(nil, "P", "/tmp/p.md", "alpha\nbeta\n", false, 60, 8, 0)
	setCursor(e, 1, 2)
	cmd := copyKey(e, ctrlC)
	if cmd == nil {
		t.Fatal("copy returned no cmd")
	}
	if got, want := cmd(), tea.SetClipboard("beta\n")(); !reflect.DeepEqual(got, want) {
		t.Errorf("cmd message %#v, want %#v", got, want)
	}
	if *e.reg != (register{text: "beta\n", linewise: true}) {
		t.Errorf("register %+v", *e.reg)
	}
	// A line-copied register pastes above the cursor line.
	setCursor(e, 0, 1)
	press(e, ctrlV)
	if got := text(e); got != "beta\nalpha\nbeta\n" {
		t.Errorf("text %q", got)
	}
}

func TestCutWithAndWithoutSelection(t *testing.T) {
	quietTerm(t)
	t.Run("selection", func(t *testing.T) {
		e := editorAt(nil, "P", "/tmp/p.md", "alpha beta\n", false, 60, 8, 0)
		press(e, shiftKey(tea.KeyRight), shiftKey(tea.KeyRight))
		cmd := copyKey(e, ctrlX)
		if text(e) != "pha beta\n" {
			t.Errorf("text %q", text(e))
		}
		if cmd == nil {
			t.Fatal("cut returned no cmd")
		}
		if got, want := cmd(), tea.SetClipboard("al")(); !reflect.DeepEqual(got, want) {
			t.Errorf("cmd message %#v, want %#v", got, want)
		}
		if !strings.Contains(plain(e.View()), "cut") {
			t.Errorf("no cut notice:\n%s", plain(e.View()))
		}
	})
	t.Run("whole line", func(t *testing.T) {
		e := editorAt(nil, "P", "/tmp/p.md", "alpha\nbeta\ngamma\n", false, 60, 8, 0)
		setCursor(e, 1, 2)
		cmd := copyKey(e, ctrlX)
		if text(e) != "alpha\ngamma\n" {
			t.Errorf("text %q", text(e))
		}
		if cmd == nil {
			t.Fatal("cut returned no cmd")
		}
		if got, want := cmd(), tea.SetClipboard("beta\n")(); !reflect.DeepEqual(got, want) {
			t.Errorf("cmd message %#v, want %#v", got, want)
		}
		if *e.reg != (register{text: "beta\n", linewise: true}) {
			t.Errorf("register %+v", *e.reg)
		}
	})
}

func TestPasteEmptyRegisterNotice(t *testing.T) {
	quietTerm(t)
	e := editorAt(nil, "P", "/tmp/p.md", "alpha\n", false, 60, 8, 0)
	press(e, ctrlV)
	if text(e) != "alpha\n" {
		t.Errorf("text changed: %q", text(e))
	}
	if !strings.Contains(plain(e.View()), "nothing copied yet") {
		t.Errorf("no notice:\n%s", plain(e.View()))
	}
}

func TestPasteRegisterAcrossEditorSessions(t *testing.T) {
	a := bootApp(t, bootConfig{files: map[string]string{
		"pages/A.md": "alpha line\n",
		"pages/B.md": "beta\n",
	}})
	a.navigate("A")
	a.Update(key("e"))
	if a.editor == nil {
		t.Fatal("editor did not open on A")
	}
	a.Update(shiftKey(tea.KeyEnd))
	a.Update(ctrlC)
	a.Update(named(tea.KeyEsc))
	if a.editor != nil {
		t.Fatal("setup: editor still open after esc on a clean buffer")
	}
	a.navigate("B")
	a.Update(key("e"))
	if a.editor == nil {
		t.Fatal("editor did not open on B")
	}
	a.Update(ctrlV)
	if got := a.editor.Content(); !strings.Contains(got, "alpha line") || !strings.Contains(got, "beta") {
		t.Errorf("content %q, want the register pasted into B", got)
	}
}

// flatten runs cmd and returns its messages, unwrapping batches.
func flatten(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	msg := cmd()
	if b, ok := msg.(tea.BatchMsg); ok {
		var out []tea.Msg
		for _, c := range b {
			out = append(out, flatten(c)...)
		}
		return out
	}
	return []tea.Msg{msg}
}

func TestCtrlCDoesNotLeaveEditor(t *testing.T) {
	a, _ := openEditor(t, map[string]string{"pages/P.md": "intro\n"}, "P")
	_, cmd := a.Update(ctrlC)
	if a.editor == nil {
		t.Fatal("ctrl+c left the editor")
	}
	for _, m := range flatten(cmd) {
		if _, quit := m.(tea.QuitMsg); quit {
			t.Fatal("ctrl+c quit the program")
		}
	}
}

// The copy cmd must reach the program: the App batches the editor's cmd.
func TestAppForwardsEditorCmd(t *testing.T) {
	a, _ := openEditor(t, map[string]string{"pages/P.md": "intro\n"}, "P")
	a.Update(shiftKey(tea.KeyEnd))
	_, cmd := a.Update(ctrlC)
	want := tea.SetClipboard("intro")()
	for _, m := range flatten(cmd) {
		if reflect.DeepEqual(m, want) {
			return
		}
	}
	t.Errorf("the App dropped the editor's clipboard cmd")
}

func TestPromptsIgnoreClipboardAndUndoKeys(t *testing.T) {
	quietTerm(t)
	prompt := func() *EditorView {
		e := editorAt(nil, "P", "/tmp/p.md", "alpha\n", false, 60, 8, 0)
		typeRunes(e, "x")
		press(e, named(tea.KeyEsc))
		if e.mode != confirmingExit {
			t.Fatal("setup: expected the exit prompt")
		}
		return e
	}
	for _, m := range []tea.KeyPressMsg{ctrlX, ctrlV, ctrlZ, ctrlY, ctrl('a')} {
		e := prompt()
		if cmd := copyKey(e, m); cmd != nil {
			t.Errorf("%s returned a cmd in the exit prompt", m)
		}
		if e.mode != confirmingExit || text(e) != "xalpha\n" {
			t.Errorf("%s: mode %v text %q, want the prompt untouched", m, e.mode, text(e))
		}
	}
	e := prompt()
	res, cmd := e.Update(ctrlC)
	if res.Exit || cmd != nil || e.mode != editing || *e.reg != (register{}) {
		t.Errorf("ctrl+c in the exit prompt: res=%+v cmd=%v mode=%v reg=%+v, want a plain cancel", res, cmd != nil, e.mode, *e.reg)
	}
}
