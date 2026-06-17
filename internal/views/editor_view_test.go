package views

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/exp/teatest"
	"github.com/muesli/termenv"
)

func TestEditorView_LoadsContentAndTracksDirty(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	e := NewEditorView(nil, "Alpha", "/tmp/Alpha.md", "first line\n", false, 80, 24)

	if got := e.Content(); got != "first line\n" {
		t.Errorf("Content: got %q, want %q", got, "first line\n")
	}
	if e.dirty() {
		t.Errorf("freshly loaded buffer should be clean")
	}

	e.ta.SetValue("first line\nsecond\n")
	if !e.dirty() {
		t.Errorf("buffer should be dirty after edit")
	}

	e.MarkSaved(e.Content())
	if e.dirty() {
		t.Errorf("buffer should be clean after MarkSaved")
	}
}

func TestEditorView_ContentEndsWithSingleNewline(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	e := NewEditorView(nil, "Alpha", "/tmp/Alpha.md", "", true, 80, 24)
	e.ta.SetValue("no trailing newline")
	if got := e.Content(); got != "no trailing newline\n" {
		t.Errorf("Content: got %q, want one trailing newline", got)
	}
	e.ta.SetValue("trailing blanks\n\n\n")
	if got := e.Content(); !strings.HasSuffix(got, "\n") || strings.HasSuffix(got, "\n\n") {
		t.Errorf("Content: got %q, want exactly one trailing newline", got)
	}
}

func TestEditorView_CleanAfterSaveWithoutTrailingNewline(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	e := NewEditorView(nil, "Alpha", "/tmp/a.md", "", true, 80, 24)
	e.ta.SetValue("no trailing newline") // user typed, no \n
	e.MarkSaved(e.Content())
	if e.dirty() {
		t.Errorf("buffer should be clean after MarkSaved even without a trailing newline")
	}
}

func TestEditorView_CleanOnOpenWhenFileLacksTrailingNewline(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	e := NewEditorView(nil, "Alpha", "/tmp/a.md", "loaded without newline", false, 80, 24)
	if e.dirty() {
		t.Errorf("freshly opened buffer should be clean even if the file lacked a trailing newline")
	}
}

func TestEditorViewOpensAtTop(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	content := "line 1\nline 2\nline 3\nline 4\nline 5\n"
	e := NewEditorView(nil, "Page", "/tmp/page.md", content, false, 40, 6)
	if got := e.ta.Line(); got != 0 {
		t.Fatalf("editor should open at row 0, got row %d", got)
	}
}

func key(s string) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)} }

func TestEditorUpdate_CtrlSRequestsSaveStaysEditing(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	e := NewEditorView(nil, "Alpha", "/tmp/a.md", "x\n", false, 80, 24)
	res, _ := e.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	if !res.Save || res.Exit {
		t.Errorf("ctrl+s: got %+v, want {Save:true, Exit:false}", res)
	}
	if e.mode != editing {
		t.Errorf("ctrl+s should stay in editing mode")
	}
}

func TestEditorUpdate_EscOnCleanExits(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	e := NewEditorView(nil, "Alpha", "/tmp/a.md", "x\n", false, 80, 24)
	res, _ := e.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if !res.Exit {
		t.Errorf("esc on clean buffer should exit; got %+v", res)
	}
}

func TestEditorUpdate_EscOnDirtyPromptsThenDiscard(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	e := NewEditorView(nil, "Alpha", "/tmp/a.md", "x\n", false, 80, 24)
	e.ta.SetValue("x\nmore\n") // make it dirty

	res, _ := e.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if res.Exit || res.Save {
		t.Errorf("esc on dirty buffer should not exit yet; got %+v", res)
	}
	if e.mode != confirmingExit {
		t.Fatalf("esc on dirty should enter confirmingExit")
	}

	res, _ = e.Update(key("c"))
	if res.Exit || e.mode != editing {
		t.Errorf("c should cancel back to editing; got %+v mode=%v", res, e.mode)
	}

	e.Update(tea.KeyMsg{Type: tea.KeyEsc})
	res, _ = e.Update(key("d"))
	if !res.Exit || res.Save {
		t.Errorf("d should exit without save; got %+v", res)
	}
}

func TestEditorUpdate_ConfirmSaveExits(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	e := NewEditorView(nil, "Alpha", "/tmp/a.md", "x\n", false, 80, 24)
	e.ta.SetValue("x\nmore\n")
	e.Update(tea.KeyMsg{Type: tea.KeyEsc}) // -> confirmingExit
	res, _ := e.Update(key("s"))
	if !res.Save || !res.Exit {
		t.Errorf("s should save and exit; got %+v", res)
	}
}

func TestEditorUpdate_TypingInsertsAndIsDirty(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	e := NewEditorView(nil, "Alpha", "/tmp/a.md", "", true, 80, 24)
	e.Update(key("h"))
	e.Update(key("i"))
	if got := e.ta.Value(); got != "hi" {
		t.Errorf("typing: got %q, want %q", got, "hi")
	}
	if !e.dirty() {
		t.Errorf("typing should mark dirty")
	}
}

func TestEditorView_Golden(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	e := NewEditorView(nil, "Alpha", "/tmp/a.md", "- first bullet\n- [[Beta]] link\n", false, 80, 12)
	teatest.RequireEqualOutput(t, []byte(e.View()))
}

func TestEditorView_ConfirmGolden(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	e := NewEditorView(nil, "Alpha", "/tmp/a.md", "x\n", false, 80, 12)
	e.ta.SetValue("x\nedited\n")
	e.Update(tea.KeyMsg{Type: tea.KeyEsc}) // -> confirmingExit
	teatest.RequireEqualOutput(t, []byte(e.View()))
}

func TestEditorUpdate_PageDownMovesCursor(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	// 40 numbered lines; height 10 so paging is meaningful.
	var sb strings.Builder
	for i := 0; i < 40; i++ {
		sb.WriteString("line\n")
	}
	e := NewEditorView(nil, "Alpha", "/tmp/a.md", sb.String(), false, 80, 10)
	// PageDown should not panic and should leave the buffer unchanged.
	before := e.ta.Value()
	e.Update(tea.KeyMsg{Type: tea.KeyPgDown})
	e.Update(tea.KeyMsg{Type: tea.KeyPgUp})
	if e.ta.Value() != before {
		t.Errorf("paging must not modify the buffer")
	}
	if e.dirty() {
		t.Errorf("paging must not mark the buffer dirty")
	}
}

func TestEditorUpdate_CtrlCCancelsConfirm(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	e := NewEditorView(nil, "Alpha", "/tmp/a.md", "x\n", false, 80, 24)
	e.ta.SetValue("x\nmore\n")
	e.Update(tea.KeyMsg{Type: tea.KeyEsc}) // -> confirmingExit
	res, _ := e.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if res.Exit || e.mode != editing {
		t.Errorf("ctrl+c at confirm should cancel back to editing; got %+v mode=%v", res, e.mode)
	}
}

func TestEditorView_SetErrorRendersInView(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	e := NewEditorView(nil, "Alpha", "/tmp/a.md", "x\n", false, 80, 6)
	e.SetError("permission denied")
	if !strings.Contains(e.View(), "permission denied") {
		t.Errorf("editor view should show the save error; got:\n%s", e.View())
	}
}

func TestEditorView_MarkSavedClearsError(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	e := NewEditorView(nil, "Alpha", "/tmp/a.md", "x\n", false, 80, 6)
	e.SetError("permission denied")
	e.MarkSaved(e.Content())
	if strings.Contains(e.View(), "permission denied") {
		t.Errorf("a successful save should clear the error from the view; got:\n%s", e.View())
	}
}

func TestEditorView_CompletionGolden(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	e := NewEditorView(loadFixture(t), "Note", "/tmp/n.md", "", true, 80, 16)
	typeRunes(e, "link to [[A")
	if !e.completer.active {
		t.Fatalf("completer should be active for the golden")
	}
	teatest.RequireEqualOutput(t, []byte(e.View()))
}

// typeRunes feeds each rune of s to the editor as an individual key press,
// mirroring real typing so the completer re-derives after every keystroke.
func typeRunes(e *EditorView, s string) {
	for _, r := range s {
		e.Update(key(string(r)))
	}
}

func TestEditorCompletion_EnterInsertsExistingLink(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	e := NewEditorView(loadFixture(t), "Note", "/tmp/n.md", "", true, 80, 24)
	typeRunes(e, "see [[Alp")
	if !e.completer.active {
		t.Fatalf("completer should be active after typing [[Alp")
	}
	e.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if got := e.ta.Value(); got != "see [[Alpha]]" {
		t.Errorf("after accept: got %q, want %q", got, "see [[Alpha]]")
	}
	if e.completer.active {
		t.Errorf("completer should close after accepting")
	}
}

func TestEditorCompletion_TabAlsoAccepts(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	e := NewEditorView(loadFixture(t), "Note", "/tmp/n.md", "", true, 80, 24)
	typeRunes(e, "[[Alp")
	e.Update(tea.KeyMsg{Type: tea.KeyTab})
	if got := e.ta.Value(); got != "[[Alpha]]" {
		t.Errorf("tab accept: got %q, want %q", got, "[[Alpha]]")
	}
}

func TestEditorCompletion_CreateRowInsertsTypedName(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	e := NewEditorView(loadFixture(t), "Note", "/tmp/n.md", "", true, 80, 24)
	typeRunes(e, "[[Zxqv")
	e.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if got := e.ta.Value(); got != "[[Zxqv]]" {
		t.Errorf("create accept: got %q, want %q", got, "[[Zxqv]]")
	}
}

func TestEditorCompletion_EscDismissesKeepsBuffer(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	e := NewEditorView(loadFixture(t), "Note", "/tmp/n.md", "", true, 80, 24)
	typeRunes(e, "[[Alp")
	res, _ := e.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if res.Exit {
		t.Errorf("esc while completing should not exit the editor")
	}
	if e.completer.active {
		t.Errorf("esc should dismiss the completer")
	}
	if got := e.ta.Value(); got != "[[Alp" {
		t.Errorf("esc must not change the buffer: got %q", got)
	}
}

func TestEditorCompletion_ArrowsStealOnlyWhenActive(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	e := NewEditorView(loadFixture(t), "Note", "/tmp/n.md", "", true, 80, 24)
	typeRunes(e, "[[")
	before := e.ta.Value()
	e.Update(tea.KeyMsg{Type: tea.KeyDown})
	if e.completer.sel != 1 {
		t.Errorf("down should move the completer selection while active; sel=%d", e.completer.sel)
	}
	if e.ta.Value() != before {
		t.Errorf("down must not edit the buffer while completing")
	}
}

func TestEditorCompletion_NotActiveInsideClosedBrackets(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	e := NewEditorView(loadFixture(t), "Note", "/tmp/n.md", "[[]] tail\n", false, 80, 24)
	e.ta.SetCursor(2) // between the [[ and ]]
	e.refreshCompleter(true)
	if e.completer.active {
		t.Errorf("completer must not activate inside an already-closed [[ ]]")
	}
}

func TestEditorCompletion_NotActiveEditingExistingLink(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	e := NewEditorView(loadFixture(t), "Note", "/tmp/n.md", "[[Alpha]] rest\n", false, 80, 24)
	e.ta.SetCursor(4) // [[Al|pha]]
	e.refreshCompleter(true)
	if e.completer.active {
		t.Errorf("completer must not activate when editing inside an existing link")
	}
}

func TestEditorCompletion_MidLineAcceptKeepsTrailingText(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	e := NewEditorView(loadFixture(t), "Note", "/tmp/n.md", "ab", false, 80, 24)
	e.ta.SetCursor(1) // single row "ab"; cursor between a and b
	typeRunes(e, "[[Alp")
	e.Update(tea.KeyMsg{Type: tea.KeyEnter}) // accept
	if got := e.ta.Value(); got != "a[[Alpha]]b" {
		t.Errorf("mid-line accept: got %q, want %q", got, "a[[Alpha]]b")
	}
}

func TestEditorCompletion_AcceptOnSecondLine(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	e := NewEditorView(loadFixture(t), "Note", "/tmp/n.md", "first", false, 80, 24)
	e.ta.CursorEnd()
	e.Update(tea.KeyMsg{Type: tea.KeyEnter}) // completer inactive -> newline, cursor to row 1
	typeRunes(e, "see [[Alp")
	e.Update(tea.KeyMsg{Type: tea.KeyEnter}) // accept
	if got := e.ta.Value(); got != "first\nsee [[Alpha]]" {
		t.Errorf("second-line accept: got %q, want %q", got, "first\nsee [[Alpha]]")
	}
}

func TestEditorCompletion_StripFitsSmallTerminal(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	e := NewEditorView(loadFixture(t), "Note", "/tmp/n.md", "", true, 40, 12)
	typeRunes(e, "[[") // empty partial -> full recent list
	if !e.completer.active {
		t.Fatalf("completer should be active")
	}
	lines := strings.Count(e.View(), "\n") + 1
	if lines > 12 {
		t.Errorf("editor view is %d lines, exceeds terminal height 12", lines)
	}
}

func TestEditorCompletion_CursorStaysVisibleAtBottom(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	// A buffer taller than the viewport, with a recognizable last line.
	var sb strings.Builder
	for i := 0; i < 60; i++ {
		sb.WriteString("filler\n")
	}
	sb.WriteString("EDITHERE")
	e := NewEditorView(loadFixture(t), "Note", "/tmp/n.md", sb.String(), false, 80, 24)
	// The editor now opens at the top; this test edits at the bottom, so move
	// the cursor to the last line first.
	for {
		before := e.ta.Line()
		e.ta.CursorDown()
		if e.ta.Line() == before {
			break
		}
	}
	// Mimic the Bubble Tea loop: a render primes the textarea viewport's
	// content. The textarea only repositions its viewport inside Update, using
	// content captured during the previous View — so without interleaved
	// renders it can't scroll, just like in the real runtime.
	_ = e.View()
	e.Update(key("x")) // edit at the bottom; reposition brings the line into view
	_ = e.View()
	if !strings.Contains(e.View(), "EDITHERE") {
		t.Fatalf("precondition: edited line should be visible before the strip opens")
	}
	// Opening the completion strip shrinks the textarea; the line being edited
	// must not scroll out of view behind the strip.
	for _, r := range "[[" {
		e.Update(key(string(r)))
		_ = e.View()
	}
	if !e.completer.active {
		t.Fatalf("completer should be active after typing [[")
	}
	if !strings.Contains(e.View(), "EDITHERE") {
		t.Errorf("the line being edited must stay visible when the strip opens")
	}
}

// --- Regression: the completion strip is a typing affordance, not a
// cursor-navigation one. Moving the caret onto an unclosed [[, opening a file
// that ends in one, or paging should never spontaneously pop the strip.

func TestEditorCompletion_OpeningFileDoesNotAutoOpen(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	// File ends in an unclosed [[Alpha (cursor lands at end on mount).
	e := NewEditorView(loadFixture(t), "Note", "/tmp/n.md", "notes [[Alpha", false, 80, 24)
	if e.completer.active {
		t.Errorf("opening a file ending in an unclosed [[ must not auto-open the completer; partial=%q", e.completer.partial)
	}
}

func TestEditorCompletion_NavigationOntoUnclosedDoesNotOpen(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	e := NewEditorView(loadFixture(t), "Note", "/tmp/n.md", "- draft [[Alpha", false, 80, 24)
	e.Update(tea.KeyMsg{Type: tea.KeyHome}) // caret to col 0 — no [[ before it, strip closed
	if e.completer.active {
		t.Fatalf("precondition: completer should be closed at line start")
	}
	e.Update(tea.KeyMsg{Type: tea.KeyEnd}) // caret back past the unclosed [[ — pure navigation
	if e.completer.active {
		t.Errorf("navigating (End) onto an unclosed [[ must not open the completer; partial=%q", e.completer.partial)
	}
}

func TestEditorCompletion_DismissThenNavStaysClosed(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	e := NewEditorView(loadFixture(t), "Note", "/tmp/n.md", "", true, 80, 24)
	typeRunes(e, "[[Alp") // strip opens
	e.Update(tea.KeyMsg{Type: tea.KeyEsc})   // dismiss
	e.Update(tea.KeyMsg{Type: tea.KeyRight}) // pure navigation, no buffer change
	if e.completer.active {
		t.Errorf("strip must stay closed after dismiss then navigate; partial=%q", e.completer.partial)
	}
}

func TestEditorCompletion_TypingIntoUnclosedAfterNavReopens(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	e := NewEditorView(loadFixture(t), "Note", "/tmp/n.md", "draft [[Alph", false, 80, 24)
	e.Update(tea.KeyMsg{Type: tea.KeyHome})
	e.Update(tea.KeyMsg{Type: tea.KeyEnd}) // navigation must not open (the fix)
	if e.completer.active {
		t.Fatalf("precondition: navigation should leave the completer closed")
	}
	e.Update(key("a")) // an actual edit — mutation — should open the strip
	if !e.completer.active {
		t.Errorf("typing into an unclosed [[ should open the completer; partial=%q", e.completer.partial)
	}
}

func TestEditorViewIsLeftInset(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	e := NewEditorView(nil, "Page", "/tmp/page.md", "hello world\n", false, 40, 10)
	out := e.View()
	for i, line := range strings.Split(out, "\n") {
		if strings.TrimSpace(line) == "" {
			continue // padding/blank rows need not be inset
		}
		if !strings.HasPrefix(line, "  ") {
			t.Fatalf("line %d not inset by 2 spaces: %q", i, line)
		}
	}
}

func TestEditorViewHasTopMargin(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	const h = 10
	e := NewEditorView(nil, "Page", "/tmp/page.md", "# Title\n", false, 40, h)
	lines := strings.Split(e.View(), "\n")
	// Match the read view's leading blank line: the editor's first row is a
	// top margin, so content starts on row 1, not row 0.
	if strings.TrimSpace(lines[0]) != "" {
		t.Fatalf("expected blank top-margin row, got %q", lines[0])
	}
	if !strings.Contains(lines[1], "# Title") {
		t.Fatalf("expected content on row 1, got %q", lines[1])
	}
	// The whole view must still fit the terminal height.
	if len(lines) > h {
		t.Fatalf("view is %d rows, exceeds height %d", len(lines), h)
	}
}

// All four use a trailing "second" line so the Enter's effect is interior to
// the buffer and survives Content()'s trailing-newline normalization (Content
// collapses trailing blank lines to a single "\n", which would otherwise hide
// a newline inserted at end-of-buffer). The editor opens at row 0; CursorEnd()
// moves to the end of that first row before each Enter.

func TestEditorEnterContinuesBullet(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	e := NewEditorView(nil, "Page", "/tmp/page.md", "- first\nsecond\n", false, 40, 10)
	e.ta.CursorEnd()
	e.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if got := e.Content(); got != "- first\n- \nsecond\n" {
		t.Fatalf("continuation: got %q, want %q", got, "- first\n- \nsecond\n")
	}
}

func TestEditorEnterContinuesNestedBullet(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	e := NewEditorView(nil, "Page", "/tmp/page.md", "  - nested\nsecond\n", false, 40, 10)
	e.ta.CursorEnd()
	e.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if got := e.Content(); got != "  - nested\n  - \nsecond\n" {
		t.Fatalf("nested continuation: got %q, want %q", got, "  - nested\n  - \nsecond\n")
	}
}

func TestEditorEnterEmptyBulletTerminates(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	e := NewEditorView(nil, "Page", "/tmp/page.md", "- \nsecond\n", false, 40, 10)
	e.ta.CursorEnd()
	e.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if got := e.Content(); got != "\nsecond\n" {
		t.Fatalf("empty-bullet terminate: got %q, want %q", got, "\nsecond\n")
	}
}

func TestEditorEnterNonBulletInsertsNewline(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	e := NewEditorView(nil, "Page", "/tmp/page.md", "plain\nsecond\n", false, 40, 10)
	e.ta.CursorEnd()
	e.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if got := e.Content(); got != "plain\n\nsecond\n" {
		t.Fatalf("non-bullet newline: got %q, want %q", got, "plain\n\nsecond\n")
	}
}

func TestEditorViewTintsHeadings(t *testing.T) {
	// Integration test: tinting must actually reach EditorView.View() output.
	// Force a color profile so lipgloss emits ANSI (a non-TTY `go test` strips
	// it otherwise). Must NOT call t.Parallel — SetColorProfile is process-global.
	orig := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(orig) })

	// The editor opens with the cursor on row 0, which is rendered raw, so put a
	// plain line there; the heading and wiki-link on later rows are non-cursor
	// rows and must be tinted.
	e := NewEditorView(nil, "Page", "/tmp/page.md", "intro\n\n# Title\n\nsee [[Foo]]\n", false, 40, 10)
	out := e.View()

	// The whole padded heading row is bolded, so the closing reset sits after
	// the trailing spaces — assert the opening bold sequence precedes the text,
	// not an exact bold-wrapped "# Title".
	if !strings.Contains(out, "\x1b[1m# Title") {
		t.Fatalf("heading not tinted bold in editor view:\n%q", out)
	}
	link := lipgloss.NewStyle().Foreground(colorHighlight).Render("[[Foo]]")
	if !strings.Contains(out, link) {
		t.Fatalf("wiki link not tinted in editor view:\n%q", out)
	}
}

func TestEditorCtrlTCyclesMarker(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	e := NewEditorView(nil, "Page", "/tmp/page.md", "- task\n", false, 40, 10)
	e.ta.CursorEnd()

	e.Update(tea.KeyMsg{Type: tea.KeyCtrlT})
	if got := e.Content(); got != "- TODO task\n" {
		t.Fatalf("plain->TODO: got %q", got)
	}
	e.Update(tea.KeyMsg{Type: tea.KeyCtrlT})
	if got := e.Content(); got != "- DONE task\n" {
		t.Fatalf("TODO->DONE: got %q", got)
	}
	e.Update(tea.KeyMsg{Type: tea.KeyCtrlT})
	if got := e.Content(); got != "- task\n" {
		t.Fatalf("DONE->plain: got %q", got)
	}
}

func TestEditorCtrlTNoOpOnNonBullet(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	e := NewEditorView(nil, "Page", "/tmp/page.md", "# heading\n", false, 40, 10)
	e.ta.CursorEnd()
	e.Update(tea.KeyMsg{Type: tea.KeyCtrlT})
	if got := e.Content(); got != "# heading\n" {
		t.Fatalf("ctrl+t on non-bullet should be a no-op, got %q", got)
	}
}

func TestEditorRepositionsAfterContinuation(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	// A buffer taller than the viewport, ending in a bullet with a unique tag.
	var sb strings.Builder
	for i := 0; i < 60; i++ {
		sb.WriteString("filler\n")
	}
	sb.WriteString("- ANCHOR")
	e := NewEditorView(nil, "Note", "/tmp/n.md", sb.String(), false, 80, 24)
	// CursorDown does not reposition the viewport, so this lands the cursor on
	// the bottom bullet while the viewport stays at the top.
	for {
		before := e.ta.Line()
		e.ta.CursorDown()
		if e.ta.Line() == before {
			break
		}
	}
	e.ta.CursorEnd()
	_ = e.View() // prime the textarea viewport content; it is still scrolled to the top
	if strings.Contains(e.View(), "ANCHOR") {
		t.Fatalf("precondition: the bottom bullet should be off-screen before Enter")
	}
	// Continuation inserts a new bullet below ANCHOR via InsertString (which does
	// not reposition); syncViewport must scroll the cursor region into view.
	e.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if !strings.Contains(e.View(), "ANCHOR") {
		t.Errorf("continuation must scroll the new bullet into view (viewport did not follow the cursor)")
	}
}

func TestEditorTabIndents(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	e := NewEditorView(nil, "Page", "/tmp/page.md", "- foo\nx\n", false, 40, 10)
	e.ta.CursorEnd() // end of "- foo" on row 0
	e.Update(tea.KeyMsg{Type: tea.KeyTab})
	if got := e.Content(); got != "  - foo\nx\n" {
		t.Fatalf("tab indent: got %q, want %q", got, "  - foo\nx\n")
	}
	// cursor rides with the text: was at col 5 (end of "- foo"), +2 after indent.
	if before, _ := e.cursorLineSplit(); len([]rune(before)) != 7 {
		t.Fatalf("tab indent cursor col: got %d, want 7", len([]rune(before)))
	}
}

func TestEditorShiftTabDedents(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	e := NewEditorView(nil, "Page", "/tmp/page.md", "  - foo\nx\n", false, 40, 10)
	e.ta.CursorEnd()
	e.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	if got := e.Content(); got != "- foo\nx\n" {
		t.Fatalf("shift+tab dedent: got %q, want %q", got, "- foo\nx\n")
	}
	// cursor rides with the text: was at col 7 (end of "  - foo"), -2 after dedent.
	if before, _ := e.cursorLineSplit(); len([]rune(before)) != 5 {
		t.Fatalf("shift+tab dedent cursor col: got %d, want 5", len([]rune(before)))
	}
}

func TestEditorShiftTabNoOpAtZeroIndent(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	e := NewEditorView(nil, "Page", "/tmp/page.md", "- foo\nx\n", false, 40, 10)
	e.ta.CursorEnd()
	e.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	if got := e.Content(); got != "- foo\nx\n" {
		t.Fatalf("shift+tab at zero indent should be a no-op: got %q", got)
	}
}
