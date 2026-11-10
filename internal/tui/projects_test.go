package tui

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ManeFunction/lazyunity/internal/state"
	"github.com/ManeFunction/lazyunity/internal/unitycli"
)

// pretendInstalled makes the inventory list one more Editor, which is on no disk.
func pretendInstalled(m *Model, e editorRow) {
	m.inv.all = append(m.inv.all, e)
	m.inv.versions[e.Version] = true
}

func selectProject(t *testing.T, m *Model, name string) projectRow {
	t.Helper()
	for _, r := range m.projectsTab.Items() {
		if r.name() == name {
			m.projectsTab.SelectKey(r.Path)
			return r
		}
	}
	t.Fatalf("no project called %q", name)
	return projectRow{}
}

func TestOpenProjectRunsTheCLI(t *testing.T) {
	m := newTestModel(t)
	space := selectProject(t, m, "Space Shooter")
	f := useFakeCLI(t, m)
	f.projects = fixture[unitycli.Project](t, "projects.json")
	drain(m, m.keymap.lookup(ctxProjects, "enter").run(m))
	if !f.called("Open", unitycli.OpenOptions{Project: space.Path}) {
		t.Fatalf("the CLI should have been asked to open the project: %v", f.Calls())
	}
	if !f.called("Projects") {
		t.Error("the list should be refreshed after opening, to update the last-opened time")
	}
	if !strings.Contains(screenText(m), "Opened Space Shooter in Unity 6000.6.8f1") {
		t.Errorf("the user should be told:\n%s", screenText(m))
	}
	if len(m.popups) != 0 {
		t.Error("a successful open needs no popup")
	}
}

func TestOpenAlreadyOpen(t *testing.T) {
	m := newTestModel(t)
	selectProject(t, m, "Space Shooter")
	f := useFakeCLI(t, m)
	f.opened.AlreadyOpen = true
	press(m, "space")
	drain(m, m.keymap.lookup(ctxProjects, "space").run(m))
	if !strings.Contains(screenText(m), "already open") {
		t.Errorf("screen:\n%s", screenText(m))
	}
}

func TestOpenAppliesAndClearsThePendingPlatform(t *testing.T) {
	m := newTestModel(t)
	old := selectProject(t, m, "Old Platformer") // Android now, 2022.3 has Android installed
	m.st.SetPending(old.Path, state.Pending{BuildTarget: "StandaloneLinux64"})
	m.refreshProjects()
	selectProject(t, m, "Old Platformer")
	f := useFakeCLI(t, m)
	drain(m, m.keymap.lookup(ctxProjects, "enter").run(m))
	if !f.called("Open", unitycli.OpenOptions{Project: old.Path, BuildTarget: "linux64"}) {
		t.Fatalf("the platform choice must reach the CLI as -buildTarget: %v", f.Calls())
	}
	if !m.st.PendingFor(old.Path).Empty() {
		t.Error("an applied choice must be forgotten")
	}
}

func TestOpenWithAnotherVersionAsksFirst(t *testing.T) {
	m := newTestModel(t)
	old := selectProject(t, m, "Old Platformer") // 2022.3.62f1
	m.st.SetPending(old.Path, state.Pending{Version: "6000.6.8f1"})
	m.refreshProjects()
	selectProject(t, m, "Old Platformer")
	f := useFakeCLI(t, m)
	cmd := m.keymap.lookup(ctxProjects, "enter").run(m)
	if cmd != nil || len(m.popups) != 1 || !strings.Contains(screenText(m), "Open with another version") {
		t.Fatalf("changing version needs a confirmation:\n%s", screenText(m))
	}
	if len(f.Calls()) != 0 {
		t.Error("nothing runs before the user agrees")
	}
	_, cmd = m.Update(keyMsg("enter")) // agree
	drain(m, cmd)
	if !f.called("Open", unitycli.OpenOptions{Project: old.Path, EditorVersion: "6000.6.8f1"}) {
		t.Errorf("after agreeing: %v", f.Calls())
	}
}

func TestOpenNeedsAnInstalledEditor(t *testing.T) {
	m := newTestModel(t)
	// Ancient Thing wants 2019.4.40f1, which is not installed, and its folder is "missing"
	m.missing = map[string]bool{}
	m.refreshProjects()
	r := selectProject(t, m, "Ancient Thing")
	os.MkdirAll(r.Path, 0o755)
	cmd := m.keymap.lookup(ctxProjects, "enter").run(m)
	if cmd != nil || len(m.popups) != 1 || !strings.Contains(screenText(m), "Editor not installed") || !strings.Contains(screenText(m), "2019.4.40f1") {
		t.Fatalf("a missing Editor should offer to install it:\n%s", screenText(m))
	}
}

func TestOpenMissingFolder(t *testing.T) {
	m := newTestModel(t)
	selectProject(t, m, "Ancient Thing") // its folder does not exist
	if cmd := m.keymap.lookup(ctxProjects, "enter").run(m); cmd != nil {
		t.Error("nothing to run")
	}
	if len(m.popups) != 0 || !strings.Contains(screenText(m), "folder of Ancient Thing is gone") {
		t.Errorf("screen:\n%s", screenText(m))
	}
}

func TestOpenWithoutTheCLIOffersADirectLaunch(t *testing.T) {
	m := newTestModel(t)
	m.cli = nil
	selectProject(t, m, "Space Shooter")
	m.keymap.lookup(ctxProjects, "enter").run(m)
	out := screenText(m)
	if len(m.popups) != 1 || !strings.Contains(out, "Open without the Unity CLI") || !strings.Contains(out, "starts signed out") {
		t.Fatalf("screen:\n%s", out)
	}
	press(m, "esc")
	if len(m.popups) != 0 {
		t.Error("the user can decline")
	}
}

func TestDirectLaunchStartsTheEditor(t *testing.T) {
	m := newTestModel(t)
	m.cli = nil
	r := selectProject(t, m, "Space Shooter")
	ed := m.editorFor("6000.6.8f1")
	p := m.planOpen(r)
	if p.route != openDirect {
		t.Fatalf("without the CLI the Editor is started directly: route %d", p.route)
	}
	m.directLaunch(r, p)
	if got := recorder(m).editors; len(got) != 1 || got[0] != ed.Location+" "+r.Path {
		t.Errorf("the Editor was started with %q", got)
	}
	if !strings.Contains(screenText(m), "signed out") {
		t.Error("the user is told the Editor starts signed out")
	}
}

func TestOpenFailureOffersDirectLaunch(t *testing.T) {
	m := newTestModel(t)
	selectProject(t, m, "Space Shooter")
	f := useFakeCLI(t, m)
	f.err["Open"] = &unitycli.Error{ExitCode: unitycli.ExitCommandFailed, Message: "the identity server did not start", Output: "details here"}
	drain(m, m.keymap.lookup(ctxProjects, "enter").run(m))
	out := screenText(m)
	for _, want := range []string{"Could not open Space Shooter", "the identity server did not start", "details here", "Open directly"} {
		if !strings.Contains(out, want) {
			t.Errorf("should contain %q:\n%s", want, out)
		}
	}
}

func TestUpgradeToUnity67GoesThroughUpgrade(t *testing.T) {
	m := newTestModel(t)
	// pretend a 6000.7 Editor is installed
	pretendInstalled(m, editorRow{Version: "6000.7.0b8", Location: "/x/Unity", Platforms: m.inv.editors()[0].Platforms})
	old := selectProject(t, m, "Old Platformer")
	m.st.SetPending(old.Path, state.Pending{Version: "6000.7.0b8"})
	m.refreshProjects()
	selectProject(t, m, "Old Platformer")
	f := useFakeCLI(t, m)
	m.keymap.lookup(ctxProjects, "enter").run(m)
	if len(m.popups) != 1 || !strings.Contains(screenText(m), "Upgrade without Unity Cloud") {
		t.Fatalf("`open` cannot move a project to 6.7, so an upgrade menu is offered:\n%s", screenText(m))
	}
	_, cmd := m.Update(keyMsg("enter"))
	drain(m, cmd)
	if !f.called("UpgradeProject", old.Path, "6000.7.0b8", unitycli.CloudOff) {
		t.Errorf("calls: %v", f.Calls())
	}
	if !m.st.PendingFor(old.Path).Empty() {
		t.Error("the choice is applied by the upgrade")
	}
}

func TestUpgradeToUnity67KeepsThePendingPlatform(t *testing.T) {
	m := newTestModel(t)
	pretendInstalled(m, editorRow{Version: "6000.7.0b8", Location: "/x/Unity", Platforms: m.inv.editors()[0].Platforms})
	old := selectProject(t, m, "Old Platformer")
	m.st.SetPending(old.Path, state.Pending{Version: "6000.7.0b8", BuildTarget: "StandaloneLinux64"})
	m.refreshProjects()
	selectProject(t, m, "Old Platformer")
	useFakeCLI(t, m)
	m.keymap.lookup(ctxProjects, "enter").run(m)
	_, cmd := m.Update(keyMsg("enter"))
	drain(m, cmd)
	if got := m.st.PendingFor(old.Path); got != (state.Pending{BuildTarget: "StandaloneLinux64"}) {
		t.Errorf("the upgrade does not switch the platform, so the choice must stay for the next open: %+v", got)
	}
}

func TestVersionMenuSetsAPendingVersion(t *testing.T) {
	m := newTestModel(t)
	space := selectProject(t, m, "Space Shooter") // 6000.6.8f1
	press(m, "v")
	if len(m.popups) != 1 {
		t.Fatal("v opens the version menu")
	}
	out := screenText(m)
	for _, want := range []string{"Unity version for Space Shooter", "6000.6.8f1", "2022.3.62f1", "project's version", "Install another version…"} {
		if !strings.Contains(out, want) {
			t.Errorf("the menu should contain %q", want)
		}
	}
	press(m, "j", "enter") // 2022.3.62f1
	r := selectProject(t, m, "Space Shooter")
	if r.Pending.Version != "2022.3.62f1" || !r.versionChanged() {
		t.Fatalf("pending = %+v", r.Pending)
	}
	if got := m.st.PendingFor(space.Path); got.Version != "2022.3.62f1" {
		t.Errorf("the choice must be remembered: %+v", got)
	}
	if !strings.Contains(screenText(m), "6000.6.8f1→2022.3.62f1") {
		t.Errorf("the list shows old→new:\n%s", screenText(m))
	}
	// choosing the project's own version again clears it
	press(m, "v", "k", "enter")
	if r := selectProject(t, m, "Space Shooter"); !r.Pending.Empty() {
		t.Errorf("choosing the project's own version should clear the choice: %+v", r.Pending)
	}
	// the choice survives a restart: it is on disk
	if err := m.st.Save(); err != nil {
		t.Fatal(err)
	}
}

func TestVersionMenuOffersToInstallAMissingVersion(t *testing.T) {
	m := newTestModel(t)
	selectProject(t, m, "Ancient Thing") // 2019.4.40f1 is not installed
	press(m, "v")
	if !strings.Contains(screenText(m), "Install 2019.4.40f1…") {
		t.Errorf("screen:\n%s", screenText(m))
	}
}

func TestPlatformMenuSetsAPendingPlatform(t *testing.T) {
	m := newTestModel(t)
	old := selectProject(t, m, "Old Platformer") // 2022.3.62f1 has Linux and Android
	press(m, "p")
	out := screenText(m)
	for _, want := range []string{"Platform for Old Platformer (Unity 2022.3.62f1)", "Linux", "Android", "current", "Add a platform module…"} {
		if !strings.Contains(out, want) {
			t.Errorf("the menu should contain %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "WebGL") {
		t.Error("only the platforms installed for that Editor are offered")
	}
	menu := m.popups[0].(*menuPopup)
	if cur, _ := menu.list.Selected(); cur.label != "Android" {
		t.Errorf("the menu should start on the project's platform, got %q", cur.label)
	}
	press(m, "k", "enter") // Linux
	r := selectProject(t, m, "Old Platformer")
	if r.Pending.BuildTarget != "StandaloneLinux64" || !r.targetChanged() {
		t.Fatalf("pending = %+v", r.Pending)
	}
	if !strings.Contains(screenText(m), "Android→Linux") {
		t.Errorf("the list shows the change:\n%s", screenText(m))
	}
	_ = old
	// picking the current platform again clears it
	press(m, "p", "j", "enter")
	if r := selectProject(t, m, "Old Platformer"); !r.Pending.Empty() {
		t.Errorf("pending = %+v", r.Pending)
	}
}

func TestChangingVersionDropsAPlatformTheNewEditorLacks(t *testing.T) {
	m := newTestModel(t)
	old := selectProject(t, m, "Old Platformer") // Android pending is fine on 2022.3 (has Android)
	m.st.SetPending(old.Path, state.Pending{BuildTarget: "Android"})
	m.refreshProjects()
	r := selectProject(t, m, "Old Platformer")
	// 6000.6.8f1 has no Android module
	m.setPendingVersion(r, "6000.6.8f1")
	got := m.st.PendingFor(old.Path)
	if got.Version != "6000.6.8f1" || got.BuildTarget != "" {
		t.Errorf("the platform must be dropped with a warning: %+v", got)
	}
	if !strings.Contains(screenText(m), "does not have that platform") {
		t.Error("the user should be told")
	}
}

func TestPlatformMenuNeedsTheEditor(t *testing.T) {
	m := newTestModel(t)
	selectProject(t, m, "Ancient Thing")
	press(m, "p")
	if len(m.popups) != 0 || !strings.Contains(screenText(m), "Unity 2019.4.40f1 is not installed") {
		t.Errorf("screen:\n%s", screenText(m))
	}
}

func TestPinRunsPinAndReloads(t *testing.T) {
	m := newTestModel(t)
	old := selectProject(t, m, "Old Platformer")
	f := useFakeCLI(t, m)
	f.projects = fixture[unitycli.Project](t, "projects.json")
	drain(m, m.keymap.lookup(ctxProjects, "f").run(m))
	if !f.called("SetPinned", old.Path, true) || !f.called("Projects") {
		t.Errorf("calls: %v", f.Calls())
	}
	if !strings.Contains(screenText(m), "Pinned Old Platformer") {
		t.Error("the user should be told")
	}

	m = newTestModel(t)                           // the reload replaced the list, so start again for the other direction
	space := selectProject(t, m, "Space Shooter") // already pinned: unpin
	f = useFakeCLI(t, m)
	drain(m, m.keymap.lookup(ctxProjects, "f").run(m))
	if !f.called("SetPinned", space.Path, false) {
		t.Errorf("calls: %v", f.Calls())
	}
}

func TestAddProject(t *testing.T) {
	m := newTestModel(t)
	dir := t.TempDir()
	proj := filepath.Join(dir, "New Game")
	os.MkdirAll(filepath.Join(proj, "ProjectSettings"), 0o755)
	os.WriteFile(filepath.Join(proj, "ProjectSettings", "ProjectVersion.txt"), []byte("m_EditorVersion: 6000.6.8f1\n"), 0o644)
	notProject := filepath.Join(dir, "Empty")
	os.MkdirAll(notProject, 0o755)
	f := useFakeCLI(t, m)

	press(m, "a")
	pp := m.popups[0].(*promptPopup)
	pp.in.Set(filepath.Join(dir, "nope"))
	press(m, "enter")
	if len(m.popups) != 1 || !strings.Contains(screenText(m), "That folder does not exist") {
		t.Error("a missing folder is refused")
	}
	pp.in.Set(notProject)
	press(m, "enter")
	if len(m.popups) != 1 || !strings.Contains(screenText(m), "Not a Unity project") {
		t.Error("a folder that is not a Unity project is refused")
	}
	pp.in.Set(proj)
	_, cmd := m.Update(keyMsg("enter"))
	drain(m, cmd)
	if !f.called("AddProjects", []string{proj}) || len(m.popups) != 0 {
		t.Errorf("calls: %v popups=%d", f.Calls(), len(m.popups))
	}
	if !strings.Contains(screenText(m), "Added New Game") {
		t.Error("the user should be told")
	}
}

func TestRemoveFromList(t *testing.T) {
	m := newTestModel(t)
	old := selectProject(t, m, "Old Platformer")
	m.st.SetPending(old.Path, state.Pending{BuildTarget: "iOS"})
	m.saveState()
	f := useFakeCLI(t, m)
	press(m, "d")
	if len(m.popups) != 1 || !strings.Contains(screenText(m), "Remove from the list") || !strings.Contains(screenText(m), "Delete the folder from disk…") {
		t.Fatalf("screen:\n%s", screenText(m))
	}
	_, cmd := m.Update(keyMsg("enter"))
	drain(m, cmd)
	if !f.called("RemoveProjects", []string{old.Path}) {
		t.Errorf("calls: %v", f.Calls())
	}
	if !m.st.PendingFor(old.Path).Empty() {
		t.Error("choices for a removed project are forgotten")
	}
	if !state.Load(m.configDir).PendingFor(old.Path).Empty() {
		t.Error("and that is saved at once, not only when lazyunity quits")
	}
	if _, err := os.Stat(old.Path); err != nil {
		t.Error("removing from the list must not touch the folder")
	}
}

func TestDeleteFromDiskNeedsTwoConfirmationsAndASafePath(t *testing.T) {
	m := newTestModel(t)
	dir := t.TempDir()
	proj := filepath.Join(dir, "Doomed")
	os.MkdirAll(filepath.Join(proj, "ProjectSettings"), 0o755)
	os.WriteFile(filepath.Join(proj, "ProjectSettings", "ProjectVersion.txt"), []byte("m_EditorVersion: 6000.6.8f1\n"), 0o644)
	os.WriteFile(filepath.Join(proj, "important.txt"), []byte("x"), 0o644)
	r := projectRow{}
	r.Title, r.Path, r.Version = "Doomed", proj, "6000.6.8f1"
	m.st.SetPending(proj, state.Pending{BuildTarget: "iOS"})
	m.saveState()
	f := useFakeCLI(t, m)

	m.deleteProjectFlow(r)
	if !strings.Contains(screenText(m), "Delete the folder of Doomed from disk?") {
		t.Fatal("first question")
	}
	press(m, "esc")
	if _, err := os.Stat(proj); err != nil {
		t.Fatal("declining the first question must delete nothing")
	}
	m.deleteProjectFlow(r)
	press(m, "enter") // first yes
	if !strings.Contains(screenText(m), "permanently deletes") {
		t.Fatal("a second question must follow")
	}
	press(m, "esc")
	if _, err := os.Stat(proj); err != nil {
		t.Fatal("declining the second question must delete nothing")
	}
	m.deleteProjectFlow(r)
	press(m, "enter")
	_, cmd := m.Update(keyMsg("enter")) // second yes: the delete task
	drain(m, cmd)
	if _, err := os.Stat(proj); !os.IsNotExist(err) {
		t.Error("the folder should be gone")
	}
	if !f.called("RemoveProjects", []string{proj}) {
		t.Errorf("the project also leaves the list: %v", f.Calls())
	}
	if !state.Load(m.configDir).PendingFor(proj).Empty() {
		t.Error("the deleted project's choices are forgotten, and that is saved")
	}
}

func TestSafeToDelete(t *testing.T) {
	dir := t.TempDir()
	proj := filepath.Join(dir, "P")
	os.MkdirAll(filepath.Join(proj, "ProjectSettings"), 0o755)
	os.WriteFile(filepath.Join(proj, "ProjectSettings", "ProjectVersion.txt"), []byte("m_EditorVersion: 2022.3.1f1\n"), 0o644)
	plain := filepath.Join(dir, "plain")
	os.MkdirAll(plain, 0o755)

	cases := []struct {
		path string
		ok   bool
	}{
		{proj, true},
		{plain, false},           // not a Unity project
		{"/", false},             // the root
		{homeDir(), false},       // the home folder
		{"relative/path", false}, // not absolute
		{proj + "/../P", false},  // not clean
		{filepath.Join(dir, "nope"), false},
		{"", false},
	}
	for _, c := range cases {
		if got := safeToDelete(c.path); (got == "") != c.ok {
			t.Errorf("safeToDelete(%q) = %q, want ok=%v", c.path, got, c.ok)
		}
	}
}

func TestDeleteRefusesAnUnsafeFolder(t *testing.T) {
	m := newTestModel(t)
	r := projectRow{}
	r.Title, r.Path = "Home", homeDir()
	m.deleteProjectFlow(r)
	if len(m.popups) != 0 || !strings.Contains(screenText(m), "Not deleting") {
		t.Errorf("screen:\n%s", screenText(m))
	}
}

func TestSortMenu(t *testing.T) {
	m := newTestModel(t)
	names := func() string {
		var n []string
		for _, r := range m.projectsTab.Items() {
			n = append(n, r.name())
		}
		return strings.Join(n, ",")
	}
	if names() != "Space Shooter,Old Platformer,Ancient Thing" {
		t.Fatalf("start: %s", names())
	}
	press(m, "s", "j", "enter") // Name
	if m.st.Sort != state.SortName {
		t.Fatalf("sort = %q", m.st.Sort)
	}
	// pinned projects stay on top, the rest by name
	if names() != "Space Shooter,Ancient Thing,Old Platformer" {
		t.Errorf("by name: %s", names())
	}
	press(m, "s", "j", "enter") // Unity version, newest first
	if m.st.Sort != state.SortVersion || names() != "Space Shooter,Old Platformer,Ancient Thing" {
		t.Errorf("by version: %s (sort %q)", names(), m.st.Sort)
	}
	press(m, "s", "k", "k", "enter") // back to last opened
	if m.st.Sort != state.SortLastOpened {
		t.Errorf("sort = %q", m.st.Sort)
	}
}

func TestProjectInfo(t *testing.T) {
	m := newTestModel(t)
	space := selectProject(t, m, "Space Shooter")
	f := useFakeCLI(t, m)
	f.projectInfo = unitycli.ProjectInfo{
		Project:          unitycli.Project{Title: "Space Shooter", Path: space.Path, Version: "6000.6.8f1", Changeset: "237b6593792b", BuildTarget: "StandaloneLinux64", RenderPipeline: "URP", CloudEnabled: true},
		ScriptingBackend: map[string]string{"Android": "IL2CPP"},
		Packages:         map[string]string{"com.unity.inputsystem": "1.20.1", "com.unity.modules.ui": "1.0.0", "com.unity.ugui": "2.6.0"},
	}
	drain(m, m.keymap.lookup(ctxProjects, "i").run(m))
	if !f.called("ProjectInfo", space.Path) {
		t.Errorf("calls: %v", f.Calls())
	}
	out := screenText(m)
	for _, want := range []string{"Space Shooter", "6000.6.8f1 (237b6593792b)", "URP", "Unity Cloud", "yes", "IL2CPP", "com.unity.inputsystem", "1.20.1", "1 built-in modules"} {
		if !strings.Contains(out, want) {
			t.Errorf("the details should contain %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "com.unity.modules.ui") {
		t.Error("built-in modules are counted, not listed")
	}
}

func TestCopyMenu(t *testing.T) {
	m := newTestModel(t)
	r := selectProject(t, m, "Space Shooter")
	press(m, "y")
	out := screenText(m)
	for _, want := range []string{"Copy", "Path", "Name", "Unity version", "Space Shooter"} {
		if !strings.Contains(out, want) {
			t.Errorf("should contain %q", want)
		}
	}
	_, cmd := m.Update(keyMsg("enter"))
	if cmd == nil || !strings.Contains(screenText(m), "Copied the path") {
		t.Error("choosing an item copies it and says so")
	}
	_ = r
	// ctrl+o copies the path directly
	m.toast = toast{}
	_, cmd = m.Update(keyMsg("ctrl+o"))
	if cmd == nil || !strings.Contains(screenText(m), "Copied") {
		t.Error("ctrl+o copies the selected path")
	}
}

func TestClean(t *testing.T) {
	m := newTestModel(t)
	space := selectProject(t, m, "Space Shooter")
	f := useFakeCLI(t, m)
	press(m, "C")
	if len(m.popups) != 1 || !strings.Contains(screenText(m), "Clean project") || !strings.Contains(screenText(m), "rebuild") {
		t.Fatalf("screen:\n%s", screenText(m))
	}
	if len(f.Calls()) != 0 {
		t.Error("nothing runs before the user agrees")
	}
	_, cmd := m.Update(keyMsg("enter"))
	drain(m, cmd)
	if !f.called("CleanProject", space.Path) || !strings.Contains(screenText(m), "Cleaned Space Shooter") {
		t.Errorf("calls: %v", f.Calls())
	}
}

func TestProjectActionsNeedAProject(t *testing.T) {
	m := newTestModel(t)
	m.projectsTab.SetItems(nil)
	for _, k := range []string{"enter", "v", "p", "f", "d", "o", "e", "i", "y", "C"} {
		press(m, k)
		if len(m.popups) != 0 {
			t.Errorf("%s with no project opened a popup", k)
		}
	}
	// the list-wide actions still work
	press(m, "s")
	if len(m.popups) != 1 {
		t.Error("sorting an empty list is harmless")
	}
	m.popups = nil
	if hints := m.keymap.hints(m, ctxProjects); len(hints) != 3 { // New, Add, Keybindings
		t.Errorf("hints with no project: %v", hints)
	}
}

func TestNewProjectNameValidation(t *testing.T) {
	good := []string{"My Game", "game-2", "a", "Ünity ゲーム"}
	bad := []string{"", ".", "..", "a/b", `a\b`, "a:b", "a*b", " lead", "trail ", "dots.", `q"`, "a|b", "a?b", "<x>", "-a", "--open", "--path"}
	for _, n := range good {
		if got := validateProjectName(n); got != "" {
			t.Errorf("%q should be fine, got %q", n, got)
		}
	}
	for _, n := range bad {
		if validateProjectName(n) == "" {
			t.Errorf("%q should be refused", n)
		}
	}
}

func TestNewProjectWizard(t *testing.T) {
	m := newTestModel(t)
	parent := t.TempDir()
	f := useFakeCLI(t, m)
	f.templates = fixture[unitycli.Template](t, "templates.json")

	cmd := m.keymap.lookup(ctxProjects, "n").run(m)
	if cmd != nil || len(m.popups) != 1 || !strings.Contains(screenText(m), "New project (1/5)") {
		t.Fatalf("step 1:\n%s", screenText(m))
	}
	press(m, "a/b", "enter") // an invalid name stays put
	press(m, "backspace", "backspace", "backspace")
	for _, k := range []string{"M", "y", "space", "G", "a", "m", "e"} {
		press(m, k)
	}
	press(m, "enter")
	if !strings.Contains(screenText(m), "New project (2/5)") {
		t.Fatalf("step 2:\n%s", screenText(m))
	}
	m.popups[0].(*promptPopup).in.Set(parent)
	press(m, "enter")
	if !strings.Contains(screenText(m), "New project (3/5)") || !strings.Contains(screenText(m), "6000.6.8f1") {
		t.Fatalf("step 3:\n%s", screenText(m))
	}
	_, cmd = m.Update(keyMsg("enter")) // the first Editor, 6000.6.8f1
	drain(m, cmd)
	if !f.called("Templates", "6000.6.8f1") {
		t.Errorf("the chosen Editor's templates are listed: %v", f.Calls())
	}
	out := screenText(m)
	if !strings.Contains(out, "New project (4/5)") || !strings.Contains(out, "3D URP") || !strings.Contains(out, "Universal 2D") {
		t.Fatalf("step 4:\n%s", out)
	}
	press(m, "j", "enter") // 3D URP
	if !strings.Contains(screenText(m), "New project (5/5)") || !strings.Contains(screenText(m), "[ ] Link to Unity Cloud") || !strings.Contains(screenText(m), "[x] Add Unity's recommended") {
		t.Fatalf("step 5:\n%s", screenText(m))
	}
	_, cmd = m.Update(keyMsg("enter")) // the defaults: no cloud, recommended packages and settings (not bare)
	drain(m, cmd)
	want := unitycli.CreateOptions{
		Name: "My Game", ParentDir: parent, EditorVersion: "6000.6.8f1", Template: "com.unity.template.urp-blank",
		Cloud: unitycli.CloudOff, Open: true,
	}
	if !f.called("CreateProject", want) {
		t.Errorf("calls: %v", f.Calls())
	}
	if !strings.Contains(screenText(m), "Created My Game") {
		t.Error("the user should be told")
	}
	if m.selectAfterLoad != "" && !strings.HasSuffix(m.selectAfterLoad, "My Game") {
		t.Errorf("selectAfterLoad = %q", m.selectAfterLoad)
	}
}

func TestNewProjectWizardRefusesAnExistingFolder(t *testing.T) {
	m := newTestModel(t)
	useFakeCLI(t, m)
	parent := t.TempDir()
	os.MkdirAll(filepath.Join(parent, "Taken"), 0o755)
	m.newProjectLocation(newProject{name: "Taken"})
	m.popups[0].(*promptPopup).in.Set(parent)
	press(m, "enter")
	if len(m.popups) != 1 || !strings.Contains(screenText(m), "already something called Taken") {
		t.Errorf("screen:\n%s", screenText(m))
	}
}

func TestNewProjectNeedsAnEditorAndTheCLI(t *testing.T) {
	m := newTestModel(t)
	m.cli = nil
	m.keymap.lookup(ctxProjects, "n").run(m)
	if !strings.Contains(screenText(m), "Unity CLI needed") {
		t.Error("without the CLI nothing can be created")
	}
	m.popups = nil
	useFakeCLI(t, m)
	m.inv.setFound(nil, nil)
	m.refreshEditors()
	cmd := m.keymap.lookup(ctxProjects, "n").run(m)
	_ = cmd
	if !strings.Contains(screenText(m), "Install a Unity Editor first") || m.panels[panelEditors].cur != 1 {
		t.Errorf("with no Editor the user is sent to the releases:\n%s", screenText(m))
	}
}

func TestCompleteDir(t *testing.T) {
	dir := t.TempDir()
	for _, d := range []string{"alpha", "alpine", "beta", "xа", "xб"} {
		os.MkdirAll(filepath.Join(dir, d), 0o755)
	}
	os.WriteFile(filepath.Join(dir, "afile"), nil, 0o644)
	sep := string(filepath.Separator)

	cases := []struct {
		in   string
		want []string
	}{
		{filepath.Join(dir, "be"), []string{filepath.Join(dir, "beta") + sep}}, // one match: complete it, with a separator
		{filepath.Join(dir, "al"), []string{filepath.Join(dir, "alp")}},        // several: as far as they agree
		{filepath.Join(dir, "a"), []string{filepath.Join(dir, "alp")}},         // the file afile is never offered
		{filepath.Join(dir, "alp"), nil},                                       // they disagree from here on: nothing to add
		{filepath.Join(dir, "zzz"), nil},
		{filepath.Join(dir, "afile"), nil},     // a file is not a folder
		{filepath.Join(dir, "nope", "x"), nil}, // a missing parent
		{dir + sep, nil},                       // alpha, alpine, beta share nothing
		{filepath.Join(dir, "x"), nil},         // xа and xб share only x: a character is never split
	}
	for _, c := range cases {
		got := completeDir(c.in)
		if len(got) != len(c.want) || (len(got) == 1 && got[0] != c.want[0]) {
			t.Errorf("completeDir(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestSelectedPath(t *testing.T) {
	m := newTestModel(t)
	if p, ok := m.selectedPath(); !ok || !strings.HasSuffix(p, "space") {
		t.Errorf("projects: %q %v", p, ok)
	}
	press(m, "2")
	if p, ok := m.selectedPath(); !ok || !strings.Contains(p, "6000.6.8f1") {
		t.Errorf("editors: %q %v", p, ok)
	}
	press(m, "3")
	if _, ok := m.selectedPath(); ok {
		t.Error("the Tools tab has no paths")
	}
	press(m, "]")
	if p, ok := m.selectedPath(); !ok || p == "" {
		t.Errorf("roots: %q %v", p, ok)
	}
	press(m, "]")
	if cmd := m.copySelectedPath(); cmd != nil || !strings.Contains(screenText(m), "Nothing here has a path") {
		t.Error("copying nothing says so")
	}
}

func TestAddPackageNeedsOpenUPM(t *testing.T) {
	m := newTestModel(t)
	press(m, "ctrl+a")
	if len(m.popups) != 0 || m.keymap.lookup(ctxProjects, "ctrl+a").enabled(m) {
		t.Errorf("without OpenUPM the action is inactive:\n%s", screenText(m))
	}
	m.openupm = openupmState{checked: true, path: "/usr/bin/openupm"}
	if !m.keymap.lookup(ctxProjects, "ctrl+a").enabled(m) {
		t.Error("with OpenUPM the action is active")
	}
	press(m, "ctrl+a")
	if _, ok := m.popups[len(m.popups)-1].(*promptPopup); !ok || !strings.Contains(screenText(m), "Add an OpenUPM package") {
		t.Errorf("with OpenUPM a package name is asked for:\n%s", screenText(m))
	}
	press(m, "enter")
	if !strings.Contains(screenText(m), "Type a package name") {
		t.Errorf("an empty name is refused:\n%s", screenText(m))
	}
}

func TestNewProjectStartsOnTheDefaultEditor(t *testing.T) {
	m := newTestModel(t)
	m.inv.editors()[1].Default = true // 2022.3.62f1, second in the list
	m.newProjectEditor(newProject{name: "G", parent: t.TempDir()})
	menu := m.popups[len(m.popups)-1].(*menuPopup)
	if it, _ := menu.list.Selected(); it.label != m.inv.editors()[1].Version {
		t.Errorf("the cursor starts on the default Editor, got %q", it.label)
	}
}

func TestEditProject(t *testing.T) {
	m := newTestModel(t)
	selectProject(t, m, "Space Shooter")
	m.env.Getenv = func(string) string { return "" }
	if cmd := m.keymap.lookup(ctxProjects, "e").run(m); cmd != nil || !strings.Contains(screenText(m), "Set os.edit in config.yml, or $EDITOR") {
		t.Errorf("without an editor the user is told how to set one:\n%s", screenText(m))
	}
	m.env.Getenv = func(k string) string { return map[string]string{"EDITOR": "vim"}[k] }
	if cmd := m.keymap.lookup(ctxProjects, "e").run(m); cmd == nil {
		t.Error("with $EDITOR the terminal is handed to it")
	}
}

func TestOpenTheProjectFolder(t *testing.T) {
	m := newTestModel(t)
	space := selectProject(t, m, "Space Shooter")
	press(m, "o")
	if got := recorder(m).folders; len(got) != 1 || got[0] != space.Path {
		t.Errorf("o shows the project's folder: %v", got)
	}
	recorder(m).err = errors.New("no file manager")
	press(m, "o")
	if !strings.Contains(screenText(m), "Could not open the folder: no file manager") {
		t.Errorf("a failure is said:\n%s", screenText(m))
	}
}

func TestAddAPackage(t *testing.T) {
	m := newTestModel(t)
	selectProject(t, m, "Space Shooter")
	m.openupm = openupmState{checked: true, path: "/usr/bin/openupm"}
	press(m, "ctrl+a")
	m.popups[len(m.popups)-1].(*promptPopup).in.Set("com.example.a com.example.b@1.2.0")
	_, cmd := m.Update(keyMsg("enter"))
	drain(m, cmd)
	if got := recorder(m).added; len(got) != 1 || strings.Join(got[0], ",") != "com.example.a,com.example.b@1.2.0" {
		t.Errorf("the packages are added with the OpenUPM CLI: %v", got)
	}
	if !strings.Contains(screenText(m), "Added com.example.a, com.example.b@1.2.0 to Space Shooter") {
		t.Errorf("screen:\n%s", screenText(m))
	}
}
