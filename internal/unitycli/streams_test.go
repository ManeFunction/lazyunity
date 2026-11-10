package unitycli

import "testing"

func TestStreamLabel(t *testing.T) {
	cases := []struct {
		in    string
		label string
		known bool
	}{
		{"tech", "Supported", true},
		{"TECH", "Supported", true},
		{"SUPPORTED", "Supported", true},
		{"lts", "LTS", true},
		{"BETA", "Beta", true},
		{"alpha", "Alpha", true},
		{"", "All", true},
		{"weird", "Weird", false},
	}
	for _, c := range cases {
		if label, known := StreamLabel(c.in); label != c.label || known != c.known {
			t.Errorf("StreamLabel(%q) = %q, %v; want %q, %v", c.in, label, known, c.label, c.known)
		}
	}
}
