package doctor

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/fiatcode-gh/weft/v2/internal/graph"
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

// tempGraph materialises a throwaway graph and returns its root. Keys are
// paths relative to the graph root (e.g. "pages/Solo.md").
func tempGraph(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, body := range files {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestRunWithBuildsReport(t *testing.T) {
	// arrange — the scanner fabricates exactly one mention for Alpha
	scanner := func(_, name string) ([]graph.UnlinkedRef, error) {
		if name == "Alpha" {
			return []graph.UnlinkedRef{{PageName: "Workbench", Line: 8}}, nil
		}
		return nil, nil
	}

	// act
	rep, err := RunWith(fixturePath(t), scanner)

	// assert
	if err != nil {
		t.Fatal(err)
	}
	if rep.Pages != 10 || rep.Journals != 9 {
		t.Errorf("counts = %d pages / %d journals, want 10 / 9", rep.Pages, rep.Journals)
	}
	if !reflect.DeepEqual(rep.Orphans, []string{"Orphan"}) {
		t.Errorf("orphans = %v, want [Orphan]", rep.Orphans)
	}
	if got := unresolvedTargets(rep); !reflect.DeepEqual(got, fixtureUnresolvedTargets) {
		t.Errorf("unresolved targets = %v, want %v", got, fixtureUnresolvedTargets)
	}
	want := []PageMentions{{Page: "Alpha", Refs: []graph.UnlinkedRef{{PageName: "Workbench", Line: 8}}}}
	if !reflect.DeepEqual(rep.Mentions, want) {
		t.Errorf("mentions = %+v, want %+v", rep.Mentions, want)
	}
	if len(rep.Warnings) != 0 {
		t.Errorf("warnings = %v, want none", rep.Warnings)
	}
	if !rep.HasFindings() {
		t.Error("fixture report must have findings")
	}
}

func TestRunWithScannerErrorFailsRun(t *testing.T) {
	// arrange
	scanner := func(_, name string) ([]graph.UnlinkedRef, error) {
		return nil, errors.New("boom")
	}

	// act
	_, err := RunWith(fixturePath(t), scanner)

	// assert — the first scanned page in ascending name order is Alpha
	if err == nil || !strings.Contains(err.Error(), "boom") || !strings.Contains(err.Error(), "Alpha") {
		t.Fatalf("err = %v, want wrapped boom mentioning Alpha", err)
	}
}

func TestRunWithCleanGraphHasNoFindings(t *testing.T) {
	// arrange — two pages linking each other: no orphans, no phantoms
	dir := tempGraph(t, map[string]string{
		"pages/Solo.md":  "- links to [[Other]]\n",
		"pages/Other.md": "- links to [[Solo]]\n",
	})
	scanner := func(_, _ string) ([]graph.UnlinkedRef, error) { return nil, nil }

	// act
	rep, err := RunWith(dir, scanner)

	// assert
	if err != nil {
		t.Fatal(err)
	}
	if rep.HasFindings() {
		t.Errorf("clean graph must have no findings: %+v", rep)
	}
}

func TestRunWithPassesThroughIndexWarnings(t *testing.T) {
	// arrange — a subdirectory under pages/ makes BuildIndex warn
	dir := tempGraph(t, map[string]string{
		"pages/Solo.md":  "- links to [[Other]]\n",
		"pages/Other.md": "- links to [[Solo]]\n",
	})
	if err := os.MkdirAll(filepath.Join(dir, "pages", "subdir"), 0o755); err != nil {
		t.Fatal(err)
	}
	scanner := func(_, _ string) ([]graph.UnlinkedRef, error) { return nil, nil }

	// act
	rep, err := RunWith(dir, scanner)

	// assert
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Warnings) != 1 || !strings.Contains(rep.Warnings[0], "skipping subdirectory") {
		t.Errorf("warnings = %v, want the one skipping-subdirectory warning", rep.Warnings)
	}
	if !rep.HasFindings() {
		t.Error("a graph with index warnings must have findings")
	}
}

// skipIfNoRipgrep skips when rg is not on PATH; Run shells out to ripgrep.
func skipIfNoRipgrep(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("rg"); err != nil {
		t.Skip("rg not on PATH; install ripgrep to run this test")
	}
}

func TestRunAgainstFixture(t *testing.T) {
	skipIfNoRipgrep(t)

	// act
	rep, err := Run(fixturePath(t))

	// assert
	if err != nil {
		t.Fatal(err)
	}
	if rep.Pages != 10 || rep.Journals != 9 {
		t.Errorf("counts = %d pages / %d journals, want 10 / 9", rep.Pages, rep.Journals)
	}
	if got := unresolvedTargets(rep); !reflect.DeepEqual(got, fixtureUnresolvedTargets) {
		t.Errorf("unresolved targets = %v, want %v", got, fixtureUnresolvedTargets)
	}
	if !reflect.DeepEqual(rep.Orphans, []string{"Orphan"}) {
		t.Errorf("orphans = %v, want [Orphan]", rep.Orphans)
	}
	if len(rep.Mentions) != 1 || rep.Mentions[0].Page != "Alpha" {
		t.Fatalf("mentions = %+v, want exactly the Alpha page", rep.Mentions)
	}
	refs := rep.Mentions[0].Refs
	if len(refs) != 1 {
		t.Fatalf("want 1 unlinked mention of Alpha, got %d: %+v", len(refs), refs)
	}
	r := refs[0]
	if r.PageName != "Workbench" || r.Line != 9 {
		t.Errorf("mention = %+v, want Workbench:9", r)
	}
	if r.Match.Start != strings.Index(r.Context, "Alpha") {
		t.Errorf("match span does not point at the Alpha mention: %+v", r)
	}
	if len(rep.Warnings) != 0 {
		t.Errorf("warnings = %v, want none", rep.Warnings)
	}
}

func TestWriteTextFullReport(t *testing.T) {
	// arrange
	rep := &Report{
		GraphPath:  "/g",
		Pages:      7,
		Journals:   9,
		Unresolved: []graph.UnresolvedLink{{Target: "DoesNotExist", Refs: []graph.Ref{{FromPage: "Beta", LineNumber: 3}}}},
		Orphans:    []string{"Orphan"},
		Mentions:   []PageMentions{{Page: "Alpha", Refs: []graph.UnlinkedRef{{PageName: "Workbench", Line: 8}}}},
		Warnings:   []string{"skipping subdirectory pages/old"},
	}

	// act
	var sb strings.Builder
	rep.WriteText(&sb)

	// assert
	want := `weft doctor · /g
7 pages · 9 journals

  unresolved links  1
  orphan pages      1
  unlinked mentions 1
  index warnings    1

Unresolved links
  DoesNotExist  Beta:3

Orphan pages
  Orphan

Unlinked mentions
  Alpha  Workbench:8

Index warnings
  skipping subdirectory pages/old
`
	if sb.String() != want {
		t.Errorf("WriteText mismatch\ngot:\n%s\nwant:\n%s", sb.String(), want)
	}
}

func TestWriteTextCleanReport(t *testing.T) {
	// arrange
	rep := &Report{GraphPath: "/g", Pages: 1, Journals: 0}

	// act
	var sb strings.Builder
	rep.WriteText(&sb)

	// assert
	want := `weft doctor · /g
1 pages · 0 journals

  unresolved links  0
  orphan pages      0
  unlinked mentions 0
  index warnings    0

graph is clean
`
	if sb.String() != want {
		t.Errorf("WriteText mismatch\ngot:\n%s\nwant:\n%s", sb.String(), want)
	}
}

func TestWriteTextOmitsZeroSections(t *testing.T) {
	// arrange — only warnings fire
	rep := &Report{
		GraphPath: "/g",
		Pages:     1,
		Warnings:  []string{"cannot stat pages/X.md: stale handle"},
	}

	// act
	var sb strings.Builder
	rep.WriteText(&sb)

	// assert — no Unresolved/Orphan/Mention sections render
	out := sb.String()
	for _, absent := range []string{"Unresolved links\n", "Orphan pages\n", "Unlinked mentions\n"} {
		if strings.Contains(out, absent) {
			t.Errorf("zero section must be omitted, found %q", absent)
		}
	}
	if !strings.Contains(out, "Index warnings\n  cannot stat pages/X.md: stale handle\n") {
		t.Errorf("warnings section missing or malformed:\n%s", out)
	}
}

// fixtureUnresolvedTargets is the fixture's phantom list: DoesNotExist plus
// every tag name in Corpus that has no page.
var fixtureUnresolvedTargets = []string{
	"a1b2c3d4e", "abc12", "add", "bad", "C#", "cafe", "café", "DoesNotExist",
	"inheading", "Lab #inner", "lead", "s", "日本",
}

func unresolvedTargets(rep *Report) []string {
	var out []string
	for _, u := range rep.Unresolved {
		out = append(out, u.Target)
	}
	return out
}
