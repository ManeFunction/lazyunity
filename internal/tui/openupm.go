package tui

import (
	"context"
	"os"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/ManeFunction/lazyunity/internal/config"
	"github.com/ManeFunction/lazyunity/internal/openupm"
	"github.com/ManeFunction/lazyunity/internal/state"
	"github.com/ManeFunction/lazyunity/internal/unitycli"
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

// openupmLatest is where the source of the OpenUPM CLI is now: the newest
// version in the npm registry, or the commit the repository's branch points to.
type openupmLatest struct {
	src    config.OpenUPM
	latest string
}

type openupmLatestMsg struct {
	openupmLatest
	err error
}

type openupmInstalledMsg struct {
	err   error
	src   config.OpenUPM
	stamp string // the file the install wrote the commit it built to
}

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

// checkOpenUPMUpdateCmd asks whether a newer OpenUPM CLI is out where it comes from.
func (m *Model) checkOpenUPMUpdateCmd() tea.Cmd {
	h, src := m.host, m.cfg.OpenUPM
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		latest, err := h.LatestOpenUPM(ctx, src)
		return openupmLatestMsg{openupmLatest: openupmLatest{src: src, latest: latest}, err: err}
	}
}

func (m *Model) onOpenUPM(msg openupmMsg) tea.Cmd {
	m.openupm = openupmState(msg)
	b := m.st.OpenUPMBuild
	switch {
	case b != nil && msg.path == "":
		// gone: whatever is installed next is not that build
		m.st.OpenUPMBuild = nil
		m.saveState()
	case b != nil && b.Version == "" && msg.version != "":
		b.Version = msg.version
		m.saveState()
	}
	m.refreshTools()
	if msg.path == "" {
		return nil
	}
	return m.checkOpenUPMUpdateCmd()
}

// openupmBuild is the build from a repository that is installed, if it is.
func (m *Model) openupmBuild() *state.OpenUPMBuild {
	b := m.st.OpenUPMBuild
	if b == nil || (b.Version != "" && m.openupm.version != "" && b.Version != m.openupm.version) {
		return nil
	}
	return b
}

// buildIs reports whether b was built from src.
func buildIs(b *state.OpenUPMBuild, src config.OpenUPM) bool {
	return !src.FromRegistry() && repoName(b.Repo, "") == repoName(src.Repo, "") &&
		strings.TrimSpace(b.Ref) == strings.TrimSpace(src.Ref)
}

// buildName is the repository, branch and commit of a build.
func buildName(b *state.OpenUPMBuild) string {
	return repoName(b.Repo, b.Ref) + " #" + shortCommit(b.Commit)
}

func shortCommit(c string) string {
	if len(c) > 7 {
		return c[:7]
	}
	return c
}

// openupmUpdate is the newer OpenUPM CLI that is out, as the row names it: a
// version from the npm registry, or a #commit of the repository. It is ""
// when there is none, or when the installed CLI is not from the configured
// source, so that there is nothing to compare it with.
func (m *Model) openupmUpdate() string {
	src, n := m.cfg.OpenUPM, m.openupmNew
	if m.openupm.path == "" || n.latest == "" || n.src != src {
		return ""
	}
	b := m.openupmBuild()
	if src.FromRegistry() {
		if b != nil {
			return ""
		}
		if (unitycli.UpdateInfo{Current: m.openupm.version, Latest: n.latest}).Available() {
			return n.latest
		}
		return ""
	}
	if b == nil || !buildIs(b, src) || b.Commit == n.latest {
		return ""
	}
	return "#" + shortCommit(n.latest)
}

// openupmRow is the OpenUPM row of the Tools tab. The npm release reads like
// the Unity CLI's row; a build from a repository names its branch and commit.
func (m *Model) openupmRow() toolRow {
	r := toolRow{id: "openupm", label: "OpenUPM CLI", run: (*Model).openupmTool}
	o, src := m.openupm, m.cfg.OpenUPM
	if !o.checked {
		r.status = "checking…"
		return r
	}
	if o.path == "" {
		r.status, r.tone = "not installed · press enter to install", toneWarn
		if !src.FromRegistry() {
			r.status += " from " + repoName(src.Repo, src.Ref)
		}
		return r
	}
	v := o.version
	if v == "" {
		v = "installed"
	}
	b, update := m.openupmBuild(), m.openupmUpdate()
	switch {
	case src.FromRegistry() && b != nil:
		r.status, r.tone = v+" · from "+buildName(b)+" · enter to install the npm release", toneWarn
	case src.FromRegistry() && update != "":
		r.status, r.tone = "update available: "+update+" · enter to update", toneWarn
	case src.FromRegistry():
		r.status, r.tone = "ready · "+v, toneGood
	case b == nil:
		r.status, r.tone = v+" · not built from "+repoName(src.Repo, src.Ref)+" · enter to install it", toneWarn
	case !buildIs(b, src):
		r.status, r.tone = v+" · from "+buildName(b)+" · enter to install "+repoName(src.Repo, src.Ref), toneWarn
	case update != "":
		r.status, r.tone = "update available: "+update+" · from "+buildName(b), toneWarn
	default:
		r.status, r.tone = v+" · from "+buildName(b), toneGood
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
	verb, to := "Install", ""
	update := m.openupmUpdate()
	switch {
	case update != "":
		verb, to = "Update", " to "+update
	case m.openupm.path != "":
		verb = "Reinstall"
	}
	var msg string
	if src.FromRegistry() {
		msg = verb + " openupm-cli" + to + ", the official release from the npm registry (npm install -g openupm-cli)?"
	} else {
		msg = verb + " openupm-cli from " + src.Repo
		if src.Ref != "" {
			msg += " (" + src.Ref + ")"
		}
		msg += to + "? It is cloned with git, built with npm, and installed with npm install -g."
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
			stamp := ""
			if !src.FromRegistry() {
				if f, err := os.CreateTemp("", "lazyunity-openupm-*.txt"); err == nil {
					stamp = f.Name()
					f.Close()
				}
			}
			return tea.ExecProcess(openupm.InstallCommand(m.goos, src, stamp), func(err error) tea.Msg {
				return openupmInstalledMsg{err: err, src: src, stamp: stamp}
			})
		},
	})
	return nil
}

func (m *Model) onOpenUPMInstalled(msg openupmInstalledMsg) tea.Cmd {
	var commit string
	if msg.stamp != "" {
		b, _ := os.ReadFile(msg.stamp)
		commit = strings.TrimSpace(string(b))
		os.Remove(msg.stamp)
	}
	if msg.err != nil {
		m.toastf(toastError, "The OpenUPM CLI was not installed: %v", msg.err)
	} else {
		m.toastf(toastOK, "Installed the OpenUPM CLI")
		// the version is filled in by the probe that follows
		m.st.OpenUPMBuild = nil
		if !msg.src.FromRegistry() && commit != "" {
			m.st.OpenUPMBuild = &state.OpenUPMBuild{Repo: strings.TrimSpace(msg.src.Repo), Ref: strings.TrimSpace(msg.src.Ref), Commit: commit}
		}
		m.saveState()
	}
	m.openupm.checked = false
	m.refreshTools()
	return m.probeOpenUPMCmd()
}
