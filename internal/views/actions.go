package views

import tea "charm.land/bubbletea/v2"

// Help group titles of the read-view actions.
const (
	groupBrowse   = "Browse"
	groupLinks    = "Links"
	groupHistory  = "History"
	groupOpen     = "Open"
	groupSync     = "Sync"
	groupEdit     = "Edit"
	groupMaintain = "Maintain"
	groupSidebar  = "Sidebar"
	groupHelp     = "Help / quit"
)

// action is one read-view command: the keys that run it and how help shows it.
type action struct {
	// keys are the KeyPressMsg.String() values that run it; none for a
	// help-only row.
	keys []string
	// key is the key as help and the palette show it; "" shares the help row
	// of the entry above.
	key string
	// title is the help description and palette title (one string, so they
	// cannot differ).
	title string
	// group is the help group title (a group* constant).
	group string
	// palette lists the action in the command palette.
	palette bool
	// sidebar also runs while the sidebar has focus.
	sidebar bool
	// run performs the action on the read view.
	run func(a *App) tea.Cmd
}

// readActions is every read-view key, in help order.
var readActions []action

// readKeys maps a key string to its action; App.Update dispatches through it.
var readKeys map[string]*action

// paletteActions are the actions the command palette lists, in help order;
// paletteLabels holds their fuzzy-match labels in lockstep.
var (
	paletteActions []*action
	paletteLabels  []string
)

// unlessSyncing refuses run with blockIfSyncing's feedback while a git sync
// rewrites the worktree.
func unlessSyncing(run func(*App) tea.Cmd) func(*App) tea.Cmd {
	return func(a *App) tea.Cmd {
		if cmd, blocked := a.blockIfSyncing(); blocked {
			return cmd
		}
		return run(a)
	}
}

// init assigns the tables in dependency order. They are not package-level
// initializers: once the ":" action exists its run reaches the palette, which
// derives from readActions — an initialization cycle.
func init() {
	readActions = []action{
		{keys: []string{keyJ, keyDown}, key: "j / k", title: "cursor down / up a row", group: groupBrowse,
			run: func(a *App) tea.Cmd { a.page.LineDown(); return nil }},
		{keys: []string{keyK, keyUp}, group: groupBrowse,
			run: func(a *App) tea.Cmd { a.page.LineUp(); return nil }},
		{keys: []string{"ctrl+d"}, key: "ctrl-d", title: "half page down", group: groupBrowse,
			run: func(a *App) tea.Cmd { a.page.HalfPageDown(); return nil }},
		{keys: []string{"ctrl+u"}, key: "ctrl-u", title: "half page up", group: groupBrowse,
			run: func(a *App) tea.Cmd { a.page.HalfPageUp(); return nil }},
		{keys: []string{"g"}, key: "g / G", title: "top / bottom", group: groupBrowse,
			run: func(a *App) tea.Cmd { a.page.GotoTop(); return nil }},
		{keys: []string{"G"}, group: groupBrowse,
			run: func(a *App) tea.Cmd { a.page.GotoBottom(); return nil }},
		{keys: []string{"tab"}, key: "tab", title: "fold / unfold heading or bullet", group: groupBrowse, palette: true,
			run: func(a *App) tea.Cmd {
				if h := a.page.ToggleFold(); h != "" {
					return a.setHint(h)
				}
				return nil
			}},
		{keys: []string{"z"}, key: "z", title: "fold page: bullets → headings → all", group: groupBrowse, palette: true,
			run: func(a *App) tea.Cmd { return a.setHint(a.page.CycleFoldLevel()) }},

		{keys: []string{"n"}, key: "n / N", title: "next / previous link or tag", group: groupLinks,
			run: func(a *App) tea.Cmd { a.page.CycleLink(+1); return nil }},
		{keys: []string{"N"}, group: groupLinks,
			run: func(a *App) tea.Cmd { a.page.CycleLink(-1); return nil }},
		{keys: []string{keyEnter}, key: "enter", title: "follow link ([[Page#Heading]] opens at the heading)", group: groupLinks, palette: true,
			run: func(a *App) tea.Cmd {
				if t := a.page.FollowCursor(); t != "" {
					return a.followLink(t)
				}
				return nil
			}},

		{keys: []string{"["}, key: "[", title: "back", group: groupHistory, palette: true,
			run: func(a *App) tea.Cmd { a.historyBack(); return nil }},
		{keys: []string{"]"}, key: "]", title: "forward", group: groupHistory, palette: true,
			run: func(a *App) tea.Cmd { a.historyForward(); return nil }},

		{keys: []string{"."}, key: ".", title: "today's journal (new: from Journal Template)", group: groupOpen, palette: true,
			run: unlessSyncing((*App).gotoToday)},
		{keys: []string{"<"}, key: "<", title: "previous journal", group: groupOpen, palette: true,
			run: func(a *App) tea.Cmd { return a.stepJournal(-1, "no earlier journal") }},
		{keys: []string{">"}, key: ">", title: "next journal", group: groupOpen, palette: true,
			run: func(a *App) tea.Cmd { return a.stepJournal(+1, "no later journal") }},
		{keys: []string{"C"}, key: "C", title: "calendar — move between journals", group: groupOpen, palette: true,
			run: func(a *App) tea.Cmd {
				a.active = NewCalendar(a.idx, a.page.Page(), a.nowFunc(), a.width, a.height)
				return nil
			}},
		{keys: []string{"O"}, key: "O", title: "on this day: week, month, years ago", group: groupOpen, palette: true,
			run: func(a *App) tea.Cmd {
				a.active = NewOnThisDay(a.idx, a.nowFunc(), a.readSnapshot, a.width, a.height)
				return nil
			}},
		{keys: []string{"ctrl+p"}, key: "ctrl-p", title: "picker — find any page", group: groupOpen, palette: true,
			run: func(a *App) tea.Cmd { a.active = NewPicker(a.idx, a.width, a.height); return nil }},
		{keys: []string{keyShiftF}, key: "F", title: "page tree: folders and namespaces", group: groupOpen, palette: true,
			run: func(a *App) tea.Cmd {
				a.active = NewTree(a.idx, a.page.Page(), a.treeOpen, a.width, a.height)
				return nil
			}},
		{keys: []string{"/"}, key: "/", title: "search the graph (ripgrep)", group: groupOpen, palette: true,
			run: func(a *App) tea.Cmd { a.active = NewSearchView(a.idx, a.width, a.height); return nil }},
		{keys: []string{"b"}, key: "b", title: "backlinks + unlinked refs for the current page", group: groupOpen, palette: true,
			run: func(a *App) tea.Cmd {
				name := a.page.Page()
				a.active = NewBacklinks(a.idx, name, a.unlinkedRefs(name), a.width, a.height)
				return nil
			}},
		{keys: []string{"T"}, key: "T", title: "todos dashboard (x: done / undo)", group: groupOpen, palette: true,
			run: func(a *App) tea.Cmd { a.active = NewTodos(a.idx, a.width, a.height); return nil }},
		{keys: []string{"A"}, key: "A", title: "agenda: overdue, today, next 7 days", group: groupOpen, palette: true,
			run: func(a *App) tea.Cmd {
				a.active = NewAgenda(a.idx, a.nowFunc(), a.width, a.height)
				return nil
			}},

		{keys: []string{"S"}, key: "S", title: "commit, pull & push the graph (git)", group: groupSync, palette: true,
			run: (*App).startSync},

		{keys: []string{"c"}, key: "c", title: "capture a line into today's journal (tab: TODO)", group: groupEdit, palette: true,
			run: func(a *App) tea.Cmd { a.openCapture(); return nil }},
		{keys: []string{keyE}, key: "e", title: "edit in-app at the cursor row", group: groupEdit, palette: true,
			run: unlessSyncing((*App).enterEditor)},
		{keys: []string{keyShiftE}, key: "E", title: "edit current page in $EDITOR", group: groupEdit, palette: true,
			run: unlessSyncing((*App).editCurrent)},

		{keys: []string{"R"}, key: "R", title: "rebuild index", group: groupMaintain, palette: true,
			run: func(a *App) tea.Cmd {
				// Async reindex — the response lands as indexLoadedMsg and
				// rebuilds PageView for the current page. The hint makes a
				// swallowed or racing R visible instead of looking like a
				// silent no-op. Errors surface in loadErr which the splash
				// overlay renders.
				return tea.Batch(a.setHint("⟳ reindexing…"), a.buildIndexCmd())
			}},
		{keys: []string{"f2"}, key: "F2", title: "rename page and its links", group: groupMaintain, palette: true,
			run: (*App).openRename},
		{keys: []string{"1"}, key: "1", title: "sidebar: page tree", group: groupSidebar, palette: true, sidebar: true,
			run: func(a *App) tea.Cmd { return a.showSidebar(sectionTree) }},
		{keys: []string{"2"}, key: "2", title: "sidebar: backlinks of this page", group: groupSidebar, palette: true, sidebar: true,
			run: func(a *App) tea.Cmd { return a.showSidebar(sectionBacklinks) }},
		{keys: []string{"3"}, key: "3", title: "sidebar: todos", group: groupSidebar, palette: true, sidebar: true,
			run: func(a *App) tea.Cmd { return a.showSidebar(sectionTodos) }},
		{keys: []string{"4"}, key: "4", title: "sidebar: agenda", group: groupSidebar, palette: true, sidebar: true,
			run: func(a *App) tea.Cmd { return a.showSidebar(sectionAgenda) }},
		{keys: []string{"5"}, key: "5", title: "sidebar: calendar", group: groupSidebar, palette: true, sidebar: true,
			run: func(a *App) tea.Cmd { return a.showSidebar(sectionCalendar) }},
		{keys: []string{"|"}, key: "|", title: "show / hide the sidebar", group: groupSidebar, palette: true, sidebar: true,
			run: (*App).toggleSidebar},

		{keys: []string{"?"}, key: "?", title: "toggle this help", group: groupHelp, palette: true,
			run: func(a *App) tea.Cmd { a.active = NewHelp(a.version, a.width, a.height); return nil }},
		{keys: []string{":"}, key: ":", title: "command palette", group: groupHelp,
			run: func(a *App) tea.Cmd { a.active = NewPalette(a.width, a.height); return nil }},
		{key: "esc", title: "close overlay", group: groupHelp},
		{keys: []string{keyQ, "ctrl+c"}, key: "q", title: "quit", group: groupHelp, palette: true,
			run: func(*App) tea.Cmd { return tea.Quit }},
	}

	readKeys = make(map[string]*action)
	for i := range readActions {
		for _, k := range readActions[i].keys {
			readKeys[k] = &readActions[i]
		}
	}

	helpGroups = buildHelpGroups(readActions)

	for i := range readActions {
		if act := &readActions[i]; act.palette {
			paletteActions = append(paletteActions, act)
			paletteLabels = append(paletteLabels, act.key+" "+act.title)
		}
	}
}
