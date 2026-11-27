package tui

import (
	"context"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/ManeFunction/lazyunity/internal/unitycli"
)

// Updating an Editor moves it to the newest patch of its release line. The
// Unity CLI's own `editors upgrade` installs without asking anything, so
// lazyunity only uses it to learn which patch is the newest, and runs the
// update itself: an install of the new patch, with the old Editor's modules
// ticked, then, if the user wants, an uninstall of the old Editor and a move
// of its projects to the new one. Both wait in the queue for the install, and
// are dropped when it fails.

// updateModulesMsg is what an update needs to offer modules: those of the new
// patch, and which of them the old Editor has.
type updateModulesMsg struct {
	from editorRow
	mods []unitycli.Module
	err  error
	// have are the modules the old Editor has, by id; haveErr is why they
	// could not be listed.
	have    map[string]bool
	haveErr error
}

// updateConfirm offers to update an Editor to the newest patch of its line.
func (m *Model) updateConfirm(e editorRow) {
	to := e.Update
	if t := m.taskOn(workEditor(to)); t != nil {
		m.toastf(toastInfo, "Already in the queue: %s", t.title)
		return
	}
	m.push(&confirmPopup{
		title: "Update Unity " + e.Version, yes: "Choose modules",
		message: fmt.Sprintf("Install Unity %s, the newest patch of Unity %s?\n\n"+
			"The modules Unity %s has are ticked in the next step. Once the install is queued, "+
			"lazyunity asks whether to uninstall %s and whether to move its projects to %s.",
			to, unityLine(e.Version), e.Version, e.Version, to),
		onYes: func(m *Model) tea.Cmd { return m.updateModulesCmd(e) },
	})
}

// updateModulesCmd lists the modules of the new patch and those the old
// Editor has, at once.
func (m *Model) updateModulesCmd(e editorRow) tea.Cmd {
	return m.withCLI(func(c unityCLI) tea.Cmd {
		m.toastf(toastInfo, "Looking up Unity %s…", e.Update)
		return func() tea.Msg {
			ctx := context.Background()
			type haveResult struct {
				mods []unitycli.Module
				err  error
			}
			ch := make(chan haveResult, 1)
			go func() {
				mods, err := c.Modules(ctx, e.Version, e.Arch)
				ch <- haveResult{mods, err}
			}()
			mods, err := c.ReleaseModules(ctx, e.Update)
			h := <-ch
			have := map[string]bool{}
			for _, x := range h.mods {
				if x.IsInstalled {
					have[x.ID] = true
				}
			}
			return updateModulesMsg{from: e, mods: mods, err: err, have: have, haveErr: h.err}
		}
	})
}

func (m *Model) onUpdateModules(msg updateModulesMsg) tea.Cmd {
	from, to := msg.from, msg.from.Update
	if msg.err != nil {
		m.showError("Looking up Unity "+to, msg.err)
		return nil
	}
	if msg.haveErr != nil {
		m.toastf(toastWarn, "Could not list the modules of Unity %s, so none are ticked: %v", from.Version, msg.haveErr)
	}
	mods := msg.mods
	sortModules(mods)
	pick := func(m *Model, picked []unitycli.Module) tea.Cmd {
		ids := make([]string, len(picked))
		for i, x := range picked {
			ids[i] = x.ID
		}
		return m.planInstall(unitycli.InstallOptions{
			Version: to, Architecture: from.Arch, Modules: ids, SetDefault: from.Default,
		}, &from)
	}
	if len(mods) == 0 {
		return pick(m, nil)
	}
	m.push(m.modulePicker(fmt.Sprintf("Update Unity %s to %s: choose modules", from.Version, to), mods, msg.have, pick))
	return nil
}

// askAfterUpdate asks, once the install of the new patch is queued, whether
// to uninstall the old Editor, then whether to move its projects to the new
// one. What the user agrees to waits in the queue for the install.
func (m *Model) askAfterUpdate(from editorRow, to string, install *task) {
	askMove := func(m *Model) tea.Cmd { m.askMoveProjects(from.Version, to, install); return nil }
	m.push(&confirmPopup{
		title: "Uninstall Unity " + from.Version, yes: "Uninstall", no: "Keep it", danger: true,
		message: fmt.Sprintf("Uninstall Unity %s once %s is installed? This deletes its folder and every module in it.",
			from.Version, to),
		onYes: func(m *Model) tea.Cmd {
			cmd := m.withCLI(func(c unityCLI) tea.Cmd { return m.enqueueAfter(install, uninstallTask(c, from)) })
			return tea.Batch(cmd, askMove(m))
		},
		onNo: askMove,
	})
}

// askMoveProjects offers to move the projects that open with one version to
// another, when there are any.
func (m *Model) askMoveProjects(from, to string, install *task) {
	var paths, names []string
	for _, r := range m.projectsTab.Items() {
		if r.effVersion() == from {
			paths, names = append(paths, r.Path), append(names, r.name())
		}
	}
	if len(paths) == 0 {
		return
	}
	shown := names
	if len(shown) > 5 {
		shown = append(shown[:5:5], fmt.Sprintf("and %d more", len(names)-5))
	}
	m.push(&confirmPopup{
		title: "Move projects to Unity " + to, yes: "Move", no: "Leave them",
		message: fmt.Sprintf("Move these projects to Unity %s?\n\n%s open with Unity %s: %s.\n\n"+
			"As with v, each one moves the next time it opens. The change applies once %s is installed.",
			to, plural(len(paths), "project"), from, strings.Join(shown, ", "), to),
		onYes: func(m *Model) tea.Cmd { return m.enqueueAfter(install, moveProjectsTask(from, to, paths)) },
	})
}

// moveProjectsTask switches projects from one Editor version to another. It
// is a task, though it only changes what lazyunity remembers, so that it shows
// in the queue and waits there for the install of the version it moves to.
func moveProjectsTask(from, to string, paths []string) *task {
	return &task{
		title: fmt.Sprintf("Moving %s to Unity %s", plural(len(paths), "project"), to),
		label: "projects to " + to,
		works: []string{workMoveProjects(from)},
		do:    func(context.Context) error { return nil },
		after: func(m *Model, err error) tea.Cmd {
			if err == nil {
				m.moveProjects(from, to, paths)
			}
			return nil
		},
	}
}

// moveProjects makes the projects that still open with from open with to, as
// picking to with v would.
func (m *Model) moveProjects(from, to string, paths []string) {
	moved := 0
	for _, path := range paths {
		r, ok := m.projectByPath(path)
		if !ok || r.effVersion() != from {
			continue // removed, or moved some other way since
		}
		p := r.Pending
		p.Version = to
		if to == r.Version {
			p.Version = ""
		}
		m.st.SetPending(path, p)
		moved++
	}
	if moved == 0 {
		return
	}
	m.saveState()
	m.refreshProjects()
	m.toastf(toastOK, "Moved %s to Unity %s", plural(moved, "project"), to)
}
