package tui

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

func selectedProject(t *testing.T, m *Model) projectRow {
	t.Helper()
	r, ok := m.projectsTab.Selected()
	if !ok {
		t.Fatal("no project selected")
	}
	return r
}

func TestFocusMovesBetweenPanels(t *testing.T) {
	m := newTestModel(t)
	if m.focus != panelProjects {
		t.Fatalf("starts on %d", m.focus)
	}
	steps := []struct {
		keys []string
		want int
	}{
		{[]string{"tab"}, panelEditors},
		{[]string{"l"}, panelOptions},
		{[]string{"right"}, panelProjects}, // wraps
		{[]string{"shift+tab"}, panelOptions},
		{[]string{"h"}, panelEditors},
		{[]string{"left"}, panelProjects},
		{[]string{"2"}, panelEditors},
		{[]string{"3"}, panelOptions},
		{[]string{"1"}, panelProjects},
	}
	for _, s := range steps {
		press(m, s.keys...)
		if m.focus != s.want {
			t.Errorf("after %v: focus %d, want %d", s.keys, m.focus, s.want)
		}
	}
}

func TestTabsSwitchWithBracketsAndRepeatedJump(t *testing.T) {
	m := newTestModel(t)
	press(m, "2")
	if m.panels[panelEditors].cur != 0 {
		t.Fatal("starts on Installed")
	}
	press(m, "]")
	if m.panels[panelEditors].cur != 1 || m.focusedTab() != tab(m.releasesTab) {
		t.Errorf("] should open Releases, cur=%d", m.panels[panelEditors].cur)
	}
	press(m, "]") // wraps around
	if m.panels[panelEditors].cur != 0 {
		t.Errorf("] should wrap to Installed, cur=%d", m.panels[panelEditors].cur)
	}
	press(m, "[")
	if m.panels[panelEditors].cur != 1 {
		t.Errorf("[ should go back, cur=%d", m.panels[panelEditors].cur)
	}
	// pressing the number of the focused panel again moves to its next tab, as in lazygit
	press(m, "2")
	if m.panels[panelEditors].cur != 0 {
		t.Errorf("repeated 2 should cycle the tab, cur=%d", m.panels[panelEditors].cur)
	}
	// a panel with a single tab ignores brackets
	press(m, "1", "]", "[")
	if m.panels[panelProjects].cur != 0 || m.focus != panelProjects {
		t.Error("the Projects panel has one tab")
	}
	// and keys act on the focused tab's context
	press(m, "3")
	if m.focusCtx() != ctxTools {
		t.Errorf("context = %v, want tools", m.focusCtx())
	}
	// the Tools panel has one tab, so brackets stay
	press(m, "]")
	if m.focusCtx() != ctxTools {
		t.Errorf("context = %v, want tools", m.focusCtx())
	}
}

func TestReleasesLoadOnFirstVisit(t *testing.T) {
	m := newTestModel(t)
	m.cli = nil // no CLI in this test: nothing can load
	press(m, "2")
	if cmd := m.switchTab(1); cmd != nil {
		t.Error("without a CLI there is nothing to load")
	}
	m.releasesTab.loaded = true
	m.releasesTab.SetItems(nil)
	if cmd := m.afterTabSwitch(); cmd != nil {
		t.Error("an already loaded tab must not reload")
	}
}

func TestNavigationKeys(t *testing.T) {
	m := newTestModel(t)
	first := selectedProject(t, m).name()
	press(m, "j")
	second := selectedProject(t, m).name()
	if second == first {
		t.Fatal("j did not move")
	}
	press(m, "down")
	third := selectedProject(t, m).name()
	press(m, "j") // at the end: stays
	if selectedProject(t, m).name() != third {
		t.Error("moving past the end must stop")
	}
	press(m, "k")
	if selectedProject(t, m).name() != second {
		t.Error("k moves up")
	}
	press(m, "<")
	if selectedProject(t, m).name() != first {
		t.Error("< goes to the top")
	}
	press(m, ">")
	if selectedProject(t, m).name() != third {
		t.Error("> goes to the bottom")
	}
	press(m, "home")
	if selectedProject(t, m).name() != first {
		t.Error("home goes to the top")
	}
	press(m, "end", "up")
	if selectedProject(t, m).name() != second {
		t.Error("end then up")
	}
}

func TestProjectsAreOrderedPinnedFirstThenByLastOpened(t *testing.T) {
	m := newTestModel(t)
	var names []string
	for _, r := range m.projectsTab.Items() {
		names = append(names, r.name())
	}
	want := []string{"Space Shooter", "Old Platformer", "Ancient Thing"}
	if strings.Join(names, ",") != strings.Join(want, ",") {
		t.Errorf("order = %v, want %v", names, want)
	}
}

func TestFilterKeys(t *testing.T) {
	m := newTestModel(t)
	press(m, "/")
	if !m.filter.active {
		t.Fatal("/ starts the filter")
	}
	// 'ancient' matches only Ancient Thing; it must not be a substring of the
	// temp folder the fixture lives in, which differs per platform.
	press(m, "a", "n", "c")
	if m.projectsTab.Len() != 1 || selectedProject(t, m).name() != "Ancient Thing" {
		t.Errorf("filter 'anc' gave %d projects", m.projectsTab.Len())
	}
	if !strings.Contains(screenText(m), "Filter: anc") {
		t.Error("the filter prompt should be on the bottom line while typing")
	}
	press(m, "backspace")
	if m.projectsTab.Filter() != "an" {
		t.Errorf("backspace: %q", m.projectsTab.Filter())
	}
	press(m, "c", "enter") // enter keeps the filter but leaves the prompt
	if m.filter.active || m.projectsTab.Filter() != "anc" {
		t.Errorf("enter: active=%v filter=%q", m.filter.active, m.projectsTab.Filter())
	}
	if !strings.Contains(screenText(m), "filter: anc") {
		t.Error("an applied filter is shown at the bottom right")
	}
	press(m, "esc") // esc clears an applied filter
	if m.projectsTab.Filter() != "" || m.projectsTab.Len() != 3 {
		t.Errorf("esc: filter %q, %d projects", m.projectsTab.Filter(), m.projectsTab.Len())
	}
	// esc while typing clears and leaves
	press(m, "/", "x", "esc")
	if m.filter.active || m.projectsTab.Filter() != "" {
		t.Error("esc while typing cancels the filter")
	}
	// a filter with no matches says so
	press(m, "/", "z", "z", "z", "enter")
	if !strings.Contains(screenText(m), "No matches for 'zzz'") {
		t.Error("an empty result needs a message")
	}
}

func TestFilterBelongsToTheFocusedTab(t *testing.T) {
	m := newTestModel(t)
	press(m, "/", "s", "p", "enter") // projects: Space Shooter
	press(m, "2")
	if m.editorsTab.Filter() != "" {
		t.Error("the Editors list must not inherit the Projects filter")
	}
	press(m, "/", "2", "0", "2", "enter")
	if m.editorsTab.Filter() != "202" {
		t.Errorf("editors filter = %q", m.editorsTab.Filter())
	}
	press(m, "1")
	if m.projectsTab.Filter() != "sp" {
		t.Errorf("projects filter = %q", m.projectsTab.Filter())
	}
}

func TestFilterTextIsPastedAndMoved(t *testing.T) {
	m := newTestModel(t)
	press(m, "/")
	m.Update(tea.PasteMsg{Content: "spa\nce"})
	if m.projectsTab.Filter() != "spa ce" {
		t.Errorf("paste: %q", m.projectsTab.Filter())
	}
	press(m, "ctrl+u")
	if m.projectsTab.Filter() != "" {
		t.Errorf("ctrl+u should clear: %q", m.projectsTab.Filter())
	}
	press(m, "o", "down") // moving in the list while typing
	press(m, "enter")
}

func TestScreenModes(t *testing.T) {
	m := newTestModel(t)
	if m.mode != modeFull {
		t.Fatal("starts in full mode")
	}
	press(m, "+")
	if m.mode != modeProjects || m.geo.panels[panelProjects].w != 120 || !m.geo.panels[panelEditors].empty() {
		t.Errorf("projects: mode=%v panels=%+v", m.mode, m.geo.panels)
	}
	if got := screen(m); !strings.Contains(got[0], "[1]─Projects") || strings.Contains(got[0], "[2]") {
		t.Errorf("projects mode shows only Projects: %q", got[0])
	}
	// tab stays among the panels on screen
	press(m, "tab")
	if m.focus != panelProjects || m.mode != modeProjects {
		t.Errorf("tab in projects mode: focus=%d mode=%v", m.focus, m.mode)
	}
	press(m, "+")
	if m.mode != modeEditors || m.focus != panelEditors || !m.geo.panels[panelProjects].empty() || m.geo.panels[panelEditors].w != 120 {
		t.Errorf("editors: mode=%v focus=%d panels=%+v", m.mode, m.focus, m.geo.panels)
	}
	out := screenText(m)
	if !strings.Contains(out, "[2]─Installed Editors") || !strings.Contains(out, "[3]─Tools") || strings.Contains(out, "[1]") {
		t.Errorf("editors mode shows panels 2 and 3:\n%s", out)
	}
	press(m, "tab")
	if m.focus != panelOptions || m.mode != modeEditors {
		t.Errorf("tab in editors mode: focus=%d mode=%v", m.focus, m.mode)
	}
	press(m, "tab")
	if m.focus != panelEditors {
		t.Errorf("tab wraps within editors mode: focus=%d", m.focus)
	}
	// jumping to a hidden panel brings it on screen
	press(m, "1")
	if m.mode != modeProjects || m.focus != panelProjects {
		t.Errorf("1 in editors mode: mode=%v focus=%d", m.mode, m.focus)
	}
	press(m, "+", "+")
	if m.mode != modeFull {
		t.Errorf("+ wraps around, mode=%v", m.mode)
	}
	press(m, "_")
	if m.mode != modeEditors {
		t.Errorf("_ goes the other way, mode=%v", m.mode)
	}
	for i, l := range screen(m) {
		if width(l) != 120 {
			t.Errorf("line %d is %d wide", i, width(l))
		}
	}
}

func TestScreenModeFromConfig(t *testing.T) {
	o := testOptions(t)
	o.Config.GUI.ScreenMode = "editors"
	m, err := New(o)
	if err != nil {
		t.Fatal(err)
	}
	if m.mode != modeEditors || m.focus == panelProjects {
		t.Errorf("mode = %v, focus = %d", m.mode, m.focus)
	}
}

func TestTooSmall(t *testing.T) {
	m := newTestModel(t)
	resize(m, 30, 8)
	out := screenText(m)
	if !strings.Contains(out, "Not enough space to render panels") {
		t.Errorf("got %q", out)
	}
	if len(screen(m)) != 8 {
		t.Errorf("%d lines", len(screen(m)))
	}
	// and it recovers
	resize(m, 120, 36)
	if !strings.Contains(screenText(m), "[1]─Projects") {
		t.Error("the interface did not come back")
	}
}

func TestBeforeTheFirstSizeNothingIsDrawn(t *testing.T) {
	m, err := New(testOptions(t))
	if err != nil {
		t.Fatal(err)
	}
	if got := m.View().Content; got != "" {
		t.Errorf("drew %q before knowing the size", got)
	}
}

func TestViewRequestsAltScreenAndMouse(t *testing.T) {
	m := newTestModel(t)
	v := m.View()
	if !v.AltScreen || v.MouseMode != tea.MouseModeCellMotion || !v.ReportFocus {
		t.Errorf("view = %+v", v)
	}
	m.cfg.GUI.MouseEvents = false
	if m.View().MouseMode != tea.MouseModeNone {
		t.Error("mouse must be off when configured off")
	}
}

func TestQuit(t *testing.T) {
	for _, k := range []string{"q", "ctrl+c"} {
		m := newTestModel(t)
		_, cmd := m.Update(keyMsg(k))
		if !m.quit || cmd == nil {
			t.Errorf("%s should quit", k)
		}
	}
}

func TestMouseSelectsRowsAndTabs(t *testing.T) {
	m := newTestModel(t)
	// click the second project row (row 1 below the top border)
	m.Update(tea.MouseClickMsg{X: 5, Y: 2, Button: tea.MouseLeft})
	if selectedProject(t, m).name() != "Old Platformer" {
		t.Errorf("selected %q", selectedProject(t, m).name())
	}
	// clicking another panel focuses it
	r := m.geo.panels[panelEditors]
	m.Update(tea.MouseClickMsg{X: r.x + 5, Y: r.y + 2, Button: tea.MouseLeft})
	if m.focus != panelEditors {
		t.Errorf("focus = %d", m.focus)
	}
	if got, _ := m.editorsTab.Selected(); got.Version != "2022.3.62f1" {
		t.Errorf("clicked editor = %q", got.Version)
	}
	// clicking the name of the Releases tab switches to it: "╭─[2]─Installed Editors - Releases"
	col := r.x + 2 + width("[2]") + 1 + width("Installed Editors - ")
	m.cli = nil
	m.Update(tea.MouseClickMsg{X: col + 1, Y: r.y, Button: tea.MouseLeft})
	if m.panels[panelEditors].cur != 1 {
		t.Errorf("tab = %d, want Releases", m.panels[panelEditors].cur)
	}
	// clicking the Installed name goes back
	m.Update(tea.MouseClickMsg{X: r.x + 2 + width("[2]") + 1 + 2, Y: r.y, Button: tea.MouseLeft})
	if m.panels[panelEditors].cur != 0 {
		t.Errorf("tab = %d, want Installed", m.panels[panelEditors].cur)
	}
	// other buttons, and clicks outside any panel, do nothing
	m.Update(tea.MouseClickMsg{X: 5, Y: 2, Button: tea.MouseRight})
	m.Update(tea.MouseClickMsg{X: 5, Y: m.h - 1, Button: tea.MouseLeft})
	if m.focus != panelEditors {
		t.Error("stray clicks changed the focus")
	}
}

func TestMouseCanBeSwitchedOff(t *testing.T) {
	m := newTestModel(t)
	m.cfg.GUI.MouseEvents = false
	m.Update(tea.MouseClickMsg{X: 5, Y: 3, Button: tea.MouseLeft})
	if selectedProject(t, m).name() != "Space Shooter" {
		t.Error("clicks must be ignored when mouse events are off")
	}
}

func TestMouseWheelScrolls(t *testing.T) {
	m := newTestModel(t)
	var many []projectRow
	for i := 0; i < 60; i++ {
		many = append(many, projectRow{Project: m.rawProjects[0]})
		many[i].Title = strings.Repeat("x", 1) + string(rune('A'+i%26)) + string(rune('a'+i/26))
		many[i].Path = many[i].Title
	}
	m.projectsTab.SetItems(many)
	_, before := m.projectsTab.Window()
	m.Update(tea.MouseWheelMsg{X: 5, Y: 5, Button: tea.MouseWheelDown})
	_, after := m.projectsTab.Window()
	if after <= before {
		t.Errorf("wheel down did not scroll: %d -> %d", before, after)
	}
	m.Update(tea.MouseWheelMsg{X: 5, Y: 5, Button: tea.MouseWheelUp})
	_, back := m.projectsTab.Window()
	if back != before {
		t.Errorf("wheel up did not scroll back: %d", back)
	}
	// the wheel acts on the panel under the pointer, not the focused one
	m.Update(tea.MouseWheelMsg{X: 100, Y: 5, Button: tea.MouseWheelDown})
	if _, off := m.projectsTab.Window(); off != before {
		t.Error("the wheel over another panel scrolled the Projects list")
	}
}

func TestRelativeTimesAndMissingProjectsShow(t *testing.T) {
	m := newTestModel(t)
	out := screenText(m)
	for _, want := range []string{"2h ago", "3d ago", "✗ missing", "★ Space Shooter", "6000.6.8f1", "Android", "Linux"} {
		if !strings.Contains(out, want) {
			t.Errorf("the screen should contain %q", want)
		}
	}
}

func TestEditorsShowPlatformsAndLTS(t *testing.T) {
	m := newTestModel(t)
	m.focus = panelEditors
	out := screenText(m)
	if !strings.Contains(out, "LTS") || !strings.Contains(out, "Linux, Android") {
		t.Errorf("editors list:\n%s", out)
	}
}

func TestStatusLine(t *testing.T) {
	m := newTestModel(t)
	if last := screen(m)[len(screen(m))-1]; strings.Contains(last, "1.0.0-beta.16") {
		t.Errorf("the CLI version is not on the bottom line: %q", last)
	}
	m.Update(authMsg{info: authInfo(true)})
	if !strings.Contains(screenText(m), "● signed in") {
		t.Error("signed in")
	}
	m.Update(authMsg{info: authInfo(false)})
	if !strings.Contains(screenText(m), "○ signed out") {
		t.Error("signed out")
	}
	m.cliStatus.State = 0 // missing
	if !strings.Contains(screenText(m), "Unity CLI not installed") {
		t.Error("a missing CLI must be said on the bottom line")
	}
}

func TestToastsShowThenExpire(t *testing.T) {
	m := newTestModel(t)
	m.toastf(toastOK, "Done it")
	if !strings.Contains(screenText(m), "Done it") {
		t.Error("toast not shown")
	}
	later := testNow.Add(10 * time.Second)
	m.now = func() time.Time { return later }
	if strings.Contains(screenText(m), "Done it") {
		t.Error("toast should expire")
	}
}
