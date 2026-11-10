package unitycli

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

// testEnv builds an Env for goos with the given regular files and PATH entry.
func testEnv(goos, home string, vars map[string]string, files map[string]string, onPath string) Env {
	return Env{
		GOOS:   goos,
		Home:   home,
		Getenv: func(k string) string { return vars[k] },
		ReadFile: func(p string) ([]byte, error) {
			if s, ok := files[p]; ok {
				return []byte(s), nil
			}
			return nil, errors.New("not found")
		},
		IsFile: func(p string) bool { _, ok := files[p]; return ok },
		LookPath: func(string) (string, error) {
			if onPath == "" {
				return "", errors.New("not on PATH")
			}
			return onPath, nil
		},
	}
}

func TestPaths(t *testing.T) {
	cases := []struct {
		name         string
		env          Env
		hubData, cli string
	}{
		{
			"linux", testEnv("linux", "/home/u", nil, nil, ""),
			"/home/u/.config/unityhub", "/home/u/.local/bin/unity",
		},
		{
			"linux with XDG_CONFIG_HOME", testEnv("linux", "/home/u", map[string]string{"XDG_CONFIG_HOME": "/cfg"}, nil, ""),
			"/cfg/unityhub", "/home/u/.local/bin/unity",
		},
		{
			"macos", testEnv("darwin", "/Users/u", nil, nil, ""),
			"/Users/u/Library/Application Support/UnityHub", "/Users/u/.unity/bin/unity",
		},
		{
			"windows", testEnv("windows", `C:\Users\u`, nil, nil, ""),
			`C:\Users\u\AppData\Roaming\UnityHub`, `C:\Users\u\AppData\Local\Unity\bin\unity.exe`,
		},
		{
			"windows with APPDATA and LOCALAPPDATA",
			testEnv("windows", `C:\Users\u`, map[string]string{"APPDATA": `D:\Roam`, "LOCALAPPDATA": `D:\Loc`}, nil, ""),
			`D:\Roam\UnityHub`, `D:\Loc\Unity\bin\unity.exe`,
		},
		{
			"UNITY_CLI_HOME", testEnv("linux", "/home/u", map[string]string{"UNITY_CLI_HOME": "/opt/ucli"}, nil, ""),
			"/home/u/.config/unityhub", "/opt/ucli/bin/unity",
		},
	}
	for _, c := range cases {
		if got := c.env.HubDataDir(); got != c.hubData {
			t.Errorf("%s: HubDataDir = %q, want %q", c.name, got, c.hubData)
		}
		if got := c.env.DefaultCLIPath(); got != c.cli {
			t.Errorf("%s: DefaultCLIPath = %q, want %q", c.name, got, c.cli)
		}
	}
}

func TestCandidates(t *testing.T) {
	files := map[string]string{
		"/home/u/.config/unityhub/cli-install.json": `{"path":"/opt/custom/unity","version":"1.0.0-beta.16"}`,
	}
	env := testEnv("linux", "/home/u", nil, files, "/usr/bin/unity")
	got := env.Candidates("/my/unity")
	want := []Candidate{
		{"/my/unity", true},
		{"/opt/custom/unity", true},
		{"/home/u/.local/bin/unity", true},
		{"/usr/bin/unity", false},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Candidates = %v, want %v", got, want)
	}

	// duplicates collapse, keeping the first (trusted) entry
	env = testEnv("linux", "/home/u", nil, nil, "/home/u/.local/bin/unity")
	got = env.Candidates("")
	if len(got) != 1 || !got[0].Trusted {
		t.Errorf("duplicate candidate not collapsed: %v", got)
	}
}

func versions(m map[string]string) VersionFunc {
	return func(_ context.Context, path string) (string, error) {
		if out, ok := m[path]; ok {
			return out, nil
		}
		return "", errors.New("exec failed")
	}
}

func TestProbe(t *testing.T) {
	const def = "/home/u/.local/bin/unity"
	ctx := context.Background()

	t.Run("missing", func(t *testing.T) {
		env := testEnv("linux", "/home/u", nil, nil, "")
		if got := Probe(ctx, env, "", versions(nil)); got.State != Missing {
			t.Errorf("state = %v", got.State)
		}
	})

	t.Run("ready", func(t *testing.T) {
		env := testEnv("linux", "/home/u", nil, map[string]string{def: ""}, "")
		got := Probe(ctx, env, "", versions(map[string]string{def: "1.0.0-beta.16\n"}))
		if got.State != Ready || got.Path != def || got.Version.String() != "1.0.0-beta.16" {
			t.Errorf("got %+v", got)
		}
	})

	t.Run("newer is ready", func(t *testing.T) {
		env := testEnv("linux", "/home/u", nil, map[string]string{def: ""}, "")
		got := Probe(ctx, env, "", versions(map[string]string{def: "1.0.0"}))
		if got.State != Ready {
			t.Errorf("state = %v", got.State)
		}
	})

	t.Run("outdated", func(t *testing.T) {
		env := testEnv("linux", "/home/u", nil, map[string]string{def: ""}, "")
		got := Probe(ctx, env, "", versions(map[string]string{def: "1.0.0-beta.9"}))
		if got.State != Outdated || got.Path != def || got.Version.PreNum != 9 {
			t.Errorf("got %+v", got)
		}
	})

	t.Run("first working candidate decides", func(t *testing.T) {
		files := map[string]string{"/configured/unity": "", def: ""}
		env := testEnv("linux", "/home/u", nil, files, "")
		got := Probe(ctx, env, "/configured/unity", versions(map[string]string{
			"/configured/unity": "1.0.0-beta.3", // old, but it is the one the user chose
			def:                 "1.0.0-beta.16",
		}))
		if got.State != Outdated || got.Path != "/configured/unity" {
			t.Errorf("got %+v", got)
		}
	})

	t.Run("a unity on PATH that is not the CLI is ignored", func(t *testing.T) {
		env := testEnv("linux", "/home/u", nil, map[string]string{"/usr/bin/unity": ""}, "/usr/bin/unity")
		got := Probe(ctx, env, "", versions(map[string]string{"/usr/bin/unity": "Unity Shell"}))
		if got.State != Missing {
			t.Errorf("got %+v", got)
		}
	})

	t.Run("a trusted file that will not report a version is outdated", func(t *testing.T) {
		env := testEnv("linux", "/home/u", nil, map[string]string{def: ""}, "")
		got := Probe(ctx, env, "", versions(nil)) // every run fails
		if got.State != Outdated || got.Path != def {
			t.Errorf("got %+v", got)
		}
	})

	t.Run("a good candidate beats an earlier broken one", func(t *testing.T) {
		files := map[string]string{"/configured/unity": "", def: ""}
		env := testEnv("linux", "/home/u", nil, files, "")
		got := Probe(ctx, env, "/configured/unity", versions(map[string]string{def: "1.0.0-beta.16"}))
		if got.State != Ready || got.Path != def {
			t.Errorf("got %+v", got)
		}
	})
}

func TestInstallCommand(t *testing.T) {
	unix := InstallCommand("linux")
	if len(unix.Args) != 5 || unix.Args[0] != "/bin/sh" || unix.Args[1] != "-c" || unix.Args[4] != ManifestURL {
		t.Errorf("unix command = %v", unix.Args)
	}
	win := InstallCommand("windows")
	if win.Args[0] != "powershell.exe" || win.Args[len(win.Args)-1] != installScriptWindows {
		t.Errorf("windows command = %v", win.Args)
	}
}
