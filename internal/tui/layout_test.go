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
	g := computeGeometry(120, 40, 0.45, modeFull, 0, true)
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
	g := computeGeometry(120, 40, 0.45, modeFull, 3, true)
	if g.tasks != (rect{0, 36, 120, 3}) || g.bar != (rect{0, 39, 120, 1}) {
		t.Errorf("tasks %+v bar %+v", g.tasks, g.bar)
	}
	covered(t, g, 120, 36)

	g = computeGeometry(120, 40, 0.45, modeFull, 0, false)
	if !g.bar.empty() {
		t.Errorf("bar = %+v", g.bar)
	}
	covered(t, g, 120, 40)
}

func TestGeometryModes(t *testing.T) {
	proj := computeGeometry(100, 30, 0.45, modeProjects, 0, true)
	if proj.panels[panelProjects] != (rect{0, 0, 100, 29}) || !proj.panels[panelEditors].empty() || !proj.panels[panelOptions].empty() {
		t.Errorf("projects mode: %+v", proj.panels)
	}
	eds := computeGeometry(100, 30, 0.45, modeEditors, 0, true)
	if !eds.panels[panelProjects].empty() || eds.panels[panelEditors].w != 100 || eds.panels[panelOptions].w != 100 ||
		eds.panels[panelEditors].y != 0 || eds.panels[panelOptions].y != eds.panels[panelEditors].h {
		t.Errorf("editors mode: %+v", eds.panels)
	}
	covered(t, eds, 100, 29)
	covered(t, proj, 100, 29)
}

func TestGeometryTooSmall(t *testing.T) {
	for _, sz := range [][2]int{{minScreenW - 1, 40}, {120, 10}, {20, 5}, {0, 0}} {
		if g := computeGeometry(sz[0], sz[1], 0.45, modeFull, 0, true); g.ok {
			t.Errorf("%dx%d must be too small", sz[0], sz[1])
		}
	}
	// a task frame eats into the height the panels need
	if g := computeGeometry(120, 14, 0.45, modeFull, 3, true); g.ok {
		t.Error("14 rows with a 3-row task frame and a hint line leaves too little")
	}
	if g := computeGeometry(minScreenW, minScreenH+1, 0.45, modeFull, 0, true); !g.ok {
		t.Error("the minimum size must fit")
	}
}

func TestGeometrySmallestStillTiles(t *testing.T) {
	g := computeGeometry(minScreenW, minScreenH+1, 0.45, modeFull, 0, true)
	covered(t, g, minScreenW, minScreenH)
	for i, r := range g.panels {
		if r.h < 3 || r.w < 20 {
			t.Errorf("panel %d is too small to use: %+v", i, r)
		}
	}
}

func TestGeometrySidePanelWidthIsBounded(t *testing.T) {
	narrow := computeGeometry(100, 30, 0.2, modeFull, 0, true)
	if narrow.panels[panelProjects].w < 28 {
		t.Errorf("projects panel = %d wide", narrow.panels[panelProjects].w)
	}
	wide := computeGeometry(100, 30, 0.8, modeFull, 0, true)
	if wide.panels[panelEditors].w < 24 {
		t.Errorf("right column = %d wide", wide.panels[panelEditors].w)
	}
}

func TestPanelAtAndRect(t *testing.T) {
	g := computeGeometry(120, 40, 0.45, modeFull, 0, true)
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
