package unity

import (
	"os"
	"path/filepath"
	"testing"
)

// versionDirWith returns a version folder whose modules.json is body.
func versionDirWith(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	if body != "" {
		if err := os.WriteFile(filepath.Join(dir, "modules.json"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func ids(ps []Platform) []string {
	var out []string
	for _, p := range ps {
		out = append(out, p.ID)
	}
	return out
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestReadModulesFixture(t *testing.T) {
	dir := t.TempDir()
	b, err := os.ReadFile(filepath.Join("..", "..", "testdata", "editor", "modules.json"))
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(dir, "modules.json"), b, 0o644)

	mods, err := ReadModules(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(mods) != 25 {
		t.Fatalf("got %d modules, want 25", len(mods))
	}
	var selected []string
	var children int
	for _, m := range mods {
		if m.Selected {
			selected = append(selected, m.ID)
		}
		if m.Parent != "" {
			children++
		}
	}
	if !equal(selected, []string{"linux-il2cpp"}) {
		t.Errorf("selected = %v", selected)
	}
	if children == 0 {
		t.Error("the Android SDK pieces should be listed with a parent")
	}

	// the real Editor here only has Linux support installed
	if got := ids(InstalledPlatforms(dir, "linux")); !equal(got, []string{"linux"}) {
		t.Errorf("platforms = %v", got)
	}
}

func TestInstalledPlatforms(t *testing.T) {
	t.Run("host platform is always there", func(t *testing.T) {
		dir := versionDirWith(t, "") // no modules.json, no PlaybackEngines
		for goos, want := range map[string]string{"linux": "linux", "windows": "windows", "darwin": "macos"} {
			if got := ids(InstalledPlatforms(dir, goos)); !equal(got, []string{want}) {
				t.Errorf("%s: %v", goos, got)
			}
		}
	})

	t.Run("selected modules add platforms in table order", func(t *testing.T) {
		dir := versionDirWith(t, `[
			{"id":"webgl","selected":true},
			{"id":"android","selected":true},
			{"id":"android-open-jdk-17","selected":true,"parent":"android"},
			{"id":"ios","selected":false},
			{"id":"windows-server","selected":true},
			{"id":"documentation","selected":true}
		]`)
		got := ids(InstalledPlatforms(dir, "linux"))
		want := []string{"windows", "linux", "android", "webgl"}
		if !equal(got, want) {
			t.Errorf("got %v, want %v", got, want)
		}
	})

	t.Run("a sub-module alone does not install a platform", func(t *testing.T) {
		dir := versionDirWith(t, `[{"id":"android","selected":false},{"id":"android-sdk-ndk-tools","selected":true,"parent":"android"}]`)
		if got := ids(InstalledPlatforms(dir, "linux")); !equal(got, []string{"linux"}) {
			t.Errorf("got %v", got)
		}
	})

	t.Run("an Editor installed by hand is read from PlaybackEngines", func(t *testing.T) {
		dir := versionDirWith(t, "")
		engines := PlaybackEnginesDir(dir, "linux")
		os.MkdirAll(filepath.Join(engines, "WebGLSupport"), 0o755)
		os.WriteFile(filepath.Join(engines, "WebGLSupport", "ivy.xml"), nil, 0o644)
		os.MkdirAll(filepath.Join(engines, "AndroidPlayer"), 0o755) // left empty: not installed
		if got := ids(InstalledPlatforms(dir, "linux")); !equal(got, []string{"linux", "webgl"}) {
			t.Errorf("got %v", got)
		}
	})

	t.Run("a broken modules.json falls back to the folders", func(t *testing.T) {
		dir := versionDirWith(t, "{not json")
		engines := PlaybackEnginesDir(dir, "linux")
		os.MkdirAll(filepath.Join(engines, "iOSSupport"), 0o755)
		os.WriteFile(filepath.Join(engines, "iOSSupport", "x"), nil, 0o644)
		if got := ids(InstalledPlatforms(dir, "linux")); !equal(got, []string{"linux", "ios"}) {
			t.Errorf("got %v", got)
		}
	})
}

func TestPlaybackEnginesDir(t *testing.T) {
	if got := PlaybackEnginesDir("v", "linux"); got != filepath.Join("v", "Editor", "Data", "PlaybackEngines") {
		t.Errorf("linux: %q", got)
	}
	if got := PlaybackEnginesDir("v", "windows"); got != filepath.Join("v", "Editor", "Data", "PlaybackEngines") {
		t.Errorf("windows: %q", got)
	}
	if got := PlaybackEnginesDir("v", "darwin"); got != filepath.Join("v", "Unity.app", "Contents", "PlaybackEngines") {
		t.Errorf("darwin: %q", got)
	}
}

func TestReadModulesMissing(t *testing.T) {
	if _, err := ReadModules(t.TempDir()); err == nil {
		t.Error("a folder without modules.json must be an error")
	}
}
