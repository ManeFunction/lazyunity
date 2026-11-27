package tui

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/ManeFunction/lazyunity/internal/config"
	"github.com/ManeFunction/lazyunity/internal/state"
	"github.com/ManeFunction/lazyunity/internal/unity"
	"github.com/ManeFunction/lazyunity/internal/unitycli"
)

func projectActions() []*action {
	on := onSelected(func(m *Model) (projectRow, bool) { return m.projectsTab.Selected() })
	return []*action{
		on(&action{id: "projects.open", ctx: ctxProjects, keys: []string{"enter", "space"}, desc: "Open in Unity", hint: "Open"},
			func(m *Model, r projectRow) tea.Cmd { return m.openProject(r) }),
		on(&action{id: "projects.version", ctx: ctxProjects, keys: []string{"v"}, desc: "Change the Unity version", hint: "Version"},
			func(m *Model, r projectRow) tea.Cmd { m.versionMenu(r, nil); return nil }),
		on(&action{id: "projects.platform", ctx: ctxProjects, keys: []string{"p"}, desc: "Change the build platform", hint: "Platform"},
			func(m *Model, r projectRow) tea.Cmd { m.platformMenu(r, nil); return nil }),
		on(&action{id: "projects.pin", ctx: ctxProjects, keys: []string{"f"}, desc: "Pin or unpin the project", hint: "Pin"},
			func(m *Model, r projectRow) tea.Cmd { return m.togglePin(r) }),
		{id: "projects.new", ctx: ctxProjects, keys: []string{"n"}, desc: "Create a new project", hint: "New",
			run: func(m *Model) tea.Cmd { return m.startNewProject() }},
		{id: "projects.add", ctx: ctxProjects, keys: []string{"a"}, desc: "Add an existing project", hint: "Add",
			run: func(m *Model) tea.Cmd { m.addProjectPrompt(); return nil }},
		on(&action{id: "projects.remove", ctx: ctxProjects, keys: []string{"d"}, desc: "Remove the project from the list, or delete it", hint: "Remove"},
			func(m *Model, r projectRow) tea.Cmd { m.removeMenu(r); return nil }),
		{id: "projects.sort", ctx: ctxProjects, keys: []string{"s"}, desc: "Sort the list",
			run: func(m *Model) tea.Cmd { m.sortMenu(); return nil }},
		on(&action{id: "projects.openFolder", ctx: ctxProjects, keys: []string{"o"}, desc: "Open the project folder in the file manager"},
			func(m *Model, r projectRow) tea.Cmd { return m.openFolder(r.Path) }),
		on(&action{id: "projects.packages", ctx: ctxProjects, keys: []string{"P"}, desc: "Add or remove OpenUPM packages (needs the OpenUPM CLI)",
			enabled: func(m *Model) bool { return m.openupm.path != "" }},
			func(m *Model, r projectRow) tea.Cmd { return m.projectPackages(r) }),
		on(&action{id: "projects.edit", ctx: ctxProjects, keys: []string{"e"}, desc: "Edit the project in a code editor"},
			func(m *Model, r projectRow) tea.Cmd { return m.editProject(r) }),
		on(&action{id: "projects.info", ctx: ctxProjects, keys: []string{"i"}, desc: "Show the project's details"},
			func(m *Model, r projectRow) tea.Cmd { return m.projectInfoCmd(r) }),
		on(&action{id: "projects.copy", ctx: ctxProjects, keys: []string{"y"}, desc: "Copy the path, name or version"},
			func(m *Model, r projectRow) tea.Cmd { m.copyMenu(r); return nil }),
		on(&action{id: "projects.clean", ctx: ctxProjects, keys: []string{"C"}, desc: "Clean the project's caches (Library, Temp ...)"},
			func(m *Model, r projectRow) tea.Cmd { m.cleanConfirm(r); return nil }),
	}
}

// ---- state helpers -------------------------------------------------------------

func (m *Model) saveState() {
	if err := m.st.Save(); err != nil {
		m.toastf(toastError, "Could not save state: %v", err)
	}
}

// forgetPending drops the version or platform chosen for a project and saves
// that, when something was chosen.
func (m *Model) forgetPending(path string) { m.settlePending(path, state.Pending{}) }

// settlePending leaves what is still pending for a project after a change was
// applied, and saves it when that changed anything.
func (m *Model) settlePending(path string, left state.Pending) {
	if m.st.PendingFor(path) == left {
		return
	}
	m.st.SetPending(path, left)
	m.saveState()
}

// editorFor finds the installed Editor of a version.
func (m *Model) editorFor(version string) *editorRow { return m.inv.find(version) }

func (r projectRow) hasPlatform(ed *editorRow, buildTarget string) bool {
	p, ok := unity.PlatformByBuildTarget(buildTarget)
	if !ok || ed == nil {
		return false
	}
	for _, q := range ed.Platforms {
		if q.ID == p.ID {
			return true
		}
	}
	return false
}

// ---- opening ----------------------------------------------------------------------

// planOpen decides how a project opens.
func (m *Model) planOpen(r projectRow) openPlan {
	return planOpen(r, m.editorFor(r.effVersion()), m.cli != nil)
}

func (m *Model) openProject(r projectRow) tea.Cmd {
	p := m.planOpen(r)
	switch p.route {
	case openMissing:
		m.toastf(toastError, "The folder of %s is gone: %s", r.name(), displayPath(r.Path))
	case openInstall:
		m.push(&confirmPopup{
			title: "Editor not installed", yes: "Install",
			message: fmt.Sprintf("%s needs Unity %s, which is not installed. Install it now?", r.name(), p.version),
			onYes:   func(m *Model) tea.Cmd { return m.installEditorFlow(p.version, p.changeset) },
			extra: []confirmKey{{key: "v", label: "Pick an installed version", run: func(m *Model) tea.Cmd {
				m.openWithInstalled(r)
				return nil
			}}},
		})
	case openDirect:
		m.push(&confirmPopup{
			title: "Open without the Unity CLI", yes: "Open anyway",
			message: p.why + " Opening the project directly works, but the Editor starts signed out, and Unity may start the official Hub to sign in.",
			onYes:   func(m *Model) tea.Cmd { return m.directLaunch(r, p) },
		})
	case openUpgrade:
		m.upgradeMenu(r, p)
	case openConfirm:
		m.push(&confirmPopup{
			title: "Open with another version", yes: "Open",
			message: fmt.Sprintf("Open %s with Unity %s? The project was made with %s, so Unity will ask to change it.", r.name(), p.version, r.Version),
			onYes:   func(m *Model) tea.Cmd { return m.runOpen(r, p) },
		})
	default:
		return m.runOpen(r, p)
	}
	return nil
}

// directLaunch starts the plan's Editor on the project, without the CLI.
func (m *Model) directLaunch(r projectRow, p openPlan) tea.Cmd {
	if err := m.host.StartEditor(p.editor.Location, r.Path, p.buildTarget); err != nil {
		m.showError("Opening "+r.name(), err)
		return nil
	}
	return m.afterOpened(r, p, "Opening %s in Unity %s (signed out)")
}

type openedMsg struct {
	row  projectRow
	plan openPlan
	res  unitycli.OpenResult
	err  error
}

// runOpen opens the project through the CLI, which signs the Editor in.
func (m *Model) runOpen(r projectRow, p openPlan) tea.Cmd {
	return m.withCLI(func(c unityCLI) tea.Cmd {
		m.toastf(toastInfo, "Opening %s…", r.name())
		return func() tea.Msg {
			res, err := c.Open(context.Background(), p.cli)
			return openedMsg{row: r, plan: p, res: res, err: err}
		}
	})
}

func (m *Model) onOpened(msg openedMsg) tea.Cmd {
	r, p := msg.row, msg.plan
	if msg.err != nil {
		var e *unitycli.Error
		text := msg.err.Error()
		if errors.As(msg.err, &e) && e.Output != "" {
			text += "\n\n" + e.Output
		}
		pop := &confirmPopup{title: "Could not open " + r.name(), message: text}
		if p.editor != nil {
			pop.yes = "Open directly"
			pop.message += "\n\nOpen it directly instead? The Editor starts signed out."
			pop.onYes = func(m *Model) tea.Cmd { return m.directLaunch(r, p) }
		} else {
			pop.yes = "OK"
			pop.onYes = func(*Model) tea.Cmd { return nil }
		}
		m.push(pop)
		return nil
	}
	if msg.res.AlreadyOpen {
		m.toastf(toastInfo, "%s is already open", r.name())
		return nil
	}
	return m.afterOpened(r, p, "Opened %s in Unity %s")
}

// afterOpened keeps what is still pending after the project opened, and
// refreshes.
func (m *Model) afterOpened(r projectRow, p openPlan, format string) tea.Cmd {
	m.settlePending(r.Path, p.after)
	m.toastf(toastOK, format, r.name(), p.version)
	if m.cfg.QuitAfterOpen {
		m.quit = true
	}
	return m.loadProjectsCmd()
}

// upgradeMenu offers to upgrade a project to Unity 6.7 or later, which `open`
// refuses to do by itself.
func (m *Model) upgradeMenu(r projectRow, p openPlan) {
	run := func(cloud unitycli.CloudChoice) func(*Model) tea.Cmd {
		return func(m *Model) tea.Cmd {
			return m.withCLI(func(c unityCLI) tea.Cmd {
				return m.enqueue(&task{
					title: fmt.Sprintf("Upgrading %s to %s", r.name(), p.version),
					label: r.name() + " → " + p.version,
					works: []string{workProject(r.Path)},
					msg:   "Unity is upgrading the project; it opens when ready",
					do:    func(ctx context.Context) error { return c.UpgradeProject(ctx, r.Path, p.version, cloud) },
					after: func(m *Model, err error) tea.Cmd {
						if err != nil {
							return nil
						}
						m.settlePending(r.Path, p.after)
						return m.loadProjectsCmd()
					},
				})
			})
		}
	}
	menu := newMenu(fmt.Sprintf("Upgrade %s to Unity %s", r.name(), p.version), []menuItem{
		{label: "Upgrade without Unity Cloud", run: run(unitycli.CloudOff),
			desc: "Unity 6.7 and later projects are tied to a Unity Cloud project. This upgrades without linking one."},
		{label: "Upgrade and link a new Unity Cloud project", run: run(unitycli.CloudNew),
			desc: "Creates a Unity Cloud project for it, under your account's default organisation."},
	})
	menu.wanted = 52
	m.push(menu)
}

// openWithInstalled opens a project whose Editor is not installed with one
// that is: it asks for the version, then the platform, then opens it.
func (m *Model) openWithInstalled(r projectRow) {
	if len(m.inv.editors()) == 0 {
		m.toastf(toastWarn, "No Unity Editor is installed")
		return
	}
	m.versionMenu(r, func(m *Model, r projectRow) tea.Cmd {
		m.platformMenu(r, func(m *Model, r projectRow) tea.Cmd { return m.openProject(r) })
		return nil
	})
}

// projectByPath is the project row of a path, as it is now.
func (m *Model) projectByPath(path string) (projectRow, bool) {
	for _, r := range m.projectsTab.list.Items() {
		if r.Path == path {
			return r, true
		}
	}
	return projectRow{}, false
}

// thenWith runs then, when set, on the project's row as it is now.
func (m *Model) thenWith(path string, then func(*Model, projectRow) tea.Cmd) tea.Cmd {
	if then == nil {
		return nil
	}
	if r, ok := m.projectByPath(path); ok {
		return then(m, r)
	}
	return nil
}

// ---- version and platform -----------------------------------------------------------

// versionMenu picks the Unity version a project opens with. With then set,
// it offers only the installed versions and calls then with the project once
// one is picked.
func (m *Model) versionMenu(r projectRow, then func(*Model, projectRow) tea.Cmd) {
	var items []menuItem
	if then == nil && !m.editorInstalled(r.Version) {
		v, cs := r.Version, r.Changeset
		items = append(items, menuItem{
			label: "Install " + v + "…", right: "the project's own version",
			run: func(m *Model) tea.Cmd { return m.installEditorFlow(v, cs) },
		})
	}
	for _, e := range m.inv.editors() {
		tags := []string{}
		if e.Version == r.Version {
			tags = append(tags, "project's version")
		}
		if e.Version == r.effVersion() && r.versionChanged() {
			tags = append(tags, "selected")
		}
		if e.Default {
			tags = append(tags, "default")
		}
		v := e.Version
		items = append(items, menuItem{
			label: e.Version, right: strings.Join(tags, ", "),
			run: func(m *Model) tea.Cmd { m.setPendingVersion(r, v); return m.thenWith(r.Path, then) },
		})
	}
	if then == nil {
		items = append(items, menuItem{label: "Install another version…", run: (*Model).showReleases})
	}
	menu := newMenu("Unity version for "+r.name(), items)
	menu.wanted = 48
	// start on the version in use
	menu.list.SelectKey(r.effVersion())
	m.push(menu)
}

func (m *Model) setPendingVersion(r projectRow, v string) {
	p := r.Pending
	if v == r.Version {
		p.Version = ""
	} else {
		p.Version = v
	}
	eff := r.Version
	if p.Version != "" {
		eff = p.Version
	}
	// a platform the new Editor does not have cannot be switched to
	if p.BuildTarget != "" && !r.hasPlatform(m.editorFor(eff), p.BuildTarget) {
		p.BuildTarget = ""
		m.toastf(toastWarn, "Unity %s does not have that platform: platform choice cleared", eff)
	}
	m.commitPending(r.Path, p)
}

// platformMenu picks the platform a project switches to when it opens. With
// then set, it calls then with the project once one is picked.
func (m *Model) platformMenu(r projectRow, then func(*Model, projectRow) tea.Cmd) {
	ver := r.effVersion()
	ed := m.editorFor(ver)
	if ed == nil {
		m.toastf(toastWarn, "Unity %s is not installed: install it first (v)", ver)
		return
	}
	var items []menuItem
	for _, p := range ed.Platforms {
		tags := []string{}
		if p.BuildTarget == r.BuildTarget {
			tags = append(tags, "current")
		}
		if r.targetChanged() && p.BuildTarget == r.Pending.BuildTarget {
			tags = append(tags, "selected")
		}
		target := p.BuildTarget
		items = append(items, menuItem{
			id: p.BuildTarget, label: p.Label, right: strings.Join(tags, ", "),
			run: func(m *Model) tea.Cmd { m.setPendingTarget(r, target); return m.thenWith(r.Path, then) },
		})
	}
	edCopy := *ed
	items = append(items, menuItem{label: "Add a platform module…", run: func(m *Model) tea.Cmd { return m.addModulesFlow(edCopy, true) }})
	menu := newMenu(fmt.Sprintf("Platform for %s (Unity %s)", r.name(), ver), items)
	menu.wanted = 48
	menu.list.SelectKey(r.effTarget())
	m.push(menu)
}

func (m *Model) setPendingTarget(r projectRow, target string) {
	p := r.Pending
	if target == r.BuildTarget {
		p.BuildTarget = ""
	} else {
		p.BuildTarget = target
	}
	m.commitPending(r.Path, p)
}

// commitPending saves what a project will change to when it next opens, and
// redraws the list with the project still selected.
func (m *Model) commitPending(path string, p state.Pending) {
	m.st.SetPending(path, p)
	m.saveState()
	m.refreshProjects()
	m.projectsTab.SelectKey(path)
}

// ---- pin, sort, remove --------------------------------------------------------------

func (m *Model) togglePin(r projectRow) tea.Cmd {
	pin := !r.IsFavorite
	ok := "Pinned " + r.name()
	if !pin {
		ok = "Unpinned " + r.name()
	}
	return m.runCLI("Pinning "+r.name(), ok,
		func(ctx context.Context, c unityCLI) error { return c.SetPinned(ctx, r.Path, pin) },
		func(m *Model) tea.Cmd { m.selectAfterLoad = r.Path; return m.loadProjectsCmd() })
}

func (m *Model) sortMenu() {
	choices := []choice{
		{id: state.SortLastOpened, label: "Last opened"},
		{id: state.SortName, label: "Name"},
		{id: state.SortVersion, label: "Unity version"},
	}
	m.choiceMenu("Sort projects", choices, m.st.SortOrder(), func(m *Model, order string) tea.Cmd {
		m.st.Sort = order
		m.saveState()
		sel, _ := m.projectsTab.Selected()
		m.refreshProjects()
		m.projectsTab.SelectKey(sel.Path)
		return nil
	})
}

func (m *Model) removeMenu(r projectRow) {
	menu := newMenu("Remove "+r.name(), []menuItem{
		{
			label: "Remove from the list", desc: "The folder and its files stay on disk. You can add the project again later.",
			run: func(m *Model) tea.Cmd {
				return m.runCLI("Removing "+r.name(), "Removed "+r.name()+" from the list",
					func(ctx context.Context, c unityCLI) error { return c.RemoveProjects(ctx, r.Path) },
					func(m *Model) tea.Cmd {
						m.forgetPending(r.Path)
						return m.loadProjectsCmd()
					})
			},
		},
		{
			label: "Delete the folder from disk…", desc: "Deletes the project's folder and everything in it. This cannot be undone.",
			run: func(m *Model) tea.Cmd { m.deleteProjectFlow(r); return nil },
		},
	})
	menu.wanted = 46
	m.push(menu)
}

// safeToDelete reports why a folder must not be deleted as a project, or "".
func safeToDelete(path string) string {
	if !filepath.IsAbs(path) || path != filepath.Clean(path) {
		return "the path is not absolute and clean"
	}
	if path == filepath.Dir(path) || path == homeDir() {
		return "that is not a project folder"
	}
	if _, err := unity.ReadProjectVersion(path); err != nil {
		return "it has no ProjectSettings/ProjectVersion.txt, so it does not look like a Unity project"
	}
	return ""
}

func (m *Model) deleteProjectFlow(r projectRow) {
	if m.refuseFake("deleting projects") { // a made-up path may be a real folder
		return
	}
	if why := safeToDelete(r.Path); why != "" {
		m.toastf(toastError, "Not deleting %s: %s", displayPath(r.Path), why)
		return
	}
	m.push(&confirmPopup{
		title: "Delete project", yes: "Continue", danger: true,
		message: fmt.Sprintf("Delete the folder of %s from disk?\n\n%s", r.name(), r.Path),
		onYes: func(m *Model) tea.Cmd {
			m.push(&confirmPopup{
				title: "Delete project for good", yes: "Delete forever", danger: true,
				message: fmt.Sprintf("This permanently deletes %s and everything in it. There is no undo and no trash.", displayPath(r.Path)),
				onYes: func(m *Model) tea.Cmd {
					c := m.cli
					return m.enqueue(&task{
						title: "Deleting " + r.name(),
						label: "delete " + r.name(),
						works: []string{workProject(r.Path)},
						// os.RemoveAll cannot be stopped halfway: a cancel or
						// quit would leave a partly deleted project behind.
						keep: true,
						do: func(ctx context.Context) error {
							if err := os.RemoveAll(r.Path); err != nil {
								return err
							}
							if c != nil {
								return c.RemoveProjects(ctx, r.Path)
							}
							return nil
						},
						after: func(m *Model, err error) tea.Cmd {
							if err == nil {
								m.forgetPending(r.Path)
							}
							return m.loadProjectsCmd()
						},
					})
				},
			})
			return nil
		},
	})
}

// ---- adding -----------------------------------------------------------------------------

func (m *Model) addProjectPrompt() {
	m.push(&promptPopup{
		title: "Add an existing project", label: "Folder of the Unity project:",
		in:       newInput(displayPath(m.projectsDir()) + string(filepath.Separator)),
		complete: completeDir,
		validate: func(text string) string {
			if why := validFolder(text); why != "" {
				return why
			}
			if _, err := unity.ReadProjectVersion(config.ExpandHome(text)); err != nil {
				return "Not a Unity project: no ProjectSettings/ProjectVersion.txt"
			}
			return ""
		},
		onSubmit: func(m *Model, text string) tea.Cmd {
			path := filepath.Clean(config.ExpandHome(text))
			return m.runCLI("Adding the project", "Added "+filepath.Base(path),
				func(ctx context.Context, c unityCLI) error { return c.AddProjects(ctx, path) },
				func(m *Model) tea.Cmd { m.selectAfterLoad = path; return m.loadProjectsCmd() })
		},
	})
}

// ---- folders, editors, info, clipboard, clean -----------------------------------------

func (m *Model) openFolder(dir string) tea.Cmd {
	if err := m.host.OpenFolder(m.cfg.OS.OpenFolder, dir); err != nil {
		m.toastf(toastError, "Could not open the folder: %v", err)
	}
	return nil
}

// editProject opens the project in the code editor.
func (m *Model) editProject(r projectRow) tea.Cmd {
	return m.edit(r.Path, r.Path, func(m *Model) {
		m.toastf(toastWarn, "Set os.edit in config.yml, or $EDITOR, to edit projects")
	}, nil)
}

type projectInfoMsg struct {
	row  projectRow
	info unitycli.ProjectInfo
	err  error
}

func (m *Model) projectInfoCmd(r projectRow) tea.Cmd {
	return m.withCLI(func(c unityCLI) tea.Cmd {
		m.toastf(toastInfo, "Reading %s…", r.name())
		return func() tea.Msg {
			info, err := c.ProjectInfo(context.Background(), r.Path)
			return projectInfoMsg{row: r, info: info, err: err}
		}
	})
}

func (m *Model) onProjectInfo(msg projectInfoMsg) {
	if msg.err != nil {
		m.showError("Project details", msg.err)
		return
	}
	r, i := msg.row, msg.info
	line := func(k, v string) string { return fmt.Sprintf("%-18s %s", k, v) }
	lines := []string{
		line("Name", r.name()),
		line("Path", r.Path),
		line("Unity version", i.Version+changesetSuffix(i.Changeset)),
		line("Platform", unity.PlatformLabel(i.BuildTarget)),
		line("Render pipeline", orDash(i.RenderPipeline)),
		line("Unity Cloud", yesNo(i.CloudEnabled)),
	}
	if t := r.LastOpened(); !t.IsZero() {
		lines = append(lines, line("Last opened", t.Local().Format("2006-01-02 15:04")+" ("+relTime(m.now(), t)+")"))
	}
	if r.Pending.Version != "" || r.Pending.BuildTarget != "" {
		lines = append(lines, "", "Chosen, applied the next time you open it:")
		if r.versionChanged() {
			lines = append(lines, "  Unity "+r.Pending.Version)
		}
		if r.targetChanged() {
			lines = append(lines, "  platform "+unity.PlatformLabel(r.Pending.BuildTarget))
		}
	}
	if len(i.ScriptingBackend) > 0 {
		var plats []string
		for p := range i.ScriptingBackend {
			plats = append(plats, p)
		}
		sort.Strings(plats)
		lines = append(lines, "", "Scripting backend")
		for _, p := range plats {
			lines = append(lines, "  "+fmt.Sprintf("%-14s %s", p, i.ScriptingBackend[p]))
		}
	}
	var pkgs, modules []string
	for name, ver := range i.Packages {
		if strings.HasPrefix(name, "com.unity.modules.") {
			modules = append(modules, name)
		} else {
			pkgs = append(pkgs, fmt.Sprintf("  %-44s %s", name, ver))
		}
	}
	sort.Strings(pkgs)
	if len(pkgs) > 0 {
		lines = append(lines, "", fmt.Sprintf("Packages (%d, and %d built-in modules)", len(pkgs), len(modules)))
		lines = append(lines, pkgs...)
	}
	m.push(&infoPopup{title: r.name(), lines: lines})
}

func changesetSuffix(cs string) string {
	if cs == "" {
		return ""
	}
	return " (" + cs + ")"
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

func (m *Model) copyMenu(r projectRow) {
	m.copyChoices(56, [][2]string{
		{"Path", r.Path}, {"Name", r.name()}, {"Unity version", r.effVersion()}, {"Folder name", filepath.Base(r.Path)},
	})
}

func (m *Model) cleanConfirm(r projectRow) {
	m.push(&confirmPopup{
		title: "Clean project", yes: "Clean", danger: true,
		message: fmt.Sprintf("Delete the cache folders of %s (Library, Temp, Logs ...)? Your assets stay. Unity rebuilds the Library the next time you open the project, which can take a long time.", r.name()),
		onYes: func(m *Model) tea.Cmd {
			return m.runCLI("Cleaning "+r.name(), "Cleaned "+r.name(),
				func(ctx context.Context, c unityCLI) error { return c.CleanProject(ctx, r.Path) }, nil)
		},
	})
}
