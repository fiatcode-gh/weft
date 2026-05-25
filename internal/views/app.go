package views

import (
	"fmt"
	"os"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/fiatcode/logseq-tui/internal/graph"
	"github.com/fiatcode/logseq-tui/internal/render"
)

// App is the top-level Bubble Tea model.
type App struct {
	idx     *graph.Index
	current string // page name currently displayed
	width   int
	height  int
	body    render.Result
	err     error
}

// New builds an App rooted at graphPath. It loads the index synchronously
// because BuildIndex is fast (<100ms for graphs we care about) and a blank
// first frame is worse than a 100ms delay.
func New(graphPath string) (*App, error) {
	idx, err := graph.BuildIndex(graphPath)
	if err != nil {
		return nil, fmt.Errorf("index %s: %w", graphPath, err)
	}
	a := &App{idx: idx, current: todayJournalName()}
	return a, nil
}

func todayJournalName() string {
	return time.Now().Format("2006-01-02")
}

func (a *App) Init() tea.Cmd { return nil }

func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m := msg.(type) {
	case tea.WindowSizeMsg:
		a.width, a.height = m.Width, m.Height
		a.refreshBody()
	case tea.KeyMsg:
		switch m.String() {
		case "q", "ctrl+c":
			return a, tea.Quit
		}
	}
	return a, nil
}

func (a *App) View() string {
	if a.err != nil {
		return fmt.Sprintf("error: %v\n\nq to quit.", a.err)
	}
	header := fmt.Sprintf("lstui — %s\n\n", a.current)
	if a.body.Styled == "" {
		return header + "(no entry yet for this page)\n\nq to quit."
	}
	return header + a.body.Styled + "\n\nq to quit."
}

func (a *App) refreshBody() {
	meta, ok := a.idx.ByName[a.current]
	if !ok {
		a.body = render.Result{}
		return
	}
	bytes, err := os.ReadFile(meta.Path)
	if err != nil {
		a.err = err
		return
	}
	res, err := render.Render(string(bytes), a.width)
	if err != nil {
		a.err = err
		return
	}
	a.body = res
}
