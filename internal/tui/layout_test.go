package tui

import "testing"

// covered checks that the rectangles tile the main area without gaps or overlap.
func covered(t *testing.T, g geometry, w, mainH int) {
	t.Helper()
	cells := make([][]int, mainH)
	for y := range cells {
		cells[y] = make([]int, w)
	}
	for i, r := range g.panels {
		for y := r.y; y < r.y+r.h; y++ {
			for x := r.x; x < r.x+r.w; x++ {
				if y >= mainH || x >= w || y < 0 || x < 0 {
					t.Fatalf("panel %d reaches outside the main area: %+v", i, r)
				}
				cells[y][x]++
			}
		}
	}
	for y := range cells {
		for x := range cells[y] {
			if cells[y][x] != 1 {
				t.Fatalf("cell (%d,%d) is covered %d times", x, y, cells[y][x])
			}
		}
	}
}

func TestGeometryNormal(t *testing.T) {
	g := computeGeometry(120, 40, 0.45, modeFull, false, 0, true, [numPanels]int{})
	if !g.ok {
		t.Fatal("120x40 must fit")
	}
	if g.panels[panelProjects] != (rect{0, 0, 54, 39}) {
		t.Errorf("projects = %+v", g.panels[panelProjects])
	}
	ed, op := g.panels[panelEditors], g.panels[panelOptions]
	if ed.x != 54 || ed.w != 66 || op.x != 54 || op.w != 66 || ed.y != 0 || op.y != ed.h || ed.h+op.h != 39 {
		t.Errorf("editors %+v options %+v", ed, op)
	}
	if ed.h <= op.h {
		t.Errorf("the Editors panel is the big one: %d vs %d", ed.h, op.h)
	}
	if g.bar != (rect{0, 39, 120, 1}) {
		t.Errorf("bar = %+v", g.bar)
	}
	if !g.tasks.empty() {
		t.Errorf("no tasks, but tasks = %+v", g.tasks)
	}
	covered(t, g, 120, 39)
}

func TestGeometryTasksFrameAndNoBar(t *testing.T) {
	g := computeGeometry(120, 40, 0.45, modeFull, false, 3, true, [numPanels]int{})
	if g.tasks != (rect{0, 36, 120, 3}) || g.bar != (rect{0, 39, 120, 1}) {
		t.Errorf("tasks %+v bar %+v", g.tasks, g.bar)
	}
	covered(t, g, 120, 36)

	g = computeGeometry(120, 40, 0.45, modeFull, false, 0, false, [numPanels]int{})
	if !g.bar.empty() {
		t.Errorf("bar = %+v", g.bar)
	}
	covered(t, g, 120, 40)
}

func TestGeometryModes(t *testing.T) {
	proj := computeGeometry(100, 30, 0.45, modeProjects, false, 0, true, [numPanels]int{})
	if proj.panels[panelProjects] != (rect{0, 0, 100, 29}) || !proj.panels[panelEditors].empty() || !proj.panels[panelOptions].empty() {
		t.Errorf("projects mode: %+v", proj.panels)
	}
	eds := computeGeometry(100, 30, 0.45, modeEditors, false, 0, true, [numPanels]int{})
	if !eds.panels[panelProjects].empty() || eds.panels[panelEditors].w != 100 || eds.panels[panelOptions].w != 100 ||
		eds.panels[panelEditors].y != 0 || eds.panels[panelOptions].y != eds.panels[panelEditors].h {
		t.Errorf("editors mode: %+v", eds.panels)
	}
	covered(t, eds, 100, 29)
	covered(t, proj, 100, 29)
}

func TestGeometryTooSmall(t *testing.T) {
	for _, sz := range [][2]int{{minScreenW - 1, 40}, {120, 10}, {20, 5}, {0, 0}} {
		if g := computeGeometry(sz[0], sz[1], 0.45, modeFull, false, 0, true, [numPanels]int{}); g.ok {
			t.Errorf("%dx%d must be too small", sz[0], sz[1])
		}
	}
	// a task frame eats into the height the panels need
	if g := computeGeometry(120, 14, 0.45, modeFull, false, 3, true, [numPanels]int{}); g.ok {
		t.Error("14 rows with a 3-row task frame and a hint line leaves too little")
	}
	if g := computeGeometry(minScreenW, minScreenH+1, 0.45, modeFull, false, 0, true, [numPanels]int{}); !g.ok {
		t.Error("the minimum size must fit")
	}
}

func TestGeometrySmallestStillTiles(t *testing.T) {
	g := computeGeometry(minScreenW, minScreenH+1, 0.45, modeFull, false, 0, true, [numPanels]int{})
	covered(t, g, minScreenW, minScreenH)
	for i, r := range g.panels {
		if r.h < 3 || r.w < 20 {
			t.Errorf("panel %d is too small to use: %+v", i, r)
		}
	}
}

func TestGeometrySidePanelWidthIsBounded(t *testing.T) {
	narrow := computeGeometry(100, 30, 0.2, modeFull, false, 0, true, [numPanels]int{})
	if narrow.panels[panelProjects].w < 28 {
		t.Errorf("projects panel = %d wide", narrow.panels[panelProjects].w)
	}
	wide := computeGeometry(100, 30, 0.8, modeFull, false, 0, true, [numPanels]int{})
	if wide.panels[panelEditors].w < 24 {
		t.Errorf("right column = %d wide", wide.panels[panelEditors].w)
	}
}

func TestPanelAtAndRect(t *testing.T) {
	g := computeGeometry(120, 40, 0.45, modeFull, false, 0, true, [numPanels]int{})
	cases := []struct{ x, y, want int }{
		{0, 0, panelProjects}, {53, 38, panelProjects},
		{54, 0, panelEditors}, {119, 5, panelEditors},
		{60, g.panels[panelOptions].y, panelOptions}, {119, 38, panelOptions},
		{10, 39, -1}, // the hint line
		{200, 5, -1},
	}
	for _, c := range cases {
		if got := g.panelAt(c.x, c.y); got != c.want {
			t.Errorf("panelAt(%d,%d) = %d, want %d", c.x, c.y, got, c.want)
		}
	}
	if (rect{}).contains(0, 0) || !(rect{1, 1, 2, 2}).contains(2, 2) || (rect{1, 1, 2, 2}).contains(3, 2) {
		t.Error("rect.contains")
	}
}

func TestParseScreenMode(t *testing.T) {
	if parseScreenMode("projects") != modeProjects || parseScreenMode("editors") != modeEditors ||
		parseScreenMode("full") != modeFull || parseScreenMode("whatever") != modeFull {
		t.Error("parseScreenMode")
	}
	if modeFull.String() != "full" || modeProjects.String() != "projects" || modeEditors.String() != "editors" {
		t.Error("String")
	}
}

func TestFirstPanelIsOnScreen(t *testing.T) {
	for _, mode := range []screenMode{modeFull, modeProjects, modeEditors} {
		if p := mode.firstPanel(); !mode.shows(p) {
			t.Errorf("%s: first panel %d is not on screen", mode, p)
		}
	}
}

func TestGeometryOptionsTakesEmptyEditorRows(t *testing.T) {
	// 39 main rows: the share gives Editors 25 and Options 14
	split := func(edNeed, opNeed int) (int, int) {
		var need [numPanels]int
		need[panelEditors], need[panelOptions] = edNeed, opNeed
		g := computeGeometry(120, 40, 0.45, modeFull, false, 0, true, need)
		covered(t, g, 120, 39)
		return g.panels[panelEditors].h, g.panels[panelOptions].h
	}
	if ed, op := split(3, 10); ed != 25 || op != 14 {
		t.Errorf("Options fits its share: %d/%d", ed, op)
	}
	if ed, op := split(3, 20); ed != 17 || op != 22 {
		t.Errorf("Options grows into the empty rows: %d/%d", ed, op)
	}
	if ed, op := split(20, 30); ed != 22 || op != 17 {
		t.Errorf("Editors keeps its content: %d/%d", ed, op)
	}
	if ed, op := split(40, 30); ed != 25 || op != 14 {
		t.Errorf("a full Editors panel keeps its share: %d/%d", ed, op)
	}
	if ed, op := split(1, 60); ed != 4 || op != 35 {
		t.Errorf("Editors keeps its smallest size: %d/%d", ed, op)
	}
}

func TestGeometryPortrait(t *testing.T) {
	g := computeGeometry(80, 50, 0.45, modeFull, true, 3, true, [numPanels]int{})
	if !g.ok {
		t.Fatal("80x50 must fit")
	}
	pr, ed, op := g.panels[panelProjects], g.panels[panelEditors], g.panels[panelOptions]
	for i, r := range g.panels {
		if r.x != 0 || r.w != 80 {
			t.Errorf("panel %d does not span the width: %+v", i, r)
		}
	}
	if pr.y != 0 || pr.h != 21 || ed.y != pr.h || op.y != ed.y+ed.h || ed.h <= op.h {
		t.Errorf("projects %+v editors %+v options %+v", pr, ed, op)
	}
	covered(t, g, 80, 46)

	// one column fits where two columns do not
	if g := computeGeometry(minPortraitW, minScreenH+1, 0.45, modeFull, true, 0, true, [numPanels]int{}); !g.ok {
		t.Error("the smallest portrait screen must fit")
	} else {
		covered(t, g, minPortraitW, minScreenH)
		for i, r := range g.panels {
			if r.h < 3 {
				t.Errorf("panel %d is too small to use: %+v", i, r)
			}
		}
	}
	if g := computeGeometry(minPortraitW-1, 50, 0.45, modeFull, true, 0, true, [numPanels]int{}); g.ok {
		t.Error("narrower than the portrait minimum must not fit")
	}
	// the other screen modes are one column already
	eds := computeGeometry(80, 50, 0.45, modeEditors, true, 0, true, [numPanels]int{})
	if !eds.panels[panelProjects].empty() || eds.panels[panelEditors].y != 0 {
		t.Errorf("editors mode: %+v", eds.panels)
	}
}
