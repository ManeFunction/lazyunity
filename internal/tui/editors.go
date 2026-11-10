package tui

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/ManeFunction/lazyunity/internal/launch"
	"github.com/ManeFunction/lazyunity/internal/unity"
	"github.com/ManeFunction/lazyunity/internal/unitycli"
)

func hasEditor(m *Model) bool { _, ok := m.editorsTab.Selected(); return ok }

func editorActions() []*action {
	on := func(f func(m *Model, e editorRow) tea.Cmd) func(*Model) tea.Cmd {
		return func(m *Model) tea.Cmd {
			e, ok := m.editorsTab.Selected()
			if !ok {
				return nil
			}
			return f(m, e)
		}
	}
	return []*action{
		{id: "editors.modules", ctx: ctxEditors, keys: []string{"enter"}, desc: "Show the Editor's modules", hint: "Modules",
			enabled: hasEditor, run: on(func(m *Model, e editorRow) tea.Cmd { return m.drillIntoModules(e) })},
		{id: "editors.addModules", ctx: ctxEditors, keys: []string{"a"}, desc: "Add modules, such as a build platform", hint: "Add module",
			enabled: hasEditor, run: on(func(m *Model, e editorRow) tea.Cmd { return m.addModulesFlow(e, false) })},
		{id: "editors.install", ctx: ctxEditors, keys: []string{"n"}, desc: "Install a new Editor version", hint: "Install",
			run: func(m *Model) tea.Cmd { return m.showReleases() }},
		{id: "editors.uninstall", ctx: ctxEditors, keys: []string{"d"}, desc: "Uninstall the Editor", hint: "Uninstall",
			enabled: hasEditor, run: on(func(m *Model, e editorRow) tea.Cmd { m.uninstallConfirm(e); return nil })},
		{id: "editors.default", ctx: ctxEditors, keys: []string{"space"}, desc: "Make this the default Editor", hint: "Default",
			enabled: hasEditor, run: on(func(m *Model, e editorRow) tea.Cmd { return m.setDefaultEditor(e) })},
		{id: "editors.upgrade", ctx: ctxEditors, keys: []string{"u"}, desc: "Upgrade to the newest patch of this release line",
			enabled: hasEditor, run: on(func(m *Model, e editorRow) tea.Cmd { m.upgradeConfirm(e); return nil })},
		{id: "editors.openFolder", ctx: ctxEditors, keys: []string{"o"}, desc: "Open the Editor's folder",
			enabled: hasEditor, run: on(func(m *Model, e editorRow) tea.Cmd { return m.openFolder(editorDir(e)) })},
		{id: "editors.copy", ctx: ctxEditors, keys: []string{"y"}, desc: "Copy the version or path",
			enabled: hasEditor, run: on(func(m *Model, e editorRow) tea.Cmd { m.copyEditorMenu(e); return nil })},
	}
}

func editorDir(e editorRow) string {
	if e.Dir != "" {
		return e.Dir
	}
	return filepath.Dir(e.Location)
}

// showReleases switches the Editors panel to the Releases tab.
func (m *Model) showReleases() tea.Cmd {
	m.setFocus(panelEditors)
	m.panels[panelEditors].drill = nil
	m.panels[panelEditors].cur = 1
	return m.afterTabSwitch()
}

func (m *Model) setDefaultEditor(e editorRow) tea.Cmd {
	c := m.cli
	return m.runCLI("Setting the default Editor", "Unity "+e.Version+" is now the default",
		func(ctx context.Context, _ *unitycli.Client) error { return c.SetDefaultEditor(ctx, e.Version) },
		func(m *Model) tea.Cmd { return m.loadEditorsCmd() })
}

func (m *Model) copyEditorMenu(e editorRow) {
	item := func(label, value string) menuItem {
		return menuItem{label: label, right: truncate(value, 44), run: func(m *Model) tea.Cmd {
			m.toastf(toastOK, "Copied the %s", strings.ToLower(label))
			return tea.SetClipboard(value)
		}}
	}
	menu := newMenu("Copy", []menuItem{item("Version", e.Version), item("Editor path", e.Location), item("Folder", editorDir(e))})
	menu.wanted = 60
	m.push(menu)
}

// projectsUsing lists the projects that open with an Editor version.
func (m *Model) projectsUsing(version string) []string {
	var names []string
	for _, p := range m.projectsTab.Items() {
		if p.Version == version {
			names = append(names, p.name())
		}
	}
	return names
}

func (m *Model) uninstallConfirm(e editorRow) {
	msg := fmt.Sprintf("Uninstall Unity %s? Its folder and every module in it are deleted.", e.Version)
	if names := m.projectsUsing(e.Version); len(names) > 0 {
		shown := names
		if len(shown) > 5 {
			shown = append(shown[:5:5], fmt.Sprintf("and %d more", len(names)-5))
		}
		msg += fmt.Sprintf("\n\n%s use it: %s.", plural(len(names), "project"), strings.Join(shown, ", "))
	}
	c := m.cli
	m.push(&confirmPopup{
		title: "Uninstall Unity " + e.Version, yes: "Uninstall", danger: true, message: msg,
		onYes: func(m *Model) tea.Cmd {
			if c == nil {
				m.showCLIMissing()
				return nil
			}
			return m.enqueue(&task{
				title: "Uninstalling Unity " + e.Version,
				label: "uninstall " + e.Version,
				works: []string{workEditor(e.Version)},
				do:    func(ctx context.Context) error { return c.Uninstall(ctx, e.Version, e.Arch) },
				after: func(m *Model, err error) tea.Cmd { return m.refreshEditorsCmd() },
			})
		},
	})
}

func (m *Model) upgradeConfirm(e editorRow) {
	c := m.cli
	m.push(&confirmPopup{
		title: "Upgrade Unity " + e.Version, yes: "Upgrade",
		message: fmt.Sprintf("Install the newest patch of Unity %s, carrying this Editor's modules over? "+
			"The new version is installed next to this one; this one stays.", unityLine(e.Version)),
		onYes: func(m *Model) tea.Cmd {
			if c == nil {
				m.showCLIMissing()
				return nil
			}
			return m.enqueue(&task{
				title: "Upgrading Unity " + e.Version,
				label: "upgrade " + e.Version,
				works: []string{workUpgradeEditor(e.Version)},
				open:  func(ctx context.Context) (*unitycli.Stream, error) { return c.UpgradeEditor(ctx, e.Version) },
				after: func(m *Model, err error) tea.Cmd { return m.refreshEditorsCmd() },
			})
		},
	})
}

func unityLine(version string) string {
	if v, ok := unity.ParseVersion(version); ok {
		return v.Line()
	}
	return version
}

// refreshEditorsCmd rescans the disk and asks the CLI again.
func (m *Model) refreshEditorsCmd() tea.Cmd {
	return tea.Batch(m.scanEditorsCmd(), m.loadEditorsCmd(), m.loadProjectsCmd())
}

// ---- modules of one Editor ----------------------------------------------------------

func moduleActions() []*action {
	on := func(f func(m *Model, e editorRow, mod unitycli.Module) tea.Cmd) func(*Model) tea.Cmd {
		return func(m *Model) tea.Cmd {
			mt, e, ok := m.modulesTab()
			if !ok {
				return nil
			}
			mod, ok := mt.Selected()
			if !ok {
				return nil
			}
			return f(m, e, mod)
		}
	}
	return []*action{
		{id: "modules.install", ctx: ctxModules, keys: []string{"space", "a", "enter"}, desc: "Install the module", hint: "Install",
			run: on(func(m *Model, e editorRow, mod unitycli.Module) tea.Cmd {
				if mod.IsInstalled {
					m.toastf(toastInfo, "%s is already installed", mod.Name)
					return nil
				}
				return m.installModules(e, []unitycli.Module{mod})
			})},
		{id: "modules.remove", ctx: ctxModules, keys: []string{"d"}, desc: "Remove the module", hint: "Remove",
			run: on(func(m *Model, e editorRow, mod unitycli.Module) tea.Cmd {
				if !mod.IsInstalled {
					m.toastf(toastInfo, "%s is not installed", mod.Name)
					return nil
				}
				m.removeModuleConfirm(e, mod)
				return nil
			})},
	}
}

// modulesTab is the list shown while drilled into an Editor, and that Editor.
func (m *Model) modulesTab() (*listTab[unitycli.Module], editorRow, bool) {
	p := m.panels[panelEditors]
	if p.drill == nil {
		return nil, editorRow{}, false
	}
	t, ok := p.drill.tab.(*listTab[unitycli.Module])
	return t, p.drill.editor, ok
}

func (m *Model) drillIntoModules(e editorRow) tea.Cmd {
	var cols moduleCols
	t := &listTab[unitycli.Module]{
		list:    newList(func(x unitycli.Module) string { return x.Name + " " + x.Category }, func(x unitycli.Module) string { return x.ID }),
		name:    "Modules",
		context: ctxModules,
		empty:   "Loading modules…",
	}
	t.prep = func(m *Model, w int) { cols = measureModules(t.Items()) }
	t.draw = func(m *Model, x unitycli.Module, w int, base lipgloss.Style) string {
		return m.drawModule(x, w, base, cols)
	}
	h := m.geo.panels[panelEditors].h - 2
	if h > 0 {
		t.setHeight(h)
	}
	p := m.panels[panelEditors]
	p.drill = &drill{title: "Editors > " + e.Version, tab: t, editor: e}
	c := m.cli
	if c == nil {
		t.empty = "The Unity CLI is needed to list modules."
		return nil
	}
	return func() tea.Msg {
		mods, err := c.Modules(context.Background(), e.Version, e.Arch)
		return modulesMsg{version: e.Version, mods: mods, err: err}
	}
}

func (m *Model) onModules(msg modulesMsg) {
	t, e, ok := m.modulesTab()
	if !ok || e.Version != msg.version {
		return
	}
	if msg.err != nil {
		t.empty = "Could not list the modules: " + msg.err.Error()
		t.SetItems(nil)
		return
	}
	sortModules(msg.mods)
	t.empty = "This Editor has no modules."
	t.SetItems(msg.mods)
}

// reloadModulesCmd refreshes the open modules list.
func (m *Model) reloadModulesCmd() tea.Cmd {
	_, e, ok := m.modulesTab()
	if !ok || m.cli == nil {
		return nil
	}
	c := m.cli
	return func() tea.Msg {
		mods, err := c.Modules(context.Background(), e.Version, e.Arch)
		return modulesMsg{version: e.Version, mods: mods, err: err}
	}
}

func (m *Model) removeModuleConfirm(e editorRow, mod unitycli.Module) {
	c := m.cli
	m.push(&confirmPopup{
		title: "Remove " + mod.Name, yes: "Remove", danger: true,
		message: fmt.Sprintf("Remove %s from Unity %s? Projects set to build for it will need it installed again.", mod.Name, e.Version),
		onYes: func(m *Model) tea.Cmd {
			return m.enqueue(&task{
				title: fmt.Sprintf("Removing %s from Unity %s", mod.Name, e.Version),
				label: e.Version + ": remove " + mod.Name,
				works: []string{workModule(e.Version, mod.ID)},
				do:    func(ctx context.Context) error { return c.RemoveModules(ctx, e.Version, e.Arch, []string{mod.ID}) },
				after: func(m *Model, err error) tea.Cmd { return tea.Batch(m.scanEditorsCmd(), m.reloadModulesCmd()) },
			})
		},
	})
}

// installModules adds modules to an installed Editor, as a task.
func (m *Model) installModules(e editorRow, mods []unitycli.Module) tea.Cmd {
	c := m.cli
	if c == nil {
		m.showCLIMissing()
		return nil
	}
	var ids, names, works, busy []string
	for _, x := range mods {
		if m.taskOn(workModule(e.Version, x.ID)) != nil {
			busy = append(busy, x.Name)
			continue
		}
		ids, names = append(ids, x.ID), append(names, x.Name)
		works = append(works, workModule(e.Version, x.ID))
	}
	if len(ids) == 0 {
		m.toastf(toastInfo, "Already in the queue: %s for Unity %s", joinNames(busy), e.Version)
		return nil
	}
	if len(busy) > 0 {
		m.toastf(toastInfo, "Skipped %s: already in the queue", joinNames(busy))
	}
	return m.enqueue(&task{
		title: fmt.Sprintf("Adding %s to Unity %s", joinNames(names), e.Version),
		label: e.Version + ": " + joinNames(names),
		works: works,
		open: func(ctx context.Context) (*unitycli.Stream, error) {
			return c.InstallModules(ctx, e.Version, e.Arch, ids)
		},
		after: func(m *Model, err error) tea.Cmd { return tea.Batch(m.scanEditorsCmd(), m.reloadModulesCmd()) },
	})
}

func joinNames(names []string) string {
	switch len(names) {
	case 0:
		return "nothing"
	case 1, 2:
		return strings.Join(names, " and ")
	}
	return fmt.Sprintf("%s and %d more", names[0], len(names)-1)
}

// ---- adding modules ------------------------------------------------------------------

type addModulesMsg struct {
	ed            editorRow
	mods          []unitycli.Module
	platformsOnly bool
	err           error
}

// addModulesFlow lists what an Editor can still take and lets the user tick
// modules to install. With platformsOnly it offers build platforms only.
func (m *Model) addModulesFlow(e editorRow, platformsOnly bool) tea.Cmd {
	c := m.cli
	if c == nil {
		m.showCLIMissing()
		return nil
	}
	m.toastf(toastInfo, "Looking up the modules of Unity %s…", e.Version)
	return func() tea.Msg {
		mods, err := c.Modules(context.Background(), e.Version, e.Arch)
		return addModulesMsg{ed: e, mods: mods, platformsOnly: platformsOnly, err: err}
	}
}

func (m *Model) onAddModules(msg addModulesMsg) {
	if msg.err != nil {
		m.showError("Listing modules", msg.err)
		return
	}
	var avail []unitycli.Module
	queued := 0
	for _, x := range msg.mods {
		if x.IsInstalled || (msg.platformsOnly && x.Category != "Platforms") {
			continue
		}
		if m.taskOn(workModule(msg.ed.Version, x.ID)) != nil {
			queued++
			continue
		}
		avail = append(avail, x)
	}
	if len(avail) == 0 && queued > 0 {
		m.toastf(toastInfo, "The rest of Unity %s's modules are already in the queue", msg.ed.Version)
		return
	}
	if len(avail) == 0 {
		what := "module"
		if msg.platformsOnly {
			what = "platform"
		}
		m.toastf(toastInfo, "Every %s of Unity %s is already installed", what, msg.ed.Version)
		return
	}
	sortModules(avail)
	m.push(m.modulePicker("Add modules to Unity "+msg.ed.Version, avail, func(m *Model, picked []unitycli.Module) tea.Cmd {
		if len(picked) == 0 {
			return nil
		}
		return m.installModules(msg.ed, picked)
	}))
}

// modulePicker is a multi-select menu of modules.
func (m *Model) modulePicker(title string, mods []unitycli.Module, submit func(m *Model, picked []unitycli.Module) tea.Cmd) *menuPopup {
	byID := map[string]unitycli.Module{}
	var items []menuItem
	for _, x := range mods {
		byID[x.ID] = x
		items = append(items, menuItem{id: x.ID, label: x.Name, right: humanBytes(x.DownloadBytes)})
	}
	menu := newMenu(title, items)
	menu.multi = true
	menu.wanted = 52
	menu.hints = []hintItem{{"Toggle", "<space>"}, {"Confirm", "<enter>"}, {"Cancel", "<esc>"}}
	menu.submit = func(m *Model, picked []menuItem) tea.Cmd {
		out := make([]unitycli.Module, 0, len(picked))
		for _, p := range picked {
			out = append(out, byID[p.id])
		}
		return submit(m, out)
	}
	return menu
}

// ---- installing a new Editor ------------------------------------------------------------

type releaseModulesMsg struct {
	version, changeset string
	mods               []unitycli.Module
	err                error
}

type planMsg struct {
	opts unitycli.InstallOptions
	plan unitycli.InstallPlan
	err  error
}

// installEditorFlow installs an Editor: pick modules, see the size, confirm.
func (m *Model) installEditorFlow(version, changeset string) tea.Cmd {
	c := m.cli
	if c == nil {
		m.showCLIMissing()
		return nil
	}
	if m.editorInstalled(version) {
		m.toastf(toastInfo, "Unity %s is already installed", version)
		return nil
	}
	if t := m.taskOn(workEditor(version)); t != nil {
		m.toastf(toastInfo, "Already in the queue: %s", t.title)
		return nil
	}
	m.toastf(toastInfo, "Looking up Unity %s…", version)
	return func() tea.Msg {
		mods, err := c.ReleaseModules(context.Background(), version)
		return releaseModulesMsg{version: version, changeset: changeset, mods: mods, err: err}
	}
}

func (m *Model) onReleaseModules(msg releaseModulesMsg) tea.Cmd {
	if msg.err != nil {
		m.showError("Looking up Unity "+msg.version, msg.err)
		return nil
	}
	mods := msg.mods
	sortModules(mods)
	pick := func(m *Model, picked []unitycli.Module) tea.Cmd {
		ids := make([]string, len(picked))
		for i, x := range picked {
			ids[i] = x.ID
		}
		return m.planInstall(unitycli.InstallOptions{
			Version: msg.version, Changeset: msg.changeset, Modules: ids,
			SetDefault: len(m.editors) == 0,
		})
	}
	if len(mods) == 0 {
		return pick(m, nil) // nothing to choose from: go straight to the install
	}
	m.push(m.modulePicker("Install Unity "+msg.version+": choose modules", mods, pick))
	return nil
}

// planInstall asks the CLI what the install would download.
func (m *Model) planInstall(o unitycli.InstallOptions) tea.Cmd {
	c := m.cli
	return func() tea.Msg {
		plan, err := c.PlanInstall(context.Background(), o)
		return planMsg{opts: o, plan: plan, err: err}
	}
}

func (m *Model) onPlan(msg planMsg) {
	if msg.err != nil {
		m.showError("Installing Unity "+msg.opts.Version, msg.err)
		return
	}
	if msg.plan.AlreadyInstalled {
		m.toastf(toastInfo, "Unity %s is already installed", msg.opts.Version)
		return
	}
	var names []string
	for _, x := range msg.plan.Modules {
		names = append(names, x.Name)
	}
	text := fmt.Sprintf("Install Unity %s", msg.opts.Version)
	if len(names) > 0 {
		text += " with " + strings.Join(names, ", ")
	}
	text += fmt.Sprintf(".\n\nDownload: %s.\n\nInstalling accepts Unity's license terms for the Editor and each selected component.",
		humanBytes(msg.plan.TotalDownloadSize))
	c := m.cli
	opts := msg.opts
	m.push(&confirmPopup{
		title: "Install Unity " + opts.Version, yes: "Install", message: text,
		onYes: func(m *Model) tea.Cmd {
			title := "Installing Unity " + opts.Version
			if len(names) > 0 {
				title += " + " + joinNames(names)
			}
			works := []string{workEditor(opts.Version)}
			for _, id := range opts.Modules {
				works = append(works, workModule(opts.Version, id))
			}
			return m.enqueue(&task{
				title: title,
				label: opts.Version,
				works: works,
				open:  func(ctx context.Context) (*unitycli.Stream, error) { return c.Install(ctx, opts) },
				after: func(m *Model, err error) tea.Cmd {
					if err == nil {
						m.toastf(toastOK, "Installed Unity %s", opts.Version)
					}
					return m.refreshEditorsCmd()
				},
			})
		},
	})
}

// ---- releases ------------------------------------------------------------------------------

func releaseActions() []*action {
	on := func(f func(m *Model, r unitycli.Release) tea.Cmd) func(*Model) tea.Cmd {
		return func(m *Model) tea.Cmd {
			r, ok := m.releasesTab.Selected()
			if !ok {
				return nil
			}
			return f(m, r)
		}
	}
	has := func(m *Model) bool { _, ok := m.releasesTab.Selected(); return ok }
	return []*action{
		{id: "releases.install", ctx: ctxReleases, keys: []string{"enter", "space"}, desc: "Install this version", hint: "Install",
			enabled: has, run: on(func(m *Model, r unitycli.Release) tea.Cmd {
				if r.Installed {
					m.toastf(toastInfo, "Unity %s is already installed", r.Version)
					return nil
				}
				return m.installEditorFlow(r.Version, "")
			})},
		{id: "releases.stream", ctx: ctxReleases, keys: []string{"ctrl+s"}, desc: "Filter by release stream (Supported, LTS, Beta, Alpha)", hint: "Stream",
			run: func(m *Model) tea.Cmd { m.streamMenu(); return nil }},
		{id: "releases.notes", ctx: ctxReleases, keys: []string{"o"}, desc: "Open the release notes in the browser",
			enabled: has, run: on(func(m *Model, r unitycli.Release) tea.Cmd {
				if err := launch.OpenURL(m.goos, "https://unity.com/releases/editor/whats-new/"+r.Version); err != nil {
					m.toastf(toastError, "Could not open the browser: %v", err)
				}
				return nil
			})},
	}
}

func (m *Model) streamMenu() {
	cur := m.releases.stream
	pick := func(stream string) func(*Model) tea.Cmd {
		return func(m *Model) tea.Cmd {
			m.releases = releasesState{stream: stream, gen: m.releases.gen + 1}
			m.st.SetStream(stream)
			m.saveState()
			m.releasesTab.SetItems(nil)
			m.releasesTab.empty = "Loading releases…"
			return m.loadReleasesCmd(true)
		}
	}
	mark := func(s string) string {
		if s == cur {
			return "current"
		}
		return ""
	}
	var items []menuItem
	for _, s := range unitycli.ReleaseStreams {
		items = append(items, menuItem{id: s.ID, label: s.Label, desc: s.Desc, right: mark(s.ID), run: pick(s.ID)})
	}
	menu := newMenu("Release stream", items)
	menu.wanted = 36
	m.push(menu)
}

// ---- the download cache ----------------------------------------------------------

type cacheInfoMsg struct {
	info unitycli.CacheInfo
	err  error
}

// cleanCacheAction is the C key of the Editors panel, in one of its contexts.
func cleanCacheAction(c ctxID) *action {
	return &action{
		id: c.group() + ".cleanCache", ctx: c, keys: []string{"C"},
		desc: "Clean the download cache (what interrupted installs left behind)",
		run:  func(m *Model) tea.Cmd { return m.cleanCacheFlow() },
	}
}

// cleanCacheFlow asks the CLI how big the download cache is, to confirm with.
func (m *Model) cleanCacheFlow() tea.Cmd {
	c := m.cli
	if c == nil {
		m.showCLIMissing()
		return nil
	}
	if m.busy() {
		// the running install downloads into the cache
		m.toastf(toastWarn, "Wait for the tasks to finish before cleaning the download cache")
		return nil
	}
	return func() tea.Msg {
		info, err := c.DownloadCache(context.Background())
		return cacheInfoMsg{info: info, err: err}
	}
}

func (m *Model) onCacheInfo(msg cacheInfoMsg) {
	if msg.err != nil {
		m.showError("Reading the download cache", msg.err)
		return
	}
	info := msg.info
	if info.FileCount == 0 && info.SizeBytes == 0 {
		m.toastf(toastInfo, "The download cache is empty")
		return
	}
	files := "files"
	if info.FileCount == 1 {
		files = "file"
	}
	c := m.cli
	m.push(&confirmPopup{
		title: "Clean download cache", yes: "Clean", danger: true,
		message: fmt.Sprintf("Delete everything in the download cache: %d %s, %s.\n\n%s\n\nThese are installers and partial downloads. Installed Editors are not touched.",
			info.FileCount, files, humanBytes(info.SizeBytes), info.Path),
		onYes: func(m *Model) tea.Cmd {
			return m.runCLI("Cleaning the download cache", "Freed "+humanBytes(info.SizeBytes),
				func(ctx context.Context, _ *unitycli.Client) error { return c.CleanDownloadCache(ctx) }, nil)
		},
	})
}
