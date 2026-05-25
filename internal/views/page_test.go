package views

import (
	"path/filepath"
	"testing"

	"github.com/charmbracelet/x/exp/teatest"

	"github.com/fiatcode/logseq-tui/internal/graph"
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
