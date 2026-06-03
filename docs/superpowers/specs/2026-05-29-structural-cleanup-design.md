# Design: peekseq structural cleanup

**Date:** 2026-05-29
**Status:** approved (design), pending implementation plan

## Goal

Remove duplication and one layer violation from the codebase without adding
architectural ceremony. The code is already small, idiomatic Go with a clean
`graph → render → views` layering and good golden-file test coverage. This pass
tightens it; it does not rearchitect it.

**Principles in force:** DRY + KISS + Single-Responsibility-as-a-lens, gated by
YAGNI, guarded by the existing test suite. Golden frames stay green **without**
`-update` unless a change is consciously intended (none expected).

**Explicitly out of scope:** DDD, ports/adapters, dependency-inversion
interfaces beyond the one `Overlay` abstraction below, and any new package not
listed here. No behavior changes — this is a pure refactor; the TUI looks and
behaves identically afterward.

## Current state (baseline)

- `go vet ./... && go test ./...` is green as of this design.
- Layering already points inward: `graph` (domain/index) ← `render`
  (presentation) ← `views` (UI); `cmd/peekseq` wires it.
- Two concentrations of structural debt:
  1. The four list overlays (`Picker`, `SearchView`, `Backlinks`, `Todos`)
     independently reimplement the same geometry, selection, scroll-window, and
     View-scaffold logic. `scrollWindow` is byte-identical across
     picker/search/backlinks; `innerWidth` is identical across all four.
  2. `views/search.go` shells out to `rg` and parses its JSON — process I/O and
     decoding living inside a Bubble Tea view (a layer violation).
- `App.Update` repeats an identical 13-line accept/cancel block per overlay
  mode, plus parallel `View` and `SetSize` switches and five typed pointers
  alongside a `modeT` enum.

## Target structure

New files/packages (all under `internal/`):

```
internal/search/          NEW package — ripgrep execution + JSON parsing
  search.go               Hit, Span, Run(graphPath, query) ([]Hit, error)
  search_test.go          parsing tests (moved from views)
internal/views/
  overlay.go        NEW   Overlay interface + OverlayResult
  listbox.go        NEW   embedded base: width/height/sel, SetSize, innerWidth,
                          moveUp/moveDown (visibleRows stays per-overlay)
  scroll.go         NEW   pure funcs: scrollWindow(sel,count,rows), clampInt
  theme.go          NEW   shared styles + palette colors
  keys.go           NEW   key-string constants
```

## Component 1 — `Overlay` interface + generic App dispatch

```go
// OverlayResult is what an overlay's Update returns to the App.
type OverlayResult struct {
    Selected string  // page to open when Accept (empty = close without navigating)
    Accept   bool
    Cancel   bool
    Cmd      tea.Cmd // optional async work (search)
}

// Overlay is a modal view layered over the page. App routes keys to the active
// overlay and closes it on Accept/Cancel.
type Overlay interface {
    Update(key string) OverlayResult
    View() string
    SetSize(w, h int)
}
```

Changes to `App`:

- Drop `mode modeT` and the five typed pointers (`picker, search, backlinks,
  todos, help`); hold a single `active Overlay` instead. The `modeT` enum and
  its constants are deleted.
- Page-mode keys that open overlays (`ctrl+p`, `/`, `b`, `T`, `?`) set
  `a.active = NewPicker(...)` etc.
- The five per-mode update blocks collapse into one:
  ```go
  if a.active != nil {
      res := a.active.Update(key)
      if res.Cancel { a.active = nil; return a, res.Cmd }
      if res.Accept {
          if res.Selected != "" { a.navigate(res.Selected) }
          a.active = nil
      }
      return a, res.Cmd
  }
  ```
- `View()` and the `SetSize` propagation lose their switches:
  `if a.active != nil { … a.active.View() / a.active.SetSize(w, h) }`.

Search's two quirks, absorbed so it fits the uniform interface:

- It needs `graphPath`; it already holds `idx`, so `Update` reads
  `s.idx.GraphPath` internally rather than taking a parameter.
- It returns a hit, not a page name. The `pageNameFromHitPath` resolution moves
  **into** `SearchView` (which already builds a `pathToName` map), and `Update`
  returns `OverlayResult{Selected: name, Accept: true}`. A hit whose path is not
  in the index resolves to `Selected: ""`, so `App` closes the overlay without
  navigating — preserving today's behavior.
- `searchDoneMsg` continues to be delivered to the active search via one type
  assertion in `App.Update`: `if s, ok := a.active.(*SearchView); ok { s.Apply(m) }`.

`Help` implements `Overlay` too, returning `OverlayResult{Cancel: true}` when
dismissed. The self-contradicting comment at the old `app.go:250` ("Help …
doesn't expose a SetSize" immediately before calling it) is deleted along with
the switch.

**Test impact:** `app_overlay_test.go` is rewritten to assert against
`a.active` instead of `a.mode` / typed pointers — via a small test-only
`activeKind()` helper or type switches. The `pageNameFromHitPath` unit test in
`app_dispatch_test.go` moves to cover `SearchView`'s resolution.

## Component 2 — `listBox` base + scroll helpers

- `scroll.go` — pure functions:
  - `scrollWindow(sel, count, rows int) (start, end int)` — the previously
    byte-identical 16-line centering window, now defined once.
  - `clampInt(v, lo, hi int) int`.
- `listbox.go` — `listBox{ width, height, sel int }` with `SetSize(w, h)`,
  `innerWidth()` (identical across overlays: `clampInt(width-2-4-4, 30, 80)`),
  `moveUp()`, `moveDown(count int)`. The four overlays **embed** `listBox` and
  delete their own copies of these fields and methods.
- `visibleRows()` stays per-overlay because the `chrome` constant (9/10/10/11)
  and the max-rows ceiling (14 vs todos' 16) genuinely differ; each body becomes
  `clampInt(b.height-chrome, listVisibleRowsMin, maxRows)`.
- `Todos` **overrides** the window logic with its existing `computeWindow`
  (which accounts for group-header and inter-group blank-line row costs).
  Embedding makes the override clean.

**View scaffold:** extract only the truly identical fragments into small
helpers — the "↑/↓ N more above/below" hint lines, the footer hint line, and the
bordered-panel render (`border.Width(inner+4).Render(body)`). Do **not** force a
single `renderList` template: the four bodies differ enough (picker's mtime
column, search's prompt + states + match highlighting, backlinks' target header,
todos' group headers) that one parameterized renderer would be a leaky
abstraction. KISS wins over DRY at this boundary.

## Component 3 — `internal/search` package

- Move `SearchHit` → `search.Hit`, `SearchSpan` → `search.Span`, `runRipgrep`,
  and `parseRipgrepJSON` into a new `internal/search` package. Public surface:
  `search.Run(graphPath, query string) ([]Hit, error)`.
- `search` depends only on the standard library (`os/exec`, `encoding/json`,
  `bufio`, `bytes`) — **not** on `graph` or `views`. Dependencies still point
  inward.
- `SearchView` keeps the presentation-only helpers (`matchesWithin`,
  `highlightMatches`) and now holds `[]search.Hit`.
- The ripgrep parsing tests move from `views` to `internal/search`; the
  `SearchView` Update/View tests stay in `views`.

## Component 4 — theme + quick wins

- `theme.go` — shared styles: `styleSel` (foreground `0` on background `12`,
  bold), `styleFaint`, `styleTitle`, `styleBorder` (rounded border + `Padding(1,
  2)`), plus named palette colors. Replace the per-file `*Sel` / `*Faint` /
  `*Title` / `*Border` vars across `app.go`, `picker.go`, `search.go`,
  `backlinks.go`, `todos.go`, `help.go`. Semantically distinct styles (search
  match emphasis, backlink position, todo markers, page-link cursor) keep their
  own vars but reference the shared palette colors where they overlap.
- **Boundary (YAGNI):** `render`'s task-marker color map duplicates `todos`'
  map, but the two live in different packages. Do **not** create a cross-package
  theme package for ~4 color constants now; leave a comment in each noting the
  duplication. Promoting to a shared `internal/theme` is a deferred follow-up if
  a third consumer appears.
- `keys.go` — constants for the repeated key strings (`keyEsc = "esc"`,
  `keyEnter`, `keyUp`, `keyDown`, `keyCtrlK`, `keyCtrlJ`, …) used across the
  overlays and `App`.
- Delete the contradictory Help/`SetSize` comment.

## Execution order

Each step is a self-contained `refactor:` (or `test:`) commit. The full suite
(`go vet ./... && go test ./...`) must be green after every step, and no golden
file gets `-update` unless a frame change is consciously intended (none are
expected in this pass).

1. **Quick wins** — `keys.go`, delete the stale comment. Isolated; goldens
   unchanged.
2. **theme.go** — style consolidation. Goldens unchanged.
3. **scroll.go + listbox.go** — extract pure helpers, embed the base in all four
   overlays, route `visibleRows` through `clampInt`. Goldens unchanged.
4. **internal/search** — extract `rg` execution + parsing, repoint `SearchView`,
   move parsing tests. Goldens unchanged.
5. **Overlay interface + generic dispatch** — introduce `overlay.go`, make all
   five overlays (including `Help`) implement `Overlay`, replace the `modeT`
   enum + typed pointers with `active Overlay`, move `pageNameFromHitPath`
   resolution into `SearchView`. Rewrite `app_overlay_test.go` and relocate the
   `pageNameFromHitPath` test.

## Risks & mitigations

- **Golden drift on the overlay/theme refactors.** Mitigation: goldens are the
  oracle; any drift is investigated as a regression, never blindly `-update`d.
- **Test rewrite churn in step 5.** Expected and accepted (user opted into the
  full interface dispatch). Confined to `app_overlay_test.go` and one test in
  `app_dispatch_test.go`.
- **Search behavior parity.** The hit→page resolution and the
  index-miss-closes-without-navigating path must match current behavior; covered
  by existing search Update tests plus the relocated resolution test.

## Success criteria

- All four overlays embed `listBox`; `scrollWindow` and `innerWidth` exist once.
- `rg` execution and JSON parsing live in `internal/search`, not `views`.
- `App` has no `modeT` enum and no per-overlay typed pointers; one `active
  Overlay` drives update/view/size.
- Shared styles live in `theme.go`; key strings live in `keys.go`.
- `go vet ./... && go test ./...` green with no golden `-update`.
- Net line count down; no new exported surface beyond `internal/search` and the
  `Overlay`/`OverlayResult` types.
