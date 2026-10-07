# The in-app editor runs on a weft-owned buffer

The in-app editor (`e`) no longer uses the Bubbles `textarea`. It edits an
`internal/buffer.Buffer` and draws its rows with the read view's own layout and
styles. It has undo and redo, selection, copy, cut and paste (with OSC 52),
find and replace, and bullet moves that carry children. It opens any markdown
file and saves it back byte for byte apart from the user's edits. Switching
between reading and editing keeps text in place, with the same colours.

Why: the textarea had no undo, selection or in-page find, refused files with
tabs, CRLF or more than 10000 lines, and under Bubbles v2 made typing on long
pages about ten times slower. Every editor workaround in the codebase existed
because of it. The goal is an editor that can replace nvim for note work.

Out of scope: live preview (rendering rows while editing), mouse, Vim mode,
kitty keyboard enhancements, multiple cursors, reading the system clipboard on
`Ctrl+V`, reloading while editing.

## Decisions

- **Buffer model.** Lines are stored without terminators, plus a per-line CRLF
  flag, so every byte string loads and saves unchanged: CRLF, mixed endings,
  tabs, a missing final newline, invalid UTF-8. New line breaks use the file's
  dominant ending. Positions are byte offsets; the cursor moves by grapheme
  cluster. There is no line cap.
- **Undo grouping.** A run of typing is one step. Every other edit (delete,
  paste, cut, completion, marker cycle, indent, bullet move, replace-all) is
  exactly one step, and so are a merge-on-save and a reload from the clash
  prompt. History survives saves and ends when the editor closes.
- **One style definition.** `render.Theme` holds the Glamour style config, the
  chroma formatter and the link and marker overlays. The read view and the
  editor both read it, so their colours cannot drift. The read view's bytes
  under the default style and named styles were pinned by goldens before the
  refactor and did not change.
- **Source rows are laid out like the read view.** `internal/render` has a block
  scanner, row geometry that wraps by aligning `ansi.Wrap` output back to source
  bytes (hanging indent for bullets, quotes, code), and a painter that copies
  Glamour's style cascade, using goldmark for inline structure and chroma for
  code fences. Syntax the read view hides is dimmed; that is the only colour
  the editor adds. All of this sits behind a `SourceLines` interface, so live
  preview can reuse it without touching editing code.
- **Equal windows.** The editor draws the read view's faint rule row above its
  status line, so both text windows are the same height and the bottom rows
  line up.
- **Real terminal cursor.** The editor sets Bubble Tea's cursor instead of
  drawing one, so it stays visible under `NO_COLOR`.
- **`e` and `Esc` keep the screen row.** `e` reads an anchor (line, row within
  the line, screen row) from the read view's row map. `Esc` maps the cursor's
  line into the file on disk through `merge.Text` and places it on the same
  screen row. An exit with no change reuses the page on screen and its row map,
  so it renders nothing. Rejected: rendering only up to the cursor line (not
  exact, since list tightness and reference definitions depend on later lines)
  and reusing the read view's cached Glamour output for the row map (keeps a
  second copy of every render and changes read-view internals).
- **`NO_COLOR` keeps attributes.** It uses the `Ascii` colour profile and an
  attribute-only copy of the plain style, so bold, italic, underline,
  strikethrough, faint and reverse stay in every screen. Selection, find
  matches and the read view's selected link remain visible. `TERM=dumb` gives
  fully unstyled output.
- **Legacy keys only.** Every binding has a normal terminal encoding, since
  kitty keyboard and modifyOtherKeys stay filtered off (see `bubbletea-v2.md`).
  `Ctrl+C` copies and never leaves the editor; `Ctrl+Y` redoes because
  `Ctrl+Shift+Z` has no legacy encoding.
- **Copy and cut with nothing selected act on the whole line**; pasting such a
  line inserts it above the cursor's line. The copy register belongs to the
  App, so a copy survives closing the editor.

## Traps

- Glamour registers its chroma style `"charm"` once per process and the first
  registration wins. Tests that change the style run in a fresh process
  (`inFreshProcess`).
- With a named `WEFT_STYLE`, the read view loses the body colour on text right
  after a link or workflow marker, because the restored sentinel ends in a
  reset. The editor colours that text correctly; parity tests sample other body
  text. Left as is.
- Nested bullets sit at the read view's columns only when the source indents by
  the style's level indent (2 by default, 4 under `NO_COLOR`); the row text
  always matches.
- Glamour wraps a long quote twice, so a quote row exactly at the inner width
  loses its last word to an extra row at the margin. The editor reproduces that.
- `Esc` after saving a 10000-line page still costs a re-render plus a row map,
  about 2 s.
- `Ctrl+V` in the picker and search fields still goes through Bubbles
  `textinput`, which can read the system clipboard.
