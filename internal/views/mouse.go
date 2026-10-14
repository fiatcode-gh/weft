package views

import tea "charm.land/bubbletea/v2"

// wheelRows is how many page rows one wheel notch scrolls.
const wheelRows = 3

// wheelKey is the key a wheel step stands for in a list: up or down a row.
func wheelKey(dir int) string {
	if dir < 0 {
		return keyUp
	}
	return keyDown
}

// mouseTarget reports whether the event is one the App acts on: the mouse is
// on, a page is showing and the cell is on screen. Like a key press, an
// accepted event clears the status hint.
func (a *App) mouseTarget(m tea.Mouse) bool {
	if a.mouseOff || a.page == nil || m.X < 0 || m.Y < 0 {
		return false
	}
	a.hint = ""
	return true
}

// mouseClick routes a left click the way keys are routed: the editor, then
// the prompts (which own the input), the open overlay, the sidebar column and
// the page.
func (a *App) mouseClick(m tea.Mouse) tea.Cmd {
	if m.Button != tea.MouseLeft || !a.mouseTarget(m) {
		return nil
	}
	switch {
	case a.editor != nil:
		return nil
	case a.capture != nil || a.rename != nil:
		return nil
	case a.active != nil:
		return nil
	}
	return a.readClick(m.X, m.Y)
}

// mouseWheel routes a vertical wheel step like mouseClick routes a click.
func (a *App) mouseWheel(m tea.Mouse) tea.Cmd {
	if (m.Button != tea.MouseWheelUp && m.Button != tea.MouseWheelDown) || !a.mouseTarget(m) {
		return nil
	}
	dir := 1
	if m.Button == tea.MouseWheelUp {
		dir = -1
	}
	switch {
	case a.editor != nil:
		return nil
	case a.capture != nil || a.rename != nil:
		return nil
	case a.active != nil:
		return nil
	}
	return a.readWheel(m.X, m.Y, dir)
}

// pageRows is how many screen rows the page and the sidebar column use: all
// but the rule and the status line.
func (a *App) pageRows() int { return max(1, a.height-2) }

// inSidebar reports whether column x belongs to the sidebar.
func (a *App) inSidebar(x int) bool { return a.side.shown && x < sidebarCols }

// readClick handles a click on the read view: the sidebar column takes focus,
// the page column drops it and follows a link or moves the row cursor.
func (a *App) readClick(x, y int) tea.Cmd {
	if y >= a.pageRows() {
		return nil
	}
	if a.inSidebar(x) {
		a.side.focused = true
		return nil
	}
	a.side.focused = false
	px := x
	if a.side.shown {
		px -= sidebarCols
	}
	row, link, ok := a.page.At(px, y)
	if !ok {
		return nil
	}
	if link >= 0 {
		a.page.SelectLink(link)
		return readKeys[keyEnter].run(a)
	}
	a.page.SelectRow(row)
	return nil
}

// readWheel scrolls the sidebar's panel by a row over the sidebar column
// (focus unchanged), else the page by wheelRows.
func (a *App) readWheel(x, y, dir int) tea.Cmd {
	if a.inSidebar(x) && y < a.pageRows() {
		return a.sidebarKey(wheelKey(dir))
	}
	a.page.Scroll(dir * wheelRows)
	return nil
}
