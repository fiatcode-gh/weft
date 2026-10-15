package graph

import (
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/fiatcode-gh/weft/v3/internal/layout"
)

// vaultPage returns the page of the read-only obsidian-vault fixture that
// name resolves to, and its index.
func vaultPage(t *testing.T, name string) (*Index, *PageMeta) {
	t.Helper()
	idx := vaultIndex(t)
	return idx, rewriteTarget(t, idx, name)
}

func TestRenameToRefusals(t *testing.T) {
	temp := func(files map[string]string, page string) func(*testing.T) (*Index, *PageMeta) {
		return func(t *testing.T) (*Index, *PageMeta) {
			idx := buildTempIndex(t, files)
			return idx, rewriteTarget(t, idx, page)
		}
	}
	vault := func(page string) func(*testing.T) (*Index, *PageMeta) {
		return func(t *testing.T) (*Index, *PageMeta) { return vaultPage(t, page) }
	}
	oldAlpha := temp(map[string]string{"Old.md": "x\n"}, "Old")
	tests := []struct {
		name  string
		setup func(*testing.T) (*Index, *PageMeta)
		to    string
		want  string
	}{
		{"journal", func(t *testing.T) (*Index, *PageMeta) {
			idx := clashIndex(t, "", "pages/x.md", "journals/2026_10_12.md")
			return idx, rewriteTarget(t, idx, "2026_10_12")
		}, "Anything", "journals cannot be renamed"},
		{"empty", oldAlpha, "", "new name is empty"},
		{"blank", oldAlpha, "  ", "new name is empty"},
		{"pipe", oldAlpha, "a|b", `a link cannot hold "|" in a page name`},
		{"bracket", oldAlpha, "a]b", `a link cannot hold "]" in a page name`},
		{"backtick", oldAlpha, "a`b", "a link cannot hold \"`\" in a page name"},
		{"control", oldAlpha, "a\tb", `a link cannot hold "\t" in a page name`},
		{"folders dotdot", vault("Home"), "a/../b", `cannot place page "a/../b": empty, "." or ".." path segment`},
		{"unchanged", oldAlpha, "Old", "name unchanged"},
		{"unchanged padded", oldAlpha, " Old ", "name unchanged"},
		{"same rel via triple lowbar", temp(map[string]string{"kb___notes.md": "x\n"}, "kb/notes"), "kb___notes", "name unchanged"},
		{"would be journal", vault("Ideas"), "2026-06-01", `"2026-06-01" would be a journal`},
		{"taken any case", temp(map[string]string{"Alpha.md": "x\n", "Beta.md": "x\n"}, "Beta"), "alpha", `"alpha" is already a page: Alpha`},
		{"taken via heading link", vault("Home"), "Alpha#Plan", `"Alpha#Plan" is already a page: Alpha`},
		{"fold-equal new name", vault("Home"), "x/Note", `"x/Note" is already a page: Note`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			idx, page := tt.setup(t)
			_, err := idx.RenameTo(page, tt.to)
			if err == nil || err.Error() != tt.want {
				t.Errorf("RenameTo(%q, %q) error = %v, want %q", page.Name, tt.to, err, tt.want)
			}
		})
	}
}

func TestRenameToCaseOnly(t *testing.T) {
	idx := buildTempIndex(t, map[string]string{"Alpha.md": "x\n"})
	got, err := idx.RenameTo(rewriteTarget(t, idx, "Alpha"), "alpha")
	if err != nil {
		t.Fatal(err)
	}
	if got.Key != "alpha" || got.Rel != "pages/alpha.md" {
		t.Errorf("got %+v, want Key alpha, Rel pages/alpha.md", got)
	}
}

func TestRenameToPlacement(t *testing.T) {
	idx := buildTempIndex(t, map[string]string{"kb___notes.md": "x\n"})
	got, err := idx.RenameTo(rewriteTarget(t, idx, "kb/notes"), "kb/Memo")
	if err != nil {
		t.Fatal(err)
	}
	wantPath := filepath.Join(idx.GraphPath, "pages", "kb___Memo.md")
	if got.Rel != "pages/kb___Memo.md" || got.Path != wantPath || got.Key != "kb/Memo" || got.Name != "kb/Memo" {
		t.Errorf("default layout: got %+v", got)
	}

	vidx, note := vaultPage(t, "a/Note")
	got, err = vidx.RenameTo(note, "kb/Notes")
	if err != nil {
		t.Fatal(err)
	}
	if got.Rel != "a/kb/Notes.md" || got.Key != "Notes" {
		t.Errorf("folders layout: got %+v, want Rel a/kb/Notes.md, Key Notes", got)
	}
}

func TestRenameToClashLoser(t *testing.T) {
	idx, loser := vaultPage(t, "b/Note")
	got, err := idx.RenameTo(loser, "Memo")
	if err != nil {
		t.Fatal(err)
	}
	if got.Rel != "b/Memo.md" || got.Key != "Memo" {
		t.Errorf("loser: got %+v, want Rel b/Memo.md, Key Memo", got)
	}
	winner, ok := idx.Resolve("a/Note")
	if !ok {
		t.Fatal("a/Note missing")
	}
	got, err = idx.RenameTo(winner, "Memo2")
	if err != nil {
		t.Fatalf("winner rename refused: %v", err)
	}
	if got.Rel != "a/Memo2.md" || got.Key != "Memo2" {
		t.Errorf("winner: got %+v", got)
	}
}

func copyTree(t *testing.T, src, dst string) {
	t.Helper()
	err := filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(target, b, 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestRenameRoundTripEveryPage(t *testing.T) {
	vault, err := filepath.Abs("../../testdata/obsidian-vault")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"fixture-graph", "obsidian-vault"} {
		src := fixturePath(t)
		if name == "obsidian-vault" {
			src = vault
		}
		lay, err := layout.Load(src)
		if err != nil {
			t.Fatal(err)
		}
		base, err := BuildIndex(src, lay)
		if err != nil {
			t.Fatal(err)
		}
		for _, p := range base.Pages {
			if p.IsJournal {
				continue
			}
			for _, newName := range []string{"Renamed Page", "renamed-page"} {
				t.Run(name+"/"+p.Rel+"/"+newName, func(t *testing.T) {
					roundTrip(t, src, lay, p.Rel, newName)
				})
			}
		}
	}
}

// counts is the per-page backlink count (by Rel) and the unresolved-target
// ref counts of idx.
func counts(idx *Index) (map[string]int, map[string]int) {
	pages := map[string]int{}
	for _, p := range idx.Pages {
		pages[p.Rel] = len(idx.linksTo[p.Path])
	}
	unres := map[string]int{}
	for k, refs := range idx.unresolved {
		unres[k] = len(refs)
	}
	return pages, unres
}

func roundTrip(t *testing.T, src string, lay layout.Layout, rel, newName string) {
	dir := t.TempDir()
	copyTree(t, src, dir)
	idx, err := BuildIndex(dir, lay)
	if err != nil {
		t.Fatal(err)
	}
	var page *PageMeta
	for i := range idx.Pages {
		if idx.Pages[i].Rel == rel {
			page = &idx.Pages[i]
		}
	}
	oldBack := len(idx.BacklinksTo(page.Key))
	oldPages, oldUnres := counts(idx)

	tgt, err := idx.RenameTo(page, newName)
	if err != nil {
		t.Fatalf("RenameTo refused: %v", err)
	}
	for _, f := range idx.Files() {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		out, n := idx.RewriteLinks(string(b), page, tgt.Key)
		if n > 0 {
			if err := os.WriteFile(f, []byte(out), 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := os.MkdirAll(filepath.Dir(tgt.Path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(page.Path, tgt.Path); err != nil {
		t.Fatal(err)
	}
	after, err := BuildIndex(dir, lay)
	if err != nil {
		t.Fatal(err)
	}
	got, ok := after.Resolve(tgt.Key)
	if !ok || got.Path != tgt.Path {
		t.Fatalf("Resolve(%q) = %v, %v; want the file at %s", tgt.Key, got, ok, tgt.Path)
	}
	if n := len(after.BacklinksTo(tgt.Key)); n != oldBack {
		t.Errorf("backlinks to renamed page = %d, was %d", n, oldBack)
	}
	newPages, newUnres := counts(after)
	for r, n := range oldPages {
		if r == rel {
			continue
		}
		if newPages[r] != n {
			t.Errorf("backlinks to %s = %d, was %d", r, newPages[r], n)
		}
	}
	if !slices.Equal(sortedKV(oldUnres), sortedKV(newUnres)) {
		t.Errorf("unresolved targets changed: %v -> %v", oldUnres, newUnres)
	}
}

func sortedKV(m map[string]int) []string {
	var out []string
	for k, v := range m {
		out = append(out, k+"\x00"+strings.Repeat("x", v))
	}
	slices.Sort(out)
	return out
}
