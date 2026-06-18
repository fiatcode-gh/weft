package views

import (
	"os"
	"path/filepath"
	"testing"

	"git.fiatcode.dev/fiatcode/weft/v2/internal/graph"
)

// quietTerm forces a colourless, dumb terminal for the duration of the test so
// the package-wide lipgloss colour profile is not primed with truecolor by
// whichever test happens to run first. Every test that builds a view or boots
// the app relies on this for stable, ANSI-free output.
func quietTerm(t *testing.T) {
	t.Helper()
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
}

// writeGraph materialises a throwaway graph from name→content entries and
// returns its root and a freshly built index. Each key is a path relative to
// the graph root (e.g. "pages/Note.md" or "journals/2026_05_24.md"); parent
// directories are created as needed. Both the pages and journals directories
// are always created so the app can lazily write a journal stub into a graph
// that started without one.
func writeGraph(t *testing.T, files map[string]string) (string, *graph.Index) {
	t.Helper()
	dir := t.TempDir()
	for _, sub := range []string{"pages", "journals"} {
		if err := os.MkdirAll(filepath.Join(dir, sub), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for rel, content := range files {
		path := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	idx, err := graph.BuildIndex(dir)
	if err != nil {
		t.Fatal(err)
	}
	return dir, idx
}
