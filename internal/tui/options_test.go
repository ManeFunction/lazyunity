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

func selectRoot(t *testing.T, m *Model, suffix string) rootRow {
	t.Helper()
	for _, r := range m.rootsTab.Items() {
		if strings.HasSuffix(r.Path, suffix) {
			m.rootsTab.SelectKey(r.Path)
			return r
		}
	}
	t.Fatalf("no root ending in %q among %v", suffix, m.rootsTab.Items())
	return rootRow{}
}

func TestRootsListShowsInstallRootAndExtras(t *testing.T) {
	m := newTestModel(t)
	extra := t.TempDir()
	m.st.AddRoot(extra)
	m.cfg.Editors.ExtraRoots = []string{"/from/config"}
	drain(m, m.scanEditorsCmd()) // the scan finds which folders are there
	press(m, "3", "]")
	out := screenText(m)
	if !strings.Contains(out, "install location") || !strings.Contains(out, "2 editors") || !strings.Contains(out, "not found") {
		t.Errorf("roots:\n%s", out)
	}
	roots := m.rootsTab.Items()
	if len(roots) != 3 || !roots[0].Install || roots[0].Source != "cli" || roots[1].Source != "state" || roots[2].Source != "config" {
		t.Errorf("roots = %+v", roots)
	}
	if roots[2].Exists {
		t.Error("/from/config does not exist")
	}
}

func TestAddARoot(t *testing.T) {
	m := newTestModel(t)
	press(m, "3", "]")
	dir := t.TempDir()
	press(m, "a")
	pp := m.popups[0].(*promptPopup)
	pp.in.Set(filepath.Join(dir, "nope"))
	press(m, "enter")
	if len(m.popups) != 1 || !strings.Contains(screenText(m), "That folder does not exist") {
		t.Error("a missing folder is refused")
	}
	pp.in.Set(dir)
	_, cmd := m.Update(keyMsg("enter"))
	if len(m.st.ExtraRoots) != 1 || m.st.ExtraRoots[0] != dir {
		t.Fatalf("roots = %v", m.st.ExtraRoots)
	}
	if cmd == nil {
		t.Error("adding a root rescans for Editors")
	}
	if _, err := os.Stat(m.st.Path()); err != nil {
		t.Error("the new root must be saved at once")
	}
	if !strings.Contains(screenText(m), "Added") {
		t.Error("the user should be told")
	}
	// adding it again is noticed
	press(m, "a")
	m.popups[0].(*promptPopup).in.Set(dir)
	press(m, "enter")
	if len(m.st.ExtraRoots) != 1 || !strings.Contains(screenText(m), "already in the list") {
		t.Error("duplicates are refused")
	}
}

func TestRemoveARoot(t *testing.T) {
	m := newTestModel(t)
	extra := t.TempDir()
	m.st.AddRoot(extra)
	m.cfg.Editors.ExtraRoots = []string{"/from/config"}
	drain(m, m.scanEditorsCmd()) // the scan finds which folders are there
	press(m, "3", "]")

	selectRoot(t, m, "Editor") // the install location
	press(m, "d")
	if len(m.popups) != 0 || !strings.Contains(screenText(m), "where new Editors are installed") {
		t.Errorf("the install location cannot be removed:\n%s", screenText(m))
	}
	selectRoot(t, m, "/from/config")
	press(m, "d")
	if len(m.popups) != 0 || !strings.Contains(screenText(m), "set in config.yml") {
		t.Errorf("a root from config.yml cannot be removed here:\n%s", screenText(m))
	}
	selectRoot(t, m, filepath.Base(extra))
	press(m, "d")
	if len(m.popups) != 1 || !strings.Contains(screenText(m), "stay installed") {
		t.Fatalf("a confirmation that says the Editors stay:\n%s", screenText(m))
	}
	_, cmd := m.Update(keyMsg("enter"))
	if len(m.st.ExtraRoots) != 0 || cmd == nil {
		t.Errorf("roots = %v", m.st.ExtraRoots)
	}
}

func TestSetInstallLocation(t *testing.T) {
	m := newTestModel(t)
	extra := t.TempDir()
	m.st.AddRoot(extra)
	m.refreshRoots()
	press(m, "3", "]")
	f := useFakeCLI(t, m)
	selectRoot(t, m, "Editor")
	press(m, "space")
	if !strings.Contains(screenText(m), "already installed there") {
		t.Error("choosing the current location is a no-op")
	}
	selectRoot(t, m, filepath.Base(extra))
	drain(m, m.keymap.lookup(ctxRoots, "space").run(m))
	if !f.called("SetInstallPath", extra) || !f.called("InstallPath") {
		t.Fatalf("calls: %v", f.Calls())
	}
	if got := m.inv.roots()[0]; got != extra {
		t.Errorf("the new install root should be adopted, got %q", got)
	}
	roots := m.rootsTab.Items()
	if !roots[0].Install || roots[0].Path != extra {
		t.Errorf("the install root comes first: %+v", roots)
	}
	// a folder that is not there cannot be the install location
	m.rootsTab.SetItems([]rootRow{{Path: "/gone", Exists: false}})
	press(m, "space")
	if !strings.Contains(screenText(m), "does not exist") {
		t.Error("a missing folder is refused")
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
	m := newTestModel(t)
	press(m, "3")
	out := screenText(m)
	for _, want := range []string{"Unity CLI", "ready · 1.0.0-beta.16", "Account", "checking…", "License", "Default project folder", "Config file", "About", "lazyunity test"} {
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
	m.Update(cliUpdateMsg{info: unitycli.UpdateInfo{Current: "1.0.0-beta.16", Latest: "1.0.0-beta.17"}})
	if !strings.Contains(screenText(m), "update available: 1.0.0-beta.17") {
		t.Errorf("a newer CLI is announced:\n%s", screenText(m))
	}
	m.Update(cliUpdateMsg{err: errors.New("offline")})
	if !strings.Contains(screenText(m), "update available: 1.0.0-beta.17") {
		t.Error("a failed check keeps what was known")
	}
	m.cliStatus.State = unitycli.Missing
	m.refreshTools()
	if !strings.Contains(screenText(m), "not installed · press enter to install") || !strings.Contains(screenText(m), "needs the Unity CLI") {
		t.Errorf("without the CLI:\n%s", screenText(m))
	}
	m.cliStatus.State = unitycli.Outdated
	m.refreshTools()
	if !strings.Contains(screenText(m), "outdated · press enter to update") {
		t.Error("an old CLI is flagged")
	}
}

func toolIndex(m *Model, id string) int {
	for i, r := range m.toolsTab.Items() {
		if r.id == id {
			return i
		}
	}
	return -1
}

func runToolRow(m *Model, id string) tea.Cmd {
	m.toolsTab.Top()
	m.toolsTab.Move(toolIndex(m, id))
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
	useFakeCLI(t, m)
	runToolRow(m, "cli")
	out := screenText(m)
	if !strings.Contains(out, "Update (self-update)") || !strings.Contains(out, "Reinstall with the official installer") {
		t.Errorf("screen:\n%s", out)
	}
}

func TestCLIStatusMessage(t *testing.T) {
	m := newTestModel(t)
	m.cli = nil
	cmd := m.onCLIStatus(cliStatusMsg{st: unitycli.Status{State: unitycli.Ready, Path: "/x/unity", Version: unitycli.MinVersion}})
	if c, ok := m.cli.(*unitycli.Client); !ok || c.Path != "/x/unity" || cmd == nil || !strings.Contains(screenText(m), "Unity CLI 1.0.0-beta.16 is ready") {
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
	pp.in.Set("/definitely/not/there")
	press(m, "enter")
	if len(m.popups) != 1 {
		t.Error("a missing folder is refused")
	}
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
	m.cfg.Editors.ExtraRoots = []string{"~/Unity-extra"}
	m.cfg.Projects.DefaultLocation = "~/Games"
	roots := m.inv.roots()
	want := filepath.Join(homeDir(), "Unity-extra")
	found := false
	for _, r := range roots {
		found = found || r == want
	}
	if !found {
		t.Errorf("roots = %v, want one to be %s", roots, want)
	}
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
