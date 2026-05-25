package views

import (
	"testing"

	"github.com/charmbracelet/x/exp/teatest"
)

func TestPaletteFiltersOnQuery(t *testing.T) {
	p := NewPalette(loadFixture(t))
	p.Update("a") // type 'a'
	p.Update("l")
	p.Update("p")
	teatest.RequireEqualOutput(t, []byte(p.View()))
}
