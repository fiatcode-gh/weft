//go:build !unix

package sync

import "os/exec"

// detach is a no-op off unix: there is no session or process group to
// manage, so GIT_TERMINAL_PROMPT, BatchMode and WaitDelay carry the load.
func detach(cmd *exec.Cmd) {}
