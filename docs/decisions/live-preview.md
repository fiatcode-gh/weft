# The editor shows a live preview by default

The in-app editor (`e`) now draws the page the way the read view draws it. Only
the line or block under the cursor turns back into source; everything else
stays rendered, in the same rows and colours as the read view. `Ctrl+R`
switches to the source look from `editor-core.md` and back. The choice lasts
until weft quits and is never written to disk. The status line shows
`[edit · preview]` or `[edit · source]`.

Why: reading a page and then editing its raw markdown made every edit a jump
between two pictures of the same page. The goal is Obsidian-style editing that
stays inside the plain-markdown line: weft never writes structure into the file.

Out of scope: mouse, Vim mode, multiple cursors, editing in rendered rows
(text is always edited as source), persisting the mode.

## Decisions

- **Chunked rendering through the read view's pipeline.** The page is split
  into chunks of top-level blocks and each chunk goes through the same
  preprocess, Glamour and post-process steps as a full render, then the rows are
  cut out of the result. Rendering the whole page per keystroke was rejected:
  at 10000 lines a full parse costs about 36 ms for `preprocess` plus 18 ms for
  goldmark, and Glamour costs about 43 µs per source line. A keystroke now
  re-renders one chunk and the chunk scan is incremental (`Scanner`).
- **Byte-identical to the full render.** The rows of a chunked render equal the
  full render, checked by differential tests over the real fixture pages and
  generated documents, at several widths, under the default style,
  `WEFT_STYLE=dark`, `notty` and `NO_COLOR`, and for CRLF as well as LF.
- **The sandwich.** A chunk renders with context stubs: a prefix (a stub
  paragraph, or a stub bullet for a chunk that continues the previous list), a
  suffix stub when a list continues after it, and the link reference
  definitions of the whole document carried to it (re-serialised, earliest
  definition wins). Wiki-link and task-marker sentinel ids start at the count
  of each before the chunk, because a sentinel's width depends on its id's
  digits and Glamour wraps on that width; every chunk has its own sentinel id
  base. The document's head and trailing blank rows are trimmed, and each line
  carries its real terminator, since CRLF pages render differently from LF.
- **Chunk-start rules.** A chunk starts only where Glamour's output cannot
  depend on the previous block. Ordered lists, task lists, definition lists,
  HTML blocks, single paragraphs, setext pairs, empty bullets and anything after
  a lone `"\r"` stay in one chunk. Beyond the first plan, a block directly after
  a heading or a rule never starts a chunk, because Glamour merges the margin
  row after a rule or heading with the next block's margin.
- **Render cache key.** It carries an is-last-chunk flag and the
  chunk's line count besides the chunk's text, so a chunk that moved from last
  to middle, or changed height, is not served stale rows.
- **Row attribution and reveal units.** Each rendered row is attributed to a
  source line as Lead, Body or Trail, with blank lines paired by counting. A
  reveal unit is the set of lines whose rows cannot be told apart: a table, a
  fence, a quote, a `:LOGBOOK:` block, a query or embed block, a wrapped bullet.
  Lines Glamour joins into one flow reveal as a whole: a paragraph's soft-broken
  lines, a quote with lazy lines, a setext heading with its underline. A
  selection reveals every unit it touches.
- **Fallback to one unit.** If a chunk's row map diverges from the rendered rows
  or is not monotone, the whole chunk is one reveal unit: it reveals entirely
  and nothing is lost. About 3% of chunks on the fixture pages do this; nested
  fences, fences in list items and reference definitions are the usual causes.
- **The cursor's line keeps its screen row.** When the line just left changes
  height, the text above shifts instead of the cursor jumping. Same rule for
  `e`, `Esc` and `Ctrl+R`.
- **Movement is by source line.** Up and Down move one source line; wrapped
  bullets are one line of movement. In live mode `PgDn` at the end of the buffer
  lands on the last line that has a display row; `Down` still reaches a trailing
  empty line.
- **Find on rendered rows.** Matches are highlighted on the rows the user sees;
  jumping to a match reveals its unit.
- **Mode ownership.** The App owns the mode per run (`editorSource`), passes it
  to each new editor and reads it back on exit, so `Esc` then `e` shows the last
  look. `NewEditorView` takes the mode; `editorStartsInSource` lets tests start
  in source.
- **Both-look test harness.** The views suite runs in live preview by default.
  A child process with `WEFT_TEST_EDITOR_SOURCE` set runs it in source mode
  (`TestSuiteInSourceMode`). That child run is skipped under `-race`; CI runs it
  in the non-race `go test ./...` step. The differential and sweep tests sample
  their corpora (`sampleStride` 7, or 17 under `-race`; `sweepStride` 8, or 16)
  so the race suite fits CI.
- **Speed at 10000 lines.** Live open about 28 ms, a keystroke about 3 ms, a
  page down about 3.5 ms on average (cold moves 2.2 to 4.9 ms), a reveal about
  0.08 ms. Source-mode open grew from about 7 ms to about 17 ms, because the
  scanner now also tracks chunk state.

## Traps

- Glamour's task-item and HTML-block context quirks are why those never start
  a chunk: their output depends on the neighbouring block.
- CRLF renders differently from LF in the read view (34 of 282 real pages), so
  chunk text carries each line's real terminator. A lone `"\r"` makes the rest
  of the page one chunk.
- Unsplittable blocks re-render whole on a keystroke (linear in the block's
  size).
- Reference definitions whose label, destination or title contain wiki syntax
  are not carried to other chunks. A page with a Glamour context dependency
  outside the tested vocabulary would show a wrong row, never lose text.
- Under `-race` Glamour is about 14 times slower than the `raceFactor` of 8,
  so the timing tests remain the flakiest proof.
- Parked: row pairing counts rows, so a wrapped letterless row can be given to
  the wrong line and then stays drawn while the right line is revealed. Reveal
  units widen over shared tags but cannot see a letterless row.
