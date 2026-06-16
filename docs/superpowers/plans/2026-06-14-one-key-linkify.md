# One-key Linkify Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** From the backlinks panel's *Unlinked references* section, pressing `l` (then confirming) wraps the selected bare-text mention as `[[<matched>]]` in its source file, reindexes, and refreshes the panel in place.

**Architecture:** A new pure function `graph.LinkifyMention` does the byte-accurate splice (fresh re-match on the raw line, absolute-offset splice). The `Backlinks` overlay grows a preview/confirm sub-state and reports a `Linkify` request; the App performs the read → splice → `edit.WriteFile` → synchronous reindex → overlay rebuild, surfacing failures inside the panel.

**Tech Stack:** Go 1.26, charmbracelet/{bubbletea,bubbles,lipgloss}, `charmbracelet/x/exp/teatest` for goldens, ripgrep (`rg`) for mention detection.

**Spec:** `docs/superpowers/specs/2026-06-14-one-key-linkify-design.md`

**Worktree:** `.worktrees/linkify`, branch `feat/linkify`. Every subagent's first action must be `cd /var/home/dhemas/Development/Projects/fiatcode/peekseq/.worktrees/linkify && pwd`.

**Pre-commit gate (every commit):** `go vet ./... && go test ./...` — both must pass.

---

## File Structure

- **Create** `internal/graph/linkify.go` — `LinkifyMention`, `ErrMentionNotFound`, the raw-line matcher, and a local copy of the Unicode word-boundary helpers (`wholeWordAt`/`isWordRune`). Pure; no IO.
- **Create** `internal/graph/linkify_test.go` — unit tests for the above.
- **Modify** `internal/views/overlay.go` — add `Linkify` + `LinkifyTarget` fields to `OverlayResult`.
- **Modify** `internal/views/backlinks.go` — confirm sub-state (`confirming`, `errMsg`), `l` handling, `SetLinkifyError`, confirm/error rendering, and the dynamic hint line.
- **Modify** `internal/views/backlinks_test.go` — confirm-state Update tests + a confirm-view golden.
- **Modify** `internal/views/app.go` — extract `(a *App) reindex()`; add the `res.Linkify` dispatch branch.
- **Modify** `internal/views/app_test.go` — end-to-end `l → y` linkify test and a "mention gone" error test.

---

## Task 1: `graph.LinkifyMention` (pure splice + matcher)

**Files:**
- Create: `internal/graph/linkify.go`
- Test: `internal/graph/linkify_test.go`

- [ ] **Step 1: Write the failing tests**

Create `internal/graph/linkify_test.go`:

```go
package graph

import (
	"errors"
	"testing"
)

func TestLinkifyMentionHappyPath(t *testing.T) {
	body := "line one\nthe Alpha ship date\nline three\n"
	got, span, err := LinkifyMention(body, 2, "Alpha")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "line one\nthe [[Alpha]] ship date\nline three\n"
	if got != want {
		t.Errorf("body =\n%q\nwant\n%q", got, want)
	}
	if body[span.Start:span.End] != "Alpha" {
		t.Errorf("span %v points at %q, want \"Alpha\"", span, body[span.Start:span.End])
	}
}

func TestLinkifyMentionPreservesCasing(t *testing.T) {
	got, _, err := LinkifyMention("a bare alpha here\n", 1, "Alpha")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := "a bare [[alpha]] here\n"; got != want {
		t.Errorf("got %q, want %q (author's casing must be preserved)", got, want)
	}
}

func TestLinkifyMentionPreservesCRLF(t *testing.T) {
	body := "intro\r\nthe Alpha ship\r\nend\r\n"
	got, _, err := LinkifyMention(body, 2, "Alpha")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := "intro\r\nthe [[Alpha]] ship\r\nend\r\n"; got != want {
		t.Errorf("got %q, want %q (CRLF must survive)", got, want)
	}
}

func TestLinkifyMentionNoTrailingNewline(t *testing.T) {
	got, _, err := LinkifyMention("only Alpha", 1, "Alpha")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := "only [[Alpha]]"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestLinkifyMentionFirstOccurrenceWins(t *testing.T) {
	got, _, err := LinkifyMention("Alpha and Alpha again\n", 1, "Alpha")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := "[[Alpha]] and Alpha again\n"; got != want {
		t.Errorf("got %q, want %q (only the first occurrence is wrapped)", got, want)
	}
}

func TestLinkifyMentionSkipsAlreadyLinked(t *testing.T) {
	// The only occurrence is already inside [[…]]; nothing left to wrap.
	_, _, err := LinkifyMention("see [[Alpha]] there\n", 1, "Alpha")
	if !errors.Is(err, ErrMentionNotFound) {
		t.Errorf("err = %v, want ErrMentionNotFound (must not double-wrap)", err)
	}
}

func TestLinkifyMentionSkipsLinkedTakesNextBare(t *testing.T) {
	got, _, err := LinkifyMention("[[Alpha]] then bare Alpha\n", 1, "Alpha")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := "[[Alpha]] then bare [[Alpha]]\n"; got != want {
		t.Errorf("got %q, want %q (skip the linked one, wrap the bare one)", got, want)
	}
}

func TestLinkifyMentionWholeWordOnly(t *testing.T) {
	// "Alphabet" must not match the whole word "Alpha".
	_, _, err := LinkifyMention("the Alphabet song\n", 1, "Alpha")
	if !errors.Is(err, ErrMentionNotFound) {
		t.Errorf("err = %v, want ErrMentionNotFound (substring must not match)", err)
	}
}

func TestLinkifyMentionUnicodeWord(t *testing.T) {
	got, _, err := LinkifyMention("schön Über alles\n", 1, "Über")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := "schön [[Über]] alles\n"; got != want {
		t.Errorf("got %q, want %q (Unicode word boundary)", got, want)
	}
}

func TestLinkifyMentionLineOutOfRange(t *testing.T) {
	_, _, err := LinkifyMention("just one line\n", 9, "Alpha")
	if !errors.Is(err, ErrMentionNotFound) {
		t.Errorf("err = %v, want ErrMentionNotFound for out-of-range line", err)
	}
}

func TestLinkifyMentionMentionGone(t *testing.T) {
	_, _, err := LinkifyMention("nothing relevant here\n", 1, "Alpha")
	if !errors.Is(err, ErrMentionNotFound) {
		t.Errorf("err = %v, want ErrMentionNotFound when line lacks the mention", err)
	}
}

func TestLinkifyMentionRegexMetaTarget(t *testing.T) {
	// A name with regex metacharacters must be matched literally.
	got, _, err := LinkifyMention("about C++ today\n", 1, "C++")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := "about [[C++]] today\n"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `cd /var/home/dhemas/Development/Projects/fiatcode/peekseq/.worktrees/linkify && go test ./internal/graph/ -run TestLinkifyMention -v`
Expected: FAIL — `undefined: LinkifyMention` / `undefined: ErrMentionNotFound`.

- [ ] **Step 3: Write the implementation**

Create `internal/graph/linkify.go`:

```go
package graph

import (
	"errors"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"git.fiatcode.dev/fiatcode/peekseq/internal/search"
)

// ErrMentionNotFound is returned by LinkifyMention when the target line is out
// of range or holds no unlinked, whole-word occurrence of the name to wrap.
var ErrMentionNotFound = errors.New("mention not found")

// LinkifyMention wraps the first unlinked, whole-word, case-insensitive
// occurrence of target on the 1-based line in body as [[<matched>]]. The matched
// bytes are preserved verbatim (Resolve is fold-keyed, so casing need not be
// normalised). The splice is by absolute byte offset, so line endings (incl.
// CRLF) and any trailing newline are preserved untouched. span is the matched
// range in the original body. It returns ErrMentionNotFound when no such
// occurrence exists — which also guards against double-wrapping an already
// [[linked]] mention and against a line that changed since detection.
func LinkifyMention(body string, line int, target string) (string, search.Span, error) {
	lineStart, lineEnd, ok := lineBounds(body, line)
	if !ok {
		return "", search.Span{}, ErrMentionNotFound
	}
	rel, ok := firstUnlinkedOccurrence(body[lineStart:lineEnd], target)
	if !ok {
		return "", search.Span{}, ErrMentionNotFound
	}
	start, end := lineStart+rel.Start, lineStart+rel.End
	newBody := body[:start] + "[[" + body[start:end] + "]]" + body[end:]
	return newBody, search.Span{Start: start, End: end}, nil
}

// lineBounds returns the [start, end) byte offsets of the 1-based line's content
// (excluding the trailing newline) within body. ok is false when line < 1 or
// runs past the end of body.
func lineBounds(body string, line int) (start, end int, ok bool) {
	if line < 1 {
		return 0, 0, false
	}
	for cur := 1; cur < line; cur++ {
		i := strings.IndexByte(body[start:], '\n')
		if i < 0 {
			return 0, 0, false
		}
		start += i + 1
	}
	if i := strings.IndexByte(body[start:], '\n'); i >= 0 {
		return start, start + i, true
	}
	return start, len(body), true
}

// firstUnlinkedOccurrence finds the first whole-word, case-insensitive, literal
// occurrence of target in line that is not inside a [[…]] link. The returned
// span is relative to line.
func firstUnlinkedOccurrence(line, target string) (search.Span, bool) {
	re := regexp.MustCompile("(?i)" + regexp.QuoteMeta(target))
	locs := re.FindAllStringIndex(line, -1)
	if locs == nil {
		return search.Span{}, false
	}
	links := wikiLinkRe.FindAllStringIndex(line, -1)
	for _, loc := range locs {
		start, end := loc[0], loc[1]
		if !wholeWordAt(line, start, end) {
			continue
		}
		inside := false
		for _, l := range links {
			if start >= l[0] && end <= l[1] {
				inside = true
				break
			}
		}
		if !inside {
			return search.Span{Start: start, End: end}, true
		}
	}
	return search.Span{}, false
}

// wholeWordAt reports whether s[start:end] is bounded by string edges or
// non-word runes on both sides (Unicode-aware), mirroring ripgrep -w. This is a
// deliberate local copy of the identical helper in internal/render: graph must
// not depend on render, and a shared package for two ~5-line helpers would be
// premature (cf. the theme-color note in theme.go).
func wholeWordAt(s string, start, end int) bool {
	if start > 0 {
		if r, _ := utf8.DecodeLastRuneInString(s[:start]); isWordRune(r) {
			return false
		}
	}
	if end < len(s) {
		if r, _ := utf8.DecodeRuneInString(s[end:]); isWordRune(r) {
			return false
		}
	}
	return true
}

func isWordRune(r rune) bool {
	return r == '_' || unicode.IsLetter(r) || unicode.IsNumber(r)
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `cd /var/home/dhemas/Development/Projects/fiatcode/peekseq/.worktrees/linkify && go test ./internal/graph/ -run TestLinkifyMention -v`
Expected: PASS (all 12 cases).

- [ ] **Step 5: Run the gate and commit**

```bash
cd /var/home/dhemas/Development/Projects/fiatcode/peekseq/.worktrees/linkify
go vet ./... && go test ./...
git add internal/graph/linkify.go internal/graph/linkify_test.go
git commit -m "feat: graph.LinkifyMention — pure byte-accurate [[…]] splice"
```

---

## Task 2: `OverlayResult` fields + `Backlinks` confirm sub-state

**Files:**
- Modify: `internal/views/overlay.go`
- Modify: `internal/views/backlinks.go`
- Test: `internal/views/backlinks_test.go`

- [ ] **Step 1: Write the failing tests**

Append to `internal/views/backlinks_test.go`:

```go
func TestBacklinksLOnUnlinkedEntersConfirm(t *testing.T) {
	t.Setenv("TERM", "dumb")
	b := NewBacklinks(loadFixture(t), "Hub", unlinkedFixture(), 80, 30)
	// Move to the first unlinked row (past the linked refs + the section header).
	for !b.rows[b.sel].unlinkedRow() {
		b.moveSel(+1)
	}
	if res := b.Update("l"); res.Linkify != nil {
		t.Fatalf("first l should only open the confirm, not request linkify: %+v", res)
	}
	if !b.confirming {
		t.Fatal("l on an unlinked row should enter the confirm sub-state")
	}
}

func TestBacklinksLOnLinkedRowIsNoop(t *testing.T) {
	t.Setenv("TERM", "dumb")
	b := NewBacklinks(loadFixture(t), "Hub", unlinkedFixture(), 80, 30)
	// sel starts on the first selectable row, which is a linked ref.
	if b.rows[b.sel].unlinkedRow() {
		t.Skip("fixture's first selectable row is unexpectedly unlinked")
	}
	b.Update("l")
	if b.confirming {
		t.Error("l on a linked row must not enter the confirm sub-state")
	}
}

func TestBacklinksConfirmYesReturnsLinkify(t *testing.T) {
	t.Setenv("TERM", "dumb")
	b := NewBacklinks(loadFixture(t), "Hub", unlinkedFixture(), 80, 30)
	for !b.rows[b.sel].unlinkedRow() {
		b.moveSel(+1)
	}
	b.Update("l")
	res := b.Update("y")
	if res.Linkify == nil {
		t.Fatal("y in confirm should return a Linkify request")
	}
	if res.LinkifyTarget != "Hub" {
		t.Errorf("LinkifyTarget = %q, want \"Hub\"", res.LinkifyTarget)
	}
	if b.confirming {
		t.Error("confirming should be cleared after y")
	}
}

func TestBacklinksConfirmEscCancels(t *testing.T) {
	t.Setenv("TERM", "dumb")
	b := NewBacklinks(loadFixture(t), "Hub", unlinkedFixture(), 80, 30)
	for !b.rows[b.sel].unlinkedRow() {
		b.moveSel(+1)
	}
	b.Update("l")
	res := b.Update(keyEsc)
	if res.Cancel {
		t.Error("esc in confirm should cancel the confirm, not the whole panel")
	}
	if res.Linkify != nil {
		t.Error("esc in confirm must not request linkify")
	}
	if b.confirming {
		t.Error("esc in confirm should clear confirming")
	}
}

func TestBacklinksSetLinkifyErrorClearsConfirm(t *testing.T) {
	t.Setenv("TERM", "dumb")
	b := NewBacklinks(loadFixture(t), "Hub", unlinkedFixture(), 80, 30)
	for !b.rows[b.sel].unlinkedRow() {
		b.moveSel(+1)
	}
	b.Update("l")
	b.SetLinkifyError("mention no longer found in Beta")
	if b.confirming {
		t.Error("SetLinkifyError should drop the confirm sub-state")
	}
	if b.errMsg == "" {
		t.Error("SetLinkifyError should record the message")
	}
}
```

This uses a small `unlinkedRow()` helper on `blRow`; add it in Step 3.

- [ ] **Step 2: Run the tests to verify they fail**

Run: `cd /var/home/dhemas/Development/Projects/fiatcode/peekseq/.worktrees/linkify && go test ./internal/views/ -run TestBacklinks -v`
Expected: FAIL — `res.Linkify` / `res.LinkifyTarget` / `b.confirming` / `b.errMsg` / `b.SetLinkifyError` / `unlinkedRow` undefined.

- [ ] **Step 3: Implement — `OverlayResult` fields, then `Backlinks` state + Update**

In `internal/views/overlay.go`, add these fields to the `OverlayResult` struct (place them just before `Cmd tea.Cmd`):

```go
	// Linkify, when non-nil, asks the App to wrap LinkifyTarget as a [[link]]
	// at this unlinked reference's location in its source file, then reindex
	// and refresh the panel. Only the Backlinks overlay sets it.
	Linkify *graph.UnlinkedRef
	// LinkifyTarget is the page name to wrap, honoured only when Linkify is set.
	LinkifyTarget string
```

Add the `graph` import to `overlay.go`:

```go
import (
	tea "github.com/charmbracelet/bubbletea"

	"git.fiatcode.dev/fiatcode/peekseq/internal/graph"
)
```

In `internal/views/backlinks.go`, add the `unlinkedRow` helper after the `blRow` type:

```go
// unlinkedRow reports whether the row is a selectable unlinked reference.
func (r blRow) unlinkedRow() bool { return r.unl != nil }
```

Add the two state fields to the `Backlinks` struct (after `rows []blRow`):

```go
	confirming bool   // l pressed on an unlinked row; preview/confirm is showing
	errMsg     string // failure feedback, rendered inside the panel
```

Replace the whole `Update` method with:

```go
func (b *Backlinks) Update(key string) OverlayResult {
	if b.confirming {
		switch key {
		case keyEnter, "y":
			r := b.rows[b.sel]
			b.confirming = false
			return OverlayResult{Linkify: r.unl, LinkifyTarget: b.target}
		case keyEsc, "n":
			b.confirming = false
		}
		return OverlayResult{}
	}
	switch key {
	case keyEsc, "b":
		return OverlayResult{Cancel: true}
	case keyUp, keyK, keyCtrlK:
		b.errMsg = ""
		b.moveSel(-1)
	case keyDown, keyJ, keyCtrlJ:
		b.errMsg = ""
		b.moveSel(+1)
	case "l":
		if b.sel >= 0 && b.sel < len(b.rows) && b.rows[b.sel].unlinkedRow() {
			b.errMsg = ""
			b.confirming = true
		}
	case keyEnter:
		if b.sel >= 0 && b.sel < len(b.rows) {
			r := b.rows[b.sel]
			if r.ref != nil {
				return OverlayResult{Selected: r.ref.FromPage, Accept: true, FocusLinkTo: b.target}
			}
			if r.unl != nil {
				return OverlayResult{Selected: r.unl.PageName, Accept: true, HighlightText: b.target}
			}
		}
	}
	return OverlayResult{}
}

// SetLinkifyError records a failure to show in the panel and drops the confirm
// sub-state, so the user returns to the list rather than being stuck confirming
// a mention that is gone. Errors must render inside the overlay — a status-bar
// hint would be invisible behind it.
func (b *Backlinks) SetLinkifyError(msg string) {
	b.errMsg = msg
	b.confirming = false
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `cd /var/home/dhemas/Development/Projects/fiatcode/peekseq/.worktrees/linkify && go test ./internal/views/ -run TestBacklinks -v`
Expected: PASS (existing Backlinks tests still pass; the 5 new ones pass).

- [ ] **Step 5: Run the gate and commit**

```bash
cd /var/home/dhemas/Development/Projects/fiatcode/peekseq/.worktrees/linkify
go vet ./... && go test ./...
git add internal/views/overlay.go internal/views/backlinks.go internal/views/backlinks_test.go
git commit -m "feat: backlinks linkify confirm sub-state + Linkify result"
```

---

## Task 3: Confirm/error rendering + dynamic hint

**Files:**
- Modify: `internal/views/backlinks.go`
- Test: `internal/views/backlinks_test.go`

- [ ] **Step 1: Write the failing tests**

Append to `internal/views/backlinks_test.go`:

```go
func TestBacklinksConfirmViewGolden(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	b := NewBacklinks(loadFixture(t), "Hub", unlinkedFixture(), 100, 30)
	for !b.rows[b.sel].unlinkedRow() {
		b.moveSel(+1)
	}
	b.Update("l")
	teatest.RequireEqualOutput(t, []byte(b.View()))
}

func TestBacklinksConfirmViewShowsBeforeAfter(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	b := NewBacklinks(loadFixture(t), "Hub", unlinkedFixture(), 100, 30)
	for !b.rows[b.sel].unlinkedRow() {
		b.moveSel(+1)
	}
	b.Update("l")
	out := b.View()
	if !strings.Contains(out, "before:") || !strings.Contains(out, "after:") {
		t.Errorf("confirm view should show before/after lines:\n%s", out)
	}
	if !strings.Contains(out, "[[Hub]]") {
		t.Errorf("confirm 'after' line should show the wrapped mention:\n%s", out)
	}
	if !strings.Contains(out, "y confirm") {
		t.Errorf("confirm view should show the y/n hint:\n%s", out)
	}
}

func TestBacklinksErrorLineRendered(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	b := NewBacklinks(loadFixture(t), "Hub", unlinkedFixture(), 100, 30)
	b.SetLinkifyError("mention no longer found in Beta")
	if !strings.Contains(b.View(), "mention no longer found in Beta") {
		t.Errorf("error message should render in the panel:\n%s", b.View())
	}
}

func TestBacklinksHintShowsLinkifyOnUnlinkedRow(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	b := NewBacklinks(loadFixture(t), "Hub", unlinkedFixture(), 100, 30)
	for !b.rows[b.sel].unlinkedRow() {
		b.moveSel(+1)
	}
	if !strings.Contains(b.View(), "l linkify") {
		t.Errorf("hint should advertise linkify when an unlinked row is selected:\n%s", b.View())
	}
}
```

Confirm `internal/views/backlinks_test.go` imports `"strings"` (it already imports `teatest`); add `"strings"` to its import block if missing.

- [ ] **Step 2: Run the tests to verify they fail**

Run: `cd /var/home/dhemas/Development/Projects/fiatcode/peekseq/.worktrees/linkify && go test ./internal/views/ -run TestBacklinks -v`
Expected: FAIL — the confirm/error/linkify-hint strings are absent from `View()` (and the new golden file does not exist yet).

- [ ] **Step 3: Implement the rendering**

In `internal/views/backlinks.go`, replace the tail of `View()` — these final lines:

```go
	sb.WriteString("\n")
	sb.WriteString(styleFaint.Render(clamp("↑/↓ select · enter open · b or esc close", inner)))
	return styleBorder.Width(inner + 4).Render(sb.String())
```

with:

```go
	sb.WriteString("\n")
	switch {
	case b.confirming:
		b.writeConfirm(&sb, inner)
	case b.errMsg != "":
		sb.WriteString(styleTitle.Render(clamp("linkify failed: "+b.errMsg, inner)))
		sb.WriteString("\n")
		sb.WriteString(styleFaint.Render(clamp(b.hintText(), inner)))
	default:
		sb.WriteString(styleFaint.Render(clamp(b.hintText(), inner)))
	}
	return styleBorder.Width(inner + 4).Render(sb.String())
```

Add these two methods after `View()`:

```go
// hintText returns the footer key legend, advertising linkify only when the
// selected row is an unlinked reference (the only row l acts on).
func (b *Backlinks) hintText() string {
	if b.sel >= 0 && b.sel < len(b.rows) && b.rows[b.sel].unlinkedRow() {
		return "↑/↓ select · enter open · l linkify · b or esc close"
	}
	return "↑/↓ select · enter open · b or esc close"
}

// writeConfirm renders the preview-and-confirm block for the selected unlinked
// row. The before/after lines are computed purely from the row's Context and
// Match span — no file IO; the authoritative re-match happens at write time.
func (b *Backlinks) writeConfirm(sb *strings.Builder, inner int) {
	u := b.rows[b.sel].unl
	before := u.Context
	after := u.Context[:u.Match.Start] + "[[" + u.Context[u.Match.Start:u.Match.End] + "]]" + u.Context[u.Match.End:]
	sb.WriteString(styleFaint.Render(clamp(fmt.Sprintf("── Linkify in %s:%d ", u.PageName, u.Line), inner)))
	sb.WriteString("\n")
	sb.WriteString(clamp("  before:  "+strings.TrimSpace(before), inner))
	sb.WriteString("\n")
	sb.WriteString(clamp("  after:   "+strings.TrimSpace(after), inner))
	sb.WriteString("\n\n")
	sb.WriteString(styleFaint.Render(clamp("  y confirm · n/esc cancel", inner)))
}
```

(`fmt`, `strings`, `styleTitle`, `styleFaint`, and `clamp` are already imported/defined in this file.)

- [ ] **Step 4: Generate the golden, inspect it, and verify tests pass**

```bash
cd /var/home/dhemas/Development/Projects/fiatcode/peekseq/.worktrees/linkify
go test ./internal/views/ -run TestBacklinksConfirmViewGolden -update
```

Then **visually inspect** the new golden and check its widths (box-drawing chars mislead the eye):

```bash
cat internal/views/testdata/TestBacklinksConfirmViewGolden.golden
awk '{ print length, $0 }' internal/views/testdata/TestBacklinksConfirmViewGolden.golden | sort -rn | head -3
```

Confirm the box border is intact, the `── Linkify in Beta:3 ──`-style header, `before:`/`after:` lines, and the `y confirm · n/esc cancel` footer all appear and nothing overflows the border. Then:

Run: `cd /var/home/dhemas/Development/Projects/fiatcode/peekseq/.worktrees/linkify && go test ./internal/views/ -run TestBacklinks -v`
Expected: PASS (all Backlinks tests, including the 4 new ones).

- [ ] **Step 5: Run the gate and commit**

```bash
cd /var/home/dhemas/Development/Projects/fiatcode/peekseq/.worktrees/linkify
go vet ./... && go test ./...
git add internal/views/backlinks.go internal/views/backlinks_test.go internal/views/testdata/TestBacklinksConfirmViewGolden.golden
git commit -m "feat: backlinks confirm preview, error line, and linkify hint"
```

---

## Task 4: Extract `(a *App) reindex()`

**Files:**
- Modify: `internal/views/app.go`

This is a small, behavior-preserving refactor (no new test; the existing journal/edit tests cover it). It exists so the linkify path and the journal path share one synchronous-reindex implementation.

- [ ] **Step 1: Add the helper and route `createJournalAndReindex` through it**

In `internal/views/app.go`, replace the body of `createJournalAndReindex` (the part after `EnsureFile`) so the function reads:

```go
func (a *App) createJournalAndReindex(name string) error {
	journalPath := filepath.Join(a.graphPath, "journals", graph.FilenameFromPageName(name))
	if _, err := edit.EnsureFile(journalPath); err != nil {
		return fmt.Errorf("cannot create journal: %w", err)
	}
	if err := a.reindex(); err != nil {
		return fmt.Errorf("reindex failed: %w", err)
	}
	return nil
}

// reindex rebuilds the in-memory index synchronously and rebinds the current
// PageView so it reflects new links/todos. Shared by createJournalAndReindex
// and the linkify path, both of which mutate the graph while a view is open and
// need the refresh before returning (no indexLoadedMsg round-trip).
func (a *App) reindex() error {
	idx, err := graph.BuildIndex(a.graphPath)
	if err != nil {
		return err
	}
	a.idx = idx
	if a.page != nil {
		a.page = NewPageView(a.idx, a.page.Page(), a.width, a.height)
	}
	return nil
}
```

- [ ] **Step 2: Run the gate (no behavior change expected)**

Run: `cd /var/home/dhemas/Development/Projects/fiatcode/peekseq/.worktrees/linkify && go vet ./... && go test ./...`
Expected: PASS — the journal-create and editor-exit tests still pass unchanged.

- [ ] **Step 3: Commit**

```bash
cd /var/home/dhemas/Development/Projects/fiatcode/peekseq/.worktrees/linkify
git add internal/views/app.go
git commit -m "refactor: extract App.reindex() from createJournalAndReindex"
```

---

## Task 5: App linkify dispatch + end-to-end tests

**Files:**
- Modify: `internal/views/app.go`
- Test: `internal/views/app_test.go`

- [ ] **Step 1: Write the failing tests**

Append to `internal/views/app_test.go` (it already imports `os`, `exec`, `filepath`, `strings`, `tea`, `graph`, and has the `key` helper):

```go
func TestAppLinkifyWritesAndRefreshes(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	if _, err := exec.LookPath("rg"); err != nil {
		t.Skip("rg not on PATH; install ripgrep to run this test")
	}
	tmp := t.TempDir()
	pages := filepath.Join(tmp, "pages")
	if err := os.MkdirAll(pages, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pages, "Topic.md"), []byte("# Topic\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	notePath := filepath.Join(pages, "Note.md")
	if err := os.WriteFile(notePath, []byte("- a bare Topic mention\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	idx, err := graph.BuildIndex(tmp)
	if err != nil {
		t.Fatal(err)
	}
	a := New(tmp, "test")
	a.idx = idx
	a.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	a.page = NewPageView(idx, "Topic", 80, 24)

	a.Update(key("b"))                    // open backlinks for Topic
	a.Update(tea.KeyMsg{Type: tea.KeyDown}) // move onto the unlinked Note row
	a.Update(key("l"))                    // open the confirm
	a.Update(key("y"))                    // confirm the write

	got, err := os.ReadFile(notePath)
	if err != nil {
		t.Fatal(err)
	}
	if want := "- a bare [[Topic]] mention\n"; string(got) != want {
		t.Fatalf("Note.md = %q, want %q", string(got), want)
	}
	// Panel refreshed: the mention is now a linked ref, so it has dropped out
	// of the unlinked list.
	bl, ok := a.active.(*Backlinks)
	if !ok {
		t.Fatalf("backlinks overlay should still be open after linkify; got %T", a.active)
	}
	for _, r := range bl.rows {
		if r.unl != nil {
			t.Errorf("linkified mention should no longer appear as unlinked: %+v", r.unl)
		}
	}
}

func TestAppLinkifyMentionGoneShowsError(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	if _, err := exec.LookPath("rg"); err != nil {
		t.Skip("rg not on PATH; install ripgrep to run this test")
	}
	tmp := t.TempDir()
	pages := filepath.Join(tmp, "pages")
	if err := os.MkdirAll(pages, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pages, "Topic.md"), []byte("# Topic\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	notePath := filepath.Join(pages, "Note.md")
	if err := os.WriteFile(notePath, []byte("- a bare Topic mention\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	idx, err := graph.BuildIndex(tmp)
	if err != nil {
		t.Fatal(err)
	}
	a := New(tmp, "test")
	a.idx = idx
	a.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	a.page = NewPageView(idx, "Topic", 80, 24)

	a.Update(key("b"))
	a.Update(tea.KeyMsg{Type: tea.KeyDown})
	a.Update(key("l"))
	// The mention vanishes from the file between detection and confirm.
	if err := os.WriteFile(notePath, []byte("- nothing here now\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	a.Update(key("y"))

	bl, ok := a.active.(*Backlinks)
	if !ok {
		t.Fatalf("panel should stay open on error; got %T", a.active)
	}
	if bl.errMsg == "" {
		t.Error("a vanished mention should set an in-panel error message")
	}
	got, _ := os.ReadFile(notePath)
	if string(got) != "- nothing here now\n" {
		t.Errorf("file must be untouched when the mention is gone; got %q", string(got))
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `cd /var/home/dhemas/Development/Projects/fiatcode/peekseq/.worktrees/linkify && go test ./internal/views/ -run TestAppLinkify -v`
Expected: FAIL — the App ignores `res.Linkify`, so `Note.md` is never rewritten and no error is set.

- [ ] **Step 3: Implement the dispatch branch**

In `internal/views/app.go`, inside `Update`'s `if a.active != nil {` block, insert the linkify branch between the `res.Cancel` handling and the `res.Accept` handling. The block becomes:

```go
		// An open overlay swallows all keys until it accepts or cancels.
		if a.active != nil {
			res := a.active.Update(key)
			if res.Cancel {
				a.active = nil
				return a, res.Cmd
			}
			if res.Linkify != nil {
				return a, a.linkify(res.Linkify, res.LinkifyTarget)
			}
			if res.Accept {
				if res.Create {
					a.navigate(res.Selected)
					a.active = nil
					return a, a.enterEditor()
				}
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
				a.active = nil
			}
			return a, res.Cmd
		}
```

Add the `linkify` method (place it next to `unlinkedRefs`):

```go
// linkify wraps the unlinked reference's mention as a [[link]] in its source
// file, then reindexes and rebuilds the backlinks overlay so the reference
// moves from Unlinked to Linked. The file is re-read and re-matched here (not
// trusting the offset captured at panel-open) so a file that changed since
// detection fails safely. Failures render inside the panel via SetLinkifyError;
// a status-bar hint would be invisible behind the overlay. Returns nil — the
// reindex is synchronous, so there is no command to run.
func (a *App) linkify(ref *graph.UnlinkedRef, target string) tea.Cmd {
	bl, _ := a.active.(*Backlinks)
	fail := func(msg string) tea.Cmd {
		if bl != nil {
			bl.SetLinkifyError(msg)
		}
		return nil
	}
	body, err := os.ReadFile(ref.FilePath)
	if err != nil {
		return fail("cannot read " + ref.PageName + ": " + err.Error())
	}
	newBody, _, err := graph.LinkifyMention(string(body), ref.Line, target)
	if err != nil {
		return fail("mention no longer found in " + ref.PageName)
	}
	if err := edit.WriteFile(ref.FilePath, []byte(newBody)); err != nil {
		return fail("write failed: " + err.Error())
	}
	if err := a.reindex(); err != nil {
		return a.setHint("reindex failed: " + err.Error())
	}
	a.active = NewBacklinks(a.idx, target, a.unlinkedRefs(target), a.width, a.height)
	return nil
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `cd /var/home/dhemas/Development/Projects/fiatcode/peekseq/.worktrees/linkify && go test ./internal/views/ -run TestAppLinkify -v`
Expected: PASS (both the write+refresh and the mention-gone error tests).

- [ ] **Step 5: Run the full gate and commit**

```bash
cd /var/home/dhemas/Development/Projects/fiatcode/peekseq/.worktrees/linkify
go vet ./... && go test ./...
git add internal/views/app.go internal/views/app_test.go
git commit -m "feat: one-key linkify — write [[link]] into the source file and refresh"
```

---

## Self-Review (completed during planning)

**Spec coverage:** Linkify single occurrence (Tasks 2/3/5) ✓; preview+confirm (Tasks 2/3) ✓; authoritative re-match at write time (Task 1 `LinkifyMention` + Task 5 `linkify`) ✓; in-place synchronous refresh (Tasks 4/5) ✓; feedback inside overlay (Tasks 2/3/5 `SetLinkifyError`) ✓; preserve casing `[[<matched>]]` (Task 1) ✓; byte-offset/CRLF/trailing-newline (Task 1) ✓; `l` keybinding + ignored on linked/header rows (Task 2) ✓; out-of-scope items (bulk/page-view/undo) correctly absent ✓.

**Placeholder scan:** No TBD/TODO; every code step shows complete code; every command has expected output.

**Type consistency:** `LinkifyMention(string, int, string) (string, search.Span, error)` and `ErrMentionNotFound` used identically in Tasks 1 and 5. `OverlayResult.Linkify *graph.UnlinkedRef` / `LinkifyTarget string` defined in Task 2, consumed in Task 5. `Backlinks.confirming`/`errMsg`/`SetLinkifyError`/`unlinkedRow()`/`hintText()`/`writeConfirm()` defined in Tasks 2–3 and used consistently. `(a *App) reindex()` defined in Task 4, called in Task 5. `NewBacklinks(idx, target, unlinked, w, h)` signature matches existing usage.
