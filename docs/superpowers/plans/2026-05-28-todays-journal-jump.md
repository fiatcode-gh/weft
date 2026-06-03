# Today's Journal Jump & Prev/Next Navigation — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add three page-view keybinds — `.` to jump to today's journal, `<` to walk to the previous journal, `>` to walk to the next journal — plus the small surfaces they need (sorted journal list on the graph index, injectable clock on the App, ephemeral hint slot in the status bar).

**Architecture:** `internal/graph` learns "what journals exist, sorted" (`Index.Journals []string`); `internal/views/App` learns "what to do with that list" using an injectable `nowFunc func() time.Time`. The new key handlers route through the existing `navigate()` chokepoint so every successful jump pushes a history entry; failures (today absent, off the end of the journal list) set a single-cycle hint string that the status bar shows in place of `? help` until the next keystroke.

**Tech Stack:** Go 1.26, Bubble Tea, `charmbracelet/bubbles/viewport`, `charmbracelet/lipgloss`, `charmbracelet/x/exp/teatest`.

**Spec:** `docs/superpowers/specs/2026-05-28-todays-journal-jump-design.md`

---

## Background for the implementer

`peekseq` is a read-only Bubble Tea TUI for browsing a Logseq graph. Key files:

- `internal/graph/index.go` — `BuildIndex(graphPath)` walks `pages/` + `journals/` and returns an `*Index`. Each `PageMeta` already has an `IsJournal bool`. Journal page names look like `2026-05-28` (derived from `2026_05_28.md` via `PageNameFromFilename`).
- `internal/graph/resolve.go` — `IsJournalFilename` + `PageNameFromFilename` already encode the Logseq date conventions.
- `internal/views/app.go` — root model. Has `modeT` enum, the history stack, `navigate(name)`, `historyBack`, `historyForward`. The `modePage` keymap dispatch is the `tea.KeyMsg` switch at app.go:271. The free function `todayJournalName()` (app.go:78) currently calls `time.Now()` directly and is invoked from `tryInitPage` to seed the first history entry.
- `internal/views/page.go` — `PageView` wraps `viewport.Model`, owns `cursor`, exposes `Offset`/`Cursor`/`Restore`/`Page`/`StatusLine`/`ScrollIndicator`.
- `internal/views/help.go` — `helpSections` slice describes the overlay; rows are `{key, desc}` pairs.

Fixture journals (`testdata/fixture-graph/journals/`): `2026-01-10`, `2026-03-15`, `2026-04-20`, `2026-05-01`, `2026-05-15`, `2026-05-22`, `2026-05-23`, `2026-05-24`, `2026-05-25`. The Jan→Mar→Apr gaps are large; the May 22→23→24→25 run is consecutive. Both shapes are useful for `<`/`>` tests.

`bootApp(t)` (`internal/views/app_test.go`) drives the full boot sequence synchronously and returns an App with a constructed PageView at 80×24. Existing dispatch tests live in `internal/views/app_dispatch_test.go` and use `a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("g")})` for key events.

**Project rules (from `AGENTS.md` and global `CLAUDE.md`):**

- TDD: failing test first, minimal implementation, then refactor if needed. Tests stay green between commits.
- Run `go vet ./... && go test ./...` before every commit — both must pass.
- Conventional Commits (`feat:`, `fix:`, `refactor:`, `test:`, `docs:`, `chore:`).
- `teatest.RequireEqualOutput` compares against goldens under `internal/views/testdata/`. After intentional UI changes, regenerate with `go test ./... -update` and visually diff before staging.
- Never point tests at the real graph (`~/Documents/fiat-codex`) — only `testdata/fixture-graph/`.

---

## File map

- **Modify:** `internal/graph/index.go` — add `Journals []string` field on `Index`, populate it in `BuildIndex` after the first directory pass.
- **Modify:** `internal/graph/index_test.go` — add tests asserting `Journals` content & emptiness.
- **Modify:** `internal/views/app.go` — promote `todayJournalName` to an App method; add `nowFunc` field with `time.Now` default; add `hint string` field; plumb `hint` through `statusBar()` and clear it at the top of `tea.KeyMsg` handling; add `journalNeighbor` helper; add `.`, `<`, `>` case branches in the `modePage` switch.
- **Modify:** `internal/views/app_test.go` — add a `bootAppAt(t, now)` helper for tests that need a deterministic clock.
- **Modify:** `internal/views/app_dispatch_test.go` — add the eight new tests.
- **Modify:** `internal/views/help.go` — three new rows in the *Open* section.
- **Modify:** `internal/views/testdata/*.golden` — regenerate help-overlay goldens.
- **Modify:** `README.md` — update the key table to include `.`, `<`, `>`.

---

## Task 1: `Index.Journals` (graph layer)

**Files:**
- Modify: `internal/graph/index.go`
- Modify: `internal/graph/index_test.go`

- [ ] **Step 1: Write the failing tests**

Append to `internal/graph/index_test.go`:

```go
func TestBuildIndexJournalsSorted(t *testing.T) {
	idx, err := BuildIndex(fixturePath(t))
	if err != nil {
		t.Fatalf("BuildIndex: %v", err)
	}

	want := []string{
		"2026-01-10", "2026-03-15", "2026-04-20", "2026-05-01",
		"2026-05-15", "2026-05-22", "2026-05-23", "2026-05-24", "2026-05-25",
	}
	if !equalSlices(idx.Journals, want) {
		t.Errorf("Journals: want %v, got %v", want, idx.Journals)
	}

	// Every entry must correspond to a PageMeta with IsJournal == true.
	for _, name := range idx.Journals {
		meta, ok := idx.ByName[name]
		if !ok {
			t.Errorf("Journals contains %q but ByName doesn't", name)
			continue
		}
		if !meta.IsJournal {
			t.Errorf("Journals entry %q has IsJournal=false", name)
		}
	}
}

func TestBuildIndexJournalsEmptyWhenNoJournals(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "pages"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "pages", "Lonely.md"), []byte("- hi\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	idx, err := BuildIndex(dir)
	if err != nil {
		t.Fatalf("BuildIndex: %v", err)
	}
	if len(idx.Journals) != 0 {
		t.Errorf("Journals: want empty slice, got %v", idx.Journals)
	}
}
```

Add `"os"` to the existing `import` block in `index_test.go` if it isn't already present.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/graph/ -run 'TestBuildIndexJournals' -v`
Expected: both tests FAIL — `idx.Journals` undefined (compile error).

- [ ] **Step 3: Add the field and populate it**

In `internal/graph/index.go`, add `"sort"` to the import block, add the field to `Index`, and populate it after the first directory pass (before the body-parsing pass).

Field:

```go
type Index struct {
	GraphPath string
	Pages     []PageMeta
	ByName    map[string]*PageMeta
	Backlinks map[string][]Ref
	Todos     []TodoBullet
	Journals  []string // journal page names, sorted ascending
}
```

Population (insert immediately before the `// Second pass: parse bodies...` comment near line 59):

```go
	// Collect journal page names sorted ascending. Names are YYYY-MM-DD so
	// lexical order matches chronological order.
	for _, p := range idx.Pages {
		if p.IsJournal {
			idx.Journals = append(idx.Journals, p.Name)
		}
	}
	sort.Strings(idx.Journals)
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/graph/ -v`
Expected: PASS for `TestBuildIndexJournalsSorted`, `TestBuildIndexJournalsEmptyWhenNoJournals`, and the pre-existing `TestBuildIndex`.

- [ ] **Step 5: Vet & full test sweep**

Run: `go vet ./... && go test ./...`
Expected: both clean.

- [ ] **Step 6: Commit**

```bash
git add internal/graph/index.go internal/graph/index_test.go
git commit -m "feat(graph): add sorted Journals slice to Index"
```

---

## Task 2: Promote `todayJournalName` to a method with injectable clock

**Files:**
- Modify: `internal/views/app.go`
- Modify: `internal/views/app_test.go`

This is a refactor — behaviour is unchanged because `nowFunc` defaults to `time.Now`. Tests confirm the method respects an injected clock and the existing boot sequence still works.

- [ ] **Step 1: Write the failing test**

Append to `internal/views/app_test.go`:

```go
import (
	// ... existing imports ...
	"time"
)

func TestTodayJournalNameUsesNowFunc(t *testing.T) {
	a := New("/unused", "test")
	a.nowFunc = func() time.Time { return time.Date(2026, 5, 23, 12, 0, 0, 0, time.UTC) }
	if got := a.todayJournalName(); got != "2026-05-23" {
		t.Errorf("todayJournalName: want 2026-05-23, got %q", got)
	}
}

// bootAppAt is like bootApp but pins App.nowFunc before the WindowSizeMsg so
// tryInitPage seeds history with a deterministic page name. Use for tests
// that exercise the . / < / > keys against the fixture.
func bootAppAt(t *testing.T, now time.Time) *App {
	t.Helper()
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	abs, err := filepath.Abs("../../testdata/fixture-graph")
	if err != nil {
		t.Fatal(err)
	}
	a := New(abs, "test")
	a.nowFunc = func() time.Time { return now }
	cmd := a.Init()
	if cmd == nil {
		t.Fatal("Init returned nil cmd")
	}
	msg := cmd()
	a.Update(msg)
	a.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	if a.page == nil {
		t.Fatal("PageView not constructed after boot")
	}
	return a
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/views/ -run TestTodayJournalNameUsesNowFunc -v`
Expected: FAIL — compile error (`a.nowFunc` undefined, `a.todayJournalName` undefined as a method).

- [ ] **Step 3: Promote to a method with the injectable clock**

In `internal/views/app.go`:

(a) Add the field to the `App` struct (insert near the existing fields, e.g. just before the history block at line 55):

```go
	// nowFunc returns "now" for today-journal resolution. Defaults to
	// time.Now; tests inject a fixed clock.
	nowFunc func() time.Time
```

(b) Set the default in `New` (replace the existing `New` body):

```go
func New(graphPath, version string) *App {
	return &App{
		graphPath: graphPath,
		mode:      modePage,
		histIdx:   -1,
		version:   version,
		nowFunc:   time.Now,
	}
}
```

(c) Replace the free function at line 78:

```go
// Old:
//   func todayJournalName() string { return time.Now().Format("2006-01-02") }
// New:
func (a *App) todayJournalName() string { return a.nowFunc().Format("2006-01-02") }
```

(d) Update the caller in `tryInitPage` (around line 95). Change `name := todayJournalName()` to `name := a.todayJournalName()`.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/views/ -v`
Expected: `TestTodayJournalNameUsesNowFunc` PASS; all pre-existing tests still PASS (the `time.Now` default preserves their behaviour).

- [ ] **Step 5: Vet & full test sweep**

Run: `go vet ./... && go test ./...`
Expected: both clean.

- [ ] **Step 6: Commit**

```bash
git add internal/views/app.go internal/views/app_test.go
git commit -m "refactor(views): make todayJournalName an App method with injectable clock"
```

---

## Task 3: `.` key jumps to today's journal (with ephemeral hint surface)

**Files:**
- Modify: `internal/views/app.go`
- Modify: `internal/views/app_dispatch_test.go`

Introduces the single-cycle `hint` field as part of the first feature that needs it.

- [ ] **Step 1: Write the failing tests**

Append to `internal/views/app_dispatch_test.go`:

```go
import (
	// ... existing imports ...
	"time"
)

func TestPeriodJumpsToTodayJournal(t *testing.T) {
	a := bootAppAt(t, time.Date(2026, 5, 23, 12, 0, 0, 0, time.UTC))
	// Boot lands on today's journal (2026-05-23) per tryInitPage seeding.
	// Navigate elsewhere first so we can verify . actually moves us.
	a.navigate("Alpha")
	startHistLen := len(a.hist)

	a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(".")})

	if got := a.page.Page(); got != "2026-05-23" {
		t.Errorf("after .: want page 2026-05-23, got %q", got)
	}
	if got := len(a.hist); got != startHistLen+1 {
		t.Errorf("history len: want %d, got %d", startHistLen+1, got)
	}
	if a.hint != "" {
		t.Errorf("hint should be empty on successful jump, got %q", a.hint)
	}
}

func TestPeriodOnAbsentTodayShowsHint(t *testing.T) {
	// 2026-06-15 has no journal in the fixture.
	a := bootAppAt(t, time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC))
	a.navigate("Alpha")
	startPage := a.page.Page()
	startHistLen := len(a.hist)

	a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(".")})

	if got := a.page.Page(); got != startPage {
		t.Errorf("after . on absent today: page changed from %q to %q", startPage, got)
	}
	if got := len(a.hist); got != startHistLen {
		t.Errorf("history grew on absent today: want %d, got %d", startHistLen, got)
	}
	if want := "no journal for 2026-06-15"; a.hint != want {
		t.Errorf("hint: want %q, got %q", want, a.hint)
	}

	// Status bar must surface the hint in place of "? help".
	bar := a.statusBar()
	if !strings.Contains(bar, "no journal for 2026-06-15") {
		t.Errorf("status bar missing hint; got:\n%s", bar)
	}
	if strings.Contains(bar, "? help") {
		t.Errorf("status bar should hide ? help while hint is set; got:\n%s", bar)
	}
}

func TestHintClearsOnNextKey(t *testing.T) {
	a := bootAppAt(t, time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC))
	a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(".")})
	if a.hint == "" {
		t.Fatal("setup: expected hint to be set by first .")
	}
	// Any subsequent key clears the hint at the top of Update.
	a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	if a.hint != "" {
		t.Errorf("hint should clear on next key, got %q", a.hint)
	}
}

func TestPeriodIdempotentOnTodayJournal(t *testing.T) {
	a := bootAppAt(t, time.Date(2026, 5, 23, 12, 0, 0, 0, time.UTC))
	// Boot already lands on today's journal.
	if got := a.page.Page(); got != "2026-05-23" {
		t.Fatalf("setup: want boot page 2026-05-23, got %q", got)
	}
	a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(".")})
	if got := a.page.Page(); got != "2026-05-23" {
		t.Errorf("after . on today: want 2026-05-23, got %q", got)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/views/ -run 'TestPeriod|TestHintClears' -v`
Expected: FAIL — compile errors (`a.hint` undefined, `.` key not handled so page stays put and length checks fail).

- [ ] **Step 3: Add the hint field and plumb it through `statusBar`**

In `internal/views/app.go`, add to the `App` struct (next to `nowFunc`):

```go
	// hint is a single-cycle right-side status replacement. Set by handlers
	// that need to surface a transient message (e.g. "no journal for ..."),
	// cleared at the top of the next tea.KeyMsg.
	hint string
```

Modify `statusBar()` (currently at app.go:372). Replace the `rightText` resolution block. The full new body of `statusBar` is:

```go
func (a *App) statusBar() string {
	left := a.page.StatusLine()
	var rightText string
	if a.hint != "" {
		rightText = a.hint
	} else {
		rightText = "? help"
		if ind := a.page.ScrollIndicator(); ind != "" {
			rightText = ind + "  " + rightText
		}
	}
	right := statusFaint.Render(rightText)
	width := a.width
	if width <= 0 {
		width = lipgloss.Width(left) + 2 + lipgloss.Width(right)
	}
	rightW := lipgloss.Width(right)
	leftBudget := width - rightW - 1
	if leftBudget < 1 {
		leftBudget = 1
	}
	left = clamp(left, leftBudget)
	rule := statusRule.Render(strings.Repeat("─", width))
	gap := width - lipgloss.Width(left) - rightW
	if gap < 1 {
		gap = 1
	}
	return rule + "\n" + left + strings.Repeat(" ", gap) + right
}
```

- [ ] **Step 4: Clear `hint` at the top of `tea.KeyMsg` handling**

In `(a *App) Update`, the `case tea.KeyMsg:` block currently starts with `key := m.String()` (around app.go:194-195). Insert a hint-clear immediately after that line:

```go
		case tea.KeyMsg:
			key := m.String()
			a.hint = ""
```

- [ ] **Step 5: Add the `.` handler**

In the `modePage` switch (app.go:271), add a new case between `case "]":` and `case "g":`:

```go
			case ".":
				today := a.todayJournalName()
				if _, ok := a.idx.ByName[today]; ok {
					a.navigate(today)
				} else {
					a.hint = "no journal for " + today
				}
```

- [ ] **Step 6: Run tests to verify they pass**

Run: `go test ./internal/views/ -run 'TestPeriod|TestHintClears' -v`
Expected: all four PASS.

- [ ] **Step 7: Vet & full test sweep**

Run: `go vet ./... && go test ./...`
Expected: both clean. (The existing `TestAppStatusBarContainsScrollIndicator` and `TestAppStatusBarHidesIndicatorWhenFits` keep passing because `a.hint` is empty in those tests.)

- [ ] **Step 8: Commit**

```bash
git add internal/views/app.go internal/views/app_dispatch_test.go
git commit -m "feat(views): add . key to jump to today's journal"
```

---

## Task 4: `<` and `>` keys walk through journals

**Files:**
- Modify: `internal/views/app.go`
- Modify: `internal/views/app_dispatch_test.go`

Adds the `journalNeighbor` helper plus the two key handlers. All gating, edge, and gap-skip behaviour locks down here.

- [ ] **Step 1: Write the failing tests**

Append to `internal/views/app_dispatch_test.go`:

```go
func TestPrevJournalWalksBackwards(t *testing.T) {
	a := bootAppAt(t, time.Date(2026, 5, 24, 12, 0, 0, 0, time.UTC))
	// Boot lands on 2026-05-24.
	a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("<")})
	if got := a.page.Page(); got != "2026-05-23" {
		t.Errorf("after <: want 2026-05-23, got %q", got)
	}
	if a.hint != "" {
		t.Errorf("hint should be empty on successful walk, got %q", a.hint)
	}
}

func TestNextJournalWalksForward(t *testing.T) {
	a := bootAppAt(t, time.Date(2026, 5, 23, 12, 0, 0, 0, time.UTC))
	a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(">")})
	if got := a.page.Page(); got != "2026-05-24" {
		t.Errorf("after >: want 2026-05-24, got %q", got)
	}
}

func TestPrevJournalSkipsGapDays(t *testing.T) {
	// Fixture has 2026-04-20 then 2026-03-15 — large gap. < from 04-20 lands
	// on 03-15, skipping the missing calendar days in between.
	a := bootAppAt(t, time.Date(2026, 4, 20, 12, 0, 0, 0, time.UTC))
	a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("<")})
	if got := a.page.Page(); got != "2026-03-15" {
		t.Errorf("after < across gap: want 2026-03-15, got %q", got)
	}
}

func TestPrevJournalAtOldestShowsHint(t *testing.T) {
	a := bootAppAt(t, time.Date(2026, 1, 10, 12, 0, 0, 0, time.UTC))
	startPage := a.page.Page()
	startHistLen := len(a.hist)
	a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("<")})
	if got := a.page.Page(); got != startPage {
		t.Errorf("< at oldest: page changed from %q to %q", startPage, got)
	}
	if got := len(a.hist); got != startHistLen {
		t.Errorf("< at oldest grew history: want %d, got %d", startHistLen, got)
	}
	if a.hint != "no earlier journal" {
		t.Errorf("hint: want %q, got %q", "no earlier journal", a.hint)
	}
}

func TestNextJournalAtNewestShowsHint(t *testing.T) {
	a := bootAppAt(t, time.Date(2026, 5, 25, 12, 0, 0, 0, time.UTC))
	startPage := a.page.Page()
	startHistLen := len(a.hist)
	a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(">")})
	if got := a.page.Page(); got != startPage {
		t.Errorf("> at newest: page changed from %q to %q", startPage, got)
	}
	if got := len(a.hist); got != startHistLen {
		t.Errorf("> at newest grew history: want %d, got %d", startHistLen, got)
	}
	if a.hint != "no later journal" {
		t.Errorf("hint: want %q, got %q", "no later journal", a.hint)
	}
}

func TestPrevNextInertOutsideJournalContext(t *testing.T) {
	a := bootAppAt(t, time.Date(2026, 5, 23, 12, 0, 0, 0, time.UTC))
	a.navigate("Alpha")
	startPage := a.page.Page()
	startHistLen := len(a.hist)

	a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("<")})
	a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(">")})

	if got := a.page.Page(); got != startPage {
		t.Errorf("<,> outside journal context: page changed from %q to %q", startPage, got)
	}
	if got := len(a.hist); got != startHistLen {
		t.Errorf("<,> outside journal context grew history: want %d, got %d", startHistLen, got)
	}
	if a.hint != "" {
		t.Errorf("<,> outside journal context: hint should be empty, got %q", a.hint)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/views/ -run 'TestPrevJournal|TestNextJournal|TestPrevNext' -v`
Expected: all six FAIL — `<` and `>` are unhandled keys today, so pages stay put and hint stays empty.

- [ ] **Step 3: Add the `journalNeighbor` helper**

In `internal/views/app.go`, add `"sort"` to the imports if not already present, then add this method below `historyForward` (around app.go:144):

```go
// journalNeighbor returns the journal name adjacent to current in
// a.idx.Journals (dir=-1 prev, dir=+1 next) and ok=true. Returns
// ("", false) when current isn't in the sorted journal list or when
// the neighbour would be off the ends.
func (a *App) journalNeighbor(current string, dir int) (string, bool) {
	js := a.idx.Journals
	i := sort.SearchStrings(js, current)
	if i == len(js) || js[i] != current {
		return "", false
	}
	j := i + dir
	if j < 0 || j >= len(js) {
		return "", false
	}
	return js[j], true
}
```

- [ ] **Step 4: Add the `<` and `>` handlers**

In the `modePage` switch in `Update`, immediately after the `case ".":` block added in Task 3, add:

```go
			case "<":
				if name, ok := a.journalNeighbor(a.page.Page(), -1); ok {
					a.navigate(name)
				} else if cur, exists := a.idx.ByName[a.page.Page()]; exists && cur.IsJournal {
					a.hint = "no earlier journal"
				}
			case ">":
				if name, ok := a.journalNeighbor(a.page.Page(), +1); ok {
					a.navigate(name)
				} else if cur, exists := a.idx.ByName[a.page.Page()]; exists && cur.IsJournal {
					a.hint = "no later journal"
				}
```

The `else if` guard distinguishes "current isn't a journal" (silent no-op, no hint) from "current is the oldest/newest journal" (hint). `journalNeighbor` returns false in both cases; we re-check `IsJournal` on the current page to know which case applies.

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/views/ -run 'TestPrevJournal|TestNextJournal|TestPrevNext' -v`
Expected: all six PASS.

- [ ] **Step 6: Vet & full test sweep**

Run: `go vet ./... && go test ./...`
Expected: both clean.

- [ ] **Step 7: Commit**

```bash
git add internal/views/app.go internal/views/app_dispatch_test.go
git commit -m "feat(views): add < and > keys to walk through journals"
```

---

## Task 5: Help overlay — three new rows + regenerate goldens

**Files:**
- Modify: `internal/views/help.go`
- Modify: `internal/views/testdata/*.golden`

- [ ] **Step 1: Find which help goldens exist**

Run: `ls internal/views/testdata/ | grep -i help`
Expected: one or more `.golden` files matching the help overlay (likely `help_*.golden`).

- [ ] **Step 2: Add the three rows to `helpSections`**

In `internal/views/help.go`, locate the `"Open"` section in `helpSections` (around lines 63-68). Add three rows ABOVE the existing four so the journal keys come first in the Open group:

```go
		{"Open", []helpRow{
			{".", "today's journal"},
			{"<", "previous journal"},
			{">", "next journal"},
			{"ctrl-p", "picker — find any page"},
			{"/", "search the graph (ripgrep)"},
			{"b", "backlinks to the current page"},
			{"T", "open todos dashboard"},
		}},
```

- [ ] **Step 3: Run help tests to see them fail (golden mismatch)**

Run: `go test ./internal/views/ -run 'TestHelp' -v`
Expected: FAIL with `RequireEqualOutput` mismatch — the rendered overlay now has three extra lines the goldens don't.

- [ ] **Step 4: Regenerate the goldens**

Run: `go test ./internal/views/ -run 'TestHelp' -update -v`
Expected: PASS; the `.golden` files are rewritten.

- [ ] **Step 5: Visually diff the regenerated goldens**

Run: `git diff internal/views/testdata/`
Confirm the diff shows ONLY the three new rows (`.`, `<`, `>`) added under *Open*. No other lines should change. If anything else differs, stop and investigate — the help layout is otherwise stable and unintended drift points to a separate problem.

- [ ] **Step 6: Vet & full test sweep**

Run: `go vet ./... && go test ./...`
Expected: both clean.

- [ ] **Step 7: Commit**

```bash
git add internal/views/help.go internal/views/testdata/
git commit -m "feat(views): document . < > journal keys in help overlay"
```

---

## Task 6: README key table update

**Files:**
- Modify: `README.md`

- [ ] **Step 1: Add three rows to the key table**

The README key table starts at `README.md:35` with this shape:

```
| Key        | Action                              |
| ---        | ---                                 |
| `Ctrl-P`   | open picker (recent + fuzzy)        |
...
| `[` / `]`  | back / forward in page history      |
...
```

Find the line `| `[` / `]`  | back / forward in page history      |` (currently README.md:44) and insert three new rows immediately after it. Use exactly this text — the first column is padded to 10 characters between backticks, the action column starts at the same offset as the existing rows:

```
| `.`        | jump to today's journal             |
| `<`        | previous journal (on a journal page)|
| `>`        | next journal (on a journal page)    |
```

If the existing table's column padding shifts in the future, mirror whatever the surrounding rows use — the goal is visual parity with the rest of the table.

- [ ] **Step 2: Vet & full test sweep**

Run: `go vet ./... && go test ./...`
Expected: both clean (README change touches no Go code).

- [ ] **Step 3: Commit**

```bash
git add README.md
git commit -m "docs: add . < > journal keys to README key table"
```

---

## Task 7: Manual smoke against the real graph

**Files:** none modified — confirmation step only.

- [ ] **Step 1: Build the binary**

Run: `go build ./cmd/peekseq`
Expected: clean build, `./peekseq` exists.

- [ ] **Step 2: Smoke run**

If the user's graph is available at `~/Documents/fiat-codex` (or `$PEEKSEQ_GRAPH`), run: `./peekseq --graph ~/Documents/fiat-codex`

Manually exercise:

1. From the boot page, press `.` — should land on today's journal (or show `no journal for <today>` hint if today's file doesn't exist).
2. From today, press `<` — lands on the previous existing journal, skipping any gap days.
3. Press `<` a few more times — walks backward through journals.
4. Press `>` repeatedly — walks forward.
5. At the newest journal, press `>` — hint `no later journal`, no navigation.
6. Press any other key (e.g. `j`) — hint clears, status bar returns to `? help`.
7. Navigate to a non-journal page (`Ctrl-P` → some page) — press `<` and `>` — both inert, no hint shown.
8. Press `?` — help overlay shows `.`, `<`, `>` in the *Open* section.
9. Press `[` — rewinds back out of the journal walk.

If anything misbehaves, file a follow-up — do NOT add fixes silently.

- [ ] **Step 3: No commit**

Smoke step has no artifacts to commit.

---

## Self-Review Notes

**Spec coverage check:**

- Spec "`.` resolution" → Task 3 (TestPeriodJumpsToTodayJournal, TestPeriodOnAbsentTodayShowsHint, TestPeriodIdempotentOnTodayJournal).
- Spec "`<` / `>` resolution" → Task 4 (six tests covering walk, gap-skip, edges, gating).
- Spec "State / `Index.Journals`" → Task 1.
- Spec "State / `nowFunc`" → Task 2.
- Spec "Status hints / ephemeral `a.hint`" → Task 3 (TestHintClearsOnNextKey + status bar assertions).
- Spec "Help overlay" → Task 5.
- Spec "Manual smoke" → Task 7.
- Spec "R interaction" — `idx.Journals` is rebuilt as part of `BuildIndex`, which already runs on `R`. No new code path needed; nothing to test beyond what Task 1 already covers (Journals populated correctly by BuildIndex).
- Spec "YAGNI list" — every excluded item stays excluded in the plan (no config.edn reading, no auto-creation, no calendar-day stepping, no wrap, no non-journal anchor, no Journals overlay, no status-bar position indicator).

**Type/name consistency check:**

- `Index.Journals []string` — same name in graph code, tests, and helper.
- `App.nowFunc func() time.Time` — same name everywhere it's referenced.
- `App.hint string` — same name in field, `statusBar()`, and the clear-on-key line.
- `(a *App) todayJournalName() string` — method form used in `tryInitPage`, `.` handler, and tests.
- `(a *App) journalNeighbor(current string, dir int) (string, bool)` — signature matches every call site.
- Hint strings: `"no journal for <YYYY-MM-DD>"`, `"no earlier journal"`, `"no later journal"` — same exact strings in handlers and test assertions.
