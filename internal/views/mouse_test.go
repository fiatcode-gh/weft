package views

import (
	"fmt"
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

// With the sidebar shown the page starts at column sidebarCols: a click on a
// link's first or last cell follows it as n+enter does, and the cell just
// left of it does not.
func TestMouseLinkClickBesideSidebar(t *testing.T) {
	mk := func() *App {
		a := newApp(t, map[string]string{
			"pages/Alpha.md": "- Links to [[Beta]] here\n- x\n",
			"pages/Beta.md":  "- b\n",
		}, 130, 24)
		a.navigate("Alpha")
		a.Update(key("3"))
		a.Update(keyEscK)
		if !a.side.shown || a.sidebarFocused() {
			t.Fatalf("sidebar shown %v focused %v, want shown, page focused", a.side.shown, a.sidebarFocused())
		}
		return a
	}
	// pageCell finds text in the page column only (the sidebar may list it too).
	pageCell := func(a *App, text string) (x, y int) {
		for y, line := range frameLines(a) {
			rest := ansi.TruncateLeft(line, sidebarCols, "")
			if i := strings.Index(rest, text); i >= 0 {
				return sidebarCols + ansi.StringWidth(rest[:i]), y
			}
		}
		t.Fatalf("%q not in the page column:\n%s", text, appText(a))
		return 0, 0
	}
	keys := mk()
	keys.Update(key("n"))
	keys.Update(keyEnterK)
	want := readStateOf(keys)
	if want.page != "Beta" {
		t.Fatalf("n+enter landed on %q, want Beta", want.page)
	}
	x0, y := pageCell(mk(), "Beta")
	for name, x := range map[string]int{"first cell": x0, "last cell": x0 + len("Beta") - 1} {
		a := mk()
		a.Update(click(x, y))
		if got := readStateOf(a); got != want {
			t.Errorf("click on the %s (%d,%d) != n+enter:\n got %+v\nwant %+v", name, x, y, got, want)
		}
	}
	a := mk()
	a.Update(click(x0-1, y))
	if a.page.Page() != "Alpha" {
		t.Errorf("click left of the link followed it to %q", a.page.Page())
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

// mouseState is everything a mouse event may change, in any context: the
// frame, the page and list state, the open overlay, the editor's cursor and
// window and the calendars' cursors.
func mouseState(a *App) string {
	s := fmt.Sprintf("%s\nframe %q\nactive %p", listStateOf(a), appText(a), a.active)
	if a.editor != nil {
		s += fmt.Sprintf("\neditor cursor %v top %v", a.editor.buf.Cursor(), a.editor.top)
	}
	if c, ok := a.active.(*Calendar); ok {
		s += fmt.Sprintf("\noverlay calendar %v", c.cursor)
	}
	if a.side.calendar != nil {
		s += fmt.Sprintf("\nside calendar %v", a.side.calendar.cursor)
	}
	return s
}

// mouseContext builds an app and the mouse events that act in it while the
// mouse is on. With off set, `m` is pressed in the read view first.
type mouseContext struct {
	name  string
	build func(t *testing.T, off bool) (*App, []tea.Msg)
}

func withM(off bool, keys ...tea.Msg) []tea.Msg {
	if off {
		return append([]tea.Msg{key("m")}, keys...)
	}
	return keys
}

func feed(a *App, msgs ...tea.Msg) {
	for _, m := range msgs {
		a.Update(m)
	}
}

var mouseFiles = map[string]string{
	"pages/Long.md":  escPage(),
	"pages/Linky.md": "- see [[Beta]] now\n- plain row\n",
	"pages/Other.md": "- o\n",
	"pages/Third.md": "- t\n",
}

func mouseContexts() []mouseContext {
	return []mouseContext{
		{"page link", func(t *testing.T, off bool) (*App, []tea.Msg) {
			a := newApp(t, mouseFiles, 80, 24)
			a.navigate("Linky")
			feed(a, withM(off)...)
			x, y := cellOf(t, a, "Beta")
			return a, []tea.Msg{click(x, y)}
		}},
		{"page row and wheel", func(t *testing.T, off bool) (*App, []tea.Msg) {
			a := newApp(t, mouseFiles, 80, 24)
			a.navigate("Long")
			feed(a, withM(off)...)
			x, y := cellOf(t, a, "L7")
			return a, []tea.Msg{click(x, y), wheel(10, 5, +1)}
		}},
		{"sidebar", func(t *testing.T, off bool) (*App, []tea.Msg) {
			a := bootApp(t, bootConfig{files: mouseFiles})
			a.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
			a.navigate("Long")
			feed(a, withM(off, key("1"), esc)...)
			return a, []tea.Msg{click(5, 0), click(5, 3), wheel(5, 5, +1)}
		}},
		{"todos overlay", func(t *testing.T, off bool) (*App, []tea.Msg) {
			a := bootAt(t, 24, bootConfig{now: paletteNow}, withM(off, key("T"))...)
			p := a.active.(*Todos)
			rx, ry := mustCell(t, a, p.visible[1].Text, 0)
			tx, ty := mustCell(t, a, "Open todos", 0)
			return a, []tea.Msg{click(rx, ry), wheel(tx, ty, +1)}
		}},
		{"calendar day", func(t *testing.T, off bool) (*App, []tea.Msg) {
			a := bootAt(t, 24, bootConfig{now: paletteNow}, withM(off, key("C"))...)
			x, y := calDayCell(t, a, a.active.(*Calendar), 12, 0)
			return a, []tea.Msg{click(x, y)}
		}},
		{"editor", func(t *testing.T, off bool) (*App, []tea.Msg) {
			a := newApp(t, mouseFiles, 80, 24)
			a.navigate("Long")
			feed(a, withM(off, key("e"))...)
			if a.editor == nil {
				t.Fatal("editor did not open")
			}
			x, y := cellOf(t, a, "L7")
			return a, []tea.Msg{click(x, y), wheel(10, 5, +1)}
		}},
	}
}

func TestMouseIgnoredWhileOff(t *testing.T) {
	for _, c := range mouseContexts() {
		t.Run(c.name, func(t *testing.T) {
			a, events := c.build(t, true)
			if !a.mouseOff {
				t.Fatal("setup: mouse still on after m")
			}
			want := mouseState(a)
			for i, ev := range events {
				a.Update(ev)
				if got := mouseState(a); got != want {
					t.Fatalf("event %d (%+v) changed the state while off:\n got %s\nwant %s", i, ev, got, want)
				}
			}

			// Each event must act when the mouse is on, or the check above proves nothing.
			for i := range events {
				a, events := c.build(t, false)
				before := mouseState(a)
				a.Update(events[i])
				if mouseState(a) == before {
					t.Errorf("event %d (%+v) does nothing while on", i, events[i])
				}
			}
		})
	}
}

// settle ends an editor a battery left open: the buffer is unchanged, so esc
// closes it without a prompt.
func settle(t *testing.T, a *App) {
	t.Helper()
	if a.editor == nil {
		return
	}
	if a.editor.dirty() {
		t.Fatal("the mouse made the buffer dirty")
	}
	a.Update(esc)
	if a.editor != nil {
		t.Fatal("esc did not close the unchanged editor")
	}
}

func TestMouseWritesNothing(t *testing.T) {
	// run drives one battery and compares the graph's files before and after.
	run := func(t *testing.T, a *App, battery func()) {
		t.Helper()
		before := graphFiles(t, a.graphPath)
		battery()
		settle(t, a)
		after := graphFiles(t, a.graphPath)
		if fmt.Sprint(before) != fmt.Sprint(after) {
			t.Errorf("the graph changed:\n before %d files\n after %d files", len(before), len(after))
			for k, v := range after {
				if old, ok := before[k]; !ok || old != v {
					t.Errorf("  %s written or created", k)
				}
			}
			for k := range before {
				if _, ok := after[k]; !ok {
					t.Errorf("  %s removed", k)
				}
			}
		}
	}
	clickCell := func(t *testing.T, a *App, text string, maxX int) {
		t.Helper()
		x, y := mustCell(t, a, text, maxX)
		a.Update(click(x, y))
	}

	t.Run("page", func(t *testing.T) {
		a := newApp(t, mouseFiles, 80, 24)
		a.navigate("Linky")
		run(t, a, func() {
			clickCell(t, a, "Beta", 0)
			a.navigate("Linky")
			clickCell(t, a, "plain", 0)
			a.navigate("Long")
			a.Update(wheel(10, 5, +1))
			a.Update(wheel(10, 5, -1))
		})
	})

	for _, c := range listCases() {
		if c.section == 0 {
			continue
		}
		t.Run("sidebar "+c.name, func(t *testing.T) {
			a := bootApp(t, bootConfig{files: c.files, now: paletteNow})
			resize(a, 120, 30)
			a.navigate(c.page)
			a.Update(key(fmt.Sprint(c.section)))
			run(t, a, func() {
				item := c.pickItem(a.side.panel())
				if item < 0 {
					t.Fatal("setup: no row to click")
				}
				text := c.text(a.side.panel(), item)
				clickCell(t, a, text, sidebarCols) // select
				clickCell(t, a, text, sidebarCols) // open
			})
		})
	}

	t.Run("sidebar calendar", func(t *testing.T) {
		a := sidebarApp(t, 120, 30)
		a.Update(key("5"))
		cal := a.side.calendar
		run(t, a, func() {
			x, y := calDayCell(t, a, cal, 12, sidebarCols) // a day without a journal
			a.Update(click(x, y))
			a.Update(click(x, y))
		})
	})

	for _, c := range listCases() {
		switch c.name {
		case "picker", "backlinks", "tree", "onthisday", "palette":
		default:
			continue
		}
		t.Run("overlay "+c.name, func(t *testing.T) {
			a := c.overlay(t, 24)
			run(t, a, func() {
				item := c.pickItem(a.active)
				if item < 0 {
					t.Fatal("setup: no row to click")
				}
				text := c.text(a.active, item)
				clickCell(t, a, text, 0) // select
				if c.name != "palette" {
					clickCell(t, a, text, 0) // open (a palette row would run an action)
				}
			})
		})
	}

	t.Run("picker create row", func(t *testing.T) {
		a := bootAt(t, 24, bootConfig{}, tea.KeyPressMsg{Code: 'p', Mod: tea.ModCtrl})
		typeText(a, "Alp")
		run(t, a, func() {
			clickCell(t, a, `＋ Create "Alp"`, 0)
			clickCell(t, a, `＋ Create "Alp"`, 0)
			if a.editor == nil {
				t.Fatal("the create row did not open the editor")
			}
		})
	})

	t.Run("editor", func(t *testing.T) {
		a := newApp(t, mouseFiles, 80, 24)
		a.navigate("Long")
		a.Update(key("e"))
		if a.editor == nil {
			t.Fatal("editor did not open")
		}
		run(t, a, func() {
			clickCell(t, a, "L7", 0)
			a.Update(wheel(10, 5, +1))
			clickCell(t, a, "L9", 0)
			a.Update(wheel(10, 5, -1))
		})
	})
}

func TestMouseOutOfRange(t *testing.T) {
	contexts := []mouseContext{
		{"read view", func(t *testing.T, _ bool) (*App, []tea.Msg) {
			a := newApp(t, mouseFiles, 80, 24)
			a.navigate("Long")
			return a, nil
		}},
		{"sidebar", func(t *testing.T, _ bool) (*App, []tea.Msg) {
			a := sidebarApp(t, 120, 30)
			a.Update(key("1"))
			return a, nil
		}},
		{"overlay", func(t *testing.T, _ bool) (*App, []tea.Msg) {
			return bootAt(t, 24, bootConfig{}, tea.KeyPressMsg{Code: 'p', Mod: tea.ModCtrl}), nil
		}},
		{"editor", func(t *testing.T, _ bool) (*App, []tea.Msg) {
			a := newApp(t, mouseFiles, 80, 24)
			a.navigate("Long")
			a.Update(key("e"))
			return a, nil
		}},
	}
	for _, c := range contexts {
		t.Run(c.name, func(t *testing.T) {
			for _, xy := range [][2]int{{-1, -1}, {0, -5}, {10000, 10000}} {
				for _, kind := range []string{"click", "wheel up", "wheel down"} {
					a, _ := c.build(t, false)
					before := mouseState(a)
					var ev tea.Msg
					switch kind {
					case "click":
						ev = click(xy[0], xy[1])
					case "wheel up":
						ev = wheel(xy[0], xy[1], -1)
					default:
						ev = wheel(xy[0], xy[1], +1)
					}
					a.Update(ev)
					// The wheel acts wherever the pointer is on screen, so only
					// off-screen-negative cells and clicks must change nothing.
					if (kind == "click" || xy[0] < 0 || xy[1] < 0) && mouseState(a) != before {
						t.Errorf("%s at %v changed the state", kind, xy)
					}
					a.View()
				}
			}
		})
	}
}
