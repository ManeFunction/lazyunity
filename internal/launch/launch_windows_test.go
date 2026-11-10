package launch

import "testing"

// cmd.exe gets the line as it was typed, quotes and all.
func TestShellCommandKeepsQuotesOnWindows(t *testing.T) {
	cmd := ShellCommand("windows", "", `dir "C:\Program Files"`, `C:\p`, func(string) string { return "" })
	want := `cmd /S /C "dir "C:\Program Files" & echo. & pause"`
	if cmd.SysProcAttr == nil || cmd.SysProcAttr.CmdLine != want {
		t.Errorf("command line = %q, want %q", cmd.SysProcAttr.CmdLine, want)
	}
}
