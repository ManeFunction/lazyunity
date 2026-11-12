package tui

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/ManeFunction/lazyunity/internal/config"
)

// configTemplate is what a new config.yml holds: every setting, commented out
// and set to its default, so the file documents itself and uncommenting a line
// is all it takes to change it. The defaults and the keybindings come from the
// code, so the template cannot drift from them.
func configTemplate() string {
	d := config.Default()
	g, t := d.GUI, d.GUI.Theme
	var b strings.Builder
	line := func(format string, a ...any) { fmt.Fprintf(&b, format+"\n", a...) }
	set := func(indent int, key, value, note string) {
		s := "# " + strings.Repeat("  ", indent) + key + ":"
		if value != "" {
			s += " " + value
		}
		if note != "" {
			s = fmt.Sprintf("%-40s # %s", s, note)
		}
		line("%s", s)
	}

	line("# lazyunity configuration. Every setting is listed below, commented out and")
	line("# set to its default: uncomment a line to change it. lazyunity only reads this")
	line("# file; when it changes, lazyunity restarts to apply it.")
	line("")
	set(0, "gui", "", "")
	set(1, "border", g.Border, "rounded, single, double, bold, hidden")
	set(1, "sidePanelWidth", strconv.FormatFloat(g.SidePanelWidth, 'f', -1, 64), "share of the screen for Projects, 0.2 to 0.8")
	set(1, "mouseEvents", strconv.FormatBool(g.MouseEvents), "")
	set(1, "showListFooter", strconv.FormatBool(g.ShowListFooter), `"1 of 20" on a list's bottom border`)
	set(1, "showPanelJumps", strconv.FormatBool(g.ShowPanelJumps), "the [1] in panel titles")
	set(1, "showBottomLine", strconv.FormatBool(g.ShowBottomLine), "the hint line")
	set(1, "screenMode", g.ScreenMode, "at startup: full, projects, editors")
	line("#   # A colour and modifiers. Colours: default, black, red, green, yellow, blue,")
	line("#   # magenta, cyan, white, their bright variants (brightblue ...) or '#rrggbb'.")
	line("#   # Modifiers: bold, dim, underline, reverse, strikethrough.")
	set(1, "theme", "", "")
	for _, c := range []struct {
		key  string
		v    []string
		note string
	}{
		{"activeBorderColor", t.ActiveBorderColor, ""},
		{"inactiveBorderColor", t.InactiveBorderColor, ""},
		{"searchingActiveBorderColor", t.SearchingActiveBorderColor, "while filtering"},
		{"optionsTextColor", t.OptionsTextColor, "the hint line"},
		{"selectedLineBgColor", t.SelectedLineBgColor, ""},
		{"selectedLineFgColor", t.SelectedLineFgColor, ""},
		{"inactiveViewSelectedLineBgColor", t.InactiveViewSelectedLineBgColor, "cursor line of an unfocused panel"},
		{"pendingColor", t.PendingColor, "a change not applied yet"},
		{"missingColor", t.MissingColor, "not installed, or a missing folder"},
		{"pinnedColor", t.PinnedColor, ""},
		{"defaultFgColor", t.DefaultFgColor, "the text"},
	} {
		set(2, c.key, yamlList(c.v), c.note)
	}
	line("")
	line("# Keys, per context. lazygit's notation works too: <c-s>, <a-x>, <space>,")
	line("# <enter>. A single key or a list; <disabled> or [] unbinds an action.")
	set(0, "keybinding", "", "")
	byCtx := make([][]*action, numContexts)
	for _, a := range buildActions() {
		byCtx[a.ctx] = append(byCtx[a.ctx], a)
	}
	for c, acts := range byCtx {
		if len(acts) == 0 {
			continue
		}
		set(1, ctxID(c).group(), "", "")
		for _, a := range acts {
			set(2, a.name(), yamlList(a.keys), a.desc)
		}
	}
	line("")
	set(0, "os", "", "")
	set(1, "edit", "''", "for e, {{dir}} is the folder; default: $VISUAL / $EDITOR")
	set(1, "openFolder", "''", "{{dir}} is the folder; default: xdg-open, open, explorer")
	set(1, "shell", "''", "runs : commands; default: $SHELL, or cmd on Windows")
	line("")
	set(0, "unity", "", "")
	set(1, "cliPath", "''", "the Unity CLI; default: found automatically")
	line("")
	set(0, "editors", "", "")
	set(1, "extraRoots", "[]", "folders with Editors, besides the CLI's install path")
	line("")
	set(0, "projects", "", "")
	set(1, "defaultLocation", "''", "where new projects go; e.g. ~/Projects")
	line("")
	set(0, "quitAfterOpen", strconv.FormatBool(d.QuitAfterOpen), "exit once a project has been opened")
	line("")
	line("# The OpenUPM CLI, installed from the Tools tab. While this stays commented out,")
	line("# the official release is installed from npm. Uncomment openupm to clone, build")
	line("# and install it from a git repository instead; ref picks a branch or tag, the")
	line("# default branch otherwise.")
	line("#")
	line("# Recommended: the fork below, until its pull request is merged upstream")
	line("# (https://github.com/openupm/openupm-cli/pull/430, not accepted yet). It drops")
	line("# deprecated dependencies (npmlog, rimraf, old npm registry libraries), so npm")
	line("# no longer floods the install with deprecation warnings, and cuts the npm audit")
	line("# findings from 19 vulnerable production packages (one critical) to 5. It also")
	line("# fixes `openupm login` with a username and password, which sent the email as")
	line("# the password and logged in to registry.npmjs.org instead of the requested")
	line("# registry. It needs Node.js 20.17+ or 22.9+ (Node 18 is dropped).")
	line("# For the official repository: repo: %s", config.OfficialOpenUPMRepo)
	line("# openupm:")
	line("#   repo: %s", config.ForkOpenUPMRepo)
	line("#   ref: %s", config.ForkOpenUPMRef)
	return b.String()
}

var plainKey = regexp.MustCompile(`^([A-Za-z0-9]|[a-z][a-z0-9+]*[a-z0-9])$`)

// yamlList writes values as a YAML flow list, quoting those YAML would read as
// something else ([, :, @ ...).
func yamlList(values []string) string {
	q := make([]string, len(values))
	for i, v := range values {
		if plainKey.MatchString(v) {
			q[i] = v
		} else {
			q[i] = "'" + strings.ReplaceAll(v, "'", "''") + "'"
		}
	}
	return "[" + strings.Join(q, ", ") + "]"
}
