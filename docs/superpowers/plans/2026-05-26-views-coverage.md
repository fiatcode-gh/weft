# Views Coverage Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Lift `internal/views` from 44.5% → ~70%+ line coverage by writing tests for the previously-untested overlays and navigation paths in `search.go`, `backlinks.go`, `app.go`, plus the 0%-coverage orphans and state-machine branches across `picker.go`, `todos.go`, `page.go`.

**Architecture:** Three layers of tests using existing patterns:

1. Pure-function unit tests for stateless helpers (`matchesWithin`, `highlightMatches`, `scrollWindow`, `shortPath`, `innerWidth`, `visibleRows`).
2. State-machine tests calling `Update` directly and asserting model fields — no `teatest` round-trip needed when behaviour is the assertion.
3. `teatest.RequireEqualOutput` goldens for rendered frame snapshots, regenerated with `go test ./... -update`.

Real `rg` runs against `testdata/fixture-graph` for `SearchCmd` / `runRipgrep`; `t.Skip` when `rg` is missing on PATH.

**Tech Stack:** Go 1.26, Bubble Tea v1, lipgloss, `github.com/charmbracelet/x/exp/teatest`, ripgrep 14.x.

**Spec:** `docs/superpowers/specs/2026-05-26-views-coverage-design.md`.

---

## Background context for the engineer

Every view test sets these env vars **before** doing anything that touches lipgloss:

```go
t.Setenv("TERM", "dumb")
t.Setenv("NO_COLOR", "1")
```

The lipgloss colour profile is package-global and primed lazily by the first test that exercises the renderer. Skipping this makes goldens flicker between truecolour and 8-colour depending on test ordering.

Reusable helper that already exists in `internal/views/page_test.go`:

```go
func loadFixture(t *testing.T) *graph.Index {
    t.Helper()
    abs, err := filepath.Abs("../../testdata/fixture-graph")
    if err != nil { t.Fatal(err) }
    idx, err := graph.BuildIndex(abs)
    if err != nil { t.Fatal(err) }
    return idx
}
```

Use it in every new test that needs an index. Don't reintroduce it locally.

And in `internal/views/app_test.go`:

```go
func bootApp(t *testing.T) *App { ... }
```

Use it for any test that needs the full `App` wired up with PageView, history, and a real window size.

**Golden update workflow** (from `AGENTS.md`):

```bash
go test ./... -update          # regenerate goldens
git diff internal/views/testdata/ # visually verify intentional changes
git add internal/views/testdata/
```

---

# Unit 1: Grow the fixture

Adds the pages and journals needed to drive multi-hit search, multi-source backlinks, and picker scroll windowing. Existing pages (`Alpha`, `Beta`, `proj/nested`) and existing journals (`2026_05_23`, `2026_05_24`) **must remain untouched** — every existing test references them by name.

### Task 1.1: Add new fixture pages

**Files:**
- Create: `testdata/fixture-graph/pages/Hub.md`
- Create: `testdata/fixture-graph/pages/kb___notes.md`
- Create: `testdata/fixture-graph/pages/Workbench.md`
- Create: `testdata/fixture-graph/pages/Orphan.md`

- [ ] **Step 1: Write `Hub.md`** (target of ≥3 backlinks)

```markdown
title:: Hub
tags:: fixture

- The hub page. Linked from [[Alpha]], [[Beta]], and today's journal.
- A self-mention of [[Hub]] should not count as a backlink.
- TODO Tidy up the hub
```

- [ ] **Step 2: Write `kb___notes.md`** (second slash-encoded page; logical name `kb/notes`)

```markdown
title:: kb/notes
tags:: fixture

- Knowledge-base notes under the kb namespace. Mentions [[Hub]] in passing.
- LATER Index the kb namespace
```

- [ ] **Step 3: Write `Workbench.md`** (mixed workflow markers)

```markdown
title:: Workbench
tags:: fixture

- TODO Replace the bench grinder belt
- LATER [#C] Sharpen the lathe chisels
- DOING Re-flatten the table saw fence
- WAITING Quote on cast-iron wing
- DONE Oil the tool chest hinges
```

- [ ] **Step 4: Write `Orphan.md`** (no in/out links — exercises degenerate paths)

```markdown
title:: Orphan
tags:: fixture

- Standalone page with no wiki-links in either direction.
- A second bullet, also unlinked.
```

- [ ] **Step 5: Commit**

```bash
git add testdata/fixture-graph/pages/Hub.md \
        testdata/fixture-graph/pages/kb___notes.md \
        testdata/fixture-graph/pages/Workbench.md \
        testdata/fixture-graph/pages/Orphan.md
git commit -m "test: add fixture pages for coverage work"
```

### Task 1.2: Add new fixture journals and link Hub from one

**Files:**
- Create: `testdata/fixture-graph/journals/2026_05_25.md`
- Create: `testdata/fixture-graph/journals/2026_05_22.md`
- Create: `testdata/fixture-graph/journals/2026_05_15.md`
- Create: `testdata/fixture-graph/journals/2026_05_01.md`
- Create: `testdata/fixture-graph/journals/2026_04_20.md`
- Create: `testdata/fixture-graph/journals/2026_03_15.md`
- Create: `testdata/fixture-graph/journals/2026_01_10.md`

- [ ] **Step 1: Write `2026_05_25.md`**

```markdown
- Linked [[Hub]] today and noted [[Workbench]] follow-ups.
- TODO Try the new picker
```

- [ ] **Step 2: Write `2026_05_22.md`**

```markdown
- Routine entry, no links.
- LATER Audit fixture coverage
```

- [ ] **Step 3: Write `2026_05_15.md`**

```markdown
- Saw [[Alpha]] again. Quiet day.
```

- [ ] **Step 4: Write `2026_05_01.md`**

```markdown
- Month boundary. References [[kb/notes]].
```

- [ ] **Step 5: Write `2026_04_20.md`**

```markdown
- Past month entry, no links.
```

- [ ] **Step 6: Write `2026_03_15.md`**

```markdown
- Older entry. Mentions [[Beta]] in a fenced code block:
  ```
  not a link to [[NotALink]]
  ```
- One ordinary bullet.
```

- [ ] **Step 7: Write `2026_01_10.md`**

```markdown
- Oldest entry. WAITING on permit renewal.
```

- [ ] **Step 8: Commit**

```bash
git add testdata/fixture-graph/journals/2026_05_25.md \
        testdata/fixture-graph/journals/2026_05_22.md \
        testdata/fixture-graph/journals/2026_05_15.md \
        testdata/fixture-graph/journals/2026_05_01.md \
        testdata/fixture-graph/journals/2026_04_20.md \
        testdata/fixture-graph/journals/2026_03_15.md \
        testdata/fixture-graph/journals/2026_01_10.md
git commit -m "test: add fixture journals for coverage work"
```

### Task 1.3: Regenerate goldens, visually diff, commit

The new fixture content changes the **todos** dashboard (Workbench adds 3 markers, journals add 2). Picker, page, and help goldens should remain stable. Confirm and regenerate.

- [ ] **Step 1: Run full test suite to see which goldens fail**

```bash
go test ./internal/views -v 2>&1 | tail -40
```

Expected: `TestTodosDashboardAllFilter` and possibly `TestTodosDashboardLaterFilter` fail with diff against golden. Other view tests pass.

If picker/page goldens also fail, stop and re-examine which tests filter against which page set — Alpha/Beta/proj/nested were assumed untouched.

- [ ] **Step 2: Regenerate goldens**

```bash
go test ./... -update
```

- [ ] **Step 3: Visually diff regenerated goldens**

```bash
git diff internal/views/testdata/
```

Expected diff: `TestTodosDashboardAllFilter.golden` and `TestTodosDashboardLaterFilter.golden` gain Workbench rows plus the journal TODO/LATER entries. No other goldens should appear in the diff.

- [ ] **Step 4: Confirm clean test run**

```bash
go vet ./... && go test ./...
```

Expected: PASS, all packages.

- [ ] **Step 5: Commit goldens**

```bash
git add internal/views/testdata/
git commit -m "test: regenerate goldens for grown fixture"
```

---

# Unit 2: `internal/views/search.go` tests

All tests live in `internal/views/search_test.go`. The existing `TestParseRipgrepJSON` stays — we're adding around it.

### Task 2.1: SearchView constructor and trivial accessors

**Files:**
- Modify: `internal/views/search_test.go`

- [ ] **Step 1: Add `TestNewSearchViewIndexesPathToName`**

```go
func TestNewSearchViewIndexesPathToName(t *testing.T) {
    t.Setenv("TERM", "dumb")
    t.Setenv("NO_COLOR", "1")
    idx := loadFixture(t)
    s := NewSearchView(idx, 100, 30)
    if s.width != 100 || s.height != 30 {
        t.Errorf("size: want 100x30, got %dx%d", s.width, s.height)
    }
    // Every indexed page maps its absolute path to its logical name.
    for _, p := range idx.Pages {
        if got := s.pathToName[p.Path]; got != p.Name {
            t.Errorf("pathToName[%q] = %q, want %q", p.Path, got, p.Name)
        }
    }
}
```

- [ ] **Step 2: Add `TestSearchAccessorsAndSetSize`**

```go
func TestSearchAccessorsAndSetSize(t *testing.T) {
    t.Setenv("TERM", "dumb")
    t.Setenv("NO_COLOR", "1")
    s := NewSearchView(loadFixture(t), 80, 24)
    if s.Query() != "" {
        t.Errorf("fresh Query: want \"\", got %q", s.Query())
    }
    s.SetQuery("foo")
    if s.Query() != "foo" {
        t.Errorf("after SetQuery: want \"foo\", got %q", s.Query())
    }
    s.SetSize(120, 40)
    if s.width != 120 || s.height != 40 {
        t.Errorf("SetSize: want 120x40, got %dx%d", s.width, s.height)
    }
}
```

- [ ] **Step 3: Run**

```bash
go test ./internal/views -run 'TestNewSearchView|TestSearchAccessors' -v
```

Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add internal/views/search_test.go
git commit -m "test: cover SearchView constructor and accessors"
```

### Task 2.2: `Update` — query typing, backspace, space

- [ ] **Step 1: Add `TestSearchUpdateTypesIntoQuery`**

```go
func TestSearchUpdateTypesIntoQuery(t *testing.T) {
    t.Setenv("TERM", "dumb")
    t.Setenv("NO_COLOR", "1")
    s := NewSearchView(loadFixture(t), 80, 24)
    for _, k := range []string{"a", "l", "p"} {
        hit, accept, cancel, cmd := s.Update(k, "/tmp/x")
        if hit != nil || accept || cancel || cmd != nil {
            t.Errorf("typing %q: want (nil,false,false,nil), got (%v,%v,%v,%v)",
                k, hit, accept, cancel, cmd)
        }
    }
    if s.query != "alp" {
        t.Errorf("query: want \"alp\", got %q", s.query)
    }
}
```

- [ ] **Step 2: Add `TestSearchUpdateBackspace`**

```go
func TestSearchUpdateBackspace(t *testing.T) {
    t.Setenv("TERM", "dumb")
    t.Setenv("NO_COLOR", "1")
    s := NewSearchView(loadFixture(t), 80, 24)
    s.SetQuery("abc")
    s.hits = []SearchHit{{FilePath: "x", Line: 1, Context: "y"}} // pretend results exist

    s.Update("backspace", "/tmp/x")
    if s.query != "ab" {
        t.Errorf("after first backspace: want \"ab\", got %q", s.query)
    }
    if s.hits != nil {
        t.Errorf("after backspace: hits must clear, got %+v", s.hits)
    }

    s.Update("backspace", "/tmp/x")
    s.Update("backspace", "/tmp/x")
    // Empty query — backspace is a no-op now.
    s.Update("backspace", "/tmp/x")
    if s.query != "" {
        t.Errorf("after draining: want \"\", got %q", s.query)
    }
}
```

- [ ] **Step 3: Add `TestSearchUpdateSpaceVariants`**

Both `" "` and `"space"` should append a space — see source comment.

```go
func TestSearchUpdateSpaceVariants(t *testing.T) {
    t.Setenv("TERM", "dumb")
    t.Setenv("NO_COLOR", "1")
    s := NewSearchView(loadFixture(t), 80, 24)
    s.SetQuery("foo")

    s.Update(" ", "/tmp/x")
    if s.query != "foo " {
        t.Errorf("after literal space: want \"foo \", got %q", s.query)
    }

    s.Update("space", "/tmp/x")
    if s.query != "foo  " {
        t.Errorf("after named space: want \"foo  \", got %q", s.query)
    }
}
```

- [ ] **Step 4: Run**

```bash
go test ./internal/views -run 'TestSearchUpdate(TypesIntoQuery|Backspace|Space)' -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/views/search_test.go
git commit -m "test: cover SearchView query input behavior"
```

### Task 2.3: `Update` — selection navigation

- [ ] **Step 1: Add `TestSearchSelectionBounds`**

```go
func TestSearchSelectionBounds(t *testing.T) {
    t.Setenv("TERM", "dumb")
    t.Setenv("NO_COLOR", "1")
    s := NewSearchView(loadFixture(t), 80, 24)
    s.hits = []SearchHit{
        {FilePath: "a", Line: 1, Context: "x"},
        {FilePath: "b", Line: 2, Context: "y"},
        {FilePath: "c", Line: 3, Context: "z"},
    }

    // up at top is a no-op (sel stays 0).
    s.Update("up", "/tmp/x")
    if s.sel != 0 {
        t.Errorf("up at top: want sel 0, got %d", s.sel)
    }

    // down advances.
    s.Update("down", "/tmp/x")
    if s.sel != 1 {
        t.Errorf("after down: want sel 1, got %d", s.sel)
    }
    // ctrl+j is an alias for down.
    s.Update("ctrl+j", "/tmp/x")
    if s.sel != 2 {
        t.Errorf("after ctrl+j: want sel 2, got %d", s.sel)
    }
    // down at bottom is a no-op.
    s.Update("down", "/tmp/x")
    if s.sel != 2 {
        t.Errorf("down at bottom: want sel 2, got %d", s.sel)
    }
    // ctrl+k is an alias for up.
    s.Update("ctrl+k", "/tmp/x")
    if s.sel != 1 {
        t.Errorf("after ctrl+k: want sel 1, got %d", s.sel)
    }
}
```

- [ ] **Step 2: Run**

```bash
go test ./internal/views -run TestSearchSelectionBounds -v
```

Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add internal/views/search_test.go
git commit -m "test: cover SearchView selection navigation"
```

### Task 2.4: `Update` — enter dispatch and esc cancel

- [ ] **Step 1: Add `TestSearchEnterEmptyQueryNoop`**

```go
func TestSearchEnterEmptyQueryNoop(t *testing.T) {
    t.Setenv("TERM", "dumb")
    t.Setenv("NO_COLOR", "1")
    s := NewSearchView(loadFixture(t), 80, 24)
    hit, accept, cancel, cmd := s.Update("enter", "/tmp/x")
    if hit != nil || accept || cancel || cmd != nil {
        t.Errorf("enter on empty query: want all-zero, got (%v,%v,%v,%v)",
            hit, accept, cancel, cmd)
    }
}
```

- [ ] **Step 2: Add `TestSearchEnterFirstTimeRunsCmd`**

```go
func TestSearchEnterFirstTimeRunsCmd(t *testing.T) {
    t.Setenv("TERM", "dumb")
    t.Setenv("NO_COLOR", "1")
    s := NewSearchView(loadFixture(t), 80, 24)
    s.SetQuery("Beta")

    hit, accept, cancel, cmd := s.Update("enter", "/tmp/x")
    if hit != nil || accept || cancel {
        t.Errorf("enter to launch: want non-accept non-cancel nil hit, got (%v,%v,%v)",
            hit, accept, cancel)
    }
    if cmd == nil {
        t.Errorf("enter to launch: want non-nil cmd, got nil")
    }
    if !s.running {
        t.Errorf("enter to launch: running flag should be set")
    }
}
```

- [ ] **Step 3: Add `TestSearchEnterWhileRunningIsNoop`**

```go
func TestSearchEnterWhileRunningIsNoop(t *testing.T) {
    t.Setenv("TERM", "dumb")
    t.Setenv("NO_COLOR", "1")
    s := NewSearchView(loadFixture(t), 80, 24)
    s.SetQuery("Beta")
    s.running = true

    hit, accept, cancel, cmd := s.Update("enter", "/tmp/x")
    if hit != nil || accept || cancel || cmd != nil {
        t.Errorf("enter while running: want all-zero, got (%v,%v,%v,%v)",
            hit, accept, cancel, cmd)
    }
}
```

- [ ] **Step 4: Add `TestSearchEnterWithHitsOpensSelection`**

```go
func TestSearchEnterWithHitsOpensSelection(t *testing.T) {
    t.Setenv("TERM", "dumb")
    t.Setenv("NO_COLOR", "1")
    s := NewSearchView(loadFixture(t), 80, 24)
    s.SetQuery("Beta")
    s.hits = []SearchHit{
        {FilePath: "/p/Alpha.md", Line: 3, Context: "links to Beta"},
        {FilePath: "/p/Hub.md", Line: 5, Context: "Beta sometimes"},
    }
    s.sel = 1

    hit, accept, cancel, cmd := s.Update("enter", "/tmp/x")
    if !accept || cancel || cmd != nil {
        t.Errorf("enter with hits: want accept, got accept=%v cancel=%v cmd=%v",
            accept, cancel, cmd)
    }
    if hit == nil || hit.FilePath != "/p/Hub.md" || hit.Line != 5 {
        t.Errorf("returned hit: want Hub.md:5, got %+v", hit)
    }
}
```

- [ ] **Step 5: Add `TestSearchEscCancels`**

```go
func TestSearchEscCancels(t *testing.T) {
    t.Setenv("TERM", "dumb")
    t.Setenv("NO_COLOR", "1")
    s := NewSearchView(loadFixture(t), 80, 24)
    hit, accept, cancel, cmd := s.Update("esc", "/tmp/x")
    if hit != nil || accept || !cancel || cmd != nil {
        t.Errorf("esc: want cancel only, got (%v,%v,%v,%v)",
            hit, accept, cancel, cmd)
    }
}
```

- [ ] **Step 6: Run**

```bash
go test ./internal/views -run 'TestSearch(Enter|Esc)' -v
```

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/views/search_test.go
git commit -m "test: cover SearchView enter dispatch and esc cancel"
```

### Task 2.5: `Apply` and `searchDoneMsg` handling

- [ ] **Step 1: Add `TestSearchApplyPopulatesHits`**

```go
func TestSearchApplyPopulatesHits(t *testing.T) {
    t.Setenv("TERM", "dumb")
    t.Setenv("NO_COLOR", "1")
    s := NewSearchView(loadFixture(t), 80, 24)
    s.running = true
    s.sel = 5 // pretend the previous result list was longer

    s.Apply(searchDoneMsg{hits: []SearchHit{
        {FilePath: "/p/A.md", Line: 1, Context: "x"},
        {FilePath: "/p/B.md", Line: 2, Context: "y"},
    }})

    if s.running {
        t.Errorf("after Apply: running should clear, still true")
    }
    if len(s.hits) != 2 {
        t.Errorf("after Apply: hits len want 2, got %d", len(s.hits))
    }
    if s.sel != 0 {
        t.Errorf("after Apply: sel must clamp to 0 when prior sel exceeded new len, got %d", s.sel)
    }
}
```

- [ ] **Step 2: Add `TestSearchApplyKeepsValidSel`**

```go
func TestSearchApplyKeepsValidSel(t *testing.T) {
    t.Setenv("TERM", "dumb")
    t.Setenv("NO_COLOR", "1")
    s := NewSearchView(loadFixture(t), 80, 24)
    s.sel = 1
    s.Apply(searchDoneMsg{hits: []SearchHit{
        {FilePath: "/p/A.md", Line: 1}, {FilePath: "/p/B.md", Line: 2},
        {FilePath: "/p/C.md", Line: 3},
    }})
    if s.sel != 1 {
        t.Errorf("sel within new bounds should survive: want 1, got %d", s.sel)
    }
}
```

- [ ] **Step 3: Add `TestSearchApplyRecordsError`**

```go
func TestSearchApplyRecordsError(t *testing.T) {
    t.Setenv("TERM", "dumb")
    t.Setenv("NO_COLOR", "1")
    s := NewSearchView(loadFixture(t), 80, 24)
    s.running = true
    s.Apply(searchDoneMsg{err: errors.New("boom")})
    if s.running {
        t.Errorf("after Apply: running should clear even on error")
    }
    if s.err == nil || s.err.Error() != "boom" {
        t.Errorf("error: want \"boom\", got %v", s.err)
    }
}
```

Add the import if missing:

```go
import "errors"
```

- [ ] **Step 4: Run**

```bash
go test ./internal/views -run TestSearchApply -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/views/search_test.go
git commit -m "test: cover SearchView Apply state transitions"
```

### Task 2.6: `runRipgrep` and `SearchCmd` against real `rg`

These exercise the real `rg` binary against `testdata/fixture-graph`. Skip when `rg` is missing from PATH.

- [ ] **Step 1: Add a small helper at the top of the file**

```go
func skipIfNoRipgrep(t *testing.T) {
    t.Helper()
    if _, err := exec.LookPath("rg"); err != nil {
        t.Skip("rg not on PATH; install ripgrep to run search tests")
    }
}
```

Add the import if missing:

```go
import "os/exec"
```

- [ ] **Step 2: Add `TestRunRipgrepFindsHits`**

```go
func TestRunRipgrepFindsHits(t *testing.T) {
    skipIfNoRipgrep(t)
    abs, err := filepath.Abs("../../testdata/fixture-graph")
    if err != nil { t.Fatal(err) }

    out, err := runRipgrep(abs, "Beta")
    if err != nil {
        t.Fatalf("runRipgrep err: %v", err)
    }
    hits := parseRipgrepJSON(out)
    if len(hits) == 0 {
        t.Fatalf("expected at least one hit for \"Beta\"")
    }
    // Every hit must point inside the fixture graph.
    for _, h := range hits {
        if !strings.HasPrefix(h.FilePath, abs) {
            t.Errorf("hit path outside fixture: %s", h.FilePath)
        }
    }
}
```

Add the imports if missing:

```go
import (
    "path/filepath"
    "strings"
)
```

- [ ] **Step 3: Add `TestRunRipgrepNoMatchReturnsEmpty`**

`rg` exits 1 when nothing matches; `runRipgrep` must swallow that.

```go
func TestRunRipgrepNoMatchReturnsEmpty(t *testing.T) {
    skipIfNoRipgrep(t)
    abs, err := filepath.Abs("../../testdata/fixture-graph")
    if err != nil { t.Fatal(err) }

    out, err := runRipgrep(abs, "thisstringshouldnotexistanywhere_xyzzy_1234")
    if err != nil {
        t.Fatalf("no-match should not error, got: %v", err)
    }
    if hits := parseRipgrepJSON(out); len(hits) != 0 {
        t.Errorf("no-match: want 0 hits, got %d", len(hits))
    }
}
```

- [ ] **Step 4: Add `TestRunRipgrepMissingDirsReturnsNil`**

When neither `pages/` nor `journals/` exists under the graph path, `runRipgrep` short-circuits without invoking `rg`.

```go
func TestRunRipgrepMissingDirsReturnsNil(t *testing.T) {
    // Intentionally no skipIfNoRipgrep — this path never invokes rg.
    tmp := t.TempDir()
    out, err := runRipgrep(tmp, "anything")
    if err != nil {
        t.Errorf("missing dirs: want nil err, got %v", err)
    }
    if out != nil {
        t.Errorf("missing dirs: want nil out, got %q", out)
    }
}
```

- [ ] **Step 5: Add `TestSearchCmdRoundtrip`**

`SearchCmd` returns a `tea.Cmd`; invoking it produces a `searchDoneMsg`.

```go
func TestSearchCmdRoundtrip(t *testing.T) {
    skipIfNoRipgrep(t)
    abs, err := filepath.Abs("../../testdata/fixture-graph")
    if err != nil { t.Fatal(err) }

    s := NewSearchView(loadFixture(t), 80, 24)
    s.SetQuery("Beta")
    cmd := s.SearchCmd(abs)
    if cmd == nil {
        t.Fatal("SearchCmd returned nil")
    }
    msg := cmd()
    done, ok := msg.(searchDoneMsg)
    if !ok {
        t.Fatalf("want searchDoneMsg, got %T", msg)
    }
    if done.err != nil {
        t.Errorf("done.err: %v", done.err)
    }
    if len(done.hits) == 0 {
        t.Errorf("done.hits: expected at least one")
    }
}
```

- [ ] **Step 6: Run**

```bash
go test ./internal/views -run 'TestRunRipgrep|TestSearchCmd' -v
```

Expected: PASS (or SKIP on machines without rg).

- [ ] **Step 7: Commit**

```bash
git add internal/views/search_test.go
git commit -m "test: cover runRipgrep and SearchCmd against fixture"
```

### Task 2.7: `hitLabel`, `shortPath`, pure helpers

- [ ] **Step 1: Add `TestHitLabelKnownVsUnknown`**

```go
func TestHitLabelKnownVsUnknown(t *testing.T) {
    t.Setenv("TERM", "dumb")
    t.Setenv("NO_COLOR", "1")
    idx := loadFixture(t)
    s := NewSearchView(idx, 80, 24)

    // Pick any indexed page and confirm the label resolves to the logical name.
    for _, p := range idx.Pages {
        if got := s.hitLabel(p.Path); got != p.Name {
            t.Errorf("known path %q: want %q, got %q", p.Path, p.Name, got)
        }
        break
    }
    // Unknown path falls back to shortPath.
    if got, want := s.hitLabel("/totally/elsewhere/file.md"), "elsewhere/file.md"; got != want {
        t.Errorf("unknown path label: want %q, got %q", want, got)
    }
}
```

- [ ] **Step 2: Add `TestShortPath`**

```go
func TestShortPath(t *testing.T) {
    cases := []struct{ in, want string }{
        {"/a/b/c/file.md", "c/file.md"},
        {"a/b.md", "a/b.md"},
        {"singlename", "singlename"},
        {"", ""},
    }
    for _, c := range cases {
        if got := shortPath(c.in); got != c.want {
            t.Errorf("shortPath(%q): want %q, got %q", c.in, c.want, got)
        }
    }
}
```

- [ ] **Step 3: Run**

```bash
go test ./internal/views -run 'TestHitLabel|TestShortPath' -v
```

Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add internal/views/search_test.go
git commit -m "test: cover hitLabel and shortPath"
```

### Task 2.8: `matchesWithin` and `highlightMatches`

- [ ] **Step 1: Add `TestMatchesWithin`**

```go
func TestMatchesWithin(t *testing.T) {
    s := &SearchView{}
    h := SearchHit{
        Context: "the quick brown fox",
        Matches: []SearchSpan{{Start: 4, End: 9}, {Start: 10, End: 15}, {Start: 16, End: 19}},
    }
    cases := []struct {
        name  string
        bytes int
        want  []SearchSpan
    }{
        {"all fit", 100, []SearchSpan{{4, 9}, {10, 15}, {16, 19}}},
        {"truncates last", 17, []SearchSpan{{4, 9}, {10, 15}, {16, 17}}},
        {"drops out-of-range", 9, []SearchSpan{{4, 9}}},
        {"zero budget yields none", 0, nil},
    }
    for _, c := range cases {
        t.Run(c.name, func(t *testing.T) {
            got := s.matchesWithin(h, c.bytes)
            if len(got) != len(c.want) {
                t.Fatalf("len: want %d, got %d (%+v)", len(c.want), len(got), got)
            }
            for i, m := range got {
                if m != c.want[i] {
                    t.Errorf("[%d]: want %v, got %v", i, c.want[i], m)
                }
            }
        })
    }
}
```

Note: `matchesWithin` returns nil when input is empty, but for the "zero budget" branch it walks the loop and only includes spans where `m.Start < ctxBytes`. With `ctxBytes=0`, the first span has `Start=4 >= 0`, so it breaks. Result: empty slice. Adjust the expectation if your reading of the source disagrees — re-read `search.go` lines 156–175.

- [ ] **Step 2: Add `TestMatchesWithinEmpty`**

```go
func TestMatchesWithinEmpty(t *testing.T) {
    s := &SearchView{}
    if got := s.matchesWithin(SearchHit{Context: "x"}, 10); got != nil {
        t.Errorf("empty Matches: want nil, got %+v", got)
    }
}
```

- [ ] **Step 3: Add `TestHighlightMatches`**

```go
func TestHighlightMatches(t *testing.T) {
    // Force a no-color profile so the bold style emits no SGR bytes and the
    // result is exactly the input when there's nothing to style.
    t.Setenv("TERM", "dumb")
    t.Setenv("NO_COLOR", "1")

    // No spans: returns input verbatim.
    if got := highlightMatches("hello world", nil); got != "hello world" {
        t.Errorf("no spans: want verbatim, got %q", got)
    }
    // Out-of-range span: skipped.
    if got := highlightMatches("hi", []SearchSpan{{Start: 10, End: 12}}); got != "hi" {
        t.Errorf("out-of-range span: want \"hi\", got %q", got)
    }
    // Degenerate (Start>=End) span: skipped.
    if got := highlightMatches("hi", []SearchSpan{{Start: 1, End: 1}}); got != "hi" {
        t.Errorf("degenerate span: want \"hi\", got %q", got)
    }
    // Backward span: skipped (m.Start < last after first match).
    in := "the quick"
    got := highlightMatches(in, []SearchSpan{{Start: 4, End: 9}, {Start: 0, End: 3}})
    if !strings.Contains(got, "quick") || !strings.HasPrefix(got, "the ") {
        t.Errorf("expected output containing styled \"quick\" with \"the \" prefix; got %q", got)
    }
}
```

- [ ] **Step 4: Run**

```bash
go test ./internal/views -run 'TestMatchesWithin|TestHighlightMatches' -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/views/search_test.go
git commit -m "test: cover matchesWithin and highlightMatches"
```

### Task 2.9: `scrollWindow`, `innerWidth`, `visibleRows` clamps

- [ ] **Step 1: Add `TestSearchScrollWindow`**

```go
func TestSearchScrollWindow(t *testing.T) {
    t.Setenv("TERM", "dumb")
    t.Setenv("NO_COLOR", "1")
    s := NewSearchView(loadFixture(t), 80, 24) // visibleRows ~ 14 at h=24
    s.hits = make([]SearchHit, 20)
    for i := range s.hits {
        s.hits[i] = SearchHit{FilePath: "/p/X.md", Line: i + 1, Context: "y"}
    }

    cases := []struct {
        name      string
        sel       int
        wantStart int // -1 means "compute end-rows"
    }{
        {"top", 0, 0},
        {"middle", 10, 10 - s.visibleRows()/2},
        {"bottom", 19, len(s.hits) - s.visibleRows()},
    }
    for _, c := range cases {
        t.Run(c.name, func(t *testing.T) {
            s.sel = c.sel
            start, end := s.scrollWindow()
            if end-start != s.visibleRows() {
                t.Errorf("window size: want %d, got %d (start=%d end=%d)",
                    s.visibleRows(), end-start, start, end)
            }
            if start != c.wantStart {
                t.Errorf("start: want %d, got %d", c.wantStart, start)
            }
            if c.sel < start || c.sel >= end {
                t.Errorf("sel %d should be in [%d,%d)", c.sel, start, end)
            }
        })
    }
}
```

- [ ] **Step 2: Add `TestSearchScrollWindowAllFit`**

```go
func TestSearchScrollWindowAllFit(t *testing.T) {
    t.Setenv("TERM", "dumb")
    t.Setenv("NO_COLOR", "1")
    s := NewSearchView(loadFixture(t), 80, 24)
    s.hits = []SearchHit{{Line: 1}, {Line: 2}, {Line: 3}}
    start, end := s.scrollWindow()
    if start != 0 || end != 3 {
        t.Errorf("all-fit: want [0,3), got [%d,%d)", start, end)
    }
}
```

- [ ] **Step 3: Add `TestSearchInnerWidthClamps`**

```go
func TestSearchInnerWidthClamps(t *testing.T) {
    s := &SearchView{width: 10}
    if got := s.innerWidth(); got != searchInnerWidthMin {
        t.Errorf("narrow term: want min %d, got %d", searchInnerWidthMin, got)
    }
    s.width = 200
    if got := s.innerWidth(); got != searchInnerWidthMax {
        t.Errorf("wide term: want max %d, got %d", searchInnerWidthMax, got)
    }
}
```

- [ ] **Step 4: Add `TestSearchVisibleRowsClamps`**

```go
func TestSearchVisibleRowsClamps(t *testing.T) {
    s := &SearchView{height: 5}
    if got := s.visibleRows(); got != searchVisibleRowsMin {
        t.Errorf("tiny term: want min %d, got %d", searchVisibleRowsMin, got)
    }
    s.height = 100
    if got := s.visibleRows(); got != searchVisibleRowsMax {
        t.Errorf("huge term: want max %d, got %d", searchVisibleRowsMax, got)
    }
}
```

- [ ] **Step 5: Run**

```bash
go test ./internal/views -run 'TestSearchScrollWindow|TestSearchInnerWidth|TestSearchVisibleRows' -v
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/views/search_test.go
git commit -m "test: cover SearchView scroll-window and width clamps"
```

### Task 2.10: `View` golden snapshots

- [ ] **Step 1: Add `TestSearchViewEmptyState`**

```go
func TestSearchViewEmptyState(t *testing.T) {
    t.Setenv("TERM", "dumb")
    t.Setenv("NO_COLOR", "1")
    s := NewSearchView(loadFixture(t), 80, 24)
    teatest.RequireEqualOutput(t, []byte(s.View()))
}
```

- [ ] **Step 2: Add `TestSearchViewWithHits`**

```go
func TestSearchViewWithHits(t *testing.T) {
    t.Setenv("TERM", "dumb")
    t.Setenv("NO_COLOR", "1")
    s := NewSearchView(loadFixture(t), 80, 24)
    s.SetQuery("Beta")
    s.hits = []SearchHit{
        {FilePath: "/abs/pages/Alpha.md", Line: 3, Context: "links to Beta",
            Matches: []SearchSpan{{Start: 9, End: 13}}},
        {FilePath: "/abs/pages/Hub.md", Line: 2, Context: "the hub mentions Beta in passing",
            Matches: []SearchSpan{{Start: 17, End: 21}}},
    }
    teatest.RequireEqualOutput(t, []byte(s.View()))
}
```

Add the import if missing:

```go
import "github.com/charmbracelet/x/exp/teatest"
```

- [ ] **Step 3: Generate goldens**

```bash
go test ./internal/views -run 'TestSearchView(EmptyState|WithHits)' -update
```

- [ ] **Step 4: Visually diff goldens**

```bash
git diff internal/views/testdata/TestSearchView*.golden
```

Expected: two new golden files, both showing a rounded-border search panel with title/prompt/hint rows. Hits golden shows two rows with `Alpha:3` and `Hub:2` labels.

- [ ] **Step 5: Run**

```bash
go test ./internal/views -run TestSearchView -v
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/views/search_test.go internal/views/testdata/TestSearchView*.golden
git commit -m "test: golden snapshots for SearchView rendering"
```

### Task 2.11: Verify `search.go` coverage target

- [ ] **Step 1: Generate per-function coverage for search.go**

```bash
go test -coverprofile=/tmp/peekseq.cov ./internal/views
go tool cover -func=/tmp/peekseq.cov | grep search.go
```

Expected: every function ≥80% except possibly `View` (60–80% acceptable — it has rendering branches conditional on terminal width that are hard to drive at fixed sizes).

If any non-View function is below 80%, identify the missing branch from `go tool cover -html=/tmp/peekseq.cov -o /tmp/peekseq.html` and add a targeted test before moving on.

- [ ] **Step 2: Commit if anything was added** (otherwise unit is complete)

```bash
git add internal/views/search_test.go
git commit -m "test: close remaining search.go coverage gaps"
```

---

# Unit 3: `backlinks.go` + `app.go` tests

`backlinks.go` is 0% across the board. `app.go` has untested `Update` overlay branches, plus `View`, `centerOverlay`, and `pageNameFromHitPath`. All tests live in `internal/views/backlinks_test.go` (new file) and additions to `internal/views/app_test.go`.

### Task 3.1: Backlinks constructor and `SetSize`

**Files:**
- Create: `internal/views/backlinks_test.go`

- [ ] **Step 1: Write the file scaffold**

```go
package views

import (
    "strings"
    "testing"

    "github.com/charmbracelet/x/exp/teatest"
)

func TestNewBacklinksLoadsRefs(t *testing.T) {
    t.Setenv("TERM", "dumb")
    t.Setenv("NO_COLOR", "1")
    idx := loadFixture(t)

    // Hub is referenced by Alpha, Beta, kb/notes, and journal 2026_05_25.
    b := NewBacklinks(idx, "Hub", 80)
    if got := len(b.refs); got < 3 {
        t.Errorf("Hub backlinks: want >=3, got %d", got)
    }
    if b.target != "Hub" {
        t.Errorf("target: want \"Hub\", got %q", b.target)
    }
    if b.width != 80 {
        t.Errorf("width: want 80, got %d", b.width)
    }
}

func TestBacklinksSetSize(t *testing.T) {
    t.Setenv("TERM", "dumb")
    t.Setenv("NO_COLOR", "1")
    b := NewBacklinks(loadFixture(t), "Hub", 80)
    b.SetSize(120, 99)
    if b.width != 120 {
        t.Errorf("after SetSize: want width 120, got %d", b.width)
    }
}
```

- [ ] **Step 2: Run**

```bash
go test ./internal/views -run 'TestNewBacklinks|TestBacklinksSetSize' -v
```

Expected: PASS. If Hub doesn't have ≥3 backlinks, double-check Task 1.1/1.2 wrote the linking content correctly.

- [ ] **Step 3: Commit**

```bash
git add internal/views/backlinks_test.go
git commit -m "test: cover Backlinks constructor"
```

### Task 3.2: `Backlinks.Update` — navigation and dispatch

- [ ] **Step 1: Add `TestBacklinksNavigation`**

```go
func TestBacklinksNavigation(t *testing.T) {
    t.Setenv("TERM", "dumb")
    t.Setenv("NO_COLOR", "1")
    b := NewBacklinks(loadFixture(t), "Hub", 80)
    if len(b.refs) < 2 {
        t.Fatalf("need >=2 refs for this test, got %d", len(b.refs))
    }

    // up at top: no-op.
    b.Update("up")
    if b.sel != 0 {
        t.Errorf("up at top: want sel 0, got %d", b.sel)
    }
    // down advances.
    b.Update("down")
    if b.sel != 1 {
        t.Errorf("after down: want sel 1, got %d", b.sel)
    }
    // 'j' alias.
    b.Update("j")
    if want := 2; b.sel != want && len(b.refs) > 2 {
        t.Errorf("after j: want sel %d, got %d", want, b.sel)
    }
    // 'k' alias decrements.
    prev := b.sel
    b.Update("k")
    if b.sel >= prev {
        t.Errorf("after k: sel should decrement from %d, got %d", prev, b.sel)
    }
}
```

- [ ] **Step 2: Add `TestBacklinksEnterReturnsFromPage`**

```go
func TestBacklinksEnterReturnsFromPage(t *testing.T) {
    t.Setenv("TERM", "dumb")
    t.Setenv("NO_COLOR", "1")
    b := NewBacklinks(loadFixture(t), "Hub", 80)
    if len(b.refs) == 0 {
        t.Skip("no Hub backlinks in fixture — skipping enter test")
    }
    b.sel = 0
    sel, accept, cancel := b.Update("enter")
    if !accept || cancel {
        t.Errorf("enter: want accept=true cancel=false, got %v/%v", accept, cancel)
    }
    if sel != b.refs[0].FromPage {
        t.Errorf("returned page: want %q, got %q", b.refs[0].FromPage, sel)
    }
}
```

- [ ] **Step 3: Add `TestBacklinksEscAndBCancel`**

```go
func TestBacklinksEscAndBCancel(t *testing.T) {
    t.Setenv("TERM", "dumb")
    t.Setenv("NO_COLOR", "1")
    b := NewBacklinks(loadFixture(t), "Hub", 80)
    for _, k := range []string{"esc", "b"} {
        sel, accept, cancel := b.Update(k)
        if sel != "" || accept || !cancel {
            t.Errorf("%s: want cancel only, got (%q,%v,%v)", k, sel, accept, cancel)
        }
    }
}
```

- [ ] **Step 4: Add `TestBacklinksNoRefsEnterNoop`**

```go
func TestBacklinksNoRefsEnterNoop(t *testing.T) {
    t.Setenv("TERM", "dumb")
    t.Setenv("NO_COLOR", "1")
    b := NewBacklinks(loadFixture(t), "Orphan", 80) // Orphan has zero refs
    if len(b.refs) != 0 {
        t.Fatalf("Orphan should have 0 backlinks, got %d", len(b.refs))
    }
    sel, accept, cancel := b.Update("enter")
    if sel != "" || accept || cancel {
        t.Errorf("enter on empty refs: want zero-valued return, got (%q,%v,%v)",
            sel, accept, cancel)
    }
}
```

- [ ] **Step 5: Run**

```bash
go test ./internal/views -run TestBacklinks -v
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/views/backlinks_test.go
git commit -m "test: cover Backlinks navigation and dispatch"
```

### Task 3.3: `Backlinks.View` and `innerWidth`

- [ ] **Step 1: Add `TestBacklinksInnerWidthClamps`**

```go
func TestBacklinksInnerWidthClamps(t *testing.T) {
    b := &Backlinks{width: 10}
    if got := b.innerWidth(); got != blInnerWidthMin {
        t.Errorf("narrow term: want %d, got %d", blInnerWidthMin, got)
    }
    b.width = 300
    if got := b.innerWidth(); got != blInnerWidthMax {
        t.Errorf("wide term: want %d, got %d", blInnerWidthMax, got)
    }
}
```

- [ ] **Step 2: Add `TestBacklinksViewWithRefsGolden`**

```go
func TestBacklinksViewWithRefsGolden(t *testing.T) {
    t.Setenv("TERM", "dumb")
    t.Setenv("NO_COLOR", "1")
    b := NewBacklinks(loadFixture(t), "Hub", 100)
    teatest.RequireEqualOutput(t, []byte(b.View()))
}
```

- [ ] **Step 3: Add `TestBacklinksViewNoRefsGolden`**

```go
func TestBacklinksViewNoRefsGolden(t *testing.T) {
    t.Setenv("TERM", "dumb")
    t.Setenv("NO_COLOR", "1")
    b := NewBacklinks(loadFixture(t), "Orphan", 100)
    out := b.View()
    if !strings.Contains(out, "no backlinks") {
        t.Errorf("view should announce empty refs; got:\n%s", out)
    }
    teatest.RequireEqualOutput(t, []byte(out))
}
```

- [ ] **Step 4: Regenerate goldens**

```bash
go test ./internal/views -run 'TestBacklinksView(WithRefs|NoRefs)Golden' -update
git diff internal/views/testdata/TestBacklinksView*.golden
```

Expected: two new goldens. With-refs golden shows the rounded-border panel with at least 3 `FromPage:line · context` rows. No-refs golden shows the panel with the "no backlinks" hint.

- [ ] **Step 5: Run**

```bash
go test ./internal/views -run 'TestBacklinks' -v
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/views/backlinks_test.go internal/views/testdata/TestBacklinksView*.golden
git commit -m "test: golden snapshots for Backlinks rendering"
```

### Task 3.4: `app.go` — overlay enter/cancel flows

The existing `app_test.go` covers history. Add tests that drive each overlay through `Update` from the page mode to confirm the mode-transition wiring.

- [ ] **Step 1: Add `TestAppOpensAndCancelsPicker`**

```go
func TestAppOpensAndCancelsPicker(t *testing.T) {
    a := bootApp(t)
    if a.mode != modePage {
        t.Fatalf("setup: want modePage, got %d", a.mode)
    }
    a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("p"), Alt: false, Paste: false}) // sanity: 'p' is a no-op in page mode
    if a.mode != modePage {
        t.Errorf("plain 'p' should not change mode")
    }
    // ctrl+p opens picker.
    a.Update(tea.KeyMsg{Type: tea.KeyCtrlP})
    if a.mode != modePicker || a.picker == nil {
        t.Fatalf("after ctrl+p: want modePicker with picker set, got mode=%d picker=%v", a.mode, a.picker)
    }
    // esc cancels back to page.
    a.Update(tea.KeyMsg{Type: tea.KeyEsc})
    if a.mode != modePage || a.picker != nil {
        t.Errorf("after esc: want modePage with picker cleared, got mode=%d picker=%v", a.mode, a.picker)
    }
}
```

- [ ] **Step 2: Add `TestAppOpensSearchAndEscapes`**

```go
func TestAppOpensSearchAndEscapes(t *testing.T) {
    a := bootApp(t)
    a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
    if a.mode != modeSearch || a.search == nil {
        t.Fatalf("after /: want modeSearch with search set, got mode=%d search=%v", a.mode, a.search)
    }
    a.Update(tea.KeyMsg{Type: tea.KeyEsc})
    if a.mode != modePage || a.search != nil {
        t.Errorf("after esc: want modePage with search cleared, got mode=%d search=%v", a.mode, a.search)
    }
}
```

- [ ] **Step 3: Add `TestAppOpensBacklinksAndCloses`**

```go
func TestAppOpensBacklinksAndCloses(t *testing.T) {
    a := bootApp(t)
    a.navigate("Hub") // page with known backlinks
    a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("b")})
    if a.mode != modeBacklinks || a.backlinks == nil {
        t.Fatalf("after b: want modeBacklinks, got mode=%d backlinks=%v", a.mode, a.backlinks)
    }
    a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("b")}) // toggle off
    if a.mode != modePage || a.backlinks != nil {
        t.Errorf("after second b: want modePage cleared, got mode=%d backlinks=%v", a.mode, a.backlinks)
    }
}
```

- [ ] **Step 4: Add `TestAppOpensTodosAndCloses`**

```go
func TestAppOpensTodosAndCloses(t *testing.T) {
    a := bootApp(t)
    a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("T")})
    if a.mode != modeTodos || a.todos == nil {
        t.Fatalf("after T: want modeTodos, got mode=%d todos=%v", a.mode, a.todos)
    }
    a.Update(tea.KeyMsg{Type: tea.KeyEsc})
    if a.mode != modePage || a.todos != nil {
        t.Errorf("after esc: want modePage cleared, got mode=%d todos=%v", a.mode, a.todos)
    }
}
```

- [ ] **Step 5: Add `TestAppOpensHelpAndCloses`**

```go
func TestAppOpensHelpAndCloses(t *testing.T) {
    a := bootApp(t)
    a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("?")})
    if a.mode != modeHelp || a.help == nil {
        t.Fatalf("after ?: want modeHelp, got mode=%d help=%v", a.mode, a.help)
    }
    a.Update(tea.KeyMsg{Type: tea.KeyEsc})
    if a.mode != modePage || a.help != nil {
        t.Errorf("after esc: want modePage cleared, got mode=%d help=%v", a.mode, a.help)
    }
}
```

- [ ] **Step 6: Add `TestAppPickerAcceptNavigates`**

```go
func TestAppPickerAcceptNavigates(t *testing.T) {
    a := bootApp(t)
    a.Update(tea.KeyMsg{Type: tea.KeyCtrlP})
    if a.mode != modePicker {
        t.Fatalf("setup: picker not open")
    }
    // Type "Alp" so the top match is Alpha.
    for _, r := range "Alp" {
        a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
    }
    a.Update(tea.KeyMsg{Type: tea.KeyEnter})
    if a.mode != modePage {
        t.Errorf("after enter: want modePage, got %d", a.mode)
    }
    if a.page.Page() != "Alpha" {
        t.Errorf("after picker accept: want page Alpha, got %q", a.page.Page())
    }
}
```

- [ ] **Step 7: Add `TestAppTodosAcceptNavigates`**

```go
func TestAppTodosAcceptNavigates(t *testing.T) {
    a := bootApp(t)
    a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("T")})
    if a.mode != modeTodos {
        t.Fatalf("setup: todos not open")
    }
    a.Update(tea.KeyMsg{Type: tea.KeyEnter}) // accept the selected (first) bullet
    if a.mode != modePage {
        t.Errorf("after enter: want modePage, got %d", a.mode)
    }
    if a.todos != nil {
        t.Errorf("todos not cleared")
    }
}
```

- [ ] **Step 8: Run**

```bash
go test ./internal/views -run 'TestAppOpens|TestAppPicker|TestAppTodos' -v
```

Expected: PASS.

- [ ] **Step 9: Commit**

```bash
git add internal/views/app_test.go
git commit -m "test: cover overlay open/close flows in App.Update"
```

### Task 3.5: `app.go` — loading splash, error splash, retry

- [ ] **Step 1: Add `TestAppLoadingSplashBeforePage`**

```go
func TestAppLoadingSplashBeforePage(t *testing.T) {
    t.Setenv("TERM", "dumb")
    t.Setenv("NO_COLOR", "1")
    a := New("/nonexistent/before/build", "test")
    v := a.View()
    if !strings.Contains(v, "peekseq") {
        t.Errorf("splash should contain title; got:\n%s", v)
    }
    if !strings.Contains(v, "Loading") {
        t.Errorf("splash should announce loading; got:\n%s", v)
    }
}
```

- [ ] **Step 2: Add `TestAppErrorSplashOnIndexFailure`**

```go
func TestAppErrorSplashOnIndexFailure(t *testing.T) {
    t.Setenv("TERM", "dumb")
    t.Setenv("NO_COLOR", "1")
    a := New("/this/path/does/not/exist", "test")
    cmd := a.Init()
    msg := cmd()
    a.Update(msg)
    v := a.View()
    if !strings.Contains(v, "failed to index") {
        t.Errorf("error splash should announce failure; got:\n%s", v)
    }
    if !strings.Contains(v, "R to retry") {
        t.Errorf("error splash should show retry hint; got:\n%s", v)
    }
}
```

- [ ] **Step 3: Add `TestAppRetryFromErrorSplash`**

```go
func TestAppRetryFromErrorSplash(t *testing.T) {
    t.Setenv("TERM", "dumb")
    t.Setenv("NO_COLOR", "1")
    a := New("/this/path/does/not/exist", "test")
    cmd := a.Init()
    a.Update(cmd())
    if a.loadErr == nil {
        t.Fatalf("setup: loadErr should be set")
    }

    _, cmd2 := a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("R")})
    if cmd2 == nil {
        t.Fatalf("R from error splash should return a retry cmd")
    }
    if a.loadErr != nil {
        t.Errorf("R should clear loadErr before rebuilding, got %v", a.loadErr)
    }
}
```

- [ ] **Step 4: Add `TestAppQuitsBeforePage`**

```go
func TestAppQuitsBeforePage(t *testing.T) {
    t.Setenv("TERM", "dumb")
    t.Setenv("NO_COLOR", "1")
    a := New("/no/such/path", "test")
    _, cmd := a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
    if cmd == nil {
        t.Errorf("q before page should return Quit cmd, got nil")
    }
}
```

- [ ] **Step 5: Run**

```bash
go test ./internal/views -run 'TestAppLoading|TestAppError|TestAppRetry|TestAppQuits' -v
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/views/app_test.go
git commit -m "test: cover loading splash, error splash, and retry"
```

### Task 3.6: `centerOverlay`, `pageNameFromHitPath`, async messages

- [ ] **Step 1: Add `TestAppCenterOverlayFallback`**

```go
func TestAppCenterOverlayFallback(t *testing.T) {
    t.Setenv("TERM", "dumb")
    t.Setenv("NO_COLOR", "1")
    a := New("/no/such/path", "test")
    // width and height are zero before a WindowSizeMsg.
    if got := a.centerOverlay("hello"); got != "hello" {
        t.Errorf("zero-size fallback: want raw content, got %q", got)
    }
}
```

- [ ] **Step 2: Add `TestAppCenterOverlayPlacesContent`**

```go
func TestAppCenterOverlayPlacesContent(t *testing.T) {
    t.Setenv("TERM", "dumb")
    t.Setenv("NO_COLOR", "1")
    a := bootApp(t)
    out := a.centerOverlay("X")
    if !strings.Contains(out, "X") {
        t.Errorf("centered output must contain content; got:\n%s", out)
    }
    if len(out) <= len("X") {
        t.Errorf("centered output should pad with whitespace; got len %d", len(out))
    }
}
```

- [ ] **Step 3: Add `TestPageNameFromHitPath`**

```go
func TestPageNameFromHitPath(t *testing.T) {
    idx := loadFixture(t)
    // Pick a known indexed page.
    var sample, name string
    for _, p := range idx.Pages {
        sample, name = p.Path, p.Name
        break
    }
    if got := pageNameFromHitPath(idx, sample); got != name {
        t.Errorf("known path: want %q, got %q", name, got)
    }
    if got := pageNameFromHitPath(idx, "/totally/unknown/file.md"); got != "" {
        t.Errorf("unknown path: want \"\", got %q", got)
    }
}
```

- [ ] **Step 4: Add `TestAppSearchDoneMsgRouting`**

```go
func TestAppSearchDoneMsgRouting(t *testing.T) {
    a := bootApp(t)
    a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
    if a.search == nil {
        t.Fatalf("setup: search not open")
    }
    a.Update(searchDoneMsg{hits: []SearchHit{
        {FilePath: "/x", Line: 1, Context: "hello"},
    }})
    if len(a.search.hits) != 1 {
        t.Errorf("searchDoneMsg should populate hits, got %d", len(a.search.hits))
    }
}
```

- [ ] **Step 5: Add `TestAppViewWithOverlay`**

```go
func TestAppViewWithOverlay(t *testing.T) {
    t.Setenv("TERM", "dumb")
    t.Setenv("NO_COLOR", "1")
    a := bootApp(t)
    a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("T")})
    v := a.View()
    // The status bar belongs to page mode; overlays replace the page entirely.
    if strings.Contains(v, "? help") {
        t.Errorf("overlay view should not render the page status bar; got:\n%s", v)
    }
    if !strings.Contains(v, "Open todos") {
        t.Errorf("overlay view should show todos title; got:\n%s", v)
    }
}
```

- [ ] **Step 6: Run**

```bash
go test ./internal/views -run 'TestAppCenter|TestPageNameFromHitPath|TestAppSearchDoneMsg|TestAppViewWithOverlay' -v
```

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/views/app_test.go
git commit -m "test: cover centerOverlay, pageNameFromHitPath, View overlay path"
```

### Task 3.7: Verify Unit 3 coverage targets

- [ ] **Step 1: Coverage for backlinks.go and app.go**

```bash
go test -coverprofile=/tmp/peekseq.cov ./internal/views
go tool cover -func=/tmp/peekseq.cov | grep -E 'backlinks\.go|app\.go'
```

Expected: every `backlinks.go` function ≥80%. For `app.go`: `Update` ≥80%, `View` ≥80%, `centerOverlay` 100%, `pageNameFromHitPath` 100%. If `Update` is still under 80%, identify the missing case in the `switch a.mode { ... switch key {` blocks and add a focused test.

- [ ] **Step 2: Commit if anything was added**

```bash
git add internal/views/app_test.go internal/views/backlinks_test.go
git commit -m "test: close remaining backlinks/app coverage gaps"
```

---

# Unit 4: Lukewarm pass — picker, todos, page state machines and orphans

### Task 4.1: `picker.go` — `consumeKey` direct tests

`consumeKey` is a tiny adapter for `textinput.Model`. Test it directly to lock in its forms.

**Files:**
- Modify: `internal/views/picker_test.go`

- [ ] **Step 1: Add the new tests**

```go
import "github.com/charmbracelet/bubbles/textinput"

func TestConsumeKeyAppendsRune(t *testing.T) {
    ti := textinput.New()
    ti, ok := consumeKey(ti, "x")
    if !ok || ti.Value() != "x" {
        t.Errorf("rune key: want \"x\"/true, got %q/%v", ti.Value(), ok)
    }
}

func TestConsumeKeyBackspace(t *testing.T) {
    ti := textinput.New()
    ti.SetValue("abc")
    ti, ok := consumeKey(ti, "backspace")
    if !ok || ti.Value() != "ab" {
        t.Errorf("backspace: want \"ab\"/true, got %q/%v", ti.Value(), ok)
    }
    ti.SetValue("")
    ti, ok = consumeKey(ti, "backspace")
    if !ok || ti.Value() != "" {
        t.Errorf("backspace on empty: want \"\"/true, got %q/%v", ti.Value(), ok)
    }
}

func TestConsumeKeySpaceVariants(t *testing.T) {
    ti := textinput.New()
    ti.SetValue("a")
    ti, ok := consumeKey(ti, " ")
    if !ok || ti.Value() != "a " {
        t.Errorf("literal space: want \"a \"/true, got %q/%v", ti.Value(), ok)
    }
    ti, ok = consumeKey(ti, "space")
    if !ok || ti.Value() != "a  " {
        t.Errorf("named space: want \"a  \"/true, got %q/%v", ti.Value(), ok)
    }
}

func TestConsumeKeyMultiCharNoop(t *testing.T) {
    ti := textinput.New()
    ti.SetValue("a")
    _, ok := consumeKey(ti, "ctrl+x")
    if ok {
        t.Errorf("multi-char key should not be consumed")
    }
}
```

- [ ] **Step 2: Run**

```bash
go test ./internal/views -run TestConsumeKey -v
```

Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add internal/views/picker_test.go
git commit -m "test: cover consumeKey adapter"
```

### Task 4.2: `picker.go` — `Update` selection + accept/cancel

- [ ] **Step 1: Add the new tests**

```go
func TestPickerUpDownBounds(t *testing.T) {
    t.Setenv("TERM", "dumb")
    t.Setenv("NO_COLOR", "1")
    p := NewPicker(loadFixture(t), 80, 30)
    // No query → matches is all choices capped at 50 with sel=0.
    if len(p.matches) < 2 {
        t.Fatalf("setup: need >=2 matches, got %d", len(p.matches))
    }

    p.Update("up") // no-op at top
    if p.sel != 0 {
        t.Errorf("up at top: want sel 0, got %d", p.sel)
    }
    p.Update("down")
    if p.sel != 1 {
        t.Errorf("after down: want sel 1, got %d", p.sel)
    }
    p.Update("ctrl+j")
    if p.sel != 2 && len(p.matches) > 2 {
        t.Errorf("after ctrl+j: want sel 2, got %d", p.sel)
    }
    p.Update("ctrl+k")
    if p.sel < 0 {
        t.Errorf("after ctrl+k: sel went negative, got %d", p.sel)
    }
}

func TestPickerEnterReturnsSelected(t *testing.T) {
    t.Setenv("TERM", "dumb")
    t.Setenv("NO_COLOR", "1")
    p := NewPicker(loadFixture(t), 80, 30)
    if len(p.matches) == 0 {
        t.Fatal("setup: no matches")
    }
    want := p.matches[0].Str
    sel, accept, cancel := p.Update("enter")
    if !accept || cancel {
        t.Errorf("enter: want accept=true cancel=false, got %v/%v", accept, cancel)
    }
    if sel != want {
        t.Errorf("returned name: want %q, got %q", want, sel)
    }
}

func TestPickerEnterEmptyNoop(t *testing.T) {
    t.Setenv("TERM", "dumb")
    t.Setenv("NO_COLOR", "1")
    p := NewPicker(loadFixture(t), 80, 30)
    // Type a query that matches nothing.
    for _, r := range "zzzzzzzzznotapage" {
        p.Update(string(r))
    }
    if len(p.matches) != 0 {
        t.Fatalf("setup: query should yield no matches, got %d", len(p.matches))
    }
    sel, accept, cancel := p.Update("enter")
    if sel != "" || accept || cancel {
        t.Errorf("enter with no matches: want zeros, got (%q,%v,%v)", sel, accept, cancel)
    }
}

func TestPickerEscCancels(t *testing.T) {
    t.Setenv("TERM", "dumb")
    t.Setenv("NO_COLOR", "1")
    p := NewPicker(loadFixture(t), 80, 30)
    sel, accept, cancel := p.Update("esc")
    if sel != "" || accept || !cancel {
        t.Errorf("esc: want cancel only, got (%q,%v,%v)", sel, accept, cancel)
    }
}
```

- [ ] **Step 2: Run**

```bash
go test ./internal/views -run 'TestPicker(UpDownBounds|EnterReturns|EnterEmpty|EscCancels)' -v
```

Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add internal/views/picker_test.go
git commit -m "test: cover Picker selection and accept/cancel"
```

### Task 4.3: `picker.go` — `scrollWindow` and orphans

- [ ] **Step 1: Add the scroll-window tests**

```go
func TestPickerScrollWindowAllFit(t *testing.T) {
    p := &Picker{height: 24}
    p.matches = make([]fuzzy.Match, 5)
    start, end := p.scrollWindow()
    if start != 0 || end != 5 {
        t.Errorf("all-fit: want [0,5), got [%d,%d)", start, end)
    }
}

func TestPickerScrollWindowAtBottom(t *testing.T) {
    p := &Picker{height: 16} // visibleRows=6 after chrome
    p.matches = make([]fuzzy.Match, 20)
    p.sel = 19
    start, end := p.scrollWindow()
    if end != 20 || (end-start) != p.visibleRows() {
        t.Errorf("at bottom: want end=20 window=%d, got [%d,%d)",
            p.visibleRows(), start, end)
    }
    if p.sel < start || p.sel >= end {
        t.Errorf("sel %d should be in window [%d,%d)", p.sel, start, end)
    }
}

func TestPickerScrollWindowMiddle(t *testing.T) {
    p := &Picker{height: 24}
    p.matches = make([]fuzzy.Match, 30)
    p.sel = 15
    start, end := p.scrollWindow()
    if p.sel < start || p.sel >= end {
        t.Errorf("sel %d should be in window [%d,%d)", p.sel, start, end)
    }
}
```

Add the import if missing:

```go
import "github.com/sahilm/fuzzy"
```

- [ ] **Step 2: Add the `SetSize` and `padTo` orphan tests**

```go
func TestPickerSetSize(t *testing.T) {
    p := &Picker{width: 80, height: 24}
    p.SetSize(100, 30)
    if p.width != 100 || p.height != 30 {
        t.Errorf("SetSize: want 100x30, got %dx%d", p.width, p.height)
    }
}

func TestPadTo(t *testing.T) {
    cases := []struct{ in string; w int; want string }{
        {"abc", 5, "abc  "},
        {"abc", 3, "abc"},
        {"abc", 2, "abc"},   // negative pad → unchanged
        {"", 3, "   "},
    }
    for _, c := range cases {
        if got := padTo(c.in, c.w); got != c.want {
            t.Errorf("padTo(%q,%d): want %q, got %q", c.in, c.w, c.want, got)
        }
    }
}
```

- [ ] **Step 3: Run**

```bash
go test ./internal/views -run 'TestPickerScrollWindow|TestPickerSetSize|TestPadTo' -v
```

Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add internal/views/picker_test.go
git commit -m "test: cover Picker scroll window, SetSize, padTo"
```

### Task 4.4: `todos.go` — `Update` deepening and orphan

**Files:**
- Modify: `internal/views/todos_test.go`

- [ ] **Step 1: Add the new tests**

```go
func TestTodosUpDownBounds(t *testing.T) {
    t.Setenv("TERM", "dumb")
    t.Setenv("NO_COLOR", "1")
    td := NewTodos(loadFixture(t), 100)
    if len(td.visible) < 2 {
        t.Fatalf("setup: need >=2 visible todos, got %d", len(td.visible))
    }
    // up at top is a no-op.
    td.Update("up")
    if td.sel != 0 {
        t.Errorf("up at top: want sel 0, got %d", td.sel)
    }
    td.Update("down")
    if td.sel != 1 {
        t.Errorf("after down: want sel 1, got %d", td.sel)
    }
    // 'j' / 'k' aliases.
    prev := td.sel
    td.Update("j")
    if td.sel <= prev {
        t.Errorf("after j: sel should advance from %d, got %d", prev, td.sel)
    }
    td.Update("k")
    if td.sel != prev {
        t.Errorf("after k: sel should restore to %d, got %d", prev, td.sel)
    }
}

func TestTodosEnterReturnsPage(t *testing.T) {
    t.Setenv("TERM", "dumb")
    t.Setenv("NO_COLOR", "1")
    td := NewTodos(loadFixture(t), 100)
    if len(td.visible) == 0 {
        t.Fatal("setup: no visible todos")
    }
    want := td.visible[0].Page
    page, accept, cancel := td.Update("enter")
    if !accept || cancel {
        t.Errorf("enter: want accept=true cancel=false, got %v/%v", accept, cancel)
    }
    if page != want {
        t.Errorf("returned page: want %q, got %q", want, page)
    }
}

func TestTodosFilterCycleEndsAtAll(t *testing.T) {
    t.Setenv("TERM", "dumb")
    t.Setenv("NO_COLOR", "1")
    td := NewTodos(loadFixture(t), 100)
    if td.filter != "" {
        t.Fatalf("initial filter: want \"\", got %q", td.filter)
    }
    // Cycle through every marker and back to all.
    expected := []string{"TODO", "LATER", "DOING", "WAITING", ""}
    for i, want := range expected {
        td.Update("t")
        if td.filter != want {
            t.Errorf("step %d: want filter %q, got %q", i, want, td.filter)
        }
    }
}

func TestTodosEscAndQCancel(t *testing.T) {
    t.Setenv("TERM", "dumb")
    t.Setenv("NO_COLOR", "1")
    for _, k := range []string{"esc", "q"} {
        td := NewTodos(loadFixture(t), 100)
        page, accept, cancel := td.Update(k)
        if page != "" || accept || !cancel {
            t.Errorf("%s: want cancel only, got (%q,%v,%v)", k, page, accept, cancel)
        }
    }
}

func TestTodosSetSize(t *testing.T) {
    td := &Todos{width: 80}
    td.SetSize(120, 99)
    if td.width != 120 {
        t.Errorf("SetSize: want width 120, got %d", td.width)
    }
}

func TestTodosSelClampedOnFilter(t *testing.T) {
    t.Setenv("TERM", "dumb")
    t.Setenv("NO_COLOR", "1")
    td := NewTodos(loadFixture(t), 100)
    if len(td.visible) < 2 {
        t.Skip("not enough bullets to clamp test")
    }
    // Move selection toward end of the all-filter list, then cycle to a
    // narrower filter. recompute must clamp sel back into range.
    td.sel = len(td.visible) - 1
    td.Update("t") // → TODO (almost certainly fewer bullets)
    if td.sel < 0 || td.sel >= len(td.visible) {
        t.Errorf("after filter: sel %d out of range [0,%d)", td.sel, len(td.visible))
    }
}
```

- [ ] **Step 2: Run**

```bash
go test ./internal/views -run 'TestTodosUpDown|TestTodosEnter|TestTodosFilter|TestTodosEscAndQ|TestTodosSetSize|TestTodosSelClamped' -v
```

Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add internal/views/todos_test.go
git commit -m "test: cover Todos navigation, filter cycle, SetSize"
```

### Task 4.5: `page.go` — `FollowCursor`, `LineUp`, `LineDown`, `HalfPageUp` orphans

**Files:**
- Modify: `internal/views/page_test.go`

- [ ] **Step 1: Add `TestPageFollowCursorNoLink`**

```go
func TestPageFollowCursorNoLink(t *testing.T) {
    t.Setenv("TERM", "dumb")
    t.Setenv("NO_COLOR", "1")
    pv := NewPageView(loadFixture(t), "Alpha", 80, 24)
    if got := pv.FollowCursor(); got != "" {
        t.Errorf("no cursor set: want \"\", got %q", got)
    }
}
```

- [ ] **Step 2: Add `TestPageFollowCursorReturnsTarget`**

```go
func TestPageFollowCursorReturnsTarget(t *testing.T) {
    t.Setenv("TERM", "dumb")
    t.Setenv("NO_COLOR", "1")
    pv := NewPageView(loadFixture(t), "Alpha", 80, 24)
    pv.CycleLink(+1)
    if got := pv.FollowCursor(); got == "" {
        t.Errorf("after CycleLink: want a target, got empty")
    }
}
```

- [ ] **Step 3: Add `TestPageLineUpDownAndHalfPageUp`**

```go
func TestPageLineUpDownAndHalfPageUp(t *testing.T) {
    t.Setenv("TERM", "dumb")
    t.Setenv("NO_COLOR", "1")
    pv := NewPageView(loadFixture(t), "Alpha", 80, 5) // small viewport

    pv.LineDown()
    afterDown := pv.Offset()
    if afterDown == 0 {
        t.Errorf("after LineDown: expected non-zero offset, got 0")
    }
    pv.LineUp()
    if got := pv.Offset(); got >= afterDown {
        t.Errorf("after LineUp: offset should retreat from %d, got %d", afterDown, got)
    }

    pv.GotoBottom()
    bottom := pv.Offset()
    pv.HalfPageUp()
    if got := pv.Offset(); got >= bottom {
        t.Errorf("after HalfPageUp from bottom: offset should retreat from %d, got %d",
            bottom, got)
    }
}
```

- [ ] **Step 4: Run**

```bash
go test ./internal/views -run 'TestPageFollowCursor|TestPageLineUpDownAndHalfPageUp' -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/views/page_test.go
git commit -m "test: cover PageView FollowCursor, LineUp/Down, HalfPageUp"
```

### Task 4.6: Final coverage verification and cleanup

- [ ] **Step 1: Generate the full coverage report**

```bash
go test -coverprofile=/tmp/peekseq.cov ./...
go tool cover -func=/tmp/peekseq.cov | tail -80
```

Expected: `git.fiatcode.dev/fiatcode/peekseq/internal/views` total ≥ 70%. Cold-spot files (`search.go`, `backlinks.go`) ≥ 80% per function (excluding any view-rendering branches conditional on width). `app.go Update` ≥ 80%.

- [ ] **Step 2: Identify and triage any remaining gaps**

```bash
go tool cover -html=/tmp/peekseq.cov -o /tmp/peekseq.html
```

Open `/tmp/peekseq.html` in a browser. For any remaining red regions in the in-scope files:

- If it's the `View` function for the wide/narrow width clamp branches, document and accept (see spec Non-goals).
- If it's a `default` branch in a `switch` that's only reachable via unknown keys, add a targeted test:

```go
func TestSearchUpdateUnknownKey(t *testing.T) {
    s := NewSearchView(loadFixture(t), 80, 24)
    s.SetQuery("foo")
    // Unknown multi-char key: falls through default, no change.
    s.Update("ctrl+alt+meta+nope", "/tmp/x")
    if s.query != "foo" {
        t.Errorf("unknown key should not alter query, got %q", s.query)
    }
}
```

- [ ] **Step 3: Run the full suite one more time**

```bash
go vet ./... && go test ./...
```

Expected: PASS, all packages.

- [ ] **Step 4: Commit any final gap fills**

```bash
git add internal/views
git commit -m "test: close final views coverage gaps"
```

---

## Self-review checklist (run after writing the plan)

This was checked during plan authoring:

1. **Spec coverage:**
   - search.go cold spot → Tasks 2.1–2.11 ✓
   - backlinks.go cold spot → Tasks 3.1–3.3 ✓
   - app.go Update/View/centerOverlay/pageNameFromHitPath → Tasks 3.4–3.6 ✓
   - 0%-coverage orphans (picker.SetSize, picker.padTo, todos.SetSize, page.FollowCursor/LineUp/LineDown/HalfPageUp) → Tasks 4.3, 4.4, 4.5 ✓
   - State-machine deepening (picker.Update/consumeKey/scrollWindow, todos.Update) → Tasks 4.1, 4.2, 4.3, 4.4 ✓
   - Fixture growth → Tasks 1.1–1.3 ✓
   - rg PATH skip → Task 2.6 helper ✓

2. **Placeholder scan:** No TBDs, no "implement later", no "similar to Task N" without code. Every code block is complete.

3. **Type consistency:** `tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(...)}` used consistently. `tea.KeyCtrlP`, `tea.KeyEnter`, `tea.KeyEsc` used for named keys. `Update` returns are positional and consistent: search `(hit, accept, cancel, cmd)`, backlinks `(sel, accept, cancel)`, todos `(page, accept, cancel)`, picker `(selected, accept, cancel)` — matches source signatures.

4. **Ambiguity:** The `matchesWithin` zero-budget edge case (Task 2.8) flagged in the test code with an inline note pointing back to source lines.
