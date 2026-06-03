# Today's Journal Jump & Prev/Next Navigation — Design

**Status:** approved, ready for plan
**Date:** 2026-05-28
**Repo:** `~/Development/Projects/fiatcode/peekseq`

## Goal

Add three keybinds to the page view:

- `.` jumps to today's journal from anywhere.
- `<` walks to the previous journal (gated to journal context).
- `>` walks to the next journal (gated to journal context).

Closes the daily-workflow gap where reaching today's journal requires
`Ctrl-P` → type `2026_05_28` → enter. Walking back through prior journals
becomes a two-keystroke loop instead of repeated picker round-trips.

## Behaviour

### Keymap

Active only in `modePage` (page view, no overlay open). Reach the page-view
key dispatch in `app.go` alongside `g` / `G` / `n` / `N` / `[` / `]`.

| Key | Action                    | Gating                                |
|-----|---------------------------|---------------------------------------|
| `.` | jump to today's journal   | always available in page view         |
| `<` | jump to previous journal  | only when current page is a journal   |
| `>` | jump to next journal      | only when current page is a journal   |

All three route through the existing `navigate(name)` chokepoint, so every
successful jump pushes a history entry and `[` rewinds out of the journal
walk like any other navigation. Overlays remain unaware of these keys.

### `.` resolution

1. Compute `today := a.todayJournalName()` — `a.nowFunc().Format("2006-01-02")`.
2. If `a.idx.ByName[today]` exists → `a.navigate(today)`.
3. Else → set a transient status `"no journal for <today>"`, no navigation,
   no history push.

Idempotent on today's journal: a second `.` calls `navigate(today)` again,
which is consistent with the existing behaviour for re-navigating to the
current page (re-renders, pushes a redundant history entry — already true
for picker-accepting the current page).

### `<` / `>` resolution

1. If the current page is not a journal → silent no-op. No status hint;
   gating is the contract, hints would be noise.
2. Else look up the current page's index in `a.idx.Journals` (binary
   search — slice is sorted). Compute neighbour:
   - `<` → previous index. At the oldest journal → transient status
     `"no earlier journal"`, no navigation.
   - `>` → next index. At the newest journal → transient status
     `"no later journal"`, no navigation.
3. On success → `a.navigate(neighbour)`.

"Previous" and "next" are by lexical order of journal page names, which is
chronological because names are `YYYY-MM-DD`. Gap days are skipped — `<`
from `2026-05-28` lands on `2026-05-26` if `2026-05-27` does not exist.

## State

### `internal/graph/Index`

New field, populated inside `BuildIndex` after the first directory pass and
before the body-parsing pass:

```go
type Index struct {
    // ... existing fields ...
    Journals []string  // journal page names, sorted ascending
}
```

Build step: filter `idx.Pages` where `IsJournal`, project `.Name`,
`sort.Strings`. Empty slice when the graph has no `journals/` directory or
no `.md` files inside it — `len == 0`, no nil-vs-empty distinction.

`internal/graph` stays date-free; the slice is just "what exists, sorted".

### `internal/views/App`

```go
type App struct {
    // ... existing fields ...
    nowFunc func() time.Time
}
```

Defaults to `time.Now` in `New`. Tests inject a fixed clock by writing
`a.nowFunc = func() time.Time { return fixedDate }` between `New` and
`Init` (the index build is synchronous in `bootApp`, so the clock is in
place before any user input).

The existing free function `func todayJournalName() string` (`app.go:78`)
becomes a method `func (a *App) todayJournalName() string` returning
`a.nowFunc().Format("2006-01-02")`. The call site in `tryInitPage` is
updated; no other callers exist. The initial-boot page therefore also
respects the injected clock, which keeps existing app-level tests
deterministic if they later need it.

### `journalNeighbor` helper

File-local in `app.go`, no new file:

```go
// journalNeighbor returns the neighbouring journal name and true, or
// "" and false if current isn't a journal, or there is no neighbour on
// that side. dir is -1 (prev) or +1 (next).
func (a *App) journalNeighbor(current string, dir int) (string, bool) {
    js := a.idx.Journals
    i := sort.SearchStrings(js, current)
    if i == len(js) || js[i] != current {
        return "", false  // current isn't a journal
    }
    j := i + dir
    if j < 0 || j >= len(js) {
        return "", false  // edge
    }
    return js[j], true
}
```

## Status hints

No transient-hint surface exists today. Add one to `App` as a single
ephemeral string:

```go
type App struct {
    // ... existing fields ...
    hint string  // single-cycle right-side status replacement
}
```

`statusBar()` (`app.go:372`) is modified so the right segment renders
`a.hint` (styled with `statusFaint`) when `a.hint != ""`, falling back to
the existing `"? help"` (plus optional scroll indicator) otherwise. The
hint occupies the same right-side slot, so the bar stays single-line and
the existing left-truncation clamp still applies.

The hint clears at the top of every key-handler entry — `Update` for
`tea.KeyMsg`, before dispatching. Any subsequent keystroke therefore
restores `"? help"`. A `WindowSizeMsg` does not clear it (resizing
shouldn't lose the message).

The three hint strings:

- `"no journal for <YYYY-MM-DD>"` — `.` with today absent.
- `"no earlier journal"` — `<` at the oldest journal.
- `"no later journal"` — `>` at the newest journal.

This is a single-purpose surface, not a general notification system. If a
future feature needs richer messaging, revisit then.

## Help overlay

Extend `helpSections` in `help.go`. Add three rows to the existing *Open*
section (where `Ctrl-P` / `b` / `T` already live):

```
.    today's journal
<    previous journal
>    next journal
```

Help golden needs regenerating once (`go test ./... -update`) and visually
diffed before staging.

## R (rebuild index) interaction

`idx.Journals` is rebuilt as part of `BuildIndex`, so `R` picks up new
journal files automatically. History entries reference pages by name, so a
journal that has since been deleted on disk shows the existing "(no entry
yet for this page)" splash when navigated to via `[`. Nothing special to
do.

## Tests

### `internal/graph/index_test.go`

New test: `TestBuildIndexJournalsSorted`
- Asserts `idx.Journals` for the fixture graph equals the expected sorted
  slice of journal page names (the fixture has 9 journals spanning Jan
  through May 2026).
- Asserts every entry corresponds to a `PageMeta` with `IsJournal == true`
  (i.e. no `pages/` entries leaked in).

New test: `TestBuildIndexJournalsEmptyWhenNoJournals`
- Builds an index against a tmp graph dir with only `pages/`. Asserts
  `len(idx.Journals) == 0`.

### `internal/views/app_dispatch_test.go`

New tests in the existing file, using `bootApp` plus a fixed `nowFunc`
hook. Pin `nowFunc` to a date that matches a fixture journal for the
"happy path" cases and a date that doesn't for the absent-today case.

- `TestPeriodJumpsToTodayJournal` — pin `nowFunc` to a fixture date that
  has a journal; press `.` from a non-journal page (e.g. via picker land
  on `Alpha`); assert current page is today's journal and history depth
  increased by one.
- `TestPeriodOnAbsentTodayShowsStatusHint` — pin `nowFunc` to a date that
  has no journal in the fixture; press `.`; assert current page unchanged,
  history depth unchanged, status surface contains `no journal for <date>`.
- `TestPeriodIdempotentOnTodayJournal` — start on today (pinned), press
  `.` again; assert still on today's journal (a redundant history push
  is acceptable per the spec).
- `TestPrevJournalWalksBackwards` — start on an interior journal; press
  `<`; assert current page is the prior journal in `idx.Journals`.
- `TestNextJournalWalksForward` — symmetric.
- `TestPrevJournalSkipsGapDays` — start on `2026-04-20` in the fixture;
  press `<`; assert landed page is `2026-03-15` (the fixture already has
  the gap between these two — no fixture change needed).
- `TestPrevJournalAtOldestShowsStatusHint` — start on the oldest
  fixture journal; press `<`; assert page unchanged, history unchanged,
  status contains `no earlier journal`.
- `TestNextJournalAtNewestShowsStatusHint` — symmetric.
- `TestPrevNextInertOutsideJournalContext` — start on a non-journal page
  (`Alpha`); press `<` then `>`; assert no navigation, no history change,
  no status hint at all.

### Goldens

Only the help-overlay golden(s) need regeneration after the three new
rows land in `helpSections`. No other view goldens are touched —
picker / page / backlinks / todos / search renders are unaffected by
keymap additions.

If the fixture grows a new journal to exercise the gap-skip test, the
todos-dashboard goldens may regenerate; re-run `go test ./... -update`
and visually diff.

### Manual smoke

On 2026-05-28 against the real graph:

1. `go run ./cmd/peekseq --graph ~/Documents/fiat-codex`
2. From any page, `.` → lands on `2026-05-28`.
3. `<` → lands on `2026-05-26` (`5_27` is missing in the real graph,
   confirming the skip-gap behaviour).
4. `<` again → `2026-05-25`.
5. `>` `>` → back to `2026-05-28`.
6. `>` once more → status `"no later journal"`, no navigation.
7. `[` → rewinds out of the journal walk back to the original page.

## Files touched

- `internal/graph/index.go` — populate `Index.Journals` in `BuildIndex`.
- `internal/graph/index_test.go` — `TestBuildIndexJournalsSorted`,
  `TestBuildIndexJournalsEmptyWhenNoJournals`.
- `internal/views/app.go` — add `nowFunc` + `hint` fields, promote
  `todayJournalName` to a method, add `journalNeighbor` helper, add the
  `.` / `<` / `>` key handlers, plumb `hint` through `statusBar()`, clear
  `hint` at the top of `tea.KeyMsg` handling.
- `internal/views/help.go` — add three rows to the *Open* section.
- `internal/views/app_dispatch_test.go` — the eight tests above.
- `internal/views/testdata/*.golden` — regenerate the help-overlay
  golden(s) only.

## YAGNI (explicitly out)

- **Reading `config.edn`** for `:journal/file-name-format`. The current
  default (`yyyy_MM_dd` filename ⇒ `YYYY-MM-DD` page name) covers every
  realistic graph. Adding EDN parsing is a much larger thread; revisit if
  a user with a non-default format files an issue.
- **Auto-creating today's journal.** Violates the read-only invariant.
  Status hint is the right escape hatch.
- **Calendar-day stepping for `<` / `>`.** Decided in brainstorm: skip
  gap days. A literal calendar walker can be added later as a separate
  binding if anyone wants one.
- **Wrapping at the edges of `idx.Journals`.** Status hint + no nav is
  less surprising than cyclic wrap.
- **Prev/next from a non-journal page** (anchored on today's date or the
  most-recent existing journal). Decided in brainstorm: gated to journal
  context. Keeps the model tight; user can always press `.` first to
  enter journal-walking mode.
- **A dedicated Journals overlay.** Symmetric with Picker/Backlinks but
  redundant — `Ctrl-P` plus the `YYYY_MM_DD` filename already lists
  every journal in recency order. Revisit if browsing-by-date becomes a
  recurring need.
- **Status-bar indicator** of journal position (e.g. `[3/32]`). Skip
  until requested.
