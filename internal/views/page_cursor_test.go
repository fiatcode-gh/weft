package views

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/colorprofile"
	uv "github.com/charmbracelet/ultraviolet"
)

// cursorPage boots a PageView on a one-page graph at width 80.
func cursorPage(t *testing.T, content string, height int) *PageView {
	t.Helper()
	_, idx := writeGraph(t, map[string]string{"pages/P.md": content})
	return NewPageView(idx, "P", 80, height)
}

func bulletsOf(prefix string, n int) string {
	var b strings.Builder
	for i := range n {
		fmt.Fprintf(&b, "- %s %d\n", prefix, i)
	}
	return b.String()
}

// screenRow is the frame row the cursor row sits on.
func screenRow(p *PageView) int { return p.CursorRow() - p.Offset() }

func TestCursorRowDrawnUnderNoColor(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	p := cursorPage(t, "- a\n- b\n", 24)
	r := p.CursorRow()
	if r < 0 || plain(p.rowText(r)) == "" || !strings.Contains(plain(p.rowText(r)), "a") {
		t.Fatalf("CursorRow = %d, want the first non-blank row", r)
	}
	rows := frameCells(p.View(), 80, colorprofile.Ascii)
	for x, c := range rows[r] {
		if c.Style.Attrs&uv.AttrReverse == 0 {
			t.Fatalf("cursor row cell %d %q is not reverse video: %q", x, c.Content, c.Style.String())
		}
	}
	for x, c := range rows[r+1] {
		if c.Style.Attrs&uv.AttrReverse != 0 {
			t.Fatalf("next row cell %d %q is reverse video, only the cursor row is", x, c.Content)
		}
	}
}

func TestCursorRowDrawnInSelectionStyle(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	p := cursorPage(t, "- see [[Target]] here\n- b\n", 24)
	r := p.CursorRow()
	rows := frameCells(p.View(), 80, colorprofile.TrueColor)
	for x, c := range rows[r][:10] {
		if !sameColour(c.Style.Bg, 12) {
			t.Fatalf("cursor row cell %d %q lacks the selection background: %q", x, c.Content, c.Style.String())
		}
	}
	if c := rows[r+1][2]; sameColour(c.Style.Bg, 12) {
		t.Fatalf("the next row has the selection background: %q", c.Style.String())
	}

	p.CycleLink(+1)
	if p.CursorRow() != r {
		t.Fatalf("n moved the cursor to row %d, want the link's row %d", p.CursorRow(), r)
	}
	rows = frameCells(p.View(), 80, colorprofile.TrueColor)
	for x, c := range cellsShowing(t, rows, "Target") {
		if !sameColour(c.Style.Bg, 11) {
			t.Errorf("link cell %d %q lost the cursor background: %q", x, c.Content, c.Style.String())
		}
	}
	for x, c := range cellsShowing(t, rows, "here") {
		if !sameColour(c.Style.Bg, 12) {
			t.Errorf("cell %d %q right of the link lost the selection background: %q", x, c.Content, c.Style.String())
		}
	}
}

func TestJMovesCursorBeforeScrolling(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	p := cursorPage(t, bulletsOf("item", 60), 10)
	first := p.CursorRow()
	for range 5 {
		p.LineDown()
	}
	if p.Offset() != 0 || p.CursorRow() != first+5 {
		t.Fatalf("after 5 moves: offset %d, cursor row %d, want 0 and %d", p.Offset(), p.CursorRow(), first+5)
	}
	for range 20 {
		p.LineDown()
	}
	if got := screenRow(p); got != 7 {
		t.Fatalf("cursor on screen row %d after 25 moves, want the last window row 7", got)
	}
	for range 3 {
		p.LineUp()
	}
	if got := screenRow(p); got != 4 {
		t.Fatalf("cursor on screen row %d after moving up 3, want 4 (the window holds still)", got)
	}
}

func TestCursorSkipsBlankRows(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	p := cursorPage(t, "# H\n\npara\n\n- b\n", 24)
	at := func() string { return strings.TrimSpace(plain(p.rowText(p.CursorRow()))) }
	if got := at(); !strings.Contains(got, "H") {
		t.Fatalf("cursor starts on %q, want the heading", got)
	}
	p.LineDown()
	if got := at(); !strings.Contains(got, "para") {
		t.Fatalf("LineDown landed on %q, want the para row", got)
	}
	p.LineUp()
	if got := at(); !strings.Contains(got, "H") {
		t.Fatalf("LineUp landed on %q, want the heading", got)
	}
}

func TestHalfPageAndTopBottomMoveCursor(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	p := cursorPage(t, bulletsOf("item", 60), 10) // window 8, half 4
	first := p.CursorRow()
	p.HalfPageDown()
	if p.Offset() != 4 || p.CursorRow() != first+4 {
		t.Fatalf("ctrl+d: offset %d row %d, want 4 and %d", p.Offset(), p.CursorRow(), first+4)
	}
	p.HalfPageDown()
	if p.Offset() != 8 || p.CursorRow() != first+8 {
		t.Fatalf("ctrl+d again: offset %d row %d, want 8 and %d", p.Offset(), p.CursorRow(), first+8)
	}
	p.HalfPageUp()
	if p.Offset() != 4 || p.CursorRow() != first+4 {
		t.Fatalf("ctrl+u: offset %d row %d, want 4 and %d", p.Offset(), p.CursorRow(), first+4)
	}
	p.GotoBottom()
	last := p.rowCount() - 1
	for !nonBlank(p.rowText(last)) {
		last--
	}
	if p.CursorRow() != last || p.Offset() != p.rowCount()-8 {
		t.Fatalf("G: row %d offset %d, want %d and %d", p.CursorRow(), p.Offset(), last, p.rowCount()-8)
	}
	p.GotoTop()
	if p.Offset() != 0 || p.CursorRow() != first {
		t.Fatalf("g: offset %d row %d, want 0 and %d", p.Offset(), p.CursorRow(), first)
	}
}

func TestNMovesCursorToLinkRow(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	var b strings.Builder
	for i := range 60 {
		switch i {
		case 30, 45:
			fmt.Fprintf(&b, "- link to [[T%d]]\n", i)
		default:
			fmt.Fprintf(&b, "- item %d\n", i)
		}
	}
	p := cursorPage(t, b.String(), 12)
	onLink := func(want string) {
		t.Helper()
		if got := plain(p.rowText(p.CursorRow())); !strings.Contains(got, want) {
			t.Fatalf("cursor row %d shows %q, want %q", p.CursorRow(), got, want)
		}
		if r := screenRow(p); r < 0 || r >= 10 {
			t.Fatalf("cursor row off screen: screen row %d", r)
		}
	}
	p.CycleLink(+1)
	onLink("T30")
	p.CycleLink(+1)
	onLink("T45")
	p.CycleLink(+1)
	onLink("T30") // wraps
	p.CycleLink(-1)
	onLink("T45") // wraps back
}
