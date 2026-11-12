package tui

import (
	"sort"

	"github.com/ManeFunction/lazyunity/internal/unity"
	"github.com/ManeFunction/lazyunity/internal/unitycli"
)

// inventory is what lazyunity knows of the installed Editors. Two sources
// tell it: a scan of the Editor folder finds what is on disk, and the CLI
// lists what it knows of, some of which may live elsewhere. The inventory
// keeps both and merges them, knows which folder the Editors live in, and
// remembers which Editors found on disk it has told the CLI about.
//
// It never reads the disk itself: scanEditors and listEditors do, in the
// commands that bring their results, so that the Update loop does not.
type inventory struct {
	goos, home string
	// install is the folder the CLI installs new Editors into, once it said
	// so; until then, the default for the platform.
	install string

	found      []foundEditor
	listed     []listedEditor
	cliListed  bool            // whether the CLI has listed its Editors yet
	registered map[string]bool // Editors told to the CLI, by location

	all      []editorRow
	versions map[string]bool
}

// foundEditor is an Editor a scan found on disk, with its platforms.
type foundEditor struct {
	unity.Installed
	platforms []unity.Platform
}

// listedEditor is an Editor the CLI lists, with its version folder and the
// platforms in it, when its location is an Editor executable.
type listedEditor struct {
	unitycli.Editor
	dir       string
	platforms []unity.Platform
}

func newInventory(goos, home, install string) *inventory {
	return &inventory{goos: goos, home: home, install: install,
		registered: map[string]bool{}, versions: map[string]bool{}}
}

// scanEditors looks for Editors in the folder and reads their platforms.
func scanEditors(goos, root string) []foundEditor {
	return withPlatforms(goos, unity.Scan(goos, []string{root}))
}

// withPlatforms reads the platforms of Editors found on disk.
func withPlatforms(goos string, eds []unity.Installed) []foundEditor {
	out := make([]foundEditor, 0, len(eds))
	for _, e := range eds {
		out = append(out, foundEditor{Installed: e, platforms: unity.InstalledPlatforms(e.Dir, goos)})
	}
	return out
}

// listEditors reads the platforms of the Editors the CLI lists. The CLI's
// locations are first rewritten to the executable's path: on darwin it reports
// the app bundle, and only then do its Editors match what a scan finds.
func listEditors(goos string, eds []unitycli.Editor) []listedEditor {
	out := make([]listedEditor, 0, len(eds))
	for _, e := range eds {
		e.Location = unity.NormalizeLocation(e.Location, goos)
		l := listedEditor{Editor: e, dir: unity.VersionDir(e.Location, goos)}
		if l.dir != "" {
			l.platforms = unity.InstalledPlatforms(l.dir, goos)
		}
		out = append(out, l)
	}
	return out
}

// installRoot is the folder new Editors are installed into, and the only
// folder Editors are looked for in: what the CLI said, else the platform
// default.
func (inv *inventory) installRoot() string {
	if inv.install != "" {
		return inv.install
	}
	return unity.DefaultInstallRoot(inv.goos, inv.home)
}

// setInstallRoot takes in where the CLI installs new Editors.
func (inv *inventory) setInstallRoot(path string) { inv.install = path }

// setFound takes in a scan.
func (inv *inventory) setFound(found []foundEditor) {
	inv.found = found
	inv.merge()
}

// setListed takes in the CLI's list.
func (inv *inventory) setListed(listed []listedEditor) {
	inv.listed, inv.cliListed = listed, true
	inv.merge()
}

func (inv *inventory) merge() {
	inv.all = mergeEditors(inv.found, inv.listed)
	inv.versions = map[string]bool{}
	for _, e := range inv.all {
		inv.versions[e.Version] = true
	}
}

// editors are the installed Editors, newest first.
func (inv *inventory) editors() []editorRow { return inv.all }

// find returns the installed Editor of a version.
func (inv *inventory) find(version string) *editorRow {
	for i := range inv.all {
		if inv.all[i].Version == version {
			return &inv.all[i]
		}
	}
	return nil
}

// has reports whether an Editor of this version is installed.
func (inv *inventory) has(version string) bool { return inv.versions[version] }

// strays are the Editors found on disk that the CLI does not list and has not
// been told about. Before the CLI has listed its own, there are none.
func (inv *inventory) strays() []string {
	if !inv.cliListed {
		return nil
	}
	known := map[string]bool{}
	for _, e := range inv.listed {
		known[e.Location] = true
	}
	var out []string
	for _, e := range inv.found {
		if !known[e.Location] && !inv.registered[e.Location] {
			out = append(out, e.Location)
		}
	}
	return out
}

// told records that the CLI was told about these Editors, so that each is
// tried once.
func (inv *inventory) told(locations []string) {
	for _, l := range locations {
		inv.registered[l] = true
	}
}

// mergeEditors combines what a scan of the disk found with what the CLI
// reports. The scan decides what is installed; the CLI adds the default Editor,
// the real version of an Editor whose folder is not named after it, and the
// Editors that live outside the Editors folder.
func mergeEditors(found []foundEditor, listed []listedEditor) []editorRow {
	byLoc := map[string]*editorRow{}
	var rows []*editorRow
	for _, e := range found {
		r := &editorRow{
			Version: e.Version, Arch: e.Architecture, Location: e.Location, Dir: e.Dir, LTS: e.LTS,
			Platforms: e.platforms,
		}
		byLoc[e.Location] = r
		rows = append(rows, r)
	}
	for _, e := range listed {
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
		r := &editorRow{
			Version: e.Version, Arch: e.Architecture, Location: e.Location, Dir: e.dir, Default: e.Default,
			Platforms: e.platforms,
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
