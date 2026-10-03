package views

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/fiatcode-gh/weft/v2/internal/graph"
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

// cloneFixtureGraph copies testdata/fixture-graph into a fresh temp dir
// so tests can create journals and edit pages without dirtying the repo
// checkout (and can run in parallel).
func cloneFixtureGraph(t *testing.T) string {
	t.Helper()
	src, err := filepath.Abs("../../testdata/fixture-graph")
	if err != nil {
		t.Fatal(err)
	}
	dst := t.TempDir()
	err = filepath.WalkDir(src, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		out := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(out, 0o755)
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(out, b, 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
	return dst
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
