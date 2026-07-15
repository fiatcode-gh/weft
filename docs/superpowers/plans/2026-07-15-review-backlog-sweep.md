# Review-Backlog Sweep Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking. Every task follows TDD: failing test → red → minimal fix → green → commit.

**Goal:** Sweep the items deliberately deferred from the 2026-07-14 review fixes (PR #31): the remaining low-severity behavior bugs, the small quality refactors, the two test-infrastructure items, and staticcheck in CI.

**Architecture:** Same codebase as PR #31 (weft: Bubble Tea TUI over a Logseq-compatible markdown graph). All line references below are against the tree at PR #31's head (`2d472d1`). Three phases: behavior fixes → quality refactors → tests & tooling. (`alias::` support was cut: the property is being migrated out of the graph itself — see the final section.)

**Tech Stack:** Go, bubbletea/bubbles/lipgloss, glamour, ripgrep. Test helpers: `writeGraph`/`quietTerm`/`bootApp` (views), `testdata/fake-editor.sh` (editor hand-off fixture).

## Global Constraints

- **Base:** branch `fix/review-backlog-sweep` off `main` *after* PR #31 merges. If #31 is not yet merged, stop and say so instead of branching off the PR branch.
- Read `AGENTS.md` first; its conventions are binding.
- TDD every task: failing test first. Test bodies use `// arrange` / `// act` / `// assert`; shared setup in helpers.
- Conventional Commits, one commit per task.
- Before EVERY commit: `go test ./... && go vet ./... && test -z "$(gofmt -l .)"` all pass. After Task 13 lands, also `go run honnef.co/go/tools/cmd/staticcheck@v0.7.0 ./...`.
- No new third-party dependencies. weft is a *navigator, not an outliner* — no scope creep.
- Phase gates: `go test -race ./...`.

---

## Phase 1 — remaining behavior fixes

### Task 1: `ctrl+c` must quit while an overlay is open

Overlays swallow every key (`app.go`, the `if a.active != nil` branch — currently the comment reads "An open overlay swallows all keys until it accepts or cancels"). `ctrl+c` quits everywhere else (`app.go:609`, `:681`, editor confirm flow); with a picker/search/todos/backlinks/help overlay open it does nothing.

**Files:**
- Modify: `internal/views/app.go` (top of the `a.active != nil` key branch)
- Test: `internal/views/app_overlay_test.go`

- [ ] **Step 1: Write the failing test**

```go
func TestCtrlCQuitsWithOverlayOpen(t *testing.T) {
	// arrange — open the picker overlay
	a := bootApp(t)
	a.Update(tea.KeyMsg{Type: tea.KeyCtrlP})
	if a.active == nil {
		t.Fatal("precondition: overlay did not open")
	}

	// act
	_, cmd := a.Update(tea.KeyMsg{Type: tea.KeyCtrlC})

	// assert
	if cmd == nil {
		t.Fatal("ctrl+c with overlay open returned no cmd")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatalf("ctrl+c with overlay open did not quit (got %T)", cmd())
	}
}
```

(If `tea.KeyCtrlP` doesn't map to the `"ctrl+p"` key string the dispatcher sees, mirror how existing overlay tests open the picker — check `app_overlay_test.go` for the established idiom first.)

- [ ] **Step 2: Run to verify failure** — `go test ./internal/views/ -run TestCtrlCQuitsWithOverlayOpen -v` — expected: FAIL (nil cmd; the overlay ignored the key).

- [ ] **Step 3: Implement** — intercept before routing to the overlay:

```go
	// An open overlay swallows all keys until it accepts or cancels —
	// except ctrl+c, which must always quit (Bubble Tea convention; it
	// works in every other mode).
	if a.active != nil {
		if key == "ctrl+c" {
			return a, tea.Quit
		}
		res := a.active.Update(key)
		...
```

- [ ] **Step 4: Run** — `go test ./internal/views/ -v` — expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/views/
git commit -m "fix(views): ctrl+c quits even while an overlay is open"
```

### Task 2: Preserve read-view scroll/cursor across PageView rebuilds

Two paths rebuild `PageView` from scratch and land the user back at the top: the `indexLoadedMsg` refresh (`app.go`, `a.page = NewPageView(a.idx, a.page.Page(), …)` — hit by `R`, sync-pull reindex, and editor save-and-exit) and the editor discard-exit path (`res.Exit && !saved`, same rebuild). `Restore` already exists and clamps out-of-range offsets, so the fix is capture-and-restore around both rebuilds.

**Files:**
- Modify: `internal/views/app.go` (the `indexLoadedMsg` handler's refresh branch and the `res.Exit`/`!saved` branch)
- Test: `internal/views/app_test.go`

- [ ] **Step 1: Write the failing tests**

```go
func TestReindexPreservesScrollPosition(t *testing.T) {
	// arrange — long page, scrolled deep
	quietTerm(t)
	dir, _ := writeGraph(t, map[string]string{
		"pages/Long.md": strings.Repeat("- line\n", 80),
	})
	a := New(dir, "test")
	a.Update(a.Init()())
	a.Update(tea.WindowSizeMsg{Width: 80, Height: 12})
	a.navigate("Long")
	for i := 0; i < 30; i++ {
		a.page.LineDown()
	}
	want := a.page.Offset()
	if want == 0 {
		t.Fatal("precondition: page not scrolled")
	}

	// act — R reindex, driven synchronously
	_, cmd := a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("R")})
	drainCmds(t, a, cmd) // deliver the batch's indexLoadedMsg; reuse/adapt the package's existing cmd-draining idiom

	// assert
	if got := a.page.Offset(); got != want {
		t.Fatalf("offset after R = %d, want %d", got, want)
	}
}

func TestEditorDiscardExitPreservesScrollPosition(t *testing.T) {
	// arrange — same long page, scrolled, then e → esc (clean buffer)
	quietTerm(t)
	dir, _ := writeGraph(t, map[string]string{
		"pages/Long.md": strings.Repeat("- line\n", 80),
	})
	a := New(dir, "test")
	a.Update(a.Init()())
	a.Update(tea.WindowSizeMsg{Width: 80, Height: 12})
	a.navigate("Long")
	for i := 0; i < 30; i++ {
		a.page.LineDown()
	}
	want := a.page.Offset()
	a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("e")})
	if a.editor == nil {
		t.Fatal("precondition: editor did not open")
	}

	// act — clean esc discards without confirm and rebuilds the page view
	a.Update(tea.KeyMsg{Type: tea.KeyEsc})

	// assert
	if a.editor != nil {
		t.Fatal("editor did not close on clean esc")
	}
	if got := a.page.Offset(); got != want {
		t.Fatalf("offset after e→esc = %d, want %d", got, want)
	}
}
```

(For `drainCmds`: `app_dispatch_test.go` already drives batched cmds synchronously — reuse that helper/idiom rather than writing a new one.)

- [ ] **Step 2: Run to verify failure** — `go test ./internal/views/ -run 'PreservesScroll' -v` — expected: FAIL (offset 0 after rebuild).

- [ ] **Step 3: Implement** — both rebuild sites get capture/restore:

`indexLoadedMsg` refresh branch:

```go
if a.page != nil {
	// Refresh path (R, sync-pull, editor save-exit): rebuild PageView
	// for the same page so it picks up new links/todos — but keep the
	// user's place. Restore clamps if the page shrank.
	off, cur := a.page.Offset(), a.page.Cursor()
	a.page = NewPageView(a.idx, a.page.Page(), a.width, a.height)
	a.page.Restore(off, cur)
}
```

Editor discard-exit branch (`res.Exit`, `!saved`):

```go
} else {
	off, cur := a.page.Offset(), a.page.Cursor()
	a.page = NewPageView(a.idx, a.page.Page(), a.width, a.height)
	a.page.Restore(off, cur)
}
```

- [ ] **Step 4: Run** — `go test ./internal/views/ -v` — expected: PASS. Watch the deep-link tests (`ScrollToTask`) — they navigate rather than refresh, so they're unaffected; if any golden asserts top-of-page after R, that assertion was pinning the bug.

- [ ] **Step 5: Commit**

```bash
git add internal/views/
git commit -m "fix(views): preserve scroll/cursor across reindex and editor-exit rebuilds"
```

### Task 3: Logbook/query strippers — block state must outrank fence state

In `stripLogbookBlocks` and `stripQueryAndEmbedBlocks` (`internal/render/page.go`), `case fence.Step(line)` is evaluated before `case inLogbook`/`inBlock`, so a ``` ``` `` line *inside* a `:LOGBOOK:` block flips fence state, leaks the block, and kills every wiki link on the rest of the page.

**Files:**
- Modify: `internal/render/page.go` (`stripLogbookBlocks`, `stripQueryAndEmbedBlocks`)
- Test: `internal/render/page_test.go`

- [ ] **Step 1: Write the failing test**

```go
func TestLogbookBlockContainingFenceLineIsFullyStripped(t *testing.T) {
	// arrange — a fence delimiter inside :LOGBOOK: metadata must not
	// flip fence state for the rest of the page
	body := "- item\n  :LOGBOOK:\n  ```\n  :END:\n- [[After]]\n"

	// act
	res, err := Render(body, 80)

	// assert
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(res.Styled, ":END:") {
		t.Fatal(":LOGBOOK: block leaked into the rendered page")
	}
	if len(res.Links) != 1 || res.Links[0].Target != "After" {
		t.Fatalf("links = %+v, want [[After]]", res.Links)
	}
}
```

- [ ] **Step 2: Run to verify failure** — `go test ./internal/render/ -run TestLogbookBlockContainingFence -v` — expected: FAIL (`:END:` leaks, 0 links).

- [ ] **Step 3: Implement** — hoist the in-block check above the switch so dropped metadata lines never feed the fence tracker (a fence delimiter inside a dropped block is garbage, not grammar); a literal `:LOGBOOK:` inside a real fence still passes through because `inLogbook` can only become true outside a fence:

```go
func stripLogbookBlocks(body string) string {
	var out strings.Builder
	out.Grow(len(body))
	lines := strings.Split(body, "\n")
	var fence graph.FenceState
	inLogbook := false
	for i, line := range lines {
		if inLogbook {
			// Everything inside the block is dropped without touching
			// fence state — a ``` line here is metadata garbage.
			if logbookEndRe.MatchString(line) {
				inLogbook = false
			}
			continue
		}
		switch {
		case fence.Step(line):
			out.WriteString(line)
		case logbookStartRe.MatchString(line):
			inLogbook = true
			continue // drop the :LOGBOOK: line; no newline either
		default:
			out.WriteString(line)
		}
		if i < len(lines)-1 {
			out.WriteByte('\n')
		}
	}
	return out.String()
}
```

Apply the identical restructure to `stripQueryAndEmbedBlocks` (`inBlock` / `queryOrEmbedRe` / closing `}}` check).

- [ ] **Step 4: Run** — `go test ./internal/render/ -v` — expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/render/
git commit -m "fix(render): logbook/query block state outranks fence state so embedded fence lines can't leak the block"
```

### Task 4: Exact-name resolution must survive a case-fold collision

`internal/graph/index.go` (the ByName/ByNameFold build loop): on a fold collision the `continue` skips the `ByName` insert too, so with both `Alpha.md` and `alpha.md` on disk, `Resolve("alpha")` returns page `Alpha` even though an exact-match page `alpha` exists — contradicting `Resolve`'s own doc ("matches name exactly, or…").

**Files:**
- Modify: `internal/graph/index.go` (collision branch)
- Test: `internal/graph/index_test.go`

- [ ] **Step 1: Write the failing test**

```go
func TestFoldCollisionKeepsExactNameResolution(t *testing.T) {
	// arrange — two pages differing only by case
	dir := t.TempDir()
	pages := filepath.Join(dir, "pages")
	if err := os.MkdirAll(pages, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"Alpha.md", "alpha.md"} {
		if err := os.WriteFile(filepath.Join(pages, n), []byte("- x\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// act
	idx, err := BuildIndex(dir)
	if err != nil {
		t.Fatal(err)
	}

	// assert — exact lookups win for BOTH; the ambiguous folded lookup
	// stays deterministic (first-walked: ReadDir is filename-sorted,
	// "Alpha.md" < "alpha.md" in ASCII)
	for name, want := range map[string]string{
		"Alpha": "Alpha",
		"alpha": "alpha",
		"ALPHA": "Alpha",
	} {
		meta, ok := idx.Resolve(name)
		if !ok || meta.Name != want {
			t.Errorf("Resolve(%q) = %v/%v, want %q", name, meta, ok, want)
		}
	}
}
```

- [ ] **Step 2: Run to verify failure** — `go test ./internal/graph/ -run TestFoldCollisionKeepsExact -v` — expected: FAIL (`Resolve("alpha")` → `Alpha`).

- [ ] **Step 3: Implement** — in the collision branch, still register the exact name (`Resolve` checks `ByName` before `ByNameFold`, so this is sufficient):

```go
if existing, ok := idx.ByNameFold[fold]; ok {
	if existing.Name != name {
		fmt.Fprintf(os.Stderr, "weft: ambiguous page name %q vs %q (case-insensitive); [[%s]] resolves to %q\n",
			name, existing.Name, fold, existing.Name)
		// The folded key stays first-wins, but an exact-name lookup
		// has no ambiguity — it must still find this page.
		if _, dup := idx.ByName[name]; !dup {
			idx.ByName[name] = &idx.Pages[i]
		}
	}
	continue
}
```

- [ ] **Step 4: Run** — `go test ./internal/graph/ -v` — expected: PASS (the existing collision-warning test at `index_test.go` still holds — folded resolution unchanged).

- [ ] **Step 5: Commit**

```bash
git add internal/graph/
git commit -m "fix(graph): exact-name resolution survives a case-fold collision"
```

### Task 5: Strip trailing `\r` from search hit context (CRLF files)

`internal/search/search.go` (`parseJSON`, the `strings.TrimRight(env.Data.Lines.Text, "\n")` line — currently `search.go:110`): a CRLF file leaves a trailing `\r` in `Hit.Context`, which leaks into rendered search/unlinked rows. `graph.lineContext` already strips it — this is the asymmetric sibling.

**Files:**
- Modify: `internal/search/search.go:110`
- Test: `internal/search/search_test.go`

- [ ] **Step 1: Write the failing test**

```go
func TestParseJSONStripsTrailingCR(t *testing.T) {
	// arrange — rg reports a CRLF file's line with \r\n intact
	input := []byte(`{"type":"match","data":{"path":{"text":"pages/A.md"},"lines":{"text":"- hit here\r\n"},"line_number":1,"submatches":[{"start":2,"end":5}]}}` + "\n")

	// act
	hits, err := parseJSON(input)

	// assert
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 1 || hits[0].Context != "- hit here" {
		t.Fatalf("Context = %q, want trailing CR stripped", hits[0].Context)
	}
}
```

- [ ] **Step 2: Run to verify failure** — `go test ./internal/search/ -run TestParseJSONStripsTrailingCR -v` — expected: FAIL (`"- hit here\r"`).

- [ ] **Step 3: Implement**

```go
ctx := strings.TrimRight(env.Data.Lines.Text, "\r\n")
```

(Match spans are offsets into the line's *leading* bytes; trimming the tail only shrinks `ctxLen`, and the existing end-clamp already handles that.)

- [ ] **Step 4: Run** — `go test ./internal/search/ -v` — expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/search/
git commit -m "fix(search): strip trailing CR from CRLF hit context"
```

### Task 6: Stop stripping `#` fragments from link targets (`[[C#]]`)

`graph/parse.go` (`appendWikiLinks`, the `strings.IndexByte(raw, '#')` strip) and `render/page.go` (`replaceWikiLinksOutsideInlineCode`, the mirrored strip) truncate `[[C#]]` to target `C`. Logseq has no `[[page#fragment]]` syntax and allows `#` in page names — the strip is lossy. Remove it from both places (they must stay aligned).

**Files:**
- Modify: `internal/graph/parse.go` (`appendWikiLinks`), `internal/render/page.go` (`replaceWikiLinksOutsideInlineCode`)
- Test: `internal/graph/parse_test.go`, `internal/render/page_test.go`

- [ ] **Step 1: Write the failing tests**

```go
// graph/parse_test.go
func TestWikiLinkTargetKeepsHash(t *testing.T) {
	links := ExtractWikiLinks("- [[C#]] and [[F#|fsharp]]\n")
	if len(links) != 2 || links[0].Target != "C#" || links[1].Target != "F#" {
		t.Fatalf("links = %+v, want C# and F#", links)
	}
}
```

```go
// render/page_test.go
func TestRenderLinkTargetKeepsHash(t *testing.T) {
	res, err := Render("- [[C#]]\n", 80)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Links) != 1 || res.Links[0].Target != "C#" {
		t.Fatalf("links = %+v, want target C#", res.Links)
	}
}
```

- [ ] **Step 2: Run to verify failure** — expected: FAIL (targets `C`/`F`).

- [ ] **Step 3: Implement** — delete the fragment strip in both files. In `parse.go` the empty-target guard (`raw == ""`) stays — it now catches only genuinely empty `[[ ]]`. In `render`, `[[#anchor]]` previously became empty-target (left literal); after the change it's a normal (likely dangling) link named `#anchor` — acceptable and consistent with graph. Remove the two `// Strip optional #block fragment` comment blocks; update any existing tests that pinned the stripping behavior (they pinned the divergence this task removes).

- [ ] **Step 4: Run** — `go test ./internal/graph/ ./internal/render/ ./internal/views/ -v` — expected: PASS, including the cross-package fence/link alignment test.

- [ ] **Step 5: Commit**

```bash
git add internal/graph/ internal/render/
git commit -m "fix(graph,render): keep # in wiki-link targets — Logseq has no fragment syntax"
```

### Task 7: Hide markdown-link URLs on lines containing backticks

`render/page.go` (`hideMarkdownLinkURLsOutsideInlineCode`) splits the line on backticks *before* matching, so a link whose text contains a code span (`[the ` + backtick + `go` + backtick + ` docs](url)`) is severed and its URL stays visible. Replace the split with span-filtering: match on the whole line, skip only matches *fully inside* a code span (literal examples), rewrite the rest.

**Files:**
- Modify: `internal/graph/refs.go` (export `inlineCodeSpans` → `InlineCodeSpans`; update its callers in `refs.go` and `linkify.go`)
- Modify: `internal/render/page.go` (`hideMarkdownLinkURLsOutsideInlineCode`)
- Test: `internal/render/page_test.go`

**Interfaces:**
- Produces: `graph.InlineCodeSpans(line string) []search.Span` — the existing unexported helper, exported verbatim (keep its unpaired-trailing-backtick semantics exactly; render must agree with parse). Render gains imports of `graph` (already present) and `search`.

- [ ] **Step 1: Write the failing tests**

```go
func TestHideMarkdownLinkURLsWithCodeSpanInText(t *testing.T) {
	got := hideMarkdownLinkURLs("see [the `go` docs](https://go.dev/doc) now\n")
	want := "see [the `go` docs](#) now\n"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestHideMarkdownLinkURLsLeavesLiteralExampleInCode(t *testing.T) {
	in := "type `[x](http://y)` literally\n"
	if got := hideMarkdownLinkURLs(in); got != in {
		t.Fatalf("literal example inside inline code was rewritten: %q", got)
	}
}
```

- [ ] **Step 2: Run to verify failure** — expected: first FAILs (URL kept), second already passes (guards the rewrite).

- [ ] **Step 3: Implement**

In graph: rename `inlineCodeSpans` to `InlineCodeSpans` (exported, same body, doc comment noting it's shared with render), update the two internal callers.

In render:

```go
// hideMarkdownLinkURLsOutsideInlineCode applies the [text](url) -> [text](#)
// rewrite to a single line. A match FULLY inside a backtick code span is a
// literal syntax example and stays untouched; a link whose text merely
// contains a code span is still a real link and gets rewritten. Uses
// graph.InlineCodeSpans so render and parse agree on what counts as code.
func hideMarkdownLinkURLsOutsideInlineCode(line string) string {
	locs := markdownLinkRe.FindAllStringSubmatchIndex(line, -1)
	if locs == nil {
		return line
	}
	code := graph.InlineCodeSpans(line)
	inCode := func(start, end int) bool {
		for _, s := range code {
			if start >= s.Start && end <= s.End {
				return true
			}
		}
		return false
	}
	var b strings.Builder
	last := 0
	for _, m := range locs {
		start, end := m[0], m[1]
		if inCode(start, end) || line[m[2]:m[3]] == "!" {
			continue // literal example, or an image — leave verbatim
		}
		b.WriteString(line[last:start])
		b.WriteString("[" + line[m[4]:m[5]] + "](#)")
		last = end
	}
	b.WriteString(line[last:])
	return b.String()
}
```

(`m[2]:m[3]` is the `(!?)` group, `m[4]:m[5]` the link text — same groups as the existing regex.)

- [ ] **Step 4: Run** — `go test ./internal/render/ ./internal/graph/ -v` — expected: PASS, including the existing balanced-paren and image tests.

- [ ] **Step 5: Commit**

```bash
git add internal/graph/ internal/render/
git commit -m "fix(render): hide markdown-link URLs on lines with backticks via span filtering"
```

### Phase 1 gate

- [ ] `go test -race ./...` — all PASS.

---

## Phase 2 — quality refactors

### Task 8: Extract `pushHistory` (three verbatim copies)

`navigateToTask`, `navigateFocusingLink`, `navigateHighlighting` (`app.go:205-267` area) each repeat the capture-current/truncate-forward/push block; `historyBack`/`historyForward` repeat the capture step. Pure refactor under green.

**Files:**
- Modify: `internal/views/app.go`

- [ ] **Step 1: Confirm green baseline** — `go test ./internal/views/` — PASS.

- [ ] **Step 2: Implement**

```go
// pushHistory captures the departing page's position into the current
// history entry, truncates any forward history, and pushes a fresh
// entry for name (which becomes current). The invariant every navigation
// relies on: the page being left always has its offset/cursor saved.
func (a *App) pushHistory(name string, taskOrdinal int) {
	if a.histIdx >= 0 && a.histIdx < len(a.hist) {
		a.hist[a.histIdx].offset = a.page.Offset()
		a.hist[a.histIdx].cursor = a.page.Cursor()
	}
	a.hist = append(a.hist[:a.histIdx+1], historyEntry{
		page:        name,
		offset:      0,
		cursor:      -1,
		taskOrdinal: taskOrdinal,
	})
	a.histIdx = len(a.hist) - 1
}
```

The three `navigate*` functions become preamble-free, e.g.:

```go
func (a *App) navigateToTask(name string, ordinal int) {
	a.pushHistory(name, ordinal)
	a.page.SetPage(name)
	if ordinal >= 0 {
		a.page.ScrollToTask(ordinal)
	}
}
```

(`navigateFocusingLink` keeps its trailing `a.hist[a.histIdx].cursor = a.page.Cursor()`; `navigateHighlighting` calls `SetPageEmphasizing` instead of `SetPage`. `historyBack`/`historyForward` keep their inline capture — they don't push.)

- [ ] **Step 3: Run** — `go test ./internal/views/ -v` — expected: PASS, especially `app_history_test.go` in full.

- [ ] **Step 4: Commit**

```bash
git add internal/views/
git commit -m "refactor(views): extract pushHistory from the navigate* family"
```

### Task 9: SearchView adopts `textinput` + `consumeKey` (like Picker)

`SearchView` hand-rolls query editing (raw string + rune-aware backspace, `search.go` `Update`); Picker already solved this with `textinput.Model` + `consumeKey`. Converge so the next input quirk is fixed once.

**Files:**
- Modify: `internal/views/search.go` (struct, `NewSearchView`, `Update`, `Query`/`SetQuery`, the View's prompt line)
- Test: existing `internal/views/search_test.go` (behavior-preserving; tests updated only where they poke the removed field)

- [ ] **Step 1: Confirm green baseline** — `go test ./internal/views/` — PASS.

- [ ] **Step 2: Implement** — read `picker.go`'s `textinput` setup and `consumeKey` first, then:

- Replace `query string` with `input textinput.Model`, initialized in `NewSearchView` exactly as Picker does (focused, no prompt styling).
- `Query()`/`SetQuery` delegate to `s.input.Value()`/`SetValue` (they're documented test hooks — keep them).
- In `Update`, replace the three hand-rolled editing cases (`keyBackspace`, `" "`/`keySpace`, single-rune default) with one:

```go
	default:
		before := s.input.Value()
		ti, handled := consumeKey(s.input, key)
		if !handled {
			return OverlayResult{}
		}
		s.input = ti
		if s.input.Value() != before {
			// Query changed: previous results and any in-flight search
			// no longer apply (gen guard drops late arrivals).
			s.gen++
			s.hits = nil
			s.searched = false
			s.running = false
			s.err = nil
		}
```

- `SearchCmd` reads `s.input.Value()` instead of `s.query`; the View's prompt renders `s.input.Value()` (keep the existing clamp).

- [ ] **Step 3: Run** — `go test ./internal/views/ -v` — expected: PASS. The generation tests from PR #31 (`TestApplyDropsResultsForEditedQuery` etc.) are the safety net here; update only constructions that set `s.query` directly (use `SetQuery`).

- [ ] **Step 4: Commit**

```bash
git add internal/views/
git commit -m "refactor(search): route query editing through textinput + consumeKey like Picker"
```

### Task 10: Sentinel constants as visible `\u` escapes (chore)

The sentinel delimiters (`render/page.go` consts, U+E000–E007) and `sentinelDigit0` (U+E010) are committed as invisible literal PUA runes — valid Go, but unreadable and fragile to copy/edit.

**Files:**
- Modify: `internal/render/page.go` (the sentinel const block and `sentinelDigit0`)

- [ ] **Step 1: Record current values** — add a temporary print or run:

```bash
python3 -c "
import re
src = open('internal/render/page.go', encoding='utf-8').read()
for m in re.finditer(r'\b(\w*[Ss]entinel\w*)\s*=\s*.(.).', src):
    print(m.group(1), hex(ord(m.group(2))))
"
```

Note each const's codepoint.

- [ ] **Step 2: Rewrite each const with its escape** — e.g.:

```go
const (
	wikiSentinelStart = "\ue000"
	wikiSentinelEnd   = "\ue001"
	wikiSentinelPad   = "\ue002"
	taskSentinelStart = "\ue003"
	taskSentinelEnd   = "\ue004"
	emphSentinelStart = "\ue005"
	emphSentinelEnd   = "\ue006"
	emphSentinelPad   = "\ue007"
)

const sentinelDigit0 = '\ue010'
```

**Use the codepoints recorded in Step 1, not this table, if they differ.** Keep all doc comments.

- [ ] **Step 3: Run** — `go test ./internal/render/ ./internal/views/ -v` — expected: PASS (sentinel round-trip tests pin the values; any failure means a transcription slip — recheck Step 1).

- [ ] **Step 4: Commit**

```bash
git add internal/render/
git commit -m "chore(render): write sentinel PUA constants as visible unicode escapes"
```

### Phase 2 gate

- [ ] `go test -race ./...` — all PASS.

---

## Phase 3 — test infrastructure + tooling

### Task 11: Cover `editCurrent`'s defensive branches

Untested branches in `internal/views/app.go` `editCurrent` / `editorExitedMsg`: (a) file deleted between index and `e` → stub recreated; (b) editor-resolution failure → hint; (c) `editorExitedMsg` with unchanged mtime → probe only, **no** reindex (the index-invalidation gate).

**Files:**
- Test: `internal/views/app_dispatch_test.go` (fixture `testdata/fake-editor.sh` already exists)

- [ ] **Step 1: Write the tests**

```go
func TestEditCurrentRecreatesDeletedFile(t *testing.T) {
	// arrange — indexed page whose file vanished before E
	quietTerm(t)
	dir, _ := writeGraph(t, map[string]string{"pages/A.md": "- x\n"})
	a := New(dir, "test")
	a.Update(a.Init()())
	a.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	a.navigate("A")
	fake, err := filepath.Abs("../../testdata/fake-editor.sh")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("VISUAL", fake)
	path := filepath.Join(dir, "pages", "A.md")
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}

	// act — editCurrent must recreate the stub before handing off
	cmd := a.editCurrent()

	// assert
	if cmd == nil {
		t.Fatalf("editCurrent returned nil cmd (hint: %q)", a.hint)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("deleted file was not recreated: %v", err)
	}
}

func TestEditCurrentSurfacesEditorResolutionFailure(t *testing.T) {
	// arrange — no resolvable editor anywhere in the chain
	quietTerm(t)
	dir, _ := writeGraph(t, map[string]string{"pages/A.md": "- x\n"})
	a := New(dir, "test")
	a.Update(a.Init()())
	a.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	a.navigate("A")
	t.Setenv("VISUAL", "/nonexistent/editor-binary")
	t.Setenv("EDITOR", "/nonexistent/editor-binary")
	t.Setenv("PATH", t.TempDir()) // hides /usr/bin/vi from LookPath's fallback

	// act
	cmd := a.editCurrent()
	if cmd != nil {
		a.Update(cmd()) // land the hint msg, matching the package idiom
	}

	// assert
	if !strings.Contains(a.hint, "cannot resolve editor") {
		t.Fatalf("hint = %q, want editor-resolution failure", a.hint)
	}
}
```

Note: `edit.Resolve` falls back to the absolute path `/usr/bin/vi`, which `LookPath` finds regardless of PATH on a machine with vi installed. If the second test can't force failure portably, skip it with `t.Skip` when `/usr/bin/vi` exists and say so in the test comment — don't fake it.

```go
func TestEditorExitUnchangedMtimeSkipsReindex(t *testing.T) {
	// arrange
	quietTerm(t)
	dir, _ := writeGraph(t, map[string]string{"pages/A.md": "- x\n"})
	a := New(dir, "test")
	a.Update(a.Init()())
	a.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	a.navigate("A")
	path := filepath.Join(dir, "pages", "A.md")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	genBefore := a.indexGen

	// act — editor exited without touching the file
	a.Update(editorExitedMsg{path: path, t0: info.ModTime()})

	// assert — no reindex was scheduled (indexGen is bumped
	// synchronously inside buildIndexCmd, so it's the observable)
	if a.indexGen != genBefore {
		t.Fatal("unchanged-mtime editor exit triggered a reindex")
	}
}
```

- [ ] **Step 2: Run** — `go test ./internal/views/ -run 'TestEditCurrent|TestEditorExitUnchanged' -v` — expected: PASS (these pin existing behavior; a FAIL is a real bug — stop and fix minimally first).

- [ ] **Step 3: Commit**

```bash
git add internal/views/
git commit -m "test(views): cover editCurrent defensive branches and the unchanged-mtime reindex gate"
```

### Task 12: View tests boot on throwaway graphs, not the checked-in fixture

`bootApp`/`bootAppAt` (`app_test.go:25,51`) point at `testdata/fixture-graph`; tests that create journals rely on `withJournalCleanup` to un-write the repo, a crashed run leaves stray files, and nothing can `t.Parallel()`. Copy the fixture into a temp dir per test instead — content-identical, zero cleanup.

**Files:**
- Modify: `internal/views/helpers_test.go` (add `cloneFixtureGraph`), `internal/views/app_test.go` (`bootApp`/`bootAppAt` use it)
- Modify: remaining direct `fixture-graph` references (`edit_test.go` ×2, `page_test.go` ×1, `app_dispatch_test.go` ×2, `search_test.go` ×1 — re-grep at execution time) and delete `withJournalCleanup` once unused

- [ ] **Step 1: Add the helper**

```go
// cloneFixtureGraph copies testdata/fixture-graph into a fresh temp dir
// so tests can create journals and edit pages without dirtying the repo
// checkout (and can run in parallel).
func cloneFixtureGraph(t *testing.T) string {
	t.Helper()
	src, err := filepath.Abs("../../testdata/fixture-graph")
	if err != nil {
		t.Fatal(err)
	}
	dst := t.TempDir()
	err = filepath.WalkDir(src, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		out := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(out, 0o755)
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(out, b, 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
	return dst
}
```

- [ ] **Step 2: Migrate** — `bootApp`/`bootAppAt` replace the `filepath.Abs("../../testdata/fixture-graph")` with `cloneFixtureGraph(t)`. Re-grep `fixture-graph` across `internal/views/*_test.go` and convert each remaining direct use the same way. Delete `withJournalCleanup` and its call sites (the temp dir makes it moot).

- [ ] **Step 3: Run** — `go test ./internal/views/ -count=1 -v | tail -20` — expected: PASS; then verify the repo tree is untouched: `git status --porcelain` after the run prints nothing under `testdata/`.

- [ ] **Step 4: Commit**

```bash
git add internal/views/
git commit -m "test(views): boot tests on cloned throwaway graphs; drop withJournalCleanup"
```

### Task 13: staticcheck — fix current findings, add to CI

Current inventory (staticcheck v0.7.0 at PR #31's head): four SA1019 deprecated viewport calls in `internal/views/page.go:164-167` and one SA4004 in `internal/views/search_test.go:307` (a range loop whose body unconditionally `break`s).

**Files:**
- Modify: `internal/views/page.go:164-167`, `internal/views/search_test.go` (~`:300-308`, `TestHitLabelKnownVsUnknown`), `.forgejo/workflows/ci.yml`

- [ ] **Step 1: Fix SA1019** — replace the deprecated viewport calls with their designated successors (named in each deprecation notice):

```go
func (p *PageView) LineDown()     { p.vp.ScrollDown(1) }
func (p *PageView) LineUp()       { p.vp.ScrollUp(1) }
func (p *PageView) HalfPageDown() { p.vp.HalfPageDown() }
func (p *PageView) HalfPageUp()   { p.vp.HalfPageUp() }
```

- [ ] **Step 2: Fix SA4004** — in `TestHitLabelKnownVsUnknown`, the `for _, p := range idx.Pages { …; break }` loop runs once by construction; say what it means:

```go
	if len(idx.Pages) == 0 {
		t.Fatal("fixture graph has no pages")
	}
	p := idx.Pages[0]
	if got := s.hitLabel(p.Path); got != p.Name {
		t.Errorf("known path %q: want %q, got %q", p.Path, p.Name, got)
	}
```

- [ ] **Step 3: Verify clean** — `go run honnef.co/go/tools/cmd/staticcheck@v0.7.0 ./...` — expected: no output, exit 0. Then `go test ./internal/views/ -v` — PASS (scroll behavior unchanged; the page-view scroll tests pin it).

- [ ] **Step 4: Add to CI** — in `.forgejo/workflows/ci.yml`, extend the Verify step:

```yaml
      - name: Verify
        run: |
          test -z "$(gofmt -l .)"
          go vet ./...
          go run honnef.co/go/tools/cmd/staticcheck@v0.7.0 ./...
          go test -race ./...
          go build ./...
```

(Pinned version so CI can't drift; bump deliberately.)

- [ ] **Step 5: Commit**

```bash
git add internal/views/ .forgejo/workflows/
git commit -m "chore: fix staticcheck findings and gate CI on staticcheck v0.7.0"
```

### Phase 3 gate (final)

- [ ] `go test -race ./...`, `go vet ./...`, `test -z "$(gofmt -l .)"`, `go run honnef.co/go/tools/cmd/staticcheck@v0.7.0 ./...`, `go build ./...` — all clean.
- [ ] Manual smoke on the real graph: ctrl+c quits from inside the picker; R keeps your place on a long page; a `:LOGBOOK:` block containing a fence line renders clean.

---

## Deliberately NOT in this plan

- **`%3F` filename decoding** — Logseq desktop is retired (2026-06); nothing writes percent-encoded page filenames anymore, and weft-created pages never did.
- **`alias::` support in weft** — the property is Logseq legacy and weft's design line keeps one linking primitive. Instead, the graph itself is migrated: every `[[peekseq]]` becomes `[[weft|peekseq]]` (display text unchanged, resolves natively) and the `alias:: peekseq` line on `pages/weft.md` is deleted — a one-off supervised rewrite done outside this plan. weft gains no alias mechanism.
- **`replaceWikiLinksOutsideInlineCode`'s backtick-split parity** — its split semantics deliberately mirror `graph.parseBody`; changing one without the other breaks the parse/render alignment that PR #31 established.
- **Overlay `OverlayResult` flags-as-tagged-union widening** — watch, don't fix (per the review's architecture notes).
