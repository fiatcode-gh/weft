# Unlinked references in the backlinks panel — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Surface unlinked references — bare-text mentions of the viewed page's name elsewhere in the graph that aren't already `[[wrapped]]` — as a second section in the backlinks panel (`b`).

**Architecture:** `internal/search` gains a whole-word `Mentions` query. `internal/graph` gains a pure `FilterUnlinked` that drops hits which are already linked, fenced, or in the page's own file (it may import `search` — verified no cycle). The `Backlinks` view renders linked + unlinked refs in one flat, scroll-spanning list. The App computes the unlinked refs on panel-open (it holds `graphPath` + index) and injects them into the view.

**Tech Stack:** Go 1.26, ripgrep (`rg`) via `os/exec`, `charmbracelet/{bubbletea,lipgloss}`, `charmbracelet/x/exp/teatest`. Reuses `search.Hit`/`search.Span`, `graph.{wikiLinkRe,fenceRe,PageNameFromFilename}`, and the view helpers `scrollWindow`/`clamp`/`clampInt`.

**Pre-commit gate (every commit):** `go vet ./... && go test ./...` — BOTH must pass. Run from the worktree root `/var/home/dhemas/Development/Projects/fiatcode/peekseq/.worktrees/unlinked-references`.

---

## File Structure

- **Modify `internal/search/search.go`** — refactor the private runner to accept ripgrep flags; add `Mentions(graphPath, name) ([]Hit, error)`.
- **Modify `internal/search/search_test.go`** — add a `Mentions` whole-word/case test.
- **Create `internal/graph/refs.go`** — `UnlinkedRef` type + pure `FilterUnlinked` (and helpers `fencedLines`, `firstUnlinkedMatch`).
- **Create `internal/graph/refs_test.go`** — pure-filter unit tests.
- **Modify `internal/views/backlinks.go`** — flat-row model (linked + unlinked sections), new `unlinked` param.
- **Modify `internal/views/backlinks_test.go`** — update call sites for the new signature; add a combined-view golden + unlinked-navigation test.
- **Modify `internal/views/app.go`** — `unlinkedRefs` helper; inject at the backlinks-open site.
- **Modify `internal/views/app_test.go`** — test `unlinkedRefs` against a temp graph.

---

## Task 1: `search.Mentions` — whole-word mention query

**Files:**
- Modify: `internal/search/search.go`
- Test: `internal/search/search_test.go`

- [ ] **Step 1: Write the failing test**

Add to `internal/search/search_test.go` (it already imports `os`, `os/exec`, `path/filepath`, `testing`; add any missing):

```go
func TestMentionsWholeWordCaseInsensitive(t *testing.T) {
	if _, err := exec.LookPath("rg"); err != nil {
		t.Skip("rg not on PATH; install ripgrep to run search tests")
	}
	tmp := t.TempDir()
	pages := filepath.Join(tmp, "pages")
	if err := os.MkdirAll(pages, 0o755); err != nil {
		t.Fatal(err)
	}
	// "Alpha" whole word (kept), "alpha" case variant (kept),
	// "Alphabet" substring (must NOT match under -w).
	body := "see Alpha here\nan alpha mention\nAlphabet soup\n"
	if err := os.WriteFile(filepath.Join(pages, "Note.md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	hits, err := Mentions(tmp, "Alpha")
	if err != nil {
		t.Fatalf("Mentions: %v", err)
	}
	if len(hits) != 2 {
		t.Fatalf("want 2 whole-word hits (Alpha, alpha), got %d: %+v", len(hits), hits)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/search/ -run TestMentionsWholeWordCaseInsensitive -v`
Expected: FAIL — `undefined: Mentions`.

- [ ] **Step 3: Refactor the runner and add `Mentions`**

In `internal/search/search.go`, replace `runRipgrep` with a flag-accepting runner and route `Run` through it (behavior for `Run` is unchanged: it still passes `--smart-case`):

```go
// runRipgrep runs `rg --json <flags...> -- <query> <pages> <journals>` and
// returns raw stdout. A ripgrep exit code of 1 (no matches) is treated as
// success. Returns (nil, nil) when neither subdirectory exists.
func runRipgrep(graphPath string, flags []string, query string) ([]byte, error) {
	args := append([]string{"--json"}, flags...)
	args = append(args, "--", query)
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
```

Update `Run` to call it (replace the existing body that called the old `runRipgrep`):

```go
// Run executes ripgrep for query against <graphPath>/pages and
// <graphPath>/journals and returns the parsed hits. It returns (nil, nil) when
// neither subdirectory exists. A ripgrep exit code of 1 (no matches) is
// treated as success.
func Run(graphPath, query string) ([]Hit, error) {
	out, err := runRipgrep(graphPath, []string{"--smart-case"}, query)
	if err != nil {
		return nil, err
	}
	return parseJSON(out), nil
}

// Mentions finds whole-word, case-insensitive, literal occurrences of name
// across pages/ and journals/. Used to detect unlinked references to a page;
// the leading/trailing word boundaries keep "Go" from matching "Google", and
// -F treats names with regex metacharacters literally.
func Mentions(graphPath, name string) ([]Hit, error) {
	out, err := runRipgrep(graphPath, []string{"-w", "-F", "--ignore-case"}, name)
	if err != nil {
		return nil, err
	}
	return parseJSON(out), nil
}
```

(The old `runRipgrep(graphPath, query)` two-arg signature and its single caller in `Run` are fully replaced by the above.)

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/search/ -run TestMentions -v`
Expected: PASS. Also run the existing search tests to confirm `Run` is unchanged: `go test ./internal/search/ -v` → all PASS.

- [ ] **Step 5: Full gate**

Run: `go vet ./... && go test ./...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/search/search.go internal/search/search_test.go
git commit -m "feat(search): add Mentions — whole-word page-name query"
```

---

## Task 2: `graph.FilterUnlinked` — the pure filter

**Files:**
- Create: `internal/graph/refs.go`
- Test: `internal/graph/refs_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/graph/refs_test.go`:

```go
package graph

import (
	"fmt"
	"testing"

	"git.fiatcode.dev/fiatcode/peekseq/internal/search"
)

func TestFilterUnlinked(t *testing.T) {
	target := "Alpha"
	targetPath := "/g/pages/Alpha.md"
	bodies := map[string]string{
		"/g/pages/Note.md":  "mentions Alpha here\nlinked [[Alpha]] already\n```\nAlpha in fence\n```\n",
		"/g/pages/Alpha.md": "Alpha self mention\n",
	}
	read := func(p string) (string, error) {
		b, ok := bodies[p]
		if !ok {
			return "", fmt.Errorf("no file %s", p)
		}
		return b, nil
	}
	hits := []search.Hit{
		{FilePath: "/g/pages/Note.md", Line: 1, Context: "mentions Alpha here", Matches: []search.Span{{Start: 9, End: 14}}},     // keep
		{FilePath: "/g/pages/Note.md", Line: 2, Context: "linked [[Alpha]] already", Matches: []search.Span{{Start: 9, End: 14}}}, // drop: inside [[ ]]
		{FilePath: "/g/pages/Note.md", Line: 4, Context: "Alpha in fence", Matches: []search.Span{{Start: 0, End: 5}}},            // drop: fenced
		{FilePath: "/g/pages/Alpha.md", Line: 1, Context: "Alpha self mention", Matches: []search.Span{{Start: 0, End: 5}}},       // drop: own file
	}
	got := FilterUnlinked(hits, target, targetPath, read)
	if len(got) != 1 {
		t.Fatalf("want 1 unlinked ref, got %d: %+v", len(got), got)
	}
	if got[0].Line != 1 || got[0].PageName != "Note" || got[0].FilePath != "/g/pages/Note.md" {
		t.Errorf("unexpected ref: %+v", got[0])
	}
}

func TestFilterUnlinkedUnreadableFileDropped(t *testing.T) {
	read := func(string) (string, error) { return "", fmt.Errorf("boom") }
	hits := []search.Hit{
		{FilePath: "/g/pages/Note.md", Line: 1, Context: "Alpha here", Matches: []search.Span{{Start: 0, End: 5}}},
	}
	if got := FilterUnlinked(hits, "Alpha", "/g/pages/Alpha.md", read); len(got) != 0 {
		t.Errorf("unreadable file should drop its hits, got %+v", got)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/graph/ -run TestFilterUnlinked -v`
Expected: FAIL — `undefined: FilterUnlinked`.

- [ ] **Step 3: Write the implementation**

Create `internal/graph/refs.go`:

```go
package graph

import (
	"path/filepath"
	"strings"

	"git.fiatcode.dev/fiatcode/peekseq/internal/search"
)

// UnlinkedRef is a bare-text mention of a page elsewhere in the graph that is
// not already a [[link]]. PageName is the referencing page (for the row label
// and navigation); Match locates the mention within Context.
type UnlinkedRef struct {
	FilePath string
	PageName string
	Line     int
	Context  string
	Match    search.Span
}

// FilterUnlinked turns raw ripgrep hits for `target` into unlinked references,
// dropping any hit that is in the target's own file, inside a fenced code
// block, or already inside a [[…]] link. read returns a file's body; a read
// error drops every hit from that file (we can't verify its fence state).
func FilterUnlinked(hits []search.Hit, target, targetPath string, read func(path string) (string, error)) []UnlinkedRef {
	fencedByFile := map[string]map[int]bool{} // nil value == unreadable file
	var out []UnlinkedRef
	for _, h := range hits {
		if h.FilePath == targetPath {
			continue
		}
		fenced, seen := fencedByFile[h.FilePath]
		if !seen {
			body, err := read(h.FilePath)
			if err != nil {
				fenced = nil
			} else {
				fenced = fencedLines(body)
			}
			fencedByFile[h.FilePath] = fenced
		}
		if fenced == nil { // unreadable
			continue
		}
		if fenced[h.Line] {
			continue
		}
		span, ok := firstUnlinkedMatch(h.Context, h.Matches)
		if !ok {
			continue
		}
		out = append(out, UnlinkedRef{
			FilePath: h.FilePath,
			PageName: PageNameFromFilename(filepath.Base(h.FilePath)),
			Line:     h.Line,
			Context:  h.Context,
			Match:    span,
		})
	}
	return out
}

// fencedLines returns the set of 1-based line numbers that fall inside (or are)
// a ``` fence, mirroring how ExtractWikiLinks skips fenced content.
func fencedLines(body string) map[int]bool {
	fenced := map[int]bool{}
	inFence := false
	for i, line := range strings.Split(body, "\n") {
		if fenceRe.MatchString(line) {
			inFence = !inFence
			fenced[i+1] = true
			continue
		}
		if inFence {
			fenced[i+1] = true
		}
	}
	return fenced
}

// firstUnlinkedMatch returns the first match span on line that does NOT fall
// within a [[…]] link span. ok is false when every match is already linked
// (or there are no matches).
func firstUnlinkedMatch(line string, matches []search.Span) (search.Span, bool) {
	links := wikiLinkRe.FindAllStringIndex(line, -1)
	for _, m := range matches {
		inside := false
		for _, l := range links {
			if m.Start >= l[0] && m.End <= l[1] {
				inside = true
				break
			}
		}
		if !inside {
			return m, true
		}
	}
	return search.Span{}, false
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/graph/ -run TestFilterUnlinked -v`
Expected: PASS (both cases).

- [ ] **Step 5: Full gate**

Run: `go vet ./... && go test ./...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/graph/refs.go internal/graph/refs_test.go
git commit -m "feat(graph): add FilterUnlinked — pure unlinked-reference filter"
```

---

## Task 3: Backlinks view — linked + unlinked sections

**Files:**
- Modify: `internal/views/backlinks.go`
- Modify: `internal/views/backlinks_test.go`
- Golden: `internal/views/testdata/TestBacklinksViewWithUnlinkedGolden.golden` (generated)

- [ ] **Step 1: Replace `backlinks.go` with the flat-row model**

Replace the entire contents of `internal/views/backlinks.go` with:

```go
package views

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"git.fiatcode.dev/fiatcode/peekseq/internal/graph"
)

// blRow is one rendered row: a non-selectable section header, or a selectable
// reference that is either a linked backlink (ref) or an unlinked one (unl).
type blRow struct {
	header bool
	text   string
	ref    *graph.Ref
	unl    *graph.UnlinkedRef
}

type Backlinks struct {
	listBox
	idx      *graph.Index
	target   string
	refs     []graph.Ref
	unlinked []graph.UnlinkedRef
	rows     []blRow
}

// NewBacklinks builds the overlay for `target`, combining the in-memory linked
// backlinks with the supplied unlinked references (the App computes those via
// ripgrep on panel-open). Self-references are filtered from the linked list.
func NewBacklinks(idx *graph.Index, target string, unlinked []graph.UnlinkedRef, width, height int) *Backlinks {
	src := idx.Backlinks[target]
	refs := make([]graph.Ref, 0, len(src))
	for _, r := range src {
		if r.FromPage == target {
			continue
		}
		refs = append(refs, r)
	}
	b := &Backlinks{
		listBox:  listBox{width: width, height: height},
		idx:      idx,
		target:   target,
		refs:     refs,
		unlinked: unlinked,
	}
	b.rows = b.buildRows()
	b.sel = b.firstSelectable()
	return b
}

// buildRows flattens linked refs, then (when present) an "Unlinked references"
// header followed by the unlinked refs, into one display list.
func (b *Backlinks) buildRows() []blRow {
	rows := make([]blRow, 0, len(b.refs)+len(b.unlinked)+1)
	for i := range b.refs {
		rows = append(rows, blRow{ref: &b.refs[i]})
	}
	if len(b.unlinked) > 0 {
		rows = append(rows, blRow{header: true, text: fmt.Sprintf("Unlinked references  (%d)", len(b.unlinked))})
		for i := range b.unlinked {
			rows = append(rows, blRow{unl: &b.unlinked[i]})
		}
	}
	return rows
}

func (b *Backlinks) firstSelectable() int {
	for i, r := range b.rows {
		if !r.header {
			return i
		}
	}
	return -1
}

// moveSel moves the selection to the next/previous selectable (non-header) row,
// clamped at the ends.
func (b *Backlinks) moveSel(dir int) {
	i := b.sel + dir
	for i >= 0 && i < len(b.rows) {
		if !b.rows[i].header {
			b.sel = i
			return
		}
		i += dir
	}
}

func (b *Backlinks) Update(key string) OverlayResult {
	switch key {
	case keyEsc, "b":
		return OverlayResult{Cancel: true}
	case keyUp, keyK, keyCtrlK:
		b.moveSel(-1)
	case keyDown, keyJ, keyCtrlJ:
		b.moveSel(+1)
	case keyEnter:
		if b.sel >= 0 && b.sel < len(b.rows) {
			r := b.rows[b.sel]
			if r.ref != nil {
				return OverlayResult{Selected: r.ref.FromPage, Accept: true}
			}
			if r.unl != nil {
				return OverlayResult{Selected: r.unl.PageName, Accept: true}
			}
		}
	}
	return OverlayResult{}
}

var blPos = lipgloss.NewStyle().Foreground(colorHighlight)

const blVisibleRowsMax = 14

// visibleRows returns how many rows the overlay renders at once. Rows scroll
// within this window when there are more of them.
func (b *Backlinks) visibleRows() int {
	// Chrome: 2 (border) + 2 (padding) + 1 (title) + 1 (blank) +
	// 1 (divider) + 1 (blank) + 1 (hint) ≈ 9 lines.
	const chrome = 9
	return clampInt(b.height-chrome, listVisibleRowsMin, blVisibleRowsMax)
}

func (b *Backlinks) View() string {
	inner := b.innerWidth()
	var sb strings.Builder
	sb.WriteString(styleTitle.Render("Backlinks"))
	sb.WriteString(styleFaint.Render(clamp(fmt.Sprintf("   → %s   (%d)", b.target, len(b.refs)), inner-len("Backlinks"))))
	sb.WriteString("\n\n")
	sb.WriteString(styleFaint.Render(strings.Repeat("─", inner)))
	sb.WriteString("\n")
	if len(b.rows) == 0 {
		sb.WriteString(styleFaint.Render("  no backlinks"))
		sb.WriteString("\n")
	}
	rowBudget := inner - 3
	start, end := scrollWindow(b.sel, len(b.rows), b.visibleRows())
	if start > 0 {
		sb.WriteString(styleFaint.Render(fmt.Sprintf("   ↑ %d more above", start)))
		sb.WriteString("\n")
	}
	for i := start; i < end; i++ {
		r := b.rows[i]
		if r.header {
			sb.WriteString("   ")
			sb.WriteString(styleFaint.Render(clamp("── "+r.text+" ", rowBudget)))
			sb.WriteString("\n")
			continue
		}
		var pos, ctx string
		if r.ref != nil {
			ctx = strings.TrimSpace(r.ref.Context)
			pos = fmt.Sprintf("%s:%d", r.ref.FromPage, r.ref.LineNumber)
		} else {
			ctx = strings.TrimSpace(r.unl.Context)
			pos = fmt.Sprintf("%s:%d", r.unl.PageName, r.unl.Line)
		}
		marker := "   " // 3-cell to match selected " ▶ " width
		var line string
		if i == b.sel {
			marker = styleSel.Render(" ▶ ")
			line = styleSel.Render(clamp(fmt.Sprintf("%s  · %s", pos, ctx), rowBudget))
		} else {
			line = clamp(blPos.Render(pos)+styleFaint.Render("  · ")+ctx, rowBudget)
		}
		sb.WriteString(marker)
		sb.WriteString(line)
		sb.WriteString("\n")
	}
	if end < len(b.rows) {
		sb.WriteString(styleFaint.Render(fmt.Sprintf("   ↓ %d more below", len(b.rows)-end)))
		sb.WriteString("\n")
	}
	sb.WriteString("\n")
	sb.WriteString(styleFaint.Render(clamp("↑/↓ select · enter open · b or esc close", inner)))
	return styleBorder.Width(inner + 4).Render(sb.String())
}
```

Note: for the linked-refs-only case (`unlinked == nil`), `rows` equals the refs and the rendered output is byte-identical to the previous implementation — the existing backlinks goldens stay valid without `-update`.

- [ ] **Step 2: Update existing call sites and tests for the new signature**

Every `NewBacklinks(idx, target, width, height)` call gains a `nil` unlinked argument:
- `internal/views/app.go` (the backlinks-open site, ~line 476): `NewBacklinks(a.idx, a.page.Page(), nil, a.width, a.height)` (Task 4 replaces `nil`).
- `internal/views/backlinks_test.go`: every call (e.g. `NewBacklinks(loadFixture(t), "Hub", 80, 30)` → `NewBacklinks(loadFixture(t), "Hub", nil, 80, 30)`), including the two golden tests and the scroll tests. Calls that build the index inline (`NewBacklinks(idx, "Alpha", 80, 24)`) get `nil` too.

- [ ] **Step 3: Verify existing behavior is preserved**

Run: `go build ./... && go test ./internal/views/ -run TestBacklinks -v`
Expected: PASS — including `TestBacklinksViewWithRefsGolden` and `TestBacklinksViewNoRefsGolden` *without* regenerating (nil unlinked ⇒ identical output). If either golden fails, the refactor changed byte output unintentionally — fix the render to match rather than `-update`.

- [ ] **Step 4: Commit the refactor**

```bash
git add internal/views/backlinks.go internal/views/app.go internal/views/backlinks_test.go
git commit -m "refactor(views): flat-row backlinks model with an unlinked section"
```

- [ ] **Step 5: Write the failing unlinked-section tests**

Add to `internal/views/backlinks_test.go` (imports `graph` already; uses `loadFixture`, `teatest`):

```go
func unlinkedFixture() []graph.UnlinkedRef {
	return []graph.UnlinkedRef{
		{FilePath: "/g/pages/Beta.md", PageName: "Beta", Line: 3, Context: "a bare Hub mention", Match: search.Span{Start: 7, End: 10}},
		{FilePath: "/g/journals/2026_05_24.md", PageName: "2026-05-24", Line: 9, Context: "Hub came up today", Match: search.Span{Start: 0, End: 3}},
	}
}

func TestBacklinksUnlinkedNavigation(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	// "Hub" has linked backlinks in the fixture; append two unlinked refs.
	b := NewBacklinks(loadFixture(t), "Hub", unlinkedFixture(), 80, 30)
	// Move selection to the last row (an unlinked ref) and open it.
	for i := 0; i < 50; i++ {
		b.Update(keyDown)
	}
	res := b.Update(keyEnter)
	if !res.Accept || res.Selected != "2026-05-24" {
		t.Errorf("enter on last unlinked row should open its page; got %+v", res)
	}
}

func TestBacklinksUnlinkedSkipsHeaderOnNav(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	b := NewBacklinks(loadFixture(t), "Hub", unlinkedFixture(), 80, 30)
	// Every reachable selection must land on a selectable (non-header) row.
	for i := 0; i < 50; i++ {
		b.Update(keyDown)
		if b.sel < 0 || b.sel >= len(b.rows) || b.rows[b.sel].header {
			t.Fatalf("selection landed on a non-selectable row: sel=%d", b.sel)
		}
	}
}

func TestBacklinksViewWithUnlinkedGolden(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	b := NewBacklinks(loadFixture(t), "Hub", unlinkedFixture(), 100, 30)
	teatest.RequireEqualOutput(t, []byte(b.View()))
}
```

Add `"git.fiatcode.dev/fiatcode/peekseq/internal/search"` to the test file's imports (for `search.Span`).

- [ ] **Step 6: Run to verify they fail, then pass**

Run: `go test ./internal/views/ -run 'TestBacklinksUnlinked|TestBacklinksViewWithUnlinkedGolden' -v`
Expected first: the two nav tests should already PASS (logic implemented in Step 1); the golden test FAILS with "no golden file". Generate it:

Run: `go test ./internal/views/ -run TestBacklinksViewWithUnlinkedGolden -update`
Then **Read** `internal/views/testdata/TestBacklinksViewWithUnlinkedGolden.golden` and confirm: the linked-refs section for Hub, then a `── Unlinked references (2)` header line, then the two unlinked rows (`Beta:3` and `2026-05-24:9` with their context). Box aligned, hint last.

- [ ] **Step 7: Re-run and full gate**

Run: `go test ./internal/views/ -run 'TestBacklinks' -v` (all pass, including the unchanged WithRefs/NoRefs goldens)
Run: `go vet ./... && go test ./...`
Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add internal/views/backlinks_test.go internal/views/testdata/TestBacklinksViewWithUnlinkedGolden.golden
git commit -m "feat(views): render and navigate the unlinked-references section"
```

---

## Task 4: App wiring — detect on panel-open

**Files:**
- Modify: `internal/views/app.go`
- Test: `internal/views/app_test.go`

- [ ] **Step 1: Write the failing test**

Add to `internal/views/app_test.go` (uses `os`, `os/exec`, `path/filepath`, `tea`, `graph` — add imports as needed):

```go
func TestAppUnlinkedRefs(t *testing.T) {
	if _, err := exec.LookPath("rg"); err != nil {
		t.Skip("rg not on PATH; install ripgrep to run this test")
	}
	tmp := t.TempDir()
	pages := filepath.Join(tmp, "pages")
	if err := os.MkdirAll(pages, 0o755); err != nil {
		t.Fatal(err)
	}
	// Alpha.md mentions its own name (must be excluded as self),
	// Note.md has one bare mention (kept) and one already-linked (dropped).
	if err := os.WriteFile(filepath.Join(pages, "Alpha.md"), []byte("# Alpha\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pages, "Note.md"), []byte("a bare Alpha mention\nand a linked [[Alpha]]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	idx, err := graph.BuildIndex(tmp)
	if err != nil {
		t.Fatal(err)
	}
	a := New(tmp, "test")
	a.idx = idx

	refs := a.unlinkedRefs("Alpha")
	if len(refs) != 1 {
		t.Fatalf("want 1 unlinked ref (Note bare mention), got %d: %+v", len(refs), refs)
	}
	if refs[0].PageName != "Note" || refs[0].Line != 1 {
		t.Errorf("unexpected ref: %+v", refs[0])
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/views/ -run TestAppUnlinkedRefs -v`
Expected: FAIL — `a.unlinkedRefs undefined`.

- [ ] **Step 3: Add the `unlinkedRefs` helper and wire it in**

In `internal/views/app.go`, ensure these imports are present (add any missing): `"os"` and `"git.fiatcode.dev/fiatcode/peekseq/internal/search"`.

Add the helper:

```go
// unlinkedRefs finds bare-text mentions of `name` elsewhere in the graph that
// aren't already links. Best-effort: a ripgrep failure yields no unlinked refs
// rather than breaking the backlinks panel — and an error hint would be
// invisible behind the overlay anyway (cf. the slice-1 hidden-hint lesson).
func (a *App) unlinkedRefs(name string) []graph.UnlinkedRef {
	hits, err := search.Mentions(a.graphPath, name)
	if err != nil {
		return nil
	}
	targetPath := ""
	if meta, ok := a.idx.ByName[name]; ok {
		targetPath = meta.Path
	}
	return graph.FilterUnlinked(hits, name, targetPath, func(p string) (string, error) {
		b, err := os.ReadFile(p)
		return string(b), err
	})
}
```

Replace the backlinks-open site (the line set to `NewBacklinks(a.idx, a.page.Page(), nil, a.width, a.height)` in Task 3) with:

```go
			name := a.page.Page()
			a.active = NewBacklinks(a.idx, name, a.unlinkedRefs(name), a.width, a.height)
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/views/ -run TestAppUnlinkedRefs -v`
Expected: PASS.

- [ ] **Step 5: Full gate**

Run: `go vet ./... && go test ./...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/views/app.go internal/views/app_test.go
git commit -m "feat(views): compute unlinked references when the backlinks panel opens"
```

---

## Task 5: Docs

**Files:**
- Modify: `internal/views/help.go`
- Modify: `AGENTS.md`, `README.md`, `CHANGELOG.md`

- [ ] **Step 1: Inspect the help backlinks/Links section**

Run: `grep -n 'Backlinks\|backlink\|Links\|b ' internal/views/help.go`
Read the surrounding lines to match the existing entry format.

- [ ] **Step 2: Update the help text (if the panel's description warrants it)**

The `b` keybinding already exists in help. If the help describes what the backlinks panel shows, extend that description to mention "linked + unlinked references". Match the file's existing entry format exactly. If a help golden exists, regenerate it:

Run: `go test ./internal/views/ -run TestHelp -update`
Then **Read** the regenerated `internal/views/testdata/TestHelpGolden.golden` and confirm it renders cleanly.

- [ ] **Step 3: Prose docs**

- `README.md`: in the backlinks/feature section, add a sentence — the backlinks panel (`b`) now shows **unlinked references** (bare-text mentions of the page that aren't yet `[[linked]]`) beneath the linked backlinks; `enter` jumps to the mention's page.
- `AGENTS.md`: add a matching note where the backlinks view is described.
- `CHANGELOG.md`: under a new `## [Unreleased]` section with `### Added` (the current top entry is `## [1.2.0]`), add:
  ```
  - Backlinks panel (`b`) now lists **unlinked references** — bare-text mentions of the page elsewhere in the graph that aren't yet `[[linked]]` — beneath the linked backlinks. Read-only; `enter` jumps to the mention.
  ```

Read each file first to match its style before editing.

- [ ] **Step 4: Full gate**

Run: `go vet ./... && go test ./...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/views/help.go internal/views/testdata AGENTS.md README.md CHANGELOG.md
git commit -m "docs: document unlinked references in help, README, AGENTS, CHANGELOG"
```

---

## Self-Review (completed during planning)

- **Spec coverage:** detection via ripgrep on the name (Task 1 `Mentions`, `-w -F --ignore-case`) ✓; matching rules — inside-`[[…]]`, fenced, own-file, whole-word, case-insensitive (Task 2 `FilterUnlinked` + Task 1 `-w`/`--ignore-case`; tests assert each) ✓; fold into backlinks panel as a second section (Task 3) ✓; `enter` navigates to the file (Task 3 `Update`, `TestBacklinksUnlinkedNavigation`) ✓; App orchestrates detection on panel-open, view stays display-only (Task 4 `unlinkedRefs` + injection) ✓; graceful degradation on rg failure without a hidden hint (Task 4) ✓; pure-filter tests + `Mentions` test + combined golden (Tasks 1–3) ✓; non-goals (linkify, aliases, namespaced-leaf, caps) — not built ✓.
- **Placeholder scan:** none — every code step is complete; the only adapt-to-style step is help wording (Task 5 Step 2), with the exact prose given for README/AGENTS/CHANGELOG.
- **Type consistency:** `search.Mentions(graphPath, name) ([]Hit, error)`, `graph.UnlinkedRef{FilePath, PageName, Line, Context, Match}`, `graph.FilterUnlinked(hits, target, targetPath, read)`, `NewBacklinks(idx, target, unlinked, width, height)`, and `(*App).unlinkedRefs(name)` are used identically across tasks. `search.Span{Start,End}` and `search.Hit{FilePath,Line,Context,Matches}` match the existing definitions read from `internal/search/search.go`.
