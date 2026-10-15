package views

import (
	"testing"

	"github.com/fiatcode-gh/weft/v2/internal/buffer"
)

// vimClickRow is the screen row of buffer line n (one display row per line in
// the texts these tests use), read off the terminal cursor.
func vimClickRow(t *testing.T, e *EditorView, n int) int {
	t.Helper()
	saved := e.buf.Cursor()
	e.buf.MoveTo(buffer.Pos{Line: n}, false)
	e.afterKey(false)
	e.View()
	_, y := cursorCell(t, e)
	e.buf.MoveTo(saved, false)
	e.afterKey(false)
	e.View()
	return y
}

func TestVimClickNormal(t *testing.T) {
	quietTerm(t)
	const body = "- alpha\n\n- bc\n"
	t.Run("past the end lands on the last grapheme", func(t *testing.T) {
		e := vimEditor(nil, body, 0)
		y := vimClickRow(t, e, 2)
		e.Click(70, y)
		if got, want := e.buf.Cursor(), (buffer.Pos{Line: 2, Col: 3}); got != want {
			t.Errorf("cursor = %v, want %v", got, want)
		}
		if e.vim.mode != vimNormal {
			t.Errorf("mode = %v, want normal", e.vim.mode)
		}
	})
	t.Run("empty line lands at column 0", func(t *testing.T) {
		e := vimEditor(nil, body, 1) // live preview folds an unvisited blank line away
		y := vimClickRow(t, e, 1)
		e.Click(70, y)
		if got, want := e.buf.Cursor(), (buffer.Pos{Line: 1}); got != want {
			t.Errorf("cursor = %v, want %v", got, want)
		}
	})
	t.Run("clears a pending command", func(t *testing.T) {
		e := vimEditor(nil, body, 0)
		vimType(e, "2d")
		if len(e.vim.pending) == 0 {
			t.Fatal("2d left nothing pending")
		}
		e.Click(2, vimClickRow(t, e, 2))
		if len(e.vim.pending) != 0 {
			t.Errorf("pending = %v after a click, want none", e.vim.pending)
		}
		vimType(e, "w") // a bare motion now, not the operand of the old 2d
		if e.dirty() {
			t.Error("the cleared 2d still ran")
		}
	})
}

func TestVimClickVisual(t *testing.T) {
	quietTerm(t)
	for _, v := range []string{"v", "V"} {
		e := vimEditor(nil, "- alpha\n\n- bc\n", 0)
		vimType(e, v+"jl")
		if _, ok := e.selection(); !ok {
			t.Fatalf("%s left no selection to test with", v)
		}
		e.Click(70, vimClickRow(t, e, 2))
		if e.vim.mode != vimNormal {
			t.Errorf("%s then click: mode = %v, want normal", v, e.vim.mode)
		}
		if got, want := e.buf.Cursor(), (buffer.Pos{Line: 2, Col: 3}); got != want {
			t.Errorf("%s then click: cursor = %v, want %v", v, got, want)
		}
		if r, ok := e.selection(); ok {
			t.Errorf("%s then click: selection %v remains", v, r)
		}
	}
}

func TestVimClickInsert(t *testing.T) {
	quietTerm(t)
	e := vimEditor(nil, "- alpha\n\n- bc\n", 0)
	vimType(e, "iZ")
	e.Click(70, vimClickRow(t, e, 2))
	if e.vim.mode != vimInsert {
		t.Fatalf("mode = %v after a click in insert, want insert", e.vim.mode)
	}
	if got, want := e.buf.Cursor(), (buffer.Pos{Line: 2, Col: 4}); got != want {
		t.Fatalf("cursor = %v, want the line end %v", got, want)
	}
	vimType(e, "X")
	if got := e.buf.Line(2); got != "- bcX" {
		t.Errorf("line 2 = %q, want typing at the line end", got)
	}
	e.Update(escKey)
	e.Update(key("u"))
	if got := e.buf.Line(0); got != "- alpha" || e.dirty() {
		t.Errorf("one u left %q dirty=%v, want the session undone as one step", got, e.dirty())
	}
}

func TestVimWheel(t *testing.T) {
	quietTerm(t)
	open := func() *EditorView {
		e := editorAt(nil, "P", "/tmp/p.md", bulletText(60), false, 40, 10, 0)
		e.enableVim()
		return e
	}
	t.Run("normal lands on a character", func(t *testing.T) {
		e := open()
		vimType(e, "$")
		e.Wheel(1)
		cur := e.buf.Cursor()
		if cur.Line == 0 {
			t.Fatal("the wheel left the cursor on line 0")
		}
		if want := e.buf.LineLast(cur); cur != want {
			t.Errorf("cursor = %v, want the last character %v", cur, want)
		}
	})
	t.Run("visual stays and grows", func(t *testing.T) {
		e := open()
		vimType(e, "v")
		e.Wheel(1)
		if e.vim.mode != vimVisual {
			t.Fatalf("mode = %v, want visual", e.vim.mode)
		}
		r, ok := e.selection()
		if !ok || r.End.Line == 0 {
			t.Errorf("selection = %v, %v, want it to reach the moved cursor", r, ok)
		}
	})
}

func TestVimMouseIgnoredInEx(t *testing.T) {
	quietTerm(t)
	e := editorAt(nil, "P", "/tmp/p.md", bulletText(60), false, 40, 10, 0)
	e.enableVim()
	vimType(e, ":w")
	cur, top := e.buf.Cursor(), e.top
	e.Click(2, 3)
	e.Wheel(1)
	if got := e.buf.Cursor(); got != cur {
		t.Errorf("cursor = %v, want %v", got, cur)
	}
	if e.top != top {
		t.Errorf("top = %v, want %v", e.top, top)
	}
	if e.vim.ex == nil || *e.vim.ex != "w" {
		t.Errorf("ex line = %v, want it left open with w", e.vim.ex)
	}
}
