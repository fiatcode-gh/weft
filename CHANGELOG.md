# Changelog

All notable changes to peekseq are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and the project
adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- One-key linkify: in the backlinks panel's **unlinked references**, press `l` on a mention to wrap it as a `[[link]]` in its source file. A before/after preview confirms the change (`y` to write, `n`/`esc` to cancel); after writing, the graph reindexes and the panel refreshes so the reference moves from unlinked to linked. The matched text is preserved verbatim (`[[<matched>]]`), and the file is re-read and re-matched at write time so a file changed since the panel opened fails safely with an in-panel message.

### Fixed

- In-app editor: the `[[` completion strip is now a typing affordance only — moving the cursor onto an unclosed `[[` (with arrows, `Home`/`End`, or `PageUp`/`PageDown`), or opening a file that ends in one, no longer spuriously pops the strip open with a "＋ Create" row. Activation is gated on an actual edit; an already-open strip still updates and closes as you navigate.

## [1.3.0] - 2026-06-13

### Added

- Backlinks panel (`b`) now lists **unlinked references** — bare-text mentions of the page elsewhere in the graph that aren't yet `[[linked]]` — beneath the linked backlinks. Read-only; `enter` jumps to the mention.
- Jumping from a backlink now lands on the reference: linked backlinks focus the back-reference link, and unlinked references highlight the mention and scroll it into view.

## [1.2.0] - 2026-06-12

### Added

- In-app markdown editor: `e` edits the current page in a full-screen buffer
  (`Ctrl+S` saves, `esc` exits with an unsaved-changes prompt); `E` opens the
  page in `$EDITOR`. The picker can create a new page by name.
- In-app editor: live `[[` wiki-link completion — type `[[` to open a
  fuzzy page-name list, `↑`/`↓` to choose, `Enter`/`Tab` to insert
  `[[Page Name]]`, `Esc` to dismiss, and a create row for new (red) links.
  Completion won't fire inside an already-closed link, and the candidate
  strip is capped so it never pushes the edited line off-screen.

## [1.1.1] - 2026-06-09

### Fixed

- TODO dashboard deep-link now lands on the bullet's actual rendered row.
  It targets the open-todo by its document-order ordinal (matched between
  the index and the render pipeline) instead of the raw source line number,
  which mis-scrolled on pages reflowed by word-wrap, bullet re-indenting, or
  `:LOGBOOK:` / `{{query}}` stripping.
- Picker and search now accept multi-byte rune input (accented Latin, CJK,
  emoji) and delete by rune on backspace, instead of dropping non-ASCII keys
  and corrupting the trailing rune.
- Pressing `e` on a cold-start today's-journal page now rebinds the page view
  to the freshly-built index (previously only `a.idx` was updated, leaving the
  view on the stale index until the next navigation).
- Search match highlighting no longer colours the truncation `…` on long
  context rows.
- Relative-time hints in the picker now read "next year" / "in N years" for
  far-future journals instead of "in NN months".

### Changed

- Backlink-context extraction splits each page body once instead of rescanning
  from the top per wiki-link (O(1) per link rather than O(body·links)).

## [1.1.0] - 2026-06-08

### Added

- Hand off the current page to `$VISUAL` / `$EDITOR` / `vi` with the
  `e` key. mtime-gated reindex on return. The only disk-writing
  surface in the project is the new `internal/edit` package; the
  read-only invariant is otherwise unchanged.
- Pressing `e` on today's journal on a cold start creates the
  empty file and opens the editor — no need to first press `.`.
  The file is only created when the user actually wants to edit,
  not on every boot, so a cold start that just browses doesn't
  leave behind empty journal stubs.

## [1.0.1] - 2026-06-03

### Fixed

- Wiki links wrapped in backticks (markdown inline code) are now left as literal
  text instead of being preprocessed as links. `\`[[Foo]]\`` displays as the
  literal `[[Foo]]` (matching the markdown spec's "inline code is literal"
  rule) rather than as a styled wiki link. The fix is per-line: split the line
  on backticks, process only the even-indexed (literal) parts, rejoin. The
  pre-v1.0.0 behaviour treated backticked wiki links as ordinary links; this
  was a real but minor markdown-conformance bug that surfaced when users
  pasted a wiki link inside inline code.

## [1.0.0] - 2026-06-03

First stable release. Read-only Logseq TUI for browsing a local graph:
recency-sorted picker, ripgrep-backed search, backlinks, and a TODO
dashboard. Built on Bubble Tea with a Glamour rendering pipeline.

### Added

- Per-page render cache in `PageView` (mtime-invalidated) so history
  navigation does not re-run the full Glamour pipeline.
- Strip `{{query …}}` and `{{embed …}}` Logseq blocks from the rendered
  page (fence-aware, mirrors the existing `:LOGBOOK:` strip).
- `[[page#block]]` wiki-links resolve to `page` and the `#block` fragment
  is dropped with a stderr warning on lookup miss. A wiki link inside
  backticks (`\`[[Foo]]\``) is left literal — markdown inline code is not
  processed for other constructs.
- Case-insensitive page resolution via a fold-keyed index, matching
  Logseq's de facto behaviour. A `pages/` or `journals/` subdirectory
  emits a stderr warning instead of being silently dropped.
- Mid-session reindex failures surface as a transient status hint
  instead of replacing the working page with the error splash.
- TODO dashboard deep-links to the bullet's line via history restore.
- Help overlay golden test; coverage for `relativeTime`, `Warmup`, and
  `Help.SetSize/Update`.
- GitHub Actions CI: `go vet` + `go test -race ./...` + `go build` on push.
- MIT LICENSE; this CHANGELOG.

### Changed

- `docs/superpowers/` design and plan documents are now part of the v1
  release (no longer ignored).

## [0.4.0] - 2026-05-26

Internal structural cleanup: extracted a shared `Overlay` interface and
`listBox` base, moved ripgrep execution into `internal/search`, and
centralized theme styles. No behaviour change.

## [0.3.0] - 2026-05-25

History back/forward (`[`/`]`), today's journal jump (`.`), and previous /
next journal walk (`<`/`>`). New history unit-test suite.

## [0.2.0] - 2026-05-25

Help overlay with version footer, in-app help, page-edge keys
(`g`/`G`/`ctrl+d`/`ctrl-u`), and width-clamping for the status bar.

## [0.1.2] - 2026-05-25

Bug-fix release: do not push a duplicate history entry when `.` is
pressed on today's journal.

## [0.1.1] - 2026-05-25

Search `b` shortcut to toggle the backlinks overlay for the current
page from anywhere.

## [0.1.0] - 2026-05-25

Initial public release. Recency-sorted picker, ripgrep-backed search,
backlinks, and a TODO dashboard. Read-only — never writes to the graph.
