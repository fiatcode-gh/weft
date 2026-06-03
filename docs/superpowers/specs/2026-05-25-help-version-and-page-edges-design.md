# Help Version, Page-Edge Keys, and Scroll Indicator — Design

**Status:** approved, ready for plan
**Date:** 2026-05-25
**Repo:** `~/Development/Projects/fiatcode/peekseq`

## Goal

Three small QOL additions before tagging a release:

1. Show the running version inside the help overlay so users on `go install`
   builds can tell what they have without quitting and running `-version`.
2. Add `g` / `G` keys to jump to the top / bottom of the current page,
   matching `less`/`vim` muscle memory.
3. Show a faint scroll-position indicator on the right of the status bar so
   the user can tell where they are in a long page.

## A. Version in help overlay

### Plumbing

`cmd/peekseq/main.go` already computes a `resolvedVersion()` string that
honours `-ldflags`, `runtime/debug.BuildInfo`, and the default `dev`.

- `views.New(graphPath string)` becomes
  `views.New(graphPath, version string)`. The `App` struct gains a
  `version string` field.
- `cmd/peekseq/main.go` passes `resolvedVersion()` into `views.New(...)`.
- `NewHelp()` becomes `NewHelp(version string)`. `Help` struct gains a
  `version string` field. The modePage `?` handler in `App.Update`
  constructs help with `NewHelp(a.version)`.

### Rendering

The help overlay's existing footer is a faint `? or esc to close` on its
own row. Replace that with a single faint row that holds the close hint on
the left and `peekseq <version>` flush right, padded by spaces to span the
content width:

```
  ?         toggle this help
  esc       close overlay
  q         quit

  ? or esc to close                          peekseq v0.1.2
```

- Compute the body's content width as the max line width across the
  already-built body — i.e. iterate the lines of the body so far and take
  `max(lipgloss.Width(line))`.
- `left := "? or esc to close"`. If `version == ""`, render only `left`
  (no version segment, no padding). Otherwise
  `right := "peekseq " + version` and build a single padded row.
- `gap := width - lipgloss.Width(left) - lipgloss.Width(right)`. If
  `gap < 2` (version unusually long, very narrow help body), put `right`
  on its own row beneath `left` instead.
- Render the whole footer through `helpFaint` so close hint and version
  share the same dim styling.

### Out of scope

- Showing version anywhere outside the help overlay (status bar, splash
  screen, etc.).
- Refreshing the version after rebuild — captured once at App construction;
  the binary doesn't change while running.

## B. `g` / `G` to top / bottom

### Keys

Scoped to `modePage` only (inert while picker / search / backlinks / todos
/ help overlays are open), matching the existing scope of `j`, `k`,
`ctrl-d`, `ctrl-u`, `[`, `]`.

- `g` — jump to the top of the current page (`vp.YOffset = 0`).
- `G` — jump to the bottom (`vp.YOffset = maxYOffset()`).

### PageView additions

Two new methods alongside `LineDown` / `LineUp` / `HalfPage*`:

```go
func (p *PageView) GotoTop()    { p.vp.GotoTop() }
func (p *PageView) GotoBottom() { p.vp.GotoBottom() }
```

`viewport.GotoTop()` / `GotoBottom()` return `[]string` of the newly visible
lines; we ignore the return value.

### App key cases

In the `modePage` switch in `App.Update`:

```go
case "g":
    a.page.GotoTop()
case "G":
    a.page.GotoBottom()
```

### Interaction with other state

- **Link cursor.** `g` / `G` only move the viewport. The link cursor stays
  where it was (same model as `j`/`k`, `ctrl-d`/`ctrl-u`). If the cursored
  link scrolls out of view, pressing `n`/`N` will re-centre on it via the
  existing `scrollToCursor` logic.
- **History.** `g` / `G` change `vp.YOffset` but do *not* push a history
  entry — they're scroll, not navigation, the same as `j`/`k`.

### Help overlay

Add one row to the "Browse" section:

```go
{"Browse", []helpRow{
    {"j / k", "scroll one line"},
    {"ctrl-d", "half page down"},
    {"ctrl-u", "half page up"},
    {"g / G", "top / bottom"},
}},
```

### Out of scope

- `gg` chord for top. `g` alone is unambiguous because no other key in the
  modePage map starts with `g`.
- `Home` / `End` aliases. Terminal delivery of those is uneven; YAGNI.

## C. Scroll indicator in status bar

### Display

The right side of the status bar gets a faint percentage immediately left
of `? help`, two spaces apart. The percentage is hidden when the page fits
the viewport (no scroll possible):

```
... Alpha · 8 links                              0%  ? help    (at top)
... Alpha · 8 links                             42%  ? help    (mid scroll)
... Alpha · 8 links                            100%  ? help    (at bottom)
... ShortPage                                        ? help    (fits viewport)
```

### PageView addition

```go
// ScrollIndicator returns "" when the page fits the viewport (no scroll
// possible), otherwise "NN%" — 0% at the top, 100% at the bottom.
func (p *PageView) ScrollIndicator() string {
    if p.vp.TotalLineCount() <= p.vp.Height {
        return ""
    }
    return fmt.Sprintf("%d%%", int(p.vp.ScrollPercent()*100))
}
```

`viewport.ScrollPercent()` is already clamped to `[0, 1]` and returns
exactly `0` at the top edge and `1` at the bottom edge, so no explicit
edge branches are needed.

`int(... * 100)` truncates; e.g. 42.6% renders as `42%`. Predictable and
matches the convention in most TUIs.

### App.statusBar() change

Replace the current `right` assignment:

```go
right := "? help"
if ind := a.page.ScrollIndicator(); ind != "" {
    right = ind + "  " + right
}
right = statusFaint.Render(right)
```

When the page fits, the right side is just `? help` flush right — no
reserved gap, no shift.

### No help-overlay change

The scroll indicator is a passive readout, not a key. The keymap is
unchanged for this piece.

## Tests

- `TestPageViewGotoTopBottom` (in `page_test.go`) — small viewport on Alpha;
  scroll mid-page; `GotoTop()` → `Offset() == 0`; `GotoBottom()` →
  `Offset() > 0` and equal to the viewport's max offset (which we can
  derive by comparing against `TotalLineCount() - Height`).
- `TestPageViewScrollIndicator` (in `page_test.go`) — three cases:
  - Big viewport over Alpha (content fits) → indicator is `""`.
  - Small viewport, `GotoTop()` → `"0%"`.
  - Small viewport, `GotoBottom()` → `"100%"`.
  - Small viewport, `HalfPageDown()` once from top → indicator matches
    regex `^\d{1,2}%$` (some intermediate percentage; exact value depends
    on viewport math, so we don't pin the number).
- `TestPageEdgeKeys` (in `app_test.go`) — drive the App through `Update`
  with `tea.KeyMsg` for `g` and `G`; assert the viewport offset changes
  accordingly (proves the keys are wired into `modePage`).
- `TestNewHelpRendersVersion` (in a new `help_test.go`) —
  `NewHelp("v0.1.2").View()` contains the literal substring
  `"peekseq v0.1.2"` and also contains `"? or esc to close"`. No golden —
  just `strings.Contains`.
- `TestNewHelpEmptyVersionHidesSegment` — `NewHelp("").View()` contains
  `"? or esc to close"` but does **not** contain the literal `"peekseq "`
  (with trailing space — distinguishes the version segment from any
  unrelated occurrence of the bare word).
- `bootApp` helper in `app_test.go` updated to pass a version string into
  `views.New(...)` — pick a stable literal like `"test"` so test output is
  deterministic.

## Files touched

- `cmd/peekseq/main.go` — pass `resolvedVersion()` into `views.New`.
- `internal/views/app.go` — `New(graphPath, version string)`, `version`
  field on `App`, `NewHelp(a.version)` wiring, two new `modePage` key
  cases (`g`, `G`), status-bar right-side update.
- `internal/views/page.go` — `GotoTop`, `GotoBottom`, `ScrollIndicator`.
- `internal/views/help.go` — `NewHelp(version string)`, `version` field,
  footer rendering, new `g / G` row in "Browse".
- `internal/views/page_test.go` — Goto and ScrollIndicator tests.
- `internal/views/app_test.go` — `bootApp` signature fix, `TestPageEdgeKeys`,
  `TestNewHelpRendersVersion`.
