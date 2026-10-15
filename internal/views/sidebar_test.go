package views

import (
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/exp/teatest/v2"
	"github.com/fiatcode-gh/weft/v3/internal/edit"
)

// sidebarApp boots the fixture clone at the given size.
func sidebarApp(t *testing.T, w, h int) *App {
	t.Helper()
	a := bootApp(t, bootConfig{now: paletteNow})
	a.Update(tea.WindowSizeMsg{Width: w, Height: h})
	return a
}

func resize(a *App, w, h int) { a.Update(tea.WindowSizeMsg{Width: w, Height: h}) }

// sectionPanelType names the built panel of a section, "" when not built.
func sectionPanelType(a *App, sec sidebarSection) string {
	var built bool
	switch sec {
	case sectionTree:
		built = a.side.tree != nil
	case sectionBacklinks:
		built = a.side.backlinks != nil
	case sectionTodos:
		built = a.side.todos != nil
	case sectionAgenda:
		built = a.side.agenda != nil
	case sectionCalendar:
		built = a.side.calendar != nil
	}
	if !built {
		return ""
	}
	return fmt.Sprintf("%T", a.side.panel())
}

func TestSidebarHiddenByDefault(t *testing.T) {
	a := sidebarApp(t, 120, 40)
	if a.side.shown {
		t.Fatal("sidebar shown at boot")
	}
	if a.page.width != 120 {
		t.Errorf("page width %d, want 120", a.page.width)
	}
	if got, want := appText(a), plain(a.page.View()+"\n"+a.statusBar()); got != want {
		t.Errorf("hidden frame differs from the plain read view\n got:\n%s\nwant:\n%s", got, want)
	}
}

func TestSidebarKeysOpenSections(t *testing.T) {
	want := []string{"*views.Tree", "*views.Backlinks", "*views.Todos", "*views.Agenda", "*views.Calendar"}
	for i, typ := range want {
		t.Run(sectionTitles[i], func(t *testing.T) {
			a := sidebarApp(t, 120, 30)
			a.navigate("Alpha")
			a.Update(key(fmt.Sprint(i + 1)))
			sec := sidebarSection(i)
			if !a.side.shown || !a.side.focused || a.side.section != sec {
				t.Fatalf("shown %v focused %v section %d, want shown focused section %d",
					a.side.shown, a.side.focused, a.side.section, sec)
			}
			if got := sectionPanelType(a, sec); got != typ {
				t.Errorf("panel %q, want %q", got, typ)
			}
			head := fmt.Sprintf("%d %s", i+1, sectionTitles[i])
			if row := frameLines(a)[0]; !strings.Contains(row, head) {
				t.Errorf("first row %q lacks %q", row, head)
			}
		})
	}
}

func TestSidebarToggleRemembersSection(t *testing.T) {
	a := sidebarApp(t, 120, 30)
	a.Update(key("|"))
	if !a.side.shown || a.side.section != sectionTree {
		t.Fatalf("first | : shown %v section %d, want the tree", a.side.shown, a.side.section)
	}
	a.Update(key("4"))
	a.Update(key("|"))
	if a.side.shown || a.side.focused || a.page.width != 120 {
		t.Fatalf("hide: shown %v focused %v page width %d", a.side.shown, a.side.focused, a.page.width)
	}
	a.Update(key("|"))
	if !a.side.shown || a.side.section != sectionAgenda {
		t.Errorf("show again: shown %v section %d, want the agenda", a.side.shown, a.side.section)
	}
}

func TestSidebarFrameGeometry(t *testing.T) {
	a := sidebarApp(t, 120, 30)
	a.Update(key("3"))
	check := func(stage string) {
		t.Helper()
		lines := frameLines(a)
		if len(lines) != a.height {
			t.Errorf("%s: %d lines, want %d", stage, len(lines), a.height)
		}
		for i, l := range lines {
			if w := lipgloss.Width(l); w != a.width {
				t.Errorf("%s: line %d is %d wide, want %d: %q", stage, i, w, a.width, l)
			}
		}
		if a.page.width != a.width-51 {
			t.Errorf("%s: page width %d, want %d", stage, a.page.width, a.width-51)
		}
		bar := strings.Split(plain(a.statusBar()), "\n")
		if len(bar) != 2 || len(lines) < 2 || lines[len(lines)-2] != bar[0] || lines[len(lines)-1] != bar[1] {
			t.Errorf("%s: last two lines are not the status bar\n%v\n%v", stage, lines[max(0, len(lines)-2):], bar)
		}
	}
	check("120x30")
	resize(a, 140, 40)
	check("140x40")
	a.Update(key("c"))
	typeText(a, "geo")
	a.Update(keyEnt)
	if a.capture != nil {
		t.Fatal("capture prompt still open")
	}
	check("after capture")
}

func TestSidebarPanelsFitColumn(t *testing.T) {
	a := sidebarApp(t, 120, 30)
	a.navigate("Alpha")
	for i := range sectionTitles {
		a.Update(key(fmt.Sprint(i + 1)))
		a.side.panel().SetSize(sidebarPanelWidth, 27)
		for n, l := range strings.Split(plain(a.side.panel().View()), "\n") {
			if w := lipgloss.Width(l); w != sidebarBoxWidth {
				t.Errorf("%s line %d is %d wide, want %d: %q", sectionTitles[i], n, w, sidebarBoxWidth, l)
			}
		}
	}
}

// putCursorOnLine moves the page cursor down until it reads source line line,
// on row rowInLine of that line.
func putCursorOnLine(t *testing.T, a *App, line, rowInLine int) Anchor {
	t.Helper()
	for range 200 {
		at, ok := a.page.ReadingAnchor()
		if !ok {
			t.Fatal("no reading anchor")
		}
		if at.Line == line && at.RowInLine == rowInLine {
			return at
		}
		a.page.LineDown()
	}
	t.Fatalf("cursor never reached line %d row %d", line, rowInLine)
	return Anchor{}
}

func TestSidebarKeepsReadingPosition(t *testing.T) {
	t.Run("short lines", func(t *testing.T) {
		a := sidebarApp(t, 120, 16)
		a.navigate("Workbench")
		for range 12 {
			a.page.LineDown()
		}
		at0, _ := a.page.ReadingAnchor()
		for _, k := range []string{"3", "|"} {
			a.Update(key(k))
			at, _ := a.page.ReadingAnchor()
			if at.Line != at0.Line || at.ScreenRow != at0.ScreenRow {
				t.Errorf("after %q: line %d row %d, want line %d row %d", k, at.Line, at.ScreenRow, at0.Line, at0.ScreenRow)
			}
		}
	})
	t.Run("long wrapping line", func(t *testing.T) {
		a := sidebarApp(t, 120, 16)
		a.navigate("Corpus")
		at0 := putCursorOnLine(t, a, 2, 0)
		for _, k := range []string{"3", "|"} {
			a.Update(key(k))
			at, _ := a.page.ReadingAnchor()
			if at.Line != at0.Line || at.ScreenRow < 0 || at.ScreenRow >= a.page.vp.Height() {
				t.Errorf("after %q: line %d row %d, want line %d on screen", k, at.Line, at.ScreenRow, at0.Line)
			}
		}
	})
}

func TestSidebarRefusedWhenNarrow(t *testing.T) {
	a := sidebarApp(t, 80, 24)
	for _, k := range []string{"3", "|"} {
		a.Update(key(k))
		if a.side.shown || a.hint != "sidebar needs 111+ columns" {
			t.Errorf("%q at 80: shown %v hint %q", k, a.side.shown, a.hint)
		}
	}
	resize(a, 110, 24)
	a.Update(key("3"))
	if a.side.shown {
		t.Error("shown at 110")
	}
	resize(a, 111, 24)
	a.Update(key("3"))
	if !a.side.shown || a.page.width != 60 {
		t.Errorf("at 111: shown %v page width %d, want shown, 60", a.side.shown, a.page.width)
	}
}

func TestSidebarHidesOnNarrowResize(t *testing.T) {
	a := sidebarApp(t, 120, 30)
	a.navigate("Workbench")
	for range 5 {
		a.page.LineDown()
	}
	at0, _ := a.page.ReadingAnchor()
	a.Update(key("3"))
	resize(a, 110, 30)
	if a.side.shown || a.side.focused || a.page.width != 110 {
		t.Fatalf("shown %v focused %v page width %d, want hidden, 110", a.side.shown, a.side.focused, a.page.width)
	}
	if at, _ := a.page.ReadingAnchor(); at.Line != at0.Line {
		t.Errorf("cursor line %d, want %d", at.Line, at0.Line)
	}
	resize(a, 120, 30)
	if a.side.shown {
		t.Error("widening reopened the sidebar")
	}
}

func TestSidebarHidesOnNarrowResizeKeepsWrappedLine(t *testing.T) {
	a := sidebarApp(t, 120, 16)
	a.navigate("Corpus")
	a.Update(key("3"))
	a.Update(keyEscK)
	at0 := putCursorOnLine(t, a, 2, 0)
	resize(a, 110, 16)
	if a.side.shown {
		t.Fatal("sidebar still shown at 110")
	}
	at, _ := a.page.ReadingAnchor()
	if at.Line != at0.Line || at.ScreenRow < 0 || at.ScreenRow >= a.page.vp.Height() {
		t.Errorf("after hide by resize: line %d row %d, want line %d on screen", at.Line, at.ScreenRow, at0.Line)
	}
}

func TestSidebarOverlaysFullScreen(t *testing.T) {
	a := sidebarApp(t, 120, 30)
	a.navigate("Hub")
	a.Update(key("3"))
	a.Update(keyEscK)
	head := "3 Todos"
	if !strings.Contains(frameLines(a)[0], head) {
		t.Fatalf("setup: header missing: %q", frameLines(a)[0])
	}
	before := appText(a)
	for _, k := range []tea.KeyPressMsg{key("b"), key("F"), key("T"), key("A"), key("C"),
		{Code: 'p', Mod: tea.ModCtrl}, key("/"), key("?"), key(":"), key("O")} {
		t.Run(k.String(), func(t *testing.T) {
			a.Update(k)
			if a.active == nil {
				t.Fatalf("%s opened no overlay", k)
			}
			got := appText(a)
			if want := plain(a.centerOverlay(a.active.View())); got != want {
				t.Errorf("frame is not the full-screen overlay\n got:\n%s\nwant:\n%s", got, want)
			}
			if strings.Contains(got, head) {
				t.Errorf("overlay frame shows the sidebar header")
			}
			a.Update(keyEscK)
			if a.active != nil {
				t.Fatalf("esc left %T open", a.active)
			}
			if after := appText(a); after != before {
				t.Errorf("frame after closing differs from before")
			}
		})
	}
	t.Run("capture", func(t *testing.T) {
		a.Update(key("c"))
		if a.capture == nil {
			t.Fatal("capture prompt did not open")
		}
		lines := frameLines(a)
		if len(lines) != a.height || !strings.Contains(lines[0], head) {
			t.Errorf("capture frame: %d lines, first %q", len(lines), lines[0])
		}
		a.Update(keyEscK)
		if appText(a) != before {
			t.Error("frame after esc differs from before")
		}
	})
	t.Run("rename", func(t *testing.T) {
		a.Update(key("f2"))
		if a.rename == nil {
			t.Fatal("f2 did not open the rename prompt")
		}
		lines := frameLines(a)
		if len(lines) != a.height || !strings.Contains(lines[0], head) {
			t.Errorf("rename frame: %d lines, first %q", len(lines), lines[0])
		}
		a.Update(keyEscK)
		if appText(a) != before {
			t.Error("frame after esc differs from before")
		}
	})
}

func TestSidebarFocusHeader(t *testing.T) {
	a := sidebarApp(t, 120, 30)
	a.Update(key("3"))
	a.side.focused = true
	if row := plain(a.side.column(5)[0]); !strings.Contains(row, "▶ 3 Todos · esc: page") {
		t.Errorf("focused header %q", row)
	}
	a.side.focused = false
	row := plain(a.side.column(5)[0])
	if !strings.Contains(row, "3 Todos · 3: focus") || strings.Contains(row, "▶") {
		t.Errorf("unfocused header %q", row)
	}
}

func TestSidebarGolden(t *testing.T) {
	a := sidebarApp(t, 120, 30)
	a.navigate("Hub")
	a.Update(key("2"))
	teatest.RequireEqualOutput(t, []byte(appText(a)))
}

// editorSidebarApp is the editor tests' setup: Workbench read through the
// sidebar at 120x16, the cursor ten rows down.
func editorSidebarApp(t *testing.T) (*App, Anchor) {
	t.Helper()
	a := sidebarApp(t, 120, 16)
	a.navigate("Workbench")
	a.Update(key("3"))
	a.Update(keyEscK)
	for range 10 {
		a.page.LineDown()
	}
	at0, _ := a.page.ReadingAnchor()
	return a, at0
}

func TestSidebarHiddenInEditor(t *testing.T) {
	a, _ := editorSidebarApp(t)
	a.Update(key("e"))
	if a.editor == nil || a.editor.width != 120 {
		t.Fatalf("editor %v", a.editor)
	}
	lines := frameLines(a)
	if lines[a.height-2] != strings.Repeat("─", 120) {
		t.Errorf("rule row %q", lines[a.height-2])
	}
	if strings.Contains(strings.Join(lines, "\n"), "3 Todos") {
		t.Error("editor frame shows the sidebar")
	}
	if last := lines[len(lines)-1]; !strings.Contains(last, "[edit · ") {
		t.Errorf("last row %q lacks the edit status", last)
	}
}

func TestSidebarEditorOpensAtReadingPosition(t *testing.T) {
	check := func(t *testing.T, a *App, at0 Anchor) {
		t.Helper()
		a.Update(key("e"))
		if a.editor == nil {
			t.Fatal("editor did not open")
		}
		if got := a.editor.buf.Cursor().Line; got != at0.Line {
			t.Errorf("editor cursor line %d, want %d", got, at0.Line)
		}
		if got := a.editor.ExitAnchor().ScreenRow; got != at0.ScreenRow {
			t.Errorf("editor screen row %d, want %d", got, at0.ScreenRow)
		}
	}
	t.Run("short lines", func(t *testing.T) {
		a, at0 := editorSidebarApp(t)
		check(t, a, at0)
	})
	t.Run("continuation row", func(t *testing.T) {
		a := sidebarApp(t, 120, 16)
		a.navigate("Corpus")
		a.Update(key("3"))
		a.Update(keyEscK)
		at0 := putCursorOnLine(t, a, 2, 1)
		check(t, a, at0)
	})
}

func TestSidebarRestoredAfterEditor(t *testing.T) {
	exits := map[string]func(a *App){
		"unchanged": func(a *App) { a.Update(keyEscK) },
		"discard": func(a *App) {
			a.Update(key("x"))
			a.Update(keyEscK)
			a.Update(key("d"))
		},
		"save": func(a *App) {
			a.Update(key("x"))
			a.Update(tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl})
			a.Update(keyEscK)
			a.Update(a.buildIndexCmd()())
		},
	}
	for name, exit := range exits {
		t.Run(name, func(t *testing.T) {
			a, at0 := editorSidebarApp(t)
			a.Update(key("e"))
			if a.editor == nil {
				t.Fatal("editor did not open")
			}
			exit(a)
			if a.editor != nil || !a.side.shown || a.side.focused || a.page.width != 69 {
				t.Fatalf("editor open %v shown %v focused %v page width %d", a.editor != nil, a.side.shown, a.side.focused, a.page.width)
			}
			at, _ := a.page.ReadingAnchor()
			if at.Line != at0.Line || at.ScreenRow != at0.ScreenRow {
				t.Errorf("line %d row %d, want line %d row %d", at.Line, at.ScreenRow, at0.Line, at0.ScreenRow)
			}
			if row := frameLines(a)[0]; !strings.Contains(row, "3 Todos") {
				t.Errorf("row 0 %q lacks the sidebar header", row)
			}
			row0 := a.page.CursorRow()
			a.Update(key("j"))
			if a.page.CursorRow() == row0 {
				t.Error("j did not move the page cursor after the editor")
			}
		})
	}
}

func TestSidebarNarrowResizeWhileEditing(t *testing.T) {
	a, _ := editorSidebarApp(t)
	a.Update(key("e"))
	resize(a, 100, 16)
	a.Update(keyEscK)
	if a.editor != nil || a.side.shown || a.page.width != 100 {
		t.Fatalf("editor open %v shown %v page width %d, want closed, hidden, 100", a.editor != nil, a.side.shown, a.page.width)
	}
	resize(a, 120, 16)
	if a.side.shown {
		t.Error("widening reopened the sidebar")
	}
}

// focusSection shows the sidebar on the section whose key is digit.
func focusSection(t *testing.T, a *App, digit string) {
	t.Helper()
	a.Update(key(digit))
	if !a.sidebarFocused() {
		t.Fatalf("%s did not focus the sidebar", digit)
	}
}

// focusTodosOn focuses the todos section and selects the Workbench row with text.
func focusTodosOn(t *testing.T, a *App, text string) {
	t.Helper()
	focusSection(t, a, "3")
	for i, b := range a.side.todos.visible {
		if b.Page == "Workbench" && b.Text == text {
			a.side.todos.sel = i
			return
		}
	}
	t.Fatalf("no Workbench todo %q", text)
}

// focusAgendaOn focuses the agenda section and selects the Workbench row with text.
func focusAgendaOn(t *testing.T, a *App, text string) {
	t.Helper()
	focusSection(t, a, "4")
	for i, it := range a.side.agenda.items {
		if it.Todo.Page == "Workbench" && it.Todo.Text == text {
			a.side.agenda.sel = i
			return
		}
	}
	t.Fatalf("no Workbench agenda row %q", text)
}

const dustTask = "Replace the dust collector filter"

// doneRow is the struck row as the 50-column panel clips it.
const doneRow = "DONE [#B] Replace t"

func TestSidebarFocusedKeysGoToPanel(t *testing.T) {
	a := sidebarApp(t, 120, 30)
	a.navigate("Workbench")
	focusSection(t, a, "3")
	row, sel := a.page.CursorRow(), a.side.todos.sel
	a.Update(key("j"))
	if a.side.todos.sel != sel+1 {
		t.Errorf("todos sel %d, want %d", a.side.todos.sel, sel+1)
	}
	if a.page.CursorRow() != row {
		t.Errorf("page row moved %d -> %d", row, a.page.CursorRow())
	}
	for _, k := range []string{"e", ":", "T"} {
		a.Update(key(k))
		if a.editor != nil || a.active != nil || a.capture != nil {
			t.Fatalf("%q reached the read view: editor %v overlay %T", k, a.editor != nil, a.active)
		}
	}
}

func TestSidebarCloseKeysFocusPage(t *testing.T) {
	cases := []struct {
		digit string
		keys  []string
	}{
		{"1", []string{"esc", "q", "F"}},
		{"2", []string{"esc", "b"}},
		{"3", []string{"esc", "q"}},
		{"4", []string{"esc", "q"}},
		{"5", []string{"esc", "q"}},
	}
	for _, c := range cases {
		for _, k := range c.keys {
			t.Run(c.digit+" "+k, func(t *testing.T) {
				a := sidebarApp(t, 120, 30)
				a.navigate("Workbench")
				focusSection(t, a, c.digit)
				var msg tea.KeyPressMsg = key(k)
				if k == "esc" {
					msg = keyEscK
				}
				_, cmd := a.Update(msg)
				if cmd != nil {
					if _, quit := cmd().(tea.QuitMsg); quit {
						t.Fatal("close key quit the app")
					}
				}
				if !a.side.shown || a.side.focused {
					t.Fatalf("shown %v focused %v, want shown, page focused", a.side.shown, a.side.focused)
				}
				if row := frameLines(a)[0]; !strings.Contains(row, c.digit+": focus") {
					t.Errorf("header %q lacks %q", row, c.digit+": focus")
				}
				row := a.page.CursorRow()
				a.Update(key("j"))
				if a.page.CursorRow() == row {
					t.Error("j did not move the page cursor")
				}
			})
		}
	}
	t.Run("backlinks q is not a close key", func(t *testing.T) {
		a := sidebarApp(t, 120, 30)
		focusSection(t, a, "2")
		a.Update(key("q"))
		if !a.sidebarFocused() {
			t.Error("q left the backlinks panel")
		}
	})
}

func TestSidebarSectionKeysWhileFocused(t *testing.T) {
	a := sidebarApp(t, 120, 30)
	focusSection(t, a, "3")
	a.Update(key("4"))
	if a.side.section != sectionAgenda || !a.sidebarFocused() {
		t.Errorf("section %v focused %v, want agenda focused", a.side.section, a.side.focused)
	}
	a.Update(key("|"))
	if a.side.shown || a.side.focused {
		t.Errorf("| left shown %v focused %v", a.side.shown, a.side.focused)
	}
}

func TestSidebarCtrlCQuits(t *testing.T) {
	a := sidebarApp(t, 120, 30)
	focusSection(t, a, "3")
	_, cmd := a.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	if cmd == nil {
		t.Fatal("ctrl+c returned no cmd")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Errorf("ctrl+c cmd yielded %T, want tea.QuitMsg", cmd())
	}
}

func TestSidebarEnterKeepsFocus(t *testing.T) {
	cases := []struct {
		name  string
		setup func(t *testing.T, a *App) (want string)
	}{
		{"todos", func(t *testing.T, a *App) string {
			focusTodosOn(t, a, dustTask)
			return "Workbench"
		}},
		{"tree", func(t *testing.T, a *App) string {
			focusSection(t, a, "1")
			for i, r := range a.side.tree.rows {
				if r.node.Page != nil && r.node.Page.Key == "Hub" {
					a.side.tree.sel = i
					return "Hub"
				}
			}
			t.Fatal("no Hub row in the tree")
			return ""
		}},
		{"backlinks", func(t *testing.T, a *App) string {
			a.navigate("Hub")
			focusSection(t, a, "2")
			b := a.side.backlinks
			if len(b.refs) == 0 || b.sel < 0 || b.rows[b.sel].ref == nil {
				t.Fatal("Hub has no linked reference selected")
			}
			return a.canonicalName(b.rows[b.sel].ref.FromPage)
		}},
		{"agenda", func(t *testing.T, a *App) string {
			focusSection(t, a, "4")
			if len(a.side.agenda.items) == 0 {
				t.Fatal("agenda is empty")
			}
			a.side.agenda.sel = 0
			return a.side.agenda.items[0].Todo.Page
		}},
		{"calendar", func(t *testing.T, a *App) string {
			focusSection(t, a, "5")
			c := a.side.calendar
			j, ok := a.idx.JournalOn(c.cursor)
			if !ok {
				t.Fatalf("no journal on the calendar cursor %v", c.cursor)
			}
			return j.Name
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			a := sidebarApp(t, 120, 30)
			a.navigate("Alpha")
			want := c.setup(t, a)
			hist := len(a.hist)
			a.Update(keyEnterK)
			if got := a.page.Page(); got != want {
				t.Errorf("page %q, want %q", got, want)
			}
			if !a.side.focused || !a.side.shown || a.active != nil {
				t.Errorf("focused %v shown %v overlay %T", a.side.focused, a.side.shown, a.active)
			}
			if len(a.hist) != hist+1 {
				t.Errorf("history %d entries, want %d", len(a.hist), hist+1)
			}
		})
	}
}

// The tree and the calendar keep their own selection when the page changes;
// only backlinks follows the page.
func TestSidebarTreeAndCalendarKeepSelectionOnPageChange(t *testing.T) {
	a := sidebarApp(t, 120, 30)
	a.navigate("Alpha")
	focusSection(t, a, "5")
	a.Update(key("l"))
	cursor := a.side.calendar.cursor
	focusSection(t, a, "1")
	tr := a.side.tree
	tr.sel = -1
	for i, r := range tr.rows {
		if r.node.Page != nil && r.node.Page.Key == "Hub" {
			tr.sel = i
		}
	}
	if tr.sel < 0 {
		t.Fatal("no Hub row in the tree")
	}
	path := tr.rows[tr.sel].node.Path
	a.Update(keyEscK)
	a.navigate("Beta")
	a.Update(keyEscK) // any message syncs the sidebar
	if tr := a.side.tree; tr.rows[tr.sel].node.Path != path {
		got := tr.rows[tr.sel].node.Path
		t.Errorf("tree selection %q after the page changed, want %q", got, path)
	}
	a.Update(key("5"))
	if !a.side.calendar.cursor.Equal(cursor) {
		t.Errorf("calendar cursor %v after the page changed, want %v", a.side.calendar.cursor, cursor)
	}
}

func TestSidebarTaskXWritesThroughGuard(t *testing.T) {
	const dust = "- TODO [#B] " + dustTask
	for _, sec := range []string{"todos", "agenda"} {
		t.Run(sec, func(t *testing.T) {
			a := sidebarApp(t, 120, 30)
			path := workbenchPath(a)
			orig, _ := os.ReadFile(path)
			var view func() string
			if sec == "todos" {
				focusTodosOn(t, a, dustTask)
				view = a.side.todos.View
			} else {
				focusAgendaOn(t, a, dustTask)
				view = a.side.agenda.View
			}
			a.Update(key("x"))
			got, _ := os.ReadFile(path)
			want := strings.Replace(string(orig), dust, "- DONE [#B] "+dustTask, 1)
			if string(got) != want {
				t.Fatalf("after x:\n%q\nwant\n%q", got, want)
			}
			if v := plain(view()); !strings.Contains(v, doneRow) {
				t.Errorf("struck row missing:\n%s", v)
			}
			a.Update(key("x"))
			got, _ = os.ReadFile(path)
			if string(got) != string(orig) {
				t.Errorf("second x should restore the original bytes:\n%q\nwant\n%q", got, orig)
			}
		})
	}
}

func TestSidebarTaskXRefusedWhileSyncing(t *testing.T) {
	a := sidebarApp(t, 120, 30)
	path := workbenchPath(a)
	orig, _ := os.ReadFile(path)
	focusTodosOn(t, a, "Wax the bench top")
	a.syncing = true
	a.Update(key("x"))
	got, _ := os.ReadFile(path)
	if string(got) != string(orig) {
		t.Error("file must be unchanged while syncing")
	}
	if !strings.Contains(a.side.todos.errMsg, "sync in progress") {
		t.Errorf("errMsg = %q, want sync in progress", a.side.todos.errMsg)
	}
}

func TestSidebarTaskXRefusedWhenFileChanged(t *testing.T) {
	a := sidebarApp(t, 120, 30)
	path := workbenchPath(a)
	focusTodosOn(t, a, "Wax the bench top")
	orig, _ := os.ReadFile(path)
	changed := string(orig) + "- added meanwhile\n"
	mutated := false
	real := a.readSnapshot
	a.readSnapshot = func(p string) (edit.Snapshot, error) {
		s, err := real(p)
		if p == path && !mutated {
			mutated = true
			if werr := os.WriteFile(p, []byte(changed), 0o644); werr != nil {
				t.Error(werr)
			}
		}
		return s, err
	}
	a.Update(key("x"))
	got, _ := os.ReadFile(path)
	if string(got) != changed {
		t.Errorf("file = %q, want the outside change untouched", got)
	}
	if !strings.Contains(a.side.todos.errMsg, "changed on disk") {
		t.Errorf("errMsg = %q, want changed on disk", a.side.todos.errMsg)
	}
}

func TestSidebarDoneRowsClearOnSwitchAndHide(t *testing.T) {
	leaves := map[string]func(a *App, digit string){
		"switch": func(a *App, digit string) {
			other := "4"
			if digit == "4" {
				other = "3"
			}
			a.Update(key(other))
			a.Update(key(digit))
		},
		"hide": func(a *App, digit string) {
			a.Update(key("|"))
			a.Update(key("|"))
		},
		"resize": func(a *App, digit string) {
			resize(a, 100, 30)
			resize(a, 120, 30)
			a.Update(key(digit))
		},
	}
	for _, digit := range []string{"3", "4"} {
		for name, leave := range leaves {
			t.Run(digit+" "+name, func(t *testing.T) {
				a := sidebarApp(t, 120, 30)
				var view func() string
				var doneLen func() int
				if digit == "3" {
					focusTodosOn(t, a, dustTask)
					view, doneLen = a.side.todos.View, func() int { return len(a.side.todos.done) }
				} else {
					focusAgendaOn(t, a, dustTask)
					view, doneLen = a.side.agenda.View, func() int { return len(a.side.agenda.done) }
				}
				a.Update(key("x"))
				if !strings.Contains(plain(view()), doneRow) || doneLen() == 0 {
					t.Fatal("setup: no struck row after x")
				}
				leave(a, digit)
				if !a.sidebarFocused() {
					t.Fatalf("setup: sidebar not back on the section (shown %v focused %v)", a.side.shown, a.side.focused)
				}
				if strings.Contains(plain(view()), doneRow) {
					t.Error("struck row still listed")
				}
				if doneLen() != 0 {
					t.Errorf("%d done rows kept", doneLen())
				}
			})
		}
	}
}

func TestSidebarCaptureFeedbackInPanel(t *testing.T) {
	a := sidebarApp(t, 120, 30)
	focusSection(t, a, "3")
	a.Update(key("c"))
	if a.capture == nil {
		t.Fatal("c did not open capture")
	}
	for _, r := range "hello" {
		a.Update(key(string(r)))
	}
	a.Update(keyEnterK)
	if a.capture != nil {
		t.Fatal("capture still open")
	}
	if !strings.HasPrefix(a.side.todos.errMsg, "captured to") {
		t.Errorf("errMsg = %q, want captured to…", a.side.todos.errMsg)
	}
	if !a.sidebarFocused() {
		t.Error("focus left the sidebar")
	}
}

// reindexKey presses R on the page (esc first when the sidebar has focus) and
// finishes the reindex it starts.
func reindexKey(t *testing.T, a *App) {
	t.Helper()
	if a.sidebarFocused() {
		a.Update(keyEscK)
	}
	_, cmd := a.Update(key("R"))
	a.Update(drainFor[indexLoadedMsg](t, cmd))
}

// pollApp is a sidebar app whose poll loop is driven by hand.
func pollApp(t *testing.T, w, h int) *App {
	t.Helper()
	a := sidebarApp(t, w, h)
	instantPolls(a)
	if poll(t, a) {
		t.Fatal("a freshly booted graph must not rebuild on the first poll")
	}
	return a
}

func todoTexts(t *Todos) []string {
	out := make([]string, len(t.visible))
	for i, b := range t.visible {
		out[i] = b.Text
	}
	return out
}

func TestSidebarRefreshesOnLiveReload(t *testing.T) {
	a := pollApp(t, 120, 30)
	focusTodosOn(t, a, dustTask)
	want, _ := a.side.todos.selectedKey()
	outsideWrite(t, a, "pages/Zeta.md", "- TODO Zeta task\n")
	expectOneRebuildKeeping(t, a, func() {
		if !strings.Contains(strings.Join(todoTexts(a.side.todos), "\n"), "Zeta task") {
			t.Error("sidebar todos lack the new task after the rebuild")
		}
		if got, _ := a.side.todos.selectedKey(); got != want {
			t.Errorf("selection %v, want %v", got, want)
		}
		if row := strings.Split(plain(frameLines(a)[0]), "\n")[0]; !strings.Contains(row, "▶ 3 Todos") {
			t.Errorf("row 0 is not the sidebar header: %q", row)
		}
	})
}

func TestSidebarRefreshNearestWhenSelectedGone(t *testing.T) {
	a := pollApp(t, 120, 30)
	focusSection(t, a, "3")
	idx := -1
	for i, b := range a.side.todos.visible {
		if b.Page == "Hub" && b.Text == "Tidy up the hub" {
			idx = i
		}
	}
	if idx < 0 {
		t.Fatal("no Hub task in the todos")
	}
	a.side.todos.sel = idx
	outsideWrite(t, a, "pages/Hub.md", "title:: Hub\ntags:: fixture\n\n- The hub page. Linked from [[Alpha]].\n")
	expectOneRebuild(t, a)
	if strings.Contains(strings.Join(todoTexts(a.side.todos), "\n"), "Tidy up the hub") {
		t.Error("the removed task is still listed")
	}
	n := len(a.side.todos.visible)
	if want := min(idx, n-1); a.side.todos.sel != want {
		t.Errorf("sel %d, want %d (old index %d of %d rows)", a.side.todos.sel, want, idx, n)
	}
}

func TestSidebarTreeAndBacklinksKeepSelectionAcrossReindex(t *testing.T) {
	t.Run("tree", func(t *testing.T) {
		a := pollApp(t, 120, 30)
		focusSection(t, a, "1")
		path := ""
		for i, r := range a.side.tree.rows {
			if r.node.Page != nil && r.node.Page.Key == "Workbench" {
				a.side.tree.sel, path = i, r.node.Path
			}
		}
		if path == "" {
			t.Fatal("no Workbench node")
		}
		outsideWrite(t, a, "pages/Aaa.md", "# Aaa\n")
		expectOneRebuild(t, a)
		tr := a.side.tree
		if tr.sel < 0 || tr.sel >= len(tr.rows) || tr.rows[tr.sel].node.Path != path {
			t.Errorf("tree selection moved off %q", path)
		}
	})
	t.Run("backlinks", func(t *testing.T) {
		a := pollApp(t, 120, 30)
		a.navigate("Hub")
		focusSection(t, a, "2")
		b := a.side.backlinks
		last := -1
		for i, r := range b.rows {
			if r.ref != nil {
				last = i
			}
		}
		if last < 1 {
			t.Fatal("Hub needs at least two linked references")
		}
		b.sel = last
		type id struct {
			from string
			line int
		}
		before := id{b.rows[last].ref.FromPage, b.rows[last].ref.LineNumber}
		n := len(b.refs)
		outsideWrite(t, a, "pages/Aaa.md", "- see [[Hub]]\n")
		expectOneRebuild(t, a)
		b = a.side.backlinks
		if len(b.refs) != n+1 {
			t.Fatalf("%d refs, want %d", len(b.refs), n+1)
		}
		if b.sel < 0 || b.rows[b.sel].ref == nil {
			t.Fatal("no reference selected")
		}
		if got := (id{b.rows[b.sel].ref.FromPage, b.rows[b.sel].ref.LineNumber}); got != before {
			t.Errorf("selection %v, want %v", got, before)
		}
	})
}

func TestSidebarAgendaUsesCurrentDateOnRefresh(t *testing.T) {
	a := sidebarApp(t, 120, 30)
	focusSection(t, a, "4")
	next := paletteNow.AddDate(0, 0, 1)
	a.nowFunc = func() time.Time { return next }
	reindexKey(t, a)
	if got := a.side.agenda.today.Format("2006-01-02"); got != "2026-05-25" {
		t.Errorf("agenda today %s, want 2026-05-25", got)
	}
	if v := plain(a.side.agenda.View()); !strings.Contains(v, "today 2026-05-25") {
		t.Errorf("agenda header lacks the new date:\n%s", v)
	}
}

func TestSidebarCalendarKeepsCursorAcrossReindex(t *testing.T) {
	a := pollApp(t, 120, 30)
	focusSection(t, a, "5")
	day := time.Date(2026, 5, 26, 0, 0, 0, 0, time.UTC)
	a.side.calendar.cursor = day
	if a.side.calendar.hasJournal(day) {
		t.Fatal("setup: day already has a journal")
	}
	outsideWrite(t, a, "journals/2026_05_26.md", "- a day\n")
	expectOneRebuild(t, a)
	c := a.side.calendar
	if !c.cursor.Equal(day) {
		t.Errorf("cursor %v, want %v", c.cursor, day)
	}
	if !c.hasJournal(day) {
		t.Error("the day shows no journal mark after the reindex")
	}
}

func TestSidebarBacklinksFollowsPage(t *testing.T) {
	a := sidebarApp(t, 120, 30)
	a.navigate("Hub")
	focusSection(t, a, "2")
	check := func(step, wantPage string) {
		t.Helper()
		if got := a.page.Page(); wantPage != "" && got != wantPage {
			t.Fatalf("%s: page %q, want %q", step, got, wantPage)
		}
		if got, want := a.side.backlinks.target, a.page.Page(); got != want {
			t.Errorf("%s: backlinks target %q, page %q", step, got, want)
		}
	}
	check("start", "Hub")
	start := a.page.Page()

	a.Update(keyEnterK)
	if a.page.Page() == start || !a.sidebarFocused() {
		t.Fatalf("enter: page %q focused %v", a.page.Page(), a.sidebarFocused())
	}
	check("enter in sidebar", "")
	moved := a.page.Page()

	a.Update(keyEscK)
	a.Update(key("["))
	check("history back", "Hub")
	a.Update(key("]"))
	check("history forward", moved)

	a.Update(key("n"))
	a.Update(keyEnterK)
	if a.page.Page() == moved {
		t.Fatal("n + enter did not follow a link")
	}
	check("link", "")

	a.Update(tea.KeyPressMsg{Code: 'p', Mod: tea.ModCtrl})
	typeText(a, "Beta")
	a.Update(keyEnterK)
	check("picker", "Beta")

	a.Update(key("."))
	check("today", "2026_05_24")
	a.Update(key("<"))
	check("previous journal", "2026_05_23")
}

func TestSidebarRefreshAfterOwnWrite(t *testing.T) {
	a := sidebarApp(t, 120, 30)
	focusTodosOn(t, a, dustTask)
	a.Update(key("x"))
	if !strings.Contains(plain(a.side.todos.View()), doneRow) {
		t.Fatal("struck row lost right after x")
	}
	reindexKey(t, a)
	if !strings.Contains(plain(a.side.todos.View()), doneRow) {
		t.Error("struck row lost after a further reindex")
	}
	a.Update(key("c"))
	a.Update(keyTabK)
	typeText(a, "from capture")
	a.Update(keyEnt)
	if a.capture != nil {
		t.Fatal("capture still open")
	}
	if !strings.Contains(strings.Join(todoTexts(a.side.todos), "\n"), "from capture") {
		t.Errorf("captured task not listed: %v", todoTexts(a.side.todos))
	}
}

func TestSidebarWritesNothing(t *testing.T) {
	a := pollApp(t, 120, 30)
	before := graphFiles(t, a.graphPath)
	for _, d := range []string{"1", "2", "3", "4", "5"} {
		a.Update(key(d))
	}
	a.Update(key("|"))
	a.Update(key("|"))
	outsideWrite(t, a, "pages/Zeta.md", "- TODO Zeta task\n")
	expectOneRebuild(t, a)
	after := graphFiles(t, a.graphPath)
	delete(after, "pages/Zeta.md")
	if !reflect.DeepEqual(before, after) {
		t.Error("the sidebar changed a graph file")
	}
}

func TestSidebarHiddenPanelsCatchUpOnShow(t *testing.T) {
	a := pollApp(t, 120, 30)
	a.Update(key("3"))
	a.Update(key("|"))
	outsideWrite(t, a, "pages/Zeta.md", "- TODO Zeta task\n")
	expectOneRebuild(t, a)
	if a.side.idx == a.idx {
		t.Fatal("hidden sidebar refreshed")
	}
	a.Update(key("3"))
	if !strings.Contains(strings.Join(todoTexts(a.side.todos), "\n"), "Zeta task") {
		t.Error("todos lack the new task after showing")
	}
}
