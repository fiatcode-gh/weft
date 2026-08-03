# PR 39 Follow-ups Implementation Plan

> Execute with pi-executing-plans, task by task.

**Goal:** Clear the two open items the 2026-07-29 audit sweep left in `[[weft Backlog]]` — the four coverage extras (PR 1) and the glamour v1.0.0 major bump + general dep refresh (PR 2).

**Spec:** the `## Open` section of `~/Documents/fiat-codex/pages/weft Backlog.md`, seeded by the audit sweep (PR 37) and worked by PRs 38–39. Lineage plans: `docs/superpowers/plans/2026-07-29-quality-audit-fixes.md`, `2026-07-29-backlog-top5.md`, `2026-07-29-invariants-sweep.md`.

## Global constraints

- Go 1.26.3 module `git.fiatcode.dev/fiatcode/weft/v2`.
- Gate before every push: `gofmt -l .` (empty), `go vet ./...`, `staticcheck ./...`, `go test -race ./...` — all green.
- Tests use `testdata/fixture-graph` or the `writeGraph` throwaway helper only — never the real graph (`$WEFT_GRAPH`).
- Golden files live in `internal/views/testdata/*.golden`; regenerate only with `go test ./... -update`, and visually diff every changed golden before staging.
- Test bodies follow `// arrange`, `// act`, `// assert` structure (project test-style convention).
- One conventional-commit per task. Two branches, two PRs: coverage extras first, dep refresh second (golden churn stays attributable to the dep bump).
- Behavior already exists for everything in PR 1 — these are pinning tests. Each task's RED step is a mutation check: break the behavior on purpose, confirm the new test goes red, restore.

## Phase A — coverage extras (branch `test/pr39-coverage-extras`, PR 1)

### Task 1: golden for `PageView.View` with an active link cursor

**Files:** `internal/views/page_test.go` (append), `internal/views/testdata/TestPageViewRendersActiveLinkCursor.golden` (created by `-update`).

The existing `TestPageViewRendersAlpha` golden renders the page with no link cursor (`cursor = -1`). `View()` splices `cursorStyle.Render(l.Display)` into the styled body when `cursor >= 0` — that branch has no golden. Alpha has at least one wiki-link (proven by `TestPageFollowCursorReturnsTarget`).

- [x] Write the test in `internal/views/page_test.go`:

```go
func TestPageViewRendersActiveLinkCursor(t *testing.T) {
 // arrange
 quietTerm(t)
 idx := loadFixture(t)
 pv := NewPageView(idx, "Alpha", 80, 24)
 pv.CycleLink(+1)
 if pv.Cursor() != 0 {
  t.Fatalf("precondition: cursor should sit on the first link, got %d", pv.Cursor())
 }

 // act + assert
 teatest.RequireEqualOutput(t, []byte(pv.View()))
}
```

- [x] Run `go test ./internal/views -run TestPageViewRendersActiveLinkCursor` — fails: no golden file.
- [x] Run it again with `-update` to write the golden.
- [x] Mutation check: temporarily delete the cursor-splice block in `PageView.View()` (page.go, the `if p.cursor >= 0 && ...` block), rerun the test — it must fail against the golden. Restore the block; test green again.
- [x] Visually diff the new golden against `TestPageViewRendersAlpha.golden` — the only difference is the cursored link's rendering.
- [x] Commit: `test: golden for PageView with an active link cursor`

### Task 2: pin `error:` render when the page file vanishes mid-session

**Files:** `internal/views/page_test.go` (append).

The behavior exists: `load()` sets `p.err` on `os.ReadFile` failure and `View()` renders `error: %v` before touching the cursor or the stale links. The reload path that reaches it mid-session is a width change (`SetSize` with a different width skips the width-keyed cache and re-reads the file). No test covers any of this today.

- [x] Write the test (imports `os`, `filepath`, `strings` are already in `page_test.go`):

```go
func TestPageViewVanishedFileRendersError(t *testing.T) {
 // arrange — a page with a link, cursor placed, then the file disappears
 quietTerm(t)
 dir, idx := writeGraph(t, map[string]string{
  "pages/Doomed.md": "- body with a link [[Alpha]]\n",
 })
 pv := NewPageView(idx, "Doomed", 80, 24)
 pv.CycleLink(+1)
 if pv.Cursor() != 0 {
  t.Fatalf("precondition: cursor should sit on the link, got %d", pv.Cursor())
 }
 if err := os.Remove(filepath.Join(dir, "pages", "Doomed.md")); err != nil {
  t.Fatal(err)
 }

 // act — a mid-session reload (terminal width change) re-reads the file
 pv.SetSize(60, 24)

 // assert — no panic, the page reports the failure
 got := pv.View()
 if !strings.Contains(got, "error:") {
  t.Errorf("vanished page should render error:, got:\n%s", got)
 }
}
```

- [x] Run `go test ./internal/views -run TestPageViewVanishedFileRendersError` — green (pinning test).
- [x] Mutation check: temporarily make `load()` ignore the `ReadFile` error (drop the `p.err = err; return` branch so it renders the empty body), rerun — the test must fail. Restore; green again.
- [x] Commit: `test: pin error render when a page file vanishes mid-session`

### Task 3: completion-strip `moveUp`

**Files:** `internal/views/complete_test.go` (append), `internal/views/editor_view_test.go` (append).

`linkCompleter.moveUp`/`moveDown` have zero direct coverage; the editor-level `TestEditorCompletion_ArrowsStealOnlyWhenActive` only exercises `KeyDown`. Two tests: completer boundary semantics, and the editor-level `KeyUp` mirror of the existing `KeyDown` test.

- [x] Write the completer test in `complete_test.go`:

```go
func TestLinkCompleterMoveBoundaries(t *testing.T) {
 // arrange
 c := &linkCompleter{
  active: true,
  sel:    0,
  cands:  []linkCandidate{{name: "One"}, {name: "Two"}, {name: "Three"}},
 }

 // act + assert — up at the top clamps, down walks, up walks back,
 // down at the bottom clamps
 c.moveUp()
 if c.sel != 0 {
  t.Errorf("moveUp at the top must clamp to 0; sel=%d", c.sel)
 }
 c.moveDown()
 if c.sel != 1 {
  t.Errorf("moveDown should advance to 1; sel=%d", c.sel)
 }
 c.moveUp()
 if c.sel != 0 {
  t.Errorf("moveUp should retreat to 0; sel=%d", c.sel)
 }
 c.sel = 2
 c.moveDown()
 if c.sel != 2 {
  t.Errorf("moveDown at the bottom must clamp to 2; sel=%d", c.sel)
 }
}
```

- [x] Write the editor-level test in `editor_view_test.go`:

```go
func TestEditorCompletion_KeyUpMovesAndClampsSelection(t *testing.T) {
 // arrange
 quietTerm(t)
 e := NewEditorView(loadFixture(t), "Note", "/tmp/n.md", "", true, 80, 24)
 typeRunes(e, "[[")
 if !e.completer.active || len(e.completer.cands) < 2 {
  t.Fatalf("precondition: active strip with 2+ candidates; active=%v cands=%d",
   e.completer.active, len(e.completer.cands))
 }
 before := e.ta.Value()
 e.Update(tea.KeyMsg{Type: tea.KeyDown})
 if e.completer.sel != 1 {
  t.Fatalf("precondition: down should move the selection to 1; sel=%d", e.completer.sel)
 }

 // act
 e.Update(tea.KeyMsg{Type: tea.KeyUp})

 // assert — up retreats, clamps at the top, and never edits the buffer
 if e.completer.sel != 0 {
  t.Errorf("up should move the selection back to 0; sel=%d", e.completer.sel)
 }
 e.Update(tea.KeyMsg{Type: tea.KeyUp})
 if e.completer.sel != 0 {
  t.Errorf("up at the first row must clamp to 0; sel=%d", e.completer.sel)
 }
 if e.ta.Value() != before {
  t.Errorf("up must not edit the buffer while completing")
 }
}
```

- [x] Run both: `go test ./internal/views -run 'TestLinkCompleterMoveBoundaries|TestEditorCompletion_KeyUpMovesAndClampsSelection'` — green (pinning).
- [x] Mutation check: invert the `moveUp` guard in `complete.go` (`c.sel > 0` → `c.sel >= 0`, so `sel` goes negative), rerun — both tests must fail. Restore; green.
- [x] Commit: `test: cover completion-strip moveUp navigation and boundaries`

### Task 4: table tests for `resolveGraphPath` / `resolvedVersion`

**Files:** `cmd/weft/main_test.go` (append).

`debugLogEnabled` and `shortenPseudoVersion` already have table tests; the two remaining CLI helpers don't. `resolvedVersion`'s `dev` case relies on the test binary's `debug.ReadBuildInfo` reporting no usable module version (`""` or `"(devel)"`) — true for `go test` builds; the comment says so.

- [x] Write the tests:

```go
func TestResolveGraphPath(t *testing.T) {
 cases := []struct {
  name string
  flag string
  env  string
  want string
 }{
  {"flag wins over env", "/flag/graph", "/env/graph", "/flag/graph"},
  {"env fallback when flag empty", "", "/env/graph", "/env/graph"},
  {"both empty", "", "", ""},
 }
 for _, tc := range cases {
  t.Run(tc.name, func(t *testing.T) {
   if got := resolveGraphPath(tc.flag, tc.env); got != tc.want {
    t.Errorf("resolveGraphPath(%q, %q) = %q, want %q",
     tc.flag, tc.env, got, tc.want)
   }
  })
 }
}

func TestResolvedVersion(t *testing.T) {
 orig := Version
 t.Cleanup(func() { Version = orig })

 cases := []struct {
  name string
  set  string
  want string
 }{
  {"ldflags-injected value wins", "v9.9.9", "v9.9.9"},
  // Test binaries carry no usable module version ("" or "(devel)"),
  // so the dev sentinel falls straight through.
  {"dev sentinel falls back to dev", "dev", "dev"},
 }
 for _, tc := range cases {
  t.Run(tc.name, func(t *testing.T) {
   Version = tc.set
   if got := resolvedVersion(); got != tc.want {
    t.Errorf("resolvedVersion() with Version=%q = %q, want %q",
     tc.set, got, tc.want)
   }
  })
 }
}
```

- [x] Run: `go test ./cmd/weft -run 'TestResolveGraphPath|TestResolvedVersion'` — green (pinning).
- [x] Mutation check: swap `resolveGraphPath`'s preference (return `envVal` first), rerun — the flag-wins case must fail. Restore; green.
- [x] Commit: `test: table tests for resolveGraphPath and resolvedVersion`

### Task 5: full gate, push, open PR 1

- [x] `gofmt -l .` → empty; `go vet ./...`; `staticcheck ./...`; `go test -race ./...` — all green.
- [x] Push branch and create the PR with `tea pr create`; body lists the four pinned behaviors and cites `[[weft Backlog]]` lineage.

## Phase B — glamour v1.0.0 + dep refresh (branch `chore/dep-refresh`, PR 2)

Cut from a fresh `main` after PR 1 merges, so golden churn is attributable to the bump alone.

### Task 6: bump glamour to v1.0.0

**Files:** `go.mod`, `go.sum`.

glamour v1.0.0 stays on the `github.com/charmbracelet/glamour` module path. The breaking migration (to `charm.land/glamour/v2`, removed `WithAutoStyle`/`WithColorProfile`) belongs to the future v2, not v1. weft's glamour API surface (`NewTermRenderer`, `WithStyles`, `WithStandardStyle`, `WithWordWrap`, `WithChromaFormatter`) is the stable core — expect no code changes, but rendering output between 0.9.1 and 1.0.0 may shift goldens.

- [ ] `go get github.com/charmbracelet/glamour@v1.0.0 && go mod tidy`
- [ ] `go build ./...` — if compile errors appear, fix them at the single call site (`internal/render/page.go`) and note each one in the PR body.
- [ ] `go test ./...` — if goldens fail, run `go test ./... -update`, then visually diff every changed golden: only style/wrap differences from the new glamour version are acceptable; structural changes (missing links, changed margins) are a stop-and-investigate.
- [ ] Commit: `chore: bump glamour to v1.0.0`

### Task 7: general dep refresh

**Files:** `go.mod`, `go.sum`.

The charm stack beyond glamour: bubbletea v1.3.10, bubbles v1.0.0, lipgloss v1.1.0, x/ansi v0.11.6, teatest (pseudo-version), termenv v0.16.0, fuzzy v0.1.2. The 2026-07-29 sweep bumped x/text and goldmark minimally for CVEs; this pass takes the rest to latest.

- [ ] `go get -u ./... && go mod tidy` — bumps direct and indirect deps to latest.
- [ ] Check `git diff go.mod`: confirm no surprise major-path migration landed (a `charm.land/...` entry is a stop-and-investigate — that would be the v2 line, not this refresh).
- [ ] `go build ./...`; fix any breakage at call sites, noting each fix for the PR body.
- [ ] `go test ./...` — golden-failure handling identical to Task 6.
- [ ] `govulncheck ./...` — must report no reachable vulnerabilities (this refresh supersedes the sweep's minimal vuln pins).
- [ ] Commit: `chore: general dep refresh`

### Task 8: full gate, push, open PR 2

- [ ] `gofmt -l .` → empty; `go vet ./...`; `staticcheck ./...`; `go test -race ./...` — all green.
- [ ] Push and create the PR with `tea pr create`; body lists the version moves (old → new), any golden regenerations with their justification, and any call-site fixes.

## Phase C — graph bookkeeping (after both PRs merge)

### Task 9: update `[[weft Backlog]]` and journal the session

- [ ] Move both `## Open` TODOs to `## Done` with PR links; keep the `glamour` deferral note's history intact in the done entry.
- [ ] Journal the session via the `journal-update` skill: both PRs, the pinning-test mutation checks, the dep-refresh outcome.
