package views

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"git.fiatcode.dev/fiatcode/peekseq/internal/search"
)

// activeKind returns a short name for the active overlay's concrete type, or
// "page" when no overlay is open.
func activeKind(a *App) string {
	switch a.active.(type) {
	case nil:
		return "page"
	case *Picker:
		return "picker"
	case *SearchView:
		return "search"
	case *Backlinks:
		return "backlinks"
	case *Todos:
		return "todos"
	case *Help:
		return "help"
	default:
		return "unknown"
	}
}

func TestAppOpensAndCancelsPicker(t *testing.T) {
	a := bootApp(t)
	if activeKind(a) != "page" {
		t.Fatalf("setup: want page, got %s", activeKind(a))
	}
	a.Update(tea.KeyMsg{Type: tea.KeyCtrlP})
	if activeKind(a) != "picker" {
		t.Fatalf("after ctrl+p: want picker, got %s", activeKind(a))
	}
	a.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if activeKind(a) != "page" {
		t.Errorf("after esc: want page, got %s", activeKind(a))
	}
}

func TestAppOpensSearchAndEscapes(t *testing.T) {
	a := bootApp(t)
	a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	if activeKind(a) != "search" {
		t.Fatalf("after /: want search, got %s", activeKind(a))
	}
	a.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if activeKind(a) != "page" {
		t.Errorf("after esc: want page, got %s", activeKind(a))
	}
}

func TestAppOpensBacklinksAndCloses(t *testing.T) {
	a := bootApp(t)
	a.navigate("Hub")
	a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("b")})
	if activeKind(a) != "backlinks" {
		t.Fatalf("after b: want backlinks, got %s", activeKind(a))
	}
	a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("b")})
	if activeKind(a) != "page" {
		t.Errorf("after second b: want page, got %s", activeKind(a))
	}
}

func TestAppOpensTodosAndCloses(t *testing.T) {
	a := bootApp(t)
	a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("T")})
	if activeKind(a) != "todos" {
		t.Fatalf("after T: want todos, got %s", activeKind(a))
	}
	a.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if activeKind(a) != "page" {
		t.Errorf("after esc: want page, got %s", activeKind(a))
	}
}

func TestAppOpensHelpAndCloses(t *testing.T) {
	a := bootApp(t)
	a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("?")})
	if activeKind(a) != "help" {
		t.Fatalf("after ?: want help, got %s", activeKind(a))
	}
	a.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if activeKind(a) != "page" {
		t.Errorf("after esc: want page, got %s", activeKind(a))
	}
}

func TestAppPickerAcceptNavigates(t *testing.T) {
	a := bootApp(t)
	a.Update(tea.KeyMsg{Type: tea.KeyCtrlP})
	if activeKind(a) != "picker" {
		t.Fatalf("setup: picker not open")
	}
	for _, r := range "Alp" {
		a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	a.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if activeKind(a) != "page" {
		t.Errorf("after enter: want page, got %s", activeKind(a))
	}
	if a.page.Page() != "Alpha" {
		t.Errorf("after picker accept: want page Alpha, got %q", a.page.Page())
	}
}

func TestAppTodosAcceptNavigates(t *testing.T) {
	a := bootApp(t)
	a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("T")})
	if activeKind(a) != "todos" {
		t.Fatalf("setup: todos not open")
	}
	a.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if activeKind(a) != "page" {
		t.Errorf("after enter: want page, got %s", activeKind(a))
	}
}

func TestAppSearchDoneMsgRouting(t *testing.T) {
	a := bootApp(t)
	a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	s, ok := a.active.(*SearchView)
	if !ok {
		t.Fatalf("setup: search not open, got %s", activeKind(a))
	}
	a.Update(searchDoneMsg{hits: []search.Hit{
		{FilePath: "/x", Line: 1, Context: "hello"},
	}})
	if len(s.hits) != 1 {
		t.Errorf("searchDoneMsg should populate hits, got %d", len(s.hits))
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

// TestAppResizePropagatesToOverlays asserts that a WindowSizeMsg arriving while
// an overlay is open updates the overlay's cached size.
func TestPickerCreate_OpensEditorOnNewPage(t *testing.T) {
	a := bootApp(t)
	a.active = NewPicker(a.idx, a.width, a.height)
	// Type a brand-new name into the open picker.
	for _, r := range "Zzz New Page" {
		a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(string(r))})
	}
	// Enter selects the create row.
	a.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if a.active != nil {
		t.Errorf("picker should be closed after create")
	}
	if a.editor == nil {
		t.Fatalf("create should open the in-app editor")
	}
	if a.editor.pageName != "Zzz New Page" {
		t.Errorf("editor page: got %q, want %q", a.editor.pageName, "Zzz New Page")
	}
	if !a.editor.isNew {
		t.Errorf("a created page's editor should have isNew=true")
	}
	wantSuffix := filepath.Join("pages", "Zzz New Page.md")
	if !strings.HasSuffix(a.editor.path, wantSuffix) {
		t.Errorf("editor path: got %q, want suffix %q", a.editor.path, wantSuffix)
	}
}

func TestAppResizePropagatesToOverlays(t *testing.T) {
	a := bootApp(t)

	openers := []struct {
		name      string
		key       tea.KeyMsg
		setupPage string
		read      func() (int, int)
	}{
		{
			name: "picker",
			key:  tea.KeyMsg{Type: tea.KeyCtrlP},
			read: func() (int, int) { return a.active.(*Picker).width, a.active.(*Picker).height },
		},
		{
			name: "search",
			key:  tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")},
			read: func() (int, int) { return a.active.(*SearchView).width, a.active.(*SearchView).height },
		},
		{
			name:      "backlinks",
			key:       tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("b")},
			setupPage: "Hub",
			read:      func() (int, int) { return a.active.(*Backlinks).width, a.active.(*Backlinks).height },
		},
		{
			name: "todos",
			key:  tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("T")},
			read: func() (int, int) { return a.active.(*Todos).width, a.active.(*Todos).height },
		},
	}

	for _, o := range openers {
		t.Run(o.name, func(t *testing.T) {
			a.active = nil // reset to page mode each iteration
			if o.setupPage != "" {
				a.navigate(o.setupPage)
			}
			a.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
			a.Update(o.key)

			a.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
			w, h := o.read()
			if w != 120 || h != 40 {
				t.Errorf("%s: want 120x40 after resize, got %dx%d", o.name, w, h)
			}

			a.Update(tea.KeyMsg{Type: tea.KeyEsc})
		})
	}
}

// TestPickerCreate_DiscardLeavesNoFile pins the behavior when a user creates a
// page via the picker, types nothing, and immediately Esc-discards: the editor
// closes cleanly, no file is written (creation is deferred to save), and the
// app doesn't crash rendering the now-fileless page.
func TestPickerCreate_DiscardLeavesNoFile(t *testing.T) {
	a := bootApp(t)
	a.active = NewPicker(a.idx, a.width, a.height)
	for _, r := range "Zzz Throwaway" {
		a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(string(r))})
	}
	a.Update(tea.KeyMsg{Type: tea.KeyEnter}) // create -> editor opens
	if a.editor == nil {
		t.Fatalf("precondition: create should open the editor")
	}
	path := a.editor.path

	// Esc on a clean (untyped) buffer exits straight to the read view.
	a.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if a.editor != nil {
		t.Errorf("esc on a clean new-page buffer should close the editor")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("discarding a new page must not write a file; stat err=%v", err)
	}
	// Rendering the page (now naming a fileless page) must not panic.
	_ = a.View()
}
