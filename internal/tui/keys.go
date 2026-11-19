package tui

import (
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"

	"github.com/ManeFunction/lazyunity/internal/config"
)

// ctxID says where an action applies: everywhere, or only while a given list has
// focus.
type ctxID int

const (
	ctxGlobal ctxID = iota
	ctxProjects
	ctxEditors // the Installed tab
	ctxModules // inside an Editor
	ctxReleases
	ctxTools
	numContexts
)

// group is the name of the context in config.yml and in the ? menu.
func (c ctxID) group() string {
	return [...]string{"universal", "projects", "editors", "modules", "releases", "tools"}[c]
}

// action is something the user can do with a key.
type action struct {
	// id is "<group>.<name>", which is also its name in config.yml.
	id  string
	ctx ctxID
	// keys are the defaults, in Bubble Tea's notation ("ctrl+s", "space").
	keys []string
	// desc is the full description, shown in the ? menu.
	desc string
	// hint is the short label for the bottom line; empty keeps it off the line.
	hint string
	// nav marks navigation keys, which the ? menu lists under their own heading.
	nav bool
	// enabled decides whether the action is available right now. nil: always.
	enabled func(*Model) bool
	// notOn is an operating system (a GOOS) the action cannot work on. There
	// the action is never available and the ? menu leaves it out.
	notOn string
	run   func(*Model) tea.Cmd
}

func (a *action) name() string { return a.id[strings.IndexByte(a.id, '.')+1:] }

// available reports whether the action can run right now.
func (a *action) available(m *Model) bool {
	return a.notOn != m.goos && (a.enabled == nil || a.enabled(m))
}

// onSelected returns a function that completes an action working on the item
// selected in a list: the action is enabled while there is one (and its own
// enabled, if it has one, agrees), and it runs on that item.
func onSelected[T any](sel func(*Model) (T, bool)) func(a *action, f func(*Model, T) tea.Cmd) *action {
	return func(a *action, f func(*Model, T) tea.Cmd) *action {
		also := a.enabled
		a.enabled = func(m *Model) bool {
			_, ok := sel(m)
			return ok && (also == nil || also(m))
		}
		a.run = func(m *Model) tea.Cmd {
			it, ok := sel(m)
			if !ok {
				return nil
			}
			return f(m, it)
		}
		return a
	}
}

// keymap resolves key presses to actions.
type keymap struct {
	actions []*action
	bound   [numContexts]map[string]*action
	// keys are the keys bound to each action, after config overrides.
	keys map[*action][]string
}

// newKeymap binds the actions, applying overrides from config.yml. Problems
// (an unknown action, two actions on one key) are returned as warnings; the
// first binding of a key wins.
func newKeymap(actions []*action, overrides map[string]config.KeyGroup) (*keymap, []string) {
	km := &keymap{actions: actions, keys: map[*action][]string{}}
	for i := range km.bound {
		km.bound[i] = map[string]*action{}
	}
	var warnings []string

	byGroup := map[string]map[string]*action{}
	for _, a := range actions {
		g := a.ctx.group()
		if byGroup[g] == nil {
			byGroup[g] = map[string]*action{}
		}
		byGroup[g][a.name()] = a
	}
	override := map[*action][]string{}
	var groups []string
	for g := range overrides {
		groups = append(groups, g)
	}
	sort.Strings(groups)
	for _, g := range groups {
		var names []string
		for n := range overrides[g] {
			names = append(names, n)
		}
		sort.Strings(names)
		for _, n := range names {
			a, ok := byGroup[g][n]
			if !ok {
				warnings = append(warnings, fmt.Sprintf("keybinding.%s.%s: no such action", g, n))
				continue
			}
			var keys []string
			given := false
			for _, k := range overrides[g][n] {
				nk := config.NormalizeKey(k)
				if nk == "" {
					continue // <disabled>
				}
				given = true
				if !pressable(nk) {
					warnings = append(warnings, fmt.Sprintf("keybinding.%s.%s: %q is not a key that can be pressed; ignored", g, n, k))
					continue
				}
				keys = append(keys, nk)
			}
			if given && len(keys) == 0 {
				warnings = append(warnings, fmt.Sprintf("keybinding.%s.%s: no usable key; the default is kept", g, n))
				continue
			}
			override[a] = keys // an empty list disables the action's keys
		}
	}

	// Keys the user configured are bound first, so they win over the defaults
	// of other actions; within each pass the first action in the table wins.
	bind := func(a *action, keys []string) {
		for _, k := range keys {
			if prev, taken := km.bound[a.ctx][k]; taken {
				warnings = append(warnings, fmt.Sprintf("key %q is bound to both %s and %s; using %s", k, prev.id, a.id, prev.id))
				continue
			}
			km.bound[a.ctx][k] = a
			km.keys[a] = append(km.keys[a], k)
		}
	}
	for _, a := range actions {
		if keys, ok := override[a]; ok {
			bind(a, keys)
		}
	}
	for _, a := range actions {
		if _, ok := override[a]; !ok {
			bind(a, a.keys)
		}
	}
	return km, warnings
}

// lookup finds the action for a key pressed while ctx has focus. The focused
// list's own bindings win over the global ones.
func (km *keymap) lookup(ctx ctxID, key string) *action {
	if a, ok := km.bound[ctx][key]; ok {
		return a
	}
	return km.bound[ctxGlobal][key]
}

// keyLabel is how a key is shown to the user.
func keyLabel(k string) string {
	switch k {
	case "space":
		return "<space>"
	case "enter":
		return "<enter>"
	case "esc":
		return "<esc>"
	case "tab":
		return "<tab>"
	case "shift+tab":
		return "<s-tab>"
	case "pgup", "pgdown", "home", "end", "up", "down", "left", "right", "backspace", "delete":
		return "<" + k + ">"
	}
	if rest, ok := strings.CutPrefix(k, "ctrl+"); ok {
		return "<c-" + rest + ">"
	}
	if rest, ok := strings.CutPrefix(k, "alt+"); ok {
		return "<a-" + rest + ">"
	}
	return k
}

// primaryKey is the key to show for an action: its first one.
func (km *keymap) primaryKey(a *action) string {
	if ks := km.keys[a]; len(ks) > 0 {
		return keyLabel(ks[0])
	}
	return ""
}

// allKeys is every key of an action, for the ? menu.
func (km *keymap) allKeys(a *action) string {
	var out []string
	for _, k := range km.keys[a] {
		out = append(out, keyLabel(k))
	}
	return strings.Join(out, ", ")
}

// hintItem is one "Description: key" entry of the bottom line.
type hintItem struct{ label, key string }

// hints lists what to show on the bottom line while ctx has focus: its own
// actions, then the global ones that carry a hint. Disabled actions are left out.
func (km *keymap) hints(m *Model, ctx ctxID) []hintItem {
	var out []hintItem
	add := func(c ctxID) {
		for _, a := range km.actions {
			if a.ctx != c || a.hint == "" {
				continue
			}
			if !a.available(m) {
				continue
			}
			if k := km.primaryKey(a); k != "" {
				out = append(out, hintItem{a.hint, k})
			}
		}
	}
	if ctx != ctxGlobal {
		add(ctx)
	}
	add(ctxGlobal)
	return out
}

// renderHints joins hints as lazygit does: "Open: enter | Version: v | ...",
// cutting the end with … when it does not fit.
func renderHints(th *Theme, items []hintItem, w int) string {
	var parts []string
	for _, h := range items {
		parts = append(parts, h.label+": "+h.key)
	}
	s := strings.Join(parts, " | ")
	return th.OptionsText.Render(truncate(s, w))
}

// keyMods are the modifiers as Bubble Tea writes them, in the order it writes
// them: a key spelled with them in another order is never pressed.
var keyMods = []string{"ctrl+", "alt+", "shift+", "meta+", "hyper+", "super+"}

// keyNames are the keys Bubble Tea names rather than writes as a character.
var keyNames = func() map[string]bool {
	names := map[string]bool{}
	for _, n := range strings.Fields("enter tab backspace esc space up down left right begin find insert delete " +
		"select pgup pgdown home end capslock scrolllock numlock printscreen pause menu") {
		names[n] = true
	}
	for i := 1; i <= 63; i++ {
		names[fmt.Sprintf("f%d", i)] = true
	}
	return names
}()

// pressable reports whether k is spelled as Bubble Tea spells a key press, so
// that a binding to it can ever match.
func pressable(k string) bool {
	for _, mod := range keyMods {
		k = strings.TrimPrefix(k, mod) // in order: each at most once
	}
	return keyNames[k] || (utf8.RuneCountInString(k) == 1 && k != " ")
}
