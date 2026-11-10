package sync

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// isolateGit blanks every ambient git/ssh setting Run and Status inherit, so
// a test controls exactly which prompt and ssh knobs are in play.
func isolateGit(t *testing.T) {
	t.Helper()
	t.Setenv("GIT_CONFIG_GLOBAL", "/dev/null")
	t.Setenv("GIT_CONFIG_SYSTEM", "/dev/null")
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	// Unset rather than blank: git treats an empty GIT_SSH_COMMAND as a set,
	// empty command.
	for _, k := range []string{"GIT_SSH_COMMAND", "GIT_SSH", "GIT_ASKPASS", "SSH_ASKPASS", "GIT_TERMINAL_PROMPT"} {
		t.Setenv(k, "")
		os.Unsetenv(k)
	}
}

// fakeSSH puts an `ssh` on PATH that appends its arguments to the returned
// log file and then fails, standing in for a real ssh transport.
func fakeSSH(t *testing.T) (argsLog string) {
	t.Helper()
	dir := t.TempDir()
	argsLog = filepath.Join(dir, "args.log")
	script := "#!/bin/sh\necho \"$@\" >> '" + argsLog + "'\nexit 1\n"
	if err := os.WriteFile(filepath.Join(dir, "ssh"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return argsLog
}

func TestRunHungRemoteReturnsShortlyAfterTimeout(t *testing.T) {
	// arrange: an ssh remote whose transport blocks. git is killed at the
	// deadline, but its ssh child used to keep the output pipe open, so Run
	// blocked until that child exited on its own.
	isolateGit(t)
	work := newRepoWithRemote(t)
	git(t, work, "remote", "set-url", "origin", "ssh://example.invalid/repo.git")
	t.Setenv("GIT_SSH_COMMAND", "sh -c 'sleep 15' x")
	withShortSyncTimeout(t, time.Second)

	// act
	start := time.Now()
	res := Run(work, testClock)
	elapsed := time.Since(start)

	// assert
	if res.Err == nil || res.Stage != "pull" {
		t.Fatalf("Stage = %q, Err = %v; want pull failure", res.Stage, res.Err)
	}
	if elapsed > 6*time.Second {
		t.Errorf("Run took %v with a 1s syncTimeout; a hung transport must not outlive the deadline", elapsed)
	}
}

func TestRunAuthRequiredRemoteFailsWithoutPrompting(t *testing.T) {
	// arrange: an HTTP remote that demands credentials.
	isolateGit(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("WWW-Authenticate", `Basic realm="weft"`)
		http.Error(w, "auth required", http.StatusUnauthorized)
	}))
	defer srv.Close()
	work := newRepoWithRemote(t)
	git(t, work, "remote", "set-url", "origin", srv.URL+"/repo.git")

	// act
	start := time.Now()
	res := Run(work, testClock)
	elapsed := time.Since(start)

	// assert: the stage fails with git's own explanation, promptly.
	if res.Err == nil || res.Stage != "pull" {
		t.Fatalf("Stage = %q, Err = %v; want pull failure", res.Stage, res.Err)
	}
	if !strings.Contains(res.Output, "terminal prompts disabled") {
		t.Errorf("git must be told never to prompt; output = %q", res.Output)
	}
	if elapsed > 10*time.Second {
		t.Errorf("Run took %v; an auth failure must not wait for input", elapsed)
	}
}

func TestRunSSHIsBatchModeByDefault(t *testing.T) {
	// arrange: an ssh remote and no user-chosen ssh command anywhere.
	isolateGit(t)
	argsLog := fakeSSH(t)
	work := newRepoWithRemote(t)
	git(t, work, "remote", "set-url", "origin", "ssh://example.invalid/repo.git")

	// act
	res := Run(work, testClock)

	// assert: ssh would otherwise ask for a passphrase or host-key
	// confirmation on a terminal weft does not hand it.
	if res.Err == nil || res.Stage != "pull" {
		t.Fatalf("Stage = %q, Err = %v; want pull failure", res.Stage, res.Err)
	}
	got, err := os.ReadFile(argsLog)
	if err != nil {
		t.Fatalf("ssh was never invoked: %v", err)
	}
	if !strings.Contains(string(got), "BatchMode=yes") {
		t.Errorf("ssh args = %q, want -o BatchMode=yes", got)
	}
}

func TestRunRespectsConfiguredSSHCommand(t *testing.T) {
	// A user who configured their own ssh command owns its options.
	tests := []struct {
		name  string
		setup func(t *testing.T, work string)
	}{
		{"core.sshCommand", func(t *testing.T, work string) {
			git(t, work, "config", "core.sshCommand", "ssh -o Custom=1")
		}},
		{"GIT_SSH_COMMAND", func(t *testing.T, work string) {
			t.Setenv("GIT_SSH_COMMAND", "ssh -o Custom=1")
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// arrange
			isolateGit(t)
			argsLog := fakeSSH(t)
			work := newRepoWithRemote(t)
			git(t, work, "remote", "set-url", "origin", "ssh://example.invalid/repo.git")
			tc.setup(t, work)

			// act
			res := Run(work, testClock)

			// assert
			if res.Err == nil || res.Stage != "pull" {
				t.Fatalf("Stage = %q, Err = %v; want pull failure", res.Stage, res.Err)
			}
			got, err := os.ReadFile(argsLog)
			if err != nil {
				t.Fatalf("ssh was never invoked: %v", err)
			}
			if !strings.Contains(string(got), "Custom=1") {
				t.Fatalf("configured ssh command was not used; args = %q", got)
			}
			if strings.Contains(string(got), "BatchMode") {
				t.Errorf("weft must not override the user's ssh command; args = %q", got)
			}
		})
	}
}

func TestStatusChecksTakeNoOptionalLocks(t *testing.T) {
	// arrange: trace2 records each git invocation's full argv.
	isolateGit(t)
	work := newRepoWithRemote(t)
	writeFile(t, work, "new.md", "hi\n")
	trace := filepath.Join(t.TempDir(), "trace.json")
	t.Setenv("GIT_TRACE2_EVENT", trace)

	// act: both callers that run `git status`.
	if _, err := Status(work); err != nil {
		t.Fatal(err)
	}
	if res := Run(work, testClock); res.Err != nil {
		t.Fatalf("Run: %v\n%s", res.Err, res.Output)
	}

	// assert: a plain `git status` refreshes the index under index.lock and
	// makes the user's concurrent git commands fail.
	raw, err := os.ReadFile(trace)
	if err != nil {
		t.Fatal(err)
	}
	seen := 0
	for _, line := range strings.Split(string(raw), "\n") {
		if !strings.Contains(line, `"event":"start"`) || !strings.Contains(line, `"status"`) {
			continue
		}
		seen++
		if !strings.Contains(line, `"--no-optional-locks"`) {
			t.Errorf("status ran without --no-optional-locks: %s", line)
		}
	}
	if seen < 2 {
		t.Fatalf("saw %d status invocations, want at least 2 (Status and Run)", seen)
	}
}
