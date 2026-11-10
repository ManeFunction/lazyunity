package tui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/ManeFunction/lazyunity/internal/config"
	"github.com/ManeFunction/lazyunity/internal/unitycli"
)

// newProject is the state of the "new project" wizard: name, location, Editor,
// template, options. Each step is a popup; Esc anywhere cancels.
type newProject struct {
	name    string
	parent  string
	version string
}

func (w newProject) path() string { return filepath.Join(w.parent, w.name) }

type templatesMsg struct {
	wiz   newProject
	items []unitycli.Template
	err   error
}

func (m *Model) startNewProject() tea.Cmd {
	if !m.needCLI() {
		return nil
	}
	if len(m.inv.editors()) == 0 {
		m.toastf(toastWarn, "Install a Unity Editor first")
		return m.showReleases()
	}
	m.push(&promptPopup{
		title: "New project (1/5)", label: "Name of the project:", in: newInput(""),
		validate: validateProjectName,
		onSubmit: func(m *Model, name string) tea.Cmd {
			m.newProjectLocation(newProject{name: name})
			return nil
		},
	})
	return nil
}

// validateProjectName rejects names that are not a single, plain folder name.
func validateProjectName(name string) string {
	switch {
	case name == "":
		return "Give the project a name"
	case name == "." || name == "..":
		return "That is not a usable name"
	case strings.ContainsAny(name, `/\`):
		return "A name cannot contain / or \\"
	case strings.ContainsAny(name, `<>:"|?*`) || strings.ContainsRune(name, 0):
		return `A name cannot contain < > : " | ? *`
	case strings.TrimSpace(name) != name || strings.HasSuffix(name, "."):
		return "A name cannot start with a space or end with a space or dot"
	case strings.HasPrefix(name, "-"):
		// the name is a positional argument of the CLI, which would read it as an option
		return "A name cannot start with -"
	}
	return ""
}

func (m *Model) newProjectLocation(w newProject) {
	m.push(&promptPopup{
		title: "New project (2/5)", label: "Create it in this folder:",
		in:       newInput(displayPath(m.projectsDir()) + string(filepath.Separator)),
		complete: completeDir,
		validate: func(text string) string {
			p := config.ExpandHome(text)
			if st, err := os.Stat(p); err != nil || !st.IsDir() {
				return "That folder does not exist"
			}
			if _, err := os.Stat(filepath.Join(p, w.name)); err == nil {
				return "There is already something called " + w.name + " there"
			}
			return ""
		},
		onSubmit: func(m *Model, text string) tea.Cmd {
			w.parent = filepath.Clean(config.ExpandHome(text))
			m.newProjectEditor(w)
			return nil
		},
	})
}

func (m *Model) newProjectEditor(w newProject) {
	var items []menuItem
	pick := ""
	for _, e := range m.inv.editors() {
		tags := []string{}
		if e.Default {
			tags = append(tags, "default")
			pick = e.Version
		}
		if e.LTS {
			tags = append(tags, "LTS")
		}
		v := e.Version
		items = append(items, menuItem{
			label: e.Version, right: strings.Join(tags, ", "),
			run: func(m *Model) tea.Cmd {
				return m.loadTemplatesCmd(newProject{name: w.name, parent: w.parent, version: v})
			},
		})
	}
	menu := newMenu("New project (3/5): Unity version", items)
	menu.wanted = 44
	menu.list.SelectKey(pick) // start on the default Editor
	m.push(menu)
}

func (m *Model) loadTemplatesCmd(w newProject) tea.Cmd {
	return m.withCLI(func(c unityCLI) tea.Cmd {
		m.toastf(toastInfo, "Looking up templates…")
		return func() tea.Msg {
			items, err := c.Templates(context.Background(), w.version)
			return templatesMsg{wiz: w, items: items, err: err}
		}
	})
}

func (m *Model) onTemplates(msg templatesMsg) {
	if msg.err != nil {
		m.showError("Listing templates", msg.err)
		return
	}
	if len(msg.items) == 0 {
		m.toastf(toastWarn, "Unity %s has no project templates installed", msg.wiz.version)
		return
	}
	var items []menuItem
	for _, t := range msg.items {
		tpl := t
		name := t.DisplayName
		if name == "" {
			name = t.Name
		}
		items = append(items, menuItem{
			label: name, right: t.Version, desc: t.Name,
			run: func(m *Model) tea.Cmd { m.newProjectOptions(msg.wiz, tpl.Name); return nil },
		})
	}
	menu := newMenu("New project (4/5): template", items)
	menu.wanted = 44
	m.push(menu)
}

func (m *Model) newProjectOptions(w newProject, template string) {
	const (
		optCloud         = "cloud"
		optRecommended   = "recommended"
		optManetoolsFull = "manetools-full"
		optManetoolsCore = "manetools-core"
	)
	items := []menuItem{
		{id: optCloud, label: "Link to Unity Cloud", desc: "Create a Unity Cloud project for it. Needs you to be signed in."},
		{id: optRecommended, label: "Add Unity's recommended packages and settings", desc: "Without this, the project is just the template.", checked: true},
	}
	openupmPath := m.openupm.path
	if openupmPath != "" {
		items = append(items,
			menuItem{id: optManetoolsFull, label: "Add ManeTools (full)", group: "manetools",
				desc: "Adds " + strings.Join(manetoolsFullPackages, " and ") + " from OpenUPM."},
			menuItem{id: optManetoolsCore, label: "Add ManeTools (w/o UI module)", group: "manetools",
				desc: "Adds " + strings.Join(manetoolsCorePackages, " and ") + " from OpenUPM, without the UI module."})
	} else {
		items = append(items, menuItem{label: "Add ManeTools (install OpenUPM to use)", disabled: "Install the OpenUPM CLI first (Options, Tools)",
			desc: "ManeTools come from OpenUPM. Install the OpenUPM CLI in the Tools tab, then create the project."})
	}
	menu := newMenu("New project (5/5): options", items)
	menu.multi = true
	menu.wanted = 56
	menu.submit = func(m *Model, picked []menuItem) tea.Cmd {
		opts := unitycli.CreateOptions{
			Name: w.name, ParentDir: w.parent, EditorVersion: w.version, Template: template,
			Cloud: unitycli.CloudOff, Bare: true, Open: true,
		}
		var packages []string
		for _, p := range picked {
			switch p.id {
			case optCloud:
				opts.Cloud = unitycli.CloudNew
			case optRecommended:
				opts.Bare = false
			case optManetoolsFull:
				packages = manetoolsFullPackages
			case optManetoolsCore:
				packages = manetoolsCorePackages
			}
		}
		return m.createProject(w, opts, openupmPath, packages)
	}
	m.push(menu)
}

// The ManeTools packages on OpenUPM.
var (
	manetoolsCorePackages = []string{"com.manefunction.tools-unity"}
	manetoolsFullPackages = []string{"com.manefunction.tools-unity", "com.manefunction.tools-unity-ui"}
)

// createProject creates the project. With packages, they are added with the
// OpenUPM CLI before the Editor opens, so Unity resolves everything at once.
func (m *Model) createProject(w newProject, opts unitycli.CreateOptions, openupmPath string, packages []string) tea.Cmd {
	return m.withCLI(func(c unityCLI) tea.Cmd { return m.queueCreate(c, w, opts, openupmPath, packages) })
}

func (m *Model) queueCreate(c unityCLI, w newProject, opts unitycli.CreateOptions, openupmPath string, packages []string) tea.Cmd {
	h := m.host
	open := opts.Open
	if len(packages) > 0 {
		opts.Open = false
	}
	return m.enqueue(&task{
		title: "Creating " + w.name,
		label: "create " + w.name,
		works: []string{workProject(w.path())},
		msg:   "Unity is creating the project; it opens when ready",
		do: func(ctx context.Context) error {
			if err := c.CreateProject(ctx, opts); err != nil {
				return err
			}
			if len(packages) == 0 {
				return nil
			}
			if err := h.AddPackages(ctx, openupmPath, w.path(), packages); err != nil {
				return fmt.Errorf("the project was created, but ManeTools were not added: %w", err)
			}
			if open {
				_, err := c.Open(ctx, unitycli.OpenOptions{Project: w.path(), EditorVersion: w.version})
				return err
			}
			return nil
		},
		after: func(m *Model, err error) tea.Cmd {
			if err == nil {
				m.toastf(toastOK, "Created %s", w.name)
				m.selectAfterLoad = w.path()
			}
			return m.loadProjectsCmd()
		},
	})
}
