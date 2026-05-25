package views

import (
	"path/filepath"
	"regexp"
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

func TestPageViewGotoTopBottom(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	idx := loadFixture(t)
	// Small viewport so Alpha (~10 styled lines) is scrollable.
	pv := NewPageView(idx, "Alpha", 80, 5)

	// Scroll a bit so GotoTop has somewhere to go back to.
	pv.HalfPageDown()
	if pv.Offset() == 0 {
		t.Fatalf("setup: HalfPageDown should have advanced offset; got 0")
	}

	pv.GotoTop()
	if got := pv.Offset(); got != 0 {
		t.Errorf("after GotoTop: want offset 0, got %d", got)
	}

	pv.GotoBottom()
	if got := pv.Offset(); got == 0 {
		t.Errorf("after GotoBottom: expected non-zero offset, got 0")
	}
}

func TestPageViewScrollIndicatorFitsViewport(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	idx := loadFixture(t)
	// Tall viewport so Alpha fits entirely — no scroll possible.
	pv := NewPageView(idx, "Alpha", 80, 100)

	if got := pv.ScrollIndicator(); got != "" {
		t.Errorf("non-scrollable indicator: want \"\", got %q", got)
	}
}

func TestPageViewScrollIndicatorTopBottom(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	idx := loadFixture(t)
	// Small viewport so Alpha is scrollable.
	pv := NewPageView(idx, "Alpha", 80, 5)

	pv.GotoTop()
	if got := pv.ScrollIndicator(); got != "0%" {
		t.Errorf("at top: want \"0%%\", got %q", got)
	}

	pv.GotoBottom()
	if got := pv.ScrollIndicator(); got != "100%" {
		t.Errorf("at bottom: want \"100%%\", got %q", got)
	}
}

func TestPageViewScrollIndicatorMid(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	idx := loadFixture(t)
	// Small viewport against Alpha's ~10 styled lines so a single
	// HalfPageDown from the top lands at a non-boundary scroll position.
	pv := NewPageView(idx, "Alpha", 80, 5)

	pv.GotoTop()
	pv.HalfPageDown()

	got := pv.ScrollIndicator()
	matched, err := regexp.MatchString(`^\d{1,2}%$`, got)
	if err != nil {
		t.Fatalf("regex error: %v", err)
	}
	if !matched {
		t.Errorf("mid-scroll: want NN%% (1-2 digits), got %q", got)
	}
	// Belt-and-braces: it should be a real mid-scroll value, not a boundary.
	if got == "0%" || got == "100%" {
		t.Errorf("mid-scroll landed on a boundary: %q", got)
	}
}
