# `1`-`5` open a sidebar beside the page

A left sidebar shows one of five panels next to the page: the page tree,
backlinks of the page, todos, the agenda or the calendar. It is hidden by
default. `1`-`5` open a section and focus it, `|` shows or hides it, `Esc`
gives focus back to the page. `Enter` opens a page on the right and keeps focus
in the sidebar.

Why: these five lists were full-screen overlays. Opening a page from one closed
it, so browsing several pages from a list meant reopening the list each time,
and an open list never refreshed.

Out of scope: mouse (added in 11c, see `mouse.md`), a right-hand or second sidebar, resizing the sidebar
by key or config, unlinked references or linkify in the sidebar, search, picker
or on-this-day as sections, saving sidebar state across runs, showing the
sidebar beside the editor, changing the overlays' behaviour.

## Decisions

- **One file.** `internal/views/sidebar.go` holds the `sidebar` state (`App.side`),
  the section type and every App method the sidebar adds. App-level tests are in
  `sidebar_test.go`.
- **Panels are reused.** The sidebar hosts the existing `*Tree`, `*Backlinks`,
  `*Todos`, `*Agenda` and `*Calendar`, one instance per section, built on first
  show and kept until weft quits, so a selection survives switching and hiding.
  The panels keep their own border, title and footer. The sidebar tree has its
  own `treeOpen`, so the `F` overlay and the sidebar never share an expansion map.
- **Fixed width.** `sidebarBoxWidth` 50 (the calendar box, the widest panel
  minimum), `sidebarPanelWidth` 54 (what `SetSize` gets: each panel keeps a
  4-cell margin), `sidebarCols` 51 (box plus one blank gutter column),
  `minPageWidth` 60, `sidebarMinWidth` 111.
- **Height.** The column has `max(1, height-2)` rows: the header row, then the
  panel's lines. A taller panel is cut at the bottom, a shorter one padded with
  blank rows.
- **Frame.** The editor and the overlays still draw over the whole screen, and
  prompts are spliced over the bottom of whatever is drawn. The read view with
  the sidebar is the column on the left of each page row, then the status bar at
  full width.
- **One page width.** `App.pageWidth()` is `width - sidebarCols` while the
  sidebar is shown, else `width`. Every place that sizes the page uses it.
  Overlays, prompts, the editor and the status bar keep `width`.
- **Reading position.** Show and hide, including hide by resize, go through
  `resizePageKeepingPlace`: the reading anchor is taken, the page is resized,
  the anchor is placed again. The cursor stays on its source line and on screen.
  A plain terminal resize keeps its old path.
- **Keys.** Six `readActions` entries in the help group `Sidebar`, between
  Maintain and Help / quit, all in the palette: `1 sidebar: page tree`,
  `2 sidebar: backlinks of this page`, `3 sidebar: todos`, `4 sidebar: agenda`,
  `5 sidebar: calendar`, `| show / hide the sidebar`. They have `sidebar: true`
  ("also runs while the sidebar has focus"). No title is longer than 34
  characters.
- **Show, switch, hide.** Showing builds the panel if needed, sets the section,
  focuses it and resizes the page. With the sidebar shown a digit switches
  section and focuses. `|` hides when shown, and shows `side.section` (the tree
  the first time) when hidden. On a terminal narrower than `sidebarMinWidth`
  showing sets the hint `sidebar needs 111+ columns` and changes nothing. The
  `WindowSizeMsg` handler hides the sidebar when it is shown and the width drops
  below 111; nothing reopens it.
- **Focus routing.** In `App.update`, after the overlay block and before
  `readKeys`, while `sidebarFocused()`: `ctrl+c` quits; a key whose action has
  `sidebar: true` runs; every other key goes to `sidebarKey`, which calls the
  panel's `Update`. A cancel result only moves focus to the page. `MarkTask`
  goes through `blockIfSyncing` then `markTask`, `Capture` calls `openCapture`,
  and the navigation results go to `openResult`, shared with the overlay path,
  with focus staying put. Keys not flagged `sidebar` (`R`, `?`, `:`) go to the
  panel, not the read view.
- **Feedback goes through `keyPanel`.** `keyPanel()` is `a.active` if set, else
  the focused sidebar panel, else nil. `blockIfSyncing`, `markTask` and
  `submitCapture` use it. `linkify` keeps `a.active`, since linkify is overlay
  only.
- **Done rows.** Leaving the todos or agenda section (switching away or hiding,
  by key or by a narrowing resize) calls `clearDone()` on that panel. A refresh
  keeps struck rows.
- **One refresh funnel.** `App.Update` wraps `update` and then calls
  `syncSidebar`. It returns early unless the sidebar is shown and a page and
  index exist. If the index changed it refreshes every built panel and records
  the index and page. Otherwise, if the page changed, it retargets backlinks.
  Navigation has no single function (`historyBack` and `historyForward` skip
  `pushHistory`; rename, the picker, journal steps and reindexes all change the
  page or index), so the wrapper is the one place that sees every change. Hidden
  panels catch up on the next show. Refresh never touches `errMsg`.
- **Identity across a refresh.** Tree: `TreeNode.Path`. Backlinks: `(FromPage,
  LineNumber)` for the same target, the first row for a new target. Todos and
  agenda: `taskKey{page, line}`. Calendar: the cursor date. If the item is gone
  the old row index is clamped to the new rows, the nearest remaining row. The
  agenda and calendar refresh on `nowFunc()`, today's date.
- **Backlinks are linked only.** The sidebar uses `linkedRefs` from the index and
  never runs ripgrep, so a refresh stays in memory.
- **Header.** Row 0, `sidebarBoxWidth` wide plus the gutter. Focused: selection
  style over ` ▶ 3 Todos · esc: page`. Unfocused: faint over
  `   3 Todos · 3: focus`. Titles: `Page tree`, `Backlinks`, `Todos`, `Agenda`,
  `Calendar`. The `▶` survives `NO_COLOR`.
- **Editor.** No editor change. `frame()` draws the editor first, so the sidebar
  is not drawn while it is open, and the editor keeps the full width. The page
  underneath stays at `pageWidth()`, so the editor opens on the line under the
  read cursor at the same screen row, and the exit puts the line back on its
  screen row on the narrow page. Focus is on the page whenever `e` can run, so
  it is on the page after the exit. A widen-then-narrow hold was considered and
  rejected: the anchor is line-based and lands on the same line and screen row,
  and the hold needs more state.
- **Session only.** No sidebar state is written anywhere.

## Traps

- A new page-sizing site must use `pageWidth()`, not `a.width`.
- A result kind a sidebar panel starts emitting needs a case in `sidebarKey`;
  the default case only handles the navigation kinds.
- Panel feedback goes through `keyPanel`, not `a.active`, or it vanishes while
  the sidebar has focus.
- Refresh must stay in memory. Running ripgrep in `syncSidebar` would block the
  UI on every reindex and page change.
- Help titles must stay short: the Sidebar group sits in the right column at
  130 wide, and a long title drops help to one column.
- The panels keep their own footers. Backlinks says `b or esc close`, which in
  the sidebar moves focus to the page.
- A toggle between navigations can land `[`/`]` a few rows off, as a terminal
  resize does today.
- On a terminal shorter than about 22 rows a panel loses its bottom rows.
