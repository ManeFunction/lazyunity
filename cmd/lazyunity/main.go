// Command lazyunity is a terminal interface for Unity projects and Editors, a
// replacement for the Unity Hub that is driven with lazygit's keys.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/ManeFunction/lazyunity/internal/config"
	"github.com/ManeFunction/lazyunity/internal/state"
	"github.com/ManeFunction/lazyunity/internal/tui"
	"github.com/ManeFunction/lazyunity/internal/unity"
	"github.com/ManeFunction/lazyunity/internal/unitycli"
)

// version is the one place the version is written. The release workflow checks
// that the tag (vX.Y.Z) matches it, so every build, go install included,
// reports the same version.
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
		// undocumented: shows the made-up machine of a YAML file, for screenshots
		fakeFile = flag.String("fake", "", "")
	)
	flag.Usage = func() {
		out := flag.CommandLine.Output()
		fmt.Fprintf(out, "Usage: lazyunity [options]\n\n"+
			"A terminal interface for Unity projects and Editors, driven with lazygit's keys.\n\nOptions:\n")
		flag.VisitAll(func(f *flag.Flag) {
			if f.Name == "fake" {
				return
			}
			line := "  -" + f.Name
			name, usage := flag.UnquoteUsage(f)
			if name != "" {
				line += " " + name
			}
			fmt.Fprintf(out, "%s\n    \t%s\n", line, usage)
		})
	}
	flag.Parse()
	if *showVersion {
		fmt.Println("lazyunity v" + version)
		return nil
	}

	dir, err := config.Dir(*configDir)
	if err != nil {
		return fmt.Errorf("finding the config folder: %w", err)
	}

	if *debugLog != "" {
		f, err := tea.LogToFile(*debugLog, "lazyunity")
		if err != nil {
			return err
		}
		defer f.Close()
	}

	var fake *tui.Fake
	if *fakeFile != "" {
		if fake, err = tui.LoadFake(*fakeFile); err != nil {
			return err
		}
	}

	// Editing the config from the Tools tab quits to start again with it applied.
	for {
		restart, err := session(dir, fake)
		if err != nil || !restart {
			return err
		}
	}
}

// session loads the config and runs the interface once; restart reports that
// it quit to be started again.
func session(dir string, fake *tui.Fake) (restart bool, err error) {
	cfg, err := config.Load(dir)
	if err != nil {
		return false, err
	}
	if fake != nil {
		return fakeSession(dir, cfg, fake)
	}
	st := state.Load(dir)

	// What is cheap to load goes in before the first frame is drawn.
	env := unitycli.DefaultEnv()
	probeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	status := unitycli.Probe(probeCtx, env, config.ExpandHome(cfg.Unity.CLIPath), nil)
	cancel()

	opts := tui.Options{
		Config: cfg, State: st, ConfigDir: dir, Version: version,
		CLI: status, Env: env, GOOS: env.GOOS,
		Warnings: append(append([]string(nil), cfg.Warnings...), st.Warnings...),
	}
	if status.State == unitycli.Ready {
		opts.Client = unitycli.New(status.Path)
		// a budget of its own: a slow probe must not starve it. When it does
		// not make it, the interface loads the projects after the first frame.
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		if ps, err := opts.Client.Projects(ctx); err == nil {
			opts.Projects = ps
		}
		cancel()
	}
	home, _ := os.UserHomeDir()
	opts.InstallRoot = unity.DefaultInstallRoot(env.GOOS, home, env.Getenv)

	model, err := tui.New(opts) // scans the Editor folders, too
	if err != nil {
		return false, err
	}
	_, err = tea.NewProgram(model).Run()
	if saveErr := st.Save(); saveErr != nil && err == nil {
		err = fmt.Errorf("saving state: %w", saveErr)
	}
	return err == nil && model.Restart(), err
}

// fakeSession runs the interface on the made-up machine of a --fake file. It
// uses the real config, for its theme and keys, but a throwaway state, so the
// real one is neither shown nor changed.
func fakeSession(dir string, cfg config.Config, fake *tui.Fake) (restart bool, err error) {
	tmp, err := os.MkdirTemp("", "lazyunity-fake-*")
	if err != nil {
		return false, err
	}
	defer os.RemoveAll(tmp)
	env := unitycli.DefaultEnv()
	opts := tui.Options{
		Config: cfg, State: state.Load(tmp), ConfigDir: dir, Version: version,
		CLI: fake.Status(), Env: env, GOOS: env.GOOS, InstallRoot: fake.InstallRoot(env.GOOS),
		Warnings: cfg.Warnings, Fake: fake,
	}
	// A file without releases shows the real ones, when there is a Unity CLI
	// to list them. Listing them changes nothing on the machine.
	if len(fake.Releases) == 0 {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		if st := unitycli.Probe(ctx, env, config.ExpandHome(cfg.Unity.CLIPath), nil); st.State == unitycli.Ready {
			opts.Client = unitycli.New(st.Path)
		}
		cancel()
	}
	model, err := tui.New(opts)
	if err != nil {
		return false, err
	}
	_, err = tea.NewProgram(model).Run()
	return err == nil && model.Restart(), err
}
