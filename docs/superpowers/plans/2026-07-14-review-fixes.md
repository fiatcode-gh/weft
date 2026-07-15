# Codebase-Review Fixes Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking. Every task follows TDD: failing test → red → minimal fix → green → commit.

**Goal:** Fix the verified bugs from the 2026-07-14 whole-codebase review — two data-loss paths, two user-reported navigation/reindex bugs, the fence/markdown-link/emphasis render defects, backlink case-folding, and a batch of views-layer state bugs — plus the small quality extractions that make those fixes land in one place.

**Architecture:** weft is a Bubble Tea TUI over a Logseq-compatible markdown graph. Packages: `internal/graph` (parse/index/resolve/linkify/refs), `internal/render` (Glamour-based page renderer with PUA sentinel substitution), `internal/views` (TUI: app model, page view, editor, overlays), `internal/edit` (the only disk-writing surface), `internal/sync` (git commit→pull --rebase→push), `internal/search` (ripgrep JSON). Fixes are grouped in four phases ordered by risk: data safety → graph/render correctness → views behavior → hygiene/tests.

**Tech Stack:** Go 1.x, bubbletea/bubbles/lipgloss, glamour, ripgrep. Tests: standard `go test`, existing helpers `writeGraph`/`quietTerm`/`bootApp` (views), `git`/`newRepoWithRemote`/`cloneSibling` (sync).

## Global Constraints

- Read `AGENTS.md` at the repo root before starting; its conventions are binding.
- TDD every task: write the failing test first, watch it fail, then fix. Test bodies use `// arrange` / `// act` / `// assert` comments; shared setup goes in helpers, not copy-paste.
- Conventional Commits: `fix:`, `feat:`, `refactor:`, `test:`, `chore:` — one commit per task.
- Before EVERY commit: `go test ./... && go vet ./... && test -z "$(gofmt -l .)"` must all pass.
- Do not add new third-party dependencies.
- weft is a *navigator, not an outliner* — no scope creep into outliner features.
- Verification of each phase: full suite `go test -race ./...` at phase end.

---

## Phase 1 — Data safety (edit + sync)

### Task 1: `EnsureFile` must not truncate a concurrently-created file (TOCTOU)

`internal/edit/editor.go:58-73` does `os.Stat` then `os.WriteFile(path, nil, 0o644)` (O_TRUNC). If the file appears between the two calls (e.g. `git pull --rebase` materializes today's journal written on another machine), it is wiped to zero bytes and the empty file is pushed everywhere on next sync.

**Files:**
- Modify: `internal/edit/editor.go:58-73`
- Test: `internal/edit/editor_test.go`

**Interfaces:**
- Produces: `EnsureFile(path string) (created bool, err error)` — same signature and contract as today; only the implementation becomes create-with-`O_EXCL`.

- [ ] **Step 1: Write the failing-by-inspection characterization tests**

The race itself can't be reproduced in a unit test; instead pin the three contract behaviors the new implementation must keep, including the one the fix is about (existing content is NEVER touched):

```go
func TestEnsureFileNeverTruncatesExisting(t *testing.T) {
	// arrange
	dir := t.TempDir()
	path := filepath.Join(dir, "journal.md")
	if err := os.WriteFile(path, []byte("- precious\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// act
	created, err := EnsureFile(path)

	// assert
	if err != nil || created {
		t.Fatalf("EnsureFile = (%v, %v), want (false, nil)", created, err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "- precious\n" {
		t.Fatalf("existing content changed: %q", got)
	}
}
```

Check `editor_test.go` for existing `EnsureFile` tests covering create-new and is-a-directory; keep them, add this one only if not already pinned.

- [ ] **Step 2: Run the test — it should PASS today (stat path). It guards the rewrite.**

Run: `go test ./internal/edit/ -run TestEnsureFile -v`

- [ ] **Step 3: Replace stat-then-write with `O_CREATE|O_EXCL`**

```go
// EnsureFile creates an empty file at path with mode 0o644 if it does
// not exist. Returns (true, nil) on create, (false, nil) if the file
// already existed, or (false, err) for any other failure (including the
// case where path resolves to a directory). Creation uses O_EXCL so a
// file that appears between check and create (e.g. a concurrent git
// pull materializing today's journal) is never truncated.
// This is the create-today-journal hook.
func EnsureFile(path string) (created bool, err error) {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err == nil {
		return true, f.Close()
	}
	if !os.IsExist(err) {
		return false, err
	}
	info, statErr := os.Stat(path)
	if statErr != nil {
		return false, statErr
	}
	if info.IsDir() {
		return false, fmt.Errorf("ensure %s: is a directory", path)
	}
	return false, nil
}
```

- [ ] **Step 4: Run the package tests**

Run: `go test ./internal/edit/ -v` — expected: all PASS (including the existing create/dir cases).

- [ ] **Step 5: Commit**

```bash
git add internal/edit/
git commit -m "fix(edit): create journal stub with O_EXCL so a concurrent pull can't be truncated"
```

### Task 2: Block graph-mutating keys while an async sync is running

`sync.Run` rewrites the worktree (`add -A` → `commit` → `pull --rebase` → `push`) in a background goroutine while the TUI stays interactive (`app.go:708-718`). A save landing mid-rebase is silently lost; `add -A` can stage editor swap/temp files. The `syncing` flag must also gate the write entry points: `e` (in-app editor), `E` ($EDITOR), `.` (journal create), and linkify.

**Files:**
- Modify: `internal/views/app.go` (key dispatch cases `keyE`, `keyShiftE`, `"."` and the `res.Linkify` overlay branch at ~`app.go:634`)
- Test: `internal/views/app_sync_test.go`

**Interfaces:**
- Produces: `func (a *App) syncBusyHint() tea.Cmd` — returns a non-nil hint cmd when `a.syncing`, nil otherwise. Later tasks don't consume it; it's internal.

Note on completeness: `S` is unreachable while either editor is open (in-app editor swallows keys; `tea.ExecProcess` suspends the TUI for $EDITOR), so gating these four entry points gives full mutual exclusion between writes and sync.

- [ ] **Step 1: Write the failing tests**

```go
func TestWriteKeysBlockedWhileSyncing(t *testing.T) {
	for _, key := range []string{"e", "E", "."} {
		t.Run(key, func(t *testing.T) {
			// arrange
			a := bootApp(t)
			a.syncing = true

			// act
			a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)})

			// assert
			if a.editor != nil {
				t.Fatal("editor opened during sync")
			}
			if !strings.Contains(a.hint, "sync in progress") {
				t.Fatalf("hint = %q, want sync-in-progress hint", a.hint)
			}
		})
	}
}
```

Note: `a.Update(...)` returns `(tea.Model, tea.Cmd)`; the hint is set via the returned cmd in some paths — follow the pattern of existing hint assertions in `app_status_test.go` (drive the returned cmd's msg through `a.Update` if needed to land the hint, as those tests do).

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/views/ -run TestWriteKeysBlockedWhileSyncing -v`
Expected: FAIL — for "e" the editor opens; for "." a journal is created.

- [ ] **Step 3: Implement the gate**

Add near `logSyncFailure` in `app.go`:

```go
// syncBusyHint gates the graph-mutating entry points while the async git
// sync goroutine is rewriting the worktree: a save landing mid
// `pull --rebase` is overwritten by the rebase checkout and silently lost,
// and `add -A` can stage editor temp files. Non-nil means "blocked".
func (a *App) syncBusyHint() tea.Cmd {
	if !a.syncing {
		return nil
	}
	return a.setHint("⟳ sync in progress — retry when it finishes")
}
```

Wire into the four dispatch sites:

```go
case keyE:
	if cmd := a.syncBusyHint(); cmd != nil {
		return a, cmd
	}
	return a, a.enterEditor()
case keyShiftE:
	if cmd := a.syncBusyHint(); cmd != nil {
		return a, cmd
	}
	return a, a.editCurrent()
case ".":
	if cmd := a.syncBusyHint(); cmd != nil {
		return a, cmd
	}
	// ... existing "." body unchanged
```

and in the overlay branch:

```go
if res.Linkify != nil {
	if cmd := a.syncBusyHint(); cmd != nil {
		return a, cmd
	}
	return a, a.linkify(res.Linkify, res.LinkifyTarget)
}
```

- [ ] **Step 4: Run the tests**

Run: `go test ./internal/views/ -run 'TestWriteKeys|TestSync' -v` then the full package. Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/views/
git commit -m "fix(views): block edit/journal/linkify while git sync is rewriting the worktree"
```

### Task 3: Give `sync.Status` the same timeout `sync.Run` has

`internal/sync/status.go:25-30` uses plain `exec.Command`. The status probe fires after every save/reindex/sync; on a hung git (stale sshfs mount, fsmonitor hook) each probe leaks a goroutine + two child processes forever.

**Files:**
- Modify: `internal/sync/status.go`
- Test: `internal/sync/sync_test.go` (reuse `TestRunTimesOutOnSlowGit`'s approach at `sync_test.go:164`)

- [ ] **Step 1: Write the failing test** — copy the structure of `TestRunTimesOutOnSlowGit` (fake slow `git` on PATH + shortened `syncTimeout`), but call `Status`:

```go
func TestStatusTimesOutOnSlowGit(t *testing.T) {
	// arrange — same slow-git PATH shim TestRunTimesOutOnSlowGit uses
	// (read that test and reuse its setup verbatim; extract a shared
	// helper if it isn't one already)
	...
	old := syncTimeout
	syncTimeout = 100 * time.Millisecond
	t.Cleanup(func() { syncTimeout = old })

	// act
	start := time.Now()
	_, err := Status(t.TempDir())

	// assert
	if err == nil {
		t.Fatal("want timeout error, got nil")
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("Status blocked %v; timeout did not apply", elapsed)
	}
}
```

- [ ] **Step 2: Run to verify failure** — `go test ./internal/sync/ -run TestStatusTimesOut -v` — expected: FAIL (blocks until the shim exits or hangs).

- [ ] **Step 3: Implement** — mirror `Run`'s runner in `Status`:

```go
run := func(args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), syncTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = repoDir
	out, err := cmd.CombinedOutput()
	return string(out), err
}
```

(add `context` and `time` imports as needed).

- [ ] **Step 4: Run** — `go test ./internal/sync/ -v` — expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/sync/
git commit -m "fix(sync): bound Status git calls with the sync timeout"
```

### Phase 1 gate

- [ ] Run `go test -race ./...` — all PASS.

---

## Phase 2 — graph + render correctness

### Task 4: Key the backlink index case-insensitively

`internal/graph/index.go:98` keys `Backlinks` by the raw as-written target while resolution is fold-keyed — `[[alpha]]` → page `Alpha` navigates but never appears in Alpha's backlinks panel (and `refs.go:85` also hides it from unlinked refs, so the mention vanishes entirely).

**Files:**
- Modify: `internal/graph/index.go:98`, `internal/views/backlinks.go:39`
- Test: `internal/graph/index_test.go`

**Interfaces:**
- Produces: `Index.Backlinks` keys are now **lowercase-folded** page names. All lookups must fold. Verify consumers first: `grep -rn 'Backlinks\[' internal/` — expected hits only `index.go:98` and `backlinks.go:39`; if more appear, fold those too.

- [ ] **Step 1: Write the failing test**

```go
func TestBacklinksAreCaseInsensitive(t *testing.T) {
	// arrange
	dir := t.TempDir()
	pages := filepath.Join(dir, "pages")
	if err := os.MkdirAll(pages, 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(name, body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(pages, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("Alpha.md", "- the target page\n")
	write("Note.md", "- see [[alpha]] here\n")

	// act
	idx, err := BuildIndex(dir)
	if err != nil {
		t.Fatal(err)
	}

	// assert — lookup by the canonical on-disk name must find the
	// case-variant link
	refs := idx.Backlinks[strings.ToLower("Alpha")]
	if len(refs) != 1 || refs[0].FromPage != "Note" {
		t.Fatalf("Backlinks[alpha] = %+v, want one ref from Note", refs)
	}
}
```

- [ ] **Step 2: Run to verify failure** — `go test ./internal/graph/ -run TestBacklinksAreCaseInsensitive -v` — expected: FAIL (refs live under key `"alpha"` only when written lowercase; here the link IS lowercase and the canonical name is `Alpha`, so the folded lookup finds nothing).

- [ ] **Step 3: Implement**

`index.go:98`:

```go
idx.Backlinks[strings.ToLower(lh.Target)] = append(idx.Backlinks[strings.ToLower(lh.Target)], Ref{
```

(hoist `key := strings.ToLower(lh.Target)` to keep it readable). Document the folded key on the `Backlinks` field comment in `types.go`/`index.go`:

```go
Backlinks  map[string][]Ref // keyed by strings.ToLower(target) — resolution is case-insensitive
```

`internal/views/backlinks.go:39`:

```go
src := idx.Backlinks[strings.ToLower(target)]
```

- [ ] **Step 4: Run** — `go test ./internal/graph/ ./internal/views/ -v` — expected: PASS (fix any existing test that asserted raw-cased keys — update those assertions to folded keys; that is the behavior change, not a regression).

- [ ] **Step 5: Commit**

```bash
git add internal/graph/ internal/views/
git commit -m "fix(graph): fold backlink keys so case-variant links appear in the backlinks panel"
```

### Task 5: Extract `mapLinesOutsideFences` in render (pure refactor)

`render/page.go` hand-rolls the identical fence-walking loop six times. Extract the transform-outside-fences shape (4 call sites) so the fence-grammar fix in Task 6/7 lands once. No behavior change.

**Files:**
- Modify: `internal/render/page.go` (`hideMarkdownLinkURLs:329`, `preprocessWikiLinks:296`, `preprocessTaskMarkers:375`, `preprocessEmphasis:475`)
- Test: existing `internal/render/page_test.go` (no new tests — refactor under green)

**Interfaces:**
- Produces: `func mapLinesOutsideFences(body string, f func(line string) string) string` (unexported). Task 7 modifies its internals.

- [ ] **Step 1: Confirm green baseline** — `go test ./internal/render/` — PASS.

- [ ] **Step 2: Add the helper and rewrite the four walkers**

```go
// mapLinesOutsideFences rewrites body line by line: lines inside (or
// delimiting) a fenced code block pass through verbatim; every other
// line goes through f. The shared walker keeps the fence grammar in one
// place for all preprocessing passes.
func mapLinesOutsideFences(body string, f func(line string) string) string {
	var out strings.Builder
	out.Grow(len(body))
	lines := strings.Split(body, "\n")
	inFence := false
	for i, line := range lines {
		switch {
		case fenceRe.MatchString(line):
			inFence = !inFence
			out.WriteString(line)
		case inFence:
			out.WriteString(line)
		default:
			out.WriteString(f(line))
		}
		if i < len(lines)-1 {
			out.WriteByte('\n')
		}
	}
	return out.String()
}
```

The four passes become:

```go
func hideMarkdownLinkURLs(body string) string {
	return mapLinesOutsideFences(body, hideMarkdownLinkURLsOutsideInlineCode)
}

func preprocessWikiLinks(body string) (string, []linkSubst) {
	var subs []linkSubst
	body = mapLinesOutsideFences(body, func(line string) string {
		return replaceWikiLinksOutsideInlineCode(line, &subs)
	})
	return body, subs
}

func preprocessTaskMarkers(body string) (string, []taskInfo) {
	var markers []taskInfo
	body = mapLinesOutsideFences(body, func(line string) string {
		// existing taskMarkerRe body, unchanged, operating on one line
		m := taskMarkerRe.FindStringSubmatch(line)
		if m == nil {
			return line
		}
		prefix, marker := m[1], m[2]
		id := len(markers)
		rest := line[len(prefix)+len(marker):]
		open := openTaskMarkers[marker] && len(rest) > 0 && (rest[0] == ' ' || rest[0] == '\t') && strings.TrimSpace(rest) != ""
		markers = append(markers, taskInfo{marker: marker, open: open})
		return prefix + fmt.Sprintf("%s%d%s", taskSentinelStart, id, taskSentinelEnd) + rest
	})
	return body, markers
}

func preprocessEmphasis(body, term string) (string, []string) {
	if term == "" {
		return body, nil
	}
	re := regexp.MustCompile(`(?i)` + regexp.QuoteMeta(term))
	var subs []string
	body = mapLinesOutsideFences(body, func(line string) string {
		return emphasizeOutsideInlineCode(line, re, &subs)
	})
	return body, subs
}
```

Keep the two block-strippers (`stripLogbookBlocks`, `stripQueryAndEmbedBlocks`) as they are — different shape (they drop lines).

- [ ] **Step 3: Run** — `go test ./internal/render/ -v` — expected: PASS, byte-identical behavior. Also run `go test ./internal/views/` (golden renders).

- [ ] **Step 4: Commit**

```bash
git add internal/render/
git commit -m "refactor(render): extract mapLinesOutsideFences shared line walker"
```

### Task 6: Fence grammar — `graph.FenceState` handles tilde and bullet-prefixed fences

Two confirmed bugs share this root: (a) `~~~` fences bypass every guard (`fenceRe` is backticks-only in graph/render/tint); (b) a bullet-prefixed opener ``- ``` `` is missed while its indented closer matches — inverting fence state and poisoning the rest of the page's index.

**Files:**
- Create: `internal/graph/fence.go`
- Create: `internal/graph/fence_test.go`
- Modify: `internal/graph/parse.go:35-57` (`parseBody`), `internal/graph/refs.go:65-79` (`fencedLines`)

**Interfaces:**
- Produces (Task 7 consumes from render/views):
  - `type FenceState struct{ ... }` — zero value = outside any fence.
  - `func (f *FenceState) Step(line string) bool` — consume one line; true when the line is fenced content or a fence delimiter.
  - `var FenceDelimiterRe *regexp.Regexp` — matches a fence delimiter line (for stateless per-line classification in the editor tint).

- [ ] **Step 1: Write the failing tests**

```go
func TestParseBodySkipsTildeFences(t *testing.T) {
	// arrange / act
	links := ExtractWikiLinks("~~~\ncode [[InsideCode]]\n~~~\n- after [[Real]]\n")

	// assert
	if len(links) != 1 || links[0].Target != "Real" {
		t.Fatalf("links = %+v, want only Real", links)
	}
}

func TestParseBodyHandlesBulletPrefixedFences(t *testing.T) {
	// arrange / act — Logseq puts fences inside bullets; the indented
	// closer must not INVERT state and swallow the rest of the page
	links := ExtractWikiLinks("- ```\n  code [[InsideCode]]\n  ```\n- after [[Real]]\n")

	// assert
	if len(links) != 1 || links[0].Target != "Real" {
		t.Fatalf("links = %+v, want only Real", links)
	}
}

func TestFenceStateMixedMarkers(t *testing.T) {
	// a ``` line inside an open ~~~ fence is content, not a toggle
	var f FenceState
	steps := []struct {
		line string
		want bool
	}{
		{"~~~", true},
		{"```", true}, // content of the tilde fence
		{"still code", true},
		{"~~~", true}, // closes
		{"- plain [[X]]", false},
	}
	for i, s := range steps {
		if got := f.Step(s.line); got != s.want {
			t.Fatalf("step %d (%q) = %v, want %v", i, s.line, got, s.want)
		}
	}
}
```

- [ ] **Step 2: Run to verify failure** — `go test ./internal/graph/ -run 'TestParseBody|TestFenceState' -v` — expected: FAIL (tilde test returns InsideCode+drops Real; bullet test returns InsideCode and drops Real; FenceState doesn't exist → compile error first).

- [ ] **Step 3: Implement `internal/graph/fence.go`**

```go
package graph

import "regexp"

// FenceDelimiterRe matches a code-fence delimiter line: optional
// indentation, an optional list-bullet prefix (Logseq nests fences
// inside bullets), then a backtick or tilde fence marker. Exported so
// the editor tint (internal/views) classifies delimiter rows with the
// same grammar the index and renderer use.
var FenceDelimiterRe = regexp.MustCompile("^\\s*(?:[-*+]\\s+)?(```|~~~)")

// FenceState tracks fenced-code state across a top-to-bottom line walk.
// The zero value means "outside any fence". A fence opened with
// backticks is closed only by backticks (tildes only by tildes),
// matching CommonMark: the other marker inside an open fence is content.
type FenceState struct {
	open byte // '`' or '~'; 0 = outside a fence
}

// Step consumes one line and reports whether that line belongs to
// fenced code — either as content or as a fence delimiter itself.
func (f *FenceState) Step(line string) bool {
	m := FenceDelimiterRe.FindStringSubmatch(line)
	if m == nil {
		return f.open != 0
	}
	marker := m[1][0]
	switch {
	case f.open == 0:
		f.open = marker
	case f.open == marker:
		f.open = 0
	}
	return true
}
```

Rewrite `parseBody` (`parse.go`) to use it and delete graph's `fenceRe`:

```go
func parseBody(body string) (lines []string, links []LinkHit, todos []TodoHit) {
	lines = strings.Split(body, "\n")
	var fence FenceState
	for i, line := range lines {
		if fence.Step(line) {
			continue
		}
		links = appendWikiLinks(links, line, i+1)
		if m := todoRe.FindStringSubmatch(line); m != nil {
			todos = append(todos, TodoHit{
				Marker:   m[1],
				Priority: m[2],
				Text:     m[3],
				Line:     i + 1,
			})
		}
	}
	return lines, links, todos
}
```

Rewrite `fencedLines` (`refs.go`):

```go
func fencedLines(body string) map[int]bool {
	fenced := map[int]bool{}
	var fence FenceState
	for i, line := range strings.Split(body, "\n") {
		if fence.Step(line) {
			fenced[i+1] = true
		}
	}
	return fenced
}
```

- [ ] **Step 4: Run** — `go test ./internal/graph/ -v` — expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/graph/
git commit -m "fix(graph): FenceState handles tilde and bullet-prefixed fences; adopt in parse and refs"
```

### Task 7: Adopt `graph.FenceState` in render and editor tint; add cross-package alignment test

Render and the editor tint still use backtick-only `fenceRe`. Swap both onto the graph grammar so index, read view, and editor agree on what is fenced. Adding a `render → graph` import is a clean downward edge (no cycle: graph does not import render).

**Files:**
- Modify: `internal/render/page.go` (delete `fenceRe:49`; `mapLinesOutsideFences`, `stripLogbookBlocks:169`, `stripQueryAndEmbedBlocks:204`)
- Modify: `internal/views/edit_tint.go:19` (`editorFenceRe`)
- Test: `internal/render/page_test.go`

- [ ] **Step 1: Write the failing tests**

```go
func TestRenderSkipsTildeAndBulletFences(t *testing.T) {
	for name, body := range map[string]string{
		"tilde":  "~~~\nsee [[Foo]] here\n~~~\n",
		"bullet": "- ```\n  see [[Foo]] here\n  ```\n",
	} {
		t.Run(name, func(t *testing.T) {
			res, err := Render(body, 80)
			if err != nil {
				t.Fatal(err)
			}
			if len(res.Links) != 0 {
				t.Fatalf("fenced [[Foo]] became a link: %+v", res.Links)
			}
		})
	}
}

// Alignment: the same corpus must be fenced identically for the index
// (graph.ExtractWikiLinks) and the read view (Render). This is the
// guard against the two grammars drifting again.
func TestFenceGrammarAlignsWithGraph(t *testing.T) {
	corpus := "- ```\n  [[A]]\n  ```\n~~~\n[[B]]\n~~~\n- [[C]] `[[D]]` text\n"
	res, err := Render(corpus, 80)
	if err != nil {
		t.Fatal(err)
	}
	var rendered []string
	for _, l := range res.Links {
		rendered = append(rendered, l.Target)
	}
	var indexed []string
	for _, h := range graph.ExtractWikiLinks(corpus) {
		indexed = append(indexed, h.Target)
	}
	if !reflect.DeepEqual(rendered, indexed) {
		t.Fatalf("render links %v != graph links %v", rendered, indexed)
	}
}
```

(add imports `reflect` and `git.fiatcode.dev/fiatcode/weft/v2/internal/graph` to page_test.go).

- [ ] **Step 2: Run to verify failure** — `go test ./internal/render/ -run 'TestRenderSkips|TestFenceGrammar' -v` — expected: FAIL (tilde/bullet fenced links render as links).

- [ ] **Step 3: Implement**

In `page.go`, delete `fenceRe` and rewrite the three walkers on `graph.FenceState`:

```go
func mapLinesOutsideFences(body string, f func(line string) string) string {
	var out strings.Builder
	out.Grow(len(body))
	lines := strings.Split(body, "\n")
	var fence graph.FenceState
	for i, line := range lines {
		if fence.Step(line) {
			out.WriteString(line)
		} else {
			out.WriteString(f(line))
		}
		if i < len(lines)-1 {
			out.WriteByte('\n')
		}
	}
	return out.String()
}
```

In `stripLogbookBlocks` and `stripQueryAndEmbedBlocks`, replace the `case fenceRe.MatchString(line): inFence = !inFence; out.WriteString(line)` + `case inFence:` pair with a `var fence graph.FenceState` and a first case `case fence.Step(line): out.WriteString(line)` (keep the logbook/query cases after it, order preserved).

In `edit_tint.go`, delete `editorFenceRe` and use the exported regex in `lineBaseStyle`:

```go
case graph.FenceDelimiterRe.MatchString(line):
	return lipgloss.NewStyle().Faint(true), true
```

(`internal/views` already imports `graph`.)

- [ ] **Step 4: Run** — `go test ./internal/render/ ./internal/views/ ./internal/graph/ -v` — expected: PASS. If any golden view test breaks because tilde/bullet fences now render literally — that's the intended fix; update the golden after reading the diff.

- [ ] **Step 5: Commit**

```bash
git add internal/render/ internal/views/
git commit -m "fix(render,views): share graph fence grammar; tilde and bullet fences respected everywhere"
```

### Task 8: Markdown links with parenthesized URLs (regression from #30)

`markdownLinkRe` (`render/page.go:48`) stops the URL at the first `)`, truncating Wikipedia-style URLs and leaving a stray `)` in the read view.

**Files:**
- Modify: `internal/render/page.go:48`
- Test: `internal/render/page_test.go`

- [ ] **Step 1: Write the failing test**

```go
func TestHideMarkdownLinkURLsBalancedParens(t *testing.T) {
	got := hideMarkdownLinkURLs("see [Go](https://en.wikipedia.org/wiki/Go_(programming_language)) now\n")
	want := "see [Go](#) now\n"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}
```

- [ ] **Step 2: Run to verify failure** — `go test ./internal/render/ -run TestHideMarkdownLinkURLsBalanced -v` — expected: FAIL with `"see [Go](#)) now\n"`.

- [ ] **Step 3: Implement** — allow one level of balanced parens in the URL (CommonMark-compatible for the realistic cases; titles and spaces still permitted):

```go
markdownLinkRe = regexp.MustCompile(`(!?)\[([^\]]*)\]\(((?:[^()]|\([^()]*\))*)\)`)
```

- [ ] **Step 4: Run** — `go test ./internal/render/ -v` — expected: PASS (existing markdown-link tests from #30 must stay green).

- [ ] **Step 5: Commit**

```bash
git add internal/render/
git commit -m "fix(render): markdown-link regex tolerates balanced parens in URLs"
```

### Task 9: Sentinel ids must not be ASCII digits (numeric search corrupts links)

`RenderWithEmphasis(body, w, "0")` matches the ASCII-digit id *inside* a wiki/task sentinel (PUA delimiters count as word boundaries), destroying the link and leaking raw PUA bytes. Encode ids in PUA digits (U+E010–U+E019) so no user-visible term can ever match inside a sentinel.

**Files:**
- Modify: `internal/render/page.go` (`sentinelRe:106-110`, id writes at `:274`, `:402`, `:535`, id reads at `:607`, `:622`, `:632`)
- Test: `internal/render/page_test.go`

**Interfaces:**
- Produces (render-internal): `encodeSentinelID(id int) string`, `decodeSentinelID(s string) (int, bool)`.

- [ ] **Step 1: Verify U+E010–E019 are unused** — Run: `grep -n $'\uE01' internal/render/page.go` — expected: no output. If any hit, shift the base to a free PUA range and adjust the code below.

- [ ] **Step 2: Write the failing test**

```go
func TestRenderWithEmphasisNumericTermKeepsSentinels(t *testing.T) {
	// arrange / act — searching a bare number must not corrupt the
	// digit-encoded ids inside wiki/task sentinels
	res, err := RenderWithEmphasis("- TODO check [[Foo]] version 0\n", 80, "0")

	// assert
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Links) != 1 || res.Links[0].Target != "Foo" {
		t.Fatalf("links = %+v, want [[Foo]] intact", res.Links)
	}
	if len(res.Tasks) != 1 {
		t.Fatalf("tasks = %v, want 1", res.Tasks)
	}
	if len(res.Finds) != 1 {
		t.Fatalf("finds = %v, want exactly the bare 0", res.Finds)
	}
	for _, r := range res.Styled {
		if r >= '\ue000' && r <= '\uf8ff' {
			t.Fatalf("raw PUA rune %U leaked into styled output", r)
		}
	}
}
```

- [ ] **Step 3: Run to verify failure** — `go test ./internal/render/ -run TestRenderWithEmphasisNumericTerm -v` — expected: FAIL (0 links, leaked PUA bytes).

- [ ] **Step 4: Implement**

```go
// sentinelDigit0 is the first of ten PUA runes (U+E010..U+E019) that
// encode the decimal digits of sentinel ids. Ids must not be ASCII
// digits: a numeric emphasis term would match inside a sentinel (the
// PUA delimiters are word boundaries) and corrupt it.
const sentinelDigit0 = '\ue010'

func encodeSentinelID(id int) string {
	var b strings.Builder
	for _, r := range strconv.Itoa(id) {
		b.WriteRune(sentinelDigit0 + (r - '0'))
	}
	return b.String()
}

func decodeSentinelID(s string) (int, bool) {
	var b strings.Builder
	for _, r := range s {
		if r < sentinelDigit0 || r > sentinelDigit0+9 {
			return 0, false
		}
		b.WriteRune('0' + (r - sentinelDigit0))
	}
	id, err := strconv.Atoi(b.String())
	return id, err == nil
}
```

`sentinelRe`: replace each `(\d+)` with `([\x{E010}-\x{E019}]+)` (three places).

Id writes — replace the `fmt.Sprintf("%s%d%s", …)` at the three sites:

```go
core := wikiSentinelStart + encodeSentinelID(id) + wikiSentinelEnd          // :274
sentinel := taskSentinelStart + encodeSentinelID(id) + taskSentinelEnd      // :402
core := emphSentinelStart + encodeSentinelID(id) + emphSentinelEnd          // :535
```

Id reads — replace `strconv.Atoi(styled[m[2]:m[3]])` (and the `m[4]`/`m[6]` twins):

```go
id, ok := decodeSentinelID(styled[m[2]:m[3]])
if !ok || id >= len(wikiSubs) {
	out.WriteString(styled[m[0]:m[1]])
	continue
}
```

- [ ] **Step 5: Run** — `go test ./internal/render/ ./internal/views/ -v` — expected: PASS (sentinel padding math is unchanged: PUA digits are width-1 like ASCII digits).

- [ ] **Step 6: Commit**

```bash
git add internal/render/
git commit -m "fix(render): encode sentinel ids as PUA digits so numeric search terms can't corrupt links"
```

### Task 10: Unlinked-ref detection and linkify must skip inline code

`refs.go:84` (`firstUnlinkedMatch`) and `linkify.go:62` (`firstUnlinkedOccurrence`) scan whole lines; parse/render skip odd backtick segments. Accepting an "unlinked ref" inside `` `alpha deploy` `` writes a dead literal link into the user's code span.

**Files:**
- Modify: `internal/graph/refs.go`, `internal/graph/linkify.go`
- Test: `internal/graph/refs_test.go`, `internal/graph/linkify_test.go`

**Interfaces:**
- Produces (graph-internal): `inlineCodeSpans(line string) []search.Span` in `refs.go`, consumed by both files.

- [ ] **Step 1: Write the failing tests**

```go
func TestFirstUnlinkedMatchSkipsInlineCode(t *testing.T) {
	// arrange — the only match sits inside `alpha deploy`
	line := "- run `alpha deploy` now"
	matches := []search.Span{{Start: strings.Index(line, "alpha"), End: strings.Index(line, "alpha") + 5}}

	// act
	_, ok := firstUnlinkedMatch(line, matches)

	// assert
	if ok {
		t.Fatal("match inside inline code offered as unlinked ref")
	}
}

func TestLinkifyMentionSkipsInlineCode(t *testing.T) {
	// arrange — first occurrence is in code, second is bare
	body := "- run `alpha deploy` then alpha again\n"

	// act
	got, span, err := LinkifyMention(body, 1, "alpha")

	// assert — the BARE mention gets wrapped, the code span is untouched
	if err != nil {
		t.Fatal(err)
	}
	want := "- run `alpha deploy` then [[alpha]] again\n"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	_ = span
}

func TestLinkifyMentionOnlyCodeOccurrenceIsNotFound(t *testing.T) {
	_, _, err := LinkifyMention("- run `alpha deploy` now\n", 1, "alpha")
	if !errors.Is(err, ErrMentionNotFound) {
		t.Fatalf("err = %v, want ErrMentionNotFound", err)
	}
}
```

- [ ] **Step 2: Run to verify failure** — `go test ./internal/graph/ -run 'InlineCode|OnlyCode' -v` — expected: FAIL (code-span occurrences are matched today).

- [ ] **Step 3: Implement**

In `refs.go`, add:

```go
// inlineCodeSpans returns the byte ranges (inclusive of the backticks)
// of inline code spans on line — the odd segments of a backtick split,
// mirroring how parse and render treat backticks. An unpaired trailing
// backtick opens no span.
func inlineCodeSpans(line string) []search.Span {
	var spans []search.Span
	start := -1
	for i := 0; i < len(line); i++ {
		if line[i] != '`' {
			continue
		}
		if start < 0 {
			start = i
		} else {
			spans = append(spans, search.Span{Start: start, End: i + 1})
			start = -1
		}
	}
	return spans
}

func spanInside(m search.Span, spans []search.Span) bool {
	for _, s := range spans {
		if m.Start >= s.Start && m.End <= s.End {
			return true
		}
	}
	return false
}
```

In `firstUnlinkedMatch`, compute `code := inlineCodeSpans(line)` once and skip `m` when `spanInside(m, code)` (alongside the existing inside-`[[…]]` check). In `firstUnlinkedOccurrence` (`linkify.go`), same: skip `loc` when `spanInside(search.Span{Start: start, End: end}, inlineCodeSpans(line))`.

- [ ] **Step 4: Run** — `go test ./internal/graph/ -v` — expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/graph/
git commit -m "fix(graph): unlinked-ref detection and linkify skip inline code spans"
```

### Phase 2 gate

- [ ] Run `go test -race ./...` — all PASS.

---

## Phase 3 — views behavior

### Task 11: Navigation must reset the scroll position (user-reported)

`PageView.SetPage` (`views/page.go:63-68`) never resets `vp.YOffset`; bubbles' `SetContent` preserves it. Navigating from a deep-scrolled page to another long page lands mid-page. History restore (`Restore`) must keep working (it sets an explicit offset after `SetPage`).

**Files:**
- Modify: `internal/views/page.go:63-79`
- Test: `internal/views/page_test.go`

- [ ] **Step 1: Write the failing test**

```go
func TestSetPageResetsScroll(t *testing.T) {
	// arrange — two pages long enough to scroll at height 10
	quietTerm(t)
	long := strings.Repeat("- line\n", 60)
	_, idx := writeGraph(t, map[string]string{
		"pages/A.md": long,
		"pages/B.md": long,
	})
	p := NewPageView(idx, "A", 80, 10)
	for i := 0; i < 20; i++ {
		p.LineDown()
	}
	if p.Offset() == 0 {
		t.Fatal("precondition: expected page A to be scrolled")
	}

	// act
	p.SetPage("B")

	// assert
	if p.Offset() != 0 {
		t.Fatalf("Offset after SetPage = %d, want 0", p.Offset())
	}
}
```

- [ ] **Step 2: Run to verify failure** — `go test ./internal/views/ -run TestSetPageResetsScroll -v` — expected: FAIL (offset carried over).

- [ ] **Step 3: Implement**

```go
// SetPage switches to a different page in the same index. The viewport
// starts at the top — a fresh navigation must not inherit the previous
// page's scroll position (history Restore sets an explicit offset after
// this when walking [ / ]).
func (p *PageView) SetPage(name string) {
	p.page = name
	p.cursor = -1
	p.emphasis = ""
	p.load()
	p.vp.GotoTop()
}
```

And in `SetPageEmphasizing`, add `p.vp.GotoTop()` after `p.load()` (before `p.scrollToFirstFind()`), so a no-finds arrival also starts at the top.

- [ ] **Step 4: Run** — `go test ./internal/views/ -v` — expected: PASS, including all history tests (`app_history_test.go` — `Restore` runs after `SetPage` and still lands on the stored offset).

- [ ] **Step 5: Commit**

```bash
git add internal/views/
git commit -m "fix(views): navigation resets scroll to top instead of inheriting previous page offset"
```

### Task 12: Reindex — drop stale results, give R feedback (user-reported)

`indexLoadedMsg` has no generation tag: a sync-pull reindex and an `R` reindex can be in flight together and an older disk walk delivered later overwrites the newer index. Separately, `R` is silent (and silently swallowed when an overlay is open), so a reindex that raced a write looks like "R didn't work".

**Files:**
- Modify: `internal/views/app.go` (`indexLoadedMsg:21`, `buildIndexCmd:153`, handler `:496`, `R` case `:719`)
- Test: `internal/views/app_test.go`

**Interfaces:**
- Produces: `indexLoadedMsg` gains `gen int`; `App` gains `indexGen int`. `buildIndexCmd` increments and stamps the generation.

- [ ] **Step 1: Write the failing test**

```go
func TestStaleIndexLoadedMsgIsDropped(t *testing.T) {
	// arrange — two reindexes in flight; the OLDER walk delivers LAST
	a := bootApp(t)
	oldCmd := a.buildIndexCmd()
	oldMsg := oldCmd().(indexLoadedMsg)
	newCmd := a.buildIndexCmd()
	newMsg := newCmd().(indexLoadedMsg)

	// act
	a.Update(newMsg)
	current := a.idx
	a.Update(oldMsg) // stale delivery

	// assert
	if a.idx != current {
		t.Fatal("stale indexLoadedMsg replaced the newer index")
	}
}
```

- [ ] **Step 2: Run to verify failure** — `go test ./internal/views/ -run TestStaleIndexLoadedMsg -v` — expected: FAIL (compile error on `gen` first, then old msg replaces idx).

- [ ] **Step 3: Implement**

```go
// indexLoadedMsg carries the result of an asynchronous graph.BuildIndex
// run. gen identifies which buildIndexCmd produced it; the handler drops
// results from any generation but the latest, so a slow walk delivered
// late can't overwrite a newer index.
type indexLoadedMsg struct {
	idx *graph.Index
	err error
	gen int
}

func (a *App) buildIndexCmd() tea.Cmd {
	a.indexGen++
	gen := a.indexGen
	path := a.graphPath
	return func() tea.Msg {
		idx, err := graph.BuildIndex(path)
		return indexLoadedMsg{idx: idx, err: err, gen: gen}
	}
}
```

Add `indexGen int` to the `App` struct. Handler, first lines of `case indexLoadedMsg:`:

```go
case indexLoadedMsg:
	if m.gen != a.indexGen {
		return a, nil // a newer reindex is in flight; drop the stale walk
	}
```

`R` feedback (`:719`):

```go
case "R":
	// Async reindex — the response lands as indexLoadedMsg and rebuilds
	// PageView for the current page. The hint makes a swallowed or
	// racing R visible instead of looking like a silent no-op.
	return a, tea.Batch(a.setHint("⟳ reindexing…"), a.buildIndexCmd())
```

- [ ] **Step 4: Run** — `go test ./internal/views/ -v` — expected: PASS (boot-path tests drive `Init`'s cmd synchronously; gen 1 matches).

- [ ] **Step 5: Commit**

```bash
git add internal/views/
git commit -m "fix(views): generation-tag reindex results and announce R so stale walks can't win"
```

### Task 13: Render cache must be keyed by width (stale render after resize)

`views/page.go:270` caches per page keyed only by mtime; a width change calls `load()` which serves the old-width render byte-for-byte.

**Files:**
- Modify: `internal/views/page.go:40-43, 270, 295`
- Test: `internal/views/page_test.go`

- [ ] **Step 1: Write the failing test**

```go
func TestResizeRerendersAtNewWidth(t *testing.T) {
	// arrange
	quietTerm(t)
	_, idx := writeGraph(t, map[string]string{
		"pages/A.md": "- " + strings.Repeat("word ", 40) + "\n",
	})
	p := NewPageView(idx, "A", 100, 24)
	before := p.result.Styled

	// act
	p.SetSize(40, 24)

	// assert — content must be re-wrapped, not served from the
	// width-100 cache entry
	if p.result.Styled == before {
		t.Fatal("resize served the stale width-100 render")
	}
	for _, line := range strings.Split(p.result.Styled, "\n") {
		if w := lipgloss.Width(line); w > 40 {
			t.Fatalf("line wider than viewport after resize: %d cells", w)
		}
	}
}
```

- [ ] **Step 2: Run to verify failure** — `go test ./internal/views/ -run TestResizeRerenders -v` — expected: FAIL (styled identical).

- [ ] **Step 3: Implement**

```go
type cachedPage struct {
	result  render.Result
	modTime time.Time
	width   int
}
```

In `load()`: cache hit requires `c.width == p.width`:

```go
if c, hit := p.cache[meta.Name]; hit && c.modTime.Equal(meta.ModTime) && c.width == p.width {
```

and store it:

```go
p.cache[meta.Name] = cachedPage{result: res, modTime: meta.ModTime, width: p.width}
```

- [ ] **Step 4: Run** — `go test ./internal/views/ -v` — expected: PASS (the `RenderCount` cache tests still pass: unchanged-width reloads still hit).

- [ ] **Step 5: Commit**

```bash
git add internal/views/
git commit -m "fix(views): key page render cache by width so resize re-wraps"
```

### Task 14: Editor resize must reposition the textarea viewport

`editor.go:206-213` pokes `repositionMsg` only when the completer is active — a plain terminal shrink leaves the cursor line off-screen until the next keystroke (the known bubbles/textarea reposition landmine, half-fixed).

**Files:**
- Modify: `internal/views/editor.go:199-214` (`layout()`)
- Test: `internal/views/editor_view_test.go`

- [ ] **Step 1: Write the failing test** — follow the existing completer-reposition test's technique in `editor_view_test.go` (build an editor, interleave `View()` calls so the textarea viewport is primed — required for scrolling to work in tests):

```go
func TestResizeKeepsCursorVisible(t *testing.T) {
	// arrange — 60-line buffer, cursor moved to the last line, tall window
	quietTerm(t)
	e := newTestEditor(t, strings.Repeat("line\n", 59)+"last-line") // use/adapt the file's existing constructor helper
	e.SetSize(80, 40)
	_ = e.View()
	for i := 0; i < 60; i++ {
		e.Update(tea.KeyMsg{Type: tea.KeyDown})
	}
	_ = e.View()

	// act — shrink hard; no keypress afterwards
	e.SetSize(80, 8)

	// assert — the cursor's line must be inside the rendered window
	if !strings.Contains(e.View(), "last-line") {
		t.Fatal("cursor line scrolled out of view after resize")
	}
}
```

(Adapt helper/constructor names to what `editor_view_test.go` actually uses — read the file first; the assertion pattern above is the contract.)

- [ ] **Step 2: Run to verify failure** — `go test ./internal/views/ -run TestResizeKeepsCursorVisible -v` — expected: FAIL (view shows lines 0–7).

- [ ] **Step 3: Implement** — ungate the poke in `layout()`:

```go
	// SetHeight/SetWidth never reposition the textarea viewport (bubbles
	// quirk: repositioning happens only inside Update), so after ANY
	// resize the cursor line can sit outside the visible window until the
	// next keystroke. Poke Update with a content-neutral message to force
	// a reposition; it's a no-op when the cursor is already visible.
	e.ta, _ = e.ta.Update(repositionMsg{})
```

(delete the `if e.completer.active` wrapper; keep the comment accurate.)

- [ ] **Step 4: Run** — `go test ./internal/views/ -v` — expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/views/
git commit -m "fix(editor): reposition textarea viewport on every resize, not only when completing"
```

### Task 15: Search — drop stale results, allow retry after error

`searchDoneMsg` is untagged: results for an edited-away query (or a previous SearchView instance) land under the current prompt (`search.go:65`, `app.go:540`). And after an rg error `searched=true` makes enter a permanent no-op.

**Files:**
- Modify: `internal/views/search.go` (`SearchCmd:49`, `searchDoneMsg:60`, `Apply:65`, `Update` query-edit cases `:105-124`)
- Modify: `internal/views/app.go` (`searchDoneMsg` handler ~`:540`)
- Test: `internal/views/search_test.go`

**Interfaces:**
- Produces: `searchDoneMsg` gains `view *SearchView` and `gen int`. `SearchView` gains `gen int` (bumped on every query mutation). App handler applies only when `m.view` is the active overlay.

- [ ] **Step 1: Write the failing tests**

```go
func TestApplyDropsResultsForEditedQuery(t *testing.T) {
	// arrange — search "foo" in flight, then the query is edited
	s := newTestSearchView(t) // adapt to the file's existing constructor helper
	s.query = "foo"
	cmd := s.SearchCmd("/nonexistent-graph")
	s.running = true
	s.Update("x") // query is now "foox"; gen bumped

	// act — the stale "foo" result arrives
	msg := cmd().(searchDoneMsg)
	s.Apply(msg)

	// assert
	if s.searched {
		t.Fatal("stale result marked the edited query as searched")
	}
	if s.hits != nil {
		t.Fatalf("stale hits installed: %+v", s.hits)
	}
}

func TestEnterRetriesAfterSearchError(t *testing.T) {
	// arrange — a failed search must not dead-end the query
	s := newTestSearchView(t)
	s.query = "foo"
	s.running = true
	s.Apply(searchDoneMsg{view: s, gen: s.gen, err: errors.New("rg failed")})

	// act
	res := s.Update(keyEnter)

	// assert — enter re-runs instead of no-op
	if res.Cmd == nil {
		t.Fatal("enter after error did not retry the search")
	}
}
```

- [ ] **Step 2: Run to verify failure** — `go test ./internal/views/ -run 'TestApplyDrops|TestEnterRetries' -v` — expected: FAIL (compile error on new fields first).

- [ ] **Step 3: Implement**

```go
type searchDoneMsg struct {
	view *SearchView // which overlay instance ran the search
	gen  int         // s.gen at launch; stale generations are dropped
	hits []search.Hit
	err  error
}

func (s *SearchView) SearchCmd(graphPath string) tea.Cmd {
	q := s.query
	gen := s.gen
	return func() tea.Msg {
		hits, err := search.Run(graphPath, q)
		return searchDoneMsg{view: s, gen: gen, hits: hits, err: err}
	}
}

// Apply installs a finished search's results. Results from an edited-away
// query (stale gen) are dropped. A failed search leaves searched=false so
// enter retries instead of dead-ending the query.
func (s *SearchView) Apply(msg searchDoneMsg) {
	if msg.gen != s.gen {
		return
	}
	s.running = false
	s.searched = msg.err == nil
	s.err = msg.err
	s.hits = msg.hits
	if s.sel >= len(s.hits) {
		s.sel = 0
	}
}
```

Add `gen int` to the `SearchView` struct. In `Update`, every branch that mutates `s.query` (backspace, space, printable rune) also does `s.gen++` and `s.running = false` (a stale in-flight search no longer owns the view). In the app handler (`app.go` ~`:540`):

```go
case searchDoneMsg:
	if sv, ok := a.active.(*SearchView); ok && sv == m.view {
		sv.Apply(m)
	}
	return a, nil
```

- [ ] **Step 4: Run** — `go test ./internal/views/ -v` — expected: PASS (existing search tests updated to construct msgs with `view`/`gen` where they build them by hand).

- [ ] **Step 5: Commit**

```bash
git add internal/views/
git commit -m "fix(search): tag results with view+generation and allow retry after rg errors"
```

### Task 16: Completion strip rows must not wrap (editor overflows terminal)

`complete.go:199` clamps labels to `inner` but the row is `" ▶ " (3) + label` inside a box whose content area is `inner-2` — long labels wrap, `rows():171` under-reserves, and the editor view exceeds the terminal height.

**Files:**
- Modify: `internal/views/complete.go:199`
- Test: `internal/views/complete_test.go`

- [ ] **Step 1: Write the failing test**

```go
func TestStripHeightMatchesRowsBudget(t *testing.T) {
	// arrange — candidate longer than a 40-col strip can show unclamped
	quietTerm(t)
	c := newTestCompleter(t, []string{strings.Repeat("x", 34)}) // adapt to the file's helper for building an active completer
	// act / assert
	if got, want := lipgloss.Height(c.View(40)), c.rows(); got != want {
		t.Fatalf("strip renders %d rows but rows() reserved %d", got, want)
	}
}
```

(Adapt construction to `complete_test.go`'s existing helpers — the invariant `lipgloss.Height(View(w)) == rows()` is the contract; also add a case for the `＋ Create %q` row, which is ~11 cells longer than the name.)

- [ ] **Step 2: Run to verify failure** — `go test ./internal/views/ -run TestStripHeight -v` — expected: FAIL (7 vs 5 at width 40).

- [ ] **Step 3: Implement** — budget the marker (3) and the border padding (2 cells beyond the `Width(inner+2)` content area):

```go
label = clamp(label, inner-5)
```

(replacing `label = clamp(label, inner)` at `complete.go:199`; add a comment: `// box content is inner-2 (padding 1,2) and the marker takes 3 cells`).

- [ ] **Step 4: Run** — `go test ./internal/views/ -v` — expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/views/
git commit -m "fix(views): clamp completion-strip labels so rows never wrap past the reserved height"
```

### Task 17: Picker row budget must include the 3-cell marker

`picker.go:194` computes `nameBudget = inner - hintCol - 2` but each row is `marker(3) + name + 2 + hint` — full-budget names with long mtime hints ("11 months ago") wrap inside the border and grow the panel.

**Files:**
- Modify: `internal/views/picker.go:194`
- Test: `internal/views/picker_test.go`

- [ ] **Step 1: Write the failing test**

```go
func TestPickerRowsNeverWrap(t *testing.T) {
	// arrange — 70-char page name plus a long relative-time hint
	quietTerm(t)
	p := newTestPicker(t, []string{strings.Repeat("n", 70)}) // adapt to picker_test.go's construction helper; pin p.now so the mtime hint renders "11 months ago"
	short := newTestPicker(t, []string{"tiny"})

	// act / assert — panel height must not depend on name length
	if lipgloss.Height(p.View()) != lipgloss.Height(short.View()) {
		t.Fatal("long name+hint row wrapped and grew the picker")
	}
}
```

- [ ] **Step 2: Run to verify failure** — `go test ./internal/views/ -run TestPickerRowsNeverWrap -v` — expected: FAIL (one extra row).

- [ ] **Step 3: Implement**

```go
nameBudget := inner - hintCol - 2 - 3 // 3 = the " ▶ "/"   " marker cells every row carries
```

- [ ] **Step 4: Run** — `go test ./internal/views/ -v` — expected: PASS (update any golden picker layouts that legitimately shift by 3 cells).

- [ ] **Step 5: Commit**

```bash
git add internal/views/
git commit -m "fix(picker): include selection marker in row width budget so rows never wrap"
```

### Phase 3 gate

- [ ] Run `go test -race ./...` — all PASS.

---

## Phase 4 — silent failures, hygiene, high-value tests

### Task 18: `search.parseJSON` must surface scanner errors (silently dropped hits)

`search.go:91` never checks `sc.Err()`: one >1MB line in rg's output (a pasted log blob in any note) aborts the scan mid-stream and silently truncates search/mentions/linkify results.

**Files:**
- Modify: `internal/search/search.go` (`parseJSON:87`, `Run:36`, `Mentions:48`)
- Test: `internal/search/search_test.go`

**Interfaces:**
- Produces: `parseJSON(b []byte) ([]Hit, error)`; `Run`/`Mentions` signatures unchanged (they already return error).

- [ ] **Step 1: Write the failing test**

```go
func TestParseJSONSurfacesScannerOverflow(t *testing.T) {
	// arrange — one line over the 1MB scanner cap, then a valid match
	huge := `{"type":"match","data":{"path":{"text":"pages/A.md"},"lines":{"text":"` +
		strings.Repeat("x", 1100*1024) + `"},"line_number":1,"submatches":[]}}`
	valid := `{"type":"match","data":{"path":{"text":"pages/B.md"},"lines":{"text":"hit"},"line_number":2,"submatches":[{"start":0,"end":3}]}}`
	input := []byte(huge + "\n" + valid + "\n")

	// act
	_, err := parseJSON(input)

	// assert — truncation must be an error, not silently missing hits
	if err == nil {
		t.Fatal("scanner overflow was swallowed; hits after the long line are silently dropped")
	}
}
```

- [ ] **Step 2: Run to verify failure** — `go test ./internal/search/ -run TestParseJSONSurfaces -v` — expected: FAIL (compile error on second return first).

- [ ] **Step 3: Implement** — change the signature and return `sc.Err()`:

```go
func parseJSON(b []byte) ([]Hit, error) {
	...
	for sc.Scan() { ... }
	if err := sc.Err(); err != nil {
		return out, fmt.Errorf("scan rg output: %w", err)
	}
	return out, nil
}
```

Propagate in both callers:

```go
hits, perr := parseJSON(out)
if perr != nil {
	return nil, perr
}
return hits, nil
```

- [ ] **Step 4: Run** — `go test ./internal/search/ -v` — expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/search/
git commit -m "fix(search): surface scanner errors instead of silently truncating rg results"
```

### Task 19: Debug/sync log — fixed location, sane WEFT_DEBUG semantics

Two confirmed problems: (a) `weft.log` is written to the process cwd (`cmd/weft/main.go:24-29`, `app.go:822-833`) — launch weft from inside the graph and the next sync `git add -A` commits the log into the notes repo (the test suite already deposited `internal/views/weft.log`); (b) `WEFT_DEBUG=0` *enables* logging (`!= ""` check).

**Files:**
- Modify: `cmd/weft/main.go` (debug-log path + `WEFT_DEBUG` parse), `internal/views/app.go:822-833` (`logSyncFailure`) and the `"✗ ... see weft.log"` hint (`:527`)
- Create: `internal/views/logpath.go`
- Test: `cmd/weft/main_test.go`, `internal/views/app_sync_test.go`

**Interfaces:**
- Produces: `views.DebugLogPath() string` — `$XDG_CACHE_HOME`-anchored path, cwd fallback. `debugLogEnabled(v string) bool` in `cmd/weft`.

- [ ] **Step 1: Write the failing tests**

```go
// cmd/weft/main_test.go
func TestDebugLogEnabled(t *testing.T) {
	cases := map[string]bool{
		"": false, "0": false, "false": false, "FALSE": false, "no": false, "off": false,
		"1": true, "true": true, "yes": true, "weft.log": true,
	}
	for v, want := range cases {
		if got := debugLogEnabled(v); got != want {
			t.Errorf("debugLogEnabled(%q) = %v, want %v", v, got, want)
		}
	}
}
```

```go
// internal/views (logpath test)
func TestDebugLogPathIsOutsideCwd(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	p := DebugLogPath()
	if !filepath.IsAbs(p) {
		t.Fatalf("DebugLogPath() = %q, want absolute cache-dir path", p)
	}
	if filepath.Base(filepath.Dir(p)) != "weft" {
		t.Fatalf("DebugLogPath() = %q, want .../weft/weft.log", p)
	}
}
```

- [ ] **Step 2: Run to verify failure** — both fail to compile (functions don't exist).

- [ ] **Step 3: Implement**

`internal/views/logpath.go`:

```go
package views

import (
	"os"
	"path/filepath"
)

// DebugLogPath is where weft writes its debug and sync-failure log.
// Anchored to the user cache dir so a weft launched from inside the
// graph can't deposit weft.log where the next sync's `git add -A`
// would commit it into the notes repo. Falls back to the process cwd
// only when the cache dir is unavailable.
func DebugLogPath() string {
	dir, err := os.UserCacheDir()
	if err != nil {
		return "weft.log"
	}
	p := filepath.Join(dir, "weft")
	if err := os.MkdirAll(p, 0o755); err != nil {
		return "weft.log"
	}
	return filepath.Join(p, "weft.log")
}
```

`cmd/weft/main.go` — replace the `os.Getenv("WEFT_DEBUG") != ""` check with:

```go
// debugLogEnabled interprets WEFT_DEBUG: empty and conventional
// falsy values disable, anything else enables.
func debugLogEnabled(v string) bool {
	switch strings.ToLower(v) {
	case "", "0", "false", "no", "off":
		return false
	}
	return true
}
```

and point `tea.LogToFile` at `views.DebugLogPath()`.

`app.go` `logSyncFailure`: open `DebugLogPath()` instead of `"weft.log"`; update the failure hint (`:527`) to include the real location:

```go
a.setHint("✗ "+m.res.Stage+" failed — see "+DebugLogPath()),
```

Delete the stray `weft.log` / `internal/views/weft.log` artifacts from the working tree (they're gitignored; just remove them).

- [ ] **Step 4: Run** — `go test ./cmd/weft/ ./internal/views/ -v` — expected: PASS, and no `weft.log` appears in the repo tree after the run (`test ! -f internal/views/weft.log`).

- [ ] **Step 5: Commit**

```bash
git add cmd/weft/ internal/views/
git commit -m "fix: anchor weft.log to the user cache dir and make WEFT_DEBUG=0 mean off"
```

### Task 20: Integration test — a failed save must keep the editor (and buffer) alive

Highest-consequence untested flow (`app.go:603-609`): `edit.WriteFile` fails → editor stays open with the buffer intact and the error surfaced. Test-only task.

**Files:**
- Test: `internal/views/app_dispatch_test.go`

- [ ] **Step 1: Write the test**

```go
func TestSaveFailureKeepsEditorAndBuffer(t *testing.T) {
	// arrange — app on a throwaway graph; open the in-app editor and type
	quietTerm(t)
	dir, _ := writeGraph(t, map[string]string{"pages/A.md": "- start\n"})
	a := New(dir, "test")
	msg := a.Init()()
	a.Update(msg)
	a.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	a.navigate("A")
	a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("e")})
	if a.editor == nil {
		t.Fatal("precondition: editor did not open")
	}
	a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	content := a.editor.Content()

	// make the save fail: page dir becomes read-only
	pages := filepath.Join(dir, "pages")
	if err := os.Chmod(pages, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(pages, 0o755) })

	// act — Ctrl+S
	a.Update(tea.KeyMsg{Type: tea.KeyCtrlS})

	// assert — editor survives, buffer intact, error surfaced
	if a.editor == nil {
		t.Fatal("failed save tore down the editor (buffer lost)")
	}
	if a.editor.Content() != content {
		t.Fatalf("buffer changed across failed save: %q", a.editor.Content())
	}
	if a.editor.errMsg == "" { // adapt to the actual error-field name in EditorView (see SetError)
		t.Fatal("save error not surfaced to the editor")
	}
}
```

(Adapt the error-field assertion to whatever `EditorView.SetError` sets — read `editor.go` first. Skip the test when running as root (`os.Geteuid() == 0`), where 0o555 doesn't block writes.)

- [ ] **Step 2: Run** — `go test ./internal/views/ -run TestSaveFailureKeeps -v` — expected: PASS (this pins existing correct behavior; if it FAILS, that's a real bug — stop and fix `app.go:603-609` minimally, then re-run).

- [ ] **Step 3: Commit**

```bash
git add internal/views/
git commit -m "test(views): pin editor survival and buffer preservation across a failed save"
```

### Task 21: Integration test — push rejection reports `Stage="push"`, `Committed=true`

Per the repo's own `weft.log`, rejected pushes are the most common real sync failure, and the only `sync.Run` stage without a test.

**Files:**
- Test: `internal/sync/sync_test.go` (reuse `git`, `newRepoWithRemote`, `cloneSibling` helpers)

- [ ] **Step 1: Write the test**

```go
func TestRunPushRejectionReportsPushStage(t *testing.T) {
	// arrange — sibling clone advances the remote; our pull --rebase
	// succeeds, but the remote moves AGAIN before our push lands
	work := newRepoWithRemote(t)
	sibling := cloneSibling(t, work)

	// local change to commit
	if err := os.WriteFile(filepath.Join(work, "pages", "A.md"), []byte("- local\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// make push rejected: install a pre-receive hook on the bare remote
	// that always refuses (simplest deterministic rejection)
	remote := strings.TrimSpace(git(t, work, "remote", "get-url", "origin"))
	hook := filepath.Join(remote, "hooks", "pre-receive")
	if err := os.WriteFile(hook, []byte("#!/bin/sh\necho rejected\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	_ = sibling

	// act
	res := Run(work, time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC))

	// assert
	if res.Err == nil || res.Stage != "push" {
		t.Fatalf("Stage = %q, Err = %v; want push failure", res.Stage, res.Err)
	}
	if !res.Committed {
		t.Fatal("local commit must be recorded even when push fails")
	}
	if res.Pushed {
		t.Fatal("Pushed must be false on rejection")
	}
}
```

(Adapt paths to how `newRepoWithRemote` lays out the work/remote dirs — read the helper first; drop the `sibling` clone if the hook alone suffices.)

- [ ] **Step 2: Run** — `go test ./internal/sync/ -run TestRunPushRejection -v` — expected: PASS (pins existing behavior at `sync.go:102-105`; a FAIL means a real bug — stop and fix minimally).

- [ ] **Step 3: Commit**

```bash
git add internal/sync/
git commit -m "test(sync): pin push-rejection result shape (Stage=push, Committed=true)"
```

### Task 22: Hygiene — gofmt the tree, add fmt gate to CI, drop dead code

**Files:**
- Modify: `internal/views/editor_view_test.go` (gofmt), `.forgejo/workflows/ci.yml`
- Delete: `graph.IsJournalFilename` (`internal/graph/resolve.go:25-27`) + its test (`resolve_test.go:22-36`)
- Modify: `internal/graph/refs.go:25` + `internal/views/app.go:450` (drop `FilterUnlinked`'s unused `target` param)

- [ ] **Step 1: gofmt** — Run: `gofmt -w internal/views/editor_view_test.go && test -z "$(gofmt -l .)"`

- [ ] **Step 2: CI gate** — in `.forgejo/workflows/ci.yml`, make the Verify step:

```yaml
      - name: Verify
        run: |
          test -z "$(gofmt -l .)"
          go vet ./...
          go test -race ./...
          go build ./...
```

- [ ] **Step 3: Dead code** — delete `IsJournalFilename` and its test; change `FilterUnlinked(hits []search.Hit, target, targetPath string, read ...)` to `FilterUnlinked(hits []search.Hit, targetPath string, read ...)` and update the single caller (`app.go:450`) plus any tests. Run `go build ./...` to catch stragglers.

- [ ] **Step 4: Run** — `go test ./... && go vet ./... && test -z "$(gofmt -l .)"` — all PASS. Also remove the stray empty `ls/` directory at the repo root if present (`rmdir ls`).

- [ ] **Step 5: Commit**

```bash
git add -A
git commit -m "chore: gofmt gate in CI, format editor_view_test, drop dead IsJournalFilename and unused param"
```

### Phase 4 gate (final)

- [ ] `go test -race ./...` — all PASS.
- [ ] `go vet ./... && test -z "$(gofmt -l .)"` — clean.
- [ ] Manual smoke (use the `verify` skill or run `weft` against a scratch graph): navigate deep-scrolled → new page opens at top; resize a page and the editor; `R` shows "⟳ reindexing…"; search a bare number on a link-bearing page — links stay intact.

---

## Backlog (reviewed, deliberately NOT in this plan)

Low-severity or scope-expanding items from the review, recorded so they aren't lost. Do not implement these without a separate decision:

- `ctrl+c` is dead while overlays are open (`app.go:627`) — inconsistent with Bubble Tea convention.
- Entering/leaving the in-app editor discards the read-view scroll position (`app.go:619`, `:513`).
- `[[C#]]` truncates at the first `#` (`parse.go:79`, `render/page.go:258`) — Logseq-compat divergence.
- Case-fold file collision (`Alpha.md` + `alpha.md`) breaks exact-name resolution for the loser (`index.go:69-77`); also untested collision determinism.
- CRLF `\r` leaks into search-row `Context` (`search.go:110` trims `\n` only; `lineContext` is the asymmetric sibling).
- Logbook/query strippers: a ``` line *inside* a `:LOGBOOK:` block flips fence state (`page.go:176-181` case order) — corrupted-file territory.
- Markdown links containing backticks never get their URL hidden (backtick split severs the match, `page.go:356`).
- `SearchView` hand-rolls text input — route through `textinput` + `consumeKey` like Picker.
- `navigate*` history-push preamble ×3 → `pushHistory(name)` helper (`app.go:196-252`).
- `alias::` page property is unhandled; Logseq `%3F`-style filename encoding is not decoded.
- `editCurrent` defensive branches untested (deleted-file recreate, editor-resolution failure, unchanged-mtime skip).
- Migrate fixture-graph-based view tests onto `writeGraph` temp dirs (parallelism + no stray journals).
- staticcheck in CI.
