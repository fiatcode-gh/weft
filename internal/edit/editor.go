// Package edit is the single disk-writing surface in peekseq. It hands a
// page's .md file off to the user's editor and detects whether the file
// changed on return. No other package in the project writes to disk.
package edit

import (
	"errors"
	"fmt"
	"os"
	"time"
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

// EnsureFile creates an empty file at path with mode 0o644 if it does
// not exist. Returns (true, nil) on create, (false, nil) if the file
// already existed, or (false, err) for any other stat/write failure
// (including the case where path resolves to a directory).
// This is the create-today-journal hook.
func EnsureFile(path string) (created bool, err error) {
	info, err := os.Stat(path)
	if err == nil {
		if info.IsDir() {
			return false, fmt.Errorf("ensure %s: is a directory", path)
		}
		return false, nil
	}
	if !os.IsNotExist(err) {
		return false, err
	}
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		return false, err
	}
	return true, nil
}

// SnapshotMtime returns the file's modification time, or time.Time{}
// (the zero value) if the file does not exist. The zero return is
// load-bearing: t0.IsZero() means the file was absent at snapshot
// time, which tells the caller (the App's editCurrent) that it
// should create the file before handing it to the editor. A non-zero
// t0 means the file was on disk before the user pressed e.
func SnapshotMtime(path string) (time.Time, error) {
	info, err := os.Stat(path)
	if err != nil {
		return time.Time{}, err
	}
	return info.ModTime(), nil
}
