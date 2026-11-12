package tui

import (
	"context"
	"path/filepath"

	tea "charm.land/bubbletea/v2"

	"github.com/ManeFunction/lazyunity/internal/config"
	"github.com/ManeFunction/lazyunity/internal/unitycli"
)

// refreshTools rebuilds the Tools tab from what is known now.
func (m *Model) refreshTools() {
	var rows []toolRow

	cli := toolRow{id: "cli", label: "Unity CLI", run: (*Model).cliTool}
	switch m.cliStatus.State {
	case unitycli.Ready:
		cli.status, cli.tone = "ready · "+m.cliStatus.Version.String(), toneGood
		if m.cliUpdate.Available() {
			cli.status, cli.tone = "update available: "+m.cliUpdate.Latest+" · enter to update", toneWarn
		}
	case unitycli.Outdated:
		cli.status, cli.tone = "outdated · press enter to update", toneWarn
	default:
		cli.status, cli.tone = "not installed · press enter to install", toneWarn
	}
	rows = append(rows, cli)

	acct := toolRow{id: "account", label: "Account", run: (*Model).accountTool}
	switch {
	case m.cliStatus.State != unitycli.Ready:
		acct.status = "needs the Unity CLI"
	case !m.authKnown:
		acct.status = "checking…"
	case m.auth.LoggedIn:
		who := m.auth.User.Email
		if who == "" {
			who = m.auth.User.Name
		}
		acct.status, acct.tone = "signed in as "+who+" · enter to sign out", toneGood
	default:
		acct.status, acct.tone = "signed out · press enter to sign in", toneWarn
	}
	rows = append(rows, acct)

	rows = append(rows, toolRow{id: "license", label: "License", status: "press enter to check",
		run: func(m *Model) tea.Cmd { m.licenseMenu(); return nil }})
	rows = append(rows, m.openupmRow())

	editors := toolRow{id: "editorsdir", label: "Editors folder",
		run: func(m *Model) tea.Cmd { m.editorsFolderPrompt(); return nil }}
	if m.cliStatus.State != unitycli.Ready {
		editors.status = "needs the Unity CLI"
	} else {
		editors.status = displayPath(m.inv.installRoot()) + " · enter to change"
	}
	rows = append(rows, editors)

	dir := m.projectsDir()
	rows = append(rows,
		toolRow{id: "projectdir", label: "Default project folder", status: displayPath(dir) + " · enter to change",
			run: func(m *Model) tea.Cmd { m.projectDirPrompt(); return nil }},
		m.configRow(),
		toolRow{id: "about", label: "About", status: "lazyunity " + m.version + " · by Mane Function",
			run: func(m *Model) tea.Cmd { m.aboutPopup(); return nil }},
		toolRow{id: "donate", label: "Donate", status: "support lazyunity on GitHub Sponsors · enter to open",
			run: func(m *Model) tea.Cmd { return m.openURL(sponsorURL) }},
	)

	m.toolsTab.SetItems(rows)
}

// configRow edits config.yml; once it was changed while a task ran, enter
// restarts to apply it instead, and e still edits it.
func (m *Model) configRow() toolRow {
	path := displayPath(filepath.Join(m.configDir, "config.yml"))
	if m.configPending {
		return toolRow{id: "config", label: "Config file", status: "restart required · enter to restart, e to edit",
			tone: toneWarn, run: (*Model).restartForConfig}
	}
	return toolRow{id: "config", label: "Config file", status: path + " · enter to edit", run: (*Model).editConfig}
}

// projectsDir is where new projects are created: the choice made in the Tools
// tab, else the config, else the folder the official Hub uses, else the home
// folder.
func (m *Model) projectsDir() string {
	if d := m.st.DefaultProjectDir; d != "" {
		return d
	}
	if d := m.cfg.Projects.DefaultLocation; d != "" {
		return config.ExpandHome(d)
	}
	return homeDir()
}

// signInCmd signs the CLI in through the browser, as a task that can be cancelled.
func (m *Model) signInCmd() tea.Cmd {
	return m.withCLI(func(c unityCLI) tea.Cmd {
		return m.enqueue(&task{
			title: "Signing in to Unity",
			label: "sign in",
			works: []string{"signin"},
			msg:   "finish signing in in your browser",
			do:    func(ctx context.Context) error { return c.Login(ctx) },
			after: func(m *Model, err error) tea.Cmd {
				if err == nil {
					m.toastf(toastOK, "Signed in")
				}
				return m.loadAuthCmd()
			},
		})
	})
}
