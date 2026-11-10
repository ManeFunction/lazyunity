package tui

import (
	tea "charm.land/bubbletea/v2"
)

// buildActions is every action of the interface, in the order the bottom line
// and the ? menu list them.
func buildActions() []*action {
	var all []*action
	all = append(all, globalActions()...)
	all = append(all, projectActions()...)
	all = append(all, editorActions()...)
	all = append(all, moduleActions()...)
	all = append(all, releaseActions()...)
	// C cleans the download cache from anywhere in the Editors panel
	for _, c := range []ctxID{ctxEditors, ctxModules, ctxReleases} {
		all = append(all, cleanCacheAction(c))
	}
	all = append(all, rootActions()...)
	all = append(all, toolActions()...)
	return all
}

func globalActions() []*action {
	nav := func(id, desc string, keys []string, f func(n navigable)) *action {
		return &action{
			id: "universal." + id, ctx: ctxGlobal, keys: keys, desc: desc, nav: true,
			run: func(m *Model) tea.Cmd {
				f(m.focusedTab().nav())
				return m.afterNav()
			},
		}
	}
	jump := func(name string, panelIdx int) *action {
		return &action{
			id: "universal.jump" + name, ctx: ctxGlobal, keys: []string{string(rune('1' + panelIdx))},
			desc: "Jump to the " + name + " panel", nav: true,
			run: func(m *Model) tea.Cmd { return m.jumpTo(panelIdx) },
		}
	}
	return []*action{
		{id: "universal.openMenu", ctx: ctxGlobal, keys: []string{"?"}, desc: "Open the keybindings menu", hint: "Keybindings",
			run: func(m *Model) tea.Cmd { m.openKeybindings(); return nil }},
		{id: "universal.quit", ctx: ctxGlobal, keys: []string{"q", "ctrl+c"}, desc: "Quit",
			run: func(m *Model) tea.Cmd { return m.requestQuit() }},
		{id: "universal.return", ctx: ctxGlobal, keys: []string{"esc"}, desc: "Clear the filter, or go back",
			run: func(m *Model) tea.Cmd { m.goBack(); return nil }},
		{id: "universal.filter", ctx: ctxGlobal, keys: []string{"/"}, desc: "Filter the list",
			run: func(m *Model) tea.Cmd { m.startFilter(); return nil }},
		{id: "universal.refresh", ctx: ctxGlobal, keys: []string{"R"}, desc: "Refresh",
			run: func(m *Model) tea.Cmd { return m.refreshAllCmd() }},
		{id: "universal.nextScreenMode", ctx: ctxGlobal, keys: []string{"+"}, desc: "Next screen mode (full, projects, editors)",
			run: func(m *Model) tea.Cmd { m.cycleMode(1); return nil }},
		{id: "universal.prevScreenMode", ctx: ctxGlobal, keys: []string{"_"}, desc: "Previous screen mode",
			run: func(m *Model) tea.Cmd { m.cycleMode(-1); return nil }},
		{id: "universal.commandLog", ctx: ctxGlobal, keys: []string{"@"}, desc: "Show the command log",
			run: func(m *Model) tea.Cmd { m.openCommandLog(); return nil }},
		{id: "universal.shell", ctx: ctxGlobal, keys: []string{":"}, desc: "Run a shell command",
			run: func(m *Model) tea.Cmd { m.shellPrompt(); return nil }},
		{id: "universal.copyPath", ctx: ctxGlobal, keys: []string{"ctrl+o"}, desc: "Copy the path of the selected item",
			run: func(m *Model) tea.Cmd { return m.copySelectedPath() }},
		{id: "universal.cancelTask", ctx: ctxGlobal, keys: []string{"X"}, desc: "Cancel the running task", hint: "Cancel task",
			enabled: func(m *Model) bool { return m.busy() },
			run:     func(m *Model) tea.Cmd { m.cancelRunning(); return nil }},
		{id: "universal.suspend", ctx: ctxGlobal, keys: []string{"ctrl+z"}, desc: "Suspend to the shell",
			run: func(m *Model) tea.Cmd { return tea.Suspend }},

		{id: "universal.nextPanel", ctx: ctxGlobal, keys: []string{"tab", "l", "right"}, desc: "Next panel", nav: true,
			run: func(m *Model) tea.Cmd { return m.movePanel(1) }},
		{id: "universal.prevPanel", ctx: ctxGlobal, keys: []string{"shift+tab", "h", "left"}, desc: "Previous panel", nav: true,
			run: func(m *Model) tea.Cmd { return m.movePanel(-1) }},
		jump("Projects", panelProjects), jump("Editors", panelEditors), jump("Options", panelOptions),
		{id: "universal.nextTab", ctx: ctxGlobal, keys: []string{"]"}, desc: "Next tab of the panel", nav: true,
			run: func(m *Model) tea.Cmd { return m.switchTab(1) }},
		{id: "universal.prevTab", ctx: ctxGlobal, keys: []string{"["}, desc: "Previous tab of the panel", nav: true,
			run: func(m *Model) tea.Cmd { return m.switchTab(-1) }},

		nav("nextItem", "Next item", []string{"j", "down"}, func(n navigable) { n.Move(1) }),
		nav("prevItem", "Previous item", []string{"k", "up"}, func(n navigable) { n.Move(-1) }),
		nav("nextPage", "Next page", []string{".", "pgdown"}, func(n navigable) { n.Page(1) }),
		nav("prevPage", "Previous page", []string{",", "pgup"}, func(n navigable) { n.Page(-1) }),
		nav("goStart", "Scroll to the top", []string{"<", "home"}, func(n navigable) { n.Top() }),
		nav("goEnd", "Scroll to the bottom", []string{">", "end"}, func(n navigable) { n.Bottom() }),
	}
}

// goBack is Esc: clear a filter, else leave the Editor's modules.
func (m *Model) goBack() {
	t := m.focusedTab()
	if t.filter() != "" {
		t.setFilter("")
		return
	}
	if p := m.focusedPanel(); p.drill != nil {
		p.drill = nil
	}
}

// movePanel moves focus to the next or previous panel on screen.
func (m *Model) movePanel(delta int) tea.Cmd {
	i := m.focus
	for range numPanels {
		i = (i + delta + numPanels) % numPanels
		if m.mode.shows(i) {
			break
		}
	}
	m.setFocus(i)
	return nil
}

// jumpTo focuses a panel; pressing the key of the focused panel again moves to
// its next tab, as in lazygit.
func (m *Model) jumpTo(i int) tea.Cmd {
	if i == m.focus {
		return m.switchTab(1)
	}
	m.setFocus(i)
	return nil
}

// switchTab moves to another tab of the focused panel, loading what the tab
// shows the first time it is looked at.
func (m *Model) switchTab(delta int) tea.Cmd {
	m.filter.active = false
	if !m.focusedPanel().switchTab(delta) {
		return nil
	}
	return m.afterTabSwitch()
}

func (m *Model) afterTabSwitch() tea.Cmd {
	if m.focusedTab() == tab(m.releasesTab) && !m.releases.loaded {
		return m.loadReleasesCmd(true)
	}
	return nil
}

// afterNav loads more releases when the cursor nears the end of the list.
func (m *Model) afterNav() tea.Cmd {
	if m.focusedTab() == tab(m.releasesTab) && m.releases.nextOffset > 0 &&
		m.releasesTab.Cursor() >= m.releasesTab.Len()-3 {
		return m.loadReleasesCmd(false)
	}
	return nil
}

// cycleMode moves to the next or previous screen mode, keeping the focus on a
// panel that is still on screen.
func (m *Model) cycleMode(delta int) {
	m.mode = (m.mode + screenMode(delta) + numModes) % numModes
	if !m.mode.shows(m.focus) {
		m.filter.active = false
		m.focus = map[bool]int{true: panelProjects, false: panelEditors}[m.mode == modeProjects]
	}
	m.relayout()
}

// openKeybindings shows every action in a menu that filters as you type.
func (m *Model) openKeybindings() {
	var local, global, navs []menuItem
	ctx := m.focusCtx()
	for _, a := range m.keymap.actions {
		if len(m.keymap.keys[a]) == 0 {
			continue
		}
		it := menuItem{label: a.desc, key: m.keymap.allKeys(a)}
		act := a
		if !a.available(m) {
			it.disabled = "Not available right now"
		}
		it.run = func(m *Model) tea.Cmd { return act.run(m) }
		switch {
		case a.nav:
			navs = append(navs, it)
		case a.ctx == ctx:
			local = append(local, it)
		case a.ctx == ctxGlobal:
			global = append(global, it)
		}
	}
	var items []menuItem
	section := func(title string, group []menuItem) {
		if len(group) > 0 {
			items = append(items, menuItem{label: title, heading: true})
			items = append(items, group...)
		}
	}
	section("Local", local)
	section("Global", global)
	section("Navigation", navs)
	menu := newMenu("Keybindings", items)
	menu.filterable = true
	menu.wanted = 64
	menu.emptyMsg = "No keybinding matches"
	m.push(menu)
}
