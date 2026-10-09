package views

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

var (
	promptNow = time.Date(2026, 5, 25, 10, 0, 0, 0, time.UTC) // a Monday
	enterKey  = tea.KeyPressMsg{Code: tea.KeyEnter}
	escKey    = tea.KeyPressMsg{Code: tea.KeyEscape}
)

// taskEditor opens content with the cursor on line and the clock fixed.
func taskEditor(content string, line int) *EditorView {
	e := editorAt(nil, "Page", "/tmp/Page.md", content, false, 80, 24, line)
	e.now = func() time.Time { return promptNow }
	setCursor(e, line, 0)
	return e
}

func typeInto(e *EditorView, s string) {
	for _, r := range s {
		e.Update(key(string(r)))
	}
}

func TestEditorDatePromptSetsScheduled(t *testing.T) {
	for in, stamp := range map[string]string{
		"2026-06-03": "<2026-06-03 Wed>",
		"today":      "<2026-05-25 Mon>",
		"tomorrow":   "<2026-05-26 Tue>",
		"+3d":        "<2026-05-28 Thu>",
		"+2w":        "<2026-06-08 Mon>",
		"fri":        "<2026-05-29 Fri>",
	} {
		e := taskEditor("- TODO x", 0)
		e.Update(altKey('s'))
		typeInto(e, in)
		e.Update(enterKey)
		if got, want := text(e), "- TODO x\n  SCHEDULED: "+stamp; got != want {
			t.Errorf("%q: got %q, want %q", in, got, want)
		}
		if e.prompt != nil {
			t.Errorf("%q: prompt still open", in)
		}
	}
}

func TestEditorDatePromptDeadline(t *testing.T) {
	e := taskEditor("- TODO x\n  SCHEDULED: <2026-05-20 Wed>", 0)
	e.Update(altKey('e'))
	typeInto(e, "tomorrow")
	e.Update(enterKey)
	if got, want := text(e), "- TODO x\n  SCHEDULED: <2026-05-20 Wed>\n  DEADLINE: <2026-05-26 Tue>"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestEditorDatePromptEmptyClears(t *testing.T) {
	e := taskEditor("- TODO x\n  SCHEDULED: <2026-05-20 Wed>", 0)
	e.Update(altKey('s'))
	if e.prompt == nil || e.prompt.current != "2026-05-20" {
		t.Fatalf("prompt = %+v, want current 2026-05-20", e.prompt)
	}
	e.Update(enterKey)
	if got := text(e); got != "- TODO x" {
		t.Errorf("got %q, want the stamp removed", got)
	}
}

func TestEditorDatePromptNothingToClear(t *testing.T) {
	e := taskEditor("- TODO x", 0)
	e.Update(altKey('e'))
	e.Update(enterKey)
	if text(e) != "- TODO x" || e.notice != "no deadline to clear" || e.prompt != nil {
		t.Errorf("text %q notice %q prompt %v", text(e), e.notice, e.prompt)
	}
}

func TestEditorDatePromptInvalidKeepsOpen(t *testing.T) {
	e := taskEditor("- TODO x", 0)
	e.Update(altKey('s'))
	typeInto(e, "abc")
	e.Update(enterKey)
	if e.prompt == nil {
		t.Fatal("prompt closed on invalid input")
	}
	if text(e) != "- TODO x" {
		t.Errorf("buffer changed: %q", text(e))
	}
	if status := plain(e.statusLine()); !strings.Contains(status, "not a date: abc") {
		t.Errorf("status %q lacks the error", status)
	}
	e.Update(key("d")) // any key clears the error
	if status := plain(e.statusLine()); strings.Contains(status, "not a date") || !strings.Contains(status, "↵ set · esc cancel") {
		t.Errorf("status %q after the next key", status)
	}
}

func TestEditorDatePromptEscCancels(t *testing.T) {
	e := taskEditor("- TODO x", 0)
	e.Update(altKey('s'))
	typeInto(e, "today")
	e.Update(escKey)
	if e.prompt != nil || text(e) != "- TODO x" {
		t.Errorf("prompt %v text %q", e.prompt, text(e))
	}
}

func TestEditorDatePromptEditingKeys(t *testing.T) {
	e := taskEditor("- TODO x", 0)
	e.Update(altKey('s'))
	typeInto(e, "tomorrowx")
	e.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
	if e.prompt.input != "tomorrow" {
		t.Errorf("after backspace input = %q", e.prompt.input)
	}
	e.Update(tea.KeyPressMsg{Code: tea.KeyUp}) // swallowed
	e.Update(ctrl('u'))
	if e.prompt.input != "" || e.buf.Cursor().Line != 0 {
		t.Errorf("after ctrl+u input = %q", e.prompt.input)
	}
}

func TestEditorDatePromptNonTaskHint(t *testing.T) {
	e := taskEditor("- plain", 0)
	e.Update(altKey('s'))
	if e.prompt != nil || e.notice != "not on a task" {
		t.Errorf("prompt %v notice %q", e.prompt, e.notice)
	}
}

func TestEditorDatePromptRowAndCursor(t *testing.T) {
	e := taskEditor("- TODO x", 0)
	before := e.textHeight()
	e.Update(altKey('s'))
	if got := e.textHeight(); got != before-1 {
		t.Errorf("textHeight = %d, want %d", got, before-1)
	}
	rows := strings.Split(plain(e.View()), "\n")
	if len(rows) != e.height {
		t.Fatalf("%d rows, want %d", len(rows), e.height)
	}
	row := strings.TrimLeft(rows[len(rows)-2], " ")
	if !strings.HasPrefix(row, "Scheduled: ") {
		t.Errorf("prompt row = %q", row)
	}
	if c := e.Cursor(); c == nil || c.Y != len(rows)-2 {
		t.Errorf("cursor = %+v, want y %d", c, len(rows)-2)
	}
}

func TestEditorAltPCyclesPriority(t *testing.T) {
	e := taskEditor("- TODO x", 0)
	e.Update(altKey('p'))
	if got := text(e); got != "- TODO [#A] x" {
		t.Errorf("got %q", got)
	}
	e.Update(ctrl('z'))
	if got := text(e); got != "- TODO x" {
		t.Errorf("after undo %q", got)
	}
	e = taskEditor("- plain", 0)
	e.Update(altKey('p'))
	if e.notice != "not on a task" {
		t.Errorf("notice %q", e.notice)
	}
}

func TestAppEditorDateUsesClockAndSaves(t *testing.T) {
	a := bootApp(t, bootConfig{now: time.Date(2026, 5, 25, 10, 0, 0, 0, time.UTC)})
	a.navigate("Workbench")
	a.Update(key("e"))
	if a.editor == nil {
		t.Fatal("editor did not open")
	}
	line := -1
	for i := 0; i < a.editor.buf.Len(); i++ {
		if a.editor.buf.Line(i) == "- TODO Fit the vise jaws" {
			line = i
		}
	}
	if line < 0 {
		t.Fatal("task line not found")
	}
	setCursor(a.editor, line, 0)
	a.Update(altKey('s'))
	for _, r := range "tomorrow" {
		a.Update(key(string(r)))
	}
	a.Update(enterKey)
	a.Update(altKey('p'))
	a.Update(ctrl('s'))
	want := "- TODO [#A] Fit the vise jaws\n  SCHEDULED: <2026-05-26 Tue>\n  - cut the jaw liners"
	if got := a.editor.Content(); !strings.Contains(got, want) {
		t.Errorf("buffer lacks %q", want)
	}
	if got := a.editor.Content(); a.editor.dirty() {
		t.Errorf("buffer still dirty after ctrl+s: %q", got)
	}
}
