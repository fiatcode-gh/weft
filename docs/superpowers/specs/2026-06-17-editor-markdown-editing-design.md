# Editor markdown-aware editing + open-at-top

> Design spec. Written 2026-06-17. Follows the editor syntax-tinting + framing
> work (PR #14, merged). Polishes the in-app editor's *behavior* the way that
> round polished its *appearance*.

## Goal

Make weft's in-app editor (`internal/views/editor.go`) behave like a native
markdown editor for the two structures the graph actually uses — `-` bullets
and Logseq workflow markers — and fix a cursor/viewport mismatch on open. Three
behaviors:

1. **Bullet continuation** — Enter on a `- ` bullet continues the list at the
   same indent; Enter on an empty bullet ends it.
2. **Marker cycle** — `ctrl+t` cycles the current bullet's workflow marker
   `plain → TODO → DONE → plain`.
3. **Open at top** — opening a page lands the cursor (and viewport) at the top,
   not the bottom.
   *Superseded: the editor now opens on
   the source line behind the read view's top visible line (or a visible link
   cursor), at the top when the page is unscrolled. The cursor/viewport
   agreement this decision protected is retained.*

## Why this shape (grounded in the real graph)

A scan of `~/Documents/fiat-codex` decided the scope:

- Tasks are **Logseq workflow markers**, not GitHub checkboxes: `- DONE` (202×),
  `- TODO` (14×), and **zero** `- [ ]` checkboxes.
- Structure is **`-` bullets nested at 2-space indent**, exclusively — no
  `*`/`+` bullets.
- Ordered `N.` lists appear **3 times** in the entire graph (negligible).

So checkbox toggling, ordered-list increment, and `*`/`+` continuation are all
**out** — YAGNI against the actual corpus. The marker vocabulary mirrors what
the tinting work (`internal/views/edit_tint.go`) already colors.

## Scope

### In
- Enter-driven `-` bullet continuation, empty-bullet termination, indent
  preservation, mid-line split, plain-marker-on-continue.
- `ctrl+t` workflow-marker cycle `plain → TODO → DONE → plain` on `- ` bullets.
- Cursor + viewport at top of buffer on editor open.
- Pure decision functions for the two key behaviors, unit-tested in isolation.

### Out (explicit non-goals)
- GitHub `- [ ]` checkboxes (absent from the graph).
- Ordered-list (`N.`) increment; `*`/`+` bullet continuation.
- The full Logseq marker set in the cycle (DOING/LATER/WAITING/NOW/CANCELED) —
  only TODO/DONE are used in practice; a 3-state cycle covers create/complete/clear.
- Multi-level outdent on empty-bullet Enter (clears the line fully instead).
- Any change to tinting, framing, save/dirty-tracking, or `[[` completion.

## Architecture

Two key-intercepts in `EditorView.Update()` (before keys reach the textarea,
beside the existing `ctrl+s`/`esc`/`pgup`/`pgdown` handling) plus a cursor move
in `NewEditorView`. The **decision logic is pure functions** in a new file
`internal/views/edit_markdown.go`; `Update()` translates their results into
textarea mutations via the textarea's own primitives (cursor moves +
`InsertString` + delete keys), exactly as `acceptCompletion()` already does, so
the textarea's cursor/scroll/wrap stay consistent. This mirrors the existing
pure/impure split (`linkCompleter` is pure; `EditorView` translates).

### New file: `internal/views/edit_markdown.go`

Pure helpers (no textarea/state dependency):

- `bulletPrefix(line string) (prefix string, ok bool)` — if `line` matches
  `^(\s*)- ` returns `indent + "- "` and `ok=true`; otherwise `ok=false`.
- `isEmptyBullet(line string) bool` — true when `line` matches `^(\s*)-\s*$`
  (marker only, no content).
- `cycleMarkerLine(line string) (newLine string, newCol, oldCol int, ok bool)` —
  given a bullet line and the current cursor column, returns the line with its
  marker advanced one step in the cycle and the cursor column adjusted by the
  marker-length delta. `ok=false` if `line` is not a `- ` bullet. (Signature
  refined in the plan; the contract is: pure transform + cursor-delta math.)

Regexes mirror the subset used by `edit_tint.go` (`^(\s*)- `, marker tokens);
they are intentionally local, per the documented `views`/`render` mirror
convention — no new shared package.

### Reusable textarea helper (in `editor.go`)

`replaceCurrentLine(newText string, newCol int)` — moves to column 0 of the
current row, deletes the old line's runes forward (exactly its rune length, so
the trailing newline is preserved), inserts `newText`, then sets the cursor
column to `newCol`. Backs both the empty-bullet terminate (`newText=""`,
`newCol=0`) and the marker cycle.

## Behavior detail

### Bullet continuation — intercept `enter`

Reached only when the completion strip is **closed** (the completer-active block
in `Update()` already consumes `enter` to accept a candidate and returns first).
Using the current logical line (from the existing `cursorLineSplit()`):

- **Not a `- ` bullet** (`bulletPrefix` returns `ok=false`) → do **not** intercept;
  fall through so the textarea inserts a normal newline.
- **Empty bullet** (`isEmptyBullet`) → terminate: `replaceCurrentLine("", 0)`.
  Ends the list; no new line inserted. Nested empty bullets clear fully (no
  multi-level outdent — deliberate v1 simplification).
- **Non-empty bullet** → continue: `e.ta.InsertString("\n" + prefix)` where
  `prefix = indent + "- "`. `InsertString` inserts at the cursor, so any
  after-cursor text rides down onto the new bullet (mid-line split works
  uniformly with end-of-line). The continuation marker is always plain `- `,
  never the source line's workflow marker.

After the edit, call `e.refreshCompleter(false)` (the inserted text contains no
`[[`, so the strip stays closed) and return the textarea's blink cmd as usual.

### Marker cycle — intercept `ctrl+t`

`ctrl+t` is not in the completer-active intercept set, so it falls through to
the main switch and works whether or not the strip is open (harmless either way).

- If the current line is not a `- ` bullet (`cycleMarkerLine` `ok=false`) →
  no-op (`return EditorResult{}, nil`).
- Otherwise compute `(newLine, newCol)` from `cycleMarkerLine` (using the current
  cursor column for the delta) and apply via `replaceCurrentLine(newLine, newCol)`.

Cycle transitions (content = text after `indent + "- "`):
- **plain → TODO**: insert `"TODO "` after `- ` (line +5; cursor +5 if it was at
  or past the marker column, else unchanged).
- **TODO → DONE**: replace the leading `TODO` with `DONE` (length unchanged;
  cursor unchanged).
- **DONE → plain**: remove the leading `"DONE "` (line −5; cursor −5, floored at
  the marker column).

Detection treats both `"TODO "`/`"DONE "` and a bare `"TODO"`/`"DONE"` (no
trailing content) as that marker.

### Open at top — in `NewEditorView`

> **Superseded.** The motivation below
> (the cursor sat off-screen while the viewport stayed at the top) is kept as
> the invariant: cursor and viewport must agree on open. The row-0 loop is
> replaced by placing the clamped anchor line on the top row of the editor window.

`textarea.SetValue` is `Reset()` + `InsertString()`, leaving the cursor at the
**end** of the buffer; `Reset()` puts the viewport at the top (`GotoTop`). The
viewport only repositions inside `Update()`, so on open the cursor is at the
bottom while the viewport is at the top — the cursor is off-screen.

Fix: after `SetValue`, move the cursor to the start —
`for e.ta.Line() > 0 { e.ta.CursorUp() }` then `e.ta.CursorStart()`. The
viewport is already at the top from `Reset()`, so cursor and viewport now agree;
no extra reposition needed. Done before `e.baseline = e.Content()` and the
initial `refreshCompleter(false)` (both unaffected — `Content()` is
cursor-independent, and the completer reads the row-0 line).

`moveToBegin()` exists in the textarea but is unexported; the `CursorUp`-loop +
`CursorStart` uses only exported API. Pages are not large enough for the O(rows)
loop to matter.

## Error handling / edge cases

- **Enter on a non-bullet** — untouched (normal newline).
- **`ctrl+t` on a heading/blank/non-bullet** — no-op, no buffer change.
- **`replaceCurrentLine` rune accounting** — deletes exactly the old line's rune
  length so it never eats the following newline or merges lines.
- **Empty buffer / new page** — `Line()==0` already; the open-at-top loop is a
  no-op. Enter inserts a normal newline (no bullet yet).
- **`- TODO ` with no content** — treated as a non-empty bullet (content
  `"TODO"`), so Enter continues with plain `- `; that's predictable.

## Testing

- **Unit (`edit_markdown_test.go`)** — pure helpers:
  - `bulletPrefix`: plain bullet, nested-indent bullet, `- TODO` bullet, a
    non-bullet line (`ok=false`), a heading.
  - `isEmptyBullet`: `- `, `  - `, `-`, vs. `- x` (false).
  - `cycleMarkerLine`: plain→TODO, TODO→DONE, DONE→plain, non-bullet (`ok=false`),
    indent + trailing content preserved, cursor-column delta for each transition.
- **Integration (`editor_view_test.go`)** — via `Update()` / `NewEditorView`:
  - `enter` on `- foo` adds `\n- ` to the buffer; `enter` on `  - bar` continues
    at 2-space indent; `enter` on an empty `- ` clears the line.
  - `ctrl+t` on `- foo` yields `- TODO foo` in `Content()`; again → `- DONE foo`;
    again → `- foo`.
  - a fresh `EditorView` over multi-line content reports `Line()==0`.
- Frame goldens are unaffected (buffer/cursor changes asserted via
  `Content()`/`Value()`/`Line()`, not golden frames).
- `go vet ./... && go test ./...` green before each commit.

## Deferred / future
- Multi-level outdent on empty-bullet Enter.
- Full Logseq marker cycle (DOING/LATER/WAITING/NOW/CANCELED).
- Ordered-list / `*`/`+` support, `[ ]` checkboxes — only if the graph's
  conventions change.
