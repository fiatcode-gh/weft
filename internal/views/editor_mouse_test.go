package views

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/fiatcode-gh/weft/v2/internal/buffer"
)

// bulletText is n one-row bullets, so a display row is a buffer line in both
// looks.
func bulletText(n int) string {
	var b strings.Builder
	for i := range n {
		fmt.Fprintf(&b, "- item %d\n", i)
	}
	return b.String()
}

// cursorCell is the screen cell of the terminal cursor.
func cursorCell(t *testing.T, e *EditorView) (x, y int) {
	t.Helper()
	c := e.Cursor()
	if c == nil {
		t.Fatal("cursor is off screen")
	}
	return c.X, c.Y
}

func TestMouseEditorRouting(t *testing.T) {
	files := map[string]string{"pages/Alpha.md": escPage()}
	a := newApp(t, files, 80, 24)
	a.navigate("Alpha")
	a.Update(key("e"))
	if a.editor == nil {
		t.Fatal("editor did not open")
	}
	before := graphFiles(t, a.graphPath)

	x, y := cellOf(t, a, "L7")
	a.Update(click(x, y))
	if got := a.editor.buf.Cursor().Line; got != 7 {
		t.Errorf("click on L7 put the cursor on line %d, want 7", got)
	}

	top := a.editor.top
	a.Update(wheel(10, 5, 1))
	if a.editor.top == top {
		t.Errorf("wheel down left the window at %v", top)
	}
	if a.editor.dirty() {
		t.Error("mouse made the buffer dirty")
	}
	after := graphFiles(t, a.graphPath)
	if fmt.Sprint(before) != fmt.Sprint(after) {
		t.Error("mouse wrote to the graph")
	}
}

func TestEditorMouseWheel(t *testing.T) {
	quietTerm(t)
	open := func() *EditorView { return editorAt(nil, "P", "/tmp/p.md", bulletText(60), false, 40, 10, 0) }

	t.Run("visible cursor stays", func(t *testing.T) {
		e := open()
		setCursor(e, 5, 3)
		e.View()
		_, y0 := cursorCell(t, e)
		e.Wheel(1)
		if e.top != (viewPos{2, 0}) {
			t.Errorf("top = %v, want {2 0}", e.top)
		}
		if got := e.buf.Cursor(); got != (buffer.Pos{Line: 5, Col: 3}) {
			t.Errorf("cursor moved to %v", got)
		}
		if _, y := cursorCell(t, e); y != y0-3 {
			t.Errorf("cursor row %d, want %d", y, y0-3)
		}
	})

	t.Run("cursor on first row follows at goal column", func(t *testing.T) {
		e := open()
		e.Wheel(1)
		setCursor(e, 2, 4)
		x0, _ := cursorCell(t, e)
		e.Wheel(1)
		if e.top != (viewPos{5, 0}) {
			t.Fatalf("top = %v, want {5 0}", e.top)
		}
		if got := e.buf.Cursor(); got != (buffer.Pos{Line: 5, Col: 4}) {
			t.Errorf("cursor = %v, want line 5 col 4", got)
		}
		x, y := cursorCell(t, e)
		if x != x0 || y != 0 {
			t.Errorf("cursor cell (%d,%d), want (%d,0)", x, y, x0)
		}
		if !e.goalOK {
			t.Error("wheel dropped the goal column")
		}
	})

	t.Run("up at the top changes nothing", func(t *testing.T) {
		e := open()
		e.Wheel(-1) // live preview opens a row below the margin row
		if e.top != (viewPos{0, -1}) {
			t.Fatalf("top = %v, want the margin row", e.top)
		}
		top, cur := e.top, e.buf.Cursor()
		e.Wheel(-1)
		if e.top != top || e.buf.Cursor() != cur {
			t.Errorf("top %v cursor %v, want %v %v", e.top, e.buf.Cursor(), top, cur)
		}
	})

	t.Run("down at the end changes nothing", func(t *testing.T) {
		e := open()
		e.buf.MoveTo(e.buf.End(), false)
		e.View()
		top, cur := e.top, e.buf.Cursor()
		e.Wheel(1)
		if e.top != top || e.buf.Cursor() != cur {
			t.Errorf("top %v cursor %v, want %v %v", e.top, e.buf.Cursor(), top, cur)
		}
	})

	t.Run("up with the cursor at the bottom", func(t *testing.T) {
		e := open()
		e.buf.MoveTo(e.buf.End(), false)
		e.View()
		top := e.top
		e.Wheel(-1)
		if want := e.rowsBefore(top, 3); e.top != want {
			t.Errorf("top = %v, want %v", e.top, want)
		}
		_, y := cursorCell(t, e)
		if y != e.textHeight()-1 {
			t.Errorf("cursor row %d, want the last window row %d", y, e.textHeight()-1)
		}
		if got := e.buf.Cursor().Line; got != e.windowBottom().line {
			t.Errorf("cursor line %d, want %d", got, e.windowBottom().line)
		}
	})
}

func TestEditorClickSource(t *testing.T) {
	quietTerm(t)
	long := strings.TrimSpace(strings.Repeat("alpha beta gamma ", 8))
	content := "# Title\nplain ascii text\n中文字符 wide\n" + long + "\ntail\n"
	e := sourceEditorAt(nil, "P", "/tmp/p.md", content, false, 40, 20, 0)

	// aim clicks the cell the cursor shows for (line, col) and expects the
	// cursor back on that position, from a cursor parked elsewhere.
	aim := func(name string, line, col, dx int) {
		t.Helper()
		setCursor(e, line, col)
		want := e.buf.Cursor()
		x, y := cursorCell(t, e)
		setCursor(e, 4, 0)
		e.View()
		e.Click(x+dx, y)
		if got := e.buf.Cursor(); got != want {
			t.Errorf("%s: click (%d,%d) landed at %v, want %v", name, x+dx, y, got, want)
		}
	}
	aim("ascii", 1, 6, 0)
	aim("cjk left half", 2, 1, 0)
	aim("cjk right half", 2, 1, 1)
	aim("continuation row", 3, 60, 0)
	if c := e.line(3); len(c.rows) < 2 {
		t.Fatalf("test line did not wrap: %d rows", len(c.rows))
	}

	setCursor(e, 1, 6)
	_, y := cursorCell(t, e)
	setCursor(e, 4, 0)
	e.View()
	e.Click(0, y)
	if got := e.buf.Cursor(); got != (buffer.Pos{Line: 1}) {
		t.Errorf("left margin click landed at %v, want line 1 col 0", got)
	}
}

func TestEditorClickLive(t *testing.T) {
	quietTerm(t)
	content := "# Heading\n\n- bullet one\n\npara one\npara two joined\n\ntail\n"
	e := editorAtLook(nil, "P", "/tmp/p.md", content, false, 50, 20, 0, false)
	e.buf.MoveTo(e.buf.End(), false)
	e.View()

	rowOf := func(word string) (x, y int) {
		t.Helper()
		for y, l := range strings.Split(plain(e.View()), "\n")[:e.textHeight()] {
			if i := strings.Index(l, word); i >= 0 {
				return len([]rune(l[:i])), y
			}
		}
		t.Fatalf("%q is not on screen:\n%s", word, plain(e.View()))
		return 0, 0
	}
	for _, tc := range []struct {
		word string
		line int
	}{{"Heading", 0}, {"bullet", 2}, {"para one", 4}, {"joined", 4}} {
		e.buf.MoveTo(e.buf.End(), false)
		e.View()
		x, y := rowOf(tc.word)
		e.Click(x, y)
		cur := e.buf.Cursor()
		if cur.Line != tc.line {
			t.Errorf("click on %q: cursor line %d, want %d", tc.word, cur.Line, tc.line)
			continue
		}
		c := e.line(cur.Line)
		if want := e.geo.OffsetAt(c.text, c.info, c.rows, 0, x); tc.word != "joined" && cur.Col != want {
			t.Errorf("click on %q: col %d, want %d", tc.word, cur.Col, want)
		}
		if !e.isShown(tc.line) {
			t.Errorf("click on %q: line not revealed", tc.word)
		}
		if cc := e.Cursor(); cc == nil || cc.Y >= e.textHeight() {
			t.Errorf("click on %q: cursor off screen", tc.word)
		}
	}
}

func TestEditorClickEnds(t *testing.T) {
	quietTerm(t)
	e := editorAt(nil, "P", "/tmp/p.md", "- short\n- longer line here\n- x\n", false, 40, 12, 0)

	setCursor(e, 2, 0)
	e.View()
	e.Click(30, 1) // row 0 of line 0, far past its text
	if got := e.buf.Cursor(); got != (buffer.Pos{Line: 0, Col: len("- short")}) {
		t.Errorf("past the end: %v, want line 0 end", got)
	}

	e.Click(5, e.textHeight()-1)
	if got := e.buf.Cursor(); got != e.buf.End() {
		t.Errorf("below the last row: %v, want %v", got, e.buf.End())
	}

	setCursor(e, 2, 0)
	e.View()
	if e.top.line != 0 || e.top.row > 0 {
		t.Fatalf("top = %v, want the top of line 0", e.top)
	}
	e.Click(3, 0)
	c := e.line(0)
	if got, want := e.buf.Cursor(), (buffer.Pos{Line: 0, Col: e.geo.OffsetAt(c.text, c.info, c.rows, 0, 3)}); got != want {
		t.Errorf("margin row: %v, want %v", got, want)
	}
}

func TestEditorClickClearsSelection(t *testing.T) {
	quietTerm(t)
	e := editorAt(nil, "P", "/tmp/p.md", bulletText(5), false, 40, 12, 0)
	shiftRight := tea.KeyPressMsg{Code: tea.KeyRight, Mod: tea.ModShift}
	press(e, shiftRight, shiftRight, shiftRight)
	if _, ok := e.buf.Selection(); !ok {
		t.Fatal("no selection to clear")
	}
	e.Click(4, 3)
	if _, ok := e.buf.Selection(); ok {
		t.Error("click kept the selection")
	}
}

func TestEditorClickIgnoredRows(t *testing.T) {
	quietTerm(t)
	type state struct {
		cur  buffer.Pos
		top  viewPos
		text string
		mode editorMode
	}
	snap := func(e *EditorView) state { return state{e.buf.Cursor(), e.top, text(e), e.mode} }
	open := func() *EditorView {
		e := editorAt(loadFixture(t), "P", "/tmp/p.md", strings.Replace(bulletText(40), "- item 3\n", "- TODO item 3\n", 1), false, 40, 12, 0)
		setCursor(e, 3, 2)
		e.View()
		return e
	}

	t.Run("rule and status", func(t *testing.T) {
		e := open()
		before := snap(e)
		e.Click(3, e.textHeight())
		e.Click(3, e.height-1)
		if snap(e) != before {
			t.Errorf("state changed: %v -> %v", before, snap(e))
		}
	})

	t.Run("completion strip", func(t *testing.T) {
		e := open()
		typeRunes(e, "[[")
		if !e.completer.active || e.completer.rows() == 0 {
			t.Fatal("strip not open")
		}
		before := snap(e)
		for y := e.textHeight(); y < e.textHeight()+e.completer.rows(); y++ {
			e.Click(3, y)
		}
		if snap(e) != before || !e.completer.active {
			t.Errorf("strip click changed state: %v -> %v, active %v", before, snap(e), e.completer.active)
		}
	})

	for name, openBar := range map[string]func(*EditorView){
		"find bar":    func(e *EditorView) { press(e, ctrl('f')) },
		"date prompt": func(e *EditorView) { press(e, altKey('s')) },
		"exit prompt": func(e *EditorView) { typeRunes(e, "x"); press(e, tea.KeyPressMsg{Code: tea.KeyEsc}) },
	} {
		t.Run(name, func(t *testing.T) {
			e := open()
			openBar(e)
			e.View()
			if e.find == nil && e.prompt == nil && e.mode == editing {
				t.Fatal("bar did not open")
			}
			before := snap(e)
			e.Click(3, 1)
			e.Wheel(1)
			e.Wheel(-1)
			if snap(e) != before {
				t.Errorf("state changed: %v -> %v", before, snap(e))
			}
		})
	}
}
