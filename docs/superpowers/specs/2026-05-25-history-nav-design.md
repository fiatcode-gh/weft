# Back/Next Navigation — Design

**Status:** approved, ready for plan
**Date:** 2026-05-25
**Repo:** `~/Development/Projects/fiatcode/peekseq`

## Goal

Add browser-style back/next navigation to the page view, restoring viewport
scroll offset and link-cursor position when stepping through history. This is
a QOL feature for sit-and-read browsing: jumping into a link and then back to
where you were should not lose your place.

## Behaviour

Mental model is a browser history stack with a current pointer:

- Every page change captures the *departing* page's `{offset, cursor}` into
  the current history entry, then pushes the new page as a fresh entry with
  default scroll/cursor.
- `[` walks back one entry, `]` walks forward one entry. Both restore the
  stored `{offset, cursor}` for the target entry.
- Following a new link / picker selection / search hit / backlinks pick /
  todos pick while not at the tail of history **truncates forward history**
  before pushing the new entry. Standard browser behaviour — no stale
  forward state after a divergence.
- At the start of history, `[` is a no-op. At the tail, `]` is a no-op.
- The initial today-journal page (set up by `tryInitPage`) is the first
  history entry.

## Keybinds

Active only in `modePage` (page view, no overlay open):

- `[` — back
- `]` — forward

Rationale: these are the only easy-to-type ASCII keys not already bound. They
do not collide with `n/N` (cycle links), `j/k` / `ctrl-d` / `ctrl-u` (scroll),
`b` (backlinks), `T` (todos), `?` (help), `R` (reindex), `/` (search),
`ctrl-p` (picker), `enter` (follow link), `q` / `ctrl-c` (quit). They also
read as "prev/next chapter" in `less` and `man`.

## State

The `App` owns history; `PageView` stays a pure renderer that exposes the
state needed to capture/restore.

```go
type historyEntry struct {
    page   string
    offset int  // PageView viewport YOffset at time of departure
    cursor int  // PageView.cursor at time of departure (-1 = no link selected)
}

// in App:
hist    []historyEntry
histIdx int  // -1 before the first page is shown
```

## Trigger points

Every existing `a.page.SetPage(name)` call site in `internal/views/app.go`
becomes a call to a new `a.navigate(name)` helper:

- picker accept (modePicker)
- search accept (modeSearch)
- backlinks accept (modeBacklinks)
- todos accept (modeTodos)
- link follow (`enter` in modePage)

`navigate(name)` does:

1. Refresh the current entry's scroll state: set
   `hist[histIdx].offset = a.page.Offset()` and
   `hist[histIdx].cursor = a.page.Cursor()`. (The page field already
   matches `a.page.Page()`; no need to rewrite it.) Skip this step on the
   first navigate call, where `histIdx == -1`.
2. Truncate `hist` to length `histIdx+1` (drop any forward history). At
   `histIdx == -1` this is `hist = hist[:0]`.
3. Append `historyEntry{page: name, offset: 0, cursor: -1}` and bump
   `histIdx`.
4. Call `a.page.SetPage(name)` (existing behaviour: resets cursor to -1 and
   reloads content).

The first page (today's journal) is created in `tryInitPage`. After it
constructs the `PageView`, seed history with
`historyEntry{page: todayJournalName(), offset: 0, cursor: -1}` and set
`histIdx = 0`. Initialise `histIdx` to `-1` in `New`.

## Back / forward handlers

Two new modePage key cases in `app.go`:

```go
case "[":
    a.historyBack()
case "]":
    a.historyForward()
```

Each handler:

1. Bounds-check the move (no-op if already at edge).
2. Capture current `{a.page.Offset(), a.page.Cursor()}` into the *current*
   `hist[histIdx]` so today's scroll/cursor is preserved if the user walks
   forward again.
3. Adjust `histIdx`.
4. Read `target := hist[histIdx]`.
5. Call `a.page.SetPage(target.page)` — loads content, resets state.
6. Call `a.page.Restore(target.offset, target.cursor)` — clamps offset to
   the new content height and sets cursor (clamped to `len(result.Links)`).

## PageView additions

Three new methods on `*PageView`:

```go
func (p *PageView) Offset() int     // returns p.vp.YOffset
func (p *PageView) Cursor() int     // returns p.cursor
func (p *PageView) Restore(offset, cursor int)
```

`Restore` clamps both inputs:

- `offset` is clamped to `[0, max(0, totalLines - vp.Height)]` where
  `totalLines` comes from the current rendered content.
- `cursor` is clamped to `[-1, len(p.result.Links)-1]`. A cursor that no
  longer points to a valid link (page content changed) silently falls back
  to `-1`.

`Restore` does not call `scrollToCursor` — the caller already has an offset.

## R (rebuild index) interaction

History is preserved across `R`. History entries reference pages by name;
if a page has been deleted, `SetPage` already shows "(no entry yet for this
page)" and the restored offset/cursor clamp safely (cursor → -1 because the
link list is empty, offset → 0).

## Status bar

No change. History count / current position is noise relative to the page
name + link counter that already lives there.

## Help overlay

Add a new section to `helpSections` in `help.go`, positioned after "Links":

```go
{"History", []helpRow{
    {"[", "back"},
    {"]", "forward"},
}},
```

## Tests

New `internal/views/app_test.go` driving the `*App` model directly (the
existing view tests construct `PageView` directly; for history we need the
App's state machine). Use the fixture graph.

- `TestHistoryBackForward` — open picker, select Alpha, open picker, select
  Beta; `[` shows Alpha; `]` shows Beta. Assert `a.page.Page()` after each.
- `TestHistoryBranchTruncatesForward` — navigate A → B, press `[` (now at
  A with forward to B), follow a link to C, press `]` — must be no-op
  (still on C).
- `TestHistoryRestoresScrollAndCursor` — on A, scroll a few lines and cycle
  to link 2, navigate to B, press `[`, assert `a.page.Offset()` and
  `a.page.Cursor()` match what they were on A.
- `TestHistoryBackAtStartNoop` — fresh App with one page, `[` is no-op.
- `TestHistoryForwardAtTailNoop` — `]` at tail is no-op.

Golden-file view tests are unaffected; this feature is keymap/state-only and
does not change rendered output unless tested via the history-specific cases
above.

## YAGNI (explicitly out)

- History depth cap. Each entry is a string and two ints; thousands fit
  trivially.
- Persistence across sessions.
- Status-bar history indicator (← prev / next →). Skip until requested.
- `Alt+left/right` bindings. TUI delivery of those is inconsistent and the
  ASCII bindings cover the use case.
- Per-page-view `SetPage` overload that takes `{offset, cursor}`. Two
  method calls in the handler is simpler and keeps `SetPage` semantics
  unchanged.

## Files touched

- `internal/views/app.go` — add history fields, `navigate`,
  `historyBack`, `historyForward`, key handlers, seed entry in
  `tryInitPage`.
- `internal/views/page.go` — add `Offset`, `Cursor`, `Restore`.
- `internal/views/help.go` — new "History" section.
- `internal/views/app_test.go` — new file, history behaviour tests.
