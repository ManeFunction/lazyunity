package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/ManeFunction/lazyunity/internal/unity"
	"github.com/ManeFunction/lazyunity/internal/unitycli"
)

// ticked are the ids of the ticked items of a menu.
func ticked(p *menuPopup) string {
	var out []string
	for _, it := range p.picked() {
		out = append(out, it.id)
	}
	return strings.Join(out, ",")
}

func topMenu(t *testing.T, m *Model) *menuPopup {
	t.Helper()
	if len(m.popups) == 0 {
		t.Fatalf("no popup:\n%s", screenText(m))
	}
	p, ok := m.popups[len(m.popups)-1].(*menuPopup)
	if !ok {
		t.Fatalf("the top popup is a %T:\n%s", m.popups[len(m.popups)-1], screenText(m))
	}
	return p
}

// Ticking a ManeTools module ticks ManeTools; unticking ManeTools unticks
// what needs it.
func TestPackagesFollowTheirDependencies(t *testing.T) {
	m := newTestModel(t)
	p := m.packagesPicker("P", installedPackages{}, nil)
	m.push(p)
	p.list.SelectKey(pkgManeToolsUI)
	press(m, "space")
	if got := ticked(p); got != pkgManeTools+","+pkgManeToolsUI {
		t.Errorf("ticking the UI module: %s", got)
	}
	p.list.SelectKey("com.manefunction.sound-maneger")
	press(m, "space")
	p.list.SelectKey(pkgManeTools)
	press(m, "space")
	if got := ticked(p); got != "" {
		t.Errorf("unticking ManeTools unticks the rest: %s", got)
	}
	p.list.SelectKey("com.cysharp.unitask")
	press(m, "space")
	if got := ticked(p); got != "com.cysharp.unitask" {
		t.Errorf("a package without dependencies stands alone: %s", got)
	}
}

func TestConfiguredPackagesComeLast(t *testing.T) {
	got := offeredPackages([]string{"com.example.a@v2.3.4", "com.example.b", "com.cysharp.unitask@2.5.0"})
	n := len(recommendedPackages)
	if len(got) != n+2 || got[n].ref() != "com.example.a@2.3.4" || got[n+1].ref() != "com.example.b" {
		t.Fatalf("offered: %+v", got[n:])
	}
	for _, p := range got {
		if p.id == "com.cysharp.unitask" && p.ref() != "com.cysharp.unitask@2.5.0" {
			t.Errorf("a configured built-in pins its version: %s", p.ref())
		}
	}
}

func writeManifest(t *testing.T, dir string, deps string) {
	t.Helper()
	os.MkdirAll(filepath.Join(dir, "Packages"), 0o755)
	if err := os.WriteFile(filepath.Join(dir, "Packages", "manifest.json"), []byte(`{"dependencies": {`+deps+`}}`), 0o644); err != nil {
		t.Fatal(err)
	}
}

// P shows what the project has with a check, adds what is ticked and,
// after asking, removes what is unticked.
func TestProjectPackages(t *testing.T) {
	m := newTestModel(t)
	r := selectProject(t, m, "Space Shooter")
	m.cfg.OpenUPM.Packages = []string{"com.example.extra@1.0.0"}
	writeManifest(t, r.Path, `"com.manefunction.tools-unity": "2.0.0", "com.manefunction.tools-unity-ui": "2.0.0"`)
	if m.keymap.lookup(ctxProjects, "P").enabled(m) {
		t.Error("without OpenUPM, P is inactive")
	}
	m.openupm = openupmState{checked: true, path: "/usr/bin/openupm"}
	press(m, "P")
	p := topMenu(t, m)
	out := screenText(m)
	if !strings.Contains(out, "[✓] ManeTools for UI") || !strings.Contains(out, "[ ] UniTask") || !strings.Contains(out, "com.example.extra") {
		t.Fatalf("installed packages are checked:\n%s", out)
	}
	p.list.SelectKey(pkgManeToolsUI)
	press(m, "space")
	if !strings.Contains(screenText(m), "[✗] ManeTools for UI") {
		t.Errorf("an unticked installed package is crossed out:\n%s", screenText(m))
	}
	p.list.SelectKey("com.example.extra")
	press(m, "space", "enter")
	c, ok := m.popups[len(m.popups)-1].(*confirmPopup)
	if !ok || !strings.Contains(c.message, pkgManeToolsUI) || !strings.Contains(c.message, "com.example.extra@1.0.0") {
		t.Fatalf("removing asks first:\n%s", screenText(m))
	}
	_, cmd := m.Update(keyMsg("enter"))
	drain(m, cmd)
	h := recorder(m)
	if len(h.removed) != 1 || strings.Join(h.removed[0], ",") != pkgManeToolsUI {
		t.Errorf("removed: %v", h.removed)
	}
	if len(h.added) != 1 || strings.Join(h.added[0], ",") != "com.example.extra@1.0.0" {
		t.Errorf("added: %v", h.added)
	}
}

func writeLock(t *testing.T, dir string, deps string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "Packages", "packages-lock.json"), []byte(`{"dependencies": {`+deps+`}}`), 0o644); err != nil {
		t.Fatal(err)
	}
}

// A package the project has only as a dependency is ticked and locked while
// a ticked package needs it. Once nothing does, it unlocks; left ticked, it
// is added to the manifest at its version, so it stays.
func TestPackagesInstalledAsDependencies(t *testing.T) {
	m := newTestModel(t)
	r := selectProject(t, m, "Space Shooter")
	writeManifest(t, r.Path, `"com.manefunction.tools-unity-ui": "2.0.0", "com.manefunction.sound-maneger": "1.0.0"`)
	writeLock(t, r.Path, `
		"com.manefunction.tools-unity": {"version": "2.1.0", "depth": 1, "source": "registry", "dependencies": {}},
		"com.manefunction.tools-unity-ui": {"version": "2.0.0", "depth": 0, "source": "registry", "dependencies": {"com.manefunction.tools-unity": "2.1.0"}},
		"com.manefunction.sound-maneger": {"version": "1.0.0", "depth": 0, "source": "registry", "dependencies": {"com.manefunction.tools-unity": "2.1.0"}}`)
	m.openupm = openupmState{checked: true, path: "/usr/bin/openupm"}
	press(m, "P")
	p := topMenu(t, m)
	if out := screenText(m); !strings.Contains(out, "[✓] ManeTools ") || !strings.Contains(out, "2.1.0 (dependency)") {
		t.Fatalf("a dependency shows as installed:\n%s", out)
	}
	p.list.SelectKey(pkgManeTools)
	press(m, "space")
	if got := ticked(p); !strings.Contains(got, pkgManeTools+",") {
		t.Errorf("a needed dependency cannot be unticked: %s", got)
	}
	if out := screenText(m); !strings.Contains(out, "needed by ManeTools for UI, Sound Maneger") {
		t.Errorf("it says why:\n%s", out)
	}
	p.list.SelectKey(pkgManeToolsUI)
	press(m, "space")
	if it, _ := p.list.Selected(); it.id != pkgManeToolsUI {
		t.Fatal("selection moved")
	}
	for _, it := range p.list.Items() {
		if it.id == pkgManeTools && it.locked == "" {
			t.Error("Sound Maneger still needs ManeTools")
		}
	}
	p.list.SelectKey("com.manefunction.sound-maneger")
	press(m, "space")
	for _, it := range p.list.Items() {
		if it.id == pkgManeTools && (it.locked != "" || !it.checked) {
			t.Errorf("with nothing needing it, ManeTools unlocks, ticked: %+v", it)
		}
	}
	press(m, "enter")
	if _, ok := m.popups[len(m.popups)-1].(*confirmPopup); !ok {
		t.Fatalf("removing asks first:\n%s", screenText(m))
	}
	_, cmd := m.Update(keyMsg("enter"))
	drain(m, cmd)
	h := recorder(m)
	if len(h.removed) != 1 || strings.Join(h.removed[0], ",") != pkgManeToolsUI+",com.manefunction.sound-maneger" {
		t.Errorf("removed: %v", h.removed)
	}
	if len(h.added) != 1 || strings.Join(h.added[0], ",") != pkgManeTools+"@2.1.0" {
		t.Errorf("ManeTools is kept at its version: %v", h.added)
	}
}

// Unticked once unlocked, a dependency goes with what needed it.
func TestUntickedDependencyGoesAway(t *testing.T) {
	m := newTestModel(t)
	in := installedPackages{
		direct: map[string]string{pkgManeToolsUI: "2.0.0"},
		lock: map[string]unity.LockedPackage{
			pkgManeTools:   {Version: "2.1.0", Depth: 1, Source: "registry"},
			pkgManeToolsUI: {Version: "2.0.0", Source: "registry", Dependencies: map[string]string{pkgManeTools: "2.1.0"}},
		},
	}
	var add, remove []string
	p := m.packagesPicker("P", in, func(_ *Model, a, r []string) tea.Cmd { add, remove = a, r; return nil })
	m.push(p)
	p.list.SelectKey(pkgManeToolsUI)
	press(m, "space")
	p.list.SelectKey(pkgManeTools)
	press(m, "space", "enter")
	if len(add) != 0 || strings.Join(remove, ",") != pkgManeToolsUI {
		t.Errorf("add %v, remove %v", add, remove)
	}
}

// A package of the manifest the popup does not offer keeps its dependencies
// locked.
func TestOtherPackagesKeepDependenciesLocked(t *testing.T) {
	m := newTestModel(t)
	in := installedPackages{
		direct: map[string]string{"com.example.other": "1.0.0"},
		lock: map[string]unity.LockedPackage{
			"com.cysharp.unitask": {Version: "2.5.0", Depth: 1, Source: "registry"},
			"com.example.other":   {Version: "1.0.0", Dependencies: map[string]string{"com.cysharp.unitask": "2.5.0"}},
		},
	}
	p := m.packagesPicker("P", in, nil)
	m.push(p)
	p.list.SelectKey("com.cysharp.unitask")
	press(m, "space")
	if got := ticked(p); got != "com.cysharp.unitask" {
		t.Errorf("ticked: %s", got)
	}
}

func TestPackagePageOpensGitHub(t *testing.T) {
	m := newTestModel(t)
	m.cfg.OpenUPM.Packages = []string{"com.example.known", "com.example.unknown"}
	p := m.packagesPicker("P", installedPackages{}, nil)
	m.push(p)
	h := recorder(m)
	h.repos = map[string]string{"com.example.known": "https://github.com/example/known"}
	p.list.SelectKey("com.cysharp.unitask")
	press(m, "o")
	for _, id := range []string{"com.example.known", "com.example.unknown"} {
		p.list.SelectKey(id)
		_, cmd := m.Update(keyMsg("o"))
		drain(m, cmd)
	}
	want := "https://github.com/Cysharp/UniTask,https://github.com/example/known,https://openupm.com/packages/com.example.unknown/"
	if got := strings.Join(h.urls, ","); got != want {
		t.Errorf("urls:\n got %s\nwant %s", got, want)
	}
	if len(m.popups) != 1 {
		t.Error("o leaves the popup open")
	}
}

// With the OpenUPM CLI, the wizard has a sixth step that adds the ticked
// packages after creating the project.
func TestNewProjectAddsPackages(t *testing.T) {
	m := newTestModel(t)
	f := useFakeCLI(t, m)
	m.openupm = openupmState{checked: true, path: "/usr/bin/openupm"}
	w := newProject{name: "G", parent: t.TempDir(), version: "6000.6.8f1"}
	m.newProjectOptions(w, "3d")
	if out := screenText(m); !strings.Contains(out, "New project (5/6)") || strings.Contains(out, "ManeTools") {
		t.Fatalf("step 5 has no packages:\n%s", out)
	}
	press(m, "enter")
	p := topMenu(t, m)
	if !strings.Contains(screenText(m), "New project (6/6): OpenUPM packages") {
		t.Fatalf("step 6:\n%s", screenText(m))
	}
	p.list.SelectKey("com.manefunction.sound-maneger")
	press(m, "space")
	_, cmd := m.Update(keyMsg("enter"))
	drain(m, cmd)
	if !f.called("CreateProject", unitycli.CreateOptions{Name: "G", ParentDir: w.parent, EditorVersion: w.version, Template: "3d", Cloud: unitycli.CloudOff}) {
		t.Errorf("calls: %v", f.Calls())
	}
	if got := recorder(m).added; len(got) != 1 || strings.Join(got[0], ",") != pkgManeTools+",com.manefunction.sound-maneger" {
		t.Errorf("added: %v", got)
	}
}

func TestNewProjectWithoutOpenUPMHasFiveSteps(t *testing.T) {
	m := newTestModel(t)
	m.newProjectOptions(newProject{name: "G", parent: t.TempDir(), version: "6000.6.8f1"}, "3d")
	if out := screenText(m); !strings.Contains(out, "New project (5/5)") || strings.Contains(out, "ManeTools") {
		t.Errorf("screen:\n%s", out)
	}
}
