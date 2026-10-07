package views

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestPasteIntoEditorInsertsText(t *testing.T) {
	a := bootApp(t)
	a.Update(key("e"))
	if a.editor == nil {
		t.Fatal("editor did not open")
	}
	a.Update(tea.PasteMsg{Content: "PASTED"})
	if !strings.Contains(a.editor.Content(), "PASTED") {
		t.Errorf("pasted text missing from %q", a.editor.Content())
	}
	if !a.editor.dirty() {
		t.Error("editor not dirty after paste")
	}
}

func TestPasteInEditorPromptIsIgnored(t *testing.T) {
	a := bootApp(t)
	a.Update(key("e"))
	a.Update(key("x"))
	a.Update(key("esc"))
	if a.editor.mode != confirmingExit {
		t.Fatalf("mode = %v, want confirmingExit", a.editor.mode)
	}
	content := a.editor.Content()
	a.Update(tea.PasteMsg{Content: "PASTED"})
	if a.editor.Content() != content {
		t.Errorf("content changed to %q", a.editor.Content())
	}
	if a.editor.mode != confirmingExit {
		t.Errorf("mode changed to %v", a.editor.mode)
	}
}

func TestPasteOutsideEditorIsIgnored(t *testing.T) {
	a := bootApp(t)
	_, cmd := a.Update(tea.PasteMsg{Content: "q"})
	if cmd != nil || a.active != nil || a.editor != nil {
		t.Errorf("read-view paste had an effect: cmd=%v active=%v editor=%v", cmd != nil, a.active, a.editor)
	}

	a.Update(tea.KeyPressMsg{Code: 'p', Mod: tea.ModCtrl})
	p, ok := a.active.(*Picker)
	if !ok {
		t.Fatalf("active = %T, want *Picker", a.active)
	}
	a.Update(tea.PasteMsg{Content: "alp"})
	if got := p.input.Value(); got != "" {
		t.Errorf("picker query = %q, want empty", got)
	}
}

func TestPasteClearsHint(t *testing.T) {
	a := bootApp(t)
	a.hint = "x"
	a.Update(tea.PasteMsg{Content: "p"})
	if a.hint != "" {
		t.Errorf("hint = %q, want cleared", a.hint)
	}
}
