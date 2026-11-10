package tui

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/ManeFunction/lazyunity/internal/config"
	"github.com/ManeFunction/lazyunity/internal/unitycli"
)

func init() {
	// Tests never look at the real PATH or open a real browser.
	lookPath = func(string) (string, error) { return "", exec.ErrNotFound }
	openInBrowser = func(goos, url string) error { return errors.New("no browser in tests") }
}

// stubBrowser records the pages opened during a test.
func stubBrowser(t *testing.T) *[]string {
	var opened []string
	prev := openInBrowser
	openInBrowser = func(goos, url string) error { opened = append(opened, url); return nil }
	t.Cleanup(func() { openInBrowser = prev })
	return &opened
}

// stubPath makes these programs exist, and nothing else.
func stubPath(t *testing.T, programs ...string) {
	prev := lookPath
	lookPath = func(name string) (string, error) {
		if slices.Contains(programs, name) {
			return "/usr/bin/" + name, nil
		}
		return "", exec.ErrNotFound
	}
	t.Cleanup(func() { lookPath = prev })
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

func TestDonateLinkOnTheBottomLine(t *testing.T) {
	m := newTestModel(t)
	opened := stubBrowser(t)
	last := screen(m)[len(screen(m))-1]
	if !strings.HasSuffix(strings.TrimRight(last, " "), donateLabel) {
		t.Fatalf("the bottom line ends with %q: %q", donateLabel, last)
	}
	m.Update(tea.MouseClickMsg{X: m.w - 2, Y: m.h - 1, Button: tea.MouseLeft})
	if len(*opened) != 1 || (*opened)[0] != sponsorURL {
		t.Errorf("a click on Donate opens %s: %v", sponsorURL, *opened)
	}
	m.Update(tea.MouseClickMsg{X: 2, Y: m.h - 1, Button: tea.MouseLeft})
	if len(*opened) != 1 {
		t.Errorf("a click elsewhere on the line opens nothing: %v", *opened)
	}
	// and from the keyboard, in the Tools tab
	selectTool(t, m, "donate")
	press(m, "enter")
	if len(*opened) != 2 || (*opened)[1] != sponsorURL {
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
	stubPath(t, "npm", "git")
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
	if err := os.WriteFile(filepath.Join(dir, "config.yml"), []byte(configTemplate), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(dir)
	if err != nil || !cfg.OpenUPM.FromRegistry() {
		t.Errorf("as written, the template installs from npm: %+v %v", cfg.OpenUPM, err)
	}
	if !strings.Contains(configTemplate, "#   repo: "+config.ForkOpenUPMRepo+"\n#   ref: "+config.ForkOpenUPMRef+"\n") ||
		!strings.Contains(configTemplate, config.OfficialOpenUPMRepo) || !strings.Contains(configTemplate, "pull/430") {
		t.Errorf("the template suggests the fork, says why, and names the official repository:\n%s", configTemplate)
	}
	// uncommenting it builds the fork's branch from git
	uncommented := strings.NewReplacer("# openupm:", "openupm:", "#   repo:", "  repo:", "#   ref:", "  ref:").Replace(configTemplate)
	os.WriteFile(filepath.Join(dir, "config.yml"), []byte(uncommented), 0o644)
	cfg, err = config.Load(dir)
	if err != nil || cfg.OpenUPM.FromRegistry() || cfg.OpenUPM.Repo != config.ForkOpenUPMRepo || cfg.OpenUPM.Ref != config.ForkOpenUPMRef {
		t.Errorf("uncommented: %+v %v", cfg.OpenUPM, err)
	}
}

func TestReleaseNotesOpenInTheBrowser(t *testing.T) {
	m := newTestModel(t)
	opened := stubBrowser(t)
	m.releasesTab.SetItems([]unitycli.Release{{Version: "6000.5.1f1", Architecture: "x86_64"}})
	press(m, "2", "]")
	press(m, "o")
	if len(*opened) != 1 || (*opened)[0] != "https://unity.com/releases/editor/whats-new/6000.5.1f1" {
		t.Errorf("o opens the release notes: %v", *opened)
	}
}
