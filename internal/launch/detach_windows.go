//go:build windows

package launch

import (
	"os/exec"
	"syscall"
)

const detachedProcess = 0x00000008 // DETACHED_PROCESS

// detach starts cmd without a console, apart from lazyunity's.
//
// No HideWindow: DETACHED_PROCESS already keeps a console program from getting
// a window, and a GUI program such as the Unity Editor can take the SW_HIDE
// that HideWindow passes for its first window and open invisible.
func detach(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP | detachedProcess}
}

// setCmdLine makes cmd run with exactly this command line, for programs such
// as cmd.exe that do not follow the quoting Go writes Args with.
func setCmdLine(cmd *exec.Cmd, line string) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.CmdLine = line
}
