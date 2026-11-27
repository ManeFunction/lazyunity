package tui

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/ManeFunction/lazyunity/internal/config"
	"github.com/ManeFunction/lazyunity/internal/openupm"
	"github.com/ManeFunction/lazyunity/internal/unity"
)

// The OpenUPM packages popup: a step of the new project wizard, and P on a
// project. It lists the built-in packages, those of config.yml and the other
// OpenUPM packages the project has, in groups, with a field at the bottom to
// add any package of the registry to the list. It ticks what the project
// already has, adds what is ticked with openupm add, and removes what is
// unticked with openupm remove.

// pkgGroup is the group of the popup a package is listed in.
type pkgGroup int

const (
	groupMine    pkgGroup = iota // ManeFunction's own packages
	groupPicks                   // others ManeFunction recommends
	groupConfig                  // openupm.packages of config.yml
	groupProject                 // the project's other OpenUPM packages, and those added in the popup
)

var groupTitles = [...]string{
	groupMine:    "Mane Packages",
	groupPicks:   "Often used",
	groupConfig:  "Your selection",
	groupProject: "Other installed",
}

// upmPackage is a package the popup offers.
type upmPackage struct {
	id      string
	name    string
	version string // pinned, or "" for the newest
	repo    string // its source on GitHub; "" asks the registry
	deps    []string
	group   pkgGroup
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

// builtinPackages are offered first, in this order.
var builtinPackages = []upmPackage{
	{id: pkgManeTools, name: "ManeTools", repo: "https://github.com/ManeFunction/ManeTools-Unity", group: groupMine},
	{id: pkgManeToolsUI, name: "ManeTools for UI", repo: "https://github.com/ManeFunction/ManeTools-UnityUI", deps: []string{pkgManeTools}, group: groupMine},
	{id: "com.manefunction.sound-maneger", name: "Sound Maneger", repo: "https://github.com/ManeFunction/sound-maneger", deps: []string{pkgManeTools}, group: groupMine},
	{id: "com.cysharp.unitask", name: "UniTask", repo: "https://github.com/Cysharp/UniTask", group: groupPicks},
	{id: "com.coffee.ui-particle", name: "Particle Effect For UGUI", repo: "https://github.com/mob-sakai/ParticleEffectForUGUI", group: groupPicks},
	{id: "com.coffee.softmask-for-ugui", name: "SoftMask For UGUI", repo: "https://github.com/mob-sakai/SoftMaskForUGUI", group: groupPicks},
	{id: "com.coffee.ui-effect", name: "UIEffect", repo: "https://github.com/mob-sakai/UIEffect", group: groupPicks},
}

// offeredPackages are the built-in packages of the groups config.yml shows,
// then those of openupm.packages, then the other OpenUPM packages of the
// project's manifest, by name. A configured package that is already in the
// list only pins its version.
func offeredPackages(o config.OpenUPM, in installedPackages) []upmPackage {
	var out []upmPackage
	for _, p := range builtinPackages {
		if (p.group == groupMine && o.ShowManePackages) || (p.group == groupPicks && o.ShowOftenUsed) {
			out = append(out, p)
		}
	}
	extra := o.Packages
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
		out = append(out, upmPackage{id: id, name: id, version: ver, group: groupConfig})
	}
	var others []string
	for id := range in.direct {
		if _, ok := at[id]; !ok && in.openupm[id] {
			others = append(others, id)
		}
	}
	sort.Strings(others)
	for _, id := range others {
		out = append(out, upmPackage{id: id, name: id, group: groupProject})
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
	// openupm are the packages of the manifest that come from the OpenUPM
	// registry.
	openupm map[string]bool
}

// readInstalled reads what the project at path has.
func readInstalled(path string) (installedPackages, error) {
	mf, err := unity.LoadManifest(path)
	if err != nil {
		return installedPackages{}, err
	}
	lock, err := unity.ReadPackagesLock(path)
	if err != nil {
		return installedPackages{}, err
	}
	in := installedPackages{direct: mf.Dependencies, lock: lock, openupm: map[string]bool{}}
	for id := range mf.Dependencies {
		lp := lock[id]
		fromLock := lp.Source == "registry" && strings.TrimRight(lp.URL, "/") == openupm.Registry
		if mf.FromRegistry(id, openupm.Registry) || fromLock {
			in.openupm[id] = true
		}
	}
	return in, nil
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

// pkgItem is the row of a package.
func pkgItem(p upmPackage, in installedPackages) menuItem {
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
	return menuItem{id: p.id, label: p.name, right: right, checked: has, installed: has}
}

// groupHeading is the id of a group's heading row.
func groupHeading(g pkgGroup) string { return "group:" + groupTitles[g] }

// pkgItems are the rows of the packages, under a heading per group that has
// any.
func pkgItems(pkgs []upmPackage, in installedPackages) []menuItem {
	items := make([]menuItem, 0, len(pkgs)+len(groupTitles))
	for i, p := range pkgs {
		if i == 0 || pkgs[i-1].group != p.group {
			items = append(items, menuItem{id: groupHeading(p.group), label: groupTitles[p.group], heading: true})
		}
		items = append(items, pkgItem(p, in))
	}
	return items
}

// packageLookupTimeout bounds asking the registry about a package typed in.
const packageLookupTimeout = 15 * time.Second

// packagesPicker is the popup. in is what the project already has; submit
// gets what to add (with pinned versions) and what to remove.
func (m *Model) packagesPicker(title string, in installedPackages, submit func(m *Model, add, remove []string) tea.Cmd) *menuPopup {
	pkgs := offeredPackages(m.cfg.OpenUPM, in)
	byID := map[string]upmPackage{}
	for _, p := range pkgs {
		byID[p.id] = p
	}
	items := pkgItems(pkgs, in)
	lock := lockDependencies(pkgs, in)
	lock(items)
	menu := newMenu(title, items)
	menu.multi = true
	menu.installs = true
	menu.wanted = 64
	follow := tickDeps(pkgs)
	menu.onToggle = func(items []menuItem, id string, on bool) {
		follow(items, id, on)
		lock(items)
	}
	menu.hints = []hintItem{{"Toggle", "<space>"}, {"GitHub", "o"}, {"Add", "<tab>"}, {"Apply", "<enter>"}, {"Cancel", "<esc>"}}
	menu.itemKeys = map[string]func(m *Model, it menuItem) tea.Cmd{
		"o": func(m *Model, it menuItem) tea.Cmd { return m.openPackagePage(byID[it.id]) },
	}
	// tick ticks a package of the list, with what it needs.
	tick := func(id string) {
		items := append([]menuItem(nil), menu.list.Items()...)
		for i := range items {
			if items[i].id == id && items[i].locked == "" {
				items[i].checked = true
			}
		}
		follow(items, id, true)
		lock(items)
		menu.list.SetItems(items)
	}
	// found adds a package the registry has to the end of the list, ticked.
	found := func(p upmPackage) {
		pkgs = append(pkgs, p)
		byID[p.id] = p
		lock, follow = lockDependencies(pkgs, in), tickDeps(pkgs)
		items := append([]menuItem(nil), menu.list.Items()...)
		if len(pkgs) == 1 || pkgs[len(pkgs)-2].group != groupProject {
			items = append(items, menuItem{id: groupHeading(groupProject), label: groupTitles[groupProject], heading: true})
		}
		items = append(items, pkgItem(p, in))
		menu.list.SetItems(items)
		tick(p.id)
	}
	e := &menuEntry{
		label: "Add: ", placeholder: "type a package name, such as com.example.package@1.2.0",
		in:    newInput(""),
		hints: []hintItem{{"Look up", "<enter>"}, {"Back to the list", "<tab>"}, {"Clear", "<esc>"}},
	}
	e.submit = func(m *Model, text string) tea.Cmd {
		if cut := trimAddCommand(text); cut != text {
			text = cut
			e.in.Set(text) // the field shows what is looked up
		}
		if !config.ValidPackage(text) {
			m.toastf(toastWarn, "%q is not a package name, optionally with @version", text)
			return nil
		}
		id, ver := config.ParsePackage(text)
		if p, ok := byID[id]; ok {
			if ver != "" && !in.has(id) {
				p.version = ver
				byID[id] = p
				for i := range pkgs {
					if pkgs[i].id == id {
						pkgs[i] = p
					}
				}
				menu.list.SetItems(replaceItem(menu.list.Items(), pkgItem(p, in)))
			}
			tick(id)
			e.in.Set("")
			m.toastf(toastInfo, "%s is already in the list: ticked", p.name)
			return nil
		}
		e.busy = "Looking up " + id + " in the OpenUPM registry…"
		h := m.host
		lookup := func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), packageLookupTimeout)
			defer cancel()
			info, err := h.LookupPackage(ctx, id)
			return thenMsg{run: func(m *Model) tea.Cmd {
				e.busy = ""
				switch {
				case errors.Is(err, openupm.ErrNoPackage):
					m.toastf(toastWarn, "%s is not in the OpenUPM registry", id)
					return nil
				case err != nil:
					m.toastf(toastError, "Could not look up %s: %v", id, err)
					return nil
				case ver != "" && !info.Versions[ver]:
					m.toastf(toastWarn, "%s has no version %s (the newest is %s)", id, ver, info.Latest)
					return nil
				}
				name := info.DisplayName
				if name == "" {
					name = id
				}
				found(upmPackage{id: id, name: name, version: ver, repo: info.Repo, group: groupProject})
				e.in.Set("")
				m.toastf(toastOK, "%s is in the list: it is added when you apply", name)
				return nil
			}}
		}
		return tea.Batch(m.startSpinner(), lookup)
	}
	menu.entry = e
	// changes is what applying the ticks in picked adds and removes.
	changes := func(picked []menuItem) (add, remove []string) {
		ticked := map[string]menuItem{}
		for _, it := range picked {
			ticked[it.id] = it
		}
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
		return add, remove
	}
	menu.submit = func(m *Model, picked []menuItem) tea.Cmd {
		add, remove := changes(picked)
		return submit(m, add, remove)
	}
	menu.pending = func() string {
		add, remove := changes(menu.picked())
		lines := make([]string, 0, len(add)+len(remove))
		for _, id := range add {
			lines = append(lines, "- adding "+id)
		}
		for _, id := range remove {
			lines = append(lines, "- removing "+id)
		}
		return strings.Join(lines, "\n")
	}

	return menu
}

// trimAddCommand turns a command copied from the OpenUPM site, such as
// "openupm add com.example.package", into the package it adds. Other text is
// returned as it is.
func trimAddCommand(text string) string {
	f := strings.Fields(text)
	if len(f) > 0 && f[0] == "npx" {
		f = f[1:]
	}
	if len(f) < 3 || (f[0] != "openupm" && f[0] != "openupm-cli") || f[1] != "add" {
		return text
	}
	for _, w := range f[2:] {
		if !strings.HasPrefix(w, "-") {
			return w
		}
	}
	return text
}

// replaceItem puts it in place of the item with its id, keeping the tick.
func replaceItem(items []menuItem, it menuItem) []menuItem {
	out := append([]menuItem(nil), items...)
	for i := range out {
		if out[i].id == it.id {
			it.checked, it.locked = out[i].checked, out[i].locked
			out[i] = it
		}
	}
	return out
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
	in, err := readInstalled(r.Path)
	if err != nil {
		m.showError("Reading the packages of "+r.name(), err)
		return nil
	}
	return m.pushProjectPackages(r, exe, in)
}

func (m *Model) pushProjectPackages(r projectRow, exe string, in installedPackages) tea.Cmd {
	menu := m.packagesPicker("OpenUPM packages for "+r.name(), in, func(m *Model, add, remove []string) tea.Cmd {
		if len(add) == 0 && len(remove) == 0 {
			return nil
		}
		return m.changePackages(r, exe, add, remove)
	})
	menu.confirmApply = true // the project changes at once: ask first
	m.push(menu)
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
