package views

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"git.fiatcode.dev/fiatcode/peekseq/internal/graph"
)

var (
	statusFaint = lipgloss.NewStyle().Faint(true)
	statusRule  = lipgloss.NewStyle().Faint(true)
	splashBold  = lipgloss.NewStyle().Bold(true)
	splashFaint = lipgloss.NewStyle().Faint(true)
)

type modeT int

const (
	modePage modeT = iota
	modePicker
	modeSearch
	modeBacklinks
	modeTodos
	modeHelp
)

// indexLoadedMsg carries the result of an asynchronous graph.BuildIndex run.
type indexLoadedMsg struct {
	idx *graph.Index
	err error
}

type App struct {
	graphPath string

	idx     *graph.Index
	loadErr error
	page    *PageView

	picker    *Picker
	search    *SearchView
	backlinks *Backlinks
	todos     *Todos
	help      *Help

	mode   modeT
	width  int
	height int

	// Browser-style page history. hist[histIdx] is the entry currently on
	// screen. histIdx == -1 before the first page is shown.
	hist    []historyEntry
	histIdx int

	// version is the binary version string shown in the help overlay
	// footer. Empty hides the version segment.
	version string
}

type historyEntry struct {
	page   string
	offset int
	cursor int
}

// New returns an App that has not yet built its index. The index is built
// asynchronously in Init so the first frame can render a "loading" splash
// instead of freezing the terminal while a large graph is walked.
func New(graphPath, version string) *App {
	return &App{graphPath: graphPath, mode: modePage, histIdx: -1, version: version}
}

func todayJournalName() string { return time.Now().Format("2006-01-02") }

func (a *App) Init() tea.Cmd { return a.buildIndexCmd() }

func (a *App) buildIndexCmd() tea.Cmd {
	path := a.graphPath
	return func() tea.Msg {
		idx, err := graph.BuildIndex(path)
		return indexLoadedMsg{idx: idx, err: err}
	}
}

// tryInitPage constructs PageView the first time both the index and a real
// terminal size are available. Constructing at the real width avoids the
// "flash from 80 cols to actual width" the previous synchronous path showed.
func (a *App) tryInitPage() {
	if a.page == nil && a.idx != nil && a.loadErr == nil && a.width > 0 {
		name := todayJournalName()
		a.page = NewPageView(a.idx, name, a.width, a.height)
		a.hist = []historyEntry{{page: name, offset: 0, cursor: -1}}
		a.histIdx = 0
	}
}

// navigate switches the page view to name and records the transition in
// history. The departing page's offset/cursor are captured into the current
// history entry, any forward history is truncated, then a fresh entry for
// the destination is pushed and becomes current.
func (a *App) navigate(name string) {
	if a.histIdx >= 0 && a.histIdx < len(a.hist) {
		a.hist[a.histIdx].offset = a.page.Offset()
		a.hist[a.histIdx].cursor = a.page.Cursor()
	}
	a.hist = append(a.hist[:a.histIdx+1], historyEntry{page: name, offset: 0, cursor: -1})
	a.histIdx = len(a.hist) - 1
	a.page.SetPage(name)
}

// historyBack walks one step backward in the history stack, restoring the
// stored offset/cursor for that entry. The departing page's current
// offset/cursor are saved into the current entry so a subsequent forward
// step lands where the user left off. No-op at the start of history.
func (a *App) historyBack() {
	if a.histIdx <= 0 {
		return
	}
	a.hist[a.histIdx].offset = a.page.Offset()
	a.hist[a.histIdx].cursor = a.page.Cursor()
	a.histIdx--
	target := a.hist[a.histIdx]
	a.page.SetPage(target.page)
	a.page.Restore(target.offset, target.cursor)
}

// historyForward walks one step forward in the history stack. Mirrors
// historyBack. No-op at the tail of history.
func (a *App) historyForward() {
	if a.histIdx < 0 || a.histIdx >= len(a.hist)-1 {
		return
	}
	a.hist[a.histIdx].offset = a.page.Offset()
	a.hist[a.histIdx].cursor = a.page.Cursor()
	a.histIdx++
	target := a.hist[a.histIdx]
	a.page.SetPage(target.page)
	a.page.Restore(target.offset, target.cursor)
}

func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m := msg.(type) {
	case indexLoadedMsg:
		if m.err != nil {
			a.loadErr = m.err
			return a, nil
		}
		a.idx = m.idx
		a.loadErr = nil
		if a.page != nil {
			// Refresh path (R): rebuild PageView for the same page so it
			// picks up new links / todos from the rebuilt index.
			a.page = NewPageView(a.idx, a.page.Page(), a.width, a.height)
		} else {
			a.tryInitPage()
		}
		return a, nil
	case searchDoneMsg:
		if a.search != nil {
			a.search.Apply(m)
		}
		return a, nil
	case tea.WindowSizeMsg:
		a.width, a.height = m.Width, m.Height
		if a.page != nil {
			a.page.SetSize(m.Width, m.Height)
		} else {
			a.tryInitPage()
		}
		// Propagate to any active overlay so its scroll-window budget and
		// inner-width calculations track the new terminal size on resize.
		// Help is content-sized and doesn't expose a SetSize.
		if a.picker != nil {
			a.picker.SetSize(m.Width, m.Height)
		}
		if a.search != nil {
			a.search.SetSize(m.Width, m.Height)
		}
		if a.backlinks != nil {
			a.backlinks.SetSize(m.Width, m.Height)
		}
		if a.todos != nil {
			a.todos.SetSize(m.Width, m.Height)
		}
		if a.help != nil {
			a.help.SetSize(m.Width, m.Height)
		}
		return a, nil
	case tea.KeyMsg:
		key := m.String()
		// While loading or in an error state, only quit + retry are honoured.
		if a.page == nil {
			switch key {
			case "q", "ctrl+c":
				return a, tea.Quit
			case "R":
				if a.loadErr != nil {
					a.loadErr = nil
					return a, a.buildIndexCmd()
				}
			}
			return a, nil
		}
		switch a.mode {
		case modePicker:
			sel, accept, cancel := a.picker.Update(key)
			if cancel {
				a.mode = modePage
				a.picker = nil
				return a, nil
			}
			if accept {
				a.navigate(sel)
				a.mode = modePage
				a.picker = nil
			}
			return a, nil
		case modeSearch:
			hit, accept, cancel, cmd := a.search.Update(key, a.idx.GraphPath)
			if cancel {
				a.mode = modePage
				a.search = nil
				return a, nil
			}
			if accept && hit != nil {
				if name := pageNameFromHitPath(a.idx, hit.FilePath); name != "" {
					a.navigate(name)
				}
				a.mode = modePage
				a.search = nil
				return a, nil
			}
			return a, cmd
		case modeBacklinks:
			sel, accept, cancel := a.backlinks.Update(key)
			if cancel {
				a.mode = modePage
				a.backlinks = nil
				return a, nil
			}
			if accept {
				a.navigate(sel)
				a.mode = modePage
				a.backlinks = nil
			}
			return a, nil
		case modeTodos:
			page, accept, cancel := a.todos.Update(key)
			if cancel {
				a.mode = modePage
				a.todos = nil
				return a, nil
			}
			if accept {
				a.navigate(page)
				a.mode = modePage
				a.todos = nil
			}
			return a, nil
		case modeHelp:
			if a.help.Update(key) {
				a.mode = modePage
				a.help = nil
			}
			return a, nil
		case modePage:
			switch key {
			case "q", "ctrl+c":
				return a, tea.Quit
			case "ctrl+p":
				a.picker = NewPicker(a.idx, a.width, a.height)
				a.mode = modePicker
			case "/":
				a.search = NewSearchView(a.idx, a.width, a.height)
				a.mode = modeSearch
			case "b":
				a.backlinks = NewBacklinks(a.idx, a.page.Page(), a.width, a.height)
				a.mode = modeBacklinks
			case "T":
				a.todos = NewTodos(a.idx, a.width, a.height)
				a.mode = modeTodos
			case "?":
				a.help = NewHelp(a.version, a.width)
				a.mode = modeHelp
			case "[":
				a.historyBack()
			case "]":
				a.historyForward()
			case "g":
				a.page.GotoTop()
			case "G":
				a.page.GotoBottom()
			case "R":
				// Async reindex — the response lands as indexLoadedMsg and
				// rebuilds PageView for the current page. Errors surface in
				// loadErr which the splash overlay renders.
				return a, a.buildIndexCmd()
			case "n":
				a.page.CycleLink(+1)
			case "N":
				a.page.CycleLink(-1)
			case "enter":
				if t := a.page.FollowCursor(); t != "" {
					a.navigate(t)
				}
			case "j", "down":
				a.page.LineDown()
			case "k", "up":
				a.page.LineUp()
			case "ctrl+d":
				a.page.HalfPageDown()
			case "ctrl+u":
				a.page.HalfPageUp()
			}
		}
	}
	return a, nil
}

func (a *App) View() string {
	if a.loadErr != nil {
		return splashBold.Render(fmt.Sprintf("peekseq — failed to index %s", a.graphPath)) +
			"\n\n" + a.loadErr.Error() +
			"\n\n" + splashFaint.Render("R to retry · q to quit")
	}
	if a.page == nil {
		return splashBold.Render("peekseq") +
			"\n\n" + splashFaint.Render(fmt.Sprintf("Loading %s ...", a.graphPath)) +
			"\n\n" + splashFaint.Render("q to quit")
	}
	var overlay string
	switch a.mode {
	case modePicker:
		overlay = a.picker.View()
	case modeSearch:
		overlay = a.search.View()
	case modeBacklinks:
		overlay = a.backlinks.View()
	case modeTodos:
		overlay = a.todos.View()
	case modeHelp:
		overlay = a.help.View()
	}
	if overlay != "" {
		return a.centerOverlay(overlay)
	}
	return a.page.View() + "\n" + a.statusBar()
}

// centerOverlay places content in the middle of the terminal. Falls back to
// the raw content when the terminal size hasn't arrived yet.
func (a *App) centerOverlay(content string) string {
	if a.width <= 0 || a.height <= 0 {
		return content
	}
	return lipgloss.Place(a.width, a.height, lipgloss.Center, lipgloss.Center, content)
}

// statusBar renders a one-line bottom bar: page title + meta on the left,
// help hint on the right, separated by enough whitespace to span the
// terminal width. A faint horizontal rule sits above it so the bar reads
// as a distinct strip even on terminals without colour.
//
// When the page title is too long to fit alongside the right segment, the
// left segment is truncated with an ellipsis. Without this clamp the bar
// overflowed the terminal width and wrapped onto a second line.
func (a *App) statusBar() string {
	left := a.page.StatusLine()
	rightText := "? help"
	if ind := a.page.ScrollIndicator(); ind != "" {
		rightText = ind + "  " + rightText
	}
	right := statusFaint.Render(rightText)
	width := a.width
	if width <= 0 {
		width = lipgloss.Width(left) + 2 + lipgloss.Width(right)
	}
	rightW := lipgloss.Width(right)
	// Reserve at least one space between left and right.
	leftBudget := width - rightW - 1
	if leftBudget < 1 {
		leftBudget = 1
	}
	left = clamp(left, leftBudget)
	rule := statusRule.Render(strings.Repeat("─", width))
	gap := width - lipgloss.Width(left) - rightW
	if gap < 1 {
		gap = 1
	}
	return rule + "\n" + left + strings.Repeat(" ", gap) + right
}

func pageNameFromHitPath(idx *graph.Index, abs string) string {
	for _, p := range idx.Pages {
		if p.Path == abs {
			return p.Name
		}
	}
	return ""
}
