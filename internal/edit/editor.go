// Package edit is the single disk-writing surface in peekseq. It hands a
// page's .md file off to the user's editor and detects whether the file
// changed on return. No other package in the project writes to disk.
package edit

import (
	"errors"
)

// Env carries the configuration the editor-resolver reads. Decoupled
// from os.Getenv so tests can inject a fixed env without touching
// process state.
type Env struct {
	Visual string // value of $VISUAL
	Editor string // value of $EDITOR
}

// Resolved is the editor binary to invoke with no args; the file path
// is appended at call time by the App.
type Resolved struct {
	Binary string
}

// errNoEditor is the sentinel returned by Resolve when none of
// $VISUAL, $EDITOR, or /usr/bin/vi resolve to a runnable binary. The
// App surfaces it as a status-bar hint.
var errNoEditor = errors.New("no editor found (set $VISUAL or $EDITOR, or install vi)")

// Resolve picks the first available editor in the standard chain:
// $VISUAL → $EDITOR → /usr/bin/vi. lookPath is injected so tests can
// simulate "set but missing" / "vi missing" without touching PATH.
func Resolve(env Env, lookPath func(string) (string, error)) (Resolved, error) {
	if env.Visual != "" {
		if path, err := lookPath(env.Visual); err == nil {
			return Resolved{Binary: path}, nil
		}
	}
	if env.Editor != "" {
		if path, err := lookPath(env.Editor); err == nil {
			return Resolved{Binary: path}, nil
		}
	}
	if path, err := lookPath("/usr/bin/vi"); err == nil {
		return Resolved{Binary: path}, nil
	}
	return Resolved{}, errNoEditor
}
