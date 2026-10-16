package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	tea "charm.land/bubbletea/v2"

	"github.com/fiatcode-gh/weft/v3/internal/crash"
	"github.com/fiatcode-gh/weft/v3/internal/userconfig"
)

// crashDSN is the crash-report endpoint. Only release builds set it:
// -ldflags "-X main.crashDSN=…" from the SENTRY_DSN repository secret.
// Empty or unparsable means reporting is off.
var crashDSN string

// reporter turns recorded panics into a printed stack and, when the build has
// a DSN and the user consented, one crash report sent after the terminal is
// restored.
type reporter struct {
	dsn     crash.DSN
	on      bool // the build has a usable DSN
	version string
	stderr  io.Writer
	silent  bool   // inside muted: weft doctor and first run report nothing
	graph   string // scrubbed from the panic value
	consent bool
	known   bool // consent was set from the loaded user config
}

func newReporter(dsn, version string, stderr io.Writer) *reporter {
	r := &reporter{version: version, stderr: stderr}
	d, err := crash.ParseDSN(dsn)
	if err == nil {
		r.dsn, r.on = d, true
	}
	return r
}

func (r *reporter) hasDSN() bool         { return r.on }
func (r *reporter) setGraph(path string) { r.graph = absGraph(path) }

// muted runs f, the weft doctor and first-run phases, which report nothing.
// A panic in f skips the reset and stays silent; one after f returns reports.
func (r *reporter) muted(f func()) {
	r.silent = true
	f()
	r.silent = false
}

func (r *reporter) setConsent(on bool) {
	r.consent, r.known = on, true
}

// absGraph is the graph path in the form crash.Scrub matches: absolute and
// clean. It is empty when the working directory can't be resolved, which
// Scrub ignores.
func absGraph(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		return ""
	}
	return filepath.Clean(abs)
}

// send posts rep once when reporting is on and consented. A failure prints one
// line; success prints nothing.
func (r *reporter) send(rep *crash.Report) {
	if r.silent || !r.on {
		return
	}
	consent := r.consent
	if !r.known {
		// A panic before the user config was loaded: read it, read-only.
		cfg, _, err := userconfig.Load()
		consent = err == nil && cfg.CrashReports
	}
	if !consent {
		return
	}
	home, _ := os.UserHomeDir()
	if err := crash.Send(r.dsn, rep, r.version, crash.Scrub{Home: home, Graph: r.graph}); err != nil {
		fmt.Fprintf(r.stderr, "weft: crash report not sent: %v\n", err)
	}
}

// runUI runs m under crash.Guard and returns the process exit code. Bubble
// Tea restores the terminal before Run returns, so the report is sent after.
func (r *reporter) runUI(m tea.Model, opts ...tea.ProgramOption) int {
	var rec crash.Recorder
	_, err := tea.NewProgram(crash.Guard(m, &rec), opts...).Run()
	rep := rec.Report()
	if rep != nil {
		if !errors.Is(err, tea.ErrProgramPanic) {
			fmt.Fprintf(r.stderr, "panic: %s\n\n%s", rep.Value, rep.Stack)
		}
		r.send(rep)
	}
	if err != nil {
		fmt.Fprintf(r.stderr, "weft: %v\n", err)
		return 1
	}
	if rep != nil {
		return 1
	}
	return 0
}

// recoverMain handles a panic outside the UI loop; defer it first in main.
func (r *reporter) recoverMain() {
	v := recover()
	if v == nil {
		return
	}
	rep := crash.Capture(v)
	fmt.Fprintf(r.stderr, "panic: %s\n\n%s", rep.Value, rep.Stack)
	r.send(rep)
	os.Exit(2)
}
