package tui

import (
	"path/filepath"
	"sort"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/ManeFunction/lazyunity/internal/state"
	"github.com/ManeFunction/lazyunity/internal/unity"
	"github.com/ManeFunction/lazyunity/internal/unitycli"
)

// ---- projects --------------------------------------------------------------

// projectRow is a project as the list shows it.
type projectRow struct {
	unitycli.Project
	// Missing is set when the project's folder is gone.
	Missing bool
	// Pending is the version or platform chosen but not applied yet.
	Pending state.Pending
}

func (r projectRow) name() string {
	if r.Title != "" {
		return r.Title
	}
	return filepath.Base(r.Path)
}

// effVersion is the Editor version the project opens with.
func (r projectRow) effVersion() string {
	if r.Pending.Version != "" {
		return r.Pending.Version
	}
	return r.Version
}

// effTarget is the build target the project is in, or will be switched to.
func (r projectRow) effTarget() string {
	if r.Pending.BuildTarget != "" {
		return r.Pending.BuildTarget
	}
	return r.BuildTarget
}

func (r projectRow) versionChanged() bool {
	return r.Pending.Version != "" && r.Pending.Version != r.Version
}

func (r projectRow) targetChanged() bool {
	return r.Pending.BuildTarget != "" && r.Pending.BuildTarget != r.BuildTarget
}

// sortProjects orders the list: pinned first, then by the chosen order.
func sortProjects(rows []projectRow, order string) {
	sort.SliceStable(rows, func(i, j int) bool {
		a, b := rows[i], rows[j]
		if a.IsFavorite != b.IsFavorite {
			return a.IsFavorite
		}
		switch order {
		case state.SortName:
			return strings.ToLower(a.name()) < strings.ToLower(b.name())
		case state.SortVersion:
			if c := unity.CompareStrings(a.effVersion(), b.effVersion()); c != 0 {
				return c > 0
			}
			return strings.ToLower(a.name()) < strings.ToLower(b.name())
		}
		return a.LastModified > b.LastModified
	})
}

// projectCols are the column widths of the project list.
type projectCols struct{ opened, version, platform int }

// measureProjects sizes the columns to the widest cell.
func (m *Model) measureProjects(rows []projectRow) projectCols {
	var opened, version, platform []string
	for _, r := range rows {
		opened = append(opened, m.openedText(r))
		version = append(version, versionCell(r))
		platform = append(platform, platformCell(r))
	}
	return projectCols{
		opened:   maxWidth(opened, 9),
		version:  maxWidth(version, 26),
		platform: maxWidth(platform, 18),
	}
}

func (m *Model) openedText(r projectRow) string {
	if r.Missing {
		return "✗ missing"
	}
	return relTime(m.now(), r.LastOpened())
}

func versionCell(r projectRow) string {
	if r.versionChanged() {
		return r.Version + "→" + r.Pending.Version
	}
	return r.Version
}

func platformCell(r projectRow) string {
	cur := unity.PlatformLabel(r.BuildTarget)
	if r.targetChanged() {
		return cur + "→" + unity.PlatformLabel(r.Pending.BuildTarget)
	}
	return cur
}

// drawProject draws one row of the project list.
func (m *Model) drawProject(r projectRow, w int, base lipgloss.Style, cols projectCols) string {
	th := m.th
	// drop columns, least important first, until the name has room
	const minName = 10
	showPlatform, showOpened := true, true
	need := func() int {
		n := 2 + cols.version + 1
		if showOpened {
			n += cols.opened + 1
		}
		if showPlatform {
			n += cols.platform + 1
		}
		return n
	}
	if w-need() < minName {
		showPlatform = false
	}
	if w-need() < minName {
		showOpened = false
	}
	nameW := max(w-need(), 1)

	row := newRow(base)
	if r.IsFavorite {
		row.text("★ ", th.Pinned)
	} else {
		row.gap(2)
	}
	nameStyle := th.Text
	if r.Missing {
		nameStyle = th.Dim
	}
	row.add(r.name(), nameStyle, nameW).gap(1)

	if showOpened {
		st := th.Dim
		if r.Missing {
			st = th.Missing
		}
		row.add(m.openedText(r), st, cols.opened).gap(1)
	}

	verStyle := th.Text
	switch {
	case r.versionChanged():
		verStyle = th.Pending
	case !m.editorInstalled(r.effVersion()):
		verStyle = th.Missing
	}
	row.add(versionCell(r), verStyle, cols.version)

	if showPlatform {
		platStyle := th.Text
		if r.targetChanged() {
			platStyle = th.Pending
		}
		row.gap(1).add(platformCell(r), platStyle, cols.platform)
	}
	return row.done(w)
}

// ---- editors ---------------------------------------------------------------

// editorRow is an installed Editor as the list shows it.
type editorRow struct {
	Version   string
	Arch      string
	Location  string // the executable
	Dir       string // the version folder, "" if unknown
	Default   bool
	LTS       bool
	Platforms []unity.Platform
}

func (r editorRow) key() string { return r.Location }

func (r editorRow) platformLabels() string {
	var out []string
	for _, p := range r.Platforms {
		out = append(out, p.Label)
	}
	return strings.Join(out, ", ")
}

type editorCols struct{ version, arch int }

func measureEditors(rows []editorRow) editorCols {
	var v, a []string
	for _, r := range rows {
		v = append(v, r.Version)
		a = append(a, r.Arch)
	}
	return editorCols{version: maxWidth(v, 16), arch: maxWidth(a, 8)}
}

func (m *Model) drawEditor(r editorRow, w int, base lipgloss.Style, cols editorCols) string {
	th := m.th
	row := newRow(base)
	if r.Default {
		row.text("★ ", th.Pinned)
	} else {
		row.gap(2)
	}
	row.add(r.Version, th.Text, cols.version).gap(1)
	if r.LTS {
		row.add("LTS", th.Success, 3)
	} else {
		row.gap(3)
	}
	row.gap(1).add(r.Arch, th.Dim, cols.arch).gap(1)
	rest := w - row.used
	if rest > 0 {
		row.add(r.platformLabels(), th.Dim, rest)
	}
	return row.done(w)
}

// mergeEditors combines what a scan of the disk found with what the CLI
// reports. The scan decides what is installed; the CLI adds the default Editor,
// the real version of an Editor whose folder is not named after it, and the
// Editors that live outside every root.
func mergeEditors(local []unity.Installed, cli []unitycli.Editor, goos string) []editorRow {
	byLoc := map[string]*editorRow{}
	var rows []*editorRow
	for _, e := range local {
		r := &editorRow{
			Version: e.Version, Arch: e.Architecture, Location: e.Location, Dir: e.Dir, LTS: e.LTS,
			Platforms: unity.InstalledPlatforms(e.Dir, goos),
		}
		byLoc[e.Location] = r
		rows = append(rows, r)
	}
	for _, e := range cli {
		if r, ok := byLoc[e.Location]; ok {
			r.Default = e.Default
			if e.Version != "" {
				r.Version = e.Version // the scan only knows the folder's name
			}
			if r.Arch == "" {
				r.Arch = e.Architecture
			}
			continue
		}
		dir := unity.VersionDir(e.Location, goos)
		r := &editorRow{
			Version: e.Version, Arch: e.Architecture, Location: e.Location, Dir: dir, Default: e.Default,
		}
		if dir != "" {
			r.Platforms = unity.InstalledPlatforms(dir, goos)
		}
		byLoc[e.Location] = r
		rows = append(rows, r)
	}
	sort.SliceStable(rows, func(i, j int) bool { return unity.CompareStrings(rows[i].Version, rows[j].Version) > 0 })
	out := make([]editorRow, len(rows))
	for i, r := range rows {
		out[i] = *r
	}
	return out
}

// ---- modules ---------------------------------------------------------------

func (m *Model) drawModule(mod unitycli.Module, w int, base lipgloss.Style, cols moduleCols) string {
	th := m.th
	row := newRow(base)
	switch {
	case mod.IsInstalled:
		row.text("[✓] ", th.Success)
	case m.moduleQueued(mod):
		row.text("[↓] ", th.Pending) // in the queue
	default:
		row.text("[ ] ", th.Dim)
	}
	nameW := max(w-4-cols.category-cols.size-2, 8)
	row.add(mod.Name, th.Text, nameW).gap(1)
	row.add(mod.Category, th.Dim, cols.category).gap(1)
	size := ""
	if !mod.IsInstalled && mod.DownloadBytes > 0 {
		size = humanBytes(mod.DownloadBytes)
	} else if mod.InstalledBytes > 0 {
		size = humanBytes(mod.InstalledBytes)
	}
	row.add(padLeft(size, cols.size), th.Dim, cols.size)
	return row.done(w)
}

// moduleQueued reports whether a task is installing or removing a module of
// the Editor drilled into.
func (m *Model) moduleQueued(mod unitycli.Module) bool {
	_, e, ok := m.modulesTab()
	return ok && m.taskOn(workModule(e.Version, mod.ID)) != nil
}

type moduleCols struct{ category, size int }

func measureModules(mods []unitycli.Module) moduleCols {
	var c, s []string
	for _, m := range mods {
		c = append(c, m.Category)
		s = append(s, humanBytes(max(m.DownloadBytes, m.InstalledBytes)))
	}
	return moduleCols{category: maxWidth(c, 16), size: maxWidth(s, 9)}
}

func padLeft(s string, w int) string {
	if n := w - width(s); n > 0 {
		return strings.Repeat(" ", n) + s
	}
	return s
}

// sortModules puts platforms first, then the rest by name.
func sortModules(mods []unitycli.Module) {
	rank := func(m unitycli.Module) int {
		if m.Category == "Platforms" {
			return 0
		}
		return 1
	}
	sort.SliceStable(mods, func(i, j int) bool {
		if rank(mods[i]) != rank(mods[j]) {
			return rank(mods[i]) < rank(mods[j])
		}
		return strings.ToLower(mods[i].Name) < strings.ToLower(mods[j].Name)
	})
}

// ---- releases --------------------------------------------------------------

// streamLabel names a release's stream for display and says how to colour it.
func (th *Theme) streamLabel(r unitycli.Release) (string, lipgloss.Style) {
	label, known := "", false
	if r.Stream != "" {
		label, known = unitycli.StreamLabel(r.Stream)
	}
	if !known && r.LTS {
		label = "LTS"
	}
	switch label {
	case "LTS":
		return label, th.Success
	case "Beta", "Alpha":
		return label, th.Warning
	}
	return label, th.Dim
}

func (m *Model) drawRelease(r unitycli.Release, w int, base lipgloss.Style) string {
	th := m.th
	row := newRow(base)
	row.add(r.Version, th.Text, 14).gap(1)
	label, st := th.streamLabel(r)
	row.add(label, st, 9).gap(1)
	row.add(r.Architecture, th.Dim, 6)
	if r.Installed {
		row.gap(1).text("✓ installed", th.Success)
	}
	return row.done(w)
}

// ---- roots and tools -------------------------------------------------------

// rootRow is a folder Editors are looked for in.
type rootRow struct {
	Path    string
	Install bool   // where new Editors are installed
	Source  string // "cli", "state" or "config"
	Exists  bool
	Editors int
}

func (m *Model) drawRoot(r rootRow, w int, base lipgloss.Style) string {
	th := m.th
	row := newRow(base)
	if r.Install {
		row.text("★ ", th.Pinned)
	} else {
		row.gap(2)
	}
	var tag string
	switch {
	case !r.Exists:
		tag = "not found"
	case r.Install:
		tag = plural(r.Editors, "editor") + " · install location"
	default:
		tag = plural(r.Editors, "editor")
	}
	tagW := min(width(tag), max(w/2, 10))
	pathW := max(w-2-tagW-1, 1)
	pathStyle := th.Text
	if !r.Exists {
		pathStyle = th.Missing
	}
	row.add(displayPath(r.Path), pathStyle, pathW).gap(1)
	row.add(padLeft(tag, tagW), th.Dim, tagW)
	return row.done(w)
}

// displayPath shortens the home folder to ~.
func displayPath(p string) string {
	if home := homeDir(); home != "" && (p == home || strings.HasPrefix(p, home+string(filepath.Separator))) {
		return "~" + strings.TrimPrefix(p, home)
	}
	return p
}

// tone is how a tool's status is coloured.
type tone int

const (
	tonePlain tone = iota
	toneGood       // green: ready, signed in
	toneWarn       // yellow: something to do
)

// toolRow is a line of the Tools tab.
type toolRow struct {
	id     string
	label  string
	status string
	tone   tone
	// run is what enter does on the row.
	run func(m *Model) tea.Cmd
}

func (m *Model) drawTool(r toolRow, w int, base lipgloss.Style) string {
	th := m.th
	row := newRow(base)
	labelW := 24
	row.add(r.label, th.Text, labelW).gap(1)
	st := th.Dim
	switch r.tone {
	case toneGood:
		st = th.Success
	case toneWarn:
		st = th.Warning
	}
	row.add(r.status, st, max(w-labelW-1, 1))
	return row.done(w)
}
