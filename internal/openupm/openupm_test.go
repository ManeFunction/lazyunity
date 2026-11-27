package openupm

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
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
	official := InstallCommand("linux", config.Default().OpenUPM, "")
	if official.Args[0] != "/bin/sh" || env(official, "LAZYUNITY_OPENUPM_REPO") != "" {
		t.Errorf("the official repository installs from the registry: %v", official.Args)
	}
	fork := InstallCommand("linux", config.OpenUPM{Repo: " https://github.com/me/openupm-cli ", Ref: "fixes"}, "/tmp/stamp")
	if env(fork, "LAZYUNITY_OPENUPM_REPO") != "https://github.com/me/openupm-cli" || env(fork, "LAZYUNITY_OPENUPM_REF") != "fixes" ||
		env(fork, "LAZYUNITY_OPENUPM_STAMP") != "/tmp/stamp" {
		t.Errorf("a fork is passed in the environment: %v", fork.Env[len(fork.Env)-3:])
	}
	for _, a := range fork.Args {
		if strings.Contains(a, "me/openupm-cli") {
			t.Errorf("the repository never reaches the shell's command line: %q", a)
		}
	}
	if win := InstallCommand("windows", config.Default().OpenUPM, ""); win.Args[0] != "powershell" {
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

func TestLatestCommit(t *testing.T) {
	git, err := exec.LookPath("git")
	if err != nil {
		t.Skip("needs git")
	}
	repo := t.TempDir()
	run := func(args ...string) string {
		t.Helper()
		cmd := exec.Command(git, args...)
		cmd.Dir = repo
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	run("init", "-q", "-b", "main")
	run("commit", "-q", "--allow-empty", "-m", "one")
	first := run("rev-parse", "HEAD")
	run("tag", "-a", "v1", "-m", "v1") // an annotated tag: the commit, not the tag object
	run("branch", "fixes")
	run("commit", "-q", "--allow-empty", "-m", "two")
	second := run("rev-parse", "HEAD")

	ctx := context.Background()
	for ref, want := range map[string]string{"": second, "main": second, "fixes": first, "v1": first} {
		if got, err := LatestCommit(ctx, git, repo, ref); err != nil || got != want {
			t.Errorf("LatestCommit(%q) = %q, %v; want %q", ref, got, err, want)
		}
	}
	if _, err := LatestCommit(ctx, git, repo, "nope"); err == nil || !strings.Contains(err.Error(), "no branch or tag nope") {
		t.Errorf("a missing ref: %v", err)
	}
}

func TestLatestRelease(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a shell script")
	}
	npm := filepath.Join(t.TempDir(), "npm")
	os.WriteFile(npm, []byte("#!/bin/sh\n[ \"$*\" = 'view openupm-cli version' ] && echo 4.6.0\n"), 0o755)
	if v, err := LatestRelease(context.Background(), npm); err != nil || v != "4.6.0" {
		t.Errorf("LatestRelease = %q, %v", v, err)
	}
	os.WriteFile(npm, []byte("#!/bin/sh\nexit 1\n"), 0o755)
	if _, err := LatestRelease(context.Background(), npm); err == nil {
		t.Error("a failed npm is an error")
	}
}

func TestLookup(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/com.example.a":
			w.Write([]byte(`{"name": "com.example.a", "dist-tags": {"latest": "2.0.0"}, "versions": {"1.0.0": {}, "2.0.0": {"displayName": "Example A", "repository": "github:example/a"}}}`))
		case "/com.example.broken":
			w.WriteHeader(http.StatusInternalServerError)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	p, err := Lookup(context.Background(), srv.Client(), srv.URL, "com.example.a")
	if err != nil || p.DisplayName != "Example A" || p.Latest != "2.0.0" || !p.Versions["1.0.0"] || p.Repo != "https://github.com/example/a" {
		t.Errorf("%+v %v", p, err)
	}
	if _, err := Lookup(context.Background(), srv.Client(), srv.URL, "com.example.gone"); !errors.Is(err, ErrNoPackage) {
		t.Errorf("a missing package: %v", err)
	}
	if _, err := Lookup(context.Background(), srv.Client(), srv.URL, "com.example.broken"); err == nil || errors.Is(err, ErrNoPackage) {
		t.Errorf("a failing registry is not a missing package: %v", err)
	}
}

func TestRepositoryURL(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/com.example.top":
			w.Write([]byte(`{"repository": {"type": "git", "url": "git+https://github.com/example/top.git"}}`))
		case "/com.example.latest":
			w.Write([]byte(`{"dist-tags": {"latest": "2.0.0"}, "versions": {"2.0.0": {"repository": "git@github.com:example/latest.git"}}}`))
		case "/com.example.none":
			w.Write([]byte(`{}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	for id, want := range map[string]string{
		"com.example.top":    "https://github.com/example/top",
		"com.example.latest": "https://github.com/example/latest",
		"com.example.none":   "",
		"com.example.gone":   "",
	} {
		got, err := RepositoryURL(context.Background(), srv.Client(), srv.URL, id)
		if got != want || (want == "") != (err != nil) {
			t.Errorf("%s: %q, %v", id, got, err)
		}
	}
}
