package tui

import (
	"errors"
	"strings"
	"testing"

	"github.com/ManeFunction/lazyunity/internal/unitycli"
)

// These tests assume lazyunity is verified with 1.0.*.
func verifiedWith10(t *testing.T) {
	t.Helper()
	min, max := unitycli.MinVersion, unitycli.MaxLine
	t.Cleanup(func() { unitycli.MinVersion, unitycli.MaxLine = min, max })
	unitycli.MinVersion, unitycli.MaxLine = unitycli.Version{Major: 1}, unitycli.Line{Major: 1, Minor: 0}
}

func ver(t *testing.T, s string) unitycli.Version {
	t.Helper()
	v, ok := unitycli.ParseVersion(s)
	if !ok {
		t.Fatalf("bad version %q", s)
	}
	return v
}

// confirmText is the message of the confirmation on top, unwrapped.
func confirmText(t *testing.T, m *Model) string {
	t.Helper()
	if len(m.popups) == 0 {
		t.Fatal("no popup")
	}
	c, ok := m.popups[len(m.popups)-1].(*confirmPopup)
	if !ok {
		t.Fatalf("not a confirmation: %T", m.popups[len(m.popups)-1])
	}
	return c.title + "\n" + c.message + "\n" + c.yes
}

func cliRowOf(m *Model) toolRow {
	m.refreshTools()
	return m.toolsTab.Items()[0]
}

func TestCLIRowFollowsTheVerifiedRange(t *testing.T) {
	verifiedWith10(t)
	m := newTestModel(t)
	f := useFakeCLI(t, m)

	// a newer line is kept off the row
	m.cliUpdate = unitycli.UpdateInfo{Current: "1.0.0", Latest: "1.1.0"}
	if row := cliRowOf(m); row.status != "ready · 1.0.0" || row.tone != toneGood {
		t.Errorf("an update beyond the verified range is not on the row: %+v", row)
	}
	// but the newest verified patch is
	m.cliVerified = ver(t, "1.0.3")
	if row := cliRowOf(m); row.status != "update available: 1.0.3 · enter to update" || row.tone != toneWarn {
		t.Errorf("a verified patch is offered in yellow: %+v", row)
	}

	// the menu offers both, the unverified one after a warning
	runToolRow(m, "cli")
	out := screenText(m)
	if !strings.Contains(out, "Update to 1.0.3") || !strings.Contains(out, "Not verified update 1.1.0 available") {
		t.Fatalf("menu:\n%s", out)
	}
	press(m, "j", "enter")
	if !strings.Contains(confirmText(t, m), "at your own risk") || len(f.Calls()) != 0 {
		t.Fatalf("an unverified update asks first:\n%s", screenText(m))
	}
	press(m, "esc")
	if len(f.Calls()) != 0 {
		t.Error("declining installs nothing")
	}
	m.popups = nil
	runToolRow(m, "cli")
	press(m, "j", "enter")
	drain(m, m.popups[len(m.popups)-1].(*confirmPopup).onYes(m))
	if !f.called("SelfUpdate", unitycli.UpdateOptions{Channel: "stable", Target: "1.1.0"}) {
		t.Errorf("calls: %v", f.Calls())
	}

	// a CLI beyond the range: yellow, with a way back
	m.cli, m.popups, m.tasks = f, nil, nil
	m.cliStatus = unitycli.Status{State: unitycli.Ready, Path: "unity", Version: ver(t, "1.1.0")}
	m.cliUpdate = unitycli.UpdateInfo{Current: ", lazyunity is checked with 1.0.*1.1.0", Latest: "1.1.0"}
	m.toast = toast{}
	if row := cliRowOf(m); !strings.HasPrefix(row.status, "1.1.0 · not verified") || row.tone != toneWarn {
		t.Errorf("an unverified CLI is yellow: %+v", row)
	}
	if !strings.Contains(m.statusView(), "Unity CLI not verified") {
		t.Errorf("the bottom line says so: %q", m.statusView())
	}
	runToolRow(m, "cli")
	out = screenText(m)
	// Rollback restores whatever the last update replaced, which may be
	// unverified too, so it is offered only in safe mode.
	if !strings.Contains(out, "Go back to verified 1.0.3") || strings.Contains(out, "Roll back to the previous version") {
		t.Errorf("menu:\n%s", out)
	}
}

func TestInstallIsPinnedToTheNewestVerified(t *testing.T) {
	verifiedWith10(t)
	m := newTestModel(t)
	m.cliStatus = unitycli.Status{}
	m.cliVerified = ver(t, "1.0.3")
	m.refreshTools()
	runToolRow(m, "cli")
	if msg := confirmText(t, m); !strings.Contains(msg, "Install Unity CLI 1.0.3, the newest version lazyunity is verified with") {
		t.Errorf("linux: %s", msg)
	}
	m.popups, m.goos = nil, "windows"
	runToolRow(m, "cli")
	if msg := confirmText(t, m); !strings.Contains(msg, "cannot install another") {
		t.Errorf("windows: %s", msg)
	}
	// the CDN not answering pins the oldest verified version
	m.popups, m.goos, m.cliVerified = nil, "linux", unitycli.Version{}
	runToolRow(m, "cli")
	if msg := confirmText(t, m); !strings.Contains(msg, "Install Unity CLI 1.0.0") {
		t.Errorf("no answer: %s", msg)
	}
}

func TestLatestVerifiedIsLoaded(t *testing.T) {
	m := newTestModel(t)
	recorder(m).verifiedCLI = "1.0.3"
	drain(m, m.reload(srcVerifiedCLI))
	if m.cliVerified.String() != "1.0.3" {
		t.Errorf("got %v", m.cliVerified)
	}
	recorder(m).verifiedCLI = ""
	drain(m, m.reload(srcVerifiedCLI))
	if m.cliVerified.String() != "1.0.3" {
		t.Error("a check that could not tell keeps what was known")
	}
}

func TestSafeMode(t *testing.T) {
	verifiedWith10(t)
	broken := cliError(unitycli.ExitBadArgs, "error: unknown option '--all'")

	// a verified CLI never goes to safe mode, nor does a network failure
	m := newTestModel(t)
	useFakeCLI(t, m)
	m.Update(projectsMsg{err: broken})
	if m.safe != nil {
		t.Error("a verified CLI is trusted: its failures are only errors")
	}
	m.cliStatus = unitycli.Status{State: unitycli.Ready, Path: "unity", Version: ver(t, "1.1.0")}
	m.Update(projectsMsg{err: cliError(unitycli.ExitNetwork, "offline")})
	m.Update(authMsg{err: cliError(unitycli.ExitAuth, "signed out")})
	if m.safe != nil {
		t.Error("the network or the sign-in failing does not mean the CLI changed")
	}

	m.popups = nil
	m.cliVerified = ver(t, "1.0.3")
	m.Update(projectsMsg{err: broken})
	if m.safe == nil || m.mode != modeSafe || m.focus != panelOptions {
		t.Fatalf("an unverified CLI that breaks a load means safe mode: safe %v mode %v", m.safe, m.mode)
	}
	msg := confirmText(t, m)
	for _, want := range []string{"Safe mode", "could not list the projects", "unknown option", "Downgrade to 1.0.3"} {
		if !strings.Contains(msg, want) {
			t.Errorf("the popup should say %q: %s", want, msg)
		}
	}
	m.popups = nil
	out := screenText(m)
	if strings.Contains(out, "[1]") || strings.Contains(out, "Account") || strings.Contains(out, "Space Shooter") {
		t.Errorf("only the Tools, trimmed to the CLI tools, are on screen:\n%s", out)
	}
	if row := cliRowOf(m); !strings.HasPrefix(row.status, "safe mode") || row.tone != toneBad {
		t.Errorf("row: %+v", row)
	}
	m.toast = toast{}
	if !strings.Contains(m.statusView(), "safe mode") {
		t.Errorf("bottom line: %q", m.statusView())
	}

	// the rest of the interface stays away
	m.cycleMode(1)
	m.setFocus(panelProjects)
	if m.mode != modeSafe || m.focus != panelOptions {
		t.Error("safe mode keeps to the Tools")
	}
	if m.loadEditorsCmd() != nil || m.needCLI() {
		t.Error("the CLI is not used for anything but mending it")
	}

	// a failed downgrade keeps the safe mode
	f := m.cli.(*fakeCLI)
	f.err["SelfUpdate"] = errors.New("offline")
	runToolRow(m, "cli")
	if !strings.Contains(screenText(m), "Roll back to the previous version") || !strings.Contains(screenText(m), "Reinstall 1.0.3 with the official installer") {
		t.Fatalf("safe mode menu:\n%s", screenText(m))
	}
	_, cmd := m.Update(keyMsg("enter"))
	drain(m, cmd)
	if !f.called("SelfUpdate", unitycli.UpdateOptions{Target: "1.0.3"}) || m.safe == nil {
		t.Fatalf("calls %v, safe %v", f.Calls(), m.safe)
	}

	// a verified CLI brings everything back
	cmd = m.onCLIStatus(cliStatusMsg{st: unitycli.Status{State: unitycli.Ready, Path: "/x/unity", Version: ver(t, "1.0.3")}})
	if m.safe != nil || m.mode != modeFull || cmd == nil {
		t.Errorf("the safe mode ends and the data loads again: safe %v mode %v", m.safe, m.mode)
	}
}

func TestForcedSafeMode(t *testing.T) {
	verifiedWith10(t)
	o := testOptions(t)
	o.Safe = true
	o.CLI.Version = ver(t, "1.0.3") // a verified CLI that breaks lazyunity anyway
	m, err := New(o)
	if err != nil {
		t.Fatal(err)
	}
	resize(m, 120, 36)
	f := useFakeCLI(t, m)
	m.cliVerified = ver(t, "1.0.3")

	if m.safe == nil || !m.safe.forced || m.mode != modeSafe || m.focus != panelOptions {
		t.Fatalf("--safe starts in safe mode: safe %+v mode %v", m.safe, m.mode)
	}
	if len(m.popups) != 0 {
		t.Error("asked for, safe mode needs no popup")
	}
	if m.loadProjectsCmd() != nil || m.loadEditorsCmd() != nil || m.needCLI() {
		t.Error("the CLI is not used for anything but changing its version")
	}
	var ids []string
	for _, r := range m.toolsTab.Items() {
		ids = append(ids, r.id)
	}
	if got := strings.Join(ids, " "); got != "cli spacer.license config spacer.config issues about" {
		t.Errorf("safe mode rows: %s", got)
	}
	opened := &recorder(m).urls
	runToolRow(m, "issues")
	if len(*opened) != 1 || (*opened)[0] != issuesURL {
		t.Errorf("Report an issue opens the issues page: %v", *opened)
	}
	if row := cliRowOf(m); row.status != "safe mode · 1.0.3 · enter for options" || row.tone != toneBad {
		t.Errorf("row: %+v", row)
	}
	m.toast = toast{}
	if !strings.Contains(m.statusView(), "started with --safe") {
		t.Errorf("bottom line: %q", m.statusView())
	}

	// already on the target: nothing to move to, but the rest is there
	runToolRow(m, "cli")
	out := screenText(m)
	if strings.Contains(out, "Downgrade to") || strings.Contains(out, "Update to") {
		t.Errorf("the CLI is on the target already:\n%s", out)
	}
	for _, want := range []string{"Roll back to the previous version", "About safe mode", "Leave safe mode"} {
		if !strings.Contains(out, want) {
			t.Errorf("the menu should offer %q:\n%s", want, out)
		}
	}

	// leaving brings everything back and loads it
	press(m, "down", "down", "down") // roll back, reinstall, about, leave
	_, cmd := m.Update(keyMsg("enter"))
	drain(m, cmd)
	if m.safe != nil || m.mode != modeFull || !f.called("Projects") {
		t.Errorf("leaving safe mode: safe %v mode %v calls %v", m.safe, m.mode, f.Calls())
	}
}

func TestFakeSafeMode(t *testing.T) {
	verifiedWith10(t)
	f := &Fake{}
	f.CLI.Version, f.CLI.Latest, f.CLI.Verified, f.CLI.Broken = "1.1.0", "1.1.0", "1.0.1", true
	o := testOptions(t)
	o.Projects, o.Local, o.Fake = nil, nil, f
	o.CLI = f.Status()
	m, err := New(o)
	if err != nil {
		t.Fatal(err)
	}
	resize(m, 120, 36)
	drain(m, m.loadProjectsCmd())
	if m.safe == nil {
		t.Fatal("a broken unverified CLI starts in safe mode")
	}
	drain(m, m.popups[0].(*confirmPopup).onYes(m))
	if m.safe != nil || m.cliStatus.Version.String() != "1.0.1" {
		t.Errorf("the downgrade mends the made-up CLI: safe %v, version %v", m.safe, m.cliStatus.Version)
	}
}

// An update beyond the verified range reaches nothing but the Unity CLI menu:
// not the row, the bottom line or a toast.
func TestUnverifiedUpdateStaysInTheMenu(t *testing.T) {
	verifiedWith10(t)
	m := newTestModel(t)
	useFakeCLI(t, m)
	m.toast = toast{}
	press(m, "3")
	m.Update(cliUpdateMsg{info: unitycli.UpdateInfo{Current: "1.0.0", Latest: "1.1.0"}})
	m.Update(verifiedCLIMsg{v: ver(t, "1.0.0"), ok: true}) // no newer verified patch
	if row := cliRowOf(m); row.status != "ready · 1.0.0" || row.tone != toneGood {
		t.Errorf("row: %+v", row)
	}
	if out := screenText(m); strings.Contains(out, "1.1.0") || strings.Contains(out, "update") {
		t.Errorf("the main screen must not mention the unverified update:\n%s", out)
	}
	runToolRow(m, "cli")
	if !strings.Contains(screenText(m), "Not verified update 1.1.0 available") {
		t.Errorf("the menu shows it:\n%s", screenText(m))
	}
}
