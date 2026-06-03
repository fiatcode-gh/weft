# Back/Next Navigation — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add browser-style back/next navigation (`[` / `]`) to the page view, restoring viewport scroll offset and link-cursor when stepping through history.

**Architecture:** `App` owns a history stack of `{page, offset, cursor}` entries plus a current index. Every existing `a.page.SetPage(name)` call site routes through a new `a.navigate(name)` helper that captures departing state, truncates forward history, and pushes the new entry. New `[` / `]` keys in `modePage` walk the stack and call new `PageView.Restore(offset, cursor)` to recover scroll + cursor. `PageView` stays a pure renderer; the App is the navigator.

**Tech Stack:** Go 1.26, Bubble Tea, `charmbracelet/bubbles/viewport`, `charmbracelet/x/exp/teatest`.

**Spec:** `docs/superpowers/specs/2026-05-25-history-nav-design.md`

---

## Background for the implementer

`peekseq` is a read-only Bubble Tea TUI for browsing a Logseq graph. Key files:

- `internal/views/app.go` — root model. Has a `modeT` enum (`modePage`, `modePicker`, `modeSearch`, `modeBacklinks`, `modeTodos`, `modeHelp`). All overlay handlers eventually call `a.page.SetPage(name)` to navigate.
- `internal/views/page.go` — `PageView` wraps `viewport.Model`, owns the rendered body, the link cursor (`p.cursor int`, `-1` = no link selected), and scroll position (`p.vp.YOffset`).
- `internal/views/help.go` — overlay listing keybinds, organised as `helpSections []{title, rows}`.
- `internal/views/picker_test.go`, `page_test.go`, etc. — view-level tests using `teatest.RequireEqualOutput` for goldens and direct method calls for behaviour.

The fixture graph lives at `testdata/fixture-graph/`. Pages: `Alpha`, `Beta`, `proj/nested`. Journals: `2026_05_23`, `2026_05_24`. `Alpha` links to `Beta`, `proj/nested`, and (aliased) back to `Beta`; it has multiple wiki-links so it's good for cursor tests.

**Project rules (from `AGENTS.md` and global CLAUDE.md):**

- TDD: failing test first, then minimal implementation, then refactor.
- Run `go vet ./... && go test ./...` before every commit — both must pass.
- Conventional Commits (`feat:`, `fix:`, `refactor:`, `test:`, `docs:`, `chore:`).
- `teatest.RequireEqualOutput` compares against goldens under `internal/views/testdata/`. After intentional UI changes, regenerate with `go test ./... -update` and visually diff before staging.

---

## File map

- **Modify:** `internal/views/page.go` — add `Offset()`, `Cursor()`, `Restore(offset, cursor int)` methods.
- **Modify:** `internal/views/page_test.go` — add tests for the three new methods.
- **Modify:** `internal/views/app.go` — add `hist`, `histIdx`, `navigate`, `historyBack`, `historyForward`; seed the first entry in `tryInitPage`; route all existing `SetPage` callers through `navigate`; bind `[` and `]` in `modePage`.
- **Create:** `internal/views/app_test.go` — history behaviour tests.
- **Modify:** `internal/views/help.go` — new "History" section.

---

## Task 1: PageView Offset/Cursor/Restore

**Files:**
- Modify: `internal/views/page.go`
- Modify: `internal/views/page_test.go`

- [ ] **Step 1: Write the failing tests**

Append to `internal/views/page_test.go`:

```go
func TestPageViewOffsetCursorAccessors(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	idx := loadFixture(t)
	// Small viewport so HalfPageDown actually moves YOffset against
	// Alpha's ~10 styled lines of content.
	pv := NewPageView(idx, "Alpha", 80, 5)

	if got := pv.Offset(); got != 0 {
		t.Errorf("fresh Offset: want 0, got %d", got)
	}
	if got := pv.Cursor(); got != -1 {
		t.Errorf("fresh Cursor: want -1, got %d", got)
	}

	pv.CycleLink(+1)
	if got := pv.Cursor(); got != 0 {
		t.Errorf("after CycleLink(+1): want cursor 0, got %d", got)
	}

	pv.HalfPageDown()
	if got := pv.Offset(); got == 0 {
		t.Errorf("after HalfPageDown: expected non-zero offset, got 0")
	}
}

func TestPageViewRestoreRoundtrip(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	idx := loadFixture(t)
	pv := NewPageView(idx, "Alpha", 80, 5)

	pv.Restore(2, 1)
	if got := pv.Offset(); got != 2 {
		t.Errorf("Restore offset: want 2, got %d", got)
	}
	if got := pv.Cursor(); got != 1 {
		t.Errorf("Restore cursor: want 1, got %d", got)
	}
}

func TestPageViewRestoreClampsCursor(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	idx := loadFixture(t)
	pv := NewPageView(idx, "Alpha", 80, 24)

	// Cursor past the link count falls back to -1.
	pv.Restore(0, 9999)
	if got := pv.Cursor(); got != -1 {
		t.Errorf("over-range cursor: want -1, got %d", got)
	}

	// Negative cursor below -1 falls back to -1.
	pv.Restore(0, -5)
	if got := pv.Cursor(); got != -1 {
		t.Errorf("under-range cursor: want -1, got %d", got)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/views/ -run 'TestPageViewOffset|TestPageViewRestore' -v`

Expected: FAIL with "pv.Offset undefined" / "pv.Cursor undefined" / "pv.Restore undefined".

- [ ] **Step 3: Add the methods**

In `internal/views/page.go`, after the `LineDown`/`LineUp`/`HalfPage` block (around line 118), insert:

```go
// Offset returns the viewport's current scroll position (YOffset).
func (p *PageView) Offset() int { return p.vp.YOffset }

// Cursor returns the current link cursor index. -1 means no link selected.
func (p *PageView) Cursor() int { return p.cursor }

// Restore sets the viewport scroll offset and link cursor in one shot. The
// viewport's SetYOffset clamps offset against the current content height.
// Cursor is clamped to a valid link index; anything outside [0, len(Links))
// falls back to -1 (no link selected). Use after SetPage to recover
// scroll/cursor state captured before a navigation.
func (p *PageView) Restore(offset, cursor int) {
	p.vp.SetYOffset(offset)
	if cursor < 0 || cursor >= len(p.result.Links) {
		cursor = -1
	}
	p.cursor = cursor
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/views/ -run 'TestPageViewOffset|TestPageViewRestore' -v`

Expected: PASS for all three.

- [ ] **Step 5: Full vet + test sweep**

Run: `go vet ./... && go test ./...`

Expected: PASS, no vet warnings.

- [ ] **Step 6: Commit**

```bash
git add internal/views/page.go internal/views/page_test.go
git commit -m "feat: add Offset/Cursor/Restore helpers to PageView"
```

---

## Task 2: App history state + navigate helper

**Files:**
- Modify: `internal/views/app.go`
- Create: `internal/views/app_test.go`

This task adds the history backbone and routes every existing `SetPage`
caller through a new `navigate` helper. Back/forward keys come in Task 3
— this task lands the data plumbing first so Task 3 has clean ground.

- [ ] **Step 1: Write the failing tests**

Create `internal/views/app_test.go`:

```go
package views

import (
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// bootApp returns an App that has loaded the fixture index and initialised
// its PageView at a known size. Exposed for history-related tests; mirrors
// the production boot sequence (index load -> WindowSizeMsg -> tryInitPage)
// minus the async hop.
func bootApp(t *testing.T) *App {
	t.Helper()
	abs, err := filepath.Abs("../../testdata/fixture-graph")
	if err != nil {
		t.Fatal(err)
	}
	a := New(abs)
	// Drive the deferred index build synchronously.
	cmd := a.Init()
	if cmd == nil {
		t.Fatal("Init returned nil cmd")
	}
	msg := cmd()
	a.Update(msg)
	// Provide a real terminal size so tryInitPage can construct PageView.
	a.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	if a.page == nil {
		t.Fatal("PageView not constructed after boot")
	}
	return a
}

func TestHistorySeededOnFirstPage(t *testing.T) {
	a := bootApp(t)
	if len(a.hist) != 1 {
		t.Fatalf("hist len: want 1, got %d", len(a.hist))
	}
	if a.histIdx != 0 {
		t.Errorf("histIdx: want 0, got %d", a.histIdx)
	}
	if a.hist[0].page != a.page.Page() {
		t.Errorf("seed entry page: want %q, got %q", a.page.Page(), a.hist[0].page)
	}
	if a.hist[0].offset != 0 || a.hist[0].cursor != -1 {
		t.Errorf("seed offset/cursor: want 0/-1, got %d/%d",
			a.hist[0].offset, a.hist[0].cursor)
	}
}

func TestNavigatePushesAndCapturesDeparting(t *testing.T) {
	a := bootApp(t)

	// The bootApp page is today's journal, which typically has no file in
	// the fixture (so scroll/cursor mutations are no-ops). Jump to Alpha
	// first to get a page with real content + links to mutate.
	a.navigate("Alpha")
	if a.page.Page() != "Alpha" {
		t.Fatalf("setup: want Alpha, got %q", a.page.Page())
	}

	// Move cursor on Alpha so we have something non-trivial to capture.
	a.page.CycleLink(+1)
	a.page.CycleLink(+1)
	wantOffset := a.page.Offset()
	wantCursor := a.page.Cursor()
	if wantCursor < 0 {
		t.Fatalf("setup: expected cursor to advance on Alpha, got %d", wantCursor)
	}

	a.navigate("Beta")

	if a.page.Page() != "Beta" {
		t.Errorf("after navigate: page want Beta, got %q", a.page.Page())
	}
	if len(a.hist) != 3 {
		t.Fatalf("hist len: want 3, got %d", len(a.hist))
	}
	if a.histIdx != 2 {
		t.Errorf("histIdx: want 2, got %d", a.histIdx)
	}
	if a.hist[1].page != "Alpha" {
		t.Errorf("entry[1].page: want Alpha, got %q", a.hist[1].page)
	}
	if a.hist[1].offset != wantOffset || a.hist[1].cursor != wantCursor {
		t.Errorf("entry[1] captured state: want %d/%d, got %d/%d",
			wantOffset, wantCursor, a.hist[1].offset, a.hist[1].cursor)
	}
	if a.hist[2].page != "Beta" || a.hist[2].offset != 0 || a.hist[2].cursor != -1 {
		t.Errorf("entry[2]: want {Beta 0 -1}, got %+v", a.hist[2])
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/views/ -run 'TestHistorySeededOnFirstPage|TestNavigatePushesAndCapturesDeparting' -v`

Expected: FAIL with "a.hist undefined" / "a.histIdx undefined" / "a.navigate undefined".

- [ ] **Step 3: Add the history fields and helper to App**

In `internal/views/app.go`, modify the `App` struct to add the fields (after the `mode/width/height` block). Replace this block:

```go
	mode   modeT
	width  int
	height int
}
```

With:

```go
	mode   modeT
	width  int
	height int

	// Browser-style page history. hist[histIdx] is the entry currently on
	// screen. histIdx == -1 before the first page is shown.
	hist    []historyEntry
	histIdx int
}

type historyEntry struct {
	page   string
	offset int
	cursor int
}
```

Update `New` to initialise `histIdx`:

Replace:

```go
func New(graphPath string) *App {
	return &App{graphPath: graphPath, mode: modePage}
}
```

With:

```go
func New(graphPath string) *App {
	return &App{graphPath: graphPath, mode: modePage, histIdx: -1}
}
```

Update `tryInitPage` to seed the first history entry when it builds the page:

Replace:

```go
func (a *App) tryInitPage() {
	if a.page == nil && a.idx != nil && a.loadErr == nil && a.width > 0 {
		a.page = NewPageView(a.idx, todayJournalName(), a.width, a.height)
	}
}
```

With:

```go
func (a *App) tryInitPage() {
	if a.page == nil && a.idx != nil && a.loadErr == nil && a.width > 0 {
		name := todayJournalName()
		a.page = NewPageView(a.idx, name, a.width, a.height)
		a.hist = []historyEntry{{page: name, offset: 0, cursor: -1}}
		a.histIdx = 0
	}
}
```

Add the `navigate` helper. Place it next to `tryInitPage`:

```go
// navigate switches the page view to name and records the transition in
// history. The departing page's offset/cursor are captured into the current
// history entry, any forward history is truncated, then a fresh entry for
// the destination is pushed and becomes current.
func (a *App) navigate(name string) {
	if a.histIdx >= 0 && a.histIdx < len(a.hist) {
		a.hist[a.histIdx].offset = a.page.Offset()
		a.hist[a.histIdx].cursor = a.page.Cursor()
	}
	a.hist = append(a.hist[:a.histIdx+1], historyEntry{page: name, offset: 0, cursor: -1})
	a.histIdx = len(a.hist) - 1
	a.page.SetPage(name)
}
```

Reroute every existing `a.page.SetPage(...)` call site in `Update` to use
`a.navigate(...)`. Five call sites total:

In `modePicker`:

```go
if accept {
    a.page.SetPage(sel)
```

becomes:

```go
if accept {
    a.navigate(sel)
```

In `modeSearch`:

```go
if name := pageNameFromHitPath(a.idx, hit.FilePath); name != "" {
    a.page.SetPage(name)
}
```

becomes:

```go
if name := pageNameFromHitPath(a.idx, hit.FilePath); name != "" {
    a.navigate(name)
}
```

In `modeBacklinks`:

```go
if accept {
    a.page.SetPage(sel)
```

becomes:

```go
if accept {
    a.navigate(sel)
```

In `modeTodos`:

```go
if accept {
    a.page.SetPage(page)
```

becomes:

```go
if accept {
    a.navigate(page)
```

In `modePage` (the `enter` case):

```go
case "enter":
    if t := a.page.FollowCursor(); t != "" {
        a.page.SetPage(t)
    }
```

becomes:

```go
case "enter":
    if t := a.page.FollowCursor(); t != "" {
        a.navigate(t)
    }
```

Leave the `indexLoadedMsg` rebuild path alone — that one rebuilds the
`PageView` for the same page after `R` and is not a navigation.

- [ ] **Step 4: Run the new tests to verify they pass**

Run: `go test ./internal/views/ -run 'TestHistorySeededOnFirstPage|TestNavigatePushesAndCapturesDeparting' -v`

Expected: PASS.

- [ ] **Step 5: Full vet + test sweep**

Run: `go vet ./... && go test ./...`

Expected: PASS. The picker/search/backlinks/todos tests still pass because
`navigate` is a strict superset of `SetPage` from their perspective.

- [ ] **Step 6: Commit**

```bash
git add internal/views/app.go internal/views/app_test.go
git commit -m "feat: add page navigation history backbone"
```

---

## Task 3: Back/forward key bindings

**Files:**
- Modify: `internal/views/app.go`
- Modify: `internal/views/app_test.go`

- [ ] **Step 1: Write the failing tests**

Append to `internal/views/app_test.go`:

```go
func TestHistoryBackForward(t *testing.T) {
	a := bootApp(t)
	startPage := a.page.Page()

	a.navigate("Alpha")
	a.navigate("Beta")

	// Press '['
	a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("[")})
	if got := a.page.Page(); got != "Alpha" {
		t.Errorf("after [: want Alpha, got %q", got)
	}

	// '[' again -> startPage
	a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("[")})
	if got := a.page.Page(); got != startPage {
		t.Errorf("after second [: want %q, got %q", startPage, got)
	}

	// ']' -> Alpha
	a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("]")})
	if got := a.page.Page(); got != "Alpha" {
		t.Errorf("after ]: want Alpha, got %q", got)
	}
}

func TestHistoryBackAtStartNoop(t *testing.T) {
	a := bootApp(t)
	startPage := a.page.Page()
	startIdx := a.histIdx

	a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("[")})

	if got := a.page.Page(); got != startPage {
		t.Errorf("page should be unchanged: want %q, got %q", startPage, got)
	}
	if a.histIdx != startIdx {
		t.Errorf("histIdx should be unchanged: want %d, got %d", startIdx, a.histIdx)
	}
}

func TestHistoryForwardAtTailNoop(t *testing.T) {
	a := bootApp(t)
	a.navigate("Alpha")
	tailIdx := a.histIdx

	a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("]")})

	if got := a.page.Page(); got != "Alpha" {
		t.Errorf("page should be unchanged: want Alpha, got %q", got)
	}
	if a.histIdx != tailIdx {
		t.Errorf("histIdx should be unchanged: want %d, got %d", tailIdx, a.histIdx)
	}
}

func TestHistoryBranchTruncatesForward(t *testing.T) {
	a := bootApp(t)
	a.navigate("Alpha")
	a.navigate("Beta")

	// Back to Alpha; forward stack still has Beta.
	a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("[")})
	if a.page.Page() != "Alpha" {
		t.Fatalf("setup: want Alpha, got %q", a.page.Page())
	}

	// Branch: navigate to a different page. Forward (Beta) must be dropped.
	a.navigate("proj/nested")

	if a.page.Page() != "proj/nested" {
		t.Errorf("after branch: want proj/nested, got %q", a.page.Page())
	}
	if a.histIdx != len(a.hist)-1 {
		t.Errorf("after branch: histIdx should be at tail, got %d (len %d)",
			a.histIdx, len(a.hist))
	}

	// ']' is now a no-op — there is no forward history.
	a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("]")})
	if a.page.Page() != "proj/nested" {
		t.Errorf("forward after branch should be no-op: got %q", a.page.Page())
	}
}

func TestHistoryRestoresScrollAndCursor(t *testing.T) {
	a := bootApp(t)
	a.navigate("Alpha")

	// On Alpha: scroll + select a link.
	a.page.CycleLink(+1)
	a.page.CycleLink(+1)
	wantCursor := a.page.Cursor()

	a.navigate("Beta")

	// Back to Alpha — cursor should be restored.
	a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("[")})
	if a.page.Page() != "Alpha" {
		t.Fatalf("after [: want Alpha, got %q", a.page.Page())
	}
	if got := a.page.Cursor(); got != wantCursor {
		t.Errorf("restored cursor: want %d, got %d", wantCursor, got)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/views/ -run 'TestHistoryBackForward|TestHistoryBackAtStartNoop|TestHistoryForwardAtTailNoop|TestHistoryBranchTruncatesForward|TestHistoryRestoresScrollAndCursor' -v`

Expected: FAIL. The `[` and `]` keys are not yet bound, so the page stays on whatever the last `navigate` set it to.

- [ ] **Step 3: Add the handlers and key bindings**

In `internal/views/app.go`, add the two handler methods next to `navigate`:

```go
// historyBack walks one step backward in the history stack, restoring the
// stored offset/cursor for that entry. The departing page's current
// offset/cursor are saved into the current entry so a subsequent forward
// step lands where the user left off. No-op at the start of history.
func (a *App) historyBack() {
	if a.histIdx <= 0 {
		return
	}
	a.hist[a.histIdx].offset = a.page.Offset()
	a.hist[a.histIdx].cursor = a.page.Cursor()
	a.histIdx--
	target := a.hist[a.histIdx]
	a.page.SetPage(target.page)
	a.page.Restore(target.offset, target.cursor)
}

// historyForward walks one step forward in the history stack. Mirrors
// historyBack. No-op at the tail of history.
func (a *App) historyForward() {
	if a.histIdx < 0 || a.histIdx >= len(a.hist)-1 {
		return
	}
	a.hist[a.histIdx].offset = a.page.Offset()
	a.hist[a.histIdx].cursor = a.page.Cursor()
	a.histIdx++
	target := a.hist[a.histIdx]
	a.page.SetPage(target.page)
	a.page.Restore(target.offset, target.cursor)
}
```

In the `modePage` switch (around line 192), add the two new key cases.
Find this section:

```go
case "?":
    a.help = NewHelp()
    a.mode = modeHelp
case "R":
```

Insert the two cases between `"?"` and `"R"` (any place inside the modePage
switch is fine — these are independent of each other):

```go
case "[":
    a.historyBack()
case "]":
    a.historyForward()
```

- [ ] **Step 4: Run the new tests to verify they pass**

Run: `go test ./internal/views/ -run 'TestHistoryBackForward|TestHistoryBackAtStartNoop|TestHistoryForwardAtTailNoop|TestHistoryBranchTruncatesForward|TestHistoryRestoresScrollAndCursor' -v`

Expected: PASS.

- [ ] **Step 5: Full vet + test sweep**

Run: `go vet ./... && go test ./...`

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/views/app.go internal/views/app_test.go
git commit -m "feat: add [ and ] keys for back/forward navigation"
```

---

## Task 4: Help overlay update

**Files:**
- Modify: `internal/views/help.go`

The help overlay does not currently have a teatest golden file — `views/testdata/` is checked at the start, and only the existing snapshot tests reference it. If a golden exists for the help view, regenerate it. Otherwise this is a pure addition.

- [ ] **Step 1: Check whether a help golden exists**

Run: `ls internal/views/testdata/ | grep -i help`

If anything turns up, you'll regenerate it after the edit. If nothing turns up, no golden update is needed.

- [ ] **Step 2: Add the History section**

In `internal/views/help.go`, find the `helpSections` slice and insert a new section after `"Links"` and before `"Open"`:

Find:

```go
	{"Links", []helpRow{
		{"n / N", "next / previous wiki-link"},
		{"enter", "follow link under cursor"},
	}},
	{"Open", []helpRow{
```

Replace with:

```go
	{"Links", []helpRow{
		{"n / N", "next / previous wiki-link"},
		{"enter", "follow link under cursor"},
	}},
	{"History", []helpRow{
		{"[", "back"},
		{"]", "forward"},
	}},
	{"Open", []helpRow{
```

- [ ] **Step 3: Regenerate goldens if needed**

If Step 1 found a help golden, run: `go test ./internal/views/ -update -run 'Help'`

Then visually diff the golden file with `git diff internal/views/testdata/` and confirm only the History section was added.

If no help golden exists, skip this step.

- [ ] **Step 4: Full vet + test sweep**

Run: `go vet ./... && go test ./...`

Expected: PASS.

- [ ] **Step 5: Manual sanity check**

Run: `go build ./cmd/peekseq && ./peekseq --graph testdata/fixture-graph`

In the running TUI:

1. Press `?` — confirm the new "History" section shows `[` / `]`.
2. Press `?` again to close.
3. Press `ctrl+p`, type "alpha", `enter` → on Alpha.
4. Press `n` a couple times to move the cursor onto a link.
5. Press `ctrl+d` to scroll.
6. Press `enter` to follow the link → on Beta.
7. Press `[` → back on Alpha, with cursor and scroll roughly where you left them.
8. Press `]` → back on Beta.
9. Press `q` to quit.

- [ ] **Step 6: Commit**

```bash
git add internal/views/help.go
# Also stage any regenerated golden from Step 3.
git commit -m "docs: document [ and ] back/forward keys in help overlay"
```

---

## Out of scope (do not implement)

- History depth cap.
- Persistence across sessions.
- Status-bar history indicator.
- `Alt+left/right` bindings.
- Preserving scroll/cursor across `R` (index rebuild). The new `Offset/Cursor/Restore` methods make this trivial later, but it is a separate QOL ask.
