# `#tag` and `#[[Multi Word]]` are links to a page

A tag is a link to the page of that name, like `[[Page]]`. The read view draws
it as a link with the `#` kept, `n`/`N` and `Enter` follow it, backlinks and
`weft doctor` count it, and unlinked-mention search skips it. In the editor,
typing `#` and a letter offers the same page-name completion as `[[`.

Why: journals written with `#kitchen` style tags were plain text to weft, so a
page's backlinks missed them. Tags stay a link, not a new kind of thing: weft
stores no tag list and writes nothing into the file.

Out of scope: a `tags::` property, a separate tag list or tag colour, `#`
inside `[[…]]` (still part of the page name), tag rename (unit 10), editing
aids beyond completion. A tag's page is an ordinary page and can hold notes.

## Decisions

- **One grammar, one definition.** A tag starts at the line start or after
  whitespace or `(`. The simple form `#name` begins with a letter and continues
  with letters, digits, `-`, `_` and `/`; any other name needs the bracket form
  `#[[Name]]`. A simple name shaped like a hex colour (3, 4, 6 or 8 hex digits
  with at least one decimal digit, as `#FAF3E7`) is not a tag. Never a tag: issue numbers (`#18`, `PR #5`),
  headings, `#+…` and `#!` lines, text inside `[[…]]`, URL fragments, code spans
  and fences. `internal/graph/tag.go` (`FindTags`, `TagStartAt`,
  `IsSimpleTagName`) is the one definition; the index, the read view, the
  editor's source painter, live preview, unlinked mentions, linkify and
  completion all call it, so they cannot disagree.
- **A markdown destination never holds a tag.** A `#` inside a markdown link
  destination (`graph.MarkdownLinkRe` group 3) does not start a tag. The read
  view rewrites destinations to `#` before it substitutes tags, so the index must
  skip the same text or it would count a tag the page never shows. This also
  covers URL fragments.
- **Tags travel as wiki-link sentinels.** The display keeps the `#`; for the
  bracket form the sentinel carries `#` plus the alias, or the target when there is none. So
  `Result.Links`, the link cursor and the live preview's per-chunk id base need
  no new kind. Measured cost: on a 10000-line page, typing and paging are
  unchanged and opening is within 10% of the same number of `[[…]]` links.
- **Completion precedence.** `[[` wins. Otherwise `#` plus a letter opens the
  list when it sits at a tag start, the cursor is at the end of the name, and
  the line is one the read view substitutes. Accepting inserts `#name`, or
  `#[[Name]]` when the name is not a simple tag name. The create row writes
  nothing to disk, as for `[[`.
- **Rejected: a separate tag kind.** It would need its own cursor stop, its own
  backlink section and its own sentinel id range for no reader-visible gain.

## Traps

- Link reference definitions (`[r]: #x`) and markdown link destinations that
  contain a backtick can make the index and the read view disagree. Wiki links
  have the same limits.
- The name rule is literal: a trailing `-`, `_` or `/` stays in the name
  (`#todo-` links to `todo-`).
