package unity

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
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

// LockFile is where Unity records every package it resolved for a project,
// with those pulled in by other packages.
var LockFile = filepath.Join("Packages", "packages-lock.json")

// LockedPackage is a package as packages-lock.json records it.
type LockedPackage struct {
	Version string `json:"version"`
	// Depth is 0 for a package of the manifest, more for one that another
	// package pulled in.
	Depth  int    `json:"depth"`
	Source string `json:"source"` // such as registry, git, embedded, builtin
	// Dependencies are the packages it needs, by name, with their versions.
	Dependencies map[string]string `json:"dependencies"`
}

// ReadPackagesLock returns the packages Unity resolved for the project at
// path, by name, from Packages/packages-lock.json. A project Unity has not
// resolved yet has no lock file: that is no packages, and no error.
func ReadPackagesLock(path string) (map[string]LockedPackage, error) {
	b, err := os.ReadFile(filepath.Join(path, LockFile))
	if errors.Is(err, fs.ErrNotExist) {
		return map[string]LockedPackage{}, nil
	}
	if err != nil {
		return nil, err
	}
	var doc struct {
		Dependencies map[string]LockedPackage `json:"dependencies"`
	}
	if err := json.Unmarshal(b, &doc); err != nil {
		return nil, fmt.Errorf("%s: %w", filepath.Join(path, LockFile), err)
	}
	if doc.Dependencies == nil {
		doc.Dependencies = map[string]LockedPackage{}
	}
	return doc.Dependencies, nil
}
