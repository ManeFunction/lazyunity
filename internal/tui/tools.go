package tui

import (
	"context"
	"path/filepath"

	tea "charm.land/bubbletea/v2"

	"github.com/ManeFunction/lazyunity/internal/unitycli"
)

func ptr[T any](v T) *T { return &v }

// refreshTools rebuilds the Tools tab from what is known now.
func (m *Model) refreshTools() {
	var rows []toolRow

	cli := toolRow{id: "cli", label: "Unity CLI"}
	switch m.cliStatus.State {
	case unitycli.Ready:
		cli.status, cli.good = "ready · "+m.cliStatus.Version.String(), ptr(true)
	case unitycli.Outdated:
		cli.status, cli.good = "outdated · press enter to update", ptr(false)
	default:
		cli.status, cli.good = "not installed · press enter to install", ptr(false)
	}
	rows = append(rows, cli)

	acct := toolRow{id: "account", label: "Account"}
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
		acct.status, acct.good = "signed in as "+who+" · enter to sign out", ptr(true)
	default:
		acct.status, acct.good = "signed out · press enter to sign in", ptr(false)
	}
	rows = append(rows, acct)

	rows = append(rows, toolRow{id: "license", label: "License", status: "press enter to check"})
	rows = append(rows, m.openupmRow())

	dir := m.projectsDir()
	rows = append(rows, toolRow{id: "projectdir", label: "Default project folder", status: displayPath(dir) + " · enter to change"})
	rows = append(rows, toolRow{id: "config", label: "Config file", status: displayPath(filepath.Join(m.configDir, "config.yml")) + " · enter to edit"})
	rows = append(rows, toolRow{id: "about", label: "About", status: "lazyunity " + m.version + " · by Mane Function"})
	rows = append(rows, toolRow{id: "donate", label: "Donate", status: "support lazyunity on GitHub Sponsors · enter to open"})

	m.toolsTab.SetItems(rows)
}

// projectsDir is where new projects are created: the choice made in the Tools
// tab, else the config, else the folder the official Hub uses, else the home
// folder.
func (m *Model) projectsDir() string {
	if d := m.st.DefaultProjectDir; d != "" {
		return d
	}
	if d := m.cfg.Projects.DefaultLocation; d != "" {
		return expandHome(d)
	}
	return homeDir()
}

// signInCmd signs the CLI in through the browser, as a task that can be cancelled.
func (m *Model) signInCmd() tea.Cmd {
	c := m.cli
	if c == nil {
		m.showCLIMissing()
		return nil
	}
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
}
