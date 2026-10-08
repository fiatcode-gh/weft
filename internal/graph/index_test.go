package graph

import (
	"os"
	"path/filepath"
	"reflect"
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
		"Alpha", "Beta", "Book Club", "Corpus", "Hub", "Kitchen", "Orphan", "Workbench", "kb/notes", "proj/nested",
	}
	if !equalSlices(names, want) {
		t.Errorf("page names: want %v, got %v", want, names)
	}

	// Backlinks: Alpha is linked from Beta, Hub, 2026-05-15, and 2026-05-24
	gotAlpha := pageNamesOfRefs(idx.BacklinksTo("Alpha"))
	sort.Strings(gotAlpha)
	wantAlpha := []string{"2026-05-15", "2026-05-24", "Beta", "Hub"}
	if !equalSlices(gotAlpha, wantAlpha) {
		t.Errorf("Alpha backlinks: want %v, got %v", wantAlpha, gotAlpha)
	}

	// Lock down Ref.Context (the source line for the backlink).
	var betaRef Ref
	var foundBetaRef bool
	for _, r := range idx.BacklinksTo("Alpha") {
		if r.FromPage == "Beta" {
			betaRef = r
			foundBetaRef = true
			break
		}
	}
	if !foundBetaRef {
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
	if len(idx.BacklinksTo("DoesNotExist")) != 1 {
		t.Errorf("dangling backlink to DoesNotExist not recorded: %v", idx.BacklinksTo("DoesNotExist"))
	}

	// Fence-internal wiki-links must NOT be extracted. Alpha has
	// `[[ShouldNotMatch]]` inside a fence; 2026-03-15 has `[[NotALink]]`.
	// If either name surfaces in Backlinks, ExtractLinks lost fence
	// awareness.
	for _, name := range []string{"ShouldNotMatch", "NotALink"} {
		if refs := idx.BacklinksTo(name); len(refs) != 0 {
			t.Errorf("%s should not be in Backlinks (fenced); got %v", name, refs)
		}
	}

	// Todos: 13 open across the fixture
	if len(idx.Todos) != 13 {
		t.Errorf("todo count: want 13, got %d (%+v)", len(idx.Todos), idx.Todos)
	}
}

func TestBuildIndexCountsTags(t *testing.T) {
	idx := buildFixtureIndex(t)

	wantKitchen := []Ref{
		{FromPage: "Corpus", LineNumber: 1, Context: "- Tags: #kitchen and (#Kitchen) and #s"},
		{FromPage: "Corpus", LineNumber: 1, Context: "- Tags: #kitchen and (#Kitchen) and #s"},
	}
	if got := idx.BacklinksTo("Kitchen"); !reflect.DeepEqual(got, wantKitchen) {
		t.Errorf("Kitchen backlinks = %+v, want %+v", got, wantKitchen)
	}
	// #[[Book Club]] and #[[Book Club|the club]] each count once.
	book := idx.BacklinksTo("Book Club")
	if len(book) != 2 || book[0].FromPage != "Corpus" || book[0].LineNumber != 2 || book[1].LineNumber != 2 {
		t.Errorf("Book Club backlinks = %+v, want 2 from Corpus line 2", book)
	}
	var nsHit bool
	for _, r := range idx.BacklinksTo("kb/notes") {
		if r.FromPage == "Corpus" && r.LineNumber == 2 {
			nsHit = true
		}
	}
	if !nsHit {
		t.Errorf("kb/notes backlinks lack Corpus:2: %+v", idx.BacklinksTo("kb/notes"))
	}
	for _, name := range []string{"incode", "alsocode", "fenced", "anchor", "frag", "inner", "18", "FAF3E7", "a1b"} {
		if refs := idx.BacklinksTo(name); len(refs) != 0 {
			t.Errorf("%q must not be a tag target, got %+v", name, refs)
		}
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
	if !equalSlices(idx.journals, want) {
		t.Errorf("journals: want %v, got %v", want, idx.journals)
	}

	// Every entry must correspond to a PageMeta with IsJournal == true.
	for _, name := range idx.journals {
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
	if got, ok := idx.JournalNeighbor("2026-01-10", 1); ok || got != "" {
		t.Errorf("JournalNeighbor on empty index = (%q, %v), want (\"\", false)", got, ok)
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

	// act
	idx, err := BuildIndex(dir)

	// assert
	if err != nil {
		t.Fatal(err)
	}
	if len(idx.Pages) != 1 {
		t.Errorf("want 1 page (the subdir file is skipped), got %d", len(idx.Pages))
	}
	// assert — in TestBuildIndexWarnsOnSubdirectory
	found := false
	for _, w := range idx.Warnings {
		if strings.Contains(w, "skipping subdirectory") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected subdirectory warning in idx.Warnings, got %v", idx.Warnings)
	}
}

// An unreadable page must not abort boot: it stays in the picker (first
// pass) but its body parse is skipped with a warning (second pass).
func TestBuildIndexCollectsUnreadablePageWarning(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores file modes")
	}
	// arrange
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "pages"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"Good.md", "Bad.md"} {
		if err := os.WriteFile(filepath.Join(dir, "pages", name), []byte("- TODO x\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	bad := filepath.Join(dir, "pages", "Bad.md")
	if err := os.Chmod(bad, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(bad, 0o644) }) // so TempDir removal works

	// act
	idx, err := BuildIndex(dir)

	// assert
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := idx.Resolve("Bad"); !ok {
		t.Error("unreadable page should stay in the index")
	}
	found := false
	for _, w := range idx.Warnings {
		if strings.Contains(w, "skipping") && strings.Contains(w, "Bad.md") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected unreadable-page warning, got %v", idx.Warnings)
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
	refs := idx.BacklinksTo("Alpha")

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

func TestBacklinksToReturnsSnapshot(t *testing.T) {
	// arrange
	idx := buildTempIndex(t, map[string]string{"Source.md": "- [[Target]]\n"})

	// act
	refs := idx.BacklinksTo("Target")
	if len(refs) != 1 {
		t.Fatalf("BacklinksTo(Target) returned %d refs, want 1", len(refs))
	}
	refs[0].FromPage = "mutated"
	got := idx.BacklinksTo("Target")

	// assert
	if got[0].FromPage != "Source" {
		t.Errorf("BacklinksTo(Target) stored ref was mutated: got %q, want %q", got[0].FromPage, "Source")
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

	// assert — every case variant must find the case-variant link.
	for _, name := range []string{"Alpha", "alpha", "ALPHA"} {
		refs := idx.BacklinksTo(name)
		if len(refs) != 1 || refs[0].FromPage != "Note" {
			t.Fatalf("BacklinksTo(%q) = %+v, want one ref from Note", name, refs)
		}
	}
}

func TestJournalNeighbor(t *testing.T) {
	// arrange
	idx := buildFixtureIndex(t)
	tests := []struct {
		name    string
		current string
		dir     int
		want    string
		ok      bool
	}{
		{name: "previous existing", current: "2026-05-24", dir: -1, want: "2026-05-23", ok: true},
		{name: "next existing", current: "2026-05-24", dir: 1, want: "2026-05-25", ok: true},
		{name: "previous phantom", current: "2026-05-26", dir: -1, want: "2026-05-25", ok: true},
		{name: "next phantom gap", current: "2026-05-02", dir: 1, want: "2026-05-15", ok: true},
		{name: "before oldest", current: "2026-01-10", dir: -1, ok: false},
		{name: "after newest", current: "2026-05-25", dir: 1, ok: false},
		{name: "not a journal", current: "Alpha", dir: -1, ok: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// act
			got, ok := idx.JournalNeighbor(tt.current, tt.dir)

			// assert
			if got != tt.want || ok != tt.ok {
				t.Errorf("JournalNeighbor(%q, %d) = (%q, %v), want (%q, %v)", tt.current, tt.dir, got, ok, tt.want, tt.ok)
			}
		})
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

	// act
	idx, err := BuildIndex(dir)

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
	found := false
	for _, w := range idx.Warnings {
		if strings.Contains(w, "ambiguous page name") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected an ambiguous-page-name warning in idx.Warnings, got %v", idx.Warnings)
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
