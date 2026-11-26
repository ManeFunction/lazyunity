package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/ManeFunction/lazyunity/internal/config"
	"github.com/ManeFunction/lazyunity/internal/unity"
)

// The recommended OpenUPM packages popup: a step of the new project wizard,
// and P on a project. It ticks what the project already has, adds what is
// ticked with openupm add, and removes what is unticked with openupm remove.

// upmPackage is a package the popup offers.
type upmPackage struct {
	id      string
	name    string
	version string // pinned, or "" for the newest
	repo    string // its source on GitHub; "" asks the registry
	deps    []string
}

// ref is what openupm add is given: the name, with the pinned version.
func (p upmPackage) ref() string {
	if p.version == "" {
		return p.id
	}
	return p.id + "@" + p.version
}

const (
	pkgManeTools   = "com.manefunction.tools-unity"
	pkgManeToolsUI = "com.manefunction.tools-unity-ui"
)

// recommendedPackages are offered first, in this order.
var recommendedPackages = []upmPackage{
	{id: pkgManeTools, name: "ManeTools", repo: "https://github.com/ManeFunction/ManeTools-Unity"},
	{id: pkgManeToolsUI, name: "ManeTools for UI", repo: "https://github.com/ManeFunction/ManeTools-UnityUI", deps: []string{pkgManeTools}},
	{id: "com.manefunction.sound-maneger", name: "Sound Maneger", repo: "https://github.com/ManeFunction/sound-maneger", deps: []string{pkgManeTools}},
	{id: "com.cysharp.unitask", name: "UniTask", repo: "https://github.com/Cysharp/UniTask"},
	{id: "com.coffee.ui-particle", name: "Particle Effect For UGUI", repo: "https://github.com/mob-sakai/ParticleEffectForUGUI"},
	{id: "com.coffee.softmask-for-ugui", name: "SoftMask For UGUI", repo: "https://github.com/mob-sakai/SoftMaskForUGUI"},
	{id: "com.coffee.ui-effect", name: "UIEffect", repo: "https://github.com/mob-sakai/UIEffect"},
}

// offeredPackages are the recommended packages, then those of
// openupm.packages in config.yml. A configured package that is already in the
// list only pins its version.
func offeredPackages(extra []string) []upmPackage {
	out := append([]upmPackage(nil), recommendedPackages...)
	at := map[string]int{}
	for i, p := range out {
		at[p.id] = i
	}
	for _, e := range extra {
		id, ver := config.ParsePackage(e)
		if id == "" {
			continue
		}
		if i, ok := at[id]; ok {
			if ver != "" {
				out[i].version = ver
			}
			continue
		}
		at[id] = len(out)
		out = append(out, upmPackage{id: id, name: id, version: ver})
	}
	return out
}

// tickDeps keeps the ticks consistent with the dependencies: ticking a
// package ticks what it needs, unticking one unticks what needs it.
func tickDeps(pkgs []upmPackage) func(items []menuItem, id string, on bool) {
	deps := map[string][]string{}
	for _, p := range pkgs {
		deps[p.id] = p.deps
	}
	return func(items []menuItem, id string, on bool) {
		set := map[string]bool{id: true}
		if on {
			for todo := []string{id}; len(todo) > 0; {
				cur := todo[0]
				todo = todo[1:]
				for _, d := range deps[cur] {
					if !set[d] {
						set[d] = true
						todo = append(todo, d)
					}
				}
			}
		} else {
			for grew := true; grew; {
				grew = false
				for p, ds := range deps {
					for _, d := range ds {
						if set[d] && !set[p] {
							set[p], grew = true, true
						}
					}
				}
			}
		}
		for i := range items {
			if set[items[i].id] {
				items[i].checked = on
			}
		}
	}
}

// packagesPicker is the popup. installed are the packages the project
// already has, by name; submit gets what to add (with pinned versions) and
// what to remove.
func (m *Model) packagesPicker(title string, installed map[string]string, submit func(m *Model, add, remove []string) tea.Cmd) *menuPopup {
	pkgs := offeredPackages(m.cfg.OpenUPM.Packages)
	byID := map[string]upmPackage{}
	items := make([]menuItem, 0, len(pkgs))
	for _, p := range pkgs {
		byID[p.id] = p
		right := p.id
		if p.name == p.id {
			right = ""
		}
		if v, ok := installed[p.id]; ok {
			if right != "" {
				right += " "
			}
			right += v
		} else if p.version != "" {
			right = strings.TrimSpace(right + " @" + p.version)
		}
		_, has := installed[p.id]
		items = append(items, menuItem{id: p.id, label: p.name, right: right, checked: has, installed: has})
	}
	menu := newMenu(title, items)
	menu.multi = true
	menu.wanted = 64
	menu.onToggle = tickDeps(pkgs)
	menu.hints = []hintItem{{"Toggle", "<space>"}, {"GitHub", "o"}, {"Apply", "<enter>"}, {"Cancel", "<esc>"}}
	menu.itemKeys = map[string]func(m *Model, it menuItem) tea.Cmd{
		"o": func(m *Model, it menuItem) tea.Cmd { return m.openPackagePage(byID[it.id]) },
	}
	menu.submit = func(m *Model, picked []menuItem) tea.Cmd {
		ticked := map[string]bool{}
		for _, it := range picked {
			ticked[it.id] = true
		}
		var add, remove []string
		for _, p := range pkgs {
			_, has := installed[p.id]
			switch {
			case ticked[p.id] && !has:
				add = append(add, p.ref())
			case !ticked[p.id] && has:
				remove = append(remove, p.id)
			}
		}
		return submit(m, add, remove)
	}
	return menu
}

// openPackagePage opens a package's source repository in the browser. One
// that is not built in is looked up in the OpenUPM registry, and without a
// repository there, its OpenUPM page opens instead.
func (m *Model) openPackagePage(p upmPackage) tea.Cmd {
	if p.repo != "" {
		if err := m.host.OpenURL(p.repo); err != nil {
			m.toastf(toastError, "Could not open the browser: %v", err)
		}
		return nil
	}
	h := m.host
	m.toastf(toastInfo, "Looking up %s…", p.id)
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		u, err := h.PackageRepo(ctx, p.id)
		if err != nil || u == "" {
			u = "https://openupm.com/packages/" + p.id + "/"
		}
		if err := h.OpenURL(u); err != nil {
			return errorMsg{what: "Opening the browser", err: err}
		}
		return nil
	}
}

// projectPackages opens the popup for a project.
func (m *Model) projectPackages(r projectRow) tea.Cmd {
	if r.Missing {
		m.toastf(toastError, "The folder of %s is gone: %s", r.name(), displayPath(r.Path))
		return nil
	}
	exe := m.openupm.path
	if exe == "" {
		m.toastf(toastWarn, "Install the OpenUPM CLI first (Options, Tools)")
		return nil
	}
	installed, err := unity.ReadManifest(r.Path)
	if err != nil {
		m.showError("Reading the packages of "+r.name(), err)
		return nil
	}
	m.push(m.packagesPicker("OpenUPM packages for "+r.name(), installed, func(m *Model, add, remove []string) tea.Cmd {
		switch {
		case len(add) == 0 && len(remove) == 0:
			return nil
		case len(remove) == 0:
			return m.changePackages(r, exe, add, nil)
		}
		msg := fmt.Sprintf("Remove %s from %s with openupm remove?", strings.Join(remove, ", "), r.name())
		if len(add) > 0 {
			msg += fmt.Sprintf("\n\nThen add %s.", strings.Join(add, ", "))
		}
		m.push(&confirmPopup{
			title: "Remove packages", yes: "Remove", danger: true, message: msg,
			onYes: func(m *Model) tea.Cmd { return m.changePackages(r, exe, add, remove) },
		})
		return nil
	}))
	return nil
}

// changePackages removes, then adds, packages of a project, as a task.
func (m *Model) changePackages(r projectRow, exe string, add, remove []string) tea.Cmd {
	h := m.host
	var parts []string
	if len(add) > 0 {
		parts = append(parts, "add "+strings.Join(add, ", "))
	}
	if len(remove) > 0 {
		parts = append(parts, "remove "+strings.Join(remove, ", "))
	}
	what := strings.Join(parts, "; ")
	return m.enqueue(&task{
		title:    "Packages of " + r.name() + ": " + what,
		label:    r.name() + ": " + what,
		works:    []string{workProject(r.Path)},
		removing: len(add) == 0,
		msg:      "OpenUPM is changing the packages",
		do: func(ctx context.Context) error {
			if len(remove) > 0 {
				if err := h.RemovePackages(ctx, exe, r.Path, remove); err != nil {
					return err
				}
			}
			if len(add) > 0 {
				return h.AddPackages(ctx, exe, r.Path, add)
			}
			return nil
		},
		after: func(m *Model, err error) tea.Cmd {
			if err == nil {
				m.toastf(toastOK, "Packages of %s: %s", r.name(), what)
			}
			return nil
		},
	})
}
