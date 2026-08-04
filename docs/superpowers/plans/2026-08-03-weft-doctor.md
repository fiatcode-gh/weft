# Weft Doctor Implementation Plan

> Execute with pi-executing-plans, task by task.

**Goal:** Ship a read-only `weft doctor` subcommand that prints unresolved wiki-links, orphan pages, unlinked mentions, and index warnings, exiting 0 clean / 1 findings / 2 operational error.
**Spec:** `docs/superpowers/specs/2026-08-03-weft-doctor-design.md`

## Global constraints

- Work happens on a feature branch: `git switch -c weft-doctor` (Task 0).
- Before every commit run, all four must pass:

  ```bash
  test -z "$(gofmt -l .)"
  go vet ./...
  "$(go env GOPATH)/bin/staticcheck" ./...
  go test -race ./...
  ```

- Tests use only the checked-in `testdata/fixture-graph/`; never point tests at the real graph (`~/Documents/fiat-codex`).
- Every task starts with a failing test and recorded RED evidence before implementation.
- Each task is executed per subagent-driven-development: an implementer subagent, then a spec review and a quality review pass, with scoped re-review after fixes.
- Teatest goldens are regenerated only with `-update` after a visual diff confirms an intentional change. The fixture edit in Task 3 is expected to leave every golden byte-identical; if any golden churns, stop and investigate before regenerating.
- Commit messages use Conventional Commits.
- Go 1.26+; ripgrep (`rg`) on PATH is a hard project dependency.

### Task 0: Seed the backlog bullet and branch

**Files:** `${WEFT_GRAPH}/pages/weft Backlog.md` (the real graph — this is the one sanctioned edit outside the repo).

- [x] Add this bullet at the top of the `## Open` section, preserving the page's voice:

```markdown
- TODO `weft doctor` — surface the graph-health counts the index already computes (unresolved links, zero-backlink pages, unlinked mentions) so retrofit runs are triggered by numbers, not vibes — design in weft repo `docs/superpowers/specs/2026-08-03-weft-doctor-design.md`
```

- [x] Commit it in the graph repo:

  ```bash
  git -C "$WEFT_GRAPH" add "pages/weft Backlog.md"
  git -C "$WEFT_GRAPH" commit -m "weft Backlog: add weft doctor item"
  ```

- [x] Create the feature branch in the weft repo:

  ```bash
  git switch -c weft-doctor
  ```

### Task 1: `graph.Index.UnresolvedLinks` accessor

**Files:** `internal/graph/index.go` (modify), `internal/graph/unresolved.go` (create), `internal/graph/unresolved_test.go` (create).

- [x] Write the failing test — create `internal/graph/unresolved_test.go`:

```go
package graph

import (
 "reflect"
 "testing"
)

func TestUnresolvedLinksFindsPhantoms(t *testing.T) {
 // arrange
 idx := buildFixtureIndex(t)

 // act
 got := idx.UnresolvedLinks()

 // assert — [[DoesNotExist]] in Beta is the only phantom in the fixture;
 // fenced [[ShouldNotMatch]] and [[NotALink]] never enter the index.
 if len(got) != 1 {
  t.Fatalf("want 1 unresolved link, got %d: %+v", len(got), got)
 }
 if got[0].Target != "DoesNotExist" {
  t.Errorf("target = %q, want DoesNotExist", got[0].Target)
 }
 want := []Ref{{FromPage: "Beta", LineNumber: 3, Context: "- This link is dangling: [[DoesNotExist]]."}}
 if !reflect.DeepEqual(got[0].Refs, want) {
  t.Errorf("refs = %+v, want %+v", got[0].Refs, want)
 }
}

func TestUnresolvedLinksResolvesCaseFolded(t *testing.T) {
 // arrange — [[alpha]] resolves to Alpha.md case-insensitively
 idx := buildTempIndex(t, map[string]string{
  "Alpha.md": "- alpha page\n",
  "Note.md":  "- see [[alpha]]\n",
 })

 // act
 got := idx.UnresolvedLinks()

 // assert
 if len(got) != 0 {
  t.Fatalf("case-folded target must resolve, got %+v", got)
 }
}

func TestUnresolvedLinksSortsCaseInsensitively(t *testing.T) {
 // arrange
 idx := buildTempIndex(t, map[string]string{
  "Note.md": "- [[zeta]] then [[Alpha Phantom]] then [[beta phantom]]\n",
 })

 // act
 got := idx.UnresolvedLinks()

 // assert — folded order: "alpha phantom" < "beta phantom" < "zeta"
 var targets []string
 for _, u := range got {
  targets = append(targets, u.Target)
 }
 want := []string{"Alpha Phantom", "beta phantom", "zeta"}
 if !reflect.DeepEqual(targets, want) {
  t.Errorf("targets = %v, want %v", targets, want)
 }
}

func TestUnresolvedLinksKeepsFirstSeenSpelling(t *testing.T) {
 // arrange — two spellings share one folded key; walk order puts Note.md first
 idx := buildTempIndex(t, map[string]string{
  "Note.md":  "- [[Phantom]]\n",
  "Other.md": "- [[PHANTOM]]\n",
 })

 // act
 got := idx.UnresolvedLinks()

 // assert — one entry with the first-seen spelling and both refs
 if len(got) != 1 {
  t.Fatalf("want 1 unresolved link, got %d: %+v", len(got), got)
 }
 if got[0].Target != "Phantom" {
  t.Errorf("target = %q, want first-seen spelling Phantom", got[0].Target)
 }
 if len(got[0].Refs) != 2 || got[0].Refs[0].FromPage != "Note" || got[0].Refs[1].FromPage != "Other" {
  t.Errorf("refs = %+v, want Note then Other", got[0].Refs)
 }
}
```

- [x] Run it, confirm it fails (compile error — `UnresolvedLinks` undefined):

  ```bash
  go test ./internal/graph/ -run TestUnresolvedLinks
  ```

- [x] Write the minimal implementation.

  In `internal/graph/index.go`, add the spelling map to the `Index` struct, right after the `backlinks` field:

```go
 backlinks   map[string][]Ref  // keyed by strings.ToLower(target) — resolution is case-insensitive
 targetSpell map[string]string // folded target → first-seen original spelling (walk order)
```

  Initialize it in `BuildIndex` alongside the other maps:

```go
 idx := &Index{
  GraphPath:   graphPath,
  ByName:      make(map[string]*PageMeta),
  ByNameFold:  make(map[string]*PageMeta),
  backlinks:   make(map[string][]Ref),
  targetSpell: make(map[string]string),
 }
```

  Record the first-seen spelling in the second-pass link loop, before the existing append:

```go
  for _, lh := range links {
   key := strings.ToLower(lh.Target)
   if _, seen := idx.targetSpell[key]; !seen {
    idx.targetSpell[key] = lh.Target
   }
   idx.backlinks[key] = append(idx.backlinks[key], Ref{
```

  Create `internal/graph/unresolved.go`:

```go
package graph

import (
 "slices"
 "sort"
 "strings"
)

// UnresolvedLink is one wiki-link target that resolves to no page, with
// every reference to it across the graph.
type UnresolvedLink struct {
 Target string
 Refs   []Ref
}

// UnresolvedLinks returns every link target that Resolve cannot find —
// exact and case-folded lookup, matching how the TUI resolves links —
// each with a cloned snapshot of its references in index walk order.
// Targets are sorted case-insensitively; case-sensitive byte order
// breaks ties (defensive: folded keys are unique, so ties do not arise
// from the backlinks map).
func (idx *Index) UnresolvedLinks() []UnresolvedLink {
 var out []UnresolvedLink
 for key, refs := range idx.backlinks {
  if _, ok := idx.Resolve(idx.targetSpell[key]); ok {
   continue
  }
  out = append(out, UnresolvedLink{
   Target: idx.targetSpell[key],
   Refs:   slices.Clone(refs),
  })
 }
 sort.Slice(out, func(i, j int) bool {
  fi, fj := strings.ToLower(out[i].Target), strings.ToLower(out[j].Target)
  if fi != fj {
   return fi < fj
  }
  return out[i].Target < out[j].Target
 })
 return out
}
```

- [x] Run the tests, confirm they pass:

  ```bash
  go test ./internal/graph/
  ```

- [x] Run the commit gate, then commit:

  ```bash
  test -z "$(gofmt -l .)" && go vet ./... && "$(go env GOPATH)/bin/staticcheck" ./... && go test -race ./...
  git add internal/graph/index.go internal/graph/unresolved.go internal/graph/unresolved_test.go
  git commit -m "feat(graph): add Index.UnresolvedLinks accessor"
  ```

### Task 2: `internal/doctor` report assembly

**Files:** `internal/doctor/doctor.go` (create), `internal/doctor/doctor_test.go` (create).

- [x] Write the failing test — create `internal/doctor/doctor_test.go`:

```go
package doctor

import (
 "errors"
 "os"
 "path/filepath"
 "reflect"
 "strings"
 "testing"

 "git.fiatcode.dev/fiatcode/weft/v2/internal/graph"
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
 if rep.Pages != 7 || rep.Journals != 9 {
  t.Errorf("counts = %d pages / %d journals, want 7 / 9", rep.Pages, rep.Journals)
 }
 if !reflect.DeepEqual(rep.Orphans, []string{"Orphan"}) {
  t.Errorf("orphans = %v, want [Orphan]", rep.Orphans)
 }
 if len(rep.Unresolved) != 1 || rep.Unresolved[0].Target != "DoesNotExist" {
  t.Errorf("unresolved = %+v, want the DoesNotExist phantom", rep.Unresolved)
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
```

- [x] Run it, confirm it fails (compile error — package `doctor` does not exist):

  ```bash
  go test ./internal/doctor/
  ```

- [x] Write the minimal implementation — create `internal/doctor/doctor.go`:

```go
// Package doctor computes a read-only health report over a weft graph:
// unresolved wiki-links, orphan pages, unlinked mentions, and index
// warnings. It never writes to the graph.
package doctor

import (
 "fmt"
 "os"
 "sort"

 "git.fiatcode.dev/fiatcode/weft/v2/internal/graph"
 "git.fiatcode.dev/fiatcode/weft/v2/internal/search"
)

// MentionScanner returns the unlinked (bare-text) references to one page.
type MentionScanner func(graphPath, name string) ([]graph.UnlinkedRef, error)

// PageMentions collects one page's bare-text mentions elsewhere in the graph.
type PageMentions struct {
 Page string              // page that is mentioned
 Refs []graph.UnlinkedRef // mention locations
}

// Report is the full graph-health result.
type Report struct {
 GraphPath  string
 Pages      int // non-journal page count
 Journals   int
 Unresolved []graph.UnresolvedLink
 Orphans    []string       // non-journal page names with no backlinks from other pages
 Mentions   []PageMentions // ascending by Page; only pages with at least one ref
 Warnings   []string       // index warnings verbatim
}

// HasFindings reports whether any health signal fired.
func (r *Report) HasFindings() bool {
 return len(r.Unresolved) > 0 || len(r.Orphans) > 0 || len(r.Mentions) > 0 || len(r.Warnings) > 0
}

// Run computes the report using the ripgrep-backed mention scanner — the
// exact pipeline the backlinks panel uses (search.Mentions +
// graph.FilterUnlinked, target's own file excluded).
func Run(graphPath string) (*Report, error) {
 idx, err := graph.BuildIndex(graphPath)
 if err != nil {
  return nil, err
 }
 return run(idx, graphPath, func(gp, name string) ([]graph.UnlinkedRef, error) {
  hits, err := search.Mentions(gp, name)
  if err != nil {
   return nil, err
  }
  targetPath := ""
  if meta, ok := idx.Resolve(name); ok {
   targetPath = meta.Path
  }
  return graph.FilterUnlinked(hits, targetPath, readFile), nil
 })
}

// RunWith computes the report with a caller-supplied mention scanner.
func RunWith(graphPath string, mentions MentionScanner) (*Report, error) {
 idx, err := graph.BuildIndex(graphPath)
 if err != nil {
  return nil, err
 }
 return run(idx, graphPath, mentions)
}

// run assembles the report from a built index. Orphans and mention targets
// are non-journal pages only; journals stay in scope as mention sources
// because the scanner greps pages/ and journals/ alike.
func run(idx *graph.Index, graphPath string, mentions MentionScanner) (*Report, error) {
 rep := &Report{
  GraphPath:  graphPath,
  Unresolved: idx.UnresolvedLinks(),
  Warnings:   idx.Warnings,
 }
 var names []string
 for _, p := range idx.Pages {
  if p.IsJournal {
   rep.Journals++
   continue
  }
  rep.Pages++
  names = append(names, p.Name)
  if !hasOtherBacklink(idx, p.Name) {
   rep.Orphans = append(rep.Orphans, p.Name)
  }
 }
 sort.Strings(rep.Orphans)
 sort.Strings(names)
 for _, name := range names {
  refs, err := mentions(graphPath, name)
  if err != nil {
   return nil, fmt.Errorf("scan unlinked mentions for %q: %w", name, err)
  }
  if len(refs) > 0 {
   rep.Mentions = append(rep.Mentions, PageMentions{Page: name, Refs: refs})
  }
 }
 return rep, nil
}

// hasOtherBacklink reports whether any page other than name links to it —
// the same self-ref exclusion the backlinks panel applies.
func hasOtherBacklink(idx *graph.Index, name string) bool {
 for _, r := range idx.BacklinksTo(name) {
  if r.FromPage != name {
   return true
  }
 }
 return false
}

func readFile(path string) (string, error) {
 b, err := os.ReadFile(path)
 return string(b), err
}
```

- [x] Run the tests, confirm they pass:

  ```bash
  go test ./internal/doctor/
  ```

- [x] Run the commit gate, then commit:

  ```bash
  test -z "$(gofmt -l .)" && go vet ./... && "$(go env GOPATH)/bin/staticcheck" ./... && go test -race ./...
  git add internal/doctor/doctor.go internal/doctor/doctor_test.go
  git commit -m "feat(doctor): assemble graph-health report"
  ```

### Task 3: Fixture unlinked mention + real-ripgrep integration test

**Files:** `internal/doctor/doctor_test.go` (modify), `testdata/fixture-graph/pages/Workbench.md` (modify).

The fixture has no cross-page bare mention yet. Every existing "Alpha" occurrence is either `[[linked]]` or inside Alpha.md itself, so one added bare mention gives Alpha exactly one unlinked reference.

- [x] Write the failing test — append to `internal/doctor/doctor_test.go` (add `"os/exec"` to the imports):

```go
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
 if rep.Pages != 7 || rep.Journals != 9 {
  t.Errorf("counts = %d pages / %d journals, want 7 / 9", rep.Pages, rep.Journals)
 }
 if len(rep.Unresolved) != 1 || rep.Unresolved[0].Target != "DoesNotExist" {
  t.Errorf("unresolved = %+v, want the DoesNotExist phantom", rep.Unresolved)
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
```

- [x] Run it, confirm it fails (no mention exists yet — `mentions` is empty):

  ```bash
  go test ./internal/doctor/ -run TestRunAgainstFixture
  ```

- [x] Add the fixture line and restore the file's mtime. The picker golden sorts by fixture mtime; fresh checkouts have all fixture files tied at checkout time, so the edited file must stay tied or the golden churns in this working tree only:

  ```bash
  printf '%s\n' '- The bench manual lives on the Alpha shelf.' >> testdata/fixture-graph/pages/Workbench.md
  touch -r testdata/fixture-graph/pages/Alpha.md testdata/fixture-graph/pages/Workbench.md
  ```

- [x] Run the tests, confirm they pass:

  ```bash
  go test ./internal/doctor/
  ```

- [x] Run the full suite WITHOUT `-update` and confirm zero golden churn — every test must pass byte-identical:

  ```bash
  go test ./...
  ```

- [x] Run the commit gate, then commit:

  ```bash
  test -z "$(gofmt -l .)" && go vet ./... && "$(go env GOPATH)/bin/staticcheck" ./... && go test -race ./...
  git add internal/doctor/doctor_test.go testdata/fixture-graph/pages/Workbench.md
  git commit -m "test(doctor): real-ripgrep integration over the fixture"
  ```

### Task 4: Plain-text report rendering

**Files:** `internal/doctor/doctor.go` (modify), `internal/doctor/doctor_test.go` (modify).

- [x] Write the failing test — append to `internal/doctor/doctor_test.go`:

```go
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
```

- [x] Run it, confirm it fails (compile error — `WriteText` undefined):

  ```bash
  go test ./internal/doctor/ -run TestWriteText
  ```

- [x] Write the minimal implementation — add to `internal/doctor/doctor.go` (add `"io"` and `"strings"` to the imports):

```go
// WriteText renders the report as plain, uncolored text: header, a fixed
// four-row summary table (zeros included), then one detail section per
// non-zero signal in table order. Detail columns pad the leading name to
// the longest name in that section plus two spaces; locations join with
// " · ". A clean report ends with the single line "graph is clean".
func (r *Report) WriteText(w io.Writer) {
 fmt.Fprintf(w, "weft doctor · %s\n", r.GraphPath)
 fmt.Fprintf(w, "%d pages · %d journals\n\n", r.Pages, r.Journals)
 fmt.Fprintf(w, "  %-17s %d\n", "unresolved links", len(r.Unresolved))
 fmt.Fprintf(w, "  %-17s %d\n", "orphan pages", len(r.Orphans))
 fmt.Fprintf(w, "  %-17s %d\n", "unlinked mentions", r.mentionTotal())
 fmt.Fprintf(w, "  %-17s %d\n", "index warnings", len(r.Warnings))
 if !r.HasFindings() {
  fmt.Fprintln(w, "\ngraph is clean")
  return
 }
 if len(r.Unresolved) > 0 {
  fmt.Fprintln(w, "\nUnresolved links")
  width := 0
  for _, u := range r.Unresolved {
   if len(u.Target) > width {
    width = len(u.Target)
   }
  }
  for _, u := range r.Unresolved {
   fmt.Fprintf(w, "  %-*s  %s\n", width, u.Target, joinRefLocations(u.Refs))
  }
 }
 if len(r.Orphans) > 0 {
  fmt.Fprintln(w, "\nOrphan pages")
  for _, name := range r.Orphans {
   fmt.Fprintf(w, "  %s\n", name)
  }
 }
 if len(r.Mentions) > 0 {
  fmt.Fprintln(w, "\nUnlinked mentions")
  width := 0
  for _, m := range r.Mentions {
   if len(m.Page) > width {
    width = len(m.Page)
   }
  }
  for _, m := range r.Mentions {
   fmt.Fprintf(w, "  %-*s  %s\n", width, m.Page, joinMentionLocations(m.Refs))
  }
 }
 if len(r.Warnings) > 0 {
  fmt.Fprintln(w, "\nIndex warnings")
  for _, msg := range r.Warnings {
   fmt.Fprintf(w, "  %s\n", msg)
  }
 }
}

func (r *Report) mentionTotal() int {
 n := 0
 for _, m := range r.Mentions {
  n += len(m.Refs)
 }
 return n
}

func joinRefLocations(refs []graph.Ref) string {
 locs := make([]string, 0, len(refs))
 for _, r := range refs {
  locs = append(locs, fmt.Sprintf("%s:%d", r.FromPage, r.LineNumber))
 }
 return strings.Join(locs, " · ")
}

func joinMentionLocations(refs []graph.UnlinkedRef) string {
 locs := make([]string, 0, len(refs))
 for _, r := range refs {
  locs = append(locs, fmt.Sprintf("%s:%d", r.PageName, r.Line))
 }
 return strings.Join(locs, " · ")
}
```

- [x] Run the tests, confirm they pass:

  ```bash
  go test ./internal/doctor/
  ```

- [x] Run the commit gate, then commit:

  ```bash
  test -z "$(gofmt -l .)" && go vet ./... && "$(go env GOPATH)/bin/staticcheck" ./... && go test -race ./...
  git add internal/doctor/doctor.go internal/doctor/doctor_test.go
  git commit -m "feat(doctor): render plain-text report"
  ```

### Task 5: CLI wiring and README

**Files:** `cmd/weft/main.go` (modify), `cmd/weft/main_test.go` (modify), `README.md` (modify).

- [x] Write the failing test — append to `cmd/weft/main_test.go` (add `"bytes"` and `"os/exec"` to the imports):

```go
// writeTempGraph materialises a throwaway graph and returns its root.
func writeTempGraph(t *testing.T, files map[string]string) string {
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

func TestRunDoctorExitCodes(t *testing.T) {
 if _, err := exec.LookPath("rg"); err != nil {
  t.Skip("rg not on PATH; install ripgrep to run this test")
 }
 fixture, err := filepath.Abs("../../testdata/fixture-graph")
 if err != nil {
  t.Fatal(err)
 }
 clean := writeTempGraph(t, map[string]string{
  "pages/Solo.md":  "- links to [[Other]]\n",
  "pages/Other.md": "- links to [[Solo]]\n",
 })

 cases := []struct {
  name  string
  args  []string
  graph string
  want  int
 }{
  {"clean graph exits 0", []string{"--graph", clean}, "", 0},
  {"findings exit 1", []string{"--graph", fixture}, "", 1},
  {"missing graph dir exits 2", []string{"--graph", filepath.Join(t.TempDir(), "nope")}, "", 2},
  {"unknown flag exits 2", []string{"--bogus"}, fixture, 2},
  {"positional arg exits 2", []string{"extra"}, fixture, 2},
  {"global graph fallback finds fixture findings", nil, fixture, 1},
  {"doctor flag wins over findings-laden global", []string{"--graph", clean}, fixture, 0},
 }
 for _, tc := range cases {
  t.Run(tc.name, func(t *testing.T) {
   var buf bytes.Buffer
   if got := runDoctor(tc.args, tc.graph, &buf); got != tc.want {
    t.Errorf("runDoctor(%v, %q) = %d, want %d", tc.args, tc.graph, got, tc.want)
   }
  })
 }
}

func TestRunDoctorHelpPrintsUsage(t *testing.T) {
 // arrange + act
 var buf bytes.Buffer
 got := runDoctor([]string{"-h"}, "ignored", &buf)

 // assert
 if got != 0 {
  t.Errorf("-h exit = %d, want 0", got)
 }
 if !strings.Contains(buf.String(), "usage: weft doctor") {
  t.Errorf("usage not printed to stdout, got %q", buf.String())
 }
}
```

  (Add `"strings"` to the imports too.)

- [x] Run it, confirm it fails (compile error — `runDoctor` undefined):

  ```bash
  go test ./cmd/weft/ -run TestRunDoctor
  ```

- [x] Write the minimal implementation in `cmd/weft/main.go`.

  Add `"errors"` and `"git.fiatcode.dev/fiatcode/weft/v2/internal/doctor"` to the imports.

  Replace the stat-based validation block in `main()` and insert the dispatch immediately after it, before `initDebugLog`:

```go
 if err := validateGraphPath(graphPath); err != nil {
  fmt.Fprintf(os.Stderr, "weft: %v\n", err)
  os.Exit(2)
 }

 if flag.Arg(0) == "doctor" {
  os.Exit(runDoctor(flag.Args()[1:], graphPath, os.Stdout))
 }
```

  Add the extracted validator (behavior-identical to the old inline messages) plus the subcommand:

```go
// validateGraphPath reports whether graphPath is an accessible directory.
func validateGraphPath(graphPath string) error {
 info, err := os.Stat(graphPath)
 if err != nil {
  return fmt.Errorf("graph path %q is not accessible: %w", graphPath, err)
 }
 if !info.IsDir() {
  return fmt.Errorf("graph path %q is not a directory", graphPath)
 }
 return nil
}

const doctorUsage = "usage: weft doctor [--graph PATH]"

// runDoctor executes the headless doctor subcommand and returns its exit
// code: 0 clean, 1 findings, 2 operational error. doctorFlagArgs are the
// args after the "doctor" word; globalGraph is the already-validated graph
// path resolved from the global --graph flag / $WEFT_GRAPH.
func runDoctor(doctorFlagArgs []string, globalGraph string, stdout io.Writer) int {
 fs := flag.NewFlagSet("doctor", flag.ContinueOnError)
 fs.SetOutput(io.Discard) // usage printing is owned by this function
 graphFlag := fs.String("graph", "", "path to Logseq graph (overrides the global --graph and $WEFT_GRAPH)")
 err := fs.Parse(doctorFlagArgs)
 switch {
 case errors.Is(err, flag.ErrHelp):
  fmt.Fprintln(stdout, doctorUsage)
  return 0
 case err != nil, fs.NArg() > 0:
  fmt.Fprintln(os.Stderr, doctorUsage)
  return 2
 }
 graphPath := globalGraph
 if *graphFlag != "" {
  graphPath = *graphFlag
  if err := validateGraphPath(graphPath); err != nil {
   fmt.Fprintf(os.Stderr, "weft: %v\n", err)
   return 2
  }
 }
 report, err := doctor.Run(graphPath)
 if err != nil {
  fmt.Fprintf(os.Stderr, "weft: doctor: %v\n", err)
  return 2
 }
 report.WriteText(stdout)
 if report.HasFindings() {
  return 1
 }
 return 0
}
```

- [x] Run the tests, confirm they pass:

  ```bash
  go test ./cmd/weft/
  ```

- [x] Update `README.md`: insert a new section between `## Usage` and `## Keys`:

````markdown
## Doctor

```bash
weft doctor                  # check the graph from --graph / $WEFT_GRAPH
weft doctor --graph PATH     # explicit path
```

`weft doctor` walks the graph once and prints a health report. It is read-only and never writes to the graph.

- **unresolved links** — `[[wiki-links]]` whose target page does not exist
- **orphan pages** — pages no other page links to (journals excluded)
- **unlinked mentions** — bare-text mentions that could become links
- **index warnings** — problems the index walk noticed (unreadable files, name collisions, subdirectories)

Exit codes: `0` graph is clean, `1` findings exist, `2` operational error (bad graph path, missing ripgrep, failed scan).
````

- [x] Smoke-test the real binary against the fixture:

  ```bash
  go run ./cmd/weft --graph testdata/fixture-graph doctor; echo "exit: $?"
  ```

  Expect the report with exit 1 (the fixture has findings).

- [x] Run the commit gate, then commit:

  ```bash
  test -z "$(gofmt -l .)" && go vet ./... && "$(go env GOPATH)/bin/staticcheck" ./... && go test -race ./...
  git add cmd/weft/main.go cmd/weft/main_test.go README.md
  git commit -m "feat: weft doctor subcommand"
  ```

## Final gate and completion

- [ ] Whole-branch review of the cumulative diff (`git diff main...weft-doctor`) against the spec: CLI surface, report model, output format, exit codes, scope rules — every section.
- [ ] Push the branch and open one Forgejo PR using the forgejo skill (`tea` CLI).
- [ ] After merge: move the backlog bullet from `Open` to `Done` in `${WEFT_GRAPH}/pages/weft Backlog.md` preserving its voice, and mark the `- TODO weft doctor ...` bullet in `${WEFT_GRAPH}/journals/2026_07_23.md` as DONE preserving its wording.
- [ ] Journal the completed session through the journal-update skill.

## Minor findings (recorded, never loop)

- Task 2 (commit e59e9a4): originally recorded as a test-name deviation (`...FailsReport` vs plan's `...FailsRun`) — the final review disproved it via `git log -S`: the committed name was always the plan's `TestRunWithScannerErrorFailsRun`; the note traced to a misread subagent report. Struck, nothing to fix.

- Task 3 (commit 7ab1242): the `touch -r` mtime-restoration step was skipped by the implementer. Reviewer confirmed zero golden impact (picker test zeroes mtimes before rendering; full suite passed byte-identical without `-update`). Orchestrator ran the `touch -r` after review to restore spec state. Future fixture edits must run it as written.
