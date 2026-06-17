package sync

import (
	"os/exec"
	"strconv"
	"strings"
)

// WorktreeStatus is a read-only snapshot of where the graph repo stands
// relative to its last commit and its upstream. It is the read sibling of Run.
type WorktreeStatus struct {
	Dirty bool // working tree has uncommitted changes
	Ahead int  // commits on HEAD not yet on the upstream branch (0 if no upstream)
}

// Unsynced reports whether there is local work not yet pushed: either
// uncommitted changes or committed-but-unpushed commits.
func (s WorktreeStatus) Unsynced() bool { return s.Dirty || s.Ahead > 0 }

// Status probes repoDir's git state without mutating anything. It returns an
// error only when repoDir is not inside a git work tree; callers treat that as
// "no indicator" rather than a failure. A missing upstream is not an error —
// Ahead is simply left 0, since there's nothing local to measure against.
func Status(repoDir string) (WorktreeStatus, error) {
	run := func(args ...string) (string, error) {
		cmd := exec.Command("git", args...)
		cmd.Dir = repoDir
		out, err := cmd.CombinedOutput()
		return string(out), err
	}

	if _, err := run("rev-parse", "--is-inside-work-tree"); err != nil {
		return WorktreeStatus{}, err
	}

	var s WorktreeStatus
	if out, err := run("status", "--porcelain"); err == nil {
		s.Dirty = strings.TrimSpace(out) != ""
	}
	// @{u}..HEAD counts commits ahead of the upstream. Errors here (most
	// commonly: no upstream configured) leave Ahead at 0.
	if out, err := run("rev-list", "--count", "@{u}..HEAD"); err == nil {
		if n, convErr := strconv.Atoi(strings.TrimSpace(out)); convErr == nil {
			s.Ahead = n
		}
	}
	return s, nil
}
