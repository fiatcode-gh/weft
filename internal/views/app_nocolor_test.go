package views

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/colorprofile"

	"github.com/fiatcode-gh/weft/v2/internal/render"
)

// NO_COLOR drops colour, never attributes: every screen keeps something bold
// and no cell keeps a colour once the renderer converts it with weft's profile.
func TestNoColorKeepsAttributesEveryScreen(t *testing.T) {
	a := bootApp(t, bootConfig{files: map[string]string{
		"pages/Home.md":  "# Big heading\n\n- TODO a task\n- see [[Home]]\n",
		"pages/Other.md": "- other\n",
	}})
	a.navigate("Home")

	screens := []struct {
		name string
		open func()
	}{
		{"read view", func() {}},
		{"picker", func() { a.Update(tea.KeyPressMsg{Code: 'p', Mod: tea.ModCtrl}) }},
		{"help", func() { a.Update(key("?")) }},
		{"todos", func() { a.Update(key("T")) }},
	}
	for _, sc := range screens {
		a.Update(tea.KeyPressMsg{Code: tea.KeyEsc}) // back to the page
		sc.open()

		p := render.ColorProfile(nil)
		if p != colorprofile.Ascii {
			t.Fatalf("%s: ColorProfile = %v, want Ascii under NO_COLOR", sc.name, p)
		}
		rows := frameCells(a.View().Content, 80, p)
		assertAnyBold(t, rows, sc.name)
		assertNoColour(t, rows, sc.name)
	}
}
