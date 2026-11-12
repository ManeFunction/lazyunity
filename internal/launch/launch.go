// Package launch starts programs that must outlive lazyunity: a Unity Editor
// opened directly, the file manager, a code editor.
package launch

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"time"
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
		return Expand(tmpl, dir)
	}
	switch goos {
	case "windows":
		return []string{"explorer", dir}, nil
	case "darwin":
		return []string{"open", dir}, nil
	}
	return []string{"xdg-open", dir}, nil
}

// Run starts a command line detached. {{dir}} in tmpl stands for dir.
func Run(tmpl, dir string) error {
	argv, err := Expand(tmpl, dir)
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
// with the terminal handed over. ErrNoEditor means neither is set.
func Edit(tmpl, path, dir string, getenv func(string) string) (*exec.Cmd, error) {
	if strings.TrimSpace(tmpl) != "" {
		return nil, Run(tmpl, path)
	}
	editor := getenv("VISUAL")
	if editor == "" {
		editor = getenv("EDITOR")
	}
	if strings.TrimSpace(editor) == "" {
		return nil, ErrNoEditor
	}
	words, err := Split(editor)
	if err != nil {
		return nil, fmt.Errorf("$EDITOR: %w", err)
	}
	cmd := exec.Command(words[0], append(words[1:], path)...)
	cmd.Dir = dir
	return cmd, nil
}

// ShellCommand runs a line of shell in dir, then waits for Enter, so that
// its output can be read before lazyunity takes the terminal back. shell is the
// shell to use on Unix; empty means $SHELL, as read by getenv, then /bin/sh.
// Windows uses cmd.
func ShellCommand(goos, shell, text, dir string, getenv func(string) string) *exec.Cmd {
	var cmd *exec.Cmd
	if goos == "windows" {
		line := text + " & echo. & pause"
		cmd = exec.Command("cmd", "/S", "/C", line)
		// cmd.exe does not read the \" that Go would quote the line with. With
		// /S it takes off the outer quotes and runs the rest as it is.
		setCmdLine(cmd, `cmd /S /C "`+line+`"`)
	} else {
		if shell == "" {
			shell = getenv("SHELL")
		}
		if shell == "" {
			shell = "/bin/sh"
		}
		cmd = exec.Command(shell, "-c", text+`; printf '\n\033[34mPress enter to return to lazyunity...\033[0m '; read _`)
	}
	cmd.Dir = dir
	return cmd
}

// Expand splits a command line into words and puts dir where {{dir}} is. The
// substitution happens after the splitting, so a folder with spaces in its name
// stays one argument.
func Expand(tmpl, dir string) ([]string, error) {
	words, err := Split(tmpl)
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

// Split cuts a command line into words. Single and double quotes group words
// and a backslash escapes the next character, as in a shell, but nothing is
// ever expanded.
func Split(s string) ([]string, error) {
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
		case r == '\\' && quote != '\'':
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
