// Package edit is the single disk-writing surface in weft. It hands a
// page's .md file off to the user's editor and detects whether the file
// changed on return. No other package in the project writes to disk.
package edit

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
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

// defaultFileMode is the permission a newly-created page file is born with,
// matching EnsureFile's bootstrap mode.
const defaultFileMode os.FileMode = 0o644

// WriteFile writes data to path atomically, creating the parent directory if
// it does not yet exist. It is the second deliberate write path in the project
// (alongside the EnsureFile bootstrap); all disk writes still funnel through
// package edit.
//
// The write goes to a temp file in the destination directory and is moved into
// place with os.Rename, so a crash mid-write leaves the original intact rather
// than truncated — load-bearing now that linkify writes to files other than
// the page being edited. The destination's existing permission bits are
// preserved (a new file gets defaultFileMode), since the temp file is born
// 0o600. "Preserved" means the rwx permission bits only: setuid/setgid/sticky
// are dropped, and owner/group become the weft process's user (the temp file is
// created by this process and renamed over the original) — acceptable for a
// single-user local graph, which is weft's only caller.
//
// path is assumed to be a regular file (the only kind weft's callers pass) —
// renaming over a symlink replaces the link, not its target. No fsync is done:
// rename guarantees atomicity against a process crash, which is the failure
// this protects against; durability against power loss is out of scope for a
// local notes TUI.
func WriteFile(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	mode := defaultFileMode
	if info, err := os.Stat(path); err == nil {
		mode = info.Mode().Perm()
	} else if !os.IsNotExist(err) {
		return err
	}

	tmp, err := os.CreateTemp(dir, ".weft-*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	// On any failure past this point, drop the temp so a botched write
	// never strands a partial file next to the page.
	defer os.Remove(tmpPath)

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpPath, mode); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
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
