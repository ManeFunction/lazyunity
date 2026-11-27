package tui

import (
	"context"
	"errors"
	"fmt"

	tea "charm.land/bubbletea/v2"

	"github.com/ManeFunction/lazyunity/internal/unitycli"
)

// The Unity CLI row and its menus follow the verified range (unitycli.MinVersion
// to unitycli.MaxLine): an update inside it is offered on the row, one beyond
// it only in the menu and after a warning. A CLI beyond the range that breaks a
// launch load puts lazyunity in safe mode, which shows only what can bring a
// verified CLI back.

type cliInstalledMsg struct{ err error }

type cliStatusMsg struct {
	st      unitycli.Status
	updated bool
}

// verifiedCLIMsg is what the CDN said about the newest verified CLI.
type verifiedCLIMsg struct {
	v  unitycli.Version
	ok bool
}

// safeMode is why lazyunity runs in safe mode.
type safeMode struct {
	// forced is set when lazyunity was started with --safe, for a CLI that
	// breaks it without a launch load failing; what and err are then empty.
	forced bool
	what   string // what could not be done, "list the projects"
	err    error
}

// latestVerifiedCmd asks Unity's CDN for the newest verified CLI release.
func (m *Model) latestVerifiedCmd() tea.Cmd {
	h := m.host
	return func() tea.Msg {
		v, ok := h.LatestVerifiedCLI(context.Background())
		return verifiedCLIMsg{v: v, ok: ok}
	}
}

// verifiedTarget is the version to go back or install to: the newest verified
// release, or the oldest one while the CDN has not answered.
func (m *Model) verifiedTarget() string {
	if m.cliVerified.Verified() {
		return m.cliVerified.String()
	}
	return unitycli.MinVersion.String()
}

// verifiedUpdate is the newest verified CLI that is newer than the installed
// one: the feed's newest when it is verified, else the newest verified release.
func (m *Model) verifiedUpdate() (unitycli.Version, bool) {
	cur := m.cliStatus.Version
	var best unitycli.Version
	found := false
	candidates := []unitycli.Version{m.cliVerified}
	if v, ok := unitycli.ParseVersion(m.cliUpdate.Latest); ok {
		candidates = append(candidates, v)
	}
	for _, v := range candidates {
		if v.Verified() && v.Compare(cur) > 0 && (!found || v.Compare(best) > 0) {
			best, found = v, true
		}
	}
	return best, found
}

// unverifiedUpdate is the feed's newest CLI when it is newer than both the
// installed one and the verified range.
func (m *Model) unverifiedUpdate() (unitycli.Version, bool) {
	v, ok := unitycli.ParseVersion(m.cliUpdate.Latest)
	if !ok || !v.BeyondVerified() || v.Compare(m.cliStatus.Version) <= 0 {
		return unitycli.Version{}, false
	}
	return v, true
}

// cliRow is the Unity CLI line of the Tools tab.
func (m *Model) cliRow() toolRow {
	row := toolRow{id: "cli", label: "Unity CLI", run: (*Model).cliTool}
	st := m.cliStatus
	ver, rng := st.Version.String(), unitycli.VerifiedRange()
	switch {
	case m.safe != nil && m.safe.forced:
		what := ver
		switch {
		case st.State == unitycli.Missing:
			what = "not installed"
		case st.Version == (unitycli.Version{}):
			what = "not working"
		}
		row.status, row.tone = "safe mode · "+what+" · enter for options", toneBad
	case m.safe != nil:
		row.status, row.tone = "safe mode · "+ver+" does not work with lazyunity · enter to downgrade", toneBad
	case st.State == unitycli.Missing:
		row.status, row.tone = "not installed · press enter to install", toneWarn
	case st.State == unitycli.Outdated && st.Version == (unitycli.Version{}):
		row.status, row.tone = "not working · press enter to reinstall", toneBad
	case st.State == unitycli.Outdated:
		row.status, row.tone = "update required: "+ver+" is older than "+rng+" · enter to update", toneBad
	case st.Unverified():
		row.status, row.tone = ver+" · not verified · enter for options", toneWarn
	default:
		row.status, row.tone = "ready · "+ver, toneGood
		if m.cliChannel() == unitycli.ChannelBeta {
			row.status += " · beta feed"
		}
		if v, ok := m.verifiedUpdate(); ok {
			row.status, row.tone = "update available: "+v.String()+" · enter to update", toneWarn
		}
	}
	return row
}

// cliTool is enter on the Unity CLI row.
func (m *Model) cliTool() tea.Cmd {
	switch {
	case m.safe != nil:
		m.safeModeMenu()
		return nil
	case m.cliStatus.State != unitycli.Ready:
		return m.installCLI()
	}

	st := m.cliStatus
	channel := m.cliChannel()
	var items []menuItem
	if v, ok := m.verifiedUpdate(); ok {
		items = append(items, menuItem{label: "Update to " + v.String(),
			desc: "Updates the CLI to " + v.String() + ", a version lazyunity is verified with.",
			run:  m.moveCLI("Updating the Unity CLI to "+v.String(), unitycli.UpdateOptions{Channel: channel, Target: v.String()})})
	} else {
		why := "No newer version lazyunity is verified with (" + unitycli.VerifiedRange() + ") is out."
		if m.cliUpdate.Latest == "" {
			why = "The update check has not answered."
		}
		items = append(items, menuItem{label: "Update", disabled: why})
	}
	if v, ok := m.unverifiedUpdate(); ok {
		items = append(items, menuItem{label: "Not verified update " + v.String() + " available",
			desc: "lazyunity " + m.version + " is not verified to work with Unity CLI " + v.String() + ". If the CLI's commands changed, parts of lazyunity may stop working. Update only if you need this version.",
			run:  func(m *Model) tea.Cmd { m.confirmUnverified(v, channel); return nil }})
	}
	if st.Unverified() {
		target := m.verifiedTarget()
		items = append(items, menuItem{label: "Go back to verified " + target,
			desc: "Moves the CLI to " + target + ", the newest version lazyunity is verified with.",
			run:  m.moveCLI("Moving the Unity CLI to "+target, unitycli.UpdateOptions{Target: target})})
	}

	other, feed := unitycli.ChannelBeta, "beta"
	switchDesc := "Follows the CLI's beta builds and installs the newest one, if it is newer. It may be newer than the versions lazyunity is verified with."
	if channel == unitycli.ChannelBeta {
		other, feed = unitycli.ChannelStable, "release"
		switchDesc = "Goes back to the CLI's releases and installs the newest one, which may be older than this beta."
	}
	items = append(items,
		menuItem{label: "Switch to the " + feed + " feed", desc: switchDesc, run: func(m *Model) tea.Cmd { return m.switchFeed(other, feed) }},
		menuItem{label: "Reinstall with the official installer", desc: m.installerDesc(), run: (*Model).installCLI})
	menu := newMenu("Unity CLI "+st.Version.String(), items)
	menu.wanted = 48
	m.push(menu)
	return nil
}

// confirmUnverified asks before installing a CLI beyond the verified range.
func (m *Model) confirmUnverified(v unitycli.Version, channel string) {
	m.push(&confirmPopup{
		title: "Not verified Unity CLI", yes: "Update anyway", danger: true,
		message: fmt.Sprintf("lazyunity %s is verified with Unity CLI %s. %s is newer and was not checked: "+
			"commands lazyunity uses may have changed, and parts of lazyunity may stop working.\n\n"+
			"If they do, lazyunity starts in safe mode and offers to go back to %s. Update at your own risk?",
			m.version, unitycli.VerifiedRange(), v, m.verifiedTarget()),
		onYes: m.moveCLI("Updating the Unity CLI to "+v.String(), unitycli.UpdateOptions{Channel: channel, Target: v.String()}),
	})
}

// switchFeed moves the CLI to another update feed; the beta feed is asked
// about first, as its newest build may be beyond the verified range.
func (m *Model) switchFeed(channel, feed string) tea.Cmd {
	run := func(m *Model) tea.Cmd {
		return m.cliTask("Switching the Unity CLI to the "+feed+" feed", "switch the Unity CLI to the "+feed+" feed",
			func(ctx context.Context, c unityCLI) error {
				return c.SelfUpdate(ctx, unitycli.UpdateOptions{Channel: channel})
			},
			func(m *Model, err error) tea.Cmd {
				if err != nil {
					return nil
				}
				m.st.CLIChannel = channel
				m.saveState()
				return m.reprobeCLI(true)
			})
	}
	if channel != unitycli.ChannelBeta {
		return run(m)
	}
	m.push(&confirmPopup{
		title: "Switch to the beta feed", yes: "Switch",
		message: "The beta feed installs the newest beta of the Unity CLI. It may be newer than the versions lazyunity is verified with (" +
			unitycli.VerifiedRange() + "), and parts of lazyunity may stop working with it. Switch anyway?",
		onYes: run,
	})
	return nil
}

// moveCLI installs another version of the CLI with self-update.
func (m *Model) moveCLI(title string, o unitycli.UpdateOptions) func(m *Model) tea.Cmd {
	return func(m *Model) tea.Cmd {
		return m.cliTask(title, "update the Unity CLI",
			func(ctx context.Context, c unityCLI) error { return c.SelfUpdate(ctx, o) },
			(*Model).afterCLIMove)
	}
}

func (m *Model) rollbackCLI() tea.Cmd {
	return m.cliTask("Rolling the Unity CLI back", "roll the Unity CLI back",
		func(ctx context.Context, c unityCLI) error { return c.Rollback(ctx) },
		(*Model).afterCLIMove)
}

// afterCLIMove looks at the CLI again once it changed. A move that failed in
// safe mode changed nothing, so the safe mode stays as it is.
func (m *Model) afterCLIMove(err error) tea.Cmd {
	if err != nil && m.safe != nil {
		return nil
	}
	return m.reprobeCLI(err == nil)
}

// cliTask runs a change to the CLI itself as a task. It works in safe mode,
// where other uses of the CLI are refused.
func (m *Model) cliTask(title, label string, do func(ctx context.Context, c unityCLI) error, after func(m *Model, err error) tea.Cmd) tea.Cmd {
	c := m.cli
	if c == nil {
		m.showCLIMissing()
		return nil
	}
	return m.enqueue(&task{
		title: title, label: label, works: []string{workCLI},
		do:    func(ctx context.Context) error { return do(ctx, c) },
		after: after,
	})
}

// installerURL is the official installer for this system.
func (m *Model) installerURL() string {
	if m.goos == "windows" {
		return "https://unity.com/install.ps1"
	}
	return "https://unity.com/install.sh"
}

// installerDesc says what the official installer will install.
func (m *Model) installerDesc() string {
	if unitycli.PinnedInstall(m.goos) {
		return "Installs Unity CLI " + m.verifiedTarget() + " with Unity's official installer."
	}
	return "Installs the newest Unity CLI with Unity's official installer, which cannot install another version."
}

// installCLI runs Unity's official installer, pinned to the newest verified
// version where the installer can be pinned.
func (m *Model) installCLI() tea.Cmd {
	if m.refuseFake("installing the Unity CLI") || m.refuseWhileBusy("installing the Unity CLI") {
		return nil
	}
	pin := ""
	var msg string
	if unitycli.PinnedInstall(m.goos) {
		pin = m.verifiedTarget()
		msg = "Install Unity CLI " + pin + ", the newest version lazyunity is verified with, with Unity's official installer (" +
			m.installerURL() + ")?\n\nThe installer may ask for your sudo password a few times."
	} else {
		msg = "Run Unity's official installer (" + m.installerURL() + ")?\n\nIt installs the newest Unity CLI and cannot install another. " +
			"lazyunity is verified with " + unitycli.VerifiedRange() + ": if the newest is newer, the Unity CLI row marks it as not verified, and its menu can move it back."
	}
	m.push(&confirmPopup{
		title: "Install the Unity CLI", yes: "Install", message: msg,
		onYes: func(m *Model) tea.Cmd {
			return tea.ExecProcess(unitycli.InstallCommand(m.goos, pin), func(err error) tea.Msg { return cliInstalledMsg{err: err} })
		},
	})
	return nil
}

// ---- safe mode ---------------------------------------------------------------

// breaksLazyunity reports whether a failed load means the CLI no longer speaks
// the way lazyunity expects, rather than that something around it failed: the
// network, the sign-in, a cancel.
func breaksLazyunity(err error) bool {
	var e *unitycli.Error
	if errors.As(err, &e) {
		return !(e.Network() || e.NotSignedIn() || e.Cancelled() || e.InstallLocked())
	}
	return err != nil // its output could not be read
}

// checkBreakage puts lazyunity in safe mode when a launch load failed with a
// CLI beyond the verified range. It reports whether lazyunity is in safe mode.
func (m *Model) checkBreakage(what string, err error) bool {
	if m.safe != nil {
		return true
	}
	if !m.cliStatus.Unverified() || !breaksLazyunity(err) {
		return false
	}
	m.enterSafeMode(&safeMode{what: what, err: err})
	m.refreshTools()
	m.toolsTab.Top()
	m.relayout()
	target := m.verifiedTarget()
	m.push(&confirmPopup{
		title: "Safe mode", yes: "Downgrade to " + target, no: "Later", danger: true,
		message: m.safeModeText() + "\n\nDowngrade the CLI to " + target + ", the newest version lazyunity is verified with?",
		onYes:   m.moveCLI("Downgrading the Unity CLI to "+target, unitycli.UpdateOptions{Target: target}),
	})
	return true
}

// enterSafeMode cuts the interface down to the CLI tools.
func (m *Model) enterSafeMode(s *safeMode) {
	m.safe = s
	m.filter.active = false
	m.mode, m.focus = modeSafe, panelOptions
}

// safeModeText explains the safe mode.
func (m *Model) safeModeText() string {
	if m.safe.forced {
		return "lazyunity was started with --safe: only the Unity CLI tools are on screen, and lazyunity uses the Unity CLI only to change its version. " +
			"lazyunity " + m.version + " is verified with Unity CLI " + unitycli.VerifiedRange() + "."
	}
	return fmt.Sprintf("Unity CLI %s is newer than the versions lazyunity %s is verified with (%s), and lazyunity could not %s with it:\n\n%v\n\n"+
		"Until the CLI works again, lazyunity runs in safe mode: only the Unity CLI tools are on screen.",
		m.cliStatus.Version, m.version, unitycli.VerifiedRange(), m.safe.what, m.safe.err)
}

// safeModeMenu is enter on the Unity CLI row in safe mode.
func (m *Model) safeModeMenu() {
	target := m.verifiedTarget()
	var items []menuItem
	// --safe can be used with any CLI, even the target itself or none
	if tv, _ := unitycli.ParseVersion(target); m.cli != nil && m.cliStatus.Version != tv {
		verb, doing := "Downgrade", "Downgrading"
		if m.cliStatus.Version.Compare(tv) < 0 {
			verb, doing = "Update", "Updating"
		}
		items = append(items, menuItem{label: verb + " to " + target, desc: "Moves the CLI to " + target + ", the newest version lazyunity is verified with.",
			run: m.moveCLI(doing+" the Unity CLI to "+target, unitycli.UpdateOptions{Target: target})})
	}
	if m.cli != nil {
		items = append(items, menuItem{label: "Roll back to the previous version", desc: "Puts back the CLI that the last update replaced.", run: (*Model).rollbackCLI})
	}
	if unitycli.PinnedInstall(m.goos) {
		items = append(items, menuItem{label: "Reinstall " + target + " with the official installer",
			desc: "For a CLI too broken to downgrade itself.", run: (*Model).installCLI})
	}
	why := "What went wrong"
	if m.safe.forced {
		why = "About safe mode"
	}
	items = append(items, menuItem{label: why, run: func(m *Model) tea.Cmd {
		m.push(&infoPopup{title: "Safe mode", lines: wrapText(m.safeModeText(), 70)})
		return nil
	}})
	if m.safe.forced {
		items = append(items, menuItem{label: "Leave safe mode",
			desc: "Brings the whole interface back and loads everything with the Unity CLI as it is now.",
			run: func(m *Model) tea.Cmd {
				m.leaveSafeMode()
				m.refreshTools()
				return m.reload(srcAll &^ srcDisk)
			}})
	}
	menu := newMenu("Unity CLI "+m.cliStatus.Version.String()+" · safe mode", items)
	menu.wanted = 48
	m.push(menu)
}

// leaveSafeMode brings the whole interface back.
func (m *Model) leaveSafeMode() {
	if m.safe == nil {
		return
	}
	m.safe = nil
	m.mode = parseScreenMode(m.cfg.GUI.ScreenMode)
	if !m.mode.shows(m.focus) {
		m.focus = m.mode.firstPanel()
	}
	m.relayout()
}
