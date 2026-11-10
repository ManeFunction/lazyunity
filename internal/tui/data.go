package tui

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"

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

type releasesMsg struct {
	stream string
	gen    int
	offset int
	page   unitycli.ReleasePage
	err    error
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

// ---- state of lazily loaded data -------------------------------------------------

// releasesState tracks the Releases tab, which is loaded when first looked at.
type releasesState struct {
	stream     string // "" (every stream), lts, tech, beta, alpha
	nextOffset int    // 0 when there is no more
	loaded     bool
	loading    bool
	// gen counts the times the list was started over, so that a page asked for
	// before is not added to the new list.
	gen int
}

// streamName is how a release stream is called on screen.
func streamName(stream string) string {
	switch stream {
	case "":
		return "All"
	case "tech":
		return "Supported"
	case "lts":
		return "LTS"
	}
	return strings.ToUpper(stream[:1]) + stream[1:]
}

const (
	releasesPerPage = 25
	// releasesPerSearch is the page size while a filter is typed: every page is
	// then loaded, so that the filter sees every release, in fewer calls.
	releasesPerSearch = 200
)

// ---- commands -----------------------------------------------------------------

func (m *Model) loadProjectsCmd() tea.Cmd {
	c := m.cli
	if c == nil {
		return nil
	}
	return func() tea.Msg {
		ps, err := c.Projects(context.Background())
		missing := map[string]bool{}
		for _, p := range ps {
			if _, serr := os.Stat(p.Path); serr != nil {
				missing[p.Path] = true
			}
		}
		return projectsMsg{projects: ps, missing: missing, err: err}
	}
}

func (m *Model) loadEditorsCmd() tea.Cmd {
	c := m.cli
	if c == nil {
		return nil
	}
	return func() tea.Msg {
		eds, err := c.Editors(context.Background())
		return editorsMsg{editors: eds, err: err}
	}
}

func (m *Model) scanEditorsCmd() tea.Cmd {
	goos, roots := m.goos, m.allRoots()
	return func() tea.Msg { return localEditorsMsg{local: unity.Scan(goos, roots)} }
}

func (m *Model) loadAuthCmd() tea.Cmd {
	c := m.cli
	if c == nil {
		return nil
	}
	return func() tea.Msg {
		info, err := c.AuthStatus(context.Background())
		return authMsg{info: info, err: err}
	}
}

func (m *Model) loadInstallPathCmd() tea.Cmd {
	c := m.cli
	if c == nil {
		return nil
	}
	return func() tea.Msg {
		p, err := c.InstallPath(context.Background())
		return installPathMsg{path: p, err: err}
	}
}

// loadReleasesCmd loads a page of releases. With reset the list starts over.
func (m *Model) loadReleasesCmd(reset bool) tea.Cmd {
	c := m.cli
	if c == nil || m.releases.loading {
		return nil
	}
	offset := m.releases.nextOffset
	if reset {
		offset = 0
	}
	stream, gen := m.releases.stream, m.releases.gen
	limit := releasesPerPage
	if m.releasesTab.filter() != "" {
		limit = releasesPerSearch
	}
	m.releases.loading = true
	load := func() tea.Msg {
		page, err := c.Releases(context.Background(), stream, offset, limit)
		return releasesMsg{stream: stream, gen: gen, offset: offset, page: page, err: err}
	}
	return tea.Batch(load, m.startSpinner())
}

// searchReleasesCmd loads the next page of releases while a filter is set, so
// that the filter looks through every release and not only the pages loaded.
func (m *Model) searchReleasesCmd() tea.Cmd {
	if m.releasesTab.filter() == "" || !m.releases.loaded || m.releases.nextOffset == 0 {
		return nil
	}
	return m.loadReleasesCmd(false)
}

// refreshAllCmd reloads everything.
func (m *Model) refreshAllCmd() tea.Cmd {
	cmds := []tea.Cmd{m.scanEditorsCmd(), probeOpenUPMCmd()}
	if m.cli != nil {
		cmds = append(cmds, m.loadProjectsCmd(), m.loadEditorsCmd(), m.loadAuthCmd(), m.loadInstallPathCmd())
		if m.releases.loaded {
			m.releases = releasesState{stream: m.releases.stream, gen: m.releases.gen + 1}
			cmds = append(cmds, m.loadReleasesCmd(true))
		}
	}
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
		return m.onReleases(msg)
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

func (m *Model) onReleases(msg releasesMsg) tea.Cmd {
	if msg.stream != m.releases.stream || msg.gen != m.releases.gen {
		return nil // the list started over while this was loading
	}
	m.releases.loading = false
	if msg.err != nil {
		m.toastf(toastError, "Could not list releases: %v", msg.err)
		m.releasesTab.empty = "Could not load releases. Press R to retry."
		return nil
	}
	items := msg.page.Items
	if msg.offset > 0 {
		items = append(append([]unitycli.Release(nil), m.releasesTab.Items()...), items...)
	}
	m.releases.loaded = true
	m.releases.nextOffset = msg.page.NextOffset
	m.releasesTab.empty = "No releases."
	m.releasesTab.SetItems(items)
	return m.searchReleasesCmd()
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
		if p = filepath.Clean(expandHome(p)); p != "." && !seen[p] {
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
