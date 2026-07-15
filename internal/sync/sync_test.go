package sync

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var testClock = time.Date(2026, 6, 17, 9, 30, 0, 0, time.UTC)

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	// Deterministic identity + no global config bleed-through.
	cmd.Env = append([]string{
		"GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null",
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t",
	}, "HOME=/dev/null")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return string(out)
}

// newRepoWithRemote returns a work tree wired to a fresh bare origin,
// with one pushed commit on the default branch.
func newRepoWithRemote(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	origin := filepath.Join(root, "origin.git")
	work := filepath.Join(root, "work")
	git(t, root, "init", "--bare", "-b", "main", origin)
	git(t, root, "clone", origin, work)
	// Run()'s own `git commit` inherits the ambient process environment, which
	// on CI has no user identity configured. Give the work tree a local
	// identity so the production commit path never depends on a global config.
	git(t, work, "config", "user.email", "t@t")
	git(t, work, "config", "user.name", "t")
	writeFile(t, work, "seed.md", "seed\n")
	git(t, work, "add", "-A")
	git(t, work, "commit", "-m", "seed")
	git(t, work, "push", "origin", "main")
	return work
}

// cloneSibling makes a second clone of work's origin and returns its path.
// It is the shared arrange step for tests that need a separate party to
// advance the remote behind work's back.
func cloneSibling(t *testing.T, work string) string {
	t.Helper()
	sibling := filepath.Join(t.TempDir(), "other")
	origin := strings.TrimSpace(git(t, work, "remote", "get-url", "origin"))
	git(t, filepath.Dir(sibling), "clone", origin, sibling)
	return sibling
}

func TestRunCleanTreeIsNoOpButPushes(t *testing.T) {
	// arrange
	work := newRepoWithRemote(t)

	// act
	res := Run(work, testClock)

	// assert
	if res.Err != nil {
		t.Fatalf("unexpected err: %v (stage %q)\n%s", res.Err, res.Stage, res.Output)
	}
	if res.Committed {
		t.Errorf("clean tree should not commit")
	}
	if res.Pulled {
		t.Errorf("clean tree with no remote advance should not pull")
	}
	if !res.Pushed {
		t.Errorf("expected push to run")
	}
}

func TestRunDirtyTreeCommitsAndPushes(t *testing.T) {
	// arrange: an uncommitted new file.
	work := newRepoWithRemote(t)
	writeFile(t, work, "new.md", "hi\n")

	// act
	res := Run(work, testClock)

	// assert
	if res.Err != nil {
		t.Fatalf("unexpected err: %v\n%s", res.Err, res.Output)
	}
	if !res.Committed || !res.Pushed {
		t.Fatalf("want committed+pushed, got %+v", res)
	}
	if msg := git(t, work, "log", "-1", "--pretty=%s"); msg != "sync: 2026-06-17 09:30\n" {
		t.Errorf("commit message = %q", msg)
	}
}

func TestRunPullBringsRemoteCommits(t *testing.T) {
	// arrange: a sibling clone pushes a commit that work doesn't have yet.
	work := newRepoWithRemote(t)
	other := cloneSibling(t, work)
	writeFile(t, other, "remote.md", "r\n")
	git(t, other, "add", "-A")
	git(t, other, "commit", "-m", "remote change")
	git(t, other, "push", "origin", "main")

	// act
	res := Run(work, testClock)

	// assert
	if res.Err != nil {
		t.Fatalf("unexpected err: %v\n%s", res.Err, res.Output)
	}
	if !res.Pulled {
		t.Errorf("expected Pulled=true when remote advanced")
	}
}

func TestRunConflictHaltsAtPull(t *testing.T) {
	// arrange: remote and work edit the same line of seed.md, so rebase conflicts.
	work := newRepoWithRemote(t)
	other := cloneSibling(t, work)
	writeFile(t, other, "seed.md", "remote\n")
	git(t, other, "commit", "-am", "remote edit")
	git(t, other, "push", "origin", "main")
	writeFile(t, work, "seed.md", "local\n")

	// act
	res := Run(work, testClock)

	// assert
	if res.Err == nil {
		t.Fatal("expected conflict error")
	}
	if res.Stage != "pull" {
		t.Errorf("stage = %q, want pull", res.Stage)
	}
	if res.Output == "" {
		t.Errorf("expected captured git output on failure")
	}
	if !res.Committed {
		t.Errorf("local commit must survive a failed pull")
	}
}

func TestRunPushRejectionReportsPushStage(t *testing.T) {
	// arrange: a local change to commit, and a bare remote whose pre-receive
	// hook refuses every push outright — the simplest deterministic
	// rejection, needing no sibling clone or non-fast-forward race.
	work := newRepoWithRemote(t)
	writeFile(t, work, "new.md", "hi\n")

	remote := strings.TrimSpace(git(t, work, "remote", "get-url", "origin"))
	hook := filepath.Join(remote, "hooks", "pre-receive")
	if err := os.WriteFile(hook, []byte("#!/bin/sh\necho rejected\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	// act
	res := Run(work, testClock)

	// assert
	if res.Err == nil || res.Stage != "push" {
		t.Fatalf("Stage = %q, Err = %v; want push failure", res.Stage, res.Err)
	}
	if !res.Committed {
		t.Fatal("local commit must be recorded even when push fails")
	}
	if res.Pushed {
		t.Fatal("Pushed must be false on rejection")
	}
	if !strings.Contains(res.Output, "rejected") {
		t.Errorf("expected captured push output to include the hook's rejection message, got %q", res.Output)
	}
}

func TestRunNonRepoIsPreflightFailure(t *testing.T) {
	// arrange: a plain directory that is not a git work tree.
	dir := t.TempDir()

	// act
	res := Run(dir, testClock)

	// assert
	if res.Err == nil || res.Stage != "preflight" {
		t.Fatalf("want preflight failure, got %+v", res)
	}
}

// withShortSyncTimeout forces syncTimeout to d for the duration of the test,
// restoring the original value on cleanup. Shared by tests that need every
// git call bound by an effectively-zero deadline (see Run and Status, which
// both build their runner from this package var).
func withShortSyncTimeout(t *testing.T, d time.Duration) {
	t.Helper()
	orig := syncTimeout
	syncTimeout = d
	t.Cleanup(func() { syncTimeout = orig })
}

func TestRunTimesOutOnSlowGit(t *testing.T) {
	// arrange: a valid repo, but an effectively-zero deadline so the very first
	// git call (the preflight) exceeds it deterministically.
	work := newRepoWithRemote(t)
	withShortSyncTimeout(t, time.Nanosecond)

	// act
	res := Run(work, testClock)

	// assert
	if res.Err == nil {
		t.Fatal("expected a deadline error when git exceeds syncTimeout")
	}
}

func TestRunRefusesWhenRebaseInProgress(t *testing.T) {
	// arrange: drive work into a conflicted (mid-rebase) state exactly like the
	// conflict test, then attempt a second sync.
	work := newRepoWithRemote(t)
	other := cloneSibling(t, work)
	writeFile(t, other, "seed.md", "remote\n")
	git(t, other, "commit", "-am", "remote edit")
	git(t, other, "push", "origin", "main")
	writeFile(t, work, "seed.md", "local\n")

	first := Run(work, testClock)
	if first.Stage != "pull" {
		t.Fatalf("precondition: first sync should halt at pull; got stage %q", first.Stage)
	}

	headBefore := strings.TrimSpace(git(t, work, "rev-parse", "HEAD"))

	// act: second sync while the rebase is still in progress.
	res := Run(work, testClock)

	// assert
	if res.Err == nil {
		t.Fatal("a second sync mid-rebase must fail, not commit conflict markers")
	}
	if res.Stage != "rebase-in-progress" {
		t.Errorf("stage = %q, want rebase-in-progress", res.Stage)
	}
	if res.Committed {
		t.Errorf("must not create a commit while a rebase is in progress")
	}
	if headAfter := strings.TrimSpace(git(t, work, "rev-parse", "HEAD")); headAfter != headBefore {
		t.Errorf("HEAD moved (%s -> %s); a refused sync must not advance the branch", headBefore, headAfter)
	}
}
