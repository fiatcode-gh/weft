# Editor syntax tinting + framing parity

> Design spec. Written 2026-06-17. Polishes the in-app editor (`internal/views/editor.go`).
>
> Goal: close the visual cliff between weft's richly-rendered read view
> (Glamour) and its flat, monochrome, flush-left edit view. Two parts:
> **framing parity** (stop the horizontal jump on mode switch) and
> **hybrid syntax tinting** (live colorization of the raw markdown buffer).

## Goal

Pressing `e` to edit a page should feel like the same app as reading it.
Today it drops the user from colored, left-inset, word-wrapped Glamour output
into undifferentiated grey text flush at column 0 and full terminal width. This
spec makes the editor (a) occupy the same horizontal box as the read view and
(b) tint the raw markdown source so headings, task markers, blockquotes, code
fences, and `[[wiki-links]]` read at a glance.

## Scope

### In

- **Framing parity.** The editor's text occupies the same horizontal box as the
  read view: a left inset matching Glamour's document margin (2 columns for the
  standard styles) and a bounded content width matching the read view's
  word-wrap width. The completion strip and status line align to the same inset.
- **Line-level tinting** (whole visible row), for every visible row *except the
  cursor's current row*:
  - **Headings** (`#`…`######` prefix) — bold.
  - **Blockquotes** (`>` prefix) — dim/faint.
  - **Task-marker lines** (`- TODO|DOING|LATER|WAITING|DONE|CANCELED|CANCELLED|NOW`)
    — color the marker using the **existing** `render.taskMarkerStyles` palette
    so the editor, read view, and todos dashboard agree.
  - **Code-fence delimiter lines** (lines matching `^\s*```` ) — dim.
- **Inline `[[wiki-link]]` tinting.** Within each tinted (non-cursor) row, match
  `[[…]]` spans (reusing the `wikiLinkRe` shape from `internal/render/page.go`)
  and tint them blue (`colorHighlight`, the read view's link color), composed
  under any line-level style.
- **Cursor-row stays raw.** The row the cursor is on is left exactly as
  `textarea` rendered it — no tinting — which both avoids cursor-cell SGR
  conflicts and gives a deliberate "reveal raw source where you edit" affordance.
- **Theme/NO_COLOR fidelity.** Reuse `theme.go`'s `colorHighlight` and
  `render`'s task-marker styles (numeric ANSI colors), so tinting honors the
  user's terminal theme and degrades under `NO_COLOR` exactly as the read view.

### Out (explicit non-goals)

- **Code-block *body* tinting.** v1 classifies each visible row independently
  with no cross-row block state. Fenced-code *delimiter* lines are dimmed, but
  body lines inside a fence are not tinted. (Tracking fence parity across a
  scrolled, soft-wrapped viewport is the expensive part; deferred.)
- **Inline tinting of non-link tokens** — emphasis (`*…*`), inline code
  (`` `…` ``), links/images other than `[[…]]`. Out for v1; line-level cues plus
  link tinting cover the loudest read-view signals.
- **Tinting the cursor's current row.** Deliberately excluded (see above).
- **Any change to editing semantics** — save, dirty-tracking, exit-confirm, and
  `[[` completion logic are untouched. This is a render-layer addition plus the
  layout inset.

## Approach

### Enabling fact

`bubbles/textarea` (v1.0.0, and v2) exposes **no per-token styling hook**: its
`Styles` struct covers only base/cursor-line/prompt, and the viewport's
`StyleLineFunc` is whole-line. Crucially, textarea injects SGR codes only into
the **cursor's own row** (cursor-line + cursor-cell styling); other visible rows
are emitted as plain text under the default text style. That makes a
post-processing decorator safe for non-cursor rows.

### Mechanism

A pure `string → string` decorator over `e.ta.View()` output:

1. Split the textarea's rendered view into visible rows.
2. Determine the cursor's **visible** row (from `textarea`'s line/row-offset
   info) and leave that row verbatim.
3. For each other row: classify by intra-row regex (heading / blockquote /
   task-marker / code-fence delimiter / normal), apply the whole-line style,
   then run the inline `[[…]]` pass over the result.
4. Rejoin. The decorated string replaces `e.ta.View()` inside `EditorView.View()`,
   before the completion strip and status line are appended.

Because the decorator only touches non-cursor rows (which carry no textarea
SGR), it does not need to parse or preserve interleaved ANSI from textarea —
keeping it robust and unit-testable.

### Framing

In `layout()`, reduce the textarea content width by the inset and offset the
rendered block left-pad to match Glamour's document margin (2 cols for standard
styles). Confirm the exact inset against the read view's rendered output so the
two boxes align; the completion strip and status line adopt the same inset.

## Components / boundaries

- **`internal/views/editor.go`** — `layout()` gains the framing inset;
  `View()` routes textarea output through the new decorator.
- **New tinting unit** (e.g. `tintMarkdownRow(row string) string` plus a
  row-classifier, in `editor.go` or a small sibling file) — pure functions, no
  textarea/state dependency. This is the isolated, independently testable core.
- **Reused, not duplicated:** `colorHighlight` (`theme.go`), task-marker styles
  and `wikiLinkRe` shape (`internal/render/page.go`). If a constant must cross
  the `views`/`render` boundary, follow the existing convention in `theme.go`'s
  note about deliberate non-unification — prefer a local mirror over a premature
  shared package, matching what the codebase already does for the 9/11/12/8
  marker colors.

## Error handling / edge cases

- **Cursor row at viewport top/bottom, soft-wrapped lines** — the decorator
  operates on visible rows as textarea laid them out; a soft-wrapped logical
  line spanning rows is classified per visible row (a heading that wraps stays
  bold on each of its rows; acceptable).
- **Empty buffer / new page** — no rows to tint; decorator is a no-op.
- **`NO_COLOR`** — styles collapse to no-ops via lipgloss, same as read view.
- **Narrow terminals** — the inset must not push content width below a sane
  floor; clamp as `layout()` already does for the completion strip.

## Testing

- **Unit tests** for the classifier + `tintMarkdownRow`: heading, blockquote,
  each task marker, code-fence delimiter, a row with a `[[link]]`, a row with a
  link inside a heading (composition), and a plain row (unchanged). Pure
  functions — no teatest needed.
- **Cursor-row-raw**: a view-level test asserting the cursor's row is emitted
  untinted while a sibling row is tinted.
- **Framing**: assert the editor's left inset/content width matches the read
  view's for a representative width.
- **Goldens**: regenerate `internal/views/testdata/*.golden` with
  `go test ./... -update` and **visually diff** before staging.
- `go vet ./... && go test ./...` green before commit.

## Deferred / future

- Code-block *body* tinting (needs cross-row fence state).
- Inline emphasis / inline-code tinting.
- The other polish levers from brainstorming (markdown-aware list/checkbox
  continuation, save-success flash + richer status line, in-buffer find) —
  separate specs.
