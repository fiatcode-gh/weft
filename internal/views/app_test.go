package views

import (
	"path/filepath"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func TestTodayJournalNameUsesNowFunc(t *testing.T) {
	a := New("/unused", "test")
	a.nowFunc = func() time.Time { return time.Date(2026, 5, 23, 12, 0, 0, 0, time.UTC) }
	if got := a.todayJournalName(); got != "2026-05-23" {
		t.Errorf("todayJournalName: want 2026-05-23, got %q", got)
	}
}

// bootAppAt is like bootApp but pins App.nowFunc before the WindowSizeMsg so
// tryInitPage seeds history with a deterministic page name. Use for tests
// that exercise the . / < / > keys against the fixture.
func bootAppAt(t *testing.T, now time.Time) *App {
	t.Helper()
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	abs, err := filepath.Abs("../../testdata/fixture-graph")
	if err != nil {
		t.Fatal(err)
	}
	a := New(abs, "test")
	a.nowFunc = func() time.Time { return now }
	cmd := a.Init()
	if cmd == nil {
		t.Fatal("Init returned nil cmd")
	}
	msg := cmd()
	a.Update(msg)
	a.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	if a.page == nil {
		t.Fatal("PageView not constructed after boot")
	}
	return a
}

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
