// Package sync performs weft's git synchronization: commit local changes,
// rebase-pull, then push. It is weft's second deliberate disk-mutating
// surface (alongside internal/edit) and shells out to the system `git`.
package sync

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Result reports what a sync did and, on failure, where it stopped.
type Result struct {
	Committed bool   // a commit was created (working tree had changes)
	Pulled    bool   // pull --rebase advanced the working tree
	Pushed    bool   // push succeeded
	Stage     string // failing stage; "" on success
	Output    string // combined output of the failing stage; "" on success
	Err       error  // non-nil on failure
}

// Run performs commit -> pull --rebase -> push against repoDir. now is the
// commit-timestamp source, injected so callers (and tests) stay deterministic.
func Run(repoDir string, now time.Time) Result {
	run := func(args ...string) (string, error) {
		cmd := exec.Command("git", args...)
		cmd.Dir = repoDir
		out, err := cmd.CombinedOutput()
		return string(out), err
	}
	// Preflight: must be inside a git work tree.
	if out, err := run("rev-parse", "--is-inside-work-tree"); err != nil {
		return Result{Stage: "preflight", Output: out, Err: err}
	}

	// Refuse if a rebase or merge is in progress: blindly add+commit would
	// capture conflict-marker-laden files. The user must resolve it in a shell.
	gitDir, err := run("rev-parse", "--git-dir")
	if err != nil {
		return Result{Stage: "preflight", Output: gitDir, Err: err}
	}
	gd := strings.TrimSpace(gitDir)
	if !filepath.IsAbs(gd) {
		gd = filepath.Join(repoDir, gd)
	}
	for _, marker := range []string{"rebase-merge", "rebase-apply", "MERGE_HEAD"} {
		if _, err := os.Stat(filepath.Join(gd, marker)); err == nil {
			return Result{
				Stage:  "rebase-in-progress",
				Output: "a rebase or merge is in progress; resolve it in a shell, then sync again",
				Err:    fmt.Errorf("rebase/merge in progress (%s present)", marker),
			}
		}
	}

	var res Result
	failWith := func(stage, out string, err error) Result {
		res.Stage, res.Output, res.Err = stage, out, err
		return res
	}

	// Commit local changes, if any.
	status, err := run("status", "--porcelain")
	if err != nil {
		return failWith("status", status, err)
	}
	if strings.TrimSpace(status) != "" {
		if out, err := run("add", "-A"); err != nil {
			return failWith("add", out, err)
		}
		msg := fmt.Sprintf("sync: %s", now.Format("2006-01-02 15:04"))
		if out, err := run("commit", "-m", msg); err != nil {
			return failWith("commit", out, err)
		}
		res.Committed = true
	}

	// Pull --rebase; detect whether HEAD advanced.
	before, _ := run("rev-parse", "HEAD")
	if out, err := run("pull", "--rebase"); err != nil {
		return failWith("pull", out, err)
	}
	after, _ := run("rev-parse", "HEAD")
	res.Pulled = strings.TrimSpace(before) != strings.TrimSpace(after)

	// Push.
	if out, err := run("push"); err != nil {
		res.Stage, res.Output, res.Err = "push", out, err
		return res
	}
	res.Pushed = true
	return res
}
