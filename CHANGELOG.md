# Changelog

All notable changes to weft are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and the project
adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- Tagged releases now publish prebuilt `weft` binaries for Linux and macOS (amd64 and arm64) on [GitHub Releases](https://github.com/fiatcode-gh/weft/releases), with a `checksums.txt`. The install instructions point there, since `go install` from the old `git.fiatcode.dev` module path can no longer fetch new versions.

### Fixed

- Creating a page or linkifying a mention while a sync is running now says "sync in progress" inside the open panel, instead of an invisible status-bar hint that made the keypress look dead.
- Index warnings (skipped subdirectories, ambiguous page names, unreadable pages) now surface as a status-bar hint pointing at `weft.log`, instead of going to stderr where the alt-screen hides them. A page whose modification time can't be read is reported too, instead of silently sinking to the bottom of the picker. An unchanged warning set no longer re-nags (hint and log entry) on every later reindex — only a new or newly-cleared set does.
- A markdown render that falls back to plain text (Glamour failure) is no longer cached, so a transient failure can't stick as an unstyled page; the cause is logged to weft.log.
- WEFT_DEBUG=1 now exits with a clear error when the debug log can't be opened, instead of silently running without the logging you asked for.
- Bullets with long task markers (WAITING, CANCELLED) no longer overflow the wrap width by a few columns, which caused stray soft-wrapped lines and slightly-off dashboard deep-link centering.
- Linkifying a mention (and creating today's journal) no longer resets the page to the top — scroll position and link cursor survive the rebuild.

## [2.4.0] - 2026-07-24

### Added

- The markdown page body now renders using your terminal's own color palette instead of a fixed built-in theme. Headings, links, inline code, blockquotes, and syntax-highlighted code fences all follow your terminal's 16 colors, so the read view matches the rest of your terminal. Set `WEFT_STYLE=<name>` to force a specific built-in Glamour theme (for example `dark` or `dracula`), or `NO_COLOR` to disable color entirely.

## [2.3.0] - 2026-07-15

### Added

- Markdown links now render as clean styled text, with the inline URL hidden in the read view. The full URL stays in the source file (visible when you edit); images and links inside code stay literal.

### Fixed

- Case-variant wiki-links now resolve: `[[alpha]]` pointing at a page named `Alpha` shows up in backlinks and unlinked references.
- `~~~` (tilde) fences and Logseq bullet-prefixed fences are recognized as code fences everywhere — parsing, backlinks, render, and editor tint — matching triple-backtick behavior.
- Inline-code spans are excluded from unlinked-ref detection and one-key linkify, so weft no longer writes a dead link into a code span.
- A save made while a background sync is mid-`pull --rebase` is no longer silently lost — graph-mutating actions wait for the sync to finish.
- Creating today's journal stub is now exclusive, so a file materialized by a concurrent `git pull` can't be truncated to empty.
- `ctrl+c` quits weft even while an overlay is open.
- Scroll and cursor position are preserved across a reindex and across exiting the editor; navigating to a page resets scroll to the top.
- Search keeps `#` in wiki-link targets, strips a trailing carriage return from CRLF match context, and surfaces scanner errors instead of silently truncating results.
- The debug log is written to the user cache directory instead of the working directory (where a sync could commit it into your graph), and `WEFT_DEBUG=0` now means off.

## [2.2.3] - 2026-06-18

### Fixed

- A failed save no longer hides its error. When you choose "save" at the exit prompt and the write fails, the editor stays open and shows the error instead of silently re-displaying the save prompt.
- Search now tells "no matches" apart from "not searched yet": a query that returns nothing reads as "no matches", and pressing Enter on it no longer re-runs the identical search.
- Sync refuses to run while a rebase or merge is in progress instead of staging and committing conflict-marked files — resolve it in a shell, then sync again.
- Sync can no longer hang indefinitely: each git command now has a timeout, so a stalled `git push` (a dead network or a credential prompt) can't wedge syncing forever.
- Case-insensitive page-name collisions (for example `Alpha.md` alongside `alpha.md`) now resolve deterministically to the first page found, with a warning on stderr, rather than depending on the order files are read.
- A `git status` failure during sync is now reported as the "status" stage rather than mislabeled as "add".

### Changed

- The picker now offers a `＋ Create` row for any name that doesn't match an existing page, even when that name is a fuzzy substring of one — so you can create "Notes" while "Nested Notes Archive" already exists.

## [2.2.2] - 2026-06-18

### Changed

- Leaner index build (startup and `R` rebuild): each page is now parsed in a single pass instead of re-splitting and re-scanning its text once per extractor, and backlink context lines no longer hold a reference to the entire page body — so a rebuild does less work and the in-memory index retains less.

## [2.2.1] - 2026-06-18

### Fixed

- Saving a page now writes atomically. The in-app editor's save and the one-key linkify (which writes to *other* pages' files) now write to a temporary file that is renamed into place, so a crash mid-write can no longer leave a page truncated or half-written. A page's existing file permissions are preserved across a save.

## [2.2.0] - 2026-06-17

### Added

- In-app git sync: press `S` to commit local changes, `pull --rebase`, then push the graph without leaving weft. It runs asynchronously with progress in the status bar (`⟳ syncing…` → `✓ synced`, or `✗ <stage> failed — see weft.log`). Conflicts are left for you to resolve in a shell — weft never edits a conflicted tree. Operates on the `--graph` directory and degrades to a status-bar hint if that directory isn't a git repository.
- Status-bar sync indicator: a `●` appears whenever the graph has uncommitted changes or unpushed commits, so you can tell at a glance when a sync is due. It refreshes after edits, reindexes, and syncs — no background polling.

### Fixed

- Help overlay (`?`) no longer overflows the terminal: it lays its key groups out in two balanced columns when the width allows, and caps its height to the screen — dropping overflow with a "resize" hint — instead of silently clipping the top and bottom (including the close hint) on shorter terminals.

## [2.1.0] - 2026-06-17

### Added

- In-app editor syntax tinting: the raw markdown buffer is now live-tinted to match the read view — headings bold, blockquotes and code-fence delimiters faint, task markers (`TODO`/`DONE`/…) and `[[wiki-links]]` colored as on the page. The line the cursor is on is left as raw source.
- In-app editor framing parity: the editor text now occupies the same horizontal box as the read view (a 2-column left inset) and gains a 1-row top margin, so text no longer jumps left or up when switching from reading to editing.
- In-app editor bullet editing: `Enter` on a `- ` bullet continues the list at the same indent (an empty bullet ends the list); `Ctrl+T` cycles the current bullet's workflow marker (plain → `TODO` → `DONE`); `Tab` / `Shift+Tab` indent / de-indent the current line by one 2-space level.
- The in-app editor now opens with the cursor at the top of the page.

### Fixed

- In-app editor: the viewport now follows the cursor after a bullet continuation, marker toggle, or indent — previously a new bullet created at the bottom of a long page stayed off-screen until the next keystroke.

## [2.0.0] - 2026-06-16

### Changed

- **Renamed the project from `peekseq` to `weft`.** This is a breaking change for
  installed users: the binary, the install path, and the environment variables
  all change.
  - Install path is now `go install git.fiatcode.dev/fiatcode/weft/v2/cmd/weft@latest`
    (the module path carries the `/v2` major-version suffix required by Go for v2+).
  - `PEEKSEQ_GRAPH` → `WEFT_GRAPH`, `PEEKSEQ_DEBUG` → `WEFT_DEBUG`,
    `PEEKSEQ_STYLE` → `WEFT_STYLE` (no backward-compatible fallback).
  - Debug log file is now `weft.log`.
  - The on-disk Logseq graph format is unchanged.

## [1.4.0] - 2026-06-16

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
