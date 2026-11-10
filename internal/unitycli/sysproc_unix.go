//go:build !windows

package unitycli

import (
	"os/exec"
	"syscall"
)

// setSysProcAttr starts the CLI in its own session, so it is not part of the
// terminal's foreground process group and survives the terminal closing.
func setSysProcAttr(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}

// interruptProcess sends SIGINT to the CLI and everything it started in its
// session (the process is its own group leader, so the group id is its pid).
func interruptProcess(cmd *exec.Cmd) error {
	if cmd.Process == nil {
		return nil
	}
	return syscall.Kill(-cmd.Process.Pid, syscall.SIGINT)
}
