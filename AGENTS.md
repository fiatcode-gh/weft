# peekseq

Bubble Tea TUI for browsing a local Logseq graph. Recency-sorted picker,
ripgrep-backed search, backlinks, TODO dashboard. Writes only via `internal/edit/`, only in response to the `e` key
(or the `.` key, when navigating to today's journal on a cold start), and only to
the file currently displayed on the page view. The `e` key on a missing
today's-journal file creates the empty stub on demand; a cold start that
doesn't press `e` (or `.`) doesn't create any file.

## Commands

```bash
go build ./cmd/peekseq                                # build binary at ./peekseq
go test ./...                                         # full test suite
go test ./internal/views -run TestPickerFiltersOnQuery  # single test
go test ./... -update                                 # regenerate teatest goldens
go vet ./...                                          # vet
go run ./cmd/peekseq --graph testdata/fixture-graph   # run against fixture
./peekseq -version                                    # print version and exit
```

Run `go vet ./... && go test ./...` before every commit — both must pass.

`teatest.RequireEqualOutput` compares against `internal/views/testdata/*.golden`. After
an intentional UI change, run with `-update` and visually diff the golden before staging.

## Layout

- `cmd/peekseq/` — entry point, flag/env wiring, ripgrep preflight, Bubble Tea boot.
- `internal/graph/` — filesystem walk, page parsing, name resolution, index.
- `internal/render/` — Glamour-based page rendering (wiki-link styling, hanging-indent,
  workflow-marker colouring, `:LOGBOOK:` stripping). Has a `Warmup()` paid before the
  TUI takes the screen to avoid chroma init flicker.
- `internal/edit/` — the single disk-writing surface in the project. Resolves
  `$VISUAL` / `$EDITOR` / `vi`, snapshots file mtime, and exposes
  `Resolve` / `EnsureFile` / `SnapshotMtime`. Invoked only by `App.editCurrent`
  in response to the `e` key.
- `internal/views/` — Bubble Tea models: `app` (root), `page`, `picker`, `search`,
  `backlinks`, `todos`, `help`.

## External deps

- **ripgrep (`rg`)** must be on PATH. `cmd/peekseq/main.go` exits 2 if missing.
- Go 1.26+ (see `go.mod`).

## Testing

- Tests use the checked-in `testdata/fixture-graph/`. **Never point tests at the
  real graph** (`~/Documents/fiat-codex`) — it's mutable and will make tests flaky.
- View tests use `teatest` (`charmbracelet/x/exp/teatest`) for golden-ish frame
  assertions; golden files live under `internal/views/testdata/`.
- `testdata/fake-editor.sh` is a POSIX shell script that stands in for a real
  editor in the App integration tests for the `e` key.

## Debugging the TUI

`PEEKSEQ_DEBUG=1 ./peekseq --graph …` mirrors Bubble Tea events to `./peekseq.log`.
The alt-screen swallows panics; tail the log to see what the model received.

## Versioning

`main.Version` is `dev` by default. `-ldflags "-X main.Version=…"` overrides it;
otherwise `runtime/debug.BuildInfo` fills it in for `go install` users.
