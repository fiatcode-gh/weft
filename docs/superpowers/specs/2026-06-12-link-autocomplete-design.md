# Slice 2 — `[[`-autocomplete in the in-app editor

> Design spec. Written 2026-06-12. Follows Slice 1 (in-app editor,
> `docs/superpowers/specs/2026-06-12-in-app-editor-design.md`).

## Goal

While editing in peekseq's in-app markdown editor, completing wiki-links as
you type — so you *connect as you write*. Typing `[[` opens a live candidate
list of page names; filtering, selecting, and accepting splices a finished
`[[Page Name]]` into the buffer. This is the payoff for owning the input
surface (Slice 1), and the "capture + connect" fusion on the roadmap.

## Scope

### In

- **Live trigger.** Typing `[[` on a logical line, with no `]]` yet between it
  and the cursor, opens a candidate strip. No explicit summon key.
- **Fixed-position strip.** Candidates render in a small bordered box between
  the textarea and the status line — *not* a floating popup anchored at the
  cursor. Always in the same place; never clips at screen edges.
- **Fuzzy filtering.** Reuses the picker's exact primitive: `sahilm/fuzzy`
  over the same deduped page-name set (`idx.Pages` names). Completion and the
  finder behave identically.
- **Keymap while the strip is active** (see §"Keymap").
- **Create row.** When the partial matches no existing page, a
  `＋ Create "<partial>"` row lets you commit a red link in one keystroke.
  Dates are **not** suppressed (unlike the picker's create row) — inserting
  `[[2026-06-12]]` is a valid journal link and writes nothing to disk.

### Out (explicit non-goals for this slice)

- Completing or editing *inside* an already-closed `[[Foo]]`.
- Alias matching — we match page **names** only, the same set as the picker.
- A cursor-anchored floating popup (compositing a box into the textarea's
  rendered rows by row/column). Deferred; the fixed strip is the MVP.
- Multi-line links.

## Keymap (while the strip is active)

| Key | Strip active | Normal (no strip) |
|-----|--------------|-------------------|
| `↑`/`↓` | move selection | move cursor (textarea) |
| `Enter` | **accept** → insert `[[Name]]`, cursor after `]]` | newline |
| `Tab` | also accept (alias) | — |
| `Esc` | **dismiss** strip, stay editing | exit / dirty-guard |
| typing (incl. space) | filters the list | — |
| `]` typed, or zero candidates | strip closes | — |

Rationale:

- **Spaces keep the strip open** — page names contain spaces (`Meeting Notes`,
  `AI Memory`), so space can't dismiss. The strip stays open until `Esc`,
  accept, a typed `]`/closing `]]`, or zero candidates.
- **Both `Enter` and `Tab` accept** — `Enter` is Logseq muscle-memory, `Tab`
  is the classic completion key.
- Accept always inserts the closing `]]` and drops the cursor after it, so you
  keep typing your sentence.

## Architecture

- **New file `internal/views/complete.go`** holding a focused `linkCompleter`
  type — the completion controller, unit-testable in isolation. It holds the
  `*graph.Index` (name set + fuzzy), the current candidate list, the selection
  index, and a `dismissed` flag. It does **not** touch the textarea directly;
  it operates on `(logical-row text, rune-column)` inputs and reports what to
  insert.
- **`EditorView` gains a `*linkCompleter`** and orchestrates: after forwarding
  a key to the textarea, it reads `Line()` + `LineInfo()` to recover
  `(logical-row text, rune-column)`, hands that to the completer to re-derive
  state, and renders the strip. The textarea remains the single source of
  truth for buffer + cursor.
- **`NewEditorView` signature grows a `*graph.Index` param.** `app.go`'s
  `enterEditor()` passes `a.idx`.
- **No new writer.** Completion splices link text into the in-memory buffer
  only. `internal/edit` stays the sole disk writer; red links remain red until
  the page is visited and edited, exactly as today.

### Cursor API (verified against bubbles v1.0.0 source)

The textarea exposes no single rune-offset getter, but the position is fully
recoverable:

- `Line()` → current logical row index.
- `LineInfo()` → `StartColumn + ColumnOffset` equals the absolute rune-column
  within the logical row (confirmed in `textarea.go` `LineInfo`, ~L840–845).
- The logical row's text is `strings.Split(Value(), "\n")[Line()]`.

So "text on the current line before the cursor" is
`[]rune(row)[:col]` — soft-wrap is irrelevant because we work in logical-row
space.

## Trigger detection & accept mechanics

- **Derived state, not a manual open/close flag.** After any
  textarea-mutating key, scan the current logical row *backwards from the
  cursor* for the nearest `[[` with no intervening `]` or `[`. If found →
  `partial` = the runes between it and the cursor → strip active with fuzzy
  results (empty partial → recent pages, mtime-sorted, capped, like the
  picker's empty query). If not found → inactive.
- **Persisted state is minimal:** the selection index, and a `dismissed` flag
  set by `Esc` and cleared as soon as `partial` changes (so re-typing
  re-opens).
- **Accept (existing page):** feed `len([]rune(partial))` backspace `KeyMsg`s
  (deletes the partial via the textarea's own cursor-aware editing), then
  `InsertString(name + "]]")`. The cursor lands after `]]` naturally — no
  `SetValue`/row-restore dance (and there is no public row setter anyway, so
  avoiding `SetValue` is load-bearing).
- **Accept (create row):** the typed partial *is* the name, so just
  `InsertString("]]")`.
- After either accept, the line reads `[[Name]]` with no open bracket, so the
  next derivation finds the strip inactive — it closes itself. No explicit
  teardown.

## Rendering

- Reuse the picker's styles (`styleSel`, `styleFaint`, `styleBorder`).
- The strip renders between the textarea and the status line: a small bordered
  box (cap ~6–8 rows, scroll within when the candidate list is longer, like
  the picker), each row a page name, the selected row marked ` ▶ `, the
  `＋ Create` row last when applicable.
- `EditorView.SetSize` reserves the strip's rows only while it is active so the
  textarea reflows to make room.
- `EditorView.View()` composes textarea + (strip if active) + status line.

## Testing (TDD)

- **`linkCompleter` unit tests:**
  - backwards partial-extraction: `see [[Foo]] and [[bar` → `bar`; a `]`
    present after the last `[[` → no trigger; empty partial → recent list.
  - fuzzy filtering against a known name set.
  - create-row presence (zero matches → row offered; dates not suppressed).
  - accept produces the correct `(backspace-count, insert-string)` pair for
    both existing-page and create cases.
- **`EditorView.Update` tests** via `KeyMsg` sequences:
  - `[[` → type → `Enter` yields `[[Meetings]]` with the cursor after `]]`.
  - `Tab` behaves as an accept alias.
  - `Esc` dismisses the strip but leaves the buffer untouched.
  - `↑`/`↓` are stolen from the textarea only while the strip is active.
- **teatest golden** for the strip render; regenerate with `-update` and
  visually inspect before staging.

## Out-of-scope follow-ups (noted, not built)

- Cursor-anchored floating popup (upgrade from the fixed strip).
- Completing inside existing `[[…]]` links / alias matching.
- Slice 3: unlinked references + one-key linkify (the first writer to files
  other than the current page).
