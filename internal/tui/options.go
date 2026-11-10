package tui

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/ManeFunction/lazyunity/internal/config"
	"github.com/ManeFunction/lazyunity/internal/launch"
	"github.com/ManeFunction/lazyunity/internal/unitycli"
)

// ---- roots ------------------------------------------------------------------------

func hasRoot(m *Model) bool { _, ok := m.rootsTab.Selected(); return ok }

func rootActions() []*action {
	on := func(f func(m *Model, r rootRow) tea.Cmd) func(*Model) tea.Cmd {
		return func(m *Model) tea.Cmd {
			r, ok := m.rootsTab.Selected()
			if !ok {
				return nil
			}
			return f(m, r)
		}
	}
	return []*action{
		{id: "roots.add", ctx: ctxRoots, keys: []string{"a"}, desc: "Add a folder to look for Editors in", hint: "Add",
			run: func(m *Model) tea.Cmd { m.addRootPrompt(); return nil }},
		{id: "roots.remove", ctx: ctxRoots, keys: []string{"d"}, desc: "Stop looking for Editors in this folder", hint: "Remove",
			enabled: hasRoot, run: on(func(m *Model, r rootRow) tea.Cmd { m.removeRoot(r); return nil })},
		{id: "roots.setInstall", ctx: ctxRoots, keys: []string{"space"}, desc: "Install new Editors into this folder", hint: "Install here",
			enabled: hasRoot, run: on(func(m *Model, r rootRow) tea.Cmd { return m.setInstallRoot(r) })},
		{id: "roots.openFolder", ctx: ctxRoots, keys: []string{"o"}, desc: "Open the folder in the file manager",
			enabled: hasRoot, run: on(func(m *Model, r rootRow) tea.Cmd { return m.openFolder(r.Path) })},
	}
}

func (m *Model) addRootPrompt() {
	m.push(&promptPopup{
		title: "Add an Editor folder", label: "A folder that contains Unity Editor folders, such as ~/Unity/Hub/Editor:",
		in: newInput(displayPath(homeDir()) + string(filepath.Separator)), complete: completeDir,
		validate: func(text string) string {
			if st, err := os.Stat(expandHome(text)); err != nil || !st.IsDir() {
				return "That folder does not exist"
			}
			return ""
		},
		onSubmit: func(m *Model, text string) tea.Cmd {
			path := filepath.Clean(expandHome(text))
			if !m.st.AddRoot(path) {
				m.toastf(toastInfo, "%s is already in the list", displayPath(path))
				return nil
			}
			m.saveState()
			m.refreshRoots()
			m.toastf(toastOK, "Added %s", displayPath(path))
			return m.scanEditorsCmd()
		},
	})
}

func (m *Model) removeRoot(r rootRow) {
	switch {
	case r.Install:
		m.toastf(toastWarn, "That is where new Editors are installed: pick another folder with space first")
		return
	case r.Source == "config":
		m.toastf(toastWarn, "That folder is set in config.yml (editors.extraRoots): remove it there")
		return
	}
	m.push(&confirmPopup{
		title: "Remove folder", yes: "Remove",
		message: fmt.Sprintf("Stop looking for Editors in %s?\n\nThe Editors in it stay installed and stay known to the Unity CLI.", displayPath(r.Path)),
		onYes: func(m *Model) tea.Cmd {
			m.st.RemoveRoot(r.Path)
			m.saveState()
			m.refreshRoots()
			return m.scanEditorsCmd()
		},
	})
}

func (m *Model) setInstallRoot(r rootRow) tea.Cmd {
	if r.Install {
		m.toastf(toastInfo, "New Editors are already installed there")
		return nil
	}
	if !r.Exists {
		m.toastf(toastError, "%s does not exist", displayPath(r.Path))
		return nil
	}
	c := m.cli
	return m.runCLI("Setting the install location", "New Editors now install into "+displayPath(r.Path),
		func(ctx context.Context, _ *unitycli.Client) error { return c.SetInstallPath(ctx, r.Path) },
		func(m *Model) tea.Cmd { return m.loadInstallPathCmd() })
}

// registerStrays tells the CLI about Editors found in the folders added by the
// user, so that its module commands and --editor-version work for them. It does
// this once for each Editor.
func (m *Model) registerStrays() tea.Cmd {
	c := m.cli
	if c == nil || !m.cliEditorsLoaded {
		return nil
	}
	known := map[string]bool{}
	for _, e := range m.cliEditors {
		known[e.Location] = true
	}
	var paths []string
	for _, e := range m.local {
		if !known[e.Location] && !m.registered[e.Location] {
			m.registered[e.Location] = true
			paths = append(paths, e.Location)
		}
	}
	if len(paths) == 0 {
		return nil
	}
	return func() tea.Msg {
		// Registering only makes the CLI's own commands work for these Editors. If
		// it is refused (Unity checks signatures on some systems) nothing is lost,
		// so it must not nag at every start; the command log keeps the evidence.
		if err := c.AddEditors(context.Background(), paths...); err != nil {
			return nil
		}
		eds, err := c.Editors(context.Background())
		return editorsMsg{editors: eds, err: err}
	}
}

// ---- tools --------------------------------------------------------------------------

func toolActions() []*action {
	return []*action{
		{id: "tools.run", ctx: ctxTools, keys: []string{"enter", "space"}, desc: "Run the selected tool", hint: "Run",
			enabled: func(m *Model) bool { _, ok := m.toolsTab.Selected(); return ok },
			run: func(m *Model) tea.Cmd {
				r, ok := m.toolsTab.Selected()
				if !ok {
					return nil
				}
				return m.runTool(r)
			}},
	}
}

func (m *Model) runTool(r toolRow) tea.Cmd {
	switch r.id {
	case "cli":
		return m.cliTool()
	case "account":
		return m.accountTool()
	case "license":
		m.licenseMenu()
	case "projectdir":
		m.projectDirPrompt()
	case "config":
		return m.editConfig()
	case "about":
		m.aboutPopup()
	case "openupm":
		return m.openupmTool()
	case "donate":
		return m.openURL(sponsorURL)
	}
	return nil
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
	c := m.cli
	menu := newMenu("Unity CLI "+m.cliStatus.Version.String(), []menuItem{
		{label: "Update (self-update)", desc: "Asks the CLI to update itself to the newest release.", run: func(m *Model) tea.Cmd {
			return m.enqueue(&task{
				title: "Updating the Unity CLI",
				label: "update the Unity CLI",
				works: []string{"cli"},
				do:    func(ctx context.Context) error { return c.SelfUpdate(ctx, "") },
				after: func(m *Model, err error) tea.Cmd { return m.reprobeCLI(err == nil) },
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
	m.refreshTools()
	m.toastf(toastOK, "Unity CLI %s is ready", msg.st.Version)
	return tea.Batch(m.loadProjectsCmd(), m.loadEditorsCmd(), m.loadAuthCmd(), m.loadInstallPathCmd())
}

func (m *Model) accountTool() tea.Cmd {
	if m.cli == nil {
		m.showCLIMissing()
		return nil
	}
	if !m.auth.LoggedIn {
		return m.signInCmd()
	}
	c := m.cli
	m.push(&confirmPopup{
		title: "Sign out", yes: "Sign out",
		message: "Sign out of " + m.auth.User.Email + "? Editors opened from lazyunity will start signed out until you sign in again.",
		onYes: func(m *Model) tea.Cmd {
			return m.runCLI("Signing out", "Signed out",
				func(ctx context.Context, _ *unitycli.Client) error { return c.Logout(ctx) },
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
	if m.cli == nil {
		m.showCLIMissing()
		return
	}
	c := m.cli
	menu := newMenu("License", []menuItem{
		{label: "Show the license status", desc: "The first time, Unity downloads its licensing client (about 110 MB).", run: func(m *Model) tea.Cmd {
			m.toastf(toastInfo, "Checking the license…")
			return func() tea.Msg {
				raw, err := c.LicenseStatus(context.Background())
				return licenseMsg{raw: raw, err: err}
			}
		}},
		{label: "Activate the free Personal license", desc: "For individuals and small teams under Unity's revenue limit.", run: func(m *Model) tea.Cmd {
			m.push(&confirmPopup{
				title: "Activate Unity Personal", yes: "Accept and activate",
				message: "Activating Unity Personal means you accept the Unity Personal license terms (https://unity.com/legal/licenses/unity-personal-license). Continue?",
				onYes: func(m *Model) tea.Cmd {
					return m.runCLI("Activating the license", "License activated",
						func(ctx context.Context, _ *unitycli.Client) error { return c.ActivatePersonalLicense(ctx) }, nil)
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
						func(ctx context.Context, _ *unitycli.Client) error { return c.ReturnLicense(ctx) }, nil)
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
		if e, ok := msg.err.(*unitycli.Error); ok && e.NeedsConfig() {
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
			if st, err := os.Stat(expandHome(text)); err != nil || !st.IsDir() {
				return "That folder does not exist"
			}
			return ""
		},
		onSubmit: func(m *Model, text string) tea.Cmd {
			m.st.DefaultProjectDir = filepath.Clean(expandHome(text))
			m.saveState()
			m.refreshTools()
			return nil
		},
	})
}

const configTemplate = "# lazyunity configuration. Every key is optional; see the README for the list.\n" +
	"# gui:\n#   sidePanelWidth: 0.45\n#   theme:\n#     activeBorderColor: [brightblue, bold]\n" +
	"\n" +
	"# The OpenUPM CLI, installed from the Tools tab. While this stays commented out,\n" +
	"# the official release is installed from npm. Uncomment openupm to clone, build\n" +
	"# and install it from a git repository instead; ref picks a branch or tag, the\n" +
	"# default branch otherwise.\n" +
	"#\n" +
	"# Recommended: the fork below, until its pull request is merged upstream\n" +
	"# (https://github.com/openupm/openupm-cli/pull/430, not accepted yet). It drops\n" +
	"# deprecated dependencies (npmlog, rimraf, old npm registry libraries), so npm\n" +
	"# no longer floods the install with deprecation warnings, and cuts the npm audit\n" +
	"# findings from 19 vulnerable production packages (one critical) to 5. It also\n" +
	"# fixes `openupm login` with a username and password, which sent the email as\n" +
	"# the password and logged in to registry.npmjs.org instead of the requested\n" +
	"# registry. It needs Node.js 20.17+ or 22.9+ (Node 18 is dropped).\n" +
	"# For the official repository: repo: " + config.OfficialOpenUPMRepo + "\n" +
	"# openupm:\n" +
	"#   repo: " + config.ForkOpenUPMRepo + "\n" +
	"#   ref: " + config.ForkOpenUPMRef + "\n"

func (m *Model) editConfig() tea.Cmd {
	path := filepath.Join(m.configDir, "config.yml")
	if _, err := os.Stat(path); err != nil {
		// give the editor something to open; a failure here only means it opens empty
		if os.MkdirAll(m.configDir, 0o755) == nil {
			os.WriteFile(path, []byte(configTemplate), 0o644)
		}
	}
	if tmpl := m.cfg.OS.Edit; tmpl != "" {
		if err := launch.Run(tmpl, path); err != nil {
			m.toastf(toastError, "Could not start the editor: %v", err)
		}
		return nil
	}
	editor := os.Getenv("VISUAL")
	if editor == "" {
		editor = os.Getenv("EDITOR")
	}
	words, err := launch.Split(editor)
	if err != nil || len(words) == 0 {
		// a popup rather than a toast: the path is long and must be readable
		m.push(&infoPopup{title: "Config file", lines: append(wrapText("Edit this file to change lazyunity's settings:", 70),
			"", path, "", "To open it from here, set $EDITOR, or os.edit in the file itself.")})
		return nil
	}
	return tea.ExecProcess(exec.Command(words[0], append(words[1:], path)...), func(err error) tea.Msg {
		if err != nil {
			return errorMsg{what: "Editor", err: err}
		}
		return toastMsg{kind: toastInfo, text: "Restart lazyunity to apply config changes"}
	})
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
	case ctxRoots:
		if r, ok := m.rootsTab.Selected(); ok {
			return r.Path, true
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
			return tea.ExecProcess(shellCommand(m.goos, m.cfg.OS.Shell, text, dir), func(err error) tea.Msg { return nil })
		},
	})
}

func shellCommand(goos, shell, text, dir string) *exec.Cmd {
	var cmd *exec.Cmd
	if goos == "windows" {
		cmd = exec.Command("cmd", "/C", text+" & echo. & pause")
	} else {
		if shell == "" {
			shell = os.Getenv("SHELL")
		}
		if shell == "" {
			shell = "/bin/sh"
		}
		cmd = exec.Command(shell, "-c", text+`; printf '\nPress enter to return to lazyunity... '; read _`)
	}
	cmd.Dir = dir
	return cmd
}
