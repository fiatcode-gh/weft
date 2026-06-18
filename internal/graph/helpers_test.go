package graph

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// fixturePath returns the absolute path to the shared testdata graph fixture.
func fixturePath(t *testing.T) string {
	t.Helper()
	p, err := filepath.Abs("../../testdata/fixture-graph")
	if err != nil {
		t.Fatalf("abs fixture path: %v", err)
	}
	return p
}

// buildFixtureIndex builds an Index over the shared testdata graph fixture.
func buildFixtureIndex(t *testing.T) *Index {
	t.Helper()
	idx, err := BuildIndex(fixturePath(t))
	if err != nil {
		t.Fatalf("BuildIndex: %v", err)
	}
	return idx
}

// buildTempIndex writes the given pages into a fresh temp graph (under pages/)
// and returns the built Index. Keys are file names relative to pages/ (e.g.
// "Alpha.md"); values are file bodies.
func buildTempIndex(t *testing.T, files map[string]string) *Index {
	t.Helper()
	dir := t.TempDir()
	pages := filepath.Join(dir, "pages")
	if err := os.MkdirAll(pages, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(pages, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	idx, err := BuildIndex(dir)
	if err != nil {
		t.Fatalf("BuildIndex: %v", err)
	}
	return idx
}

// mapReader returns a reader closure over a fixed map of file bodies, the shape
// FilterUnlinked expects. A missing path yields an error.
func mapReader(bodies map[string]string) func(string) (string, error) {
	return func(p string) (string, error) {
		b, ok := bodies[p]
		if !ok {
			return "", fmt.Errorf("no file %s", p)
		}
		return b, nil
	}
}
