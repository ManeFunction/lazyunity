package unitycli

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

func ctx5(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	t.Cleanup(cancel)
	return ctx
}

func TestProjects(t *testing.T) {
	c := fakeClient(t, map[string]fakeResp{"projects --all": {File: "projects.json"}})
	ps, err := c.Projects(ctx5(t))
	if err != nil {
		t.Fatal(err)
	}
	if len(ps) != 1 {
		t.Fatalf("got %d projects", len(ps))
	}
	p := ps[0]
	if p.Title != "Demo Project" || p.Version != "6000.6.8f1" || p.BuildTarget != "StandaloneLinux64" ||
		p.IsFavorite || !p.CloudEnabled || p.RenderPipeline != "URP" {
		t.Errorf("unexpected project: %+v", p)
	}
	if got := p.LastOpened(); got.IsZero() || got.UnixMilli() != 1793890375239 {
		t.Errorf("LastOpened = %v", got)
	}
	if (Project{}).LastOpened().IsZero() == false {
		t.Error("a project with no date should have a zero LastOpened")
	}
}

func TestEditorsModulesReleasesTemplates(t *testing.T) {
	c := fakeClient(t, map[string]fakeResp{
		"editors --installed":                                       {File: "editors-installed.json"},
		"modules 6000.6.8f1":                                        {File: "modules.json"},
		"releases --limit 3 --offset 0":                             {File: "releases.json"},
		"templates --installed --brief --editor-version 6000.6.8f1": {File: "templates.json"},
		"install-path --get":                                        {File: "install-path.json"},
	})
	ctx := ctx5(t)

	eds, err := c.Editors(ctx)
	if err != nil || len(eds) != 1 {
		t.Fatalf("Editors: %v %v", eds, err)
	}
	if e := eds[0]; e.Version != "6000.6.8f1" || e.Alias != "6.6.8f1" || e.Architecture != "x86_64" ||
		!strings.HasSuffix(e.Location, "Editor/Unity") || len(e.ModuleNames) != 1 || e.Default {
		t.Errorf("editor: %+v", e)
	}

	mods, err := c.Modules(ctx, "6000.6.8f1", "")
	if err != nil || len(mods) == 0 {
		t.Fatalf("Modules: %d %v", len(mods), err)
	}
	var sawPlatform bool
	for _, m := range mods {
		if m.ID == "" || m.Name == "" {
			t.Errorf("module without id or name: %+v", m)
		}
		if m.Category == "Platforms" && m.DownloadBytes > 0 {
			sawPlatform = true
		}
	}
	if !sawPlatform {
		t.Error("no platform module with a download size")
	}

	page, err := c.Releases(ctx, "", 0, 3)
	if err != nil || len(page.Items) != 3 {
		t.Fatalf("Releases: %+v %v", page, err)
	}
	if page.NextOffset != 3 {
		t.Errorf("NextOffset = %d, want 3", page.NextOffset)
	}
	if page.Items[0].Version == "" || page.Items[0].Stream == "" {
		t.Errorf("release: %+v", page.Items[0])
	}

	ts, err := c.Templates(ctx, "6000.6.8f1")
	if err != nil || len(ts) != 2 || ts[1].Name != "com.unity.template.urp-blank" || ts[1].DisplayName != "3D URP" {
		t.Errorf("Templates: %+v %v", ts, err)
	}

	ip, err := c.InstallPath(ctx)
	if err != nil || ip != "/home/user/Unity/Hub/Editor" {
		t.Errorf("InstallPath = %q, %v", ip, err)
	}
}

func TestReleasesPaging(t *testing.T) {
	c := fakeClient(t, map[string]fakeResp{
		"releases --limit 25 --offset 50 --stream lts": {Stdout: `{"success":true,"command":"releases","data":{"items":[],"nextPage":null}}`},
	})
	page, err := c.Releases(ctx5(t), "lts", 50, 25)
	if err != nil || page.NextOffset != 0 || len(page.Items) != 0 {
		t.Errorf("got %+v, %v", page, err)
	}
}

func TestAuthStatus(t *testing.T) {
	ctx := ctx5(t)

	in := fakeClient(t, map[string]fakeResp{"auth status": {File: "auth-status-in.json"}})
	info, err := in.AuthStatus(ctx)
	if err != nil || !info.LoggedIn || info.User.Email != "test@example.com" || info.Mode != "oauth" {
		t.Errorf("signed in: %+v %v", info, err)
	}

	out := fakeClient(t, map[string]fakeResp{"auth status": {File: "auth-status-out.json", Exit: ExitAuth}})
	info, err = out.AuthStatus(ctx)
	if err != nil || info.LoggedIn {
		t.Errorf("signed out should not be an error: %+v %v", info, err)
	}

	broken := fakeClient(t, map[string]fakeResp{"auth status": {Stderr: "boom\nsecond", Exit: ExitNetwork}})
	if _, err = broken.AuthStatus(ctx); err == nil {
		t.Error("a network failure must be an error")
	}
}

func TestErrorMapping(t *testing.T) {
	notAProject := `{"success":false,"command":"projects info","data":null,"errors":[{"code":"NOT_A_PROJECT","message":"Not a Unity project."}],"warnings":[]}`
	c := fakeClient(t, map[string]fakeResp{
		"projects info /nope":    {Stdout: notAProject, Exit: ExitNotFound},
		"projects info /plain":   {Stderr: "something broke\nmore detail", Exit: 1},
		"projects info /badjson": {Stdout: "garbage", Exit: 0},
		"projects info /lying":   {Stdout: `{"success":false,"errors":[{"code":"X","message":"said no"}]}`, Exit: 0},
		"install-modules --editor-version 1 --module a --child-modules --accept-eula --yes": {Exit: ExitInstallLocked, Stdout: "{}"},
	})
	ctx := ctx5(t)

	_, err := c.ProjectInfo(ctx, "/nope")
	var e *Error
	if !errors.As(err, &e) {
		t.Fatalf("err = %v", err)
	}
	if !e.NotFound() || e.Code != "NOT_A_PROJECT" || e.Message != "Not a Unity project." || e.ExitCode != ExitNotFound {
		t.Errorf("error: %+v", e)
	}
	if e.NotSignedIn() || e.Cancelled() || e.Network() || e.NeedsConfig() || e.InstallLocked() {
		t.Error("wrong predicates true")
	}

	_, err = c.ProjectInfo(ctx, "/plain")
	if !errors.As(err, &e) || e.Message != "something broke" || e.ExitCode != 1 {
		t.Errorf("plain failure: %+v", e)
	}

	_, err = c.ProjectInfo(ctx, "/badjson")
	if !errors.As(err, &e) || !strings.Contains(e.Message, "unexpected output") {
		t.Errorf("bad json: %+v", e)
	}

	_, err = c.ProjectInfo(ctx, "/lying")
	if !errors.As(err, &e) || e.Code != "X" || e.Message != "said no" {
		t.Errorf("success=false with exit 0: %+v", e)
	}

	// an error without a message falls back to describing the exit code
	if got := (&Error{Args: []string{"open", "x"}, ExitCode: 6}).Error(); got != "unity open x: exit code 6" {
		t.Errorf("Error() = %q", got)
	}
}

func TestCannotStart(t *testing.T) {
	c := New("/definitely/not/here/unity")
	_, err := c.Projects(ctx5(t))
	var e *Error
	if !errors.As(err, &e) || e.ExitCode != -1 || !strings.Contains(e.Message, "could not start") {
		t.Errorf("err = %v", err)
	}
}

func TestEnvironment(t *testing.T) {
	c := fakeClient(t, nil)
	ctx := ctx5(t)

	env, err := c.Run(ctx, "env-probe")
	if err != nil {
		t.Fatal(err)
	}
	var d map[string]string
	json.Unmarshal(env.Data, &d)
	if d["format"] != "json" || d["nonInteractive"] != "1" || d["banner"] != "1" {
		t.Errorf("normal run environment: %v", d)
	}

	env, err = c.run(ctx, runOpts{interactive: true}, []string{"env-probe"})
	if err != nil {
		t.Fatal(err)
	}
	json.Unmarshal(env.Data, &d)
	if d["nonInteractive"] != "" {
		t.Errorf("an interactive run must not set UNITY_NON_INTERACTIVE, got %q", d["nonInteractive"])
	}
}

// TestArguments pins the exact command lines, which is what keeps lazyunity in
// step with the CLI. The fake exits 99 on any argument list it does not expect.
func TestArguments(t *testing.T) {
	ok := fakeResp{Stdout: `{"success":true,"command":"x","data":{}}`}
	table := map[string]fakeResp{
		"projects add /a /b":                                   ok,
		"projects remove /a --force":                           ok,
		"projects pin /a":                                      ok,
		"projects unpin /a":                                    ok,
		"projects clean /a --yes":                              ok,
		"projects upgrade /a --to 6000.7.0f1 --yes --no-cloud": ok,
		"editors add /e":                                       ok,
		"editors default 6000.6.8f1":                           ok,
		"install-path --set /new":                              ok,
		"editors module remove 6000.6.8f1 --module webgl --module ios --architecture x86_64 --yes": ok,
		"uninstall 6000.6.8f1 --yes":                ok,
		"auth logout --yes":                         ok,
		"license activate --personal --accept-eula": ok,
		"license return --yes":                      ok,
		"self-update --yes --channel beta":          ok,
		"open /p":                                   {Stdout: `{"success":true,"command":"open","data":{"alreadyOpen":true}}`},
		"open /p --editor-version 6000.6.8f1 --editor-path /e/Unity --build-target StandaloneLinux64":                                      ok,
		"projects create My Game --path /par --editor-version 6000.6.8f1 --template com.unity.template.urp-blank --no-cloud --bare --open": ok,
		"projects create G2 --path /par --editor-version 6000.6.8f1 --cloud":                                                               ok,
		"install 6000.5.1f1 --changeset abc --architecture arm64 --module webgl --module android --dry-run":                                {File: "install-dryrun.ndjson"},
		"editors default":         {Stdout: `{"success":true,"data":{"default":"6000.6.8f1"}}`},
		"editors info 6000.5.1f1": {File: "editors-info.json"},
	}
	c := fakeClient(t, table)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute) // about 30 calls; a race build starts slowly
	defer cancel()

	check := func(name string, err error) {
		t.Helper()
		if err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
	check("AddProjects", c.AddProjects(ctx, "/a", "/b"))
	check("RemoveProjects", c.RemoveProjects(ctx, "/a"))
	check("pin", c.SetPinned(ctx, "/a", true))
	check("unpin", c.SetPinned(ctx, "/a", false))
	check("CleanProject", c.CleanProject(ctx, "/a"))
	check("UpgradeProject", c.UpgradeProject(ctx, "/a", "6000.7.0f1", CloudOff))
	check("AddEditors", c.AddEditors(ctx, "/e"))
	check("SetDefaultEditor", c.SetDefaultEditor(ctx, "6000.6.8f1"))
	check("SetInstallPath", c.SetInstallPath(ctx, "/new"))
	check("RemoveModules", c.RemoveModules(ctx, "6000.6.8f1", "x86_64", []string{"webgl", "ios"}))
	check("Uninstall", c.Uninstall(ctx, "6000.6.8f1", ""))
	check("Logout", c.Logout(ctx))
	check("ActivatePersonalLicense", c.ActivatePersonalLicense(ctx))
	check("ReturnLicense", c.ReturnLicense(ctx))
	check("SelfUpdate", c.SelfUpdate(ctx, "beta"))

	res, err := c.Open(ctx, OpenOptions{Project: "/p"})
	check("Open", err)
	if !res.AlreadyOpen {
		t.Error("Open should report alreadyOpen")
	}
	_, err = c.Open(ctx, OpenOptions{Project: "/p", EditorVersion: "6000.6.8f1", EditorPath: "/e/Unity", BuildTarget: "StandaloneLinux64"})
	check("Open with options", err)

	check("CreateProject", c.CreateProject(ctx, CreateOptions{
		Name: "My Game", ParentDir: "/par", EditorVersion: "6000.6.8f1",
		Template: "com.unity.template.urp-blank", Bare: true, Open: true,
	}))
	check("CreateProject cloud", c.CreateProject(ctx, CreateOptions{
		Name: "G2", ParentDir: "/par", EditorVersion: "6000.6.8f1", Cloud: CloudNew,
	}))

	plan, err := c.PlanInstall(ctx, InstallOptions{
		Version: "6000.5.1f1", Changeset: "abc", Architecture: "arm64", Modules: []string{"webgl", "android"},
	})
	check("PlanInstall", err)
	if plan.AlreadyInstalled || plan.Editor.Version != "6000.5.1f1" || plan.TotalDownloadSize != 5996112104 ||
		len(plan.Modules) != 1 || plan.Modules[0].ID != "webgl" {
		t.Errorf("plan: %+v", plan)
	}

	def, err := c.DefaultEditor(ctx)
	if err != nil || def != "6000.6.8f1" {
		t.Errorf("DefaultEditor = %q, %v", def, err)
	}
	mods, err := c.ReleaseModules(ctx, "6000.5.1f1")
	if err != nil || len(mods) == 0 {
		t.Errorf("ReleaseModules: %d %v", len(mods), err)
	}

	// every invocation is in the log, in order, rendered like a shell command
	log := c.Log()
	if len(log) == 0 || log[0].Command() != "unity projects add /a /b" {
		t.Errorf("first log entry: %v", log)
	}
	var quoted bool
	for _, e := range log {
		if e.Command() == `unity projects create "My Game" --path /par --editor-version 6000.6.8f1 --template com.unity.template.urp-blank --no-cloud --bare --open` {
			quoted = true
		}
	}
	if !quoted {
		t.Error("an argument with a space should be quoted in the log")
	}
}

func TestDefaultEditorNone(t *testing.T) {
	c := fakeClient(t, map[string]fakeResp{"editors default": {Stdout: `{"success":true,"data":{"default":null}}`}})
	def, err := c.DefaultEditor(ctx5(t))
	if err != nil || def != "" {
		t.Errorf("DefaultEditor = %q, %v", def, err)
	}
}

func TestLogHookAndLimit(t *testing.T) {
	c := fakeClient(t, map[string]fakeResp{"projects pin /a": {Stdout: `{"success":true,"data":{}}`}})
	var seen int
	c.OnLog(func(LogEntry) { seen++ })

	// a real call is logged and reported to the hook
	if err := c.SetPinned(ctx5(t), "/a", true); err != nil {
		t.Fatal(err)
	}
	if seen != 1 || len(c.Log()) != 1 {
		t.Fatalf("hook saw %d, log holds %d", seen, len(c.Log()))
	}

	// the log is bounded and keeps the newest entries
	for i := 0; i < maxLogEntries+5; i++ {
		c.record(LogEntry{Args: []string{"n", string(rune('a' + i%26))}, ExitCode: i})
	}
	log := c.Log()
	if len(log) != maxLogEntries {
		t.Fatalf("log holds %d entries, want %d", len(log), maxLogEntries)
	}
	if last := log[len(log)-1]; last.ExitCode != maxLogEntries+4 {
		t.Errorf("newest entry lost: %+v", last)
	}
	if seen != maxLogEntries+6 {
		t.Errorf("hook saw %d entries, want %d", seen, maxLogEntries+6)
	}
}

func TestRunCancel(t *testing.T) {
	c := fakeClient(t, map[string]fakeResp{"projects pin /slow": {Sleep: "60s"}})
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(300 * time.Millisecond)
		cancel()
	}()
	start := time.Now()
	err := c.SetPinned(ctx, "/slow", true)
	if err == nil {
		t.Fatal("a cancelled command must fail")
	}
	if d := time.Since(start); d > 15*time.Second {
		t.Errorf("cancelling took %v", d)
	}
}

func TestReleaseModulesReadsEditorsInfo(t *testing.T) {
	c := fakeClient(t, map[string]fakeResp{"editors info 6000.5.1f1": {File: "editors-info.json"}})
	mods, err := c.ReleaseModules(context.Background(), "6000.5.1f1")
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]Module{}
	for _, m := range mods {
		byID[m.ID] = m
	}
	android, ok := byID["android"]
	if !ok || android.Name != "Android Build Support" || android.Category != "Platforms" || android.DownloadBytes == 0 || android.InstalledBytes == 0 {
		t.Errorf("android = %+v", android)
	}
	if byID["language-ja"].Category != "Language packs (Preview)" || byID["documentation"].Category != "Documentation" {
		t.Errorf("categories: %+v %+v", byID["language-ja"], byID["documentation"])
	}
}
