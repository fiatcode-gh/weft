package views

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/exp/teatest/v2"
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
			t.Skipf("f2 did not open the rename prompt")
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
			if a.editor != nil || !a.side.shown || a.page.width != 69 {
				t.Fatalf("editor open %v shown %v page width %d", a.editor != nil, a.side.shown, a.page.width)
			}
			at, _ := a.page.ReadingAnchor()
			if at.Line != at0.Line || at.ScreenRow != at0.ScreenRow {
				t.Errorf("line %d row %d, want line %d row %d", at.Line, at.ScreenRow, at0.Line, at0.ScreenRow)
			}
			if row := frameLines(a)[0]; !strings.Contains(row, "3 Todos") {
				t.Errorf("row 0 %q lacks the sidebar header", row)
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
