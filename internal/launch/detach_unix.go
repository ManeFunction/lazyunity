//go:build !windows

package launch

import (
	"os/exec"
	"syscall"
)

// detach puts cmd in its own session, so closing the terminal does not take it
// down.
func detach(cmd *exec.Cmd) { cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true} }
