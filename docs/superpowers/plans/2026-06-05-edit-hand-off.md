# Editor Hand-off (`e`) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add an `e` key in the page view that hands the current page's `.md` file off to the user's editor (`$VISUAL` → `$EDITOR` → `/usr/bin/vi`), then resumes the TUI and reindexes the graph if and only if the file's mtime changed. Empty files are auto-created for today's journal if it doesn't exist on disk yet.

**Architecture:** A new `internal/edit` package owns the only disk-writing surface in the project (`Resolve`, `EnsureFile`, `SnapshotMtime`). `App.Update` gets a new `e` case that snapshots the mtime, calls `editCurrent()`, and yields a `tea.ExecProcess` cmd. A new `editorExitedMsg` handler in `App.Update` mtime-gates the reindex. No changes to `graph`, `render`, `search`, or any other view.

**Tech Stack:** Go 1.26, Bubble Tea (`tea.ExecProcess`), POSIX shell (for the fake editor test fixture).

**Spec:** `docs/superpowers/specs/2026-06-05-edit-hand-off-design.md`

---

## Background for the implementer

`peekseq` is a Bubble Tea TUI for browsing a Logseq graph. Key files you'll touch:

- `internal/views/app.go` — root model. The `tea.KeyMsg` switch at the bottom of `Update` (around line 293) is the page-view key dispatch. `setHint(s)` is the existing 3-second fading status-bar message; reuse it. `buildIndexCmd()` is the existing async reindex chokepoint; reuse it.
- `internal/views/keys.go` — key-string constants. Currently holds `keyQ`, `keyJ`, `keyK`, etc. Multi-use keys live here; single-use stay at the call site.
- `internal/views/help.go` — `helpSections` slice (line 43) drives the help overlay. Each section is `{title, []helpRow{key, desc}}`.
- `internal/views/testdata/TestHelpGolden.golden` — golden file. Add a row to the *Maintain* section, then regenerate with `go test ./... -update` and visually diff.

`internal/views/app_test.go` has `bootApp(t)` (line 49) and `bootAppAt(t, now)` (line 22) helpers that drive the full boot synchronously and return an `App` with a constructed `PageView` at 80×24. Existing dispatch tests live in `app_dispatch_test.go` and use `a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("g")})` for key events. **The edit tests in this plan follow that same pattern** — direct `Update` calls, not `teatest.NewProgram`. The teatest framework in this project is currently only used for `RequireEqualOutput` golden checks; driving the model with `tea.ExecProcess` is unnecessary when we can construct the `editorExitedMsg` directly and assert on the returned `tea.Cmd`.

`testdata/fixture-graph/` has fixture pages and journals. Use it for all tests — never point tests at the real graph.

**Project rules (from `AGENTS.md` and global `CLAUDE.md`):**

- TDD: failing test first, minimal implementation, then refactor if needed. Tests stay green between commits.
- Run `go vet ./... && go test ./...` before every commit — both must pass.
- Conventional Commits (`feat:`, `fix:`, `refactor:`, `test:`, `docs:`, `chore:`).
- `teatest.RequireEqualOutput` compares against goldens under `internal/views/testdata/`. After intentional UI changes, regenerate with `go test ./... -update` and visually diff before staging.
- Never point tests at the real graph (`~/Documents/fiat-codex`) — only `testdata/fixture-graph/`.

---

## File map

**New files**

- `internal/edit/editor.go` — `Env`, `Resolved`, `errNoEditor`, `Resolve`, `EnsureFile`, `SnapshotMtime`. The only disk-writing surface in the project.
- `internal/edit/editor_test.go` — unit tests for the above (TDD).
- `internal/views/edit_test.go` — direct-`Update` tests for the App integration paths.
- `testdata/fake-editor.sh` — POSIX shell script fixture. Currently a placeholder; not needed for unit tests but reserved for future end-to-end coverage.

**Modified files**

- `internal/views/app.go` — add `editorExitedMsg` type, `editCurrent()` method, `e` key case in dispatch, `editorExitedMsg` case in `Update`. Add `"os"` and `"os/exec"` imports.
- `internal/views/keys.go` — add `keyE = "e"` constant.
- `internal/views/help.go` — add one row to the *Maintain* section.
- `internal/views/testdata/TestHelpGolden.golden` — regenerate.
- `AGENTS.md` — drop the "never writes to the graph" line; add the editor info; add a test fixture note.
- `README.md` — add `e` to the keymap table; update the "Scope" section to reflect the relaxed invariant.
- `CHANGELOG.md` — new `## [Unreleased]` entry under `### Added`.

---

## Task 1: `internal/edit/editor.go` — `Resolve` and types

**Files:**
- Create: `internal/edit/editor.go`
- Create: `internal/edit/editor_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/edit/editor_test.go`:

```go
package edit

import (
	"errors"
	"testing"
)

// fakeLookPath returns a lookPath that resolves a fixed set of names and
// returns ErrNotFound for everything else. errNotFound matches the
// surface of exec.LookPath on missing binaries.
var errNotFound = errors.New("not found")

func fakeLookPath(present map[string]string) func(string) (string, error) {
	return func(bin string) (string, error) {
		if p, ok := present[bin]; ok {
			return p, nil
		}
		return "", errNotFound
	}
}

func TestResolve(t *testing.T) {
	cases := []struct {
		name string
		env  Env
		lp   func(string) (string, error)
		want string
	}{
		{
			name: "VISUAL set, present",
			env:  Env{Visual: "vim", Editor: "emacs"},
			lp:   fakeLookPath(map[string]string{"vim": "/usr/bin/vim", "emacs": "/usr/bin/emacs"}),
			want: "/usr/bin/vim",
		},
		{
			name: "VISUAL set, missing; EDITOR present",
			env:  Env{Visual: "nvim", Editor: "emacs"},
			lp:   fakeLookPath(map[string]string{"emacs": "/usr/bin/emacs"}),
			want: "/usr/bin/emacs",
		},
		{
			name: "VISUAL and EDITOR empty, vi present",
			env:  Env{},
			lp:   fakeLookPath(map[string]string{"/usr/bin/vi": "/usr/bin/vi"}),
			want: "/usr/bin/vi",
		},
		{
			name: "VISUAL empty, EDITOR present, vi also present — EDITOR wins",
			env:  Env{Editor: "micro"},
			lp:   fakeLookPath(map[string]string{"micro": "/usr/bin/micro", "/usr/bin/vi": "/usr/bin/vi"}),
			want: "/usr/bin/micro",
		},
		{
			name: "VISUAL set but missing; EDITOR set but missing; vi missing",
			env:  Env{Visual: "nvim", Editor: "nano"},
			lp:   fakeLookPath(map[string]string{}),
			want: "",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Resolve(tc.env, tc.lp)
			if tc.want == "" {
				if err == nil {
					t.Fatalf("want error, got %+v", got)
				}
				if !errors.Is(err, errNoEditor) {
					t.Errorf("want errNoEditor, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.Binary != tc.want {
				t.Errorf("Binary: want %q, got %q", tc.want, got.Binary)
			}
		})
	}
}
```

- [ ] **Step 2: Run the test, verify it fails**

Run: `go test ./internal/edit/ -run TestResolve -v`
Expected: FAIL — `internal/edit` package does not exist yet, build error.

- [ ] **Step 3: Write the minimal implementation**

Create `internal/edit/editor.go`:

```go
// Package edit is the single disk-writing surface in peekseq. It hands a
// page's .md file off to the user's editor and detects whether the file
// changed on return. No other package in the project writes to disk.
package edit

import (
	"errors"
)

// Env carries the configuration the editor-resolver reads. Decoupled
// from os.Getenv so tests can inject a fixed env without touching
// process state.
type Env struct {
	Visual string // value of $VISUAL
	Editor string // value of $EDITOR
}

// Resolved is the editor binary to invoke with no args; the file path
// is appended at call time by the App.
type Resolved struct {
	Binary string
}

// errNoEditor is the sentinel returned by Resolve when none of
// $VISUAL, $EDITOR, or /usr/bin/vi resolve to a runnable binary. The
// App surfaces it as a status-bar hint.
var errNoEditor = errors.New("no editor found (set $VISUAL or $EDITOR, or install vi)")

// Resolve picks the first available editor in the standard chain:
// $VISUAL → $EDITOR → /usr/bin/vi. lookPath is injected so tests can
// simulate "set but missing" / "vi missing" without touching PATH.
// On success, Resolved.Binary is the *resolved path* from lookPath
// (e.g. "/usr/bin/vim"), not the input name — `exec.Command` would
// re-lookPath it anyway, but the resolved path is what the tests
// assert against and what downstream callers should treat as the
// final answer.
func Resolve(env Env, lookPath func(string) (string, error)) (Resolved, error) {
	if env.Visual != "" {
		if path, err := lookPath(env.Visual); err == nil {
			return Resolved{Binary: path}, nil
		}
	}
	if env.Editor != "" {
		if path, err := lookPath(env.Editor); err == nil {
			return Resolved{Binary: path}, nil
		}
	}
	if path, err := lookPath("/usr/bin/vi"); err == nil {
		return Resolved{Binary: path}, nil
	}
	return Resolved{}, errNoEditor
}
```

- [ ] **Step 4: Run the test, verify it passes**

Run: `go test ./internal/edit/ -run TestResolve -v`
Expected: PASS (5 subtests).

- [ ] **Step 5: Run go vet to confirm clean**

Run: `go vet ./...`
Expected: no diagnostics.

- [ ] **Step 6: Commit**

```bash
git add internal/edit/editor.go internal/edit/editor_test.go
git commit -m "feat(edit): add Resolve for VISUAL/EDITOR/vi chain"
```

---

## Task 2: `EnsureFile`

**Files:**
- Modify: `internal/edit/editor.go`
- Modify: `internal/edit/editor_test.go`

- [ ] **Step 1: Append the failing test**

Append to `internal/edit/editor_test.go`:

```go
func TestEnsureFile(t *testing.T) {
	t.Run("missing file is created empty", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "new.md")

		created, err := EnsureFile(path)
		if err != nil {
			t.Fatalf("EnsureFile: %v", err)
		}
		if !created {
			t.Errorf("want created=true, got false")
		}
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("stat: %v", err)
		}
		if info.Size() != 0 {
			t.Errorf("want empty file, got %d bytes", info.Size())
		}
		if info.Mode().Perm() != 0o644 {
			t.Errorf("want mode 0o644, got %v", info.Mode().Perm())
		}
	})

	t.Run("existing file is left alone", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "existing.md")
		if err := os.WriteFile(path, []byte("hello\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		before, _ := os.Stat(path)

		created, err := EnsureFile(path)
		if err != nil {
			t.Fatalf("EnsureFile: %v", err)
		}
		if created {
			t.Errorf("want created=false, got true")
		}
		after, _ := os.Stat(path)
		if before.ModTime() != after.ModTime() {
			t.Errorf("mtime changed: before=%v after=%v", before.ModTime(), after.ModTime())
		}
		body, _ := os.ReadFile(path)
		if string(body) != "hello\n" {
			t.Errorf("content changed: got %q", body)
		}
	})

	t.Run("path is a directory — error", func(t *testing.T) {
		dir := t.TempDir()
		_, err := EnsureFile(dir)
		if err == nil {
			t.Errorf("want error, got nil")
		}
	})
}
```

Add `"os"` and `"path/filepath"` to the import block of `editor_test.go`.

- [ ] **Step 2: Run the test, verify it fails**

Run: `go test ./internal/edit/ -run TestEnsureFile -v`
Expected: FAIL — `EnsureFile` undefined.

- [ ] **Step 3: Implement `EnsureFile`**

Append to `internal/edit/editor.go`:

```go
import "os" // add to existing import block

// EnsureFile creates an empty file at path with mode 0o644 if it does
// not exist. Returns (true, nil) on create, (false, nil) if the file
// already existed (and is a regular file), or (false, err) for any
// other stat/write failure — including the case where path exists but
// is a directory rather than a file. This is the create-today-journal
// hook; the directory case is surfaced as an error rather than
// silently no-op'ing because a directory at the journal-file path is
// a user error worth reporting.
func EnsureFile(path string) (created bool, err error) {
	info, err := os.Stat(path)
	if err == nil {
		if info.IsDir() {
			return false, fmt.Errorf("ensure %s: is a directory", path)
		}
		return false, nil
	}
	if !os.IsNotExist(err) {
		return false, err
	}
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		return false, err
	}
	return true, nil
}
```

- [ ] **Step 4: Run the test, verify it passes**

Run: `go test ./internal/edit/ -v`
Expected: PASS (5 Resolve subtests + 3 EnsureFile subtests).

- [ ] **Step 5: Commit**

```bash
git add internal/edit/editor.go internal/edit/editor_test.go
git commit -m "feat(edit): add EnsureFile for create-today-journal hook"
```

---

## Task 3: `SnapshotMtime`

**Files:**
- Modify: `internal/edit/editor.go`
- Modify: `internal/edit/editor_test.go`

- [ ] **Step 1: Append the failing test**

Append to `internal/edit/editor_test.go`:

```go
func TestSnapshotMtime(t *testing.T) {
	t.Run("existing file returns ModTime", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "page.md")
		if err := os.WriteFile(path, []byte("x\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		got, err := SnapshotMtime(path)
		if err != nil {
			t.Fatalf("SnapshotMtime: %v", err)
		}
		if got.IsZero() {
			t.Errorf("want non-zero mtime, got zero")
		}
	})

	t.Run("missing file returns zero mtime and ENOENT", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "nope.md")
		got, err := SnapshotMtime(path)
		if !os.IsNotExist(err) {
			t.Errorf("want ENOENT, got %v", err)
		}
		if !got.IsZero() {
			t.Errorf("want zero mtime, got %v", got)
		}
	})

	t.Run("mtime advances after write", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "page.md")
		if err := os.WriteFile(path, []byte("a\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		first, err := SnapshotMtime(path)
		if err != nil {
			t.Fatal(err)
		}
		// Sleep just past the filesystem mtime resolution (most are
		// 1s on Linux ext4, 1ns on tmpfs; 10ms is a safe margin).
		time.Sleep(10 * time.Millisecond)
		if err := os.WriteFile(path, []byte("bb\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		second, err := SnapshotMtime(path)
		if err != nil {
			t.Fatal(err)
		}
		if !second.After(first) {
			t.Errorf("want second > first; first=%v second=%v", first, second)
		}
	})
}
```

Add `"time"` to the imports of `editor_test.go`.

- [ ] **Step 2: Run the test, verify it fails**

Run: `go test ./internal/edit/ -run TestSnapshotMtime -v`
Expected: FAIL — `SnapshotMtime` undefined.

- [ ] **Step 3: Implement `SnapshotMtime`**

Append to `internal/edit/editor.go`:

```go
import "time" // add to existing import block

// SnapshotMtime returns the file's modification time, or time.Time{}
// (the zero value) if the file does not exist. The zero return is
// load-bearing: it lets the caller distinguish "the file was just
// created by EnsureFile" (t0 == 0) from "the file was on disk before
// the user pressed e" (t0 > 0).
func SnapshotMtime(path string) (time.Time, error) {
	info, err := os.Stat(path)
	if err != nil {
		return time.Time{}, err
	}
	return info.ModTime(), nil
}
```

- [ ] **Step 4: Run the test, verify it passes**

Run: `go test ./internal/edit/ -v`
Expected: PASS (5 Resolve + 3 EnsureFile + 3 SnapshotMtime subtests).

- [ ] **Step 5: Run the full project test suite as a sanity check**

Run: `go vet ./... && go test ./...`
Expected: green; the new package compiles and its tests pass; the rest of the project is unaffected (no callers yet).

- [ ] **Step 6: Commit**

```bash
git add internal/edit/editor.go internal/edit/editor_test.go
git commit -m "feat(edit): add SnapshotMtime for change detection"
```

---

## Task 4: Fake editor fixture

**Files:**
- Create: `testdata/fake-editor.sh`

This fixture is reserved for any future end-to-end test that wants to drive a real subprocess. The plan's tests use direct-`Update` calls and don't need it, but the spec keeps it in the file map for completeness. Add a minimal stub and a smoke test.

- [ ] **Step 1: Write the script**

Create `testdata/fake-editor.sh`:

```sh
#!/bin/sh
# fake-editor.sh — test fixture for the editor hand-off.
# argv: $1 = file path, $2 = action
#
# Actions:
#   noop    exit 0, do nothing
#   touch   bump mtime only (no content change)
#   append  append "edited" to the file, bump mtime
#   delete  rm the file
#   fail    exit 7
set -e
case "$2" in
  noop)    : ;;
  touch)   touch "$1" ;;
  append)  echo "edited" >> "$1" ;;
  delete)  rm "$1" ;;
  fail)    exit 7 ;;
  *)       echo "fake-editor: unknown action $2" >&2; exit 2 ;;
esac
```

- [ ] **Step 2: Make it executable and smoke test**

Run:

```bash
chmod +x testdata/fake-editor.sh
tmp=$(mktemp)
testdata/fake-editor.sh "$tmp" noop   && [ -f "$tmp" ] && echo OK
testdata/fake-editor.sh "$tmp" append && grep -q edited "$tmp" && echo OK
testdata/fake-editor.sh "$tmp" delete && [ ! -f "$tmp" ] && echo OK
rm -f "$tmp"
```

Expected: three `OK` lines. The `fail` and `touch` cases are exercised by the App integration tests in Task 5.

- [ ] **Step 3: Commit**

```bash
git add testdata/fake-editor.sh
git commit -m "test: add fake-editor fixture for editor hand-off tests"
```

---

## Task 5: App integration — `keyE`, `editCurrent`, `editorExitedMsg`

**Files:**
- Modify: `internal/views/keys.go`
- Modify: `internal/views/app.go`
- Create: `internal/views/edit_test.go`

- [ ] **Step 1: Add the `keyE` constant**

Edit `internal/views/keys.go` — add a new line inside the `const ( … )` block, alphabetically after `keyDown`:

```go
	keyDown      = "down"
	keyE         = "e"
```

- [ ] **Step 2: Write the failing App-level tests**

Create `internal/views/edit_test.go`:

```go
package views

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// pressE sends the e key to a booted app and returns the resulting cmd.
func pressE(t *testing.T, a *App) tea.Cmd {
	t.Helper()
	_, cmd := a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("e")})
	return cmd
}

// keyEnter sends the Enter key — used to navigate to a real page so
// the e key has a target path to edit.
func keyEnter() tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyEnter}
}

// TestEditKey_DispatchesToCmd sends the e key on the page view and
// asserts that a non-nil cmd is returned. The cmd is the
// tea.ExecProcess wrapper; we don't run it (no real TTY here), we just
// confirm the dispatch fires.
func TestEditKey_DispatchesToCmd(t *testing.T) {
	a := bootApp(t)
	// Move to Alpha so we have a real file to target.
	a.Update(keyEnter())
	a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("A")})
	if a.page.Page() != "Alpha" {
		t.Fatalf("expected to land on Alpha, got %q", a.page.Page())
	}
	cmd := pressE(t, a)
	if cmd == nil {
		t.Errorf("e key should return a tea.ExecProcess cmd; got nil")
	}
}

// TestEditorExitedMsg_NoReindexOnNoChange constructs the message that
// tea.ExecProcess would yield when the child exits cleanly without
// changing the file. The handler should return (nil cmd) — no reindex.
func TestEditorExitedMsg_NoReindexOnNoChange(t *testing.T) {
	a := bootApp(t)
	abs, err := filepath.Abs("../../testdata/fixture-graph")
	if err != nil {
		t.Fatal(err)
	}
	// Any existing fixture page is fine — we never actually call the
	// editor. Alpha.md is in the fixture.
	path := filepath.Join(abs, "pages", "Alpha.md")
	t0, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat Alpha.md: %v", err)
	}

	_, cmd := a.Update(editorExitedMsg{path: path, t0: t0.ModTime(), err: nil})
	if cmd != nil {
		t.Errorf("unchanged file should not reindex; got non-nil cmd")
	}
}

// TestEditorExitedMsg_TriggersReindexOnMtimeChange: the file's mtime
// has advanced; the handler should return a non-nil cmd (the
// buildIndexCmd).
func TestEditorExitedMsg_TriggersReindexOnMtimeChange(t *testing.T) {
	a := bootApp(t)
	abs, err := filepath.Abs("../../testdata/fixture-graph")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(abs, "pages", "Alpha.md")

	// Pick a t0 well before the real mtime so the comparison advances.
	t0 := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)

	_, cmd := a.Update(editorExitedMsg{path: path, t0: t0, err: nil})
	if cmd == nil {
		t.Errorf("mtime change should trigger reindex; got nil cmd")
	}
}

// TestEditorExitedMsg_NoReindexOnDelete: post-exit stat returns
// ENOENT — the user deleted the file in the editor. Silent no-op.
func TestEditorExitedMsg_NoReindexOnDelete(t *testing.T) {
	a := bootApp(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "deleted.md")
	// Don't create the file — os.Stat will return ENOENT.
	_, cmd := a.Update(editorExitedMsg{path: path, t0: time.Time{}, err: nil})
	if cmd != nil {
		t.Errorf("deleted file should not reindex; got non-nil cmd")
	}
}

// TestEditorExitedMsg_HintOnError: non-nil err from the editor (e.g.
// :cq in vim, or signal). The handler should return a setHint cmd
// (non-nil because setHint schedules a tea.Tick).
func TestEditorExitedMsg_HintOnError(t *testing.T) {
	a := bootApp(t)
	_, cmd := a.Update(editorExitedMsg{path: "/nonexistent", t0: time.Time{}, err: errFake("editor exited 7")})
	if cmd == nil {
		t.Errorf("editor error should surface a hint cmd; got nil")
	}
}

// TestEditKey_CreatesMissingTodayJournal: when the current page is
// today's journal and the file does not exist on disk, dispatching
// `e` runs EnsureFile synchronously inside editCurrent (before the
// tea.ExecProcess cmd is returned). The file is created empty with
// mode 0o644. We assert on the file's existence and size after
// dispatch — we don't run the returned cmd.
func TestEditKey_CreatesMissingTodayJournal(t *testing.T) {
	// Build a temp graph: pages/ + journals/ where today's journal file
	// is absent. BuildIndex will still list it in idx.Pages (journals
	// are always inserted regardless of file presence — verify against
	// the existing behaviour in the codebase).
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "journals"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "pages"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "pages", "Anchor.md"), []byte("anchor\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// No journal file written — today's journal is missing on disk.

	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	a := New(dir, "test")
	a.nowFunc = func() time.Time { return time.Date(2026, 6, 5, 12, 0, 0, 0, time.UTC) }
	// Drive the boot synchronously.
	cmd := a.Init()
	if cmd != nil {
		a.Update(cmd())
	}
	a.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

	today := "2026-06-05"
	if a.page.Page() != today {
		t.Fatalf("expected boot page %q, got %q", today, a.page.Page())
	}
	journalPath := filepath.Join(dir, "journals", today+".md")
	if _, err := os.Stat(journalPath); !os.IsNotExist(err) {
		t.Fatalf("precondition: journal file should not exist; stat err=%v", err)
	}

	// Dispatch `e`. editCurrent runs EnsureFile synchronously, then
	// returns the tea.ExecProcess cmd. We don't need to run the cmd.
	_, _ = a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("e")})

	info, err := os.Stat(journalPath)
	if err != nil {
		t.Fatalf("journal file should have been created; stat err=%v", err)
	}
	if info.Size() != 0 {
		t.Errorf("journal file should be empty; got %d bytes", info.Size())
	}
	if info.Mode().Perm() != 0o644 {
		t.Errorf("journal file mode: want 0o644, got %v", info.Mode().Perm())
	}
}

// TestEditKey_HintWhenNoEditor: when no env var resolves and /usr/bin/vi
// is missing, editCurrent returns a setHint cmd (which is non-nil —
// setHint schedules a tea.Tick to clear the hint). We test the
// non-TTY case by setting VISUAL and EDITOR to non-existent binaries
// and accepting the result on environments where /usr/bin/vi is also
// missing. On environments where vi exists, the test asserts the cmd
// is non-nil (either hint or tea.ExecProcess) — both are valid since
// the dispatch must produce *some* cmd.
func TestEditKey_HintWhenNoEditor(t *testing.T) {
	a := bootApp(t)
	a.Update(keyEnter())
	a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("A")})
	if a.page.Page() != "Alpha" {
		t.Fatalf("expected to land on Alpha, got %q", a.page.Page())
	}
	t.Setenv("VISUAL", "__nonexistent_peekseq_visual__")
	t.Setenv("EDITOR", "__nonexistent_peekseq_editor__")
	_, cmd := a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("e")})
	if cmd == nil {
		t.Errorf("e key should return some cmd even when no editor resolves; got nil")
	}
	// When /usr/bin/vi exists on the test host, the cmd is a
	// tea.ExecProcess wrapper; we don't run it, so we just confirm
	// dispatch fired. When vi is missing, cmd is a setHint tea.Tick —
	// also non-nil. Either branch satisfies the assertion above.
}

type errFake string

func (e errFake) Error() string { return string(e) }
```

- [ ] **Step 3: Run the tests, verify they fail**

Run: `go test ./internal/views/ -run TestEdit -v`
Expected: FAIL — `editorExitedMsg` is undefined.

- [ ] **Step 4: Add `editorExitedMsg` type to app.go**

In `internal/views/app.go`, after the `indexLoadedMsg` type declaration (around line 18), add:

```go
// editorExitedMsg is delivered when the child editor process returns.
// path is the file we handed to the editor; t0 is the pre-edit mtime
// snapshot (zero if the file did not exist before EnsureFile ran).
// err is non-nil when the editor exited non-zero or failed to launch.
type editorExitedMsg struct {
	path string
	t0   time.Time
	err  error
}
```

- [ ] **Step 5: Run the tests, verify `editorExitedMsg` is at least defined**

Run: `go test ./internal/views/ -run TestEdit -v`
Expected: still FAIL — the dispatch and handler are not implemented yet. The error message should now reference `editorExitedMsg` / `editCurrent` / the `e` case being missing.

- [ ] **Step 6: Add `editCurrent` method and dispatch + handler cases**

In `internal/views/app.go`:

1. Add imports (if not already present):
   - `"os"`
   - `"os/exec"`
   - `"git.fiatcode.dev/fiatcode/peekseq/internal/edit"`

2. Add the method (place it after `navigateAt` at the bottom of the method block, before `journalNeighbor`):

```go
// editCurrent snapshots the current page's file mtime, ensures the
// file exists (creating an empty one for today's journal if needed),
// resolves the user's editor, and returns a tea.ExecProcess cmd that
// hands the file off. The child editor's exit yields an
// editorExitedMsg, which the Update case below mtime-gates against a
// reindex.
func (a *App) editCurrent() tea.Cmd {
	page := a.page.Page()
	meta, ok := a.idx.ByName[page]
	if !ok {
		return a.setHint("page not in index: " + page)
	}
	path := meta.Path

	t0, statErr := edit.SnapshotMtime(path)
	if statErr != nil && !os.IsNotExist(statErr) {
		return a.setHint("cannot stat: " + statErr.Error())
	}

	if t0.IsZero() {
		if _, err := edit.EnsureFile(path); err != nil {
			return a.setHint("cannot create journal: " + err.Error())
		}
	}

	resolved, err := edit.Resolve(
		edit.Env{Visual: os.Getenv("VISUAL"), Editor: os.Getenv("EDITOR")},
		exec.LookPath,
	)
	if err != nil {
		return a.setHint(err.Error())
	}

	return tea.ExecProcess(exec.Command(resolved.Binary, path), func(cmdErr error) tea.Msg {
		return editorExitedMsg{path: path, t0: t0, err: cmdErr}
	})
}
```

3. Add the `e` case to the page-view key switch (the `switch key {` block in `Update`, around line 293). Place it next to the other single-letter keys (e.g. after `case "R":`):

```go
case "e":
    return a, a.editCurrent()
```

4. Add the `editorExitedMsg` case to `Update` (in the top-level `switch m := msg.(type)` block, alongside `case indexLoadedMsg` and `case searchDoneMsg`):

```go
case editorExitedMsg:
    if m.err != nil {
        return a, a.setHint("editor exited: " + m.err.Error())
    }
    info, err := os.Stat(m.path)
    if err != nil {
        if os.IsNotExist(err) {
            return a, nil
        }
        return a, a.setHint("cannot stat: " + err.Error())
    }
    if info.ModTime().Equal(m.t0) {
        return a, nil
    }
    return a, a.buildIndexCmd()
```

- [ ] **Step 7: Run the tests, verify they pass**

Run: `go test ./internal/views/ -run TestEdit -v`
Expected: PASS (5 subtests).

- [ ] **Step 8: Run the full project test suite**

Run: `go vet ./... && go test ./...`
Expected: green. The new `e` key is dispatched but no existing test or golden exercises the page view after pressing `e`, so no goldens should be affected.

- [ ] **Step 9: Commit**

```bash
git add internal/views/keys.go internal/views/app.go internal/views/edit_test.go
git commit -m "feat(views): hand off current page to \$EDITOR with e key"
```

---

## Task 6: Help overlay update

**Files:**
- Modify: `internal/views/help.go`
- Modify: `internal/views/testdata/TestHelpGolden.golden`

- [ ] **Step 1: Add the `e` row to the Maintain section**

In `internal/views/help.go`, edit the Maintain section (line 70) to add a new row before the closing `}}`:

```go
	{"Maintain", []helpRow{
		{"e", "edit current page in $EDITOR"},
		{"R", "rebuild index"},
	}},
```

This places `e` ahead of `R` so it appears first in the rendered list. (`R` is alphabetically adjacent; `e` is the new addition.)

- [ ] **Step 2: Run the help golden test, see it fail**

Run: `go test ./internal/views/ -run TestHelpGolden -v`
Expected: FAIL — golden no longer matches.

- [ ] **Step 3: Regenerate the golden**

Run: `go test ./internal/views/ -run TestHelpGolden -update`
Expected: PASS. The golden file is rewritten with the new row.

- [ ] **Step 4: Visually diff the regenerated golden**

Open `internal/views/testdata/TestHelpGolden.golden`. Confirm:
- The *Maintain* section now has two rows: `e ... edit current page in $EDITOR` and `R ... rebuild index`.
- No other sections changed.
- The footer (`? or esc to close    peekseq v1.0.0`) is unchanged.

If anything else changed, revert the golden (`git checkout -- internal/views/testdata/TestHelpGolden.golden`) and investigate the unintended change.

- [ ] **Step 5: Run the full project test suite**

Run: `go vet ./... && go test ./...`
Expected: green.

- [ ] **Step 6: Commit**

```bash
git add internal/views/help.go internal/views/testdata/TestHelpGolden.golden
git commit -m "docs(help): document the e (edit) key"
```

---

## Task 7: Doc updates

**Files:**
- Modify: `AGENTS.md`
- Modify: `README.md`
- Modify: `CHANGELOG.md`

- [ ] **Step 1: Update `AGENTS.md`**

In `AGENTS.md`, the first paragraph currently reads:

> Read-only Bubble Tea TUI for browsing a local Logseq graph. Recency-sorted picker, ripgrep-backed search, backlinks, TODO dashboard. Never writes to the graph.

Replace it with:

> Bubble Tea TUI for browsing a local Logseq graph. Recency-sorted picker, ripgrep-backed search, backlinks, TODO dashboard. Writes only via `internal/edit/`, only in response to the `e` key, and only to the file currently displayed on the page view.

Then in the *Layout* section, add a new bullet after the `internal/render/` line:

> - `internal/edit/` — editor hand-off (the single disk-writing surface in the project). Resolves `$VISUAL` / `$EDITOR` / `vi`, snapshots file mtime, ensures today's journal file exists, and exposes `Resolve` / `EnsureFile` / `SnapshotMtime`.

- [ ] **Step 2: Update `README.md`**

In the *Keys* table, insert a new row between `R` and `Esc`:

```markdown
| `e`        | edit current page in `$EDITOR`        |
```

In the *Scope* section (line 74), change:

> Read-only. No editing, no fold/unfold, no filesystem-watch live reload (use `R`).

to:

> Read-only except for the `e` key, which hands the current page's file to `$EDITOR`. No fold/unfold, no filesystem-watch live reload (use `R`).

In the first paragraph (line 3), change:

> A terminal browser for a local Logseq graph. Recency-sorted page picker, ripgrep-backed full-text search, backlinks, and a TODO dashboard. Renders pages with hanging-indent bullets, coloured workflow markers (TODO/DOING/LATER/WAITING/DONE/CANCELED/NOW), and highlighted wiki-links you can step through. Never writes to the graph.

to:

> A terminal browser for a local Logseq graph. Recency-sorted page picker, ripgrep-backed full-text search, backlinks, and a TODO dashboard. Renders pages with hanging-indent bullets, coloured workflow markers (TODO/DOING/LATER/WAITING/DONE/CANCELED/NOW), and highlighted wiki-links you can step through. Press `e` to edit the current page in your `$EDITOR` — peekseq is otherwise read-only.

- [ ] **Step 3: Update `CHANGELOG.md`**

At the top of `CHANGELOG.md`, replace the `## [Unreleased]` placeholder section with:

```markdown
## [Unreleased]

### Added

- Hand off the current page to `$VISUAL` / `$EDITOR` / `vi` with the `e`
  key. mtime-gated reindex on return. Today's journal can be created
  and edited if it doesn't exist yet. The only disk-writing surface in
  the project is the new `internal/edit` package; the read-only
  invariant is otherwise unchanged.
```

- [ ] **Step 4: Run the full project test suite**

Run: `go vet ./... && go test ./...`
Expected: green.

- [ ] **Step 5: Commit**

```bash
git add AGENTS.md README.md CHANGELOG.md
git commit -m "docs: announce e (edit) key in AGENTS/README/CHANGELOG"
```

---

## Self-review checklist

After completing all tasks, run through this list:

- [ ] `git log --oneline main..HEAD` shows 7 commits (Tasks 1, 2, 3, 4, 5, 6, 7).
- [ ] `go vet ./... && go test ./...` is green.
- [ ] `git grep -n "TODO\|FIXME\|TBD" internal/edit/ internal/views/edit_test.go` returns no matches.
- [ ] `git grep -n "os.WriteFile\|os.Create\|os.OpenFile" -- '*.go' ':!internal/edit/*.go'` returns no matches — the only disk-writing surface is `internal/edit`.
- [ ] The help golden diff is exactly one new row in the *Maintain* section.
- [ ] `internal/edit/editor.go` exports `Resolve`, `EnsureFile`, `SnapshotMtime`, `Env`, `Resolved`, and the unexported `errNoEditor`.
- [ ] `AGENTS.md` no longer says "Never writes to the graph."
- [ ] `CHANGELOG.md` has a new `## [Unreleased]` `### Added` block.
