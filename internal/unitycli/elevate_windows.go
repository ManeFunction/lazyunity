//go:build windows

package unitycli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	shell32             = windows.NewLazySystemDLL("shell32.dll")
	procShellExecuteExW = shell32.NewProc("ShellExecuteExW")
	user32              = windows.NewLazySystemDLL("user32.dll")
	procForegroundWnd   = user32.NewProc("GetForegroundWindow")
)

// shellExecuteInfo is SHELLEXECUTEINFOW.
type shellExecuteInfo struct {
	cbSize       uint32
	fMask        uint32
	hwnd         uintptr
	lpVerb       *uint16
	lpFile       *uint16
	lpParameters *uint16
	lpDirectory  *uint16
	nShow        int32
	hInstApp     uintptr
	lpIDList     uintptr
	lpClass      *uint16
	hkeyClass    uintptr
	dwHotKey     uint32
	hIcon        uintptr
	hProcess     windows.Handle
}

const (
	seeMaskNoCloseProcess = 0x00000040
	seeMaskNoAsync        = 0x00000100
	seeMaskFlagNoUI       = 0x00000400
	swHide                = 0
)

// runElevatedWindows runs the CLI as administrator: Windows shows its UAC prompt, and
// the CLI runs in a new, elevated process. That process cannot write to
// lazyunity's handles, so it goes through cmd.exe, which sends its output to
// temporary files that are read back once it ends.
var runElevated = runElevatedWindows

func runElevatedWindows(ctx context.Context, path string, args []string) (processResult, error) {
	dir, err := os.MkdirTemp("", "lazyunity-elevated-*")
	if err != nil {
		return processResult{ExitCode: -1}, err
	}
	defer os.RemoveAll(dir)
	outPath, errPath := filepath.Join(dir, "out"), filepath.Join(dir, "err")

	line, err := cmdLine(append([]string{path}, args...))
	if err != nil {
		return processResult{ExitCode: -1}, err
	}
	redirOut, err := cmdQuote(outPath)
	if err != nil {
		return processResult{ExitCode: -1}, err
	}
	redirErr, err := cmdQuote(errPath)
	if err != nil {
		return processResult{ExitCode: -1}, err
	}
	// /s: cmd drops the outer quotes and runs the rest as written.
	params := fmt.Sprintf(`/d /s /c "%s > %s 2> %s"`, line, redirOut, redirErr)

	cmdExe := filepath.Join(os.Getenv("SystemRoot"), "System32", "cmd.exe")
	if os.Getenv("SystemRoot") == "" {
		cmdExe = "cmd.exe"
	}
	verb, _ := windows.UTF16PtrFromString("runas")
	file, err := windows.UTF16PtrFromString(cmdExe)
	if err != nil {
		return processResult{ExitCode: -1}, err
	}
	par, err := windows.UTF16PtrFromString(params)
	if err != nil {
		return processResult{ExitCode: -1}, err
	}
	wd, err := windows.UTF16PtrFromString(dir)
	if err != nil {
		return processResult{ExitCode: -1}, err
	}
	// Owned by the window in front (the terminal lazyunity runs in), the
	// prompt comes up in front instead of blinking in the taskbar.
	hwnd, _, _ := procForegroundWnd.Call()
	info := shellExecuteInfo{
		fMask:        seeMaskNoCloseProcess | seeMaskNoAsync | seeMaskFlagNoUI,
		hwnd:         hwnd,
		lpVerb:       verb,
		lpFile:       file,
		lpParameters: par,
		lpDirectory:  wd,
		nShow:        swHide,
	}
	info.cbSize = uint32(unsafe.Sizeof(info))
	if ok, _, callErr := procShellExecuteExW.Call(uintptr(unsafe.Pointer(&info))); ok == 0 {
		if errors.Is(callErr, windows.ERROR_CANCELLED) {
			return processResult{ExitCode: -1}, errElevationDeclined
		}
		return processResult{ExitCode: -1}, callErr
	}
	if info.hProcess == 0 {
		return processResult{ExitCode: -1}, errors.New("Windows started no process")
	}
	defer windows.CloseHandle(info.hProcess)

	for {
		ev, err := windows.WaitForSingleObject(info.hProcess, 200)
		if err != nil {
			return processResult{ExitCode: -1}, err
		}
		if ev == windows.WAIT_OBJECT_0 {
			break
		}
		if ctx.Err() != nil {
			// The elevated process may refuse a non-elevated caller; then it
			// runs to its end on its own.
			_ = windows.TerminateProcess(info.hProcess, ExitTerminated)
			time.Sleep(interruptGrace / 5)
			return processResult{ExitCode: ExitTerminated}, nil
		}
	}
	var code uint32
	if err := windows.GetExitCodeProcess(info.hProcess, &code); err != nil {
		return processResult{ExitCode: -1}, err
	}
	res := processResult{ExitCode: int(code)}
	if b, err := os.ReadFile(outPath); err == nil {
		res.Stdout = string(b)
	}
	if b, err := os.ReadFile(errPath); err == nil {
		res.Stderr = string(b)
	}
	return res, nil
}

// cmdLine quotes each argument for cmd.exe.
func cmdLine(args []string) (string, error) {
	parts := make([]string, len(args))
	for i, a := range args {
		q, err := cmdQuote(a)
		if err != nil {
			return "", err
		}
		parts[i] = q
	}
	return strings.Join(parts, " "), nil
}

// cmdQuote puts s in double quotes, where cmd.exe takes &, |, <, > and spaces
// literally. A double quote or a percent sign (cmd expands %VAR% even inside
// quotes) cannot be passed safely, so they are refused.
func cmdQuote(s string) (string, error) {
	if strings.ContainsAny(s, "\"%\r\n") {
		return "", fmt.Errorf("cannot pass %q to an elevated process", s)
	}
	return `"` + s + `"`, nil
}
