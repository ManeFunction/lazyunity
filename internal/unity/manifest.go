package unity

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// ManifestFile is where a project lists the packages it depends on.
var ManifestFile = filepath.Join("Packages", "manifest.json")

// ReadManifest returns the packages the project at path depends on, by name,
// with the version (or URL) each is asked for, from Packages/manifest.json.
func ReadManifest(path string) (map[string]string, error) {
	b, err := os.ReadFile(filepath.Join(path, ManifestFile))
	if err != nil {
		return nil, err
	}
	var doc struct {
		Dependencies map[string]string `json:"dependencies"`
	}
	if err := json.Unmarshal(b, &doc); err != nil {
		return nil, fmt.Errorf("%s: %w", filepath.Join(path, ManifestFile), err)
	}
	if doc.Dependencies == nil {
		doc.Dependencies = map[string]string{}
	}
	return doc.Dependencies, nil
}
