package views

import (
	"testing"

	"github.com/charmbracelet/x/exp/teatest"
)

func TestPaletteFiltersOnQuery(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	p := NewPalette(loadFixture(t))
	p.Update("a") // type 'a'
	p.Update("l")
	p.Update("p")
	teatest.RequireEqualOutput(t, []byte(p.View()))
}
