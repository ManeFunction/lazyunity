package tui

import (
	"os"
	"path/filepath"
	"sort"

	"github.com/ManeFunction/lazyunity/internal/config"
	"github.com/ManeFunction/lazyunity/internal/unity"
	"github.com/ManeFunction/lazyunity/internal/unitycli"
)

// inventory is what lazyunity knows of the installed Editors. Two sources
// tell it: a scan of the Editor folders finds what is on disk, and the CLI
// lists what it knows of, some of which may live elsewhere. The inventory
// keeps both and merges them, knows which folders the Editors are looked for
// in, and remembers which Editors found on disk it has told the CLI about.
//
// It never reads the disk itself: scanEditors and listEditors do, in the
// commands that bring their results, so that the Update loop does not.
type inventory struct {
	goos, home string
	// install is the folder the CLI installs new Editors into, once it said
	// so; until then, the default for the platform.
	install string
	// extra lists the folders added in the Roots tab and in config.yml.
	extra func() []string

	found      []foundEditor
	missing    map[string]bool // the Editor folders the last scan could not read
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

func newInventory(goos, home, install string, extra func() []string) *inventory {
	return &inventory{goos: goos, home: home, install: install, extra: extra,
		missing: map[string]bool{}, registered: map[string]bool{}, versions: map[string]bool{}}
}

// scanEditors looks for Editors in the roots and reads their platforms. It
// also reports the roots it could not read.
func scanEditors(goos string, roots []string) (found []foundEditor, missing map[string]bool) {
	missing = map[string]bool{}
	for _, r := range roots {
		if _, err := os.Stat(r); err != nil {
			missing[r] = true
		}
	}
	return withPlatforms(goos, unity.Scan(goos, roots)), missing
}

// withPlatforms reads the platforms of Editors found on disk.
func withPlatforms(goos string, eds []unity.Installed) []foundEditor {
	out := make([]foundEditor, 0, len(eds))
	for _, e := range eds {
		out = append(out, foundEditor{Installed: e, platforms: unity.InstalledPlatforms(e.Dir, goos)})
	}
	return out
}

// listEditors reads the platforms of the Editors the CLI lists.
func listEditors(goos string, eds []unitycli.Editor) []listedEditor {
	out := make([]listedEditor, 0, len(eds))
	for _, e := range eds {
		l := listedEditor{Editor: e, dir: unity.VersionDir(e.Location, goos)}
		if l.dir != "" {
			l.platforms = unity.InstalledPlatforms(l.dir, goos)
		}
		out = append(out, l)
	}
	return out
}

// roots is every folder Editors are looked for in: the install location
// first, then the extra folders, each once.
func (inv *inventory) roots() []string {
	var out []string
	seen := map[string]bool{}
	add := func(p string) {
		if p = filepath.Clean(config.ExpandHome(p)); p != "." && !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}
	if inv.install != "" {
		add(inv.install)
	} else {
		add(unity.DefaultInstallRoot(inv.goos, inv.home))
	}
	if inv.extra != nil {
		for _, r := range inv.extra() {
			add(r)
		}
	}
	return out
}

// setInstallRoot takes in where the CLI installs new Editors.
func (inv *inventory) setInstallRoot(path string) { inv.install = path }

// setFound takes in a scan.
func (inv *inventory) setFound(found []foundEditor, missing map[string]bool) {
	inv.found, inv.missing = found, missing
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

// exists reports whether the last scan could read a root. A root not scanned
// yet is taken to be there.
func (inv *inventory) exists(root string) bool { return !inv.missing[root] }

// foundIn counts the Editors the last scan found in a root.
func (inv *inventory) foundIn(root string) int {
	n := 0
	for _, e := range inv.found {
		if filepath.Clean(e.Root) == root {
			n++
		}
	}
	return n
}

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
// Editors that live outside every root.
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
