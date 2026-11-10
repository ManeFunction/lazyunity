package unity

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// LocalModule is a module of an installed Editor, as recorded in the version
// folder's modules.json.
type LocalModule struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Category string `json:"category"`
	Parent   string `json:"parent"`
	Selected bool   `json:"selected"` // installed
	Visible  bool   `json:"visible"`
	Hidden   bool   `json:"hidden"`
}

// ReadModules reads <versionDir>/modules.json. Child modules appear in the file
// twice, nested and again with Parent set; only the flat entries are returned.
func ReadModules(versionDir string) ([]LocalModule, error) {
	b, err := os.ReadFile(filepath.Join(versionDir, "modules.json"))
	if err != nil {
		return nil, err
	}
	var mods []LocalModule
	if err := json.Unmarshal(b, &mods); err != nil {
		return nil, err
	}
	return mods, nil
}

// PlaybackEnginesDir is where an Editor keeps its platform support.
func PlaybackEnginesDir(versionDir, goos string) string {
	if goos == "darwin" {
		return filepath.Join(versionDir, "Unity.app", "Contents", "PlaybackEngines")
	}
	return filepath.Join(versionDir, "Editor", "Data", "PlaybackEngines")
}

// InstalledPlatforms lists the platforms an Editor can build for: the one the
// Editor's own operating system builds natively, plus every platform whose
// module is installed. It reads modules.json and checks the PlaybackEngines
// folder, so it also works for Editors that were installed by hand and have no
// modules.json. The result follows the order of Platforms.
func InstalledPlatforms(versionDir, goos string) []Platform {
	installed := map[string]bool{}

	if mods, err := ReadModules(versionDir); err == nil {
		for _, m := range mods {
			if !m.Selected || m.Parent != "" {
				continue
			}
			if p, ok := PlatformForModule(m.ID); ok {
				installed[p.ID] = true
			}
		}
	}

	engines := PlaybackEnginesDir(versionDir, goos)
	for _, p := range Platforms {
		if p.PlaybackEngine == "" {
			continue
		}
		if entries, err := os.ReadDir(filepath.Join(engines, p.PlaybackEngine)); err == nil && len(entries) > 0 {
			installed[p.ID] = true
		}
	}

	var out []Platform
	for _, p := range Platforms {
		if installed[p.ID] || p.HostOS == goos {
			out = append(out, p)
		}
	}
	return out
}
