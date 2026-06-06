# Changelog

All notable changes to peekseq are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and the project
adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- Hand off the current page to `$VISUAL` / `$EDITOR` / `vi` with the
  `e` key. mtime-gated reindex on return. The only disk-writing
  surface in the project is the new `internal/edit` package; the
  read-only invariant is otherwise unchanged.
- Pressing `.` on a missing today's-journal file creates an empty
  journal and lands you on it, so `e` is reachable on a cold start
  without leaving the TUI.

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
