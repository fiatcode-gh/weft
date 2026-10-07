package views

import (
	"testing"

	"github.com/charmbracelet/colorprofile"
	uv "github.com/charmbracelet/ultraviolet"
)

// The row under the cursor is coloured like every other row: the terminal
// cursor is not drawn into the text, so the cursor row needs no special case.
func TestEditorColoursTheCursorRow(t *testing.T) {
	if !inFreshProcess(t, map[string]string{"NO_COLOR": ""}) {
		return
	}
	e := editorAt(nil, "P", "/tmp/p.md", "## Under the cursor\n\n## Elsewhere\n", false, 60, 10, 0)
	if cursorPos(e).Line != 0 {
		t.Fatal("setup: the cursor must sit on the first heading")
	}
	rows := frameCells(e.View(), 60, colorprofile.TrueColor)
	under := cellsShowing(t, rows, "Under")[0].Style
	other := cellsShowing(t, rows, "Elsewhere")[0].Style
	if under.IsZero() {
		t.Fatalf("the cursor's heading row carries no style: %q", under.String())
	}
	if !under.Equal(&other) {
		t.Errorf("cursor row style %q differs from the other heading's %q", under.String(), other.String())
	}
}

// Frame-level smoke of the painter's rule: only syntax the read view hides
// is dimmed; the text it shows is not.
func TestEditorDimsOnlySyntax(t *testing.T) {
	if !inFreshProcess(t, map[string]string{"NO_COLOR": ""}) {
		return
	}
	e := editorAt(nil, "P", "/tmp/p.md", "plain **strong** end\n", false, 60, 10, 0)
	rows := frameCells(e.View(), 60, colorprofile.TrueColor)
	faint := func(c uv.Cell) bool { return c.Style.Attrs&uv.AttrFaint != 0 }

	for _, c := range cellsShowing(t, rows, "**strong**") {
		switch c.Content {
		case "*":
			if !faint(c) {
				t.Errorf("delimiter %q is not dimmed", c.Content)
			}
		default:
			if faint(c) || c.Style.Attrs&uv.AttrBold == 0 {
				t.Errorf("strong text %q: style %q, want bold and not dimmed", c.Content, c.Style.String())
			}
		}
	}
	for _, c := range cellsShowing(t, rows, "plain") {
		if faint(c) {
			t.Errorf("plain text %q is dimmed", c.Content)
		}
	}
}
