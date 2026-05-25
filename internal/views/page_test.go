package views

import (
	"path/filepath"
	"testing"

	"github.com/charmbracelet/x/exp/teatest"

	"git.fiatcode.dev/fiatcode/peekseq/internal/graph"
)

func loadFixture(t *testing.T) *graph.Index {
	t.Helper()
	abs, err := filepath.Abs("../../testdata/fixture-graph")
	if err != nil {
		t.Fatal(err)
	}
	idx, err := graph.BuildIndex(abs)
	if err != nil {
		t.Fatal(err)
	}
	return idx
}

func TestPageViewRendersAlpha(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	idx := loadFixture(t)
	pv := NewPageView(idx, "Alpha", 80, 24)
	teatest.RequireEqualOutput(t, []byte(pv.View()))
}

func TestPageViewOffsetCursorAccessors(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	idx := loadFixture(t)
	// Small viewport so HalfPageDown actually moves YOffset against
	// Alpha's ~10 styled lines of content.
	pv := NewPageView(idx, "Alpha", 80, 5)

	if got := pv.Offset(); got != 0 {
		t.Errorf("fresh Offset: want 0, got %d", got)
	}
	if got := pv.Cursor(); got != -1 {
		t.Errorf("fresh Cursor: want -1, got %d", got)
	}

	pv.CycleLink(+1)
	if got := pv.Cursor(); got != 0 {
		t.Errorf("after CycleLink(+1): want cursor 0, got %d", got)
	}

	pv.HalfPageDown()
	if got := pv.Offset(); got == 0 {
		t.Errorf("after HalfPageDown: expected non-zero offset, got 0")
	}
}

func TestPageViewRestoreRoundtrip(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	idx := loadFixture(t)
	pv := NewPageView(idx, "Alpha", 80, 5)

	pv.Restore(2, 1)
	if got := pv.Offset(); got != 2 {
		t.Errorf("Restore offset: want 2, got %d", got)
	}
	if got := pv.Cursor(); got != 1 {
		t.Errorf("Restore cursor: want 1, got %d", got)
	}
}

func TestPageViewRestoreClampsCursor(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	idx := loadFixture(t)
	pv := NewPageView(idx, "Alpha", 80, 24)

	// Cursor past the link count falls back to -1.
	pv.Restore(0, 9999)
	if got := pv.Cursor(); got != -1 {
		t.Errorf("over-range cursor: want -1, got %d", got)
	}

	// Negative cursor below -1 falls back to -1.
	pv.Restore(0, -5)
	if got := pv.Cursor(); got != -1 {
		t.Errorf("under-range cursor: want -1, got %d", got)
	}
}
