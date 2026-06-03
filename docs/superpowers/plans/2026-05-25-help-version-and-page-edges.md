# Help Version, Page-Edge Keys, and Scroll Indicator — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Show the running version inside the help overlay, add `g`/`G` keys to jump to the top/bottom of the current page, and add a faint scroll-position percentage on the right side of the status bar.

**Architecture:** Three independent QOL additions in `internal/views`. (A) Version plumbed `cmd/peekseq/main.go` → `views.New(graphPath, version)` → `App.version` → `NewHelp(version)`; help footer renders `? or esc to close` (left) and `peekseq <version>` (right) padded to the help body's max line width. (B) Two new `PageView` methods delegating to `viewport.GotoTop`/`GotoBottom`, plus `g`/`G` cases in `modePage`, plus a new "Browse" help row. (C) New `PageView.ScrollIndicator()` returning `""` for non-scrollable pages or `"NN%"` otherwise, consumed in `App.statusBar()` to the left of `? help`.

**Tech Stack:** Go 1.26, Bubble Tea, `charmbracelet/bubbles/viewport`, `charmbracelet/lipgloss`.

**Spec:** `docs/superpowers/specs/2026-05-25-help-version-and-page-edges-design.md`

---

## Background for the implementer

`peekseq` is a read-only Bubble Tea TUI for browsing a Logseq graph. Relevant files:

- `cmd/peekseq/main.go` — wiring entry point. Already has `resolvedVersion()` that returns the right version string for the running binary (ldflags > BuildInfo > `"dev"`).
- `internal/views/app.go` — root Bubble Tea model. `App.statusBar()` renders the bottom row beneath the page view. `App.Update`'s `modePage` switch is where new keys go. `tryInitPage` constructs the `PageView`.
- `internal/views/page.go` — `PageView` wraps `viewport.Model`. Existing methods `LineDown`/`LineUp`/`HalfPageDown`/`HalfPageUp` delegate one-line wrappers around viewport calls — follow that style for the new ones.
- `internal/views/help.go` — `Help` is a stateless overlay. `helpSections` slice drives rendering; the footer row is the last thing built by `View()`.
- `internal/views/app_test.go` — has `bootApp(t)` helper that drives the production boot sequence. Currently calls `views.New(abs)`; this needs to become `views.New(abs, "test")` so callers updating in lockstep with the signature change still compile.

The fixture graph at `testdata/fixture-graph/` contains pages `Alpha`, `Beta`, `proj/nested`. `Alpha` has ~10 styled lines, which is **long enough to scroll** when the viewport is small (height 5) but **fits entirely** when the viewport is tall (height 24). Both regimes are used in this plan.

`bubbles/viewport` v1.0.0 API used here:
- `GotoTop()` / `GotoBottom()` — set YOffset to extreme; return `[]string` (ignored).
- `ScrollPercent()` — returns clamped `[0, 1]`; returns `1.0` when `Height >= len(lines)`.
- `TotalLineCount()` — total number of lines including off-screen.
- `Height` — viewport height in rows.

**Project rules (from `AGENTS.md` + global CLAUDE.md):**

- TDD: failing test first, then minimal implementation.
- `go vet ./... && go test ./...` before every commit — both must pass.
- Conventional Commits (`feat:`, `fix:`, `refactor:`, `test:`, `docs:`, `chore:`).
- Commit directly on `main` (user confirmed for this branch of work).
- Avoid lipgloss color-profile caching issues by setting `TERM=dumb` and `NO_COLOR=1` in any new test that constructs a rendered view.

---

## File map

- **Modify:** `cmd/peekseq/main.go` — pass `resolvedVersion()` into `views.New`.
- **Modify:** `internal/views/app.go` — `New(graphPath, version)`, `version` field on `App`, pass version to `NewHelp`, add `g`/`G` cases in modePage, update right side of `statusBar`.
- **Modify:** `internal/views/help.go` — `NewHelp(version)`, `version` field on `Help`, footer with version, new `g / G` row in "Browse".
- **Modify:** `internal/views/page.go` — `GotoTop`, `GotoBottom`, `ScrollIndicator`.
- **Modify:** `internal/views/page_test.go` — Goto and ScrollIndicator tests.
- **Modify:** `internal/views/app_test.go` — `bootApp` signature fix, `TestPageEdgeKeys`, `TestAppStatusBarContainsScrollIndicator`.
- **Create:** `internal/views/help_test.go` — version footer tests.

---

## Task 1: Plumb version through and render in help footer

**Files:**
- Modify: `cmd/peekseq/main.go`
- Modify: `internal/views/app.go`
- Modify: `internal/views/help.go`
- Modify: `internal/views/app_test.go` (bootApp signature only)
- Create: `internal/views/help_test.go`

Bundles signature change + footer rendering because the signature change has cascading effects across files and produces no user-visible behaviour on its own. Splitting into a no-op refactor commit and a rendering commit would just add churn.

- [ ] **Step 1: Write the failing tests**

Create `internal/views/help_test.go`:

```go
package views

import (
	"strings"
	"testing"
)

func TestNewHelpRendersVersion(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	view := NewHelp("v0.1.2").View()
	if !strings.Contains(view, "peekseq v0.1.2") {
		t.Errorf("help view missing version segment; got:\n%s", view)
	}
	if !strings.Contains(view, "? or esc to close") {
		t.Errorf("help view missing close hint; got:\n%s", view)
	}
}

func TestNewHelpEmptyVersionHidesSegment(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	view := NewHelp("").View()
	if !strings.Contains(view, "? or esc to close") {
		t.Errorf("help view missing close hint; got:\n%s", view)
	}
	// The "peekseq " (with trailing space) prefix only appears as part of
	// the version segment. Empty version must not render it.
	if strings.Contains(view, "peekseq ") {
		t.Errorf("help view should omit version segment for empty version; got:\n%s", view)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/views/ -run 'TestNewHelp' -v`

Expected: FAIL with "too many arguments in call to NewHelp" (or similar compile error — `NewHelp` currently takes no args).

- [ ] **Step 3: Update `NewHelp` signature and add `version` field**

In `internal/views/help.go`, replace:

```go
type Help struct{}

func NewHelp() *Help { return &Help{} }
```

With:

```go
type Help struct {
	version string
}

func NewHelp(version string) *Help { return &Help{version: version} }
```

- [ ] **Step 4: Update `Help.View()` to render the version-aware footer**

In `internal/views/help.go`, replace the existing footer block at the end of `View()`:

```go
	b.WriteString("\n")
	b.WriteString(helpFaint.Render("? or esc to close"))
	return helpBorder.Render(b.String())
}
```

With:

```go
	// Footer: close hint on the left, version on the right, padded to the
	// max line width of the body built so far. Version segment is omitted
	// when h.version is empty.
	body := b.String()
	contentWidth := 0
	for _, line := range strings.Split(body, "\n") {
		if w := lipgloss.Width(line); w > contentWidth {
			contentWidth = w
		}
	}

	left := "? or esc to close"
	var footer string
	if h.version == "" {
		footer = left
	} else {
		right := "peekseq " + h.version
		gap := contentWidth - lipgloss.Width(left) - lipgloss.Width(right)
		if gap < 2 {
			// Version too long to fit inline alongside the close hint.
			footer = left + "\n" + right
		} else {
			footer = left + strings.Repeat(" ", gap) + right
		}
	}

	b.WriteString("\n")
	b.WriteString(helpFaint.Render(footer))
	return helpBorder.Render(b.String())
}
```

- [ ] **Step 5: Add `version` to `App` and route it into `NewHelp`**

In `internal/views/app.go`, update the `App` struct. Find the existing block (the `hist`/`histIdx` block ends the struct):

```go
	// Browser-style page history. hist[histIdx] is the entry currently on
	// screen. histIdx == -1 before the first page is shown.
	hist    []historyEntry
	histIdx int
}
```

Replace with:

```go
	// Browser-style page history. hist[histIdx] is the entry currently on
	// screen. histIdx == -1 before the first page is shown.
	hist    []historyEntry
	histIdx int

	// version is the binary version string shown in the help overlay
	// footer. Empty hides the version segment.
	version string
}
```

Update `New` to accept and store it. Replace:

```go
func New(graphPath string) *App {
	return &App{graphPath: graphPath, mode: modePage, histIdx: -1}
}
```

With:

```go
func New(graphPath, version string) *App {
	return &App{graphPath: graphPath, mode: modePage, histIdx: -1, version: version}
}
```

Update the `?` handler in `Update`. Find:

```go
case "?":
    a.help = NewHelp()
    a.mode = modeHelp
```

Replace with:

```go
case "?":
    a.help = NewHelp(a.version)
    a.mode = modeHelp
```

- [ ] **Step 6: Pass version from main**

In `cmd/peekseq/main.go`, find:

```go
app := views.New(graphPath)
```

Replace with:

```go
app := views.New(graphPath, resolvedVersion())
```

- [ ] **Step 7: Update the `bootApp` test helper signature**

In `internal/views/app_test.go`, find:

```go
	a := New(abs)
```

Replace with:

```go
	a := New(abs, "test")
```

- [ ] **Step 8: Run help tests to verify they pass**

Run: `go test ./internal/views/ -run 'TestNewHelp' -v`

Expected: PASS for both.

- [ ] **Step 9: Full vet + test sweep**

Run: `go vet ./... && go test ./...`

Expected: PASS. All existing tests still pass (the only behaviour change is the help footer; no other tests inspect it).

- [ ] **Step 10: Commit**

```bash
git add cmd/peekseq/main.go internal/views/app.go internal/views/app_test.go internal/views/help.go internal/views/help_test.go
git commit -m "feat: show running version in help overlay footer"
```

---

## Task 2: `g` / `G` to top / bottom

**Files:**
- Modify: `internal/views/page.go`
- Modify: `internal/views/page_test.go`
- Modify: `internal/views/app.go`
- Modify: `internal/views/app_test.go`
- Modify: `internal/views/help.go` (one row in helpSections)

- [ ] **Step 1: Write the failing tests**

Append to `internal/views/page_test.go`:

```go
func TestPageViewGotoTopBottom(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	idx := loadFixture(t)
	// Small viewport so Alpha (~10 styled lines) is scrollable.
	pv := NewPageView(idx, "Alpha", 80, 5)

	// Scroll a bit so GotoTop has somewhere to go back to.
	pv.HalfPageDown()
	if pv.Offset() == 0 {
		t.Fatalf("setup: HalfPageDown should have advanced offset; got 0")
	}

	pv.GotoTop()
	if got := pv.Offset(); got != 0 {
		t.Errorf("after GotoTop: want offset 0, got %d", got)
	}

	pv.GotoBottom()
	if got := pv.Offset(); got == 0 {
		t.Errorf("after GotoBottom: expected non-zero offset, got 0")
	}
}
```

Append to `internal/views/app_test.go`:

```go
func TestPageEdgeKeys(t *testing.T) {
	a := bootApp(t)
	// Navigate to Alpha so we have real content that can scroll. Boot
	// page (today's journal) is typically empty in the fixture.
	a.navigate("Alpha")

	// Shrink the viewport so Alpha is scrollable.
	a.Update(tea.WindowSizeMsg{Width: 80, Height: 5})

	// 'G' jumps to bottom.
	a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("G")})
	bottomOffset := a.page.Offset()
	if bottomOffset == 0 {
		t.Errorf("after G: expected non-zero offset, got 0")
	}

	// 'g' jumps back to top.
	a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("g")})
	if got := a.page.Offset(); got != 0 {
		t.Errorf("after g: want offset 0, got %d", got)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/views/ -run 'TestPageViewGotoTopBottom|TestPageEdgeKeys' -v`

Expected: FAIL. `GotoTop`/`GotoBottom` undefined; `g`/`G` keys not bound (offset stays 0 after G).

- [ ] **Step 3: Add `GotoTop` and `GotoBottom` to PageView**

In `internal/views/page.go`, find the existing block:

```go
// LineDown/LineUp/HalfPage delegates to viewport.
func (p *PageView) LineDown()     { p.vp.LineDown(1) }
func (p *PageView) LineUp()       { p.vp.LineUp(1) }
func (p *PageView) HalfPageDown() { p.vp.HalfViewDown() }
func (p *PageView) HalfPageUp()   { p.vp.HalfViewUp() }
```

Replace with:

```go
// LineDown/LineUp/HalfPage/Goto delegates to viewport.
func (p *PageView) LineDown()     { p.vp.LineDown(1) }
func (p *PageView) LineUp()       { p.vp.LineUp(1) }
func (p *PageView) HalfPageDown() { p.vp.HalfViewDown() }
func (p *PageView) HalfPageUp()   { p.vp.HalfViewUp() }
func (p *PageView) GotoTop()      { p.vp.GotoTop() }
func (p *PageView) GotoBottom()   { p.vp.GotoBottom() }
```

- [ ] **Step 4: Wire `g` and `G` in `App.Update`**

In `internal/views/app.go`, locate the `modePage` switch. Find the existing back/forward block:

```go
case "[":
    a.historyBack()
case "]":
    a.historyForward()
```

Insert the two new cases immediately after them (still inside the modePage switch):

```go
case "[":
    a.historyBack()
case "]":
    a.historyForward()
case "g":
    a.page.GotoTop()
case "G":
    a.page.GotoBottom()
```

- [ ] **Step 5: Add help row for `g / G`**

In `internal/views/help.go`, find the existing "Browse" section:

```go
{"Browse", []helpRow{
    {"j / k", "scroll one line"},
    {"ctrl-d", "half page down"},
    {"ctrl-u", "half page up"},
}},
```

Replace with:

```go
{"Browse", []helpRow{
    {"j / k", "scroll one line"},
    {"ctrl-d", "half page down"},
    {"ctrl-u", "half page up"},
    {"g / G", "top / bottom"},
}},
```

- [ ] **Step 6: Run the new tests to verify they pass**

Run: `go test ./internal/views/ -run 'TestPageViewGotoTopBottom|TestPageEdgeKeys' -v`

Expected: PASS.

- [ ] **Step 7: Full vet + test sweep**

Run: `go vet ./... && go test ./...`

Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add internal/views/page.go internal/views/page_test.go internal/views/app.go internal/views/app_test.go internal/views/help.go
git commit -m "feat: add g / G keys to jump to top / bottom of page"
```

---

## Task 3: Scroll indicator in status bar

**Files:**
- Modify: `internal/views/page.go`
- Modify: `internal/views/page_test.go`
- Modify: `internal/views/app.go`
- Modify: `internal/views/app_test.go`

- [ ] **Step 1: Write the failing tests**

Append to `internal/views/page_test.go`:

```go
func TestPageViewScrollIndicatorFitsViewport(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	idx := loadFixture(t)
	// Tall viewport so Alpha fits entirely — no scroll possible.
	pv := NewPageView(idx, "Alpha", 80, 100)

	if got := pv.ScrollIndicator(); got != "" {
		t.Errorf("non-scrollable indicator: want \"\", got %q", got)
	}
}

func TestPageViewScrollIndicatorTopBottom(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	idx := loadFixture(t)
	// Small viewport so Alpha is scrollable.
	pv := NewPageView(idx, "Alpha", 80, 5)

	pv.GotoTop()
	if got := pv.ScrollIndicator(); got != "0%" {
		t.Errorf("at top: want \"0%%\", got %q", got)
	}

	pv.GotoBottom()
	if got := pv.ScrollIndicator(); got != "100%" {
		t.Errorf("at bottom: want \"100%%\", got %q", got)
	}
}

func TestPageViewScrollIndicatorMid(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	idx := loadFixture(t)
	pv := NewPageView(idx, "Alpha", 80, 5)

	pv.GotoTop()
	pv.HalfPageDown()

	got := pv.ScrollIndicator()
	matched, err := regexp.MatchString(`^\d{1,2}%$`, got)
	if err != nil {
		t.Fatalf("regex error: %v", err)
	}
	if !matched {
		t.Errorf("mid-scroll: want NN%% (1-2 digits), got %q", got)
	}
}
```

This test file already imports `testing` and `github.com/charmbracelet/x/exp/teatest`. Add `"regexp"` to the import block at the top — find:

```go
import (
	"path/filepath"
	"testing"

	"github.com/charmbracelet/x/exp/teatest"

	"git.fiatcode.dev/fiatcode/peekseq/internal/graph"
)
```

Replace with:

```go
import (
	"path/filepath"
	"regexp"
	"testing"

	"github.com/charmbracelet/x/exp/teatest"

	"git.fiatcode.dev/fiatcode/peekseq/internal/graph"
)
```

Append to `internal/views/app_test.go`:

```go
func TestAppStatusBarContainsScrollIndicator(t *testing.T) {
	a := bootApp(t)
	a.navigate("Alpha")
	// Shrink the viewport so Alpha is scrollable.
	a.Update(tea.WindowSizeMsg{Width: 80, Height: 5})

	a.page.GotoBottom()

	bar := a.statusBar()
	if !strings.Contains(bar, "100%") {
		t.Errorf("status bar missing 100%% indicator; got:\n%s", bar)
	}
	if !strings.Contains(bar, "? help") {
		t.Errorf("status bar missing close hint; got:\n%s", bar)
	}
}

func TestAppStatusBarHidesIndicatorWhenFits(t *testing.T) {
	a := bootApp(t)
	a.navigate("Alpha")
	// Force a very tall viewport so Alpha is guaranteed to fit regardless
	// of future content changes.
	a.Update(tea.WindowSizeMsg{Width: 80, Height: 200})

	bar := a.statusBar()
	if strings.Contains(bar, "%") {
		t.Errorf("status bar should hide percentage when page fits; got:\n%s", bar)
	}
	if !strings.Contains(bar, "? help") {
		t.Errorf("status bar missing close hint; got:\n%s", bar)
	}
}
```

Add `"strings"` to the import block at the top of `app_test.go`. Find:

```go
import (
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)
```

Replace with:

```go
import (
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/views/ -run 'TestPageViewScrollIndicator|TestAppStatusBar' -v`

Expected: FAIL — `ScrollIndicator` undefined.

- [ ] **Step 3: Add `ScrollIndicator` to PageView**

In `internal/views/page.go`, locate the existing accessor block (just above the styles `var (...)` block, or wherever `Offset`/`Cursor` live):

```go
// Offset returns the viewport's current scroll position (YOffset).
func (p *PageView) Offset() int { return p.vp.YOffset }

// Cursor returns the current link cursor index. -1 means no link selected.
func (p *PageView) Cursor() int { return p.cursor }
```

Add a new method directly below `Cursor`:

```go
// ScrollIndicator returns "" when the page fits the viewport (no scroll
// possible), otherwise "NN%" — 0% at the top, 100% at the bottom.
// viewport.ScrollPercent is already clamped to [0, 1] and returns exactly
// 0 at YOffset=0 and 1 at the max offset.
func (p *PageView) ScrollIndicator() string {
	if p.vp.TotalLineCount() <= p.vp.Height {
		return ""
	}
	return fmt.Sprintf("%d%%", int(p.vp.ScrollPercent()*100))
}
```

`fmt` is already imported in `page.go` (used by `StatusLine`). No import changes needed.

- [ ] **Step 4: Update `App.statusBar()` to include the indicator**

In `internal/views/app.go`, find `statusBar`:

```go
func (a *App) statusBar() string {
	left := a.page.StatusLine()
	right := statusFaint.Render("? help")
	width := a.width
	if width <= 0 {
		width = lipgloss.Width(left) + 2 + lipgloss.Width(right)
	}
	rule := statusRule.Render(strings.Repeat("─", width))
	gap := width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		gap = 1
	}
	return rule + "\n" + left + strings.Repeat(" ", gap) + right
}
```

Replace with:

```go
func (a *App) statusBar() string {
	left := a.page.StatusLine()
	rightText := "? help"
	if ind := a.page.ScrollIndicator(); ind != "" {
		rightText = ind + "  " + rightText
	}
	right := statusFaint.Render(rightText)
	width := a.width
	if width <= 0 {
		width = lipgloss.Width(left) + 2 + lipgloss.Width(right)
	}
	rule := statusRule.Render(strings.Repeat("─", width))
	gap := width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		gap = 1
	}
	return rule + "\n" + left + strings.Repeat(" ", gap) + right
}
```

- [ ] **Step 5: Run the new tests to verify they pass**

Run: `go test ./internal/views/ -run 'TestPageViewScrollIndicator|TestAppStatusBar' -v`

Expected: PASS for all five.

- [ ] **Step 6: Full vet + test sweep**

Run: `go vet ./... && go test ./...`

Expected: PASS.

- [ ] **Step 7: Manual sanity check**

Run: `go build ./cmd/peekseq && ./peekseq --graph testdata/fixture-graph`

In the running TUI:

1. Press `?` — confirm "History" + "Browse" sections show the right keys, and the footer shows `peekseq dev` (or whatever version the local build resolved to).
2. Press `esc` to close help.
3. Press `ctrl+p`, type "alpha", `enter` → on Alpha.
4. Resize the terminal small enough that Alpha scrolls, or scroll with `ctrl-d` if it already does at your current size.
5. Confirm the status bar right side now shows `NN%  ? help`.
6. Press `g` → top, indicator shows `0%`.
7. Press `G` → bottom, indicator shows `100%`.
8. Resize the terminal large enough that Alpha fits — the percentage disappears, leaving just `? help`.
9. Press `q` to quit.

If you cannot run interactively, build to verify compilation and document that step.

- [ ] **Step 8: Commit**

```bash
git add internal/views/page.go internal/views/page_test.go internal/views/app.go internal/views/app_test.go
git commit -m "feat: show scroll position in status bar"
```

---

## Out of scope (do not implement)

- `gg` chord for top.
- `Home`/`End` aliases.
- Refreshing the version after rebuild.
- Showing version anywhere outside the help overlay.
- Customising the indicator format via env var.
