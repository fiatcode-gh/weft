# Slice 1 — In-app markdown editor + picker page-creation

Date: 2026-06-12
Status: proposed

## Context & goal

peekseq is evolving from a read-mostly *navigator* of a Logseq-format graph into a
terminal **knowledge base** focused on three verbs: **capture, find, connect**.
Slice 1 delivers the foundation everything else sits on: a single, unified, in-app
editing surface, plus the ability to create new pages.

The durable requirement is **plain markdown files + wiki-links, git-syncable**.
Logseq is the inspiration, not a spec — but peekseq stays Logseq-*readable* so the
existing graph at `~/Documents/fiat-codex` and the `journal-update` / `memory-update`
tooling that writes it keep working untouched. No migration.

Out of scope (later slices): `[[` autocomplete in the editor (Slice 2); unlinked
references + linkify (Slice 3); queries, block model, outliner editing, WYSIWYG.

## Decisions (settled in brainstorming)

1. **Unified editing.** `e` opens an in-app raw-markdown editor for *any* page.
   `E` (shift) keeps the existing `$EDITOR` handoff as a power-user escape hatch.
   This is the one behavioral change to an existing binding: today `e` shells out;
   now `e` edits in-app and `E` shells out.
2. **Save / exit model (Model A).** `Ctrl+S` writes to disk; `Esc` exits to the
   rendered read view. If the buffer is dirty, `Esc` raises a **save / discard /
   cancel** prompt; on that prompt `Esc`/cancel returns to editing with nothing lost.
   A stray keystroke can never silently mutate the graph.
3. **Cursor lands at top of file** on entering edit. Uniform across all pages.
   *Superseded: the editor now opens on
   the source line behind the read view's top visible line (or a visible link
   cursor), at the top when unscrolled — so the reader isn't made to scroll
   back down to the passage they were reading. The cursor/viewport agreement
   invariant is retained.*
4. **Navigation:** textarea built-ins (arrows, `Alt`+arrows word-jump, `Ctrl+A`/`E`
   line ends, `Ctrl+Home`/`Ctrl+End` file top/bottom) **plus** `PageUp`/`PageDown`,
   which v1.0.0 textarea lacks. Readline-style, not modal vim (that's what `E` is for).
5. **File creation deferred to save**, for any page. `e` on a not-yet-existent page
   opens an empty buffer; the file is written only on `Ctrl+S`. `e`-then-discard
   creates nothing — tightening the "writes only on deliberate action" invariant.
   Routing: date-shaped name → `journals/YYYY_MM_DD.md`; any other name →
   `pages/<name>.md` with `/` mangled to `___` (flat `pages/`, no subdirectories).
6. **Picker page-creation.** When the picker query matches no existing page, a
   synthetic `＋ Create "<name>"` row appears; selecting it navigates to that page
   and drops straight into the editor on an empty buffer. Picker-created pages always
   land in `pages/`. Query is normalized: trim spaces, strip a trailing `.md`
   (case-insensitive) so a typed extension never doubles to `somepage.md.md`.
   **Create is suppressed for date-shaped queries** (`YYYY-MM-DD`) — those belong to
   the journal flow (`.`), and a `pages/2026-06-15.md` lookalike would be a footgun.

## Architecture

### New component: `internal/views/editor.go` — `EditorView`

A **full-screen mode**, not a centered overlay. Wraps `bubbles/textarea`.

```
type EditorView struct {
    ta       textarea.Model
    path     string   // target file (may not exist yet)
    pageName string   // logical page name, for the status line
    baseline string   // content as loaded; dirty == ta.Value() != baseline
    isNew    bool     // file did not exist when editing began
    saved    bool     // at least one successful Ctrl+S this session
    mode     editorMode // editing | confirmingExit
    width, height int
}

type editorMode int // editing, confirmingExit
```

State machine:

- **editing** — the textarea handles all keys *except* the three we intercept:
  - `Ctrl+S` → save (write file, clear dirty, mark `saved`); stay in editing.
  - `Esc` (or `Ctrl+C`) → if clean, request exit; if dirty, enter `confirmingExit`.
    `Ctrl+C` is treated as `Esc` here so a reflexive force-quit can't nuke unsaved work.
  - `PageUp` / `PageDown` → move the cursor a viewport-height of lines.
- **confirmingExit** — renders `Save changes?  [s]ave · [d]iscard · [c]ancel`:
  - `s` → `{Save: true, Exit: true}` (save then leave).
  - `d` → `{Exit: true}` — discard the unsaved buffer (already-saved content stays on disk).
  - `c` / `Esc` → back to editing, buffer intact.
  - any other key → ignored (stays in `confirmingExit`).

`dirty()` is computed as `ta.Value() != baseline`. On a successful save, `baseline`
is set to the written content, `dirty` becomes false, `isNew` becomes false.

### Result contract

Mirror the `OverlayResult` pattern. `EditorView.Update(key string)` returns:

```
type EditorResult struct {
    Save bool // write the current buffer to disk, then reindex-on-exit bookkeeping
    Exit bool // leave the editor and return to the read view
}
```

The App performs the actual disk write (keeping `internal/edit` the sole writer);
`EditorView` exposes `Content() string` (buffer, normalized to end in exactly one
`\n`) and `MarkSaved(content string)`.

### App integration (`internal/views/app.go`)

- Add field `editor *EditorView`. **Precedence:** when `editor != nil` it owns the
  screen and all keys — checked before `active` (overlays) and the page key switch.
  Editing is exclusive; no overlay opens while editing.
- `WindowSizeMsg` propagates to `editor.SetSize` (as it already does for `active`).
- **`e`** → `a.enterEditor()` (new). **`E`** → `a.editCurrent()` (the existing
  `$EDITOR` path, unchanged, just rebound from `e`).

`enterEditor()`:
1. `name := a.page.Page()`.
2. Resolve the target path: if `name` is in the index → `meta.Path`; else compute
   the deferred path — date-shaped → `journals/FilenameFromPageName(name)`,
   otherwise → `pages/FilenameFromPageName(name)`. Do **not** create the file.
3. Read existing content if the file exists (raw bytes, no trim); empty buffer if not.
4. Build `EditorView`, `ta.Focus()`, set `a.editor`.

Handling `EditorResult` in `Update` (Save is processed before Exit):
- **Save** → `path := a.editor.path`; `err := edit.WriteFile(path, []byte(a.editor.Content()))`.
  On error: `setHint`, stay in the editor (buffer preserved) and **cancel any Exit**
  in the same result — a failed save must never drop the buffer. On success:
  `a.editor.MarkSaved(content)`. No reindex yet — see below.
- **Exit** (only if Save, when present, succeeded) → `a.editor = nil`. If the editor
  `saved` anything, return `a.buildIndexCmd()`;
  the resulting `indexLoadedMsg` rebuilds `a.page` for the current name against the
  fresh index (existing handler already does this), so a newly-created page now
  renders its content. If nothing was saved, just refresh the page view from the
  current index.

**Reindex on exit, not per save.** While editing full-screen, nothing on screen reads
the index, so re-walking the graph on every `Ctrl+S` is wasted work. `Ctrl+S` writes
the file (durability); the single reindex happens once on exit when `saved` is true.
Discarding after a save still reindexes — the discard only drops the unsaved delta;
the last-saved content is already on disk.

### New write surface: `internal/edit`

Add the project's second deliberate write path, keeping `edit` the only package that
writes to disk:

```
// WriteFile writes data to path, creating the parent directory if needed.
func WriteFile(path string, data []byte) error
```

`os.MkdirAll(filepath.Dir(path), 0o755)` then `os.WriteFile(path, data, 0o644)`. This
covers a fresh graph missing `pages/`. `EnsureFile` / `SnapshotMtime` / `Resolve`
are unchanged.

### Picker page-creation (`internal/views/picker.go`)

- Track the normalized query and a `createName string` (empty when create isn't offered).
- After each search, set `createName` when **all** hold: normalized query is non-empty;
  `graph.IsJournalPageName(normalized)` is false; `idx.Resolve(normalized)` returns
  `false` (no exact or case-folded existing page). Normalization: `strings.TrimSpace`,
  then strip one trailing `.md` (case-insensitive).
- Render: when `createName != ""`, append a `＋ Create "<createName>"` row after the
  fuzzy matches; it participates in selection (`p.sel` ranges over matches + 1).
- `Enter` on the create row → `OverlayResult{Accept: true, Create: true, Selected: createName}`.
- App, on `Accept && Create`: `a.navigate(Selected)` then `a.enterEditor()` — the
  read view shows "(no entry yet)", the editor opens empty, `pages/<name>.md` is
  written on save (date-shaped is already excluded, so routing is always `pages/`).
- Update the stale comment at `picker.go:50` ("won't invent rows for files that don't
  exist yet") — that contract is intentionally changing.

Add `Create bool` to `OverlayResult` (`overlay.go`).

## View / layout

Edit mode renders the focused textarea filling the screen above a status line, mirroring
the page view's `height-2` budget. The edit status line shows: page name, an `[edit]`
tag, a dirty marker (`●` when unsaved), and key hints (`^S save · esc exit`). The
`confirmingExit` prompt renders in place of the hints (or as a centered one-liner over
the editor). The read view, `$EDITOR` handoff, and overlays are visually unchanged.

## Error handling

- **Write failure** (`WriteFile`) → hint, stay in editor; buffer never lost.
- **Read failure** entering an existing file → hint, don't open the editor.
- **`E` with no editor resolvable** → existing `errNoEditor` hint, unchanged.
- **Reindex failure on exit** → existing `indexLoadedMsg` path keeps the old index + hints.
- **Concurrent external edit** of the open file (another process writes it while the
  in-app editor holds it) is **not** detected in Slice 1 — the in-app save overwrites.
  Rare while the user is inside the TUI; noted as a known limitation, revisit if it bites.

## Testing

TDD, fixture-graph only (never the real graph). Goldens via `teatest` + `-update`.

- `internal/edit/editor_test.go`: `WriteFile` creates parent dir, writes, overwrites.
- `internal/views/editor_test.go`:
  - `e` loads existing content into the textarea.
  - `Ctrl+S` writes the file (temp graph) and clears dirty.
  - `Esc` on a clean buffer exits to read.
  - `Esc` on a dirty buffer shows the prompt; `s` saves+exits, `d` discards+exits,
    `c`/`Esc` returns to editing.
  - `PageUp`/`PageDown` move the cursor.
  - new page: `e` on a non-existent page → empty buffer; save creates `pages/<name>.md`;
    discard-without-save creates nothing.
  - golden frames for the editor view and the confirm prompt.
- `internal/views/picker_test.go`:
  - create row appears for a non-existing, non-date query; absent for an existing page;
    absent for a date-shaped query; trailing `.md` stripped from the name.
  - selecting create returns `{Accept, Create, Selected: <normalized name>}`.
- `internal/views/app_*_test.go`:
  - `e` opens the in-app editor; `E` still shells to `$EDITOR` (existing `fake-editor.sh`
    test moves to `E`).
  - save → exit → page view reflects saved content (reindex on exit).
  - picker create → navigate + editor opens on an empty buffer.

## Docs to update (in the implementation plan)

`AGENTS.md` (the `e`/`E` split, the new write path, `editor.go`), `README.md`,
the in-app `?` help overlay (`help.go`), and `CHANGELOG.md`.
