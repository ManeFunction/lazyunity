package tui

import (
	"context"
	"os"
	"path/filepath"

	tea "charm.land/bubbletea/v2"

	"github.com/ManeFunction/lazyunity/internal/config"
	"github.com/ManeFunction/lazyunity/internal/unity"
	"github.com/ManeFunction/lazyunity/internal/unitycli"
)

// ---- messages ------------------------------------------------------------------

type projectsMsg struct {
	projects []unitycli.Project
	missing  map[string]bool
	err      error
}

type editorsMsg struct {
	editors []unitycli.Editor
	err     error
}

type localEditorsMsg struct{ local []unity.Installed }

type authMsg struct {
	info unitycli.AuthInfo
	err  error
}

type installPathMsg struct {
	path string
	err  error
}

type modulesMsg struct {
	version string
	mods    []unitycli.Module
	err     error
}

// errorMsg reports a failed background command.
type errorMsg struct {
	what string
	err  error
}

// toastMsg shows a message on the bottom line.
type toastMsg struct {
	kind toastKind
	text string
}

// ---- commands -----------------------------------------------------------------

func (m *Model) loadProjectsCmd() tea.Cmd {
	return m.cliCmd(func(ctx context.Context, c unityCLI) tea.Msg {
		ps, err := c.Projects(ctx)
		missing := map[string]bool{}
		for _, p := range ps {
			if _, serr := os.Stat(p.Path); serr != nil {
				missing[p.Path] = true
			}
		}
		return projectsMsg{projects: ps, missing: missing, err: err}
	})
}

func (m *Model) loadEditorsCmd() tea.Cmd {
	return m.cliCmd(func(ctx context.Context, c unityCLI) tea.Msg {
		eds, err := c.Editors(ctx)
		return editorsMsg{editors: eds, err: err}
	})
}

func (m *Model) scanEditorsCmd() tea.Cmd {
	goos, roots := m.goos, m.allRoots()
	return func() tea.Msg { return localEditorsMsg{local: unity.Scan(goos, roots)} }
}

func (m *Model) loadAuthCmd() tea.Cmd {
	return m.cliCmd(func(ctx context.Context, c unityCLI) tea.Msg {
		info, err := c.AuthStatus(ctx)
		return authMsg{info: info, err: err}
	})
}

func (m *Model) loadInstallPathCmd() tea.Cmd {
	return m.cliCmd(func(ctx context.Context, c unityCLI) tea.Msg {
		p, err := c.InstallPath(ctx)
		return installPathMsg{path: p, err: err}
	})
}

// refreshAllCmd reloads everything.
func (m *Model) refreshAllCmd() tea.Cmd {
	cmds := []tea.Cmd{
		m.scanEditorsCmd(), probeOpenUPMCmd(),
		m.loadProjectsCmd(), m.loadEditorsCmd(), m.loadAuthCmd(), m.loadInstallPathCmd(),
	}
	cmds = append(cmds, m.releasesTab.refresh(m))
	return tea.Batch(cmds...)
}

// ---- results ------------------------------------------------------------------

// handleData takes in the results of background commands.
func (m *Model) handleData(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case projectsMsg:
		if msg.err != nil {
			m.toastf(toastError, "Could not list projects: %v", msg.err)
			return nil
		}
		m.rawProjects, m.missing = msg.projects, msg.missing
		m.refreshProjects()
		if m.selectAfterLoad != "" {
			m.projectsTab.SelectKey(m.selectAfterLoad)
			m.selectAfterLoad = ""
		}
	case editorsMsg:
		if msg.err != nil {
			m.toastf(toastError, "Could not list editors: %v", msg.err)
			return nil
		}
		m.cliEditors, m.cliEditorsLoaded = msg.editors, true
		m.refreshEditors()
		return m.registerStrays()
	case localEditorsMsg:
		m.local = msg.local
		m.refreshEditors()
		return m.registerStrays()
	case authMsg:
		if msg.err != nil {
			m.toastf(toastError, "Could not check the sign-in: %v", msg.err)
			return nil
		}
		m.auth, m.authKnown = msg.info, true
		m.refreshTools()
	case installPathMsg:
		if msg.err == nil && msg.path != "" {
			m.installRoot = msg.path
			m.refreshRoots()
			return m.scanEditorsCmd()
		}
	case releasesMsg:
		return m.releasesTab.onPage(m, msg)
	case openedMsg:
		return m.onOpened(msg)
	case projectInfoMsg:
		m.onProjectInfo(msg)
	case modulesMsg:
		m.onModules(msg)
	case addModulesMsg:
		m.onAddModules(msg)
	case releaseModulesMsg:
		return m.onReleaseModules(msg)
	case planMsg:
		m.onPlan(msg)
	case cacheInfoMsg:
		m.onCacheInfo(msg)
	case templatesMsg:
		m.onTemplates(msg)
	case licenseMsg:
		m.onLicense(msg)
	case cliInstalledMsg:
		if msg.err != nil {
			m.showError("Installing the Unity CLI", msg.err)
		}
		return m.reprobeCLI(msg.err == nil)
	case cliStatusMsg:
		return m.onCLIStatus(msg)
	case openupmMsg:
		m.openupm = openupmState(msg)
		m.refreshTools()
	case openupmInstalledMsg:
		return m.onOpenUPMInstalled(msg)
	case toastMsg:
		m.toastf(msg.kind, "%s", msg.text)
	case errorMsg:
		m.toastf(toastError, "%s: %v", msg.what, msg.err)
	}
	return nil
}

// ---- rebuilding the lists --------------------------------------------------------------

// refreshProjects rebuilds the project list from the data and the remembered choices.
func (m *Model) refreshProjects() {
	rows := make([]projectRow, len(m.rawProjects))
	for i, p := range m.rawProjects {
		rows[i] = projectRow{Project: p, Missing: m.missing[p.Path], Pending: m.st.PendingFor(p.Path)}
	}
	sortProjects(rows, m.st.SortOrder())
	m.projectsTab.SetItems(rows)
}

// refreshEditors merges the scan of the disk with what the CLI reported.
func (m *Model) refreshEditors() {
	m.editors = mergeEditors(m.local, m.cliEditors, m.goos)
	m.installed = map[string]bool{}
	for _, e := range m.editors {
		m.installed[e.Version] = true
	}
	m.editorsTab.SetItems(m.editors)
	m.refreshRoots()
}

// allRoots is every folder Editors are looked for in: the install location
// first, then the folders added in the Roots tab and in the config.
func (m *Model) allRoots() []string {
	var out []string
	seen := map[string]bool{}
	add := func(p string) {
		if p = filepath.Clean(config.ExpandHome(p)); p != "." && !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}
	if m.installRoot != "" {
		add(m.installRoot)
	} else {
		add(unity.DefaultInstallRoot(m.goos, homeDir()))
	}
	for _, r := range m.st.ExtraRoots {
		add(r)
	}
	for _, r := range m.cfg.Editors.ExtraRoots {
		add(r)
	}
	return out
}

// refreshRoots rebuilds the Roots tab.
func (m *Model) refreshRoots() {
	install := m.allRoots()[0]
	counts := map[string]int{}
	for _, e := range m.local {
		counts[filepath.Clean(e.Root)]++
	}
	inState := map[string]bool{}
	for _, r := range m.st.ExtraRoots {
		inState[filepath.Clean(r)] = true
	}
	var rows []rootRow
	for _, p := range m.allRoots() {
		_, err := os.Stat(p)
		src := "config"
		switch {
		case p == install:
			src = "cli"
		case inState[p]:
			src = "state"
		}
		rows = append(rows, rootRow{Path: p, Install: p == install, Source: src, Exists: err == nil, Editors: counts[p]})
	}
	m.rootsTab.SetItems(rows)
}
