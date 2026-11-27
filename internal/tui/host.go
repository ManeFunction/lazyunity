package tui

import (
	"context"
	"errors"
	"net/http"
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
	// LatestVerifiedCLI asks Unity's CDN for the newest Unity CLI release in
	// the verified range; ok is false when it could not tell.
	LatestVerifiedCLI(ctx context.Context) (v unitycli.Version, ok bool)
	// AddPackages adds packages to the project in dir with the OpenUPM CLI.
	AddPackages(ctx context.Context, exe, dir string, packages []string) error
	// RemovePackages removes packages from the project in dir with the OpenUPM CLI.
	RemovePackages(ctx context.Context, exe, dir string, packages []string) error
	// PackageRepo asks the OpenUPM registry for the web page of a package's
	// source repository.
	PackageRepo(ctx context.Context, id string) (string, error)
	// LookupPackage asks the OpenUPM registry for a package.
	LookupPackage(ctx context.Context, id string) (openupm.Package, error)
}

// machine is the host lazyunity runs on.
type machine struct{ env unitycli.Env }

func (h machine) OpenURL(url string) error { return launch.OpenURL(h.env.GOOS, url) }

func (h machine) OpenFolder(tmpl, dir string) error { return launch.OpenFolder(h.env.GOOS, tmpl, dir) }

func (h machine) StartEditor(exe, project, buildTarget string) error {
	return launch.Editor(exe, project, buildTarget)
}

func (h machine) Edit(tmpl, path, dir string, getenv func(string) string) (*exec.Cmd, error) {
	return launch.Edit(h.env.GOOS, tmpl, path, dir, getenv)
}

func (h machine) FindOpenUPM(ctx context.Context) (string, string) {
	path, err := openupm.Find(h.env.LookPath, h.env.Home)
	if err != nil {
		return "", ""
	}
	return path, openupm.Version(ctx, path)
}

func (h machine) LatestVerifiedCLI(ctx context.Context) (unitycli.Version, bool) {
	return unitycli.LatestVerified(ctx, nil)
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

func (h machine) RemovePackages(ctx context.Context, exe, dir string, packages []string) error {
	return openupm.Remove(ctx, exe, dir, packages)
}

func (h machine) PackageRepo(ctx context.Context, id string) (string, error) {
	return openupm.RepositoryURL(ctx, http.DefaultClient, openupm.Registry, id)
}

func (h machine) LookupPackage(ctx context.Context, id string) (openupm.Package, error) {
	return openupm.Lookup(ctx, http.DefaultClient, openupm.Registry, id)
}

// edit opens path in the code editor: os.edit from config.yml, started on its
// own, or else $VISUAL or $EDITOR, run in dir with the terminal handed over.
// noEditor runs when none is set; done, if not nil, is the message once a
// terminal editor has quit. An editor started on its own cannot be waited
// for, so done then comes when lazyunity's terminal is focused again: the
// user is back from the editor.
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
		m.onFocus = done
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
