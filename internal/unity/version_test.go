package unity

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
)

func TestParseVersion(t *testing.T) {
	cases := []struct {
		in   string
		want Version
		ok   bool
	}{
		{"6000.6.8f1", Version{6000, 6, 8, 'f', 1, 0}, true},
		{"2022.3.62f1", Version{2022, 3, 62, 'f', 1, 0}, true},
		{"6000.7.0b8", Version{6000, 7, 0, 'b', 8, 0}, true},
		{"7000.0.0a16", Version{7000, 0, 0, 'a', 16, 0}, true},
		{"2019.4.1f1c1", Version{2019, 4, 1, 'f', 1, 1}, true},
		{"2021.3.5p2", Version{2021, 3, 5, 'p', 2, 0}, true},
		{" 6000.6.8f1\r\n", Version{6000, 6, 8, 'f', 1, 0}, true},
		{"5.6.7f1", Version{5, 6, 7, 'f', 1, 0}, true}, // old releases still parse
		{"6000.6.8", Version{}, false},
		{"6000.6.8x1", Version{}, false},
		{"abc", Version{}, false},
		{"", Version{}, false},
	}
	for _, c := range cases {
		got, ok := ParseVersion(c.in)
		if ok != c.ok || got != c.want {
			t.Errorf("ParseVersion(%q) = %+v, %v; want %+v, %v", c.in, got, ok, c.want, c.ok)
		}
		if ok && got.String() != trim(c.in) {
			t.Errorf("String() of %q = %q", c.in, got.String())
		}
	}
}

func trim(s string) string {
	for len(s) > 0 && (s[0] == ' ' || s[0] == '\r' || s[0] == '\n') {
		s = s[1:]
	}
	for len(s) > 0 && (s[len(s)-1] == ' ' || s[len(s)-1] == '\r' || s[len(s)-1] == '\n') {
		s = s[:len(s)-1]
	}
	return s
}

func TestCompare(t *testing.T) {
	order := []string{
		"2019.4.1f1", "2019.4.1f1c1", "2019.4.2f1", "2021.3.5f1", "2021.3.5p1", "2022.3.9f1", "2022.3.10f1",
		"6000.0.1a1", "6000.0.1a9", "6000.0.1b1", "6000.0.1f1", "6000.6.8f1", "6000.7.0b8", "7000.0.0a16",
	}
	for i, a := range order {
		for j, b := range order {
			va, _ := ParseVersion(a)
			vb, _ := ParseVersion(b)
			want := 0
			if i < j {
				want = -1
			} else if i > j {
				want = 1
			}
			if got := va.Compare(vb); got != want {
				t.Errorf("%s.Compare(%s) = %d, want %d", a, b, got, want)
			}
		}
	}
}

func TestCompareStrings(t *testing.T) {
	vs := []string{"2022.3.9f1", "garbage", "6000.6.8f1", "2022.3.10f1", "6000.7.0b8", "also-garbage"}
	sort.SliceStable(vs, func(i, j int) bool { return CompareStrings(vs[i], vs[j]) > 0 })
	want := []string{"6000.7.0b8", "6000.6.8f1", "2022.3.10f1", "2022.3.9f1", "garbage", "also-garbage"}
	if !reflect.DeepEqual(vs, want) {
		t.Errorf("sorted = %v, want %v", vs, want)
	}
}

func TestAtLeast(t *testing.T) {
	cases := []struct {
		v            string
		major, minor int
		want         bool
	}{
		{"6000.7.0f1", 6000, 7, true},
		{"6000.7.0b8", 6000, 7, true},
		{"6000.6.9f1", 6000, 7, false},
		{"7000.0.0a16", 6000, 7, true},
		{"2022.3.1f1", 6000, 7, false},
		{"2019.1.0f1", 2019, 1, true},
		{"2018.4.36f1", 2019, 1, false},
		{"??", 2019, 1, false},
	}
	for _, c := range cases {
		if got := AtLeast(c.v, c.major, c.minor); got != c.want {
			t.Errorf("AtLeast(%q, %d, %d) = %v", c.v, c.major, c.minor, got)
		}
	}
}

func TestLine(t *testing.T) {
	v, _ := ParseVersion("2022.3.10f1")
	if v.Line() != "2022.3" {
		t.Errorf("%+v", v)
	}
}

func TestReadProjectVersion(t *testing.T) {
	write := func(content string) string {
		dir := t.TempDir()
		if err := os.MkdirAll(filepath.Join(dir, "ProjectSettings"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, ProjectVersionFile), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		return dir
	}

	got, err := ReadProjectVersion(write("m_EditorVersion: 6000.6.8f1\nm_EditorVersionWithRevision: 6000.6.8f1 (237b6593792b)\n"))
	if err != nil || got != "6000.6.8f1" {
		t.Errorf("unix file: %q, %v", got, err)
	}

	// CRLF must not leave a \r behind (it would stop the version matching an Editor)
	got, err = ReadProjectVersion(write("m_EditorVersion: 2022.3.62f1\r\nm_EditorVersionWithRevision: 2022.3.62f1 (abc)\r\n"))
	if err != nil || got != "2022.3.62f1" {
		t.Errorf("CRLF file: %q, %v", got, err)
	}

	if _, err = ReadProjectVersion(write("something: else\n")); err == nil {
		t.Error("a file without m_EditorVersion must be an error")
	}
	if _, err = ReadProjectVersion(t.TempDir()); err == nil {
		t.Error("a folder that is not a project must be an error")
	}
}
