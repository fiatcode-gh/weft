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
	if err := os.WriteFile(filepath.Join(work, "seed.md"), []byte("seed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, work, "add", "-A")
	git(t, work, "commit", "-m", "seed")
	git(t, work, "push", "origin", "main")
	return work
}

func TestRunCleanTreeIsNoOpButPushes(t *testing.T) {
	work := newRepoWithRemote(t)
	res := Run(work, testClock)
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
	work := newRepoWithRemote(t)
	if err := os.WriteFile(filepath.Join(work, "new.md"), []byte("hi\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	res := Run(work, testClock)
	if res.Err != nil {
		t.Fatalf("unexpected err: %v\n%s", res.Err, res.Output)
	}
	if !res.Committed || !res.Pushed {
		t.Fatalf("want committed+pushed, got %+v", res)
	}
	msg := git(t, work, "log", "-1", "--pretty=%s")
	if msg != "sync: 2026-06-17 09:30\n" {
		t.Errorf("commit message = %q", msg)
	}
}

func TestRunPullBringsRemoteCommits(t *testing.T) {
	work := newRepoWithRemote(t)
	// A second clone pushes a commit that `work` doesn't have yet.
	other := filepath.Join(t.TempDir(), "other")
	origin := git(t, work, "remote", "get-url", "origin")
	git(t, filepath.Dir(other), "clone", trim(origin), other)
	if err := os.WriteFile(filepath.Join(other, "remote.md"), []byte("r\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, other, "add", "-A")
	git(t, other, "commit", "-m", "remote change")
	git(t, other, "push", "origin", "main")

	res := Run(work, testClock)
	if res.Err != nil {
		t.Fatalf("unexpected err: %v\n%s", res.Err, res.Output)
	}
	if !res.Pulled {
		t.Errorf("expected Pulled=true when remote advanced")
	}
}

func TestRunConflictHaltsAtPull(t *testing.T) {
	work := newRepoWithRemote(t)
	// Remote edits seed.md...
	other := filepath.Join(t.TempDir(), "other")
	origin := git(t, work, "remote", "get-url", "origin")
	git(t, filepath.Dir(other), "clone", trim(origin), other)
	os.WriteFile(filepath.Join(other, "seed.md"), []byte("remote\n"), 0o644)
	git(t, other, "commit", "-am", "remote edit")
	git(t, other, "push", "origin", "main")
	// ...and work edits the same line, so rebase conflicts.
	os.WriteFile(filepath.Join(work, "seed.md"), []byte("local\n"), 0o644)

	res := Run(work, testClock)
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

func TestRunNonRepoIsPreflightFailure(t *testing.T) {
	res := Run(t.TempDir(), testClock)
	if res.Err == nil || res.Stage != "preflight" {
		t.Fatalf("want preflight failure, got %+v", res)
	}
}

func trim(s string) string { return strings.TrimSpace(s) }
