package views

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

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
		return a.overlayClick(m.X, m.Y)
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
		return a.overlayResult(sendKey(a.active, wheelKey(dir)))
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
		if y == 0 {
			return nil
		}
		return a.sidebarClick(x, y)
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

// rowPanel is a list panel whose View records which body line holds which
// item (listBox), so a click on a row can pick it.
type rowPanel interface {
	Overlay
	selected() int
	drawnRows() *rowMap
	anchor()
	dropAnchor()
}

// sendKey is how a key or wheel step reaches a panel: a list panel's window
// is released from a click's anchor first, so the next one after a click
// centres the selection again.
func sendKey(p Overlay, key string) OverlayResult {
	if r, ok := p.(rowPanel); ok {
		r.dropAnchor()
	}
	return p.Update(key)
}

// pickRow is a click on item: on the selected item it is the enter key, else
// it moves the selection with the panel's own up and down keys (so header
// skipping and error clearing are the keys') until item is selected.
func pickRow(p rowPanel, item int) OverlayResult {
	if p.selected() == item {
		return p.Update(keyEnter)
	}
	for p.selected() != item {
		cur := p.selected()
		down := cur < item
		if down {
			p.Update(keyDown)
		} else {
			p.Update(keyUp)
		}
		if now := p.selected(); now == cur || (down && now > item) || (!down && now < item) {
			break
		}
	}
	return OverlayResult{}
}

// clickPanel handles a left click at (x, y), relative to the top-left corner
// of p's box and inside it. A row of a list panel is picked; everything else
// (title, divider, footer, headers, blank lines, other panels) does nothing.
func clickPanel(p Overlay, x, y int) OverlayResult {
	rp, ok := p.(rowPanel)
	if !ok {
		return OverlayResult{}
	}
	line := y - styleBorder.GetBorderTopSize() - styleBorder.GetPaddingTop()
	item, ok := rp.drawnRows().itemAt(line)
	if !ok {
		return OverlayResult{}
	}
	res := pickRow(rp, item)
	rp.anchor()
	return res
}

// boxOrigin is the top-left screen cell of a w by h box that centerOverlay
// placed: Lip Gloss splits the spare space with the leading side rounded
// down, and leaves a box larger than the screen at the origin.
func (a *App) boxOrigin(w, h int) (x, y int) {
	if a.width <= 0 || a.height <= 0 {
		return 0, 0
	}
	return max(0, (a.width-w)/2), max(0, (a.height-h)/2)
}

// panelBox renders p once and reports the size of its box.
func panelBox(p Overlay) (w, h int) {
	view := p.View()
	return lipgloss.Width(view), strings.Count(view, "\n") + 1
}

// overlayClick hits the open overlay's box: a click outside it does nothing.
func (a *App) overlayClick(x, y int) tea.Cmd {
	w, h := panelBox(a.active)
	ox, oy := a.boxOrigin(w, h)
	if x < ox || y < oy || x >= ox+w || y >= oy+h {
		return nil
	}
	return a.overlayResult(clickPanel(a.active, x-ox, y-oy))
}

// sidebarClick hits the sidebar panel's box, which sits under the header row.
func (a *App) sidebarClick(x, y int) tea.Cmd {
	p := a.side.panel()
	if p == nil {
		return nil
	}
	w, h := panelBox(p)
	if x >= w || y-1 >= h {
		return nil
	}
	return a.sidebarResult(clickPanel(p, x, y-1))
}
