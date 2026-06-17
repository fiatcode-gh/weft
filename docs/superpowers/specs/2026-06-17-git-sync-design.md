# Git sync — in-app `S`

> Design spec. Written 2026-06-17.
>
> Brings the graph's git sync in-house. Today it lives in
> `~/.config/fish/functions/weft-sync.fish` — a manual shell function the user
> runs outside weft. This makes sync a first-class, observable action *inside*
> the TUI, and weft's **second deliberate disk-mutating surface** alongside
> `internal/edit/`.

## Goal

A top-level `S` keybind runs the same commit → pull → push cycle the fish
function performs, against the graph weft was launched with. Sync runs off the
UI thread; the status bar reports progress and outcome. On a successful pull
that changed the working tree, the graph index is rebuilt so pulled pages are
immediately visible. On any failure, weft bails cleanly — it never attempts
conflict resolution inside the alt-screen.

## Scope

### In

- **Manual sync only.** A single top-level `S` keybind triggers the full
  cycle on demand. No automatic sync.
- **Same sequence as the script.** `status --porcelain` → if dirty, `add -A`
  + `commit -m "sync: YYYY-MM-DD HH:MM"` → `pull --rebase` → `push`.
- **Operates on the `--graph` directory** weft was launched with — never a
  hardcoded path.
- **Async, hint-only feedback.** `⟳ syncing…` on start; `✓ synced` or
  `✗ <stage> failed — see weft.log` on completion, via the existing
  `setHint` transient status-bar mechanism.
- **Reindex on pull.** When the pull changed the working tree, rebuild the
  graph index so pulled pages/journals appear without a manual `R`.
- **Clean bail on failure.** Full git output for the failing stage is written
  to `weft.log`; the working tree is left exactly as the shell function would
  leave it, so dropping to a terminal to resolve conflicts Just Works.
- **Single-flight.** A second `S` while a sync is in flight is a no-op with an
  `⟳ already syncing` hint.

### Out (explicit non-goals)

- **Auto-sync on launch or quit.** Manual-first. Auto-pull-on-launch may come
  later once the manual path is proven.
- **In-TUI conflict resolution.** The alt-screen is hostile to merge-conflict
  editing; weft bails to the shell instead. Not a deferral — a deliberate
  boundary.
- **Output pane / overlay.** No in-app view of git output. The hint plus
  `weft.log` is the whole reporting surface for v1.
- **Embedding git (`go-git`).** Sync shells out to the system `git`.
- **Editor coupling.** None needed — see below.

## Why no editor coupling

weft's in-app editor is **modal**: `App.editor` is non-nil only while editing,
and all keystrokes route to it during that time (so `S` types the letter `S`,
never triggers sync). Exiting the editor with unsaved changes forces a
`confirmingExit` prompt — you either save (`Ctrl+S`, which writes to disk) or
explicitly discard. There is no path that leaves a dangling in-memory dirty
buffer once `App.editor == nil`.

Therefore, whenever the `S` keybind is reachable (page view, no editor, no
overlay), disk is already authoritative. Sync operates on the working tree like
the shell function — not as a tradeoff, but because weft's structure guarantees
it. No dirty-buffer flush, no refuse-on-dirty logic.

## Architecture

### `internal/sync/` (new package)

The second deliberate mutating surface, mirroring `internal/edit/`'s
discipline. Pure git orchestration over `os/exec`; no Bubble Tea types.

```go
// Result reports what a sync did and, on failure, where it stopped.
type Result struct {
    Committed bool   // a commit was created (working tree had changes)
    Pulled    bool   // pull --rebase updated the working tree
    Pushed    bool   // push succeeded
    Stage     string // failing stage: "preflight"|"add"|"commit"|"pull"|"push"; "" on success
    Output    string // combined stdout+stderr of the failing stage; "" on success
    Err       error  // non-nil on failure
}

// Run performs commit -> pull --rebase -> push against repoDir.
// now is the commit-timestamp source, injected so tests are deterministic.
func Run(repoDir string, now time.Time) Result
```

- **Pre-flight:** verify `repoDir` is inside a git work tree
  (`git rev-parse --is-inside-work-tree`). If not, return
  `Result{Stage: "preflight", Err: …}` without shelling further.
- **Pulled detection:** capture `rev-parse HEAD` before and after `pull
  --rebase`; `Pulled` is true when they differ. This drives reindex-on-pull.
- Each stage's failure short-circuits the rest and populates `Stage`/`Output`/
  `Err`.

### App wiring (`internal/views/app.go`)

- A function-type field carries the sync runner so view tests can stub it
  without shelling out:

  ```go
  type syncRunner func(repoDir string) sync.Result
  ```

  `App` holds a `syncRunner` defaulting to a closure over `sync.Run` with the
  real clock; tests inject a stub. `App` already knows the graph directory
  (from `--graph` wiring) and reuses it as `repoDir`.
- A new `S` case in the **page-view** key handler (not the editor path, not the
  picker/search/help overlay handlers). Guarded by a `syncing bool` flag:
  - if `syncing`, return `setHint("⟳ already syncing")`.
  - else set `syncing = true`, return a `tea.Batch` of `setHint("⟳ syncing…")`
    and a `tea.Cmd` that runs the runner off-thread and emits `syncDoneMsg`.
- `syncDoneMsg{ res sync.Result }` handling clears `syncing` and:
  - **success** → `setHint("✓ synced")`; if `res.Pulled`, also return
    `buildIndexCmd()` so the graph reflects pulled files.
  - **failure** → append `res.Output` to `weft.log` (the same path
    `WEFT_DEBUG` mirrors to, written here regardless of the debug flag), then
    `setHint("✗ " + res.Stage + " failed — see weft.log")`.

## Data flow

```
press S (page view, not syncing)
  └─ syncing=true; setHint("⟳ syncing…") + go runner(repoDir)
       └─ sync.Run: preflight → [add+commit if dirty] → pull --rebase → push
            └─ syncDoneMsg{res}
                 ├─ success: syncing=false; setHint("✓ synced");
                 │            if res.Pulled → buildIndexCmd()
                 └─ failure: syncing=false; append res.Output to weft.log;
                              setHint("✗ <stage> failed — see weft.log")
```

## Error handling

- **Not a git repo:** `preflight` stage; hint `✗ preflight failed — see
  weft.log`. (Expected when run against the fixture graph, which is not a repo —
  so the keybind degrades gracefully rather than erroring loudly.)
- **Pull conflict / rebase halt:** `pull` stage; working tree left mid-rebase
  exactly as `git` leaves it; full output in `weft.log`; user resolves in a
  shell. weft does not touch the conflicted tree.
- **Push rejected (non-fast-forward, auth, offline):** `push` stage; the local
  commit and successful rebase are preserved; hint + log; re-running `S` after
  the cause is fixed completes the push.
- **Concurrent `S`:** suppressed by the single-flight flag.

## Testing

- **`internal/sync/`** — build throwaway repos in `t.TempDir()` wired to a
  local **bare remote** (also under `TempDir`). Zero network, never touches the
  real or fixture graph. Cases: clean no-op sync; dirty → commit + push;
  pull brings remote commits (`Pulled == true`); forced conflict halts at
  `pull` with populated `Output`; non-repo dir → `preflight`. `now` is a fixed
  `time.Time` so commit messages are deterministic.
- **`internal/views/`** — teatest golden frames for the three hint states
  (`⟳ syncing…`, `✓ synced`, `✗ <stage> failed`) and the `⟳ already syncing`
  guard, driven by a stubbed `syncRunner`. A stub returning `Pulled: true`
  asserts the index rebuild fires. View tests never shell out to git.

## Relationship to `weft-sync.fish`

The shell function keeps working standalone; this spec does not remove it.
It can be retired once the in-app path is proven in daily use. The ASCII banner
and per-stage coloured output are intentionally not reproduced — the one-line
hint plus `weft.log` is the whole reporting surface.
