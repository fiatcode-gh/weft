package views

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/fiatcode-gh/weft/v3/internal/search"
)

// topicFiles is a graph where Topic has three linked references and two bare
// mentions, so Backlinks shows its "Unlinked references" header.
var topicFiles = map[string]string{
	"pages/Topic.md": "- topic\n",
	"pages/Aa.md":    "- see [[Topic]] first\n- and [[Topic]] again\n",
	"pages/Bb.md":    "- also [[Topic]]\n",
	"pages/Cc.md":    "- a bare Topic mention\n- another Topic one\n",
}

var (
	listDown = tea.KeyPressMsg{Code: tea.KeyDown}
	listUp   = tea.KeyPressMsg{Code: tea.KeyUp}
)

// panelSel is the selected item of a list panel.
func panelSel(p Overlay) int {
	switch p := p.(type) {
	case *Picker:
		return p.sel
	case *SearchView:
		return p.sel
	case *Palette:
		return p.sel
	case *Backlinks:
		return p.sel
	case *Todos:
		return p.sel
	case *Agenda:
		return p.sel
	case *Tree:
		return p.sel
	case *OnThisDay:
		return p.sel
	}
	return -1
}

// listState is everything a list click may change, wherever the panel lives.
type listState struct {
	readState
	kind    string
	sel     int
	sideSel int
	hint    string
	editor  bool
	prompts [2]bool
}

func (s listState) String() string {
	return fmt.Sprintf("{page %s row %d off %d cursor %d hist %d focused %v | %s sel %d side %d hint %q editor %v prompts %v | frame %d bytes}",
		s.page, s.row, s.off, s.cursor, s.hist, s.focused, s.kind, s.sel, s.sideSel, s.hint, s.editor, s.prompts, len(s.text))
}

// noFrame is the state without the rendered frame.
func (s listState) noFrame() listState {
	s.text = ""
	return s
}

func listStateOf(a *App) listState {
	s := listState{readState: readStateOf(a), kind: activeKind(a), sel: -1, sideSel: -1, hint: a.hint, editor: a.editor != nil}
	s.prompts = [2]bool{a.capture != nil, a.rename != nil}
	if a.active != nil {
		s.sel = panelSel(a.active)
	}
	if a.side.shown {
		if p := a.side.panel(); p != nil {
			s.sideSel = panelSel(p)
		}
	}
	return s
}

// findCell is the screen cell of the first frame line whose columns before
// maxX contain text (maxX <= 0: the whole line).
func findCell(a *App, text string, maxX int) (x, y int, ok bool) {
	for y, line := range frameLines(a) {
		if maxX > 0 {
			line = ansi.Truncate(line, maxX, "")
		}
		if i := strings.Index(line, text); i >= 0 {
			return ansi.StringWidth(line[:i]), y, true
		}
	}
	return 0, 0, false
}

func mustCell(t *testing.T, a *App, text string, maxX int) (int, int) {
	t.Helper()
	x, y, ok := findCell(a, text, maxX)
	if !ok {
		t.Fatalf("%q not on screen:\n%s", text, appText(a))
	}
	return x, y
}

// headerCell finds the line whose box content starts with prefix.
func headerCell(a *App, prefix string, maxX int) (x, y int, ok bool) {
	for y, line := range frameLines(a) {
		if maxX > 0 {
			line = ansi.Truncate(line, maxX, "")
		}
		if !strings.HasPrefix(strings.TrimSpace(line), "│") {
			continue
		}
		inner := strings.TrimSpace(strings.Trim(strings.TrimSpace(line), "│"))
		if strings.HasPrefix(inner, prefix) {
			return strings.Index(line, prefix), y, true
		}
	}
	return 0, 0, false
}

// dividerCell finds the panel's divider rule (a box line, not the border).
func dividerCell(t *testing.T, a *App, maxX int) (int, int) {
	t.Helper()
	for y, line := range frameLines(a) {
		if maxX > 0 {
			line = ansi.Truncate(line, maxX, "")
		}
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, "│") && strings.Contains(trim, "─────") {
			return ansi.StringWidth(line[:strings.Index(line, "─")]), y
		}
	}
	t.Fatalf("no divider on screen:\n%s", appText(a))
	return 0, 0
}

// listCase describes one list panel for the click tests.
type listCase struct {
	name  string
	title string
	// overlay opens the panel as an overlay of the given terminal height.
	overlay func(t *testing.T, h int) *App
	// section and page open it in the sidebar (0 = not a sidebar panel).
	section int
	page    string
	files   map[string]string
	count   func(p Overlay) int
	// text is a string unique to item i's row ("" for a header).
	text func(p Overlay, i int) string
	// pick is the item the tests click first.
	pick func(p Overlay) int
	// headers are prefixes of the header lines the panel draws.
	headers func(p Overlay) []string
	// more says the panel overflows at height 16 in both directions.
	more bool
}

func shortText(s string) string {
	r := []rune(s)
	if len(r) > 9 { // sidebar rows clamp the text early
		r = r[:9]
	}
	return string(r)
}

func nextSelectable(p Overlay, count func(Overlay) int, skip func(Overlay, int) bool) int {
	for i := panelSel(p) + 1; i < count(p); i++ {
		if !skip(p, i) {
			return i
		}
	}
	return -1
}

func bootAt(t *testing.T, h int, cfg bootConfig, keys ...tea.Msg) *App {
	t.Helper()
	a := bootApp(t, cfg)
	a.Update(tea.WindowSizeMsg{Width: 80, Height: h})
	for _, k := range keys {
		a.Update(k)
	}
	return a
}

func listCases() []listCase {
	todoPages := func(ts []string) []string {
		seen := map[string]bool{}
		var out []string
		for _, p := range ts {
			if !seen[p] {
				seen[p] = true
				out = append(out, p)
			}
		}
		return out
	}
	return []listCase{
		{
			name: "picker", title: "Find a page", more: true,
			overlay: func(t *testing.T, h int) *App {
				return bootAt(t, h, bootConfig{}, tea.KeyPressMsg{Code: 'p', Mod: tea.ModCtrl})
			},
			count: func(p Overlay) int { return len(p.(*Picker).matches) },
			text:  func(p Overlay, i int) string { return p.(*Picker).matches[i].Str },
		},
		{
			name: "search", title: "Search the graph", more: true,
			overlay: func(t *testing.T, h int) *App {
				a := bootAt(t, h, bootConfig{}, key("/"))
				s := a.active.(*SearchView)
				typeText(a, "hit")
				var hits []search.Hit
				pages := []string{"Alpha", "Beta", "Hub", "Orphan"}
				for i := 0; i < 20; i++ {
					hits = append(hits, search.Hit{
						FilePath: filepath.Join(a.graphPath, "pages", pages[i%len(pages)]+".md"),
						Line:     i + 1, Context: fmt.Sprintf("hit-%02d", i+1),
					})
				}
				a.Update(searchDoneMsg{view: s, gen: s.gen, hits: hits})
				return a
			},
			count: func(p Overlay) int { return len(p.(*SearchView).hits) },
			text:  func(p Overlay, i int) string { return p.(*SearchView).hits[i].Context },
		},
		{
			name: "palette", title: "Commands", more: true,
			overlay: func(t *testing.T, h int) *App { return bootAt(t, h, bootConfig{}, key(":")) },
			count:   func(p Overlay) int { return len(p.(*Palette).matches) },
			text: func(p Overlay, i int) string {
				pl := p.(*Palette)
				return paletteActions[pl.matches[i].Index].title
			},
		},
		{
			name: "backlinks", title: "Backlinks", section: 2, page: "Topic", files: topicFiles,
			overlay: func(t *testing.T, h int) *App {
				if _, err := exec.LookPath("rg"); err != nil {
					t.Skip("rg not on PATH")
				}
				a := bootAt(t, h, bootConfig{files: topicFiles})
				a.navigate("Topic")
				a.Update(key("b"))
				return a
			},
			count: func(p Overlay) int { return len(p.(*Backlinks).rows) },
			text: func(p Overlay, i int) string {
				r := p.(*Backlinks).rows[i]
				switch {
				case r.ref != nil:
					return fmt.Sprintf("%s:%d", r.ref.FromPage, r.ref.LineNumber)
				case r.unl != nil:
					return fmt.Sprintf("%s:%d", r.unl.PageName, r.unl.Line)
				}
				return ""
			},
			pick: func(p Overlay) int {
				return nextSelectable(p, func(p Overlay) int { return len(p.(*Backlinks).rows) },
					func(p Overlay, i int) bool { return p.(*Backlinks).rows[i].header })
			},
			headers: func(p Overlay) []string { return []string{"── Unlinked references"} },
		},
		{
			name: "todos", title: "Open todos", section: 3, page: "Hub",
			overlay: func(t *testing.T, h int) *App { return bootAt(t, h, bootConfig{now: paletteNow}, key("T")) },
			count:   func(p Overlay) int { return len(p.(*Todos).visible) },
			text:    func(p Overlay, i int) string { return shortText(p.(*Todos).visible[i].Text) },
			headers: func(p Overlay) []string {
				var pages []string
				for _, b := range p.(*Todos).visible {
					pages = append(pages, b.Page)
				}
				return todoPages(pages)
			},
		},
		{
			name: "agenda", title: "Agenda", section: 4, page: "Hub",
			overlay: func(t *testing.T, h int) *App { return bootAt(t, h, bootConfig{now: paletteNow}, key("A")) },
			count:   func(p Overlay) int { return len(p.(*Agenda).items) },
			text:    func(p Overlay, i int) string { return shortText(p.(*Agenda).items[i].Todo.Text) },
			headers: func(p Overlay) []string {
				var sections []string
				for _, it := range p.(*Agenda).items {
					sections = append(sections, it.Section.String())
				}
				return todoPages(sections)
			},
		},
		{
			name: "tree", title: "Page tree", section: 1, page: "Hub",
			overlay: func(t *testing.T, h int) *App { return bootAt(t, h, bootConfig{}, key("F")) },
			count:   func(p Overlay) int { return len(p.(*Tree).rows) },
			text:    func(p Overlay, i int) string { return p.(*Tree).rows[i].node.Label },
			pick: func(p Overlay) int {
				return nextSelectable(p, func(p Overlay) int { return len(p.(*Tree).rows) },
					func(p Overlay, i int) bool { return len(p.(*Tree).rows[i].node.Children) > 0 })
			},
		},
		{
			name: "onthisday", title: "On this day",
			overlay: func(t *testing.T, h int) *App {
				a := bootApp(t, bootConfig{files: otdFiles, now: calNow})
				a.Update(tea.WindowSizeMsg{Width: 80, Height: h})
				a.Update(key("O"))
				return a
			},
			count: func(p Overlay) int { return len(p.(*OnThisDay).entries) },
			text: func(p Overlay, i int) string {
				e := p.(*OnThisDay).entries[i]
				return e.Label + " · " + e.Journal.Date.Format("2006-01-02 Mon")
			},
		},
	}
}

func (c listCase) pickItem(p Overlay) int {
	if c.pick != nil {
		return c.pick(p)
	}
	if i := panelSel(p) + 1; i < c.count(p) {
		return i
	}
	return -1
}

// itemWithText is the index of the item of p whose row text is text. The
// picker orders pages by modification time, which differs between two clones
// of the fixture, so a twin finds the clicked page by name, not by index.
func (c listCase) itemWithText(p Overlay, text string) int {
	for i := range c.count(p) {
		if c.text(p, i) == text {
			return i
		}
	}
	return -1
}

// stepTo presses down/up on a twin until item is selected.
func stepTo(t *testing.T, a *App, p func() Overlay, item int) {
	t.Helper()
	for range 60 {
		cur := panelSel(p())
		switch {
		case cur == item:
			return
		case cur < item:
			a.Update(listDown)
		default:
			a.Update(listUp)
		}
	}
	t.Fatalf("keys never reached item %d (at %d)", item, panelSel(p()))
}

func TestMouseListOverlays(t *testing.T) {
	for _, c := range listCases() {
		t.Run(c.name+"/select then open", func(t *testing.T) {
			a := c.overlay(t, 24)
			p := a.active
			item := c.pickItem(p)
			if item < 0 {
				t.Fatalf("setup: no row after the selection (%d items)", c.count(p))
			}
			text := c.text(p, item)
			x, y := mustCell(t, a, text, 0)
			a.Update(click(x, y))
			if a.active != p || panelSel(p) != item {
				t.Fatalf("first click: active same %v, sel %d, want item %d", a.active == p, panelSel(p), item)
			}
			x, y = mustCell(t, a, text, 0)
			a.Update(click(x, y))

			twin := c.overlay(t, 24)
			stepTo(t, twin, func() Overlay { return twin.active }, c.itemWithText(twin.active, text))
			twin.Update(keyEnt)
			if got, want := listStateOf(a), listStateOf(twin); got != want {
				t.Errorf("second click != enter:\n got %+v\nwant %+v", got, want)
			}
		})

		// Two clones of the fixture can order the picker differently (pages
		// tie on modification time), so the wheel twin compares state, not frame.
		t.Run(c.name+"/wheel", func(t *testing.T) {
			a := c.overlay(t, 24)
			twin := c.overlay(t, 24)
			x, y := mustCell(t, a, c.title, 0)
			a.Update(wheel(x, y, +1))
			twin.Update(listDown)
			if got, want := listStateOf(a).noFrame(), listStateOf(twin).noFrame(); got != want || got.sel == 0 {
				t.Errorf("wheel down != down:\n got %+v\nwant %+v", got, want)
			}
			a.Update(wheel(x, y, -1))
			twin.Update(listUp)
			if got, want := listStateOf(a).noFrame(), listStateOf(twin).noFrame(); got != want {
				t.Errorf("wheel up != up:\n got %+v\nwant %+v", got, want)
			}
		})

		t.Run(c.name+"/chrome", func(t *testing.T) {
			a := c.overlay(t, 16)
			if c.more {
				for range 9 {
					a.Update(listDown)
				}
			}
			tx, ty := mustCell(t, a, c.title, 0)
			fx, fy := mustCell(t, a, "↑/↓", 0)
			dx, dy := dividerCell(t, a, 0)
			cells := map[string][2]int{"title": {tx, ty}, "blank": {tx, ty + 1}, "footer": {fx, fy}, "divider": {dx, dy}}
			for _, more := range []string{"more above", "more below"} {
				x, y, ok := findCell(a, more, 0)
				if c.more && !ok {
					t.Fatalf("no %q line at height 16:\n%s", more, appText(a))
				}
				if ok {
					cells[more] = [2]int{x, y}
				}
			}
			if c.headers != nil {
				for _, h := range c.headers(a.active) {
					if x, y, ok := headerCell(a, h, 0); ok {
						cells["header "+h] = [2]int{x, y}
					}
				}
			}
			for name, xy := range cells {
				before := listStateOf(a)
				a.Update(click(xy[0], xy[1]))
				if got := listStateOf(a); got != before {
					t.Errorf("click on %s (%d,%d) changed the panel:\n before %+v\n after  %+v", name, xy[0], xy[1], before, got)
				}
			}
		})
	}
}

func TestMouseListHeadersExist(t *testing.T) {
	for _, name := range []string{"todos", "agenda", "backlinks"} {
		for _, c := range listCases() {
			if c.name != name {
				continue
			}
			a := c.overlay(t, 24)
			found := 0
			for _, h := range c.headers(a.active) {
				if _, _, ok := headerCell(a, h, 0); ok {
					found++
				}
			}
			if found == 0 {
				t.Errorf("%s: no header line on screen, so the header click test proves nothing:\n%s", name, appText(a))
			}
		}
	}
}

func TestMousePickerCreateRow(t *testing.T) {
	open := func(t *testing.T) *App {
		a := bootAt(t, 24, bootConfig{}, tea.KeyPressMsg{Code: 'p', Mod: tea.ModCtrl})
		typeText(a, "Alp")
		p := a.active.(*Picker)
		if p.createName != "Alp" || len(p.matches) == 0 {
			t.Fatalf("setup: createName %q, %d matches", p.createName, len(p.matches))
		}
		return a
	}
	for _, syncing := range []bool{false, true} {
		a, twin := open(t), open(t)
		a.syncing, twin.syncing = syncing, syncing
		p := a.active.(*Picker)
		create := len(p.matches)
		x, y := mustCell(t, a, `＋ Create "Alp"`, 0)
		a.Update(click(x, y))
		if p.sel != create || a.active != p {
			t.Fatalf("syncing=%v: first click: sel %d, want create row %d", syncing, p.sel, create)
		}
		x, y = mustCell(t, a, `＋ Create "Alp"`, 0)
		a.Update(click(x, y))

		stepTo(t, twin, func() Overlay { return twin.active }, create)
		twin.Update(keyEnt)
		if got, want := listStateOf(a), listStateOf(twin); got != want {
			t.Errorf("syncing=%v: second click != enter:\n got %+v\nwant %+v", syncing, got, want)
		}
		if opened := a.editor != nil; opened == syncing {
			t.Errorf("syncing=%v: editor open %v", syncing, opened)
		}
		if syncing && !strings.Contains(appText(a), syncBusyMsg) {
			t.Errorf("syncing: the picker shows no busy message:\n%s", appText(a))
		}
	}
}

func TestMouseSidebarListPanels(t *testing.T) {
	for _, c := range listCases() {
		if c.section == 0 {
			continue
		}
		open := func(t *testing.T, focused bool) *App {
			a := bootApp(t, bootConfig{files: c.files, now: paletteNow})
			resize(a, 120, 30)
			a.navigate(c.page)
			a.Update(key(fmt.Sprint(c.section)))
			if !a.side.shown || !a.sidebarFocused() {
				t.Fatalf("%s: sidebar not focused after its key", c.name)
			}
			if !focused {
				a.Update(esc)
			}
			return a
		}
		side := func(a *App) Overlay { return a.side.panel() }

		t.Run(c.name+"/select then open", func(t *testing.T) {
			a := open(t, false)
			p := side(a)
			item := c.pickItem(p)
			if item < 0 {
				t.Fatalf("setup: no row after the selection (%d items)", c.count(p))
			}
			text := c.text(p, item)
			x, y := mustCell(t, a, text, sidebarCols)
			a.Update(click(x, y))
			if !a.sidebarFocused() || panelSel(side(a)) != item || a.active != nil {
				t.Fatalf("first click: focused %v sel %d active %v, want focused item %d", a.sidebarFocused(), panelSel(side(a)), a.active, item)
			}
			x, y = mustCell(t, a, text, sidebarCols)
			a.Update(click(x, y))

			twin := open(t, true)
			stepTo(t, twin, func() Overlay { return side(twin) }, c.itemWithText(twin.side.panel(), text))
			twin.Update(keyEnt)
			if got, want := listStateOf(a), listStateOf(twin); got != want {
				t.Errorf("second click != enter:\n got %+v\nwant %+v", got, want)
			}
			if !a.sidebarFocused() {
				t.Error("second click moved the focus out of the sidebar")
			}
		})

		t.Run(c.name+"/wheel", func(t *testing.T) {
			a := open(t, false)
			sel := panelSel(side(a))
			a.Update(wheel(5, 5, +1))
			if got := panelSel(side(a)); got != sel+1 || a.sidebarFocused() {
				t.Errorf("wheel: sel %d focused %v, want %d unfocused", got, a.sidebarFocused(), sel+1)
			}
		})

		t.Run(c.name+"/chrome", func(t *testing.T) {
			a := open(t, false)
			p := side(a)
			tx, ty := mustCell(t, a, c.title, sidebarCols)
			fx, fy := mustCell(t, a, "↑/↓", sidebarCols)
			dx, dy := dividerCell(t, a, sidebarCols)
			cells := map[string][2]int{"title": {tx, ty}, "blank": {tx, ty + 1}, "footer": {fx, fy}, "divider": {dx, dy}, "header": {5, 0}}
			if item := c.pickItem(p); item >= 0 {
				_, y := mustCell(t, a, c.text(p, item), sidebarCols)
				cells["gutter"] = [2]int{sidebarCols - 1, y}
			}
			if c.headers != nil {
				for _, h := range c.headers(p) {
					if x, y, ok := headerCell(a, h, sidebarCols); ok {
						cells["header "+h] = [2]int{x, y}
					}
				}
			}
			want := listStateOf(open(t, true))
			for name, xy := range cells {
				a := open(t, false)
				a.Update(click(xy[0], xy[1]))
				if got := listStateOf(a); got != want {
					t.Errorf("click on %s (%d,%d): want focus only:\n want %+v\n got  %+v", name, xy[0], xy[1], want, got)
					if got.text != want.text {
						t.Logf("frame:\n%s\nwant:\n%s", got.text, want.text)
					}
				}
			}
		})
	}
}

func TestMouseTreeGroupToggle(t *testing.T) {
	group := func(tr *Tree) int {
		for i, r := range tr.rows {
			if len(r.node.Children) > 0 && i != tr.sel {
				return i
			}
		}
		t.Fatal("setup: no group row to click")
		return -1
	}
	for _, where := range []string{"overlay", "sidebar"} {
		t.Run(where, func(t *testing.T) {
			var a *App
			var tr *Tree
			maxX := 0
			if where == "overlay" {
				a = bootAt(t, 24, bootConfig{}, key("F"))
				tr = a.active.(*Tree)
			} else {
				a = sidebarApp(t, 120, 30)
				a.navigate("Hub")
				a.Update(key("1"))
				a.Update(esc)
				tr = a.side.tree
				maxX = sidebarCols
			}
			g := group(tr)
			label := tr.rows[g].node.Label + " ("
			path := tr.rows[g].node.Path
			rows := len(tr.rows)
			clickGroup := func() {
				x, y := mustCell(t, a, label, maxX)
				a.Update(click(x, y))
			}
			clickGroup()
			if tr.sel != g || tr.open[path] {
				t.Fatalf("first click: sel %d open %v, want selected and still closed", tr.sel, tr.open[path])
			}
			clickGroup()
			if !tr.open[path] || len(tr.rows) <= rows {
				t.Fatalf("second click: open %v rows %d (was %d), want expanded", tr.open[path], len(tr.rows), rows)
			}
			clickGroup()
			if tr.open[path] || len(tr.rows) != rows {
				t.Fatalf("third click: open %v rows %d (was %d), want collapsed", tr.open[path], len(tr.rows), rows)
			}
		})
	}
}

func TestMouseHelpAndPreview(t *testing.T) {
	t.Run("help", func(t *testing.T) {
		a := bootAt(t, 40, bootConfig{}, key("?"))
		h := a.active
		before := listStateOf(a)
		for _, msg := range []tea.Msg{click(40, 12), click(5, 5), wheel(40, 12, +1), wheel(40, 12, -1)} {
			a.Update(msg)
			if got := listStateOf(a); got != before || a.active != h {
				t.Fatalf("%T changed help:\n before %+v\n after  %+v", msg, before, got)
			}
		}
	})
	t.Run("rename preview", func(t *testing.T) {
		mk := func() *App {
			a := bootAt(t, 24, bootConfig{})
			renamePreviewed(t, a, "Hub", "Center")
			a.Update(tea.WindowSizeMsg{Width: 80, Height: 13})
			return a
		}
		a, twin := mk(), mk()
		pv := a.active
		before := listStateOf(a)
		x, y := mustCell(t, a, "Rename page", 0)
		a.Update(click(x, y))
		a.Update(click(x, y+3))
		if got := listStateOf(a); got != before || a.active != pv {
			t.Fatalf("click changed the preview:\n before %+v\n after  %+v", before, got)
		}
		a.Update(wheel(x, y, +1))
		twin.Update(listDown)
		if got, want := listStateOf(a), listStateOf(twin); got != want || got == before {
			t.Errorf("wheel down != down (or no scroll):\n got %+v\nwant %+v\nbefore %+v", got, want, before)
		}
		a.Update(wheel(x, y, -1))
		twin.Update(listUp)
		if got, want := listStateOf(a), listStateOf(twin); got != want || got != before {
			t.Errorf("wheel up != up:\n got %+v\nwant %+v", got, want)
		}
	})
}

func TestMousePromptsIgnore(t *testing.T) {
	cases := map[string]func(t *testing.T) *App{
		"capture": func(t *testing.T) *App {
			a := bootAt(t, 24, bootConfig{}, key("c"))
			return a
		},
		"capture over todos": func(t *testing.T) *App {
			return bootAt(t, 24, bootConfig{}, key("T"), key("c"))
		},
		"rename prompt": func(t *testing.T) *App {
			a := bootAt(t, 24, bootConfig{})
			a.navigate("Alpha")
			a.Update(keyF2)
			return a
		},
	}
	for name, mk := range cases {
		t.Run(name, func(t *testing.T) {
			a := mk(t)
			if a.capture == nil && a.rename == nil {
				t.Fatal("setup: no prompt open")
			}
			before := listStateOf(a)
			for _, msg := range []tea.Msg{click(40, 12), click(3, 2), click(40, 22), wheel(40, 12, +1), wheel(40, 12, -1)} {
				a.Update(msg)
				if got := listStateOf(a); got != before {
					t.Fatalf("%T changed the view:\n before %+v\n after  %+v", msg, before, got)
				}
			}
		})
	}
}

func TestMouseClickOutsideOverlay(t *testing.T) {
	for _, c := range listCases() {
		t.Run(c.name, func(t *testing.T) {
			a := c.overlay(t, 24)
			before := listStateOf(a)
			for _, xy := range [][2]int{{0, 0}, {79, 23}, {79, 0}, {0, 23}} {
				a.Update(click(xy[0], xy[1]))
				if got := listStateOf(a); got != before {
					t.Errorf("click at %v changed the overlay:\n before %+v\n after  %+v", xy, before, got)
				}
			}
		})
	}
}
