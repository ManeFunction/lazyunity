package unitycli

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCachedBytes(t *testing.T) {
	dir := t.TempDir()
	write := func(name string, size int) {
		t.Helper()
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, make([]byte, size), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("Unity-6000.6.9f1.tar.xz", 300)
	write("UnitySetup-Android-Support-for-Editor-6000.6.9f1.tar.xz", 20)
	write("Unity-6000.6.9f10.tar.xz", 4000)          // another version
	write("Unity-16000.6.9f1.tar.xz", 5000)          // not a whole word
	write("portable-abc/Unity-6000.6.9f1.tar.xz", 7) // a --download-directory install's

	if got := CachedBytes(dir, "6000.6.9f1"); got != 320 {
		t.Errorf("CachedBytes = %d, want the 320 bytes of the two files of that version", got)
	}
	if got := CachedBytes(dir, "6000.5.1f1"); got != 0 {
		t.Errorf("nothing cached for another version, got %d", got)
	}
	if got := CachedBytes(filepath.Join(dir, "missing"), "6000.6.9f1"); got != 0 {
		t.Errorf("a missing folder has nothing cached, got %d", got)
	}
}
