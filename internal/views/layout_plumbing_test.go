package views

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/fiatcode-gh/weft/v2/internal/layout"
)

// layoutApp builds an App over a temp graph whose config is cfg ("" = no
// config file), with one anchor page so the app boots on a page.
func layoutApp(t *testing.T, cfg string) (string, *App) {
	t.Helper()
	quietTerm(t)
	dir := t.TempDir()
	if cfg != "" {
		path := filepath.Join(dir, ".weft", "config.toml")
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(cfg), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(filepath.Join(dir, "pages"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "pages", "Anchor.md"), []byte("anchor\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	lay, err := layout.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	a := New(dir, lay, "test")
	if cmd := a.Init(); cmd != nil {
		a.Update(cmd())
	}
	a.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	return dir, a
}

func TestEnterEditorPlacesNewPage(t *testing.T) {
	cases := []struct {
		name string
		cfg  string
		want string
	}{
		{"triple-lowbar default dir", "", "pages/kb___new.md"},
		{"folders default dir", "namespace_files = \"folders\"\n", "pages/kb/new.md"},
		{"triple-lowbar notes dir", "pages_dir = \"notes\"\n", "notes/kb___new.md"},
		{"folders notes dir", "pages_dir = \"notes\"\nnamespace_files = \"folders\"\n", "notes/kb/new.md"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// arrange
			dir, a := layoutApp(t, tc.cfg)
			a.navigate("kb/new")

			// act
			pressE(t, a)
			if a.editor == nil {
				t.Fatalf("e on a new page should open the editor; hint %q", a.hint)
			}
			a.Update(key("h"))
			a.Update(tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl})

			// assert
			got, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(tc.want)))
			if err != nil {
				t.Fatalf("saved file at %s: %v", tc.want, err)
			}
			if !strings.Contains(string(got), "h") {
				t.Errorf("content = %q, want it to contain the typed text", got)
			}
		})
	}
}

func TestEnterEditorRejectsUnplaceableName(t *testing.T) {
	// arrange
	dir, a := layoutApp(t, "namespace_files = \"folders\"\n")
	a.navigate("../escape")

	// act
	pressE(t, a)

	// assert
	if a.editor != nil {
		t.Fatal("editor opened for an unplaceable name")
	}
	if want := `cannot place page "../escape"`; !strings.Contains(a.hint, want) {
		t.Errorf("hint = %q, want it to contain %q", a.hint, want)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(dir), "escape.md")); !os.IsNotExist(err) {
		t.Errorf("a file outside the graph exists or stat failed unexpectedly: %v", err)
	}
}
