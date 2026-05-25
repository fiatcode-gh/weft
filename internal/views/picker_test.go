package views

import (
	"testing"
	"time"

	"github.com/charmbracelet/x/exp/teatest"
)

func TestPickerFiltersOnQuery(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	p := NewPicker(loadFixture(t), 80, 30)
	// Zero out mtimes so the relative-time hint doesn't drift with
	// checkout time. relativeTime returns "" for a zero mtime.
	for i := range p.choices {
		p.choices[i].mtime = time.Time{}
	}
	p.Update("a") // type 'a'
	p.Update("l")
	p.Update("p")
	teatest.RequireEqualOutput(t, []byte(p.View()))
}
