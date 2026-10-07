package main

import (
	"bytes"
	"io"
	"os"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/fiatcode-gh/weft/v2/internal/render"
)

// probeSequences are written by Bubble Tea v2.0.10 on its own; weft v2.5
// never sent them. The DECRQM and kitty queries get replies that leak into
// the shell when weft quits first, and the kitty push / modifyOtherKeys
// switch the terminal to an enhanced key encoding the contract keeps off.
// Dropping all of them keeps weft's terminal traffic what it was under
// Bubble Tea v1. editor-core owns revisiting the kitty entries.
var probeSequences = [][]byte{
	[]byte(ansi.RequestModeSynchronizedOutput),
	[]byte(ansi.RequestModeUnicodeCore),
	[]byte(ansi.RequestKittyKeyboard),
	[]byte(ansi.PushKittyKeyboard(1)),
	[]byte(ansi.PopKittyKeyboard(1)),
	[]byte(ansi.SetModifyOtherKeys2),
	[]byte(ansi.ResetModifyOtherKeys),
}

// quietStdout is the program's output: the terminal file minus
// probeSequences. It embeds *os.File so Bubble Tea still finds the TTY
// (size, raw mode). Bubble Tea writes each frame or query in one Write
// (pinned by TestProgramOptionsSilenceTerminalProbes), so a sequence is
// never split across calls. Frames without a probe pass through without
// allocating. WriteString and ReadFrom, which the embedded file would
// otherwise promote and which io.WriteString and io.Copy prefer over Write,
// are routed through Write so no path skips the filter.
type quietStdout struct{ *os.File }

func (q quietStdout) Write(p []byte) (int, error) {
	out := p
	for _, seq := range probeSequences {
		if bytes.Contains(out, seq) {
			out = bytes.ReplaceAll(out, seq, nil)
		}
	}
	if _, err := q.File.Write(out); err != nil {
		return 0, err
	}
	return len(p), nil
}

func (q quietStdout) WriteString(s string) (int, error) { return q.Write([]byte(s)) }

// ReadFrom reads r to the end and writes it in one Write, so a probe
// sequence cannot straddle two chunks and slip past the filter.
func (q quietStdout) ReadFrom(r io.Reader) (int64, error) {
	b, err := io.ReadAll(r)
	if err != nil {
		return 0, err
	}
	n, err := q.Write(b)
	return int64(n), err
}

// programOptions wires weft's Bubble Tea program to out (the terminal).
func programOptions(out *os.File) []tea.ProgramOption {
	return []tea.ProgramOption{
		tea.WithOutput(quietStdout{out}),
		tea.WithColorProfile(render.ColorProfile(out)),
	}
}
