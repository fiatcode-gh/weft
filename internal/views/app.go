package views

import (
	"fmt"
	"log"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/fiatcode/logseq-tui/internal/graph"
)

var (
	footerStyle = lipgloss.NewStyle().Faint(true)
	splashBold  = lipgloss.NewStyle().Bold(true)
	splashFaint = lipgloss.NewStyle().Faint(true)
)

type modeT int

const (
	modePage modeT = iota
	modePalette
	modeSearch
	modeBacklinks
	modeTodos
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

	palette   *Palette
	search    *SearchView
	backlinks *Backlinks
	todos     *Todos

	mode   modeT
	width  int
	height int
}

// New returns an App that has not yet built its index. The index is built
// asynchronously in Init so the first frame can render a "loading" splash
// instead of freezing the terminal while a large graph is walked.
func New(graphPath string) *App {
	return &App{graphPath: graphPath, mode: modePage}
}

func todayJournalName() string { return time.Now().Format("2006-01-02") }

func (a *App) Init() tea.Cmd {
	log.Printf("Init: scheduling buildIndexCmd for %q", a.graphPath)
	return a.buildIndexCmd()
}

func (a *App) buildIndexCmd() tea.Cmd {
	path := a.graphPath
	return func() tea.Msg {
		log.Printf("buildIndexCmd: BuildIndex start for %q", path)
		idx, err := graph.BuildIndex(path)
		if err != nil {
			log.Printf("buildIndexCmd: BuildIndex err=%v", err)
		} else {
			log.Printf("buildIndexCmd: BuildIndex ok, %d pages", len(idx.Pages))
		}
		return indexLoadedMsg{idx: idx, err: err}
	}
}

// tryInitPage constructs PageView the first time both the index and a real
// terminal size are available. Constructing at the real width avoids the
// "flash from 80 cols to actual width" the previous synchronous path showed.
func (a *App) tryInitPage() {
	if a.page == nil && a.idx != nil && a.loadErr == nil && a.width > 0 {
		log.Printf("tryInitPage: building PageView at %dx%d for %q", a.width, a.height, todayJournalName())
		a.page = NewPageView(a.idx, todayJournalName(), a.width, a.height)
		log.Printf("tryInitPage: PageView built")
	} else {
		log.Printf("tryInitPage: not ready (page=%v idx=%v loadErr=%v width=%d)", a.page != nil, a.idx != nil, a.loadErr, a.width)
	}
}

func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m := msg.(type) {
	case indexLoadedMsg:
		log.Printf("Update: indexLoadedMsg err=%v idx=%v width=%d", m.err, m.idx != nil, a.width)
		a.idx = m.idx
		a.loadErr = m.err
		a.tryInitPage()
		return a, nil
	case searchDoneMsg:
		if a.search != nil {
			a.search.Apply(m)
		}
		return a, nil
	case tea.WindowSizeMsg:
		log.Printf("Update: WindowSizeMsg w=%d h=%d idx=%v page=%v", m.Width, m.Height, a.idx != nil, a.page != nil)
		a.width, a.height = m.Width, m.Height
		if a.page != nil {
			a.page.SetSize(m.Width, m.Height)
		} else {
			a.tryInitPage()
		}
		return a, nil
	case tea.KeyMsg:
		key := m.String()
		// While loading or in an error state, only quit is honoured.
		if a.page == nil {
			if key == "q" || key == "ctrl+c" {
				return a, tea.Quit
			}
			return a, nil
		}
		switch a.mode {
		case modePalette:
			sel, accept, cancel := a.palette.Update(key)
			if cancel {
				a.mode = modePage
				a.palette = nil
				return a, nil
			}
			if accept {
				a.page.SetPage(sel)
				a.mode = modePage
				a.palette = nil
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
					a.page.SetPage(name)
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
				a.page.SetPage(sel)
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
				a.page.SetPage(page)
				a.mode = modePage
				a.todos = nil
			}
			return a, nil
		case modePage:
			switch key {
			case "q", "ctrl+c":
				return a, tea.Quit
			case "ctrl+p":
				a.palette = NewPalette(a.idx)
				a.mode = modePalette
			case "/":
				a.search = NewSearchView(a.idx)
				a.mode = modeSearch
			case "b":
				a.backlinks = NewBacklinks(a.idx, a.page.Page())
				a.mode = modeBacklinks
			case "T":
				a.todos = NewTodos(a.idx)
				a.mode = modeTodos
			case "R":
				if idx, err := graph.BuildIndex(a.idx.GraphPath); err == nil {
					a.idx = idx
					a.page = NewPageView(idx, a.page.Page(), a.width, a.height)
				}
			case "n":
				a.page.CycleLink(+1)
			case "N":
				a.page.CycleLink(-1)
			case "enter":
				if t := a.page.FollowCursor(); t != "" {
					a.page.SetPage(t)
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
		return splashBold.Render(fmt.Sprintf("lstui — failed to index %s", a.graphPath)) +
			"\n\n" + a.loadErr.Error() +
			"\n\n" + splashFaint.Render("q to quit")
	}
	if a.page == nil {
		return splashBold.Render("lstui") +
			"\n\n" + splashFaint.Render(fmt.Sprintf("Loading %s ...", a.graphPath)) +
			"\n\n" + splashFaint.Render("q to quit")
	}
	switch a.mode {
	case modePalette:
		return a.palette.View()
	case modeSearch:
		return a.search.View()
	case modeBacklinks:
		return a.backlinks.View()
	case modeTodos:
		return a.todos.View()
	}
	keys := "ctrl-p palette · / search · b backlinks · T todos · R refresh · n/N link · enter follow · q quit"
	return a.page.View() + "\n" + footerStyle.Render(keys)
}

func pageNameFromHitPath(idx *graph.Index, abs string) string {
	for _, p := range idx.Pages {
		if p.Path == abs {
			return p.Name
		}
	}
	return ""
}
