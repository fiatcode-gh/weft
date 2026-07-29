# Quality-Audit Fixes Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix the confirmed findings of the 2026-07-29 full-repo quality audit: three silent data-corruption bugs (case-mismatch duplicate files, editor sanitizer corruption, single-line embed swallowing pages), four sync/async robustness defects, two reachable dependency vulnerabilities, two critical test gaps, and README/comment drift.

**Architecture:** Every fix is small and local; no new packages or types beyond one bool field and one helper method. Work happens on one branch (`fix/quality-audit-sweep`) with one conventional commit per task, shipped as a single sweep PR on git.fiatcode.dev (repo precedent: PR #32). Tasks 1–8 are strict TDD; task 9 is a dependency bump gated on goldens; task 10 is docs-only.

**Tech Stack:** Go 1.26, Bubble Tea / bubbles / lipgloss, teatest goldens, ripgrep, git via os/exec.

## Global Constraints

- Run `go vet ./... && go test ./...` before **every** commit — both must pass (AGENTS.md).
- `gofmt -l .` must be empty; CI also runs `staticcheck ./...` and `go test -race ./...`.
- Tests must never point at the real graph (`~/Documents/fiat-codex`); use `writeGraph` / `cloneFixtureGraph` helpers in `internal/views/helpers_test.go` or the `internal/sync` repo fixtures.
- Test bodies use `// arrange`, `// act`, `// assert` comments; shared setup goes into helpers, not copy-paste (user's global TDD/clean-code rule).
- Conventional Commits (`fix:`, `test:`, `chore:`, `docs:`).
- teatest goldens live in `internal/views/testdata/`; after an intentional UI change run `go test ./... -update` and visually diff before staging. None of these tasks should change goldens except possibly Task 9.
- Comments state constraints the code can't show — no narration, no changelog-speak (repo convention from PR #33).

---

### Task 1: Canonicalize page names at the navigation boundary

Following `[[weft]]` when the file is `Weft.md` renders fine (case-insensitive `Resolve`) but stores the raw name; `e` then misses the exact-match `ByName` lookup, opens an empty buffer, and `Ctrl+S` creates a case-colliding `pages/weft.md`. `E` shows a bogus "page not in index" hint.

**Files:**
- Modify: `internal/views/app.go` (`navigateToTask` ~line 230, `navigateFocusingLink` ~242, `navigateHighlighting` ~253, `editCurrent` ~328, `enterEditor` ~391; new helper `canonicalName`)
- Test: `internal/views/app_canonical_test.go` (new)

**Interfaces:**
- Consumes: `(*graph.Index).Resolve(name string) (*graph.PageMeta, bool)` (`internal/graph/resolve.go:55`), `writeGraph` / `quietTerm` helpers.
- Produces: `(*App).canonicalName(name string) string` — used only within app.go; `bootAppWithGraph(t, files)` test helper reused by Task 2's app-level test.

- [ ] **Step 1: Write the failing tests**

Create `internal/views/app_canonical_test.go`:

```go
package views

import (
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// bootAppWithGraph boots an App against a throwaway graph built from
// name→content entries, mirroring bootApp's production boot sequence.
func bootAppWithGraph(t *testing.T, files map[string]string) *App {
	t.Helper()
	quietTerm(t)
	dir, _ := writeGraph(t, files)
	a := New(dir, "test")
	cmd := a.Init()
	if cmd == nil {
		t.Fatal("Init returned nil cmd")
	}
	a.Update(cmd())
	a.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	if a.page == nil {
		t.Fatal("PageView not constructed after boot")
	}
	return a
}

func TestNavigateCanonicalizesCaseMismatchedTarget(t *testing.T) {
	// arrange: the file is Weft.md; a link elsewhere says [[weft]].
	a := bootAppWithGraph(t, map[string]string{
		"pages/Weft.md": "- the real page\n",
	})

	// act
	a.navigate("weft")

	// assert: the view adopts the indexed page's exact name; an
	// unindexed name still passes through untouched (lazy creation).
	if got := a.page.Page(); got != "Weft" {
		t.Errorf("page name = %q, want canonical %q", got, "Weft")
	}
	a.navigate("Brand New")
	if got := a.page.Page(); got != "Brand New" {
		t.Errorf("unindexed name = %q, want pass-through %q", got, "Brand New")
	}
}

func TestEnterEditorOnCaseMismatchedTargetOpensExistingFile(t *testing.T) {
	// arrange
	a := bootAppWithGraph(t, map[string]string{
		"pages/Weft.md": "- existing content\n",
	})
	a.navigate("weft")

	// act
	_ = a.enterEditor()

	// assert: the editor must target the existing file with its content,
	// not a fresh case-colliding pages/weft.md.
	if a.editor == nil {
		t.Fatal("editor did not open")
	}
	if got := filepath.Base(a.editor.path); got != "Weft.md" {
		t.Errorf("editor path = %q, want Weft.md", got)
	}
	if a.editor.isNew {
		t.Error("existing page treated as new")
	}
	if !strings.Contains(a.editor.Content(), "existing content") {
		t.Error("existing content not loaded into the editor")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/views -run 'TestNavigateCanonicalizes|TestEnterEditorOnCaseMismatched' -v`
Expected: both FAIL — page name stays `"weft"`, editor path is `weft.md` with `isNew` true.

- [ ] **Step 3: Implement**

In `internal/views/app.go`, add near `navigate`:

```go
// canonicalName maps a raw wiki-link target to the indexed page's exact
// (filename-derived) name, so downstream exact-match lookups agree with
// the case-insensitive Resolve the read view uses. Unindexed names pass
// through unchanged — they name pages that don't exist yet.
func (a *App) canonicalName(name string) string {
	if meta, ok := a.idx.Resolve(name); ok {
		return meta.Name
	}
	return name
}
```

Add `name = a.canonicalName(name)` as the first line of `navigateToTask`, `navigateFocusingLink`, and `navigateHighlighting` (all navigation funnels through these three; `navigate` delegates to `navigateToTask`).

Defense-in-depth for any remaining raw-name path: in `editCurrent` replace `meta, ok := a.idx.ByName[page]` with `meta, ok := a.idx.Resolve(page)`, and inside its post-reindex recheck replace `newMeta, ok := a.idx.ByName[page]` with `newMeta, ok := a.idx.Resolve(page)`. In `enterEditor` replace `if meta, ok := a.idx.ByName[name]; ok` with `if meta, ok := a.idx.Resolve(name); ok`.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/views -run 'TestNavigateCanonicalizes|TestEnterEditorOnCaseMismatched' -v` → PASS, then `go vet ./... && go test ./...` → all pass.

- [ ] **Step 5: Commit**

```bash
git add internal/views/app.go internal/views/app_canonical_test.go
git commit -m "fix: canonicalize wiki-link targets at navigation, resolve case-insensitively in editors"
```

---

### Task 2: Refuse in-app editing when the textarea would alter the content

`bubbles/textarea`'s sanitizer silently rewrites content on load: CRLF → doubled newlines, tabs → 4 spaces, invalid UTF-8 dropped, and a package-level 10 000-line cap truncates (`MaxHeight = 0` does **not** lift it — the comment on `editor.go:62` is wrong). `baseline` is captured post-mutation, so `dirty()` gives no warning and `Ctrl+S` corrupts the file.

**Files:**
- Modify: `internal/views/editor.go` (`EditorView` struct ~line 35, `NewEditorView` ~59, fix comment at 62)
- Modify: `internal/views/app.go` (`enterEditor` ~406)
- Test: `internal/views/edit_test.go` (append), `internal/views/app_canonical_test.go` (reuse `bootAppWithGraph`)

**Interfaces:**
- Consumes: `bootAppWithGraph` from Task 1; `NewEditorView(idx, name, path, content, isNew, w, h)`; `(*EditorView).Content()`.
- Produces: `(*EditorView).LoadDiverged() bool`.

- [ ] **Step 1: Write the failing tests**

Append to `internal/views/edit_test.go`:

```go
func TestNewEditorViewFlagsSanitizerDivergence(t *testing.T) {
	// arrange
	cases := []struct {
		name    string
		content string
		want    bool
	}{
		{"clean", "- a\n- b\n", false},
		{"no trailing newline", "- a", false},
		{"empty new page", "", false},
		{"crlf", "a\r\nb\r\n", true},
		{"tab indent", "code:\n\tindented\n", true},
		{"over textarea line cap", strings.Repeat("x\n", 10001), true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// act
			e := NewEditorView(nil, "P", "unused.md", tc.content, false, 80, 24)

			// assert
			if got := e.LoadDiverged(); got != tc.want {
				t.Errorf("LoadDiverged = %v, want %v", got, tc.want)
			}
		})
	}
}
```

Append to `internal/views/app_canonical_test.go`:

```go
func TestEnterEditorRefusesFileTheTextareaWouldAlter(t *testing.T) {
	// arrange: a CRLF file — the sanitizer would double every line break.
	a := bootAppWithGraph(t, map[string]string{
		"pages/Crlf.md": "a\r\nb\r\n",
	})
	a.navigate("Crlf")

	// act
	_ = a.enterEditor()

	// assert: refused, with a hint pointing at the $EDITOR handoff.
	if a.editor != nil {
		t.Fatal("editor opened on a file it would corrupt")
	}
	if !strings.Contains(a.hint, "E") {
		t.Errorf("hint = %q, want a pointer to E / $EDITOR", a.hint)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/views -run 'TestNewEditorViewFlagsSanitizerDivergence|TestEnterEditorRefuses' -v`
Expected: compile FAIL (`LoadDiverged` undefined).

- [ ] **Step 3: Implement**

In `internal/views/editor.go`: add field `loadDiverged bool` to `EditorView`. Fix the wrong comment on the `MaxHeight` line:

```go
ta.MaxHeight = 0 // lift textarea's default 99-line height cap (a separate hard 10000-line insert cap remains — see loadDiverged)
```

In `NewEditorView`, immediately after `e.baseline = e.Content()`:

```go
// The textarea's input sanitizer can silently alter content on load —
// CRLF becomes doubled newlines, tabs become spaces, invalid UTF-8 is
// dropped, and a hard 10000-line cap truncates. baseline was captured
// post-mutation, so dirty() can't warn; saving would corrupt the file.
// Record the divergence so the App can refuse in-app editing.
e.loadDiverged = e.Content() != strings.TrimRight(content, "\n")+"\n"
```

Add accessor below `Content()`:

```go
// LoadDiverged reports whether priming the textarea altered the loaded
// content; a diverged buffer must never be written back over the file.
func (e *EditorView) LoadDiverged() bool { return e.loadDiverged }
```

In `internal/views/app.go` `enterEditor`, replace the last two lines (`a.editor = NewEditorView(...)` / `return a.editor.Focus()`) with:

```go
e := NewEditorView(a.idx, name, path, content, isNew, a.width, a.height)
if e.LoadDiverged() {
	return a.setHint("in-app editor would alter this file (CRLF, tabs, or >10000 lines) — press E to edit externally")
}
a.editor = e
return a.editor.Focus()
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/views -run 'TestNewEditorViewFlagsSanitizerDivergence|TestEnterEditorRefuses' -v` → PASS, then `go vet ./... && go test ./...` → all pass.

- [ ] **Step 5: Commit**

```bash
git add internal/views/editor.go internal/views/app.go internal/views/edit_test.go internal/views/app_canonical_test.go
git commit -m "fix: refuse in-app editing when the textarea sanitizer would corrupt the file"
```

---

### Task 3: Single-line `{{query}}` / `{{embed}}` must not swallow the rest of the page

`stripQueryAndEmbedBlocks` sets `inBlock` on any opener line without checking for a same-line `}}` close, so `{{embed [[X]]}}` (single-line in essentially every case) drops everything to EOF: links dead, task deep-links misaligned, page renders blank.

**Files:**
- Modify: `internal/render/page.go` (`stripQueryAndEmbedBlocks`, the `queryOrEmbedRe` case at ~line 281)
- Test: `internal/render/page_test.go` (append near `TestRenderPageStripsQueryAndEmbedBlocks` at line 258)

**Interfaces:**
- Consumes: existing test helpers `mustRender(t, body, width)` and `plainText(out)` in `page_test.go`.
- Produces: nothing new — behavior fix only.

- [ ] **Step 1: Write the failing test**

```go
func TestRenderSingleLineQueryOrEmbedDropsOnlyItsLine(t *testing.T) {
	// arrange: single-line embed and query, with real content after them.
	body := "{{embed [[Other]]}}\n\n- TODO one\n- TODO two\n\n{{query (todo)}}\nSee [[Somewhere]] for details.\n"

	// act
	out := mustRender(t, body, 80)

	// assert
	plain := plainText(out)
	if strings.Contains(plain, "{{embed") || strings.Contains(plain, "{{query") {
		t.Errorf("marker line leaked into output:\n%s", plain)
	}
	for _, want := range []string{"one", "two", "Somewhere"} {
		if !strings.Contains(plain, want) {
			t.Errorf("content after a single-line block was dropped (missing %q):\n%s", want, plain)
		}
	}
	if len(out.Tasks) != 2 {
		t.Errorf("Tasks = %d, want 2 (todos-dashboard deep-link ordinals depend on this)", len(out.Tasks))
	}
	if len(out.Links) != 1 {
		t.Errorf("Links = %d, want 1 ([[Somewhere]])", len(out.Links))
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/render -run TestRenderSingleLineQueryOrEmbed -v`
Expected: FAIL — everything after the first line is missing, `Tasks = 0`, `Links = 0`.

- [ ] **Step 3: Implement**

In `stripQueryAndEmbedBlocks`, replace the opener case:

```go
case queryOrEmbedRe.MatchString(line):
	// A same-line `}}` closes the block immediately — `{{embed [[X]]}}`
	// is single-line in practice; without this check the rest of the
	// page is swallowed waiting for a bare `}}` line that never comes.
	if !strings.HasSuffix(strings.TrimRight(line, " \t"), "}}") {
		inBlock = true
	}
	continue // drop the opening line either way
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/render -v` (the existing multi-line strip test at line 258 must also still pass), then `go vet ./... && go test ./...`.

- [ ] **Step 5: Commit**

```bash
git add internal/render/page.go internal/render/page_test.go
git commit -m "fix: single-line {{query}}/{{embed}} no longer swallows the rest of the page"
```

---

### Task 4: Sync failure log must include the error; degrade the hint when the log is unwritable

`logSyncFailure` writes only `Stage` + `Output`; `Result.Err` (the only guaranteed-non-nil field — `context deadline exceeded` vs auth vs missing git) is dropped. And when the log itself can't be opened, the hint still says "see weft.log" — pointing at a file that was never written.

**Files:**
- Modify: `internal/views/app.go` (`logSyncFailure` ~line 869, `syncDoneMsg` branch ~530)
- Test: `internal/views/app_sync_test.go` (append)

**Interfaces:**
- Consumes: `DebugLogPath()` (controlled in tests via `t.Setenv("XDG_CACHE_HOME", …)` — `os.UserCacheDir` honors it on Linux), `bootApp`, `errSyncTest`, `syncpkg.Result`.
- Produces: `logSyncFailure` now returns `error`.

- [ ] **Step 1: Write the failing tests**

Append to `internal/views/app_sync_test.go`:

```go
func TestSyncFailureLogIncludesError(t *testing.T) {
	// arrange: a private cache dir so the test owns weft.log.
	cache := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cache)
	a := bootApp(t)

	// act: a push killed by timeout — empty Output, the cause only in Err.
	a.Update(syncDoneMsg{res: syncpkg.Result{Stage: "push", Output: "", Err: errSyncTest}})

	// assert
	b, err := os.ReadFile(filepath.Join(cache, "weft", "weft.log"))
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	if !strings.Contains(string(b), errSyncTest.Error()) {
		t.Errorf("log %q does not record the sync error", string(b))
	}
}

func TestSyncFailureHintFallsBackInlineWhenLogUnwritable(t *testing.T) {
	// arrange: make the log path unopenable — a directory where the
	// file should be — without touching the checkout's cwd.
	cache := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cache)
	if err := os.MkdirAll(filepath.Join(cache, "weft", "weft.log"), 0o755); err != nil {
		t.Fatal(err)
	}
	a := bootApp(t)

	// act
	model, _ := a.Update(syncDoneMsg{res: syncpkg.Result{Stage: "push", Err: errSyncTest}})
	a = model.(*App)

	// assert: the hint must carry the error itself, not point at a log
	// that was never written.
	if !strings.Contains(a.hint, errSyncTest.Error()) {
		t.Errorf("hint = %q, want inline error", a.hint)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/views -run 'TestSyncFailureLogIncludesError|TestSyncFailureHintFallsBack' -v`
Expected: first FAILs (log lacks "boom"), second FAILs (hint says "see …weft.log").

- [ ] **Step 3: Implement**

Replace `logSyncFailure`:

```go
// logSyncFailure appends a failing sync's stage, error, and captured git
// output to DebugLogPath(), the same cache-dir path WEFT_DEBUG mirrors
// to — written regardless of the flag so the hint's "see <path>" pointer
// is valid. res.Err matters most: a timed-out push often has no output,
// and the error is the only way to tell timeout from rejection. Returns
// the write error so the caller can degrade the hint instead of pointing
// at a log that was never written.
func (a *App) logSyncFailure(res syncpkg.Result) error {
	f, err := os.OpenFile(DebugLogPath(), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = fmt.Fprintf(f, "sync %s failed at %s: %v\n%s\n",
		res.Stage, a.nowFunc().Format(time.RFC3339), res.Err, res.Output)
	return err
}
```

In the `syncDoneMsg` branch, replace the failure arm:

```go
if m.res.Err != nil {
	hint := "✗ " + m.res.Stage + " failed — see " + DebugLogPath()
	if err := a.logSyncFailure(m.res); err != nil {
		hint = "✗ " + m.res.Stage + " failed: " + m.res.Err.Error()
	}
	// A failed sync may still have changed state (e.g. committed
	// then failed to push), so re-probe.
	return a, tea.Batch(a.setHint(hint), a.statusProbeCmd())
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/views -run 'TestSyncFailure' -v` (includes the pre-existing `TestSyncFailureHintNamesStage`), then `go vet ./... && go test ./...`.

- [ ] **Step 5: Commit**

```bash
git add internal/views/app.go internal/views/app_sync_test.go
git commit -m "fix: record the sync error in weft.log; inline it in the hint when the log is unwritable"
```

---

### Task 5: A failed status probe must not clear the unsynced indicator

`a.unsynced = m.err == nil && m.st.Unsynced()` collapses every probe failure (corrupted `.git/index`, timeout) into the reassuring "no ●" state.

**Files:**
- Modify: `internal/views/app.go` (`statusProbedMsg` branch, line ~547)
- Test: `internal/views/app_sync_test.go` (append)

**Interfaces:** consumes `bootApp`, `statusProbedMsg{err: …}`; produces nothing new.

- [ ] **Step 1: Write the failing test**

```go
func TestStatusProbeErrorKeepsLastKnownIndicator(t *testing.T) {
	// arrange: last successful probe said "unsynced".
	a := bootApp(t)
	a.unsynced = true

	// act: a probe failure says nothing about actual sync state.
	model, _ := a.Update(statusProbedMsg{err: errSyncTest})
	a = model.(*App)

	// assert
	if !a.unsynced {
		t.Error("probe error cleared the unsynced indicator to a false all-clear")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/views -run TestStatusProbeErrorKeepsLastKnown -v` → FAIL.

- [ ] **Step 3: Implement**

```go
case statusProbedMsg:
	// A failed probe says nothing about sync state; keep the last
	// known value rather than clearing the ● to a false all-clear.
	if m.err == nil {
		a.unsynced = m.st.Unsynced()
	}
	return a, nil
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/views -run 'TestStatusProbe|TestSync' -v`, then `go vet ./... && go test ./...`.

- [ ] **Step 5: Commit**

```bash
git add internal/views/app.go internal/views/app_sync_test.go
git commit -m "fix: keep last known sync indicator when the status probe fails"
```

---

### Task 6: Non-zero `$EDITOR` exit must still run the mtime-gated refresh

`editorExitedMsg` returns early on `m.err != nil`, but a non-zero exit doesn't mean nothing was written (vim `:w` then `:cq`): the file changed, the reindex is skipped, and the read view shows stale content.

**Files:**
- Modify: `internal/views/app.go` (`editorExitedMsg` branch, lines ~555-572)
- Test: `internal/views/app_sync_test.go` (append — it already has `drainFor`)

**Interfaces:** consumes `bootApp`, `drainFor[indexLoadedMsg]`, `editorExitedMsg{path, t0, err}`; produces nothing new.

- [ ] **Step 1: Write the failing test**

```go
func TestEditorErrorExitStillReindexesChangedFile(t *testing.T) {
	// arrange: an indexed page whose on-disk mtime differs from the
	// snapshot taken at editor launch — i.e. the editor wrote the file.
	a := bootApp(t)
	path := a.idx.Pages[0].Path
	t0 := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)

	// act: the editor exits non-zero (vim :cq after :w).
	model, cmd := a.Update(editorExitedMsg{path: path, t0: t0, err: errSyncTest})
	a = model.(*App)

	// assert: the exit is surfaced AND the reindex still runs.
	if !strings.Contains(a.hint, "editor exited") {
		t.Errorf("hint = %q, want editor-exited", a.hint)
	}
	drainFor[indexLoadedMsg](t, cmd)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/views -run TestEditorErrorExitStillReindexes -v`
Expected: FAIL — `drainFor` times out because the early return dropped the reindex.

- [ ] **Step 3: Implement**

Replace the `editorExitedMsg` branch:

```go
case editorExitedMsg:
	// A non-zero exit (vim :cq, a crashed wrapper) does not mean nothing
	// was written — surface it, but still run the mtime-gated refresh
	// below so a real save isn't left rendering stale.
	var exitHint tea.Cmd
	if m.err != nil {
		exitHint = a.setHint("editor exited: " + m.err.Error())
	}
	info, err := os.Stat(m.path)
	if err != nil {
		if os.IsNotExist(err) {
			return a, tea.Batch(exitHint, a.statusProbeCmd())
		}
		return a, tea.Batch(exitHint, a.setHint("cannot stat: "+err.Error()))
	}
	if info.ModTime().Equal(m.t0) {
		// Unchanged by the editor — but editCurrent may have just
		// created a journal stub, so still refresh the indicator.
		return a, tea.Batch(exitHint, a.statusProbeCmd())
	}
	// Changed: the reindex's indexLoadedMsg refreshes the indicator.
	return a, tea.Batch(exitHint, a.buildIndexCmd())
```

(`tea.Batch` ignores nil cmds, so the success path is unchanged.)

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/views -v`, then `go vet ./... && go test ./...`. The `E`-handoff integration tests (fake-editor.sh) must still pass.

- [ ] **Step 5: Commit**

```bash
git add internal/views/app.go internal/views/app_sync_test.go
git commit -m "fix: reindex after a non-zero editor exit when the file changed"
```

---

### Task 7: Picker `＋ Create` must respect the sync-busy gate

The `res.Create` branch reaches `enterEditor()` without `syncBusyHint()` — the only graph-mutating path reachable during `pull --rebase`; a save there is silently overwritten by the rebase checkout (the loss `syncBusyHint`'s own comment describes).

**Files:**
- Modify: `internal/views/app.go` (`res.Create` branch, lines ~658-662)
- Test: `internal/views/app_sync_test.go` (append)

**Interfaces:** consumes the `Overlay` interface (`internal/views/overlay.go`: `Update(key string) OverlayResult; View() string; SetSize(w, h int)`); produces a `stubOverlay` test type.

- [ ] **Step 1: Write the failing test**

```go
// stubOverlay drives App.Update's overlay-dispatch branch with a canned
// OverlayResult, standing in for a real picker/backlinks overlay.
type stubOverlay struct{ res OverlayResult }

func (s stubOverlay) Update(string) OverlayResult { return s.res }
func (s stubOverlay) View() string                { return "" }
func (s stubOverlay) SetSize(int, int)            {}

func TestPickerCreateBlockedWhileSyncing(t *testing.T) {
	// arrange: a sync in flight and a picker about to create a page.
	a := bootApp(t)
	a.syncing = true
	a.active = stubOverlay{res: OverlayResult{Accept: true, Create: true, Selected: "Brand New"}}

	// act
	model, _ := a.Update(tea.KeyMsg{Type: tea.KeyEnter})
	a = model.(*App)

	// assert: no editor mid-rebase, same contract as e/E/./linkify.
	if a.editor != nil {
		t.Fatal("create opened the editor during a sync")
	}
	if !strings.Contains(a.hint, "sync in progress") {
		t.Errorf("hint = %q, want sync-in-progress", a.hint)
	}
}
```

(Needs `tea "github.com/charmbracelet/bubbletea"` — already imported in `app_sync_test.go`.)

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/views -run TestPickerCreateBlockedWhileSyncing -v`
Expected: FAIL — the editor opens.

- [ ] **Step 3: Implement**

Mirror the `res.Linkify` gate three lines above:

```go
if res.Create {
	if cmd := a.syncBusyHint(); cmd != nil {
		return a, cmd
	}
	a.navigate(res.Selected)
	a.active = nil
	return a, a.enterEditor()
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/views -run 'TestPickerCreate' -v` (existing picker-create tests must still pass), then `go vet ./... && go test ./...`.

- [ ] **Step 5: Commit**

```bash
git add internal/views/app.go internal/views/app_sync_test.go
git commit -m "fix: gate picker page-creation behind the sync-busy check"
```

---

### Task 8: Close the two critical test gaps (sync commit stage, search span clamps)

Test-only task. (a) `sync.Run`'s commit-stage failure — the "fresh machine, no git identity" case — has zero coverage; (b) `parseJSON`'s span clamps (`search.go:122-130`) guard an alt-screen panic and are fully uncovered.

**Files:**
- Test: `internal/sync/sync_test.go` (append helper + test)
- Test: `internal/search/search_test.go` (append)

**Interfaces:** consumes `git(t, dir, args...)`, `writeFile`, `testClock` (`internal/sync/sync_test.go`); `parseJSON([]byte) ([]Hit, error)` and the `Span{Start, End}` type (`internal/search`). Produces `newRepoWithRemoteNoIdentity` helper.

- [ ] **Step 1: Write the sync commit-failure test**

Append to `internal/sync/sync_test.go`:

```go
// newRepoWithRemoteNoIdentity is newRepoWithRemote minus the local
// user.name/user.email config, for tests exercising commit failure on a
// machine with no git identity. The seed commit's identity comes from
// the env the git() helper injects.
func newRepoWithRemoteNoIdentity(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	origin := filepath.Join(root, "origin.git")
	work := filepath.Join(root, "work")
	git(t, root, "init", "--bare", "-b", "main", origin)
	git(t, root, "clone", origin, work)
	writeFile(t, work, "seed.md", "seed\n")
	git(t, work, "add", "-A")
	git(t, work, "commit", "-m", "seed")
	git(t, work, "push", "origin", "main")
	return work
}

func TestRunCommitFailureReportsCommitStage(t *testing.T) {
	// arrange: a dirty tree and no committer identity anywhere — the
	// fresh-machine failure mode. Run()'s git inherits the test process
	// env, so blank every identity source it consults.
	work := newRepoWithRemoteNoIdentity(t)
	writeFile(t, work, "new.md", "hi\n")
	t.Setenv("GIT_CONFIG_GLOBAL", "/dev/null")
	t.Setenv("GIT_CONFIG_SYSTEM", "/dev/null")
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("EMAIL", "")
	t.Setenv("GIT_AUTHOR_NAME", "")
	t.Setenv("GIT_AUTHOR_EMAIL", "")
	t.Setenv("GIT_COMMITTER_NAME", "")
	t.Setenv("GIT_COMMITTER_EMAIL", "")

	// act
	res := Run(work, testClock)

	// assert: the stage tells the user what to fix in a shell.
	if res.Err == nil || res.Stage != "commit" {
		t.Fatalf("Stage = %q, Err = %v; want commit failure", res.Stage, res.Err)
	}
	if res.Committed {
		t.Error("Committed must be false when commit fails")
	}
	if res.Output == "" {
		t.Error("expected captured git output naming the identity problem")
	}
}
```

- [ ] **Step 2: Run it, verify it exercises the intended branch**

Run: `go test ./internal/sync -run TestRunCommitFailureReportsCommitStage -v`
Expected: PASS (it pins existing correct behavior). Sanity-check it fails the right way by temporarily asserting `res.Stage == "push"` — expect FAIL with `Stage = "commit"` — then restore. If the test itself fails because git still finds an identity, inspect `res` — the env list above must fully cover it.

- [ ] **Step 3: Write the parseJSON clamp test**

Append to `internal/search/search_test.go`:

```go
func TestParseJSONClampsIndentShiftedSpans(t *testing.T) {
	// arrange: 4 leading spaces are trimmed from the context, shifting
	// spans left by 4. After the shift: {0,3}→{-4,-1} falls entirely in
	// the indent (dropped), {2,6}→{-2,2} straddles it (clamped to {0,2}),
	// {8,99}→{4,95} overhangs the line (clamped to {4,6}), {5,5}→{1,1}
	// is degenerate (dropped). A regression here slices Hit.Context out
	// of range and panics inside the alt-screen.
	line := `{"type":"match","data":{"path":{"text":"pages/X.md"},"lines":{"text":"    abcdef\n"},"line_number":3,"submatches":[{"start":0,"end":3},{"start":2,"end":6},{"start":8,"end":99},{"start":5,"end":5}]}}` + "\n"

	// act
	hits, err := parseJSON([]byte(line))

	// assert
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 1 {
		t.Fatalf("hits = %d, want 1", len(hits))
	}
	want := []Span{{Start: 0, End: 2}, {Start: 4, End: 6}}
	got := hits[0].Matches
	if len(got) != len(want) {
		t.Fatalf("spans = %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("span[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
	for _, sp := range got {
		if sp.Start < 0 || sp.End > len(hits[0].Context) || sp.Start >= sp.End {
			t.Errorf("span %+v can slice Context %q out of range", sp, hits[0].Context)
		}
	}
}
```

- [ ] **Step 4: Run the full gate**

Run: `go test ./internal/search -run TestParseJSONClamps -v` → PASS, then `go vet ./... && go test ./...`.

- [ ] **Step 5: Commit**

```bash
git add internal/sync/sync_test.go internal/search/search_test.go
git commit -m "test: pin sync commit-stage failure reporting and search span clamping"
```

---

### Task 9: Bump the two vulnerable dependencies

`govulncheck` reports two vulnerabilities reachable from weft: `golang.org/x/text` v0.30.0 (GO-2026-5970, infinite loop, fixed v0.39.0) and `github.com/yuin/goldmark` v1.7.13 (GO-2026-5320, XSS, fixed v1.7.17). Bump to exactly the fixed versions — not `@latest` — to minimize render-behavior churn (goldmark v1.8 and glamour v1.0 are out; upgrading those is a separate decision).

**Files:**
- Modify: `go.mod`, `go.sum`

- [ ] **Step 1: Bump**

```bash
go get golang.org/x/text@v0.39.0 github.com/yuin/goldmark@v1.7.17
go mod tidy
```

- [ ] **Step 2: Verify the vulns are gone**

Run: `go run golang.org/x/vuln/cmd/govulncheck@latest ./...`
Expected: "No vulnerabilities found" affecting weft's code (informational module-level findings are acceptable).

- [ ] **Step 3: Full gate + golden check**

Run: `go vet ./... && go test ./... && gofmt -l .`
Expected: all pass with **no golden changes**. If a `teatest` golden fails, run the failing test with `-update`, `git diff` the golden, and visually confirm the change is a benign goldmark rendering difference before staging — if it's not obviously benign, stop and report instead of updating.

- [ ] **Step 4: Commit**

```bash
git add go.mod go.sum
git commit -m "chore: bump x/text and goldmark to patched versions (GO-2026-5970, GO-2026-5320)"
```

---

### Task 10: Fix README/AGENTS.md drift and four stale comments

Docs-only. Every item below was verified against the code during the audit.

**Files:**
- Modify: `README.md`, `AGENTS.md`, `internal/views/editor.go`, `internal/edit/editor.go`, `internal/views/app.go`

- [ ] **Step 1: README fixes**

1. **Env table, `WEFT_STYLE` row** (~line 79) — replace the row's Effect cell with:
   `Force a Glamour markdown style (`dark`, `light`, `ascii`, `notty`) instead of weft's default terminal-palette style, which follows your terminal's own colors.`
2. **"What is not supported", block-references bullet** (~line 94) — replace with:
   `- **Block references** don't exist — `#` is an ordinary character in page names, so `[[page#block]]` links to a page literally named `page#block`.`
3. **Same list, case-insensitivity bullet** (~line 98) — replace with:
   `- **Case-insensitive page-name uniqueness** is not enforced — `Alpha.md` and `alpha.md` can coexist (weft warns on stderr and resolves links deterministically to the first). Link resolution itself is case-insensitive: `[[alpha]]` finds `Alpha`.`
4. **Install section** (~line 15) — replace the requirement sentence with:
   `Requires Go 1.26+ and [ripgrep](https://github.com/BurntSushi/ripgrep) on PATH (the search view consumes rg's `--json` output).`
   (The "added in rg 14" claim is wrong — `--json` dates to rg 0.10.)
5. **Intro** (~line 5): "a `＋ Create` row to drop a red link in one keystroke" → "a `＋ Create` row to insert a link to the not-yet-created page in one keystroke". (All links render identically; nothing is red.)

- [ ] **Step 2: AGENTS.md fix**

Line ~13: "an unmatched name offers a create row that inserts a red link without writing to disk" → "an unmatched name offers a create row that inserts a link to the not-yet-created page without writing to disk".

- [ ] **Step 3: Comment fixes**

1. `internal/views/editor.go` ~line 240 (Update doc): "In editing mode every key except the intercepts (ctrl+s, esc/ctrl+c, pgup/pgdown) is forwarded to the textarea" → "In editing mode every key except the intercepts (ctrl+s, esc/ctrl+c, pgup/pgdown, the markdown helpers enter/ctrl+t/tab/shift+tab, and the completion-strip keys while it is open) is forwarded to the textarea".
2. `internal/views/editor.go` ~line 22 (`editorInset` doc): "whose standard-style document margin is 2 columns" → "whose document margin is 2 columns".
3. `internal/edit/editor.go` ~line 140 (`SnapshotMtime` doc): "before the user pressed e" → "before the user pressed E".
4. `internal/views/app.go` ~line 46 (`editorExitedMsg` doc): "reindexes inline in the res.Exit branch" → "schedules an async reindex in the res.Exit branch".

- [ ] **Step 4: Gate and commit**

Run: `go vet ./... && go test ./... && gofmt -l .` (comment-only Go changes — must all pass untouched).

```bash
git add README.md AGENTS.md internal/views/editor.go internal/edit/editor.go internal/views/app.go
git commit -m "docs: fix README/AGENTS drift (WEFT_STYLE default, block refs, case-folding, rg floor) and stale comments"
```

---

## Finish

After all tasks: run the full gate one final time (`gofmt -l .` empty, `go vet ./...`, `staticcheck ./...`, `go test -race ./...`, `go build ./...` — the exact CI set), then open a single PR on git.fiatcode.dev via the forgejo skill:

- Title: `fix: quality-audit sweep — data-corruption, sync robustness, deps, docs`
- Body: summarize per-task, referencing the audit findings.

Out of scope (deferred to backlog, not this plan): index warnings surfaced in-app instead of stderr, Glamour render-failure caching, `WEFT_DEBUG` silent log-open failure, task-sentinel width padding, linkify scroll restore, `OverlayResult` constructors, shared open-task predicate, glamour v1 major bump.
