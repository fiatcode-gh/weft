package graph

import (
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
	"unsafe"
)

func TestBuildIndex(t *testing.T) {
	idx := buildFixtureIndex(t)

	// Pages
	names := make([]string, 0, len(idx.Pages))
	for _, p := range idx.Pages {
		names = append(names, p.Name)
	}
	sort.Strings(names)
	want := []string{
		"2026-01-10", "2026-03-15", "2026-04-20", "2026-05-01",
		"2026-05-15", "2026-05-22", "2026-05-23", "2026-05-24", "2026-05-25",
		"Alpha", "Beta", "Hub", "Orphan", "Workbench", "kb/notes", "proj/nested",
	}
	if !equalSlices(names, want) {
		t.Errorf("page names: want %v, got %v", want, names)
	}

	// Backlinks: Alpha is linked from Beta, Hub, 2026-05-15, and 2026-05-24
	gotAlpha := pageNamesOfRefs(idx.Backlinks[strings.ToLower("Alpha")])
	sort.Strings(gotAlpha)
	wantAlpha := []string{"2026-05-15", "2026-05-24", "Beta", "Hub"}
	if !equalSlices(gotAlpha, wantAlpha) {
		t.Errorf("Alpha backlinks: want %v, got %v", wantAlpha, gotAlpha)
	}

	// Lock down Ref.Context (the source line for the backlink).
	var betaRef *Ref
	for i, r := range idx.Backlinks[strings.ToLower("Alpha")] {
		if r.FromPage == "Beta" {
			betaRef = &idx.Backlinks[strings.ToLower("Alpha")][i]
			break
		}
	}
	if betaRef == nil {
		t.Fatalf("no Beta→Alpha backlink found")
	}
	wantCtx := "- Beta links back to [[Alpha]]."
	if betaRef.Context != wantCtx {
		t.Errorf("Beta→Alpha context: want %q, got %q", wantCtx, betaRef.Context)
	}
	if betaRef.LineNumber != 1 {
		t.Errorf("Beta→Alpha line: want 1, got %d", betaRef.LineNumber)
	}

	// Dangling refs still recorded
	if len(idx.Backlinks[strings.ToLower("DoesNotExist")]) != 1 {
		t.Errorf("dangling backlink to DoesNotExist not recorded: %v", idx.Backlinks[strings.ToLower("DoesNotExist")])
	}

	// Fence-internal wiki-links must NOT be extracted. Alpha has
	// `[[ShouldNotMatch]]` inside a fence; 2026-03-15 has `[[NotALink]]`.
	// If either name surfaces in Backlinks, ExtractWikiLinks lost fence
	// awareness.
	for _, name := range []string{"ShouldNotMatch", "NotALink"} {
		if refs := idx.Backlinks[strings.ToLower(name)]; len(refs) != 0 {
			t.Errorf("%s should not be in Backlinks (fenced); got %v", name, refs)
		}
	}

	// Todos: 13 open across the fixture
	if len(idx.Todos) != 13 {
		t.Errorf("todo count: want 13, got %d (%+v)", len(idx.Todos), idx.Todos)
	}
}

func pageNamesOfRefs(refs []Ref) []string {
	out := make([]string, 0, len(refs))
	for _, r := range refs {
		out = append(out, r.FromPage)
	}
	return out
}

func equalSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestBuildIndexJournalsSorted(t *testing.T) {
	idx := buildFixtureIndex(t)

	want := []string{
		"2026-01-10", "2026-03-15", "2026-04-20", "2026-05-01",
		"2026-05-15", "2026-05-22", "2026-05-23", "2026-05-24", "2026-05-25",
	}
	if !equalSlices(idx.Journals, want) {
		t.Errorf("Journals: want %v, got %v", want, idx.Journals)
	}

	// Every entry must correspond to a PageMeta with IsJournal == true.
	for _, name := range idx.Journals {
		meta, ok := idx.ByName[name]
		if !ok {
			t.Errorf("Journals contains %q but ByName doesn't", name)
			continue
		}
		if !meta.IsJournal {
			t.Errorf("Journals entry %q has IsJournal=false", name)
		}
	}
}

func TestBuildIndexResolvesCaseInsensitively(t *testing.T) {
	idx := buildFixtureIndex(t)

	// Fold-keyed shadow map is populated.
	if _, ok := idx.ByNameFold["alpha"]; !ok {
		t.Errorf("byNameFold missing 'alpha' (page is 'Alpha' in fixture)")
	}

	// All three case variants resolve to the same case-preserving page.
	for _, q := range []string{"alpha", "ALPHA", "Alpha"} {
		got, ok := idx.Resolve(q)
		if !ok {
			t.Errorf("Resolve(%q) = (_, false), want (_, true)", q)
			continue
		}
		if got.Name != "Alpha" {
			t.Errorf("Resolve(%q).Name = %q, want %q", q, got.Name, "Alpha")
		}
	}

	// Missing name returns (nil, false).
	if got, ok := idx.Resolve("does-not-exist"); ok || got != nil {
		t.Errorf("Resolve(\"does-not-exist\") = (%+v, %v), want (nil, false)", got, ok)
	}

	// Numeric journal names are unaffected by case-folding.
	journalPage := "2026-05-24"
	if _, ok := idx.ByName[journalPage]; !ok {
		if len(idx.Journals) == 0 {
			t.Skip("no journals in fixture; skipping numeric-name assertion")
		}
		journalPage = idx.Journals[0]
	}
	got, ok := idx.Resolve(journalPage)
	if !ok {
		t.Errorf("Resolve(%q) = (_, false), want (_, true)", journalPage)
	} else if got.Name != journalPage {
		t.Errorf("Resolve(%q).Name = %q, want %q", journalPage, got.Name, journalPage)
	}
}

func TestBuildIndexJournalsEmptyWhenNoJournals(t *testing.T) {
	// arrange
	idx := buildTempIndex(t, map[string]string{"Lonely.md": "- hi\n"})

	// assert
	if len(idx.Journals) != 0 {
		t.Errorf("Journals: want empty slice, got %v", idx.Journals)
	}
}

func TestBuildIndexWarnsOnSubdirectory(t *testing.T) {
	// arrange — a page plus a nested file under pages/sub/ that must be skipped.
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "pages", "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "pages", "Alpha.md"), []byte("# Alpha\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "pages", "sub", "nested.md"), []byte("# nested\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	oldStderr := os.Stderr
	os.Stderr = w
	defer func() { os.Stderr = oldStderr }()

	// act
	idx, err := BuildIndex(dir)
	w.Close()
	out, _ := io.ReadAll(r)
	os.Stderr = oldStderr

	// assert
	if err != nil {
		t.Fatal(err)
	}
	if len(idx.Pages) != 1 {
		t.Errorf("want 1 page (the subdir file is skipped), got %d", len(idx.Pages))
	}
	if !strings.Contains(string(out), "skipping subdirectory") {
		t.Errorf("expected subdirectory warning on stderr, got:\n%s", out)
	}
}

func TestBuildIndexAssignsTodoOrdinals(t *testing.T) {
	// arrange
	body := "- DONE done one\n- TODO open one\n- LATER open two\n"
	idx := buildTempIndex(t, map[string]string{"P.md": body})

	// act
	var got []int
	for _, b := range idx.Todos {
		if b.Page == "P" {
			got = append(got, b.Ordinal)
		}
	}

	// assert — two open todos on P, ordinals 0 and 1 in document order (DONE skipped).
	if len(got) != 2 || got[0] != 0 || got[1] != 1 {
		t.Errorf("ordinals: want [0 1], got %v", got)
	}
}

func TestBacklinkContextIsTheSourceLine(t *testing.T) {
	// arrange
	body := "- intro\r\n- mentions [[Alpha]] here\n- outro\n"
	idx := buildTempIndex(t, map[string]string{"Src.md": body})

	// act
	refs := idx.Backlinks[strings.ToLower("Alpha")]

	// assert
	if len(refs) != 1 {
		t.Fatalf("want 1 backlink to Alpha, got %d", len(refs))
	}
	if refs[0].Context != "- mentions [[Alpha]] here" {
		t.Errorf("context: want the source line, got %q", refs[0].Context)
	}
	if refs[0].LineNumber != 2 {
		t.Errorf("line number: want 2, got %d", refs[0].LineNumber)
	}
}

func TestBuildIndexTodoOrdinalsSkipDoneAndCountPriority(t *testing.T) {
	// arrange
	body := "- TODO [#A] first with priority\n- DONE done in the middle\n- LATER third open\n"
	idx := buildTempIndex(t, map[string]string{"Q.md": body})

	// act
	var got []struct {
		text    string
		ordinal int
	}
	for _, b := range idx.Todos {
		if b.Page == "Q" {
			got = append(got, struct {
				text    string
				ordinal int
			}{b.Text, b.Ordinal})
		}
	}

	// assert
	if len(got) != 2 || got[0].ordinal != 0 || got[1].ordinal != 1 {
		t.Fatalf("ordinals: want two todos with ordinals 0,1, got %+v", got)
	}
}

func TestLineContextDoesNotPinFileBody(t *testing.T) {
	// arrange: a large body whose lines are substrings of one big backing array
	// (that's how strings.Split works). If lineContext hands back such a
	// substring directly, the single retained context line keeps the ENTIRE
	// body alive for the life of the index — pinning page text we don't need.
	body := strings.Repeat("filler noise line\n", 2000) + "the real context line"
	lines := strings.Split(body, "\n")

	// act
	ctx := lineContext(lines, len(lines)) // the last line

	// assert: correct value...
	if ctx != "the real context line" {
		t.Fatalf("context value: want %q, got %q", "the real context line", ctx)
	}
	// ...and it must NOT alias the big body's backing array.
	if stringAliases(body, ctx) {
		t.Fatal("lineContext result aliases the file body backing array — it pins the whole body in memory")
	}
	runtime.KeepAlive(body)
}

func TestBacklinksAreCaseInsensitive(t *testing.T) {
	// arrange
	dir := t.TempDir()
	pages := filepath.Join(dir, "pages")
	if err := os.MkdirAll(pages, 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(name, body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(pages, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("Alpha.md", "- the target page\n")
	// The link is written [[ALPHA]] — a case that differs from BOTH the
	// canonical page name "Alpha" and its lowercase form "alpha". Pre-fix the
	// write site keyed Backlinks["ALPHA"] verbatim, so the folded lookup
	// Backlinks["alpha"] missed. The fix folds the write key too.
	write("Note.md", "- see [[ALPHA]] here\n")

	// act
	idx, err := BuildIndex(dir)
	if err != nil {
		t.Fatal(err)
	}

	// assert — lookup by the canonical on-disk name must find the
	// case-variant link
	refs := idx.Backlinks[strings.ToLower("Alpha")]
	if len(refs) != 1 || refs[0].FromPage != "Note" {
		t.Fatalf("Backlinks[alpha] = %+v, want one ref from Note", refs)
	}
}

func TestBuildIndexFoldCollisionIsDeterministicAndWarns(t *testing.T) {
	// arrange — two pages that fold to the same key; readdir-sorted order puts
	// "Alpha.md" (ASCII 'A'=65) before "alpha.md" ('a'=97), so "Alpha" wins.
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "pages"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "pages", "Alpha.md"), []byte("# Alpha\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "pages", "alpha.md"), []byte("# alpha\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	oldStderr := os.Stderr
	os.Stderr = w
	defer func() { os.Stderr = oldStderr }()

	// act
	idx, err := BuildIndex(dir)
	w.Close()
	out, _ := io.ReadAll(r)
	os.Stderr = oldStderr

	// assert
	if err != nil {
		t.Fatal(err)
	}
	// "alpha" is an exact match for the second page and must resolve to it
	// (not the fold-collision winner); "ALPHA" has no exact match, so it
	// falls back to the deterministic first-walked folded entry, "Alpha".
	for q, want := range map[string]string{"alpha": "alpha", "ALPHA": "Alpha", "Alpha": "Alpha"} {
		got, ok := idx.Resolve(q)
		if !ok {
			t.Fatalf("Resolve(%q) = (_, false), want (_, true)", q)
		}
		if got.Name != want {
			t.Errorf("Resolve(%q).Name = %q, want %q", q, got.Name, want)
		}
	}
	if !strings.Contains(string(out), "ambiguous page name") {
		t.Errorf("expected an ambiguous-page-name warning on stderr, got:\n%s", out)
	}
}

func TestFoldCollisionKeepsExactNameResolution(t *testing.T) {
	// arrange — two pages differing only by case
	dir := t.TempDir()
	pages := filepath.Join(dir, "pages")
	if err := os.MkdirAll(pages, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"Alpha.md", "alpha.md"} {
		if err := os.WriteFile(filepath.Join(pages, n), []byte("- x\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// act
	idx, err := BuildIndex(dir)
	if err != nil {
		t.Fatal(err)
	}

	// assert — exact lookups win for BOTH; the ambiguous folded lookup
	// stays deterministic (first-walked: ReadDir is filename-sorted,
	// "Alpha.md" < "alpha.md" in ASCII)
	for name, want := range map[string]string{
		"Alpha": "Alpha",
		"alpha": "alpha",
		"ALPHA": "Alpha",
	} {
		meta, ok := idx.Resolve(name)
		if !ok || meta.Name != want {
			t.Errorf("Resolve(%q) = %v/%v, want %q", name, meta, ok, want)
		}
	}
}

// stringAliases reports whether sub's backing bytes lie inside parent's — i.e.
// sub is a substring sharing parent's allocation rather than an independent copy.
func stringAliases(parent, sub string) bool {
	if len(parent) == 0 || len(sub) == 0 {
		return false
	}
	p := uintptr(unsafe.Pointer(unsafe.StringData(parent)))
	s := uintptr(unsafe.Pointer(unsafe.StringData(sub)))
	return s >= p && s < p+uintptr(len(parent))
}
