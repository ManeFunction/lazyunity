package unity

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadManifest(t *testing.T) {
	dir := t.TempDir()
	if _, err := ReadManifest(dir); err == nil {
		t.Error("a project without a manifest is an error")
	}
	os.MkdirAll(filepath.Join(dir, "Packages"), 0o755)
	os.WriteFile(filepath.Join(dir, ManifestFile), []byte(`{"dependencies": {"com.cysharp.unitask": "2.5.11"}, "scopedRegistries": []}`), 0o644)
	deps, err := ReadManifest(dir)
	if err != nil || deps["com.cysharp.unitask"] != "2.5.11" || len(deps) != 1 {
		t.Errorf("%v %v", deps, err)
	}
}

func TestManifestFromRegistry(t *testing.T) {
	mf := Manifest{ScopedRegistries: []ScopedRegistry{{URL: "https://package.openupm.com/", Scopes: []string{"com.example", "jp.keijiro.klak"}}}}
	for id, want := range map[string]bool{
		"com.example": true, "com.example.a": true, "com.examples.a": false,
		"jp.keijiro.klak": true, "jp.keijiro.klakndi": false, "com.unity.ugui": false,
	} {
		if got := mf.FromRegistry(id, "https://package.openupm.com"); got != want {
			t.Errorf("%s: %v", id, got)
		}
	}
	if mf.FromRegistry("com.example.a", "https://other.example") {
		t.Error("another registry")
	}
}

func TestReadPackagesLock(t *testing.T) {
	dir := t.TempDir()
	if lock, err := ReadPackagesLock(dir); err != nil || len(lock) != 0 {
		t.Errorf("a project without a lock file has no packages: %v %v", lock, err)
	}
	os.MkdirAll(filepath.Join(dir, "Packages"), 0o755)
	os.WriteFile(filepath.Join(dir, LockFile), []byte(`{"dependencies": {"com.a": {"version": "1.2.0", "depth": 1, "source": "registry", "dependencies": {"com.b": "2.0.0"}}}}`), 0o644)
	lock, err := ReadPackagesLock(dir)
	if err != nil || lock["com.a"].Version != "1.2.0" || lock["com.a"].Depth != 1 || lock["com.a"].Dependencies["com.b"] != "2.0.0" {
		t.Errorf("%+v %v", lock, err)
	}
	os.WriteFile(filepath.Join(dir, LockFile), []byte(`{`), 0o644)
	if _, err := ReadPackagesLock(dir); err == nil {
		t.Error("a broken lock file is an error")
	}
}
