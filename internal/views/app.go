package views

import (
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/fiatcode/logseq-tui/internal/graph"
)

type App struct {
	idx    *graph.Index
	page   *PageView
	width  int
	height int
}

func New(graphPath string) (*App, error) {
	idx, err := graph.BuildIndex(graphPath)
	if err != nil {
		return nil, fmt.Errorf("index %s: %w", graphPath, err)
	}
	a := &App{idx: idx}
	a.page = NewPageView(idx, todayJournalName(), 80, 24)
	return a, nil
}

func todayJournalName() string { return time.Now().Format("2006-01-02") }

func (a *App) Init() tea.Cmd { return nil }

func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m := msg.(type) {
	case tea.WindowSizeMsg:
		a.width, a.height = m.Width, m.Height
		a.page.SetSize(m.Width, m.Height)
	case tea.KeyMsg:
		switch m.String() {
		case "q", "ctrl+c":
			return a, tea.Quit
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
	return a, nil
}

func (a *App) View() string {
	return a.page.View() + "\n[n/N] link  [enter] follow  [j/k] scroll  [q] quit"
}
