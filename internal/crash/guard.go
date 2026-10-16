package crash

import (
	"sync"

	tea "charm.land/bubbletea/v2"
)

// Recorder keeps the first panic a guarded program reports. The zero value is
// ready to use.
type Recorder struct {
	mu  sync.Mutex
	rep *Report
}

// Report returns the first recorded panic, or nil when none.
func (r *Recorder) Report() *Report {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.rep
}

func (r *Recorder) record(rep *Report) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.rep == nil {
		r.rep = rep
	}
}

// panicMsg carries a background command's panic to the UI loop.
type panicMsg struct{ rep *Report }

type guarded struct {
	inner tea.Model
	rec   *Recorder
}

// Guard wraps m so a panic in Init, Update, View or a command is recorded in
// rec before Bubble Tea's own panic handling restores the terminal. Not
// covered: tea.Sequence children and tea.ExecProcess callbacks.
func Guard(m tea.Model, rec *Recorder) tea.Model { return &guarded{m, rec} }

// repanic records a panic and re-panics the same value so Bubble Tea prints
// it. It must be deferred directly so Capture sees the panicking stack.
func (g *guarded) repanic() {
	v := recover()
	if v == nil {
		return
	}
	if _, ok := v.(*Report); !ok {
		g.rec.record(Capture(v))
	}
	panic(v)
}

func (g *guarded) Init() tea.Cmd {
	defer g.repanic()
	return g.wrap(g.inner.Init())
}

func (g *guarded) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	defer g.repanic()
	if pm, ok := msg.(panicMsg); ok {
		panic(pm.rep)
	}
	var cmd tea.Cmd
	g.inner, cmd = g.inner.Update(msg)
	return g, g.wrap(cmd)
}

func (g *guarded) View() tea.View {
	defer g.repanic()
	return g.inner.View()
}

// wrap makes cmd report a panic as a panicMsg instead of crashing its
// goroutine, recursing into batches.
func (g *guarded) wrap(cmd tea.Cmd) tea.Cmd {
	if cmd == nil {
		return nil
	}
	return func() (msg tea.Msg) {
		defer func() {
			if v := recover(); v != nil {
				rep := Capture(v)
				g.rec.record(rep)
				msg = panicMsg{rep}
			}
		}()
		msg = cmd()
		if b, ok := msg.(tea.BatchMsg); ok {
			wrapped := make(tea.BatchMsg, len(b))
			for i, c := range b {
				wrapped[i] = g.wrap(c)
			}
			return wrapped
		}
		return msg
	}
}
