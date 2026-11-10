// Command lazyunity is a terminal interface for Unity projects and Editors, a
// replacement for the Unity Hub that is driven with lazygit's keys.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/ManeFunction/lazyunity/internal/config"
	"github.com/ManeFunction/lazyunity/internal/state"
	"github.com/ManeFunction/lazyunity/internal/tui"
	"github.com/ManeFunction/lazyunity/internal/unity"
	"github.com/ManeFunction/lazyunity/internal/unitycli"
)

// version is set at build time with -ldflags "-X main.version=...".
var version = "0.1.0"

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "lazyunity:", err)
		os.Exit(1)
	}
}

func run() error {
	var (
		showVersion = flag.Bool("version", false, "print the version and exit")
		configDir   = flag.String("config-dir", "", "folder holding config.yml and state.yml")
		debugLog    = flag.String("debug-log", "", "write debug messages to this file")
	)
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage: lazyunity [options]\n\n"+
			"A terminal interface for Unity projects and Editors, driven with lazygit's keys.\n\nOptions:\n")
		flag.PrintDefaults()
	}
	flag.Parse()
	if *showVersion {
		fmt.Println("lazyunity", version)
		return nil
	}

	dir, err := config.Dir(*configDir)
	if err != nil {
		return fmt.Errorf("finding the config folder: %w", err)
	}
	cfg, err := config.Load(dir)
	if err != nil {
		return err
	}
	st := state.Load(dir)

	if *debugLog != "" {
		f, err := tea.LogToFile(*debugLog, "lazyunity")
		if err != nil {
			return err
		}
		defer f.Close()
	}

	// What is cheap to load goes in before the first frame is drawn.
	env := unitycli.DefaultEnv()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	status := unitycli.Probe(ctx, env, config.ExpandHome(cfg.Unity.CLIPath), nil)

	opts := tui.Options{
		Config: cfg, State: st, ConfigDir: dir, Version: version,
		CLI: status, Env: env, GOOS: env.GOOS,
	}
	if status.State == unitycli.Ready {
		opts.Client = unitycli.New(status.Path)
		if ps, err := opts.Client.Projects(ctx); err == nil {
			opts.Projects = ps
		} else {
			opts.Warnings = append(opts.Warnings, "could not list projects: "+err.Error())
		}
	}
	home, _ := os.UserHomeDir()
	opts.InstallRoot = unity.DefaultInstallRoot(env.GOOS, home)
	opts.Local = unity.Scan(env.GOOS, rootsFor(opts.InstallRoot, st, cfg))

	model, err := tui.New(opts)
	if err != nil {
		return err
	}
	_, err = tea.NewProgram(model).Run()
	if saveErr := st.Save(); saveErr != nil && err == nil {
		err = fmt.Errorf("saving state: %w", saveErr)
	}
	return err
}

// rootsFor lists the folders to look for Editors in before the CLI has said
// where it installs them.
func rootsFor(installRoot string, st *state.State, cfg config.Config) []string {
	roots := []string{installRoot}
	roots = append(roots, st.ExtraRoots...)
	roots = append(roots, cfg.Editors.ExtraRoots...)
	for i, r := range roots {
		roots[i] = filepath.Clean(config.ExpandHome(r))
	}
	return roots
}
