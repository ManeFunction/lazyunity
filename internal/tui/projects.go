package tui

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/ManeFunction/lazyunity/internal/config"
	"github.com/ManeFunction/lazyunity/internal/launch"
	"github.com/ManeFunction/lazyunity/internal/state"
	"github.com/ManeFunction/lazyunity/internal/unity"
	"github.com/ManeFunction/lazyunity/internal/unitycli"
)

func projectActions() []*action {
	on := onSelected(func(m *Model) (projectRow, bool) { return m.projectsTab.Selected() })
	return []*action{
		on(&action{id: "projects.open", ctx: ctxProjects, keys: []string{"enter", "space"}, desc: "Open in Unity, signed in", hint: "Open"},
			func(m *Model, r projectRow) tea.Cmd { return m.openProject(r) }),
		on(&action{id: "projects.version", ctx: ctxProjects, keys: []string{"v"}, desc: "Change the Unity version", hint: "Version"},
			func(m *Model, r projectRow) tea.Cmd { m.versionMenu(r); return nil }),
		on(&action{id: "projects.platform", ctx: ctxProjects, keys: []string{"p"}, desc: "Change the build platform", hint: "Platform"},
			func(m *Model, r projectRow) tea.Cmd { m.platformMenu(r); return nil }),
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
		on(&action{id: "projects.addPackage", ctx: ctxProjects, keys: []string{"ctrl+a"}, desc: "Add an OpenUPM package to the project (needs the OpenUPM CLI)",
			enabled: func(m *Model) bool { return m.openupm.path != "" }},
			func(m *Model, r projectRow) tea.Cmd { return m.addPackagePrompt(r) }),
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
func (m *Model) forgetPending(path string) {
	if m.st.PendingFor(path).Empty() {
		return
	}
	m.st.SetPending(path, state.Pending{})
	m.saveState()
}

// editorFor finds the installed Editor of a version.
func (m *Model) editorFor(version string) *editorRow {
	for i := range m.editors {
		if m.editors[i].Version == version {
			return &m.editors[i]
		}
	}
	return nil
}

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

type openedMsg struct {
	row projectRow
	res unitycli.OpenResult
	err error
}

func (m *Model) openProject(r projectRow) tea.Cmd {
	if r.Missing {
		m.toastf(toastError, "The folder of %s is gone: %s", r.name(), displayPath(r.Path))
		return nil
	}
	ver := r.effVersion()
	ed := m.editorFor(ver)
	if ed == nil {
		changeset := ""
		if ver == r.Version {
			changeset = r.Changeset
		}
		m.push(&confirmPopup{
			title: "Editor not installed", yes: "Install",
			message: fmt.Sprintf("%s needs Unity %s, which is not installed. Install it now?", r.name(), ver),
			onYes:   func(m *Model) tea.Cmd { return m.installEditorFlow(ver, changeset) },
		})
		return nil
	}
	switch {
	case !unity.AtLeast(ver, 2019, 1):
		m.confirmDirect(r, ed, fmt.Sprintf("The Unity CLI cannot sign in Editors older than 2019.1, and %s is one.", ver))
		return nil
	case m.cli == nil:
		m.confirmDirect(r, ed, "The Unity CLI is not available.")
		return nil
	}
	if r.versionChanged() && unity.AtLeast(ver, 6000, 7) && !unity.AtLeast(r.Version, 6000, 7) {
		m.upgradeMenu(r, ver)
		return nil
	}
	if r.versionChanged() {
		m.push(&confirmPopup{
			title: "Open with another version", yes: "Open",
			message: fmt.Sprintf("Open %s with Unity %s? The project was made with %s, so Unity will ask to change it.", r.name(), ver, r.Version),
			onYes:   func(m *Model) tea.Cmd { return m.runOpen(r) },
		})
		return nil
	}
	return m.runOpen(r)
}

// confirmDirect offers to launch the Editor without the CLI.
func (m *Model) confirmDirect(r projectRow, ed *editorRow, why string) {
	m.push(&confirmPopup{
		title: "Open without the Unity CLI", yes: "Open anyway",
		message: why + " Opening the project directly works, but the Editor starts signed out, and Unity may start the official Hub to sign in.",
		onYes:   func(m *Model) tea.Cmd { return m.directLaunch(r, ed) },
	})
}

func (m *Model) directLaunch(r projectRow, ed *editorRow) tea.Cmd {
	target := ""
	if r.targetChanged() {
		if p, ok := unity.PlatformByBuildTarget(r.Pending.BuildTarget); ok {
			target = p.CLITarget
		}
	}
	if err := launch.Editor(ed.Location, r.Path, target); err != nil {
		m.showError("Opening "+r.name(), err)
		return nil
	}
	return m.afterOpened(r, "Opening %s in Unity %s (signed out)")
}

// runOpen opens the project through the CLI, which signs the Editor in.
func (m *Model) runOpen(r projectRow) tea.Cmd {
	c := m.cli
	opts := unitycli.OpenOptions{Project: r.Path}
	if r.versionChanged() {
		opts.EditorVersion = r.Pending.Version
	}
	if r.targetChanged() {
		if p, ok := unity.PlatformByBuildTarget(r.Pending.BuildTarget); ok {
			opts.BuildTarget = p.CLITarget
		}
	}
	m.toastf(toastInfo, "Opening %s…", r.name())
	return func() tea.Msg {
		res, err := c.Open(context.Background(), opts)
		return openedMsg{row: r, res: res, err: err}
	}
}

func (m *Model) onOpened(msg openedMsg) tea.Cmd {
	r := msg.row
	if msg.err != nil {
		var e *unitycli.Error
		text := msg.err.Error()
		if errors.As(msg.err, &e) && e.Output != "" {
			text += "\n\n" + e.Output
		}
		ed := m.editorFor(r.effVersion())
		pop := &confirmPopup{title: "Could not open " + r.name(), message: text}
		if ed != nil {
			pop.yes = "Open directly"
			pop.message += "\n\nOpen it directly instead? The Editor starts signed out."
			pop.onYes = func(m *Model) tea.Cmd { return m.directLaunch(r, ed) }
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
	return m.afterOpened(r, "Opened %s in Unity %s")
}

// afterOpened forgets the choices that have now been applied, and refreshes.
func (m *Model) afterOpened(r projectRow, format string) tea.Cmd {
	m.forgetPending(r.Path)
	m.toastf(toastOK, format, r.name(), r.effVersion())
	if m.cfg.QuitAfterOpen {
		m.quit = true
	}
	return m.loadProjectsCmd()
}

// upgradeMenu offers to upgrade a project to Unity 6.7 or later, which `open`
// refuses to do by itself.
func (m *Model) upgradeMenu(r projectRow, to string) {
	run := func(cloud unitycli.CloudChoice) func(*Model) tea.Cmd {
		return func(m *Model) tea.Cmd {
			c := m.cli
			return m.enqueue(&task{
				title: fmt.Sprintf("Upgrading %s to %s", r.name(), to),
				label: r.name() + " → " + to,
				works: []string{workProject(r.Path)},
				msg:   "Unity is upgrading the project; it opens when ready",
				do:    func(ctx context.Context) error { return c.UpgradeProject(ctx, r.Path, to, cloud) },
				after: func(m *Model, err error) tea.Cmd {
					if err != nil {
						return nil
					}
					// the upgrade applies the version only: a platform choice
					// waits for the next open
					m.st.SetPending(r.Path, state.Pending{BuildTarget: r.Pending.BuildTarget})
					m.saveState()
					return m.loadProjectsCmd()
				},
			})
		}
	}
	menu := newMenu(fmt.Sprintf("Upgrade %s to Unity %s", r.name(), to), []menuItem{
		{label: "Upgrade without Unity Cloud", run: run(unitycli.CloudOff),
			desc: "Unity 6.7 and later projects are tied to a Unity Cloud project. This upgrades without linking one."},
		{label: "Upgrade and link a new Unity Cloud project", run: run(unitycli.CloudNew),
			desc: "Creates a Unity Cloud project for it, under your account's default organisation."},
	})
	menu.wanted = 52
	m.push(menu)
}

// ---- version and platform -----------------------------------------------------------

func (m *Model) versionMenu(r projectRow) {
	var items []menuItem
	if !m.editorInstalled(r.Version) {
		v, cs := r.Version, r.Changeset
		items = append(items, menuItem{
			label: "Install " + v + "…", right: "the project's own version",
			run: func(m *Model) tea.Cmd { return m.installEditorFlow(v, cs) },
		})
	}
	for _, e := range m.editors {
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
			run: func(m *Model) tea.Cmd { m.setPendingVersion(r, v); return nil },
		})
	}
	items = append(items, menuItem{label: "Install another version…", run: (*Model).showReleases})
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
	m.st.SetPending(r.Path, p)
	m.saveState()
	m.refreshProjects()
	m.projectsTab.SelectKey(r.Path)
}

func (m *Model) platformMenu(r projectRow) {
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
			run: func(m *Model) tea.Cmd { m.setPendingTarget(r, target); return nil },
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
	m.st.SetPending(r.Path, p)
	m.saveState()
	m.refreshProjects()
	m.projectsTab.SelectKey(r.Path)
}

// ---- pin, sort, remove --------------------------------------------------------------

func (m *Model) togglePin(r projectRow) tea.Cmd {
	c := m.cli
	pin := !r.IsFavorite
	ok := "Pinned " + r.name()
	if !pin {
		ok = "Unpinned " + r.name()
	}
	return m.runCLI("Pinning "+r.name(), ok,
		func(ctx context.Context, _ *unitycli.Client) error { return c.SetPinned(ctx, r.Path, pin) },
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
	c := m.cli
	menu := newMenu("Remove "+r.name(), []menuItem{
		{
			label: "Remove from the list", desc: "The folder and its files stay on disk. You can add the project again later.",
			run: func(m *Model) tea.Cmd {
				return m.runCLI("Removing "+r.name(), "Removed "+r.name()+" from the list",
					func(ctx context.Context, _ *unitycli.Client) error { return c.RemoveProjects(ctx, r.Path) },
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

// completeDir completes a folder path as far as it is unambiguous.
func completeDir(text string) []string {
	out := completeExpanded(config.ExpandHome(text))
	if strings.HasPrefix(text, "~") {
		for i, o := range out {
			out[i] = displayPath(o) // keep the ~ the user typed
		}
	}
	return out
}

// dirMatches lists the folders (full text expanded) whose names continue text,
// and the folder they are in. Hidden folders show only once a dot is typed.
func dirMatches(text string) (dir string, names []string) {
	dir, base := filepath.Split(text)
	if dir == "" {
		dir = "."
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return dir, nil
	}
	for _, e := range entries {
		if !strings.HasPrefix(e.Name(), base) || (base == "" && strings.HasPrefix(e.Name(), ".")) {
			continue
		}
		if st, err := os.Stat(filepath.Join(dir, e.Name())); err != nil || !st.IsDir() {
			continue
		}
		names = append(names, e.Name())
	}
	sort.Strings(names)
	return dir, names
}

func completeExpanded(text string) []string {
	dir, names := dirMatches(text)
	base := filepath.Base(text)
	if strings.HasSuffix(text, string(filepath.Separator)) || text == "" {
		base = ""
	}
	switch len(names) {
	case 0:
		return nil
	case 1:
		return []string{filepath.Join(dir, names[0]) + string(filepath.Separator)}
	}
	common := commonPrefix(names)
	if len(common) > len(base) {
		return []string{filepath.Join(dir, common)}
	}
	return nil
}

// commonPrefix is the longest prefix every name starts with, cut between
// characters, never inside one.
func commonPrefix(names []string) string {
	common := []rune(names[0])
	for _, n := range names[1:] {
		r := []rune(n)
		i := 0
		for i < len(common) && i < len(r) && common[i] == r[i] {
			i++
		}
		common = common[:i]
	}
	return string(common)
}

func (m *Model) addProjectPrompt() {
	m.push(&promptPopup{
		title: "Add an existing project", label: "Folder of the Unity project:",
		in:       newInput(displayPath(m.projectsDir()) + string(filepath.Separator)),
		complete: completeDir,
		validate: func(text string) string {
			p := config.ExpandHome(text)
			if st, err := os.Stat(p); err != nil || !st.IsDir() {
				return "That folder does not exist"
			}
			if _, err := unity.ReadProjectVersion(p); err != nil {
				return "Not a Unity project: no ProjectSettings/ProjectVersion.txt"
			}
			return ""
		},
		onSubmit: func(m *Model, text string) tea.Cmd {
			path := filepath.Clean(config.ExpandHome(text))
			c := m.cli
			return m.runCLI("Adding the project", "Added "+filepath.Base(path),
				func(ctx context.Context, _ *unitycli.Client) error { return c.AddProjects(ctx, path) },
				func(m *Model) tea.Cmd { m.selectAfterLoad = path; return m.loadProjectsCmd() })
		},
	})
}

// addPackagePrompt asks for an OpenUPM package name and adds it to the project
// with the OpenUPM CLI.
func (m *Model) addPackagePrompt(r projectRow) tea.Cmd {
	if r.Missing {
		m.toastf(toastError, "The folder of %s is gone: %s", r.name(), displayPath(r.Path))
		return nil
	}
	exe := m.openupm.path
	if exe == "" {
		m.toastf(toastWarn, "Install the OpenUPM CLI first (Options, Tools)")
		return nil
	}
	m.push(&promptPopup{
		title: "Add an OpenUPM package", label: "Package name, such as com.example.package or com.example.package@1.2.0:",
		in: newInput(""),
		validate: func(text string) string {
			if text == "" {
				return "Type a package name"
			}
			return ""
		},
		onSubmit: func(m *Model, text string) tea.Cmd {
			pkgs := strings.Fields(text)
			return m.enqueue(&task{
				title: "Adding " + strings.Join(pkgs, ", ") + " to " + r.name(),
				label: "add " + strings.Join(pkgs, ", "),
				works: []string{workProject(r.Path)},
				msg:   "OpenUPM is adding the package",
				do:    func(ctx context.Context) error { return openupmAdd(ctx, exe, r.Path, pkgs) },
				after: func(m *Model, err error) tea.Cmd {
					if err == nil {
						m.toastf(toastOK, "Added %s to %s", strings.Join(pkgs, ", "), r.name())
					}
					return nil
				},
			})
		},
	})
	return nil
}

// ---- folders, editors, info, clipboard, clean -----------------------------------------

func (m *Model) openFolder(dir string) tea.Cmd {
	if err := launch.OpenFolder(m.goos, m.cfg.OS.OpenFolder, dir); err != nil {
		m.toastf(toastError, "Could not open the folder: %v", err)
	}
	return nil
}

// editProject opens the project in the configured code editor, or in $VISUAL /
// $EDITOR in the terminal.
func (m *Model) editProject(r projectRow) tea.Cmd {
	if tmpl := m.cfg.OS.Edit; tmpl != "" {
		if err := launch.Run(tmpl, r.Path); err != nil {
			m.toastf(toastError, "Could not start the editor: %v", err)
		}
		return nil
	}
	editor := os.Getenv("VISUAL")
	if editor == "" {
		editor = os.Getenv("EDITOR")
	}
	if editor == "" {
		m.toastf(toastWarn, "Set os.edit in config.yml, or $EDITOR, to edit projects")
		return nil
	}
	words, err := launch.Split(editor)
	if err != nil || len(words) == 0 {
		m.toastf(toastError, "Cannot run $EDITOR: %v", err)
		return nil
	}
	cmd := exec.Command(words[0], append(words[1:], ".")...)
	cmd.Dir = r.Path
	return tea.ExecProcess(cmd, func(err error) tea.Msg {
		if err != nil {
			return errorMsg{what: "Editor", err: err}
		}
		return nil
	})
}

type projectInfoMsg struct {
	row  projectRow
	info unitycli.ProjectInfo
	err  error
}

func (m *Model) projectInfoCmd(r projectRow) tea.Cmd {
	c := m.cli
	if c == nil {
		m.showCLIMissing()
		return nil
	}
	m.toastf(toastInfo, "Reading %s…", r.name())
	return func() tea.Msg {
		info, err := c.ProjectInfo(context.Background(), r.Path)
		return projectInfoMsg{row: r, info: info, err: err}
	}
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

// copyChoices opens a Copy menu of label and value pairs; picking one puts its
// value on the clipboard.
func (m *Model) copyChoices(wanted int, choices [][2]string) {
	var items []menuItem
	for _, c := range choices {
		label, value := c[0], c[1]
		items = append(items, menuItem{label: label, right: truncate(value, wanted-16), run: func(m *Model) tea.Cmd {
			m.toastf(toastOK, "Copied the %s", strings.ToLower(label))
			return tea.SetClipboard(value)
		}})
	}
	menu := newMenu("Copy", items)
	menu.wanted = wanted
	m.push(menu)
}

func (m *Model) cleanConfirm(r projectRow) {
	c := m.cli
	m.push(&confirmPopup{
		title: "Clean project", yes: "Clean", danger: true,
		message: fmt.Sprintf("Delete the regenerable folders of %s (Library, Temp, Logs ...)? Your assets are not touched, but Unity has to rebuild the Library the next time you open it, which can take a long time.", r.name()),
		onYes: func(m *Model) tea.Cmd {
			return m.runCLI("Cleaning "+r.name(), "Cleaned "+r.name(),
				func(ctx context.Context, _ *unitycli.Client) error { return c.CleanProject(ctx, r.Path) }, nil)
		},
	})
}
