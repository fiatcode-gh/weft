# Editor Reposition Fix + Tab Indent Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix the in-app editor's viewport not following the cursor after a buffer-mutating intercept, and bind Tab/Shift+Tab to indent/de-indent the current line.

**Architecture:** A `syncViewport()` helper pokes the textarea's reposition (via the existing `repositionMsg{}`) after intercept mutations that bypass `ta.Update`. Tab/Shift+Tab reuse the existing `replaceCurrentLine` with two pure indent/de-indent helpers.

**Tech Stack:** Go 1.26+, `charmbracelet/bubbles@v1.0.0` (textarea), `charmbracelet/bubbletea` (KeyMsg).

## Global Constraints

- Run `go vet ./... && go test ./...` before every commit — both must pass (final state fully green).
- Touches only `internal/views/`. No disk writes; `internal/edit/` stays the only writer.
- Indent unit is 2 spaces (matches the graph's nesting). Tab indents the current line only (not a bullet subtree).
- Tab while the completion strip is OPEN still accepts a completion (the completer-active block consumes `tab` and returns before the main switch) — do not change that.
- No change to tinting, framing, save/dirty, marker-cycle/continuation logic, or open-at-top.
- Reposition mechanism: reuse the existing `repositionMsg{}` type and the `e.ta, _ = e.ta.Update(repositionMsg{})` pattern already in `layout()`. Do not add a new message type.

---

### Task 1: Viewport reposition fix (`syncViewport`)

Intercepts that mutate the buffer without routing a message through `ta.Update` leave the viewport stale (the textarea only calls `repositionView` at the end of `Update`). The continuation path (`InsertString`) is the visible bug; fold an explicit reposition into `replaceCurrentLine` too so all intercepts are covered.

**Files:**
- Modify: `internal/views/editor.go` (new `syncViewport`; `replaceCurrentLine`; the `enter` continuation path)
- Test: `internal/views/editor_view_test.go`

**Interfaces:**
- Consumes: `repositionMsg` (existing), `e.ta` (textarea.Model).
- Produces: `func (e *EditorView) syncViewport()`.

- [ ] **Step 1: Write the failing test**

Add to `internal/views/editor_view_test.go`:

```go
func TestEditorRepositionsAfterContinuation(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	// A buffer taller than the viewport, ending in a bullet with a unique tag.
	var sb strings.Builder
	for i := 0; i < 60; i++ {
		sb.WriteString("filler\n")
	}
	sb.WriteString("- ANCHOR")
	e := NewEditorView(nil, "Note", "/tmp/n.md", sb.String(), false, 80, 24)
	// CursorDown does not reposition the viewport, so this lands the cursor on
	// the bottom bullet while the viewport stays at the top.
	for {
		before := e.ta.Line()
		e.ta.CursorDown()
		if e.ta.Line() == before {
			break
		}
	}
	e.ta.CursorEnd()
	_ = e.View() // prime the textarea viewport content; it is still scrolled to the top
	if strings.Contains(e.View(), "ANCHOR") {
		t.Fatalf("precondition: the bottom bullet should be off-screen before Enter")
	}
	// Continuation inserts a new bullet below ANCHOR via InsertString (which does
	// not reposition); syncViewport must scroll the cursor region into view.
	e.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if !strings.Contains(e.View(), "ANCHOR") {
		t.Errorf("continuation must scroll the new bullet into view (viewport did not follow the cursor)")
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/views -run TestEditorRepositionsAfterContinuation -v`
Expected: FAIL at the final assertion — after Enter the viewport is still at the top, so "ANCHOR" is not in `View()`.

- [ ] **Step 3: Add `syncViewport` and call it from the intercepts**

In `internal/views/editor.go`, add the helper (e.g. just after `replaceCurrentLine`):

```go
// syncViewport repositions the textarea viewport onto the cursor after an
// intercept mutates the buffer without routing a message through ta.Update
// (the only place the textarea calls repositionView). repositionMsg is the
// content-neutral message layout() already uses for the same purpose.
func (e *EditorView) syncViewport() { e.ta, _ = e.ta.Update(repositionMsg{}) }
```

In `replaceCurrentLine`, add the reposition as the last line:

```go
func (e *EditorView) replaceCurrentLine(newText string, newCol int) {
	before, after := e.cursorLineSplit()
	oldLen := len([]rune(before + after))
	e.ta.CursorEnd()
	for i := 0; i < oldLen; i++ {
		e.ta, _ = e.ta.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	}
	e.ta.InsertString(newText)
	e.ta.SetCursor(newCol)
	e.syncViewport()
}
```

In `Update`'s `case "enter":`, add `e.syncViewport()` after the continuation insert. Change:

```go
	if prefix, ok := bulletPrefix(line); ok {
		e.ta.InsertString("\n" + prefix)
		e.refreshCompleter(false)
		return EditorResult{}, nil
	}
```
to:
```go
	if prefix, ok := bulletPrefix(line); ok {
		e.ta.InsertString("\n" + prefix)
		e.syncViewport()
		e.refreshCompleter(false)
		return EditorResult{}, nil
	}
```

(The empty-bullet terminate path calls `replaceCurrentLine`, which now repositions — no separate change needed there.)

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/views -run TestEditorRepositionsAfterContinuation -v`
Expected: PASS

- [ ] **Step 5: Full suite + vet**

Run: `go vet ./... && go test ./...`
Expected: PASS, no golden drift.

- [ ] **Step 6: Commit**

```bash
git add internal/views/editor.go internal/views/editor_view_test.go
git commit -m "fix: reposition editor viewport after bullet continuation"
```

---

### Task 2: Tab / Shift+Tab indent and de-indent

**Files:**
- Modify: `internal/views/edit_markdown.go` (new `indentLine`, `dedentLine`)
- Modify: `internal/views/editor.go` (`tab` / `shift+tab` cases in `Update`)
- Test: `internal/views/edit_markdown_test.go`, `internal/views/editor_view_test.go`

**Interfaces:**
- Consumes: `replaceCurrentLine` (existing), `cursorLineSplit()` (existing).
- Produces:
  - `func indentLine(line string) string`
  - `func dedentLine(line string) (string, int)` — returns the de-indented line and the number of leading spaces removed (0–2).

- [ ] **Step 1: Write the failing pure tests**

Add to `internal/views/edit_markdown_test.go`:

```go
func TestIndentLine(t *testing.T) {
	if got := indentLine("- foo"); got != "  - foo" {
		t.Errorf("indentLine(%q) = %q, want %q", "- foo", got, "  - foo")
	}
	if got := indentLine("  - bar"); got != "    - bar" {
		t.Errorf("indentLine(%q) = %q, want %q", "  - bar", got, "    - bar")
	}
}

func TestDedentLine(t *testing.T) {
	cases := []struct {
		line        string
		wantLine    string
		wantRemoved int
	}{
		{"  - foo", "- foo", 2},
		{" - foo", "- foo", 1},
		{"- foo", "- foo", 0},
		{"    x", "  x", 2},
	}
	for _, c := range cases {
		gotLine, gotRemoved := dedentLine(c.line)
		if gotLine != c.wantLine || gotRemoved != c.wantRemoved {
			t.Errorf("dedentLine(%q) = (%q,%d), want (%q,%d)", c.line, gotLine, gotRemoved, c.wantLine, c.wantRemoved)
		}
	}
}
```

- [ ] **Step 2: Write the failing integration tests**

Add to `internal/views/editor_view_test.go`:

```go
func TestEditorTabIndents(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	e := NewEditorView(nil, "Page", "/tmp/page.md", "- foo\nx\n", false, 40, 10)
	e.ta.CursorEnd() // end of "- foo" on row 0
	e.Update(tea.KeyMsg{Type: tea.KeyTab})
	if got := e.Content(); got != "  - foo\nx\n" {
		t.Fatalf("tab indent: got %q, want %q", got, "  - foo\nx\n")
	}
}

func TestEditorShiftTabDedents(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	e := NewEditorView(nil, "Page", "/tmp/page.md", "  - foo\nx\n", false, 40, 10)
	e.ta.CursorEnd()
	e.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	if got := e.Content(); got != "- foo\nx\n" {
		t.Fatalf("shift+tab dedent: got %q, want %q", got, "- foo\nx\n")
	}
}

func TestEditorShiftTabNoOpAtZeroIndent(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	e := NewEditorView(nil, "Page", "/tmp/page.md", "- foo\nx\n", false, 40, 10)
	e.ta.CursorEnd()
	e.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	if got := e.Content(); got != "- foo\nx\n" {
		t.Fatalf("shift+tab at zero indent should be a no-op: got %q", got)
	}
}
```

- [ ] **Step 3: Run the tests to verify they fail**

Run: `go test ./internal/views -run 'TestIndentLine|TestDedentLine|TestEditorTabIndents|TestEditorShiftTabDedents|TestEditorShiftTabNoOpAtZeroIndent' -v`
Expected: FAIL — `indentLine`/`dedentLine` undefined; Tab currently inserts a literal tab (not handled).

- [ ] **Step 4: Implement the pure helpers**

In `internal/views/edit_markdown.go`, add:

```go
// indentLine adds one 2-space indent level to the front of line.
func indentLine(line string) string { return "  " + line }

// dedentLine removes up to one 2-space indent level from the front of line,
// returning the new line and the number of leading spaces actually removed (0-2).
func dedentLine(line string) (string, int) {
	n := 0
	for n < 2 && n < len(line) && line[n] == ' ' {
		n++
	}
	return line[n:], n
}
```

- [ ] **Step 5: Wire `tab` / `shift+tab` in `Update`**

In `internal/views/editor.go`, in the main `switch msg.String()` (after the `ctrl+t` case, before the closing `}`), add:

```go
	case "tab":
		before, after := e.cursorLineSplit()
		oldCol := len([]rune(before))
		e.replaceCurrentLine(indentLine(before+after), oldCol+2)
		e.refreshCompleter(false)
		return EditorResult{}, nil
	case "shift+tab":
		before, after := e.cursorLineSplit()
		oldCol := len([]rune(before))
		newLine, removed := dedentLine(before + after)
		newCol := oldCol - removed
		if newCol < 0 {
			newCol = 0
		}
		e.replaceCurrentLine(newLine, newCol)
		e.refreshCompleter(false)
		return EditorResult{}, nil
```

(The completer-active block earlier in `Update` consumes `tab` to accept a completion and returns first, so this `case "tab"` is reached only when the strip is closed. `shift+tab` is not in the completer-active set; de-indenting while the strip is open is harmless.)

- [ ] **Step 6: Run the tests to verify they pass**

Run: `go test ./internal/views -run 'TestIndentLine|TestDedentLine|TestEditorTabIndents|TestEditorShiftTabDedents|TestEditorShiftTabNoOpAtZeroIndent' -v`
Expected: PASS (all five).

- [ ] **Step 7: Full suite + vet**

Run: `go vet ./... && go test ./...`
Expected: PASS, no golden drift.

- [ ] **Step 8: Commit**

```bash
git add internal/views/edit_markdown.go internal/views/editor.go internal/views/editor_view_test.go
git commit -m "feat: Tab/Shift+Tab indent and de-indent the current line in the editor"
```

---

### Task 3: Document the new keys

**Files:**
- Modify: `internal/views/help.go` (the `Edit` section of `helpSections`)
- Modify: `AGENTS.md`
- Test: regenerate `internal/views/testdata/TestHelpGolden.golden`

**Interfaces:** none (help strings / docs).

- [ ] **Step 1: Add help rows**

In `internal/views/help.go`, in the `"Edit"` section's `rows`, add after the `ctrl-t` row:

```go
		{"tab", "indent the current line"},
		{"shift-tab", "de-indent the current line"},
```

- [ ] **Step 2: Update AGENTS.md**

In `AGENTS.md`, in the in-app editor paragraph, append after the existing `Enter`/`Ctrl+T`/open-at-top sentence:

```
`Tab` / `Shift+Tab` indent / de-indent the current line by one 2-space level.
```

- [ ] **Step 3: Verify + regenerate the help golden**

Run: `go vet ./... && go test ./...`
If `TestHelpGolden` fails because the Edit section grew by two rows, regenerate it: `go test ./internal/views -update`, then inspect `git diff -- internal/views/testdata` — the ONLY change must be the two new rows (`tab`, `shift-tab`) in the Edit section. If any other golden changed or it fails for another reason, STOP and report.

- [ ] **Step 4: Re-run + commit**

Run: `go vet ./... && go test ./...` (expected: PASS)

```bash
git add internal/views/help.go AGENTS.md internal/views/testdata
git commit -m "docs: document editor Tab/Shift+Tab indent keys"
```

---

## Self-Review

**Spec coverage:**
- Reposition fix (syncViewport, fold into replaceCurrentLine + continuation poke) → Task 1. ✓
- Tab/Shift+Tab indent (pure helpers + wiring, 2-space unit, current line, strip-open Tab untouched) → Task 2. ✓
- Docs (help rows + AGENTS) → Task 3. ✓
- Deferred #2 (hanging indent) — out of scope, logged as weft TODO; no task. ✓

**Placeholder scan:** No TBD/vague steps — every code step shows full code and exact expected output.

**Type consistency:** `syncViewport()`, `indentLine(string) string`, `dedentLine(string) (string, int)`, `replaceCurrentLine(string, int)` named/typed identically across defining and consuming tasks. `tea.KeyEnter`/`tea.KeyTab`/`tea.KeyShiftTab`/`tea.KeyBackspace` are bubbletea key types; `e.ta.CursorEnd()`/`CursorDown()`/`Line()` are textarea methods used in tests.

**Risk notes for the executor:**
- Task 1's test relies on `CursorDown` NOT repositioning the viewport (verified against bubbles@v1.0.0) — that's why the precondition (ANCHOR off-screen) holds. If the precondition assertion fails, the textarea version's navigation behavior changed; stop and report rather than deleting the precondition.
- Task 2: do not remove the existing completer-active `tab` handling; the new `case "tab"` is only for the strip-closed path.
