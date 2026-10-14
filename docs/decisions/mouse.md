# The mouse works by default; `m` turns it off

The wheel scrolls the page, lists, the calendar and the editor. A click follows
a link, picks a list row or a calendar day, focuses the sidebar and places the
editor cursor. `m` turns the mouse off for the session (and on again), so the
terminal's own text selection works; the status bar says `mouse off`.

Why: weft was keyboard-only. New users reach for the mouse first, and scrolling
a long page or picking a row from a list is quicker with it.

Out of scope: drag to select, double or triple click, hover effects, right
click and middle-click paste, dragging the sidebar edge, mouse in the `$EDITOR`
handoff, a config option for the default, the editor's status line showing
`mouse off`, clicking the status bar or rule row, folding by click, closing an
overlay by clicking outside it, horizontal wheel, holding a centred overlay
box's screen position when its height changes.

## Decisions

1. **Mode.** `App.mouseOff` (zero value: on) is session-only and never written.
   `App.View` sets `MouseMode` to `tea.MouseModeCellMotion` unless it is set, in
   every frame. No `AllMotion`: hover would flood `Update`. Bubble Tea resets
   the mode on quit and around `tea.ExecProcess`, so `$EDITOR` gets no mouse
   reporting and needs no code.
2. **`m` is an action-table entry** (`readActions`, before `?`), with a help row
   and a palette entry, so it follows the action-table rule. In the editor,
   overlays and prompts they see keys first, so `m` stays text there.
3. **Marker.** `statusBar()` prefixes `mouse off  ` to the right-hand text while
   off; it is hidden while a hint shows. Nothing shows while on.
4. **One funnel.** `internal/views/mouse.go` has `mouseClick` and `mouseWheel`;
   `App.update` has one case for each message. Release and motion messages are
   not handled (no drag). The order mirrors key routing: nothing when off or
   with no page; left button or vertical wheel only; clear the hint; editor;
   prompts (ignored: they own the keys); open overlay; sidebar column; page.
   Modifiers are ignored and negative coordinates are ignored at every target.
5. **Reuse the key path.** Every effect goes through the code its key uses:
   follow is `readKeys[keyEnter].run`; a wheel step over a list is the panel's
   up or down key through `overlayResult` or `sidebarKey`; a second click is
   `Update(keyEnter)`; the first click steps with the panel's own up and down
   keys until `selected()` is the item (`pickRow`). Refusals, `errMsg` clearing,
   header skipping and sync guards are therefore the keys' own.
   `overlayResult` and `sidebarResult` are the result handling the key path uses.
6. **Read-view geometry.** Page rows are screen rows `0` to `max(1, height-2)-1`
   (`pageRows`); the rule row and the status line do nothing. With the sidebar
   shown, `x < sidebarCols` is the sidebar column, and the page column is
   `x - sidebarCols`.
7. **Page API.** `PageView.At(x, y)` gives the row and the link under a cell;
   `SelectLink`, `SelectRow` and `Scroll` are what a click or wheel call. A
   link is always on one styled row, so a wrapped link is a link on a
   continuation row of a wrapped bullet. Columns under a fold marker hit no link.
8. **Page click and wheel.** A click unfocuses the sidebar, then follows the link
   (`SelectLink` plus the `enter` action) or moves the row cursor (`SelectRow`).
   The wheel over the page is `Scroll(dir * wheelRows)`, `wheelRows` is 3; the
   cursor stays unless it leaves the screen.
9. **Sidebar.** A click in the sidebar column focuses it; the header row only
   focuses; any other row goes to `clickPanel` with the box at (0, 1). The
   gutter column is outside the box. The wheel over the sidebar never changes
   focus.
10. **Overlay boxes.** `boxOrigin` repeats how `centerOverlay` places a box
    (Lip Gloss splits spare space with the leading side rounded down). A click
    renders the panel once, measures it (`panelBox`) and does nothing outside
    the box; the overlay stays open. The wheel goes to the open overlay wherever
    it is.
11. **`clickPanel`.** It maps a click inside the box to a body line and column
    from the border and padding sizes. A `rowPanel` gets `drawnRows().itemAt`
    then `pickRow`; `*Calendar` gets `dayAt` then `pickDay`; Help and the rename
    preview do nothing.
12. **`rowMap`** (`listbox.go`). Each list panel's `View` calls `drawn.reset()`
    and `span`s each selectable item as it draws it, so the map is the one that
    was drawn. The panels are Picker (matches and the create row), SearchView,
    Palette, Backlinks (not while confirming), Todos and Agenda (task rows, not
    group headers), Tree and OnThisDay (a head plus its preview lines is one
    span).
13. **Calendar.** `View` records `Calendar.gridTop`; `dayAt` maps a body line and
    column to a day (cells are 5 wide); `pickDay` is what a move key does to
    the cursor. A click on the cursor's day is `Update(keyEnter)`; the wheel is
    up or down, one week.
14. **Editor** (`editor_mouse.go`). `Click` and `Wheel` return at once unless the
    editor is editing with no find bar and no prompt. A click on the text area
    places the cursor: exact character in source mode, the line in live preview
    (the landing rule of paging), the end of the buffer below the last row;
    then `MoveTo` clears the selection and completion follows the cursor as
    after `left`/`right`. The wheel moves the window up to `wheelRows` rows and
    moves the cursor only as far as needed to stay on screen.
15. **Clicks never add writes.** A click reaches a write only where `enter`
    does (a palette action, the picker's create row opening the editor, no save).
    The backlinks confirm is not clickable. `overlayResult` keeps the
    `blockIfSyncing` guards.
16. **A click keeps the list still** (user decision, 2026-10-14). A click on a
    row selects it (or, on the selected row, opens it) without scrolling the
    list: the panel draws the window of the frame that was clicked until the
    next key or wheel step reaches it, which centres the selection as before.
    Keyboard-only use never anchors, so keyboard behaviour is unchanged. One
    mechanism serves all eight list panels, overlay and sidebar:
    `listBox.anchor`/`dropAnchor`, with `listBox.window` and
    `listBox.groupWindow` choosing between the anchored and the centred
    window. The anchor is kept while the selection is still drawn in the
    window and dropped otherwise, which covers refresh, shrink, resize and
    filter typing. `sendKey` drops the anchor before a key or wheel step
    reaches a panel.

## Traps

- A new list panel's `View` must `reset` and `span` its rows into `rowMap`, or
  clicks miss. It must take its window from `listBox.window` or
  `groupWindow`, not `scrollWindow`, or a click scrolls it.
- A new key route into a list panel must go through `sendKey`, or the anchor
  outlives the key and the next key does not centre the selection.
- A centred overlay whose box height changes after a click (a tree group
  expanded in `F`, a footer error line) moves on screen: the anchor holds the
  list window, not the box position.
- A new overlay result kind needs a case in `overlayResult` and `sidebarResult`.
- `Calendar.gridTop` is set by `View`; hit-test only after a `View`.
- Help row titles stay at most 31 characters. The two-column help needs 127
  columns today (`helpColGutter`, `helpChrome`) and its golden is 130 wide; a
  longer title in the right column widens it.
- Text clicks are ignored while the editor's find bar or date prompt is open.
