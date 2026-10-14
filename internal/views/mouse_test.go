package views

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func statusLine(a *App) string {
	lines := frameLines(a)
	return lines[len(lines)-1]
}

func wantMouse(t *testing.T, a *App, mode tea.MouseMode, marker bool, when string) {
	t.Helper()
	if got := a.View().MouseMode; got != mode {
		t.Errorf("%s: MouseMode = %v, want %v", when, got, mode)
	}
	if got := strings.Contains(statusLine(a), "mouse off"); got != marker {
		t.Errorf("%s: 'mouse off' in status = %v, want %v: %q", when, got, marker, statusLine(a))
	}
}

func TestMouseDefaultOnAndToggle(t *testing.T) {
	a := bootApp(t)
	wantMouse(t, a, tea.MouseModeCellMotion, false, "start")
	a.Update(key("m"))
	wantMouse(t, a, tea.MouseModeNone, true, "after m")
	a.Update(key("m"))
	wantMouse(t, a, tea.MouseModeCellMotion, false, "after second m")

	a.Update(key("m"))
	a.setHint("x")
	if strings.Contains(statusLine(a), "mouse off") {
		t.Errorf("marker shown while a hint is up: %q", statusLine(a))
	}
}

func TestMouseToggleInSidebarAndPalette(t *testing.T) {
	a := sidebarApp(t, 120, 30)
	a.Update(key("1"))
	if !a.side.focused {
		t.Fatal("sidebar not focused after 1")
	}
	a.Update(key("m"))
	wantMouse(t, a, tea.MouseModeNone, true, "sidebar m")
	if !a.side.focused {
		t.Error("m moved focus out of the sidebar")
	}

	a.Update(key(":"))
	typeText(a, "mouse")
	a.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	wantMouse(t, a, tea.MouseModeCellMotion, false, "palette")
}

func TestMouseKeyIsTextInEditorAndPrompts(t *testing.T) {
	a := bootApp(t)
	a.Update(key("e"))
	if a.editor == nil {
		t.Fatal("editor did not open")
	}
	before := text(a.editor)
	a.Update(key("m"))
	if got := text(a.editor); got == before || !strings.Contains(got, "m") {
		t.Errorf("m not inserted into the buffer")
	}
	if a.View().MouseMode != tea.MouseModeCellMotion {
		t.Error("m in the editor toggled the mouse")
	}

	b := bootApp(t)
	b.Update(key("c"))
	if b.capture == nil {
		t.Fatal("capture did not open")
	}
	b.Update(key("m"))
	if b.View().MouseMode != tea.MouseModeCellMotion {
		t.Error("m in capture toggled the mouse")
	}
	if !strings.Contains(appText(b), "m") {
		t.Error("capture frame lacks the typed m")
	}

	c := bootApp(t)
	c.Update(tea.KeyPressMsg{Code: 'p', Mod: tea.ModCtrl})
	if c.active == nil {
		t.Fatal("picker did not open")
	}
	c.Update(key("m"))
	if c.View().MouseMode != tea.MouseModeCellMotion {
		t.Error("m in the picker toggled the mouse")
	}
}

func click(x, y int) tea.MouseClickMsg {
	return tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft}
}

func wheel(x, y, dir int) tea.MouseWheelMsg {
	b := tea.MouseWheelDown
	if dir < 0 {
		b = tea.MouseWheelUp
	}
	return tea.MouseWheelMsg{X: x, Y: y, Button: b}
}

// cellOf is the screen cell of the first occurrence of text in the frame.
func cellOf(t *testing.T, a *App, text string) (x, y int) {
	t.Helper()
	for y, line := range frameLines(a) {
		if i := strings.Index(line, text); i >= 0 {
			return ansi.StringWidth(line[:i]), y
		}
	}
	t.Fatalf("%q not on screen:\n%s", text, appText(a))
	return 0, 0
}

// readState is everything a read-view mouse event may change.
type readState struct {
	text         string
	page         string
	row, off     int
	cursor, hist int
	focused      bool
}

func readStateOf(a *App) readState {
	return readState{appText(a), a.page.Page(), a.page.CursorRow(), a.page.Offset(), a.page.Cursor(), a.histIdx, a.side.focused}
}

func longApp(t *testing.T, w, h int) *App {
	t.Helper()
	a := newApp(t, map[string]string{"pages/Long.md": escPage()}, w, h)
	a.navigate("Long")
	return a
}

func TestMouseReadWheel(t *testing.T) {
	a := longApp(t, 80, 24)
	for range 10 {
		a.page.LineDown()
	}
	row := a.page.CursorRow()
	a.Update(wheel(10, 5, +1))
	if a.page.Offset() != 3 || a.page.CursorRow() != row {
		t.Errorf("wheel down: offset %d row %d, want 3 and %d", a.page.Offset(), a.page.CursorRow(), row)
	}

	a.page.Reposition(a.page.Offset(), a.page.Offset())
	a.Update(wheel(10, 5, +1))
	if a.page.Offset() != 6 || a.page.CursorRow() != 6 {
		t.Errorf("cursor on the top row: offset %d row %d, want 6 and 6", a.page.Offset(), a.page.CursorRow())
	}

	b := longApp(t, 80, 24)
	before := readStateOf(b)
	b.Update(wheel(10, 5, -1))
	if got := readStateOf(b); got != before {
		t.Errorf("wheel up at the top changed the view: %+v -> %+v", before, got)
	}

	b.Update(key("G"))
	before = readStateOf(b)
	b.Update(wheel(10, 5, +1))
	if got := readStateOf(b); got != before {
		t.Errorf("wheel down at the bottom changed the view: %+v -> %+v", before, got)
	}
	for range 10 {
		b.page.LineUp()
	}
	row, off := b.page.CursorRow(), b.page.Offset()
	b.Update(wheel(10, 5, -1))
	if b.page.Offset() != off-3 || b.page.CursorRow() != row {
		t.Errorf("wheel up from the bottom: offset %d row %d, want %d and %d", b.page.Offset(), b.page.CursorRow(), off-3, row)
	}

	b.setHint("x")
	b.Update(wheel(10, 5, -1))
	if b.hint != "" {
		t.Errorf("hint %q survived a wheel", b.hint)
	}
}

// twinLinkClick clicks the link shown as display in one app and reaches the
// same link (index k) with n and enter in a second; both must end alike.
func twinLinkClick(t *testing.T, mk func() *App, display string, k int) {
	t.Helper()
	a, b := mk(), mk()
	start := a.page.Page()
	x, y := cellOf(t, a, display)
	a.Update(click(x, y))
	for i := 0; i < 20 && b.page.Cursor() != k; i++ {
		b.Update(key("n"))
	}
	if b.page.Cursor() != k {
		t.Fatalf("n never reached link %d", k)
	}
	b.Update(keyEnterK)
	if a.page.Page() == start {
		t.Fatalf("click on %q did not follow the link", display)
	}
	if sa, sb := readStateOf(a), readStateOf(b); sa != sb {
		t.Errorf("click and n+enter differ:\n click %+v\n keys  %+v", sa, sb)
	}
}

func TestMouseLinkClickMatchesKeys(t *testing.T) {
	mk := func() *App {
		a := newApp(t, map[string]string{
			"pages/Alpha.md": "- Links to [[Beta]] and [[Gamma|the other one]] here\n- x\n",
			"pages/Beta.md":  "- b\n",
			"pages/Gamma.md": "- g\n",
		}, 80, 24)
		a.navigate("Alpha")
		return a
	}
	for k, display := range []string{"Beta", "the other one"} {
		t.Run(display, func(t *testing.T) { twinLinkClick(t, mk, display, k) })
	}
}

func TestMouseWrappedLinkClick(t *testing.T) {
	mk := func() *App {
		a := newApp(t, map[string]string{
			"pages/kb___notes.md": "- Knowledge-base notes under the kb namespace. Mentions [[Hub]] in passing.\n",
			"pages/Hub.md":        "- hub\n",
		}, 60, 24)
		a.navigate("kb/notes")
		return a
	}
	a := mk()
	_, y := cellOf(t, a, "Hub")
	lines := frameLines(a)
	if strings.Contains(lines[y], "•") || strings.TrimSpace(lines[y-1]) == "" {
		t.Fatalf("fixture changed: Hub is not on a continuation row:\n%s", appText(a))
	}
	twinLinkClick(t, mk, "Hub", 0)
}

func TestMouseRowClick(t *testing.T) {
	a := longApp(t, 80, 24)
	off, cur := a.page.Offset(), a.page.Cursor()
	a.Update(click(10, 5))
	if a.page.CursorRow() != off+5 || a.page.Offset() != off || a.page.Cursor() != cur {
		t.Errorf("row click: row %d offset %d cursor %d, want %d %d %d", a.page.CursorRow(), a.page.Offset(), a.page.Cursor(), off+5, off, cur)
	}

	files := map[string]string{"pages/Gaps.md": "first para\n\nsecond para\n\nthird\n"}
	g := newApp(t, files, 80, 24)
	g.navigate("Gaps")
	twin := newApp(t, files, 80, 24)
	twin.navigate("Gaps")
	blank := -1
	for r := range g.page.rowCount() {
		if g.page.rowBlank(r) {
			blank = r
			break
		}
	}
	if blank < 0 {
		t.Fatal("fixture has no blank row")
	}
	twin.page.Reposition(twin.page.Offset(), blank)
	g.Update(click(10, blank))
	if g.page.CursorRow() != twin.page.CursorRow() || g.page.CursorRow() == blank {
		t.Errorf("blank row click: row %d, want %d (not %d)", g.page.CursorRow(), twin.page.CursorRow(), blank)
	}

	before := readStateOf(g)
	g.Update(click(10, 20))
	if got := readStateOf(g); got != before {
		t.Errorf("click past the last row changed the view: %+v -> %+v", before, got)
	}

	e := newApp(t, map[string]string{"pages/Empty.md": ""}, 80, 24)
	e.navigate("Empty")
	before = readStateOf(e)
	e.Update(click(10, 3))
	if got := readStateOf(e); got != before {
		t.Errorf("click on an empty page changed the view: %+v -> %+v", before, got)
	}

	s := sidebarApp(t, 120, 30)
	s.navigate("Alpha")
	s.Update(key("1"))
	row := -1
	for r := range s.page.rowCount() {
		if r != s.page.CursorRow() && !s.page.rowBlank(r) {
			row = r
			break
		}
	}
	s.Update(click(sidebarCols+10, row-s.page.Offset()))
	if s.page.CursorRow() != row || s.sidebarFocused() {
		t.Errorf("sidebar shown: row %d focused %v, want row %d unfocused", s.page.CursorRow(), s.sidebarFocused(), row)
	}
}

func TestMouseStatusAndRuleClicksIgnored(t *testing.T) {
	for _, shown := range []bool{false, true} {
		a := sidebarApp(t, 120, 30)
		a.navigate("Alpha")
		if shown {
			a.Update(key("1"))
			a.Update(esc)
		}
		before := readStateOf(a)
		for _, y := range []int{28, 29} {
			for _, x := range []int{5, 60} {
				a.Update(click(x, y))
				if got := readStateOf(a); got != before {
					t.Errorf("sidebar=%v click (%d,%d) changed the view: %+v -> %+v", shown, x, y, before, got)
				}
			}
		}
	}
}

func TestMouseSidebarFocus(t *testing.T) {
	a := sidebarApp(t, 120, 30)
	a.Update(key("1"))
	a.Update(esc)
	if a.sidebarFocused() {
		t.Fatal("setup: sidebar still focused after esc")
	}
	sel := a.side.tree.sel
	a.Update(click(5, 0))
	if !a.sidebarFocused() || a.side.tree.sel != sel {
		t.Errorf("header click: focused %v sel %d, want true and %d", a.sidebarFocused(), a.side.tree.sel, sel)
	}
	a.Update(click(70, 3))
	if a.sidebarFocused() {
		t.Error("page click left the sidebar focused")
	}
	a.Update(click(50, 3))
	if !a.sidebarFocused() || a.side.tree.sel != sel {
		t.Errorf("gutter click: focused %v sel %d, want true and %d", a.sidebarFocused(), a.side.tree.sel, sel)
	}
}

func TestMouseSidebarWheel(t *testing.T) {
	a := bootApp(t, bootConfig{files: map[string]string{
		"pages/Long.md": escPage(), "pages/Other.md": "- o\n", "pages/Third.md": "- t\n",
	}})
	a.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	a.navigate("Long")
	a.Update(key("1"))
	a.Update(esc)
	sel, off := a.side.tree.sel, a.page.Offset()
	a.Update(wheel(5, 5, +1))
	if a.side.tree.sel != sel+1 || a.sidebarFocused() || a.page.Offset() != off {
		t.Errorf("sidebar wheel: sel %d focused %v offset %d, want %d false %d", a.side.tree.sel, a.sidebarFocused(), a.page.Offset(), sel+1, off)
	}
	sel = a.side.tree.sel
	a.Update(wheel(70, 5, +1))
	if a.side.tree.sel != sel || a.page.Offset() != off+3 {
		t.Errorf("page wheel: sel %d offset %d, want %d %d", a.side.tree.sel, a.page.Offset(), sel, off+3)
	}
}

func TestMouseIgnoredInputs(t *testing.T) {
	for _, shown := range []bool{false, true} {
		a := sidebarApp(t, 120, 30)
		a.navigate("Alpha")
		if shown {
			a.Update(key("1"))
		}
		before := readStateOf(a)
		for name, msg := range map[string]tea.Msg{
			"right click":      tea.MouseClickMsg{X: 70, Y: 5, Button: tea.MouseRight},
			"middle click":     tea.MouseClickMsg{X: 70, Y: 5, Button: tea.MouseMiddle},
			"wheel left":       tea.MouseWheelMsg{X: 70, Y: 5, Button: tea.MouseWheelLeft},
			"wheel right":      tea.MouseWheelMsg{X: 70, Y: 5, Button: tea.MouseWheelRight},
			"negative x click": click(-1, 5),
			"negative y click": click(70, -1),
			"negative x wheel": wheel(-1, 5, +1),
			"negative y wheel": wheel(70, -1, +1),
		} {
			a.Update(msg)
			if got := readStateOf(a); got != before {
				t.Errorf("sidebar=%v %s changed the view: %+v -> %+v", shown, name, before, got)
			}
		}
	}
}

func TestPageAt(t *testing.T) {
	a := newApp(t, map[string]string{
		"pages/P.md":    "- Parent has [[Beta]] link\n  - child with [[Gamma]]\n- tail\n",
		"pages/Beta.md": "- b\n", "pages/Gamma.md": "- g\n",
	}, 80, 24)
	a.navigate("P")
	p := a.page
	bx, by := cellOf(t, a, "Beta")
	gx, gy := cellOf(t, a, "Gamma")

	if _, l, ok := p.At(bx, by); !ok || l != 0 {
		t.Errorf("At(Beta first cell) link %d ok %v, want 0 true", l, ok)
	}
	if _, l, ok := p.At(bx+len("Beta")-1, by); !ok || l != 0 {
		t.Errorf("At(Beta last cell) link %d ok %v, want 0 true", l, ok)
	}
	if _, l, ok := p.At(bx+len("Beta"), by); !ok || l != -1 {
		t.Errorf("At(past Beta) link %d ok %v, want -1 true", l, ok)
	}
	if _, l, _ := p.At(gx, gy); l != 1 {
		t.Errorf("At(Gamma) link %d, want 1", l)
	}
	for _, c := range [][2]int{{-1, by}, {bx, -1}, {bx, 22}, {bx, p.rowCount()}} {
		if _, _, ok := p.At(c[0], c[1]); ok {
			t.Errorf("At(%d,%d) ok, want not", c[0], c[1])
		}
	}

	// Fold the parent: its child's link is hidden and the marker covers
	// the columns past the text kept.
	p.Reposition(0, by)
	a.Update(key("z"))
	if !p.linkHidden(1) {
		t.Fatalf("setup: child link not hidden after folding:\n%s", appText(a))
	}
	if _, l, ok := p.At(gx, by+1); ok && l == 1 {
		t.Error("hidden link hit")
	}
	row, l, ok := p.At(79, by)
	if !ok || l != -1 || row != by {
		t.Errorf("At(marker column) = %d %d %v, want row %d, no link", row, l, ok, by)
	}
}
