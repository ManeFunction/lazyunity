//go:build windows

package unitycli

import "testing"

func TestCmdLine(t *testing.T) {
	got, err := cmdLine([]string{`C:\Program Files\Unity CLI\unity.exe`, "--format", "json", "uninstall", "6000.6.9f1", "--yes"})
	if want := `"C:\Program Files\Unity CLI\unity.exe" "--format" "json" "uninstall" "6000.6.9f1" "--yes"`; err != nil || got != want {
		t.Errorf("cmdLine = %s, %v; want %s", got, err, want)
	}
	for _, bad := range []string{`a"b`, "%PATH%", "a\nb"} {
		if _, err := cmdQuote(bad); err == nil {
			t.Errorf("cmdQuote(%q) should be refused", bad)
		}
	}
}
