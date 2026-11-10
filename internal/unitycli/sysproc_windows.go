//go:build windows

package unitycli

import (
	"os/exec"
	"strconv"
	"syscall"
)

const (
	detachedProcess = 0x00000008 // DETACHED_PROCESS
	createNoWindow  = 0x08000000 // CREATE_NO_WINDOW
)

// setSysProcAttr starts the CLI without a console window, detached from
// lazyunity's console.
func setSysProcAttr(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP | detachedProcess,
		HideWindow:    true,
	}
}

// interruptProcess stops the CLI. A detached process has no console to send
// Ctrl+C to, so it is killed, together with every process it started:
// killing unity.exe alone would leave its helpers running, and an installer
// among them keeps the install lock.
func interruptProcess(cmd *exec.Cmd) error {
	if cmd.Process == nil {
		return nil
	}
	tree := exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(cmd.Process.Pid))
	tree.SysProcAttr = &syscall.SysProcAttr{CreationFlags: createNoWindow, HideWindow: true}
	if tree.Run() == nil {
		return nil
	}
	return cmd.Process.Kill()
}
