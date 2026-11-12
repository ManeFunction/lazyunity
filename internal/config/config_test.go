package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func writeConfig(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "config.yml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestMissingFileGivesDefaults(t *testing.T) {
	cfg, err := Load(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(cfg, Default()) {
		t.Error("a missing config.yml must give the defaults")
	}
	d := Default()
	if d.GUI.SidePanelWidth != 0.45 || d.GUI.Border != "rounded" || !d.GUI.MouseEvents ||
		!d.GUI.ShowPanelJumps || !d.GUI.ShowListFooter || !d.GUI.ShowBottomLine || d.GUI.ScreenMode != "full" {
		t.Errorf("defaults: %+v", d.GUI)
	}
	if got := d.GUI.Theme.ActiveBorderColor; !reflect.DeepEqual(got, []string{"brightblue", "bold"}) {
		t.Errorf("the default theme must be blue, got %v", got)
	}
}

func TestPartialFileKeepsOtherDefaults(t *testing.T) {
	dir := writeConfig(t, `
gui:
  border: double
  sidePanelWidth: 0.5
  theme:
    activeBorderColor: ['#5c9dff', bold]
os:
  edit: code {{dir}}
unity:
  cliPath: /opt/unity
projects:
  defaultLocation: /games
quitAfterOpen: true
`)
	cfg, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.GUI.Border != "double" || cfg.GUI.SidePanelWidth != 0.5 {
		t.Errorf("gui: %+v", cfg.GUI)
	}
	if !reflect.DeepEqual(cfg.GUI.Theme.ActiveBorderColor, []string{"#5c9dff", "bold"}) {
		t.Errorf("theme: %v", cfg.GUI.Theme.ActiveBorderColor)
	}
	// untouched keys keep their defaults
	if !cfg.GUI.MouseEvents || cfg.GUI.ScreenMode != "full" ||
		!reflect.DeepEqual(cfg.GUI.Theme.OptionsTextColor, Default().GUI.Theme.OptionsTextColor) {
		t.Errorf("defaults were lost: %+v", cfg.GUI)
	}
	if cfg.OS.Edit != "code {{dir}}" || cfg.Unity.CLIPath != "/opt/unity" || !cfg.QuitAfterOpen ||
		cfg.Projects.DefaultLocation != "/games" {
		t.Errorf("config: %+v", cfg)
	}
}

func TestKeybindingsAcceptStringOrList(t *testing.T) {
	dir := writeConfig(t, `
keybinding:
  universal:
    quit: q
    openMenu: ['?', '<f1>']
  projects:
    open: <enter>
`)
	cfg, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.Keybinding["universal"]["quit"]; !reflect.DeepEqual([]string(got), []string{"q"}) {
		t.Errorf("quit = %v", got)
	}
	if got := cfg.Keybinding["universal"]["openMenu"]; !reflect.DeepEqual([]string(got), []string{"?", "<f1>"}) {
		t.Errorf("openMenu = %v", got)
	}
	if got := cfg.Keybinding["projects"]["open"]; !reflect.DeepEqual([]string(got), []string{"<enter>"}) {
		t.Errorf("open = %v", got)
	}
}

func TestInvalidConfigIsRejected(t *testing.T) {
	cases := map[string]string{
		"bad border":       "gui:\n  border: fancy\n",
		"bad screen mode":  "gui:\n  screenMode: huge\n",
		"width too small":  "gui:\n  sidePanelWidth: 0.05\n",
		"width too big":    "gui:\n  sidePanelWidth: 0.95\n",
		"not yaml":         "gui: [unclosed\n",
		"wrong type":       "gui:\n  mouseEvents: sometimes\n",
		"key is a mapping": "keybinding:\n  universal:\n    quit: {a: b}\n",
		"empty key":        "keybinding:\n  universal:\n    quit: ['']\n",
	}
	for name, body := range cases {
		cfg, err := Load(writeConfig(t, body))
		if err == nil {
			t.Errorf("%s: expected an error", name)
			continue
		}
		if !strings.Contains(err.Error(), "config.yml") {
			t.Errorf("%s: the error should name the file: %v", name, err)
		}
		if !reflect.DeepEqual(cfg, Default()) {
			t.Errorf("%s: a rejected config must fall back to the defaults", name)
		}
	}
}

func TestDir(t *testing.T) {
	t.Setenv("LAZYUNITY_CONFIG_DIR", "")
	if got, _ := Dir("/explicit"); got != "/explicit" {
		t.Errorf("override: %q", got)
	}
	t.Setenv("LAZYUNITY_CONFIG_DIR", "/from/env")
	if got, _ := Dir(""); got != "/from/env" {
		t.Errorf("env: %q", got)
	}
	if got, _ := Dir("/explicit"); got != "/explicit" {
		t.Errorf("the flag beats the environment: %q", got)
	}
	t.Setenv("LAZYUNITY_CONFIG_DIR", "")
	t.Setenv("XDG_CONFIG_HOME", "/xdg")
	t.Setenv("HOME", "/home/x")
	if got, err := Dir(""); err != nil || filepath.Base(got) != "lazyunity" {
		t.Errorf("default: %q, %v", got, err)
	}
}

func TestNormalizeKey(t *testing.T) {
	cases := map[string]string{
		"<c-s>":      "ctrl+s",
		"<C-S>":      "ctrl+s",
		"<a-x>":      "alt+x",
		"<space>":    "space",
		"<enter>":    "enter",
		"<esc>":      "esc",
		"<backtab>":  "shift+tab",
		"<s-tab>":    "shift+tab",
		"<pgup>":     "pgup",
		"<pagedown>": "pgdown",
		"<disabled>": "",
		"?":          "?",
		"q":          "q",
		"ctrl+c":     "ctrl+c",
		"<":          "<",
		">":          ">",
		"<>":         "<>",
		"<f1>":       "<f1>",
	}
	for in, want := range cases {
		if got := NormalizeKey(in); got != want {
			t.Errorf("NormalizeKey(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestExpandHome(t *testing.T) {
	t.Setenv("HOME", "/home/u")
	t.Setenv("USERPROFILE", "/home/u")
	for in, want := range map[string]string{
		"~": "/home/u", "~/Projects": "/home/u/Projects", "/abs/~": "/abs/~", "rel": "rel", "": "", "~user": "~user",
	} {
		if got := ExpandHome(in); filepath.ToSlash(got) != want {
			t.Errorf("ExpandHome(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestOpenUPMFromRegistry(t *testing.T) {
	for _, c := range []struct {
		o    OpenUPM
		want bool
	}{
		{Default().OpenUPM, true},
		{OpenUPM{Repo: "  "}, true},
		{OpenUPM{Repo: OfficialOpenUPMRepo}, false}, // set by the user: built from git
		{OpenUPM{Repo: OfficialOpenUPMRepo, Ref: "master"}, false},
		{OpenUPM{Repo: "https://github.com/ManeFunction/openupm-cli"}, false},
	} {
		if got := c.o.FromRegistry(); got != c.want {
			t.Errorf("%+v: FromRegistry = %v", c.o, got)
		}
	}
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "config.yml"), []byte("openupm:\n  repo: https://github.com/me/openupm-cli\n  ref: fixes\n"), 0o644)
	cfg, err := Load(dir)
	if err != nil || cfg.OpenUPM.Repo != "https://github.com/me/openupm-cli" || cfg.OpenUPM.Ref != "fixes" {
		t.Errorf("Load: %+v %v", cfg.OpenUPM, err)
	}
	os.WriteFile(filepath.Join(dir, "config.yml"), []byte("openupm:\n  repo: --upload-pack=evil\n"), 0o644)
	if _, err := Load(dir); err == nil {
		t.Error("a repo that looks like an option is refused")
	}
	os.WriteFile(filepath.Join(dir, "config.yml"), []byte("openupm:\n  ref: fixes\n"), 0o644)
	if _, err := Load(dir); err == nil {
		t.Error("a ref without a repo is refused")
	}
}

func TestRetiredSettingsWarn(t *testing.T) {
	cfg, err := Load(writeConfig(t, "editors:\n  extraRoots: [/opt/unity]\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Warnings) != 1 || !strings.Contains(cfg.Warnings[0], "editors.extraRoots") {
		t.Errorf("warnings: %v", cfg.Warnings)
	}
	if cfg, _ := Load(writeConfig(t, "quitAfterOpen: true\n")); len(cfg.Warnings) != 0 {
		t.Errorf("a current config must not warn: %v", cfg.Warnings)
	}
}
