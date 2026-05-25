package views

import (
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/fiatcode/logseq-tui/internal/graph"
)

type modeT int

const (
	modePage modeT = iota
	modePalette
	modeSearch
	modeBacklinks
	modeTodos
)

type App struct {
	idx       *graph.Index
	page      *PageView
	palette   *Palette
	search    *SearchView
	backlinks *Backlinks
	todos     *Todos
	mode      modeT
	width     int
	height    int
}

func New(graphPath string) (*App, error) {
	idx, err := graph.BuildIndex(graphPath)
	if err != nil {
		return nil, fmt.Errorf("index %s: %w", graphPath, err)
	}
	a := &App{idx: idx, mode: modePage}
	a.page = NewPageView(idx, todayJournalName(), 80, 24)
	return a, nil
}

func todayJournalName() string { return time.Now().Format("2006-01-02") }

func (a *App) Init() tea.Cmd { return nil }

func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m := msg.(type) {
	case searchDoneMsg:
		if a.search != nil {
			a.search.Apply(m)
		}
		return a, nil
	case tea.WindowSizeMsg:
		a.width, a.height = m.Width, m.Height
		a.page.SetSize(m.Width, m.Height)
	case tea.KeyMsg:
		key := m.String()
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
	return a.page.View() + "\n[ctrl-p] palette  [/] search  [b] backlinks  [T] todos  [R] refresh  [n/N] link  [enter] follow  [q] quit"
}

func pageNameFromHitPath(idx *graph.Index, abs string) string {
	for _, p := range idx.Pages {
		if p.Path == abs {
			return p.Name
		}
	}
	return ""
}
