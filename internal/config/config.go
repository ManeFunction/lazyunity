// Package config reads lazyunity's user-authored config.yml. The file is only
// ever read: anything the app itself remembers lives in the state package.
package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config is the contents of config.yml. Its shape follows lazygit's config
// where an equivalent setting exists, so lazygit users can guess the keys.
type Config struct {
	GUI        GUI                 `yaml:"gui"`
	Keybinding map[string]KeyGroup `yaml:"keybinding"`
	OS         OS                  `yaml:"os"`
	Unity      Unity               `yaml:"unity"`
	Editors    Editors             `yaml:"editors"`
	Projects   Projects            `yaml:"projects"`
	OpenUPM    OpenUPM             `yaml:"openupm"`
	// QuitAfterOpen exits lazyunity once a project has been opened.
	QuitAfterOpen bool `yaml:"quitAfterOpen"`
}

// GUI is the look of the interface.
type GUI struct {
	Theme Theme `yaml:"theme"`
	// Border is rounded, single, double, bold or hidden.
	Border string `yaml:"border"`
	// SidePanelWidth is the share of the screen the Projects panel takes.
	SidePanelWidth float64 `yaml:"sidePanelWidth"`
	MouseEvents    bool    `yaml:"mouseEvents"`
	ShowListFooter bool    `yaml:"showListFooter"` // "1 of 20" on the bottom border
	ShowPanelJumps bool    `yaml:"showPanelJumps"` // the [1] in panel titles
	ShowBottomLine bool    `yaml:"showBottomLine"` // the hint line
	// ScreenMode is the mode at startup: full (every panel), projects (only
	// Projects) or editors (Editors and Options).
	ScreenMode string `yaml:"screenMode"`
}

// Theme holds colours. Each entry is a list of a colour and modifiers, as in
// lazygit: [blue, bold]. Colours are default, black, red, green, yellow, blue,
// magenta, cyan, white, their bright variants (brightblue ...) or '#rrggbb'.
// Modifiers are bold, dim, underline, reverse and strikethrough.
type Theme struct {
	ActiveBorderColor          []string `yaml:"activeBorderColor"`
	InactiveBorderColor        []string `yaml:"inactiveBorderColor"`
	SearchingActiveBorderColor []string `yaml:"searchingActiveBorderColor"`
	OptionsTextColor           []string `yaml:"optionsTextColor"`
	SelectedLineBgColor        []string `yaml:"selectedLineBgColor"`
	SelectedLineFgColor        []string `yaml:"selectedLineFgColor"`
	// InactiveViewSelectedLineBgColor marks the cursor line of a panel that is
	// not focused.
	InactiveViewSelectedLineBgColor []string `yaml:"inactiveViewSelectedLineBgColor"`
	// PendingColor marks a version or platform change that has not been
	// applied yet; MissingColor a version that is not installed or a missing
	// folder; PinnedColor the pin.
	PendingColor []string `yaml:"pendingColor"`
	MissingColor []string `yaml:"missingColor"`
	PinnedColor  []string `yaml:"pinnedColor"`
	// DefaultFgColor is the text colour.
	DefaultFgColor []string `yaml:"defaultFgColor"`
}

// KeyGroup maps an action name to its keys, e.g. quit: [q, <c-c>].
type KeyGroup map[string]StringList

// StringList accepts a single string or a list in YAML.
type StringList []string

// UnmarshalYAML implements yaml.Unmarshaler.
func (s *StringList) UnmarshalYAML(n *yaml.Node) error {
	switch n.Kind {
	case yaml.ScalarNode:
		*s = StringList{n.Value}
		return nil
	case yaml.SequenceNode:
		var l []string
		if err := n.Decode(&l); err != nil {
			return err
		}
		*s = l
		return nil
	}
	return fmt.Errorf("line %d: expected a key or a list of keys", n.Line)
}

// OS is how lazyunity talks to the operating system.
type OS struct {
	// Edit opens a project in a code editor. {{dir}} is the project folder.
	// Empty: use $VISUAL or $EDITOR in the terminal.
	Edit string `yaml:"edit"`
	// OpenFolder opens a folder in the file manager. {{dir}} is the folder.
	// Empty: the system's default (xdg-open, open, explorer).
	OpenFolder string `yaml:"openFolder"`
	// Shell is the program that runs ':' commands. Empty: $SHELL, or cmd on Windows.
	Shell string `yaml:"shell"`
}

// Unity is about the Unity CLI.
type Unity struct {
	// CLIPath is the CLI to use. Empty: look for it.
	CLIPath string `yaml:"cliPath"`
}

// Editors is about Editor discovery.
type Editors struct {
	// ExtraRoots are folders with Editors in them, besides the CLI's install path.
	ExtraRoots []string `yaml:"extraRoots"`
}

// Projects is about projects.
type Projects struct {
	// DefaultLocation is the folder new projects are created in.
	DefaultLocation string `yaml:"defaultLocation"`
}

// OfficialOpenUPMRepo is where openupm-cli is developed.
const OfficialOpenUPMRepo = "https://github.com/openupm/openupm-cli"

// ForkOpenUPMRepo and ForkOpenUPMRef are the maintained fork of openupm-cli
// that the config template suggests, commented out, until its pull request
// (openupm/openupm-cli#430) is merged.
const (
	ForkOpenUPMRepo = "https://github.com/ManeFunction/openupm-cli"
	ForkOpenUPMRef  = "no-more-legacy"
)

// OpenUPM is about the OpenUPM CLI, which the Tools tab installs.
type OpenUPM struct {
	// Repo is a git repository of openupm-cli to clone, build and install with
	// npm: the official one or a fork. Empty (the default): the official
	// release is installed from the npm registry.
	Repo string `yaml:"repo"`
	// Ref is the branch or tag of Repo to build. Empty: its default branch.
	Ref string `yaml:"ref"`
}

// FromRegistry reports whether the OpenUPM CLI comes from the npm registry,
// which it does unless a repository is set.
func (o OpenUPM) FromRegistry() bool { return strings.TrimSpace(o.Repo) == "" }

// Default returns the built-in configuration.
func Default() Config {
	return Config{
		GUI: GUI{
			Theme: Theme{
				ActiveBorderColor:               []string{"brightblue", "bold"},
				InactiveBorderColor:             []string{"default"},
				SearchingActiveBorderColor:      []string{"cyan", "bold"},
				OptionsTextColor:                []string{"brightblue"},
				SelectedLineBgColor:             []string{"blue"},
				SelectedLineFgColor:             []string{"bold"},
				InactiveViewSelectedLineBgColor: []string{"brightblack"},
				PendingColor:                    []string{"yellow"},
				MissingColor:                    []string{"red"},
				PinnedColor:                     []string{"yellow"},
				DefaultFgColor:                  []string{"default"},
			},
			Border:         "rounded",
			SidePanelWidth: 0.45,
			MouseEvents:    true,
			ShowListFooter: true,
			ShowPanelJumps: true,
			ShowBottomLine: true,
			ScreenMode:     "full",
		},
	}
}

// Dir is the folder holding config.yml and state.yml. override, if not empty,
// wins; then $LAZYUNITY_CONFIG_DIR; then the user's config folder.
func Dir(override string) (string, error) {
	if override != "" {
		return override, nil
	}
	if d := os.Getenv("LAZYUNITY_CONFIG_DIR"); d != "" {
		return d, nil
	}
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "lazyunity"), nil
}

// Load reads config.yml from dir. A missing file is not an error; the defaults
// are returned. Keys that are absent keep their default.
func Load(dir string) (Config, error) {
	cfg := Default()
	path := filepath.Join(dir, "config.yml")
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return cfg, nil
	}
	if err != nil {
		return cfg, err
	}
	if err := yaml.Unmarshal(b, &cfg); err != nil {
		return Default(), fmt.Errorf("%s: %w", path, err)
	}
	if err := cfg.validate(); err != nil {
		return Default(), fmt.Errorf("%s: %w", path, err)
	}
	return cfg, nil
}

func (c *Config) validate() error {
	switch c.GUI.Border {
	case "rounded", "single", "double", "bold", "hidden":
	default:
		return fmt.Errorf("gui.border: %q is not one of rounded, single, double, bold, hidden", c.GUI.Border)
	}
	switch c.GUI.ScreenMode {
	case "full", "projects", "editors":
	default:
		return fmt.Errorf("gui.screenMode: %q is not one of full, projects, editors", c.GUI.ScreenMode)
	}
	if strings.HasPrefix(strings.TrimSpace(c.OpenUPM.Repo), "-") || strings.HasPrefix(strings.TrimSpace(c.OpenUPM.Ref), "-") {
		return fmt.Errorf("openupm: repo and ref cannot start with '-'")
	}
	if c.OpenUPM.FromRegistry() && strings.TrimSpace(c.OpenUPM.Ref) != "" {
		return fmt.Errorf("openupm.ref: a branch or tag needs openupm.repo, the repository to build it from")
	}
	if w := c.GUI.SidePanelWidth; w < 0.2 || w > 0.8 {
		return fmt.Errorf("gui.sidePanelWidth: %v is not between 0.2 and 0.8", w)
	}
	for group, keys := range c.Keybinding {
		for action, list := range keys {
			for _, k := range list {
				if strings.TrimSpace(k) == "" {
					return fmt.Errorf("keybinding.%s.%s: empty key", group, action)
				}
			}
		}
	}
	return nil
}

// NormalizeKey turns a key written in lazygit's notation (<c-s>, <a-x>,
// <space>, <backtab>) into Bubble Tea's (ctrl+s, alt+x, space, shift+tab).
// Anything else is returned as it is.
func NormalizeKey(k string) string {
	if !strings.HasPrefix(k, "<") || !strings.HasSuffix(k, ">") || len(k) < 3 {
		return k
	}
	inner := k[1 : len(k)-1]
	lower := strings.ToLower(inner)
	switch {
	case strings.HasPrefix(lower, "c-") && len(inner) == 3:
		return "ctrl+" + lower[2:]
	case strings.HasPrefix(lower, "a-") && len(inner) == 3:
		return "alt+" + lower[2:]
	}
	switch lower {
	case "backtab", "s-tab":
		return "shift+tab"
	case "pgup", "pgdown", "home", "end", "up", "down", "left", "right",
		"enter", "esc", "tab", "space", "backspace", "delete", "insert":
		return lower
	case "pageup":
		return "pgup"
	case "pagedown":
		return "pgdown"
	case "disabled":
		return ""
	}
	return k
}

// ExpandHome turns a leading ~ into the user's home folder, for paths written in
// the config. Anything else is returned as it is.
func ExpandHome(p string) string {
	if p != "~" && !strings.HasPrefix(p, "~/") && !strings.HasPrefix(p, `~\`) {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return p
	}
	return filepath.Join(home, p[1:])
}
