package views

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
)

const (
	// sidebarBoxWidth is the width every panel's box draws at: the calendar
	// box (inner 44 + 6) is the widest minimum of the five.
	sidebarBoxWidth = 50
	// sidebarPanelWidth is the width handed to a panel's SetSize. Every panel
	// keeps a 4-cell margin in innerWidth, so its box draws sidebarBoxWidth.
	sidebarPanelWidth = sidebarBoxWidth + 4
	// sidebarCols is the column the sidebar takes from the page: the box plus
	// one blank gutter column.
	sidebarCols = sidebarBoxWidth + 1
	// minPageWidth is the narrowest page the sidebar may leave.
	minPageWidth = 60
	// sidebarMinWidth is the narrowest terminal that can show the sidebar.
	sidebarMinWidth = sidebarCols + minPageWidth
)

// sidebarSection is which panel the sidebar column shows.
type sidebarSection uint8

const (
	sectionTree sidebarSection = iota
	sectionBacklinks
	sectionTodos
	sectionAgenda
	sectionCalendar
)

var sectionTitles = [...]string{"Page tree", "Backlinks", "Todos", "Agenda", "Calendar"}

// sidebar is the left column's state: one panel per section, built on first
// show and kept for the session. Session only, never written anywhere.
type sidebar struct {
	shown, focused bool
	section        sidebarSection

	tree      *Tree
	backlinks *Backlinks
	todos     *Todos
	agenda    *Agenda
	calendar  *Calendar

	// treeOpen is the sidebar tree's own expanded groups, so it never shares
	// a map with the F overlay.
	treeOpen treeExpansion
}

// panel is the current section's panel, or an untyped nil when it is not built.
func (s *sidebar) panel() Overlay {
	switch s.section {
	case sectionTree:
		if s.tree != nil {
			return s.tree
		}
	case sectionBacklinks:
		if s.backlinks != nil {
			return s.backlinks
		}
	case sectionTodos:
		if s.todos != nil {
			return s.todos
		}
	case sectionAgenda:
		if s.agenda != nil {
			return s.agenda
		}
	case sectionCalendar:
		if s.calendar != nil {
			return s.calendar
		}
	}
	return nil
}

// setSize sizes every built panel for a terminal of the given height.
func (s *sidebar) setSize(height int) {
	h := max(1, height-3)
	if s.tree != nil {
		s.tree.SetSize(sidebarPanelWidth, h)
	}
	if s.backlinks != nil {
		s.backlinks.SetSize(sidebarPanelWidth, h)
	}
	if s.todos != nil {
		s.todos.SetSize(sidebarPanelWidth, h)
	}
	if s.agenda != nil {
		s.agenda.SetSize(sidebarPanelWidth, h)
	}
	if s.calendar != nil {
		s.calendar.SetSize(sidebarPanelWidth, h)
	}
}

// header is the focus cue row of the column.
func (s *sidebar) header() string {
	name := fmt.Sprintf("%d %s", int(s.section)+1, sectionTitles[s.section])
	if s.focused {
		return styleSel.Render(padTo(" ▶ "+name+" · esc: page", sidebarBoxWidth)) + " "
	}
	return styleFaint.Render(padTo("   "+name+fmt.Sprintf(" · %d: focus", int(s.section)+1), sidebarBoxWidth)) + " "
}

// column is the sidebar drawn as exactly rows lines, each sidebarCols wide.
func (s *sidebar) column(rows int) []string {
	lines := []string{s.header()}
	if p := s.panel(); p != nil {
		lines = append(lines, strings.Split(p.View(), "\n")...)
	}
	out := make([]string, rows)
	for i := range out {
		if i < len(lines) {
			out[i] = padTo(lines[i], sidebarCols)
		} else {
			out[i] = strings.Repeat(" ", sidebarCols)
		}
	}
	return out
}

// pageWidth is the width the page is laid out at.
func (a *App) pageWidth() int {
	if a.side.shown {
		return a.width - sidebarCols
	}
	return a.width
}

// resizePageKeepingPlace resizes the page to pageWidth, keeping the cursor on
// its source line and screen row.
func (a *App) resizePageKeepingPlace() {
	if a.page == nil {
		return
	}
	at, ok := a.page.ReadingAnchor()
	a.page.SetSize(a.pageWidth(), a.height)
	if ok {
		a.page.PlaceAnchor(at)
	}
}

// ensurePanel builds the section's panel when it does not exist yet.
func (a *App) ensurePanel(sec sidebarSection) {
	s := &a.side
	w, h := sidebarPanelWidth, max(1, a.height-3)
	switch sec {
	case sectionTree:
		if s.tree == nil {
			if s.treeOpen == nil {
				s.treeOpen = treeExpansion{}
			}
			s.tree = NewTree(a.idx, a.page.Page(), s.treeOpen, w, h)
		}
	case sectionBacklinks:
		if s.backlinks == nil {
			s.backlinks = NewBacklinks(a.idx, a.page.Page(), nil, w, h)
		}
	case sectionTodos:
		if s.todos == nil {
			s.todos = NewTodos(a.idx, w, h)
		}
	case sectionAgenda:
		if s.agenda == nil {
			s.agenda = NewAgenda(a.idx, a.nowFunc(), w, h)
		}
	case sectionCalendar:
		if s.calendar == nil {
			s.calendar = NewCalendar(a.idx, a.page.Page(), a.nowFunc(), w, h)
		}
	}
}

// showSidebar shows the sidebar on sec and focuses it, or refuses on a
// terminal too narrow to leave minPageWidth for the page.
func (a *App) showSidebar(sec sidebarSection) tea.Cmd {
	if a.page == nil || a.idx == nil {
		return nil
	}
	if !a.side.shown && a.width < sidebarMinWidth {
		return a.setHint(fmt.Sprintf("sidebar needs %d+ columns", sidebarMinWidth))
	}
	wasShown := a.side.shown
	if wasShown && a.side.section != sec {
		a.side.leaveSection()
	}
	a.ensurePanel(sec)
	a.side.section = sec
	a.side.shown = true
	a.side.focused = true
	if !wasShown {
		a.resizePageKeepingPlace()
	}
	return nil
}

// toggleSidebar hides the sidebar, or shows the last section.
func (a *App) toggleSidebar() tea.Cmd {
	if a.side.shown {
		a.hideSidebar()
		return nil
	}
	return a.showSidebar(a.side.section)
}

// hideSidebar hides the sidebar and gives the page the full width.
func (a *App) hideSidebar() {
	a.side.leaveSection()
	a.side.shown = false
	a.side.focused = false
	a.resizePageKeepingPlace()
}

// sidebarFocused reports whether keys go to the sidebar's panel.
func (a *App) sidebarFocused() bool { return a.side.focused && a.side.shown }

// keyPanel is the panel that takes keys and shows feedback: the open overlay,
// else the focused sidebar panel, else nil.
func (a *App) keyPanel() Overlay {
	if a.active != nil {
		return a.active
	}
	if a.sidebarFocused() {
		return a.side.panel()
	}
	return nil
}

// sidebarKey hands key to the focused sidebar panel and acts on its result.
// Navigation keeps the focus in the sidebar; only the panel's own cancel moves
// it back to the page.
func (a *App) sidebarKey(key string) tea.Cmd {
	res := a.side.panel().Update(key)
	switch res.kind {
	case overlayResultCancel:
		a.side.focused = false
	case overlayResultMarkTask:
		if cmd, blocked := a.blockIfSyncing(); blocked {
			return cmd
		}
		return a.markTask(res.mark)
	case overlayResultCapture:
		a.openCapture()
	default:
		a.openResult(res)
	}
	return nil
}

// leaveSection drops the struck rows of the todos or agenda panel, which
// stay listed only while the user is in that section.
func (s *sidebar) leaveSection() {
	switch s.section {
	case sectionTodos:
		if s.todos != nil {
			s.todos.clearDone()
		}
	case sectionAgenda:
		if s.agenda != nil {
			s.agenda.clearDone()
		}
	}
}

// readView is the page with its status bar, with the sidebar column on the
// left when shown.
func (a *App) readView() string {
	if !a.side.shown {
		return a.page.View() + "\n" + a.statusBar()
	}
	rows := max(1, a.height-2)
	col := a.side.column(rows)
	page := strings.Split(a.page.View(), "\n")
	var sb strings.Builder
	for i := range rows {
		sb.WriteString(col[i])
		if i < len(page) {
			sb.WriteString(page[i])
		}
		sb.WriteByte('\n')
	}
	sb.WriteString(a.statusBar())
	return sb.String()
}
