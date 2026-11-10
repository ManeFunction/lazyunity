package tui

import (
	"context"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/ManeFunction/lazyunity/internal/openupm"
)

// The OpenUPM CLI's row of the Tools tab and its install. The OpenUPM package
// does the work; this is what the user sees of it.

// openupmState is what is known about the installed OpenUPM CLI.
type openupmState struct {
	checked bool
	path    string // "" when not installed
	version string
}

type openupmMsg openupmState

type openupmInstalledMsg struct{ err error }

// probeOpenUPMCmd looks for the OpenUPM CLI and asks it for its version.
func (m *Model) probeOpenUPMCmd() tea.Cmd {
	h := m.host
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		path, version := h.FindOpenUPM(ctx)
		return openupmMsg{checked: true, path: path, version: version}
	}
}

// openupmStatus is the OpenUPM row of the Tools tab.
func (m *Model) openupmRow() toolRow {
	r := toolRow{id: "openupm", label: "OpenUPM CLI", run: (*Model).openupmTool}
	o := m.openupm
	switch {
	case !o.checked:
		r.status = "checking…"
	case o.path == "":
		r.status, r.tone = "not installed · enter to install", toneWarn
	default:
		v := o.version
		if v == "" {
			v = "installed"
		}
		r.status, r.tone = v+" · enter to reinstall", toneGood
	}
	if src := m.cfg.OpenUPM; !src.FromRegistry() {
		r.status += " · from " + repoName(src.Repo, src.Ref)
	}
	return r
}

// repoName is a repository URL without its scheme, with the ref if there is one.
func repoName(repo, ref string) string {
	s := strings.TrimSuffix(strings.TrimRight(repo, "/"), ".git")
	s = strings.TrimPrefix(strings.TrimPrefix(s, "https://"), "http://")
	s = strings.TrimPrefix(s, "github.com/")
	if ref != "" {
		s += "@" + ref
	}
	return s
}

// openupmTool installs the OpenUPM CLI, after saying how.
func (m *Model) openupmTool() tea.Cmd {
	if m.refuseWhileBusy("installing the OpenUPM CLI") {
		return nil
	}
	src := m.cfg.OpenUPM
	needs := []string{"npm"}
	if !src.FromRegistry() {
		needs = append(needs, "git")
	}
	var missing []string
	for _, p := range needs {
		if _, err := m.env.LookPath(p); err != nil {
			missing = append(missing, p)
		}
	}
	if len(missing) > 0 {
		m.push(&infoPopup{title: "OpenUPM CLI", lines: wrapText(
			"Installing the OpenUPM CLI needs "+strings.Join(missing, " and ")+", which "+
				map[bool]string{true: "is", false: "are"}[len(missing) == 1]+" not on the PATH.\n\n"+
				"Install Node.js (it comes with npm) from https://nodejs.org or your package manager"+
				map[bool]string{true: ", and git,", false: ""}[len(needs) > 1]+" then try again.", 70)})
		return nil
	}
	verb := "Install"
	if m.openupm.path != "" {
		verb = "Reinstall"
	}
	var msg string
	if src.FromRegistry() {
		msg = verb + " openupm-cli, the official release from the npm registry (npm install -g openupm-cli)?"
	} else {
		msg = verb + " openupm-cli from " + src.Repo
		if src.Ref != "" {
			msg += " (" + src.Ref + ")"
		}
		msg += "? It is cloned with git, built with npm, and installed with npm install -g."
	}
	msg += "\n\n"
	if src.FromRegistry() {
		msg += "To build it from a git repository instead (a fork, say), set openupm.repo in config.yml."
	} else {
		msg += "The repository is set by openupm.repo in config.yml; without it, the npm release is installed."
	}
	m.push(&confirmPopup{
		title: verb + " the OpenUPM CLI", yes: verb, message: msg,
		onYes: func(m *Model) tea.Cmd {
			return tea.ExecProcess(openupm.InstallCommand(m.goos, src), func(err error) tea.Msg { return openupmInstalledMsg{err: err} })
		},
	})
	return nil
}

func (m *Model) onOpenUPMInstalled(msg openupmInstalledMsg) tea.Cmd {
	if msg.err != nil {
		m.toastf(toastError, "The OpenUPM CLI was not installed: %v", msg.err)
	} else {
		m.toastf(toastOK, "Installed the OpenUPM CLI")
	}
	m.openupm.checked = false
	m.refreshTools()
	return m.probeOpenUPMCmd()
}
