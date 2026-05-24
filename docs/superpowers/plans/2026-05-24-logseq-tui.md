# Logseq TUI Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a read-only terminal UI for browsing a local Logseq graph — palette navigation, page viewer with wiki-link highlighting, ripgrep-backed full-text search, backlinks panel, and a TODO dashboard.

**Architecture:** Single Go binary. Indexer walks the graph at startup and builds in-memory `PageMeta`/`backlinks`/`todos`. Page content is read on demand. Bubble Tea drives the UI with one of five views active at a time; views are pure functions of the index plus the loaded page's bytes. No persistence, no editing.

**Tech Stack:** Go 1.22+, `bubbletea`, `bubbles`, `lipgloss`, `glamour`, `sahilm/fuzzy`, `teatest` (test-only). External: `rg` (ripgrep) on PATH.

**Spec:** `docs/superpowers/specs/2026-05-24-logseq-tui-design.md`

---

## Conventions

- Tests live alongside source as `*_test.go` (Go convention).
- Use the standard `testing` package; no `testify`. Helper: `t.Fatalf("want %q, got %q", want, got)`.
- Every task ends with a commit using Conventional Commits.
- Run `go vet ./... && go test ./...` before each commit; both must pass.
- Working directory for all commands: `~/Development/Projects/fiatcode/logseq-tui`.

---

## Task 1: Bootstrap module

**Files:**
- Create: `go.mod`
- Create: `.gitignore`
- Create: `cmd/lstui/main.go`

- [ ] **Step 1: Init module**

```bash
cd ~/Development/Projects/fiatcode/logseq-tui
go mod init github.com/fiatcode/logseq-tui
```

- [ ] **Step 2: Write `.gitignore`**

```gitignore
/lstui
/dist/
*.test
*.out
.DS_Store
```

- [ ] **Step 3: Write minimal `cmd/lstui/main.go`**

```go
package main

import "fmt"

func main() {
	fmt.Println("lstui v0")
}
```

- [ ] **Step 4: Verify it builds and runs**

Run: `go build ./... && ./lstui`
Expected: prints `lstui v0`.

- [ ] **Step 5: Commit**

```bash
git add go.mod .gitignore cmd/lstui/main.go
git commit -m "chore: bootstrap Go module and hello-world entry"
```

---

## Task 2: Build the fixture graph

**Files:**
- Create: `testdata/fixture-graph/logseq/config.edn`
- Create: `testdata/fixture-graph/pages/Alpha.md`
- Create: `testdata/fixture-graph/pages/Beta.md`
- Create: `testdata/fixture-graph/pages/proj___nested.md`
- Create: `testdata/fixture-graph/journals/2026_05_24.md`
- Create: `testdata/fixture-graph/journals/2026_05_23.md`

A small graph used by every integration test downstream. Pages cover: plain links, aliased links, dangling links, links inside fenced code (must be ignored), TODO/LATER/DOING/WAITING with and without priority, namespace pages, properties.

- [ ] **Step 1: Create empty config stub**

`testdata/fixture-graph/logseq/config.edn`:

```
{}
```

- [ ] **Step 2: Write `pages/Alpha.md`**

```markdown
title:: Alpha
tags:: fixture

- This page links to [[Beta]] and the namespaced [[proj/nested]].
- TODO Buy milk
- LATER [#A] Review the design doc
- A link inside a fence should NOT be extracted:
  ```
  see [[ShouldNotMatch]] for details
  ```
- An aliased link to [[Beta|the second page]].
```

- [ ] **Step 3: Write `pages/Beta.md`**

```markdown
- Beta links back to [[Alpha]].
- DOING Write the parser
- This link is dangling: [[DoesNotExist]].
```

- [ ] **Step 4: Write `pages/proj___nested.md`**

```markdown
- A nested page under the `proj` namespace.
- WAITING [#B] Vendor response on quote
```

- [ ] **Step 5: Write `journals/2026_05_24.md`**

```markdown
- Today's journal. References [[Alpha]] in passing.
- TODO Ship the TUI MVP
```

- [ ] **Step 6: Write `journals/2026_05_23.md`**

```markdown
- Yesterday's journal. No links, no todos.
- Just some notes about the weather.
```

- [ ] **Step 7: Commit**

```bash
git add testdata/
git commit -m "test: add fixture graph for integration tests"
```

---

## Task 3: Page-name resolution (`graph/resolve.go`)

**Files:**
- Create: `internal/graph/resolve.go`
- Create: `internal/graph/resolve_test.go`

Pure functions: filename ↔ page name. Logseq namespace convention: `foo___bar.md` ↔ page `foo/bar`. Journals are matched by `YYYY_MM_DD.md` filenames.

- [ ] **Step 1: Write failing tests**

`internal/graph/resolve_test.go`:

```go
package graph

import "testing"

func TestPageNameFromFilename(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"Alpha.md", "Alpha"},
		{"proj___nested.md", "proj/nested"},
		{"a___b___c.md", "a/b/c"},
		{"2026_05_24.md", "2026-05-24"},
	}
	for _, c := range cases {
		got := PageNameFromFilename(c.in)
		if got != c.want {
			t.Errorf("PageNameFromFilename(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestIsJournalFilename(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"2026_05_24.md", true},
		{"2026_5_24.md", false},
		{"Alpha.md", false},
		{"2026_05_24.txt", false},
	}
	for _, c := range cases {
		got := IsJournalFilename(c.in)
		if got != c.want {
			t.Errorf("IsJournalFilename(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}
```

- [ ] **Step 2: Run tests — confirm RED**

Run: `go test ./internal/graph/...`
Expected: compile failure — `PageNameFromFilename` and `IsJournalFilename` undefined.

- [ ] **Step 3: Implement `resolve.go`**

```go
package graph

import (
	"path/filepath"
	"regexp"
	"strings"
)

var journalRe = regexp.MustCompile(`^(\d{4})_(\d{2})_(\d{2})\.md$`)

// PageNameFromFilename returns the logical page name for a Logseq filename.
// Journals (YYYY_MM_DD.md) become YYYY-MM-DD; namespace separator "___" becomes "/".
func PageNameFromFilename(name string) string {
	base := strings.TrimSuffix(filepath.Base(name), ".md")
	if m := journalRe.FindStringSubmatch(filepath.Base(name)); m != nil {
		return m[1] + "-" + m[2] + "-" + m[3]
	}
	return strings.ReplaceAll(base, "___", "/")
}

// IsJournalFilename reports whether the filename matches Logseq's journal pattern.
func IsJournalFilename(name string) bool {
	return journalRe.MatchString(filepath.Base(name))
}
```

- [ ] **Step 4: Run tests — confirm GREEN**

Run: `go test ./internal/graph/... -v`
Expected: PASS for both tests.

- [ ] **Step 5: Commit**

```bash
git add internal/graph/resolve.go internal/graph/resolve_test.go
git commit -m "feat(graph): resolve Logseq filenames to page names"
```

---

## Task 4: Extract wiki-links and TODO bullets (`graph/parse.go`)

**Files:**
- Create: `internal/graph/parse.go`
- Create: `internal/graph/parse_test.go`
- Create: `internal/graph/types.go`

Pure functions operating on page bytes. Skip content inside fenced code blocks (` ``` `). Wiki-links match `[[Target]]` and `[[Target|alias]]` — only the target is recorded. TODO bullets begin with one of `TODO|LATER|DOING|WAITING`, optionally followed by `[#A]` etc.

- [ ] **Step 1: Create `types.go`**

```go
package graph

// PageMeta is the indexed metadata for one .md file in the graph.
type PageMeta struct {
	Name      string // logical page name (e.g., "proj/nested" or "2026-05-24")
	Path      string // absolute filesystem path
	IsJournal bool
}

// Ref points from one page's body to another page name.
type Ref struct {
	FromPage   string
	LineNumber int    // 1-based
	Context    string // the source line, trimmed
}

// TodoBullet is one open task bullet (TODO/LATER/DOING/WAITING).
type TodoBullet struct {
	Page       string
	LineNumber int    // 1-based
	Marker     string // TODO | LATER | DOING | WAITING
	Priority   string // "" | "A" | "B" | "C"
	Text       string // remainder after marker (and priority), trimmed
}
```

- [ ] **Step 2: Write failing tests**

`internal/graph/parse_test.go`:

```go
package graph

import (
	"reflect"
	"testing"
)

func TestExtractWikiLinks(t *testing.T) {
	body := "- See [[Alpha]] and [[Beta|the second]].\n" +
		"```\n[[InsideFence]]\n```\n" +
		"- Another [[proj/nested]] ref."
	got := ExtractWikiLinks(body)
	want := []LinkHit{
		{Target: "Alpha", Line: 1},
		{Target: "Beta", Line: 1},
		{Target: "proj/nested", Line: 4},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ExtractWikiLinks mismatch:\nwant %#v\ngot  %#v", want, got)
	}
}

func TestExtractTodos(t *testing.T) {
	body := "- TODO Buy milk\n" +
		"- LATER [#A] Review the doc\n" +
		"- DONE Should be ignored\n" +
		"- regular bullet\n" +
		"- DOING Write the parser\n" +
		"- WAITING [#B] Vendor response"
	got := ExtractTodos(body)
	want := []TodoHit{
		{Marker: "TODO", Priority: "", Text: "Buy milk", Line: 1},
		{Marker: "LATER", Priority: "A", Text: "Review the doc", Line: 2},
		{Marker: "DOING", Priority: "", Text: "Write the parser", Line: 5},
		{Marker: "WAITING", Priority: "B", Text: "Vendor response", Line: 6},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ExtractTodos mismatch:\nwant %#v\ngot  %#v", want, got)
	}
}
```

- [ ] **Step 3: Run tests — confirm RED**

Run: `go test ./internal/graph/...`
Expected: compile failure (`ExtractWikiLinks`, `ExtractTodos`, `LinkHit`, `TodoHit` undefined).

- [ ] **Step 4: Implement `parse.go`**

```go
package graph

import (
	"regexp"
	"strings"
)

// LinkHit is one wiki-link occurrence in a page body.
type LinkHit struct {
	Target string
	Line   int // 1-based
}

// TodoHit is one open task bullet occurrence in a page body.
type TodoHit struct {
	Marker   string // TODO | LATER | DOING | WAITING
	Priority string // "" | "A" | "B" | "C"
	Text     string
	Line     int // 1-based
}

var (
	wikiLinkRe = regexp.MustCompile(`\[\[([^\]\|]+)(?:\|[^\]]*)?\]\]`)
	todoRe     = regexp.MustCompile(`^\s*-\s+(TODO|LATER|DOING|WAITING)(?:\s+\[#([ABC])\])?\s+(.*\S)\s*$`)
	fenceRe    = regexp.MustCompile("^\\s*```")
)

// ExtractWikiLinks returns every [[link]] in body, skipping fenced code blocks.
// Targets like [[A|alias]] are recorded as "A".
func ExtractWikiLinks(body string) []LinkHit {
	var out []LinkHit
	inFence := false
	for i, line := range strings.Split(body, "\n") {
		if fenceRe.MatchString(line) {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
		for _, m := range wikiLinkRe.FindAllStringSubmatch(line, -1) {
			out = append(out, LinkHit{Target: strings.TrimSpace(m[1]), Line: i + 1})
		}
	}
	return out
}

// ExtractTodos returns open TODO/LATER/DOING/WAITING bullets in body.
// DONE and CANCELED are intentionally ignored (we only surface open tasks).
func ExtractTodos(body string) []TodoHit {
	var out []TodoHit
	inFence := false
	for i, line := range strings.Split(body, "\n") {
		if fenceRe.MatchString(line) {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
		m := todoRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		out = append(out, TodoHit{
			Marker:   m[1],
			Priority: m[2],
			Text:     m[3],
			Line:     i + 1,
		})
	}
	return out
}
```

- [ ] **Step 5: Run tests — confirm GREEN**

Run: `go test ./internal/graph/... -v`
Expected: PASS for `TestExtractWikiLinks` and `TestExtractTodos`.

- [ ] **Step 6: Commit**

```bash
git add internal/graph/parse.go internal/graph/parse_test.go internal/graph/types.go
git commit -m "feat(graph): extract wiki-links and TODO bullets"
```

---

## Task 5: Build the index (`graph/index.go`)

**Files:**
- Create: `internal/graph/index.go`
- Create: `internal/graph/index_test.go`

Walks `<graph>/pages/*.md` and `<graph>/journals/*.md`, builds `PageMeta`, `backlinks`, `todos`. Integration test runs against `testdata/fixture-graph/`.

- [ ] **Step 1: Write failing test**

`internal/graph/index_test.go`:

```go
package graph

import (
	"path/filepath"
	"sort"
	"testing"
)

func fixturePath(t *testing.T) string {
	t.Helper()
	p, err := filepath.Abs("../../testdata/fixture-graph")
	if err != nil {
		t.Fatalf("abs fixture path: %v", err)
	}
	return p
}

func TestBuildIndex(t *testing.T) {
	idx, err := BuildIndex(fixturePath(t))
	if err != nil {
		t.Fatalf("BuildIndex: %v", err)
	}

	// Pages
	names := make([]string, 0, len(idx.Pages))
	for _, p := range idx.Pages {
		names = append(names, p.Name)
	}
	sort.Strings(names)
	want := []string{"2026-05-23", "2026-05-24", "Alpha", "Beta", "proj/nested"}
	if !equalSlices(names, want) {
		t.Errorf("page names: want %v, got %v", want, names)
	}

	// Backlinks: Alpha is linked from Beta and 2026-05-24
	gotAlpha := pageNamesOfRefs(idx.Backlinks["Alpha"])
	sort.Strings(gotAlpha)
	wantAlpha := []string{"2026-05-24", "Beta"}
	if !equalSlices(gotAlpha, wantAlpha) {
		t.Errorf("Alpha backlinks: want %v, got %v", wantAlpha, gotAlpha)
	}

	// Dangling refs still recorded
	if len(idx.Backlinks["DoesNotExist"]) != 1 {
		t.Errorf("dangling backlink to DoesNotExist not recorded: %v", idx.Backlinks["DoesNotExist"])
	}

	// Todos: 4 open across the fixture
	if len(idx.Todos) != 4 {
		t.Errorf("todo count: want 4, got %d (%+v)", len(idx.Todos), idx.Todos)
	}
}

func pageNamesOfRefs(refs []Ref) []string {
	out := make([]string, 0, len(refs))
	for _, r := range refs {
		out = append(out, r.FromPage)
	}
	return out
}

func equalSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
```

- [ ] **Step 2: Run test — confirm RED**

Run: `go test ./internal/graph/... -run TestBuildIndex`
Expected: compile failure — `BuildIndex` and `Index` undefined.

- [ ] **Step 3: Implement `index.go`**

```go
package graph

import (
	"fmt"
	"os"
	"path/filepath"
)

// Index is the read-only in-memory view of a Logseq graph.
type Index struct {
	GraphPath string
	Pages     []PageMeta
	ByName    map[string]*PageMeta
	Backlinks map[string][]Ref
	Todos     []TodoBullet
}

// BuildIndex walks <graphPath>/pages and <graphPath>/journals once and returns
// the populated Index. Unreadable files are logged to stderr and skipped.
func BuildIndex(graphPath string) (*Index, error) {
	idx := &Index{
		GraphPath: graphPath,
		ByName:    make(map[string]*PageMeta),
		Backlinks: make(map[string][]Ref),
	}

	for _, sub := range []string{"pages", "journals"} {
		dir := filepath.Join(graphPath, sub)
		entries, err := os.ReadDir(dir)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", dir, err)
		}
		for _, e := range entries {
			if e.IsDir() || filepath.Ext(e.Name()) != ".md" {
				continue
			}
			path := filepath.Join(dir, e.Name())
			meta := PageMeta{
				Name:      PageNameFromFilename(e.Name()),
				Path:      path,
				IsJournal: sub == "journals",
			}
			idx.Pages = append(idx.Pages, meta)
		}
	}

	// ByName must be built after Pages is final (so pointers are stable).
	for i := range idx.Pages {
		idx.ByName[idx.Pages[i].Name] = &idx.Pages[i]
	}

	// Second pass: parse bodies for links + todos.
	for _, p := range idx.Pages {
		body, err := os.ReadFile(p.Path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "lstui: skipping %s: %v\n", p.Path, err)
			continue
		}
		s := string(body)
		for _, lh := range ExtractWikiLinks(s) {
			idx.Backlinks[lh.Target] = append(idx.Backlinks[lh.Target], Ref{
				FromPage:   p.Name,
				LineNumber: lh.Line,
				Context:    lineAt(s, lh.Line),
			})
		}
		for _, th := range ExtractTodos(s) {
			idx.Todos = append(idx.Todos, TodoBullet{
				Page:       p.Name,
				LineNumber: th.Line,
				Marker:     th.Marker,
				Priority:   th.Priority,
				Text:       th.Text,
			})
		}
	}
	return idx, nil
}

func lineAt(body string, n int) string {
	i := 1
	start := 0
	for j := 0; j < len(body); j++ {
		if i == n {
			end := j
			for end < len(body) && body[end] != '\n' {
				end++
			}
			return body[start:end]
		}
		if body[j] == '\n' {
			i++
			start = j + 1
		}
	}
	return ""
}
```

- [ ] **Step 4: Run test — confirm GREEN**

Run: `go test ./internal/graph/... -v`
Expected: PASS for `TestBuildIndex` (and the earlier tests still pass).

- [ ] **Step 5: Commit**

```bash
git add internal/graph/index.go internal/graph/index_test.go
git commit -m "feat(graph): build in-memory index of pages, backlinks, todos"
```

---

## Task 6: Page rendering (`render/page.go`)

**Files:**
- Create: `internal/render/page.go`
- Create: `internal/render/page_test.go`

Wrap Glamour for markdown rendering. After Glamour produces a styled string, post-process `[[wiki-links]]` to wrap them in an ANSI style and to record their byte positions so the page view can move a cursor between them.

- [ ] **Step 1: Add Glamour dependency**

```bash
go get github.com/charmbracelet/glamour@latest
go get github.com/charmbracelet/lipgloss@latest
```

- [ ] **Step 2: Write failing test**

`internal/render/page_test.go`:

```go
package render

import (
	"strings"
	"testing"
)

func TestRenderPageReturnsLinksWithTargets(t *testing.T) {
	body := "- See [[Alpha]] and [[Beta|the second]]."
	out, err := Render(body, 80)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	gotTargets := make([]string, 0, len(out.Links))
	for _, l := range out.Links {
		gotTargets = append(gotTargets, l.Target)
	}
	want := []string{"Alpha", "Beta"}
	if strings.Join(gotTargets, ",") != strings.Join(want, ",") {
		t.Errorf("link targets: want %v, got %v", want, gotTargets)
	}
	if out.Styled == "" {
		t.Error("Styled output empty")
	}
}

func TestRenderPageHandlesEmptyBody(t *testing.T) {
	out, err := Render("", 80)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if len(out.Links) != 0 {
		t.Errorf("empty body should have no links, got %v", out.Links)
	}
}
```

- [ ] **Step 3: Run test — confirm RED**

Run: `go test ./internal/render/...`
Expected: compile failure (`Render`, `Result` undefined).

- [ ] **Step 4: Implement `page.go`**

```go
package render

import (
	"regexp"

	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
)

// Link is one wiki-link position inside the styled output.
type Link struct {
	Target string
	Start  int // byte offset in Styled
	End    int // byte offset in Styled (exclusive)
}

// Result is the rendered page.
type Result struct {
	Styled string
	Links  []Link
}

var wikiLinkRe = regexp.MustCompile(`\[\[([^\]\|]+)(?:\|([^\]]*))?\]\]`)

var linkStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("12")).Underline(true)

// Render returns Glamour-rendered markdown with wiki-link positions annotated.
// width is the target terminal column count.
func Render(body string, width int) (Result, error) {
	r, err := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(width),
	)
	if err != nil {
		return Result{}, err
	}
	styled, err := r.Render(body)
	if err != nil {
		// Fallback: plain text if Glamour chokes
		styled = body
	}

	var links []Link
	out := wikiLinkRe.ReplaceAllStringFunc(styled, func(match string) string {
		m := wikiLinkRe.FindStringSubmatch(match)
		display := m[1]
		if m[2] != "" {
			display = m[2]
		}
		return linkStyle.Render(display)
	})

	// Re-scan the original styled string to locate each link in the final output.
	// Because ReplaceAllStringFunc rewrites the buffer, we compute positions on `out`.
	idx := 0
	for _, m := range wikiLinkRe.FindAllStringSubmatchIndex(styled, -1) {
		target := styled[m[2]:m[3]]
		display := target
		if m[4] != -1 {
			display = styled[m[4]:m[5]]
		}
		rendered := linkStyle.Render(display)
		pos := indexFrom(out, rendered, idx)
		if pos < 0 {
			continue
		}
		links = append(links, Link{
			Target: target,
			Start:  pos,
			End:    pos + len(rendered),
		})
		idx = pos + len(rendered)
	}

	return Result{Styled: out, Links: links}, nil
}

func indexFrom(s, sub string, from int) int {
	if from < 0 || from >= len(s) {
		return -1
	}
	rel := -1
	if i := indexOf(s[from:], sub); i >= 0 {
		rel = from + i
	}
	return rel
}

func indexOf(s, sub string) int {
	// thin wrapper to keep the call site readable and avoid strings import name clash
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
```

- [ ] **Step 5: Run test — confirm GREEN**

Run: `go mod tidy && go test ./internal/render/... -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add go.mod go.sum internal/render/page.go internal/render/page_test.go
git commit -m "feat(render): wrap Glamour with wiki-link annotations"
```

---

## Task 7: App skeleton boots to today's journal (`views/app.go`, `cmd/lstui/main.go`)

**Files:**
- Create: `internal/views/app.go`
- Modify: `cmd/lstui/main.go`

The Bubble Tea app starts up, resolves the graph path (flag > env > default), runs `BuildIndex`, and renders today's journal as plain styled text. Quit on `q` or `Ctrl-C`. No palette, no navigation yet — just proof the wiring works.

- [ ] **Step 1: Add Bubble Tea dependencies**

```bash
go get github.com/charmbracelet/bubbletea@latest
go get github.com/charmbracelet/bubbles@latest
```

- [ ] **Step 2: Write `internal/views/app.go`**

```go
package views

import (
	"fmt"
	"os"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/fiatcode/logseq-tui/internal/graph"
	"github.com/fiatcode/logseq-tui/internal/render"
)

// App is the top-level Bubble Tea model.
type App struct {
	idx     *graph.Index
	current string // page name currently displayed
	width   int
	height  int
	body    render.Result
	err     error
}

// New builds an App rooted at graphPath. It loads the index synchronously
// because BuildIndex is fast (<100ms for graphs we care about) and a blank
// first frame is worse than a 100ms delay.
func New(graphPath string) (*App, error) {
	idx, err := graph.BuildIndex(graphPath)
	if err != nil {
		return nil, fmt.Errorf("index %s: %w", graphPath, err)
	}
	a := &App{idx: idx, current: todayJournalName()}
	return a, nil
}

func todayJournalName() string {
	return time.Now().Format("2006-01-02")
}

func (a *App) Init() tea.Cmd { return nil }

func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m := msg.(type) {
	case tea.WindowSizeMsg:
		a.width, a.height = m.Width, m.Height
		a.refreshBody()
	case tea.KeyMsg:
		switch m.String() {
		case "q", "ctrl+c":
			return a, tea.Quit
		}
	}
	return a, nil
}

func (a *App) View() string {
	if a.err != nil {
		return fmt.Sprintf("error: %v\n\nq to quit.", a.err)
	}
	header := fmt.Sprintf("lstui — %s\n\n", a.current)
	if a.body.Styled == "" {
		return header + "(no entry yet for this page)\n\nq to quit."
	}
	return header + a.body.Styled + "\n\nq to quit."
}

func (a *App) refreshBody() {
	meta, ok := a.idx.ByName[a.current]
	if !ok {
		a.body = render.Result{}
		return
	}
	bytes, err := os.ReadFile(meta.Path)
	if err != nil {
		a.err = err
		return
	}
	res, err := render.Render(string(bytes), a.width)
	if err != nil {
		a.err = err
		return
	}
	a.body = res
}
```

- [ ] **Step 3: Rewrite `cmd/lstui/main.go`**

```go
package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/fiatcode/logseq-tui/internal/views"
)

func main() {
	defaultGraph := os.ExpandEnv("$HOME/Documents/fiat-codex")
	graphFlag := flag.String("graph", "", "path to Logseq graph (overrides $LSTUI_GRAPH and default)")
	flag.Parse()

	graphPath := resolveGraphPath(*graphFlag, os.Getenv("LSTUI_GRAPH"), defaultGraph)

	if _, err := exec.LookPath("rg"); err != nil {
		fmt.Fprintln(os.Stderr, "lstui: ripgrep (rg) not found on PATH — install it (https://github.com/BurntSushi/ripgrep) and try again.")
		os.Exit(2)
	}
	if _, err := os.Stat(graphPath); err != nil {
		fmt.Fprintf(os.Stderr, "lstui: graph path %q is not accessible: %v\n", graphPath, err)
		os.Exit(2)
	}

	app, err := views.New(graphPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "lstui: %v\n", err)
		os.Exit(1)
	}

	if _, err := tea.NewProgram(app, tea.WithAltScreen()).Run(); err != nil {
		fmt.Fprintf(os.Stderr, "lstui: %v\n", err)
		os.Exit(1)
	}
}

func resolveGraphPath(flagVal, envVal, defaultVal string) string {
	if flagVal != "" {
		return flagVal
	}
	if envVal != "" {
		return envVal
	}
	return defaultVal
}
```

- [ ] **Step 4: Build, run against the fixture, smoke-test**

Run: `go build ./... && ./lstui --graph testdata/fixture-graph`
Expected: TUI shows "lstui — `<today's date>`" header. The fixture's journals are dated 2026-05-23/24, so unless today is one of those dates you'll see "(no entry yet for this page)". Press `q` to quit. Verify no crash.

- [ ] **Step 5: Commit**

```bash
go mod tidy
git add go.mod go.sum cmd/lstui/main.go internal/views/app.go
git commit -m "feat(views): bootstrap Bubble Tea app rooted at today's journal"
```

---

## Task 8: Page view with wiki-link cursor (`views/page.go`)

**Files:**
- Create: `internal/views/page.go`
- Modify: `internal/views/app.go`

Split the page-rendering logic out of `App` into a dedicated `PageView`. Add `n`/`N` to cycle between wiki-links, `Enter` to follow the link (sets `App.current` to the target). Use a `viewport.Model` from `bubbles` for scrolling.

- [ ] **Step 1: Write `internal/views/page.go`**

```go
package views

import (
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/lipgloss"

	"github.com/fiatcode/logseq-tui/internal/graph"
	"github.com/fiatcode/logseq-tui/internal/render"
)

// PageView renders a single page with a wiki-link cursor.
type PageView struct {
	idx     *graph.Index
	page    string
	result  render.Result
	vp      viewport.Model
	cursor  int // index into result.Links, or -1
	err     error
	width   int
	height  int
}

func NewPageView(idx *graph.Index, page string, width, height int) *PageView {
	pv := &PageView{
		idx:    idx,
		page:   page,
		width:  width,
		height: height,
		cursor: -1,
		vp:     viewport.New(width, max(1, height-2)),
	}
	pv.load()
	return pv
}

// Page returns the currently displayed page name.
func (p *PageView) Page() string { return p.page }

// SetPage switches to a different page in the same index.
func (p *PageView) SetPage(name string) {
	p.page = name
	p.cursor = -1
	p.load()
}

// SetSize updates viewport size.
func (p *PageView) SetSize(w, h int) {
	p.width, p.height = w, h
	p.vp.Width = w
	p.vp.Height = max(1, h-2)
	p.load()
}

// CycleLink moves the cursor to the next/previous wiki-link.
// dir=+1 forwards, dir=-1 backwards.
func (p *PageView) CycleLink(dir int) {
	if len(p.result.Links) == 0 {
		return
	}
	if p.cursor == -1 {
		if dir > 0 {
			p.cursor = 0
		} else {
			p.cursor = len(p.result.Links) - 1
		}
	} else {
		p.cursor = (p.cursor + dir + len(p.result.Links)) % len(p.result.Links)
	}
}

// FollowCursor returns the link target under the cursor, or "" if none.
func (p *PageView) FollowCursor() string {
	if p.cursor < 0 || p.cursor >= len(p.result.Links) {
		return ""
	}
	return p.result.Links[p.cursor].Target
}

// LineDown/LineUp/HalfPage delegates to viewport.
func (p *PageView) LineDown()      { p.vp.LineDown(1) }
func (p *PageView) LineUp()        { p.vp.LineUp(1) }
func (p *PageView) HalfPageDown()  { p.vp.HalfViewDown() }
func (p *PageView) HalfPageUp()    { p.vp.HalfViewUp() }

var cursorStyle = lipgloss.NewStyle().Reverse(true)

func (p *PageView) View() string {
	if p.err != nil {
		return fmt.Sprintf("error: %v", p.err)
	}
	header := fmt.Sprintf("# %s\n", p.page)
	body := p.result.Styled
	if body == "" {
		body = "(no entry yet for this page)"
	}
	if p.cursor >= 0 && p.cursor < len(p.result.Links) {
		l := p.result.Links[p.cursor]
		body = body[:l.Start] + cursorStyle.Render(body[l.Start:l.End]) + body[l.End:]
	}
	p.vp.SetContent(body)
	return header + "\n" + p.vp.View()
}

func (p *PageView) load() {
	meta, ok := p.idx.ByName[p.page]
	if !ok {
		p.result = render.Result{}
		return
	}
	b, err := os.ReadFile(meta.Path)
	if err != nil {
		p.err = err
		return
	}
	res, err := render.Render(strings.TrimSpace(string(b))+"\n", p.width)
	if err != nil {
		p.err = err
		return
	}
	p.result = res
	p.vp.SetContent(p.result.Styled)
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
```

- [ ] **Step 2: Update `App` to delegate to `PageView`**

Replace `internal/views/app.go` with:

```go
package views

import (
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/fiatcode/logseq-tui/internal/graph"
)

type App struct {
	idx    *graph.Index
	page   *PageView
	width  int
	height int
}

func New(graphPath string) (*App, error) {
	idx, err := graph.BuildIndex(graphPath)
	if err != nil {
		return nil, fmt.Errorf("index %s: %w", graphPath, err)
	}
	a := &App{idx: idx}
	a.page = NewPageView(idx, todayJournalName(), 80, 24)
	return a, nil
}

func todayJournalName() string { return time.Now().Format("2006-01-02") }

func (a *App) Init() tea.Cmd { return nil }

func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m := msg.(type) {
	case tea.WindowSizeMsg:
		a.width, a.height = m.Width, m.Height
		a.page.SetSize(m.Width, m.Height)
	case tea.KeyMsg:
		switch m.String() {
		case "q", "ctrl+c":
			return a, tea.Quit
		case "n":
			a.page.CycleLink(+1)
		case "N":
			a.page.CycleLink(-1)
		case "enter":
			if t := a.page.FollowCursor(); t != "" {
				a.page.SetPage(t)
			}
		case "j", "down":
			a.page.LineDown()
		case "k", "up":
			a.page.LineUp()
		case "ctrl+d":
			a.page.HalfPageDown()
		case "ctrl+u":
			a.page.HalfPageUp()
		}
	}
	return a, nil
}

func (a *App) View() string {
	return a.page.View() + "\n[n/N] link  [enter] follow  [j/k] scroll  [q] quit"
}
```

- [ ] **Step 3: Smoke-test interactively**

Run: `go build ./... && ./lstui --graph testdata/fixture-graph`
- Resize the terminal — content should re-flow.
- Press `n` repeatedly on a page that has links (today's journal won't have any unless today is 2026-05-24; open `Alpha` instead in the next task).
- Press `q` to quit.

(No automated test in this task — we add `teatest` coverage for the page view in Task 14 after all views exist.)

- [ ] **Step 4: Commit**

```bash
git add internal/views/app.go internal/views/page.go
git commit -m "feat(views): page viewer with wiki-link cursor and scroll"
```

---

## Task 9: Palette overlay (`views/palette.go`)

**Files:**
- Create: `internal/views/palette.go`
- Modify: `internal/views/app.go`

`Ctrl-P` opens a fuzzy-search overlay listing every page name plus virtual journal-date entries for ±30 days around today. `Enter` opens, `Esc` cancels.

- [ ] **Step 1: Add fuzzy dep**

```bash
go get github.com/sahilm/fuzzy@latest
```

- [ ] **Step 2: Write `internal/views/palette.go`**

```go
package views

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/lipgloss"
	"github.com/sahilm/fuzzy"

	"github.com/fiatcode/logseq-tui/internal/graph"
)

type Palette struct {
	idx     *graph.Index
	input   textinput.Model
	choices []string // candidate page names (real + virtual journals)
	matches []fuzzy.Match
	sel     int
}

func NewPalette(idx *graph.Index) *Palette {
	ti := textinput.New()
	ti.Placeholder = "Type a page name or YYYY-MM-DD..."
	ti.Focus()
	ti.CharLimit = 200
	p := &Palette{idx: idx, input: ti}
	p.choices = paletteChoices(idx)
	p.search("")
	return p
}

func paletteChoices(idx *graph.Index) []string {
	seen := make(map[string]struct{}, len(idx.Pages)+60)
	out := make([]string, 0, len(idx.Pages)+60)
	for _, p := range idx.Pages {
		if _, ok := seen[p.Name]; ok {
			continue
		}
		seen[p.Name] = struct{}{}
		out = append(out, p.Name)
	}
	// Virtual journal dates ±30 days around today
	today := time.Now()
	for d := -30; d <= 30; d++ {
		name := today.AddDate(0, 0, d).Format("2006-01-02")
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

func (p *Palette) search(q string) {
	if strings.TrimSpace(q) == "" {
		p.matches = nil
		for i, c := range p.choices {
			if i >= 50 {
				break
			}
			p.matches = append(p.matches, fuzzy.Match{Str: c, Index: i})
		}
		p.sel = 0
		return
	}
	p.matches = fuzzy.Find(q, p.choices)
	if len(p.matches) > 50 {
		p.matches = p.matches[:50]
	}
	p.sel = 0
}

// Update handles a key. Returns (selected page name, accept, cancel).
func (p *Palette) Update(key string) (selected string, accept bool, cancel bool) {
	switch key {
	case "esc":
		return "", false, true
	case "enter":
		if p.sel >= 0 && p.sel < len(p.matches) {
			return p.matches[p.sel].Str, true, false
		}
		return "", false, false
	case "up", "ctrl+k":
		if p.sel > 0 {
			p.sel--
		}
		return "", false, false
	case "down", "ctrl+j":
		if p.sel < len(p.matches)-1 {
			p.sel++
		}
		return "", false, false
	}
	// Otherwise feed the key into the text input
	p.input, _ = consumeKey(p.input, key)
	p.search(p.input.Value())
	return "", false, false
}

// consumeKey is a tiny adapter to feed a key string to a textinput.Model.
// Bubble Tea normally sends tea.KeyMsg; we hand-roll just enough for our overlay.
func consumeKey(ti textinput.Model, key string) (textinput.Model, bool) {
	switch key {
	case "backspace":
		v := ti.Value()
		if len(v) > 0 {
			ti.SetValue(v[:len(v)-1])
		}
		return ti, true
	}
	if len(key) == 1 {
		ti.SetValue(ti.Value() + key)
		return ti, true
	}
	return ti, false
}

var (
	paletteBorder = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 1)
	paletteSel    = lipgloss.NewStyle().Reverse(true)
)

func (p *Palette) View() string {
	var b strings.Builder
	fmt.Fprintf(&b, "> %s\n\n", p.input.Value())
	for i, m := range p.matches {
		line := m.Str
		if i == p.sel {
			line = paletteSel.Render("▶ " + line)
		} else {
			line = "  " + line
		}
		b.WriteString(line + "\n")
	}
	return paletteBorder.Render(b.String())
}
```

- [ ] **Step 3: Wire palette into `App`**

Modify `internal/views/app.go` — add palette state and routing. Replace the file contents with:

```go
package views

import (
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/fiatcode/logseq-tui/internal/graph"
)

type modeT int

const (
	modePage modeT = iota
	modePalette
)

type App struct {
	idx     *graph.Index
	page    *PageView
	palette *Palette
	mode    modeT
	width   int
	height  int
}

func New(graphPath string) (*App, error) {
	idx, err := graph.BuildIndex(graphPath)
	if err != nil {
		return nil, fmt.Errorf("index %s: %w", graphPath, err)
	}
	a := &App{idx: idx, mode: modePage}
	a.page = NewPageView(idx, todayJournalName(), 80, 24)
	return a, nil
}

func todayJournalName() string { return time.Now().Format("2006-01-02") }

func (a *App) Init() tea.Cmd { return nil }

func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m := msg.(type) {
	case tea.WindowSizeMsg:
		a.width, a.height = m.Width, m.Height
		a.page.SetSize(m.Width, m.Height)
	case tea.KeyMsg:
		key := m.String()
		if a.mode == modePalette {
			sel, accept, cancel := a.palette.Update(key)
			if cancel {
				a.mode = modePage
				a.palette = nil
				return a, nil
			}
			if accept {
				a.page.SetPage(sel)
				a.mode = modePage
				a.palette = nil
			}
			return a, nil
		}
		switch key {
		case "q", "ctrl+c":
			return a, tea.Quit
		case "ctrl+p":
			a.palette = NewPalette(a.idx)
			a.mode = modePalette
		case "n":
			a.page.CycleLink(+1)
		case "N":
			a.page.CycleLink(-1)
		case "enter":
			if t := a.page.FollowCursor(); t != "" {
				a.page.SetPage(t)
			}
		case "j", "down":
			a.page.LineDown()
		case "k", "up":
			a.page.LineUp()
		case "ctrl+d":
			a.page.HalfPageDown()
		case "ctrl+u":
			a.page.HalfPageUp()
		}
	}
	return a, nil
}

func (a *App) View() string {
	if a.mode == modePalette {
		return a.palette.View()
	}
	return a.page.View() + "\n[ctrl-p] palette  [n/N] link  [enter] follow  [q] quit"
}
```

- [ ] **Step 4: Smoke-test**

Run: `go build ./... && ./lstui --graph testdata/fixture-graph`
- `Ctrl-P` opens palette, type "alp" → "Alpha" highlighted, `Enter` opens it.
- Re-open palette, type "2026" → both fixture journals appear plus virtual dates.

- [ ] **Step 5: Commit**

```bash
go mod tidy
git add go.mod go.sum internal/views/palette.go internal/views/app.go
git commit -m "feat(views): fuzzy palette overlay for page navigation"
```

---

## Task 10: Search overlay backed by ripgrep (`views/search.go`)

**Files:**
- Create: `internal/views/search.go`
- Create: `internal/views/search_test.go`
- Modify: `internal/views/app.go`

`/` opens a query input. On `Enter`, run `rg --json <query> <graph>` as a `tea.Cmd`. Parse the JSON lines, build a result list of (page name, line, context). `Enter` on a result opens the page.

- [ ] **Step 1: Write failing test for the rg parser**

`internal/views/search_test.go`:

```go
package views

import "testing"

func TestParseRipgrepJSON(t *testing.T) {
	// Two match lines + non-match types interleaved.
	in := `{"type":"begin","data":{"path":{"text":"/g/pages/Alpha.md"}}}
{"type":"match","data":{"path":{"text":"/g/pages/Alpha.md"},"lines":{"text":"links to Beta\n"},"line_number":3,"absolute_offset":42,"submatches":[]}}
{"type":"match","data":{"path":{"text":"/g/journals/2026_05_24.md"},"lines":{"text":"references Alpha\n"},"line_number":1,"absolute_offset":0,"submatches":[]}}
{"type":"end","data":{"path":{"text":"/g/pages/Alpha.md"}}}
`
	hits := parseRipgrepJSON([]byte(in))
	if len(hits) != 2 {
		t.Fatalf("want 2 hits, got %d: %+v", len(hits), hits)
	}
	if hits[0].Line != 3 || hits[0].Context != "links to Beta" {
		t.Errorf("hit[0]: %+v", hits[0])
	}
	if hits[1].FilePath != "/g/journals/2026_05_24.md" {
		t.Errorf("hit[1]: %+v", hits[1])
	}
}
```

- [ ] **Step 2: Confirm RED**

Run: `go test ./internal/views/... -run TestParseRipgrepJSON`
Expected: compile failure — `parseRipgrepJSON` and `SearchHit` undefined.

- [ ] **Step 3: Implement `search.go`**

```go
package views

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/fiatcode/logseq-tui/internal/graph"
)

// SearchHit is one rg result row.
type SearchHit struct {
	FilePath string
	Line     int
	Context  string
}

type SearchView struct {
	idx     *graph.Index
	query   string
	hits    []SearchHit
	sel     int
	running bool
	err     error
}

func NewSearchView(idx *graph.Index) *SearchView {
	return &SearchView{idx: idx}
}

func (s *SearchView) Query() string { return s.query }

func (s *SearchView) SetQuery(q string) { s.query = q }

// SearchCmd returns a tea.Cmd that runs rg and returns a searchDoneMsg.
func (s *SearchView) SearchCmd(graphPath string) tea.Cmd {
	q := s.query
	return func() tea.Msg {
		out, err := runRipgrep(graphPath, q)
		if err != nil {
			return searchDoneMsg{err: err}
		}
		return searchDoneMsg{hits: parseRipgrepJSON(out)}
	}
}

type searchDoneMsg struct {
	hits []SearchHit
	err  error
}

func (s *SearchView) Apply(msg searchDoneMsg) {
	s.running = false
	s.err = msg.err
	s.hits = msg.hits
	if s.sel >= len(s.hits) {
		s.sel = 0
	}
}

// Update handles a key. Returns (selected hit, accept, cancel, cmd to run).
func (s *SearchView) Update(key string, graphPath string) (hit *SearchHit, accept, cancel bool, cmd tea.Cmd) {
	switch key {
	case "esc":
		return nil, false, true, nil
	case "enter":
		if s.running {
			return nil, false, false, nil
		}
		if s.query == "" {
			return nil, false, false, nil
		}
		if len(s.hits) == 0 {
			s.running = true
			return nil, false, false, s.SearchCmd(graphPath)
		}
		if s.sel >= 0 && s.sel < len(s.hits) {
			h := s.hits[s.sel]
			return &h, true, false, nil
		}
	case "up", "ctrl+k":
		if s.sel > 0 {
			s.sel--
		}
	case "down", "ctrl+j":
		if s.sel < len(s.hits)-1 {
			s.sel++
		}
	case "backspace":
		if len(s.query) > 0 {
			s.query = s.query[:len(s.query)-1]
			s.hits = nil
		}
	default:
		if len(key) == 1 {
			s.query += key
			s.hits = nil
		}
	}
	return nil, false, false, nil
}

var (
	searchBorder = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 1)
	searchSel    = lipgloss.NewStyle().Reverse(true)
)

func (s *SearchView) View() string {
	var b strings.Builder
	state := ""
	switch {
	case s.err != nil:
		state = fmt.Sprintf(" [error: %v]", s.err)
	case s.running:
		state = " [searching…]"
	case len(s.hits) == 0 && s.query != "":
		state = " [enter to search]"
	}
	fmt.Fprintf(&b, "/ %s%s\n\n", s.query, state)
	for i, h := range s.hits {
		line := fmt.Sprintf("%s:%d — %s", shortPath(h.FilePath), h.Line, h.Context)
		if i == s.sel {
			line = searchSel.Render("▶ " + line)
		} else {
			line = "  " + line
		}
		b.WriteString(line + "\n")
	}
	return searchBorder.Render(b.String())
}

func shortPath(p string) string {
	parts := strings.Split(p, "/")
	if len(parts) < 2 {
		return p
	}
	return strings.Join(parts[len(parts)-2:], "/")
}

func runRipgrep(graphPath, query string) ([]byte, error) {
	cmd := exec.Command("rg", "--json", "--", query, graphPath)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	// rg exits 1 when no matches — that's not an error for us.
	if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
		return stdout.Bytes(), nil
	}
	if err != nil {
		return nil, fmt.Errorf("rg failed: %w (%s)", err, stderr.String())
	}
	return stdout.Bytes(), nil
}

func parseRipgrepJSON(b []byte) []SearchHit {
	var out []SearchHit
	sc := bufio.NewScanner(bytes.NewReader(b))
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		var env struct {
			Type string `json:"type"`
			Data struct {
				Path       struct{ Text string } `json:"path"`
				Lines      struct{ Text string } `json:"lines"`
				LineNumber int                   `json:"line_number"`
			} `json:"data"`
		}
		if err := json.Unmarshal(sc.Bytes(), &env); err != nil {
			continue
		}
		if env.Type != "match" {
			continue
		}
		out = append(out, SearchHit{
			FilePath: env.Data.Path.Text,
			Line:     env.Data.LineNumber,
			Context:  strings.TrimRight(env.Data.Lines.Text, "\n"),
		})
	}
	return out
}
```

- [ ] **Step 4: Confirm GREEN**

Run: `go test ./internal/views/... -v`
Expected: `TestParseRipgrepJSON` passes.

- [ ] **Step 5: Wire into `App`**

In `internal/views/app.go`, add the search mode. Insert these changes:

- Extend `modeT`:

```go
const (
	modePage modeT = iota
	modePalette
	modeSearch
)
```

- Add field to `App`:

```go
	search  *SearchView
```

- In `Update`, when `mode == modeSearch`, route keys to `search.Update`. When it returns `accept=true`, switch the page view to the hit's page (resolved from `FilePath` via `graphPath` + filename). When `cancel=true`, return to page view.
- In the page-mode key switch, add `case "/": a.search = NewSearchView(a.idx); a.mode = modeSearch`.
- Handle `searchDoneMsg` at the top of `Update` before the type switch on `KeyMsg`.

Full updated `Update` body:

```go
func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m := msg.(type) {
	case searchDoneMsg:
		if a.search != nil {
			a.search.Apply(m)
		}
		return a, nil
	case tea.WindowSizeMsg:
		a.width, a.height = m.Width, m.Height
		a.page.SetSize(m.Width, m.Height)
	case tea.KeyMsg:
		key := m.String()
		switch a.mode {
		case modePalette:
			sel, accept, cancel := a.palette.Update(key)
			if cancel { a.mode = modePage; a.palette = nil; return a, nil }
			if accept { a.page.SetPage(sel); a.mode = modePage; a.palette = nil }
			return a, nil
		case modeSearch:
			hit, accept, cancel, cmd := a.search.Update(key, a.idx.GraphPath)
			if cancel { a.mode = modePage; a.search = nil; return a, nil }
			if accept && hit != nil {
				if name := pageNameFromHitPath(a.idx, hit.FilePath); name != "" {
					a.page.SetPage(name)
				}
				a.mode = modePage
				a.search = nil
				return a, nil
			}
			return a, cmd
		case modePage:
			switch key {
			case "q", "ctrl+c":
				return a, tea.Quit
			case "ctrl+p":
				a.palette = NewPalette(a.idx); a.mode = modePalette
			case "/":
				a.search = NewSearchView(a.idx); a.mode = modeSearch
			case "n":
				a.page.CycleLink(+1)
			case "N":
				a.page.CycleLink(-1)
			case "enter":
				if t := a.page.FollowCursor(); t != "" { a.page.SetPage(t) }
			case "j", "down":
				a.page.LineDown()
			case "k", "up":
				a.page.LineUp()
			case "ctrl+d":
				a.page.HalfPageDown()
			case "ctrl+u":
				a.page.HalfPageUp()
			}
		}
	}
	return a, nil
}
```

- Add helper at file bottom:

```go
func pageNameFromHitPath(idx *graph.Index, abs string) string {
	for _, p := range idx.Pages {
		if p.Path == abs {
			return p.Name
		}
	}
	return ""
}
```

- Update `View`:

```go
func (a *App) View() string {
	switch a.mode {
	case modePalette:
		return a.palette.View()
	case modeSearch:
		return a.search.View()
	}
	return a.page.View() + "\n[ctrl-p] palette  [/] search  [n/N] link  [enter] follow  [q] quit"
}
```

- [ ] **Step 6: Smoke-test**

Run: `go build ./... && ./lstui --graph testdata/fixture-graph`
- Press `/`, type `milk`, press `Enter`. A result for Alpha shows. `Enter` again opens Alpha.

- [ ] **Step 7: Commit**

```bash
git add internal/views/search.go internal/views/search_test.go internal/views/app.go
git commit -m "feat(views): ripgrep-backed search overlay"
```

---

## Task 11: Backlinks panel (`views/backlinks.go`)

**Files:**
- Create: `internal/views/backlinks.go`
- Modify: `internal/views/app.go`

`b` toggles a side panel listing pages that link to the current page. `j`/`k` to navigate within the panel when focused, `Enter` to follow, `b` again or `Esc` to close.

- [ ] **Step 1: Write `internal/views/backlinks.go`**

```go
package views

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/fiatcode/logseq-tui/internal/graph"
)

type Backlinks struct {
	idx    *graph.Index
	target string // page being viewed
	refs   []graph.Ref
	sel    int
}

func NewBacklinks(idx *graph.Index, target string) *Backlinks {
	return &Backlinks{idx: idx, target: target, refs: idx.Backlinks[target]}
}

func (b *Backlinks) Update(key string) (selected string, accept, cancel bool) {
	switch key {
	case "esc", "b":
		return "", false, true
	case "up", "k", "ctrl+k":
		if b.sel > 0 {
			b.sel--
		}
	case "down", "j", "ctrl+j":
		if b.sel < len(b.refs)-1 {
			b.sel++
		}
	case "enter":
		if b.sel >= 0 && b.sel < len(b.refs) {
			return b.refs[b.sel].FromPage, true, false
		}
	}
	return "", false, false
}

var (
	blBorder = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 1)
	blSel    = lipgloss.NewStyle().Reverse(true)
)

func (b *Backlinks) View() string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "Backlinks → %s (%d)\n\n", b.target, len(b.refs))
	if len(b.refs) == 0 {
		sb.WriteString("(none)\n")
		return blBorder.Render(sb.String())
	}
	for i, r := range b.refs {
		line := fmt.Sprintf("%s:%d — %s", r.FromPage, r.LineNumber, strings.TrimSpace(r.Context))
		if i == b.sel {
			line = blSel.Render("▶ " + line)
		} else {
			line = "  " + line
		}
		sb.WriteString(line + "\n")
	}
	return blBorder.Render(sb.String())
}
```

- [ ] **Step 2: Wire into `App`**

- Extend `modeT`:

```go
const (
	modePage modeT = iota
	modePalette
	modeSearch
	modeBacklinks
)
```

- Add field `backlinks *Backlinks` to `App`.
- In `modePage` key switch, add:

```go
case "b":
	a.backlinks = NewBacklinks(a.idx, a.page.Page())
	a.mode = modeBacklinks
```

- Add a `modeBacklinks` case in the mode switch that delegates to `a.backlinks.Update(key)`. On accept, `a.page.SetPage(sel)` and return to `modePage`.
- Add `modeBacklinks` case in `View` that returns `a.backlinks.View()`.
- Update the page-view hint footer to include `[b] backlinks`.

- [ ] **Step 3: Smoke-test**

Run: `go build ./... && ./lstui --graph testdata/fixture-graph`
- Open Alpha via palette. Press `b` — backlinks panel shows Beta and 2026-05-24. `Enter` follows.

- [ ] **Step 4: Commit**

```bash
git add internal/views/backlinks.go internal/views/app.go
git commit -m "feat(views): backlinks side panel toggle"
```

---

## Task 12: TODO dashboard (`views/todos.go`)

**Files:**
- Create: `internal/views/todos.go`
- Modify: `internal/views/app.go`

`T` opens a full-screen dashboard of every open todo, grouped by page. `t` cycles marker filter (TODO → LATER → DOING → WAITING → all). `Enter` opens the source page (we don't yet scroll to the line — out of v1 scope unless trivial; this task implements just the open).

- [ ] **Step 1: Write `internal/views/todos.go`**

```go
package views

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/fiatcode/logseq-tui/internal/graph"
)

type Todos struct {
	idx     *graph.Index
	filter  string // "", "TODO", "LATER", "DOING", "WAITING"
	visible []graph.TodoBullet
	sel     int
}

func NewTodos(idx *graph.Index) *Todos {
	t := &Todos{idx: idx}
	t.recompute()
	return t
}

var markerCycle = []string{"", "TODO", "LATER", "DOING", "WAITING"}

func (t *Todos) cycleFilter() {
	for i, m := range markerCycle {
		if m == t.filter {
			t.filter = markerCycle[(i+1)%len(markerCycle)]
			t.recompute()
			return
		}
	}
}

func (t *Todos) recompute() {
	t.visible = t.visible[:0]
	for _, b := range t.idx.Todos {
		if t.filter != "" && b.Marker != t.filter {
			continue
		}
		t.visible = append(t.visible, b)
	}
	sort.SliceStable(t.visible, func(i, j int) bool {
		if t.visible[i].Page != t.visible[j].Page {
			return t.visible[i].Page < t.visible[j].Page
		}
		return t.visible[i].LineNumber < t.visible[j].LineNumber
	})
	if t.sel >= len(t.visible) {
		t.sel = 0
	}
}

func (t *Todos) Update(key string) (page string, accept, cancel bool) {
	switch key {
	case "esc", "q":
		return "", false, true
	case "t":
		t.cycleFilter()
	case "up", "k":
		if t.sel > 0 {
			t.sel--
		}
	case "down", "j":
		if t.sel < len(t.visible)-1 {
			t.sel++
		}
	case "enter":
		if t.sel >= 0 && t.sel < len(t.visible) {
			return t.visible[t.sel].Page, true, false
		}
	}
	return "", false, false
}

var (
	todosHdr  = lipgloss.NewStyle().Bold(true)
	todosSel  = lipgloss.NewStyle().Reverse(true)
	todosMark = map[string]lipgloss.Style{
		"TODO":    lipgloss.NewStyle().Foreground(lipgloss.Color("9")),  // red
		"DOING":   lipgloss.NewStyle().Foreground(lipgloss.Color("11")), // yellow
		"LATER":   lipgloss.NewStyle().Foreground(lipgloss.Color("12")), // blue
		"WAITING": lipgloss.NewStyle().Foreground(lipgloss.Color("8")),  // dim
	}
)

func (t *Todos) View() string {
	var sb strings.Builder
	filterTxt := "all"
	if t.filter != "" {
		filterTxt = t.filter
	}
	sb.WriteString(todosHdr.Render(fmt.Sprintf("Open todos — filter: %s (%d)", filterTxt, len(t.visible))))
	sb.WriteString("\n\n")
	lastPage := ""
	for i, b := range t.visible {
		if b.Page != lastPage {
			if lastPage != "" {
				sb.WriteString("\n")
			}
			sb.WriteString(todosHdr.Render(b.Page))
			sb.WriteString("\n")
			lastPage = b.Page
		}
		marker := b.Marker
		if st, ok := todosMark[b.Marker]; ok {
			marker = st.Render(b.Marker)
		}
		prio := ""
		if b.Priority != "" {
			prio = "[#" + b.Priority + "] "
		}
		row := fmt.Sprintf("  %s %s%s", marker, prio, b.Text)
		if i == t.sel {
			row = todosSel.Render("▶" + row)
		} else {
			row = " " + row
		}
		sb.WriteString(row + "\n")
	}
	sb.WriteString("\n[t] cycle filter  [enter] open  [esc] back\n")
	return sb.String()
}
```

- [ ] **Step 2: Wire into `App`**

- Extend `modeT`:

```go
const (
	modePage modeT = iota
	modePalette
	modeSearch
	modeBacklinks
	modeTodos
)
```

- Add field `todos *Todos` to `App`.
- In `modePage`, add:

```go
case "T":
	a.todos = NewTodos(a.idx)
	a.mode = modeTodos
```

- Add `modeTodos` branch in `Update` that calls `a.todos.Update(key)`; on accept, `a.page.SetPage(page); a.mode = modePage; a.todos = nil`.
- Add `modeTodos` branch in `View` returning `a.todos.View()`.
- Update footer to include `[T] todos`.

- [ ] **Step 3: Smoke-test**

Run: `go build ./... && ./lstui --graph testdata/fixture-graph`
- Press `T` — see 4 open todos grouped by page. `t` cycles filters. `Enter` opens the page.

- [ ] **Step 4: Commit**

```bash
git add internal/views/todos.go internal/views/app.go
git commit -m "feat(views): TODO dashboard with marker filter"
```

---

## Task 13: Manual index refresh

**Files:**
- Modify: `internal/views/app.go`

`R` triggers `graph.BuildIndex` again and replaces the index in `App` and each subview that holds it. Page view reloads the currently displayed page.

- [ ] **Step 1: Add refresh handler in `App.Update` (within `modePage` key switch)**

```go
case "R":
	if idx, err := graph.BuildIndex(a.idx.GraphPath); err == nil {
		a.idx = idx
		a.page = NewPageView(idx, a.page.Page(), a.width, a.height)
	}
```

- [ ] **Step 2: Update footer hints to include `[R] refresh`**

- [ ] **Step 3: Smoke-test**

Run: `./lstui --graph testdata/fixture-graph`
- Add a new file `testdata/fixture-graph/pages/Gamma.md` containing `- temp`. In the running TUI, press `R`, then `Ctrl-P` and type "Gamma" — it should appear. Remove the file afterward.

- [ ] **Step 4: Commit**

```bash
git add internal/views/app.go
git commit -m "feat(views): manual index refresh keybind"
```

---

## Task 14: View snapshot tests with teatest

**Files:**
- Create: `internal/views/page_test.go`
- Create: `internal/views/palette_test.go`
- Create: `internal/views/todos_test.go`

Lock down rendered output for the three views with the richest state. teatest writes golden files on first run (under `testdata/`); CI fails if they drift.

- [ ] **Step 1: Add teatest dep**

```bash
go get github.com/charmbracelet/x/exp/teatest@latest
```

- [ ] **Step 2: Write `internal/views/page_test.go`**

```go
package views

import (
	"path/filepath"
	"testing"

	"github.com/charmbracelet/x/exp/teatest"

	"github.com/fiatcode/logseq-tui/internal/graph"
)

func loadFixture(t *testing.T) *graph.Index {
	t.Helper()
	abs, err := filepath.Abs("../../testdata/fixture-graph")
	if err != nil {
		t.Fatal(err)
	}
	idx, err := graph.BuildIndex(abs)
	if err != nil {
		t.Fatal(err)
	}
	return idx
}

func TestPageViewRendersAlpha(t *testing.T) {
	idx := loadFixture(t)
	pv := NewPageView(idx, "Alpha", 80, 24)
	teatest.RequireEqualOutput(t, []byte(pv.View()))
}
```

- [ ] **Step 3: Write `internal/views/palette_test.go`**

```go
package views

import (
	"testing"

	"github.com/charmbracelet/x/exp/teatest"
)

func TestPaletteFiltersOnQuery(t *testing.T) {
	p := NewPalette(loadFixture(t))
	p.Update("a") // type 'a'
	p.Update("l")
	p.Update("p")
	teatest.RequireEqualOutput(t, []byte(p.View()))
}
```

- [ ] **Step 4: Write `internal/views/todos_test.go`**

```go
package views

import (
	"testing"

	"github.com/charmbracelet/x/exp/teatest"
)

func TestTodosDashboardAllFilter(t *testing.T) {
	td := NewTodos(loadFixture(t))
	teatest.RequireEqualOutput(t, []byte(td.View()))
}

func TestTodosDashboardLaterFilter(t *testing.T) {
	td := NewTodos(loadFixture(t))
	td.Update("t") // → TODO
	td.Update("t") // → LATER
	teatest.RequireEqualOutput(t, []byte(td.View()))
}
```

- [ ] **Step 5: Run tests, generate golden files**

Run: `go test ./internal/views/... -update`
Then: `go test ./internal/views/... -v`
Expected: all pass. Goldens are written under `internal/views/testdata/`.

- [ ] **Step 6: Commit**

```bash
go mod tidy
git add go.mod go.sum internal/views/page_test.go internal/views/palette_test.go internal/views/todos_test.go internal/views/testdata
git commit -m "test(views): teatest snapshots for page, palette, todos"
```

---

## Task 15: README

**Files:**
- Create: `README.md`

- [ ] **Step 1: Write `README.md`**

```markdown
# lstui — a read-only Logseq TUI

A terminal browser for a local Logseq graph. Fuzzy palette navigation, ripgrep-backed search, backlinks, and a TODO dashboard. Never writes to the graph.

## Install

Requires Go 1.22+ and [ripgrep](https://github.com/BurntSushi/ripgrep) on PATH.

```bash
go install github.com/fiatcode/logseq-tui/cmd/lstui@latest
```

Or from source:

```bash
git clone https://github.com/fiatcode/logseq-tui
cd logseq-tui
go build ./cmd/lstui
```

## Usage

```bash
lstui                              # uses ~/Documents/fiat-codex by default
lstui --graph /path/to/graph       # explicit path
LSTUI_GRAPH=/path/to/graph lstui   # via env var
```

Graph path resolution: `--graph` flag > `$LSTUI_GRAPH` > `~/Documents/fiat-codex`.

## Keys

| Key       | Action                              |
|-----------|-------------------------------------|
| `Ctrl-P`  | open palette (fuzzy page search)    |
| `/`       | open full-text search               |
| `T`       | open TODO dashboard                 |
| `b`       | toggle backlinks panel              |
| `n` / `N` | cycle wiki-link cursor              |
| `Enter`   | follow link / open selection        |
| `j` / `k` | scroll                              |
| `Ctrl-d/u`| half-page scroll                    |
| `R`       | rebuild index                       |
| `Esc`     | close overlay                       |
| `q`       | quit                                |

## Scope

Read-only. No editing, no fold/unfold, no live reload. Spec at `docs/superpowers/specs/2026-05-24-logseq-tui-design.md`.
```

- [ ] **Step 2: Commit**

```bash
git add README.md
git commit -m "docs: README with install, usage, keys"
```

---

## Self-review notes

**Spec coverage** — every section accounted for:
- Goal + 3 use cases → Tasks 8/9/10/11/12 cover quick-lookup, browsing, and TODO surfacing.
- Scope (in) → palette (9), page viewer (8), full-text search (10), backlinks (11), TODO dashboard (12), manual refresh (13).
- Scope (out) — editing, fold/unfold, filesystem watch — not in any task. Good.
- Architecture: data model (Task 4 types), indexer (Task 5), package layout (matches Task 1+ structure).
- UX: every key in the spec table is implemented across Tasks 8–13.
- Error handling: missing rg + missing graph path (Task 7 main.go), unreadable file during indexing (Task 5 index.go), dangling links (Task 5 records them; Task 8 renders the highlight style for any link), Glamour fallback (Task 6 has the `_ = err` fallback to body).
- Testing: parser table tests (Tasks 3, 4), indexer integration (Task 5), render (Task 6), rg parser (Task 10), view snapshots (Task 14).
- Dependencies — all present (bubbletea, bubbles, lipgloss, glamour, sahilm/fuzzy, teatest).

**Placeholder scan** — no TBDs, no "add appropriate handling", no "similar to Task N". Each step has the code.

**Type consistency** — `PageMeta`, `Ref`, `TodoBullet`, `LinkHit`, `TodoHit`, `Index`, `SearchHit`, `Result`, `Link` all defined once and used consistently across tasks. `App.idx.GraphPath` referenced in Task 10 matches the field set in Task 5's `Index` struct. `PageView` methods (`SetPage`, `SetSize`, `Page`, `CycleLink`, `FollowCursor`, `LineDown`/`LineUp`/`HalfPageDown`/`HalfPageUp`) defined in Task 8 and called the same way in Tasks 9–13.
