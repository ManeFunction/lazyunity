package unity

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Installed is an Editor found on disk.
type Installed struct {
	Version      string
	Dir          string // the version folder: <root>/<version>
	Location     string // the Editor executable
	Root         string // the install root it was found under
	Architecture string // x86_64 or arm64, "" if unknown
	LTS          bool
}

// ExecutableRel is the Editor executable's path inside a version folder.
func ExecutableRel(goos string) string {
	switch goos {
	case "windows":
		return filepath.Join("Editor", "Unity.exe")
	case "darwin":
		return filepath.Join("Unity.app", "Contents", "MacOS", "Unity")
	default:
		return filepath.Join("Editor", "Unity")
	}
}

// DefaultInstallRoot is where Unity's tools install Editors unless told
// otherwise.
func DefaultInstallRoot(goos, home string) string {
	switch goos {
	case "windows":
		return `C:\Program Files\Unity\Hub\Editor`
	case "darwin":
		return "/Applications/Unity/Hub/Editor"
	default:
		return filepath.Join(home, "Unity", "Hub", "Editor")
	}
}

// VersionDir returns the version folder that contains an Editor executable
// (the folder that also holds modules.json), or "" if location is not an
// Editor executable for goos. location must use the host's path separator.
func VersionDir(location, goos string) string {
	rel := ExecutableRel(goos)
	if !strings.HasSuffix(location, string(filepath.Separator)+rel) {
		return ""
	}
	return strings.TrimSuffix(location, string(filepath.Separator)+rel)
}

// Scan finds the Editors installed under the given roots: every folder that
// directly contains an Editor for goos. The same Editor found under two roots
// is listed once. The result is sorted newest first.
func Scan(goos string, roots []string) []Installed {
	rel := ExecutableRel(goos)
	seen := map[string]bool{}
	var out []Installed
	for _, root := range roots {
		entries, err := os.ReadDir(root)
		if err != nil {
			continue // a missing root is normal, e.g. before the first install
		}
		for _, e := range entries {
			dir := filepath.Join(root, e.Name())
			if st, err := os.Stat(dir); err != nil || !st.IsDir() { // follows symlinks
				continue
			}
			exe := filepath.Join(dir, rel)
			if st, err := os.Stat(exe); err != nil || st.IsDir() {
				continue
			}
			if seen[exe] {
				continue
			}
			seen[exe] = true
			ed := Installed{Version: e.Name(), Dir: dir, Location: exe, Root: root}
			readMetadata(&ed)
			out = append(out, ed)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return CompareStrings(out[i].Version, out[j].Version) > 0 })
	return out
}

// readMetadata fills in what <version>/metadata.hub.json knows.
func readMetadata(ed *Installed) {
	b, err := os.ReadFile(filepath.Join(ed.Dir, "metadata.hub.json"))
	if err != nil {
		return
	}
	var m struct {
		Architecture string `json:"architecture"`
		IsLTS        *bool  `json:"isLTS"`
	}
	if json.Unmarshal(b, &m) != nil {
		return
	}
	ed.Architecture = m.Architecture
	ed.LTS = m.IsLTS != nil && *m.IsLTS
}
