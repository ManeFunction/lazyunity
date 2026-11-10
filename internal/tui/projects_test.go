package tui

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/ManeFunction/lazyunity/internal/state"
)

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

func reloadProjectsResponse() fakeResp { return fakeResp{File: "projects.json"} }

func TestOpenProjectRunsTheCLI(t *testing.T) {
	m := newTestModel(t)
	space := selectProject(t, m, "Space Shooter")
	c := useFakeCLI(t, m, map[string]fakeResp{
		"open " + space.Path: {Stdout: `{"success":true,"command":"open","data":{"alreadyOpen":false}}`},
		"projects --all":     reloadProjectsResponse(),
	})
	drain(m, m.keymap.lookup(ctxProjects, "enter").run(m))
	if !called(c, "open "+space.Path) {
		t.Fatalf("the CLI should have been asked to open the project: %v", calls(c))
	}
	if !called(c, "projects --all") {
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
	space := selectProject(t, m, "Space Shooter")
	useFakeCLI(t, m, map[string]fakeResp{
		"open " + space.Path: {Stdout: `{"success":true,"command":"open","data":{"alreadyOpen":true}}`},
	})
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
	c := useFakeCLI(t, m, map[string]fakeResp{
		"open " + old.Path + " --build-target linux64": {Stdout: `{"success":true,"command":"open","data":{}}`},
		"projects --all": reloadProjectsResponse(),
	})
	drain(m, m.keymap.lookup(ctxProjects, "enter").run(m))
	if !called(c, "open "+old.Path+" --build-target linux64") {
		t.Fatalf("the platform choice must reach the CLI as -buildTarget: %v", calls(c))
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
	c := useFakeCLI(t, m, map[string]fakeResp{
		"open " + old.Path + " --editor-version 6000.6.8f1": {Stdout: `{"success":true,"command":"open","data":{}}`},
		"projects --all": reloadProjectsResponse(),
	})
	cmd := m.keymap.lookup(ctxProjects, "enter").run(m)
	if cmd != nil || len(m.popups) != 1 || !strings.Contains(screenText(m), "Open with another version") {
		t.Fatalf("changing version needs a confirmation:\n%s", screenText(m))
	}
	if len(calls(c)) != 0 {
		t.Error("nothing runs before the user agrees")
	}
	_, cmd = m.Update(keyMsg("enter")) // agree
	drain(m, cmd)
	if !called(c, "open "+old.Path+" --editor-version 6000.6.8f1") {
		t.Errorf("after agreeing: %v", calls(c))
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
	marker := filepath.Join(t.TempDir(), "args")
	script := ed.Location
	os.WriteFile(script, []byte("#!/bin/sh\nprintf '%s ' \"$@\" > '"+marker+"'\n"), 0o755)
	m.directLaunch(r, ed)
	var got []byte
	for i := 0; i < 200 && len(got) == 0; i++ {
		got, _ = os.ReadFile(marker)
		if len(got) == 0 {
			sleepMs(10)
		}
	}
	if !strings.HasPrefix(string(got), "-projectPath "+r.Path) {
		t.Errorf("the Editor was started with %q", got)
	}
	if !strings.Contains(screenText(m), "signed out") {
		t.Error("the user is told the Editor starts signed out")
	}
}

func TestOpenFailureOffersDirectLaunch(t *testing.T) {
	m := newTestModel(t)
	space := selectProject(t, m, "Space Shooter")
	useFakeCLI(t, m, map[string]fakeResp{
		"open " + space.Path: {Stdout: `{"success":false,"errors":[{"code":"X","message":"the identity server did not start"}]}`, Exit: 6, Stderr: "details here"},
	})
	drain(m, m.keymap.lookup(ctxProjects, "enter").run(m))
	out := screenText(m)
	for _, want := range []string{"Could not open Space Shooter", "the identity server did not start", "Open directly"} {
		if !strings.Contains(out, want) {
			t.Errorf("should contain %q:\n%s", want, out)
		}
	}
}

func TestUpgradeToUnity67GoesThroughUpgrade(t *testing.T) {
	m := newTestModel(t)
	// pretend a 6000.7 Editor is installed
	m.editors = append(m.editors, editorRow{Version: "6000.7.0b8", Location: "/x/Unity", Platforms: m.editors[0].Platforms})
	m.installed["6000.7.0b8"] = true
	old := selectProject(t, m, "Old Platformer")
	m.st.SetPending(old.Path, state.Pending{Version: "6000.7.0b8"})
	m.refreshProjects()
	selectProject(t, m, "Old Platformer")
	c := useFakeCLI(t, m, map[string]fakeResp{
		"projects upgrade " + old.Path + " --to 6000.7.0b8 --yes --no-cloud": {Stdout: okJSON},
		"projects --all": reloadProjectsResponse(),
	})
	m.keymap.lookup(ctxProjects, "enter").run(m)
	if len(m.popups) != 1 || !strings.Contains(screenText(m), "Upgrade without Unity Cloud") {
		t.Fatalf("`open` cannot move a project to 6.7, so an upgrade menu is offered:\n%s", screenText(m))
	}
	_, cmd := m.Update(keyMsg("enter"))
	drain(m, cmd)
	if !called(c, "projects upgrade "+old.Path+" --to 6000.7.0b8 --yes --no-cloud") {
		t.Errorf("calls: %v", calls(c))
	}
	if !m.st.PendingFor(old.Path).Empty() {
		t.Error("the choice is applied by the upgrade")
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
	c := useFakeCLI(t, m, map[string]fakeResp{
		"projects pin " + old.Path: {Stdout: okJSON},
		"projects --all":           reloadProjectsResponse(),
	})
	drain(m, m.keymap.lookup(ctxProjects, "f").run(m))
	if !called(c, "projects pin "+old.Path) || !called(c, "projects --all") {
		t.Errorf("calls: %v", calls(c))
	}
	if !strings.Contains(screenText(m), "Pinned Old Platformer") {
		t.Error("the user should be told")
	}

	m = newTestModel(t)                           // the reload replaced the list, so start again for the other direction
	space := selectProject(t, m, "Space Shooter") // already pinned: unpin
	c = useFakeCLI(t, m, map[string]fakeResp{
		"projects unpin " + space.Path: {Stdout: okJSON},
		"projects --all":               reloadProjectsResponse(),
	})
	drain(m, m.keymap.lookup(ctxProjects, "f").run(m))
	if !called(c, "projects unpin "+space.Path) {
		t.Errorf("calls: %v", calls(c))
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
	c := useFakeCLI(t, m, map[string]fakeResp{
		"projects add " + proj: {Stdout: okJSON},
		"projects --all":       reloadProjectsResponse(),
	})

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
	if !called(c, "projects add "+proj) || len(m.popups) != 0 {
		t.Errorf("calls: %v popups=%d", calls(c), len(m.popups))
	}
	if !strings.Contains(screenText(m), "Added New Game") {
		t.Error("the user should be told")
	}
}

func TestRemoveFromList(t *testing.T) {
	m := newTestModel(t)
	old := selectProject(t, m, "Old Platformer")
	m.st.SetPending(old.Path, state.Pending{BuildTarget: "iOS"})
	c := useFakeCLI(t, m, map[string]fakeResp{
		"projects remove " + old.Path + " --force": {Stdout: okJSON},
		"projects --all": reloadProjectsResponse(),
	})
	press(m, "d")
	if len(m.popups) != 1 || !strings.Contains(screenText(m), "Remove from the list") || !strings.Contains(screenText(m), "Delete the folder from disk…") {
		t.Fatalf("screen:\n%s", screenText(m))
	}
	_, cmd := m.Update(keyMsg("enter"))
	drain(m, cmd)
	if !called(c, "projects remove "+old.Path+" --force") {
		t.Errorf("calls: %v", calls(c))
	}
	if !m.st.PendingFor(old.Path).Empty() {
		t.Error("choices for a removed project are forgotten")
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
	c := useFakeCLI(t, m, map[string]fakeResp{
		"projects remove " + proj + " --force": {Stdout: okJSON},
		"projects --all":                       reloadProjectsResponse(),
	})

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
	if !called(c, "projects remove "+proj+" --force") {
		t.Errorf("the project also leaves the list: %v", calls(c))
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
	info := `{"success":true,"command":"projects info","data":{"title":"Space Shooter","path":"` + space.Path + `","version":"6000.6.8f1","changeset":"237b6593792b","buildTarget":"StandaloneLinux64","renderPipeline":"URP","cloudEnabled":true,"scriptingBackend":{"Android":"IL2CPP"},"packages":{"com.unity.inputsystem":"1.20.1","com.unity.modules.ui":"1.0.0","com.unity.ugui":"2.6.0"}}}`
	useFakeCLI(t, m, map[string]fakeResp{"projects info " + space.Path: {Stdout: info}})
	drain(m, m.keymap.lookup(ctxProjects, "i").run(m))
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
	c := useFakeCLI(t, m, map[string]fakeResp{"projects clean " + space.Path + " --yes": {Stdout: okJSON}})
	press(m, "C")
	if len(m.popups) != 1 || !strings.Contains(screenText(m), "Clean project") || !strings.Contains(screenText(m), "rebuild") {
		t.Fatalf("screen:\n%s", screenText(m))
	}
	if len(calls(c)) != 0 {
		t.Error("nothing runs before the user agrees")
	}
	_, cmd := m.Update(keyMsg("enter"))
	drain(m, cmd)
	if !called(c, "projects clean "+space.Path+" --yes") || !strings.Contains(screenText(m), "Cleaned Space Shooter") {
		t.Errorf("calls: %v", calls(c))
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
	bad := []string{"", ".", "..", "a/b", `a\b`, "a:b", "a*b", " lead", "trail ", "dots.", `q"`, "a|b", "a?b", "<x>"}
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
	c := useFakeCLI(t, m, map[string]fakeResp{
		"templates --installed --brief --editor-version 6000.6.8f1":                                                                           {File: "templates.json"},
		"projects create My Game --path " + parent + " --editor-version 6000.6.8f1 --template com.unity.template.urp-blank --no-cloud --open": {Stdout: okJSON},
		"projects --all": reloadProjectsResponse(),
	})

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
	out := screenText(m)
	if !strings.Contains(out, "New project (4/5)") || !strings.Contains(out, "3D URP") || !strings.Contains(out, "Universal 2D") {
		t.Fatalf("step 4:\n%s", out)
	}
	press(m, "j", "enter") // 3D URP
	if !strings.Contains(screenText(m), "New project (5/5)") || !strings.Contains(screenText(m), "[ ] Link to Unity Cloud") || !strings.Contains(screenText(m), "[x] Add Unity's recommended") {
		t.Fatalf("step 5:\n%s", screenText(m))
	}
	_, cmd = m.Update(keyMsg("enter")) // the defaults: no cloud, recommended packages and settings (no --bare)
	drain(m, cmd)
	if len(calls(c)) == 0 || !called(c, "projects create My Game --path "+parent+" --editor-version 6000.6.8f1 --template com.unity.template.urp-blank --no-cloud --open") {
		t.Errorf("calls: %v", calls(c))
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
	useFakeCLI(t, m, nil)
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
	useFakeCLI(t, m, nil)
	m.editors, m.local = nil, nil
	m.refreshEditors()
	cmd := m.keymap.lookup(ctxProjects, "n").run(m)
	_ = cmd
	if !strings.Contains(screenText(m), "Install a Unity Editor first") || m.panels[panelEditors].cur != 1 {
		t.Errorf("with no Editor the user is sent to the releases:\n%s", screenText(m))
	}
}

func TestCompleteDir(t *testing.T) {
	dir := t.TempDir()
	for _, d := range []string{"alpha", "alpine", "beta"} {
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
	}
	for _, c := range cases {
		got := completeDir(c.in)
		if len(got) != len(c.want) || (len(got) == 1 && got[0] != c.want[0]) {
			t.Errorf("completeDir(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestExpandHome(t *testing.T) {
	h := homeDir()
	if expandHome("~") != h || expandHome("~/x") != filepath.Join(h, "x") || expandHome("/a/~") != "/a/~" || expandHome("a") != "a" {
		t.Error("expandHome")
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

func TestOpenupmAddRunsInTheProject(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a shell script")
	}
	dir := t.TempDir()
	proj := filepath.Join(dir, "P")
	os.MkdirAll(proj, 0o755)
	exe := filepath.Join(dir, "openupm")
	os.WriteFile(exe, []byte("#!/bin/sh\necho \"$@\" > args.txt\n"), 0o755)
	if err := openupmAdd(context.Background(), exe, proj, []string{"a.b", "a.c"}); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(filepath.Join(proj, "args.txt"))
	if strings.TrimSpace(string(got)) != "add a.b a.c" {
		t.Errorf("args = %q", got)
	}
	os.WriteFile(exe, []byte("#!/bin/sh\necho boom >&2\nexit 3\n"), 0o755)
	if err := openupmAdd(context.Background(), exe, proj, []string{"a.b"}); err == nil || !strings.Contains(err.Error(), "boom") {
		t.Errorf("a failure carries the output: %v", err)
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
