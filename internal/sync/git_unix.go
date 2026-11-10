//go:build unix

package sync

import (
	"os/exec"
	"syscall"
)

// detach starts git in its own session, so it has no controlling terminal to
// prompt on (opening /dev/tty fails), and makes the timeout kill the whole
// process group, so ssh and remote helpers die with git instead of holding
// its output pipe open.
func detach(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	cmd.Cancel = func() error {
		if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL); err != nil {
			return cmd.Process.Kill()
		}
		return nil
	}
}
