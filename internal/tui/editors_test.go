package tui

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ManeFunction/lazyunity/internal/unity"
	"github.com/ManeFunction/lazyunity/internal/unitycli"
)

const streamOK = `{"type":"result","success":true,"command":"install","data":{}}`

func selectEditor(t *testing.T, m *Model, version string) editorRow {
	t.Helper()
	for _, e := range m.editorsTab.Items() {
		if e.Version == version {
			m.editorsTab.SelectKey(e.Location)
			return e
		}
	}
	t.Fatalf("no editor %s", version)
	return editorRow{}
}

func TestDrillIntoModulesAndBack(t *testing.T) {
	m := newTestModel(t)
	press(m, "2")
	selectEditor(t, m, "6000.6.8f1")
	useFakeCLI(t, m, map[string]fakeResp{"modules 6000.6.8f1 --architecture x86_64": {File: "modules.json"}})
	_, cmd := m.Update(keyMsg("enter"))
	if !strings.Contains(screenText(m), "[2]─Editors > 6000.6.8f1") || !strings.Contains(screenText(m), "Loading modules…") {
		t.Fatalf("drilling in shows the Editor in the title while loading:\n%s", screenText(m))
	}
	drain(m, cmd)
	out := screenText(m)
	for _, want := range []string{"Android Build Support", "Platforms", "Web Build Support", "[ ]", "Documentation"} {
		if !strings.Contains(out, want) {
			t.Errorf("modules should show %q:\n%s", want, out)
		}
	}
	tab, _, ok := m.modulesTab()
	if !ok {
		t.Fatal("no modules tab")
	}
	if first, _ := tab.Selected(); first.Category != "Platforms" {
		t.Errorf("platforms come first, got %q", first.Category)
	}
	if m.focusCtx() != ctxModules {
		t.Errorf("keys now act on modules: %v", m.focusCtx())
	}
	// switching tabs is off while drilled in; esc goes back
	press(m, "]")
	if m.panels[panelEditors].drill == nil {
		t.Error("] must not leave the modules")
	}
	press(m, "esc")
	if m.panels[panelEditors].drill != nil || !strings.Contains(screenText(m), "[2]─Installed Editors - Releases") {
		t.Error("esc should go back to the Editors")
	}
}

func TestModulesListFailureIsShown(t *testing.T) {
	m := newTestModel(t)
	press(m, "2")
	selectEditor(t, m, "6000.6.8f1")
	useFakeCLI(t, m, map[string]fakeResp{"modules 6000.6.8f1 --architecture x86_64": {Stderr: "no network", Exit: 7}})
	_, cmd := m.Update(keyMsg("enter"))
	drain(m, cmd)
	if !strings.Contains(screenText(m), "Could not list the modules") {
		t.Errorf("screen:\n%s", screenText(m))
	}
}

func TestInstallAModuleFromTheModulesList(t *testing.T) {
	m := newTestModel(t)
	press(m, "2")
	selectEditor(t, m, "6000.6.8f1")
	c := useFakeCLI(t, m, map[string]fakeResp{
		"modules 6000.6.8f1 --architecture x86_64": {File: "modules.json"},
		"install-modules --editor-version 6000.6.8f1 --module android --architecture x86_64 --child-modules --accept-eula --yes --format ndjson": {
			Lines: []string{`{"type":"progress","pct":40,"msg":"Downloading Android Build Support...","phase":"download"}`, streamOK}},
	})
	_, cmd := m.Update(keyMsg("enter"))
	drain(m, cmd)
	tab, _, _ := m.modulesTab()
	tab.SelectKey("android")
	if sel, _ := tab.Selected(); sel.ID != "android" {
		t.Fatalf("selected %q", sel.ID)
	}
	cmd = m.keymap.lookup(ctxModules, "space").run(m)
	if len(m.tasks) != 1 || !strings.Contains(m.tasks[0].title, "Adding Android Build Support to Unity 6000.6.8f1") {
		t.Fatalf("a task should start: %v", m.tasks)
	}
	drain(m, cmd)
	if !called(c, "install-modules --editor-version 6000.6.8f1 --module android --architecture x86_64 --child-modules --accept-eula --yes --format ndjson") {
		t.Errorf("calls: %v", calls(c))
	}
	// the open modules list is refreshed afterwards
	n := 0
	for _, got := range calls(c) {
		if got == "modules 6000.6.8f1 --architecture x86_64" {
			n++
		}
	}
	if n < 2 {
		t.Errorf("the module list should be reloaded after the install: %v", calls(c))
	}
}

func TestInstallingAnInstalledModuleIsRefused(t *testing.T) {
	m := newTestModel(t)
	press(m, "2")
	selectEditor(t, m, "6000.6.8f1")
	useFakeCLI(t, m, map[string]fakeResp{"modules 6000.6.8f1 --architecture x86_64": {File: "modules.json"}})
	_, cmd := m.Update(keyMsg("enter"))
	drain(m, cmd)
	// the fixture has nothing installed: mark one as installed
	tab, _, _ := m.modulesTab()
	items := append(tab.Items()[:0:0], tab.Items()...)
	items[0].IsInstalled = true
	tab.SetItems(items)
	tab.Top()
	m.keymap.lookup(ctxModules, "space").run(m)
	if len(m.tasks) != 0 || !strings.Contains(screenText(m), "is already installed") {
		t.Errorf("screen:\n%s", screenText(m))
	}
	// and removing a module that is not installed
	tab.Bottom()
	m.keymap.lookup(ctxModules, "d").run(m)
	if len(m.popups) != 0 || !strings.Contains(screenText(m), "is not installed") {
		t.Errorf("screen:\n%s", screenText(m))
	}
}

func TestRemoveAModule(t *testing.T) {
	m := newTestModel(t)
	press(m, "2")
	selectEditor(t, m, "6000.6.8f1")
	c := useFakeCLI(t, m, map[string]fakeResp{
		"modules 6000.6.8f1 --architecture x86_64":                                    {File: "modules.json"},
		"editors module remove 6000.6.8f1 --module webgl --architecture x86_64 --yes": {Stdout: okJSON},
	})
	_, cmd := m.Update(keyMsg("enter"))
	drain(m, cmd)
	tab, _, _ := m.modulesTab()
	items := append(tab.Items()[:0:0], tab.Items()...)
	for i := range items {
		if items[i].ID == "webgl" {
			items[i].IsInstalled = true
		}
	}
	tab.SetItems(items)
	tab.SelectKey("webgl")
	m.keymap.lookup(ctxModules, "d").run(m)
	if len(m.popups) != 1 || !strings.Contains(screenText(m), "Remove Web Build Support") {
		t.Fatalf("a confirmation first:\n%s", screenText(m))
	}
	_, cmd = m.Update(keyMsg("enter"))
	drain(m, cmd)
	if !called(c, "editors module remove 6000.6.8f1 --module webgl --architecture x86_64 --yes") {
		t.Errorf("calls: %v", calls(c))
	}
}

func TestAddModulesFromTheEditorList(t *testing.T) {
	m := newTestModel(t)
	press(m, "2")
	selectEditor(t, m, "6000.6.8f1")
	c := useFakeCLI(t, m, map[string]fakeResp{
		"modules 6000.6.8f1 --architecture x86_64": {File: "modules.json"},
		"install-modules --editor-version 6000.6.8f1 --module android --module ios --architecture x86_64 --child-modules --accept-eula --yes --format ndjson": {Lines: []string{streamOK}},
	})
	drain(m, m.keymap.lookup(ctxEditors, "a").run(m))
	menu, ok := m.popups[len(m.popups)-1].(*menuPopup)
	if !ok || !menu.multi {
		t.Fatalf("a multi-select menu of modules should open:\n%s", screenText(m))
	}
	if !strings.Contains(screenText(m), "Add modules to Unity 6000.6.8f1") || !strings.Contains(screenText(m), "Android Build Support") {
		t.Errorf("screen:\n%s", screenText(m))
	}
	// tick Android and iOS, in the order they appear
	for _, it := range menu.list.Items() {
		if it.label == "Android Build Support" || it.label == "iOS Build Support" {
			menu.list.SelectKey(it.id)
			press(m, "space")
		}
	}
	_, cmd := m.Update(keyMsg("enter"))
	drain(m, cmd)
	if !called(c, "install-modules --editor-version 6000.6.8f1 --module android --module ios --architecture x86_64 --child-modules --accept-eula --yes --format ndjson") {
		t.Errorf("calls: %v", calls(c))
	}
}

func TestAddPlatformFromTheProjectPlatformMenu(t *testing.T) {
	m := newTestModel(t)
	selectProject(t, m, "Space Shooter")
	useFakeCLI(t, m, map[string]fakeResp{"modules 6000.6.8f1 --architecture x86_64": {File: "modules.json"}})
	press(m, "p")
	menu := m.popups[0].(*menuPopup)
	menu.list.SelectKey("Add a platform module…")
	_, cmd := m.Update(keyMsg("enter"))
	drain(m, cmd)
	out := screenText(m)
	if !strings.Contains(out, "Android Build Support") || strings.Contains(out, "Documentation") {
		t.Errorf("only platforms are offered from here:\n%s", out)
	}
}

func TestNothingLeftToAdd(t *testing.T) {
	m := newTestModel(t)
	e := m.editors[0]
	m.onAddModules(addModulesMsg{ed: e, mods: nil})
	if len(m.popups) != 0 || !strings.Contains(screenText(m), "already installed") {
		t.Errorf("screen:\n%s", screenText(m))
	}
}

func TestInstallANewEditorEndToEnd(t *testing.T) {
	m := newTestModel(t)
	c := useFakeCLI(t, m, map[string]fakeResp{
		"editors info 6000.5.1f1":                     {File: "editors-info.json"},
		"install 6000.5.1f1 --module webgl --dry-run": {File: "install-dryrun.ndjson"},
		"install 6000.5.1f1 --module webgl --child-modules --accept-eula --yes --format ndjson": {
			Lines: []string{`{"type":"progress","pct":10,"msg":"Downloading Unity...","phase":"download"}`, streamOK}},
		"editors --installed": editorsResponse(m),
		"projects --all":      reloadProjectsResponse(),
	})
	drain(m, m.installEditorFlow("6000.5.1f1", ""))
	menu, ok := m.popups[len(m.popups)-1].(*menuPopup)
	if !ok || !menu.multi || !strings.Contains(screenText(m), "Install Unity 6000.5.1f1: choose modules") {
		t.Fatalf("a module picker:\n%s", screenText(m))
	}
	if out := screenText(m); !strings.Contains(out, "Toggle: <space> | Confirm: <enter> | Cancel: <esc>") || strings.Contains(out, "│Platforms") {
		t.Errorf("the box under the picker shows its keys, not the category:\n%s", out)
	}
	menu.list.SelectKey("webgl")
	press(m, "space")
	_, cmd := m.Update(keyMsg("enter"))
	drain(m, cmd) // asks the CLI for the plan
	out := screenText(m)
	for _, want := range []string{"Install Unity 6000.5.1f1 with Web Build Support", "Download: 6.0 GB", "license terms"} {
		if !strings.Contains(out, want) {
			t.Errorf("the confirmation should contain %q:\n%s", want, out)
		}
	}
	if len(m.tasks) != 0 {
		t.Error("nothing is installed before the user confirms")
	}
	_, cmd = m.Update(keyMsg("enter")) // Install
	if len(m.tasks) != 1 || !strings.Contains(m.tasks[0].title, "Installing Unity 6000.5.1f1 + Web Build Support") {
		t.Fatalf("the install task: %v", m.tasks)
	}
	drain(m, cmd)
	if !called(c, "install 6000.5.1f1 --module webgl --child-modules --accept-eula --yes --format ndjson") {
		t.Errorf("calls: %v", calls(c))
	}
	if !called(c, "editors --installed") {
		t.Error("the Editor list is refreshed when the install ends")
	}
	if !strings.Contains(screenText(m), "Installed Unity 6000.5.1f1") {
		t.Errorf("screen:\n%s", screenText(m))
	}
}

func TestInstallEditorAlreadyInstalled(t *testing.T) {
	m := newTestModel(t)
	useFakeCLI(t, m, nil)
	if cmd := m.installEditorFlow("6000.6.8f1", ""); cmd != nil || !strings.Contains(screenText(m), "already installed") {
		t.Errorf("screen:\n%s", screenText(m))
	}
	// the CLI can also say so when asked for a plan
	m.toast = toast{}
	m.onPlan(planMsg{opts: unitycli.InstallOptions{Version: "6000.5.1f1"}, plan: unitycli.InstallPlan{AlreadyInstalled: true}})
	if len(m.popups) != 0 || !strings.Contains(screenText(m), "Unity 6000.5.1f1 is already installed") {
		t.Errorf("screen:\n%s", screenText(m))
	}
}

func TestInstallFailsWithoutAnEditorList(t *testing.T) {
	m := newTestModel(t)
	useFakeCLI(t, m, map[string]fakeResp{"editors info 9999.1.1f1": {
		Stdout: `{"success":false,"errors":[{"code":"NOT_FOUND","message":"No such release"}]}`, Exit: 11}})
	drain(m, m.installEditorFlow("9999.1.1f1", ""))
	if len(m.popups) != 1 || !strings.Contains(screenText(m), "No such release") {
		t.Errorf("screen:\n%s", screenText(m))
	}
}

func TestUninstallListsTheProjectsThatUseIt(t *testing.T) {
	m := newTestModel(t)
	press(m, "2")
	selectEditor(t, m, "2022.3.62f1") // Old Platformer uses it
	c := useFakeCLI(t, m, map[string]fakeResp{
		"uninstall 2022.3.62f1 --architecture x86_64 --yes": {Stdout: okJSON},
		"editors --installed":                               editorsResponse(m),
		"projects --all":                                    reloadProjectsResponse(),
	})
	press(m, "d")
	out := screenText(m)
	if len(m.popups) != 1 || !strings.Contains(out, "Uninstall Unity 2022.3.62f1") || !strings.Contains(out, "1 project use it: Old Platformer") {
		t.Fatalf("the confirmation names the projects:\n%s", out)
	}
	if len(calls(c)) != 0 {
		t.Error("nothing runs before the user agrees")
	}
	_, cmd := m.Update(keyMsg("enter"))
	drain(m, cmd)
	if !called(c, "uninstall 2022.3.62f1 --architecture x86_64 --yes") || !called(c, "editors --installed") {
		t.Errorf("calls: %v", calls(c))
	}
}

func TestUninstallDeclined(t *testing.T) {
	m := newTestModel(t)
	press(m, "2")
	selectEditor(t, m, "6000.6.8f1")
	c := useFakeCLI(t, m, nil)
	press(m, "d", "esc")
	if len(calls(c)) != 0 || len(m.popups) != 0 {
		t.Error("declining does nothing")
	}
}

func TestSetDefaultEditor(t *testing.T) {
	m := newTestModel(t)
	press(m, "2")
	selectEditor(t, m, "2022.3.62f1")
	c := useFakeCLI(t, m, map[string]fakeResp{
		"editors default 2022.3.62f1": {Stdout: okJSON},
		"editors --installed":         editorsResponse(m),
	})
	drain(m, m.keymap.lookup(ctxEditors, "space").run(m))
	if !called(c, "editors default 2022.3.62f1") || !strings.Contains(screenText(m), "Unity 2022.3.62f1 is now the default") {
		t.Errorf("calls: %v", calls(c))
	}
}

func TestUpgradeEditor(t *testing.T) {
	m := newTestModel(t)
	press(m, "2")
	selectEditor(t, m, "2022.3.62f1")
	c := useFakeCLI(t, m, map[string]fakeResp{
		"editors upgrade 2022.3.62f1 --yes --accept-eula --format ndjson": {Lines: []string{streamOK}},
		"editors --installed": editorsResponse(m),
		"projects --all":      reloadProjectsResponse(),
	})
	press(m, "u")
	if len(m.popups) != 1 || !strings.Contains(screenText(m), "newest patch of Unity 2022.3") {
		t.Fatalf("screen:\n%s", screenText(m))
	}
	_, cmd := m.Update(keyMsg("enter"))
	drain(m, cmd)
	if !called(c, "editors upgrade 2022.3.62f1 --yes --accept-eula --format ndjson") {
		t.Errorf("calls: %v", calls(c))
	}
}

func TestEditorCopyMenu(t *testing.T) {
	m := newTestModel(t)
	press(m, "2", "y")
	out := screenText(m)
	for _, want := range []string{"Version", "Editor path", "Folder", "6000.6.8f1"} {
		if !strings.Contains(out, want) {
			t.Errorf("should contain %q", want)
		}
	}
}

func TestReleasesLoadFilterAndPage(t *testing.T) {
	m := newTestModel(t)
	c := useFakeCLI(t, m, map[string]fakeResp{
		"releases --limit 25 --offset 0 --stream tech": {File: "releases.json"}, // 3 items, next offset 3
		"releases --limit 25 --offset 3 --stream tech": {Stdout: `{"success":true,"data":{"items":[{"version":"2021.3.1f1","architecture":"x86_64","stream":"LTS","lts":true,"installed":false}],"nextPage":null}}`},
		"releases --limit 25 --offset 0 --stream lts":  {Stdout: `{"success":true,"data":{"items":[{"version":"6000.0.85f2","architecture":"x86_64","stream":"LTS","lts":true,"installed":false}],"nextPage":null}}`},
	})
	press(m, "2")
	cmd := m.switchTab(1)
	if !strings.Contains(screenText(m), "Loading releases…") {
		t.Error("the tab says it is loading")
	}
	drain(m, cmd)
	if m.releasesTab.Len() != 3 || m.releases.nextOffset != 3 {
		t.Fatalf("%d releases, next offset %d", m.releasesTab.Len(), m.releases.nextOffset)
	}
	if strings.Contains(screenText(m), "Loading releases…") {
		t.Error("loaded")
	}
	// moving to the end loads the next page
	drain(m, m.keymap.lookup(ctxReleases, "j").run(m))
	drain(m, m.keymap.lookup(ctxReleases, "j").run(m))
	if m.releasesTab.Len() != 4 || m.releases.nextOffset != 0 {
		t.Errorf("the next page should be appended: %d releases, next offset %d (calls %v)", m.releasesTab.Len(), m.releases.nextOffset, calls(c))
	}
	// the stream filter starts the list over
	press(m, "ctrl+s")
	if !strings.Contains(screenText(m), "Release stream") {
		t.Fatal("stream menu")
	}
	_, cmd = m.Update(keyMsg("j")) // LTS is the second item
	_, cmd = m.Update(keyMsg("enter"))
	drain(m, cmd)
	if m.releasesTab.Len() != 1 || m.releases.stream != "lts" || m.st.ReleaseStream != "lts" {
		t.Errorf("filtered: %d releases, stream %q, remembered %q (calls %v)", m.releasesTab.Len(), m.releases.stream, m.st.ReleaseStream, calls(c))
	}
	if item, _ := m.releasesTab.Selected(); item.Version != "6000.0.85f2" {
		t.Errorf("the list was replaced, got %q", item.Version)
	}
}

func TestReleasesStartOnSupported(t *testing.T) {
	m := newTestModel(t)
	if m.releases.stream != "tech" {
		t.Errorf("a fresh install lists Supported releases, stream = %q", m.releases.stream)
	}
	press(m, "2", "]", "ctrl+s")
	out := screenText(m)
	if !strings.Contains(out, "Supported") || strings.Contains(out, "Tech") {
		t.Errorf("the stream is called Supported:\n%s", out)
	}
	// "All" is remembered as such, not as the default
	m.st.SetStream("")
	if m.st.ReleaseStream != "all" || m.st.Stream() != "" {
		t.Errorf("all: %q %q", m.st.ReleaseStream, m.st.Stream())
	}
}

func TestReleasesFilterRowSearchesEveryPage(t *testing.T) {
	m := newTestModel(t)
	page := func(items string, next string) fakeResp {
		return fakeResp{Stdout: `{"success":true,"data":{"items":[` + items + `],"nextPage":` + next + `}}`}
	}
	rel := func(v string) string {
		return `{"version":"` + v + `","architecture":"x86_64","stream":"SUPPORTED","lts":false,"installed":false}`
	}
	c := useFakeCLI(t, m, map[string]fakeResp{
		"releases --limit 25 --offset 0 --stream tech":  page(rel("6000.6.8f1")+","+rel("6000.6.7f1"), "2"),
		"releases --limit 200 --offset 2 --stream tech": page(rel("6000.5.3f1"), "3"),
		"releases --limit 200 --offset 3 --stream tech": page(rel("6000.4.1f1"), "null"),
	})
	press(m, "2")
	drain(m, m.switchTab(1))
	out := screenText(m)
	if !strings.Contains(out, "Filter:") || !strings.Contains(out, "press / to type a version") {
		t.Fatalf("the Releases tab has a filter row:\n%s", out)
	}
	press(m, "/")
	if strings.Contains(screen(m)[len(screen(m))-1], "Filter:") {
		t.Error("the filter is typed in the tab, not on the bottom line")
	}
	for _, k := range []string{"6", "0", "0", "0", ".", "4"} {
		_, cmd := m.Update(keyMsg(k))
		drain(m, cmd)
	}
	if m.releasesTab.Len() != 1 || m.releases.nextOffset != 0 {
		t.Fatalf("every page is searched: %d matches, next offset %d (calls %v)", m.releasesTab.Len(), m.releases.nextOffset, calls(c))
	}
	if r, _ := m.releasesTab.Selected(); r.Version != "6000.4.1f1" {
		t.Errorf("match = %q", r.Version)
	}
	press(m, "enter")
	if m.filter.active || m.releasesTab.filter() != "6000.4" || !strings.Contains(screenText(m), "6000.4") {
		t.Errorf("enter keeps the filter:\n%s", screenText(m))
	}
}

func TestReleasesShowWhenMoreIsLoading(t *testing.T) {
	m := newTestModel(t)
	press(m, "2", "]")
	m.releases.loading = true
	if !strings.Contains(screenText(m), "loading more") {
		t.Errorf("a line under the list says more is loading:\n%s", screenText(m))
	}
	m.releases.loading = false
	if strings.Contains(screenText(m), "loading more") || !strings.Contains(screenText(m), "Supported") {
		t.Errorf("then names the stream:\n%s", screenText(m))
	}
}

func TestReleasesInstallAlreadyInstalled(t *testing.T) {
	m := newTestModel(t)
	m.onReleases(releasesMsg{stream: m.releases.stream, page: unitycli.ReleasePage{Items: []unitycli.Release{
		{Version: "6000.6.8f1", Architecture: "x86_64", Stream: "SUPPORTED", Installed: true},
	}}})
	press(m, "2", "]")
	m.keymap.lookup(ctxReleases, "enter").run(m)
	if !strings.Contains(screenText(m), "already installed") || len(m.popups) != 0 {
		t.Errorf("screen:\n%s", screenText(m))
	}
	if !strings.Contains(screenText(m), "✓ installed") {
		t.Error("installed releases are marked in the list")
	}
}

func TestStreamLabels(t *testing.T) {
	m := newTestModel(t)
	cases := map[string]string{"LTS": "LTS", "BETA": "Beta", "ALPHA": "Alpha", "TECH": "Supported", "SUPPORTED": "Supported", "weird": "Weird", "": ""}
	for in, want := range cases {
		if got, _ := m.th.streamLabel(unitycli.Release{Stream: in}); got != want {
			t.Errorf("streamLabel(%q) = %q, want %q", in, got, want)
		}
	}
	if got, _ := m.th.streamLabel(unitycli.Release{Stream: "X", LTS: true}); got != "LTS" {
		t.Errorf("a release flagged LTS is labelled LTS: %q", got)
	}
}

func TestCompleteDirKeepsTilde(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	if err := os.Mkdir(filepath.Join(home, "Projects"), 0o755); err != nil {
		t.Fatal(err)
	}
	got := completeDir("~/Pro")
	if len(got) != 1 || got[0] != "~/Projects/" {
		t.Errorf("completeDir = %q, want ~/Projects/", got)
	}
}

func TestCleanDownloadCache(t *testing.T) {
	info := `{"success":true,"command":"cache info","data":{"path":"/home/u/.config/unityhub/downloads","sizeBytes":1509051525,"size":"1.4 GB","fileCount":3}}`
	empty := `{"success":true,"command":"cache info","data":{"path":"/home/u/.config/unityhub/downloads","sizeBytes":0,"fileCount":0}}`

	m := newTestModel(t)
	press(m, "2")
	useFakeCLI(t, m, map[string]fakeResp{"cache info": {Stdout: info}, "cache clean --yes": {Stdout: okJSON}})
	_, cmd := m.Update(keyMsg("C"))
	drain(m, cmd)
	out := screenText(m)
	for _, want := range []string{"Clean download cache", "3 files", "1.5 GB", "/home/u/.config/unityhub/downloads"} {
		if !strings.Contains(out, want) {
			t.Errorf("the confirmation should show %q:\n%s", want, out)
		}
	}
	if _, ok := m.popups[len(m.popups)-1].(*confirmPopup); !ok {
		t.Fatalf("C asks first")
	}
	_, cmd = m.Update(keyMsg("enter"))
	drain(m, cmd)
	if len(m.popups) != 0 || !strings.Contains(screenText(m), "Freed 1.5 GB") {
		t.Errorf("confirming cleans the cache:\n%s", screenText(m))
	}

	// the Releases tab has the key too; an empty cache needs no popup
	m = newTestModel(t)
	press(m, "2")
	press(m, "2")
	if m.focusCtx() != ctxReleases {
		t.Fatalf("context = %v, want releases", m.focusCtx())
	}
	useFakeCLI(t, m, map[string]fakeResp{"cache info": {Stdout: empty}})
	_, cmd = m.Update(keyMsg("C"))
	drain(m, cmd)
	if len(m.popups) != 0 || !strings.Contains(screenText(m), "The download cache is empty") {
		t.Errorf("an empty cache is only reported:\n%s", screenText(m))
	}

	// not while an install is downloading into it
	m = newTestModel(t)
	press(m, "2")
	useFakeCLI(t, m, map[string]fakeResp{})
	m.enqueue(&task{title: "Installing", do: func(ctx context.Context) error { <-ctx.Done(); return ctx.Err() }})
	if cmd := m.cleanCacheFlow(); cmd != nil || !strings.Contains(screenText(m), "Wait for the tasks to finish") {
		t.Errorf("cleaning while busy is refused:\n%s", screenText(m))
	}
	m.cancelTasks()

	// the Projects panel keeps C for cleaning a project
	if a := m.keymap.lookup(ctxProjects, "C"); a == nil || a.id != "projects.clean" {
		t.Errorf("C in Projects = %v", a)
	}
	for _, c := range []ctxID{ctxEditors, ctxModules, ctxReleases} {
		if a := m.keymap.lookup(c, "C"); a == nil || a.name() != "cleanCache" {
			t.Errorf("C in %s = %v", c.group(), a)
		}
	}
}

func TestMergeEditorsTakesTheVersionFromTheCLI(t *testing.T) {
	local := []unity.Installed{{Version: "unity-2022.3", Location: "/opt/unity-2022.3/Editor/Unity", Dir: "/opt/unity-2022.3"}}
	cli := []unitycli.Editor{{Version: "2022.3.62f1", Location: "/opt/unity-2022.3/Editor/Unity", Architecture: "x86_64", Default: true}}
	got := mergeEditors(local, cli, "linux")
	if len(got) != 1 {
		t.Fatalf("one Editor, listed once: %+v", got)
	}
	if e := got[0]; e.Version != "2022.3.62f1" || !e.Default || e.Arch != "x86_64" {
		t.Errorf("the CLI knows the real version of an Editor in a folder not named after it: %+v", e)
	}
}
