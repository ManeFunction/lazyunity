package tui

import (
	"context"
	"errors"
	"os/exec"

	tea "charm.land/bubbletea/v2"

	"github.com/ManeFunction/lazyunity/internal/config"
	"github.com/ManeFunction/lazyunity/internal/launch"
	"github.com/ManeFunction/lazyunity/internal/openupm"
	"github.com/ManeFunction/lazyunity/internal/unitycli"
)

// host is what the interface does to the machine around it, other than
// through the Unity CLI: it starts the programs that outlive lazyunity, and it
// runs the OpenUPM CLI. machine is the real one; the tests record instead.
type host interface {
	OpenURL(url string) error
	// OpenFolder shows dir in the file manager; tmpl is os.openFolder.
	OpenFolder(tmpl, dir string) error
	// StartEditor starts a Unity Editor on a project, without the Unity CLI.
	StartEditor(exe, project, buildTarget string) error
	// Edit opens path in a code editor, as launch.Edit does.
	Edit(tmpl, path, dir string, getenv func(string) string) (*exec.Cmd, error)
	// FindOpenUPM looks for the OpenUPM CLI and asks for its version. path is
	// "" when it is not installed.
	FindOpenUPM(ctx context.Context) (path, version string)
	// LatestOpenUPM asks where src is now: the newest version in the npm
	// registry, or the commit the repository's branch points to.
	LatestOpenUPM(ctx context.Context, src config.OpenUPM) (string, error)
	// AddPackages adds packages to the project in dir with the OpenUPM CLI.
	AddPackages(ctx context.Context, exe, dir string, packages []string) error
}

// machine is the host lazyunity runs on.
type machine struct{ env unitycli.Env }

func (h machine) OpenURL(url string) error { return launch.OpenURL(h.env.GOOS, url) }

func (h machine) OpenFolder(tmpl, dir string) error { return launch.OpenFolder(h.env.GOOS, tmpl, dir) }

func (h machine) StartEditor(exe, project, buildTarget string) error {
	return launch.Editor(exe, project, buildTarget)
}

func (h machine) Edit(tmpl, path, dir string, getenv func(string) string) (*exec.Cmd, error) {
	return launch.Edit(tmpl, path, dir, getenv)
}

func (h machine) FindOpenUPM(ctx context.Context) (string, string) {
	path, err := openupm.Find(h.env.LookPath, h.env.Home)
	if err != nil {
		return "", ""
	}
	return path, openupm.Version(ctx, path)
}

func (h machine) LatestOpenUPM(ctx context.Context, src config.OpenUPM) (string, error) {
	tool := "npm"
	if !src.FromRegistry() {
		tool = "git"
	}
	exe, err := h.env.LookPath(tool)
	if err != nil {
		return "", err
	}
	if src.FromRegistry() {
		return openupm.LatestRelease(ctx, exe)
	}
	return openupm.LatestCommit(ctx, exe, src.Repo, src.Ref)
}

func (h machine) AddPackages(ctx context.Context, exe, dir string, packages []string) error {
	return openupm.Add(ctx, exe, dir, packages)
}

// edit opens path in the code editor: os.edit from config.yml, started on its
// own, or else $VISUAL or $EDITOR, run in dir with the terminal handed over.
// noEditor runs when none is set; done, if not nil, is the message once a
// terminal editor has quit.
func (m *Model) edit(path, dir string, noEditor func(m *Model), done tea.Msg) tea.Cmd {
	cmd, err := m.host.Edit(m.cfg.OS.Edit, path, dir, m.getenv)
	switch {
	case errors.Is(err, launch.ErrNoEditor):
		noEditor(m)
		return nil
	case err != nil:
		m.toastf(toastError, "Could not start the editor: %v", err)
		return nil
	case cmd == nil: // started on its own
		return nil
	}
	return tea.ExecProcess(cmd, func(err error) tea.Msg {
		if err != nil {
			return errorMsg{what: "Editor", err: err}
		}
		return done
	})
}

// getenv reads an environment variable of the machine lazyunity runs on.
func (m *Model) getenv(key string) string { return m.env.Getenv(key) }
