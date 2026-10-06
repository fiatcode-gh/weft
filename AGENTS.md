# weft

Bubble Tea TUI for a daily-journal-driven markdown knowledge base — browse,
search, and edit a local graph of flat `.md` pages and journals linked by
`[[wiki-links]]`. Recency-sorted
picker (with page-creation), ripgrep-backed search, backlinks, TODO dashboard.
Within weft, graph files are written only through `internal/edit/`: the in-app
editor (`e`) saves the displayed
buffer on `Ctrl+S` (the save compares the disk to the content the editor opened
or last saved, by content; it three-way-merges non-overlapping changes using the
pure `internal/merge`, otherwise shows an overwrite/reload/keep-editing prompt;
linkify refuses to write a file that changed under it) and creates a page's file lazily on first save; `E` hands the
file to `$EDITOR`. File creation for a new page is deferred until save, so
opening then discarding never touches disk. `e` refuses to open a file whose
content the in-app textarea would alter on load (CRLF line endings, tabs, or
more than 10000 lines), showing a status-bar hint pointing at `E`/`$EDITOR`
instead. While editing, typing `[[` opens a
live fuzzy completion list of page names (`↑`/`↓` to choose, `Enter`/`Tab` to
insert `[[Page Name]]`, `Esc` to dismiss); an unmatched name offers a create row
that inserts a link to the not-yet-created page without writing to disk. The in-app editor live-tints
markdown (headings, blockquotes, code-fence delimiters, task markers, and
`[[wiki-links]]`) and insets text to match the read view's left margin; the
cursor's current row is shown as raw source.
Enter continues a `- ` bullet at the same indent (empty bullet ends the list);
`Ctrl+T` cycles the current bullet's workflow marker (plain → TODO → DONE);
`Tab` / `Shift+Tab` indent / de-indent the current line by one 2-space level.
The editor opens with the cursor on the source line matching the read view's top visible line (or the visible link cursor, when one is set).

Press `S` to sync the graph to git — commit local changes, `pull --rebase`,
then push — run asynchronously off the UI thread with the outcome in the status
bar (`⟳ syncing…` → `✓ synced`, or `✗ <stage> failed — see weft.log`, with the
status-bar hint showing the resolved absolute path under the user cache dir). A `●` in
the status bar flags an unsynced graph (uncommitted changes or unpushed
commits). `internal/sync/` is the project's second deliberate disk-mutating
surface, alongside `internal/edit/`; it shells out to the system `git` and bails
to the shell on conflicts rather than resolving them in the alt-screen.

## Commands

```bash
go build ./cmd/weft                                     # build binary at ./weft
go test ./...                                           # full test suite
go test ./internal/views -run TestPickerFiltersOnQuery  # single test
go test ./... -update                                   # regenerate teatest goldens
go vet ./...                                            # vet
go run ./cmd/weft --graph testdata/fixture-graph        # run against fixture
./weft -version                                         # print version and exit
```

Run `go vet ./... && go test ./...` before every commit — both must pass.

`teatest.RequireEqualOutput` compares against `internal/views/testdata/*.golden`. After
an intentional UI change, run with `-update` and visually diff the golden before staging.

## Layout

- `cmd/weft/` — entry point, flag/env wiring, ripgrep preflight, Bubble Tea boot.
- `internal/graph/` — filesystem walk, page parsing, name resolution, index.
- `internal/render/` — Glamour-based page rendering (wiki-link styling, hanging-indent,
  workflow-marker colouring, `:LOGBOOK:` stripping). Has a `Warmup()` paid before the
  TUI takes the screen to avoid chroma init flicker.
- `internal/edit/` — one of two deliberate disk-mutating surfaces (with
  `internal/sync/`). Resolves
  `$VISUAL` / `$EDITOR` / `vi`, snapshots file mtime, and exposes
  `Resolve` / `EnsureFile` / `ReadSnapshot` / `WriteFileIfUnchanged` / `SnapshotMtime`.
  Every replacing write is content-guarded: `writeFile` is unexported. Invoked by the in-app editor (`e`,
  saves on `Ctrl+S`) and by `App.editCurrent` for `$EDITOR` handoff (`E`).
- `internal/merge/` — pure git-style three-way line merge (adjacent changes
  conflict; both sides inserting at the same spot keeps mine then theirs) used by
  the editor save. No disk access.
- `internal/sync/` — git orchestration over `os/exec` (no `go-git`); the second
  deliberate disk-mutating surface. `Run` does commit → `pull --rebase` → push
  for the `S` keybind (async, reported via a status-bar hint, failure output
  appended to `weft.log` under the user cache dir, per `internal/views/logpath.go`);
  the read-only `Status` reports whether the work tree
  is dirty or ahead of upstream, driving the status-bar `●` indicator. Never
  edits a conflicted tree — conflicts bail to the shell.
- `internal/views/` — Bubble Tea models: `app` (root), `page`, `picker`, `search`,
  `backlinks`, `todos`, `help`, `editor` (in-app markdown editor). The backlinks
  view (`b`) shows two sections: linked references (`[[…]]` mentions) then unlinked
  references (bare-text mentions not yet wiki-linked); read-only, `enter` jumps to the
  mention's page and lands on the reference: linked backlinks focus the back-reference
  link (cursor + scroll), unlinked references highlight the mention and scroll it into view.

## External deps

- **ripgrep (`rg`)** must be on PATH. `cmd/weft/main.go` exits 2 if missing.
- Go 1.26+ (see `go.mod`).

## Testing

- Tests use the checked-in `testdata/fixture-graph/`. **Never point tests at the
  real graph** (`~/Documents/fiat-codex`) — it's mutable and will make tests flaky.
- View tests use `teatest` (`charmbracelet/x/exp/teatest`) for golden-ish frame
  assertions; golden files live under `internal/views/testdata/`.
- `testdata/fake-editor.sh` is a POSIX shell script that stands in for a real
  editor in the App integration tests for the `E` key (`$EDITOR` handoff).

## Debugging the TUI

`WEFT_DEBUG=1 ./weft --graph …` mirrors Bubble Tea events to `weft.log` under
the user cache dir (`internal/views.DebugLogPath()`; falls back to `./weft.log`
only if the cache dir is unavailable). The alt-screen swallows panics; tail the
log to see what the model received.

## Versioning

`main.Version` is `dev` by default. `-ldflags "-X main.Version=…"` overrides it;
otherwise `runtime/debug.BuildInfo` fills it in for `go install` users.

Pushing a `vX.Y.Z` tag runs `.github/workflows/release.yml`: it vets and tests,
then GoReleaser (`.goreleaser.yaml`) builds linux/darwin amd64/arm64 binaries
with `main.Version` set to the tag and publishes a GitHub Release. The release
notes are the tag's `## [X.Y.Z]` section of `CHANGELOG.md`; the workflow fails
before publishing when that section is missing, so rename `## [Unreleased]`
before tagging.
