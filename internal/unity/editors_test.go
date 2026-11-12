package unity

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
)

// makeEditor creates <root>/<version> laid out for goos and returns the
// executable's path.
func makeEditor(t *testing.T, root, version, goos string) string {
	t.Helper()
	exe := filepath.Join(root, version, ExecutableRel(goos))
	if err := os.MkdirAll(filepath.Dir(exe), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(exe, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return exe
}

func TestExecutableRel(t *testing.T) {
	cases := map[string]string{
		"linux":   filepath.Join("Editor", "Unity"),
		"windows": filepath.Join("Editor", "Unity.exe"),
		"darwin":  filepath.Join("Unity.app", "Contents", "MacOS", "Unity"),
	}
	for goos, want := range cases {
		if got := ExecutableRel(goos); got != want {
			t.Errorf("%s: %q, want %q", goos, got, want)
		}
	}
}

func TestDefaultInstallRoot(t *testing.T) {
	if got := DefaultInstallRoot("linux", "/home/u"); got != "/home/u/Unity/Hub/Editor" {
		t.Errorf("linux: %q", got)
	}
	if got := DefaultInstallRoot("darwin", "/Users/u"); got != "/Applications/Unity/Hub/Editor" {
		t.Errorf("darwin: %q", got)
	}
	if got := DefaultInstallRoot("windows", `C:\Users\u`); got != `C:\Program Files\Unity\Hub\Editor` {
		t.Errorf("windows: %q", got)
	}
}

func TestVersionDir(t *testing.T) {
	root := filepath.Join("opt", "unity")
	for _, goos := range []string{"linux", "windows", "darwin"} {
		dir := filepath.Join(root, "6000.6.8f1")
		exe := filepath.Join(dir, ExecutableRel(goos))
		if got := VersionDir(exe, goos); got != dir {
			t.Errorf("%s: VersionDir(%q) = %q, want %q", goos, exe, got, dir)
		}
		if goos == "darwin" {
			app := filepath.Join(dir, appRel)
			if got := VersionDir(NormalizeLocation(app, goos), goos); got != dir {
				t.Errorf("%s: VersionDir of the normalized %q = %q, want %q", goos, app, got, dir)
			}
		}
	}
	if got := VersionDir(filepath.Join("opt", "somewhere", "unity"), "linux"); got != "" {
		t.Errorf("a path that is not an Editor executable gave %q", got)
	}
}

func TestNormalizeLocation(t *testing.T) {
	dir := filepath.Join("/Editors", "6000.6.8f1")
	app := filepath.Join(dir, appRel)
	exe := filepath.Join(app, "Contents", "MacOS", "Unity")
	if got := NormalizeLocation(app, "darwin"); got != exe {
		t.Errorf("darwin: NormalizeLocation(%q) = %q, want %q", app, got, exe)
	}
	if got := NormalizeLocation(exe, "darwin"); got != exe {
		t.Errorf("an executable comes back as it is: %q", got)
	}
	if got := NormalizeLocation(dir, "darwin"); got != dir {
		t.Errorf("a version folder comes back as it is: %q", got)
	}
	for _, goos := range []string{"linux", "windows"} {
		if got := NormalizeLocation(app, goos); got != app {
			t.Errorf("%s: NormalizeLocation(%q) = %q", goos, app, got)
		}
	}
}

func TestScanAllLayouts(t *testing.T) {
	for _, goos := range []string{"linux", "windows", "darwin"} {
		t.Run(goos, func(t *testing.T) {
			root := t.TempDir()
			e1 := makeEditor(t, root, "6000.6.8f1", goos)
			e2 := makeEditor(t, root, "2022.3.62f1", goos)
			makeEditor(t, root, "2022.3.9f1", goos)

			// things that are not Editors
			os.MkdirAll(filepath.Join(root, "not-an-editor", "Editor"), 0o755) // folder without the executable
			os.WriteFile(filepath.Join(root, "stray-file.txt"), nil, 0o644)    // file at the root
			os.MkdirAll(filepath.Join(root, "empty"), 0o755)

			got := Scan(goos, []string{root})
			var versions []string
			for _, e := range got {
				versions = append(versions, e.Version)
			}
			want := []string{"6000.6.8f1", "2022.3.62f1", "2022.3.9f1"} // newest first, 62 before 9
			if !reflect.DeepEqual(versions, want) {
				t.Fatalf("versions = %v, want %v", versions, want)
			}
			if got[0].Location != e1 || got[1].Location != e2 || got[0].Root != root ||
				got[0].Dir != filepath.Join(root, "6000.6.8f1") {
				t.Errorf("unexpected paths: %+v", got[0])
			}
		})
	}
}

func TestScanMultipleRoots(t *testing.T) {
	a, b := t.TempDir(), t.TempDir()
	makeEditor(t, a, "6000.6.8f1", "linux")
	makeEditor(t, b, "2021.3.5f1", "linux")

	got := Scan("linux", []string{a, filepath.Join(a, "missing"), b, a}) // a twice, plus a missing root
	if len(got) != 2 || got[0].Version != "6000.6.8f1" || got[1].Version != "2021.3.5f1" {
		t.Errorf("got %+v", got)
	}
	if got[1].Root != b {
		t.Errorf("Root = %q, want %q", got[1].Root, b)
	}
	if got := Scan("linux", nil); len(got) != 0 {
		t.Errorf("no roots gave %v", got)
	}
}

func TestScanFollowsSymlinks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need privileges on Windows")
	}
	real, root := t.TempDir(), t.TempDir()
	makeEditor(t, real, "2023.2.20f1", "linux")
	if err := os.Symlink(filepath.Join(real, "2023.2.20f1"), filepath.Join(root, "2023.2.20f1")); err != nil {
		t.Fatal(err)
	}
	got := Scan("linux", []string{root})
	if len(got) != 1 || got[0].Version != "2023.2.20f1" {
		t.Errorf("a symlinked Editor folder was missed: %+v", got)
	}
}

func TestScanMetadata(t *testing.T) {
	root := t.TempDir()
	makeEditor(t, root, "6000.6.8f1", "linux")
	makeEditor(t, root, "2022.3.62f1", "linux")
	makeEditor(t, root, "2021.3.5f1", "linux")
	write := func(version, body string) {
		if err := os.WriteFile(filepath.Join(root, version, "metadata.hub.json"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("6000.6.8f1", `{"productName":"Unity 6.6","architecture":"x86_64","isLTS":null}`)
	write("2022.3.62f1", `{"architecture":"arm64","isLTS":true}`)
	write("2021.3.5f1", `this is not json`)

	byVersion := map[string]Installed{}
	for _, e := range Scan("linux", []string{root}) {
		byVersion[e.Version] = e
	}
	if e := byVersion["6000.6.8f1"]; e.Architecture != "x86_64" || e.LTS {
		t.Errorf("6000: %+v", e)
	}
	if e := byVersion["2022.3.62f1"]; e.Architecture != "arm64" || !e.LTS {
		t.Errorf("2022: %+v", e)
	}
	if e := byVersion["2021.3.5f1"]; e.Architecture != "" || e.LTS {
		t.Errorf("broken metadata must be ignored: %+v", e)
	}
}
