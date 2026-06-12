# In-app Markdown Editor + Picker Page-Creation — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace peekseq's read-only posture with a single in-app raw-markdown editor (`e`), keep `$EDITOR` as a power-user escape (`E`), and let the picker create new pages.

**Architecture:** A new full-screen `EditorView` (wrapping `bubbles/textarea`) becomes App state alongside `page`/`active`, taking over key + view when set. Saving routes through a new `edit.WriteFile`, keeping `internal/edit` the only disk-writer; the index rebuilds once on exit. The picker gains a synthetic `＋ Create` row that drops straight into the editor.

**Tech Stack:** Go 1.26, Bubble Tea v1.3, Bubbles v1.0.0 (`textarea`, `textinput`, `fuzzy`), teatest goldens.

**Spec:** `docs/superpowers/specs/2026-06-12-in-app-editor-design.md`

---

## File Structure

- **Create** `internal/views/editor.go` — `EditorView`: textarea wrapper, save/exit state machine, page-scroll, status line. One responsibility: the in-app editing mode.
- **Create** `internal/views/editor_view_test.go` — unit tests for `EditorView` in isolation (the existing `edit_test.go` stays focused on App-level `e`/`E` dispatch).
- **Modify** `internal/edit/editor.go` — add `WriteFile`.
- **Modify** `internal/edit/editor_test.go` — add `WriteFile` tests.
- **Modify** `internal/views/app.go` — `editor` field; `e`→`enterEditor`, `E`→`editCurrent`; editor key/view precedence; `EditorResult` handling; size propagation.
- **Modify** `internal/views/edit_test.go` — repoint existing `e` tests (`e` now opens the in-app editor; `$EDITOR` dispatch moves to `E`).
- **Modify** `internal/views/overlay.go` — add `Create bool` to `OverlayResult`.
- **Modify** `internal/views/picker.go` — create-row logic + query normalization; fix the stale read-only comment.
- **Modify** `internal/views/picker_test.go` — create-row tests.
- **Modify** `internal/views/help.go` — keymap rows for the new editor.
- **Modify** `AGENTS.md`, `README.md`, `CHANGELOG.md` — document the editor.

Keys involved (Bubble Tea `KeyMsg.String()` values): `ctrl+s`, `esc`, `ctrl+c`, `pgup`, `pgdown`, `e`, `E`.

---

## Task 1: `edit.WriteFile`

**Files:**
- Modify: `internal/edit/editor.go`
- Test: `internal/edit/editor_test.go`

- [ ] **Step 1: Write the failing test**

Append to `internal/edit/editor_test.go`:

```go
func TestWriteFile(t *testing.T) {
	t.Run("writes content and creates parent dir", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "pages", "New Page.md")
		if err := WriteFile(path, []byte("hello\n")); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read back: %v", err)
		}
		if string(body) != "hello\n" {
			t.Errorf("content: got %q, want %q", body, "hello\n")
		}
		info, _ := os.Stat(path)
		if info.Mode().Perm() != 0o644 {
			t.Errorf("mode: got %v, want 0o644", info.Mode().Perm())
		}
	})

	t.Run("overwrites existing file", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "p.md")
		if err := os.WriteFile(path, []byte("old\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := WriteFile(path, []byte("new\n")); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
		body, _ := os.ReadFile(path)
		if string(body) != "new\n" {
			t.Errorf("content: got %q, want %q", body, "new\n")
		}
	})
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/edit/ -run TestWriteFile`
Expected: FAIL — `undefined: WriteFile`.

- [ ] **Step 3: Write minimal implementation**

Add to `internal/edit/editor.go` (the import block already has `os`; add `path/filepath`):

```go
// WriteFile writes data to path, creating the parent directory if it does
// not yet exist. It is the second deliberate write path in the project
// (alongside the EnsureFile bootstrap); all disk writes still funnel
// through package edit.
func WriteFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/edit/ -run TestWriteFile`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/edit/editor.go internal/edit/editor_test.go
git commit -m "feat(edit): add WriteFile for the in-app editor save path"
```

---

## Task 2: `EditorView` model (construction, content, dirty, view)

**Files:**
- Create: `internal/views/editor.go`
- Test: `internal/views/editor_view_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/views/editor_view_test.go`:

```go
package views

import (
	"strings"
	"testing"
)

func TestEditorView_LoadsContentAndTracksDirty(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	e := NewEditorView("Alpha", "/tmp/Alpha.md", "first line\n", false, 80, 24)

	if got := e.Content(); got != "first line\n" {
		t.Errorf("Content: got %q, want %q", got, "first line\n")
	}
	if e.dirty() {
		t.Errorf("freshly loaded buffer should be clean")
	}

	e.ta.SetValue("first line\nsecond\n")
	if !e.dirty() {
		t.Errorf("buffer should be dirty after edit")
	}

	e.MarkSaved(e.Content())
	if e.dirty() {
		t.Errorf("buffer should be clean after MarkSaved")
	}
}

func TestEditorView_ContentEndsWithSingleNewline(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	e := NewEditorView("Alpha", "/tmp/Alpha.md", "", true, 80, 24)
	e.ta.SetValue("no trailing newline")
	if got := e.Content(); got != "no trailing newline\n" {
		t.Errorf("Content: got %q, want one trailing newline", got)
	}
	e.ta.SetValue("trailing blanks\n\n\n")
	if got := e.Content(); !strings.HasSuffix(got, "\n") || strings.HasSuffix(got, "\n\n") {
		t.Errorf("Content: got %q, want exactly one trailing newline", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/views/ -run TestEditorView_`
Expected: FAIL — `undefined: NewEditorView`.

- [ ] **Step 3: Write minimal implementation**

Create `internal/views/editor.go`:

```go
package views

import (
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
)

type editorMode int

const (
	editing editorMode = iota
	confirmingExit
)

// EditorView is peekseq's in-app raw-markdown editor: a full-screen mode
// (not a centered overlay) that wraps bubbles/textarea. It owns the
// edit/save/exit state machine; the App performs the actual disk write so
// internal/edit stays the only writer.
type EditorView struct {
	ta            textarea.Model
	path          string // target file (may not exist yet)
	pageName      string // logical page name, for the status line
	baseline      string // content as last loaded/saved
	isNew         bool
	saved         bool // at least one successful save this session
	mode          editorMode
	width, height int
}

// EditorResult is what EditorView.Update reports to the App.
type EditorResult struct {
	Save bool // App writes Content() to path
	Exit bool // App tears down the editor and returns to the read view
}

// NewEditorView builds an editor for page `name` targeting `path`, primed
// with `content` (empty for a not-yet-created page). isNew records whether
// the file existed at open time.
func NewEditorView(name, path, content string, isNew bool, width, height int) *EditorView {
	ta := textarea.New()
	ta.CharLimit = 0  // no length cap
	ta.MaxHeight = 0  // no line cap — pages can exceed textarea's default 99
	ta.ShowLineNumbers = false
	ta.Prompt = ""
	ta.SetValue(content)
	e := &EditorView{
		ta:       ta,
		path:     path,
		pageName: name,
		baseline: content,
		isNew:    isNew,
		width:    width,
		height:   height,
	}
	e.SetSize(width, height)
	return e
}

// Focus focuses the textarea and returns its (cursor-blink) command.
func (e *EditorView) Focus() tea.Cmd { return e.ta.Focus() }

// SetSize resizes the textarea, reserving one row for the status line.
func (e *EditorView) SetSize(w, h int) {
	e.width, e.height = w, h
	e.ta.SetWidth(w)
	e.ta.SetHeight(max(1, h-1))
}

// dirty reports whether the buffer differs from the last loaded/saved content.
func (e *EditorView) dirty() bool { return e.ta.Value() != e.baseline }

// Content is the buffer normalized to end in exactly one newline.
func (e *EditorView) Content() string {
	return strings.TrimRight(e.ta.Value(), "\n") + "\n"
}

// MarkSaved records a successful save: the given content becomes the new
// clean baseline and the file now exists.
func (e *EditorView) MarkSaved(content string) {
	e.baseline = content
	e.saved = true
	e.isNew = false
}

func (e *EditorView) View() string {
	return e.ta.View() + "\n" + e.statusLine()
}

func (e *EditorView) statusLine() string {
	if e.mode == confirmingExit {
		return styleFaint.Render("Save changes?  ") +
			styleTitle.Render("[s]") + styleFaint.Render("ave · ") +
			styleTitle.Render("[d]") + styleFaint.Render("iscard · ") +
			styleTitle.Render("[c]") + styleFaint.Render("ancel")
	}
	mark := ""
	if e.dirty() {
		mark = " ●"
	}
	left := styleTitle.Render(e.pageName) + styleFaint.Render(" [edit]"+mark)
	right := styleFaint.Render("^S save · esc exit")
	return left + "  " + right
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/views/ -run TestEditorView_`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/views/editor.go internal/views/editor_view_test.go
git commit -m "feat(views): EditorView model — textarea wrapper, dirty tracking, content normalization"
```

---

## Task 3: `EditorView.Update` — key state machine + page scroll

**Files:**
- Modify: `internal/views/editor.go`
- Test: `internal/views/editor_view_test.go`

- [ ] **Step 1: Write the failing test**

Append to `internal/views/editor_view_test.go`:

```go
import (
	// add to the existing import block:
	tea "github.com/charmbracelet/bubbletea"
)

func key(s string) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)} }

func TestEditorUpdate_CtrlSRequestsSaveStaysEditing(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	e := NewEditorView("Alpha", "/tmp/a.md", "x\n", false, 80, 24)
	res, _ := e.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	if !res.Save || res.Exit {
		t.Errorf("ctrl+s: got %+v, want {Save:true, Exit:false}", res)
	}
	if e.mode != editing {
		t.Errorf("ctrl+s should stay in editing mode")
	}
}

func TestEditorUpdate_EscOnCleanExits(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	e := NewEditorView("Alpha", "/tmp/a.md", "x\n", false, 80, 24)
	res, _ := e.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if !res.Exit {
		t.Errorf("esc on clean buffer should exit; got %+v", res)
	}
}

func TestEditorUpdate_EscOnDirtyPromptsThenDiscard(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	e := NewEditorView("Alpha", "/tmp/a.md", "x\n", false, 80, 24)
	e.ta.SetValue("x\nmore\n") // make it dirty

	res, _ := e.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if res.Exit || res.Save {
		t.Errorf("esc on dirty buffer should not exit yet; got %+v", res)
	}
	if e.mode != confirmingExit {
		t.Fatalf("esc on dirty should enter confirmingExit")
	}

	// cancel returns to editing
	res, _ = e.Update(key("c"))
	if res.Exit || e.mode != editing {
		t.Errorf("c should cancel back to editing; got %+v mode=%v", res, e.mode)
	}

	// re-enter prompt, then discard
	e.Update(tea.KeyMsg{Type: tea.KeyEsc})
	res, _ = e.Update(key("d"))
	if !res.Exit || res.Save {
		t.Errorf("d should exit without save; got %+v", res)
	}
}

func TestEditorUpdate_ConfirmSaveExits(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	e := NewEditorView("Alpha", "/tmp/a.md", "x\n", false, 80, 24)
	e.ta.SetValue("x\nmore\n")
	e.Update(tea.KeyMsg{Type: tea.KeyEsc}) // -> confirmingExit
	res, _ := e.Update(key("s"))
	if !res.Save || !res.Exit {
		t.Errorf("s should save and exit; got %+v", res)
	}
}

func TestEditorUpdate_TypingInsertsAndIsDirty(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	e := NewEditorView("Alpha", "/tmp/a.md", "", true, 80, 24)
	e.Update(key("h"))
	e.Update(key("i"))
	if got := e.ta.Value(); got != "hi" {
		t.Errorf("typing: got %q, want %q", got, "hi")
	}
	if !e.dirty() {
		t.Errorf("typing should mark dirty")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/views/ -run TestEditorUpdate_`
Expected: FAIL — `e.Update undefined`.

- [ ] **Step 3: Write minimal implementation**

Add to `internal/views/editor.go`:

```go
// Update handles one key and reports whether the App should save/exit.
// In editing mode every key except the intercepts (ctrl+s, esc/ctrl+c,
// pgup/pgdown) is forwarded to the textarea. The returned tea.Cmd is the
// textarea's own (cursor blink) command, which the App must propagate.
func (e *EditorView) Update(msg tea.KeyMsg) (EditorResult, tea.Cmd) {
	if e.mode == confirmingExit {
		switch msg.String() {
		case "s":
			return EditorResult{Save: true, Exit: true}, nil
		case "d":
			return EditorResult{Exit: true}, nil
		case "c", keyEsc:
			e.mode = editing
		}
		return EditorResult{}, nil // ignore everything else
	}

	switch msg.String() {
	case "ctrl+s":
		return EditorResult{Save: true}, nil
	case keyEsc, "ctrl+c":
		if e.dirty() {
			e.mode = confirmingExit
			return EditorResult{}, nil
		}
		return EditorResult{Exit: true}, nil
	case "pgup":
		e.scrollPage(-1)
		return EditorResult{}, nil
	case "pgdown":
		e.scrollPage(+1)
		return EditorResult{}, nil
	}

	var cmd tea.Cmd
	e.ta, cmd = e.ta.Update(msg)
	return EditorResult{}, cmd
}

// scrollPage moves the cursor by one viewport-height of lines by feeding the
// textarea that many up/down keys, reusing its built-in line navigation and
// viewport tracking. v1.0.0 textarea has no PageUp/PageDown of its own.
func (e *EditorView) scrollPage(dir int) {
	steps := max(1, e.ta.Height()-1)
	k := tea.KeyMsg{Type: tea.KeyDown}
	if dir < 0 {
		k = tea.KeyMsg{Type: tea.KeyUp}
	}
	for i := 0; i < steps; i++ {
		e.ta, _ = e.ta.Update(k)
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/views/ -run TestEditorUpdate_`
Expected: PASS.

- [ ] **Step 5: Add a golden for the editor view and the confirm prompt**

Append to `internal/views/editor_view_test.go`:

```go
import (
	// add to the import block:
	"github.com/charmbracelet/x/exp/teatest"
)

func TestEditorView_Golden(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	e := NewEditorView("Alpha", "/tmp/a.md", "- first bullet\n- [[Beta]] link\n", false, 80, 12)
	teatest.RequireEqualOutput(t, []byte(e.View()))
}

func TestEditorView_ConfirmGolden(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	e := NewEditorView("Alpha", "/tmp/a.md", "x\n", false, 80, 12)
	e.ta.SetValue("x\nedited\n")
	e.Update(tea.KeyMsg{Type: tea.KeyEsc}) // -> confirmingExit
	teatest.RequireEqualOutput(t, []byte(e.View()))
}
```

Run: `go test ./internal/views/ -run TestEditorView_ -update`
Then inspect the new goldens under `internal/views/testdata/` before staging.
Run again without `-update`: `go test ./internal/views/ -run TestEditorView_` → PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/views/editor.go internal/views/editor_view_test.go internal/views/testdata/
git commit -m "feat(views): EditorView key state machine, page scroll, goldens"
```

---

## Task 4: App integration — `e`/`E`, editor precedence, save→reindex

**Files:**
- Modify: `internal/views/app.go`
- Modify: `internal/views/keys.go` (add `keyShiftE`)
- Test: `internal/views/edit_test.go`

- [ ] **Step 1: Write the failing tests (repoint existing + add new)**

In `internal/views/edit_test.go`, add a helper next to `pressE`:

```go
// pressShiftE sends the E key (the $EDITOR escape hatch).
func pressShiftE(t *testing.T, a *App) tea.Cmd {
	t.Helper()
	_, cmd := a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("E")})
	return cmd
}
```

Replace `TestEditKey_DispatchesToCmd` with two tests:

```go
func TestE_EntersInAppEditor(t *testing.T) {
	a := bootApp(t)
	a.navigate("Alpha")
	if cmd := pressE(t, a); cmd == nil {
		t.Errorf("e should return the textarea focus cmd; got nil")
	}
	if a.editor == nil {
		t.Fatalf("e should open the in-app editor")
	}
	if a.editor.pageName != "Alpha" {
		t.Errorf("editor page: got %q, want Alpha", a.editor.pageName)
	}
	if a.editor.Content() == "\n" || a.editor.Content() == "" {
		t.Errorf("editor should load Alpha's content; got %q", a.editor.Content())
	}
}

func TestShiftE_DispatchesToEditorProcess(t *testing.T) {
	a := bootApp(t)
	a.navigate("Alpha")
	if cmd := pressShiftE(t, a); cmd == nil {
		t.Errorf("E should return a tea.ExecProcess cmd; got nil")
	}
	if a.editor != nil {
		t.Errorf("E must not open the in-app editor")
	}
}
```

Rename `TestEditKey_ColdStartCreatesTodayJournal` → `TestShiftE_ColdStartCreatesTodayJournal` and change its `pressE` call to `pressShiftE` (the `$EDITOR` path still creates the journal eagerly; everything else in that test is unchanged).

Add a new test for the deferred-creation `e` behavior (paste the cold-start setup from `TestShiftE_ColdStartCreatesTodayJournal`, then):

```go
func TestE_ColdStart_DefersCreation(t *testing.T) {
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
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	a := New(dir, "test")
	a.nowFunc = func() time.Time { return time.Date(2026, 6, 5, 12, 0, 0, 0, time.UTC) }
	if cmd := a.Init(); cmd != nil {
		a.Update(cmd())
	}
	a.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

	today := "2026-06-05"
	journalPath := filepath.Join(dir, "journals", graph.FilenameFromPageName(today))

	pressE(t, a)
	if a.editor == nil {
		t.Fatalf("e on cold-start should open the editor")
	}
	if _, err := os.Stat(journalPath); !os.IsNotExist(err) {
		t.Errorf("e must NOT create the journal file before save; stat err=%v", err)
	}

	// Type, then Ctrl+S — the file appears now.
	a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("h")})
	a.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	if _, err := os.Stat(journalPath); err != nil {
		t.Errorf("ctrl+s should create the journal file; stat err=%v", err)
	}
}
```

In `TestDotKey_CreatesMissingTodayJournal_ThenEditReachable`, change the final block (`if cmd := pressE(t, a); cmd == nil { ... }`) to:

```go
	if cmd := pressE(t, a); cmd == nil {
		t.Errorf("after . created journal, e should open the editor (non-nil focus cmd); got nil")
	}
	if a.editor == nil {
		t.Errorf("after . created journal, e should open the in-app editor")
	}
```

Rename `TestEditKey_EnsureFileStillUsed` → `TestShiftE_EnsureFileStillUsed` and change its final `pressE` to `pressShiftE` (it exercises the `$EDITOR` path).

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/views/ -run 'TestE_|TestShiftE_|TestDotKey_'`
Expected: FAIL — `a.editor` undefined / `keyShiftE` not wired.

- [ ] **Step 3: Wire the App**

In `internal/views/keys.go`, add to the const block:

```go
	keyShiftE = "E"
```

In `internal/views/app.go`, add the field to `App`:

```go
	// editor is the full-screen in-app editor, or nil when not editing.
	// When non-nil it owns all keys and the whole screen.
	editor *EditorView
```

Add `enterEditor` (next to `editCurrent`):

```go
// enterEditor opens the in-app editor on the current page. The target file
// path is computed but NOT created — a brand-new page is written to disk
// only on save (Ctrl+S). Journals route to journals/, every other name to
// pages/ (flat, "/" mangled to "___" by FilenameFromPageName).
func (a *App) enterEditor() tea.Cmd {
	name := a.page.Page()
	var path string
	if meta, ok := a.idx.ByName[name]; ok {
		path = meta.Path
	} else {
		sub := "pages"
		if graph.IsJournalPageName(name) {
			sub = "journals"
		}
		path = filepath.Join(a.graphPath, sub, graph.FilenameFromPageName(name))
	}
	content, isNew := "", true
	if b, err := os.ReadFile(path); err == nil {
		content, isNew = string(b), false
	} else if !os.IsNotExist(err) {
		return a.setHint("cannot read: " + err.Error())
	}
	a.editor = NewEditorView(name, path, content, isNew, a.width, a.height)
	return a.editor.Focus()
}
```

In `App.Update`, the `case tea.WindowSizeMsg` block — after the existing `if a.active != nil { a.active.SetSize(...) }`, add:

```go
		if a.editor != nil {
			a.editor.SetSize(m.Width, m.Height)
		}
```

In `App.Update`'s `case tea.KeyMsg`, immediately after the `if a.page == nil { ... }` block and BEFORE the `if a.active != nil { ... }` block, add the editor branch (it consumes the raw `m` so the textarea receives runes):

```go
		if a.editor != nil {
			res, taCmd := a.editor.Update(m)
			if res.Save {
				content := a.editor.Content()
				if err := edit.WriteFile(a.editor.path, []byte(content)); err != nil {
					return a, tea.Batch(taCmd, a.setHint("cannot save: "+err.Error()))
				}
				a.editor.MarkSaved(content)
			}
			if res.Exit {
				saved := a.editor.saved
				a.editor = nil
				if saved {
					return a, tea.Batch(taCmd, a.buildIndexCmd())
				}
				a.page = NewPageView(a.idx, a.page.Page(), a.width, a.height)
			}
			return a, taCmd
		}
```

Change the page-level key switch: replace the existing `case keyE:` body and add `keyShiftE`:

```go
		case keyE:
			return a, a.enterEditor()
		case keyShiftE:
			return a, a.editCurrent()
```

In `App.View`, after the `if a.page == nil { ... }` guard and before `if a.active != nil { ... }`, add:

```go
	if a.editor != nil {
		return a.editor.View()
	}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/views/ -run 'TestE_|TestShiftE_|TestDotKey_'`
Expected: PASS.
Run the whole package: `go test ./internal/views/`
Expected: PASS (no other test referenced the old `e`→ExecProcess behavior).

- [ ] **Step 5: Commit**

```bash
git add internal/views/app.go internal/views/keys.go internal/views/edit_test.go
git commit -m "feat(views): wire in-app editor to e; move \$EDITOR handoff to E"
```

---

## Task 5: Picker page-creation row

**Files:**
- Modify: `internal/views/overlay.go`
- Modify: `internal/views/picker.go`
- Test: `internal/views/picker_test.go`

- [ ] **Step 1: Write the failing test**

Append to `internal/views/picker_test.go`:

```go
func typeQuery(p *Picker, s string) {
	for _, r := range s {
		p.Update(string(r))
	}
}

func TestPickerCreate_OfferedForNewName(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	p := NewPicker(loadFixture(t), 80, 30)
	typeQuery(p, "Brand New Page")
	if p.createName != "Brand New Page" {
		t.Errorf("createName: got %q, want %q", p.createName, "Brand New Page")
	}
}

func TestPickerCreate_StripsMdExtension(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	p := NewPicker(loadFixture(t), 80, 30)
	typeQuery(p, "Notes.md")
	if p.createName != "Notes" {
		t.Errorf("createName: got %q, want %q (trailing .md stripped)", p.createName, "Notes")
	}
}

func TestPickerCreate_SuppressedForExistingPage(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	p := NewPicker(loadFixture(t), 80, 30)
	typeQuery(p, "Alpha") // exists in the fixture
	if p.createName != "" {
		t.Errorf("createName should be empty for an existing page; got %q", p.createName)
	}
}

func TestPickerCreate_SuppressedForDateShaped(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	p := NewPicker(loadFixture(t), 80, 30)
	typeQuery(p, "2026-06-15")
	if p.createName != "" {
		t.Errorf("createName should be empty for a date-shaped query; got %q", p.createName)
	}
}

func TestPickerCreate_EnterReturnsCreateResult(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	p := NewPicker(loadFixture(t), 80, 30)
	typeQuery(p, "Zzz New") // no fuzzy matches in fixture
	// Selection should sit on the create row (the only row).
	res := p.Update(keyEnter)
	if !res.Accept || !res.Create || res.Selected != "Zzz New" {
		t.Errorf("enter on create row: got %+v, want {Accept, Create, Selected:\"Zzz New\"}", res)
	}
}
```

(Confirm the fixture has a page named `Alpha`; the existing `TestPickerFiltersOnQuery` types `alp`, so it does. `loadFixture` is already defined in the package's test files.)

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/views/ -run TestPickerCreate_`
Expected: FAIL — `p.createName` undefined / `res.Create` undefined.

- [ ] **Step 3a: Add the result field**

In `internal/views/overlay.go`, add to `OverlayResult`:

```go
	// Create, when Accept is true, means "create the page named Selected"
	// rather than open an existing one. Only the Picker sets it.
	Create bool
```

- [ ] **Step 3b: Implement create logic in the picker**

In `internal/views/picker.go`:

Add `"strings"` is already imported; add the field to the `Picker` struct:

```go
	createName string // non-empty when a "＋ Create" row should be offered
```

Replace the stale comment on `pickerChoices` (`picker.go:50`, "...won't invent rows for files that don't exist yet.") with:

```go
// pickerChoices returns the picker candidate list sorted with the most
// recently modified files first. Only real on-disk pages and journals
// appear here; the synthetic "＋ Create" row for a not-yet-existent page is
// handled separately (see refreshCreate / createName).
```

Add a helper and call it from `search`:

```go
// normalizeQuery trims the query and strips a single trailing ".md" so a
// typed extension never doubles to "somepage.md.md".
func normalizeQuery(q string) string {
	q = strings.TrimSpace(q)
	if len(q) >= 3 && strings.EqualFold(q[len(q)-3:], ".md") {
		q = strings.TrimSpace(q[:len(q)-3])
	}
	return q
}

// refreshCreate decides whether to offer a "＋ Create" row for the current
// query: a non-empty, non-date-shaped name that resolves to no existing page.
func (p *Picker) refreshCreate(q string) {
	name := normalizeQuery(q)
	if name == "" || graph.IsJournalPageName(name) {
		p.createName = ""
		return
	}
	if _, ok := p.idx.Resolve(name); ok {
		p.createName = ""
		return
	}
	p.createName = name
}
```

At the end of `search`, before each `return`, the selection is reset to 0; add `p.refreshCreate(q)` as the first line of `search`:

```go
func (p *Picker) search(q string) {
	p.refreshCreate(q)
	// ... existing body unchanged ...
}
```

Update `Update`'s `keyEnter` case to account for the create row sitting just past the matches:

```go
	case keyEnter:
		if p.createName != "" && p.sel == len(p.matches) {
			return OverlayResult{Selected: p.createName, Accept: true, Create: true}
		}
		if p.sel >= 0 && p.sel < len(p.matches) {
			return OverlayResult{Selected: p.matches[p.sel].Str, Accept: true}
		}
		return OverlayResult{}
```

Update `keyDown`/`keyCtrlJ` to allow moving onto the create row by passing the augmented count:

```go
	case keyUp, keyCtrlK:
		p.moveUp()
		return OverlayResult{}
	case keyDown, keyCtrlJ:
		p.moveDown(p.rowCount())
		return OverlayResult{}
```

Add `rowCount` near `visibleRows`:

```go
// rowCount is the number of selectable rows: fuzzy matches plus the optional
// create row.
func (p *Picker) rowCount() int {
	if p.createName != "" {
		return len(p.matches) + 1
	}
	return len(p.matches)
}
```

In `View`, render the create row after the match loop (and handle the empty-matches branch so create still shows). Replace the `if len(p.matches) == 0 { ... }` early-return block with one that still renders a create row, and append the create row after the match loop:

After the `for i := start; i < end; i++ { ... }` loop and its "↓ N more below" block, before the trailing blank + hint, add:

```go
	if p.createName != "" {
		marker := "   "
		label := fmt.Sprintf("＋ Create %q", p.createName)
		if p.sel == len(p.matches) {
			marker = styleSel.Render(" ▶ ")
			label = styleSel.Render(label)
		} else {
			label = styleFaint.Render(label)
		}
		b.WriteString(marker)
		b.WriteString(clamp(label, inner))
		b.WriteString("\n")
	}
```

And change the empty-matches early return so it does not short-circuit when a create row exists. Replace:

```go
	if len(p.matches) == 0 {
		b.WriteString(styleFaint.Render("  no matches"))
		...
		return styleBorder.Width(inner + 4).Render(b.String())
	}
```

with:

```go
	if len(p.matches) == 0 && p.createName == "" {
		b.WriteString(styleFaint.Render("  no matches"))
		b.WriteString("\n")
		b.WriteString("\n")
		b.WriteString(styleFaint.Render(clamp("↑/↓ select · enter open · esc cancel", inner)))
		return styleBorder.Width(inner + 4).Render(b.String())
	}
	if len(p.matches) == 0 {
		b.WriteString(styleFaint.Render("  no matches"))
		b.WriteString("\n")
	}
```

(The `scrollWindow`/match loop runs over `len(p.matches)`, which is 0 here — it renders nothing, then the create row renders below.)

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/views/ -run TestPickerCreate_`
Expected: PASS.
Run existing picker goldens: `go test ./internal/views/ -run TestPicker`
Expected: PASS (queries in existing tests match real pages, so no create row appears and goldens are unchanged).

- [ ] **Step 5: Commit**

```bash
git add internal/views/overlay.go internal/views/picker.go internal/views/picker_test.go
git commit -m "feat(views): picker offers a Create row for new page names"
```

---

## Task 6: App handles picker `Create` → open editor

**Files:**
- Modify: `internal/views/app.go`
- Test: `internal/views/app_overlay_test.go`

- [ ] **Step 1: Write the failing test**

Append to `internal/views/app_overlay_test.go`:

```go
func TestPickerCreate_OpensEditorOnNewPage(t *testing.T) {
	a := bootApp(t)
	a.active = NewPicker(a.idx, a.width, a.height)
	// Type a brand-new name into the open picker.
	for _, r := range "Zzz New Page" {
		a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(string(r))})
	}
	// Enter selects the create row.
	a.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if a.active != nil {
		t.Errorf("picker should be closed after create")
	}
	if a.editor == nil {
		t.Fatalf("create should open the in-app editor")
	}
	if a.editor.pageName != "Zzz New Page" {
		t.Errorf("editor page: got %q, want %q", a.editor.pageName, "Zzz New Page")
	}
	if !a.editor.isNew {
		t.Errorf("a created page's editor should have isNew=true")
	}
	wantSuffix := filepath.Join("pages", "Zzz New Page.md")
	if !strings.HasSuffix(a.editor.path, wantSuffix) {
		t.Errorf("editor path: got %q, want suffix %q", a.editor.path, wantSuffix)
	}
}
```

Ensure `app_overlay_test.go` imports `path/filepath` and `strings` (add if missing).

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/views/ -run TestPickerCreate_OpensEditor`
Expected: FAIL — picker create result isn't handled; `a.editor` stays nil.

- [ ] **Step 3: Handle Create in the overlay-result block**

In `App.Update`, the `if a.active != nil { ... }` block currently handles `res.Accept` by navigating. Add a Create branch ahead of the normal navigate. Replace:

```go
				if res.Accept {
					if res.Selected != "" {
						if res.DeepLink {
							a.navigateToTask(res.Selected, res.TaskOrdinal)
						} else {
							a.navigate(res.Selected)
						}
					}
					a.active = nil
				}
				return a, res.Cmd
```

with:

```go
				if res.Accept {
					if res.Create {
						a.navigate(res.Selected)
						a.active = nil
						return a, a.enterEditor()
					}
					if res.Selected != "" {
						if res.DeepLink {
							a.navigateToTask(res.Selected, res.TaskOrdinal)
						} else {
							a.navigate(res.Selected)
						}
					}
					a.active = nil
				}
				return a, res.Cmd
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/views/ -run TestPickerCreate_OpensEditor`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/views/app.go internal/views/app_overlay_test.go
git commit -m "feat(views): picker Create navigates and opens the editor on a new page"
```

---

## Task 7: Help overlay + docs

**Files:**
- Modify: `internal/views/help.go`
- Modify: `internal/views/help_test.go` (golden refresh)
- Modify: `AGENTS.md`, `README.md`, `CHANGELOG.md`

- [ ] **Step 1: Update the help keymap**

In `internal/views/help.go`, change the `"Maintain"` section and add an `"Edit"` section before it:

```go
	{"Edit", []helpRow{
		{"e", "edit current page in-app"},
		{"E", "edit current page in $EDITOR"},
		{"ctrl-s", "save (while editing)"},
		{"pgup/pgdn", "page up / down (while editing)"},
		{"esc", "leave editor (prompts if unsaved)"},
	}},
	{"Maintain", []helpRow{
		{"R", "rebuild index"},
	}},
```

- [ ] **Step 2: Refresh the help golden**

Run: `go test ./internal/views/ -run TestHelp -update`
Inspect the changed golden under `internal/views/testdata/`, then:
Run: `go test ./internal/views/ -run TestHelp`
Expected: PASS.

- [ ] **Step 3: Update AGENTS.md**

In `AGENTS.md`, update the top paragraph and the `internal/edit/` and `internal/views/` bullets to describe: `e` opens the in-app `EditorView` (writes via `edit.WriteFile` on `Ctrl+S`, file creation deferred to save); `E` retains the `$EDITOR` handoff; `internal/views/editor.go` is the new editor mode; the picker can create pages. Concretely, replace the first paragraph:

```markdown
Bubble Tea TUI for browsing and editing a local Logseq graph. Recency-sorted
picker (with page-creation), ripgrep-backed search, backlinks, TODO dashboard.
Writes only via `internal/edit/`: the in-app editor (`e`) saves the displayed
buffer on `Ctrl+S` and creates a page's file lazily on first save; `E` hands the
file to `$EDITOR`. File creation for a new page is deferred until save, so
opening then discarding never touches disk.
```

And add to the `internal/views/` bullet: `editor` (full-screen in-app markdown editor).

- [ ] **Step 4: Update README.md and CHANGELOG.md**

Add a CHANGELOG entry under the current unreleased section:

```markdown
- In-app markdown editor: `e` edits the current page in a full-screen buffer
  (`Ctrl+S` saves, `esc` exits with an unsaved-changes prompt); `E` opens the
  page in `$EDITOR`. The picker can create a new page by name.
```

Update the README's keybinding/feature list to mention `e` (in-app edit) vs `E` (`$EDITOR`) and picker page-creation, matching the help overlay wording.

- [ ] **Step 5: Full verification**

Run: `go vet ./... && go test ./...`
Expected: PASS (both, per AGENTS.md's pre-commit gate).

- [ ] **Step 6: Commit**

```bash
git add internal/views/help.go internal/views/testdata/ AGENTS.md README.md CHANGELOG.md
git commit -m "docs: document in-app editor (e/E), save, and picker page-creation"
```

---

## Self-Review

**Spec coverage:**
- Unified editing `e` in-app / `E` `$EDITOR` → Task 4. ✓
- Model A save/exit + dirty prompt (cancel = back) → Tasks 2, 3 (state machine), 4 (save→write). ✓
- Cursor at top → default textarea behavior on `SetValue` (cursor starts at 0,0); no special code needed. ✓
- PageUp/PageDown + readline nav → Task 3 (`scrollPage`); other nav is textarea default. ✓
- Deferred creation, any page, journals/ vs pages/ routing → Task 4 (`enterEditor`) + Task 1 (`WriteFile` MkdirAll). ✓
- Picker create row, normalize/strip `.md`, suppress date-shaped, suppress existing → Task 5; open editor → Task 6. ✓
- Reindex on exit only → Task 4 (Exit branch calls `buildIndexCmd` only when `saved`). ✓
- `Ctrl+C` = `Esc` in editor → Task 3. ✓
- Concurrent-external-edit not handled → intentional, documented in spec; no task. ✓
- Docs/help → Task 7. ✓

**Placeholder scan:** No TBD/TODO; every code step shows complete code; commands have expected output. ✓

**Type consistency:** `EditorView`, `EditorResult{Save,Exit}`, `NewEditorView(name,path,content,isNew,w,h)`, `Content()`, `MarkSaved(content)`, `dirty()`, `Focus()`, `SetSize`, `scrollPage`, `editorMode{editing,confirmingExit}`, `keyShiftE`, `OverlayResult.Create`, `Picker.createName`, `normalizeQuery`, `refreshCreate`, `rowCount` — all defined before use and referenced consistently. ✓
