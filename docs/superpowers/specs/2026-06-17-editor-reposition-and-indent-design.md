# Editor viewport reposition fix + Tab indent/de-indent

> Design spec. Written 2026-06-17. Follow-up to the editor markdown-editing work
> (bullet continuation, ctrl+t marker cycle, open-at-top) after manual smoke
> testing surfaced one bug and one ergonomics gap.

## Goal

Two changes to the in-app editor (`internal/views/editor.go`):

1. **Reposition fix (bug).** When an intercept mutates the buffer (Enter bullet
   continuation, empty-bullet terminate, ctrl+t marker cycle), the textarea's
   viewport does not follow the cursor until the next keystroke — so a new
   bullet created at the bottom of a page stays off-screen until you type.
2. **Tab / Shift+Tab indent (ergonomics).** Bind Tab to indent the current line
   by one 2-space level and Shift+Tab to de-indent, when the completion strip is
   closed.

## Why the bug happens

`bubbles/textarea` repositions its viewport only at the end of `Model.Update`
(`textarea.go:1087`, `m.repositionView()`), which runs for any message. weft's
intercepts (the `enter` and `ctrl+t` cases in `EditorView.Update`) mutate the
buffer through `InsertString`/`replaceCurrentLine` and `return` early — they
never pass a message through `ta.Update`, so `repositionView` is never called.
The viewport only catches up on the next key that *is* forwarded (typing).

The fix reuses the existing `repositionMsg{}` mechanism (`layout()` already
sends it through `ta.Update` to reposition after the completion strip resizes
the textarea): poke `ta.Update(repositionMsg{})` after each intercept mutation.

## Scope

### In
- A `syncViewport()` helper that sends `repositionMsg{}` through `ta.Update` to
  reposition the viewport onto the cursor. Called after every intercept buffer
  mutation: continuation, empty-bullet terminate, ctrl+t, and the new Tab/Shift+Tab.
- `tab` (strip closed) → indent the current line by 2 spaces; `shift+tab` →
  de-indent by up to 2 leading spaces. Both consume the key (no literal tab).
- Pure helpers `indentLine` / `dedentLine` in `internal/views/edit_markdown.go`.
- Help-overlay rows + AGENTS.md note for the new keys.

### Out (explicit non-goals)
- **Hanging-indent of wrapped bullet text** (smoke-test finding #2). Blocked on
  `bubbles/textarea`: its prompt is a single fixed-width gutter
  (`getPromptString` pads every visual row to the same width) and wrapped
  continuation rows always restart at that gutter — there is no per-line
  variable indent. Matching the read view's hanging indent would require forking
  textarea's wrap/render or post-processing its wrapped output (which shifts
  content horizontally and breaks cursor-column tracking). Deferred; logged as a
  weft TODO. Revisit only if textarea is replaced.
- Indenting a bullet's nested children together (Tab indents the current line
  only — YAGNI).
- Changing the completion-accept behavior of Tab while the strip is open.
- Any change to tinting, framing, save/dirty, marker-cycle/continuation logic,
  or open-at-top.

## Approach

### `syncViewport` (reposition fix)

```go
// syncViewport repositions the textarea viewport onto the cursor after an
// intercept mutates the buffer without routing a message through ta.Update
// (the only place the textarea calls repositionView). repositionMsg is the
// content-neutral message layout() already uses for the same purpose.
func (e *EditorView) syncViewport() { e.ta, _ = e.ta.Update(repositionMsg{}) }
```

Fold the call into `replaceCurrentLine` (so empty-bullet terminate, ctrl+t, and
the new Tab/Shift+Tab all get it) and add one explicit `e.syncViewport()` after
the continuation `InsertString("\n" + prefix)`.

Note: the textarea repositions using viewport content captured during the
previous `View()`, so the reposition takes effect on the next render — which is
exactly the real Bubble Tea loop (a `View()` every frame). Tests prime with a
`View()` call before the mutating `Update`, mirroring the existing
`TestEditorCompletion_CursorStaysVisibleAtBottom`.

### Tab / Shift+Tab indent

Pure helpers (in `edit_markdown.go`):

```go
// indentLine adds one 2-space indent level to the front of line.
func indentLine(line string) string { return "  " + line }

// dedentLine removes up to one 2-space indent level from the front of line,
// returning the new line and the number of leading spaces actually removed.
func dedentLine(line string) (string, int) {
	n := 0
	for n < 2 && n < len(line) && line[n] == ' ' {
		n++
	}
	return line[n:], n
}
```

In `EditorView.Update`'s main `switch msg.String()` (reached only when the strip
is closed — the completer-active block consumes `tab` to accept a candidate):

```go
	case "tab":
		before, after := e.cursorLineSplit()
		oldCol := len([]rune(before))
		e.replaceCurrentLine(indentLine(before+after), oldCol+2)
		e.refreshCompleter(false)
		return EditorResult{}, nil
	case "shift+tab":
		before, after := e.cursorLineSplit()
		oldCol := len([]rune(before))
		newLine, removed := dedentLine(before + after)
		newCol := oldCol - removed
		if newCol < 0 {
			newCol = 0
		}
		e.replaceCurrentLine(newLine, newCol)
		e.refreshCompleter(false)
		return EditorResult{}, nil
```

`replaceCurrentLine` already clamps the column to the new line length and (per
this spec) repositions the viewport. Indent operates on the current line
generally (any line, not only bullets) — predictable, and the graph is
bullet-heavy so the current line is normally a bullet.

## Components / boundaries
- `internal/views/edit_markdown.go` — `indentLine`, `dedentLine` (pure).
- `internal/views/editor.go` — `syncViewport`; `replaceCurrentLine` gains the
  reposition poke; `Update` gains `tab`/`shift+tab` cases and a `syncViewport`
  call on the continuation path.
- `internal/views/help.go`, `AGENTS.md` — docs.

## Error handling / edge cases
- **Shift+Tab on a line with 0–1 leading spaces** — removes what's there (0 or
  1); `newCol` floored at 0. No-op when already flush.
- **Tab/Shift+Tab on a non-bullet line** — still indents/dedents the line
  (general behavior); harmless.
- **Shift+Tab while the strip is open** — `shift+tab` isn't in the
  completer-active intercept set, so it reaches the main switch and de-indents;
  rare and harmless.
- **Reposition with no prior render** — `repositionView` uses last-render
  content; in the runtime a `View()` precedes every `Update`, so it always has
  content. Tests prime with `View()`.

## Testing
- **Pure (`edit_markdown_test.go`)**: `indentLine` (prepends 2 spaces);
  `dedentLine` (2-space line → removed 2; 1-space → removed 1; 0-space → removed
  0, unchanged).
- **Integration (`editor_view_test.go`)**:
  - Tab on `- foo` (cursor at end) → `Content()` is `  - foo\n…`; cursor shifted +2.
  - Shift+Tab on `  - foo` → back to `- foo\n…`.
  - Shift+Tab on `- foo` (no indent) → unchanged.
  - Reposition: a buffer taller than the viewport ending in a bullet; prime with
    `View()`, navigate to the bottom bullet, `Update(enter)`, then assert the new
    bullet line is visible in `View()` without any further keystroke.
- **Docs**: `tab`/`shift-tab` rows added to the help Edit section; regen
  `TestHelpGolden.golden` (only those rows change); AGENTS.md note.
- `go vet ./... && go test ./...` green before each commit.

## Deferred / future
- Hanging-indent of wrapped bullet text (finding #2) — see Out; logged as a weft
  TODO.
- Indenting a bullet subtree (children) with one Tab.
