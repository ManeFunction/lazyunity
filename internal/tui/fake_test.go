package tui

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/ManeFunction/lazyunity/internal/config"
	"github.com/ManeFunction/lazyunity/internal/unitycli"
)

// TestFakeSample shows the sample file of the fake mode, which must keep
// loading as the format changes.
func TestFakeSample(t *testing.T) {
	f, err := LoadFake(filepath.Join("..", "..", "testdata", "promo.yml"))
	if err != nil {
		t.Fatal(err)
	}
	o := testOptions(t)
	o.Projects, o.Local, o.Fake = nil, nil, f
	o.CLI, o.InstallRoot = f.Status(), f.InstallRoot("linux")
	m, err := New(o)
	if err != nil {
		t.Fatal(err)
	}
	defer m.cancelTasks()
	resize(m, 130, 36)
	drain(m, m.loadAuthCmd())
	drain(m, m.probeOpenUPMCmd())
	// the running task: it starts, reports its frame, and then holds
	for _, c := range m.startCmd().(tea.BatchMsg) {
		if msg, ok := c().(taskStartedMsg); ok {
			_, cmd := m.Update(msg)
			m.Update(cmd())
		}
	}

	out := screenText(m)
	t.Log("\n" + out)
	for _, want := range []string{
		"Neon Drift", "✗ missing", "6000.3.28f1→6000.6.8f1",
		"6000.6.8f1", "2022.3.62f1 LTS", "Linux, Windows, Android, iOS",
		"signed in as alex@example.com", "ready · 4.5.3",
		"Installing 6000.7.0b8 + Web Build Support", "42%",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("the screen has no %q", want)
		}
	}
}

func TestFakeRejectsUnknownPlatforms(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fake.yml")
	os.WriteFile(path, []byte("editors:\n  - version: 6000.6.8f1\n    platforms: [gameboy]\n"), 0o644)
	if _, err := LoadFake(path); err == nil || !strings.Contains(err.Error(), "gameboy") {
		t.Fatalf("got %v, want an unknown platform error", err)
	}
}

func TestAgoDuration(t *testing.T) {
	for in, want := range map[string]time.Duration{
		"": 0, "45m": 45 * time.Minute, "2h": 2 * time.Hour, "3d": 72 * time.Hour, "2w": 14 * 24 * time.Hour,
	} {
		if got, err := agoDuration(in); err != nil || got != want {
			t.Errorf("agoDuration(%q) = %v, %v; want %v", in, got, err, want)
		}
	}
	if _, err := agoDuration("soon"); err == nil {
		t.Error("agoDuration(\"soon\") did not fail")
	}
}

// Without releases in the file, the real CLI lists them, marked installed by
// the made-up Editors.
func TestFakeRealReleases(t *testing.T) {
	real := &fakeCLI{err: map[string]error{}, releases: map[string]unitycli.ReleasePage{
		releasesKey("lts", 0, 25): {Items: []unitycli.Release{
			{Version: "6000.3.28f1", Stream: "LTS"}, {Version: "2022.3.62f1", Stream: "LTS", Installed: true},
		}},
	}}
	d := &demoCLI{real: real, editors: []unitycli.Editor{{Version: "6000.3.28f1"}}}
	page, err := d.Releases(context.Background(), "lts", 0, 25)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 2 || !page.Items[0].Installed || page.Items[1].Installed {
		t.Fatalf("got %+v, want 6000.3.28f1 installed and 2022.3.62f1 not", page.Items)
	}
	if !real.called("Releases", "lts", 0, 25) {
		t.Error("the real CLI was not asked for the releases")
	}
}

// With openupm.repo in the config, the fake OpenUPM CLI is a build of it.
func TestFakeOpenUPMFromRepo(t *testing.T) {
	for _, c := range []struct{ latest, want string }{
		{"", "4.5.3 · from someone/openupm-cli@main #3f9c2a7"},
		{"aaaaaaa1234", "update available: #aaaaaaa · from someone/openupm-cli@main #3f9c2a7"},
	} {
		f := &Fake{OpenUPM: FakeOpenUPM{Version: "4.5.3", LatestCommit: c.latest}}
		o := testOptions(t)
		o.Projects, o.Local, o.Fake = nil, nil, f
		o.Config.OpenUPM = config.OpenUPM{Repo: "https://github.com/someone/openupm-cli", Ref: "main"}
		m, err := New(o)
		if err != nil {
			t.Fatal(err)
		}
		drain(m, m.probeOpenUPMCmd())
		if got := m.openupmRow().status; got != c.want {
			t.Errorf("latestCommit %q: got %q, want %q", c.latest, got, c.want)
		}
	}
}
