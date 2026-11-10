// Package state holds what lazyunity remembers between runs: choices made in
// the interface, as opposed to the user-authored config.yml.
package state

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Pending is a change chosen for a project that applies the next time it is
// opened.
type Pending struct {
	// Version is the Editor version to open the project with. Empty: the
	// project's own.
	Version string `yaml:"version,omitempty"`
	// BuildTarget is the Unity BuildTarget enum name to switch to. Empty: leave
	// the project's active target alone.
	BuildTarget string `yaml:"buildTarget,omitempty"`
}

// Empty reports whether nothing is pending.
func (p Pending) Empty() bool { return p.Version == "" && p.BuildTarget == "" }

// Sort orders the project list.
const (
	SortLastOpened = "lastOpened"
	SortName       = "name"
	SortVersion    = "version"
)

// State is the contents of state.yml.
type State struct {
	// ExtraRoots are Editor folders the user added in the Roots tab.
	ExtraRoots []string `yaml:"extraRoots,omitempty"`
	// Pending maps a project path to its pending change.
	Pending map[string]Pending `yaml:"pending,omitempty"`
	// Sort is how projects are ordered; empty means SortLastOpened.
	Sort string `yaml:"sort,omitempty"`
	// DefaultProjectDir is the folder new projects go in, when the user chose one.
	DefaultProjectDir string `yaml:"defaultProjectDir,omitempty"`
	// CLIPromptDismissed is set by "don't ask again" on the Unity CLI prompt.
	CLIPromptDismissed bool `yaml:"cliPromptDismissed,omitempty"`
	// ReleaseStream is the stream the Releases tab lists: all, lts, tech, beta
	// or alpha. Empty means the default, tech (Unity's "Supported" releases).
	ReleaseStream string `yaml:"releaseStream,omitempty"`

	path string
}

// Load reads state.yml from dir. A missing or unreadable file gives an empty
// state: losing a remembered choice must never stop lazyunity from starting.
func Load(dir string) *State {
	s := &State{path: filepath.Join(dir, "state.yml")}
	b, err := os.ReadFile(s.path)
	if err != nil {
		return s
	}
	if yaml.Unmarshal(b, s) != nil {
		*s = State{path: s.path}
	}
	return s
}

// Save writes the state atomically.
func (s *State) Save() error {
	if s.path == "" {
		return errors.New("state has no path")
	}
	b, err := yaml.Marshal(s)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(s.path), "state-*.tmp")
	if err != nil {
		return err
	}
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	if err := os.Rename(tmp.Name(), s.path); err != nil {
		os.Remove(tmp.Name())
		return fmt.Errorf("saving state: %w", err)
	}
	return nil
}

// Path is where the state is stored.
func (s *State) Path() string { return s.path }

// PendingFor returns the pending change of a project.
func (s *State) PendingFor(project string) Pending { return s.Pending[project] }

// SetPending records a pending change, or clears it when p is empty.
func (s *State) SetPending(project string, p Pending) {
	if p.Empty() {
		delete(s.Pending, project)
		return
	}
	if s.Pending == nil {
		s.Pending = map[string]Pending{}
	}
	s.Pending[project] = p
}

// AddRoot remembers an Editor folder. It reports false if it was already known.
func (s *State) AddRoot(path string) bool {
	for _, r := range s.ExtraRoots {
		if r == path {
			return false
		}
	}
	s.ExtraRoots = append(s.ExtraRoots, path)
	return true
}

// RemoveRoot forgets an Editor folder. It reports whether it was known.
func (s *State) RemoveRoot(path string) bool {
	for i, r := range s.ExtraRoots {
		if r == path {
			s.ExtraRoots = append(s.ExtraRoots[:i], s.ExtraRoots[i+1:]...)
			return true
		}
	}
	return false
}

// Release streams, as `unity releases --stream` takes them. StreamAll lists
// every stream; StreamTech is what Unity calls Supported releases.
const (
	StreamAll  = "all"
	StreamTech = "tech"
)

// Stream returns the release stream to list: "" for every stream, else the
// value for --stream. Nothing chosen yet means the Supported (tech) stream.
func (s *State) Stream() string {
	switch s.ReleaseStream {
	case "":
		return StreamTech
	case StreamAll:
		return ""
	}
	return s.ReleaseStream
}

// SetStream remembers the release stream; "" means every stream.
func (s *State) SetStream(stream string) {
	if stream == "" {
		stream = StreamAll
	}
	s.ReleaseStream = stream
}

// SortOrder returns the effective sort, defaulting to SortLastOpened.
func (s *State) SortOrder() string {
	switch s.Sort {
	case SortName, SortVersion:
		return s.Sort
	}
	return SortLastOpened
}
