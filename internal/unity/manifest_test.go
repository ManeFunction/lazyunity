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
