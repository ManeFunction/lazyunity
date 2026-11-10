package unitycli

import "testing"

func TestParseVersion(t *testing.T) {
	cases := []struct {
		in   string
		want Version
		ok   bool
	}{
		{"1.0.0-beta.16", Version{1, 0, 0, "beta", 16}, true},
		{" 1.2.3 \n", Version{1, 2, 3, "", 0}, true},
		{"1.0.0-RC.2", Version{1, 0, 0, "rc", 2}, true},
		{"1.0", Version{}, false},
		{"unity 1.0.0", Version{}, false},
		{"", Version{}, false},
	}
	for _, c := range cases {
		got, ok := ParseVersion(c.in)
		if ok != c.ok || got != c.want {
			t.Errorf("ParseVersion(%q) = %v, %v; want %v, %v", c.in, got, ok, c.want, c.ok)
		}
	}
}

func TestParseVersionOutput(t *testing.T) {
	cases := map[string]string{
		"1.0.0-beta.16\n":                  "1.0.0-beta.16",
		"\n\nunity version 1.0.0-beta.9\n": "1.0.0-beta.9",
		"banner line\n1.2.3\n":             "1.2.3",
	}
	for in, want := range cases {
		v, ok := parseVersionOutput(in)
		if !ok || v.String() != want {
			t.Errorf("parseVersionOutput(%q) = %v, %v; want %s", in, v, ok, want)
		}
	}
	if _, ok := parseVersionOutput("Unity Shell 7.0"); ok {
		t.Error("parsed a version out of something that is not one")
	}
}

func TestVersionCompare(t *testing.T) {
	order := []string{
		"0.9.9", "1.0.0-alpha.3", "1.0.0-beta.2", "1.0.0-beta.10", "1.0.0-beta.16",
		"1.0.0-rc.1", "1.0.0", "1.0.1", "1.1.0", "2.0.0-beta.1", "2.0.0",
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

func TestMinVersionString(t *testing.T) {
	if got := MinVersion.String(); got != "1.0.0-beta.16" {
		t.Errorf("MinVersion = %s", got)
	}
}

func TestTail(t *testing.T) {
	if got := tail("  short \n", 10); got != "short" {
		t.Errorf("tail short = %q", got)
	}
	if got := tail("abcdefghij", 4); got != "…ghij" {
		t.Errorf("tail cut = %q", got)
	}
	// must not split a multi-byte rune
	got := tail("ааааа", 5) // 2-byte runes, 10 bytes
	if got != "…аа" {
		t.Errorf("tail on rune boundary = %q", got)
	}
}
