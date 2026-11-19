package launch

import (
	"os/exec"
	"strings"
	"testing"
)

// cmd.exe gets the line as it was typed, quotes and all.
func TestShellCommandKeepsQuotesOnWindows(t *testing.T) {
	cmd := ShellCommand("windows", "", `dir "C:\Program Files"`, `C:\p`, func(string) string { return "" })
	want := `cmd /S /C "dir "C:\Program Files" & echo. & <nul set /p "=` + returnPrompt + `" & pause >nul"`
	if cmd.SysProcAttr == nil || cmd.SysProcAttr.CmdLine != want {
		t.Errorf("command line = %q, want %q", cmd.SysProcAttr.CmdLine, want)
	}
}

// PowerShell runs the line, then prints the prompt from the environment and
// waits for Enter. windows-latest has both PowerShells.
func TestShellCommandRunsPowerShell(t *testing.T) {
	for _, sh := range []string{"pwsh", "powershell"} {
		path, err := exec.LookPath(sh)
		if err != nil {
			continue
		}
		cmd := ShellCommand("windows", path, `Write-Output "it's $(1+1)"`, t.TempDir(), func(string) string { return "" })
		cmd.Stdin = strings.NewReader("\r\n")
		out, err := cmd.Output()
		if err != nil {
			t.Errorf("%s: %v", sh, err)
			continue
		}
		if !strings.HasPrefix(string(out), "it's 2") || !strings.HasSuffix(string(out), returnPrompt) {
			t.Errorf("%s: output = %q", sh, out)
		}
	}
}
