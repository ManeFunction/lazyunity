package tui

import (
	"context"
	"os"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/ManeFunction/lazyunity/internal/unitycli"
)

// ---- messages ------------------------------------------------------------------

type projectsMsg struct {
	projects []unitycli.Project
	missing  map[string]bool
	err      error
}

type editorsMsg struct {
	editors []listedEditor
	err     error
}

// localEditorsMsg is a scan of the Editors folder.
type localEditorsMsg struct {
	found []foundEditor
}

type authMsg struct {
	info unitycli.AuthInfo
	err  error
}

// cliUpdateMsg is the answer of the CLI update check.
type cliUpdateMsg struct {
	info unitycli.UpdateInfo
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
	exists := m.exists
	return m.cliCmd(func(ctx context.Context, c unityCLI) tea.Msg {
		ps, err := c.Projects(ctx)
		return projectsMsg{projects: ps, missing: missingPaths(ps, exists), err: err}
	})
}

// missingPaths are the projects whose folder is not there.
func missingPaths(ps []unitycli.Project, exists func(path string) bool) map[string]bool {
	missing := map[string]bool{}
	for _, p := range ps {
		if !exists(p.Path) {
			missing[p.Path] = true
		}
	}
	return missing
}

// pathExists reports whether something is at path.
func pathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func (m *Model) loadEditorsCmd() tea.Cmd {
	goos := m.goos
	return m.cliCmd(func(ctx context.Context, c unityCLI) tea.Msg {
		eds, err := c.Editors(ctx)
		return editorsMsg{editors: listEditors(goos, eds), err: err}
	})
}

func (m *Model) scanEditorsCmd() tea.Cmd {
	goos, root, scan := m.goos, m.inv.installRoot(), m.scan
	return func() tea.Msg {
		return localEditorsMsg{found: scan(goos, root)}
	}
}

func (m *Model) loadAuthCmd() tea.Cmd {
	return m.cliCmd(func(ctx context.Context, c unityCLI) tea.Msg {
		info, err := c.AuthStatus(ctx)
		return authMsg{info: info, err: err}
	})
}

// checkCLIUpdateCmd asks whether a newer Unity CLI is out.
func (m *Model) checkCLIUpdateCmd() tea.Cmd {
	channel := m.cliChannel()
	return m.cliCmd(func(ctx context.Context, c unityCLI) tea.Msg {
		ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		info, err := c.CheckUpdate(ctx, channel)
		return cliUpdateMsg{info: info, err: err}
	})
}

func (m *Model) loadInstallPathCmd() tea.Cmd {
	return m.cliCmd(func(ctx context.Context, c unityCLI) tea.Msg {
		p, err := c.InstallPath(ctx)
		return installPathMsg{path: p, err: err}
	})
}

// source is something lazyunity shows that can be loaded again. reload maps
// each one to its load, so what "reload everything" means is written once.
type source uint

const (
	srcProjects    source = 1 << iota // the CLI's project list
	srcEditors                        // the CLI's Editor list
	srcDisk                           // the scan of the Editors folder
	srcAuth                           // who is signed in
	srcInstallPath                    // where new Editors go
	srcCLIUpdate                      // whether a newer Unity CLI is out
	srcOpenUPM                        // the OpenUPM CLI
	srcReleases                       // the Releases tab, once it was opened

	// srcCLI is what comes from the Unity CLI; it is all loaded again once the
	// CLI becomes usable.
	srcCLI = srcProjects | srcEditors | srcAuth | srcInstallPath | srcCLIUpdate
	// srcAll is everything.
	srcAll = srcCLI | srcDisk | srcOpenUPM | srcReleases
)

// reload loads the sources in s again.
func (m *Model) reload(s source) tea.Cmd {
	loads := []struct {
		src  source
		load func() tea.Cmd
	}{
		{srcProjects, m.loadProjectsCmd},
		{srcEditors, m.loadEditorsCmd},
		{srcDisk, m.scanEditorsCmd},
		{srcAuth, m.loadAuthCmd},
		{srcInstallPath, m.loadInstallPathCmd},
		{srcCLIUpdate, m.checkCLIUpdateCmd},
		{srcOpenUPM, m.probeOpenUPMCmd},
		{srcReleases, func() tea.Cmd { return m.releasesTab.refresh(m) }},
	}
	var cmds []tea.Cmd
	for _, l := range loads {
		if s&l.src != 0 {
			cmds = append(cmds, l.load())
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
		m.inv.setListed(msg.editors)
		m.refreshEditors()
		return m.registerStrays()
	case localEditorsMsg:
		m.inv.setFound(msg.found)
		m.refreshEditors()
		return m.registerStrays()
	case authMsg:
		if msg.err != nil {
			m.toastf(toastError, "Could not check the sign-in: %v", msg.err)
			return nil
		}
		m.auth, m.authKnown = msg.info, true
		m.refreshTools()
	case cliUpdateMsg:
		// a failed check (offline, say) is not worth a word: the row stays as it was
		if msg.err == nil {
			m.cliUpdate = msg.info
			m.refreshTools()
		}
	case installPathMsg:
		if msg.err == nil && msg.path != "" {
			m.inv.setInstallRoot(msg.path)
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
		return m.onOpenUPM(msg)
	case openupmLatestMsg:
		// like the CLI's, a failed check is not worth a word
		if msg.err == nil {
			m.openupmNew = msg.openupmLatest
			m.refreshTools()
		}
	case openupmInstalledMsg:
		return m.onOpenUPMInstalled(msg)
	case toastMsg:
		m.toastf(msg.kind, "%s", msg.text)
	case configEditedMsg:
		return m.onConfigEdited(msg)
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

// refreshEditors shows the inventory's Editors.
func (m *Model) refreshEditors() {
	m.editorsTab.SetItems(m.inv.editors())
}
