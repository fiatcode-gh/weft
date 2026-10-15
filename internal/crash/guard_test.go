package crash

import (
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

const panicText = "guard fixture boom"

type armMsg struct{}

func updatePanics() { panic(panicText) }
func viewPanics()   { panic(panicText) }
func initPanics()   { panic(panicText) }

func panicCmd() tea.Msg { panic(panicText) }
func nopCmd() tea.Msg   { return nil }

// fixture panics at where once armMsg arrives (or from Init for "init").
type fixture struct {
	where string
	armed bool
}

func (f *fixture) Init() tea.Cmd {
	if f.where == "init" {
		initPanics()
	}
	return tea.Tick(150*time.Millisecond, func(time.Time) tea.Msg { return armMsg{} })
}

func (f *fixture) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if _, ok := msg.(armMsg); !ok {
		return f, nil
	}
	switch f.where {
	case "update":
		updatePanics()
	case "view":
		f.armed = true
	case "cmd":
		return f, panicCmd
	case "batch":
		return f, tea.Batch(nopCmd, panicCmd)
	}
	return f, nil
}

func (f *fixture) View() tea.View {
	if f.armed && f.where == "view" {
		viewPanics()
	}
	v := tea.NewView("fixture")
	v.AltScreen = true
	return v
}

// runGuarded runs m under Guard and returns what the run left behind. It
// reads stderr and output right after Run returns, which proves the panic
// print is synchronous.
func runGuarded(t *testing.T, m tea.Model) (rec *Recorder, stderr, out string, err error) {
	t.Helper()
	dir := t.TempDir()
	errFile, ferr := os.Create(dir + "/stderr")
	if ferr != nil {
		t.Fatal(ferr)
	}
	outFile, ferr := os.Create(dir + "/out")
	if ferr != nil {
		t.Fatal(ferr)
	}
	old := os.Stderr
	os.Stderr = errFile
	defer func() { os.Stderr = old }()

	rec = &Recorder{}
	p := tea.NewProgram(Guard(m, rec),
		tea.WithOutput(outFile),
		tea.WithInput(strings.NewReader("")),
		tea.WithEnvironment([]string{"TERM=xterm-256color"}),
		tea.WithWindowSize(80, 24),
	)
	_, err = p.Run()

	os.Stderr = old
	e, _ := os.ReadFile(errFile.Name())
	o, _ := os.ReadFile(outFile.Name())
	return rec, string(e), string(o), err
}

func TestGuardRecordsPanics(t *testing.T) {
	for _, tc := range []struct {
		where string
		fn    string
	}{
		{"update", "updatePanics"},
		{"view", "viewPanics"},
		{"cmd", "panicCmd"},
		{"batch", "panicCmd"},
		{"init", "initPanics"},
	} {
		t.Run(tc.where, func(t *testing.T) {
			rec, stderr, out, err := runGuarded(t, &fixture{where: tc.where})
			if !errors.Is(err, tea.ErrProgramPanic) {
				t.Fatalf("err = %v, want ErrProgramPanic", err)
			}
			r := rec.Report()
			if r == nil {
				t.Fatal("no report recorded")
			}
			if r.Value != panicText {
				t.Errorf("Value = %q, want %q", r.Value, panicText)
			}
			found := false
			for _, f := range r.frames {
				if f.Function == tc.fn {
					found = true
				}
			}
			if !found {
				t.Errorf("frames lack %s: %+v", tc.fn, r.frames)
			}
			for _, want := range []string{"Caught panic:", panicText, tc.fn} {
				if !strings.Contains(stderr, want) {
					t.Errorf("stderr lacks %q:\n%s", want, stderr)
				}
			}
			if tc.where == "init" {
				return
			}
			on := strings.LastIndex(out, "\x1b[?1049h")
			if on < 0 {
				t.Fatalf("no alt-screen enter in output %q", out)
			}
			if !strings.Contains(out[on:], "\x1b[?1049l") {
				t.Errorf("terminal not restored after last alt-screen enter: %q", out)
			}
		})
	}
}

func TestGuardFirstPanicWins(t *testing.T) {
	rec, _, _, _ := runGuarded(t, &fixture{where: "batch"})
	first := rec.Report()
	if first == nil {
		t.Fatal("no report")
	}
	if rec.Report() != first {
		t.Error("Report changed between calls")
	}
	rec.record(Capture("later"))
	if rec.Report() != first {
		t.Error("record replaced the first report")
	}
}

// twoPanics returns a batch whose children both panic with distinct values.
type twoPanics struct{ fixture }

func (m *twoPanics) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if _, ok := msg.(armMsg); ok {
		return m, tea.Batch(
			func() tea.Msg { panic("one") },
			func() tea.Msg { panic("two") },
		)
	}
	return m, nil
}

func TestGuardTwoBatchPanicsKeepOne(t *testing.T) {
	rec, _, _, _ := runGuarded(t, &twoPanics{})
	r := rec.Report()
	if r == nil || (r.Value != "one" && r.Value != "two") {
		t.Fatalf("report = %+v, want one of the two panics", r)
	}
	if rec.Report() != r {
		t.Error("Report changed between calls")
	}
}

// quitter counts updates and quits after the tick.
type quitter struct{ updates int }

func (q *quitter) Init() tea.Cmd {
	return tea.Tick(20*time.Millisecond, func(time.Time) tea.Msg { return armMsg{} })
}

func (q *quitter) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if _, ok := msg.(armMsg); ok {
		q.updates++
		return q, tea.Quit
	}
	return q, nil
}

func (q *quitter) View() tea.View { return tea.NewView("q") }

func TestGuardPassesThrough(t *testing.T) {
	q := &quitter{}
	rec := &Recorder{}
	g := Guard(q, rec)
	next, _ := g.Update(armMsg{})
	if next != g {
		t.Error("Update did not return the guard")
	}
	if q.updates != 1 {
		t.Errorf("inner updates = %d, want 1", q.updates)
	}

	q2 := &quitter{}
	rec, _, _, err := runGuarded(t, q2)
	if err != nil {
		t.Fatalf("Run err = %v", err)
	}
	if rec.Report() != nil {
		t.Errorf("unexpected report %+v", rec.Report())
	}
	if q2.updates != 1 {
		t.Errorf("inner updates = %d, want 1", q2.updates)
	}
}
