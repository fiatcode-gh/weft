package graph

import (
	"os"
	"path/filepath"
	"slices"
	"sort"
	"testing"

	"github.com/fiatcode-gh/weft/v2/internal/layout"
)

// writeTree writes files (graph-relative slash paths → body) under dir and
// returns the layout loaded from dir.
func writeTree(t *testing.T, dir string, files map[string]string) layout.Layout {
	t.Helper()
	for rel, body := range files {
		path := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	lay, err := layout.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	return lay
}

func rels(idx *Index) []string {
	out := make([]string, 0, len(idx.Pages))
	for _, p := range idx.Pages {
		out = append(out, p.Rel)
	}
	return out
}

func TestBuildIndexWalksConfiguredFolders(t *testing.T) {
	// arrange
	dir := t.TempDir()
	lay := writeTree(t, dir, map[string]string{
		".weft/config.toml":         "pages_dir = \"notes\"\njournals_dir = \"notes/daily\"\n",
		"notes/A.md":                "a\n",
		"notes/sub/B.md":            "b\n",
		"notes/.obsidian/C.md":      "c\n",
		"notes/daily/2026_05_24.md": "j\n",
		"notes/x.txt":               "x\n",
		"pages/D.md":                "d\n",
	})

	// act
	idx, err := BuildIndex(dir, lay)

	// assert
	if err != nil {
		t.Fatal(err)
	}
	got := rels(idx)
	sort.Strings(got)
	want := []string{"notes/A.md", "notes/daily/2026_05_24.md", "notes/sub/B.md"}
	if !slices.Equal(got, want) {
		t.Fatalf("rels = %v, want %v", got, want)
	}
	if b, ok := idx.ByName["B"]; !ok || b.Rel != "notes/sub/B.md" {
		t.Errorf("ByName[B] = %+v, want Rel notes/sub/B.md", b)
	}
	if len(idx.Warnings) != 0 {
		t.Errorf("warnings = %v, want none", idx.Warnings)
	}
	j := idx.Pages[slices.IndexFunc(idx.Pages, func(p PageMeta) bool { return p.Rel == "notes/daily/2026_05_24.md" })]
	if !j.IsJournal {
		t.Error("notes/daily/2026_05_24.md should be a journal")
	}
	if j.Path != filepath.Join(dir, "notes", "daily", "2026_05_24.md") {
		t.Errorf("Path = %q", j.Path)
	}
}

func TestBuildIndexRootPages(t *testing.T) {
	// arrange
	dir := t.TempDir()
	lay := writeTree(t, dir, map[string]string{
		".weft/config.toml": "pages_dir = \"\"\njournals_dir = \"daily\"\n",
		"Home.md":           "h\n",
		"a/Note.md":         "n\n",
		".git/x.md":         "x\n",
		".trash/y.md":       "y\n",
	})

	// act
	idx, err := BuildIndex(dir, lay)

	// assert
	if err != nil {
		t.Fatal(err)
	}
	got := rels(idx)
	sort.Strings(got)
	if want := []string{"Home.md", "a/Note.md"}; !slices.Equal(got, want) {
		t.Fatalf("rels = %v, want %v", got, want)
	}
}

func TestBuildIndexRecursesDefaultFolders(t *testing.T) {
	// arrange
	dir := t.TempDir()
	lay := writeTree(t, dir, map[string]string{"pages/sub/nested.md": "n\n"})

	// act
	idx, err := BuildIndex(dir, lay)

	// assert
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := idx.ByName["nested"]; !ok {
		t.Errorf("nested not indexed: %v", rels(idx))
	}
	if len(idx.Warnings) != 0 {
		t.Errorf("warnings = %v, want none", idx.Warnings)
	}
}

func TestBuildIndexDefaultWalkOrder(t *testing.T) {
	// arrange
	fixture := fixturePath(t)
	var want []string
	for _, sub := range []string{"pages", "journals"} {
		entries, err := os.ReadDir(filepath.Join(fixture, sub))
		if err != nil {
			t.Fatal(err)
		}
		var names []string
		for _, e := range entries {
			if filepath.Ext(e.Name()) == ".md" {
				names = append(names, sub+"/"+e.Name())
			}
		}
		sort.Strings(names)
		want = append(want, names...)
	}

	// act
	idx, err := BuildIndex(fixture, layout.Default())

	// assert
	if err != nil {
		t.Fatal(err)
	}
	if got := rels(idx); !slices.Equal(got, want) {
		t.Fatalf("rels = %v, want %v", got, want)
	}
	if len(want) != 19 {
		t.Errorf("fixture has %d pages, plan expects 10 + 9", len(want))
	}
	for _, p := range idx.Pages {
		if p.Path != filepath.Join(fixture, filepath.FromSlash(p.Rel)) {
			t.Errorf("Path %q does not match Rel %q", p.Path, p.Rel)
		}
	}
}

func TestBuildIndexMissingRootsIsEmpty(t *testing.T) {
	// act
	idx, err := BuildIndex(t.TempDir(), layout.Default())

	// assert
	if err != nil {
		t.Fatal(err)
	}
	if len(idx.Pages) != 0 {
		t.Errorf("pages = %v, want none", rels(idx))
	}
}

func TestBuildIndexRootThatIsAFileFails(t *testing.T) {
	// arrange
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "pages"), []byte("not a dir"), 0o644); err != nil {
		t.Fatal(err)
	}

	// act
	_, err := BuildIndex(dir, layout.Default())

	// assert
	if err == nil {
		t.Fatal("want an error for a regular file in place of pages/")
	}
}

func TestNewPagePath(t *testing.T) {
	// arrange
	dir := t.TempDir()
	lay := writeTree(t, dir, map[string]string{".weft/config.toml": "namespace_files = \"folders\"\n"})
	idx, err := BuildIndex(dir, lay)
	if err != nil {
		t.Fatal(err)
	}

	// act
	got, err := idx.NewPagePath("kb/new")
	_, bad := idx.NewPagePath("../x")

	// assert
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(dir, "pages", "kb", "new.md"); got != want {
		t.Errorf("NewPagePath = %q, want %q", got, want)
	}
	if bad == nil {
		t.Error("NewPagePath(../x) should fail in folders mode")
	}
}
