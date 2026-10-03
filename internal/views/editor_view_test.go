package views

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/exp/teatest"
	"github.com/muesli/termenv"
)

func TestEditorView_LoadsContentAndTracksDirty(t *testing.T) {
	quietTerm(t)
	e := NewEditorView(nil, "Alpha", "/tmp/Alpha.md", "first line\n", false, 80, 24, 0)

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
	quietTerm(t)
	e := NewEditorView(nil, "Alpha", "/tmp/Alpha.md", "", true, 80, 24, 0)
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
	quietTerm(t)
	e := NewEditorView(nil, "Alpha", "/tmp/a.md", "", true, 80, 24, 0)
	e.ta.SetValue("no trailing newline") // user typed, no \n
	e.MarkSaved(e.Content())
	if e.dirty() {
		t.Errorf("buffer should be clean after MarkSaved even without a trailing newline")
	}
}

func TestEditorView_CleanOnOpenWhenFileLacksTrailingNewline(t *testing.T) {
	quietTerm(t)
	e := NewEditorView(nil, "Alpha", "/tmp/a.md", "loaded without newline", false, 80, 24, 0)
	if e.dirty() {
		t.Errorf("freshly opened buffer should be clean even if the file lacked a trailing newline")
	}
}

func TestEditorViewOpensAtAnchorLine(t *testing.T) {
	quietTerm(t)
	content := "line 1\nline 2\nline 3\nline 4\nline 5\n"

	e := NewEditorView(nil, "Page", "/tmp/page.md", content, false, 40, 6, 0)
	if got := e.ta.Line(); got != 0 {
		t.Fatalf("anchor 0: got row %d, want 0", got)
	}

	e = NewEditorView(nil, "Page", "/tmp/page.md", content, false, 40, 6, 3)
	if got := e.ta.Line(); got != 3 {
		t.Fatalf("anchor 3: got row %d, want 3", got)
	}
	if li := e.ta.LineInfo(); li.StartColumn+li.ColumnOffset != 0 {
		t.Errorf("anchor 3: cursor column = %d, want 0", li.StartColumn+li.ColumnOffset)
	}
}

func TestEditorViewAnchorClamps(t *testing.T) {
	quietTerm(t)
	content := "line 1\nline 2\nline 3\nline 4\nline 5\n"

	e := NewEditorView(nil, "Page", "/tmp/page.md", content, false, 40, 6, 9999)
	if got, want := e.ta.Line(), e.ta.LineCount()-1; got != want {
		t.Errorf("anchor 9999: got row %d, want %d", got, want)
	}

	e = NewEditorView(nil, "Page", "/tmp/page.md", content, false, 40, 6, -1)
	if got := e.ta.Line(); got != 0 {
		t.Errorf("anchor -1: got row %d, want 0", got)
	}
}

func fillerLines(n int) []string {
	lines := make([]string, n)
	for i := range lines {
		lines[i] = fmt.Sprintf("filler %d\n", i+1)
	}
	return lines
}

func TestEditorViewAnchorStaysClean(t *testing.T) {
	quietTerm(t)
	content := strings.Join(fillerLines(100), "")
	e := NewEditorView(nil, "Page", "/tmp/page.md", content, false, 80, 12, 50)
	if e.dirty() {
		t.Errorf("buffer opened at an anchor should be clean")
	}
	if e.LoadDiverged() {
		t.Errorf("LoadDiverged should be false for clean content at an anchor")
	}
}

func TestEditorViewAnchorKeepsCursorVisible(t *testing.T) {
	quietTerm(t)
	lines := fillerLines(100)
	lines[50] = "ANCHORED LINE\n"
	e := NewEditorView(nil, "Page", "/tmp/page.md", strings.Join(lines, ""), false, 80, 12, 50)
	if got := e.ta.Line(); got != 50 {
		t.Fatalf("got row %d, want 50", got)
	}
	v := e.View()
	if !strings.Contains(v, "ANCHORED LINE") {
		t.Errorf("viewport does not show the anchored line:\n%s", v)
	}
	if strings.Contains(v, "filler 99") {
		t.Errorf("viewport stayed at the end of the buffer:\n%s", v)
	}
}

func key(s string) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)} }

func TestEditorUpdate_CtrlSRequestsSaveStaysEditing(t *testing.T) {
	quietTerm(t)
	e := NewEditorView(nil, "Alpha", "/tmp/a.md", "x\n", false, 80, 24, 0)
	res, _ := e.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	if !res.Save || res.Exit {
		t.Errorf("ctrl+s: got %+v, want {Save:true, Exit:false}", res)
	}
	if e.mode != editing {
		t.Errorf("ctrl+s should stay in editing mode")
	}
}

func TestEditorUpdate_EscOnCleanExits(t *testing.T) {
	quietTerm(t)
	e := NewEditorView(nil, "Alpha", "/tmp/a.md", "x\n", false, 80, 24, 0)
	res, _ := e.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if !res.Exit {
		t.Errorf("esc on clean buffer should exit; got %+v", res)
	}
}

func TestEditorUpdate_EscOnDirtyPromptsThenDiscard(t *testing.T) {
	quietTerm(t)
	e := NewEditorView(nil, "Alpha", "/tmp/a.md", "x\n", false, 80, 24, 0)
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
	quietTerm(t)
	e := NewEditorView(nil, "Alpha", "/tmp/a.md", "x\n", false, 80, 24, 0)
	e.ta.SetValue("x\nmore\n")
	e.Update(tea.KeyMsg{Type: tea.KeyEsc}) // -> confirmingExit
	res, _ := e.Update(key("s"))
	if !res.Save || !res.Exit {
		t.Errorf("s should save and exit; got %+v", res)
	}
}

func TestEditorUpdate_TypingInsertsAndIsDirty(t *testing.T) {
	quietTerm(t)
	e := NewEditorView(nil, "Alpha", "/tmp/a.md", "", true, 80, 24, 0)
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
	quietTerm(t)
	e := NewEditorView(nil, "Alpha", "/tmp/a.md", "- first bullet\n- [[Beta]] link\n", false, 80, 12, 0)
	teatest.RequireEqualOutput(t, []byte(e.View()))
}

func TestEditorView_ConfirmGolden(t *testing.T) {
	quietTerm(t)
	e := NewEditorView(nil, "Alpha", "/tmp/a.md", "x\n", false, 80, 12, 0)
	e.ta.SetValue("x\nedited\n")
	e.Update(tea.KeyMsg{Type: tea.KeyEsc}) // -> confirmingExit
	teatest.RequireEqualOutput(t, []byte(e.View()))
}

func TestEditorUpdate_PageDownMovesCursor(t *testing.T) {
	quietTerm(t)
	// 40 lines; height 10 so paging is meaningful.
	var sb strings.Builder
	for i := 0; i < 40; i++ {
		sb.WriteString("line\n")
	}
	e := NewEditorView(nil, "Alpha", "/tmp/a.md", sb.String(), false, 80, 10, 0)
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
	quietTerm(t)
	e := NewEditorView(nil, "Alpha", "/tmp/a.md", "x\n", false, 80, 24, 0)
	e.ta.SetValue("x\nmore\n")
	e.Update(tea.KeyMsg{Type: tea.KeyEsc}) // -> confirmingExit
	res, _ := e.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if res.Exit || e.mode != editing {
		t.Errorf("ctrl+c at confirm should cancel back to editing; got %+v mode=%v", res, e.mode)
	}
}

func TestEditorView_SetErrorRendersInView(t *testing.T) {
	quietTerm(t)
	e := NewEditorView(nil, "Alpha", "/tmp/a.md", "x\n", false, 80, 6, 0)
	e.SetError("permission denied")
	if !strings.Contains(e.View(), "permission denied") {
		t.Errorf("editor view should show the save error; got:\n%s", e.View())
	}
}

func TestEditorView_MarkSavedClearsError(t *testing.T) {
	quietTerm(t)
	e := NewEditorView(nil, "Alpha", "/tmp/a.md", "x\n", false, 80, 6, 0)
	e.SetError("permission denied")
	e.MarkSaved(e.Content())
	if strings.Contains(e.View(), "permission denied") {
		t.Errorf("a successful save should clear the error from the view; got:\n%s", e.View())
	}
}

func TestEditorView_CompletionGolden(t *testing.T) {
	quietTerm(t)
	e := NewEditorView(loadFixture(t), "Note", "/tmp/n.md", "", true, 80, 16, 0)
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
	quietTerm(t)
	e := NewEditorView(loadFixture(t), "Note", "/tmp/n.md", "", true, 80, 24, 0)
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
	quietTerm(t)
	e := NewEditorView(loadFixture(t), "Note", "/tmp/n.md", "", true, 80, 24, 0)
	typeRunes(e, "[[Alp")
	e.Update(tea.KeyMsg{Type: tea.KeyTab})
	if got := e.ta.Value(); got != "[[Alpha]]" {
		t.Errorf("tab accept: got %q, want %q", got, "[[Alpha]]")
	}
}

func TestEditorCompletion_CreateRowInsertsTypedName(t *testing.T) {
	quietTerm(t)
	e := NewEditorView(loadFixture(t), "Note", "/tmp/n.md", "", true, 80, 24, 0)
	typeRunes(e, "[[Zxqv")
	e.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if got := e.ta.Value(); got != "[[Zxqv]]" {
		t.Errorf("create accept: got %q, want %q", got, "[[Zxqv]]")
	}
}

func TestEditorCompletion_EscDismissesKeepsBuffer(t *testing.T) {
	quietTerm(t)
	e := NewEditorView(loadFixture(t), "Note", "/tmp/n.md", "", true, 80, 24, 0)
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
	quietTerm(t)
	e := NewEditorView(loadFixture(t), "Note", "/tmp/n.md", "", true, 80, 24, 0)
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

func TestEditorCompletion_KeyUpMovesAndClampsSelection(t *testing.T) {
	// arrange
	quietTerm(t)
	e := NewEditorView(loadFixture(t), "Note", "/tmp/n.md", "", true, 80, 24, 0)
	typeRunes(e, "[[")
	if !e.completer.active || len(e.completer.cands) < 2 {
		t.Fatalf("precondition: active strip with 2+ candidates; active=%v cands=%d",
			e.completer.active, len(e.completer.cands))
	}
	before := e.ta.Value()
	e.Update(tea.KeyMsg{Type: tea.KeyDown})
	if e.completer.sel != 1 {
		t.Fatalf("precondition: down should move the selection to 1; sel=%d", e.completer.sel)
	}

	// act
	e.Update(tea.KeyMsg{Type: tea.KeyUp})

	// assert — up retreats, clamps at the top, and never edits the buffer
	if e.completer.sel != 0 {
		t.Errorf("up should move the selection back to 0; sel=%d", e.completer.sel)
	}
	e.Update(tea.KeyMsg{Type: tea.KeyUp})
	if e.completer.sel != 0 {
		t.Errorf("up at the first row must clamp to 0; sel=%d", e.completer.sel)
	}
	if e.ta.Value() != before {
		t.Errorf("up must not edit the buffer while completing")
	}
}

func TestEditorCompletion_NotActiveInsideClosedBrackets(t *testing.T) {
	quietTerm(t)
	e := NewEditorView(loadFixture(t), "Note", "/tmp/n.md", "[[]] tail\n", false, 80, 24, 0)
	e.ta.SetCursor(2) // between the [[ and ]]
	e.refreshCompleter(true)
	if e.completer.active {
		t.Errorf("completer must not activate inside an already-closed [[ ]]")
	}
}

func TestEditorCompletion_NotActiveEditingExistingLink(t *testing.T) {
	quietTerm(t)
	e := NewEditorView(loadFixture(t), "Note", "/tmp/n.md", "[[Alpha]] rest\n", false, 80, 24, 0)
	e.ta.SetCursor(4) // [[Al|pha]]
	e.refreshCompleter(true)
	if e.completer.active {
		t.Errorf("completer must not activate when editing inside an existing link")
	}
}

func TestEditorCompletion_MidLineAcceptKeepsTrailingText(t *testing.T) {
	quietTerm(t)
	e := NewEditorView(loadFixture(t), "Note", "/tmp/n.md", "ab", false, 80, 24, 0)
	e.ta.SetCursor(1) // single row "ab"; cursor between a and b
	typeRunes(e, "[[Alp")
	e.Update(tea.KeyMsg{Type: tea.KeyEnter}) // accept
	if got := e.ta.Value(); got != "a[[Alpha]]b" {
		t.Errorf("mid-line accept: got %q, want %q", got, "a[[Alpha]]b")
	}
}

func TestEditorCompletion_AcceptOnSecondLine(t *testing.T) {
	quietTerm(t)
	e := NewEditorView(loadFixture(t), "Note", "/tmp/n.md", "first", false, 80, 24, 0)
	e.ta.CursorEnd()
	e.Update(tea.KeyMsg{Type: tea.KeyEnter}) // completer inactive -> newline, cursor to row 1
	typeRunes(e, "see [[Alp")
	e.Update(tea.KeyMsg{Type: tea.KeyEnter}) // accept
	if got := e.ta.Value(); got != "first\nsee [[Alpha]]" {
		t.Errorf("second-line accept: got %q, want %q", got, "first\nsee [[Alpha]]")
	}
}

func TestEditorCompletion_StripFitsSmallTerminal(t *testing.T) {
	quietTerm(t)
	e := NewEditorView(loadFixture(t), "Note", "/tmp/n.md", "", true, 40, 12, 0)
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
	quietTerm(t)
	// A buffer taller than the viewport, with a recognizable last line.
	var sb strings.Builder
	for i := 0; i < 60; i++ {
		sb.WriteString("filler\n")
	}
	sb.WriteString("EDITHERE")
	e := NewEditorView(loadFixture(t), "Note", "/tmp/n.md", sb.String(), false, 80, 24, 0)
	// The editor opens at the top; this test edits at the bottom, so move
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

// Regression: a plain terminal resize (completer never involved) must also
// reposition the textarea viewport onto the cursor. SetHeight/SetWidth never
// reposition (bubbles quirk: only Update does), so without an unconditional
// poke in layout() the cursor line can scroll off-screen after a shrink and
// stay hidden until the next keystroke.
func TestEditorResizeKeepsCursorVisible(t *testing.T) {
	// arrange — a buffer taller than the viewport, cursor moved to the last
	// line, tall window so the cursor line renders comfortably.
	quietTerm(t)
	content := strings.Repeat("filler\n", 59) + "last-line"
	e := NewEditorView(nil, "Note", "/tmp/n.md", content, false, 80, 40, 0)
	for {
		before := e.ta.Line()
		e.ta.CursorDown()
		if e.ta.Line() == before {
			break
		}
	}
	// Mimic the Bubble Tea loop: a render primes the textarea viewport's
	// content, same technique as TestEditorCompletion_CursorStaysVisibleAtBottom.
	_ = e.View()

	// act — shrink hard; no keypress afterwards.
	e.SetSize(80, 8)

	// assert — the cursor's line must be inside the rendered window.
	if !strings.Contains(e.View(), "last-line") {
		t.Fatal("cursor line scrolled out of view after resize")
	}
}

// --- Regression: the completion strip is a typing affordance, not a
// cursor-navigation one. Moving the caret onto an unclosed [[, opening a file
// that ends in one, or paging should never spontaneously pop the strip.

func TestEditorCompletion_OpeningFileDoesNotAutoOpen(t *testing.T) {
	quietTerm(t)
	// File ends in an unclosed [[Alpha (cursor lands at end on mount).
	e := NewEditorView(loadFixture(t), "Note", "/tmp/n.md", "notes [[Alpha", false, 80, 24, 0)
	if e.completer.active {
		t.Errorf("opening a file ending in an unclosed [[ must not auto-open the completer; partial=%q", e.completer.partial)
	}
}

func TestEditorCompletion_NavigationOntoUnclosedDoesNotOpen(t *testing.T) {
	quietTerm(t)
	e := NewEditorView(loadFixture(t), "Note", "/tmp/n.md", "- draft [[Alpha", false, 80, 24, 0)
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
	quietTerm(t)
	e := NewEditorView(loadFixture(t), "Note", "/tmp/n.md", "", true, 80, 24, 0)
	typeRunes(e, "[[Alp")                    // strip opens
	e.Update(tea.KeyMsg{Type: tea.KeyEsc})   // dismiss
	e.Update(tea.KeyMsg{Type: tea.KeyRight}) // pure navigation, no buffer change
	if e.completer.active {
		t.Errorf("strip must stay closed after dismiss then navigate; partial=%q", e.completer.partial)
	}
}

func TestEditorCompletion_TypingIntoUnclosedAfterNavReopens(t *testing.T) {
	quietTerm(t)
	e := NewEditorView(loadFixture(t), "Note", "/tmp/n.md", "draft [[Alph", false, 80, 24, 0)
	e.Update(tea.KeyMsg{Type: tea.KeyHome})
	e.Update(tea.KeyMsg{Type: tea.KeyEnd}) // navigation must not open
	if e.completer.active {
		t.Fatalf("precondition: navigation should leave the completer closed")
	}
	e.Update(key("a")) // an actual edit — mutation — should open the strip
	if !e.completer.active {
		t.Errorf("typing into an unclosed [[ should open the completer; partial=%q", e.completer.partial)
	}
}

func TestEditorViewIsLeftInset(t *testing.T) {
	quietTerm(t)
	e := NewEditorView(nil, "Page", "/tmp/page.md", "hello world\n", false, 40, 10, 0)
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
	quietTerm(t)
	const h = 10
	e := NewEditorView(nil, "Page", "/tmp/page.md", "# Title\n", false, 40, h, 0)
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
	quietTerm(t)
	e := NewEditorView(nil, "Page", "/tmp/page.md", "- first\nsecond\n", false, 40, 10, 0)
	e.ta.CursorEnd()
	e.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if got := e.Content(); got != "- first\n- \nsecond\n" {
		t.Fatalf("continuation: got %q, want %q", got, "- first\n- \nsecond\n")
	}
}

func TestEditorEnterContinuesNestedBullet(t *testing.T) {
	quietTerm(t)
	e := NewEditorView(nil, "Page", "/tmp/page.md", "  - nested\nsecond\n", false, 40, 10, 0)
	e.ta.CursorEnd()
	e.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if got := e.Content(); got != "  - nested\n  - \nsecond\n" {
		t.Fatalf("nested continuation: got %q, want %q", got, "  - nested\n  - \nsecond\n")
	}
}

func TestEditorEnterEmptyBulletTerminates(t *testing.T) {
	quietTerm(t)
	e := NewEditorView(nil, "Page", "/tmp/page.md", "- \nsecond\n", false, 40, 10, 0)
	e.ta.CursorEnd()
	e.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if got := e.Content(); got != "\nsecond\n" {
		t.Fatalf("empty-bullet terminate: got %q, want %q", got, "\nsecond\n")
	}
}

func TestEditorEnterNonBulletInsertsNewline(t *testing.T) {
	quietTerm(t)
	e := NewEditorView(nil, "Page", "/tmp/page.md", "plain\nsecond\n", false, 40, 10, 0)
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
	e := NewEditorView(nil, "Page", "/tmp/page.md", "intro\n\n# Title\n\nsee [[Foo]]\n", false, 40, 10, 0)
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
	quietTerm(t)
	e := NewEditorView(nil, "Page", "/tmp/page.md", "- task\n", false, 40, 10, 0)
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
	quietTerm(t)
	e := NewEditorView(nil, "Page", "/tmp/page.md", "# heading\n", false, 40, 10, 0)
	e.ta.CursorEnd()
	e.Update(tea.KeyMsg{Type: tea.KeyCtrlT})
	if got := e.Content(); got != "# heading\n" {
		t.Fatalf("ctrl+t on non-bullet should be a no-op, got %q", got)
	}
}

func TestEditorRepositionsAfterContinuation(t *testing.T) {
	quietTerm(t)
	// A buffer taller than the viewport, ending in a bullet with a unique tag.
	var sb strings.Builder
	for i := 0; i < 60; i++ {
		sb.WriteString("filler\n")
	}
	sb.WriteString("- ANCHOR")
	e := NewEditorView(nil, "Note", "/tmp/n.md", sb.String(), false, 80, 24, 0)
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
	quietTerm(t)
	e := NewEditorView(nil, "Page", "/tmp/page.md", "- foo\nx\n", false, 40, 10, 0)
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
	quietTerm(t)
	e := NewEditorView(nil, "Page", "/tmp/page.md", "  - foo\nx\n", false, 40, 10, 0)
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
	quietTerm(t)
	e := NewEditorView(nil, "Page", "/tmp/page.md", "- foo\nx\n", false, 40, 10, 0)
	e.ta.CursorEnd()
	e.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	if got := e.Content(); got != "- foo\nx\n" {
		t.Fatalf("shift+tab at zero indent should be a no-op: got %q", got)
	}
}

func TestEditorUpdate_FailedSaveExitShowsErrorNotConfirmPrompt(t *testing.T) {
	quietTerm(t)
	e := NewEditorView(nil, "Alpha", "/tmp/a.md", "x\n", false, 80, 6, 0)
	e.ta.SetValue("x\nmore\n") // make it dirty

	e.Update(tea.KeyMsg{Type: tea.KeyEsc}) // -> confirmingExit
	res, _ := e.Update(key("s"))           // request save+exit
	if !res.Save || !res.Exit {
		t.Fatalf("s should request save and exit; got %+v", res)
	}
	// The App write failed and kept the editor open. The editor must be back
	// in editing mode so the error renders instead of the confirm prompt.
	if e.mode != editing {
		t.Fatalf("after save+exit request, mode should reset to editing; got %v", e.mode)
	}
	e.SetError("permission denied")
	view := e.View()
	if !strings.Contains(view, "permission denied") {
		t.Errorf("failed save+exit should show the error; got:\n%s", view)
	}
	if strings.Contains(view, "Save changes?") {
		t.Errorf("failed save+exit must not still show the confirm prompt; got:\n%s", view)
	}
}

// Characterization: accepting a completion after moving the cursor left within
// the typed partial relocates the trailing partial runes to AFTER the closed
// link, identical to the deliberate mid-line-accept behavior. This is pinned so
// any future change to acceptCompletion is a conscious decision, not a silent
// regression. See plan 2026-06-18-weft-review-bugs.md, Task 2.
func TestEditorCompletion_AcceptAfterLeftMoveRelocatesTail(t *testing.T) {
	quietTerm(t)
	e := NewEditorView(loadFixture(t), "Note", "/tmp/n.md", "", true, 80, 24, 0)
	typeRunes(e, "[[Alph")                  // buffer "[[Alph", cursor at end
	e.Update(tea.KeyMsg{Type: tea.KeyLeft}) // cursor after "Alp"
	e.Update(tea.KeyMsg{Type: tea.KeyLeft}) // cursor after "Al"
	if !e.completer.active {
		t.Fatalf("completer should still be active after left moves; partial=%q", e.completer.partial)
	}
	e.Update(tea.KeyMsg{Type: tea.KeyEnter}) // accept "Alpha"
	if got := e.ta.Value(); got != "[[Alpha]]ph" {
		t.Errorf("documented current behavior: got %q, want %q", got, "[[Alpha]]ph")
	}
}

func BenchmarkEditorViewLargePage(b *testing.B) {
	var sb strings.Builder
	for i := 0; i < 2000; i++ {
		sb.WriteString("- a [[Link]] line with `code` and # not-a-heading\n")
	}
	e := NewEditorView(nil, "Big", "/tmp/big.md", sb.String(), false, 80, 40, 0)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = e.View()
	}
}
