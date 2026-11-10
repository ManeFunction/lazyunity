package launch

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
	"time"
)

func TestSplit(t *testing.T) {
	cases := []struct {
		in   string
		want []string
		err  bool
	}{
		{"code {{dir}}", []string{"code", "{{dir}}"}, false},
		{"  a   b\tc ", []string{"a", "b", "c"}, false},
		{`open -a "Visual Studio Code" {{dir}}`, []string{"open", "-a", "Visual Studio Code", "{{dir}}"}, false},
		{`echo 'it is' fine`, []string{"echo", "it is", "fine"}, false},
		{`echo "it's"`, []string{"echo", "it's"}, false},
		{`a\ b c`, []string{"a b", "c"}, false},
		{`echo ""`, []string{"echo", ""}, false},
		{"$HOME `x` *", []string{"$HOME", "`x`", "*"}, false}, // nothing is expanded
		{"", nil, false},
		{`echo "open`, nil, true},
		{`echo\`, nil, true},
	}
	for _, c := range cases {
		got, err := Split(c.in)
		if (err != nil) != c.err {
			t.Errorf("Split(%q) error = %v, want error %v", c.in, err, c.err)
			continue
		}
		if !c.err && !reflect.DeepEqual(got, c.want) && !(len(got) == 0 && len(c.want) == 0) {
			t.Errorf("Split(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestExpandKeepsFolderNamesWhole(t *testing.T) {
	got, err := Expand("code --goto {{dir}}/Assets", "/home/u/My Game")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"code", "--goto", "/home/u/My Game/Assets"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
	if _, err := Expand("   ", "/x"); err == nil {
		t.Error("an empty command must be an error")
	}
	if _, err := Expand(`code "{{dir}}`, "/x"); err == nil {
		t.Error("a broken command must be an error")
	}
}

func TestFolderCommand(t *testing.T) {
	cases := []struct {
		goos, tmpl string
		want       []string
	}{
		{"linux", "", []string{"xdg-open", "/p/My Game"}},
		{"darwin", "", []string{"open", "/p/My Game"}},
		{"windows", "", []string{"explorer", "/p/My Game"}},
		{"linux", "nautilus {{dir}}", []string{"nautilus", "/p/My Game"}},
		{"darwin", "  ", []string{"open", "/p/My Game"}},
	}
	for _, c := range cases {
		got, err := FolderCommand(c.goos, c.tmpl, "/p/My Game")
		if err != nil || !reflect.DeepEqual(got, c.want) {
			t.Errorf("FolderCommand(%q, %q) = %q, %v; want %q", c.goos, c.tmpl, got, err, c.want)
		}
	}
}

func waitForFile(t *testing.T, path string) []byte {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		if b, err := os.ReadFile(path); err == nil && len(b) > 0 {
			return b
		}
		if time.Now().After(deadline) {
			t.Fatalf("%s never appeared", path)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestRunStartsADetachedProcess(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("needs a POSIX shell")
	}
	dir := t.TempDir()
	marker := filepath.Join(dir, "marker")
	if err := Run(`sh -c "echo started > '`+marker+`'"`, dir); err != nil {
		t.Fatal(err)
	}
	waitForFile(t, marker)
}

func TestRunReportsAMissingProgram(t *testing.T) {
	if err := Run("definitely-not-a-program-xyz {{dir}}", "/x"); err == nil {
		t.Error("starting a missing program must fail")
	}
	if err := Editor("/definitely/not/here/Unity", "/p", ""); err == nil {
		t.Error("starting a missing Editor must fail")
	}
}

func TestEditorPassesProjectAndTarget(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("needs a POSIX shell")
	}
	dir := t.TempDir()
	out := filepath.Join(dir, "args")
	script := filepath.Join(dir, "Unity")
	os.WriteFile(script, []byte("#!/bin/sh\nprintf '%s\\n' \"$@\" > '"+out+"'\n"), 0o755)
	if err := Editor(script, "/p/My Game", "android"); err != nil {
		t.Fatal(err)
	}
	want := "-projectPath\n/p/My Game\n-buildTarget\nandroid\n"
	if got := string(waitForFile(t, out)); got != want {
		t.Errorf("arguments = %q, want %q", got, want)
	}
}
