package views

import (
	"strings"
	"testing"
)

func TestNewHelpRendersVersion(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	view := NewHelp("v0.1.2").View()
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
	view := NewHelp("").View()
	if !strings.Contains(view, "? or esc to close") {
		t.Errorf("help view missing close hint; got:\n%s", view)
	}
	// The "peekseq " (with trailing space) prefix only appears as part of
	// the version segment. Empty version must not render it.
	if strings.Contains(view, "peekseq ") {
		t.Errorf("help view should omit version segment for empty version; got:\n%s", view)
	}
}
