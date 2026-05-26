package views

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestPageEdgeKeys(t *testing.T) {
	a := bootApp(t)
	// Navigate to Alpha so we have real content that can scroll. Boot
	// page (today's journal) is typically empty in the fixture.
	a.navigate("Alpha")

	// Shrink the viewport so Alpha is scrollable.
	a.Update(tea.WindowSizeMsg{Width: 80, Height: 5})

	// 'G' jumps to bottom.
	a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("G")})
	bottomOffset := a.page.Offset()
	if bottomOffset == 0 {
		t.Errorf("after G: expected non-zero offset, got 0")
	}

	// 'g' jumps back to top.
	a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("g")})
	if got := a.page.Offset(); got != 0 {
		t.Errorf("after g: want offset 0, got %d", got)
	}
}

func TestAppStatusBarContainsScrollIndicator(t *testing.T) {
	a := bootApp(t)
	a.navigate("Alpha")
	// Shrink the viewport so Alpha is scrollable.
	a.Update(tea.WindowSizeMsg{Width: 80, Height: 5})

	a.page.GotoBottom()

	bar := a.statusBar()
	if !strings.Contains(bar, "100%") {
		t.Errorf("status bar missing 100%% indicator; got:\n%s", bar)
	}
	if !strings.Contains(bar, "? help") {
		t.Errorf("status bar missing close hint; got:\n%s", bar)
	}
}

func TestAppStatusBarHidesIndicatorWhenFits(t *testing.T) {
	a := bootApp(t)
	a.navigate("Alpha")
	// Force a very tall viewport so Alpha is guaranteed to fit regardless
	// of future content changes.
	a.Update(tea.WindowSizeMsg{Width: 80, Height: 200})

	bar := a.statusBar()
	if strings.Contains(bar, "%") {
		t.Errorf("status bar should hide percentage when page fits; got:\n%s", bar)
	}
	if !strings.Contains(bar, "? help") {
		t.Errorf("status bar missing close hint; got:\n%s", bar)
	}
}

func TestAppLoadingSplashBeforePage(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	a := New("/nonexistent/before/build", "test")
	v := a.View()
	if !strings.Contains(v, "peekseq") {
		t.Errorf("splash should contain title; got:\n%s", v)
	}
	if !strings.Contains(v, "Loading") {
		t.Errorf("splash should announce loading; got:\n%s", v)
	}
}

func TestAppErrorSplashOnIndexFailure(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	a := New("/some/graph", "test")
	a.Update(indexLoadedMsg{err: errors.New("synthetic build failure")})
	v := a.View()
	if !strings.Contains(v, "failed to index") {
		t.Errorf("error splash should announce failure; got:\n%s", v)
	}
	if !strings.Contains(v, "R to retry") {
		t.Errorf("error splash should show retry hint; got:\n%s", v)
	}
}

func TestAppRetryFromErrorSplash(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	a := New("/some/graph", "test")
	a.Update(indexLoadedMsg{err: errors.New("synthetic build failure")})
	if a.loadErr == nil {
		t.Fatalf("setup: loadErr should be set")
	}

	_, cmd := a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("R")})
	if cmd == nil {
		t.Fatalf("R from error splash should return a retry cmd")
	}
	if a.loadErr != nil {
		t.Errorf("R should clear loadErr before rebuilding, got %v", a.loadErr)
	}
}

func TestAppQuitsBeforePage(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	a := New("/no/such/path", "test")
	_, cmd := a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	if cmd == nil {
		t.Errorf("q before page should return Quit cmd, got nil")
	}
}

func TestPageNameFromHitPath(t *testing.T) {
	idx := loadFixture(t)
	var sample, name string
	for _, p := range idx.Pages {
		sample, name = p.Path, p.Name
		break
	}
	if got := pageNameFromHitPath(idx, sample); got != name {
		t.Errorf("known path: want %q, got %q", name, got)
	}
	if got := pageNameFromHitPath(idx, "/totally/unknown/file.md"); got != "" {
		t.Errorf("unknown path: want \"\", got %q", got)
	}
}

func TestAppPageKeyDispatch(t *testing.T) {
	a := bootApp(t)
	a.navigate("Alpha")
	a.Update(tea.WindowSizeMsg{Width: 80, Height: 5}) // scrollable

	a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	if a.page.Cursor() < 0 {
		t.Errorf("n should advance cursor from -1, got %d", a.page.Cursor())
	}

	a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("N")})
	a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	afterJ := a.page.Offset()
	if afterJ == 0 {
		t.Fatalf("setup: j should advance offset, got 0")
	}

	a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	if a.page.Offset() >= afterJ {
		t.Errorf("k should retreat from %d, got %d", afterJ, a.page.Offset())
	}

	a.Update(tea.KeyMsg{Type: tea.KeyCtrlD})
	afterCtrlD := a.page.Offset()
	if afterCtrlD == 0 {
		t.Fatalf("setup: ctrl+d should advance offset, got 0")
	}
	a.Update(tea.KeyMsg{Type: tea.KeyCtrlU})
	if a.page.Offset() >= afterCtrlD {
		t.Errorf("ctrl+u should retreat from %d, got %d", afterCtrlD, a.page.Offset())
	}
}

func TestAppEnterFollowsLink(t *testing.T) {
	a := bootApp(t)
	a.navigate("Alpha")
	a.page.CycleLink(+1)
	target := a.page.FollowCursor()
	if target == "" {
		t.Fatalf("setup: no link target available")
	}

	a.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if a.page.Page() != target {
		t.Errorf("enter should navigate to %q, got %q", target, a.page.Page())
	}
}

func TestAppReindexFromPage(t *testing.T) {
	a := bootApp(t)
	_, cmd := a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("R")})
	if cmd == nil {
		t.Errorf("R from page mode should return a reindex cmd")
	}
}
