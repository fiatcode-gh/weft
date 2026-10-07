package views

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func ctrl(r rune) tea.KeyPressMsg   { return tea.KeyPressMsg{Code: r, Mod: tea.ModCtrl} }
func altKey(r rune) tea.KeyPressMsg { return tea.KeyPressMsg{Code: r, Mod: tea.ModAlt} }
func named(c rune) tea.KeyPressMsg  { return tea.KeyPressMsg{Code: c} }

// TestEditorKeyTable pins §6's editing keys: each case presses one key on a
// fixed fixture and checks the resulting text and cursor.
func TestEditorKeyTable(t *testing.T) {
	quietTerm(t)
	const fixture = "alpha BeTa gamma\nsecond line\n- item\n"
	cases := []struct {
		name         string
		msg          tea.KeyPressMsg
		line, col    int // cursor before
		want         string
		wantL, wantC int
	}{
		{"type", key("x"), 0, 6, "alpha xBeTa gamma\nsecond line\n- item\n", 0, 7},
		{"space", tea.KeyPressMsg{Code: ' ', Text: " "}, 0, 6, "alpha  BeTa gamma\nsecond line\n- item\n", 0, 7},
		{"enter", named(tea.KeyEnter), 0, 6, "alpha \nBeTa gamma\nsecond line\n- item\n", 1, 0},
		{"enter continues a bullet", named(tea.KeyEnter), 2, 6, "alpha BeTa gamma\nsecond line\n- item\n- \n", 3, 2},
		{"backspace", named(tea.KeyBackspace), 0, 6, "alphaBeTa gamma\nsecond line\n- item\n", 0, 5},
		{"ctrl+h", ctrl('h'), 0, 6, "alphaBeTa gamma\nsecond line\n- item\n", 0, 5},
		{"delete", named(tea.KeyDelete), 0, 5, "alphaBeTa gamma\nsecond line\n- item\n", 0, 5},
		{"ctrl+d", ctrl('d'), 0, 5, "alphaBeTa gamma\nsecond line\n- item\n", 0, 5},
		{"left", named(tea.KeyLeft), 0, 6, fixture, 0, 5},
		{"ctrl+b", ctrl('b'), 0, 6, fixture, 0, 5},
		{"right", named(tea.KeyRight), 0, 6, fixture, 0, 7},
		{"down", named(tea.KeyDown), 0, 6, fixture, 1, 6},
		{"ctrl+n", ctrl('n'), 0, 6, fixture, 1, 6},
		{"up", named(tea.KeyUp), 1, 6, fixture, 0, 6},
		{"ctrl+p", ctrl('p'), 1, 6, fixture, 0, 6},
		{"home", named(tea.KeyHome), 0, 6, fixture, 0, 0},
		{"end", named(tea.KeyEnd), 0, 6, fixture, 0, 16},
		{"ctrl+e", ctrl('e'), 0, 6, fixture, 0, 16},
		{"alt+left", tea.KeyPressMsg{Code: tea.KeyLeft, Mod: tea.ModAlt}, 0, 6, fixture, 0, 0},
		{"alt+b", altKey('b'), 0, 11, fixture, 0, 6},
		{"ctrl+left", tea.KeyPressMsg{Code: tea.KeyLeft, Mod: tea.ModCtrl}, 0, 11, fixture, 0, 6},
		{"alt+right", tea.KeyPressMsg{Code: tea.KeyRight, Mod: tea.ModAlt}, 0, 6, fixture, 0, 10},
		{"alt+f", altKey('f'), 0, 6, fixture, 0, 10},
		{"ctrl+right", tea.KeyPressMsg{Code: tea.KeyRight, Mod: tea.ModCtrl}, 0, 6, fixture, 0, 10},
		{"alt+backspace", tea.KeyPressMsg{Code: tea.KeyBackspace, Mod: tea.ModAlt}, 0, 6, "BeTa gamma\nsecond line\n- item\n", 0, 0},
		{"ctrl+w", ctrl('w'), 0, 6, "BeTa gamma\nsecond line\n- item\n", 0, 0},
		{"alt+delete", tea.KeyPressMsg{Code: tea.KeyDelete, Mod: tea.ModAlt}, 0, 5, "alpha gamma\nsecond line\n- item\n", 0, 5},
		{"alt+d", altKey('d'), 0, 5, "alpha gamma\nsecond line\n- item\n", 0, 5},
		{"ctrl+k", ctrl('k'), 0, 6, "alpha \nsecond line\n- item\n", 0, 6},
		{"ctrl+k joins at line end", ctrl('k'), 0, 16, "alpha BeTa gammasecond line\n- item\n", 0, 16},
		{"ctrl+u", ctrl('u'), 0, 6, "BeTa gamma\nsecond line\n- item\n", 0, 0},
		{"alt+c", altKey('c'), 0, 6, "alpha Beta gamma\nsecond line\n- item\n", 0, 10},
		{"alt+l", altKey('l'), 0, 6, "alpha beta gamma\nsecond line\n- item\n", 0, 10},
		{"alt+u", altKey('u'), 0, 6, "alpha BETA gamma\nsecond line\n- item\n", 0, 10},
		{"ctrl+home", tea.KeyPressMsg{Code: tea.KeyHome, Mod: tea.ModCtrl}, 1, 3, fixture, 0, 0},
		{"alt+<", altKey('<'), 1, 3, fixture, 0, 0},
		{"ctrl+end", tea.KeyPressMsg{Code: tea.KeyEnd, Mod: tea.ModCtrl}, 1, 3, fixture, 3, 0},
		{"alt+>", altKey('>'), 1, 3, fixture, 3, 0},
		{"pgdown clamps to the last row", named(tea.KeyPgDown), 0, 3, fixture, 3, 0},
		{"pgup clamps to the first row", named(tea.KeyPgUp), 1, 3, fixture, 0, 3},
		{"tab indents a bullet", named(tea.KeyTab), 2, 4, "alpha BeTa gamma\nsecond line\n  - item\n", 2, 6},
		{"shift+tab outdents", tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift}, 2, 4, fixture, 2, 4},
		{"ctrl+a selects all", ctrl('a'), 0, 6, fixture, 3, 0},
		{"ctrl+x cuts the line", ctrl('x'), 0, 6, "second line\n- item\n", 0, 6},
		{"ctrl+c copies and changes nothing", ctrl('c'), 0, 6, fixture, 0, 6},
		{"ctrl+z with nothing to undo", ctrl('z'), 0, 6, fixture, 0, 6},
		{"ctrl+t cycles a marker", ctrl('t'), 2, 4, "alpha BeTa gamma\nsecond line\n- TODO item\n", 2, 9},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			content := fixture
			if tc.name == "shift+tab outdents" {
				content = "alpha BeTa gamma\nsecond line\n  - item\n"
				tc.line, tc.col = 2, 6
			}
			e := editorAt(nil, "P", "/tmp/p.md", content, false, 40, 10, 0)
			setCursor(e, tc.line, tc.col)
			e.Update(tc.msg)
			if got := text(e); got != tc.want {
				t.Errorf("text = %q, want %q", got, tc.want)
			}
			if l, c := cursorRowCol(e); l != tc.wantL || c != tc.wantC {
				t.Errorf("cursor = (%d,%d), want (%d,%d)", l, c, tc.wantL, tc.wantC)
			}
		})
	}
}

func TestEditorKeyTableSaveAndEscape(t *testing.T) {
	quietTerm(t)
	e := editorAt(nil, "P", "/tmp/p.md", "x\n", false, 40, 10, 0)
	if res, _ := e.Update(ctrl('s')); !res.Save || res.Exit {
		t.Errorf("ctrl+s = %+v, want Save only", res)
	}
	if res, _ := e.Update(named(tea.KeyEsc)); !res.Exit {
		t.Errorf("esc on a clean buffer = %+v, want Exit", res)
	}
}

// Chords with no binding change nothing.
func TestEditorUnboundChordsAreInert(t *testing.T) {
	quietTerm(t)
	chords := []tea.KeyPressMsg{
		ctrl('g'),
		ctrl('r'),
		{Code: 'c', Mod: tea.ModCtrl | tea.ModShift},
		{Code: tea.KeyBackspace, Mod: tea.ModCtrl},
		{Code: tea.KeyDelete, Mod: tea.ModCtrl},
		{Code: tea.KeyUp, Mod: tea.ModAlt | tea.ModShift},
	}
	const content = "alpha beta gamma\nsecond\n"
	fresh := func() *EditorView {
		e := editorAt(nil, "P", "/tmp/p.md", content, false, 40, 10, 0)
		setCursor(e, 0, 6)
		return e
	}
	for _, c := range chords {
		t.Run(c.String(), func(t *testing.T) {
			e := fresh()
			wantRow, wantCol := cursorRowCol(e)
			res, _ := e.Update(c)
			if res != (EditorResult{}) {
				t.Errorf("result = %+v, want none", res)
			}
			row, col := cursorRowCol(e)
			if text(e) != content {
				t.Errorf("content changed: %q", text(e))
			}
			if row != wantRow || col != wantCol {
				t.Errorf("cursor moved to %d,%d, want %d,%d", row, col, wantRow, wantCol)
			}
			if _, ok := e.buf.Selection(); ok {
				t.Error("chord started a selection")
			}
		})
	}

	t.Run("alt+right still moves a word", func(t *testing.T) {
		e := fresh()
		_, before := cursorRowCol(e)
		e.Update(tea.KeyPressMsg{Code: tea.KeyRight, Mod: tea.ModAlt})
		if _, after := cursorRowCol(e); after <= before {
			t.Errorf("column %d -> %d, want it to advance", before, after)
		}
	})
	t.Run("ctrl+w still deletes a word", func(t *testing.T) {
		e := fresh()
		e.Update(tea.KeyPressMsg{Code: 'w', Mod: tea.ModCtrl})
		if want := "beta gamma\nsecond\n"; text(e) != want {
			t.Errorf("content %q, want %q", text(e), want)
		}
	})
}

// The terminal draws the cursor: the App's view carries its cell, and the
// prompts, which own the keys, hide it.
func TestEditorShowsTerminalCursor(t *testing.T) {
	a, _ := openEditor(t, map[string]string{"pages/P.md": "intro\n"}, "P")
	cur := a.View().Cursor
	if cur == nil {
		t.Fatal("no cursor while editing")
	}
	if cur.X != 2 || cur.Y != 1 {
		t.Errorf("cursor at (%d,%d), want (2,1): left inset and the margin row", cur.X, cur.Y)
	}
	a.Update(key("a"))
	a.Update(key("b"))
	if cur := a.View().Cursor; cur == nil || cur.X != 4 || cur.Y != 1 {
		t.Errorf("after typing: cursor %+v, want (4,1)", cur)
	}
	a.Update(named(tea.KeyEsc)) // dirty: exit prompt
	if a.editor == nil || a.editor.mode != confirmingExit {
		t.Fatal("setup: expected the exit prompt")
	}
	if cur := a.View().Cursor; cur != nil {
		t.Errorf("cursor %+v in the exit prompt, want none", cur)
	}
}

func TestEditorEnterBeyondDefaultMaxHeight(t *testing.T) {
	quietTerm(t)
	e := editorAt(nil, "P", "/tmp/p.md", strings.Repeat("line\n", 150), false, 80, 24, 149)
	cursorEnd(e)
	before := e.buf.Len()
	e.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if got := e.buf.Len(); got != before+1 {
		t.Errorf("line count %d -> %d, want one more", before, got)
	}
}
