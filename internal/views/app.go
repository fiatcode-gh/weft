package views

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"git.fiatcode.dev/fiatcode/peekseq/internal/edit"
	"git.fiatcode.dev/fiatcode/peekseq/internal/graph"
)

// indexLoadedMsg carries the result of an asynchronous graph.BuildIndex run.
type indexLoadedMsg struct {
	idx *graph.Index
	err error
}

// editorExitedMsg is delivered when the child editor process returns.
// path is the file we handed to the editor; t0 is the pre-edit mtime
// snapshot (zero if the file did not exist before EnsureFile ran).
// err is non-nil when the editor exited non-zero or failed to launch.
type editorExitedMsg struct {
	path string
	t0   time.Time
	err  error
}

// hintTTL is how long a status-bar hint stays before fading on its own.
const hintTTL = 3 * time.Second

// hintExpireMsg is delivered by the timer started in setHint. The handler
// ignores it when gen doesn't match the current hintGen — i.e. when a newer
// hint or keystroke has superseded the one that scheduled this tick.
type hintExpireMsg struct{ gen int }

type App struct {
	graphPath string

	idx     *graph.Index
	loadErr error
	page    *PageView

	// active is the overlay layered over the page, or nil when the page has
	// focus. Set when an open-overlay key is pressed; cleared on Accept/Cancel.
	active Overlay

	width  int
	height int

	// nowFunc returns "now" for today-journal resolution. Defaults to
	// time.Now; tests inject a fixed clock.
	nowFunc func() time.Time

	// hint is a transient right-side status replacement. Set via setHint
	// (which schedules a hintTTL tick) and cleared either at the top of the
	// next tea.KeyMsg or by a matching hintExpireMsg. hintGen is bumped each
	// time a hint is set so stale ticks ignore themselves.
	hint    string
	hintGen int

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
	// line is a 1-based source-line deep-link target recorded when the
	// entry was created via navigateAt (e.g. picking a TODO from the
	// dashboard). 0 means "no deep-link target" — Restore ignores it
	// and the page stays at its stored offset.
	line int
}

// New returns an App that has not yet built its index. The index is built
// asynchronously in Init so the first frame can render a "loading" splash
// instead of freezing the terminal while a large graph is walked.
func New(graphPath, version string) *App {
	return &App{
		graphPath: graphPath,
		histIdx:   -1,
		version:   version,
		nowFunc:   time.Now,
	}
}

func (a *App) todayJournalName() string { return a.nowFunc().Format("2006-01-02") }

// setHint stores s as the active status-bar hint, bumps hintGen, and returns
// a tea.Cmd that delivers a hintExpireMsg after hintTTL. The handler clears
// the hint only when the message's gen still matches hintGen — keystrokes or
// follow-up hints between now and the tick make the message a no-op.
func (a *App) setHint(s string) tea.Cmd {
	a.hint = s
	a.hintGen++
	gen := a.hintGen
	return tea.Tick(hintTTL, func(time.Time) tea.Msg { return hintExpireMsg{gen: gen} })
}

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
		name := a.todayJournalName()
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
	a.navigateAt(name, 0)
}

// navigateAt is navigate plus a deep-link target. When targetLine > 0, the
// new history entry stores it as the restore target and the page is
// scrolled to that line on first display. Used by the Todos dashboard so
// pressing Enter on a bullet lands the user on its line, not the page top.
// Restore ignores the line — it stays a one-shot jump applied at SetPage
// time, not a property the user can rewind into.
func (a *App) navigateAt(name string, targetLine int) {
	if a.histIdx >= 0 && a.histIdx < len(a.hist) {
		a.hist[a.histIdx].offset = a.page.Offset()
		a.hist[a.histIdx].cursor = a.page.Cursor()
	}
	a.hist = append(a.hist[:a.histIdx+1], historyEntry{
		page:   name,
		offset: 0,
		cursor: -1,
		line:   targetLine,
	})
	a.histIdx = len(a.hist) - 1
	a.page.SetPage(name)
	if targetLine > 0 {
		a.page.ScrollToLine(targetLine)
	}
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

// editCurrent snapshots the current page's file mtime, ensures the
// file exists (creating an empty one for today's journal if needed),
// resolves the user's editor, and returns a tea.ExecProcess cmd that
// hands the file off. The child editor's exit yields an
// editorExitedMsg, which the Update case below mtime-gates against a
// reindex.
func (a *App) editCurrent() tea.Cmd {
	page := a.page.Page()
	meta, ok := a.idx.ByName[page]
	if !ok {
		return a.setHint("page not in index: " + page)
	}
	path := meta.Path

	t0, statErr := edit.SnapshotMtime(path)
	if statErr != nil && !os.IsNotExist(statErr) {
		return a.setHint("cannot stat: " + statErr.Error())
	}

	if t0.IsZero() {
		// The file is absent on disk. The `.` handler already creates
		// and reindexes when this is today's journal, so by the time
		// the user reaches `e` the file normally exists. This branch
		// catches the race where a file was deleted between `.` and
		// `e` (or an external tool removed it) and recreates the
		// empty stub so the editor can open it.
		if _, err := edit.EnsureFile(path); err != nil {
			return a.setHint("cannot create journal: " + err.Error())
		}
	}

	resolved, err := edit.Resolve(
		edit.Env{Visual: os.Getenv("VISUAL"), Editor: os.Getenv("EDITOR")},
		exec.LookPath,
	)
	if err != nil {
		return a.setHint("cannot resolve editor: " + err.Error())
	}

	return tea.ExecProcess(exec.Command(resolved.Binary, path), func(cmdErr error) tea.Msg {
		return editorExitedMsg{path: path, t0: t0, err: cmdErr}
	})
}

// journalNeighbor returns the closest existing journal in a.idx.Journals in
// direction dir (-1 prev, +1 next) given that current is a journal-shaped
// name (YYYY-MM-DD).
//
// When current is in idx.Journals the neighbour is the immediate sibling.
// When current is journal-shaped but absent (e.g. phantom-today: peekseq
// opens on today's date but the file isn't on disk yet), the insertion
// point in the sorted slice is used — dir=-1 returns the closest earlier
// existing journal, dir=+1 the closest later one. Returns ok=false when
// current isn't a journal-shaped name or when the chosen direction would
// fall off the ends of the list.
func (a *App) journalNeighbor(current string, dir int) (string, bool) {
	if !graph.IsJournalPageName(current) {
		return "", false
	}
	js := a.idx.Journals
	i := sort.SearchStrings(js, current)
	var j int
	if i < len(js) && js[i] == current {
		j = i + dir
	} else if dir < 0 {
		j = i - 1
	} else {
		j = i
	}
	if j < 0 || j >= len(js) {
		return "", false
	}
	return js[j], true
}

func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m := msg.(type) {
	case indexLoadedMsg:
		if m.err != nil {
			if a.page != nil {
				// Mid-session reindex failed — keep the old index and
				// tell the user via a hint. The working page stays on
				// screen; the boot path (a.page == nil) still surfaces
				// the splash so the user can retry.
				return a, a.setHint("reindex failed: " + m.err.Error())
			}
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
		if s, ok := a.active.(*SearchView); ok {
			s.Apply(m)
		}
		return a, nil
	case editorExitedMsg:
		if m.err != nil {
			return a, a.setHint("editor exited: " + m.err.Error())
		}
		info, err := os.Stat(m.path)
		if err != nil {
			if os.IsNotExist(err) {
				return a, nil
			}
			return a, a.setHint("cannot stat: " + err.Error())
		}
		if info.ModTime().Equal(m.t0) {
			return a, nil
		}
		return a, a.buildIndexCmd()
	case hintExpireMsg:
		if m.gen == a.hintGen {
			a.hint = ""
		}
		return a, nil
	case tea.WindowSizeMsg:
		a.width, a.height = m.Width, m.Height
		if a.page != nil {
			a.page.SetSize(m.Width, m.Height)
		} else {
			a.tryInitPage()
		}
		// Propagate the new size to any open overlay so its scroll-window
		// budget and inner-width tracking stay correct on resize.
		if a.active != nil {
			a.active.SetSize(m.Width, m.Height)
		}
		return a, nil
	case tea.KeyMsg:
		key := m.String()
		a.hint = ""
		// While loading or in an error state, only quit + retry are honoured.
		if a.page == nil {
			switch key {
			case keyQ, "ctrl+c":
				return a, tea.Quit
			case "R":
				if a.loadErr != nil {
					a.loadErr = nil
					return a, a.buildIndexCmd()
				}
			}
			return a, nil
		}
		// An open overlay swallows all keys until it accepts or cancels.
		if a.active != nil {
			res := a.active.Update(key)
			if res.Cancel {
				a.active = nil
				return a, res.Cmd
			}
			if res.Accept {
				if res.Selected != "" {
					a.navigateAt(res.Selected, res.Line)
				}
				a.active = nil
			}
			return a, res.Cmd
		}
		switch key {
		case keyQ, "ctrl+c":
			return a, tea.Quit
		case "ctrl+p":
			a.active = NewPicker(a.idx, a.width, a.height)
		case "/":
			a.active = NewSearchView(a.idx, a.width, a.height)
		case "b":
			a.active = NewBacklinks(a.idx, a.page.Page(), a.width, a.height)
		case "T":
			a.active = NewTodos(a.idx, a.width, a.height)
		case "?":
			a.active = NewHelp(a.version, a.width)
		case "[":
			a.historyBack()
		case "]":
			a.historyForward()
		case ".":
			today := a.todayJournalName()
			if _, ok := a.idx.ByName[today]; !ok {
				// Today's journal file is missing on disk. Create
				// it (via the same internal/edit hook that `e` uses)
				// and rebuild the index synchronously so the
				// navigate below lands on a now-existing journal.
				// BuildIndex is fast on small graphs and matches the
				// reindex shape used by the `R` key.
				journalPath := filepath.Join(a.graphPath, "journals", graph.FilenameFromPageName(today))
				if _, err := edit.EnsureFile(journalPath); err != nil {
					return a, a.setHint("cannot create journal: " + err.Error())
				}
				idx, err := graph.BuildIndex(a.graphPath)
				if err != nil {
					return a, a.setHint("reindex failed: " + err.Error())
				}
				a.idx = idx
				if a.page != nil {
					a.page = NewPageView(a.idx, a.page.Page(), a.width, a.height)
				}
			}
			if a.page.Page() != today {
				a.navigate(today)
			}
		case "<":
			page := a.page.Page()
			if name, ok := a.journalNeighbor(page, -1); ok {
				a.navigate(name)
			} else if graph.IsJournalPageName(page) {
				return a, a.setHint("no earlier journal")
			}
		case ">":
			page := a.page.Page()
			if name, ok := a.journalNeighbor(page, +1); ok {
				a.navigate(name)
			} else if graph.IsJournalPageName(page) {
				return a, a.setHint("no later journal")
			}
		case "g":
			a.page.GotoTop()
		case "G":
			a.page.GotoBottom()
		case "R":
			// Async reindex — the response lands as indexLoadedMsg and
			// rebuilds PageView for the current page. Errors surface in
			// loadErr which the splash overlay renders.
			return a, a.buildIndexCmd()
		case keyE:
			return a, a.editCurrent()
		case "n":
			a.page.CycleLink(+1)
		case "N":
			a.page.CycleLink(-1)
		case keyEnter:
			if t := a.page.FollowCursor(); t != "" {
				a.navigate(t)
			}
		case keyJ, keyDown:
			a.page.LineDown()
		case keyK, keyUp:
			a.page.LineUp()
		case "ctrl+d":
			a.page.HalfPageDown()
		case "ctrl+u":
			a.page.HalfPageUp()
		}
	}
	return a, nil
}

func (a *App) View() string {
	if a.loadErr != nil {
		return styleTitle.Render(fmt.Sprintf("peekseq — failed to index %s", a.graphPath)) +
			"\n\n" + a.loadErr.Error() +
			"\n\n" + styleFaint.Render("R to retry · q to quit")
	}
	if a.page == nil {
		return styleTitle.Render("peekseq") +
			"\n\n" + styleFaint.Render(fmt.Sprintf("Loading %s ...", a.graphPath)) +
			"\n\n" + styleFaint.Render("q to quit")
	}
	if a.active != nil {
		return a.centerOverlay(a.active.View())
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
	var rightText string
	if a.hint != "" {
		rightText = a.hint
	} else {
		rightText = "? help"
		if ind := a.page.ScrollIndicator(); ind != "" {
			rightText = ind + "  " + rightText
		}
	}
	right := styleFaint.Render(rightText)
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
	rule := styleFaint.Render(strings.Repeat("─", width))
	gap := width - lipgloss.Width(left) - rightW
	if gap < 1 {
		gap = 1
	}
	return rule + "\n" + left + strings.Repeat(" ", gap) + right
}
