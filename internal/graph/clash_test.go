package graph

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"

	"github.com/fiatcode-gh/weft/v3/internal/layout"
	"github.com/fiatcode-gh/weft/v3/internal/search"
)

func vaultIndex(t *testing.T) *Index {
	t.Helper()
	dir, err := filepath.Abs("../../testdata/obsidian-vault")
	if err != nil {
		t.Fatal(err)
	}
	lay, err := layout.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	idx, err := BuildIndex(dir, lay)
	if err != nil {
		t.Fatal(err)
	}
	return idx
}

// clashIndex builds a graph from graph-relative files (empty bodies) under
// the config text cfg ("" = default layout).
func clashIndex(t *testing.T, cfg string, rels ...string) *Index {
	t.Helper()
	files := map[string]string{}
	for _, r := range rels {
		files[r] = "- x\n"
	}
	if cfg != "" {
		files[layout.ConfigFile] = cfg
	}
	dir := t.TempDir()
	lay := writeTree(t, dir, files)
	idx, err := BuildIndex(dir, lay)
	if err != nil {
		t.Fatal(err)
	}
	return idx
}

func resolveRel(t *testing.T, idx *Index, target string) string {
	t.Helper()
	p, ok := idx.Resolve(target)
	if !ok {
		t.Fatalf("Resolve(%q) failed", target)
	}
	return p.Rel
}

func keyOf(t *testing.T, idx *Index, rel string) string {
	t.Helper()
	for _, p := range idx.Pages {
		if p.Rel == rel {
			return p.Key
		}
	}
	t.Fatalf("no page %s", rel)
	return ""
}

func TestVaultIndex(t *testing.T) {
	idx := vaultIndex(t)

	var pages, journals []string
	for _, p := range idx.Pages {
		if p.IsJournal {
			journals = append(journals, p.Name)
			if p.Date.IsZero() {
				t.Errorf("journal %s has no date", p.Name)
			}
		} else {
			pages = append(pages, p.Name)
		}
		if p.Rel[0] == '.' {
			t.Errorf("indexed hidden-folder file %s", p.Rel)
		}
	}
	slices.Sort(pages)
	if want := []string{"Alpha", "Home", "Ideas", "Note", "Note"}; !slices.Equal(pages, want) {
		t.Errorf("pages = %v, want %v", pages, want)
	}
	if want := []string{"2026-05-24", "2026-05-25"}; !slices.Equal(journals, want) {
		t.Errorf("journals = %v, want %v", journals, want)
	}

	for target, want := range map[string]string{
		"Note":           "a/Note.md",
		"a/Note":         "a/Note.md",
		"B/NOTE":         "b/Note.md",
		"Projects/Alpha": "Projects/Alpha.md",
		"2026-05-24":     "Daily/2026-05-24.md",
		"Alpha":          "Projects/Alpha.md",
	} {
		if got := resolveRel(t, idx, target); got != want {
			t.Errorf("Resolve(%q) = %s, want %s", target, got, want)
		}
	}
	if p, ok := idx.Resolve("x/Note"); ok {
		t.Errorf("Resolve(x/Note) = %s, want failure", p.Rel)
	}

	if k := keyOf(t, idx, "a/Note.md"); k != "Note" {
		t.Errorf("a/Note.md key = %q", k)
	}
	if k := keyOf(t, idx, "b/Note.md"); k != "b/Note" {
		t.Errorf("b/Note.md key = %q", k)
	}

	wantWarn := `page name "Note" is shared by 2 files (a/Note.md, b/Note.md); [[Note]] opens a/Note.md`
	if !slices.Equal(idx.Warnings, []string{wantWarn}) {
		t.Errorf("warnings = %q, want exactly %q", idx.Warnings, wantWarn)
	}

	if d, ok := idx.ResolveLink("Alpha#Plan"); !ok || d.Page.Rel != "Projects/Alpha.md" || d.Heading != "Plan" {
		t.Errorf("ResolveLink(Alpha#Plan) = %+v, %v", d, ok)
	}

	if refs := idx.BacklinksTo("b/Note"); len(refs) != 1 || refs[0].FromPage != "Home" || refs[0].LineNumber != 1 {
		t.Errorf("BacklinksTo(b/Note) = %+v, want one ref from Home:1", refs)
	}
	if refs := idx.BacklinksTo("Note"); len(refs) != 2 || refs[0].FromPage != "Home" || refs[1].FromPage != "Home" ||
		refs[0].LineNumber != 1 || refs[1].LineNumber != 1 {
		t.Errorf("BacklinksTo(Note) = %+v, want two refs from Home:1", refs)
	}
	home := idx.BacklinksTo("Home")
	if len(home) != 1 || home[0].FromPage != "b/Note" {
		t.Errorf("BacklinksTo(Home) = %+v, want only b/Note", home)
	}

	var found bool
	for _, td := range idx.Todos {
		if td.Text == "b task" {
			found = true
			if td.Page != "b/Note" {
				t.Errorf("b task Page = %q, want b/Note", td.Page)
			}
		}
	}
	if !found {
		t.Errorf("todo %q not indexed: %+v", "b task", idx.Todos)
	}
}

func TestClashJournalWins(t *testing.T) {
	idx := clashIndex(t, "", "pages/2026_05_24.md", "journals/2026_05_24.md", "pages/Note.md", "journals/Note.md")

	if got := resolveRel(t, idx, "2026_05_24"); got != "journals/2026_05_24.md" {
		t.Errorf("Resolve(2026_05_24) = %s, want the journal", got)
	}
	if k := keyOf(t, idx, "pages/2026_05_24.md"); k != "pages/2026_05_24" {
		t.Errorf("page key = %q, want pages/2026_05_24", k)
	}
	if got := resolveRel(t, idx, "pages/2026_05_24"); got != "pages/2026_05_24.md" {
		t.Errorf("Resolve(pages/2026_05_24) = %s", got)
	}
	want := `page name "2026_05_24" is shared by 2 files (journals/2026_05_24.md, pages/2026_05_24.md); [[2026_05_24]] opens journals/2026_05_24.md`
	if !slices.Contains(idx.Warnings, want) {
		t.Errorf("warnings %q lack %q", idx.Warnings, want)
	}

	// A non-journal in journals/ ties on depth: byte order picks journals/.
	if got := resolveRel(t, idx, "Note"); got != "journals/Note.md" {
		t.Errorf("Resolve(Note) = %s, want journals/Note.md", got)
	}
	if k := keyOf(t, idx, "pages/Note.md"); k != "pages/Note" {
		t.Errorf("pages/Note.md key = %q", k)
	}
	if len(idx.Warnings) != 2 {
		t.Errorf("warnings = %q, want the two shared-name lines only", idx.Warnings)
	}
}

func TestClashShallowestThenAlphabetical(t *testing.T) {
	idx := clashIndex(t, `pages_dir = ""`+"\n", "Note.md", "x/a/Note.md", "y/a/Note.md")
	if got := resolveRel(t, idx, "Note"); got != "Note.md" {
		t.Errorf("Resolve(Note) = %s, want Note.md", got)
	}
	for rel, want := range map[string]string{"Note.md": "Note", "x/a/Note.md": "a/Note", "y/a/Note.md": "y/a/Note"} {
		if k := keyOf(t, idx, rel); k != want {
			t.Errorf("%s key = %q, want %q", rel, k, want)
		}
		if got := resolveRel(t, idx, want); got != rel {
			t.Errorf("Resolve(%q) = %s, want %s", want, got, rel)
		}
	}
}

func TestExactNameBeatsPath(t *testing.T) {
	idx := clashIndex(t, `pages_dir = ""`+"\n", "x___y.md", "x/y.md", "z/y.md")
	if got := resolveRel(t, idx, "x/y"); got != "x___y.md" {
		t.Errorf("Resolve(x/y) = %s, want x___y.md", got)
	}
	if got := resolveRel(t, idx, "y"); got != "x/y.md" {
		t.Errorf("Resolve(y) = %s, want x/y.md", got)
	}
	if k := keyOf(t, idx, "x/y.md"); k != "y" {
		t.Errorf("x/y.md key = %q, want y", k)
	}
	if k := keyOf(t, idx, "z/y.md"); k != "z/y" {
		t.Errorf("z/y.md key = %q, want z/y", k)
	}
}

func TestUnreachablePageWarns(t *testing.T) {
	idx := clashIndex(t, `pages_dir = ""`+"\n", "x___y.md", "x/y.md", "a/y.md")
	if got := resolveRel(t, idx, "y"); got != "a/y.md" {
		t.Errorf("Resolve(y) = %s, want a/y.md", got)
	}
	if k := keyOf(t, idx, "x/y.md"); k != "x/y" {
		t.Errorf("x/y.md key = %q, want x/y", k)
	}
	if got := resolveRel(t, idx, "x/y"); got != "x___y.md" {
		t.Errorf("Resolve(x/y) = %s, want the documented hole x___y.md", got)
	}
	want := "x/y.md cannot be opened by a link: every path suffix opens another page"
	if !slices.Contains(idx.Warnings, want) {
		t.Errorf("warnings %q lack %q", idx.Warnings, want)
	}
}

func TestFoldedBacklinksStaySeparate(t *testing.T) {
	idx := buildTempIndex(t, map[string]string{
		"Alpha.md": "- a\n",
		"alpha.md": "- b\n",
		"Src.md":   "- [[ALPHA]] [[alpha]]\n",
	})
	if got := pageNamesOfRefs(idx.BacklinksTo("Alpha")); !slices.Equal(got, []string{"Src"}) {
		t.Errorf("BacklinksTo(Alpha) from %v, want Src once", got)
	}
	if got := pageNamesOfRefs(idx.BacklinksTo("alpha")); !slices.Equal(got, []string{"Src"}) {
		t.Errorf("BacklinksTo(alpha) from %v, want Src once", got)
	}
}

func TestVaultSearchCoversOnlyIndexedFiles(t *testing.T) {
	if _, err := exec.LookPath("rg"); err != nil {
		t.Skip("rg not on PATH")
	}
	idx := vaultIndex(t)

	hits, err := search.Run(idx.Files(), "Alpha")
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) == 0 {
		t.Fatal("no hits for Alpha")
	}
	for _, h := range hits {
		if _, ok := idx.PageAt(h.FilePath); !ok {
			t.Errorf("hit in unindexed file %s", h.FilePath)
		}
	}

	hits, err = search.Mentions(idx.Files(), "Alpha")
	if err != nil {
		t.Fatal(err)
	}
	refs := idx.FilterUnlinked(hits, "", func(p string) (string, error) {
		b, err := os.ReadFile(p)
		return string(b), err
	})
	var got []string
	for _, r := range refs {
		got = append(got, r.PageName)
	}
	if !slices.Contains(got, "Ideas") {
		t.Errorf("mention pages = %v, want Ideas (Daily/Ideas.md mapped by path)", got)
	}
}
