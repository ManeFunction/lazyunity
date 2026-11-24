package tui

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/ManeFunction/lazyunity/internal/config"
	"github.com/ManeFunction/lazyunity/internal/unitycli"
)

func TestEditorsFolderRowAndPrompt(t *testing.T) {
	m := newTestModel(t)
	f := useFakeCLI(t, m)
	press(m, "3")
	m.toolsTab.SelectKey("editorsdir")
	out := screenText(m)
	if !strings.Contains(out, "Editors folder") {
		t.Errorf("the tools should show the Editors folder:\n%s", out)
	}
	if row, ok := m.toolsTab.Selected(); !ok || row.status != displayPath(m.inv.installRoot())+" · enter to change" {
		t.Errorf("the row shows the folder: %+v", row)
	}

	// enter opens the prompt; a missing folder asks to be created, and
	// declining comes back to the prompt
	press(m, "enter")
	pp := m.popups[0].(*promptPopup)
	pp.in.Set(filepath.Join(m.inv.installRoot(), "nope"))
	press(m, "enter")
	if c, ok := m.popups[len(m.popups)-1].(*confirmPopup); !ok || !strings.Contains(c.message, "does not exist") {
		t.Fatalf("a missing folder asks to be created:\n%s", screenText(m))
	}
	press(m, "esc")
	if len(m.popups) != 1 || m.popups[0] != pp || f.called("SetInstallPath", filepath.Join(m.inv.installRoot(), "nope")) {
		t.Errorf("declining comes back to the prompt: %v", f.Calls())
	}

	// the current folder is a no-op
	pp.in.Set(m.inv.installRoot())
	press(m, "enter")
	if !strings.Contains(m.toast.text, "already install into") {
		t.Errorf("choosing the current folder is a no-op, toast: %q", m.toast.text)
	}

	// a different folder is set through the CLI
	other := t.TempDir()
	press(m, "enter")
	m.popups[0].(*promptPopup).in.Set(other)
	_, cmd := m.Update(keyMsg("enter"))
	drain(m, cmd)
	if !f.called("SetInstallPath", other) || !f.called("InstallPath") {
		t.Errorf("calls: %v", f.Calls())
	}
	if got := m.inv.installRoot(); got != other {
		t.Errorf("the new Editors folder should be adopted, got %q", got)
	}
	if !strings.Contains(m.toast.text, "New Editors now install into") {
		t.Errorf("the user should be told: %q", m.toast.text)
	}
}

func TestStrayEditorsAreRegisteredOnceAndQuietly(t *testing.T) {
	m := newTestModel(t)
	f := useFakeCLI(t, m)
	f.err["AddEditors"] = cliError(1, "signature check failed")
	if cmd := m.registerStrays(); cmd != nil {
		t.Error("nothing is registered before the CLI's own list is known")
	}
	m.inv.setListed(nil) // the CLI lists none of them
	cmd := m.registerStrays()
	if cmd == nil {
		t.Fatal("Editors the CLI does not know about should be registered")
	}
	drain(m, cmd)
	if len(m.popups) != 0 || strings.Contains(screenText(m), "signature") {
		t.Errorf("a refused registration must stay quiet:\n%s", screenText(m))
	}
	if calls := f.Calls(); len(calls) != 1 || !f.called("AddEditors", []string{m.inv.editors()[0].Location, m.inv.editors()[1].Location}) {
		t.Errorf("calls: %v", calls)
	}
	if again := m.registerStrays(); again != nil {
		t.Error("each Editor is tried once per run")
	}
	// Editors the CLI already lists are left alone
	m2 := newTestModel(t)
	useFakeCLI(t, m2)
	m2.inv.setListed(listEditors("linux", cliEditors(m2)))
	if cmd := m2.registerStrays(); cmd != nil {
		t.Error("known Editors need no registering")
	}
}

func TestToolsTabRows(t *testing.T) {
	verifiedWith10(t) // the versions below are inside and around 1.0.*
	m := newTestModel(t)
	press(m, "3")
	out := screenText(m)
	for _, want := range []string{"Unity CLI", "ready · 1.0.0", "Account", "checking…", "License", "Default project folder", "Config file", "OpenUPM CLI"} {
		if !strings.Contains(out, want) {
			t.Errorf("the tools should contain %q:\n%s", want, out)
		}
	}
	m.Update(authMsg{info: unitycli.AuthInfo{LoggedIn: true, User: struct {
		Name  string `json:"name"`
		Email string `json:"email"`
	}{Email: "me@example.com"}}})
	if !strings.Contains(screenText(m), "signed in as me@example.com") {
		t.Error("the account is shown")
	}
	m.Update(cliUpdateMsg{info: unitycli.UpdateInfo{Current: "1.0.0", Latest: "1.0.1"}})
	if !strings.Contains(screenText(m), "update available: 1.0.1") {
		t.Errorf("a newer CLI is announced:\n%s", screenText(m))
	}
	m.Update(cliUpdateMsg{err: errors.New("offline")})
	if !strings.Contains(screenText(m), "update available: 1.0.1") {
		t.Error("a failed check keeps what was known")
	}
	m.cliStatus.State = unitycli.Missing
	m.refreshTools()
	if !strings.Contains(screenText(m), "not installed · press enter to install") || !strings.Contains(screenText(m), "needs the Unity CLI") {
		t.Errorf("without the CLI:\n%s", screenText(m))
	}
	m.cliStatus = unitycli.Status{State: unitycli.Outdated, Version: unitycli.Version{Minor: 9, Patch: 5}}
	m.refreshTools()
	if row := m.toolsTab.Items()[0]; row.status != "update required: 0.9.5 is older than 1.0.* · enter to update" || row.tone != toneBad {
		t.Errorf("an old CLI must be updated, in red: %+v", row)
	}
	m.cliStatus = unitycli.Status{State: unitycli.Outdated, Path: "/x/unity"}
	m.refreshTools()
	if row := m.toolsTab.Items()[0]; row.status != "not working · press enter to reinstall" || row.tone != toneBad {
		t.Errorf("a CLI that reports no version must be reinstalled, in red: %+v", row)
	}
	if !strings.Contains(out, "╌ ╌ ╌") {
		t.Errorf("spacers draw as a sparse dashed rule:\n%s", out)
	}
	m.toolsTab.Bottom() // About and Donate are below the fold in this screen
	if out := screenText(m); !strings.Contains(out, "About") || !strings.Contains(out, "lazyunity test") {
		t.Errorf("the bottom of the tools should contain About and Donate:\n%s", out)
	}
}

func runToolRow(m *Model, id string) tea.Cmd {
	m.toolsTab.Top()
	m.toolsTab.SelectKey(id)
	press(m, "3")
	m.panels[panelOptions].cur = 0
	return m.keymap.lookup(ctxTools, "enter").run(m)
}

func TestCLIToolInstallsWhenMissing(t *testing.T) {
	m := newTestModel(t)
	m.cliStatus.State = unitycli.Missing
	m.refreshTools()
	runToolRow(m, "cli")
	if len(m.popups) != 1 || !strings.Contains(screenText(m), "Install the Unity CLI") || !strings.Contains(screenText(m), "sudo password") {
		t.Fatalf("screen:\n%s", screenText(m))
	}
	// on Windows there is no sudo
	m.popups = nil
	m.goos = "windows"
	runToolRow(m, "cli")
	if strings.Contains(screenText(m), "sudo") || !strings.Contains(screenText(m), "install.ps1") {
		t.Errorf("windows:\n%s", screenText(m))
	}
}

func TestCLIToolOffersUpdateWhenReady(t *testing.T) {
	m := newTestModel(t)
	f := useFakeCLI(t, m)
	runToolRow(m, "cli")
	out := screenText(m)
	if !strings.Contains(out, "Update") || !strings.Contains(out, "Reinstall with the official installer") {
		t.Errorf("screen:\n%s", out)
	}
	if strings.Contains(out, "Update to") {
		t.Errorf("without an answer from the update check there is nothing to update to:\n%s", out)
	}

	m.popups = nil
	m.cliUpdate = unitycli.UpdateInfo{Current: "1.0.0", Latest: "1.0.4"}
	runToolRow(m, "cli")
	_, cmd := m.Update(keyMsg("enter"))
	drain(m, cmd)
	if !f.called("SelfUpdate", unitycli.UpdateOptions{Channel: "stable", Target: "1.0.4"}) {
		t.Errorf("Update installs the verified version it names: %v", f.Calls())
	}
}

func TestCLIStatusMessage(t *testing.T) {
	verifiedWith10(t)
	m := newTestModel(t)
	m.cli = nil
	cmd := m.onCLIStatus(cliStatusMsg{st: unitycli.Status{State: unitycli.Ready, Path: "/x/unity", Version: unitycli.MinVersion}})
	if c, ok := m.cli.(*unitycli.Client); !ok || c.Path != "/x/unity" || cmd == nil || !strings.Contains(screenText(m), "Unity CLI 1.0.0 is ready") {
		t.Errorf("a ready CLI is adopted and the data is loaded: cli=%v", m.cli)
	}
	m.onCLIStatus(cliStatusMsg{st: unitycli.Status{State: unitycli.Outdated}})
	if m.cli != nil || m.cliStatus.State != unitycli.Outdated {
		t.Error("an outdated CLI is dropped")
	}
}

func TestAccountToolSignsOut(t *testing.T) {
	m := newTestModel(t)
	m.auth, m.authKnown = unitycli.AuthInfo{LoggedIn: true}, true
	m.refreshTools()
	f := useFakeCLI(t, m) // signed out after the logout
	runToolRow(m, "account")
	if len(m.popups) != 1 || !strings.Contains(screenText(m), "Sign out") {
		t.Fatalf("a confirmation:\n%s", screenText(m))
	}
	_, cmd := m.Update(keyMsg("enter"))
	drain(m, cmd)
	if !f.called("Logout") || !f.called("AuthStatus") || m.auth.LoggedIn || !strings.Contains(screenText(m), "Signed out") {
		t.Errorf("calls: %v, loggedIn=%v", f.Calls(), m.auth.LoggedIn)
	}
}

func TestAccountToolSignsInWhenSignedOut(t *testing.T) {
	m := newTestModel(t)
	m.auth, m.authKnown = unitycli.AuthInfo{}, true
	m.refreshTools()
	f := useFakeCLI(t, m)
	f.auth = unitycli.AuthInfo{LoggedIn: true} // after the sign-in
	cmd := runToolRow(m, "account")
	if len(m.tasks) != 1 || m.tasks[0].title != "Signing in to Unity" {
		t.Fatalf("sign-in is a task the user can cancel: %v", m.tasks)
	}
	drain(m, cmd)
	if !f.called("Login") || !m.auth.LoggedIn || !strings.Contains(screenText(m), "Signed in") {
		t.Errorf("calls: %v", f.Calls())
	}
}

func TestLicenseMenu(t *testing.T) {
	m := newTestModel(t)
	f := useFakeCLI(t, m)
	f.license = json.RawMessage(`{"licenses":[{"type":"Personal"}]}`)
	runToolRow(m, "license")
	out := screenText(m)
	for _, want := range []string{"Show the license status", "Activate the free Personal license", "Return the license"} {
		if !strings.Contains(out, want) {
			t.Errorf("should contain %q", want)
		}
	}
	_, cmd := m.Update(keyMsg("enter")) // show status
	drain(m, cmd)
	if !f.called("LicenseStatus") || !strings.Contains(screenText(m), "Personal") || !strings.Contains(screenText(m), "License status") {
		t.Errorf("screen:\n%s", screenText(m))
	}
}

func TestLicenseStatusWithNoLicense(t *testing.T) {
	m := newTestModel(t)
	f := useFakeCLI(t, m)
	f.license = json.RawMessage(`{"active":false}`)
	f.err["LicenseStatus"] = cliError(unitycli.ExitNeedsConfig, "none")
	runToolRow(m, "license")
	_, cmd := m.Update(keyMsg("enter"))
	drain(m, cmd)
	if !strings.Contains(screenText(m), "No active license") {
		t.Errorf("screen:\n%s", screenText(m))
	}
}

func TestActivatePersonalLicenseNeedsConsent(t *testing.T) {
	m := newTestModel(t)
	f := useFakeCLI(t, m)
	runToolRow(m, "license")
	press(m, "j", "enter")
	if len(m.popups) != 1 || !strings.Contains(screenText(m), "Activate Unity Personal") || !strings.Contains(screenText(m), "license terms") {
		t.Fatalf("the terms must be shown first:\n%s", screenText(m))
	}
	if len(f.Calls()) != 0 {
		t.Error("nothing is activated before the user accepts")
	}
	_, cmd := m.Update(keyMsg("enter"))
	drain(m, cmd)
	if !f.called("ActivatePersonalLicense") {
		t.Errorf("calls: %v", f.Calls())
	}
}

func TestProjectDirPrompt(t *testing.T) {
	m := newTestModel(t)
	dir := t.TempDir()
	runToolRow(m, "projectdir")
	pp := m.popups[0].(*promptPopup)
	file := filepath.Join(dir, "file")
	os.WriteFile(file, nil, 0o644)
	pp.in.Set(filepath.Join(file, "sub"))
	press(m, "enter")
	if len(m.popups) != 1 || pp.err == "" || !strings.HasPrefix(pp.err, "A file is in the way") {
		t.Errorf("a path through a file is refused:\n%s", screenText(m))
	}

	// a missing folder is created, with its parents, once the user agrees
	missing := filepath.Join(dir, "a", "b")
	pp.in.Set(missing)
	press(m, "enter")
	if c, ok := m.popups[len(m.popups)-1].(*confirmPopup); !ok || !strings.Contains(c.message, "does not exist") {
		t.Fatalf("a missing folder asks to be created:\n%s", screenText(m))
	}
	press(m, "enter")
	if st, err := os.Stat(missing); err != nil || !st.IsDir() || m.st.DefaultProjectDir != missing || len(m.popups) != 0 {
		t.Errorf("the folder is created and chosen: %v, %q", err, m.st.DefaultProjectDir)
	}

	runToolRow(m, "projectdir")
	pp = m.popups[0].(*promptPopup)
	pp.in.Set(dir)
	press(m, "enter")
	if m.st.DefaultProjectDir != dir || m.projectsDir() != dir {
		t.Errorf("DefaultProjectDir = %q", m.st.DefaultProjectDir)
	}
	for _, r := range m.toolsTab.Items() {
		if r.id == "projectdir" && !strings.Contains(r.status, displayPath(dir)) {
			t.Errorf("the Tools tab shows the new folder, got %q", r.status)
		}
	}
}

func TestProjectsDirPrecedence(t *testing.T) {
	m := newTestModel(t)
	if m.projectsDir() != homeDir() {
		t.Errorf("default = %q", m.projectsDir())
	}
	m.cfg.Projects.DefaultLocation = "/from/config"
	if m.projectsDir() != "/from/config" {
		t.Errorf("config = %q", m.projectsDir())
	}
	m.st.DefaultProjectDir = "/from/state"
	if m.projectsDir() != "/from/state" {
		t.Errorf("state wins = %q", m.projectsDir())
	}
}

func TestConfigToolCreatesTheFileAndShowsItsPath(t *testing.T) {
	m := newTestModel(t)
	t.Setenv("VISUAL", "")
	t.Setenv("EDITOR", "")
	runToolRow(m, "config")
	path := filepath.Join(m.configDir, "config.yml")
	b, err := os.ReadFile(path)
	if err != nil || !strings.Contains(string(b), "lazyunity configuration") {
		t.Fatalf("the config file should be created with a comment: %v", err)
	}
	// and the file it creates is valid config
	if _, err := config.Load(m.configDir); err != nil {
		t.Errorf("the template must load: %v", err)
	}
	if len(m.popups) != 1 || !strings.Contains(screenText(m), "Config file") || !strings.Contains(flatten(screenText(m)), path) {
		t.Errorf("without an editor, the full path is shown in a popup:\n%s", screenText(m))
	}
	m.popups = nil
	// an existing file is left alone
	os.WriteFile(path, []byte("quitAfterOpen: true\n"), 0o644)
	runToolRow(m, "config")
	if b, _ := os.ReadFile(path); string(b) != "quitAfterOpen: true\n" {
		t.Error("an existing config must not be overwritten")
	}
}

func TestAboutPopup(t *testing.T) {
	m := newTestModel(t)
	runToolRow(m, "about")
	out := screenText(m)
	for _, want := range []string{"About lazyunity", "lazyunity test", "Unity CLI", "ready", "config.yml", "state.yml"} {
		if !strings.Contains(out, want) {
			t.Errorf("should contain %q:\n%s", want, out)
		}
	}
}

func TestCommandLog(t *testing.T) {
	m := newTestModel(t)
	press(m, "@")
	if !strings.Contains(screenText(m), "No Unity CLI commands have run yet") {
		t.Error("an empty log says so")
	}
	press(m, "esc")
	f := useFakeCLI(t, m)
	f.log = []unitycli.LogEntry{
		{Args: []string{"projects", "--all"}, Stdout: `{"success":true}`},
		{Args: []string{"install-path", "--get"}, Stdout: `{"success":true,"data":{"path":"/x"}}`},
	}
	press(m, "@")
	out := screenText(m)
	if !strings.Contains(out, "unity install-path --get") || !strings.Contains(out, "unity projects --all") || !strings.Contains(out, "exit 0") {
		t.Fatalf("the log lists the commands:\n%s", out)
	}
	menu := m.popups[0].(*menuPopup)
	if first, _ := menu.list.Selected(); !strings.Contains(first.label, "install-path") {
		t.Errorf("newest first, got %q", first.label)
	}
	_, cmd := m.Update(keyMsg("enter"))
	drain(m, cmd)
	if !strings.Contains(screenText(m), `"path":"/x"`) {
		t.Errorf("the entry shows the output:\n%s", screenText(m))
	}
}

func TestCancelTaskActionOnlyWhenBusy(t *testing.T) {
	m := newTestModel(t)
	a := m.keymap.lookup(ctxProjects, "X")
	if a == nil || a.enabled(m) {
		t.Fatal("the cancel action exists but is off when nothing runs")
	}
	for _, h := range m.keymap.hints(m, ctxProjects) {
		if h.label == "Cancel task" {
			t.Error("the hint must not show when idle")
		}
	}
	enqueueForever(m)
	if !a.enabled(m) {
		t.Error("enabled while a task runs")
	}
	found := false
	for _, h := range m.keymap.hints(m, ctxProjects) {
		found = found || h.label == "Cancel task"
	}
	if !found {
		t.Error("the hint shows while a task runs")
	}
	press(m, "X")
	if m.running() == nil || m.running().cancel == nil {
		t.Fatal("task lost")
	}
}

func TestShellPrompt(t *testing.T) {
	m := newTestModel(t)
	press(m, ":")
	if len(m.popups) != 1 || !strings.Contains(screenText(m), "Run a shell command") || !strings.Contains(screenText(m), "Runs in") {
		t.Fatalf("screen:\n%s", screenText(m))
	}
	press(m, "enter")
	if len(m.popups) != 1 || !strings.Contains(screenText(m), "Type a command") {
		t.Error("an empty command is refused")
	}
	press(m, "esc")
}

func TestTildeInConfiguredPathsIsExpanded(t *testing.T) {
	m := newTestModel(t)
	m.cfg.Projects.DefaultLocation = "~/Games"
	if m.projectsDir() != filepath.Join(homeDir(), "Games") {
		t.Errorf("projectsDir = %q", m.projectsDir())
	}
}

func TestInstallersWaitForRunningTasks(t *testing.T) {
	m := newTestModel(t)
	m.cliStatus.State = unitycli.Missing
	m.tasks = []*task{{title: "Installing 6000.7.0b8", status: taskRunning}}

	m.cliTool()
	if len(m.popups) != 0 || !strings.Contains(screenText(m), "Wait for the running tasks") {
		t.Errorf("the Unity CLI install waits:\n%s", screenText(m))
	}
	m.toast = toast{}
	m.openupmTool()
	if len(m.popups) != 0 || !strings.Contains(screenText(m), "Wait for the running tasks") {
		t.Errorf("the OpenUPM install waits:\n%s", screenText(m))
	}

	m.tasks[0].status = taskDone
	m.cliTool()
	if len(m.popups) != 1 {
		t.Errorf("with nothing running the Unity CLI install asks:\n%s", screenText(m))
	}
}

func TestEveryToolRowDoesSomething(t *testing.T) {
	m := newTestModel(t)
	for _, r := range m.toolsTab.Items() {
		if r.spacer {
			continue // spacers group the rows; enter does nothing on them
		}
		if r.run == nil {
			t.Errorf("the %s row does nothing on enter", r.label)
		}
	}
}

func TestEditedConfigRelaunches(t *testing.T) {
	m := newTestModel(t)
	path := filepath.Join(m.configDir, "config.yml")
	os.MkdirAll(m.configDir, 0o755)
	before := []byte("quitAfterOpen: false\n")
	os.WriteFile(path, before, 0o644)

	// unchanged: nothing happens
	m.Update(configEditedMsg{before: before})
	if m.Restart() || m.quit {
		t.Fatal("an unchanged config must not relaunch")
	}

	// broken: kept running, with the reason shown
	os.WriteFile(path, []byte("quitAfterOpen: [\n"), 0o644)
	m.Update(configEditedMsg{before: before})
	if m.Restart() || m.quit || !strings.Contains(screenText(m), "Config not applied") {
		t.Fatalf("a config that does not load must not relaunch:\n%s", screenText(m))
	}
	m.popups = nil

	// changed and valid: quits to relaunch
	os.WriteFile(path, []byte("quitAfterOpen: true\n"), 0o644)
	_, cmd := m.Update(configEditedMsg{before: before})
	if !m.Restart() || !m.quit || cmd == nil {
		t.Error("a changed config should quit to relaunch")
	}
}

func TestConfigEditedWhileBusyAsksToRestartFromTheRow(t *testing.T) {
	m := newTestModel(t)
	path := filepath.Join(m.configDir, "config.yml")
	os.MkdirAll(m.configDir, 0o755)
	before := []byte("quitAfterOpen: false\n")
	os.WriteFile(path, []byte("quitAfterOpen: true\n"), 0o644)
	m.tasks = []*task{{title: "Installing 6000.7.0b8", status: taskRunning}}

	m.Update(configEditedMsg{before: before})
	if m.Restart() || m.quit {
		t.Fatal("must not relaunch while a task runs")
	}
	m.toolsTab.SelectKey("config")
	r, _ := m.toolsTab.Selected()
	if !strings.Contains(r.status, "restart required") {
		t.Fatalf("the config row should ask for a restart: %q", r.status)
	}

	// enter on the row asks first, as the task would be cancelled
	runToolRow(m, "config")
	if m.Restart() || len(m.popups) != 1 || !strings.Contains(screenText(m), "cancels it") {
		t.Fatalf("restarting over a task should be confirmed:\n%s", screenText(m))
	}
	m.popups = nil

	// once idle, enter restarts straight away
	m.tasks = nil
	runToolRow(m, "config")
	if !m.Restart() || !m.quit {
		t.Error("enter on the row should restart once nothing runs")
	}
}

func TestEEditsTheConfigFromTheToolsTab(t *testing.T) {
	m := newTestModel(t)
	t.Setenv("VISUAL", "")
	t.Setenv("EDITOR", "")
	m.toolsTab.Top() // on the CLI row, not the config one
	press(m, "3")
	m.panels[panelOptions].cur = 0
	m.keymap.lookup(ctxTools, "e").run(m)
	if !strings.Contains(screenText(m), "Config file") {
		t.Errorf("e should open the config from any row:\n%s", screenText(m))
	}
}

func TestCLIFeedSwitch(t *testing.T) {
	verifiedWith10(t)
	m := newTestModel(t)
	f := useFakeCLI(t, m)
	if m.cliChannel() != unitycli.ChannelStable {
		t.Fatalf("a release build follows the release feed, got %q", m.cliChannel())
	}
	toBeta := unitycli.UpdateOptions{Channel: "beta"}
	switchToBeta := func() {
		t.Helper()
		m.popups = nil
		runToolRow(m, "cli")
		if !strings.Contains(screenText(m), "Switch to the beta feed") {
			t.Fatalf("screen:\n%s", screenText(m))
		}
		press(m, "j", "enter") // past Update
		if !strings.Contains(screenText(m), "may be newer than the versions lazyunity is verified with") {
			t.Fatalf("the beta feed is asked about first:\n%s", screenText(m))
		}
		_, cmd := m.Update(keyMsg("enter"))
		drain(m, cmd)
	}
	f.err["SelfUpdate"] = errors.New("offline")
	switchToBeta()
	if !f.called("SelfUpdate", toBeta) || m.st.CLIChannel != "" {
		t.Fatalf("a failed switch is not remembered: channel %q, calls %v", m.st.CLIChannel, f.Calls())
	}

	delete(f.err, "SelfUpdate")
	switchToBeta()
	if m.st.CLIChannel != unitycli.ChannelBeta || m.cliChannel() != unitycli.ChannelBeta {
		t.Fatalf("the beta feed is remembered: %q (calls %v)", m.st.CLIChannel, f.Calls())
	}

	// the CLI is still a release build, but the chosen feed wins
	m.cli, m.popups = f, nil
	m.cliStatus = unitycli.Status{State: unitycli.Ready, Version: unitycli.MinVersion}
	m.refreshTools()
	if !strings.Contains(screenText(m), "ready · 1.0.0 · beta feed") {
		t.Errorf("the feed is on the Tools row:\n%s", screenText(m))
	}
	drain(m, m.checkCLIUpdateCmd())
	if !f.called("CheckUpdate", "beta") {
		t.Errorf("updates are checked on the chosen feed: %v", f.Calls())
	}
	m.cliUpdate = unitycli.UpdateInfo{Current: "1.0.0", Latest: "1.0.1-beta.2"}
	m.tasks = nil // finished tasks hold their work until they leave the list
	runToolRow(m, "cli")
	if !strings.Contains(screenText(m), "Switch to the release feed") {
		t.Fatalf("screen:\n%s", screenText(m))
	}
	_, cmd := m.Update(keyMsg("enter")) // Update
	drain(m, cmd)
	if !f.called("SelfUpdate", unitycli.UpdateOptions{Channel: "beta", Target: "1.0.1-beta.2"}) {
		t.Errorf("Update stays on the beta feed: %v", f.Calls())
	}

	// going back to releases needs no question
	m.cli, m.popups, m.tasks = f, nil, nil // the update probed the test machine, which has no CLI
	m.cliStatus = unitycli.Status{State: unitycli.Ready, Version: unitycli.MinVersion}
	runToolRow(m, "cli")
	press(m, "j")
	_, cmd = m.Update(keyMsg("enter"))
	drain(m, cmd)
	if !f.called("SelfUpdate", unitycli.UpdateOptions{Channel: "stable"}) {
		t.Errorf("calls: %v", f.Calls())
	}
}

func TestCLIChannelFollowsABetaBuild(t *testing.T) {
	m := newTestModel(t)
	m.cliStatus.Version = unitycli.Version{Major: 1, Minor: 1, Pre: "beta", PreNum: 2}
	if m.cliChannel() != unitycli.ChannelBeta {
		t.Errorf("a beta build follows the beta feed, got %q", m.cliChannel())
	}
	m.st.CLIChannel = unitycli.ChannelStable
	if m.cliChannel() != unitycli.ChannelStable {
		t.Errorf("a chosen feed wins, got %q", m.cliChannel())
	}
}

// With os.edit set the editor runs on its own and cannot be waited for: the
// edit is looked at once the user is back in lazyunity's terminal.
func TestConfigEditedInADetachedEditorAppliesOnFocus(t *testing.T) {
	m := newTestModel(t)
	m.cfg.OS.Edit = "code {{path}}"
	path := filepath.Join(m.configDir, "config.yml")
	os.MkdirAll(m.configDir, 0o755)
	os.WriteFile(path, []byte("quitAfterOpen: false\n"), 0o644)

	if cmd := m.editConfig(); cmd != nil {
		t.Fatal("a detached editor hands nothing to run")
	}
	if len(recorder(m).editors) != 1 {
		t.Fatalf("os.edit should start the editor: %v", recorder(m).editors)
	}
	os.WriteFile(path, []byte("quitAfterOpen: true\n"), 0o644)
	_, cmd := m.Update(tea.FocusMsg{})
	drain(m, cmd)
	if !m.Restart() || !m.quit {
		t.Error("the changed config should relaunch once the terminal is focused again")
	}
}

func TestConfigThatCannotBeCreatedSaysSo(t *testing.T) {
	m := newTestModel(t)
	blocker := filepath.Join(t.TempDir(), "file")
	os.WriteFile(blocker, nil, 0o644)
	m.configDir = filepath.Join(blocker, "lazyunity") // a folder under a file cannot be made
	m.cfg.OS.Edit = "code {{path}}"
	m.editConfig()
	if !strings.Contains(screenText(m), "Could not create the config file") || len(recorder(m).editors) != 0 {
		t.Errorf("the failure is shown and no editor is started:\n%s", screenText(m))
	}
}
