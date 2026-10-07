# Editor opens at the reading position

Pressing `e` on a scrolled page opens the in-app editor with the source line
behind the read view's top visible line on the editor's top row. A selected
link that is on screen wins over the top line. An unscrolled page with no
visible selected link still opens at the top of the file.

This supersedes the "open at top" decision in
`docs/superpowers/specs/2026-06-12-in-app-editor-design.md` (decision 3) and
`docs/superpowers/specs/2026-06-17-editor-markdown-editing-design.md`. Their
motivation, that the cursor and the viewport must agree on open, is kept.

Out of scope: the exit path (scroll restore already worked), re-centring the
read view after save, a per-page remembered cursor, and the `E` (`$EDITOR`)
handoff.

## Decisions

- **The row-to-line map lives in `internal/render`.** The view layer never
  re-derives wrapping, fences or stripped `:LOGBOOK:` / query blocks. Rejected:
  proportional estimation (ignores wrapping and stripped lines) and searching
  the source for the visible text (breaks on bullet glyphs, hanging indent and
  stripped blocks).
- **The map comes from a second Glamour render of a tagged copy**, aligned to
  the real render by visible row text. The real render is untouched, so page
  output stays byte-identical. Tagging the real render was tried and rejected:
  a private-use marker at line start broke list, heading and fence syntax; at
  line end it took a column and shifted wrapping; a zero-width U+2060 changed
  paragraph-join spacing.
- **The tag is `ESC` + the line number spelled in CSI-intermediate bytes + `z`.**
  It is zero-width for reflow and `x/ansi`, and none of its bytes start
  goldmark inline syntax. `ESC[n@` failed (reflow indent and padding only end
  an escape on a letter); `ESC[nz` failed (goldmark treats `[` as a link
  opener). Tags go after the last word character, never inside link
  destinations, autolinks or HTML, and never after a closing emphasis
  delimiter (that changes CommonMark flanking).
- **The map is computed lazily, only when `e` needs an anchor**
  (`render.SourceRows`, memoised per page load). Computing it on every render
  doubled Glamour time on every page load and search arrival for data read only
  on `e` (+95 ms on a 3000-bullet page).
- **The read view renders `TrimSpace(file)`, the editor edits the raw file.**
  `enterEditor` adds the number of trimmed leading lines to the anchor.
- **The anchor goes on the editor's top row.** The bubbles v1.0.0 textarea
  scrolls only as far as needed to show the cursor, so the anchor first landed
  on the bottom row and the editor showed the screen above the reading
  position. *Superseded by `editor-core.md`: the anchor line now keeps the
  screen row it had in the read view, in both directions.*

## Traps

- The textarea's viewport has no lines until its first `View()`, and
  `repositionView` cannot scroll an empty viewport. Construction-time scrolling
  needs a render first. *No longer applies: the textarea is gone
  (`editor-core.md`).*
- Unusual markdown (reference-label line endings, raw HTML blocks, wrapped
  table cells) can map a few rows to a neighbouring line. Alignment keeps it in
  range; it never panics.
