package tui

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/ManeFunction/lazyunity/internal/config"
	"github.com/ManeFunction/lazyunity/internal/launch"
	"github.com/ManeFunction/lazyunity/internal/unitycli"
)

// ---- editors folder ----------------------------------------------------------------

// editorsFolderPrompt changes the Editors folder: the one folder new Editors
// are installed into and looked for in, which the Unity CLI keeps.
func (m *Model) editorsFolderPrompt() {
	if !m.needCLI() {
		return
	}
	m.push(&promptPopup{
		title: "Editors folder", label: "New Editors install into this folder, and installed Editors are looked for here:",
		in: newInput(displayPath(m.inv.installRoot()) + string(filepath.Separator)), complete: completeDir,
		validate: func(text string) string {
			if st, err := os.Stat(config.ExpandHome(text)); err != nil || !st.IsDir() {
				return "That folder does not exist"
			}
			return ""
		},
		onSubmit: func(m *Model, text string) tea.Cmd {
			path := filepath.Clean(config.ExpandHome(text))
			if path == m.inv.installRoot() {
				m.toastf(toastInfo, "Editors already install into that folder")
				return nil
			}
			return m.runCLI("Setting the Editors folder", "New Editors now install into "+displayPath(path),
				func(ctx context.Context, c unityCLI) error { return c.SetInstallPath(ctx, path) },
				func(m *Model) tea.Cmd { return m.loadInstallPathCmd() })
		},
	})
}

// registerStrays tells the CLI about Editors it found on disk but does not
// list, so that its module commands and --editor-version work for them. It
// does this once for each Editor.
func (m *Model) registerStrays() tea.Cmd {
	paths, goos := m.inv.strays(), m.goos
	if len(paths) == 0 {
		return nil
	}
	cmd := m.cliCmd(func(ctx context.Context, c unityCLI) tea.Msg {
		// Registering only makes the CLI's own commands work for these Editors. If
		// it is refused (Unity checks signatures on some systems) nothing is lost,
		// so it must not nag at every start; the command log keeps the evidence.
		if err := c.AddEditors(ctx, paths...); err != nil {
			return nil
		}
		eds, err := c.Editors(ctx)
		return editorsMsg{editors: listEditors(goos, eds), err: err}
	})
	if cmd != nil {
		m.inv.told(paths)
	}
	return cmd
}

// ---- tools --------------------------------------------------------------------------

func toolActions() []*action {
	on := onSelected(func(m *Model) (toolRow, bool) { return m.toolsTab.Selected() })
	return []*action{
		on(&action{id: "tools.run", ctx: ctxTools, keys: []string{"enter", "space"}, desc: "Run the selected tool", hint: "Run"},
			func(m *Model, r toolRow) tea.Cmd { return r.run(m) }),
		{id: "tools.editConfig", ctx: ctxTools, keys: []string{"e"}, desc: "Edit the config file", hint: "Edit config",
			run: (*Model).editConfig},
	}
}

type cliInstalledMsg struct{ err error }

type cliStatusMsg struct {
	st      unitycli.Status
	updated bool
}

// cliTool installs or updates the Unity CLI.
func (m *Model) cliTool() tea.Cmd {
	install := func(m *Model) tea.Cmd {
		if m.refuseWhileBusy("installing the Unity CLI") {
			return nil
		}
		note := ""
		if m.goos != "windows" {
			note = "\n\nThe installer may ask for your sudo password a few times."
		}
		m.push(&confirmPopup{
			title: "Install the Unity CLI", yes: "Install",
			message: "Run Unity's official installer (https://unity.com/install" + map[bool]string{true: ".ps1", false: ".sh"}[m.goos == "windows"] +
				")?" + note,
			onYes: func(m *Model) tea.Cmd {
				return tea.ExecProcess(unitycli.InstallCommand(m.goos), func(err error) tea.Msg { return cliInstalledMsg{err: err} })
			},
		})
		return nil
	}
	if m.cliStatus.State != unitycli.Ready {
		return install(m)
	}
	updateDesc := "Asks the CLI to update itself to the newest release."
	if m.cliUpdate.Available() {
		updateDesc = "Updates the CLI to " + m.cliUpdate.Latest + "."
	}
	menu := newMenu("Unity CLI "+m.cliStatus.Version.String(), []menuItem{
		{label: "Update (self-update)", desc: updateDesc, run: func(m *Model) tea.Cmd {
			return m.withCLI(func(c unityCLI) tea.Cmd {
				return m.enqueue(&task{
					title: "Updating the Unity CLI",
					label: "update the Unity CLI",
					works: []string{"cli"},
					do:    func(ctx context.Context) error { return c.SelfUpdate(ctx, "") },
					after: func(m *Model, err error) tea.Cmd { return m.reprobeCLI(err == nil) },
				})
			})
		}},
		{label: "Reinstall with the official installer", run: install},
	})
	menu.wanted = 44
	m.push(menu)
	return nil
}

// reprobeCLI looks for the CLI again, after it was installed or updated.
func (m *Model) reprobeCLI(updated bool) tea.Cmd {
	env, configured := m.env, m.cfg.Unity.CLIPath
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return cliStatusMsg{st: unitycli.Probe(ctx, env, configured, nil), updated: updated}
	}
}

func (m *Model) onCLIStatus(msg cliStatusMsg) tea.Cmd {
	m.cliStatus = msg.st
	if msg.st.State != unitycli.Ready {
		m.cli = nil
		m.refreshTools()
		m.toastf(toastWarn, "The Unity CLI is not usable yet")
		return nil
	}
	m.cli = unitycli.New(msg.st.Path)
	m.cliUpdate = unitycli.UpdateInfo{}
	m.refreshTools()
	m.toastf(toastOK, "Unity CLI %s is ready", msg.st.Version)
	return tea.Batch(m.loadProjectsCmd(), m.loadEditorsCmd(), m.loadAuthCmd(), m.loadInstallPathCmd(), m.checkCLIUpdateCmd())
}

func (m *Model) accountTool() tea.Cmd {
	if !m.needCLI() {
		return nil
	}
	if !m.auth.LoggedIn {
		return m.signInCmd()
	}
	m.push(&confirmPopup{
		title: "Sign out", yes: "Sign out",
		message: "Sign out of " + m.auth.User.Email + "? Editors opened from lazyunity will start signed out until you sign in again.",
		onYes: func(m *Model) tea.Cmd {
			return m.runCLI("Signing out", "Signed out",
				func(ctx context.Context, c unityCLI) error { return c.Logout(ctx) },
				func(m *Model) tea.Cmd { return m.loadAuthCmd() })
		},
	})
	return nil
}

type licenseMsg struct {
	raw json.RawMessage
	err error
}

func (m *Model) licenseMenu() {
	if !m.needCLI() {
		return
	}
	menu := newMenu("License", []menuItem{
		{label: "Show the license status", desc: "The first time, Unity downloads its licensing client (about 110 MB).", run: func(m *Model) tea.Cmd {
			return m.withCLI(func(c unityCLI) tea.Cmd {
				m.toastf(toastInfo, "Checking the license…")
				return func() tea.Msg {
					raw, err := c.LicenseStatus(context.Background())
					return licenseMsg{raw: raw, err: err}
				}
			})
		}},
		{label: "Activate the free Personal license", desc: "For individuals and small teams under Unity's revenue limit.", run: func(m *Model) tea.Cmd {
			m.push(&confirmPopup{
				title: "Activate Unity Personal", yes: "Accept and activate",
				message: "Activating Unity Personal means you accept the Unity Personal license terms (https://unity.com/legal/licenses/unity-personal-license). Continue?",
				onYes: func(m *Model) tea.Cmd {
					return m.runCLI("Activating the license", "License activated",
						func(ctx context.Context, c unityCLI) error { return c.ActivatePersonalLicense(ctx) }, nil)
				},
			})
			return nil
		}},
		{label: "Return the license", desc: "Frees the license so it can be used on another computer.", run: func(m *Model) tea.Cmd {
			m.push(&confirmPopup{
				title: "Return the license", yes: "Return", danger: true,
				message: "Return the active license? Editors cannot be used on this computer until it is activated again.",
				onYes: func(m *Model) tea.Cmd {
					return m.runCLI("Returning the license", "License returned",
						func(ctx context.Context, c unityCLI) error { return c.ReturnLicense(ctx) }, nil)
				},
			})
			return nil
		}},
	})
	menu.wanted = 52
	m.push(menu)
}

func (m *Model) onLicense(msg licenseMsg) {
	var lines []string
	if len(msg.raw) > 0 {
		var buf bytes.Buffer
		if json.Indent(&buf, msg.raw, "", "  ") == nil {
			lines = strings.Split(buf.String(), "\n")
		}
	}
	if msg.err != nil {
		var e *unitycli.Error
		if errors.As(msg.err, &e) && e.NeedsConfig() {
			lines = append([]string{"No active license. Choose \"Activate the free Personal license\" in this menu.", ""}, lines...)
		} else {
			m.showError("License status", msg.err)
			return
		}
	}
	if len(lines) == 0 {
		lines = []string{"The CLI returned no license information."}
	}
	m.push(&infoPopup{title: "License status", lines: lines})
}

func (m *Model) projectDirPrompt() {
	m.push(&promptPopup{
		title: "Default project folder", label: "New projects are created in this folder:",
		in: newInput(displayPath(m.projectsDir())), complete: completeDir,
		validate: func(text string) string {
			if st, err := os.Stat(config.ExpandHome(text)); err != nil || !st.IsDir() {
				return "That folder does not exist"
			}
			return ""
		},
		onSubmit: func(m *Model, text string) tea.Cmd {
			m.st.DefaultProjectDir = filepath.Clean(config.ExpandHome(text))
			m.saveState()
			m.refreshTools()
			return nil
		},
	})
}

func (m *Model) editConfig() tea.Cmd {
	path := filepath.Join(m.configDir, "config.yml")
	if _, err := os.Stat(path); err != nil {
		// give the editor something to open; a failure here only means it opens empty
		if os.MkdirAll(m.configDir, 0o755) == nil {
			os.WriteFile(path, []byte(configTemplate()), 0o644)
		}
	}
	before, _ := os.ReadFile(path)
	return m.edit(path, m.configDir, func(m *Model) {
		// a popup rather than a toast: the path is long and must be readable
		m.push(&infoPopup{title: "Config file", lines: append(wrapText("Edit this file to change lazyunity's settings:", 70),
			"", path, "", "To open it from here, set $EDITOR, or os.edit in the file itself.")})
	}, configEditedMsg{before: before})
}

// configEditedMsg comes once the terminal editor on config.yml has quit;
// before is what the file held when it was opened.
type configEditedMsg struct{ before []byte }

// onConfigEdited relaunches lazyunity to apply a changed config. While a task
// runs it only marks the Config file row, which then restarts on enter.
func (m *Model) onConfigEdited(msg configEditedMsg) tea.Cmd {
	after, err := os.ReadFile(filepath.Join(m.configDir, "config.yml"))
	if err != nil || bytes.Equal(after, msg.before) {
		return nil
	}
	if !m.configLoads() {
		return nil
	}
	if m.busy() {
		m.configPending = true
		m.refreshTools()
		m.toastf(toastInfo, "Config changed: restart from Tools › Config file to apply it")
		return nil
	}
	m.relaunch()
	return nil
}

// configLoads reports whether config.yml loads, showing why when it does not.
func (m *Model) configLoads() bool {
	_, err := config.Load(m.configDir)
	if err != nil {
		m.push(&infoPopup{title: "Config not applied", lines: append(wrapText("The edited config.yml does not load, so lazyunity keeps the settings it started with:", 70),
			append([]string{""}, wrapText(err.Error(), 70)...)...)})
	}
	return err == nil
}

// restartForConfig applies a config changed while a task ran, asking first
// when a task would still be cancelled by it.
func (m *Model) restartForConfig() tea.Cmd {
	if !m.configLoads() {
		return nil
	}
	if m.busy() {
		m.push(&confirmPopup{
			title: "Restart", yes: "Restart", danger: true,
			message: "A task is still running. Restarting to apply the config cancels it.",
			onYes: func(m *Model) tea.Cmd {
				m.cancelTasks()
				m.relaunch()
				return nil
			},
		})
		return nil
	}
	m.relaunch()
	return nil
}

// relaunch quits for main to start lazyunity again with the config reloaded.
func (m *Model) relaunch() {
	m.restart = true
	m.quit = true
}

func (m *Model) aboutPopup() {
	lines := []string{
		"lazyunity " + m.version,
		"A lazy-style terminal interface for Unity, built on the Unity CLI.",
		"",
		fmt.Sprintf("%-16s %s", "Author", authorName),
		fmt.Sprintf("%-16s %s", "Repository", repoURL),
		fmt.Sprintf("%-16s %s", "Donate", sponsorURL),
		"",
		fmt.Sprintf("%-16s %s", "Unity CLI", m.cliStatus.Path+" ("+m.cliStatus.Version.String()+", "+m.cliStatus.State.String()+")"),
		fmt.Sprintf("%-16s %s", "Config", filepath.Join(m.configDir, "config.yml")),
		fmt.Sprintf("%-16s %s", "State", m.st.Path()),
	}
	m.push(&infoPopup{title: "About lazyunity", lines: lines})
}

// ---- command log, shell, copy, cancel -------------------------------------------------

func (m *Model) openCommandLog() {
	var entries []unitycli.LogEntry
	if m.cli != nil {
		entries = m.cli.Log()
	}
	var items []menuItem
	for i := len(entries) - 1; i >= 0; i-- {
		e := entries[i]
		status := fmt.Sprintf("exit %d · %s", e.ExitCode, e.Duration.Round(10*time.Millisecond))
		items = append(items, menuItem{
			label: truncate(e.Command(), 70), right: status,
			run: func(m *Model) tea.Cmd { m.showLogEntry(e); return nil },
		})
	}
	menu := newMenu("Command log", items)
	menu.emptyMsg = "No Unity CLI commands have run yet"
	menu.wanted = 100
	m.push(menu)
}

func (m *Model) showLogEntry(e unitycli.LogEntry) {
	lines := wrapText(e.Command(), 90)
	lines = append(lines, "", fmt.Sprintf("started %s · took %s · exit code %d", e.Time.Format("15:04:05"), e.Duration.Round(time.Millisecond), e.ExitCode))
	if e.Stdout != "" {
		lines = append(lines, "", "Output:")
		for _, l := range strings.Split(e.Stdout, "\n") {
			lines = append(lines, wrapText(l, 90)...)
		}
	}
	if e.Stderr != "" {
		lines = append(lines, "", "Errors:")
		for _, l := range strings.Split(e.Stderr, "\n") {
			lines = append(lines, wrapText(l, 90)...)
		}
	}
	m.push(&infoPopup{title: "Command", lines: lines})
}

// selectedPath is the path of whatever is selected in the focused list.
func (m *Model) selectedPath() (string, bool) {
	switch m.focusCtx() {
	case ctxProjects:
		if r, ok := m.projectsTab.Selected(); ok {
			return r.Path, true
		}
	case ctxEditors:
		if r, ok := m.editorsTab.Selected(); ok {
			return editorDir(r), true
		}
	}
	return "", false
}

func (m *Model) copySelectedPath() tea.Cmd {
	p, ok := m.selectedPath()
	if !ok {
		m.toastf(toastInfo, "Nothing here has a path to copy")
		return nil
	}
	m.toastf(toastOK, "Copied %s", displayPath(p))
	return tea.SetClipboard(p)
}

// shellPrompt runs a command in the selected project's folder, with the
// terminal handed over, then waits for Enter.
func (m *Model) shellPrompt() {
	dir, ok := m.selectedPath()
	if !ok || m.focusCtx() == ctxEditors {
		dir = homeDir()
	}
	m.push(&promptPopup{
		title: "Run a shell command", label: "Runs in " + displayPath(dir) + ":", in: newInput(""),
		validate: func(s string) string {
			if s == "" {
				return "Type a command"
			}
			return ""
		},
		onSubmit: func(m *Model, text string) tea.Cmd {
			cmd := launch.ShellCommand(m.goos, m.cfg.OS.Shell, text, dir, m.getenv)
			return tea.ExecProcess(cmd, func(err error) tea.Msg { return nil })
		},
	})
}
