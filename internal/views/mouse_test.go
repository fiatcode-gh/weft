package views

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func statusLine(a *App) string {
	lines := frameLines(a)
	return lines[len(lines)-1]
}

func wantMouse(t *testing.T, a *App, mode tea.MouseMode, marker bool, when string) {
	t.Helper()
	if got := a.View().MouseMode; got != mode {
		t.Errorf("%s: MouseMode = %v, want %v", when, got, mode)
	}
	if got := strings.Contains(statusLine(a), "mouse off"); got != marker {
		t.Errorf("%s: 'mouse off' in status = %v, want %v: %q", when, got, marker, statusLine(a))
	}
}

func TestMouseDefaultOnAndToggle(t *testing.T) {
	a := bootApp(t)
	wantMouse(t, a, tea.MouseModeCellMotion, false, "start")
	a.Update(key("m"))
	wantMouse(t, a, tea.MouseModeNone, true, "after m")
	a.Update(key("m"))
	wantMouse(t, a, tea.MouseModeCellMotion, false, "after second m")

	a.Update(key("m"))
	a.setHint("x")
	if strings.Contains(statusLine(a), "mouse off") {
		t.Errorf("marker shown while a hint is up: %q", statusLine(a))
	}
}

func TestMouseToggleInSidebarAndPalette(t *testing.T) {
	a := sidebarApp(t, 120, 30)
	a.Update(key("1"))
	if !a.side.focused {
		t.Fatal("sidebar not focused after 1")
	}
	a.Update(key("m"))
	wantMouse(t, a, tea.MouseModeNone, true, "sidebar m")
	if !a.side.focused {
		t.Error("m moved focus out of the sidebar")
	}

	a.Update(key(":"))
	typeText(a, "mouse")
	a.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	wantMouse(t, a, tea.MouseModeCellMotion, false, "palette")
}

func TestMouseKeyIsTextInEditorAndPrompts(t *testing.T) {
	a := bootApp(t)
	a.Update(key("e"))
	if a.editor == nil {
		t.Fatal("editor did not open")
	}
	before := text(a.editor)
	a.Update(key("m"))
	if got := text(a.editor); got == before || !strings.Contains(got, "m") {
		t.Errorf("m not inserted into the buffer")
	}
	if a.View().MouseMode != tea.MouseModeCellMotion {
		t.Error("m in the editor toggled the mouse")
	}

	b := bootApp(t)
	b.Update(key("c"))
	if b.capture == nil {
		t.Fatal("capture did not open")
	}
	b.Update(key("m"))
	if b.View().MouseMode != tea.MouseModeCellMotion {
		t.Error("m in capture toggled the mouse")
	}
	if !strings.Contains(appText(b), "m") {
		t.Error("capture frame lacks the typed m")
	}

	c := bootApp(t)
	c.Update(tea.KeyPressMsg{Code: 'p', Mod: tea.ModCtrl})
	if c.active == nil {
		t.Fatal("picker did not open")
	}
	c.Update(key("m"))
	if c.View().MouseMode != tea.MouseModeCellMotion {
		t.Error("m in the picker toggled the mouse")
	}
}
