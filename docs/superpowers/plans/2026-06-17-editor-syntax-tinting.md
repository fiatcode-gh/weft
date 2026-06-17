# Editor Syntax Tinting + Framing Parity Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make weft's in-app editor (`internal/views/editor.go`) visually match the read view — same left inset, and live tinting of headings, blockquotes, code-fence delimiters, task markers, and `[[wiki-links]]`.

**Architecture:** A pure `string → string` decorator post-processes `textarea.View()` output. Each visible row that carries no ANSI escape (every row except the cursor's own row, which textarea renders with its `CursorLine` background) is classified and tinted; the cursor row passes through verbatim ("reveal raw source where you edit"). A separate, small `layout()` change insets the textarea by 2 columns to match Glamour's document margin. No editing/save/completion logic changes.

**Tech Stack:** Go 1.26+, `charmbracelet/bubbles@v1.0.0` (textarea), `charmbracelet/lipgloss`, `charmbracelet/x/exp/teatest` (golden frame tests), `sahilm/fuzzy` (unrelated, in completer).

## Global Constraints

- Run `go vet ./... && go test ./...` before every commit — both must pass.
- `internal/edit/` stays the only disk writer; this work touches only `internal/views/` (render layer). No writes.
- Reuse the existing palette: `colorHighlight` (`internal/views/theme.go`) for links; mirror `internal/render/page.go`'s task-marker color palette locally in `views` (the render symbols are unexported, and `theme.go`'s note documents that mirroring ~4 marker colors across the `views`/`render` boundary is the deliberate convention — do NOT introduce a shared theme package).
- Numeric ANSI colors only (honor terminal theme + `NO_COLOR`), consistent with the rest of the codebase.
- Glamour's document left margin is **2 columns** (measured: paragraphs/bullets at lead=2). The editor inset constant is `2`.
- After any intentional golden change, run `go test ./internal/views -update` and **visually diff** the `.golden` before staging.

---

### Task 1: Editor framing inset (2-column left margin)

Match the read view's left margin so text doesn't jump flush-left on entering the editor. The textarea content width shrinks by the inset; the rendered editor view (textarea + completion strip + status line) is prefixed with the inset spaces so the whole editor aligns.

**Files:**
- Modify: `internal/views/editor.go` (`layout()`, `View()`, add `editorInset` const)
- Test: `internal/views/editor_view_test.go` (add a test); goldens under `internal/views/testdata/`

**Interfaces:**
- Consumes: existing `EditorView` fields `width`, `ta`, `completer`, `statusLine()`.
- Produces: `const editorInset = 2`; `View()` output where every line begins with 2 leading spaces. `Content()`, dirty-tracking, and completion behavior are unchanged.

- [ ] **Step 1: Write the failing test**

Add to `internal/views/editor_view_test.go`:

```go
func TestEditorViewIsLeftInset(t *testing.T) {
	e := NewEditorView(nil, "Page", "/tmp/page.md", "hello world\n", false, 40, 10)
	out := e.View()
	for i, line := range strings.Split(out, "\n") {
		if strings.TrimSpace(line) == "" {
			continue // padding/blank rows need not be inset
		}
		if !strings.HasPrefix(line, "  ") {
			t.Fatalf("line %d not inset by 2 spaces: %q", i, line)
		}
	}
}
```

If `strings` isn't already imported in the test file, add it.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/views -run TestEditorViewIsLeftInset -v`
Expected: FAIL — current `View()` emits lines flush at column 0.

- [ ] **Step 3: Implement the inset**

In `internal/views/editor.go`, add the constant near the top (after the `editorMode` consts):

```go
// editorInset is the left margin (in columns) applied to the whole editor
// view so its text occupies the same horizontal box as the Glamour-rendered
// read view, whose standard-style document margin is 2 columns. Without this,
// switching from read to edit jumps the text flush-left.
const editorInset = 2
```

In `layout()`, reduce the textarea width by the inset. Change:

```go
	e.ta.SetWidth(e.width)
```
to:
```go
	e.ta.SetWidth(max(1, e.width-editorInset))
```

In `View()`, prefix every line with the inset and shrink the strip's width budget. Replace the whole `View()` body:

```go
func (e *EditorView) View() string {
	v := e.ta.View()
	if strip := e.completer.View(max(1, e.width-editorInset)); strip != "" {
		v += "\n" + strip
	}
	v += "\n" + e.statusLine()
	return indentBlock(v, editorInset)
}
```

Add the helper (in `editor.go`):

```go
// indentBlock prefixes every line of s with n spaces. Blank lines are left
// empty so the inset never adds trailing whitespace to padding rows.
func indentBlock(s string, n int) string {
	pad := strings.Repeat(" ", n)
	lines := strings.Split(s, "\n")
	for i, ln := range lines {
		if ln != "" {
			lines[i] = pad + ln
		}
	}
	return strings.Join(lines, "\n")
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/views -run TestEditorViewIsLeftInset -v`
Expected: PASS

- [ ] **Step 5: Regenerate and inspect goldens**

Run: `go test ./internal/views -update`
Then inspect the diff of `internal/views/testdata/*.golden` (e.g. `git diff -- internal/views/testdata`): editor frames should now show a 2-space left margin; nothing else should change. If a non-editor golden changed, stop and investigate.

- [ ] **Step 6: Full suite + vet**

Run: `go vet ./... && go test ./...`
Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add internal/views/editor.go internal/views/editor_view_test.go internal/views/testdata
git commit -m "feat: inset in-app editor to match read-view left margin"
```

---

### Task 2: Pure markdown row-tinting module

The tinting core, with zero textarea/state dependency: classify a plain row, tint it, and skip rows that already carry ANSI (the cursor row). Fully unit-tested in isolation.

**Files:**
- Create: `internal/views/edit_tint.go`
- Test: `internal/views/edit_tint_test.go`

**Interfaces:**
- Consumes: `colorHighlight`, `styleTitle`, `styleFaint` (from `theme.go`).
- Produces:
  - `func tintView(view string) string` — splits on `"\n"`; any line containing an ESC byte (`0x1b`) is returned verbatim, every other line goes through `tintLine`.
  - `func tintLine(line string) string` — classifies and tints one plain row; returns a normal row unchanged (no ANSI added).
  - `var editorLinkTint`, `var editorMarkerTint map[string]lipgloss.Style` — local palette mirrors.

- [ ] **Step 1: Write the failing tests**

Create `internal/views/edit_tint_test.go`:

```go
package views

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

const esc = "\x1b"

// hasSGR reports whether s contains any ANSI escape (i.e. was styled).
func hasSGR(s string) bool { return strings.Contains(s, esc) }

func TestTintLineNormalUnchanged(t *testing.T) {
	in := "just a plain paragraph"
	if got := tintLine(in); got != in {
		t.Fatalf("normal line should be unchanged; got %q", got)
	}
}

func TestTintLineHeadingIsBold(t *testing.T) {
	got := tintLine("# Heading")
	want := lipgloss.NewStyle().Bold(true).Render("# Heading")
	if got != want {
		t.Fatalf("heading not bold:\n got %q\nwant %q", got, want)
	}
}

func TestTintLineBlockquoteIsFaint(t *testing.T) {
	got := tintLine("> a quote")
	want := lipgloss.NewStyle().Faint(true).Render("> a quote")
	if got != want {
		t.Fatalf("blockquote not faint:\n got %q\nwant %q", got, want)
	}
}

func TestTintLineCodeFenceIsFaint(t *testing.T) {
	got := tintLine("```go")
	if !hasSGR(got) {
		t.Fatalf("code fence should be styled; got %q", got)
	}
}

func TestTintLineWikiLinkTinted(t *testing.T) {
	got := tintLine("see [[Foo Bar]] now")
	link := lipgloss.NewStyle().Foreground(colorHighlight).Render("[[Foo Bar]]")
	if !strings.Contains(got, link) {
		t.Fatalf("wiki link span not tinted:\n got %q\nwant substring %q", got, link)
	}
	if !strings.HasPrefix(got, "see ") || !strings.HasSuffix(got, " now") {
		t.Fatalf("text around link altered: %q", got)
	}
}

func TestTintLineTaskMarkerColored(t *testing.T) {
	got := tintLine("- TODO buy milk")
	marker := editorMarkerTint["TODO"].Render("TODO")
	if !strings.Contains(got, marker) {
		t.Fatalf("task marker not colored:\n got %q\nwant substring %q", got, marker)
	}
	if !strings.HasPrefix(got, "- ") {
		t.Fatalf("bullet prefix altered: %q", got)
	}
}

func TestTintLineHeadingWithLinkComposes(t *testing.T) {
	got := tintLine("# See [[Foo]]")
	// The link span must be both bold (heading base) and link-colored.
	want := lipgloss.NewStyle().Foreground(colorHighlight).Bold(true).Render("[[Foo]]")
	if !strings.Contains(got, want) {
		t.Fatalf("heading+link did not compose bold+color:\n got %q\nwant substring %q", got, want)
	}
}

func TestTintViewSkipsRowsWithSGR(t *testing.T) {
	cursorRow := lipgloss.NewStyle().Background(lipgloss.Color("0")).Render("# Heading on cursor row")
	in := "# normal heading\n" + cursorRow
	out := tintView(in)
	lines := strings.Split(out, "\n")
	if lines[1] != cursorRow {
		t.Fatalf("cursor row (pre-styled) must pass through verbatim:\n got %q\nwant %q", lines[1], cursorRow)
	}
	if lines[0] != lineToBold("# normal heading") {
		t.Fatalf("non-cursor heading should be tinted; got %q", lines[0])
	}
}

// lineToBold mirrors the expected heading render for the assertion above.
func lineToBold(s string) string { return lipgloss.NewStyle().Bold(true).Render(s) }
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/views -run 'TestTint' -v`
Expected: FAIL to compile — `tintLine`, `tintView`, `editorMarkerTint` undefined.

- [ ] **Step 3: Implement the tinting module**

Create `internal/views/edit_tint.go`:

```go
package views

import (
	"regexp"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Row-classification patterns for the in-app editor's live tinting. These
// intentionally mirror (a subset of) internal/render/page.go's patterns rather
// than importing them: those symbols are unexported, and theme.go documents
// that mirroring the handful of marker colors across the views/render boundary
// is preferred over a premature shared package.
var (
	editorHeadingRe = regexp.MustCompile(`^#{1,6}(\s|$)`)
	editorQuoteRe   = regexp.MustCompile(`^\s*>`)
	editorFenceRe   = regexp.MustCompile("^\\s*```")
	editorTaskRe    = regexp.MustCompile(`^(\s*-\s+)(TODO|DOING|LATER|WAITING|DONE|CANCELED|CANCELLED|NOW)\b`)
	editorLinkRe    = regexp.MustCompile(`\[\[[^\]\n]*\]\]`)
)

// editorLinkTint is the wiki-link color, matching the read view (linkStyle in
// internal/render/page.go uses color 12) and theme.go's colorHighlight.
var editorLinkTint = lipgloss.NewStyle().Foreground(colorHighlight)

// editorMarkerTint mirrors internal/render/page.go's taskMarkerStyles so the
// editor, read view, and todos dashboard color workflow markers identically.
var editorMarkerTint = map[string]lipgloss.Style{
	"TODO":      lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Bold(true),
	"DOING":     lipgloss.NewStyle().Foreground(lipgloss.Color("11")).Bold(true),
	"LATER":     lipgloss.NewStyle().Foreground(lipgloss.Color("12")).Bold(true),
	"WAITING":   lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Bold(true),
	"DONE":      lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Bold(true),
	"CANCELED":  lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Strikethrough(true),
	"CANCELLED": lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Strikethrough(true),
	"NOW":       lipgloss.NewStyle().Foreground(lipgloss.Color("13")).Bold(true),
}

// tintSpan is a byte range within a row that gets its own style, composed over
// the row's whole-line base style.
type tintSpan struct {
	start, end int
	style      lipgloss.Style
}

// tintView decorates the textarea's rendered output. Each line is tinted
// EXCEPT lines that already contain an ANSI escape — in practice the only such
// line is the cursor's current row, which textarea renders with its CursorLine
// background. Leaving it verbatim both avoids clobbering the cursor styling and
// gives the deliberate "raw source on the line you're editing" affordance.
func tintView(view string) string {
	lines := strings.Split(view, "\n")
	for i, ln := range lines {
		if strings.ContainsRune(ln, 0x1b) {
			continue // pre-styled (cursor row / end-of-buffer filler)
		}
		lines[i] = tintLine(ln)
	}
	return strings.Join(lines, "\n")
}

// tintLine classifies one plain (ANSI-free) row and returns it styled. A normal
// row (no heading/quote/fence/task class and no wiki-link) is returned
// unchanged so it stays ANSI-free.
func tintLine(line string) string {
	if strings.TrimSpace(line) == "" {
		return line
	}

	base, hasBase := lineBaseStyle(line)
	spans := collectSpans(line)

	if !hasBase && len(spans) == 0 {
		return line
	}
	if len(spans) == 0 {
		return base.Render(line)
	}
	return renderSpans(line, base, hasBase, spans)
}

// lineBaseStyle returns the whole-line style for a row's block class and
// whether any class matched. Task-marker lines carry no whole-line style (only
// the marker token is colored, via collectSpans), so they return ok=false here.
func lineBaseStyle(line string) (lipgloss.Style, bool) {
	switch {
	case editorHeadingRe.MatchString(line):
		return lipgloss.NewStyle().Bold(true), true
	case editorFenceRe.MatchString(line):
		return lipgloss.NewStyle().Faint(true), true
	case editorQuoteRe.MatchString(line):
		return lipgloss.NewStyle().Faint(true), true
	}
	return lipgloss.Style{}, false
}

// collectSpans gathers the inline token spans for a row: every wiki-link, plus
// the workflow marker on a task line. Spans are returned sorted by start and
// are non-overlapping for these patterns.
func collectSpans(line string) []tintSpan {
	var spans []tintSpan
	for _, loc := range editorLinkRe.FindAllStringIndex(line, -1) {
		spans = append(spans, tintSpan{loc[0], loc[1], editorLinkTint})
	}
	if m := editorTaskRe.FindStringSubmatchIndex(line); m != nil {
		// Submatch 2 (group index 2) is the marker word: indices m[4]:m[5].
		marker := line[m[4]:m[5]]
		if st, ok := editorMarkerTint[marker]; ok {
			spans = append(spans, tintSpan{m[4], m[5], st})
		}
	}
	sort.Slice(spans, func(i, j int) bool { return spans[i].start < spans[j].start })
	return spans
}

// renderSpans rebuilds the row by segments: gap text gets the base style, span
// text gets its own style composed over the base (so a link inside a heading is
// both colored and bold). Building by concatenation (rather than nesting
// styles) avoids premature ANSI resets clobbering surrounding styling.
func renderSpans(line string, base lipgloss.Style, hasBase bool, spans []tintSpan) string {
	var b strings.Builder
	pos := 0
	emitGap := func(s string) {
		if s == "" {
			return
		}
		if hasBase {
			b.WriteString(base.Render(s))
		} else {
			b.WriteString(s)
		}
	}
	for _, sp := range spans {
		if sp.start < pos { // defensive: skip an overlapping span
			continue
		}
		emitGap(line[pos:sp.start])
		style := sp.style
		if hasBase {
			style = style.Inherit(base) // keep span color, gain base bold/faint
		}
		b.WriteString(style.Render(line[sp.start:sp.end]))
		pos = sp.end
	}
	emitGap(line[pos:])
	return b.String()
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/views -run 'TestTint' -v`
Expected: PASS (all `TestTint*`).

- [ ] **Step 5: Full suite + vet**

Run: `go vet ./... && go test ./...`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/views/edit_tint.go internal/views/edit_tint_test.go
git commit -m "feat: pure markdown row-tinting module for the editor"
```

---

### Task 3: Wire tinting into the editor view

Route the textarea output through `tintView` inside `EditorView.View()`, before the inset is applied.

**Files:**
- Modify: `internal/views/editor.go` (`View()`)
- Test: `internal/views/editor_view_test.go`; goldens under `internal/views/testdata/`

**Interfaces:**
- Consumes: `tintView` (Task 2), `indentBlock` + `editorInset` (Task 1).
- Produces: `View()` whose textarea region is tinted (non-cursor rows) and inset.

- [ ] **Step 1: Write the failing test**

Add to `internal/views/editor_view_test.go`:

```go
func TestEditorViewTintsHeadings(t *testing.T) {
	e := NewEditorView(nil, "Page", "/tmp/page.md", "# Title\n\nbody text\n", false, 40, 10)
	out := e.View()
	// The heading row should carry bold SGR somewhere in the view.
	boldTitle := lipgloss.NewStyle().Bold(true).Render("# Title")
	if !strings.Contains(out, boldTitle) {
		t.Fatalf("heading not tinted bold in editor view:\n%q", out)
	}
}
```

Ensure the test file imports `"github.com/charmbracelet/lipgloss"` (add if missing).

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/views -run TestEditorViewTintsHeadings -v`
Expected: FAIL — `View()` does not yet tint.

- [ ] **Step 3: Wire `tintView` into `View()`**

In `internal/views/editor.go`, change the first line of `View()` from:

```go
	v := e.ta.View()
```
to:
```go
	v := tintView(e.ta.View())
```

(The completion strip and status line are appended after and are not run through `tintView`.)

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/views -run TestEditorViewTintsHeadings -v`
Expected: PASS

- [ ] **Step 5: Regenerate and inspect goldens**

Run: `go test ./internal/views -update`
Inspect `git diff -- internal/views/testdata`: editor frames with headings/links/tasks/quotes should now show tinting; the cursor's current row must remain its plain-with-cursor rendering. Confirm no non-editor golden changed.

- [ ] **Step 6: Full suite + vet**

Run: `go vet ./... && go test ./...`
Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add internal/views/editor.go internal/views/editor_view_test.go internal/views/testdata
git commit -m "feat: tint markdown syntax live in the in-app editor"
```

---

### Task 4: Docs — help/AGENTS note

Record the new editor behavior so it's discoverable.

**Files:**
- Modify: `AGENTS.md` (the `internal/edit/` / editor description paragraph)

**Interfaces:** none (prose only).

- [ ] **Step 1: Update AGENTS.md**

In `AGENTS.md`, in the intro paragraph describing the in-app editor (the sentence about `e` saving on `Ctrl+S`), append one sentence:

```
The in-app editor live-tints markdown (headings, blockquotes, code-fence
delimiters, task markers, and `[[wiki-links]]`) and insets text to match the
read view's left margin; the cursor's current row is shown as raw source.
```

- [ ] **Step 2: Verify build/tests unaffected**

Run: `go vet ./... && go test ./...`
Expected: PASS (docs-only change).

- [ ] **Step 3: Commit**

```bash
git add AGENTS.md
git commit -m "docs: note editor syntax tinting + inset in AGENTS.md"
```

---

## Self-Review

**Spec coverage:**
- Framing parity (inset, aligned strip/status) → Task 1. ✓
- Line-level tinting (headings bold, blockquotes/fence faint, task markers via mirrored palette) → Task 2 (`lineBaseStyle`, `collectSpans`/`editorMarkerTint`). ✓
- Inline `[[wiki-link]]` tinting with composition over line base → Task 2 (`collectSpans`/`renderSpans`, `TestTintLineHeadingWithLinkComposes`). ✓
- Cursor-row stays raw → Task 2 (`tintView` skips ANSI rows) + Task 3 wiring + golden check. ✓
- Theme/NO_COLOR fidelity → numeric ANSI palette reused; lipgloss no-ops under NO_COLOR. ✓
- Out-of-scope (code-block bodies, non-link inline tokens, cursor-row tinting) → not implemented, by design. ✓
- Pure-function unit-testability + golden regen → Tasks 2 and 3. ✓

**Placeholder scan:** No TBD/TODO/"handle edge cases" — every code step shows full code. ✓

**Type consistency:** `tintView`/`tintLine`/`lineBaseStyle`/`collectSpans`/`renderSpans`/`tintSpan`/`editorLinkTint`/`editorMarkerTint`/`editorInset`/`indentBlock` are named identically across the tasks that define and consume them. `editorTaskRe` group index 2 → submatch byte indices `m[4]:m[5]` (group 0 = whole, group 1 = prefix, group 2 = marker). ✓

**Risk note for the executor:** Task 2's `tintView` relies on non-cursor rows being ANSI-free under default textarea styles (`Text` style is empty; only `CursorLine` carries a background). The editor does not customize `FocusedStyle`, so this holds. If a future change sets `FocusedStyle.Text` to a colored style, `tintView` would no-op on all rows — revisit then.
