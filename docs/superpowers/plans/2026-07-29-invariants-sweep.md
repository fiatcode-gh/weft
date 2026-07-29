# Invariants Sweep Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Centralize open-task, overlay-outcome, backlink-folding, and journal-ordering invariants while preserving weft's user-visible behavior.

**Architecture:** Export one graph-owned open-task predicate and consume it from both extraction and rendering. Replace `OverlayResult`'s independent public fields with a private tagged result constructed through named helpers, and move backlink folding plus journal binary search behind `graph.Index` methods. Finish with test-helper and documentation cleanup; do not change dependencies or UI output.

**Tech Stack:** Go 1.26+, Bubble Tea, Glamour, lipgloss, ripgrep-backed search, standard `testing`, race detector, `go vet`, Staticcheck.

## Global Constraints

- Work only in `/var/home/dhemas/Development/Projects/fiatcode/weft/.claude/worktrees/invariants-sweep` on branch `refactor/invariants-sweep`, based on merged PR #38 at `origin/main`.
- Every subagent prompt must begin by requiring:

  ```bash
  cd /var/home/dhemas/Development/Projects/fiatcode/weft/.claude/worktrees/invariants-sweep
  test "$(git rev-parse --abbrev-ref HEAD)" = "refactor/invariants-sweep"
  ```

- Use `openai-codex/gpt-5.6-terra` or stronger for every implementer and per-task reviewer; use `openai-codex/gpt-5.6-sol` for the whole-branch final review. Never use the cheapest tier.
- TDD per task: add a failing API/characterization test first, run it and preserve the exact RED command/output in the implementer's report, then write the minimal implementation.
- Structure new test bodies as `// arrange`, `// act`, `// assert`; extract repeated setup rather than duplicating it.
- Before every commit, run the full gate:

  ```bash
  test -z "$(gofmt -l .)"
  go vet ./...
  "$(go env GOPATH)/bin/staticcheck" ./...
  go test -race ./...
  ```

- Stage files first, then **immediately before every commit** run and visibly report:

  ```bash
  branch=$(git rev-parse --abbrev-ref HEAD)
  printf 'branch=%s\n' "$branch"
  test "$branch" = "refactor/invariants-sweep"
  ```

  The task-specific `git commit` command shown below must be the next command.

- Never use bare `git stash`, `git stash pop`, or any stash-stack mutation.
- For each implementation task: fresh implementer → fresh spec reviewer → fresh quality reviewer. If either reviewer finds issues, send only those findings back to the same implementer, then run a scoped re-review for the finding set.
- Teatest goldens must remain byte-identical. Do not run `go test ./... -update` unless an intentional frame change is first identified and its golden diff is visually reviewed; no frame change is expected.
- Do not upgrade Glamour or refresh any dependency.

## File Structure

### Task 1 — open-task contract

- Modify `internal/graph/parse.go`: export `IsOpenTask` and make extraction use it.
- Modify `internal/graph/parse_test.go`: predicate grammar table.
- Modify `internal/render/page.go`: remove `openTaskMarkers`; ask graph whether the original line is open.
- Modify `internal/render/page_test.go`: graph/render agreement and emphasis left-boundary coverage.

### Task 2 — overlay variants and adjacent coverage

- Modify `internal/views/overlay.go`: tagged result kind, private payload, constructors.
- Modify `internal/views/app.go`: switch on result kind.
- Modify `internal/views/{picker,search,todos,backlinks,help}.go`: return constructors.
- Modify `internal/views/{picker,search,todos,backlinks,help}_test.go`: assert result kinds/payloads rather than public booleans.
- Modify `internal/views/app_sync_test.go`: construct canned create outcome through `overlayCreate`.
- Add or modify `internal/views/overlay_test.go`: constructor characterization.
- Modify `internal/search/search_test.go`: malformed-regex error coverage.

### Task 3 — Index-owned lookup/order invariants

- Modify `internal/graph/index.go`: private backlink/journal storage and `BacklinksTo`/`JournalNeighbor` methods.
- Modify `internal/graph/index_test.go`: method-level case folding and journal-neighbor tables.
- Modify `internal/views/backlinks.go`: use `BacklinksTo` and remove caller-side folding.
- Modify `internal/views/backlinks_test.go`: use method lookup; keep scroll tests focused on view-owned rows.
- Modify `internal/views/app.go`: call `Index.JournalNeighbor`; remove `App.journalNeighbor` and `sort` import.

### Task 4 — boot helper and documentation cleanup

- Modify `internal/views/app_test.go`: one configurable `bootApp` sequence.
- Modify `internal/views/app_canonical_test.go`: remove duplicate helper and migrate calls.
- Modify `internal/views/app_dispatch_test.go`: replace `bootAppAt` calls.
- Modify `internal/views/app_history_test.go`: replace `bootAppAt` calls.
- Modify `internal/render/page.go`: document same-line query/embed closure.
- Modify `internal/graph/types.go`: correct `PageMeta.Path` documentation without changing behavior.

---

### Task 1: Share the Open-Task Predicate

**Interfaces:**

- Consumes: existing `todoRe`, `parseBody`, `render.preprocessTaskMarkers`, and `graph.ExtractTodos` behavior.
- Produces: `func graph.IsOpenTask(line string) bool`; render no longer owns an open-marker set or grammar.

- [ ] **Step 1: Add the failing graph predicate table**

Add to `internal/graph/parse_test.go`:

```go
func TestIsOpenTask(t *testing.T) {
 // arrange
 tests := []struct {
  name string
  line string
  want bool
 }{
  {name: "todo", line: "- TODO buy milk", want: true},
  {name: "indented priority", line: "  - WAITING [#B] vendor", want: true},
  {name: "done", line: "- DONE shipped", want: false},
  {name: "punctuation adjacent", line: "- TODO: not a task", want: false},
  {name: "empty text", line: "- LATER   ", want: false},
  {name: "not a bullet", line: "TODO buy milk", want: false},
 }

 for _, tt := range tests {
  t.Run(tt.name, func(t *testing.T) {
   // act
   got := IsOpenTask(tt.line)

   // assert
   if got != tt.want {
    t.Errorf("IsOpenTask(%q) = %v, want %v", tt.line, got, tt.want)
   }
  })
 }
}
```

- [ ] **Step 2: Run the predicate test and record RED**

Run:

```bash
go test ./internal/graph -run '^TestIsOpenTask$' -count=1
```

Expected: compile failure `undefined: IsOpenTask`.

- [ ] **Step 3: Add render agreement and left-boundary coverage**

Add a table-driven test to `internal/render/page_test.go` that checks `preprocessTaskMarkers` against `graph.IsOpenTask` for open, done, punctuation-adjacent, empty, priority, and indented lines:

```go
func TestPreprocessTaskMarkersUsesGraphOpenTaskPredicate(t *testing.T) {
 // arrange
 lines := []string{
  "- TODO buy milk",
  "  - WAITING [#B] vendor",
  "- DONE shipped",
  "- TODO: not a task",
  "- LATER   ",
 }

 for _, line := range lines {
  t.Run(line, func(t *testing.T) {
   // act
   _, markers := preprocessTaskMarkers(line)

   // assert
   if len(markers) != 1 {
    t.Fatalf("marker count = %d, want 1", len(markers))
   }
   if got, want := markers[0].open, graph.IsOpenTask(line); got != want {
    t.Errorf("render open = %v, graph open = %v", got, want)
   }
  })
 }
}
```

Add the source-adjacent emphasis case:

```go
func TestRenderWithEmphasisRejectsMissingLeftWordBoundary(t *testing.T) {
 // arrange
 body := "xAlpha Alpha\n"

 // act
 out := mustRenderEmphasis(t, body, 80, "Alpha")

 // assert
 if len(out.Finds) != 1 {
  t.Fatalf("want only the standalone Alpha emphasized, got %d finds", len(out.Finds))
 }
}
```

The emphasis case is characterization coverage and may already pass; the task's RED gate is the missing shared API.

- [ ] **Step 4: Implement the graph-owned predicate**

In `internal/graph/parse.go`, add:

```go
// IsOpenTask reports whether line is an open TODO/LATER/DOING/WAITING bullet.
func IsOpenTask(line string) bool {
 return todoRe.MatchString(line)
}
```

In `parseBody`, guard extraction with the exported predicate while retaining the existing captures:

```go
if IsOpenTask(line) {
 m := todoRe.FindStringSubmatch(line)
 todos = append(todos, TodoHit{
  Marker:   m[1],
  Priority: m[2],
  Text:     m[3],
  Line:     i + 1,
 })
}
```

- [ ] **Step 5: Make render consume the graph predicate**

In `internal/render/page.go`:

1. Delete `openTaskMarkers` and its guard comment.
2. In `preprocessTaskMarkers`, retain marker capture/styling but replace the duplicated `rest` grammar with:

```go
open := graph.IsOpenTask(line)
```

Keep `rest` for reconstructing the sentinel line. Update the nearby comment to state that graph owns open-task classification.

- [ ] **Step 6: Run focused tests GREEN**

```bash
go test ./internal/graph -run 'Test(IsOpenTask|ExtractTodos)$' -count=1
go test ./internal/render -run 'Test(PreprocessTaskMarkersUsesGraphOpenTaskPredicate|RenderRecordsOpenTaskPositions|RenderOpenTaskExcludesPunctuationAdjacentMarker|RenderOpenTaskOrdinalAlignmentWithInterleavedAndPriority|RenderWithEmphasisRejectsMissingLeftWordBoundary)$' -count=1
```

Expected: PASS.

- [ ] **Step 7: Run the full gate and commit**

Run the Global Constraints full gate, stage the four task files, verify the feature branch immediately before commit, then commit:

```bash
git add internal/graph/parse.go internal/graph/parse_test.go internal/render/page.go internal/render/page_test.go
branch=$(git rev-parse --abbrev-ref HEAD)
printf 'branch=%s\n' "$branch"
test "$branch" = "refactor/invariants-sweep"
git commit -m "refactor: share open task predicate"
```

- [ ] **Step 8: Run spec review, quality review, and scoped re-review**

Spec reviewer checks exact grammar agreement and that Glamour/dependencies are untouched. Quality reviewer checks naming, comments, no needless regex divergence, and test clarity. Send findings to the same implementer; every fix commit repeats the full gate and immediate branch check. Re-review only the reported findings.

---

### Task 2: Make Overlay Outcomes Explicit

**Interfaces:**

- Consumes: `Overlay.Update(string) OverlayResult`, `tea.Cmd`, `graph.UnlinkedRef`, and current App dispatch behavior.
- Produces: private `overlayResultKind`, private result payload, and constructors `overlayCancel`, `overlayCommand`, `overlayOpen`, `overlayCreate`, `overlayOpenTask`, `overlayFocusLink`, `overlayHighlight`, `overlayLinkify`.

- [ ] **Step 1: Add failing constructor tests**

Create `internal/views/overlay_test.go` with a test covering every constructor. Use an actual no-op command and an unlinked ref; assert the exact kind and relevant payload while all unrelated payload remains zero:

```go
func TestOverlayResultConstructors(t *testing.T) {
 // arrange
 cmd := func() tea.Msg { return "done" }
 ref := &graph.UnlinkedRef{PageName: "Note", Line: 3}

 // act / assert
 if got := (OverlayResult{}); got.kind != overlayResultNone {
  t.Fatalf("zero result kind = %v", got.kind)
 }
 if got := overlayCancel(); got.kind != overlayResultCancel {
  t.Fatalf("overlayCancel kind = %v", got.kind)
 }
 if got := overlayCommand(cmd); got.kind != overlayResultCommand || got.cmd == nil {
  t.Fatalf("overlayCommand = %+v", got)
 }
 if got := overlayOpen("Alpha"); got.kind != overlayResultOpen || got.page != "Alpha" {
  t.Fatalf("overlayOpen = %+v", got)
 }
 if got := overlayCreate("New"); got.kind != overlayResultCreate || got.page != "New" {
  t.Fatalf("overlayCreate = %+v", got)
 }
 if got := overlayOpenTask("Alpha", 2); got.kind != overlayResultOpenTask || got.page != "Alpha" || got.taskOrdinal != 2 {
  t.Fatalf("overlayOpenTask = %+v", got)
 }
 if got := overlayFocusLink("Note", "Alpha"); got.kind != overlayResultFocusLink || got.page != "Note" || got.target != "Alpha" {
  t.Fatalf("overlayFocusLink = %+v", got)
 }
 if got := overlayHighlight("Note", "Alpha"); got.kind != overlayResultHighlight || got.page != "Note" || got.target != "Alpha" {
  t.Fatalf("overlayHighlight = %+v", got)
 }
 if got := overlayLinkify(ref, "Alpha"); got.kind != overlayResultLinkify || got.ref != ref || got.target != "Alpha" {
  t.Fatalf("overlayLinkify = %+v", got)
 }
}
```

`graph.UnlinkedRef.Line` is the existing 1-based source-line field used above.

- [ ] **Step 2: Run constructor test and record RED**

```bash
go test ./internal/views -run '^TestOverlayResultConstructors$' -count=1
```

Expected: compile failures for undefined constructors/kinds and missing private fields.

- [ ] **Step 3: Add source-adjacent picker and malformed-regex coverage**

In `internal/views/picker_test.go`, add a test that types an exact non-existing name which still has real fuzzy matches, presses `down` once per real match, and asserts selection reaches `len(p.matches)` (the create row), then Enter returns the create outcome:

```go
func TestPickerCreate_RowReachedByDownPastFuzzyMatches(t *testing.T) {
 // arrange
 quietTerm(t)
 _, idx := writeGraph(t, map[string]string{"pages/Nested Notes Archive.md": "# x\n"})
 p := NewPicker(idx, 80, 30)
 typeQuery(p, "Notes")
 if len(p.matches) == 0 || p.createName != "Notes" {
  t.Fatalf("setup: matches=%d createName=%q", len(p.matches), p.createName)
 }

 // act
 for range p.matches {
  p.Update(keyDown)
 }
 res := p.Update(keyEnter)

 // assert
 if p.sel != len(p.matches) {
  t.Fatalf("selection = %d, want create row %d", p.sel, len(p.matches))
 }
 if res.kind != overlayResultCreate || res.page != "Notes" {
  t.Fatalf("enter on create row = %+v", res)
 }
}
```

In `internal/search/search_test.go`, add:

```go
func TestRunMalformedRegexSurfacesRipgrepFailure(t *testing.T) {
 skipIfNoRipgrep(t)

 // arrange
 graph := fixtureGraph(t)

 // act
 _, err := Run(graph, "[")

 // assert
 if err == nil || !strings.Contains(err.Error(), "rg failed") {
  t.Fatalf("Run malformed regex error = %v, want rg failed", err)
 }
}
```

These are characterization tests; the constructor test supplies RED.

- [ ] **Step 4: Implement the tagged result and constructors**

Replace the fields in `internal/views/overlay.go` with:

```go
type overlayResultKind uint8

const (
 overlayResultNone overlayResultKind = iota
 overlayResultCancel
 overlayResultCommand
 overlayResultOpen
 overlayResultCreate
 overlayResultOpenTask
 overlayResultFocusLink
 overlayResultHighlight
 overlayResultLinkify
)

type OverlayResult struct {
 kind        overlayResultKind
 page        string
 taskOrdinal int
 target      string
 ref         *graph.UnlinkedRef
 cmd         tea.Cmd
}

func overlayCancel() OverlayResult { return OverlayResult{kind: overlayResultCancel} }
func overlayCommand(cmd tea.Cmd) OverlayResult {
 return OverlayResult{kind: overlayResultCommand, cmd: cmd}
}
func overlayOpen(name string) OverlayResult {
 return OverlayResult{kind: overlayResultOpen, page: name}
}
func overlayCreate(name string) OverlayResult {
 return OverlayResult{kind: overlayResultCreate, page: name}
}
func overlayOpenTask(name string, ordinal int) OverlayResult {
 return OverlayResult{kind: overlayResultOpenTask, page: name, taskOrdinal: ordinal}
}
func overlayFocusLink(name, target string) OverlayResult {
 return OverlayResult{kind: overlayResultFocusLink, page: name, target: target}
}
func overlayHighlight(name, text string) OverlayResult {
 return OverlayResult{kind: overlayResultHighlight, page: name, target: text}
}
func overlayLinkify(ref *graph.UnlinkedRef, target string) OverlayResult {
 return OverlayResult{kind: overlayResultLinkify, ref: ref, target: target}
}
```

Retain `OverlayResult{}` as the valid no-action zero value. Update the type comment to describe the tagged contract rather than field precedence.

- [ ] **Step 5: Convert overlay producers**

Use this exact mapping:

- cancel literals → `overlayCancel()`;
- search command literal → `overlayCommand(s.SearchCmd(...))`;
- normal page selection → `overlayOpen(name)`;
- picker create → `overlayCreate(name)`;
- todos selection → `overlayOpenTask(page, ordinal)`;
- linked backlink → `overlayFocusLink(fromPage, target)`;
- unlinked backlink → `overlayHighlight(pageName, target)`;
- confirmed linkify → `overlayLinkify(ref, target)`;
- no action remains `OverlayResult{}`.

Apply it in `picker.go`, `search.go`, `todos.go`, `backlinks.go`, and `help.go`.

- [ ] **Step 6: Convert App dispatch to one kind switch**

Replace `res.Cancel`/`Linkify`/`Accept` precedence in `internal/views/app.go` with:

```go
res := a.active.Update(key)
switch res.kind {
case overlayResultCancel:
 a.active = nil
case overlayResultCommand:
 return a, res.cmd
case overlayResultLinkify:
 if cmd, blocked := a.blockIfSyncing(); blocked {
  return a, cmd
 }
 return a, a.linkify(res.ref, res.target)
case overlayResultCreate:
 if cmd, blocked := a.blockIfSyncing(); blocked {
  return a, cmd
 }
 a.navigate(res.page)
 a.active = nil
 return a, a.enterEditor()
case overlayResultOpen:
 if res.page != "" {
  a.navigate(res.page)
 }
 a.active = nil
case overlayResultOpenTask:
 a.navigateToTask(res.page, res.taskOrdinal)
 a.active = nil
case overlayResultFocusLink:
 a.navigateFocusingLink(res.page, res.target)
 a.active = nil
case overlayResultHighlight:
 a.navigateHighlighting(res.page, res.target)
 a.active = nil
}
return a, nil
```

Preserve empty-page behavior only for `overlayResultOpen`; the other open constructors are emitted only with selected rows. Preserve blocked create/linkify behavior by returning before clearing `a.active`.

- [ ] **Step 7: Migrate tests and canned results**

Update overlay tests to assert `res.kind`, `res.page`, `res.taskOrdinal`, `res.target`, `res.ref`, or `res.cmd` as appropriate. In `app_sync_test.go`, replace the composite create literal with:

```go
a.active = stubOverlay{res: overlayCreate("Brand New")}
```

Do not add compatibility accessors for removed booleans; compile failures are the checklist for complete migration.

- [ ] **Step 8: Run focused tests GREEN**

```bash
go test ./internal/search -run 'TestRun(MalformedRegexSurfacesRipgrepFailure|FindsHits|NoMatchReturnsEmpty)$' -count=1
go test ./internal/views -run 'Test(OverlayResultConstructors|Picker|Search|Todos|Backlinks|Help|CreateBlockedWhileSyncing|LinkifyBlockedWhileSyncing|App.*Overlay)' -count=1
```

Expected: PASS. Also run `rg -n 'Accept:|Cancel:|Create:|Selected:|DeepLink:|FocusLinkTo:|HighlightText:|LinkifyTarget:|Cmd:' internal/views` and confirm no `OverlayResult` field construction remains.

- [ ] **Step 9: Run the full gate and commit**

Stage all Task 2 files, verify the feature branch immediately before commit, then:

```bash
git commit -m "refactor: make overlay outcomes explicit"
```

- [ ] **Step 10: Run spec review, quality review, and scoped re-review**

Spec reviewer enumerates every old outcome and proves it maps to one constructor and App branch. Quality reviewer checks the switch has no hidden precedence, zero value is a no-op, commands do not close overlays, blocked mutations keep overlays visible, and tests remain readable. Fix and re-review only concrete findings.

---

### Task 3: Put Index Invariants Behind Methods

**Interfaces:**

- Consumes: `Index.Resolve`, case-insensitive backlink semantics, ascending journal names, and existing prev/next behavior.
- Produces: `func (*Index) BacklinksTo(string) []Ref` and `func (*Index) JournalNeighbor(string, int) (string, bool)`; `backlinks` and `journals` are private.

- [ ] **Step 1: Add failing method tests**

In `internal/graph/index_test.go`, change `TestBacklinksAreCaseInsensitive` to call all case variants through the new method:

```go
for _, name := range []string{"Alpha", "alpha", "ALPHA"} {
 refs := idx.BacklinksTo(name)
 if len(refs) != 1 || refs[0].FromPage != "Note" {
  t.Fatalf("BacklinksTo(%q) = %+v, want one ref from Note", name, refs)
 }
}
```

Add:

```go
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
```

- [ ] **Step 2: Run method tests and record RED**

```bash
go test ./internal/graph -run 'Test(BacklinksAreCaseInsensitive|JournalNeighbor)$' -count=1
```

Expected: compile failures for undefined `BacklinksTo` and `JournalNeighbor`.

- [ ] **Step 3: Implement private storage and methods**

In `internal/graph/index.go`:

- rename `Backlinks` to `backlinks` and `Journals` to `journals`;
- initialize/write/read those private fields in `BuildIndex`;
- keep `sort.Strings(idx.journals)` inside graph;
- add:

```go
// BacklinksTo returns references to name using the index's case-insensitive key.
func (idx *Index) BacklinksTo(name string) []Ref {
 return idx.backlinks[strings.ToLower(name)]
}

// JournalNeighbor returns the nearest indexed journal in dir (-1 or +1).
// Phantom journal-shaped dates use their insertion point in the sorted index.
func (idx *Index) JournalNeighbor(current string, dir int) (string, bool) {
 if !IsJournalPageName(current) {
  return "", false
 }
 i := sort.SearchStrings(idx.journals, current)
 j := i
 if i < len(idx.journals) && idx.journals[i] == current {
  j = i + dir
 } else if dir < 0 {
  j = i - 1
 }
 if j < 0 || j >= len(idx.journals) {
  return "", false
 }
 return idx.journals[j], true
}
```

This reproduces existing `App.journalNeighbor` behavior; do not add direction validation not present today.

- [ ] **Step 4: Migrate graph tests**

Within package `graph`, update storage-invariant tests to `idx.journals` and user-facing lookup assertions to `idx.BacklinksTo(name)`. Keep direct private-field checks only where the test specifically verifies sorted storage or fold-collision construction.

- [ ] **Step 5: Migrate view callers and tests**

In `internal/views/backlinks.go`:

```go
src := idx.BacklinksTo(target)
```

Remove the now-unused `strings` import.

In `internal/views/app.go`, replace both `a.journalNeighbor(...)` calls with `a.idx.JournalNeighbor(...)`, delete `App.journalNeighbor`, and remove the `sort` import.

In `internal/views/backlinks_test.go`:

- replace read-only `idx.Backlinks[strings.ToLower("Hub")]` with `idx.BacklinksTo("Hub")`;
- for scroll-only tests, construct `b := NewBacklinks(...)`, append synthetic refs directly to `b.refs`, then rebuild `b.rows = b.buildRows()` instead of mutating Index internals;
- remove `strings.ToLower` uses/import only if no unrelated string assertion still needs `strings`.

- [ ] **Step 6: Run focused tests GREEN**

```bash
go test ./internal/graph -run 'Test(BuildIndex|Backlink|JournalNeighbor|FoldCollision)' -count=1
go test ./internal/views -run 'Test(Backlinks|PrevJournal|NextJournal|PrevNext)' -count=1
```

Expected: PASS. Run `rg -n '\.Backlinks\b|\.Journals\b|journalNeighbor' internal --glob '*.go'` and confirm only intentional prose/history remains—no direct field access or App method.

- [ ] **Step 7: Run the full gate and commit**

Stage Task 3 files, verify the feature branch immediately before commit, then:

```bash
git commit -m "refactor: encapsulate graph index invariants"
```

- [ ] **Step 8: Run spec review, quality review, and scoped re-review**

Spec reviewer checks all non-graph callers use methods and prev/next phantom semantics are unchanged. Quality reviewer checks private storage, method docs, binary-search edge cases, returned-slice expectations, imports, and focused view tests. Fix and re-review findings.

---

### Task 4: Consolidate Boot Setup and Correct Contracts

**Interfaces:**

- Consumes: existing test graph helpers `cloneFixtureGraph` and `writeGraph`, `App.nowFunc`, and production boot order `Init` → index message → `WindowSizeMsg`.
- Produces: one `bootApp(t, ...bootConfig)` helper; corrected comments only in production code.

- [ ] **Step 1: Rewrite one call to establish RED**

In `internal/views/app_dispatch_test.go`, change the first line of `TestPeriodJumpsToTodayJournal` to:

```go
a := bootApp(t, bootConfig{now: time.Date(2026, 5, 23, 12, 0, 0, 0, time.UTC)})
```

- [ ] **Step 2: Run the focused test and record RED**

```bash
go test ./internal/views -run '^TestPeriodJumpsToTodayJournal$' -count=1
```

Expected: compile failure `undefined: bootConfig` or too many arguments to `bootApp`.

- [ ] **Step 3: Implement the single configurable boot helper**

In `internal/views/app_test.go`, replace `bootAppAt` and `bootApp` with:

```go
type bootConfig struct {
 files map[string]string
 now   time.Time
}

// bootApp loads a test graph and initializes PageView through the production
// boot order. With no config it clones the shared fixture.
func bootApp(t *testing.T, configs ...bootConfig) *App {
 t.Helper()
 quietTerm(t)
 if len(configs) > 1 {
  t.Fatalf("bootApp accepts at most one config, got %d", len(configs))
 }
 cfg := bootConfig{}
 if len(configs) == 1 {
  cfg = configs[0]
 }

 graphPath := cloneFixtureGraph(t)
 if cfg.files != nil {
  graphPath, _ = writeGraph(t, cfg.files)
 }
 a := New(graphPath, "test")
 if !cfg.now.IsZero() {
  now := cfg.now
  a.nowFunc = func() time.Time { return now }
 }
 cmd := a.Init()
 if cmd == nil {
  t.Fatal("Init returned nil cmd")
 }
 a.Update(cmd())
 a.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
 if a.page == nil {
  t.Fatal("PageView not constructed after boot")
 }
 return a
}
```

Avoid cloning the fixture when custom files are supplied: implement the `graphPath` branch so only one graph helper runs. The final code should be:

```go
var graphPath string
if cfg.files != nil {
 graphPath, _ = writeGraph(t, cfg.files)
} else {
 graphPath = cloneFixtureGraph(t)
}
```

- [ ] **Step 4: Migrate every helper call and delete the duplicate**

- replace `bootAppAt(t, now)` with `bootApp(t, bootConfig{now: now})` in `app_dispatch_test.go` and `app_history_test.go`;
- replace `bootAppWithGraph(t, files)` with `bootApp(t, bootConfig{files: files})` in `app_test.go` and `app_canonical_test.go`;
- delete `bootAppWithGraph` from `app_canonical_test.go` and clean unused imports;
- retain all plain `bootApp(t)` calls unchanged.

Run:

```bash
rg -n 'bootAppAt|bootAppWithGraph' internal/views
```

Expected: no matches.

- [ ] **Step 5: Correct the two production comments**

In `internal/render/page.go`, change the `stripQueryAndEmbedBlocks` doc to state that a block closes either with `}}` on its own line or on the opening line for the self-closing form, and that fenced literals remain intact.

In `internal/graph/types.go`, change the `Path` field comment to:

```go
Path string // filesystem path; absolute only when the supplied graph path is absolute
```

Do not call `filepath.Abs` and do not change `BuildIndex` behavior.

- [ ] **Step 6: Run focused tests GREEN**

```bash
go test ./internal/views -run 'Test(TodayJournalNameUsesNowFunc|Period|PrevJournal|NextJournal|NavigateCanonicalizes|Reindex|IndexWarnings)' -count=1
go test ./internal/render -run 'TestRender(SingleLineQueryOrEmbed|PageStripsQueryAndEmbedBlocks)' -count=1
go test ./internal/graph -run 'TestBuildIndex' -count=1
```

Expected: PASS. No golden regeneration.

- [ ] **Step 7: Run the full gate and commit**

Stage Task 4 files, verify the feature branch immediately before commit, then:

```bash
git commit -m "test: consolidate app boot helper"
```

The comment corrections belong in this adjacent cleanup commit; do not create a separate docs-only production commit.

- [ ] **Step 8: Run spec review, quality review, and scoped re-review**

Spec reviewer checks there is one boot sequence, custom graphs do not clone the fixture first, pinned time is assigned before index delivery/resize, path behavior is unchanged, and same-line closure is documented. Quality reviewer checks optional-config ergonomics, closure capture, imports, and test readability.

---

### Task 5: Whole-Branch Review and Completion

**Interfaces:**

- Consumes: all four task commits, the approved design spec, this plan, the authoritative graph backlog, and Forgejo PR workflow.
- Produces: a fully reviewed green branch, updated backlog, one sweep PR, and a journal entry.

- [ ] **Step 1: Run the full branch gate from a clean worktree**

```bash
cd /var/home/dhemas/Development/Projects/fiatcode/weft/.claude/worktrees/invariants-sweep
test "$(git rev-parse --abbrev-ref HEAD)" = "refactor/invariants-sweep"
test -z "$(gofmt -l .)"
go vet ./...
"$(go env GOPATH)/bin/staticcheck" ./...
go test -race ./...
git status --short
git diff --check origin/main...HEAD
git log --oneline origin/main..HEAD
```

Expected: all checks pass; only intentional commits appear; worktree is clean.

- [ ] **Step 2: Dispatch whole-branch final review**

Use a fresh `openai-codex/gpt-5.6-sol` reviewer. Require it to `cd` into the worktree and verify the branch before reading. Give it:

- `docs/superpowers/specs/2026-07-29-invariants-sweep-design.md`;
- `docs/superpowers/plans/2026-07-29-invariants-sweep.md`;
- `git diff origin/main...HEAD`;
- the original backlog bullets and explicit exclusion of Glamour v1.0.0.

Ask for cross-task correctness, plan-level omissions, illegal overlay states, graph/render ordinal agreement, folded/sorted invariant leaks, behavior drift, missing tests, and accidental dependency/golden changes. The reviewer reports only; it does not edit or commit.

- [ ] **Step 3: Fix final-review findings with scoped re-reviews**

For each real finding, dispatch a terra-or-stronger implementer with the exact file/behavior correction. Every fix uses TDD where behavior changes, full gate, staging, immediate branch verification, and a conventional commit. Then dispatch scoped spec and quality re-reviewers for only that finding set. Repeat until clear.

- [ ] **Step 4: Push and open one Forgejo PR**

```bash
cd /var/home/dhemas/Development/Projects/fiatcode/weft/.claude/worktrees/invariants-sweep
git push -u origin refactor/invariants-sweep
```

Use the Forgejo skill. Try `tea pr create`; if linked-worktree detection fails, use the skill's curl fallback with `git credential fill` without printing credentials. Target `main`, title with Conventional Commit style, and include:

- the three invariant boundaries;
- bundled docs/helper/coverage items;
- explicit Glamour exclusion;
- RED evidence summary per task;
- per-task spec/quality reviews and whole-branch final review;
- final gate results and golden status.

- [ ] **Step 5: Update the authoritative backlog**

Read `${WEFT_GRAPH}/pages/weft Backlog.md` again immediately before editing. Move these completed bullets from `## Open` to `## Done`, preserving the page's terse voice and adding `[PR N](https://git.fiatcode.dev/fiatcode/weft/pulls/N)` plus the session date:

- shared open-task predicate;
- `OverlayResult` constructors/tagged outcomes;
- Index-owned backlink/journal invariants;
- picker create-row, malformed-regex search, and emphasis left-boundary coverage extras;
- boot helper consolidation;
- `stripQueryAndEmbedBlocks` comment;
- `PageMeta.Path` comment.

Leave `glamour v1.0.0 major bump + general dep refresh` at the top of `## Open`. Leave deferred coverage bullets for PageView, completion-strip, and CLI helpers in `Open`; rewrite the coverage bullet to contain only those remaining cases. Do not commit the external graph file in the weft repository.

- [ ] **Step 6: Journal the session**

Invoke the `journal-update` skill. Record the invariant sweep, key design decisions, tests/gates, review process, PR link, and the deferred Glamour session in the user's weft graph.

- [ ] **Step 7: Report completion**

Report the PR URL, branch, commits, final gate evidence, backlog path updated, journal entry, and any intentionally deferred coverage. Do not claim merge; the requested deliverable is an open sweep PR.
