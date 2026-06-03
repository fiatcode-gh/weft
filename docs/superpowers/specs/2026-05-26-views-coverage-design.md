# Views Coverage — Design

**Status:** draft, awaiting approval
**Date:** 2026-05-26
**Repo:** `~/Development/Projects/fiatcode/peekseq`

## Goal

Lift `internal/views` from 44.5% line coverage to ~70%+ by writing tests for
the code paths that have never executed under `go test`. Two outcomes:

1. **Regression protection** for the previously-untested overlays and
   navigation paths.
2. **Bug discovery** as a side-effect — none of this code has ever run in a
   test, so we expect at least a handful of latent issues to surface.

Coverage % is the proxy metric; *exercising the previously-dead paths* is the
real goal.

## Scope

### In

**Cold spots (currently 0% on most functions):**

- `internal/views/search.go` — every function except `parseRipgrepJSON`
  (already 85%). Specifically: `NewSearchView`, `hitLabel`, `SetSize`,
  `Query`, `SetQuery`, `SearchCmd`, `Apply`, `Update`, `matchesWithin`,
  `highlightMatches`, `innerWidth`, `visibleRows`, `scrollWindow`, `View`,
  `shortPath`, `runRipgrep`.
- `internal/views/backlinks.go` — entire file (`NewBacklinks`, `SetSize`,
  `Update`, `innerWidth`, `View`).
- `internal/views/app.go` — `Update` (currently 21%), `View` (0%),
  `centerOverlay` (0%), `pageNameFromHitPath` (0%).

**0%-coverage orphans in lukewarm files:**

- `picker.go` — `SetSize`, `padTo`
- `todos.go` — `SetSize`
- `page.go` — `FollowCursor`, `LineUp`, `LineDown`, `HalfPageUp`

**State-machine deepening in lukewarm files:**

- `picker.go` — `Update` (29% → ≥80%), `consumeKey` (36% → ≥80%),
  `scrollWindow` (25% → ≥80%)
- `todos.go` — `Update` (30% → ≥80%)

### Out

- `picker.go relativeTime` (cosmetic polish, returns diminishing).
- `page.go View` 60% → higher (not in any 0% function; can fall out of
  other tests).
- New features, refactors, helper extractions beyond what each test needs.
- Integration / end-to-end test scaffolding beyond what `teatest` already
  gives us.

## Testing patterns

The repo's existing test style is the reference — match it, don't invent a
new one.

- **`teatest`** for view-level tests (`app_test.go`, `picker_test.go` are
  the templates).
- **Golden files** under `internal/views/testdata/*.golden` where rendered
  frame output is the assertion. Regenerate with
  `go test ./... -update` and **visually diff before staging** — that's the
  AGENTS.md-mandated flow.
- **Prefer direct model-state assertions over goldens** when checking
  `Update` behaviour — cursor moves, mode transitions, query state,
  filter cycling. Goldens are brittle for state-machine assertions; reserve
  them for "this is what the user sees" snapshots.
- **`runRipgrep` / `SearchCmd`**: drive real `rg` against
  `testdata/fixture-graph`. If `rg` is missing on PATH, `t.Skip` with a
  clear reason — mirrors the runtime preflight in `cmd/peekseq/main.go`.
  No subprocess mocking; the value of the test is exercising the actual
  binary peekseq depends on.

## Fixture changes

Today's fixture (`testdata/fixture-graph/`: 3 pages, 2 journals) is too thin
for the scope. It can't drive:

- Multi-hit search across many files
- Multi-source backlinks (a page linked from ≥3 others)
- Picker scroll windowing (need more pages than fit on screen)
- `relativeTime` branches beyond "today" and "1 day ago"

Extend to roughly **6–8 pages and 8–10 journals**, picked to drive specific
code paths:

- One page with ≥3 inbound backlinks
- One page using `___` slash encoding in its filename (in addition to the
  existing `proj___nested.md`)
- One page with mixed workflow markers (TODO / LATER / DONE / WAITING)
- Journal filenames spanning today, ~3 days ago, ~10 days ago, and
  ~60 days ago to exercise `relativeTime`'s `today` / `N days ago` /
  `last week` / `weeks ago` branches via the picker's relative-time
  column (full `relativeTime` coverage stays out of scope)

The fixture grow will invalidate every existing golden because the picker
view lists all pages. Plan: regenerate via `-update`, visually diff, commit
the fixture additions + regenerated goldens as **one focused commit** so
reviewers see the intentional churn.

## Sequencing

Four logical units, each independently shippable:

1. **Grow the fixture**, regenerate goldens, ensure existing tests stay
   green. Foundation for the rest.
2. **`search.go` tests.** Largest single-file gain.
3. **`backlinks.go` + `app.go`** Update/View/centerOverlay tests.
4. **Lukewarm pass:** picker/todos state-machine deepening **and** the
   0%-coverage orphans across picker, todos, and page.

Each unit is its own PR. If unit 2 surfaces a real bug, fix it in the same
PR when small; split into a `fix:` PR if it needs its own design.

## Risks and unknowns

- **CI ripgrep availability.** Tests `t.Skip` when `rg` is absent. If CI
  doesn't have `rg`, the new search tests effectively don't run there.
  Mitigation: confirm CI has `rg`; if not, surface as a separate item
  (don't paper over with mocks).
- **Bug discovery slows the plan.** If unit 2 or 4 surfaces ≥3 real bugs,
  each may need triage; the design budget assumes a small handful. Worst
  case: split into "tests" PR (skipping the buggy assertion with a
  pointer to a follow-up) and "fixes" PR.
- **Golden churn cost.** Growing the fixture invalidates every existing
  golden. Mitigation: do fixture + regenerated goldens as one commit, with
  a clear before/after in the PR description.

## Non-goals (explicit)

- Hitting any particular coverage number is a *signal*, not the target.
  80%+ per cold-spot file is the operational threshold, but if a function
  has irreducible-to-test branches (e.g. `os.Getenv`-style env fallback),
  we accept lower and document why.
- No changes to test helpers, fixture loader code, or `views/util.go`
  beyond what these tests specifically need.
- No new features. Bug fixes that emerge from tests are in scope; feature
  additions are not.
