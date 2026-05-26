package views

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestAppOpensAndCancelsPicker(t *testing.T) {
	a := bootApp(t)
	if a.mode != modePage {
		t.Fatalf("setup: want modePage, got %d", a.mode)
	}
	a.Update(tea.KeyMsg{Type: tea.KeyCtrlP})
	if a.mode != modePicker || a.picker == nil {
		t.Fatalf("after ctrl+p: want modePicker with picker set, got mode=%d picker=%v", a.mode, a.picker)
	}
	a.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if a.mode != modePage || a.picker != nil {
		t.Errorf("after esc: want modePage with picker cleared, got mode=%d picker=%v", a.mode, a.picker)
	}
}

func TestAppOpensSearchAndEscapes(t *testing.T) {
	a := bootApp(t)
	a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	if a.mode != modeSearch || a.search == nil {
		t.Fatalf("after /: want modeSearch with search set, got mode=%d search=%v", a.mode, a.search)
	}
	a.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if a.mode != modePage || a.search != nil {
		t.Errorf("after esc: want modePage with search cleared, got mode=%d search=%v", a.mode, a.search)
	}
}

func TestAppOpensBacklinksAndCloses(t *testing.T) {
	a := bootApp(t)
	a.navigate("Hub")
	a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("b")})
	if a.mode != modeBacklinks || a.backlinks == nil {
		t.Fatalf("after b: want modeBacklinks, got mode=%d backlinks=%v", a.mode, a.backlinks)
	}
	a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("b")})
	if a.mode != modePage || a.backlinks != nil {
		t.Errorf("after second b: want modePage cleared, got mode=%d backlinks=%v", a.mode, a.backlinks)
	}
}

func TestAppOpensTodosAndCloses(t *testing.T) {
	a := bootApp(t)
	a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("T")})
	if a.mode != modeTodos || a.todos == nil {
		t.Fatalf("after T: want modeTodos, got mode=%d todos=%v", a.mode, a.todos)
	}
	a.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if a.mode != modePage || a.todos != nil {
		t.Errorf("after esc: want modePage cleared, got mode=%d todos=%v", a.mode, a.todos)
	}
}

func TestAppOpensHelpAndCloses(t *testing.T) {
	a := bootApp(t)
	a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("?")})
	if a.mode != modeHelp || a.help == nil {
		t.Fatalf("after ?: want modeHelp, got mode=%d help=%v", a.mode, a.help)
	}
	a.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if a.mode != modePage || a.help != nil {
		t.Errorf("after esc: want modePage cleared, got mode=%d help=%v", a.mode, a.help)
	}
}

func TestAppPickerAcceptNavigates(t *testing.T) {
	a := bootApp(t)
	a.Update(tea.KeyMsg{Type: tea.KeyCtrlP})
	if a.mode != modePicker {
		t.Fatalf("setup: picker not open")
	}
	for _, r := range "Alp" {
		a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	a.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if a.mode != modePage {
		t.Errorf("after enter: want modePage, got %d", a.mode)
	}
	if a.page.Page() != "Alpha" {
		t.Errorf("after picker accept: want page Alpha, got %q", a.page.Page())
	}
}

func TestAppTodosAcceptNavigates(t *testing.T) {
	a := bootApp(t)
	a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("T")})
	if a.mode != modeTodos {
		t.Fatalf("setup: todos not open")
	}
	a.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if a.mode != modePage {
		t.Errorf("after enter: want modePage, got %d", a.mode)
	}
	if a.todos != nil {
		t.Errorf("todos not cleared")
	}
}

func TestAppSearchDoneMsgRouting(t *testing.T) {
	a := bootApp(t)
	a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	if a.search == nil {
		t.Fatalf("setup: search not open")
	}
	a.Update(searchDoneMsg{hits: []SearchHit{
		{FilePath: "/x", Line: 1, Context: "hello"},
	}})
	if len(a.search.hits) != 1 {
		t.Errorf("searchDoneMsg should populate hits, got %d", len(a.search.hits))
	}
}

func TestAppViewWithOverlay(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	a := bootApp(t)
	a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("T")})
	v := a.View()
	if strings.Contains(v, "? help") {
		t.Errorf("overlay view should not render the page status bar; got:\n%s", v)
	}
	if !strings.Contains(v, "Open todos") {
		t.Errorf("overlay view should show todos title; got:\n%s", v)
	}
}

func TestAppCenterOverlayFallback(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	a := New("/no/such/path", "test")
	if got := a.centerOverlay("hello"); got != "hello" {
		t.Errorf("zero-size fallback: want raw content, got %q", got)
	}
}

func TestAppCenterOverlayPlacesContent(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	a := bootApp(t)
	out := a.centerOverlay("X")
	if !strings.Contains(out, "X") {
		t.Errorf("centered output must contain content; got:\n%s", out)
	}
	if len(out) <= len("X") {
		t.Errorf("centered output should pad with whitespace; got len %d", len(out))
	}
}
