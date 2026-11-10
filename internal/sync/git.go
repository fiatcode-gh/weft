package sync

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"time"
)

// killGrace is how long Run and Status wait, after killing a timed-out git,
// for its output pipe to close before abandoning it (exec.Cmd.WaitDelay).
// Backstop for descendants the platform kill hook (detach) could not reach.
var killGrace = time.Second

// newGitRunner returns a function that runs git in repoDir and returns its
// combined output. Every invocation is bounded by syncTimeout and can never
// prompt: weft owns the terminal while it runs, so a credential or host-key
// prompt would corrupt the screen and hang the sync waiting for keys the
// user cannot meaningfully give. Authentication that needs a prompt instead
// fails the stage with git's own explanation in the output.
func newGitRunner(repoDir string) func(args ...string) (string, error) {
	env := append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	if !userChoseSSH(repoDir) {
		env = append(env, "GIT_SSH_COMMAND=ssh -o BatchMode=yes")
	}
	return func(args ...string) (string, error) {
		return runGit(repoDir, env, args...)
	}
}

// userChoseSSH reports whether the user picked their own ssh command, through
// GIT_SSH_COMMAND, GIT_SSH or core.sshCommand. Those own their options, so
// weft leaves them alone instead of forcing BatchMode on top.
func userChoseSSH(repoDir string) bool {
	if os.Getenv("GIT_SSH_COMMAND") != "" || os.Getenv("GIT_SSH") != "" {
		return true
	}
	out, err := runGit(repoDir, os.Environ(), "config", "--get", "core.sshCommand")
	return err == nil && strings.TrimSpace(out) != ""
}

func runGit(repoDir string, env []string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), syncTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = repoDir
	cmd.Env = env
	cmd.WaitDelay = killGrace
	detach(cmd)
	out, err := cmd.CombinedOutput()
	return string(out), err
}
