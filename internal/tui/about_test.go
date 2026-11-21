package tui

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/ManeFunction/lazyunity/internal/config"
	"github.com/ManeFunction/lazyunity/internal/state"
	"github.com/ManeFunction/lazyunity/internal/unitycli"
)

// stubPath makes these programs exist on the model's PATH, and nothing else.
func stubPath(m *Model, programs ...string) {
	m.env.LookPath = func(name string) (string, error) {
		if slices.Contains(programs, name) {
			return "/usr/bin/" + name, nil
		}
		return "", exec.ErrNotFound
	}
}

func selectTool(t *testing.T, m *Model, id string) {
	t.Helper()
	press(m, "3")
	if m.panels[panelOptions].cur != 0 {
		press(m, "]")
	}
	if !m.toolsTab.SelectKey(id) {
		t.Fatalf("no %s tool", id)
	}
}

func TestAppVersionEndsTheBottomLine(t *testing.T) {
	m := newTestModel(t)
	m.version = "1.2.3"
	last := strings.TrimRight(screen(m)[len(screen(m))-1], " ")
	if !strings.HasSuffix(last, " · 1.2.3") {
		t.Fatalf("the bottom line ends with the app's version: %q", last)
	}
	if strings.Contains(last, "Donate") {
		t.Errorf("no Donate link on the bottom line: %q", last)
	}
	if !strings.Contains(m.barView(m.w), m.th.Text.Render("1.2.3")) {
		t.Errorf("the app's version is drawn in the text colour, not dimmed")
	}
}

func TestDonateTool(t *testing.T) {
	m := newTestModel(t)
	opened := &recorder(m).urls
	selectTool(t, m, "donate")
	press(m, "enter")
	if len(*opened) != 1 || (*opened)[0] != sponsorURL {
		t.Errorf("the Donate tool opens the page: %v", *opened)
	}
}

func TestAboutNamesTheAuthorAndTheRepository(t *testing.T) {
	m := newTestModel(t)
	selectTool(t, m, "about")
	press(m, "enter")
	out := screenText(m)
	for _, want := range []string{"Ilia Petrov-Komotskii (Mane Function)", repoURL, sponsorURL} {
		if !strings.Contains(out, want) {
			t.Errorf("About should show %q:\n%s", want, out)
		}
	}
}

func TestOpenUPMToolRow(t *testing.T) {
	m := newTestModel(t)
	status := func() string { return m.openupmRow().status }
	if r := status(); r != "checking…" {
		t.Errorf("before the probe: %q", r)
	}
	m.Update(openupmMsg{checked: true})
	if r := status(); r != "not installed · press enter to install" {
		t.Errorf("not installed: %q", r)
	}

	// the npm release reads like the Unity CLI's row
	official := m.cfg.OpenUPM
	if cmd := m.onOpenUPM(openupmMsg{checked: true, path: "/usr/bin/openupm", version: "4.5.3"}); cmd == nil {
		t.Error("once installed, updates are looked for")
	}
	if r := status(); r != "ready · 4.5.3" {
		t.Errorf("installed: %q", r)
	}
	m.Update(openupmLatestMsg{openupmLatest: openupmLatest{src: official, latest: "4.5.3"}})
	if r := status(); r != "ready · 4.5.3" {
		t.Errorf("up to date: %q", r)
	}
	m.Update(openupmLatestMsg{openupmLatest: openupmLatest{src: official, latest: "4.6.0"}})
	if r := m.openupmRow(); r.status != "update available: 4.6.0 · enter to update" || r.tone != toneWarn {
		t.Errorf("an update: %q", r.status)
	}

	// a build from a repository names its branch and commit
	fork := config.OpenUPM{Repo: "https://github.com/ManeFunction/openupm-cli.git", Ref: "fixes"}
	m.cfg.OpenUPM = fork
	if r := status(); r != "4.5.3 · not built from ManeFunction/openupm-cli@fixes · enter to install it" {
		t.Errorf("a fork that lazyunity did not build: %q", r)
	}
	m.st.OpenUPMBuild = &state.OpenUPMBuild{Repo: "https://github.com/ManeFunction/openupm-cli", Ref: "fixes", Commit: "0123456789abcdef", Version: "4.5.3"}
	if r := m.openupmRow(); r.status != "4.5.3 · from ManeFunction/openupm-cli@fixes #0123456" || r.tone != toneGood {
		t.Errorf("a fork: %q", r.status)
	}
	m.Update(openupmLatestMsg{openupmLatest: openupmLatest{src: fork, latest: "0123456789abcdef"}})
	if r := status(); r != "4.5.3 · from ManeFunction/openupm-cli@fixes #0123456" {
		t.Errorf("the branch has no new commits: %q", r)
	}
	m.Update(openupmLatestMsg{openupmLatest: openupmLatest{src: fork, latest: "fedcba9876543210"}})
	if r := m.openupmRow(); r.status != "update available: #fedcba9 · from ManeFunction/openupm-cli@fixes #0123456" || r.tone != toneWarn {
		t.Errorf("a new commit on the branch, of the same version: %q", r.status)
	}

	m.cfg.OpenUPM = config.OpenUPM{Repo: fork.Repo, Ref: "main"}
	if r := status(); r != "4.5.3 · from ManeFunction/openupm-cli@fixes #0123456 · enter to install ManeFunction/openupm-cli@main" {
		t.Errorf("another branch is configured: %q", r)
	}
	m.cfg.OpenUPM = official
	if r := status(); r != "4.5.3 · from ManeFunction/openupm-cli@fixes #0123456 · enter to install the npm release" {
		t.Errorf("the npm release is configured: %q", r)
	}
	m.onOpenUPM(openupmMsg{checked: true, path: "/usr/bin/openupm", version: "4.6.0"})
	if r := status(); r != "ready · 4.6.0" {
		t.Errorf("another version replaced the build: %q", r)
	}
	m.onOpenUPM(openupmMsg{checked: true})
	if m.st.OpenUPMBuild != nil {
		t.Error("once the CLI is gone, so is the build")
	}
}

// TestOpenUPMStatus pins every kind of the OpenUPM status to the probe, the
// remembered build, the update check and the configured source.
func TestOpenUPMStatus(t *testing.T) {
	registry := config.OpenUPM{}
	fork := config.OpenUPM{Repo: "https://github.com/ManeFunction/openupm-cli.git", Ref: "fixes"}
	other := config.OpenUPM{Repo: "https://github.com/ManeFunction/openupm-cli.git", Ref: "main"}
	installed := openupmState{checked: true, path: "/usr/bin/openupm", version: "4.5.3"}
	build := &state.OpenUPMBuild{Repo: "https://github.com/ManeFunction/openupm-cli", Ref: "fixes", Commit: "0123456789abcdef", Version: "4.5.3"}
	stale := &state.OpenUPMBuild{Repo: build.Repo, Ref: build.Ref, Commit: build.Commit, Version: "4.4.0"}

	for _, c := range []struct {
		name    string
		src     config.OpenUPM
		probe   openupmState
		build   *state.OpenUPMBuild
		latest  openupmLatest
		kind    openupmKind
		version string
		update  string
		built   bool // the status names the build
	}{
		{name: "before the probe", src: registry, kind: openupmChecking},
		{name: "not installed", src: registry, probe: openupmState{checked: true}, kind: openupmMissing},
		{name: "no version reported", src: registry, probe: openupmState{checked: true, path: "/usr/bin/openupm"}, kind: openupmReady, version: "installed"},

		{name: "npm, not checked yet", src: registry, probe: installed, kind: openupmReady, version: "4.5.3"},
		{name: "npm, current", src: registry, probe: installed, latest: openupmLatest{src: registry, latest: "4.5.3"}, kind: openupmReady, version: "4.5.3"},
		{name: "npm, newer out", src: registry, probe: installed, latest: openupmLatest{src: registry, latest: "4.6.0"}, kind: openupmUpdate, version: "4.5.3", update: "4.6.0"},
		{name: "npm, check for another source", src: registry, probe: installed, latest: openupmLatest{src: fork, latest: "fedcba9876543210"}, kind: openupmReady, version: "4.5.3"},
		{name: "npm wanted, repository build installed", src: registry, probe: installed, build: build, kind: openupmOther, version: "4.5.3", built: true},

		{name: "repository wanted, npm installed", src: fork, probe: installed, kind: openupmOther, version: "4.5.3"},
		{name: "repository, another branch installed", src: other, probe: installed, build: build, kind: openupmOther, version: "4.5.3", built: true},
		{name: "repository, build replaced since", src: fork, probe: installed, build: stale, kind: openupmOther, version: "4.5.3"},
		{name: "repository, not checked yet", src: fork, probe: installed, build: build, kind: openupmReady, version: "4.5.3", built: true},
		{name: "repository, current", src: fork, probe: installed, build: build, latest: openupmLatest{src: fork, latest: build.Commit}, kind: openupmReady, version: "4.5.3", built: true},
		{name: "repository, newer commit", src: fork, probe: installed, build: build, latest: openupmLatest{src: fork, latest: "fedcba9876543210"}, kind: openupmUpdate, version: "4.5.3", update: "#fedcba9", built: true},
		{name: "repository, check for another branch", src: fork, probe: installed, build: build, latest: openupmLatest{src: other, latest: "fedcba9876543210"}, kind: openupmReady, version: "4.5.3", built: true},
	} {
		t.Run(c.name, func(t *testing.T) {
			m := newTestModel(t)
			m.cfg.OpenUPM, m.openupm, m.st.OpenUPMBuild, m.openupmNew = c.src, c.probe, c.build, c.latest
			st := m.openupmStatus()
			if st.kind != c.kind || st.version != c.version || st.update != c.update || (st.build != nil) != c.built {
				t.Errorf("got kind %d, version %q, update %q, build %v; want kind %d, version %q, update %q, build %v",
					st.kind, st.version, st.update, st.build != nil, c.kind, c.version, c.update, c.built)
			}
		})
	}
}

func TestOpenUPMInstallRecordsTheCommit(t *testing.T) {
	m := newTestModel(t)
	fork := config.OpenUPM{Repo: " https://github.com/me/openupm-cli ", Ref: "fixes"}
	stamp := filepath.Join(t.TempDir(), "stamp")
	os.WriteFile(stamp, []byte("0123456789abcdef\n"), 0o644)
	m.onOpenUPMInstalled(openupmInstalledMsg{src: fork, stamp: stamp})
	if b := m.st.OpenUPMBuild; b == nil || *b != (state.OpenUPMBuild{Repo: "https://github.com/me/openupm-cli", Ref: "fixes", Commit: "0123456789abcdef"}) {
		t.Errorf("the build: %+v", b)
	}
	if _, err := os.Stat(stamp); err == nil {
		t.Error("the stamp is removed")
	}
	m.onOpenUPM(openupmMsg{checked: true, path: "/usr/bin/openupm", version: "4.5.3"})
	if b := state.Load(filepath.Dir(m.st.Path())).OpenUPMBuild; b == nil || b.Version != "4.5.3" {
		t.Errorf("the probe fills in the version, and it is saved: %+v", b)
	}
	m.onOpenUPMInstalled(openupmInstalledMsg{src: config.OpenUPM{}})
	if m.st.OpenUPMBuild != nil {
		t.Error("the npm release is not a build")
	}
}

func TestOpenUPMInstallNeedsNode(t *testing.T) {
	m := newTestModel(t)
	m.Update(openupmMsg{checked: true})
	selectTool(t, m, "openupm")
	press(m, "enter")
	if out := screenText(m); !strings.Contains(out, "needs npm") || !strings.Contains(out, "nodejs.org") {
		t.Errorf("without npm the user is told what to install:\n%s", out)
	}
	press(m, "esc")
	m.cfg.OpenUPM = config.OpenUPM{Repo: "https://github.com/me/openupm-cli"}
	press(m, "enter")
	if out := screenText(m); !strings.Contains(out, "needs npm and git") {
		t.Errorf("a fork also needs git:\n%s", out)
	}
}

func TestOpenUPMInstallAsks(t *testing.T) {
	m := newTestModel(t)
	stubPath(m, "npm", "git")
	m.Update(openupmMsg{checked: true})
	selectTool(t, m, "openupm")
	press(m, "enter")
	if out := screenText(m); !strings.Contains(out, "npm install -g openupm-cli") || !strings.Contains(out, "openupm.repo") {
		t.Errorf("the official release comes from npm:\n%s", out)
	}
	press(m, "esc")
	m.cfg.OpenUPM = config.OpenUPM{Repo: "https://github.com/me/openupm-cli", Ref: "fixes"}
	press(m, "enter")
	if out := screenText(m); !strings.Contains(out, "from https://github.com/me/openupm-cli") || !strings.Contains(out, "(fixes)") || !strings.Contains(out, "built with npm") {
		t.Errorf("a fork is built from source:\n%s", out)
	}
	// installing hands the terminal over; afterwards the tool looks again
	cmd := m.onOpenUPMInstalled(openupmInstalledMsg{})
	if cmd == nil || m.openupm.checked || !strings.Contains(m.toast.text, "Installed") {
		t.Errorf("after the install: checked=%v toast=%q", m.openupm.checked, m.toast.text)
	}
}

func TestConfigTemplateOffersTheForkCommentedOut(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "config.yml"), []byte(configTemplate()), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(dir)
	if err != nil || !cfg.OpenUPM.FromRegistry() {
		t.Errorf("as written, the template installs from npm: %+v %v", cfg.OpenUPM, err)
	}
	if !strings.Contains(configTemplate(), "#   repo: "+config.ForkOpenUPMRepo+"\n#   ref: "+config.ForkOpenUPMRef+"\n") ||
		!strings.Contains(configTemplate(), config.OfficialOpenUPMRepo) || !strings.Contains(configTemplate(), "pull/430") {
		t.Errorf("the template suggests the fork, says why, and names the official repository:\n%s", configTemplate())
	}
	// uncommenting it builds the fork's branch from git
	uncommented := strings.NewReplacer("# openupm:", "openupm:", "#   repo:", "  repo:", "#   ref:", "  ref:").Replace(configTemplate())
	os.WriteFile(filepath.Join(dir, "config.yml"), []byte(uncommented), 0o644)
	cfg, err = config.Load(dir)
	if err != nil || cfg.OpenUPM.FromRegistry() || cfg.OpenUPM.Repo != config.ForkOpenUPMRepo || cfg.OpenUPM.Ref != config.ForkOpenUPMRef {
		t.Errorf("uncommented: %+v %v", cfg.OpenUPM, err)
	}
}

func TestReleaseNotesOpenInTheBrowser(t *testing.T) {
	m := newTestModel(t)
	opened := &recorder(m).urls
	m.releasesTab.SetItems([]unitycli.Release{{Version: "6000.5.1f1", Architecture: "x86_64"}})
	press(m, "2", "]")
	press(m, "o")
	if len(*opened) != 1 || (*opened)[0] != "https://unity.com/releases/editor/whats-new/6000.5.1f1" {
		t.Errorf("o opens the release notes: %v", *opened)
	}
}

func TestConfigTemplateListsEverySettingAtItsDefault(t *testing.T) {
	// uncomment every setting line, leaving the explanations and the openupm
	// block (which has no default to restore) commented
	var out []string
	setting := regexp.MustCompile(`^# ( *[a-z][A-Za-z]*:.*)$`)
	for _, l := range strings.Split(configTemplate(), "\n") {
		if m := setting.FindStringSubmatch(l); m != nil && !strings.HasPrefix(m[1], "openupm:") && !strings.HasPrefix(m[1], "  repo:") && !strings.HasPrefix(m[1], "  ref:") {
			l = m[1]
		}
		out = append(out, l)
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "config.yml"), []byte(strings.Join(out, "\n")), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(dir)
	if err != nil {
		t.Fatalf("uncommented, the template does not load: %v\n%s", err, strings.Join(out, "\n"))
	}
	km, warnings := newKeymap(buildActions(), cfg.Keybinding)
	if len(warnings) > 0 {
		t.Errorf("the template's keybindings warn: %q", warnings)
	}
	def, _ := newKeymap(buildActions(), nil)
	for _, a := range buildActions() {
		if cfg.Keybinding[a.ctx.group()][a.name()] == nil {
			t.Errorf("the template misses %s", a.id)
		}
	}
	for i := range km.bound {
		if len(km.bound[i]) != len(def.bound[i]) {
			t.Errorf("context %d: %d keys bound, %d by default", i, len(km.bound[i]), len(def.bound[i]))
		}
		for k, a := range def.bound[i] {
			if got := km.bound[i][k]; got == nil || got.id != a.id {
				t.Errorf("context %d: key %q is not bound to %s", i, k, a.id)
			}
		}
	}
	cfg.Keybinding = nil
	if !reflect.DeepEqual(cfg, config.Default()) {
		t.Errorf("the template's values are not the defaults:\n%+v\n%+v", cfg, config.Default())
	}
}

// The install asks with the verb that matches the row: install, update to the
// newer one, or reinstall.
func TestOpenUPMInstallVerb(t *testing.T) {
	m := newTestModel(t)
	m.env.LookPath = func(name string) (string, error) { return "/usr/bin/" + name, nil }
	title := func() string {
		m.popups = nil
		m.openupmTool()
		if len(m.popups) != 1 {
			t.Fatalf("no confirmation:\n%s", screenText(m))
		}
		return m.popups[0].(*confirmPopup).title
	}
	m.Update(openupmMsg{checked: true})
	if got := title(); got != "Install the OpenUPM CLI" {
		t.Errorf("not installed: %q", got)
	}
	m.onOpenUPM(openupmMsg{checked: true, path: "/usr/bin/openupm", version: "4.5.3"})
	if got := title(); got != "Reinstall the OpenUPM CLI" {
		t.Errorf("installed: %q", got)
	}
	m.Update(openupmLatestMsg{openupmLatest: openupmLatest{src: m.cfg.OpenUPM, latest: "4.6.0"}})
	if got := title(); got != "Update the OpenUPM CLI" {
		t.Errorf("an update: %q", got)
	}
	if msg := m.popups[0].(*confirmPopup).message; !strings.Contains(msg, "openupm-cli to 4.6.0,") {
		t.Errorf("the update names the version: %q", msg)
	}
}
