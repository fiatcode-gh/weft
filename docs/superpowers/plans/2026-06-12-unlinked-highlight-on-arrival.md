# Highlight unlinked mentions on arrival — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** When you `enter` on an unlinked reference in the backlinks panel, the destination page lands with the bare-text mention highlighted and scrolled into view.

**Architecture:** Add a third sentinel type ("emphasis") to the render pipeline, reusing the proven wiki-link/task sentinel machinery — `preprocessEmphasis` wraps whole-word, case-insensitive occurrences of a term (outside fences/inline-code/`[[…]]`) in PUA sentinels before Glamour; the restore pass styles them as a highlight and records byte offsets in `Result.Finds`. `PageView` re-renders with the term (bypassing its per-page cache) and scrolls to the first find. The App passes the viewed page's name as the emphasis term when navigating from an unlinked row.

**Tech Stack:** Go 1.26, `charmbracelet/glamour` + `lipgloss`, `regexp`. Reuses `preprocessWikiLinks`/`sentinelRe`/the restore loop in `internal/render/page.go` and the `ScrollToTask` offset→row math in `internal/views/page.go`.

**Pre-commit gate (every commit):** `go vet ./... && go test ./...` — BOTH must pass. Run from `/var/home/dhemas/Development/Projects/fiatcode/peekseq/.worktrees/unlinked-references`.

---

## File Structure

- **Modify `internal/render/page.go`** — `emphasisStyle`, emphasis PUA sentinel consts, extend `sentinelRe`, `preprocessEmphasis` + `emphasizeOutsideInlineCode`, `Result.Finds`, `RenderWithEmphasis` (and `Render` delegates to it), restore-loop emphasis case.
- **Modify `internal/render/page_test.go`** — emphasis render tests.
- **Modify `internal/views/page.go`** — `emphasis` field, `SetPage` reset, `SetPageEmphasizing`, `load()` cache-bypass-when-emphasis, `scrollToFirstFind`.
- **Modify `internal/views/page_test.go`** — PageView emphasis/scroll/clear test.
- **Modify `internal/views/overlay.go`** — `OverlayResult.HighlightText`.
- **Modify `internal/views/backlinks.go`** — unlinked `enter` sets `HighlightText`.
- **Modify `internal/views/backlinks_test.go`** — unlinked-row HighlightText assertion.
- **Modify `internal/views/app.go`** — `navigateHighlighting` + Accept branch.
- **Modify `internal/views/app_test.go`** — `navigateHighlighting` integration test.

---

## Task 1: Render-level emphasis highlight

**Files:**
- Modify: `internal/render/page.go`
- Test: `internal/render/page_test.go`

- [ ] **Step 1: Write the failing tests**

Add to `internal/render/page_test.go` (match the file's existing env setup — if other render tests set `NO_COLOR`/`TERM`, do the same; the assertions below rely on `Finds`, not on ANSI):

```go
func TestRenderWithEmphasisRecordsFinds(t *testing.T) {
	res, err := RenderWithEmphasis("see Alpha here\n", 80, "Alpha")
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Finds) != 1 {
		t.Fatalf("want 1 find, got %d", len(res.Finds))
	}
	if !strings.Contains(res.Styled, "Alpha") {
		t.Errorf("emphasised term should still appear in output")
	}
}

func TestRenderNoEmphasisNoFinds(t *testing.T) {
	res, err := Render("see Alpha here\n", 80)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Finds) != 0 {
		t.Errorf("Render should record no finds; got %d", len(res.Finds))
	}
}

func TestRenderWithEmphasisSkipsLinkCodeFence(t *testing.T) {
	body := "bare Alpha here\n" +
		"a [[Alpha]] link\n" +
		"inline `Alpha` code\n" +
		"```\nAlpha in fence\n```\n"
	res, err := RenderWithEmphasis(body, 80, "Alpha")
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Finds) != 1 {
		t.Fatalf("only the bare mention should be highlighted; got %d finds", len(res.Finds))
	}
}

func TestRenderWithEmphasisWholeWordCasePreserved(t *testing.T) {
	res, err := RenderWithEmphasis("an alpha and Alphabet\n", 80, "Alpha")
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Finds) != 1 {
		t.Fatalf("want 1 find (alpha, not Alphabet); got %d", len(res.Finds))
	}
	if !strings.Contains(res.Styled, "alpha") {
		t.Errorf("original casing 'alpha' must be preserved in output")
	}
	if !strings.Contains(res.Styled, "Alphabet") {
		t.Errorf("Alphabet must remain in output untouched")
	}
}
```

- [ ] **Step 2: Run to verify they fail**

Run: `go test ./internal/render/ -run 'TestRenderWithEmphasis|TestRenderNoEmphasisNoFinds' -v`
Expected: FAIL — `undefined: RenderWithEmphasis` and `res.Finds` undefined.

- [ ] **Step 3: Implement**

In `internal/render/page.go`:

(a) Add the emphasis style near `linkStyle`:
```go
// emphasisStyle highlights a searched/arrived-at term on the page. Reverse
// video stands out from linkStyle (blue underline) and degrades to plain text
// under NO_COLOR/notty.
var emphasisStyle = lipgloss.NewStyle().Reverse(true)
```

(b) Add a NEW PUA sentinel pair, in a range DISTINCT from the existing wiki/task sentinels. First READ the existing `wikiSentinelStart/End/Pad` and `taskSentinelStart/End` constant values (they're PUA codepoints) and choose codepoints that do not collide. Add to the `const (...)` block:
```go
	emphSentinelStart = "" // emphasis sentinel — distinct PUA range from wiki/task
	emphSentinelEnd   = ""
```
(If ``/`` collide with the existing constants, pick other unused PUA codepoints and note it. The requirement: distinct from wiki and task sentinels.)

(c) Extend `sentinelRe` to also match the emphasis sentinel as a third alternation (a new capture group):
```go
var sentinelRe = regexp.MustCompile(
	wikiSentinelStart + `(\d+)` + wikiSentinelEnd + `(?:` + wikiSentinelPad + `)*` +
		`|` + taskSentinelStart + `(\d+)` + taskSentinelEnd +
		`|` + emphSentinelStart + `(\d+)` + emphSentinelEnd,
)
```
Now group 1 = `m[2:3]` (wiki id), group 2 = `m[4:5]` (task id), group 3 = `m[6:7]` (emphasis id).

(d) Add `Finds` to `Result`:
```go
type Result struct {
	Styled string
	Links  []Link
	Tasks  []int
	// Finds holds the byte offset in Styled of each highlighted emphasis-term
	// occurrence, in document order. Empty unless rendered with an emphasis term.
	Finds []int
}
```

(e) Add the emphasis preprocessing (place near `preprocessWikiLinks`):
```go
// preprocessEmphasis wraps whole-word, case-insensitive occurrences of term in
// emphasis sentinels (outside fences and inline code), returning the rewritten
// body and the original matched substrings indexed by sentinel id (so casing is
// preserved on restore). Returns (body, nil) when term is empty. Run AFTER
// preprocessWikiLinks/preprocessTaskMarkers so [[term]] occurrences are already
// sentinels and only bare mentions match.
func preprocessEmphasis(body, term string) (string, []string) {
	if term == "" {
		return body, nil
	}
	re := regexp.MustCompile(`(?i)\b` + regexp.QuoteMeta(term) + `\b`)
	var subs []string
	var out strings.Builder
	out.Grow(len(body))
	inFence := false
	lines := strings.Split(body, "\n")
	for i, line := range lines {
		switch {
		case fenceRe.MatchString(line):
			inFence = !inFence
			out.WriteString(line)
		case inFence:
			out.WriteString(line)
		default:
			out.WriteString(emphasizeOutsideInlineCode(line, re, &subs))
		}
		if i < len(lines)-1 {
			out.WriteByte('\n')
		}
	}
	return out.String(), subs
}

// emphasizeOutsideInlineCode replaces term matches with emphasis sentinels, but
// only outside backtick-delimited inline code (mirrors
// replaceWikiLinksOutsideInlineCode).
func emphasizeOutsideInlineCode(line string, re *regexp.Regexp, subs *[]string) string {
	parts := strings.Split(line, "`")
	for i, part := range parts {
		if i%2 == 1 {
			continue // inside inline code
		}
		parts[i] = re.ReplaceAllStringFunc(part, func(match string) string {
			id := len(*subs)
			*subs = append(*subs, match)
			return fmt.Sprintf("%s%d%s", emphSentinelStart, id, emphSentinelEnd)
		})
	}
	return strings.Join(parts, "`")
}
```

(f) Replace `Render` and add `RenderWithEmphasis`. Find the current `func Render(body string, width int) (Result, error)` and change it to:
```go
// Render returns Glamour-rendered markdown with wiki-link positions annotated.
// width is the target terminal column count.
func Render(body string, width int) (Result, error) {
	return RenderWithEmphasis(body, width, "")
}

// RenderWithEmphasis is Render plus highlighting whole-word occurrences of
// emphasis on the page (recorded in Result.Finds). emphasis == "" is identical
// to Render.
func RenderWithEmphasis(body string, width int, emphasis string) (Result, error) {
	body = stripLogbookBlocks(body)
	body = stripQueryAndEmbedBlocks(body)
	pre, wikiSubs := preprocessWikiLinks(body)
	pre, taskMarkers := preprocessTaskMarkers(pre)
	pre, emphSubs := preprocessEmphasis(pre, emphasis)

	r, err := rendererFor(width)
	if err != nil {
		return Result{}, err
	}
	styled, err := r.Render(pre)
	if err != nil {
		styled = pre
	}
	styled = indentWrappedBullets(styled)

	var out strings.Builder
	out.Grow(len(styled))
	links := make([]Link, 0, len(wikiSubs))
	var tasks []int
	var finds []int
	last := 0
	for _, m := range sentinelRe.FindAllStringSubmatchIndex(styled, -1) {
		out.WriteString(styled[last:m[0]])
		last = m[1]
		switch {
		case m[2] >= 0: // wiki-link sentinel
			id, err := strconv.Atoi(styled[m[2]:m[3]])
			if err != nil || id < 0 || id >= len(wikiSubs) {
				out.WriteString(styled[m[0]:m[1]])
				continue
			}
			rendered := linkStyle.Render(wikiSubs[id].display)
			start := out.Len()
			out.WriteString(rendered)
			links = append(links, Link{
				Target:  wikiSubs[id].target,
				Display: wikiSubs[id].display,
				Start:   start,
				End:     start + len(rendered),
			})
		case m[4] >= 0: // task-marker sentinel
			id, err := strconv.Atoi(styled[m[4]:m[5]])
			if err != nil || id < 0 || id >= len(taskMarkers) {
				out.WriteString(styled[m[0]:m[1]])
				continue
			}
			if taskMarkers[id].open {
				tasks = append(tasks, out.Len())
			}
			out.WriteString(renderTaskMarker(taskMarkers[id].marker))
		case m[6] >= 0: // emphasis sentinel
			id, err := strconv.Atoi(styled[m[6]:m[7]])
			if err != nil || id < 0 || id >= len(emphSubs) {
				out.WriteString(styled[m[0]:m[1]])
				continue
			}
			finds = append(finds, out.Len())
			out.WriteString(emphasisStyle.Render(emphSubs[id]))
		}
	}
	out.WriteString(styled[last:])

	return Result{Styled: out.String(), Links: links, Tasks: tasks, Finds: finds}, nil
}
```

- [ ] **Step 4: Run to verify they pass**

Run: `go test ./internal/render/ -v`
Expected: PASS — the new emphasis tests AND all pre-existing render tests (wiki-link, task, fence, etc.) unchanged.

- [ ] **Step 5: Full gate**

Run: `go vet ./... && go test ./...`
Expected: PASS. (Page-view goldens unaffected — `PageView.load` still calls `Render` with no emphasis.)

- [ ] **Step 6: Commit**

```bash
git add internal/render/page.go internal/render/page_test.go
git commit -m "feat(render): emphasis-term highlight with Finds offsets"
```

---

## Task 2: PageView emphasis + scroll

**Files:**
- Modify: `internal/views/page.go`
- Test: `internal/views/page_test.go`

- [ ] **Step 1: Write the failing test**

Add to `internal/views/page_test.go` (add imports `os`, `path/filepath` if missing; `graph` is already used):

```go
func TestPageViewEmphasizeScrollsThenClears(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	tmp := t.TempDir()
	pages := filepath.Join(tmp, "pages")
	if err := os.MkdirAll(pages, 0o755); err != nil {
		t.Fatal(err)
	}
	var sb strings.Builder
	for i := 0; i < 80; i++ {
		sb.WriteString("- filler line\n")
	}
	sb.WriteString("- a bare Alpha mention near the bottom\n")
	if err := os.WriteFile(filepath.Join(pages, "Note.md"), []byte(sb.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	idx, err := graph.BuildIndex(tmp)
	if err != nil {
		t.Fatal(err)
	}
	pv := NewPageView(idx, "Note", 80, 24)
	if len(pv.result.Finds) != 0 {
		t.Fatalf("no emphasis yet → no finds; got %d", len(pv.result.Finds))
	}
	pv.SetPageEmphasizing("Note", "Alpha")
	if len(pv.result.Finds) == 0 {
		t.Fatalf("emphasis should record finds")
	}
	if pv.Offset() == 0 {
		t.Errorf("should scroll toward the bottom mention; offset still 0")
	}
	// Plain navigation clears the highlight and must not serve a cached
	// highlighted render.
	pv.SetPage("Note")
	if pv.emphasis != "" {
		t.Errorf("SetPage should clear emphasis")
	}
	if len(pv.result.Finds) != 0 {
		t.Errorf("plain render must have no finds; got %d", len(pv.result.Finds))
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/views/ -run TestPageViewEmphasizeScrollsThenClears -v`
Expected: FAIL — `pv.SetPageEmphasizing` undefined and `pv.emphasis` undefined.

- [ ] **Step 3: Implement** — in `internal/views/page.go`:

(a) Add the `emphasis` field to the `PageView` struct (near `cursor int`):
```go
	emphasis string // transient term to highlight on arrival; "" = none
```

(b) Update `SetPage` to clear emphasis:
```go
// SetPage switches to a different page in the same index.
func (p *PageView) SetPage(name string) {
	p.page = name
	p.cursor = -1
	p.emphasis = ""
	p.load()
}
```

(c) Add `SetPageEmphasizing` (after `SetPage`):
```go
// SetPageEmphasizing switches to name and highlights whole-word occurrences of
// term (the page navigated from), scrolling to the first. The highlight is
// transient: a later SetPage or history Restore clears it.
func (p *PageView) SetPageEmphasizing(name, term string) {
	p.page = name
	p.cursor = -1
	p.emphasis = term
	p.load()
	p.scrollToFirstFind()
}
```

(d) Update `load()` to bypass the cache when an emphasis term is set, and use `RenderWithEmphasis`:
```go
func (p *PageView) load() {
	p.err = nil
	p.result = render.Result{}
	meta, ok := p.idx.Resolve(p.page)
	if !ok {
		return
	}
	// The per-page cache only holds plain (un-emphasised) renders. When an
	// emphasis term is set the render is transient — never read or write the
	// cache, so a later plain navigation can't be served a highlighted version.
	if p.emphasis == "" {
		if c, hit := p.cache[meta.Name]; hit && c.modTime.Equal(meta.ModTime) {
			p.result = c.result
			p.vp.SetContent(p.result.Styled)
			return
		}
	}
	b, err := os.ReadFile(meta.Path)
	if err != nil {
		p.err = err
		return
	}
	atomic.AddInt64(&renderCount, 1)
	body := strings.TrimSpace(string(b)) + "\n"
	var res render.Result
	if p.emphasis != "" {
		res, err = render.RenderWithEmphasis(body, p.width, p.emphasis)
	} else {
		res, err = render.Render(body, p.width)
	}
	if err != nil {
		p.err = err
		return
	}
	p.result = res
	if p.emphasis == "" {
		p.cache[meta.Name] = cachedPage{result: res, modTime: meta.ModTime}
	}
	p.vp.SetContent(p.result.Styled)
}
```

(e) Add `scrollToFirstFind` (near `ScrollToTask`):
```go
// scrollToFirstFind centres the viewport on the first highlighted emphasis
// occurrence, if any.
func (p *PageView) scrollToFirstFind() {
	if len(p.result.Finds) == 0 {
		return
	}
	off := p.result.Finds[0]
	if off < 0 || off > len(p.result.Styled) {
		return
	}
	row := strings.Count(p.result.Styled[:off], "\n")
	target := row - p.vp.Height/2
	if target < 0 {
		target = 0
	}
	p.vp.SetYOffset(target)
}
```

- [ ] **Step 4: Run to verify it passes**

Run: `go test ./internal/views/ -run TestPageViewEmphasizeScrollsThenClears -v`
Expected: PASS.

- [ ] **Step 5: Full gate**

Run: `go vet ./... && go test ./...`
Expected: PASS — existing page-view tests/goldens unchanged (plain `SetPage` path is unchanged except the `emphasis=""` reset, which doesn't alter output).

- [ ] **Step 6: Commit**

```bash
git add internal/views/page.go internal/views/page_test.go
git commit -m "feat(views): PageView highlights and scrolls to an emphasis term"
```

---

## Task 3: Wire unlinked-row navigation to highlight

**Files:**
- Modify: `internal/views/overlay.go`, `internal/views/backlinks.go`, `internal/views/app.go`
- Test: `internal/views/backlinks_test.go`, `internal/views/app_test.go`

- [ ] **Step 1: Write the failing tests**

In `internal/views/backlinks_test.go`, add:
```go
func TestBacklinksUnlinkedRefHighlights(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	b := NewBacklinks(loadFixture(t), "Hub", unlinkedFixture(), 80, 30)
	// Move to the last row (an unlinked ref) and accept it.
	for i := 0; i < 50; i++ {
		b.Update(keyDown)
	}
	res := b.Update(keyEnter)
	if !res.Accept || res.HighlightText != "Hub" {
		t.Errorf("unlinked-ref enter should set HighlightText=Hub; got %+v", res)
	}
	if res.FocusLinkTo != "" {
		t.Errorf("unlinked-ref enter must not set FocusLinkTo; got %q", res.FocusLinkTo)
	}
}
```

In `internal/views/app_test.go`, add a `navigateHighlighting` integration test. Build the App on a temp graph with a tall page that bare-mentions a target, using the SAME initialization pattern as the existing `bootApp`/`TestAppUnlinkedRefs` tests (read those first). Skeleton:
```go
func TestAppNavigateHighlighting(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	tmp := t.TempDir()
	pages := filepath.Join(tmp, "pages")
	if err := os.MkdirAll(pages, 0o755); err != nil {
		t.Fatal(err)
	}
	var sb strings.Builder
	for i := 0; i < 60; i++ {
		sb.WriteString("- filler\n")
	}
	sb.WriteString("- mentions Hub down here\n")
	if err := os.WriteFile(filepath.Join(pages, "Note.md"), []byte(sb.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pages, "Hub.md"), []byte("# Hub\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	idx, err := graph.BuildIndex(tmp)
	if err != nil {
		t.Fatal(err)
	}
	a := New(tmp, "test")
	a.idx = idx
	a.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	a.page = NewPageView(idx, "Hub", 80, 24) // start somewhere with a page view
	a.navigateHighlighting("Note", "Hub")
	if a.page.Page() != "Note" {
		t.Fatalf("should navigate to Note; got %q", a.page.Page())
	}
	if len(a.page.result.Finds) == 0 {
		t.Errorf("destination should have highlighted finds for the bare Hub mention")
	}
}
```
If `New` + manual `a.page` / `a.width` setup diverges from how the existing tests do it, follow the existing pattern (e.g. reuse `bootApp` if it accepts a graph path, or replicate its steps). Do NOT weaken the `Finds` assertion; if you cannot construct a working App+PageView in a test, report it rather than asserting something trivial.

- [ ] **Step 2: Run to verify they fail**

Run: `go test ./internal/views/ -run 'TestBacklinksUnlinkedRefHighlights|TestAppNavigateHighlighting' -v`
Expected: FAIL — `HighlightText` field and `navigateHighlighting` undefined.

- [ ] **Step 3: Implement**

(a) `internal/views/overlay.go` — add to `OverlayResult`:
```go
	// HighlightText, when Accept is true and non-empty, asks the App to navigate
	// to Selected and highlight occurrences of this term on the destination
	// (scrolling to the first). Only the Backlinks overlay sets it, for unlinked
	// refs.
	HighlightText string
```

(b) `internal/views/backlinks.go` — in `Update`'s `keyEnter` case, the unlinked branch:
```go
			if r.unl != nil {
				return OverlayResult{Selected: r.unl.PageName, Accept: true, HighlightText: b.target}
			}
```
(Leave the `r.ref != nil` linked branch with `FocusLinkTo: b.target` unchanged.)

(c) `internal/views/app.go` — add after `navigateFocusingLink`:
```go
// navigateHighlighting navigates to name and highlights occurrences of term
// (the page navigated from) on the destination, scrolling to the first — used
// for unlinked references, which have no link to focus a cursor on. One-shot:
// the new history entry stores no emphasis, so [ / ] restore lands without it.
func (a *App) navigateHighlighting(name, term string) {
	if a.histIdx >= 0 && a.histIdx < len(a.hist) {
		a.hist[a.histIdx].offset = a.page.Offset()
		a.hist[a.histIdx].cursor = a.page.Cursor()
	}
	a.hist = append(a.hist[:a.histIdx+1], historyEntry{
		page:        name,
		offset:      0,
		cursor:      -1,
		taskOrdinal: -1,
	})
	a.histIdx = len(a.hist) - 1
	a.page.SetPageEmphasizing(name, term)
}
```
And update the Accept handler's `if res.Selected != "" { ... }` block to add the `HighlightText` branch (after `FocusLinkTo`, before `DeepLink`):
```go
				if res.Selected != "" {
					if res.FocusLinkTo != "" {
						a.navigateFocusingLink(res.Selected, res.FocusLinkTo)
					} else if res.HighlightText != "" {
						a.navigateHighlighting(res.Selected, res.HighlightText)
					} else if res.DeepLink {
						a.navigateToTask(res.Selected, res.TaskOrdinal)
					} else {
						a.navigate(res.Selected)
					}
				}
```

- [ ] **Step 4: Run to verify they pass**

Run: `go test ./internal/views/ -run 'TestBacklinksUnlinkedRefHighlights|TestAppNavigateHighlighting' -v`
Expected: PASS.

- [ ] **Step 5: Full gate**

Run: `go vet ./... && go test ./...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/views/overlay.go internal/views/backlinks.go internal/views/app.go internal/views/backlinks_test.go internal/views/app_test.go
git commit -m "feat(views): highlight the mention when jumping from an unlinked ref"
```

---

## Task 4: Docs

**Files:**
- Modify: `README.md`, `AGENTS.md`, `CHANGELOG.md`

- [ ] **Step 1: Update the docs**

Read each file first to match style, then:
- `README.md`: in the backlinks section, extend the unlinked-references sentence — `enter` on an unlinked reference now jumps to the page **with the mention highlighted and scrolled into view** (linked backlinks land on the back-reference link).
- `AGENTS.md`: extend the backlinks note to mention the on-arrival highlight for unlinked refs (and the link-cursor focus for linked refs).
- `CHANGELOG.md`: under the existing `## [Unreleased]` → `### Added`, append:
  ```
  - Jumping from a backlink now lands on the reference: linked backlinks focus the back-reference link, and unlinked references highlight the mention and scroll it into view.
  ```

- [ ] **Step 2: Full gate**

Run: `go vet ./... && go test ./...`
Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add README.md AGENTS.md CHANGELOG.md
git commit -m "docs: backlink jumps land on the reference (highlight / focus)"
```

---

## Self-Review (completed during planning)

- **Spec coverage:** `RenderWithEmphasis` + `preprocessEmphasis` + emphasis sentinel + `Result.Finds` + `emphasisStyle` (Task 1) ✓; skip inside link/inline-code/fence via post-wiki ordering + backtick split (Task 1, tested) ✓; whole-word + case-insensitive + casing preserved (Task 1 regex `(?i)\b…\b` storing original match, tested) ✓; PageView emphasis field + cache bypass + scroll + clear-on-SetPage (Task 2, tested incl. cache-not-polluted) ✓; `OverlayResult.HighlightText`, unlinked-row sets it, App `navigateHighlighting` + Accept branch ordering (Task 3, tested) ✓; one-shot (history stores no emphasis) ✓; docs (Task 4) ✓.
- **Placeholder scan:** none — every code step is complete. The two adapt-to-existing-style points (render test env in Task 1 Step 1; App test init in Task 3 Step 1) name the exact existing patterns to follow and forbid weakening assertions.
- **Type consistency:** `RenderWithEmphasis(body, width, emphasis string) (Result, error)`, `Render` delegates to it; `Result.Finds []int`; `preprocessEmphasis(body, term) (string, []string)`; `PageView.SetPageEmphasizing(name, term string)`, `scrollToFirstFind()`, `emphasis` field; `OverlayResult.HighlightText string`; `App.navigateHighlighting(name, term string)` — used consistently across tasks. Sentinel restore uses `m[6:7]` for the third capture group (consistent with `sentinelRe`'s added alternation).
