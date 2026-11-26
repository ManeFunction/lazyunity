package tui

import (
	"context"
	"fmt"
	"sort"
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

// installedPackages is what a project has: the packages of its manifest,
// and every package Unity resolved, with those pulled in by others.
type installedPackages struct {
	direct map[string]string              // Packages/manifest.json, by name
	lock   map[string]unity.LockedPackage // Packages/packages-lock.json, by name
}

// dependency reports whether a package is there only because another one
// needs it, and with which version.
func (in installedPackages) dependency(id string) (string, bool) {
	if _, ok := in.direct[id]; ok {
		return "", false
	}
	lp, ok := in.lock[id]
	return lp.Version, ok
}

func (in installedPackages) has(id string) bool {
	_, direct := in.direct[id]
	_, dep := in.dependency(id)
	return direct || dep
}

// lockDependencies locks the packages the project has only as dependencies
// while a ticked package, or one of the manifest the popup does not offer,
// still needs them: they stay ticked and cannot be unticked. Once nothing
// that stays needs one, it unlocks, and left ticked, it is added to the
// manifest so that it stays.
func lockDependencies(pkgs []upmPackage, in installedPackages) func(items []menuItem) {
	graph := map[string][]string{}
	names := map[string]string{}
	offered := map[string]bool{}
	for _, p := range pkgs {
		graph[p.id] = append(graph[p.id], p.deps...)
		names[p.id] = p.name
		offered[p.id] = true
	}
	for id, lp := range in.lock {
		for d := range lp.Dependencies {
			graph[id] = append(graph[id], d)
		}
	}
	var fixed []string // what stays whatever is ticked
	for id := range in.direct {
		if !offered[id] {
			fixed = append(fixed, id)
		}
	}
	sort.Strings(fixed)
	// needs reports whether root needs id, directly or not.
	needs := func(root, id string) bool {
		seen := map[string]bool{root: true}
		for todo := []string{root}; len(todo) > 0; {
			cur := todo[0]
			todo = todo[1:]
			for _, d := range graph[cur] {
				if d == id {
					return true
				}
				if !seen[d] {
					seen[d] = true
					todo = append(todo, d)
				}
			}
		}
		return false
	}
	return func(items []menuItem) {
		roots := append([]string(nil), fixed...)
		for _, it := range items {
			if it.checked {
				roots = append(roots, it.id)
			}
		}
		for i := range items {
			items[i].locked = ""
			if _, dep := in.dependency(items[i].id); !dep {
				continue
			}
			var by []string
			for _, r := range roots {
				if r != items[i].id && needs(r, items[i].id) {
					if n := names[r]; n != "" {
						by = append(by, n)
					} else {
						by = append(by, r)
					}
				}
			}
			if len(by) > 0 {
				items[i].checked = true
				items[i].locked = fmt.Sprintf("%s is needed by %s: untick those to change it", items[i].label, strings.Join(by, ", "))
			}
		}
	}
}

// packagesPicker is the popup. in is what the project already has; submit
// gets what to add (with pinned versions) and what to remove.
func (m *Model) packagesPicker(title string, in installedPackages, submit func(m *Model, add, remove []string) tea.Cmd) *menuPopup {
	pkgs := offeredPackages(m.cfg.OpenUPM.Packages)
	byID := map[string]upmPackage{}
	items := make([]menuItem, 0, len(pkgs))
	for _, p := range pkgs {
		byID[p.id] = p
		right := p.id
		if p.name == p.id {
			right = ""
		}
		v, direct := in.direct[p.id]
		depV, dep := in.dependency(p.id)
		switch {
		case direct:
			right = strings.TrimSpace(right + " " + v)
		case dep:
			right = strings.TrimSpace(right + " " + depV + " (dependency)")
		case p.version != "":
			right = strings.TrimSpace(right + " @" + p.version)
		}
		has := in.has(p.id)
		items = append(items, menuItem{id: p.id, label: p.name, right: right, checked: has, installed: has})
	}
	lock := lockDependencies(pkgs, in)
	lock(items)
	menu := newMenu(title, items)
	menu.multi = true
	menu.wanted = 64
	follow := tickDeps(pkgs)
	menu.onToggle = func(items []menuItem, id string, on bool) {
		follow(items, id, on)
		lock(items)
	}
	menu.hints = []hintItem{{"Toggle", "<space>"}, {"GitHub", "o"}, {"Apply", "<enter>"}, {"Cancel", "<esc>"}}
	menu.itemKeys = map[string]func(m *Model, it menuItem) tea.Cmd{
		"o": func(m *Model, it menuItem) tea.Cmd { return m.openPackagePage(byID[it.id]) },
	}
	menu.submit = func(m *Model, picked []menuItem) tea.Cmd {
		ticked := map[string]menuItem{}
		for _, it := range picked {
			ticked[it.id] = it
		}
		var add, remove []string
		for _, p := range pkgs {
			it, on := ticked[p.id]
			_, direct := in.direct[p.id]
			depV, dep := in.dependency(p.id)
			switch {
			case on && dep && it.locked == "":
				// nothing that stays needs it any more: keep it, at its version
				ref := p.ref()
				if p.version == "" && depV != "" && in.lock[p.id].Source == "registry" {
					ref = p.id + "@" + depV
				}
				add = append(add, ref)
			case on && !direct && !dep:
				add = append(add, p.ref())
			case !on && direct:
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
	direct, err := unity.ReadManifest(r.Path)
	if err == nil {
		var lock map[string]unity.LockedPackage
		if lock, err = unity.ReadPackagesLock(r.Path); err == nil {
			return m.pushProjectPackages(r, exe, installedPackages{direct: direct, lock: lock})
		}
	}
	m.showError("Reading the packages of "+r.name(), err)
	return nil
}

func (m *Model) pushProjectPackages(r projectRow, exe string, in installedPackages) tea.Cmd {
	m.push(m.packagesPicker("OpenUPM packages for "+r.name(), in, func(m *Model, add, remove []string) tea.Cmd {
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
