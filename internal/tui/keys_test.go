package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/ManeFunction/lazyunity/internal/config"
)

func TestDefaultKeymapIsSound(t *testing.T) {
	actions := buildActions()
	ids := map[string]bool{}
	for _, a := range actions {
		if ids[a.id] {
			t.Errorf("duplicate action id %s", a.id)
		}
		ids[a.id] = true
		if !strings.HasPrefix(a.id, a.ctx.group()+".") {
			t.Errorf("%s does not belong to the %s group", a.id, a.ctx.group())
		}
		if a.desc == "" || a.run == nil {
			t.Errorf("%s needs a description and a handler", a.id)
		}
		if len(a.keys) == 0 {
			t.Errorf("%s has no default key", a.id)
		}
	}
	km, warnings := newKeymap(actions, nil)
	if len(warnings) != 0 {
		t.Errorf("the defaults conflict with themselves: %v", warnings)
	}
	// every default key reaches its action
	for _, a := range actions {
		for _, k := range a.keys {
			if got := km.bound[a.ctx][k]; got != a {
				t.Errorf("%s: key %q resolves to %v", a.id, k, got)
			}
		}
	}
}

// TestLazygitKeysAreThere pins the keys that lazygit users reach for.
func TestLazygitKeysAreThere(t *testing.T) {
	km, _ := newKeymap(buildActions(), nil)
	want := map[string]string{
		"?": "universal.openMenu", "q": "universal.quit", "ctrl+c": "universal.quit", "esc": "universal.return",
		"/": "universal.filter", "R": "universal.refresh", "+": "universal.nextScreenMode", "_": "universal.prevScreenMode",
		"j": "universal.nextItem", "k": "universal.prevItem", "down": "universal.nextItem", "up": "universal.prevItem",
		"h": "universal.prevPanel", "l": "universal.nextPanel", "tab": "universal.nextPanel", "shift+tab": "universal.prevPanel",
		"1": "universal.jumpProjects", "2": "universal.jumpEditors", "3": "universal.jumpOptions",
		"[": "universal.prevTab", "]": "universal.nextTab",
		",": "universal.prevPage", ".": "universal.nextPage", "<": "universal.goStart", ">": "universal.goEnd",
		"ctrl+z": "universal.suspend",
	}
	for key, id := range want {
		a := km.lookup(ctxGlobal, key)
		if a == nil || a.id != id {
			t.Errorf("key %q should be %s, got %v", key, id, a)
		}
	}
}

func TestKeymapOverrides(t *testing.T) {
	cfg := map[string]config.KeyGroup{
		"universal": {
			"quit":    {"x", "<c-q>"}, // replaces q and ctrl+c
			"refresh": {"<disabled>"}, // switches R off
			"bogus":   {"z"},          // no such action
			"filter":  {"<f2>"},       // a key this program cannot name still binds
		},
		"nonsense": {"quit": {"y"}},
	}
	km, warnings := newKeymap(buildActions(), cfg)
	if a := km.lookup(ctxGlobal, "x"); a == nil || a.id != "universal.quit" {
		t.Errorf("x: %v", a)
	}
	if a := km.lookup(ctxGlobal, "ctrl+q"); a == nil || a.id != "universal.quit" {
		t.Errorf("<c-q> should become ctrl+q: %v", a)
	}
	if km.lookup(ctxGlobal, "q") != nil || km.lookup(ctxGlobal, "ctrl+c") != nil {
		t.Error("overriding an action must drop its default keys")
	}
	if km.lookup(ctxGlobal, "R") != nil {
		t.Error("<disabled> must switch the action off")
	}
	joined := strings.Join(warnings, "\n")
	for _, want := range []string{"universal.bogus", "nonsense.quit"} {
		if !strings.Contains(joined, want) {
			t.Errorf("expected a warning about %s, got %q", want, joined)
		}
	}
}

func TestKeymapUserKeysBeatDefaults(t *testing.T) {
	// j is the default of nextItem, which comes after refresh in the table
	cfg := map[string]config.KeyGroup{"universal": {"refresh": {"j"}}}
	km, warnings := newKeymap(buildActions(), cfg)
	if a := km.lookup(ctxGlobal, "j"); a == nil || a.id != "universal.refresh" {
		t.Errorf("the key the user chose must win, got %v", a)
	}
	if a := km.lookup(ctxGlobal, "down"); a == nil || a.id != "universal.nextItem" {
		t.Errorf("the other default key of nextItem must survive, got %v", a)
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], `"j"`) || !strings.Contains(warnings[0], "using universal.refresh") {
		t.Errorf("warnings = %v", warnings)
	}
}

func TestKeymapTwoUserKeysConflictFirstWins(t *testing.T) {
	cfg := map[string]config.KeyGroup{"universal": {"refresh": {"F"}, "filter": {"F"}}}
	km, warnings := newKeymap(buildActions(), cfg)
	// table order decides: filter comes before refresh
	if a := km.lookup(ctxGlobal, "F"); a == nil || a.id != "universal.filter" {
		t.Errorf("got %v", a)
	}
	if len(warnings) != 1 {
		t.Errorf("warnings = %v", warnings)
	}
}

func TestLocalKeysShadowGlobalOnes(t *testing.T) {
	run := func(string) func(*Model) tea.Cmd { return func(*Model) tea.Cmd { return nil } }
	acts := []*action{
		{id: "universal.a", ctx: ctxGlobal, keys: []string{"x"}, desc: "a", run: run("")},
		{id: "projects.b", ctx: ctxProjects, keys: []string{"x"}, desc: "b", run: run("")},
	}
	km, warnings := newKeymap(acts, nil)
	if len(warnings) != 0 {
		t.Errorf("the same key in different contexts is allowed: %v", warnings)
	}
	if a := km.lookup(ctxProjects, "x"); a.id != "projects.b" {
		t.Errorf("projects: %s", a.id)
	}
	if a := km.lookup(ctxEditors, "x"); a.id != "universal.a" {
		t.Errorf("editors fall back to the global one: %s", a.id)
	}
}

func TestKeyLabels(t *testing.T) {
	cases := map[string]string{
		"space": "<space>", "enter": "<enter>", "esc": "<esc>", "tab": "<tab>", "shift+tab": "<s-tab>",
		"ctrl+s": "<c-s>", "alt+x": "<a-x>", "pgup": "<pgup>", "up": "<up>", "j": "j", "?": "?", "R": "R",
	}
	for in, want := range cases {
		if got := keyLabel(in); got != want {
			t.Errorf("keyLabel(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestKeyLabelRoundTripsThroughConfigNotation(t *testing.T) {
	// what the menu shows can be pasted into config.yml
	for _, k := range []string{"ctrl+s", "alt+x", "space", "enter", "esc", "tab", "shift+tab", "pgup", "j", "?"} {
		if got := config.NormalizeKey(keyLabel(k)); got != k {
			t.Errorf("%q -> %q -> %q", k, keyLabel(k), got)
		}
	}
}

func TestHintsAndTruncation(t *testing.T) {
	m := newTestModel(t)
	hints := m.keymap.hints(m, ctxProjects)
	if len(hints) == 0 || hints[len(hints)-1] != (hintItem{"Keybindings", "?"}) {
		t.Errorf("the line must end with the keybindings hint: %v", hints)
	}
	th := m.th
	full := renderHints(th, []hintItem{{"Open", "enter"}, {"Version", "v"}, {"Keybindings", "?"}}, 100)
	if got := strings.TrimSpace(stripANSI(full)); got != "Open: enter | Version: v | Keybindings: ?" {
		t.Errorf("hints = %q", got)
	}
	cut := renderHints(th, []hintItem{{"Open", "enter"}, {"Version", "v"}, {"Keybindings", "?"}}, 20)
	if width(cut) > 20 || !strings.HasSuffix(stripANSI(cut), "…") {
		t.Errorf("a long line must be cut with an ellipsis: %q", stripANSI(cut))
	}
}
