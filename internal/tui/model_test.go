package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/ManeFunction/lazyunity/internal/config"
	"github.com/ManeFunction/lazyunity/internal/state"
	"github.com/ManeFunction/lazyunity/internal/unity"
	"github.com/ManeFunction/lazyunity/internal/unitycli"
)

var testNow = time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)

// testOptions is a deterministic starting point: a ready CLI, a few projects,
// two Editors on disk. Nothing is read from the real machine.
func testOptions(t *testing.T) Options {
	t.Helper()
	dir := t.TempDir()
	root := filepath.Join(dir, "Hub", "Editor")
	mk := func(ver string, meta string) unity.Installed {
		exe := filepath.Join(root, ver, unity.ExecutableRel("linux"))
		if err := os.MkdirAll(filepath.Dir(exe), 0o755); err != nil {
			t.Fatal(err)
		}
		os.WriteFile(exe, nil, 0o755)
		os.WriteFile(filepath.Join(root, ver, "metadata.hub.json"), []byte(meta), 0o644)
		return unity.Installed{Version: ver, Dir: filepath.Join(root, ver), Location: exe, Root: root}
	}
	e1 := mk("6000.6.8f1", `{"architecture":"x86_64"}`)
	e2 := mk("2022.3.62f1", `{"architecture":"x86_64","isLTS":true}`)
	e1.Architecture, e2.Architecture, e2.LTS = "x86_64", "x86_64", true
	os.MkdirAll(filepath.Join(e2.Dir, "Editor", "Data", "PlaybackEngines", "AndroidPlayer"), 0o755)
	os.WriteFile(filepath.Join(e2.Dir, "Editor", "Data", "PlaybackEngines", "AndroidPlayer", "x"), nil, 0o644)

	ago := func(d time.Duration) int64 { return testNow.Add(-d).UnixMilli() }
	return Options{
		Config:    config.Default(),
		State:     state.Load(filepath.Join(dir, "cfg")),
		ConfigDir: filepath.Join(dir, "cfg"),
		Version:   "test",
		CLI:       unitycli.Status{State: unitycli.Ready, Path: "/usr/bin/unity", Version: unitycli.MinVersion},
		Env:       unitycli.Env{GOOS: "linux", Home: dir},
		GOOS:      "linux",
		Now:       func() time.Time { return testNow },
		Projects: []unitycli.Project{
			{Title: "Space Shooter", Path: filepath.Join(dir, "p", "space"), Version: "6000.6.8f1", BuildTarget: "StandaloneLinux64", LastModified: ago(2 * time.Hour), IsFavorite: true},
			{Title: "Old Platformer", Path: filepath.Join(dir, "p", "old"), Version: "2022.3.62f1", BuildTarget: "Android", LastModified: ago(3 * 24 * time.Hour)},
			{Title: "Ancient Thing", Path: filepath.Join(dir, "p", "ancient"), Version: "2019.4.40f1", BuildTarget: "StandaloneLinux64", LastModified: ago(400 * 24 * time.Hour)},
		},
		Local:       []unity.Installed{e1, e2},
		InstallRoot: root,
	}
}

// newTestModel builds a Model at 120x36 with its first layout done.
func newTestModel(t *testing.T) *Model {
	t.Helper()
	m, err := New(testOptions(t))
	if err != nil {
		t.Fatal(err)
	}
	// the test projects' folders exist, except the last
	for _, p := range m.rawProjects[:2] {
		os.MkdirAll(p.Path, 0o755)
	}
	m.missing = map[string]bool{m.rawProjects[2].Path: true}
	m.refreshProjects()
	resize(m, 120, 36)
	return m
}

func resize(m *Model, w, h int) {
	m.Update(tea.WindowSizeMsg{Width: w, Height: h})
}

// press sends keys: single characters are typed, names such as "enter" or
// "ctrl+s" are sent as that key.
func press(m *Model, keys ...string) {
	for _, k := range keys {
		m.Update(keyMsg(k))
	}
}

func keyMsg(k string) tea.KeyPressMsg {
	special := map[string]rune{
		"enter": tea.KeyEnter, "esc": tea.KeyEscape, "tab": tea.KeyTab, "up": tea.KeyUp, "down": tea.KeyDown,
		"left": tea.KeyLeft, "right": tea.KeyRight, "pgup": tea.KeyPgUp, "pgdown": tea.KeyPgDown,
		"home": tea.KeyHome, "end": tea.KeyEnd, "backspace": tea.KeyBackspace,
	}
	if r, ok := special[k]; ok {
		return tea.KeyPressMsg{Code: r}
	}
	if k == "shift+tab" {
		return tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift}
	}
	if k == "space" {
		return tea.KeyPressMsg{Code: ' ', Text: " "}
	}
	if rest, ok := strings.CutPrefix(k, "ctrl+"); ok {
		return tea.KeyPressMsg{Code: rune(rest[0]), Mod: tea.ModCtrl}
	}
	r := []rune(k)[0]
	return tea.KeyPressMsg{Code: r, Text: k}
}

// screen is the rendered interface without colours, one string per line.
func screen(m *Model) []string {
	return strings.Split(ansi.Strip(m.View().Content), "\n")
}

func screenText(m *Model) string { return strings.Join(screen(m), "\n") }

func TestRenderOverview(t *testing.T) {
	m := newTestModel(t)
	out := screenText(m)
	t.Log("\n" + out)
	if len(screen(m)) != 36 {
		t.Errorf("the screen is %d lines tall, want 36", len(screen(m)))
	}
	for i, l := range screen(m) {
		if width(l) != 120 {
			t.Errorf("line %d is %d cells wide, want 120: %q", i, width(l), l)
		}
	}
}

func authInfo(loggedIn bool) unitycli.AuthInfo { return unitycli.AuthInfo{LoggedIn: loggedIn} }

// drain executes a command, and every command its messages lead to, feeding the
// messages back into the model. Clock ticks are dropped: they only reschedule
// themselves.
func drain(m *Model, cmd tea.Cmd) {
	if cmd == nil {
		return
	}
	msg := cmd()
	switch msg := msg.(type) {
	case nil:
		return
	case tickMsg, spinMsg:
		return
	case tea.QuitMsg:
		m.quit = true
		return
	case tea.BatchMsg:
		for _, c := range msg {
			drain(m, c)
		}
		return
	}
	// tea.Sequence and similar wrap their commands in an unexported message
	if seq := sequenceOf(msg); seq != nil {
		for _, c := range seq {
			drain(m, c)
		}
		return
	}
	_, next := m.Update(msg)
	drain(m, next)
}

func sleepMs(n int) { time.Sleep(time.Duration(n) * time.Millisecond) }

func TestListFooterCanBeTurnedOff(t *testing.T) {
	m := newTestModel(t)
	if !strings.Contains(screenText(m), "1 of 3") {
		t.Fatalf("the Projects panel shows where the cursor is:\n%s", screenText(m))
	}
	m.cfg.GUI.ShowListFooter = false
	if strings.Contains(screenText(m), "1 of 3") {
		t.Errorf("gui.showListFooter: false hides it:\n%s", screenText(m))
	}
}

func TestNewScansTheEditorFoldersItself(t *testing.T) {
	o := testOptions(t)
	extra := t.TempDir()
	o.State.ExtraRoots = []string{extra, extra + string(filepath.Separator)} // the same folder twice
	o.Config.Editors.ExtraRoots = []string{extra}
	o.Local = nil
	m, err := New(o)
	if err != nil {
		t.Fatal(err)
	}
	if len(m.editors) != 2 {
		t.Errorf("both Editors of the install folder are found: %+v", m.editors)
	}
	if got := m.allRoots(); len(got) != 2 {
		t.Errorf("each folder is looked in once: %q", got)
	}
}
