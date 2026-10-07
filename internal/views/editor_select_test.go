package views

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/colorprofile"
	uv "github.com/charmbracelet/ultraviolet"

	"github.com/fiatcode-gh/weft/v2/internal/buffer"
)

const selectFixture = "alpha beta gamma\nsecond line here\nthird\n"

func selected(e *EditorView) (buffer.Range, bool) { return e.buf.Selection() }

func TestSelectionKeys(t *testing.T) {
	quietTerm(t)
	mod := func(c rune, m tea.KeyMod) tea.KeyPressMsg { return tea.KeyPressMsg{Code: c, Mod: m} }
	const shiftAlt = tea.ModShift | tea.ModAlt
	const shiftCtrl = tea.ModShift | tea.ModCtrl
	cases := []struct {
		name          string
		extend, plain tea.KeyPressMsg
	}{
		{"shift+left", mod(tea.KeyLeft, tea.ModShift), named(tea.KeyLeft)},
		{"shift+right", mod(tea.KeyRight, tea.ModShift), named(tea.KeyRight)},
		{"shift+up", mod(tea.KeyUp, tea.ModShift), named(tea.KeyUp)},
		{"shift+down", mod(tea.KeyDown, tea.ModShift), named(tea.KeyDown)},
		{"shift+home", mod(tea.KeyHome, tea.ModShift), named(tea.KeyHome)},
		{"shift+end", mod(tea.KeyEnd, tea.ModShift), named(tea.KeyEnd)},
		{"shift+pgup", mod(tea.KeyPgUp, tea.ModShift), named(tea.KeyPgUp)},
		{"shift+pgdown", mod(tea.KeyPgDown, tea.ModShift), named(tea.KeyPgDown)},
		{"alt+shift+left", mod(tea.KeyLeft, shiftAlt), mod(tea.KeyLeft, tea.ModAlt)},
		{"alt+shift+right", mod(tea.KeyRight, shiftAlt), mod(tea.KeyRight, tea.ModAlt)},
		{"alt+shift+b", mod('b', shiftAlt), altKey('b')},
		{"alt+shift+f", mod('f', shiftAlt), altKey('f')},
		{"ctrl+shift+left", mod(tea.KeyLeft, shiftCtrl), mod(tea.KeyLeft, tea.ModCtrl)},
		{"ctrl+shift+right", mod(tea.KeyRight, shiftCtrl), mod(tea.KeyRight, tea.ModCtrl)},
		{"ctrl+shift+home", mod(tea.KeyHome, shiftCtrl), mod(tea.KeyHome, tea.ModCtrl)},
		{"ctrl+shift+end", mod(tea.KeyEnd, shiftCtrl), mod(tea.KeyEnd, tea.ModCtrl)},
	}
	start := buffer.Pos{Line: 1, Col: 7}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ref := editorAt(nil, "P", "/tmp/p.md", selectFixture, false, 60, 8, 0)
			setCursor(ref, 1, 7)
			press(ref, tc.plain)
			dest := cursorPos(ref)
			if dest == start {
				t.Fatalf("setup: %s does not move from %v", tc.plain, start)
			}

			e := editorAt(nil, "P", "/tmp/p.md", selectFixture, false, 60, 8, 0)
			setCursor(e, 1, 7)
			press(e, tc.extend)
			if got := cursorPos(e); got != dest {
				t.Errorf("cursor %v, want %v (the unshifted move)", got, dest)
			}
			r, ok := selected(e)
			if !ok {
				t.Fatalf("no selection after %s", tc.name)
			}
			if r.Start != start && r.End != start {
				t.Errorf("selection %v does not start at %v", r, start)
			}
			if text(e) != selectFixture {
				t.Errorf("text changed: %q", text(e))
			}
		})
	}

	t.Run("a second extend keeps the anchor", func(t *testing.T) {
		e := editorAt(nil, "P", "/tmp/p.md", selectFixture, false, 60, 8, 0)
		setCursor(e, 1, 7)
		press(e, shiftKey(tea.KeyRight), shiftKey(tea.KeyRight), shiftKey(tea.KeyDown))
		r, ok := selected(e)
		if !ok || r.Start != start || r.End.Line != 2 {
			t.Errorf("selection %v ok=%v, want it from %v into line 2", r, ok, start)
		}
	})
	t.Run("right then left is empty", func(t *testing.T) {
		e := editorAt(nil, "P", "/tmp/p.md", selectFixture, false, 60, 8, 0)
		setCursor(e, 1, 7)
		press(e, shiftKey(tea.KeyRight), shiftKey(tea.KeyLeft))
		if r, ok := selected(e); ok {
			t.Errorf("selection %v, want none", r)
		}
	})
	t.Run("shift+down crosses lines", func(t *testing.T) {
		e := editorAt(nil, "P", "/tmp/p.md", selectFixture, false, 60, 8, 0)
		setCursor(e, 0, 3)
		press(e, shiftKey(tea.KeyDown))
		r, ok := selected(e)
		if !ok || r.Start != (buffer.Pos{Line: 0, Col: 3}) || r.End != (buffer.Pos{Line: 1, Col: 3}) {
			t.Errorf("selection %v ok=%v", r, ok)
		}
		if got := e.buf.Text(r); got != "ha beta gamma\nsec" {
			t.Errorf("selected text %q", got)
		}
	})
	t.Run("ctrl+a selects everything", func(t *testing.T) {
		e := editorAt(nil, "P", "/tmp/p.md", selectFixture, false, 60, 8, 0)
		setCursor(e, 1, 4)
		press(e, ctrl('a'))
		r, ok := selected(e)
		if !ok || r.Start != (buffer.Pos{}) || r.End != e.buf.End() || cursorPos(e) != e.buf.End() {
			t.Errorf("selection %v ok=%v cursor %v, want the whole buffer", r, ok, cursorPos(e))
		}
	})
	t.Run("ctrl+a on an empty buffer", func(t *testing.T) {
		e := editorAt(nil, "P", "/tmp/p.md", "", true, 60, 8, 0)
		press(e, ctrl('a'))
		if r, ok := selected(e); ok {
			t.Errorf("selection %v on an empty buffer", r)
		}
		typeRunes(e, "x")
		if text(e) != "x" {
			t.Errorf("text %q after typing", text(e))
		}
	})
}

func TestUnshiftedMovesLeaveTheSelection(t *testing.T) {
	quietTerm(t)
	build := func() *EditorView {
		e := editorAt(nil, "P", "/tmp/p.md", selectFixture, false, 60, 8, 0)
		setCursor(e, 1, 3)
		press(e, shiftKey(tea.KeyRight), shiftKey(tea.KeyRight), shiftKey(tea.KeyRight)) // 3..6
		return e
	}
	t.Run("left collapses to the start", func(t *testing.T) {
		e := build()
		press(e, named(tea.KeyLeft))
		if _, ok := selected(e); ok || cursorPos(e) != (buffer.Pos{Line: 1, Col: 3}) {
			t.Errorf("cursor %v sel=%v, want collapsed at 1:3", cursorPos(e), ok)
		}
	})
	t.Run("right collapses to the end", func(t *testing.T) {
		e := build()
		press(e, named(tea.KeyRight))
		if _, ok := selected(e); ok || cursorPos(e) != (buffer.Pos{Line: 1, Col: 6}) {
			t.Errorf("cursor %v sel=%v, want collapsed at 1:6", cursorPos(e), ok)
		}
	})
	t.Run("left collapses a backwards selection to its start", func(t *testing.T) {
		e := editorAt(nil, "P", "/tmp/p.md", selectFixture, false, 60, 8, 0)
		setCursor(e, 1, 6)
		press(e, shiftKey(tea.KeyLeft), shiftKey(tea.KeyLeft), shiftKey(tea.KeyLeft)) // cursor 3, anchor 6
		press(e, named(tea.KeyLeft))
		if _, ok := selected(e); ok || cursorPos(e) != (buffer.Pos{Line: 1, Col: 3}) {
			t.Errorf("cursor %v sel=%v, want collapsed at 1:3", cursorPos(e), ok)
		}
	})
	t.Run("right collapses a backwards selection to its end", func(t *testing.T) {
		e := editorAt(nil, "P", "/tmp/p.md", selectFixture, false, 60, 8, 0)
		setCursor(e, 1, 6)
		press(e, shiftKey(tea.KeyLeft), shiftKey(tea.KeyLeft), shiftKey(tea.KeyLeft))
		press(e, named(tea.KeyRight))
		if _, ok := selected(e); ok || cursorPos(e) != (buffer.Pos{Line: 1, Col: 6}) {
			t.Errorf("cursor %v sel=%v, want collapsed at 1:6", cursorPos(e), ok)
		}
	})
	t.Run("up moves from the cursor", func(t *testing.T) {
		e := build()
		press(e, named(tea.KeyUp))
		if _, ok := selected(e); ok || cursorPos(e).Line != 0 {
			t.Errorf("cursor %v sel=%v, want line 0 and no selection", cursorPos(e), ok)
		}
	})
	t.Run("end moves from the cursor", func(t *testing.T) {
		e := build()
		press(e, named(tea.KeyEnd))
		if _, ok := selected(e); ok || cursorPos(e) != (buffer.Pos{Line: 1, Col: 16}) {
			t.Errorf("cursor %v sel=%v, want line end", cursorPos(e), ok)
		}
	})
}

func TestTypingReplacesSelection(t *testing.T) {
	quietTerm(t)
	e := editorAt(nil, "P", "/tmp/p.md", "alpha beta\n", false, 60, 8, 0)
	setCursor(e, 0, 2)
	press(e, shiftKey(tea.KeyRight), shiftKey(tea.KeyRight), shiftKey(tea.KeyRight)) // "pha"
	typeRunes(e, "XY")
	if got := text(e); got != "alXY beta\n" {
		t.Fatalf("text %q", got)
	}
	press(e, ctrlZ)
	if got := text(e); got != "alpha beta\n" {
		t.Errorf("one undo gave %q, want the original", got)
	}
}

func TestBackspaceDeletesSelection(t *testing.T) {
	quietTerm(t)
	e := editorAt(nil, "P", "/tmp/p.md", "alpha beta\n", false, 60, 8, 0)
	setCursor(e, 0, 2)
	press(e, shiftKey(tea.KeyRight), shiftKey(tea.KeyRight), shiftKey(tea.KeyRight))
	press(e, named(tea.KeyBackspace))
	if got := text(e); got != "al beta\n" {
		t.Errorf("text %q", got)
	}
}

func TestTabIndentsSelectedLines(t *testing.T) {
	quietTerm(t)
	const list = "- a\n  - a1\n- b\n  - b1\n- c\n"
	e := editorAt(nil, "P", "/tmp/p.md", list, false, 60, 10, 0)
	setCursor(e, 0, 2)
	press(e, shiftKey(tea.KeyDown), shiftKey(tea.KeyDown)) // lines 0..2
	press(e, named(tea.KeyTab))
	if got, want := text(e), "  - a\n    - a1\n  - b\n  - b1\n- c\n"; got != want {
		t.Errorf("text %q, want %q", got, want)
	}
	press(e, ctrlZ)
	if got := text(e); got != list {
		t.Errorf("one undo gave %q, want the original", got)
	}
}

func selectionCells(t *testing.T, e *EditorView, profile colorprofile.Profile) [][]uv.Cell {
	t.Helper()
	return frameCells(e.View(), e.width, profile)
}

// The selection is drawn with Reverse, which survives NO_COLOR (attributes
// stay): the contract's "selection visible under NO_COLOR".
func TestSelectionVisible(t *testing.T) {
	for _, env := range []struct {
		name, noColor string
		profile       colorprofile.Profile
	}{
		{"colour", "", colorprofile.TrueColor},
		{"NO_COLOR", "1", colorprofile.Ascii},
	} {
		t.Run(env.name, func(t *testing.T) {
			if !inFreshProcess(t, map[string]string{"NO_COLOR": env.noColor}) {
				return
			}
			e := editorAt(nil, "P", "/tmp/p.md", "alpha beta gamma\n", false, 60, 8, 0)
			setCursor(e, 0, 2)
			press(e, shiftKey(tea.KeyRight), shiftKey(tea.KeyRight), shiftKey(tea.KeyRight)) // "pha"
			rows := selectionCells(t, e, env.profile)
			cells := cellsShowing(t, rows, "alpha")
			for i, c := range cells {
				rev := c.Style.Attrs&uv.AttrReverse != 0
				if want := i >= 2 && i < 5; rev != want {
					t.Errorf("cell %d %q reverse=%v, want %v (style %q)", i, c.Content, rev, want, c.Style.String())
				}
			}
			if env.noColor != "" {
				assertNoColour(t, rows, "selection under NO_COLOR")
			}
		})
	}
}

// A selected line break is one selected cell after the line's last character.
func TestSelectedLineBreakIsOneCell(t *testing.T) {
	quietTerm(t)
	e := editorAt(nil, "P", "/tmp/p.md", "ab\ncd\n", false, 60, 8, 0)
	setCursor(e, 0, 2)
	press(e, shiftKey(tea.KeyRight)) // the break after "ab"
	rows := selectionCells(t, e, colorprofile.Ascii)
	row := rows[1] // row 0 is the margin
	var got []string
	for x, c := range row {
		if c.Style.Attrs&uv.AttrReverse != 0 {
			got = append(got, string(rune('0'+x)))
		}
	}
	if want := "4"; strings.Join(got, "") != want { // margin 2 + "ab" = cells 2,3; break at 4
		t.Errorf("reverse cells at x=%v, want only %s (row %v)", got, want, row)
	}
	for _, c := range rows[2] {
		if c.Style.Attrs&uv.AttrReverse != 0 {
			t.Error("the next line is drawn selected")
		}
	}
}
