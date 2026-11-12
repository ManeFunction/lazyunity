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
	if r := m.openupmRow(); r.status != "checking…" {
		t.Errorf("before the probe: %q", r.status)
	}
	m.Update(openupmMsg{checked: true})
	if r := m.openupmRow(); !strings.Contains(r.status, "not installed") {
		t.Errorf("not installed: %q", r.status)
	}
	m.Update(openupmMsg{checked: true, path: "/usr/bin/openupm", version: "4.5.3"})
	if r := m.openupmRow(); !strings.HasPrefix(r.status, "4.5.3") || strings.Contains(r.status, "from") {
		t.Errorf("installed: %q", r.status)
	}
	m.cfg.OpenUPM = config.OpenUPM{Repo: "https://github.com/ManeFunction/openupm-cli.git", Ref: "fixes"}
	if r := m.openupmRow(); !strings.Contains(r.status, "from ManeFunction/openupm-cli@fixes") {
		t.Errorf("a fork is named: %q", r.status)
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
