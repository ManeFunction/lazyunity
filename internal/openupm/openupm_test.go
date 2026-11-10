package openupm

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/ManeFunction/lazyunity/internal/config"
)

func TestInstallCommand(t *testing.T) {
	env := func(c *exec.Cmd, key string) string {
		v := ""
		for _, e := range c.Env {
			if after, ok := strings.CutPrefix(e, key+"="); ok {
				v = after // the last one wins, as in exec
			}
		}
		return v
	}
	official := InstallCommand("linux", config.Default().OpenUPM)
	if official.Args[0] != "/bin/sh" || env(official, "LAZYUNITY_OPENUPM_REPO") != "" {
		t.Errorf("the official repository installs from the registry: %v", official.Args)
	}
	fork := InstallCommand("linux", config.OpenUPM{Repo: " https://github.com/me/openupm-cli ", Ref: "fixes"})
	if env(fork, "LAZYUNITY_OPENUPM_REPO") != "https://github.com/me/openupm-cli" || env(fork, "LAZYUNITY_OPENUPM_REF") != "fixes" {
		t.Errorf("a fork is passed in the environment: %v", fork.Env[len(fork.Env)-2:])
	}
	for _, a := range fork.Args {
		if strings.Contains(a, "me/openupm-cli") {
			t.Errorf("the repository never reaches the shell's command line: %q", a)
		}
	}
	if win := InstallCommand("windows", config.Default().OpenUPM); win.Args[0] != "powershell" {
		t.Errorf("windows: %v", win.Args)
	}
	if runtime.GOOS != "windows" {
		if out, err := exec.Command("/bin/sh", "-n", "-c", shScript).CombinedOutput(); err != nil {
			t.Errorf("the install script does not parse: %v\n%s", err, out)
		}
	}
}

func TestFindFallsBackToUserBin(t *testing.T) {
	home := t.TempDir()
	want := filepath.Join(home, ".local", "bin", "openupm")
	lookPath := func(name string) (string, error) {
		if name == want {
			return want, nil
		}
		return "", exec.ErrNotFound
	}
	if got, err := Find(lookPath, home); err != nil || got != want {
		t.Errorf("Find = %q, %v; want %q", got, err, want)
	}
	if _, err := Find(lookPath, ""); err == nil {
		t.Error("without a home folder there is only the PATH")
	}
	onPath := func(name string) (string, error) { return "/usr/bin/" + name, nil }
	if got, _ := Find(onPath, home); got != "/usr/bin/openupm" {
		t.Errorf("the PATH comes first: %q", got)
	}
}

func TestAddAndVersion(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a shell script")
	}
	dir := t.TempDir()
	proj := filepath.Join(dir, "P")
	os.MkdirAll(proj, 0o755)
	exe := filepath.Join(dir, "openupm")
	os.WriteFile(exe, []byte("#!/bin/sh\necho \"$@\" > args.txt\n"), 0o755)
	if err := Add(context.Background(), exe, proj, []string{"a.b", "a.c"}); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(filepath.Join(proj, "args.txt"))
	if strings.TrimSpace(string(got)) != "add a.b a.c" {
		t.Errorf("args = %q", got)
	}
	os.WriteFile(exe, []byte("#!/bin/sh\necho boom >&2\nexit 3\n"), 0o755)
	if err := Add(context.Background(), exe, proj, []string{"a.b"}); err == nil || !strings.Contains(err.Error(), "boom") {
		t.Errorf("a failure carries the output: %v", err)
	}

	os.WriteFile(exe, []byte("#!/bin/sh\necho '4.2.0 (node 22)'\n"), 0o755)
	if v := Version(context.Background(), exe); v != "4.2.0" {
		t.Errorf("Version = %q", v)
	}
	if v := Version(context.Background(), filepath.Join(dir, "nope")); v != "" {
		t.Errorf("a missing CLI has no version: %q", v)
	}
}
