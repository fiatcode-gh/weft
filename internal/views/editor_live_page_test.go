package views

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// livePageAt is the cursor's line and column and its raw row.
func livePageAt(e *EditorView) string {
	row, _ := e.rawRow()
	c := e.buf.Cursor()
	return fmt.Sprintf("%d:%d (raw row %d)", c.Line, c.Col, row)
}

// A page walks display rows, and a row of a line that is on screen raw (the
// cursor's own) is that raw row, so a line taller than the window is paged
// through exactly as in source mode.
func TestLivePagingThroughLineTallerThanWindow(t *testing.T) {
	skipInSourceRun(t)
	quietTerm(t)
	page := "intro\n\n" + strings.Repeat("x", 2000) + "\n\nafter\n"
	open := func(source bool) *EditorView {
		e := NewEditorView(nil, "P", "/tmp/p.md", page, false, 40, 12, Anchor{Line: 2, ScreenRow: 1}, nil, source)
		_ = e.View()
		return e
	}
	live, src := open(false), open(true)
	if c := live.buf.Cursor(); c.Line != 2 || c.Col != 0 {
		t.Fatalf("setup: cursor at %d:%d, want 2:0", c.Line, c.Col)
	}
	for i := range 4 {
		tap(live, tea.KeyPgDown)
		tap(src, tea.KeyPgDown)
		if l, s := livePageAt(live), livePageAt(src); l != s {
			t.Fatalf("PgDn %d: live preview at %s, source at %s", i+1, l, s)
		}
	}
	if c := live.buf.Cursor(); c.Line != 2 || c.Col <= 0 {
		t.Fatalf("after four PgDn the cursor is at %d:%d, want it well inside line 2", c.Line, c.Col)
	}
	for i := range 3 {
		tap(live, tea.KeyPgUp)
		tap(src, tea.KeyPgUp)
		if l, s := livePageAt(live), livePageAt(src); l != s {
			t.Fatalf("PgUp %d: live preview at %s, source at %s", i+1, l, s)
		}
	}
	setCursor(live, 2, 1500)
	setCursor(src, 2, 1500)
	live.syncReveal()
	for i := range 3 {
		tap(live, tea.KeyPgUp)
		tap(src, tea.KeyPgUp)
		if l, s := livePageAt(live), livePageAt(src); l != s {
			t.Fatalf("PgUp %d from column 1500: live preview at %s, source at %s", i+1, l, s)
		}
	}
}

// A page down from inside the last line's wrapped rows, with only the trailing
// empty line (which has no row) after it, goes to the line's last row, never
// back to its first.
func TestLivePgDownInWrappedLastLineGoesForward(t *testing.T) {
	skipInSourceRun(t)
	quietTerm(t)
	e := liveEditor("- "+strings.Repeat("wrapping words ", 14)+"\n", 30, 14)
	if e.buf.Len() != 2 {
		t.Fatalf("setup: %d lines, want the bullet and the empty one after the final newline", e.buf.Len())
	}
	rows := e.line(0).rows
	if len(rows) < 5 {
		t.Fatalf("setup: the bullet wraps to %d rows, want at least 5", len(rows))
	}
	setCursor(e, 0, rows[3].Start)
	e.syncReveal()
	if row, _ := e.rawRow(); row != 3 {
		t.Fatalf("setup: cursor on raw row %d, want 3", row)
	}
	tap(e, tea.KeyPgDown)
	if row, _ := e.rawRow(); e.buf.Cursor().Line != 0 || row != len(rows)-1 {
		t.Fatalf("PgDn: cursor at %s, want the line's last raw row %d", livePageAt(e), len(rows)-1)
	}
	tap(e, tea.KeyPgDown)
	if row, _ := e.rawRow(); e.buf.Cursor().Line != 0 || row != len(rows)-1 {
		t.Errorf("second PgDn: cursor at %s, want it to stay on the last raw row %d", livePageAt(e), len(rows)-1)
	}
}

// TestLivePagingMoves runs the scripts of TestEditorPagingMoves in live
// preview. Their traces count source rows, so what is checked is what does not
// depend on the look: a page moves the cursor h-1 display rows of the layout it
// started in (clamped at both ends); it lands on the raw row of a line that is
// on screen raw (the cursor's) and on the first raw row of any other; and the
// screen column is the remembered one, cut at the row's end.
func TestLivePagingMoves(t *testing.T) {
	skipInSourceRun(t)
	quietTerm(t)
	tests := []struct {
		name         string
		content      string
		w, h, anchor int
		right        int
		keys         string
	}{
		{"plain-h12-end", plainLines(120), 60, 12, 110, 0, "DDDDDUUU"},
		{"mixed-col", mixedLines(90), 40, 14, 0, 12, "DDDDDDDDDUUUUUUUUUUDDUU"},
		{"mixed-col2", mixedLines(90), 40, 11, 40, 20, "DDDUUUUDDDDDDDDDDDDDDDDDDDDDDD"},
		{"wrapped-w30", wrappedLines(40), 30, 12, 0, 7, "DDDDDDDDDDDDDDUUUUUUUUUUUUUUUUUU"},
		{"wrapped-end", wrappedLines(40), 30, 12, 38, 4, "DDDUUUDDD"},
		{"tiny-h5", plainLines(120), 60, 5, 0, 0, "DDDUUU"},
		{"short-buf", "- a\n- b\n- c\n", 60, 12, 0, 0, "DDUUDU"},
		{"empty", "", 60, 12, 0, 0, "DUDU"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e := editorAtLook(nil, "P", "/tmp/p.md", tc.content, false, tc.w, tc.h, tc.anchor, false)
			_ = e.View()
			for range tc.right {
				e.Update(tea.KeyPressMsg{Code: tea.KeyRight})
			}
			h := e.textHeight()
			_, goal := e.cursorRow()
			for n, k := range tc.keys {
				code, dir := tea.KeyPgUp, -1
				if k == 'D' {
					code, dir = tea.KeyPgDown, +1
				}
				// The layout before the key: every display row of a line that has any.
				var rows []viewPos
				for l := range e.buf.Len() {
					for r := range e.rowCount(l) {
						rows = append(rows, viewPos{l, r})
					}
				}
				cur, _ := e.cursorRow()
				at := -1
				for i, p := range rows {
					if p == cur {
						at = i
					}
				}
				if at < 0 {
					t.Fatalf("key %d (%c): the cursor's row %v is not a display row", n, k, cur)
				}
				target := rows[clampInt(at+dir*(h-1), 0, len(rows)-1)]
				_, raw := e.liveRows(target.line)
				wantRow := 0
				if raw || target.line == cur.line {
					wantRow = clampInt(target.row-e.leadRows(target.line), 0, len(e.line(target.line).rows)-1)
				}
				e.Update(tea.KeyPressMsg{Code: code})
				c := e.line(target.line)
				maxOff := e.geo.OffsetAt(c.text, c.info, c.rows, wantRow, 1<<30)
				_, maxX := e.geo.CellX(c.text, c.info, c.rows, maxOff)
				wantX := min(goal, maxX)
				gotRow, gotX := e.rawRow()
				if e.buf.Cursor().Line != target.line || gotRow != wantRow || gotX != wantX {
					t.Fatalf("key %d (%c) from %v: cursor on line %d raw row %d column %d, want line %d raw row %d column %d",
						n, k, cur, e.buf.Cursor().Line, gotRow, gotX, target.line, wantRow, wantX)
				}
				_ = e.View()
			}
		})
	}
}
