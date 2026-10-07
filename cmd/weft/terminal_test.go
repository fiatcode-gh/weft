package main

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func tempOut(t *testing.T) *os.File {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "out")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { f.Close() })
	return f
}

func readAll(t *testing.T, f *os.File) []byte {
	t.Helper()
	b, err := os.ReadFile(f.Name())
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestQuietStdoutDropsProbeSequences(t *testing.T) {
	f := tempOut(t)
	var in, want strings.Builder
	for i, seq := range probeSequences {
		plain := string(rune('A'+i)) + "-"
		in.WriteString(plain)
		in.Write(seq)
		want.WriteString(plain)
	}
	keep := "\x1b[?1049h\x1b[94mtext\x1b[m"
	in.WriteString(keep)
	want.WriteString(keep)

	n, err := quietStdout{f}.Write([]byte(in.String()))
	if err != nil || n != in.Len() {
		t.Fatalf("Write = (%d, %v), want (%d, nil)", n, err, in.Len())
	}
	if got := string(readAll(t, f)); got != want.String() {
		t.Fatalf("file = %q, want %q", got, want.String())
	}
}

// *os.File also offers WriteString and ReadFrom, which io.WriteString and
// io.Copy prefer over Write; neither may carry a probe past the filter.
func TestQuietStdoutFiltersEveryWritePath(t *testing.T) {
	in := "a-" + string(probeSequences[0]) + "b-" + string(probeSequences[len(probeSequences)-1]) + "\x1b[94mtext\x1b[m"
	want := "a-b-\x1b[94mtext\x1b[m"
	tests := []struct {
		name  string
		write func(q quietStdout) (int64, error)
	}{
		{"io.WriteString", func(q quietStdout) (int64, error) {
			n, err := io.WriteString(q, in)
			return int64(n), err
		}},
		{"io.Copy", func(q quietStdout) (int64, error) {
			// a Reader without WriteTo, so io.Copy goes through q.ReadFrom
			return io.Copy(q, struct{ io.Reader }{strings.NewReader(in)})
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := tempOut(t)
			n, err := tc.write(quietStdout{f})
			if err != nil || n != int64(len(in)) {
				t.Fatalf("wrote (%d, %v), want (%d, nil)", n, err, len(in))
			}
			if got := string(readAll(t, f)); got != want {
				t.Fatalf("file = %q, want %q", got, want)
			}
		})
	}
}

func TestQuietStdoutPlainFrameDoesNotAllocate(t *testing.T) {
	f := tempOut(t)
	frame := []byte(strings.Repeat("\x1b[94mline of text\x1b[m\r\n", 40))
	q := quietStdout{f}
	if allocs := testing.AllocsPerRun(100, func() { _, _ = q.Write(frame) }); allocs != 0 {
		t.Fatalf("allocs = %v, want 0", allocs)
	}
}

type stopMsg struct{}

type stubModel struct{}

func (stubModel) Init() tea.Cmd {
	return tea.Tick(150*time.Millisecond, func(time.Time) tea.Msg { return stopMsg{} })
}

func (m stubModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if _, ok := msg.(stopMsg); ok {
		return m, tea.Quit
	}
	return m, nil
}

func (stubModel) View() tea.View {
	v := tea.NewView("hello")
	v.AltScreen = true
	return v
}

// clipboardModel copies a fixed text to the terminal clipboard on start.
type clipboardModel struct{ stubModel }

func (clipboardModel) Init() tea.Cmd {
	return tea.Batch(tea.SetClipboard("copied text"), stubModel{}.Init())
}

func (m clipboardModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	_, cmd := m.stubModel.Update(msg)
	return m, cmd
}

func runStub(t *testing.T, opts ...tea.ProgramOption) {
	t.Helper()
	runModel(t, stubModel{}, opts...)
}

func runModel(t *testing.T, m tea.Model, opts ...tea.ProgramOption) {
	t.Helper()
	common := []tea.ProgramOption{
		tea.WithInput(strings.NewReader("")),
		tea.WithEnvironment([]string{"TERM=xterm-256color"}),
		tea.WithWindowSize(80, 24),
	}
	if _, err := tea.NewProgram(m, append(opts, common...)...).Run(); err != nil {
		t.Fatal(err)
	}
}

func TestProgramOptionsSilenceTerminalProbes(t *testing.T) {
	raw := tempOut(t)
	runStub(t, tea.WithOutput(raw))
	rawOut := readAll(t, raw)
	for _, seq := range probeSequences {
		if !bytes.Contains(rawOut, seq) {
			t.Fatalf("Bubble Tea no longer writes %q; re-check probeSequences", seq)
		}
	}

	f := tempOut(t)
	runStub(t, programOptions(f)...)
	out := readAll(t, f)
	for _, seq := range probeSequences {
		if bytes.Contains(out, seq) {
			t.Errorf("probe %q reached the terminal", seq)
		}
	}
	if !bytes.Contains(out, []byte("\x1b[?1049h")) {
		t.Errorf("alt-screen entry missing; frames no longer reach the terminal: %q", out)
	}
}

// OSC 52 (what tea.SetClipboard writes) must pass quietStdout: the editor's
// copy and cut reach the terminal clipboard through it.
func TestProgramOptionsPassClipboard(t *testing.T) {
	f := tempOut(t)
	runModel(t, clipboardModel{}, programOptions(f)...)
	want := ansi.SetSystemClipboard("copied text")
	out := readAll(t, f)
	if !bytes.Contains(out, []byte(want)) {
		t.Errorf("OSC 52 %q did not reach the output", want)
	}
	t.Logf("OSC 52 on the wire: %q", want)
}
