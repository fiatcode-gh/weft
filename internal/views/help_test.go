package views

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/exp/teatest/v2"
)

func TestNewHelpRendersVersion(t *testing.T) {
	quietTerm(t)
	view := plain(NewHelp("v0.1.2", 80, 0).View())
	if !strings.Contains(view, "weft v0.1.2") {
		t.Errorf("help view missing version segment; got:\n%s", view)
	}
	if !strings.Contains(view, "? or esc to close") {
		t.Errorf("help view missing close hint; got:\n%s", view)
	}
}

func TestNewHelpEmptyVersionHidesSegment(t *testing.T) {
	quietTerm(t)
	view := plain(NewHelp("", 80, 0).View())
	if !strings.Contains(view, "? or esc to close") {
		t.Errorf("help view missing close hint; got:\n%s", view)
	}
	// The "weft " (with trailing space) prefix only appears as part of
	// the version segment. Empty version must not render it.
	if strings.Contains(view, "weft ") {
		t.Errorf("help view should omit version segment for empty version; got:\n%s", view)
	}
}

// TestHelpRespectsNarrowWidth asserts that the help panel doesn't exceed
// the terminal width even when the natural content (e.g. "ctrl-p ...
// picker — find any page") would otherwise be wider.
func TestHelpRespectsNarrowWidth(t *testing.T) {
	quietTerm(t)
	const w = 30
	view := plain(NewHelp("v0.1.2", w, 0).View())
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
	quietTerm(t)
	view := plain(NewHelp("v0.1.2", 0, 0).View())
	// With no width constraint the panel renders two content-sized columns,
	// far wider than 30 cells.
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
	quietTerm(t)
	// Width 130 is wide enough to exercise the two-column layout; height 0
	// disables the height cap so the golden captures the full panel.
	h := NewHelp("v1.0.0", 130, 0)
	teatest.RequireEqualOutput(t, []byte(plain(h.View())))
}

func TestHelpSetSizeUpdatesWidth(t *testing.T) {
	quietTerm(t)
	h := NewHelp("v1.0.0", 80, 0)
	h.SetSize(30, 10) // shrink below natural content width
	for _, line := range strings.Split(plain(h.View()), "\n") {
		if lipgloss.Width(line) > 30 {
			t.Errorf("after SetSize(30,_) line exceeds 30: w=%d line=%q",
				lipgloss.Width(line), line)
		}
	}
}

// TestHelpHeightGuardClipsToTerminal asserts the panel never renders taller
// than the terminal: body lines past the cap are dropped, the close hint
// stays visible, and a "resize" indicator signals the truncation.
func TestHelpHeightGuardClipsToTerminal(t *testing.T) {
	quietTerm(t)
	const ht = 12
	view := plain(NewHelp("v1.0.0", 130, ht).View())
	if n := len(strings.Split(view, "\n")); n > ht {
		t.Errorf("panel taller than terminal: %d lines > height %d:\n%s", n, ht, view)
	}
	if !strings.Contains(view, "? or esc to close") {
		t.Errorf("close hint must survive the height clip; got:\n%s", view)
	}
	if !strings.Contains(view, "resize") {
		t.Errorf("clipped panel should show a resize indicator; got:\n%s", view)
	}
}

func TestHelpQClosesOverlay(t *testing.T) {
	h := NewHelp("v1.0.0", 80, 0)
	if res := h.Update("q"); res.kind != overlayResultCancel {
		t.Errorf("q should close the help overlay; got %+v", res)
	}
}

func TestHelpUnboundKeyIsNoop(t *testing.T) {
	h := NewHelp("v1.0.0", 80, 0)
	if res := h.Update("x"); res.kind != overlayResultNone {
		t.Errorf("unbound key should be a no-op; got %+v", res)
	}
}
