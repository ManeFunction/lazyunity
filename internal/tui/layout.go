package tui

import "math"

// rect is a region of the screen in cells.
type rect struct{ x, y, w, h int }

func (r rect) empty() bool { return r.w <= 0 || r.h <= 0 }

func (r rect) contains(x, y int) bool {
	return x >= r.x && x < r.x+r.w && y >= r.y && y < r.y+r.h
}

// screenMode is lazygit's + and _ : which panels are on screen.
type screenMode int

const (
	modeFull     screenMode = iota // every panel
	modeProjects                   // only Projects
	modeEditors                    // Editors above Options
	numModes
)

func (s screenMode) String() string { return [...]string{"full", "projects", "editors"}[s] }

func parseScreenMode(name string) screenMode {
	switch name {
	case "projects":
		return modeProjects
	case "editors":
		return modeEditors
	}
	return modeFull
}

// shows reports whether the mode puts the panel on screen.
func (s screenMode) shows(panel int) bool {
	switch s {
	case modeProjects:
		return panel == panelProjects
	case modeEditors:
		return panel != panelProjects
	}
	return true
}

// modeShowing is the smaller mode that has the panel on screen.
func modeShowing(panel int) screenMode {
	if panel == panelProjects {
		return modeProjects
	}
	return modeEditors
}

// the panels, by index
const (
	panelProjects = iota
	panelEditors
	panelOptions
	numPanels
)

const (
	minScreenW = 52 // 28 for Projects and 24 for the right column
	minScreenH = 11
	// editorShare is the part of the right column the Editors panel takes.
	editorShare = 0.65
)

// geometry is where everything goes on screen.
type geometry struct {
	ok     bool // false when the terminal is too small
	panels [numPanels]rect
	tasks  rect // empty when no task is running
	bar    rect // the hint line, empty when switched off
}

func (g geometry) panelAt(x, y int) int {
	for i, r := range g.panels {
		if r.contains(x, y) {
			return i
		}
	}
	return -1
}

// computeGeometry lays out the screen.
//
// In full mode the Projects panel is on the left, Editors above Options on the
// right; the other modes give their panels the whole width. A task frame, when
// there is one, spans the width above the hint line.
func computeGeometry(w, h int, sideWidth float64, mode screenMode, tasksH int, showBar bool) geometry {
	var g geometry
	barH := 0
	if showBar {
		barH = 1
	}
	if w < minScreenW || h < minScreenH+tasksH+barH {
		return g
	}
	g.ok = true
	mainH := h - barH - tasksH
	if tasksH > 0 {
		g.tasks = rect{0, mainH, w, tasksH}
	}
	if barH > 0 {
		g.bar = rect{0, h - 1, w, 1}
	}

	// splitColumn stacks two panels in a column, the first taking the share.
	splitColumn := func(x, colW int, first, second int) {
		top := clamp(int(math.Round(float64(mainH)*editorShare)), 4, mainH-3)
		g.panels[first] = rect{x, 0, colW, top}
		g.panels[second] = rect{x, top, colW, mainH - top}
	}

	switch mode {
	case modeProjects:
		g.panels[panelProjects] = rect{0, 0, w, mainH}
	case modeEditors:
		splitColumn(0, w, panelEditors, panelOptions)
	default:
		left := clamp(int(math.Round(float64(w)*sideWidth)), 28, w-24)
		g.panels[panelProjects] = rect{0, 0, left, mainH}
		splitColumn(left, w-left, panelEditors, panelOptions)
	}
	return g
}
