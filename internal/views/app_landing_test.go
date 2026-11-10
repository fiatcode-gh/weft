package views

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func numberedPage(n int) string {
	var sb strings.Builder
	for i := 1; i <= n; i++ {
		fmt.Fprintf(&sb, "- line %d\n", i)
	}
	return sb.String()
}

// The place a saved exit hands to the reindex belongs to the page it was
// saved from: a page opened before that reindex lands starts at the top.
func TestSavedExitPlacementDoesNotFollowNavigation(t *testing.T) {
	navs := []struct {
		name  string
		leave func(a *App)
	}{
		{"navigate", func(a *App) { a.navigate("Other") }},
		{"history forward", func(a *App) {
			a.navigate("Other")
			a.historyBack()
			a.historyForward()
		}},
	}
	for _, tc := range navs {
		t.Run(tc.name, func(t *testing.T) {
			a := bootApp(t, bootConfig{files: map[string]string{
				"pages/Long.md":  numberedPage(120),
				"pages/Other.md": numberedPage(120),
			}})
			a.navigate("Long")
			for range 80 {
				a.Update(key("j"))
			}
			if a.page.Offset() == 0 {
				t.Fatal("precondition: moving down did not scroll Long")
			}
			a.Update(key("e"))
			typeApp(a, "x")
			a.Update(tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl})
			_, cmd := a.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
			if a.editor != nil {
				t.Fatal("Esc did not close the editor")
			}
			loaded := drainFor[indexLoadedMsg](t, cmd)

			tc.leave(a)
			a.Update(loaded)

			if got := a.page.Page(); got != "Other" {
				t.Fatalf("page = %q, want Other", got)
			}
			if got := a.page.Offset(); got != 0 {
				t.Errorf("Other opened at offset %d, want the top: Long's place leaked", got)
			}
			if a.pendingPlace != nil {
				t.Error("placement left pending after the load landed")
			}
		})
	}
}

// Without navigation the placement still lands on the page it was saved from.
func TestSavedExitPlacementStillLandsOnSamePage(t *testing.T) {
	a := bootApp(t, bootConfig{files: map[string]string{"pages/Long.md": numberedPage(120)}})
	a.navigate("Long")
	for range 80 {
		a.Update(key("j"))
	}
	off := a.page.Offset()
	a.Update(key("e"))
	typeApp(a, "x")
	a.Update(tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl})
	_, cmd := a.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	a.Update(drainFor[indexLoadedMsg](t, cmd))

	if got := a.page.Offset(); got != off {
		t.Errorf("Long offset after save-exit = %d, want %d", got, off)
	}
}

const ghostSource = "pages/Source.md"

func ghostGraph() map[string]string {
	return map[string]string{
		ghostSource: numberedPage(60) + "- see [[ghost]]\n" + numberedPage(10),
	}
}

func requireOnGhostLink(t *testing.T, a *App) {
	t.Helper()
	if got := a.page.Page(); got != "Source" {
		t.Fatalf("page = %q, want Source", got)
	}
	if a.page.Cursor() < 0 {
		t.Fatal("no link focused on the source page")
	}
	if got := a.page.FollowCursor(); !strings.EqualFold(got, "ghost") {
		t.Errorf("focused link = %q, want the ghost link", got)
	}
	if a.page.Offset() == 0 {
		t.Error("source page left at the top; the focused link is off screen")
	}
}

// A backlink of a page with no file still focuses the link it came from.
func TestBacklinksOverlayFocusesLinkToUnresolvedPage(t *testing.T) {
	a := bootApp(t, bootConfig{files: ghostGraph()})
	a.navigate("Ghost")
	a.Update(key("b"))
	a.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	requireOnGhostLink(t, a)
}

func TestSidebarBacklinksFocusLinkToUnresolvedPage(t *testing.T) {
	a := bootApp(t, bootConfig{files: ghostGraph()})
	a.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	a.navigate("Ghost")
	focusSection(t, a, "2")
	a.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	requireOnGhostLink(t, a)
}
