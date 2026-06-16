# weft

Bubble Tea TUI for browsing and editing a local Logseq graph. Recency-sorted
picker (with page-creation), ripgrep-backed search, backlinks, TODO dashboard.
Writes only via `internal/edit/`: the in-app editor (`e`) saves the displayed
buffer on `Ctrl+S` and creates a page's file lazily on first save; `E` hands the
file to `$EDITOR`. File creation for a new page is deferred until save, so
opening then discarding never touches disk. While editing, typing `[[` opens a
live fuzzy completion list of page names (`↑`/`↓` to choose, `Enter`/`Tab` to
insert `[[Page Name]]`, `Esc` to dismiss); an unmatched name offers a create row
that inserts a red link without writing to disk.

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
- `internal/edit/` — the single disk-writing surface in the project. Resolves
  `$VISUAL` / `$EDITOR` / `vi`, snapshots file mtime, and exposes
  `Resolve` / `EnsureFile` / `WriteFile` / `SnapshotMtime`. Invoked by the in-app editor (`e`,
  saves on `Ctrl+S`) and by `App.editCurrent` for `$EDITOR` handoff (`E`).
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

`WEFT_DEBUG=1 ./weft --graph …` mirrors Bubble Tea events to `./weft.log`.
The alt-screen swallows panics; tail the log to see what the model received.

## Versioning

`main.Version` is `dev` by default. `-ldflags "-X main.Version=…"` overrides it;
otherwise `runtime/debug.BuildInfo` fills it in for `go install` users.
