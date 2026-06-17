# Editor Markdown-Aware Editing + Open-at-Top Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Give weft's in-app editor `-` bullet continuation, a `ctrl+t` workflow-marker cycle (plain→TODO→DONE→plain), and cursor-at-top-on-open.

**Architecture:** Pure decision functions in a new `internal/views/edit_markdown.go`; `EditorView.Update()` intercepts `enter` and `ctrl+t` (before keys reach the textarea, beside `ctrl+s`/`esc`) and applies the results through the textarea's own primitives via a reusable `replaceCurrentLine` helper — mirroring how `acceptCompletion()` already mutates the buffer. The open-at-top fix moves the cursor to row 0 in `NewEditorView`.

**Tech Stack:** Go 1.26+, `charmbracelet/bubbles@v1.0.0` (textarea), `charmbracelet/bubbletea` (KeyMsg), `regexp`.

## Global Constraints

- Run `go vet ./... && go test ./...` before every commit — both must pass.
- Touches only `internal/views/` (render/UI layer). No disk writes; `internal/edit/` stays the only writer.
- Marker vocabulary is **TODO** and **DONE** only (the 3-state cycle plain→TODO→DONE→plain). Do NOT implement DOING/LATER/WAITING/NOW/CANCELED, `[ ]` checkboxes, ordered `N.` lists, or `*`/`+` bullets — all explicit non-goals.
- Bullet marker is `-` followed by a space; nesting indent is whatever leading whitespace the line has (graph uses 2-space, but preserve whatever is there).
- Continuation always inserts a **plain `- `** — never carries a workflow marker forward.
- Empty-bullet Enter clears the line fully (no multi-level outdent).
- No change to tinting, framing, save/dirty-tracking, or `[[` completion behavior.
- Regexes are local to `views` (mirror convention with `edit_tint.go`); do NOT add a shared package or import `internal/render` symbols.

---

### Task 1: Pure markdown-editing helpers

The decision logic, with zero textarea/state dependency — fully unit-testable.

**Files:**
- Create: `internal/views/edit_markdown.go`
- Test: `internal/views/edit_markdown_test.go`

**Interfaces:**
- Consumes: nothing (pure, stdlib only).
- Produces:
  - `func bulletPrefix(line string) (prefix string, ok bool)` — `ok=true` and `prefix=indent+"- "` when `line` matches `^(\s*)- `; else `ok=false`.
  - `func isEmptyBullet(line string) bool` — true when `line` matches `^\s*-\s*$`.
  - `func cycleMarkerLine(line string, oldCol int) (newLine string, newCol int, ok bool)` — advances the bullet's marker one step (plain→TODO→DONE→plain), returning the rewritten line and the cursor column adjusted by the marker-length delta; `ok=false` if `line` is not a `^(\s*)- ` bullet.

- [ ] **Step 1: Write the failing tests**

Create `internal/views/edit_markdown_test.go`:

```go
package views

import "testing"

func TestBulletPrefix(t *testing.T) {
	cases := []struct {
		line       string
		wantPrefix string
		wantOK     bool
	}{
		{"- foo", "- ", true},
		{"  - nested", "  - ", true},
		{"- TODO task", "- ", true},
		{"# heading", "", false},
		{"plain text", "", false},
		{"-no space", "", false},
	}
	for _, c := range cases {
		gotPrefix, gotOK := bulletPrefix(c.line)
		if gotPrefix != c.wantPrefix || gotOK != c.wantOK {
			t.Errorf("bulletPrefix(%q) = (%q,%v), want (%q,%v)", c.line, gotPrefix, gotOK, c.wantPrefix, c.wantOK)
		}
	}
}

func TestIsEmptyBullet(t *testing.T) {
	cases := []struct {
		line string
		want bool
	}{
		{"- ", true},
		{"  - ", true},
		{"-", true},
		{"  -", true},
		{"- x", false},
		{"  - foo", false},
		{"plain", false},
	}
	for _, c := range cases {
		if got := isEmptyBullet(c.line); got != c.want {
			t.Errorf("isEmptyBullet(%q) = %v, want %v", c.line, got, c.want)
		}
	}
}

func TestCycleMarkerLine(t *testing.T) {
	// oldCol is the cursor column; pick one past the marker so deltas apply.
	cases := []struct {
		name        string
		line        string
		oldCol      int
		wantNewLine string
		wantNewCol  int
		wantOK      bool
	}{
		{"plain to TODO", "- foo", 5, "- TODO foo", 10, true},
		{"TODO to DONE", "- TODO foo", 10, "- DONE foo", 10, true},
		{"DONE to plain", "- DONE foo", 10, "- foo", 5, true},
		{"nested plain to TODO preserves indent", "  - bar", 7, "  - TODO bar", 12, true},
		{"non-bullet no-op", "# heading", 3, "", 0, false},
		{"cursor before marker unaffected on insert", "- foo", 1, "- TODO foo", 1, true},
	}
	for _, c := range cases {
		gotLine, gotCol, gotOK := cycleMarkerLine(c.line, c.oldCol)
		if gotOK != c.wantOK || (c.wantOK && (gotLine != c.wantNewLine || gotCol != c.wantNewCol)) {
			t.Errorf("%s: cycleMarkerLine(%q,%d) = (%q,%d,%v), want (%q,%d,%v)",
				c.name, c.line, c.oldCol, gotLine, gotCol, gotOK, c.wantNewLine, c.wantNewCol, c.wantOK)
		}
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/views -run 'TestBulletPrefix|TestIsEmptyBullet|TestCycleMarkerLine' -v`
Expected: FAIL to compile — `bulletPrefix`, `isEmptyBullet`, `cycleMarkerLine` undefined.

- [ ] **Step 3: Implement the helpers**

Create `internal/views/edit_markdown.go`:

```go
package views

import (
	"regexp"
	"strings"
)

// Markdown-editing patterns for the in-app editor. Mirror the bullet/marker
// shapes used by edit_tint.go rather than importing internal/render's
// unexported symbols (the documented views/render mirror convention).
var (
	editBulletRe   = regexp.MustCompile(`^(\s*)- `)
	editEmptyBullet = regexp.MustCompile(`^\s*-\s*$`)
	editBulletBody = regexp.MustCompile(`^(\s*)- (.*)$`)
)

// bulletPrefix reports whether line is a "- " bullet and, if so, returns the
// prefix (indent + "- ") to start a continuation line at the same indent.
func bulletPrefix(line string) (string, bool) {
	m := editBulletRe.FindStringSubmatch(line)
	if m == nil {
		return "", false
	}
	return m[1] + "- ", true
}

// isEmptyBullet reports whether line is a bullet marker with no content
// (just "-" / "- " at some indent), i.e. an Enter here should end the list.
func isEmptyBullet(line string) bool {
	return editEmptyBullet.MatchString(line)
}

// cycleMarkerLine advances a bullet's workflow marker one step in the cycle
// plain -> TODO -> DONE -> plain, returning the rewritten line and the cursor
// column shifted by the marker-length change. ok is false when line is not a
// "- " bullet. oldCol is the caller's current cursor column on the line.
func cycleMarkerLine(line string, oldCol int) (string, int, bool) {
	m := editBulletBody.FindStringSubmatch(line)
	if m == nil {
		return "", 0, false
	}
	indent, content := m[1], m[2]
	markerCol := len([]rune(indent)) + 2 // column just after "- "

	var newContent string
	var delta int
	switch {
	case content == "TODO" || strings.HasPrefix(content, "TODO "):
		// TODO -> DONE: replace the leading "TODO" with "DONE" (length unchanged).
		newContent = "DONE" + content[len("TODO"):]
		delta = 0
	case content == "DONE" || strings.HasPrefix(content, "DONE "):
		// DONE -> plain: drop the leading "DONE " (or bare "DONE").
		if strings.HasPrefix(content, "DONE ") {
			newContent = content[len("DONE "):]
			delta = -len("DONE ")
		} else {
			newContent = ""
			delta = -len("DONE")
		}
	default:
		// plain -> TODO: prepend "TODO ".
		newContent = "TODO " + content
		delta = len("TODO ")
	}

	newLine := indent + "- " + newContent
	newCol := oldCol
	if oldCol >= markerCol {
		newCol = oldCol + delta
		if newCol < markerCol {
			newCol = markerCol
		}
	}
	if max := len([]rune(newLine)); newCol > max {
		newCol = max
	}
	return newLine, newCol, true
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/views -run 'TestBulletPrefix|TestIsEmptyBullet|TestCycleMarkerLine' -v`
Expected: PASS (all three).

- [ ] **Step 5: Full suite + vet**

Run: `go vet ./... && go test ./...`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/views/edit_markdown.go internal/views/edit_markdown_test.go
git commit -m "feat: pure helpers for editor bullet continuation + marker cycle"
```

---

### Task 2: Open the editor at the top of the buffer

`textarea.SetValue` leaves the cursor at the end; move it to row 0 so the cursor and the (already-top) viewport agree on open.

**Files:**
- Modify: `internal/views/editor.go` (`NewEditorView`)
- Test: `internal/views/editor_view_test.go`

**Interfaces:**
- Consumes: existing `NewEditorView`, `e.ta` (textarea.Model).
- Produces: a fresh `EditorView` over multi-line content reports `e.ta.Line() == 0`.

- [ ] **Step 1: Write the failing test**

Add to `internal/views/editor_view_test.go`:

```go
func TestEditorViewOpensAtTop(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	content := "line 1\nline 2\nline 3\nline 4\nline 5\n"
	e := NewEditorView(nil, "Page", "/tmp/page.md", content, false, 40, 6)
	if got := e.ta.Line(); got != 0 {
		t.Fatalf("editor should open at row 0, got row %d", got)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/views -run TestEditorViewOpensAtTop -v`
Expected: FAIL — cursor lands at the last row after `SetValue`.

- [ ] **Step 3: Move the cursor to the top in `NewEditorView`**

In `internal/views/editor.go`, in `NewEditorView`, after the `_ = e.ta.Focus()` line and before `e.baseline = e.Content()`, insert:

```go
	// SetValue leaves the cursor at the end of the buffer; Reset() (inside
	// SetValue) already put the viewport at the top. Move the cursor to the top
	// so cursor and viewport agree on open instead of the cursor sitting
	// off-screen at the bottom of a long page.
	for e.ta.Line() > 0 {
		e.ta.CursorUp()
	}
	e.ta.CursorStart()
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/views -run TestEditorViewOpensAtTop -v`
Expected: PASS

- [ ] **Step 5: Confirm no golden drift + full suite**

Run: `go vet ./... && go test ./...`
Expected: PASS with no golden changes (the editor goldens use short content and already render from the top; cursor-row position is not encoded in them). If a golden test fails, STOP and report — do not run `-update`.

- [ ] **Step 6: Commit**

```bash
git add internal/views/editor.go internal/views/editor_view_test.go
git commit -m "fix: open the in-app editor at the top of the buffer"
```

---

### Task 3: `replaceCurrentLine` helper + Enter bullet continuation

**Files:**
- Modify: `internal/views/editor.go` (`Update`, new `replaceCurrentLine`)
- Test: `internal/views/editor_view_test.go`

**Interfaces:**
- Consumes: `bulletPrefix`, `isEmptyBullet` (Task 1); `cursorLineSplit()`, `refreshCompleter()` (existing).
- Produces: `func (e *EditorView) replaceCurrentLine(newText string, newCol int)` — used here and by Task 4. `Update()` handles `enter` for bullets.

- [ ] **Step 1: Write the failing tests**

Add to `internal/views/editor_view_test.go`:

```go
// All four use a trailing "second" line so the Enter's effect is interior to
// the buffer and survives Content()'s trailing-newline normalization (Content
// collapses trailing blank lines to a single "\n", which would otherwise hide
// a newline inserted at end-of-buffer). The editor opens at row 0; CursorEnd()
// moves to the end of that first row before each Enter.

func TestEditorEnterContinuesBullet(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	e := NewEditorView(nil, "Page", "/tmp/page.md", "- first\nsecond\n", false, 40, 10)
	e.ta.CursorEnd()
	e.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if got := e.Content(); got != "- first\n- \nsecond\n" {
		t.Fatalf("continuation: got %q, want %q", got, "- first\n- \nsecond\n")
	}
}

func TestEditorEnterContinuesNestedBullet(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	e := NewEditorView(nil, "Page", "/tmp/page.md", "  - nested\nsecond\n", false, 40, 10)
	e.ta.CursorEnd()
	e.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if got := e.Content(); got != "  - nested\n  - \nsecond\n" {
		t.Fatalf("nested continuation: got %q, want %q", got, "  - nested\n  - \nsecond\n")
	}
}

func TestEditorEnterEmptyBulletTerminates(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	e := NewEditorView(nil, "Page", "/tmp/page.md", "- \nsecond\n", false, 40, 10)
	e.ta.CursorEnd()
	e.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if got := e.Content(); got != "\nsecond\n" {
		t.Fatalf("empty-bullet terminate: got %q, want %q", got, "\nsecond\n")
	}
}

func TestEditorEnterNonBulletInsertsNewline(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	e := NewEditorView(nil, "Page", "/tmp/page.md", "plain\nsecond\n", false, 40, 10)
	e.ta.CursorEnd()
	e.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if got := e.Content(); got != "plain\n\nsecond\n" {
		t.Fatalf("non-bullet newline: got %q, want %q", got, "plain\n\nsecond\n")
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/views -run 'TestEditorEnter' -v`
Expected: FAIL — Enter currently inserts a plain newline (e.g. continuation yields `"- first\n\n"`, not `"- first\n- \n"`); `replaceCurrentLine` undefined if referenced.

- [ ] **Step 3: Add `replaceCurrentLine`**

In `internal/views/editor.go`, add this method (e.g. just after `acceptCompletion`):

```go
// replaceCurrentLine rewrites the logical line the cursor is on: it deletes the
// line's existing runes (forward from column 0, exactly its rune length so the
// trailing newline is untouched), inserts newText, then places the cursor at
// column newCol. Used for empty-bullet termination and marker cycling.
func (e *EditorView) replaceCurrentLine(newText string, newCol int) {
	before, after := e.cursorLineSplit()
	oldLen := len([]rune(before + after))
	e.ta.CursorStart()
	for i := 0; i < oldLen; i++ {
		e.ta, _ = e.ta.Update(tea.KeyMsg{Type: tea.KeyDelete})
	}
	e.ta.InsertString(newText)
	e.ta.SetCursor(newCol)
}
```

- [ ] **Step 4: Intercept `enter` in `Update`**

In `internal/views/editor.go`, in the main `switch msg.String()` in `Update` (the one with `ctrl+s`/`esc`/`pgup`/`pgdown`), add a `case "enter":` before the closing brace of that switch (after the `pgdown` case):

```go
	case "enter":
		before, after := e.cursorLineSplit()
		line := before + after
		if isEmptyBullet(line) {
			e.replaceCurrentLine("", 0)
			e.refreshCompleter(false)
			return EditorResult{}, nil
		}
		if prefix, ok := bulletPrefix(line); ok {
			e.ta.InsertString("\n" + prefix)
			e.refreshCompleter(false)
			return EditorResult{}, nil
		}
		// Non-bullet: do not return — fall past the switch so the textarea
		// inserts a normal newline below.
```

This relies on Go switch semantics: a matched `case` that does not `return` continues execution after the `switch`, where the existing `e.ta, cmd = e.ta.Update(msg)` forwards the Enter to the textarea. (The completer-active block above already consumes `enter` to accept a candidate and returns first, so this case is reached only when the strip is closed.)

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go test ./internal/views -run 'TestEditorEnter' -v`
Expected: PASS (all four).

- [ ] **Step 6: Full suite + vet**

Run: `go vet ./... && go test ./...`
Expected: PASS, no golden drift.

- [ ] **Step 7: Commit**

```bash
git add internal/views/editor.go internal/views/editor_view_test.go
git commit -m "feat: continue/terminate - bullets on Enter in the editor"
```

---

### Task 4: `ctrl+t` workflow-marker cycle

**Files:**
- Modify: `internal/views/editor.go` (`Update`)
- Test: `internal/views/editor_view_test.go`

**Interfaces:**
- Consumes: `cycleMarkerLine` (Task 1); `replaceCurrentLine` (Task 3); `cursorLineSplit()`, `refreshCompleter()` (existing).
- Produces: `Update()` handles `ctrl+t`.

- [ ] **Step 1: Write the failing tests**

Add to `internal/views/editor_view_test.go`:

```go
func TestEditorCtrlTCyclesMarker(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	e := NewEditorView(nil, "Page", "/tmp/page.md", "- task\n", false, 40, 10)
	e.ta.CursorEnd()

	e.Update(tea.KeyMsg{Type: tea.KeyCtrlT})
	if got := e.Content(); got != "- TODO task\n" {
		t.Fatalf("plain->TODO: got %q", got)
	}
	e.Update(tea.KeyMsg{Type: tea.KeyCtrlT})
	if got := e.Content(); got != "- DONE task\n" {
		t.Fatalf("TODO->DONE: got %q", got)
	}
	e.Update(tea.KeyMsg{Type: tea.KeyCtrlT})
	if got := e.Content(); got != "- task\n" {
		t.Fatalf("DONE->plain: got %q", got)
	}
}

func TestEditorCtrlTNoOpOnNonBullet(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	e := NewEditorView(nil, "Page", "/tmp/page.md", "# heading\n", false, 40, 10)
	e.ta.CursorEnd()
	e.Update(tea.KeyMsg{Type: tea.KeyCtrlT})
	if got := e.Content(); got != "# heading\n" {
		t.Fatalf("ctrl+t on non-bullet should be a no-op, got %q", got)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/views -run 'TestEditorCtrlT' -v`
Expected: FAIL — `ctrl+t` is currently forwarded to the textarea (transpose), not handled.

- [ ] **Step 3: Intercept `ctrl+t` in `Update`**

In `internal/views/editor.go`, in the same main `switch msg.String()`, add after the new `enter` case:

```go
	case "ctrl+t":
		before, after := e.cursorLineSplit()
		oldCol := len([]rune(before))
		if newLine, newCol, ok := cycleMarkerLine(before+after, oldCol); ok {
			e.replaceCurrentLine(newLine, newCol)
			e.refreshCompleter(false)
		}
		return EditorResult{}, nil
```

(`ctrl+t` always returns — consuming the key even on a non-bullet, so the textarea's transpose binding never fires while editing.)

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/views -run 'TestEditorCtrlT' -v`
Expected: PASS (both).

- [ ] **Step 5: Full suite + vet**

Run: `go vet ./... && go test ./...`
Expected: PASS, no golden drift.

- [ ] **Step 6: Commit**

```bash
git add internal/views/editor.go internal/views/editor_view_test.go
git commit -m "feat: cycle TODO/DONE marker with ctrl+t in the editor"
```

---

### Task 5: Document the new keys (help overlay + AGENTS)

**Files:**
- Modify: `internal/views/help.go` (the `Edit` section of `helpSections`)
- Modify: `AGENTS.md`
- Test: `internal/views/help_test.go` (if it asserts on specific rows — otherwise none)

**Interfaces:** none (docs/help strings).

- [ ] **Step 1: Add help rows**

In `internal/views/help.go`, in the `helpSections` slice, find the `"Edit"` section's `rows` and add these two rows (after the `pgup/pgdn` row):

```go
		{"enter", "continue list bullet (empty bullet ends it)"},
		{"ctrl-t", "cycle TODO / DONE on the current bullet"},
```

- [ ] **Step 2: Update AGENTS.md**

In `AGENTS.md`, in the in-app editor paragraph (the one ending "...the cursor's current row is shown as raw source."), append:

```
Enter continues a `- ` bullet at the same indent (empty bullet ends the list);
`Ctrl+T` cycles the current bullet's workflow marker (plain → TODO → DONE); the
editor opens with the cursor at the top of the page.
```

- [ ] **Step 3: Verify build/tests**

Run: `go vet ./... && go test ./...`
Expected: PASS. If `help_test.go` golden/row assertions fail because the help panel grew, regenerate with `go test ./internal/views -update`, inspect the diff (only the Edit section gains two rows), and re-run. If it fails for any other reason, STOP and report.

- [ ] **Step 4: Commit**

```bash
git add internal/views/help.go AGENTS.md internal/views/testdata
git commit -m "docs: document editor bullet continuation + ctrl-t marker cycle"
```

---

## Self-Review

**Spec coverage:**
- Bullet continuation (non-bullet passthrough, plain, nested-indent, mid-line via InsertString, plain-marker-on-continue, empty-bullet terminate) → Task 1 (`bulletPrefix`/`isEmptyBullet`) + Task 3 (wiring + tests). ✓
- Marker cycle plain→TODO→DONE→plain, non-bullet no-op, indent/content preserved, cursor delta → Task 1 (`cycleMarkerLine`) + Task 4 (wiring). ✓
- Open at top → Task 2. ✓
- `replaceCurrentLine` shared helper → Task 3 (defined), Task 4 (reused). ✓
- Pure/impure split, regex-mirror convention, marker vocab TODO/DONE only → Task 1. ✓
- Docs (help + AGENTS) → Task 5. ✓
- Out-of-scope items absent (checkboxes, ordered lists, `*`/`+`, full marker set, multi-level outdent). ✓

**Placeholder scan:** No TBD/"handle edge cases" — every code step shows full code and exact expected output.

**Type consistency:** `bulletPrefix`/`isEmptyBullet`/`cycleMarkerLine`/`replaceCurrentLine` names and signatures match between defining and consuming tasks. `cycleMarkerLine(line, oldCol) → (newLine, newCol, ok)` used identically in Task 4. Enter case relies on Go's no-fallthrough-continues-after-switch — called out explicitly. `tea.KeyCtrlT`/`tea.KeyEnter`/`tea.KeyDelete` are the bubbletea key types; `e.ta.CursorEnd()` is used in tests to position the cursor.

**Risk note for the executor:** The `case "enter"` non-bullet path must NOT `return` — it falls through to the post-switch `e.ta.Update(msg)` that inserts the newline. If you add a `default:` clause or restructure the switch, preserve that fall-past behavior, or non-bullet Enter will stop inserting newlines. Verify with `TestEditorEnterNonBulletInsertsNewline`.
