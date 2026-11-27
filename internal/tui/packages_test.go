package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/ManeFunction/lazyunity/internal/config"
	"github.com/ManeFunction/lazyunity/internal/openupm"
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

func TestConfiguredPackagesComeAfterTheBuiltIns(t *testing.T) {
	in := installedPackages{
		direct:  map[string]string{"com.example.z": "1.0.0", "com.example.b": "1.0.0", "com.unity.ugui": "2.0.0"},
		openupm: map[string]bool{"com.example.z": true, "com.example.b": true},
	}
	got := offeredPackages(config.OpenUPM{Packages: []string{"com.example.a@v2.3.4", "com.example.b", "com.cysharp.unitask@2.5.0"}, ShowManePackages: true, ShowOftenUsed: true}, in)
	n := len(builtinPackages)
	if len(got) != n+3 || got[n].ref() != "com.example.a@2.3.4" || got[n+1].ref() != "com.example.b" || got[n+2].id != "com.example.z" {
		t.Fatalf("offered: %+v", got[n:])
	}
	if got[n].group != groupConfig || got[n+1].group != groupConfig || got[n+2].group != groupProject {
		t.Errorf("groups: %+v", got[n:])
	}
	for _, p := range got {
		if p.id == "com.cysharp.unitask" && p.ref() != "com.cysharp.unitask@2.5.0" {
			t.Errorf("a configured built-in pins its version: %s", p.ref())
		}
	}
}

// openupm.showManePackages and openupm.showOftenUsed hide their groups. A
// hidden package the project has is still listed, with the other installed.
func TestBuiltInGroupsCanBeHidden(t *testing.T) {
	in := installedPackages{
		direct:  map[string]string{pkgManeTools: "2.0.0"},
		openupm: map[string]bool{pkgManeTools: true},
	}
	groups := func(o config.OpenUPM) string {
		var out []string
		for _, p := range offeredPackages(o, in) {
			out = append(out, groupTitles[p.group]+":"+p.id)
		}
		return strings.Join(out, ",")
	}
	if got := groups(config.OpenUPM{ShowOftenUsed: true}); strings.Contains(got, "Mane Packages") || !strings.Contains(got, "Often used:") || !strings.HasSuffix(got, "Other installed:"+pkgManeTools) {
		t.Errorf("Mane Packages hidden: %s", got)
	}
	if got := groups(config.OpenUPM{ShowManePackages: true}); strings.Contains(got, "Often used") || !strings.HasPrefix(got, "Mane Packages:"+pkgManeTools) {
		t.Errorf("Often used hidden: %s", got)
	}
	if got := groups(config.OpenUPM{}); got != "Other installed:"+pkgManeTools {
		t.Errorf("both hidden: %s", got)
	}
	if d := config.Default().OpenUPM; !d.ShowManePackages || !d.ShowOftenUsed {
		t.Error("both groups show by default")
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
	press(m, "space")
	if !strings.Contains(screenText(m), "[↓] com.example.extra") {
		t.Errorf("a ticked package to install has the pending arrow:\n%s", screenText(m))
	}
	press(m, "enter")
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

// The popup lists its packages in four groups, each under a heading, the
// last one being the project's other OpenUPM packages, which can be removed.
func TestPackagesAreGrouped(t *testing.T) {
	m := newTestModel(t)
	r := selectProject(t, m, "Space Shooter")
	m.cfg.OpenUPM.Packages = []string{"com.example.extra"}
	os.MkdirAll(filepath.Join(r.Path, "Packages"), 0o755)
	os.WriteFile(filepath.Join(r.Path, "Packages", "manifest.json"), []byte(`{
		"dependencies": {"com.example.other": "3.0.0", "com.unity.ugui": "2.0.0", "jp.keijiro.klak": "1.0.0"},
		"scopedRegistries": [{"name": "package.openupm.com", "url": "https://package.openupm.com", "scopes": ["com.example", "jp.keijiro.klak"]}]
	}`), 0o644)
	m.openupm = openupmState{checked: true, path: "/usr/bin/openupm"}
	press(m, "P")
	p := topMenu(t, m)
	var order []string
	for _, it := range p.list.Items() {
		if it.heading {
			order = append(order, "# "+it.label)
		} else {
			order = append(order, it.id)
		}
	}
	want := "# Mane Packages," + pkgManeTools + "," + pkgManeToolsUI + ",com.manefunction.sound-maneger," +
		"# Often used,com.cysharp.unitask,com.coffee.ui-particle,com.coffee.softmask-for-ugui,com.coffee.ui-effect," +
		"# Your selection,com.example.extra," +
		"# Other installed,com.example.other,jp.keijiro.klak"
	if got := strings.Join(order, ","); got != want {
		t.Errorf("items:\n got %s\nwant %s", got, want)
	}
	// the headings are not counted
	if got := p.list.footer(); got != "1 of 10" {
		t.Errorf("footer: %q", got)
	}
	p.list.Bottom()
	if got := p.list.footer(); got != "10 of 10" {
		t.Errorf("footer at the bottom: %q", got)
	}
	p.list.Top()
	if _, first := p.list.Window(); first != 0 {
		t.Errorf("the first heading scrolled out of view: window starts at %d", first)
	}
	if out := screenText(m); !strings.Contains(out, "─── Other installed") || !strings.Contains(out, "[✓] com.example.other") || !strings.Contains(out, "Add: ") {
		t.Errorf("screen:\n%s", out)
	}
	var add, remove []string
	p = m.packagesPicker("P", mustInstalled(t, r.Path), func(_ *Model, a, rm []string) tea.Cmd { add, remove = a, rm; return nil })
	p.confirmApply = true
	m.push(p)
	p.list.SelectKey("jp.keijiro.klak")
	press(m, "space", "enter")
	c, ok := m.popups[len(m.popups)-1].(*confirmPopup)
	if !ok || c.title != "Apply changes" || c.danger || !strings.Contains(c.message, "- removing jp.keijiro.klak") {
		t.Fatalf("removing asks, as closing with changes does, in the usual colours:\n%s", screenText(m))
	}
	press(m, "esc") // back to the list
	if _, ok := m.popups[len(m.popups)-1].(*menuPopup); !ok || len(remove) != 0 {
		t.Fatal("back returns to the list without applying")
	}
	press(m, "enter", "enter")
	if len(add) != 0 || strings.Join(remove, ",") != "jp.keijiro.klak" {
		t.Errorf("add %v, remove %v", add, remove)
	}
}

func mustInstalled(t *testing.T, dir string) installedPackages {
	t.Helper()
	in, err := readInstalled(dir)
	if err != nil {
		t.Fatal(err)
	}
	return in
}

// A package is OpenUPM's when the lock file says it came from there, even
// without a scope for it.
func TestOpenUPMPackagesFromTheLockFile(t *testing.T) {
	dir := t.TempDir()
	writeManifest(t, dir, `"com.example.a": "1.0.0", "com.example.b": "1.0.0"`)
	writeLock(t, dir, `"com.example.a": {"version": "1.0.0", "source": "registry", "url": "https://package.openupm.com"},
		"com.example.b": {"version": "1.0.0", "source": "registry", "url": "https://packages.unity.com"}`)
	in := mustInstalled(t, dir)
	if !in.openupm["com.example.a"] || in.openupm["com.example.b"] {
		t.Errorf("openupm: %v", in.openupm)
	}
}

// lookupStep runs the batch an entry's enter gave and returns the lookup's
// answer, without acting on it.
func lookupStep(t *testing.T, cmd tea.Cmd) thenMsg {
	t.Helper()
	if cmd == nil {
		t.Fatal("no lookup")
	}
	msg := cmd()
	if then, ok := msg.(thenMsg); ok {
		return then
	}
	batch, ok := msg.(tea.BatchMsg)
	if !ok {
		t.Fatalf("not a batch: %T", msg)
	}
	for _, c := range batch {
		if c == nil {
			continue
		}
		if msg, ok := c().(thenMsg); ok {
			return msg
		}
	}
	t.Fatal("no lookup in the batch")
	return thenMsg{}
}

// A package typed in the field is looked up in the registry, with the popup
// and the app held open meanwhile. Found, it joins the last group, ticked,
// and is added once the popup is applied.
func TestTypedPackageIsLookedUpThenListed(t *testing.T) {
	m := newTestModel(t)
	h := recorder(m)
	h.known = map[string]openupm.Package{"com.example.found": {Name: "com.example.found", DisplayName: "Found", Latest: "1.2.0", Versions: map[string]bool{"1.0.0": true, "1.2.0": true}}}
	var add []string
	p := m.packagesPicker("P", installedPackages{}, func(_ *Model, a, _ []string) tea.Cmd { add = a; return nil })
	m.push(p)
	press(m, "end", "down")
	if !p.entry.focused {
		t.Fatal("down on the last item moves into the field")
	}
	for _, k := range "com.example.found@1.0.0" {
		press(m, string(k))
	}
	_, cmd := m.Update(keyMsg("enter"))
	if p.holding() == "" || !strings.Contains(screenText(m), "Looking up com.example.found") {
		t.Fatalf("the lookup shows:\n%s", screenText(m))
	}
	press(m, "esc", "q")
	m.Update(keyMsg("ctrl+c"))
	if len(m.popups) != 1 || m.quit {
		t.Fatal("while it looks up, the popup and the app stay open")
	}
	m.Update(lookupStep(t, cmd))
	if p.holding() != "" || p.entry.in.String() != "" {
		t.Error("done, the field is free and empty")
	}
	items := p.list.Items()
	last, heading := items[len(items)-1], items[len(items)-2]
	if last.id != "com.example.found" || !last.checked || last.label != "Found" || heading.label != "Other installed" {
		t.Fatalf("the package joins the last group, ticked: %+v %+v", heading, last)
	}
	if len(recorder(m).added) != 0 {
		t.Error("nothing is added before the popup is applied")
	}
	press(m, "enter")
	if strings.Join(add, ",") != "com.example.found@1.0.0" {
		t.Errorf("added: %v", add)
	}
}

func TestTypedPackageNotFound(t *testing.T) {
	m := newTestModel(t)
	recorder(m).known = map[string]openupm.Package{"com.example.v": {Name: "com.example.v", Latest: "1.0.0", Versions: map[string]bool{"1.0.0": true}}}
	p := m.packagesPicker("P", installedPackages{}, func(*Model, []string, []string) tea.Cmd { return nil })
	m.push(p)
	n := p.list.Len()
	press(m, "tab")
	for _, tc := range []struct{ text, says string }{
		{"com.example.gone", "com.example.gone is not in the OpenUPM registry"},
		{"com.example.v@9.0.0", "com.example.v has no version 9.0.0"},
	} {
		p.entry.in.Set(tc.text)
		_, cmd := m.Update(keyMsg("enter"))
		m.Update(lookupStep(t, cmd))
		if !strings.Contains(screenText(m), tc.says) || p.list.Len() != n || p.entry.in.String() != tc.text {
			t.Errorf("%s: nothing is listed, the text stays:\n%s", tc.text, screenText(m))
		}
	}
	p.entry.in.Set("not a name!")
	if _, cmd := m.Update(keyMsg("enter")); cmd != nil || !strings.Contains(screenText(m), "is not a package name") {
		t.Errorf("a bad name is refused without a lookup:\n%s", screenText(m))
	}
	p.entry.in.Set("com.cysharp.unitask")
	if _, cmd := m.Update(keyMsg("enter")); cmd != nil || ticked(p) != "com.cysharp.unitask" {
		t.Errorf("a package of the list is ticked, not looked up: %s", ticked(p))
	}
	p.entry.in.Set("")
	press(m, "enter")
	if len(m.popups) != 0 {
		t.Error("enter in an empty field applies the popup")
	}
}

// Closing the popup with unapplied changes asks whether to apply or discard
// them; without changes it just closes.
func TestPackagesAskBeforeDiscarding(t *testing.T) {
	m := newTestModel(t)
	r := selectProject(t, m, "Space Shooter")
	writeManifest(t, r.Path, `"com.manefunction.tools-unity": "2.0.0"`)
	m.openupm = openupmState{checked: true, path: "/usr/bin/openupm"}
	press(m, "P")
	topMenu(t, m)
	press(m, "esc")
	if len(m.popups) != 0 {
		t.Fatalf("without changes esc closes the popup:\n%s", screenText(m))
	}
	press(m, "P")
	topMenu(t, m).list.SelectKey("com.cysharp.unitask")
	press(m, "space", "esc")
	c, ok := m.popups[len(m.popups)-1].(*confirmPopup)
	if !ok || !strings.Contains(c.message, "- adding com.cysharp.unitask") {
		t.Fatalf("esc with changes asks:\n%s", screenText(m))
	}
	press(m, "esc") // back to the list, the tick kept
	if p := topMenu(t, m); len(p.picked()) == 0 {
		t.Fatal("back keeps the changes")
	}
	press(m, "esc", "d")
	if len(m.popups) != 0 || len(recorder(m).added) != 0 {
		t.Fatalf("discard closes without applying: %d popups, added %v", len(m.popups), recorder(m).added)
	}
	press(m, "P")
	topMenu(t, m).list.SelectKey("com.cysharp.unitask")
	press(m, "space", "enter")
	c, ok = m.popups[len(m.popups)-1].(*confirmPopup)
	if !ok || c.title != "Apply changes" || len(c.extra) != 0 {
		t.Fatalf("enter asks before adding too, with no discard:\n%s", screenText(m))
	}
	press(m, "esc", "q")
	_, cmd := m.Update(keyMsg("enter"))
	drain(m, cmd)
	if len(m.popups) != 0 || len(recorder(m).added) != 1 {
		t.Errorf("apply adds the package: %d popups, added %v", len(m.popups), recorder(m).added)
	}
}

// A command copied from the OpenUPM site is cut down to its package.
func TestTrimAddCommand(t *testing.T) {
	for in, want := range map[string]string{
		"openupm add com.example.package":            "com.example.package",
		"  openupm add com.example.package@1.2.0  ":  "com.example.package@1.2.0",
		"openupm-cli add com.example.package":        "com.example.package",
		"npx openupm-cli add -f com.example.package": "com.example.package",
		"com.example.package":                        "com.example.package",
		"openupm remove com.example.package":         "openupm remove com.example.package",
		"openupm add":                                "openupm add",
	} {
		if got := trimAddCommand(in); got != want {
			t.Errorf("trimAddCommand(%q) = %q, want %q", in, got, want)
		}
	}
}

// The command the OpenUPM site shows can be pasted as it is: the package in it
// is looked up.
func TestPastedAddCommandIsLookedUp(t *testing.T) {
	m := newTestModel(t)
	h := recorder(m)
	h.known = map[string]openupm.Package{"com.example.found": {Name: "com.example.found", DisplayName: "Found", Latest: "1.2.0", Versions: map[string]bool{"1.2.0": true}}}
	p := m.packagesPicker("P", installedPackages{}, func(*Model, []string, []string) tea.Cmd { return nil })
	m.push(p)
	m.Update(tea.PasteMsg{Content: "openupm add com.example.found"})
	_, cmd := m.Update(keyMsg("enter"))
	if !strings.Contains(screenText(m), "Looking up com.example.found ") {
		t.Fatalf("the package of the command is looked up:\n%s", screenText(m))
	}
	m.Update(lookupStep(t, cmd))
	if items := p.list.Items(); items[len(items)-1].id != "com.example.found" {
		t.Errorf("the package is listed: %+v", items[len(items)-1])
	}
}
