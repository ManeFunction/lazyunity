package tui

import (
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
)

func TestRelTime(t *testing.T) {
	now := time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		ago  time.Duration
		want string
	}{
		{0, "now"},
		{30 * time.Second, "now"},
		{-5 * time.Minute, "now"}, // the future (clock skew) is not shown as negative
		{time.Minute, "1m ago"},
		{59 * time.Minute, "59m ago"},
		{time.Hour, "1h ago"},
		{23 * time.Hour, "23h ago"},
		{24 * time.Hour, "1d ago"},
		{13 * 24 * time.Hour, "13d ago"},
		{14 * 24 * time.Hour, "2w ago"},
		{59 * 24 * time.Hour, "8w ago"},
		{60 * 24 * time.Hour, "2mo ago"},
		{364 * 24 * time.Hour, "12mo ago"},
		{365 * 24 * time.Hour, "1y ago"},
		{3 * 365 * 24 * time.Hour, "3y ago"},
	}
	for _, c := range cases {
		if got := relTime(now, now.Add(-c.ago)); got != c.want {
			t.Errorf("relTime(%v ago) = %q, want %q", c.ago, got, c.want)
		}
	}
	if got := relTime(now, time.Time{}); got != "never" {
		t.Errorf("zero time = %q", got)
	}
}

func TestHumanBytes(t *testing.T) {
	cases := map[int64]string{
		0: "0 B", 999: "999 B", 1000: "1.0 kB", 2936241: "2.9 MB", 470250280: "470.3 MB",
		1822966051: "1.8 GB", 4385104080: "4.4 GB", 1500000000000: "1.5 TB",
	}
	for n, want := range cases {
		if got := humanBytes(n); got != want {
			t.Errorf("humanBytes(%d) = %q, want %q", n, got, want)
		}
	}
}

func TestTruncateAndFit(t *testing.T) {
	if got := truncate("hello", 10); got != "hello" {
		t.Errorf("short string changed: %q", got)
	}
	if got := truncate("hello world", 6); got != "hello…" {
		t.Errorf("truncate = %q", got)
	}
	if got := truncate("hello", 0); got != "" {
		t.Errorf("zero width = %q", got)
	}
	if got := fit("ab", 5); got != "ab   " {
		t.Errorf("fit pad = %q", got)
	}
	if got := fit("abcdefgh", 5); got != "abcd…" {
		t.Errorf("fit cut = %q", got)
	}
	// wide characters count as two cells
	if got := width("日本語"); got != 6 {
		t.Errorf("width of CJK = %d", got)
	}
	if got := fit("日本語", 5); width(got) != 5 {
		t.Errorf("fit of CJK = %q (%d cells)", got, width(got))
	}
	// colour codes cost no width and survive truncation intact
	styled := "\x1b[31mred text\x1b[0m"
	if got := fit(styled, 6); width(got) != 6 || ansi.Strip(got) != "red t…" {
		t.Errorf("fit of styled = %q", got)
	}
	if got := padRight(styled, 10); width(got) != 10 {
		t.Errorf("padRight of styled = %q", got)
	}
}

func TestPluralAndClamp(t *testing.T) {
	if plural(1, "project") != "1 project" || plural(0, "project") != "0 projects" || plural(3, "editor") != "3 editors" {
		t.Error("plural")
	}
	if clamp(5, 0, 3) != 3 || clamp(-1, 0, 3) != 0 || clamp(2, 0, 3) != 2 || clamp(9, 0, -1) != 0 {
		t.Error("clamp")
	}
}

func stripANSI(s string) string { return ansi.Strip(s) }
