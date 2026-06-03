# peekseq Structural Cleanup Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Remove duplication and one layer violation from the peekseq TUI — factor the four list overlays onto a shared base, extract ripgrep into its own package, unify App's overlay dispatch behind an `Overlay` interface, and centralize styles/keys — with zero behavior change.

**Architecture:** Pure refactor under a green test suite. New `internal/search` package owns rg execution + JSON parsing. New `internal/views` files (`overlay.go`, `listbox.go`, `scroll.go`, `theme.go`, `keys.go`) hold the shared abstractions. `App` drops its `modeT` enum and five typed overlay pointers for one `active Overlay`. Golden frames must stay byte-identical — **never** run `go test -update` in this plan.

**Tech Stack:** Go 1.26, Bubble Tea, lipgloss, `charmbracelet/x/exp/teatest` (golden frames), `sahilm/fuzzy`, ripgrep (`rg`) on PATH.

**Reference spec:** `docs/superpowers/specs/2026-05-29-structural-cleanup-design.md`

---

## Conventions for every task

- The verification command throughout is:
  ```bash
  go vet ./... && go test ./...
  ```
  Expected: all four packages `ok`, **no** golden diffs. If any golden frame
  differs, STOP and investigate — do not `-update`.
- Commits use Conventional Commits. Each task is its own commit.
- These are refactors: behavior is unchanged, so most tasks make the edit, run
  the suite green, and commit. Genuinely new code (Task 3's pure functions) is
  written test-first.

## File structure (created / modified)

```
internal/search/search.go          CREATE — Hit, Span, Run, runRipgrep, parseJSON
internal/search/search_test.go     CREATE — parsing/Run tests moved from views
internal/views/keys.go             CREATE — repeated key-string constants
internal/views/theme.go            CREATE — shared styles + palette colors
internal/views/scroll.go           CREATE — scrollWindow, clampInt (pure)
internal/views/scroll_test.go      CREATE — unit tests for the pure functions
internal/views/listbox.go          CREATE — embedded listBox base
internal/views/overlay.go          CREATE — Overlay interface + OverlayResult
internal/views/app.go              MODIFY — generic dispatch, drop modeT/pointers
internal/views/picker.go           MODIFY — embed listBox, use shared theme/keys
internal/views/search.go           MODIFY — use internal/search, embed listBox, Overlay
internal/views/backlinks.go        MODIFY — embed listBox, Overlay
internal/views/todos.go            MODIFY — embed listBox, Overlay
internal/views/page.go             MODIFY — use shared theme styles
internal/views/help.go             MODIFY — Overlay (OverlayResult), shared theme
internal/views/app_overlay_test.go MODIFY — assert against a.active
internal/views/app_dispatch_test.go MODIFY — relocate pageNameFromHitPath test
internal/views/search_test.go      MODIFY — split parsing tests out, adapt signatures
```

---

## Task 1: Key-string constants + delete stale comment

Centralize the key strings that repeat across `App` and the overlays. Scope is
deliberately limited to keys used in **more than one file** (YAGNI): App-only
single-use keys (`ctrl+p`, `/`, `g`, `G`, `R`, `n`, `N`, `.`, `<`, `>`, `[`,
`]`, `?`, `ctrl+d`, `ctrl+u`) stay as literals.

**Files:**
- Create: `internal/views/keys.go`
- Modify: `internal/views/picker.go`, `internal/views/search.go`, `internal/views/backlinks.go`, `internal/views/todos.go`, `internal/views/help.go`, `internal/views/app.go`

- [ ] **Step 1: Create the constants file**

Create `internal/views/keys.go`:

```go
package views

// Key strings shared by the App key handler and the overlay Update methods.
// These mirror the tea.KeyMsg.String() values Bubble Tea produces. Only keys
// used in more than one file live here; single-use keys stay as literals at
// their call site.
const (
	keyEsc       = "esc"
	keyEnter     = "enter"
	keyUp        = "up"
	keyDown      = "down"
	keyK         = "k"
	keyJ         = "j"
	keyCtrlK     = "ctrl+k"
	keyCtrlJ     = "ctrl+j"
	keyQ         = "q"
	keySpace     = "space"
	keyBackspace = "backspace"
)
```

- [ ] **Step 2: Replace literals in the overlay switch statements**

Apply these exact `replace_all` edits (each replaces a `case` label or its
matching string literal; only the listed strings change):

In `internal/views/picker.go`:
- `case "esc":` → `case keyEsc:`
- `case "enter":` → `case keyEnter:`
- `case "up", "ctrl+k":` → `case keyUp, keyCtrlK:`
- `case "down", "ctrl+j":` → `case keyDown, keyCtrlJ:`
- in `consumeKey`: `case "backspace":` → `case keyBackspace:` and `case "space":` → `case keySpace:`

In `internal/views/search.go` (`Update`):
- `case "esc":` → `case keyEsc:`
- `case "enter":` → `case keyEnter:`
- `case "up", "ctrl+k":` → `case keyUp, keyCtrlK:`
- `case "down", "ctrl+j":` → `case keyDown, keyCtrlJ:`
- `case "backspace":` → `case keyBackspace:`
- `case " ", "space":` → `case " ", keySpace:`

In `internal/views/backlinks.go` (`Update`):
- `case "esc", "b":` → `case keyEsc, "b":`
- `case "up", "k", "ctrl+k":` → `case keyUp, keyK, keyCtrlK:`
- `case "down", "j", "ctrl+j":` → `case keyDown, keyJ, keyCtrlJ:`
- `case "enter":` → `case keyEnter:`

In `internal/views/todos.go` (`Update`):
- `case "esc", "q":` → `case keyEsc, keyQ:`
- `case "up", "k":` → `case keyUp, keyK:`
- `case "down", "j":` → `case keyDown, keyJ:`
- `case "enter":` → `case keyEnter:`

In `internal/views/help.go` (`Update`):
- `case "esc", "?", "q":` → `case keyEsc, "?", keyQ:`

In `internal/views/app.go` — replace only the keys that have shared constants;
leave single-use literals (`ctrl+p`, `/`, `g`, `G`, `R`, `n`, `N`, `.`, `<`,
`>`, `[`, `]`, `?`, `ctrl+d`, `ctrl+u`, `ctrl+c`) as-is:
- `case "q", "ctrl+c":` → `case keyQ, "ctrl+c":` (both occurrences — the
  loading-state switch ~line 273 and the modePage switch ~line 347)
- `case "enter":` → `case keyEnter:`
- `case "j", "down":` → `case keyJ, keyDown:`
- `case "k", "up":` → `case keyK, keyUp:`

- [ ] **Step 3: Delete the self-contradicting comment**

In `internal/views/app.go`, inside the `tea.WindowSizeMsg` case, delete these
three comment lines that precede the `if a.help != nil` block:

```go
		// Propagate to any active overlay so its scroll-window budget and
		// inner-width calculations track the new terminal size on resize.
		// Help is content-sized and doesn't expose a SetSize.
```

Replace with a single accurate line:

```go
		// Propagate the new size to any open overlay so its scroll-window
		// budget and inner-width tracking stay correct on resize.
```

- [ ] **Step 4: Run the suite**

Run: `go vet ./... && go test ./...`
Expected: all packages `ok`, no golden diffs.

- [ ] **Step 5: Commit**

```bash
git add internal/views/keys.go internal/views/picker.go internal/views/search.go \
        internal/views/backlinks.go internal/views/todos.go internal/views/help.go \
        internal/views/app.go
git commit -m "refactor(views): centralize repeated key strings, fix stale resize comment"
```

---

## Task 2: Shared theme (styles + palette)

Consolidate the duplicated lipgloss styles into one `theme.go`. Visually
identical — goldens must not change.

**Files:**
- Create: `internal/views/theme.go`
- Modify: `internal/views/app.go`, `internal/views/picker.go`, `internal/views/search.go`, `internal/views/backlinks.go`, `internal/views/todos.go`, `internal/views/help.go`, `internal/views/page.go`

- [ ] **Step 1: Create the theme file**

Create `internal/views/theme.go`:

```go
package views

import "github.com/charmbracelet/lipgloss"

// Palette. Numeric ANSI colors so the TUI honors the user's terminal theme.
//
// NOTE: internal/render/page.go and the todos marker map (todos.go) duplicate
// the task-marker colors 9/11/12/8. They are intentionally NOT unified across
// the package boundary — see docs/superpowers/specs/2026-05-29-structural-cleanup-design.md
// (a shared theme package for ~4 constants would be premature).
var (
	colorHighlight = lipgloss.Color("12") // blue: links, positions, group headers, selection bg
	colorCursor    = lipgloss.Color("11") // bright yellow: link cursor, search-match emphasis
	colorSelFg     = lipgloss.Color("0")  // black: selected-row foreground
)

// Styles shared verbatim by multiple views.
var (
	styleTitle  = lipgloss.NewStyle().Bold(true)
	styleFaint  = lipgloss.NewStyle().Faint(true)
	styleSel    = lipgloss.NewStyle().Foreground(colorSelFg).Background(colorHighlight).Bold(true)
	styleBorder = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(1, 2)
)
```

- [ ] **Step 2: Repoint app.go**

In `internal/views/app.go`, delete the `var ( statusFaint … splashFaint )`
block (lines ~15-20) and apply `replace_all`:
- `statusFaint` → `styleFaint`
- `statusRule` → `styleFaint`
- `splashBold` → `styleTitle`
- `splashFaint` → `styleFaint`

- [ ] **Step 3: Repoint picker.go**

In `internal/views/picker.go`, delete the `var ( pickerBorder … pickerFaint )`
block and apply `replace_all`:
- `pickerBorder` → `styleBorder`
- `pickerTitle` → `styleTitle`
- `pickerPrompt` → `styleFaint`
- `pickerSel` → `styleSel`
- `pickerFaint` → `styleFaint`

- [ ] **Step 4: Repoint search.go**

In `internal/views/search.go`, replace the `var ( searchBorder … searchMatch )`
block with only the two distinct styles, referencing the palette:

```go
var (
	searchHitPos = lipgloss.NewStyle().Foreground(colorHighlight)
	searchMatch  = lipgloss.NewStyle().Bold(true).Foreground(colorCursor)
)
```

Then `replace_all`:
- `searchBorder` → `styleBorder`
- `searchTitle` → `styleTitle`
- `searchPrompt` → `styleFaint`
- `searchSel` → `styleSel`
- `searchFaint` → `styleFaint`

- [ ] **Step 5: Repoint backlinks.go**

In `internal/views/backlinks.go`, replace the `var ( blBorder … blSel )` block
with only the distinct position style:

```go
var blPos = lipgloss.NewStyle().Foreground(colorHighlight)
```

Then `replace_all`:
- `blBorder` → `styleBorder`
- `blTitle` → `styleTitle`
- `blFaint` → `styleFaint`
- `blSel` → `styleSel`

- [ ] **Step 6: Repoint todos.go**

In `internal/views/todos.go`, replace the `var ( todosBorder … )` block with
the distinct group style + the marker map (keep the marker map local, with its
duplication note):

```go
var todosGroup = lipgloss.NewStyle().Bold(true).Foreground(colorHighlight)

// todosMark colors each workflow marker. Mirrors the task-marker palette in
// internal/render/page.go; intentionally not shared across the package
// boundary (see theme.go).
var todosMark = map[string]lipgloss.Style{
	"TODO":    lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Bold(true),  // red
	"DOING":   lipgloss.NewStyle().Foreground(colorCursor).Bold(true),          // yellow
	"LATER":   lipgloss.NewStyle().Foreground(colorHighlight).Bold(true),       // blue
	"WAITING": lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Bold(true),  // dim
}
```

Then `replace_all`:
- `todosBorder` → `styleBorder`
- `todosTitle` → `styleTitle`
- `todosFaint` → `styleFaint`
- `todosSel` → `styleSel`

- [ ] **Step 7: Repoint help.go**

In `internal/views/help.go`, replace the `var ( helpBorder … helpSection )`
block with the help-specific styles (its border padding is `0,2`, distinct from
the shared `1,2`):

```go
var (
	helpBorder  = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 2)
	helpKey     = lipgloss.NewStyle().Bold(true).Foreground(colorHighlight)
	helpSection = lipgloss.NewStyle().Faint(true).Italic(true)
)
```

Then `replace_all`:
- `helpTitle` → `styleTitle`
- `helpFaint` → `styleFaint`

- [ ] **Step 8: Repoint page.go**

In `internal/views/page.go`, replace the `cursorStyle` definition to reference
the palette and drop `titleStyle`/`metaStyle`:

```go
var cursorStyle = lipgloss.NewStyle().
	Background(colorCursor). // bright yellow
	Foreground(colorSelFg).  // black
	Bold(true).
	Underline(true)
```

Then `replace_all`:
- `titleStyle` → `styleTitle`
- `metaStyle` → `styleFaint`

- [ ] **Step 9: Run the suite**

Run: `go vet ./... && go test ./...`
Expected: all packages `ok`, **no** golden diffs (styles are byte-identical).

- [ ] **Step 10: Commit**

```bash
git add internal/views/theme.go internal/views/app.go internal/views/picker.go \
        internal/views/search.go internal/views/backlinks.go internal/views/todos.go \
        internal/views/help.go internal/views/page.go
git commit -m "refactor(views): centralize shared styles and palette in theme.go"
```

---

## Task 3: Pure scroll/clamp helpers (test-first)

Extract the byte-identical scroll-window math and the repeated min/max clamp
into pure functions. These are genuinely new functions, so write the test
first.

**Files:**
- Create: `internal/views/scroll.go`
- Test: `internal/views/scroll_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/views/scroll_test.go`:

```go
package views

import "testing"

func TestClampInt(t *testing.T) {
	cases := []struct {
		v, lo, hi, want int
	}{
		{5, 0, 10, 5},
		{-1, 0, 10, 0},
		{11, 0, 10, 10},
		{0, 0, 10, 0},
		{10, 0, 10, 10},
	}
	for _, c := range cases {
		if got := clampInt(c.v, c.lo, c.hi); got != c.want {
			t.Errorf("clampInt(%d,%d,%d): want %d, got %d", c.v, c.lo, c.hi, c.want, got)
		}
	}
}

func TestScrollWindowAllFit(t *testing.T) {
	start, end := scrollWindow(2, 3, 10)
	if start != 0 || end != 3 {
		t.Errorf("count <= rows: want [0,3), got [%d,%d)", start, end)
	}
}

func TestScrollWindowCenters(t *testing.T) {
	const count, rows = 20, 6
	cases := []struct {
		name      string
		sel       int
		wantStart int
	}{
		{"top", 0, 0},
		{"middle", 10, 10 - rows/2},
		{"bottom", 19, count - rows},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			start, end := scrollWindow(c.sel, count, rows)
			if end-start != rows {
				t.Errorf("window size: want %d, got %d ([%d,%d))", rows, end-start, start, end)
			}
			if start != c.wantStart {
				t.Errorf("start: want %d, got %d", c.wantStart, start)
			}
			if c.sel < start || c.sel >= end {
				t.Errorf("sel %d not in [%d,%d)", c.sel, start, end)
			}
		})
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/views/ -run 'TestClampInt|TestScrollWindow' -v`
Expected: FAIL — `undefined: clampInt` / `undefined: scrollWindow`.

- [ ] **Step 3: Write the implementation**

Create `internal/views/scroll.go`:

```go
package views

// clampInt returns v constrained to the inclusive range [lo, hi].
func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// scrollWindow returns the [start, end) slice indices of a list of `count`
// items to render in a window of `rows` rows, keeping `sel` visible and
// centering it when the list overflows the window.
func scrollWindow(sel, count, rows int) (start, end int) {
	if rows >= count {
		return 0, count
	}
	half := rows / 2
	start = sel - half
	if start < 0 {
		start = 0
	}
	end = start + rows
	if end > count {
		end = count
		start = end - rows
	}
	return start, end
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/views/ -run 'TestClampInt|TestScrollWindow' -v`
Expected: PASS.

- [ ] **Step 5: Run the full suite**

Run: `go vet ./... && go test ./...`
Expected: all packages `ok`.

- [ ] **Step 6: Commit**

```bash
git add internal/views/scroll.go internal/views/scroll_test.go
git commit -m "refactor(views): extract pure scrollWindow and clampInt helpers"
```

---

## Task 4: Embed a shared `listBox` base in the four overlays

Replace each overlay's copy of `width/height/sel`, `SetSize`, `innerWidth`,
selection movement, and `scrollWindow` with an embedded `listBox` + the Task 3
helpers. `visibleRows` stays per-overlay (its `chrome`/max differ).

**Files:**
- Create: `internal/views/listbox.go`
- Modify: `internal/views/picker.go`, `internal/views/search.go`, `internal/views/backlinks.go`, `internal/views/todos.go`, `internal/views/search_test.go`

- [ ] **Step 1: Create the base**

Create `internal/views/listbox.go`:

```go
package views

const (
	listInnerWidthMin  = 30
	listInnerWidthMax  = 80
	listVisibleRowsMin = 6
)

// listBox holds the geometry and selection state shared by the scrollable list
// overlays (picker, search, backlinks, todos). Concrete overlays embed it.
type listBox struct {
	width, height int
	sel           int
}

// SetSize updates the cached terminal dimensions.
func (b *listBox) SetSize(w, h int) { b.width, b.height = w, h }

// innerWidth is the content-column budget: terminal width minus border (2),
// padding (4), and a 4-cell safety margin, clamped to [30, 80].
func (b *listBox) innerWidth() int {
	return clampInt(b.width-2-4-4, listInnerWidthMin, listInnerWidthMax)
}

// moveUp/moveDown move the selection one row, clamped to [0, count).
func (b *listBox) moveUp() {
	if b.sel > 0 {
		b.sel--
	}
}

func (b *listBox) moveDown(count int) {
	if b.sel < count-1 {
		b.sel++
	}
}
```

- [ ] **Step 2: Migrate picker.go**

In `internal/views/picker.go`:

Replace the `Picker` struct's geometry/selection fields with the embed. Change:

```go
type Picker struct {
	idx     *graph.Index
	input   textinput.Model
	choices []pickerChoice
	names   []string // derived from choices, kept in lockstep for fuzzy.Find
	matches []fuzzy.Match
	sel     int
	width   int       // terminal width snapshot, for layout
	height  int       // terminal height snapshot, for scroll window sizing
	now     time.Time // captured at construction for stable relative-time hints
}
```

to:

```go
type Picker struct {
	listBox
	idx     *graph.Index
	input   textinput.Model
	choices []pickerChoice
	names   []string // derived from choices, kept in lockstep for fuzzy.Find
	matches []fuzzy.Match
	now     time.Time // captured at construction for stable relative-time hints
}
```

In `NewPicker`, change the literal `p := &Picker{idx: idx, input: ti, width: width, height: height, now: time.Now()}` to:

```go
	p := &Picker{listBox: listBox{width: width, height: height}, idx: idx, input: ti, now: time.Now()}
```

Delete the `SetSize` method, the `innerWidth` method, and the `scrollWindow`
method (now provided by `listBox` / `scroll.go`).

Delete the `pickerInnerWidthMax`/`pickerInnerWidthMin`/`pickerVisibleRowsMin`
constants, keeping only:

```go
const pickerVisibleRowsMax = 14 // ceiling on simultaneously-shown matches
```

Rewrite `visibleRows`:

```go
func (p *Picker) visibleRows() int {
	// Picker chrome: 2 (border) + 2 (padding) + 1 (title) + 1 (blank) +
	// 1 (prompt) + 1 (divider) + 1 (blank) + 1 (hint) ≈ 10 lines.
	const chrome = 10
	return clampInt(p.height-chrome, listVisibleRowsMin, pickerVisibleRowsMax)
}
```

In `Update`, replace the selection arms:
- `if p.sel > 0 { p.sel-- }` → `p.moveUp()`
- `if p.sel < len(p.matches)-1 { p.sel++ }` → `p.moveDown(len(p.matches))`

In `View`, replace `start, end := p.scrollWindow()` with:

```go
	start, end := scrollWindow(p.sel, len(p.matches), p.visibleRows())
```

- [ ] **Step 3: Migrate backlinks.go**

In `internal/views/backlinks.go`:

Change the struct to embed `listBox` and drop `sel/width/height`:

```go
type Backlinks struct {
	listBox
	idx    *graph.Index
	target string // page being viewed
	refs   []graph.Ref
}
```

In `NewBacklinks`, change the returned literal to:

```go
	return &Backlinks{listBox: listBox{width: width, height: height}, idx: idx, target: target, refs: refs}
```

Delete the `SetSize`, `innerWidth`, and `scrollWindow` methods. Delete
`blInnerWidthMax`/`blInnerWidthMin`/`blVisibleRowsMin`, keeping:

```go
const blVisibleRowsMax = 14
```

Rewrite `visibleRows`:

```go
func (b *Backlinks) visibleRows() int {
	// Chrome: 2 (border) + 2 (padding) + 1 (title) + 1 (blank) +
	// 1 (divider) + 1 (blank) + 1 (hint) ≈ 9 lines.
	const chrome = 9
	return clampInt(b.height-chrome, listVisibleRowsMin, blVisibleRowsMax)
}
```

In `Update`:
- `if b.sel > 0 { b.sel-- }` → `b.moveUp()`
- `if b.sel < len(b.refs)-1 { b.sel++ }` → `b.moveDown(len(b.refs))`

In `View`, replace `start, end := b.scrollWindow()` with:

```go
	start, end := scrollWindow(b.sel, len(b.refs), b.visibleRows())
```

- [ ] **Step 4: Migrate todos.go**

In `internal/views/todos.go`:

Change the struct:

```go
type Todos struct {
	listBox
	idx     *graph.Index
	filter  string // "", "TODO", "LATER", "DOING", "WAITING"
	visible []graph.TodoBullet
}
```

In `NewTodos`, change `t := &Todos{idx: idx, width: width, height: height}` to:

```go
	t := &Todos{listBox: listBox{width: width, height: height}, idx: idx}
```

Delete the `SetSize` and `innerWidth` methods. Delete `todosInnerWidthMax` /
`todosInnerWidthMin` / `todosVisibleRowsMin`, keeping:

```go
const todosVisibleRowsMax = 16
```

Rewrite `visibleRows`:

```go
func (t *Todos) visibleRows() int {
	// Chrome: 2 (border) + 2 (padding) + 1 (title) + 1 (blank) + 1 (divider)
	// + 1 (blank) + 1 (hint) ≈ 9 lines. Add 2 more to leave headroom for
	// scroll-position hints when they're shown.
	const chrome = 11
	return clampInt(t.height-chrome, listVisibleRowsMin, todosVisibleRowsMax)
}
```

In `Update`:
- `if t.sel > 0 { t.sel-- }` → `t.moveUp()`
- `if t.sel < len(t.visible)-1 { t.sel++ }` → `t.moveDown(len(t.visible))`

`computeWindow` stays as-is — it already calls `t.visibleRows()` and uses
`t.sel`, both still valid.

- [ ] **Step 5: Migrate search.go (geometry only)**

In `internal/views/search.go`:

Change the struct to embed `listBox` and drop `sel/width/height` (keep the
other fields):

```go
type SearchView struct {
	listBox
	idx        *graph.Index
	query      string
	hits       []SearchHit
	running    bool
	err        error
	pathToName map[string]string // file path → logical page name, for tidy row prefixes
}
```

In `NewSearchView`, change `s := &SearchView{idx: idx, width: width, height: height}` to:

```go
	s := &SearchView{listBox: listBox{width: width, height: height}, idx: idx}
```

Delete the `SetSize`, `innerWidth`, and `scrollWindow` methods. Delete
`searchInnerWidthMax`/`searchInnerWidthMin`/`searchVisibleRowsMin`, keeping:

```go
const searchVisibleRowsMax = 14
```

Rewrite `visibleRows`:

```go
func (s *SearchView) visibleRows() int {
	// Chrome: 2 (border) + 2 (padding) + 1 (title) + 1 (blank) + 1 (prompt)
	// + 1 (divider) + 1 (blank) + 1 (hint) ≈ 10 lines.
	const chrome = 10
	return clampInt(s.height-chrome, listVisibleRowsMin, searchVisibleRowsMax)
}
```

In `Update`, replace the selection arms:
- `if s.sel > 0 { s.sel-- }` → `s.moveUp()`
- `if s.sel < len(s.hits)-1 { s.sel++ }` → `s.moveDown(len(s.hits))`

In `View`, replace `start, end := s.scrollWindow()` with:

```go
	start, end := scrollWindow(s.sel, len(s.hits), s.visibleRows())
```

- [ ] **Step 6: Update the affected search tests**

In `internal/views/search_test.go`, three tests reference removed
methods/consts/struct-literal fields. Edit:

`TestSearchScrollWindow` — replace `start, end := s.scrollWindow()` with:

```go
			start, end := scrollWindow(s.sel, len(s.hits), s.visibleRows())
```

`TestSearchScrollWindowAllFit` — replace `start, end := s.scrollWindow()` with:

```go
	start, end := scrollWindow(s.sel, len(s.hits), s.visibleRows())
```

`TestSearchInnerWidthClamps` — rewrite the whole function (embedded-field
literal + renamed consts):

```go
func TestSearchInnerWidthClamps(t *testing.T) {
	s := &SearchView{listBox: listBox{width: 10}}
	if got := s.innerWidth(); got != listInnerWidthMin {
		t.Errorf("narrow term: want min %d, got %d", listInnerWidthMin, got)
	}
	s.width = 200
	if got := s.innerWidth(); got != listInnerWidthMax {
		t.Errorf("wide term: want max %d, got %d", listInnerWidthMax, got)
	}
}
```

`TestSearchVisibleRowsClamps` — rewrite (embedded-field literal + renamed min):

```go
func TestSearchVisibleRowsClamps(t *testing.T) {
	s := &SearchView{listBox: listBox{height: 5}}
	if got := s.visibleRows(); got != listVisibleRowsMin {
		t.Errorf("tiny term: want min %d, got %d", listVisibleRowsMin, got)
	}
	s.height = 100
	if got := s.visibleRows(); got != searchVisibleRowsMax {
		t.Errorf("huge term: want max %d, got %d", searchVisibleRowsMax, got)
	}
}
```

`TestMatchesWithin` and `TestMatchesWithinEmpty` construct `&SearchView{}` with
no fields — those stay valid (empty embed). No change needed there.

- [ ] **Step 7: Run the suite**

Run: `go vet ./... && go test ./...`
Expected: all packages `ok`, no golden diffs.

- [ ] **Step 8: Commit**

```bash
git add internal/views/listbox.go internal/views/picker.go internal/views/search.go \
        internal/views/backlinks.go internal/views/todos.go internal/views/search_test.go
git commit -m "refactor(views): embed shared listBox base in the four overlays"
```

---

## Task 5: Extract ripgrep into `internal/search`

Move rg execution + JSON parsing out of the views layer. `SearchView` keeps the
presentation helpers and now consumes `search.Hit`.

**Files:**
- Create: `internal/search/search.go`, `internal/search/search_test.go`
- Modify: `internal/views/search.go`, `internal/views/search_test.go`

- [ ] **Step 1: Create the search package**

Create `internal/search/search.go` (the `runRipgrep`/`parseJSON` bodies are
moved verbatim from `views/search.go`, with types renamed `SearchHit`→`Hit`,
`SearchSpan`→`Span`, and `parseRipgrepJSON`→`parseJSON`):

```go
// Package search runs ripgrep against a Logseq graph and parses its JSON
// output into structured hits. It performs process I/O and decoding so the UI
// layer can consume plain []Hit values.
package search

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Hit is one ripgrep result row.
type Hit struct {
	FilePath string
	Line     int
	Context  string
	// Matches are byte offsets within Context that the query matched, so the
	// view can emphasise them in the rendered list.
	Matches []Span
}

// Span is a [Start, End) byte range inside Hit.Context.
type Span struct {
	Start, End int
}

// Run executes ripgrep for query against <graphPath>/pages and
// <graphPath>/journals and returns the parsed hits. It returns (nil, nil) when
// neither subdirectory exists. A ripgrep exit code of 1 (no matches) is
// treated as success.
func Run(graphPath, query string) ([]Hit, error) {
	out, err := runRipgrep(graphPath, query)
	if err != nil {
		return nil, err
	}
	return parseJSON(out), nil
}

func runRipgrep(graphPath, query string) ([]byte, error) {
	// --smart-case keeps the search case-insensitive unless the pattern
	// contains an uppercase letter, which matches what most interactive
	// users expect from a quick search.
	args := []string{"--json", "--smart-case", "--", query}
	baseArgs := len(args)
	for _, sub := range []string{"pages", "journals"} {
		p := filepath.Join(graphPath, sub)
		if _, err := os.Stat(p); err == nil {
			args = append(args, p)
		}
	}
	if len(args) == baseArgs {
		// No pages/ or journals/ dir — nothing to search.
		return nil, nil
	}
	cmd := exec.Command("rg", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
		return stdout.Bytes(), nil
	}
	if err != nil {
		return nil, fmt.Errorf("rg failed: %w (%s)", err, stderr.String())
	}
	return stdout.Bytes(), nil
}

func parseJSON(b []byte) []Hit {
	var out []Hit
	sc := bufio.NewScanner(bytes.NewReader(b))
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		var env struct {
			Type string `json:"type"`
			Data struct {
				Path       struct{ Text string } `json:"path"`
				Lines      struct{ Text string } `json:"lines"`
				LineNumber int                   `json:"line_number"`
				Submatches []struct {
					Start int `json:"start"`
					End   int `json:"end"`
				} `json:"submatches"`
			} `json:"data"`
		}
		if err := json.Unmarshal(sc.Bytes(), &env); err != nil {
			continue
		}
		if env.Type != "match" {
			continue
		}
		ctx := strings.TrimRight(env.Data.Lines.Text, "\n")
		// Strip leading whitespace from indented bullets so list rows line
		// up. Match spans are byte offsets into the *original* line, so we
		// also shift them by the number of bytes we trimmed.
		trimmed := strings.TrimLeft(ctx, " \t")
		shift := len(ctx) - len(trimmed)
		ctx = trimmed
		ctxLen := len(ctx)
		spans := make([]Span, 0, len(env.Data.Submatches))
		for _, sm := range env.Data.Submatches {
			start := sm.Start - shift
			end := sm.End - shift
			if end <= 0 || start >= ctxLen || start >= end {
				continue // span fell entirely inside the trimmed indent, or is degenerate
			}
			if start < 0 {
				start = 0
			}
			if end > ctxLen {
				end = ctxLen
			}
			spans = append(spans, Span{Start: start, End: end})
		}
		out = append(out, Hit{
			FilePath: env.Data.Path.Text,
			Line:     env.Data.LineNumber,
			Context:  ctx,
			Matches:  spans,
		})
	}
	return out
}
```

- [ ] **Step 2: Create the search package tests**

Create `internal/search/search_test.go` (moved from `views/search_test.go`:
`TestParseRipgrepJSON`, `TestRunRipgrepFindsHits`, `TestRunRipgrepNoMatchReturnsEmpty`,
`TestRunRipgrepMissingDirsReturnsNil`, plus the `skipIfNoRipgrep` helper —
renamed to the package-local API). The fixture path gains one `..` segment
because tests now run from `internal/search`:

```go
package search

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func skipIfNoRipgrep(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("rg"); err != nil {
		t.Skip("rg not on PATH; install ripgrep to run search tests")
	}
}

func TestParseJSON(t *testing.T) {
	in := `{"type":"begin","data":{"path":{"text":"/g/pages/Alpha.md"}}}
{"type":"match","data":{"path":{"text":"/g/pages/Alpha.md"},"lines":{"text":"links to Beta\n"},"line_number":3,"absolute_offset":42,"submatches":[{"match":{"text":"Beta"},"start":9,"end":13}]}}
{"type":"match","data":{"path":{"text":"/g/journals/2026_05_24.md"},"lines":{"text":"references Alpha\n"},"line_number":1,"absolute_offset":0,"submatches":[]}}
{"type":"end","data":{"path":{"text":"/g/pages/Alpha.md"}}}
`
	hits := parseJSON([]byte(in))
	if len(hits) != 2 {
		t.Fatalf("want 2 hits, got %d: %+v", len(hits), hits)
	}
	if hits[0].Line != 3 || hits[0].Context != "links to Beta" {
		t.Errorf("hit[0]: %+v", hits[0])
	}
	if len(hits[0].Matches) != 1 || hits[0].Matches[0].Start != 9 || hits[0].Matches[0].End != 13 {
		t.Errorf("hit[0].Matches: want [{9 13}], got %+v", hits[0].Matches)
	}
	if hits[1].FilePath != "/g/journals/2026_05_24.md" {
		t.Errorf("hit[1]: %+v", hits[1])
	}
	if len(hits[1].Matches) != 0 {
		t.Errorf("hit[1].Matches: want 0, got %d", len(hits[1].Matches))
	}
}

func TestRunFindsHits(t *testing.T) {
	skipIfNoRipgrep(t)
	abs, err := filepath.Abs("../../testdata/fixture-graph")
	if err != nil {
		t.Fatal(err)
	}
	hits, err := Run(abs, "Beta")
	if err != nil {
		t.Fatalf("Run err: %v", err)
	}
	if len(hits) == 0 {
		t.Fatalf("expected at least one hit for \"Beta\"")
	}
	for _, h := range hits {
		if !strings.HasPrefix(h.FilePath, abs) {
			t.Errorf("hit path outside fixture: %s", h.FilePath)
		}
	}
}

func TestRunNoMatchReturnsEmpty(t *testing.T) {
	skipIfNoRipgrep(t)
	abs, err := filepath.Abs("../../testdata/fixture-graph")
	if err != nil {
		t.Fatal(err)
	}
	hits, err := Run(abs, "thisstringshouldnotexistanywhere_xyzzy_1234")
	if err != nil {
		t.Fatalf("no-match should not error, got: %v", err)
	}
	if len(hits) != 0 {
		t.Errorf("no-match: want 0 hits, got %d", len(hits))
	}
}

func TestRunMissingDirsReturnsNil(t *testing.T) {
	tmp := t.TempDir()
	hits, err := Run(tmp, "anything")
	if err != nil {
		t.Errorf("missing dirs: want nil err, got %v", err)
	}
	if hits != nil {
		t.Errorf("missing dirs: want nil hits, got %+v", hits)
	}
}
```

- [ ] **Step 3: Repoint views/search.go to the new package**

In `internal/views/search.go`:

Add the import (in the existing import block):

```go
	"git.fiatcode.dev/fiatcode/peekseq/internal/search"
```

Delete the `SearchHit` and `SearchSpan` type declarations, and delete the
`runRipgrep` and `parseRipgrepJSON` functions (now in `internal/search`).

`replace_all` within the file:
- `[]SearchHit` → `[]search.Hit`
- `SearchHit{` → `search.Hit{`
- `SearchHit)` → `search.Hit)` (the `*SearchHit` return type and `matchesWithin(h SearchHit` parameter)
- `*SearchHit` → `*search.Hit`
- `[]SearchSpan` → `[]search.Span`
- `SearchSpan{` → `search.Span{`

Rewrite `SearchCmd` to call `search.Run` (signature unchanged — still takes
graphPath; Task 6 makes it internal):

```go
// SearchCmd returns a tea.Cmd that runs rg and returns a searchDoneMsg.
func (s *SearchView) SearchCmd(graphPath string) tea.Cmd {
	q := s.query
	return func() tea.Msg {
		hits, err := search.Run(graphPath, q)
		if err != nil {
			return searchDoneMsg{err: err}
		}
		return searchDoneMsg{hits: hits}
	}
}
```

Confirm the leftover imports are still all used (`bufio`, `bytes`,
`encoding/json`, `os`, `os/exec`, `path/filepath` were only used by the moved
functions — **remove** them from `views/search.go`'s import block; keep `fmt`,
`strings`, `tea`, `lipgloss`, `graph`, and the new `search`).

- [ ] **Step 4: Update views/search_test.go for the type move**

In `internal/views/search_test.go`:

Add the import:

```go
	"git.fiatcode.dev/fiatcode/peekseq/internal/search"
```

Delete these four tests (now living in `internal/search`): `TestParseRipgrepJSON`,
`TestRunRipgrepFindsHits`, `TestRunRipgrepNoMatchReturnsEmpty`,
`TestRunRipgrepMissingDirsReturnsNil`. Delete the now-unused `skipIfNoRipgrep`
helper **only if** no remaining test in the file references it —
`TestSearchCmdRoundtrip` still calls it, so **keep** `skipIfNoRipgrep`.

`replace_all` in the remaining tests:
- `[]SearchHit` → `[]search.Hit`
- `SearchHit{` → `search.Hit{`
- `[]SearchSpan` → `[]search.Span`
- `SearchSpan{` → `search.Span{`

After these edits the surviving imports must include `errors`, `path/filepath`,
`strings`, `testing`, `teatest`, and `search`. Remove `os/exec` if no longer
referenced (it was only used by the moved `skipIfNoRipgrep`… which we kept, so
**keep** `os/exec`).

- [ ] **Step 5: Run the suite**

Run: `go vet ./... && go test ./...`
Expected: five packages now (`cmd/peekseq`, `internal/graph`, `internal/render`,
`internal/search`, `internal/views`) all `ok`, no golden diffs.

- [ ] **Step 6: Commit**

```bash
git add internal/search/ internal/views/search.go internal/views/search_test.go
git commit -m "refactor: extract ripgrep execution into internal/search package"
```

---

## Task 6: `Overlay` interface + generic App dispatch

The atomic step: define `Overlay`/`OverlayResult`, make all five overlays
implement it, move hit→page resolution into `SearchView`, and replace App's
`modeT` enum + five pointers with one `active Overlay`. Intermediate states
won't compile, so this is one commit; run the suite only at the end.

**Files:**
- Create: `internal/views/overlay.go`
- Modify: `internal/views/app.go`, `internal/views/picker.go`, `internal/views/search.go`, `internal/views/backlinks.go`, `internal/views/todos.go`, `internal/views/help.go`, `internal/views/app_overlay_test.go`, `internal/views/app_dispatch_test.go`, `internal/views/search_test.go`

- [ ] **Step 1: Define the interface**

Create `internal/views/overlay.go`:

```go
package views

import tea "github.com/charmbracelet/bubbletea"

// OverlayResult is what an overlay's Update reports back to the App.
type OverlayResult struct {
	// Selected is the page to open when Accept is true. Empty means "accept
	// but navigate nowhere" (e.g. a search hit whose file isn't indexed).
	Selected string
	Accept   bool    // user chose Selected; App navigates (if non-empty) and closes
	Cancel   bool    // user dismissed; App closes the overlay
	Cmd      tea.Cmd // optional async work to run (search launches rg here)
}

// Overlay is a modal view layered over the page. App routes keys to the active
// overlay and closes it when Update reports Accept or Cancel.
type Overlay interface {
	Update(key string) OverlayResult
	View() string
	SetSize(w, h int)
}
```

- [ ] **Step 2: Convert Picker.Update**

In `internal/views/picker.go`, rewrite `Update` to return `OverlayResult`:

```go
// Update handles a key and reports the result to the App.
func (p *Picker) Update(key string) OverlayResult {
	switch key {
	case keyEsc:
		return OverlayResult{Cancel: true}
	case keyEnter:
		if p.sel >= 0 && p.sel < len(p.matches) {
			return OverlayResult{Selected: p.matches[p.sel].Str, Accept: true}
		}
		return OverlayResult{}
	case keyUp, keyCtrlK:
		p.moveUp()
		return OverlayResult{}
	case keyDown, keyCtrlJ:
		p.moveDown(len(p.matches))
		return OverlayResult{}
	}
	// Otherwise feed the key into the text input.
	p.input, _ = consumeKey(p.input, key)
	p.search(p.input.Value())
	return OverlayResult{}
}
```

- [ ] **Step 3: Convert Backlinks.Update**

In `internal/views/backlinks.go`, rewrite `Update`:

```go
func (b *Backlinks) Update(key string) OverlayResult {
	switch key {
	case keyEsc, "b":
		return OverlayResult{Cancel: true}
	case keyUp, keyK, keyCtrlK:
		b.moveUp()
	case keyDown, keyJ, keyCtrlJ:
		b.moveDown(len(b.refs))
	case keyEnter:
		if b.sel >= 0 && b.sel < len(b.refs) {
			return OverlayResult{Selected: b.refs[b.sel].FromPage, Accept: true}
		}
	}
	return OverlayResult{}
}
```

- [ ] **Step 4: Convert Todos.Update**

In `internal/views/todos.go`, rewrite `Update`:

```go
func (t *Todos) Update(key string) OverlayResult {
	switch key {
	case keyEsc, keyQ:
		return OverlayResult{Cancel: true}
	case "t":
		t.cycleFilter()
	case keyUp, keyK:
		t.moveUp()
	case keyDown, keyJ:
		t.moveDown(len(t.visible))
	case keyEnter:
		if t.sel >= 0 && t.sel < len(t.visible) {
			return OverlayResult{Selected: t.visible[t.sel].Page, Accept: true}
		}
	}
	return OverlayResult{}
}
```

- [ ] **Step 5: Convert Help.Update**

In `internal/views/help.go`, rewrite `Update` to satisfy the interface:

```go
// Update reports whether the overlay should close. There's no state to mutate.
func (h *Help) Update(key string) OverlayResult {
	switch key {
	case keyEsc, "?", keyQ:
		return OverlayResult{Cancel: true}
	}
	return OverlayResult{}
}
```

- [ ] **Step 6: Convert SearchView.Update and absorb hit resolution**

In `internal/views/search.go`:

Add a resolution helper that turns a hit's file path into a logical page name
(moved from App's `pageNameFromHitPath`, reusing the existing `pathToName` map):

```go
// pageName returns the logical page name for a hit's file path, or "" when the
// file isn't in the index (e.g. a result outside pages/ and journals/).
func (s *SearchView) pageName(filePath string) string {
	return s.pathToName[filePath]
}
```

Rewrite `Update` to the interface signature, reading `graphPath` from the index
and returning a resolved page name:

```go
func (s *SearchView) Update(key string) OverlayResult {
	switch key {
	case keyEsc:
		return OverlayResult{Cancel: true}
	case keyEnter:
		if s.running || s.query == "" {
			return OverlayResult{}
		}
		if len(s.hits) == 0 {
			s.running = true
			return OverlayResult{Cmd: s.SearchCmd(s.idx.GraphPath)}
		}
		if s.sel >= 0 && s.sel < len(s.hits) {
			return OverlayResult{Selected: s.pageName(s.hits[s.sel].FilePath), Accept: true}
		}
		return OverlayResult{}
	case keyUp, keyCtrlK:
		s.moveUp()
	case keyDown, keyCtrlJ:
		s.moveDown(len(s.hits))
	case keyBackspace:
		if len(s.query) > 0 {
			s.query = s.query[:len(s.query)-1]
			s.hits = nil
		}
	case " ", keySpace:
		s.query += " "
		s.hits = nil
	default:
		if len(key) == 1 {
			s.query += key
			s.hits = nil
		}
	}
	return OverlayResult{}
}
```

(`SearchCmd` keeps its `graphPath` parameter; it's now only called from inside
`Update`.)

- [ ] **Step 7: Rewrite App to use a single active Overlay**

In `internal/views/app.go`:

Delete the `modeT` type and its `const ( modePage … modeHelp )` block.

In the `App` struct, delete the five typed pointers and the `mode modeT` field,
replacing them with one field:

```go
	// active is the overlay layered over the page, or nil when the page has
	// focus. Set when an open-overlay key is pressed; cleared on Accept/Cancel.
	active Overlay
```

(Keep all other fields: `graphPath`, `idx`, `loadErr`, `page`, `width`,
`height`, `nowFunc`, `hint`, `hintGen`, `hist`, `histIdx`, `version`.)

In `New`, delete the `mode: modePage,` initializer line.

In the `indexLoadedMsg` case: unchanged.

In the `searchDoneMsg` case, route via type assertion:

```go
	case searchDoneMsg:
		if s, ok := a.active.(*SearchView); ok {
			s.Apply(m)
		}
		return a, nil
```

In the `tea.WindowSizeMsg` case, replace the five overlay `SetSize` nil-checks
with one:

```go
		if a.active != nil {
			a.active.SetSize(m.Width, m.Height)
		}
```

(Keep the `a.page.SetSize` / `a.tryInitPage` logic above it unchanged.)

In the `tea.KeyMsg` case, after the `a.page == nil` loading guard, replace the
entire `switch a.mode { … }` block with a generic dispatch followed by the
page-mode key switch:

```go
		// An open overlay swallows all keys until it accepts or cancels.
		if a.active != nil {
			res := a.active.Update(key)
			if res.Cancel {
				a.active = nil
				return a, res.Cmd
			}
			if res.Accept {
				if res.Selected != "" {
					a.navigate(res.Selected)
				}
				a.active = nil
			}
			return a, res.Cmd
		}
		switch key {
		case keyQ, "ctrl+c":
			return a, tea.Quit
		case "ctrl+p":
			a.active = NewPicker(a.idx, a.width, a.height)
		case "/":
			a.active = NewSearchView(a.idx, a.width, a.height)
		case "b":
			a.active = NewBacklinks(a.idx, a.page.Page(), a.width, a.height)
		case "T":
			a.active = NewTodos(a.idx, a.width, a.height)
		case "?":
			a.active = NewHelp(a.version, a.width)
		case "[":
			a.historyBack()
		case "]":
			a.historyForward()
		case ".":
			today := a.todayJournalName()
			if _, ok := a.idx.ByName[today]; !ok {
				return a, a.setHint("no journal for " + today)
			} else if a.page.Page() != today {
				a.navigate(today)
			}
		case "<":
			page := a.page.Page()
			if name, ok := a.journalNeighbor(page, -1); ok {
				a.navigate(name)
			} else if graph.IsJournalPageName(page) {
				return a, a.setHint("no earlier journal")
			}
		case ">":
			page := a.page.Page()
			if name, ok := a.journalNeighbor(page, +1); ok {
				a.navigate(name)
			} else if graph.IsJournalPageName(page) {
				return a, a.setHint("no later journal")
			}
		case "g":
			a.page.GotoTop()
		case "G":
			a.page.GotoBottom()
		case "R":
			return a, a.buildIndexCmd()
		case "n":
			a.page.CycleLink(+1)
		case "N":
			a.page.CycleLink(-1)
		case keyEnter:
			if t := a.page.FollowCursor(); t != "" {
				a.navigate(t)
			}
		case keyJ, keyDown:
			a.page.LineDown()
		case keyK, keyUp:
			a.page.LineUp()
		case "ctrl+d":
			a.page.HalfPageDown()
		case "ctrl+u":
			a.page.HalfPageUp()
		}
```

In `View`, replace the `switch a.mode { … overlay = … }` block with:

```go
	if a.active != nil {
		return a.centerOverlay(a.active.View())
	}
	return a.page.View() + "\n" + a.statusBar()
```

(Delete the now-unused `overlay` variable and the trailing
`if overlay != "" { … }` block.)

Delete the `pageNameFromHitPath` function (its logic moved into
`SearchView.pageName`).

- [ ] **Step 8: Rewrite app_overlay_test.go**

`internal/views/app_overlay_test.go` asserts against `a.mode` and the deleted
pointers. Replace the whole file with assertions against `a.active`. Add a
small test helper that names the active overlay's concrete type:

```go
package views

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"git.fiatcode.dev/fiatcode/peekseq/internal/search"
)

// activeKind returns a short name for the active overlay's concrete type, or
// "page" when no overlay is open.
func activeKind(a *App) string {
	switch a.active.(type) {
	case nil:
		return "page"
	case *Picker:
		return "picker"
	case *SearchView:
		return "search"
	case *Backlinks:
		return "backlinks"
	case *Todos:
		return "todos"
	case *Help:
		return "help"
	default:
		return "unknown"
	}
}

func TestAppOpensAndCancelsPicker(t *testing.T) {
	a := bootApp(t)
	if activeKind(a) != "page" {
		t.Fatalf("setup: want page, got %s", activeKind(a))
	}
	a.Update(tea.KeyMsg{Type: tea.KeyCtrlP})
	if activeKind(a) != "picker" {
		t.Fatalf("after ctrl+p: want picker, got %s", activeKind(a))
	}
	a.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if activeKind(a) != "page" {
		t.Errorf("after esc: want page, got %s", activeKind(a))
	}
}

func TestAppOpensSearchAndEscapes(t *testing.T) {
	a := bootApp(t)
	a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	if activeKind(a) != "search" {
		t.Fatalf("after /: want search, got %s", activeKind(a))
	}
	a.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if activeKind(a) != "page" {
		t.Errorf("after esc: want page, got %s", activeKind(a))
	}
}

func TestAppOpensBacklinksAndCloses(t *testing.T) {
	a := bootApp(t)
	a.navigate("Hub")
	a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("b")})
	if activeKind(a) != "backlinks" {
		t.Fatalf("after b: want backlinks, got %s", activeKind(a))
	}
	a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("b")})
	if activeKind(a) != "page" {
		t.Errorf("after second b: want page, got %s", activeKind(a))
	}
}

func TestAppOpensTodosAndCloses(t *testing.T) {
	a := bootApp(t)
	a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("T")})
	if activeKind(a) != "todos" {
		t.Fatalf("after T: want todos, got %s", activeKind(a))
	}
	a.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if activeKind(a) != "page" {
		t.Errorf("after esc: want page, got %s", activeKind(a))
	}
}

func TestAppOpensHelpAndCloses(t *testing.T) {
	a := bootApp(t)
	a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("?")})
	if activeKind(a) != "help" {
		t.Fatalf("after ?: want help, got %s", activeKind(a))
	}
	a.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if activeKind(a) != "page" {
		t.Errorf("after esc: want page, got %s", activeKind(a))
	}
}

func TestAppPickerAcceptNavigates(t *testing.T) {
	a := bootApp(t)
	a.Update(tea.KeyMsg{Type: tea.KeyCtrlP})
	if activeKind(a) != "picker" {
		t.Fatalf("setup: picker not open")
	}
	for _, r := range "Alp" {
		a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	a.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if activeKind(a) != "page" {
		t.Errorf("after enter: want page, got %s", activeKind(a))
	}
	if a.page.Page() != "Alpha" {
		t.Errorf("after picker accept: want page Alpha, got %q", a.page.Page())
	}
}

func TestAppTodosAcceptNavigates(t *testing.T) {
	a := bootApp(t)
	a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("T")})
	if activeKind(a) != "todos" {
		t.Fatalf("setup: todos not open")
	}
	a.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if activeKind(a) != "page" {
		t.Errorf("after enter: want page, got %s", activeKind(a))
	}
}

func TestAppSearchDoneMsgRouting(t *testing.T) {
	a := bootApp(t)
	a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	s, ok := a.active.(*SearchView)
	if !ok {
		t.Fatalf("setup: search not open, got %s", activeKind(a))
	}
	a.Update(searchDoneMsg{hits: []search.Hit{
		{FilePath: "/x", Line: 1, Context: "hello"},
	}})
	if len(s.hits) != 1 {
		t.Errorf("searchDoneMsg should populate hits, got %d", len(s.hits))
	}
}

func TestAppViewWithOverlay(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	a := bootApp(t)
	a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("T")})
	v := a.View()
	if strings.Contains(v, "? help") {
		t.Errorf("overlay view should not render the page status bar; got:\n%s", v)
	}
	if !strings.Contains(v, "Open todos") {
		t.Errorf("overlay view should show todos title; got:\n%s", v)
	}
}

func TestAppCenterOverlayFallback(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	a := New("/no/such/path", "test")
	if got := a.centerOverlay("hello"); got != "hello" {
		t.Errorf("zero-size fallback: want raw content, got %q", got)
	}
}

func TestAppCenterOverlayPlacesContent(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	a := bootApp(t)
	out := a.centerOverlay("X")
	if !strings.Contains(out, "X") {
		t.Errorf("centered output must contain content; got:\n%s", out)
	}
	if len(out) <= len("X") {
		t.Errorf("centered output should pad with whitespace; got len %d", len(out))
	}
}

// TestAppResizePropagatesToOverlays asserts that a WindowSizeMsg arriving while
// an overlay is open updates the overlay's cached size.
func TestAppResizePropagatesToOverlays(t *testing.T) {
	a := bootApp(t)

	openers := []struct {
		name      string
		key       tea.KeyMsg
		setupPage string
		read      func() (int, int)
	}{
		{
			name: "picker",
			key:  tea.KeyMsg{Type: tea.KeyCtrlP},
			read: func() (int, int) { return a.active.(*Picker).width, a.active.(*Picker).height },
		},
		{
			name: "search",
			key:  tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")},
			read: func() (int, int) { return a.active.(*SearchView).width, a.active.(*SearchView).height },
		},
		{
			name:      "backlinks",
			key:       tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("b")},
			setupPage: "Hub",
			read:      func() (int, int) { return a.active.(*Backlinks).width, a.active.(*Backlinks).height },
		},
		{
			name: "todos",
			key:  tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("T")},
			read: func() (int, int) { return a.active.(*Todos).width, a.active.(*Todos).height },
		},
	}

	for _, o := range openers {
		t.Run(o.name, func(t *testing.T) {
			a.active = nil // reset to page mode each iteration
			if o.setupPage != "" {
				a.navigate(o.setupPage)
			}
			a.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
			a.Update(o.key)

			a.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
			w, h := o.read()
			if w != 120 || h != 40 {
				t.Errorf("%s: want 120x40 after resize, got %dx%d", o.name, w, h)
			}

			a.Update(tea.KeyMsg{Type: tea.KeyEsc})
		})
	}
}
```

- [ ] **Step 9: Move the resolution test into search_test.go**

In `internal/views/app_dispatch_test.go`, delete the entire
`TestPageNameFromHitPath` function (lines 123-136 — it ends just before
`TestAppPageKeyDispatch`). Its `idx`/`sample`/`name` locals are confined to that
function, so no unused-variable cleanup is needed. The new
`TestSearchEnterResolvesHitToPageName` (below) replaces its coverage. Keep the
rest of `app_dispatch_test.go` intact.

In `internal/views/search_test.go`, replace `TestSearchEnterWithHitsOpensSelection`
(which asserted on a returned `*SearchHit`) with one that exercises the new
in-overlay resolution against a real fixture path, plus a miss case. First, add
a helper to grab any indexed page + its path:

```go
func TestSearchEnterResolvesHitToPageName(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	idx := loadFixture(t)
	s := NewSearchView(idx, 80, 24)
	s.SetQuery("Beta")

	// A hit whose path IS in the index resolves to that page's name.
	want := idx.Pages[0]
	s.hits = []search.Hit{{FilePath: want.Path, Line: 1, Context: "x"}}
	s.sel = 0
	res := s.Update("enter")
	if !res.Accept || res.Cancel {
		t.Errorf("enter with indexed hit: want Accept, got %+v", res)
	}
	if res.Selected != want.Name {
		t.Errorf("resolved page: want %q, got %q", want.Name, res.Selected)
	}

	// A hit whose path is NOT in the index accepts but selects nothing.
	s.hits = []search.Hit{{FilePath: "/totally/unknown/file.md", Line: 1, Context: "x"}}
	s.sel = 0
	res = s.Update("enter")
	if !res.Accept {
		t.Errorf("enter with unknown hit: want Accept, got %+v", res)
	}
	if res.Selected != "" {
		t.Errorf("unknown hit should resolve to empty page, got %q", res.Selected)
	}
}
```

Then update the remaining search Update tests to the single-argument signature
and `OverlayResult` return. Apply these whole-function rewrites:

`TestSearchUpdateTypesIntoQuery`:

```go
func TestSearchUpdateTypesIntoQuery(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	s := NewSearchView(loadFixture(t), 80, 24)
	for _, k := range []string{"a", "l", "p"} {
		if res := s.Update(k); res != (OverlayResult{}) {
			t.Errorf("typing %q: want zero result, got %+v", k, res)
		}
	}
	if s.query != "alp" {
		t.Errorf("query: want \"alp\", got %q", s.query)
	}
}
```

`TestSearchUpdateBackspace`:

```go
func TestSearchUpdateBackspace(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	s := NewSearchView(loadFixture(t), 80, 24)
	s.SetQuery("abc")
	s.hits = []search.Hit{{FilePath: "x", Line: 1, Context: "y"}}

	s.Update("backspace")
	if s.query != "ab" {
		t.Errorf("after first backspace: want \"ab\", got %q", s.query)
	}
	if s.hits != nil {
		t.Errorf("after backspace: hits must clear, got %+v", s.hits)
	}

	s.Update("backspace")
	s.Update("backspace")
	s.Update("backspace")
	if s.query != "" {
		t.Errorf("after draining: want \"\", got %q", s.query)
	}
}
```

`TestSearchUpdateSpaceVariants`:

```go
func TestSearchUpdateSpaceVariants(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	s := NewSearchView(loadFixture(t), 80, 24)
	s.SetQuery("foo")

	s.Update(" ")
	if s.query != "foo " {
		t.Errorf("after literal space: want \"foo \", got %q", s.query)
	}

	s.Update("space")
	if s.query != "foo  " {
		t.Errorf("after named space: want \"foo  \", got %q", s.query)
	}
}
```

`TestSearchSelectionBounds`:

```go
func TestSearchSelectionBounds(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	s := NewSearchView(loadFixture(t), 80, 24)
	s.hits = []search.Hit{
		{FilePath: "a", Line: 1, Context: "x"},
		{FilePath: "b", Line: 2, Context: "y"},
		{FilePath: "c", Line: 3, Context: "z"},
	}

	s.Update("up")
	if s.sel != 0 {
		t.Errorf("up at top: want sel 0, got %d", s.sel)
	}
	s.Update("down")
	if s.sel != 1 {
		t.Errorf("after down: want sel 1, got %d", s.sel)
	}
	s.Update("ctrl+j")
	if s.sel != 2 {
		t.Errorf("after ctrl+j: want sel 2, got %d", s.sel)
	}
	s.Update("down")
	if s.sel != 2 {
		t.Errorf("down at bottom: want sel 2, got %d", s.sel)
	}
	s.Update("ctrl+k")
	if s.sel != 1 {
		t.Errorf("after ctrl+k: want sel 1, got %d", s.sel)
	}
}
```

`TestSearchEnterEmptyQueryNoop`:

```go
func TestSearchEnterEmptyQueryNoop(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	s := NewSearchView(loadFixture(t), 80, 24)
	if res := s.Update("enter"); res != (OverlayResult{}) {
		t.Errorf("enter on empty query: want zero result, got %+v", res)
	}
}
```

`TestSearchEnterFirstTimeRunsCmd`:

```go
func TestSearchEnterFirstTimeRunsCmd(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	s := NewSearchView(loadFixture(t), 80, 24)
	s.SetQuery("Beta")

	res := s.Update("enter")
	if res.Accept || res.Cancel || res.Selected != "" {
		t.Errorf("enter to launch: want a Cmd only, got %+v", res)
	}
	if res.Cmd == nil {
		t.Errorf("enter to launch: want non-nil Cmd, got nil")
	}
	if !s.running {
		t.Errorf("enter to launch: running flag should be set")
	}
}
```

`TestSearchEnterWhileRunningIsNoop`:

```go
func TestSearchEnterWhileRunningIsNoop(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	s := NewSearchView(loadFixture(t), 80, 24)
	s.SetQuery("Beta")
	s.running = true

	if res := s.Update("enter"); res != (OverlayResult{}) {
		t.Errorf("enter while running: want zero result, got %+v", res)
	}
}
```

`TestSearchEscCancels`:

```go
func TestSearchEscCancels(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	s := NewSearchView(loadFixture(t), 80, 24)
	res := s.Update("esc")
	if !res.Cancel || res.Accept || res.Cmd != nil {
		t.Errorf("esc: want Cancel only, got %+v", res)
	}
}
```

`TestSearchCmdRoundtrip` — `SearchCmd` keeps its `graphPath` parameter, so this
test is unaffected by the signature change; leave it as-is.

- [ ] **Step 10: Run the full suite**

Run: `go vet ./... && go test ./...`
Expected: all five packages `ok`, **no** golden diffs. If `go vet` reports an
unused import or variable, remove it (the `tea` import in app.go is still used
by `tea.Quit`/`tea.Cmd`; `graph` is still used).

- [ ] **Step 11: Commit**

```bash
git add internal/views/overlay.go internal/views/app.go internal/views/picker.go \
        internal/views/search.go internal/views/backlinks.go internal/views/todos.go \
        internal/views/help.go internal/views/app_overlay_test.go \
        internal/views/app_dispatch_test.go internal/views/search_test.go
git commit -m "refactor(views): unify overlay dispatch behind an Overlay interface"
```

---

## Final verification

- [ ] **Run the full suite one more time**

Run: `go vet ./... && go test ./...`
Expected: `cmd/peekseq`, `internal/graph`, `internal/render`, `internal/search`,
`internal/views` all `ok`. No golden frame was `-update`d.

- [ ] **Confirm the structural goals**

```bash
grep -rn "modeT\|a.picker\|a.search\|a.backlinks\|a.todos\|a.help" internal/views/*.go
```
Expected: no matches (the enum and typed pointers are gone).

```bash
grep -rn "func.*scrollWindow\|func.*innerWidth" internal/views/
```
Expected: `scrollWindow` defined once in `scroll.go`; `innerWidth` defined once
in `listbox.go`.

```bash
grep -rln "runRipgrep\|parseRipgrepJSON\|parseJSON" internal/views/
```
Expected: no matches (rg code lives only in `internal/search`).

- [ ] **Spot-check the binary** (optional, manual)

```bash
go build ./cmd/peekseq && ./peekseq --graph testdata/fixture-graph
```
Open each overlay (`ctrl+p`, `/`, `b`, `T`, `?`), navigate, and confirm the TUI
behaves identically to before. Quit with `q`.

---

## Self-review notes

- **Spec coverage:** Component 1 (Overlay interface + dispatch) → Task 6;
  Component 2 (listBox + scroll) → Tasks 3-4; Component 3 (internal/search) →
  Task 5; Component 4 (theme + keys + comment) → Tasks 1-2. All covered.
- **Behavior parity:** search hit-resolution and the index-miss-closes-without-
  navigating path are covered by `TestSearchEnterResolvesHitToPageName`.
- **No golden -update** is restated at every verification step.
