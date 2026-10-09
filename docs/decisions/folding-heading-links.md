# The read view has a row cursor and folds; `[[Page#Heading]]` opens at the heading

The read view moves a row cursor with `j`/`k`, `Ctrl+D`/`Ctrl+U` and `g`/`G`.
`Tab` folds the heading section or list item under the cursor, and `z` cycles
the whole page through top-level bullets folded, headings only, and all shown.
A folded row ends with a faint `▸ N lines`. Folds are view-only: they last for
the session and nothing is written to the file. `[[Page#Heading]]` (and
`#[[Page#Heading]]`) opens the page with that heading at the top, counts as a
backlink of the page, and is not unresolved in `weft doctor`. `e` opens the
editor on the cursor row's line.

Why: long pages had no way to collapse what you were not reading, no way to
link to a part of a page, and the read view had no notion of "the line I am
on" — `e` guessed from the top visible line or the selected link. Files stay
plain markdown. weft is a navigator, not an outliner: no block IDs, no saved
fold state, no zoom, no embeds.

Out of scope: folding in the editor or live preview, persisted folds, same-page
`[[#Heading]]`, slugs or anchors, a table of contents, rename rewriting, and a
faster row map.

## Decisions

- **Row cursor.** `PageView` owns `top` (first visible row) and `row` (the
  cursor, a visible-row index). The cursor rests only on non-blank rows; `j`/`k`
  step to the next or previous non-blank row, `Ctrl+D`/`Ctrl+U` move cursor and
  top by half the window, `g`/`G` go to the first and last non-blank row. The
  view scrolls the minimum to keep the cursor on screen. The link cursor
  (`Cursor()`) stays a separate selection: `n`/`N` cycle it and put the row
  cursor on its row, `Enter` follows it.
- **Window-only drawing.** The viewport is fed only the rows on screen each
  frame (`SetContentLines`), not the whole page, so a frame costs the window
  rather than the page. Unfolded rows away from the cursor keep their rendered
  bytes. The cursor row is its plain text padded to the page width in `styleSel`
  (`Bold+Reverse` under `NO_COLOR`); a selected link on it keeps `cursorStyle`.
- **`e` and `Esc`.** `e` anchors on the cursor row's line (the first non-blank
  row of its own at or after the cursor, else before) and keeps its screen row.
  `Esc` puts the editor's line on its screen row with the cursor on it. When the
  exit anchor equals the open anchor and the body is unchanged, the read view
  restores the exact `(top, row)` it had at `e`, so the round trip is
  byte-identical.
- **One heading definition.** `graph.Headings(body)` is used by both link
  resolution and the fold outline. It mirrors `render.Scanner`'s classification
  line for line (graph cannot import render); `internal/render/heading_parity_test.go`
  pins the two together. `graph.HeadingKey` is the key for both a heading and a
  link fragment: the lower-cased display text, with wiki links shown by their
  display text, markdown links by their text, `` ` `` `*` `**` `__` `~~` and
  word-edge `_` removed, whitespace collapsed and trimmed.
- **Fold model.** Owners are the entries of `render.Outline(body)`: headings
  with a non-blank section, and list items (bullet or ordered) with non-blank
  children. An owner hides source lines `[Start, End)`, and the marker count is
  `End-Start`. A row is hidden when its `SourceRows` line is hidden. Both the
  outline and the row map come from the source text, so a fold never re-renders
  the page.
- **Fold lifetime.** `foldStore` on `App`, keyed by `PageMeta.Path`, holds the
  body the entry applies to, the folded owner lines, the `z` level and the
  plain render's row-map memo. A load whose body differs from the stored body
  drops the entry, which covers save, `x`, capture and an outside edit. It lives
  on `App` because `NewPageView` is rebuilt on navigation, history and reindex.
  Nothing is written.
- **Jumps unfold their target.** Task, backlink focus, heading link, find
  highlight and the editor's `Esc` (`PlaceAnchor`) remove every fold whose
  range holds the target line before placing. `n`/`N` skip hidden links, and a
  fold that hides the selected link deselects it.
- **Search opens at the top, so nothing to unfold.** A search hit opens the page
  at its first row as before; no landing on the hit line was added, so a fold
  can never hide the hit. A search hit keeps existing folds and lands on the
  first row.
- **Heading-link rule.** `Index.ResolveLink` first tries `Resolve(target)`, so a
  page literally named `Page#Heading` wins. Otherwise it splits at the last `#`;
  the page part must be non-empty and resolve, the fragment must be non-blank,
  and the page must have a heading with that `HeadingKey`. The first matching
  heading is the destination. `[[#x]]`, `[[C#]]` and `[[Lab #inner]]` keep
  their old meaning. A heading that has gone when the page opens leaves the page
  at the top with the hint `heading not found: …`.
- **Two-phase index keying.** `BuildIndex` parses every page first (headings,
  links, todos) and keys links afterwards, because a link may point at a page
  not yet walked. A link that resolves only as a heading is keyed under its
  page, so `BacklinksTo`, the backlinks panel, orphan and unresolved counts
  follow without other changes. The walk order of refs is kept.
- **Performance.** Measured at the start of the unit on a 10000-line page,
  width 100, height 40 (the page of headings is one `##` every 200 lines and one
  `###` every 50):

  | Page | Render (open) | `render.SourceRows` | rows | `View()` per key before |
  |------|---------------|---------------------|------|--------------------------|
  | 2500 mixed | 104 ms | 167 ms | 3004 | 5.0 ms |
  | 10000 mixed | 445 ms | 749 ms | 12004 | — |
  | 10000 headings | 842 ms | 1585 ms | 14004 | 19.2 ms (18.0 ms in `viewport.SetContent`'s width scan) |

  Benchmarks after the change (`go test ./internal/views -run '^$' -bench 'Read|Fold' -benchtime 5x`,
  AMD Ryzen 7 8845HS):

  | Benchmark | Time per op |
  |-----------|-------------|
  | `BenchmarkReadKey10000` (one key + `View()`) | 0.36 ms |
  | `BenchmarkReadOpen10000` | 427 ms |
  | `BenchmarkFoldFirst10000` (first `Tab`/`z`, builds the row map) | 955 ms |
  | `BenchmarkFoldToggle10000` | 0.04 ms |
  | `BenchmarkFoldLevel10000` | 0.47 ms |

  Bounds enforced by tests: cursor and scroll keys never build the row map; one
  key plus `View()` on a 10000-line page stays within `perfReadKeyMS`; opening
  a page without folds stays within 1.1× its render plus 20 ms; the first
  `Tab`/`z` within 1.1× a fresh `render.SourceRows` plus 50 ms (no worse than
  `e` was); later `Tab`/`z` within `perfFoldMS`; the row map is computed once
  per (page body, width) per session across navigation, history, reindex and
  editor round trips. Known cost kept: `e` with the cursor off the first row of
  a 10000-line page pays the row map once (about 0.75–1.6 s), as `e` on a
  scrolled page already did; after that, repeat `e`, `Tab`, `z` and `Esc` are
  fast.

## Rejected

- **Re-rendering a folded copy of the source.** It would change the bytes of the
  unfolded page and cost a render on every `Tab`. Hiding rows by their source
  line from one render is exact and cheap.
- **Saved fold state.** It writes structure metadata into the file or beside it,
  against the navigator rule.
- **Slugs for headings.** Two spellings of one heading would then differ from
  what the reader sees; matching the displayed text needs no new syntax.
- **Searching the styled text for headings.** Styling, wrapping and escapes make
  that fragile; headings come from the source via `graph.Headings`.

## Traps

- `- ## x` is a bullet with the text `## x`, not a heading; `graph.Headings` and
  `render.Scanner` both treat it so.
- `render.SourceRows` can diverge from the rendered rows in rare layouts (its
  `alignRows` middle section), so a fold can hide a neighbouring row. A row
  with no source line is never hidden.
- Emphasis renders (`SetPageEmphasizing`) build their own row map for that load
  and do not use or fill the memo in the fold store.
- A fold on a page with no row map (rendering failed to produce one) answers
  `cannot fold this page` and keeps the folds it has.
- A link written `[[Page#Heading]]` on a page where `Page` has no such
  heading is unresolved, exactly as a missing page is.
