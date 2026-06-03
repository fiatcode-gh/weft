package views

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/exp/teatest"
)

func TestNewHelpRendersVersion(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	view := NewHelp("v0.1.2", 80).View()
	if !strings.Contains(view, "peekseq v0.1.2") {
		t.Errorf("help view missing version segment; got:\n%s", view)
	}
	if !strings.Contains(view, "? or esc to close") {
		t.Errorf("help view missing close hint; got:\n%s", view)
	}
}

func TestNewHelpEmptyVersionHidesSegment(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	view := NewHelp("", 80).View()
	if !strings.Contains(view, "? or esc to close") {
		t.Errorf("help view missing close hint; got:\n%s", view)
	}
	// The "peekseq " (with trailing space) prefix only appears as part of
	// the version segment. Empty version must not render it.
	if strings.Contains(view, "peekseq ") {
		t.Errorf("help view should omit version segment for empty version; got:\n%s", view)
	}
}

// TestHelpRespectsNarrowWidth asserts that the help panel doesn't exceed
// the terminal width even when the natural content (e.g. "ctrl-p ...
// picker — find any page") would otherwise be wider.
func TestHelpRespectsNarrowWidth(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	const w = 30
	view := NewHelp("v0.1.2", w).View()
	for _, line := range strings.Split(view, "\n") {
		if lipgloss.Width(line) > w {
			t.Errorf("line exceeds width %d: w=%d, line=%q", w, lipgloss.Width(line), line)
		}
	}
}

// TestHelpZeroWidthDisablesClamp checks the documented escape hatch:
// passing width=0 leaves the panel content-sized (used by tests that
// don't care about width).
func TestHelpZeroWidthDisablesClamp(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	view := NewHelp("v0.1.2", 0).View()
	// At least one body row is "ctrl-p" + ~10 padding + "picker — find
	// any page" ≈ 35 cells; with no clamp the panel ends up wider than 30.
	max := 0
	for _, line := range strings.Split(view, "\n") {
		if w := lipgloss.Width(line); w > max {
			max = w
		}
	}
	if max < 30 {
		t.Errorf("width=0 should keep content-sized panel; max line width %d looks clamped", max)
	}
}

func TestHelpGolden(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	h := NewHelp("v1.0.0", 80)
	teatest.RequireEqualOutput(t, []byte(h.View()))
}

func TestHelpSetSizeUpdatesWidth(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	h := NewHelp("v1.0.0", 80)
	h.SetSize(30, 10) // shrink below natural content width
	for _, line := range strings.Split(h.View(), "\n") {
		if lipgloss.Width(line) > 30 {
			t.Errorf("after SetSize(30,_) line exceeds 30: w=%d line=%q",
				lipgloss.Width(line), line)
		}
	}
}

func TestHelpQClosesOverlay(t *testing.T) {
	h := NewHelp("v1.0.0", 80)
	if res := h.Update("q"); !res.Cancel {
		t.Errorf("q should close the help overlay; got %+v", res)
	}
}

func TestHelpUnboundKeyIsNoop(t *testing.T) {
	h := NewHelp("v1.0.0", 80)
	if res := h.Update("x"); res.Accept || res.Cancel {
		t.Errorf("unbound key should be a no-op; got %+v", res)
	}
}
