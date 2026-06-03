package graph

import (
	"os"
	"path/filepath"
	"sort"
	"testing"
)

func fixturePath(t *testing.T) string {
	t.Helper()
	p, err := filepath.Abs("../../testdata/fixture-graph")
	if err != nil {
		t.Fatalf("abs fixture path: %v", err)
	}
	return p
}

func TestBuildIndex(t *testing.T) {
	idx, err := BuildIndex(fixturePath(t))
	if err != nil {
		t.Fatalf("BuildIndex: %v", err)
	}

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

	// Backlinks: Alpha is linked from Beta and 2026-05-24
	gotAlpha := pageNamesOfRefs(idx.Backlinks["Alpha"])
	sort.Strings(gotAlpha)
	wantAlpha := []string{"2026-05-15", "2026-05-24", "Beta", "Hub"}
	if !equalSlices(gotAlpha, wantAlpha) {
		t.Errorf("Alpha backlinks: want %v, got %v", wantAlpha, gotAlpha)
	}

	// Lock down Ref.Context (the source line for the backlink).
	var betaRef *Ref
	for i, r := range idx.Backlinks["Alpha"] {
		if r.FromPage == "Beta" {
			betaRef = &idx.Backlinks["Alpha"][i]
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
	if len(idx.Backlinks["DoesNotExist"]) != 1 {
		t.Errorf("dangling backlink to DoesNotExist not recorded: %v", idx.Backlinks["DoesNotExist"])
	}

	// Fence-internal wiki-links must NOT be extracted. Alpha has
	// `[[ShouldNotMatch]]` inside a fence; 2026-03-15 has `[[NotALink]]`.
	// If either name surfaces in Backlinks, ExtractWikiLinks lost fence
	// awareness.
	for _, name := range []string{"ShouldNotMatch", "NotALink"} {
		if refs := idx.Backlinks[name]; len(refs) != 0 {
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
	idx, err := BuildIndex(fixturePath(t))
	if err != nil {
		t.Fatalf("BuildIndex: %v", err)
	}

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
	idx, err := BuildIndex(fixturePath(t))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := idx.ByNameFold["alpha"]; !ok {
		t.Errorf("byNameFold missing 'alpha' (page is 'Alpha' in fixture)")
	}
	// The case-preserving name is what callers should get back.
	if got, ok := idx.Resolve("alpha"); !ok || got.Name != "Alpha" {
		t.Errorf("Resolve(alpha) = (%+v, %v), want Alpha/true", got, ok)
	}
}

func TestBuildIndexJournalsEmptyWhenNoJournals(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "pages"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "pages", "Lonely.md"), []byte("- hi\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	idx, err := BuildIndex(dir)
	if err != nil {
		t.Fatalf("BuildIndex: %v", err)
	}
	if len(idx.Journals) != 0 {
		t.Errorf("Journals: want empty slice, got %v", idx.Journals)
	}
}
