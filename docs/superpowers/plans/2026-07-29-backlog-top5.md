# Backlog Top-5 Sweep Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Work the top five items of the weft backlog: overlay-visible sync-busy feedback, BuildIndex warnings surfaced in-app, no caching of failed Glamour renders, a fail-loud `WEFT_DEBUG`, a width-padded task-marker sentinel, and scroll restore across the synchronous reindex.

**Architecture:** All changes stay inside the existing layering (`graph` ← `render`/`views` ← `cmd/weft`; `render` never imports `views`). In-panel feedback copies the `Backlinks.SetLinkifyError` precedent into `Picker`; warnings ride on the `Index` struct and are logged/hinted by the views layer; the render fallback is flagged on `render.Result` so the views cache can skip it.

**Tech Stack:** Go 1.26, Bubble Tea / lipgloss / Glamour, teatest goldens, ripgrep on PATH.

## Global Constraints

- Worktree: `/var/home/dhemas/Development/Projects/fiatcode/weft/.claude/worktrees/backlog-top5` — every shell command runs here (`cd` into it first; the subagent shell does NOT inherit it).
- Branch: `fix/backlog-top5`. **Immediately before every `git commit`, run `git rev-parse --abbrev-ref HEAD` and verify it prints `fix/backlog-top5`. If it prints anything else, STOP — do not commit.**
- Full gate before every commit, all four must pass:
  `gofmt -l .` (must print nothing) · `go vet ./...` · `"$(go env GOPATH)/bin/staticcheck" ./...` · `go test -race ./...`
- Conventional commit messages (`fix:`, `test:`, `docs:` …).
- Never point tests at the real graph (`~/Documents/fiat-codex`); use `t.TempDir()` / `writeGraph` / the fixture graph.
- teatest goldens (`internal/views/testdata/*.golden`) may only be regenerated (`go test ./... -update`) after visually diffing the change; unexpected golden diffs are a bug in your change.
- Test bodies use `// arrange`, `// act`, `// assert` structure and reuse the existing helpers (`quietTerm`, `writeGraph`, `bootApp*`, `loadFixture`, `typeQuery`) rather than copy-pasting setup.
- CHANGELOG entries go under a `## [Unreleased]` → `### Fixed` section at the top of `CHANGELOG.md` (Task 1 creates it; later tasks append one bullet each, user-visible phrasing like the existing entries).

---

### Task 1: Overlay-visible sync-busy feedback

The sync-busy gate (`App.syncBusyHint`, `internal/views/app.go:882-891`) sets a status-bar hint, but two of its five call sites fire while an overlay is open (`app.go:677-682` linkify, `app.go:684-691` picker create) and `App.View` renders the overlay *instead of* the status bar (`app.go:809-827`), so the keypress looks dead. `Backlinks` already solved in-panel messaging with `errMsg` + `SetLinkifyError` (`internal/views/backlinks.go:137-144`, rendered at `:209-220`). Give `Picker` the same channel and make the gate overlay-aware.

**Files:**
- Modify: `internal/views/picker.go` (field, `SetError`, footer render, clear-on-key)
- Modify: `internal/views/app.go:882-891` (gate) and its five call sites (`:678`, `:685`, `:726-728`, `:779-781`, `:784-787`)
- Test: `internal/views/picker_test.go`, `internal/views/app_sync_test.go`
- Modify: `CHANGELOG.md` (create `## [Unreleased]` / `### Fixed`)

**Interfaces:**
- Consumes: `Backlinks.SetLinkifyError(msg string)` (exists, unchanged).
- Produces: `(p *Picker) SetError(msg string)`; `(a *App) blockIfSyncing() (cmd tea.Cmd, blocked bool)` replacing `syncBusyHint()` (delete the old method). No other task depends on these.

- [ ] **Step 1: Write the failing picker unit test** (in `picker_test.go`)

```go
// Feedback for a blocked action must render inside the overlay — the status
// bar is hidden behind it (see Backlinks.SetLinkifyError for the precedent).
func TestPickerSetErrorRendersInPanel(t *testing.T) {
	// arrange
	quietTerm(t)
	p := NewPicker(loadFixture(t), 80, 30)

	// act
	p.SetError("sync in progress — retry when it finishes")

	// assert
	if !strings.Contains(p.View(), "sync in progress — retry when it finishes") {
		t.Fatalf("expected in-panel message, got:\n%s", p.View())
	}
	p.Update("a") // any subsequent key clears it
	if strings.Contains(p.View(), "sync in progress") {
		t.Fatalf("expected message cleared on next key, got:\n%s", p.View())
	}
}
```

- [ ] **Step 2: Verify it fails** — `go test ./internal/views -run TestPickerSetErrorRendersInPanel` → compile error `p.SetError undefined`. That is the RED evidence; record it.

- [ ] **Step 3: Implement the picker side.** Add to the `Picker` struct (`picker.go:22-31`): `errMsg string // feedback line rendered inside the panel; see SetError`. Add:

```go
// SetError records a message to show inside the panel, above the footer.
// Feedback for a blocked action must render inside the overlay — a
// status-bar hint would be invisible behind it (same rationale as
// Backlinks.SetLinkifyError). Cleared on the next key.
func (p *Picker) SetError(msg string) {
	p.errMsg = msg
}
```

At the very top of `Picker.Update` (`picker.go:128`), before the `switch`, add `p.errMsg = ""`. In `View()`, both footer sites (the early-return branch at `:203-209` and the main path at `:258-259`) currently write `"\n"` + the key legend; factor them into one helper and render the message above the legend, exactly the Backlinks shape:

```go
// writeFooter renders the blank spacer, the optional in-panel message, and
// the key legend. The message line grows the panel by one row, matching how
// Backlinks renders errMsg.
func (p *Picker) writeFooter(b *strings.Builder, inner int) {
	b.WriteString("\n")
	if p.errMsg != "" {
		b.WriteString(styleTitle.Render(clamp(p.errMsg, inner)))
		b.WriteString("\n")
	}
	b.WriteString(styleFaint.Render(clamp("↑/↓ select · enter open · esc cancel", inner)))
}
```

Do NOT change `visibleRows()`/`chrome` (`picker.go:183-188`) — Backlinks also lets the message grow the panel by one line.

- [ ] **Step 4: Run the test** — `go test ./internal/views -run TestPickerSetErrorRendersInPanel` → PASS; run `go test ./internal/views` to confirm no golden regressed.

- [ ] **Step 5: Write the failing app-level tests.** In `app_sync_test.go`, add a new test using a *real* Picker (the existing `TestPickerCreateBlockedWhileSyncing` at `:225-242` uses `stubOverlay` and stays as-is — it now pins the status-bar fallback for unknown overlay types):

```go
// A blocked create must say so inside the picker: the status bar is hidden
// behind the overlay, so a hint there reads as a dead keypress.
func TestPickerCreateBlockedWhileSyncingShowsInPanelMessage(t *testing.T) {
	// arrange
	quietTerm(t)
	a := bootApp(t)
	a.syncing = true
	p := NewPicker(a.idx, 80, 24)
	typeQuery(p, "Brand New Page")
	if p.createName == "" {
		t.Fatal("arrange failed: expected a create row")
	}
	for p.sel < len(p.matches) { // move selection onto the create row
		p.moveDown(p.rowCount())
	}
	a.active = p

	// act
	a.Update(tea.KeyMsg{Type: tea.KeyEnter})

	// assert
	if a.active == nil {
		t.Fatal("picker should stay open when create is blocked")
	}
	if a.editor != nil {
		t.Fatal("editor must not open while syncing")
	}
	if !strings.Contains(p.View(), "sync in progress") {
		t.Fatalf("expected in-panel busy message, got:\n%s", p.View())
	}
}
```

Also update `TestLinkifyBlockedWhileSyncing` (`app_sync_test.go:164-215`): replace its `a.hint` assertion with `if !strings.Contains(b.View(), "sync in progress") { t.Fatalf(...) }` (it constructs the `*Backlinks` as `b`; add `quietTerm(t)` to its arrange if not already present). Keep its file-unchanged assertion.

- [ ] **Step 6: Verify they fail** — `go test ./internal/views -run 'TestPickerCreateBlockedWhileSyncingShowsInPanelMessage|TestLinkifyBlockedWhileSyncing'` → both FAIL (message currently goes to `a.hint`). Record RED.

- [ ] **Step 7: Implement the gate.** In `app.go`, replace `syncBusyHint` (keep its doc comment's first sentence about WHY the gate exists) with:

```go
// blockIfSyncing gates the graph-mutating entry points while the async git
// sync goroutine is rewriting the worktree: a save landing mid
// `pull --rebase` is overwritten by the rebase checkout and silently lost,
// and `add -A` can stage editor temp files. Reports blocked=true while a
// sync runs. Feedback goes into the open overlay when it has an in-panel
// channel — the status bar is hidden behind an overlay — falling back to a
// status-bar hint; cmd is non-nil only for that fallback.
func (a *App) blockIfSyncing() (cmd tea.Cmd, blocked bool) {
	if !a.syncing {
		return nil, false
	}
	const msg = "sync in progress — retry when it finishes"
	switch o := a.active.(type) {
	case *Backlinks:
		o.SetLinkifyError(msg)
		return nil, true
	case *Picker:
		o.SetError(msg)
		return nil, true
	default:
		return a.setHint("⟳ " + msg), true
	}
}
```

Update all five call sites to the two-value form, e.g. at `app.go:678`:

```go
			if res.Linkify != nil {
				if cmd, blocked := a.blockIfSyncing(); blocked {
					return a, cmd
				}
				return a, a.linkify(res.Linkify, res.LinkifyTarget)
			}
```

(same shape at `:685`, and at the `.`/`e`/`E` sites where `a.active` is nil so the default branch keeps today's hint behavior).

- [ ] **Step 8: Run the full package** — `go test -race ./internal/views` → PASS (including the untouched `TestWriteKeysBlockedWhileSyncing` and stub-overlay test).

- [ ] **Step 9: CHANGELOG.** Add at the top of `CHANGELOG.md` (after the intro paragraph):

```markdown
## [Unreleased]

### Fixed

- Creating a page or linkifying a mention while a sync is running now says "sync in progress" inside the open panel, instead of an invisible status-bar hint that made the keypress look dead.
```

- [ ] **Step 10: Full gate, then commit**

```bash
gofmt -l . && go vet ./... && "$(go env GOPATH)/bin/staticcheck" ./... && go test -race ./...
git rev-parse --abbrev-ref HEAD   # MUST print fix/backlog-top5
git add internal/views/picker.go internal/views/app.go internal/views/picker_test.go internal/views/app_sync_test.go CHANGELOG.md
git commit -m "fix(views): show sync-busy feedback inside the open overlay"
```

---

### Task 2: Surface BuildIndex warnings in-app

`graph.BuildIndex` writes three kinds of warning to stderr (`internal/graph/index.go:43` subdir skip, `:71` case-fold collision, `:98` unreadable page) — invisible under the alt-screen. A fourth problem is silent: an `e.Info()` error (`index.go:55-57`) zeroes `ModTime`, sinking the page in the recency picker with no trace. Collect all four on the `Index`, log them to `DebugLogPath()`, and hint `indexed with N warnings — see <path>`. Note: `graph` must not import `views` (dependency direction), so the log/hint side lives in `views`.

**Files:**
- Modify: `internal/graph/index.go`
- Modify: `internal/views/app.go` (`indexLoadedMsg` handler `:518-547`, `reindex()` `:322-332`, new helpers near `logSyncFailure` `:893-909`)
- Test: `internal/graph/index_test.go` (rewrite `TestBuildIndexWarnsOnSubdirectory` `:175-212` and `TestBuildIndexFoldCollisionIsDeterministicAndWarns` `:334-383`; add unreadable-page test), `internal/views/app_test.go`
- Modify: `README.md:98-103` (stderr wording), `CHANGELOG.md`

**Interfaces:**
- Produces: `Index.Warnings []string` (new field); `(a *App) logIndexWarnings(warnings []string) error`; `(a *App) indexWarningsHint(warnings []string) tea.Cmd` (nil when no warnings). No other task depends on these.
- Consumes: `DebugLogPath()` (`internal/views/logpath.go:13`), `a.setHint`, `a.nowFunc`.

- [ ] **Step 1: Rewrite the two stderr tests to expect `Index.Warnings`.** In both tests, delete the `os.Pipe`/`os.Stderr` swap scaffolding entirely. Replace the stderr assertions with:

```go
	// assert — in TestBuildIndexWarnsOnSubdirectory
	found := false
	for _, w := range idx.Warnings {
		if strings.Contains(w, "skipping subdirectory") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected subdirectory warning in idx.Warnings, got %v", idx.Warnings)
	}
```

(and the same shape with `"ambiguous page name"` in the collision test, keeping that test's existing `Resolve` determinism assertions). Add a new test — follow the arrange style already used in `index_test.go` for writing fixture files:

```go
// An unreadable page must not abort boot: it stays in the picker (first
// pass) but its body parse is skipped with a warning (second pass).
func TestBuildIndexCollectsUnreadablePageWarning(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores file modes")
	}
	// arrange
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "pages"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"Good.md", "Bad.md"} {
		if err := os.WriteFile(filepath.Join(dir, "pages", name), []byte("- TODO x\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	bad := filepath.Join(dir, "pages", "Bad.md")
	if err := os.Chmod(bad, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(bad, 0o644) }) // so TempDir removal works

	// act
	idx, err := BuildIndex(dir)

	// assert
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := idx.Resolve("Bad"); !ok {
		t.Error("unreadable page should stay in the index")
	}
	found := false
	for _, w := range idx.Warnings {
		if strings.Contains(w, "skipping") && strings.Contains(w, "Bad.md") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected unreadable-page warning, got %v", idx.Warnings)
	}
}
```

- [ ] **Step 2: Verify RED** — `go test ./internal/graph` → compile error `idx.Warnings undefined`. Record it.

- [ ] **Step 3: Implement the graph side.** Add to `Index` (`index.go:12-20`):

```go
	// Warnings collects non-fatal problems found during the walk (skipped
	// subdirectories, case-fold collisions, unreadable pages, stat
	// failures). The TUI logs and hints them; BuildIndex never writes to
	// stderr — under the alt-screen nobody would see it.
	Warnings []string
```

Add a helper and convert the three `fmt.Fprintf(os.Stderr, "weft: …")` calls (dropping the `weft: ` prefix — the log line gets its own prefix in views):

```go
func (idx *Index) warnf(format string, args ...any) {
	idx.Warnings = append(idx.Warnings, fmt.Sprintf(format, args...))
}
```

- `:43` → `idx.warnf("skipping subdirectory %s (weft does not recurse — use the ___ namespace convention instead)", filepath.Join(dir, e.Name()))`
- `:71` → `idx.warnf("ambiguous page name %q vs %q (case-insensitive); [[%s]] resolves to %q", name, existing.Name, fold, existing.Name)`
- `:98` → `idx.warnf("skipping %s: %v", p.Path, err)`

Give the stat failure a warning (`index.go:55-57`):

```go
		if info, err := e.Info(); err == nil {
			meta.ModTime = info.ModTime()
		} else {
			idx.warnf("cannot stat %s: %v — page will sort last in the picker", path, err)
		}
```

(no test forces this branch — `DirEntry.Info` failure needs a delete race; the branch is two lines and reviewer-verified). Update the `BuildIndex` doc comment (`index.go:22-23`): "Unreadable files are skipped and recorded in Index.Warnings." Check whether `os` is still needed in the import block (it is — `ReadDir`/`ReadFile`).

- [ ] **Step 4: Verify graph green** — `go test ./internal/graph` → PASS.

- [ ] **Step 5: Gate + first commit**

```bash
gofmt -l . && go vet ./... && "$(go env GOPATH)/bin/staticcheck" ./... && go test -race ./...
git rev-parse --abbrev-ref HEAD   # MUST print fix/backlog-top5
git add internal/graph/index.go internal/graph/index_test.go
git commit -m "fix(graph): collect BuildIndex warnings on the Index instead of stderr"
```

- [ ] **Step 6: Write the failing views test** (in `app_test.go`):

```go
// Index warnings must reach the user: hinted in the status bar and
// appended to the debug log (stderr is invisible under the alt-screen).
func TestIndexWarningsSurfaceAsHintAndLog(t *testing.T) {
	// arrange
	quietTerm(t)
	cache := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cache)
	a := bootAppWithGraph(t, map[string]string{
		"pages/Home.md":       "- hello\n",
		"pages/sub/Nested.md": "- nested\n", // pages/sub/ → one subdir warning
	})

	// assert — boot indexing already ran inside bootAppWithGraph
	if !strings.Contains(a.hint, "indexed with 1 warning — see ") {
		t.Fatalf("hint = %q, want indexed-with-warnings hint", a.hint)
	}
	logBytes, err := os.ReadFile(filepath.Join(cache, "weft", "weft.log"))
	if err != nil {
		t.Fatalf("expected warnings appended to the debug log: %v", err)
	}
	if !strings.Contains(string(logBytes), "skipping subdirectory") {
		t.Fatalf("log missing warning detail, got:\n%s", logBytes)
	}
}
```

Check `writeGraph` (`helpers_test.go:61-83`) creates parent dirs for `pages/sub/Nested.md`; if it doesn't, `os.MkdirAll` the subdir in the test before boot instead. If `bootAppWithGraph`'s boot asserts break on the hint, adapt — but they only require `a.page != nil`.

- [ ] **Step 7: Verify RED** — `go test ./internal/views -run TestIndexWarningsSurfaceAsHintAndLog` → FAIL (empty hint). Record it.

- [ ] **Step 8: Implement the views side.** Next to `logSyncFailure` (`app.go:893-909`), add:

```go
// logIndexWarnings appends index-build warnings to DebugLogPath() — written
// regardless of WEFT_DEBUG so the hint's "see <path>" pointer is valid
// (mirrors logSyncFailure). Returns the write error so the caller can
// degrade the hint instead of pointing at a log that was never written.
func (a *App) logIndexWarnings(warnings []string) error {
	f, err := os.OpenFile(DebugLogPath(), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	for _, w := range warnings {
		if _, err := fmt.Fprintf(f, "index warning at %s: %s\n", a.nowFunc().Format(time.RFC3339), w); err != nil {
			return err
		}
	}
	return nil
}

// indexWarningsHint logs warnings and returns the status-bar hint command,
// or nil when there are none.
func (a *App) indexWarningsHint(warnings []string) tea.Cmd {
	n := len(warnings)
	if n == 0 {
		return nil
	}
	noun := "warnings"
	if n == 1 {
		noun = "warning"
	}
	hint := fmt.Sprintf("indexed with %d %s — see %s", n, noun, DebugLogPath())
	if err := a.logIndexWarnings(warnings); err != nil {
		hint = fmt.Sprintf("indexed with %d %s: %s", n, noun, warnings[0])
	}
	return a.setHint(hint)
}
```

In the `indexLoadedMsg` handler's success path (`app.go:533-547`), after `a.idx = m.idx` and the page rebuild, replace the final `return a, a.statusProbeCmd()` for the success path with:

```go
		return a, tea.Batch(a.statusProbeCmd(), a.indexWarningsHint(m.idx.Warnings))
```

(`tea.Batch` ignores nil cmds; make sure the error paths above keep their existing returns). In `reindex()` (`app.go:322-332`), after `a.idx = idx`, add log-only surfacing (its callers run under an open overlay, where a hint is invisible — Task 1's territory; the log still captures the details):

```go
	if len(idx.Warnings) > 0 {
		// Best-effort: nothing to degrade to here — an overlay usually
		// covers the status bar when this path runs (linkify, journal
		// create), so persist the details and move on.
		_ = a.logIndexWarnings(idx.Warnings)
	}
```

- [ ] **Step 9: Verify green** — `go test -race ./internal/views` → PASS, including the pre-existing hint/dispatch tests.

- [ ] **Step 10: Docs.** In `README.md` (~lines 98-103) replace both "warns on stderr" phrasings: collisions → "weft records a warning (status bar points at `weft.log`) and resolves links deterministically to the first"; subdirectories → "skipped with a warning surfaced in-app". Append to CHANGELOG `### Fixed`:

```markdown
- Index warnings (skipped subdirectories, ambiguous page names, unreadable pages) now surface as a status-bar hint pointing at `weft.log`, instead of going to stderr where the alt-screen hides them. A page whose modification time can't be read is reported too, instead of silently sinking to the bottom of the picker.
```

- [ ] **Step 11: Gate + second commit**

```bash
gofmt -l . && go vet ./... && "$(go env GOPATH)/bin/staticcheck" ./... && go test -race ./...
git rev-parse --abbrev-ref HEAD   # MUST print fix/backlog-top5
git add internal/views/app.go internal/views/app_test.go README.md CHANGELOG.md
git commit -m "fix(views): surface index warnings as a status hint and weft.log entries"
```

---

### Task 3: Don't cache a failed Glamour render

`RenderWithEmphasis` swallows a Glamour failure (`internal/render/page.go:626-630`): `styled = pre`, error discarded, function returns `nil` error — so `PageView.load` caches the unstyled result by mtime (`internal/views/page.go:295-302`) and the fallback sticks until the file changes. Flag the fallback on `render.Result`, skip the cache, log the cause. `render` cannot import `views` (cycle), so the flag travels on the Result and `views` does the logging.

**Files:**
- Modify: `internal/render/page.go` (Result field `:33-45`, seam + fallback `:622-630`, return `:683`)
- Modify: `internal/views/page.go` (`load()` `:265-304`, new `renderPage` var, new `logRenderFallback`)
- Test: `internal/render/page_test.go`, `internal/views/page_test.go`

**Interfaces:**
- Produces: `render.Result.FallbackErr error`; package-private `render.glamourRender` var (test seam); package-private `views.renderPage` var (test seam, `= render.RenderWithEmphasis`).
- Consumes: `DebugLogPath()`.

- [ ] **Step 1: Write the failing render test** (in `page_test.go`; match `mustRender`'s actual signature at `:14-33`):

```go
// A Glamour failure falls back to un-styled text — that must be visible to
// the caller (FallbackErr) so it can skip its cache: a transient failure
// cached by mtime becomes a sticky unstyled page.
func TestRenderReportsGlamourFallback(t *testing.T) {
	// arrange — no input reliably makes Glamour fail, so swap the seam
	orig := glamourRender
	t.Cleanup(func() { glamourRender = orig })
	glamourRender = func(*glamour.TermRenderer, string) (string, error) {
		return "", errors.New("boom")
	}

	// act
	res, err := Render("- TODO hello [[Alpha]]\n", 40)

	// assert
	if err != nil {
		t.Fatalf("fallback must not be an error: %v", err)
	}
	if res.FallbackErr == nil {
		t.Fatal("expected FallbackErr to record the Glamour failure")
	}
	if !strings.Contains(res.Styled, "hello") {
		t.Fatalf("fallback should still carry the page text, got:\n%s", res.Styled)
	}
}
```

- [ ] **Step 2: Verify RED** — `go test ./internal/render -run TestRenderReportsGlamourFallback` → compile errors (`glamourRender`, `FallbackErr` undefined). Record it.

- [ ] **Step 3: Implement the render side.** Add near `rendererCache` (`page.go:148-151`):

```go
// glamourRender invokes the width-cached renderer. A var so tests can
// simulate a Glamour failure — no markdown input reliably triggers one.
var glamourRender = func(r *glamour.TermRenderer, in string) (string, error) {
	return r.Render(in)
}
```

Add to `Result` (`page.go:33-45`):

```go
	// FallbackErr is non-nil when Glamour failed and Styled carries the
	// un-styled source (sentinels still substituted). Callers must not
	// cache the result — the failure may be transient.
	FallbackErr error
```

Rework the render call (`page.go:626-630`) — keep the fallback error in its own variable so the later `return` can carry it:

```go
	styled, fallbackErr := glamourRender(r, pre)
	if fallbackErr != nil {
		// Fallback: plain text if Glamour chokes. Recorded on the Result so
		// the caller can skip its cache and log the cause.
		styled = pre
	}
```

and the return (`page.go:683`): `return Result{Styled: out.String(), Links: links, Tasks: tasks, Finds: finds, FallbackErr: fallbackErr}, nil`.

- [ ] **Step 4: Verify green** — `go test ./internal/render` → PASS.

- [ ] **Step 5: Write the failing views test** (in `internal/views/page_test.go`; `RenderCount` lives at `page.go:17-24`):

```go
// A fallback render must not enter the mtime-keyed cache (a transient
// Glamour failure would stick as an unstyled page until the file changes),
// and its cause must land in the debug log.
func TestPageViewSkipsCacheAndLogsOnRenderFallback(t *testing.T) {
	// arrange
	quietTerm(t)
	cache := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cache)
	orig := renderPage
	t.Cleanup(func() { renderPage = orig })
	renderPage = func(body string, width int, emphasis string) (render.Result, error) {
		return render.Result{Styled: body, FallbackErr: errors.New("boom")}, nil
	}
	_, idx := writeGraph(t, map[string]string{"pages/Alpha.md": "- hi\n"})
	p := NewPageView(idx, "Alpha", 80, 24)
	before := RenderCount()

	// act — a revisit would be served from the cache if the fallback were cached
	p.SetPage("Alpha")

	// assert
	if got := RenderCount() - before; got != 1 {
		t.Fatalf("expected a fresh render on revisit (cache skipped), got %d new renders", got)
	}
	logBytes, err := os.ReadFile(filepath.Join(cache, "weft", "weft.log"))
	if err != nil {
		t.Fatalf("expected fallback logged: %v", err)
	}
	if !strings.Contains(string(logBytes), "render fallback") {
		t.Fatalf("log missing fallback entry:\n%s", logBytes)
	}
}
```

- [ ] **Step 6: Verify RED** — compile error (`renderPage` undefined). Record it.

- [ ] **Step 7: Implement the views side.** In `page.go`, add near `renderCount`:

```go
// renderPage is the single entry into the render package — a var so tests
// can inject render results. RenderWithEmphasis("") is identical to Render.
var renderPage = render.RenderWithEmphasis
```

In `load()` replace the `if p.emphasis != "" { … } else { … }` pair (`:289-294`) with `res, err = renderPage(body, p.width, p.emphasis)`. After the existing `err` check (`:295-298`):

```go
	if res.FallbackErr != nil {
		// Un-styled fallback: log the cause; the cache skip below keeps a
		// transient Glamour failure from sticking until the mtime changes.
		logRenderFallback(p.page, res.FallbackErr)
	}
	p.result = res
	if p.emphasis == "" && res.FallbackErr == nil {
		p.cache[meta.Name] = cachedPage{result: res, modTime: meta.ModTime, width: p.width}
	}
```

Add (in `page.go`, `time` is already imported):

```go
// logRenderFallback best-effort appends a Glamour-fallback notice to
// DebugLogPath(). A write failure is dropped: the page already shows its
// readable raw-text fallback, and there is no status-bar in this layer to
// degrade to.
func logRenderFallback(page string, cause error) {
	f, err := os.OpenFile(DebugLogPath(), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "render fallback for %q at %s: %v\n", page, time.Now().Format(time.RFC3339), cause)
}
```

- [ ] **Step 8: Verify green** — `go test -race ./internal/views ./internal/render` → PASS (cache tests at `page_test.go:181-206` must still pass — the normal path still caches).

- [ ] **Step 9: CHANGELOG** (`### Fixed`): `- A markdown render that falls back to plain text (Glamour failure) is no longer cached, so a transient failure can't stick as an unstyled page; the cause is logged to weft.log.`

- [ ] **Step 10: Gate + commit**

```bash
gofmt -l . && go vet ./... && "$(go env GOPATH)/bin/staticcheck" ./... && go test -race ./...
git rev-parse --abbrev-ref HEAD   # MUST print fix/backlog-top5
git add internal/render/page.go internal/render/page_test.go internal/views/page.go internal/views/page_test.go CHANGELOG.md
git commit -m "fix(render): flag Glamour fallback so views skip caching and log it"
```

---

### Task 4: `WEFT_DEBUG=1` fails loud when the log can't open

`cmd/weft/main.go:100-104` swallows the `tea.LogToFile` error — the user explicitly asked for logging, pre-alt-screen is the one moment stderr is free, and every sibling preflight failure already does `fmt.Fprintln(os.Stderr, …)` + `os.Exit(2)` (`main.go:82-98`). Extract a testable helper and exit 2 on failure.

**Files:**
- Modify: `cmd/weft/main.go`
- Test: `cmd/weft/main_test.go`
- Modify: `CHANGELOG.md`

**Interfaces:**
- Produces: `initDebugLog(val string) (io.Closer, error)` in package main. Nothing else depends on it.

- [ ] **Step 1: Write the failing tests** (in `main_test.go`):

```go
func TestInitDebugLogDisabledIsNoOp(t *testing.T) {
	f, err := initDebugLog("")
	if err != nil || f != nil {
		t.Fatalf("disabled flag must be a no-op, got f=%v err=%v", f, err)
	}
}

func TestInitDebugLogOpensFile(t *testing.T) {
	// arrange
	cache := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cache)
	t.Cleanup(func() { log.SetOutput(os.Stderr) }) // tea.LogToFile redirects the stdlib logger

	// act
	f, err := initDebugLog("1")

	// assert
	if err != nil {
		t.Fatal(err)
	}
	if f == nil {
		t.Fatal("expected an open log file")
	}
	f.Close()
	if _, err := os.Stat(filepath.Join(cache, "weft", "weft.log")); err != nil {
		t.Fatalf("log file should exist: %v", err)
	}
}

// The user explicitly asked for logging; a swallowed open error means the
// request silently no-ops and the alt-screen hides that anything is wrong.
func TestInitDebugLogFailsWhenLogUnopenable(t *testing.T) {
	// arrange — a directory where the log file should be forces the open error
	cache := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cache)
	if err := os.MkdirAll(filepath.Join(cache, "weft", "weft.log"), 0o755); err != nil {
		t.Fatal(err)
	}

	// act + assert
	if _, err := initDebugLog("1"); err == nil {
		t.Fatal("expected an error when the debug log cannot be opened")
	}
}
```

- [ ] **Step 2: Verify RED** — `go test ./cmd/weft` → compile error (`initDebugLog` undefined). Record it.

- [ ] **Step 3: Implement.** In `main.go` (add `io` to imports):

```go
// initDebugLog wires the stdlib logger (Bubble Tea's debug mirror) to the
// weft debug log when the WEFT_DEBUG value enables it. Returns the file to
// close on exit. An open failure is returned, not swallowed: the user
// explicitly asked for logging, and pre-alt-screen is the one moment stderr
// can still tell them it isn't happening.
func initDebugLog(val string) (io.Closer, error) {
	if !debugLogEnabled(val) {
		return nil, nil
	}
	f, err := tea.LogToFile(views.DebugLogPath(), "weft")
	if err != nil {
		return nil, err
	}
	return f, nil
}
```

(the two-step return avoids a typed-nil `io.Closer` on the error path). Replace `main.go:100-104` with:

```go
	debugLog, err := initDebugLog(os.Getenv("WEFT_DEBUG"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "weft: WEFT_DEBUG is set but the debug log can't be opened: %v\n", err)
		os.Exit(2)
	}
	if debugLog != nil {
		defer debugLog.Close()
	}
```

- [ ] **Step 4: Verify green** — `go test ./cmd/weft` → PASS.

- [ ] **Step 5: Docs.** CHANGELOG `### Fixed`: `- WEFT_DEBUG=1 now exits with a clear error when the debug log can't be opened, instead of silently running without the logging you asked for.` Grep `README.md` and `AGENTS.md` for `WEFT_DEBUG`; update only if they describe the silent behavior (the AGENTS.md fallback-path sentence is about DebugLogPath and stays true).

- [ ] **Step 6: Gate + commit**

```bash
gofmt -l . && go vet ./... && "$(go env GOPATH)/bin/staticcheck" ./... && go test -race ./...
git rev-parse --abbrev-ref HEAD   # MUST print fix/backlog-top5
git add cmd/weft/main.go cmd/weft/main_test.go CHANGELOG.md
git commit -m "fix(cli): exit loudly when WEFT_DEBUG's log can't be opened"
```

---

### Task 5: Width-pad the task-marker sentinel

Wiki-link and emphasis sentinels pad to the restored text's display width (`internal/render/page.go:372-374`, `:576-578`) so Glamour's word-wrap reserves the right columns; the task sentinel doesn't (`:466-467`) — a 3-cell sentinel stands in for up to 9 cells of `CANCELLED`, so long-marker bullets render past the wrap width: spurious terminal soft-wraps and row drift in `ScrollToTask` (`internal/views/page.go:206-220`, which counts `\n` in the styled output).

**Files:**
- Modify: `internal/render/page.go` (`:90-99` consts, `:111-115` regex, `:444-470` preprocess)
- Test: `internal/render/page_test.go` (model on `TestRenderWikiLinkWrapsAtRightMargin` `:145-168`)
- Modify: `CHANGELOG.md`

**Interfaces:**
- Produces: nothing new externally — output-shape fix only. `sentinelRe` group indices must NOT shift (use a non-capturing pad group, like the other two alternatives).

- [ ] **Step 1: Write the failing test:**

```go
// A task marker renders as its literal text (WAITING = 7 cells, CANCELLED
// = 9) but its sentinel is ~3 cells, so Glamour under-reserves and the
// restored line can overflow the wrap width — spurious soft-wraps and
// ScrollToTask row drift. The sentinel must pad to the marker's width like
// the wiki-link and emphasis sentinels do.
func TestRenderTaskMarkerWrapsAtRightMargin(t *testing.T) {
	for _, marker := range []string{"WAITING", "CANCELLED"} {
		body := "- " + marker + " alpha beta gamma delta epsilon zeta eta theta iota\n"
		res := mustRender(t, body, 40)
		for _, line := range strings.Split(res.Styled, "\n") {
			if w := lipgloss.Width(line); w > 40 {
				t.Errorf("%s: line is %d cells wide, want <= 40: %q", marker, w, line)
			}
		}
	}
}
```

Match `mustRender`'s real signature (`page_test.go:14-33`). **The RED run must show actual overflow** — if both markers pass pre-fix, lengthen the filler words until at least one line exceeds 40, and only then proceed.

- [ ] **Step 2: Verify RED** — `go test ./internal/render -run TestRenderTaskMarkerWrapsAtRightMargin` → FAIL with lines wider than 40. Record the failing widths.

- [ ] **Step 3: Implement.** Add to the const block (`page.go:90-99`): `taskSentinelPad = "\ue008" // width-padding for the task sentinel (see wikiSentinelPad)` (E008 is free: E004 = wiki pad, E007 = emph pad, E010–E019 = digits; write it as the escape sequence, matching the neighboring consts) — and update the emphasis const's range comment if it enumerates used codepoints. Give the task alternative of `sentinelRe` (`page.go:113`) the same non-capturing pad group as the other two:

```go
		`|` + taskSentinelStart + `([\x{E010}-\x{E019}]+)` + taskSentinelEnd + `(?:` + taskSentinelPad + `)*` +
```

In `preprocessTaskMarkers` (`page.go:466-467`):

```go
		sentinel := taskSentinelStart + encodeSentinelID(id) + taskSentinelEnd
		// Pad to the marker's display width so Glamour's word-wrap reserves
		// the columns the restored marker text will occupy (same trick as
		// the wiki-link and emphasis sentinels, page.go:372).
		if pad := lipgloss.Width(marker) - lipgloss.Width(sentinel); pad > 0 {
			sentinel += strings.Repeat(taskSentinelPad, pad)
		}
		return prefix + sentinel + rest
```

(`lipgloss` is already imported in this file.)

- [ ] **Step 4: Verify green AND golden-neutral** — `go test -race ./internal/render ./internal/views` → PASS. The width-80 goldens (`TestPageViewRendersAlpha.golden` has short TODO/LATER bullets) must NOT change; a golden diff here means the padding altered a non-wrapping line — that's a bug, not a golden refresh. Only if a *legitimately wrapping* fixture line moved may you visually diff and regenerate with `-update`, noting it in the commit body.

- [ ] **Step 5: CHANGELOG** (`### Fixed`): `- Bullets with long task markers (WAITING, CANCELLED) no longer overflow the wrap width by a few columns, which caused stray soft-wrapped lines and slightly-off dashboard deep-link centering.`

- [ ] **Step 6: Gate + commit**

```bash
gofmt -l . && go vet ./... && "$(go env GOPATH)/bin/staticcheck" ./... && go test -race ./...
git rev-parse --abbrev-ref HEAD   # MUST print fix/backlog-top5
git add internal/render/page.go internal/render/page_test.go CHANGELOG.md
git commit -m "fix(render): width-pad the task-marker sentinel to stop wrap overflow"
```

---

### Task 6: Preserve scroll across the synchronous reindex

`App.reindex()` (`internal/views/app.go:322-332`) rebuilds the current `PageView` from scratch — offset 0, cursor -1 — while the async `indexLoadedMsg` path restores both (`app.go:535-541` via `Offset()`/`Cursor()`/`Restore()`, `internal/views/page.go:172,175,188-199`). After linkify (`y`) + `Esc` the page jumps to the top; the `.`-key journal-create path shares the gap.

**Files:**
- Modify: `internal/views/app.go:322-332`
- Test: `internal/views/app_test.go` (model on `TestReindexPreservesScrollPosition` `:265-295`)
- Modify: `CHANGELOG.md`

**Interfaces:**
- Consumes: `PageView.Offset() int`, `PageView.Cursor() int`, `PageView.Restore(offset, cursor int)` — all exist, unchanged.

- [ ] **Step 1: Write the failing test:**

```go
// reindex() — the synchronous path used by linkify and journal-create —
// rebuilt the PageView from scratch, discarding scroll offset and link
// cursor: after y+Esc the page jumped to the top. Sync twin of
// TestReindexPreservesScrollPosition, which pins the async path.
func TestSyncReindexPreservesScrollPosition(t *testing.T) {
	// arrange
	quietTerm(t)
	a := bootAppWithGraph(t, map[string]string{
		"pages/Long.md": strings.Repeat("- line\n", 80),
	})
	a.navigate("Long")
	for i := 0; i < 30; i++ {
		a.page.LineDown()
	}
	want := a.page.Offset()
	if want == 0 {
		t.Fatal("arrange failed: page did not scroll")
	}

	// act
	if err := a.reindex(); err != nil {
		t.Fatal(err)
	}

	// assert
	if got := a.page.Offset(); got != want {
		t.Fatalf("offset after sync reindex = %d, want %d", got, want)
	}
}
```

(Copy the exact scroll idiom from `TestReindexPreservesScrollPosition` at `app_test.go:265-295` — if it scrolls differently than `LineDown()`, match it.)

- [ ] **Step 2: Verify RED** — `go test ./internal/views -run TestSyncReindexPreservesScrollPosition` → FAIL with offset 0. Record it.

- [ ] **Step 3: Implement** — mirror the async path's restore in `reindex()`:

```go
	a.idx = idx
	if a.page != nil {
		// Same-page rebuild: keep the reader's place, exactly like the
		// async indexLoadedMsg path. Restore clamps if the page shrank.
		off, cur := a.page.Offset(), a.page.Cursor()
		a.page = NewPageView(a.idx, a.page.Page(), a.width, a.height)
		a.page.Restore(off, cur)
	}
	return nil
```

(If Task 2 already added the warnings block to this function, keep it — insert the restore around the existing `NewPageView` line only.)

- [ ] **Step 4: Verify green** — `go test -race ./internal/views` → PASS (linkify tests `app_test.go:178-253` included; they skip without `rg`, which is installed here).

- [ ] **Step 5: CHANGELOG** (`### Fixed`): `- Linkifying a mention (and creating today's journal) no longer resets the page to the top — scroll position and link cursor survive the rebuild.`

- [ ] **Step 6: Gate + commit**

```bash
gofmt -l . && go vet ./... && "$(go env GOPATH)/bin/staticcheck" ./... && go test -race ./...
git rev-parse --abbrev-ref HEAD   # MUST print fix/backlog-top5
git add internal/views/app.go internal/views/app_test.go CHANGELOG.md
git commit -m "fix(views): preserve scroll and cursor across the synchronous reindex"
```

---

## After all tasks

Whole-branch final review (superpowers:requesting-code-review), then PR to `main` on git.fiatcode.dev via the forgejo skill — `tea pr create` fails from a linked worktree; use the skill's curl fallback with `git credential fill`. Then update `${WEFT_GRAPH}/pages/weft Backlog.md` (move the five finished items out of Open, keep its voice) and journal via weft:journal-update.
