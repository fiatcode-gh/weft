package views

import (
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// bootApp returns an App that has loaded the fixture index and initialised
// its PageView at a known size. Exposed for history-related tests; mirrors
// the production boot sequence (index load -> WindowSizeMsg -> tryInitPage)
// minus the async hop.
func bootApp(t *testing.T) *App {
	t.Helper()
	// Match page_test.go's environment so the package-wide lipgloss color
	// profile is not primed with truecolor by whichever test runs first.
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	abs, err := filepath.Abs("../../testdata/fixture-graph")
	if err != nil {
		t.Fatal(err)
	}
	a := New(abs, "test")
	// Drive the deferred index build synchronously.
	cmd := a.Init()
	if cmd == nil {
		t.Fatal("Init returned nil cmd")
	}
	msg := cmd()
	a.Update(msg)
	// Provide a real terminal size so tryInitPage can construct PageView.
	a.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	if a.page == nil {
		t.Fatal("PageView not constructed after boot")
	}
	return a
}
