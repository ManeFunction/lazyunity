package unitycli

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// Env is everything locating the CLI needs from the machine, so that every
// operating system's layout can be tested on any host.
type Env struct {
	GOOS     string
	Home     string
	Getenv   func(string) string
	ReadFile func(string) ([]byte, error)
	IsFile   func(string) bool
	LookPath func(string) (string, error)
}

// DefaultEnv describes the machine lazyunity is running on.
func DefaultEnv() Env {
	home, _ := os.UserHomeDir()
	return Env{
		GOOS:     runtime.GOOS,
		Home:     home,
		Getenv:   os.Getenv,
		ReadFile: os.ReadFile,
		IsFile: func(p string) bool {
			st, err := os.Stat(p)
			return err == nil && st.Mode().IsRegular()
		},
		LookPath: exec.LookPath,
	}
}

func (e Env) join(parts ...string) string {
	sep := "/"
	if e.GOOS == "windows" {
		sep = `\`
	}
	return strings.Join(parts, sep)
}

// HubDataDir is the folder where Unity's tools keep their data (the folder the
// official Hub also uses): cli-install.json lives there.
func (e Env) HubDataDir() string {
	switch e.GOOS {
	case "windows":
		base := e.Getenv("APPDATA")
		if base == "" {
			base = e.join(e.Home, "AppData", "Roaming")
		}
		return e.join(base, "UnityHub")
	case "darwin":
		return e.join(e.Home, "Library", "Application Support", "UnityHub")
	default:
		base := e.Getenv("XDG_CONFIG_HOME")
		if base == "" {
			base = e.join(e.Home, ".config")
		}
		return e.join(base, "unityhub")
	}
}

// DefaultCLIPath is where Unity's installer puts the CLI.
func (e Env) DefaultCLIPath() string {
	exe := "unity"
	if e.GOOS == "windows" {
		exe = "unity.exe"
	}
	if home := e.Getenv("UNITY_CLI_HOME"); home != "" {
		return e.join(home, "bin", exe)
	}
	switch e.GOOS {
	case "windows":
		base := e.Getenv("LOCALAPPDATA")
		if base == "" {
			base = e.join(e.Home, "AppData", "Local")
		}
		return e.join(base, "Unity", "bin", exe)
	case "darwin":
		return e.join(e.Home, ".unity", "bin", exe)
	default:
		return e.join(e.Home, ".local", "bin", exe)
	}
}

// Candidate is a place the CLI might be.
type Candidate struct {
	Path string
	// Trusted candidates are believed to be Unity's CLI because a user setting
	// or Unity's installer named them. Others (a `unity` found on PATH, which
	// on some Linux desktops is something else entirely) only count when they
	// answer --version like the Unity CLI does.
	Trusted bool
}

// Candidates lists where to look for the CLI, best first.
func (e Env) Candidates(configured string) []Candidate {
	var out []Candidate
	seen := map[string]bool{}
	add := func(p string, trusted bool) {
		if p == "" || seen[p] {
			return
		}
		seen[p] = true
		out = append(out, Candidate{Path: p, Trusted: trusted})
	}
	add(configured, true)
	if b, err := e.ReadFile(e.join(e.HubDataDir(), "cli-install.json")); err == nil {
		var rec struct {
			Path string `json:"path"`
		}
		if json.Unmarshal(b, &rec) == nil {
			add(rec.Path, true)
		}
	}
	add(e.DefaultCLIPath(), true)
	if p, err := e.LookPath("unity"); err == nil {
		add(p, false)
	}
	return out
}

// State says whether a usable CLI is installed.
type State int

const (
	// Missing means there is no Unity CLI.
	Missing State = iota
	// Outdated means a CLI exists but is older than MinVersion or unreadable.
	Outdated
	// Ready means the CLI can be used.
	Ready
)

func (s State) String() string {
	return [...]string{"missing", "outdated", "ready"}[s]
}

// Status is the result of looking for the CLI.
type Status struct {
	State   State
	Path    string  // the CLI to use; set for Ready and Outdated
	Version Version // zero when unknown
}

// VersionFunc returns the output of `<path> --version`.
type VersionFunc func(ctx context.Context, path string) (string, error)

// Probe finds the CLI. The first candidate that is a real Unity CLI decides:
// Ready when recent enough, Outdated otherwise.
func Probe(ctx context.Context, env Env, configured string, version VersionFunc) Status {
	if version == nil {
		version = runVersion
	}
	broken := Status{} // a trusted file that would not report a version
	for _, c := range env.Candidates(configured) {
		if !env.IsFile(c.Path) {
			continue
		}
		out, err := version(ctx, c.Path)
		v, ok := parseVersionOutput(out)
		if err != nil || !ok {
			if c.Trusted && broken.State == Missing {
				broken = Status{State: Outdated, Path: c.Path}
			}
			continue
		}
		if v.Compare(MinVersion) >= 0 {
			return Status{State: Ready, Path: c.Path, Version: v}
		}
		return Status{State: Outdated, Path: c.Path, Version: v}
	}
	return broken
}

// runVersion runs `<path> --version`, giving up after a few seconds.
func runVersion(ctx context.Context, path string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, path, "--version")
	cmd.Env = append(os.Environ(), "UNITY_NO_BANNER=1", "UNITY_NO_PAGER=1", "UNITY_NON_INTERACTIVE=1")
	out, err := cmd.Output()
	return string(out), err
}
