# `[[`-autocomplete in the in-app editor — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** While editing in peekseq's in-app markdown editor, typing `[[` opens a live, fuzzy-filtered candidate list of page names; selecting one splices a finished `[[Page Name]]` into the buffer.

**Architecture:** A new `linkCompleter` type (`internal/views/complete.go`) holds pure completion logic — it takes the text before the cursor, derives an active/partial/candidate state, and reports what to insert. `EditorView` owns a `*linkCompleter`, recovers `(logical-row text, rune-column)` from the textarea via `Line()` + `LineInfo()` after each key, drives the completer, and renders a fixed candidate strip between the textarea and the status line. The textarea remains the single source of truth for buffer + cursor; accept is done by feeding backspace `KeyMsg`s then `InsertString`, so no `SetValue`/row-restore is needed. Completion writes nothing to disk — `internal/edit` stays the sole writer.

**Tech Stack:** Go 1.26, `charmbracelet/bubbles/textarea` v1.0.0, `charmbracelet/bubbletea` v1.3.10, `sahilm/fuzzy`, `charmbracelet/x/exp/teatest`. Reuses existing `pickerChoices`, `scrollWindow`, `clamp`, and the `styleSel`/`styleFaint`/`styleBorder` theme styles.

**Pre-commit gate (every commit):** `go vet ./... && go test ./...` — BOTH must pass. Run from the worktree root `/var/home/dhemas/Development/Projects/fiatcode/peekseq/.worktrees/link-autocomplete`.

---

## File Structure

- **Create `internal/views/complete.go`** — the `linkCompleter` type and its pure logic (`newLinkCompleter`, `refresh`, `extractPartial`, `buildCands`, `selected`, `moveUp`/`moveDown`, `dismiss`, `View`, `rows`). No textarea dependency.
- **Create `internal/views/complete_test.go`** — unit tests for the pure logic.
- **Modify `internal/views/editor.go`** — `EditorView` gains a `completer` field; `NewEditorView` grows a leading `*graph.Index` param; new helpers `textBeforeCursor`, `refreshCompleter`, `acceptCompletion`, `layout`; `Update`, `SetSize`, and `View` are extended.
- **Modify `internal/views/editor_view_test.go`** — update existing `NewEditorView` call sites for the new signature; add completion behavior tests.
- **Modify `internal/views/app.go`** — `enterEditor` passes `a.idx` to `NewEditorView`.
- **Modify `internal/views/help.go`** — add the `[[` completion keys to the editor help section.
- **Modify `AGENTS.md`, `README.md`, `CHANGELOG.md`** — document the feature.

---

## Task 1: `linkCompleter` pure logic

**Files:**
- Create: `internal/views/complete.go`
- Test: `internal/views/complete_test.go`

This task builds the completion controller in isolation — no `EditorView` wiring. It compiles and tests green on its own (unused package-level types/funcs are legal in Go).

- [ ] **Step 1: Write the failing tests**

Create `internal/views/complete_test.go`:

```go
package views

import "testing"

func TestExtractPartial(t *testing.T) {
	cases := []struct {
		name    string
		before  string
		want    string
		wantOK  bool
	}{
		{"open bracket simple", "see [[bar", "bar", true},
		{"empty partial just opened", "x [[", "", true},
		{"second open after a closed link", "see [[Foo]] and [[bar", "bar", true},
		{"closed link only", "see [[Foo]] x", "", false},
		{"no bracket at all", "plain text", "", false},
		{"partial contains closing bracket", "[[Foo]", "", false},
		{"page name with spaces stays one partial", "[[Meeting Notes", "Meeting Notes", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := extractPartial(tc.before)
			if got != tc.want || ok != tc.wantOK {
				t.Errorf("extractPartial(%q) = (%q, %v), want (%q, %v)", tc.before, got, ok, tc.want, tc.wantOK)
			}
		})
	}
}

func TestLinkCompleterRefresh(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	c := newLinkCompleter(loadFixture(t))

	// No open bracket -> inactive.
	c.refresh("just text")
	if c.active {
		t.Errorf("should be inactive with no open bracket")
	}

	// Open bracket with a matching partial -> active, fuzzy match present.
	c.refresh("link to [[Alp")
	if !c.active {
		t.Fatalf("should be active for [[Alp")
	}
	cand, ok := c.selected()
	if !ok || cand.name != "Alpha" || cand.create {
		t.Errorf("selected() = (%+v, %v), want Alpha (non-create)", cand, ok)
	}

	// A partial that matches nothing -> a single create row.
	c.refresh("[[Zxqv")
	got, ok := c.selected()
	if !ok || !got.create || got.name != "Zxqv" {
		t.Errorf("selected() = (%+v, %v), want create row for Zxqv", got, ok)
	}

	// Dates are NOT suppressed for the create row.
	c.refresh("[[2026-06-12")
	got, ok = c.selected()
	if !ok || !got.create || got.name != "2026-06-12" {
		t.Errorf("date create row: got (%+v, %v), want create row for the date", got, ok)
	}
}

func TestLinkCompleterDismissAndReopen(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	c := newLinkCompleter(loadFixture(t))
	c.refresh("[[Alp")
	c.dismiss()
	if c.active {
		t.Errorf("dismiss should deactivate")
	}
	// Same partial stays dismissed.
	c.refresh("[[Alp")
	if c.active {
		t.Errorf("re-deriving the same partial after dismiss should stay inactive")
	}
	// A changed partial reopens.
	c.refresh("[[Alph")
	if !c.active {
		t.Errorf("a changed partial should reopen the completer")
	}
}

func TestLinkCompleterEmptyPartialShowsRecent(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	c := newLinkCompleter(loadFixture(t))
	c.refresh("[[")
	if !c.active {
		t.Fatalf("[[ with empty partial should be active")
	}
	if len(c.cands) == 0 {
		t.Errorf("empty partial should list recent pages")
	}
	if c.cands[0].create {
		t.Errorf("recent list should not start with a create row")
	}
}

func TestLinkCompleterNilIndex(t *testing.T) {
	c := newLinkCompleter(nil)
	c.refresh("[[Alp")
	// With no index there are no real pages, but a non-empty partial still
	// offers a create row.
	if !c.active {
		t.Errorf("nil index with a non-empty partial should still offer create")
	}
	c.refresh("[[")
	if c.active {
		t.Errorf("nil index with an empty partial has nothing to show")
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/views/ -run 'TestExtractPartial|TestLinkCompleter' -v`
Expected: FAIL — `undefined: extractPartial`, `undefined: newLinkCompleter`.

- [ ] **Step 3: Write the implementation**

Create `internal/views/complete.go`:

```go
package views

import (
	"fmt"
	"strings"

	"github.com/sahilm/fuzzy"

	"git.fiatcode.dev/fiatcode/peekseq/internal/graph"
)

// maxCompleterRows caps how many candidate rows the strip shows at once; it
// scrolls within this window when the list is longer.
const maxCompleterRows = 8

// linkCandidate is one row in the completion strip. create marks the synthetic
// "＋ Create" row offered when the partial matches no existing page.
type linkCandidate struct {
	name   string
	create bool
}

// linkCompleter is the pure logic behind the editor's live [[ completion. It
// holds the page-name corpus and derives its active/partial/candidate state
// from the text before the cursor (refresh). It never touches the textarea;
// EditorView translates between the two.
type linkCompleter struct {
	choices   []pickerChoice // deduped, mtime-sorted page names (reused from picker)
	names     []string       // lockstep with choices, for fuzzy.Find
	active    bool
	dismissed bool   // user pressed esc; stays closed until the partial changes
	partial   string // runes between the open [[ and the cursor
	cands     []linkCandidate
	sel       int
}

func newLinkCompleter(idx *graph.Index) *linkCompleter {
	c := &linkCompleter{}
	if idx != nil {
		c.choices = pickerChoices(idx)
		c.names = make([]string, len(c.choices))
		for i, ch := range c.choices {
			c.names[i] = ch.name
		}
	}
	return c
}

// extractPartial finds the active [[ completion query in the text before the
// cursor: the runes after the nearest "[[" with no intervening bracket. It
// returns ("", false) when there is no open link to complete.
func extractPartial(before string) (string, bool) {
	i := strings.LastIndex(before, "[[")
	if i < 0 {
		return "", false
	}
	partial := before[i+2:]
	if strings.ContainsAny(partial, "[]") {
		return "", false
	}
	return partial, true
}

// refresh re-derives the completer state from the text before the cursor. It
// is called after every textarea-mutating key.
func (c *linkCompleter) refresh(before string) {
	partial, ok := extractPartial(before)
	if !ok {
		c.active = false
		c.dismissed = false
		c.partial = ""
		c.cands = nil
		return
	}
	if partial != c.partial {
		c.partial = partial
		c.dismissed = false
		c.sel = 0
	}
	if c.dismissed {
		c.active = false
		return
	}
	c.cands = c.buildCands(partial)
	c.active = len(c.cands) > 0
	if c.sel >= len(c.cands) {
		c.sel = 0
	}
}

// buildCands returns the candidate rows for a partial: fuzzy matches over the
// page corpus, or — when nothing matches a non-empty partial — a single
// create row. An empty partial lists the recent (mtime-sorted) pages.
func (c *linkCompleter) buildCands(partial string) []linkCandidate {
	var out []linkCandidate
	if partial == "" {
		for _, ch := range c.choices {
			out = append(out, linkCandidate{name: ch.name})
		}
		return out
	}
	for _, m := range fuzzy.Find(partial, c.names) {
		out = append(out, linkCandidate{name: m.Str})
	}
	if len(out) == 0 {
		out = append(out, linkCandidate{name: partial, create: true})
	}
	return out
}

func (c *linkCompleter) selected() (linkCandidate, bool) {
	if !c.active || c.sel < 0 || c.sel >= len(c.cands) {
		return linkCandidate{}, false
	}
	return c.cands[c.sel], true
}

func (c *linkCompleter) moveUp() {
	if c.sel > 0 {
		c.sel--
	}
}

func (c *linkCompleter) moveDown() {
	if c.sel < len(c.cands)-1 {
		c.sel++
	}
}

// dismiss closes the strip until the partial changes (esc).
func (c *linkCompleter) dismiss() {
	c.dismissed = true
	c.active = false
}

// rows is how many terminal rows the rendered strip occupies (candidates,
// capped, plus the box border), or 0 when inactive. EditorView uses this to
// shrink the textarea so the strip fits.
func (c *linkCompleter) rows() int {
	if !c.active || len(c.cands) == 0 {
		return 0
	}
	n := len(c.cands)
	if n > maxCompleterRows {
		n = maxCompleterRows
	}
	return n + 2 // top + bottom border
}

// View renders the candidate strip, or "" when inactive.
func (c *linkCompleter) View(width int) string {
	if !c.active || len(c.cands) == 0 {
		return ""
	}
	inner := width - 4
	if inner < 10 {
		inner = 10
	}
	start, end := scrollWindow(c.sel, len(c.cands), maxCompleterRows)
	var b strings.Builder
	for i := start; i < end; i++ {
		cand := c.cands[i]
		label := cand.name
		if cand.create {
			label = fmt.Sprintf("＋ Create %q", cand.name)
		}
		label = clamp(label, inner)
		marker := "   "
		if i == c.sel {
			marker = styleSel.Render(" ▶ ")
			label = styleSel.Render(label)
		} else {
			label = styleFaint.Render(label)
		}
		b.WriteString(marker)
		b.WriteString(label)
		if i < end-1 {
			b.WriteString("\n")
		}
	}
	return styleBorder.Width(inner + 2).Render(b.String())
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/views/ -run 'TestExtractPartial|TestLinkCompleter' -v`
Expected: PASS (all subtests).

- [ ] **Step 5: Run the full gate**

Run: `go vet ./... && go test ./...`
Expected: PASS. (Existing editor/picker tests still green; the new file is unused by other code so nothing else changes.)

- [ ] **Step 6: Commit**

```bash
git add internal/views/complete.go internal/views/complete_test.go
git commit -m "feat: add linkCompleter — pure [[-completion logic"
```

---

## Task 2: Wire `linkCompleter` into `EditorView`

**Files:**
- Modify: `internal/views/editor.go`
- Modify: `internal/views/app.go:308`
- Modify: `internal/views/editor_view_test.go`
- Test: `internal/views/editor_view_test.go`

- [ ] **Step 1: Update the `NewEditorView` signature and add the field/helpers (no behavior tests yet — make it compile)**

In `internal/views/editor.go`:

Add the field to the `EditorView` struct (after `width, height int`):

```go
	width, height int
	completer     *linkCompleter
```

Change the `NewEditorView` signature and wire the completer. Replace the current signature line and the construction block:

```go
func NewEditorView(idx *graph.Index, name, path, content string, isNew bool, width, height int) *EditorView {
	ta := textarea.New()
	ta.CharLimit = 0 // no length cap
	ta.MaxHeight = 0 // no line cap — pages can exceed textarea's default 99
	ta.ShowLineNumbers = false
	ta.Prompt = ""
	ta.SetValue(content)
	e := &EditorView{
		ta:        ta,
		path:      path,
		pageName:  name,
		baseline:  content,
		isNew:     isNew,
		width:     width,
		height:    height,
		completer: newLinkCompleter(idx),
	}
	e.SetSize(width, height)
	_ = e.ta.Focus() // blink cmd not needed here; the App calls Focus() again when it mounts the editor
	e.baseline = e.Content() // normalize so open-time dirty() is accurate
	e.refreshCompleter()
	return e
}
```

Add the import for graph at the top of `editor.go` (alongside the existing imports):

```go
import (
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"

	"git.fiatcode.dev/fiatcode/peekseq/internal/graph"
)
```

Add these helpers to `editor.go` (after the `dirty` method is fine):

```go
// textBeforeCursor returns the text on the current logical row up to the
// cursor. The textarea exposes no rune-offset getter, but Line() gives the row
// and LineInfo().StartColumn+ColumnOffset reconstructs the absolute column.
func (e *EditorView) textBeforeCursor() string {
	lines := strings.Split(e.ta.Value(), "\n")
	row := e.ta.Line()
	if row < 0 || row >= len(lines) {
		return ""
	}
	li := e.ta.LineInfo()
	col := li.StartColumn + li.ColumnOffset
	runes := []rune(lines[row])
	if col > len(runes) {
		col = len(runes)
	}
	return string(runes[:col])
}

// refreshCompleter re-derives completion state from the cursor position and
// re-lays-out the view so the strip fits.
func (e *EditorView) refreshCompleter() {
	e.completer.refresh(e.textBeforeCursor())
	e.layout()
}

// acceptCompletion splices the selected candidate into the buffer. For an
// existing page it deletes the typed partial (via backspaces, so the
// textarea's own cursor tracking stays correct) and inserts "Name]]". For the
// create row it keeps the typed name and only closes the link with "]]".
func (e *EditorView) acceptCompletion() {
	cand, ok := e.completer.selected()
	if !ok {
		return
	}
	if cand.create {
		e.ta.InsertString("]]")
	} else {
		for range []rune(e.completer.partial) {
			e.ta, _ = e.ta.Update(tea.KeyMsg{Type: tea.KeyBackspace})
		}
		e.ta.InsertString(cand.name + "]]")
	}
	e.refreshCompleter() // line now ends the link, so the completer deactivates
}

// layout sizes the textarea, reserving one row for the status line plus the
// completion strip's rows while it is active.
func (e *EditorView) layout() {
	e.ta.SetWidth(e.width)
	h := e.height - 1 - e.completer.rows()
	e.ta.SetHeight(max(1, h))
}
```

Update `SetSize` to route through `layout`:

```go
// SetSize resizes the textarea, reserving rows for the status line and the
// active completion strip.
func (e *EditorView) SetSize(w, h int) {
	e.width, e.height = w, h
	e.layout()
}
```

- [ ] **Step 2: Update `app.go` and the existing test call sites so the package compiles**

In `internal/views/app.go`, change line ~308:

```go
	a.editor = NewEditorView(a.idx, name, path, content, isNew, a.width, a.height)
```

In `internal/views/editor_view_test.go`, every existing `NewEditorView(...)` call gains a leading `nil` index argument (these tests don't exercise completion). For example:

```go
	e := NewEditorView(nil, "Alpha", "/tmp/Alpha.md", "first line\n", false, 80, 24)
```

Do this for all existing call sites in that file (the ones in `TestEditorView_LoadsContentAndTracksDirty`, `TestEditorView_ContentEndsWithSingleNewline`, `TestEditorView_CleanAfterSaveWithoutTrailingNewline`, `TestEditorView_CleanOnOpenWhenFileLacksTrailingNewline`, `TestEditorUpdate_CtrlSRequestsSaveStaysEditing`, `TestEditorUpdate_EscOnCleanExits`, `TestEditorUpdate_EscOnDirtyPromptsThenDiscard`, `TestEditorUpdate_ConfirmSaveExits`, `TestEditorUpdate_TypingInsertsAndIsDirty`, `TestEditorView_Golden`, `TestEditorView_ConfirmGolden`, `TestEditorUpdate_PageDownMovesCursor`, `TestEditorUpdate_CtrlCCancelsConfirm`, `TestEditorView_SetErrorRendersInView`, `TestEditorView_MarkSavedClearsError`).

- [ ] **Step 3: Verify the package compiles and existing tests still pass**

Run: `go build ./... && go test ./internal/views/ -run TestEditor -v`
Expected: PASS — the signature change is absorbed and existing editor behavior is unchanged (completer is inactive in all these cases).

- [ ] **Step 4: Commit the wiring**

```bash
git add internal/views/editor.go internal/views/app.go internal/views/editor_view_test.go
git commit -m "feat: wire linkCompleter into EditorView"
```

- [ ] **Step 5: Write the failing completion-behavior tests**

Add to `internal/views/editor_view_test.go`. (`key` and `loadFixture` already exist in the package.)

```go
// typeRunes feeds each rune of s to the editor as an individual key press,
// mirroring real typing so the completer re-derives after every keystroke.
func typeRunes(e *EditorView, s string) {
	for _, r := range s {
		e.Update(key(string(r)))
	}
}

func TestEditorCompletion_EnterInsertsExistingLink(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	e := NewEditorView(loadFixture(t), "Note", "/tmp/n.md", "", true, 80, 24)
	typeRunes(e, "see [[Alp")
	if !e.completer.active {
		t.Fatalf("completer should be active after typing [[Alp")
	}
	e.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if got := e.ta.Value(); got != "see [[Alpha]]" {
		t.Errorf("after accept: got %q, want %q", got, "see [[Alpha]]")
	}
	if e.completer.active {
		t.Errorf("completer should close after accepting")
	}
}

func TestEditorCompletion_TabAlsoAccepts(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	e := NewEditorView(loadFixture(t), "Note", "/tmp/n.md", "", true, 80, 24)
	typeRunes(e, "[[Alp")
	e.Update(tea.KeyMsg{Type: tea.KeyTab})
	if got := e.ta.Value(); got != "[[Alpha]]" {
		t.Errorf("tab accept: got %q, want %q", got, "[[Alpha]]")
	}
}

func TestEditorCompletion_CreateRowInsertsTypedName(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	e := NewEditorView(loadFixture(t), "Note", "/tmp/n.md", "", true, 80, 24)
	typeRunes(e, "[[Zxqv")
	e.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if got := e.ta.Value(); got != "[[Zxqv]]" {
		t.Errorf("create accept: got %q, want %q", got, "[[Zxqv]]")
	}
}

func TestEditorCompletion_EscDismissesKeepsBuffer(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	e := NewEditorView(loadFixture(t), "Note", "/tmp/n.md", "", true, 80, 24)
	typeRunes(e, "[[Alp")
	res, _ := e.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if res.Exit {
		t.Errorf("esc while completing should not exit the editor")
	}
	if e.completer.active {
		t.Errorf("esc should dismiss the completer")
	}
	if got := e.ta.Value(); got != "[[Alp" {
		t.Errorf("esc must not change the buffer: got %q", got)
	}
}

func TestEditorCompletion_ArrowsStealOnlyWhenActive(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	e := NewEditorView(loadFixture(t), "Note", "/tmp/n.md", "", true, 80, 24)
	typeRunes(e, "[[")
	before := e.ta.Value()
	e.Update(tea.KeyMsg{Type: tea.KeyDown})
	if e.completer.sel != 1 {
		t.Errorf("down should move the completer selection while active; sel=%d", e.completer.sel)
	}
	if e.ta.Value() != before {
		t.Errorf("down must not edit the buffer while completing")
	}
}
```

- [ ] **Step 6: Run the new tests to verify they fail**

Run: `go test ./internal/views/ -run TestEditorCompletion -v`
Expected: FAIL — `Update` does not yet intercept completion keys, so Enter inserts a newline, arrows move the cursor, etc.

- [ ] **Step 7: Add the completion-key orchestration to `Update`**

In `internal/views/editor.go`, in the `Update` method, insert the active-completer intercept block immediately **after** the `confirmingExit` block and **before** the existing `switch msg.String()`:

```go
	if e.completer.active {
		switch msg.String() {
		case keyUp:
			e.completer.moveUp()
			return EditorResult{}, nil
		case keyDown:
			e.completer.moveDown()
			return EditorResult{}, nil
		case keyEnter, "tab":
			e.acceptCompletion()
			return EditorResult{}, nil
		case keyEsc, "ctrl+c":
			e.completer.dismiss()
			e.layout()
			return EditorResult{}, nil
		}
	}
```

Then, in the existing `switch msg.String()`, update the `pgup`/`pgdown` cases to re-derive the completer after scrolling (the cursor moved):

```go
	case "pgup":
		e.scrollPage(-1)
		e.refreshCompleter()
		return EditorResult{}, nil
	case "pgdown":
		e.scrollPage(+1)
		e.refreshCompleter()
		return EditorResult{}, nil
	}
```

Finally, replace the fall-through textarea-forward block at the end of `Update` so it refreshes the completer after the key is applied:

```go
	var cmd tea.Cmd
	e.ta, cmd = e.ta.Update(msg)
	e.refreshCompleter()
	return EditorResult{}, cmd
```

- [ ] **Step 8: Run the new tests to verify they pass**

Run: `go test ./internal/views/ -run TestEditorCompletion -v`
Expected: PASS (all five).

- [ ] **Step 9: Run the full gate**

Run: `go vet ./... && go test ./...`
Expected: PASS.

- [ ] **Step 10: Commit**

```bash
git add internal/views/editor.go internal/views/editor_view_test.go
git commit -m "feat: live [[-completion key handling in the editor"
```

---

## Task 3: Render the candidate strip + golden

**Files:**
- Modify: `internal/views/editor.go` (`View`)
- Test: `internal/views/editor_view_test.go`
- Golden: `internal/views/testdata/TestEditorView_CompletionGolden.golden` (generated)

- [ ] **Step 1: Compose the strip into `EditorView.View`**

In `internal/views/editor.go`, replace `View`:

```go
func (e *EditorView) View() string {
	v := e.ta.View()
	if strip := e.completer.View(e.width); strip != "" {
		v += "\n" + strip
	}
	return v + "\n" + e.statusLine()
}
```

- [ ] **Step 2: Write the golden test**

Add to `internal/views/editor_view_test.go`:

```go
func TestEditorView_CompletionGolden(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	e := NewEditorView(loadFixture(t), "Note", "/tmp/n.md", "", true, 80, 16)
	typeRunes(e, "link to [[A")
	if !e.completer.active {
		t.Fatalf("completer should be active for the golden")
	}
	teatest.RequireEqualOutput(t, []byte(e.View()))
}
```

- [ ] **Step 3: Generate and inspect the golden**

Run: `go test ./internal/views/ -run TestEditorView_CompletionGolden -update`
Then **Read** `internal/views/testdata/TestEditorView_CompletionGolden.golden` and confirm visually:
- the textarea content `link to [[A` is shown,
- a bordered strip with candidate page names sits below it (Alpha selected/marked with ` ▶ `),
- the status line is last.

If the strip looks wrong, fix `linkCompleter.View` / `EditorView.View` and regenerate.

- [ ] **Step 4: Run the test against the golden**

Run: `go test ./internal/views/ -run TestEditorView_CompletionGolden -v`
Expected: PASS.

- [ ] **Step 5: Run the full gate**

Run: `go vet ./... && go test ./...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/views/editor.go internal/views/editor_view_test.go internal/views/testdata/TestEditorView_CompletionGolden.golden
git commit -m "feat: render the [[-completion candidate strip"
```

---

## Task 4: Docs & help

**Files:**
- Modify: `internal/views/help.go`
- Modify: `AGENTS.md`
- Modify: `README.md`
- Modify: `CHANGELOG.md`

- [ ] **Step 1: Inspect the current editor help section**

Run: `grep -n 'edit\|Edit\|\^S\|esc exit' internal/views/help.go`
Read the surrounding lines to match the existing format of the editor help entries.

- [ ] **Step 2: Add the completion keys to the editor help section**

In `internal/views/help.go`, in the editor/edit section, add entries describing the completion keys, matching the existing row format exactly. The entries to add (adapt wording/format to the file's existing style):
- `[[` — start a wiki-link; pick a page to complete it
- `↑/↓` — choose a completion (while the list is open)
- `enter / tab` — insert the selected link
- `esc` — dismiss the completion list

- [ ] **Step 3: Update the help golden if one exists**

Run: `go test ./internal/views/ -run TestHelp -update`
Then **Read** `internal/views/testdata/TestHelpGolden.golden` and confirm the new lines render correctly.

- [ ] **Step 4: Document the feature in the prose docs**

In `README.md`, in the editor/usage section, add a sentence: typing `[[` in the in-app editor opens a live page-name completion list; arrows choose, Enter/Tab insert `[[Page Name]]`, Esc dismisses, and an unmatched name offers a one-key "Create" to drop a red link.

In `AGENTS.md`, add a matching note wherever the editor's behavior is described.

In `CHANGELOG.md`, under the `[Unreleased]` section's `Added`, add:
```
- In-app editor: live `[[` wiki-link completion — fuzzy page-name list, Enter/Tab to insert `[[Page Name]]`, Esc to dismiss, and a create row for new (red) links.
```

- [ ] **Step 5: Run the full gate**

Run: `go vet ./... && go test ./...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/views/help.go internal/views/testdata AGENTS.md README.md CHANGELOG.md
git commit -m "docs: document [[-completion in help, README, AGENTS, CHANGELOG"
```

---

## Self-Review (completed during planning)

- **Spec coverage:** live trigger (Task 2 Update intercepts + refresh-after-key) ✓; fixed strip (Task 3 View) ✓; fuzzy via picker primitive (Task 1 `buildCands` uses `fuzzy.Find` over `pickerChoices` names) ✓; keymap incl. spaces-keep-open (`extractPartial` only stops on brackets) and Enter+Tab accept (Task 2 Step 7) ✓; Esc dismiss (Task 2) ✓; create row, dates not suppressed (Task 1 `buildCands`, `TestLinkCompleterRefresh` date case) ✓; cursor recovery via `Line()`+`LineInfo()` (Task 2 `textBeforeCursor`) ✓; accept via backspaces+`InsertString` (Task 2 `acceptCompletion`) ✓; no new writer (completion only mutates the in-memory buffer) ✓; tests incl. golden ✓.
- **Placeholder scan:** none — every code step shows complete code; the only adapt-to-style step is the help wording (Task 4 Step 2), with the exact entries listed.
- **Type consistency:** `linkCompleter`/`linkCandidate` fields and methods (`refresh`, `buildCands`, `selected`, `moveUp`, `moveDown`, `dismiss`, `rows`, `View`) are used consistently across tasks; `NewEditorView(idx, name, path, content, isNew, w, h)` signature matches every call site updated in Task 2.
