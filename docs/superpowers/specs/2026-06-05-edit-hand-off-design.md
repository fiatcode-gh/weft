# Editor Hand-off (`e`) — Design

**Status:** approved, ready for plan
**Date:** 2026-06-05
**Repo:** `~/Development/Projects/fiatcode/peekseq`

## Goal

Add an `e` key in the page view that hands the current page's `.md` file off
to the user's editor, then resumes the TUI and reindexes the graph if (and
only if) the file changed.

Closes the "peekseq is great for browsing but I have to leave it to write
anything" gap without making peekseq an editor. The user's `$EDITOR` (or
`$VISUAL`, falling back to `/usr/bin/vi`) stays the source of truth for
editing; peekseq stays the source of truth for browsing and indexing.

The read-only invariant is relaxed to "read-only unless the user presses
`e`," and the *only* place that writes to disk is the new `internal/edit`
package.

## Scope

**In scope**

- `e` key in the page view → suspend the TUI, run the editor on the
  current page's `.md` file, resume, mtime-gated reindex.
- Status-bar hints for editor failures (missing binary, non-zero exit,
  post-edit stat failure).
- Mtime-based change detection: no reindex when the file is unchanged.

**Deferred (will be revisited in a follow-up PR)**

- *Today-journal create-on-edit from the `e` key.* The spec initially
  called for `e` to create today's journal file when it doesn't exist
  on disk yet. Implementation surfaced a pre-existing limitation: the
  `.` (today's journal jump) and `e` (edit current page) handlers
  both require the page to be in `a.idx.ByName`, and `BuildIndex`
  only lists pages whose files exist on disk. So a user whose
  journal file for today has never been created cannot reach
  today's journal via `.` (existing `setHint("no journal for
  <today>")` fires), and the create-on-edit branch in `editCurrent`
  is currently unreachable from the UI. The `EnsureFile` function
  and the `editCurrent` create branch are kept (correct in
  isolation; a unit test exercises them) and will become reachable
  once a follow-up PR adds a phantom-today entry to `BuildIndex` (or
  modifies the `.` handler to create the file when missing).

**Out of scope (YAGNI)**

- Inline TUI editor, modal text-input mode, or any in-TUI text
  manipulation.
- New-page creation from the picker or any other view.
- Page rename, page move, or any other multi-file operation.
- Multi-page edits (one editor invocation = one file).
- `--editor` flag or `$PEEKSEQ_EDITOR` env var; the standard
  `$VISUAL` / `$EDITOR` / `vi` chain is the configuration surface.
- An editor-missing fatal-exit at startup (peekseq remains usable
  read-only when no editor is installed).
- A bundled modal-overlay confirmation step before launching the
  editor. The `e` key is consent enough.
- Phantom-today entry in `BuildIndex` (or `.`-key auto-create) — see
  the "Deferred" note above. Tracked as a follow-up.

## Architecture

### New package: `internal/edit`

A new sibling of `internal/render` and `internal/search`. The only
package in the project that writes to disk. Three exported functions:

```go
package edit

// Env is the configuration the editor-resolver reads. Decoupled from
// os.Getenv so tests can inject a fixed env without touching process
// state.
type Env struct {
    Visual string // value of $VISUAL
    Editor string // value of $EDITOR
}

// Resolved is the editor binary to invoke with no args (we append the
// file path at call time).
type Resolved struct {
    Binary string
}

// Resolve picks the first available editor in the standard chain:
// $VISUAL → $EDITOR → /usr/bin/vi. lookPath is injected so tests can
// simulate "set but missing" / "vi missing" without touching the real
// PATH. Returns errNoEditor only when all three lookups fail.
func Resolve(env Env, lookPath func(string) (string, error)) (Resolved, error)

// EnsureFile creates an empty file at path with mode 0o644 if it does
// not exist. Returns (true, nil) on create, (false, nil) if the file
// already existed, or (false, err) for any other stat/write failure.
// This is the create-today-journal hook.
func EnsureFile(path string) (created bool, err error)

// SnapshotMtime returns the file's modification time, or
// time.Time{} (the zero value) if the file does not exist. The zero
// return is load-bearing: it lets the caller distinguish "the file
// was just created by EnsureFile" (t0 == 0) from "the file was on
// disk before the user pressed e" (t0 > 0).
func SnapshotMtime(path string) (time.Time, error)

// errNoEditor is the sentinel returned by Resolve when none of
// $VISUAL, $EDITOR, or /usr/bin/vi resolve to a runnable binary.
var errNoEditor = errors.New("no editor found (set $VISUAL or $EDITOR, or install vi)")
```

No other file in the project writes to disk. This boundary is the new
write contract.

### App integration (`internal/views/app.go`)

Two new pieces of state on `App`, one new key, one new message.

**New message**

```go
// editorExitedMsg is delivered when the child editor process returns.
// path is the file we handed to the editor; t0 is the pre-edit mtime
// snapshot (zero if the file did not exist before EnsureFile ran).
type editorExitedMsg struct {
    path string
    t0   time.Time
    err  error // non-nil when the editor exited non-zero or failed to launch
}
```

**New case in the page-view key dispatch** (sibling to `g`, `G`, `n`,
`N`, `[`, `]`, etc.):

```go
case "e":
    return a, a.editCurrent()
```

**`App.editCurrent()`** — the orchestrator:

1. `page := a.page.Page()` — the page the user is looking at.
2. `path := a.idx.ByName[page].Path` — file path.
3. `t0, statErr := edit.SnapshotMtime(path)`.
   - If `statErr` is non-nil and is *not* `os.IsNotExist` (e.g.
     EACCES on the parent directory), bail with `setHint("cannot
     stat: <err>")` and do not launch the editor.
   - Otherwise `t0` is either the file's real mtime or `time.Time{}`
     (zero), both of which the rest of the flow handles.
4. If `t0.IsZero()`: `edit.EnsureFile(path)`. On error → `setHint`
   "cannot create journal: <err>", no editor launch, no reindex.
5. `resolved, err := edit.Resolve(edit.Env{Visual: os.Getenv("VISUAL"),
   Editor: os.Getenv("EDITOR")}, exec.LookPath)`. On error →
   `setHint("cannot resolve editor: " + err.Error())`, no reindex.
6. Return a `tea.Cmd` that:
   - snapshots `t0` (captured by value, not by reference),
   - runs `tea.ExecProcess(exec.Command(resolved.Binary, path))`,
   - yields an `editorExitedMsg{path, t0, err}` when the child exits.

**New case in `App.Update`**:

```go
case editorExitedMsg:
    if m.err != nil {
        a.setHint("editor exited: " + m.err.Error())
        return a, nil
    }
    info, err := os.Stat(m.path)
    if err != nil {
        if os.IsNotExist(err) {
            return a, nil // file deleted in editor — silent no-op
        }
        return a, a.setHint("cannot stat: " + err.Error())
    }
    if info.ModTime().Equal(m.t0) {
        return a, nil // unchanged
    }
    return a, a.buildIndexCmd() // mtime advanced — reindex
```

The `buildIndexCmd()` path is the *existing* reindex chokepoint; it
yields `indexLoadedMsg`, the handler of which already rebuilds the
current `PageView` with the new index. We do not write a second
reindex path; we trigger the one that's already there.

### Key map

- Add `keyE = "e"` to `internal/views/keys.go` (consistency with
  `keyQ`, `keyJ`, `keyK`, etc.). Not strictly required — `app.go` can
  use the literal `"e"` directly — but matches the file's
  "multi-use keys live here, single-use stay at the call site" rule
  (and the key will be referenced from tests and the help overlay
  renderer, so a constant pays for itself).

### Help overlay

Add a row to the help table in `internal/views/help.go`:

```
e   edit current page in $EDITOR
```

`help_test.go` needs a one-line update to the expected rows string
(its golden file under `internal/views/testdata/`). The change is
intentional, not a regression — re-run `go test ./... -update` and
visually diff the golden before staging.

## Data flow (end to end)

```
                 ┌─────────────────────────────────────────────┐
                 │ 1. App.Update receives tea.KeyMsg{"e"}      │
                 │    page = a.page.Page()                    │
                 │    path  = a.idx.ByName[page].Path          │
                 │    t0    = edit.SnapshotMtime(path)        │
                 │    if t0.IsZero(): edit.EnsureFile(path)   │
                 │    resolved = edit.Resolve(...)            │
                 │    → returns a tea.Cmd wrapping             │
                 │      tea.ExecProcess(exec.Command(         │
                 │        resolved.Binary, path))             │
                 └─────────────────────────────────────────────┘
                                     │
                                     ▼
                 ┌─────────────────────────────────────────────┐
                 │ 2. tea.ExecProcess suspends alt-screen,    │
                 │    runs editor as child process.           │
                 │    peekseq is paused, not running.          │
                 │    Editor takes over the terminal.          │
                 └─────────────────────────────────────────────┘
                                     │
                                     ▼
                 ┌─────────────────────────────────────────────┐
                 │ 3. Editor exits → tea.ExecProcess          │
                 │    restores alt-screen, yields              │
                 │    editorExitedMsg{path, t0, err}           │
                 │    to App.Update.                          │
                 │    → on err: setHint, no reindex.          │
                 │    → on stat ENOENT: silent no-op.          │
                 │    → on stat != t0: buildIndexCmd()        │
                 │      (yields indexLoadedMsg, which          │
                 │       rebuilds PageView for current page).  │
                 └─────────────────────────────────────────────┘
```

## Component: error handling

The edit path is the only disk-writing part of the project. Every
failure mode gets an explicit answer.

| Failure                              | Detection                             | Behaviour                                                                                       |
|--------------------------------------|---------------------------------------|-------------------------------------------------------------------------------------------------|
| `VISUAL`/`EDITOR` set, binary absent | `exec.LookPath` returns ENOENT        | Skip that candidate, try the next in the chain. Eventually `errNoEditor`.                       |
| All three missing                    | `Resolve` returns `errNoEditor`       | `setHint("cannot resolve editor: " + errNoEditor.Error())` — `"cannot resolve editor: no editor found (set $VISUAL or $EDITOR, or install vi)"`. No reindex. |
| Editor exits non-zero                | `tea.ExecProcess` yields `err != nil` | `setHint("editor exited: <err>")`. No reindex. We don't try to distinguish `:cq` from "saved ok". |
| Editor killed by signal              | `err` non-nil with signal info        | Same hint.                                                                                      |
| File deleted in editor               | `os.Stat` ENOENT post-exit            | Silent no-op. Page still renders from the *old* index entry; user can press `R` to refresh.     |
| `os.Stat` post-exit EACCES           | wrapped `os.Stat` error               | `setHint("cannot stat: <err>")`. No reindex.                                                    |
| `EnsureFile` fails (journal create)  | `os.WriteFile` returns error          | `setHint("cannot create journal: <err>")`. We bail *before* launching the editor.               |
| Editor preserves mtime, content changed | `t1 == t0` despite a real edit     | Silent no-op. Documented edge case; user can press `R` to force a reindex.                      |
| External tool writes the file during edit | mtime advances (treated as "user changed something") | Reindex fires on return. Same race that already exists with `R`; not a new problem.            |

**Two design points worth flagging**

1. **Editor failure is non-fatal.** peekseq never *requires* the user
   to have an editor installed. The `rg`-style "exit 2 on startup if
   vi is missing" pattern is not applied here. Reasoning: vi is
   optional in modern installs (Windows, Alpine without busybox, etc.),
   and a missing editor should not block read-only browsing.

2. **No retries, no rollback.** The editor is the user's tool.
   peekseq doesn't try to be clever about it. Undo lives in the editor
   (or in `git`, which most Logseq graphs are under). peekseq's only
   job is a clean hand-off and a clean resume.

## Testing

### Layer 1: unit tests for `internal/edit/`

`internal/edit/editor_test.go`. Pure-function tests, no teatest, no
process spawning. `Resolve` is parameterised on a `lookPath` shim so
test cases can simulate any PATH state.

```go
func TestResolve(t *testing.T) {
    cases := []struct {
        name    string
        env     Env
        lp      func(string) (string, error)
        want    string
        wantErr bool
    }{
        {"VISUAL set, present",  Env{"vim", ""},  lpPresent, "vim",  false},
        {"VISUAL set, missing",  Env{"nvim", ""}, lpMissing, "",     true /* falls through */},
        {"VISUAL missing, EDITOR present", Env{"", "emacs"}, mixed, "emacs", false},
        {"VISUAL missing, EDITOR missing, vi present", Env{"", ""}, viOnly, "/usr/bin/vi", false},
        {"all missing",          Env{}, allMissing, "", true},
        {"VISUAL empty, EDITOR empty, vi present", Env{"", ""}, viOnly, "/usr/bin/vi", false},
        {"VISUAL set, missing; EDITOR set, present", Env{"x", "y"}, fallthrough, "y", false},
    }
    // ...
}

func TestEnsureFile(t *testing.T) {
    // existing file → (false, nil), no write
    // missing file → (true, nil), empty file at 0o644
    // permission error → (false, err)
}

func TestSnapshotMtime(t *testing.T) {
    // existing file → returns ModTime
    // missing file → returns time.Time{} (zero) and a stat error
    // permission error → returns the error, time is zero
}
```

### Layer 2: teatest integration tests

`internal/views/edit_test.go`. Drives the `App` through the full
`e` keypress → editor exit → mtime-gated reindex round-trip using a
fake editor script.

**The fake editor** is a POSIX shell script at
`testdata/fake-editor.sh`, mode 0755, with an action dispatched on
`$2`:

```sh
#!/bin/sh
# testdata/fake-editor.sh
# argv: $1 = file path, $2 = action
case "$2" in
  noop)    : ;;                      # exit 0, do nothing
  touch)   touch "$1" ;;             # bump mtime only, no content change
  append)  echo "edited" >> "$1" ;;  # add a line, bump mtime
  delete)  rm "$1" ;;                # delete the file
  fail)    exit 7 ;;                 # non-zero exit
esac
```

`testdata/` is the same directory the existing fixture-graph lives
in; we add the fake editor as a sibling. The script is registered
in `.gitignore`-style fashion: it has a shebang and is intended to
be checked in. (`testdata/fixture-graph/` is already checked in
whole; adding one more file there is consistent.)

**Test cases** — each one sets `VISUAL=testdata/fake-editor.sh <action>`
via `t.Setenv` (auto-restored after the test), instantiates an
`App` with the fixture graph, and sends an `e` keypress:

| Test                                       | Action          | Expected                                                                       |
|--------------------------------------------|-----------------|--------------------------------------------------------------------------------|
| `TestEdit_Noop_NoReindex`                  | `noop`          | No `indexLoadedMsg` within a 500ms window. Hint is empty or "no changes".      |
| `TestEdit_Touch_TriggersReindex`           | `touch`         | Exactly one `indexLoadedMsg` within 1s. Page view refreshes.                   |
| `TestEdit_Append_TriggersReindex`          | `append`        | Reindex; the file now contains `edited`.                                       |
| `TestEdit_Fail_HintNoReindex`              | `fail`          | Hint reads "editor exited: ..."; no `indexLoadedMsg`.                          |
| `TestEdit_Delete_NoReindex`                | `delete`        | File gone; no reindex; old page still renders.                                 |
| `TestEdit_VisualPreferred`                 | `noop` (VISUAL and EDITOR both set) | Assert child argv[0] is VISUAL.                                       |
| `TestEdit_EditorFallback`                  | `noop` (VISUAL=missing-bin, EDITOR=present) | Assert child argv[0] is EDITOR.                                |

The "no editor installed" case is covered by the Layer 1
`TestResolve` table — `allMissing` returns `errNoEditor`, and the
App integration code surfaces it as a hint. There is no
App-level teatest for it because reliably stubbing
`/usr/bin/vi`-is-missing on CI is fragile; the unit test owns
the case via the `lookPath` shim.

The spec's original "missing-journal create" case was dropped
because the production flow is currently unreachable from the
UI — see the "Deferred" note in the In-scope section above. The
`EnsureFile` function and the `editCurrent` create branch are
exercised in unit tests (`TestEnsureFile`'s "missing file is
created empty" subtest) and remain ready for when the phantom-today
follow-up lands.

### Layer 3: golden files

No new page-view golden. The pre-`e` state is unchanged; the
post-`e` state is too mtime-dependent for a frame-equal golden.
The teatest assertions are the goldens.

`help_test.go` golden is updated to add the `e` row — the only
intentional visual change.

## Out of scope (YAGNI)

- Inline TUI editor.
- New-page creation from picker.
- Page rename / move.
- Multi-file operations.
- `--editor` flag (standard env chain is enough; can add later if
  asked).
- Concurrent-edit detection beyond mtime.
- Editor-missing fatal-exit at startup.
- Modal confirmation overlay before launching the editor.

## Risks & mitigations

| Risk                                                              | Likelihood | Mitigation                                                                                          |
|-------------------------------------------------------------------|------------|-----------------------------------------------------------------------------------------------------|
| `tea.ExecProcess` doesn't restore alt-screen correctly           | Low        | It's the Bubble Tea–blessed primitive. Manual smoke test in dev; CI can't catch alt-screen issues.  |
| Editor preserves mtime despite writing (rare, some sync tools)    | Very low   | Documented; user can press `R` to force a reindex.                                                  |
| External Logseq write during edit                                 | Low        | Treated as "user changed something"; reindex handles it. Same race as `R`; not a new problem.       |
| `teatest.NewProgram` environment isolation: child editor must see `VISUAL` | Low | Set env via `t.Setenv` in the parent test process; the child inherits the parent's env on `ExecProcess`. Verify in the first test. |
| Test runner finds `testdata/fake-editor.sh` from a different cwd | Low        | Resolve the path relative to the test's `testing.T.TempDir()` or to `runtime.Caller(0)`; not to the process's cwd. |

## Doc updates (post-implementation, pre-commit)

- `AGENTS.md` — drop the "never writes to the graph" line; replace
  with: "writes only via `internal/edit/`, only in response to the
  `e` key, and only to the file currently displayed on the page
  view." Update the Commands section to mention the new
  `testdata/fake-editor.sh`.
- `README.md` — add `e   edit current page in $EDITOR` to the
  keymap section.
- `CHANGELOG.md` — new entry under `## [Unreleased]` / `### Added`:
  "Hand off the current page to `$VISUAL`/`$EDITOR`/`vi` with the
  `e` key. mtime-gated reindex on return. Today's journal can be
  created and edited if it doesn't exist yet."
- `internal/views/help.go` — add the `e` row to the keymap table.
  Regenerate the help golden via `go test ./... -update` and
  visually diff before staging.

## Success criteria

- `internal/edit/editor.go` exists with `Resolve`, `EnsureFile`,
  `SnapshotMtime`, the `Env` and `Resolved` types, and the
  `errNoEditor` sentinel.
- `App.Update` dispatches `e` to a new `editCurrent` method that
  uses `tea.ExecProcess` to hand off to the editor.
- `editorExitedMsg` is a new package-internal message type
  (matching the existing `indexLoadedMsg` pattern) with a dedicated
  case in `App.Update` that mtime-gates the reindex.
- `testdata/fake-editor.sh` is checked in; the App integration is
  exercised by ≥6 direct-`Update` tests in
  `internal/views/edit_test.go`. (The spec's original "≥7 teatest
  cases" was relaxed to "≥6 direct-Update tests" because the
  no-editor case is fully owned by the Layer 1 `TestResolve` unit
  test, and the create-today-journal case was deferred — see the
  "Deferred" note in the In-scope section above.)
- `internal/edit/editor_test.go` has ≥8 unit tests covering the
  `Resolve` decision table and the `EnsureFile` /
  `SnapshotMtime` happy paths.
- The help overlay renders an `e` row; its golden is updated
  intentionally.
- `AGENTS.md`, `README.md`, `CHANGELOG.md` are updated.
- `go vet ./... && go test ./...` is green.
- No other file in the project writes to disk; the `internal/edit`
  boundary holds.
