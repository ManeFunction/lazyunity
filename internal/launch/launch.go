// Package launch starts programs that must outlive lazyunity: a Unity Editor
// opened directly, the file manager, a code editor.
package launch

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"time"
	"unicode/utf16"
)

// Editor starts a Unity Editor directly, without the Unity CLI. The Editor then
// starts signed out. It runs on its own, detached from the terminal.
func Editor(exe, project, buildTarget string) error {
	args := []string{"-projectPath", project}
	if buildTarget != "" {
		args = append(args, "-buildTarget", buildTarget)
	}
	return start(exec.Command(exe, args...))
}

// OpenFolder shows dir in the file manager. tmpl, when not empty, is the command
// to run instead of the system's default; {{dir}} stands for the folder.
//
// On Linux the file manager is asked over D-Bus first (the same call that "show
// in folder" uses), because xdg-open follows the inode/directory handler, which
// can be a terminal. xdg-open is the fallback.
func OpenFolder(goos, tmpl, dir string) error {
	if goos == "linux" && strings.TrimSpace(tmpl) == "" && showFolderDBus(dir) {
		return nil
	}
	return openWith(goos, tmpl, dir)
}

func openWith(goos, tmpl, dir string) error {
	argv, err := FolderCommand(goos, tmpl, dir)
	if err != nil {
		return err
	}
	return start(exec.Command(argv[0], argv[1:]...))
}

// showFolderDBus asks org.freedesktop.FileManager1 to show dir. It reports
// whether a file manager took the request.
func showFolderDBus(dir string) bool {
	gdbus, err := exec.LookPath("gdbus")
	if err != nil {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	uri := (&url.URL{Scheme: "file", Path: dir}).String()
	cmd := exec.CommandContext(ctx, gdbus, "call", "--session",
		"--dest", "org.freedesktop.FileManager1", "--object-path", "/org/freedesktop/FileManager1",
		"--method", "org.freedesktop.FileManager1.ShowFolders", "['"+uri+"']", "")
	return cmd.Run() == nil
}

// OpenURL opens a web page in the default browser.
func OpenURL(goos, link string) error {
	return openWith(goos, "", link) // the same system command opens both
}

// FolderCommand is the command OpenFolder runs.
func FolderCommand(goos, tmpl, dir string) ([]string, error) {
	if strings.TrimSpace(tmpl) != "" {
		return Expand(goos, tmpl, dir)
	}
	switch goos {
	case "windows":
		return []string{"explorer", dir}, nil
	case "darwin":
		return []string{"open", dir}, nil
	}
	return []string{"xdg-open", dir}, nil
}

// Run starts a command line detached. {{dir}} in tmpl stands for dir; goos
// decides how the line is split (see Split).
func Run(goos, tmpl, dir string) error {
	argv, err := Expand(goos, tmpl, dir)
	if err != nil {
		return err
	}
	return start(exec.Command(argv[0], argv[1:]...))
}

// ErrNoEditor is Edit's answer when no code editor is set.
var ErrNoEditor = errors.New("no code editor is set")

// Edit opens path in a code editor. With tmpl, the os.edit command line of
// config.yml ({{dir}} standing for path), the editor is started on its own and
// the returned command is nil. Otherwise it is $VISUAL or $EDITOR, as read by
// getenv: the command returned runs it on path, in dir, and is meant to be run
// with the terminal handed over. ErrNoEditor means neither is set. goos decides
// how the command lines are split (see Split).
func Edit(goos, tmpl, path, dir string, getenv func(string) string) (*exec.Cmd, error) {
	if strings.TrimSpace(tmpl) != "" {
		return nil, Run(goos, tmpl, path)
	}
	editor := getenv("VISUAL")
	if editor == "" {
		editor = getenv("EDITOR")
	}
	if strings.TrimSpace(editor) == "" {
		return nil, ErrNoEditor
	}
	words, err := Split(goos, editor)
	if err != nil {
		return nil, fmt.Errorf("$EDITOR: %w", err)
	}
	cmd := exec.Command(words[0], append(words[1:], path)...)
	cmd.Dir = dir
	return cmd, nil
}

// returnPrompt is the blue line that waits for the user before lazyunity takes
// the terminal back. A Unix shell gets it in returnPromptVar, so it is never
// part of the shell's code. cmd.exe expands even %VAR% before it parses the
// line, so there it is written into the line and must stay free of the
// characters cmd.exe reads (see TestReturnPromptIsSafeForCmd).
const returnPrompt = "\x1b[34mPress enter to return to lazyunity...\x1b[0m "

const returnPromptVar = "LAZYUNITY_RETURN_PROMPT"

// ShellCommand runs a line of shell in dir, then waits for Enter, so that
// its output can be read before lazyunity takes the terminal back. shell is
// os.shell from config.yml. On Unix, empty means $SHELL, as read by getenv,
// then /bin/sh. On Windows, empty means cmd; pwsh and powershell run the line
// as PowerShell, and any other shell (Git Bash, say) is taken for a Unix one.
// $SHELL is not read on Windows, where Git Bash sets it to a path that only
// its own programs understand.
func ShellCommand(goos, shell, text, dir string, getenv func(string) string) *exec.Cmd {
	var cmd *exec.Cmd
	switch {
	case goos != "windows":
		if shell == "" {
			shell = getenv("SHELL")
		}
		if shell == "" {
			shell = "/bin/sh"
		}
		cmd = unixShell(shell, text)
	case shell == "" || isShell(shell, "cmd"):
		cmd = cmdShell(text)
	case isShell(shell, "pwsh", "powershell"):
		cmd = powerShell(shell, text)
	default:
		cmd = unixShell(shell, text)
	}
	cmd.Dir = dir
	return cmd
}

// isShell reports whether the program shell, a name or a path with either
// separator, is one of names, with or without .exe, in any case.
func isShell(shell string, names ...string) bool {
	base := strings.ToLower(shell[strings.LastIndexAny(shell, `/\`)+1:])
	base = strings.TrimSuffix(base, ".exe")
	for _, n := range names {
		if base == n {
			return true
		}
	}
	return false
}

func unixShell(shell, text string) *exec.Cmd {
	cmd := exec.Command(shell, "-c", text+`; printf '\n%s' "$`+returnPromptVar+`"; read lazyunity_return`) // not read _: _ is read-only in fish
	cmd.Env = append(os.Environ(), returnPromptVar+"="+returnPrompt)
	return cmd
}

func cmdShell(text string) *exec.Cmd {
	// set /p prints the prompt without a newline; pause waits silently.
	line := text + ` & echo. & <nul set /p "=` + returnPrompt + `" & pause >nul`
	cmd := exec.Command("cmd", "/S", "/C", line)
	// cmd.exe does not read the \" that Go would quote the line with. With
	// /S it takes off the outer quotes and runs the rest as it is.
	setCmdLine(cmd, `cmd /S /C "`+line+`"`)
	return cmd
}

// powerShell runs text as PowerShell. The script goes in -EncodedCommand
// (base64 of UTF-16LE), so no quoting rule of Windows or of PowerShell ever
// touches it, and the prompt comes from the environment, as on Unix.
func powerShell(shell, text string) *exec.Cmd {
	script := text + "\n" + `Write-Host ''; [Console]::Write($env:` + returnPromptVar + `); [void][Console]::ReadLine()`
	cmd := exec.Command(shell, "-NoLogo", "-NoProfile", "-EncodedCommand", encodePowerShell(script))
	cmd.Env = append(os.Environ(), returnPromptVar+"="+returnPrompt)
	return cmd
}

// encodePowerShell encodes a script for PowerShell's -EncodedCommand.
func encodePowerShell(script string) string {
	u := utf16.Encode([]rune(script))
	b := make([]byte, 2*len(u))
	for i, c := range u {
		binary.LittleEndian.PutUint16(b[2*i:], c)
	}
	return base64.StdEncoding.EncodeToString(b)
}

// Expand splits a command line into words and puts dir where {{dir}} is. The
// substitution happens after the splitting, so a folder with spaces in its name
// stays one argument.
func Expand(goos, tmpl, dir string) ([]string, error) {
	words, err := Split(goos, tmpl)
	if err != nil {
		return nil, err
	}
	if len(words) == 0 {
		return nil, errors.New("empty command")
	}
	for i, w := range words {
		words[i] = strings.ReplaceAll(w, "{{dir}}", dir)
	}
	return words, nil
}

// Split cuts a command line into words. Single and double quotes group words,
// as in a shell, but nothing is ever expanded. On Unix a backslash escapes the
// next character. On Windows it is an ordinary character, since it separates
// the folders of a path: C:\Tools\nvim.exe stays as it is written.
func Split(goos, s string) ([]string, error) {
	escapes := goos != "windows"
	var words []string
	var cur strings.Builder
	inWord := false
	var quote rune
	escaped := false
	for _, r := range s {
		switch {
		case escaped:
			cur.WriteRune(r)
			escaped = false
		case escapes && r == '\\' && quote != '\'':
			escaped = true
			inWord = true
		case quote != 0:
			if r == quote {
				quote = 0
			} else {
				cur.WriteRune(r)
			}
		case r == '\'' || r == '"':
			quote = r
			inWord = true
		case r == ' ' || r == '\t':
			if inWord {
				words = append(words, cur.String())
				cur.Reset()
				inWord = false
			}
		default:
			cur.WriteRune(r)
			inWord = true
		}
	}
	if quote != 0 {
		return nil, fmt.Errorf("unterminated %c in command", quote)
	}
	if escaped {
		return nil, errors.New("command ends with a backslash")
	}
	if inWord {
		words = append(words, cur.String())
	}
	return words, nil
}

// start runs cmd on its own: no terminal and output thrown away. lazyunity does
// not wait for it, but a goroutine reaps it once it exits, so that it does not
// stay a zombie until lazyunity quits.
func start(cmd *exec.Cmd) error {
	devnull, err := os.OpenFile(os.DevNull, os.O_RDWR, 0)
	if err != nil {
		return err
	}
	defer devnull.Close()
	cmd.Stdin, cmd.Stdout, cmd.Stderr = devnull, devnull, devnull
	detach(cmd)
	if err := cmd.Start(); err != nil {
		return err
	}
	go cmd.Wait() // how a detached program ends is not reported
	return nil
}
