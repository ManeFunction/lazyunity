package unity

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// ManifestFile is where a project lists the packages it depends on.
var ManifestFile = filepath.Join("Packages", "manifest.json")

// Manifest is what Packages/manifest.json says.
type Manifest struct {
	// Dependencies are the packages the project depends on, by name, with
	// the version (or URL) each is asked for.
	Dependencies map[string]string `json:"dependencies"`
	// ScopedRegistries are the registries packages come from, other than
	// Unity's own.
	ScopedRegistries []ScopedRegistry `json:"scopedRegistries"`
}

// ScopedRegistry is a registry of the manifest, with the package scopes
// that come from it.
type ScopedRegistry struct {
	Name   string   `json:"name"`
	URL    string   `json:"url"`
	Scopes []string `json:"scopes"`
}

// FromRegistry reports whether the package id comes from the registry at
// url: one of its scopes is the name itself, or a prefix of it up to a dot.
func (mf Manifest) FromRegistry(id, url string) bool {
	for _, r := range mf.ScopedRegistries {
		if strings.TrimRight(r.URL, "/") != strings.TrimRight(url, "/") {
			continue
		}
		for _, s := range r.Scopes {
			if id == s || strings.HasPrefix(id, s+".") {
				return true
			}
		}
	}
	return false
}

// LoadManifest reads Packages/manifest.json of the project at path.
func LoadManifest(path string) (Manifest, error) {
	b, err := os.ReadFile(filepath.Join(path, ManifestFile))
	if err != nil {
		return Manifest{}, err
	}
	var doc Manifest
	if err := json.Unmarshal(b, &doc); err != nil {
		return Manifest{}, fmt.Errorf("%s: %w", filepath.Join(path, ManifestFile), err)
	}
	if doc.Dependencies == nil {
		doc.Dependencies = map[string]string{}
	}
	return doc, nil
}

// ReadManifest returns the packages the project at path depends on, by name,
// with the version (or URL) each is asked for, from Packages/manifest.json.
func ReadManifest(path string) (map[string]string, error) {
	mf, err := LoadManifest(path)
	return mf.Dependencies, err
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
	// URL is the registry a package of source registry came from.
	URL string `json:"url"`
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
