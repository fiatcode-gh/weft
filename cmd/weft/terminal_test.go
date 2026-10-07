package main

import (
	"bytes"
	"os"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
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

func runStub(t *testing.T, opts ...tea.ProgramOption) {
	t.Helper()
	common := []tea.ProgramOption{
		tea.WithInput(strings.NewReader("")),
		tea.WithEnvironment([]string{"TERM=xterm-256color"}),
		tea.WithWindowSize(80, 24),
	}
	if _, err := tea.NewProgram(stubModel{}, append(opts, common...)...).Run(); err != nil {
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
