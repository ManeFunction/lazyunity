package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/ManeFunction/lazyunity/internal/unitycli"
)

// fakeCLI is an in-memory Unity CLI. Every method records the call and
// answers from the fields below; a field left empty answers nothing, without
// an error. err makes a method fail, by name. How the calls are spelled on a
// real command line is unitycli's business, tested there.
type fakeCLI struct {
	mu    sync.Mutex
	calls []string

	projects       []unitycli.Project
	projectInfo    unitycli.ProjectInfo
	opened         unitycli.OpenResult
	templates      []unitycli.Template
	editors        []unitycli.Editor
	installPath    string
	modules        []unitycli.Module               // of an installed Editor
	releaseModules []unitycli.Module               // of a release, before it is installed
	releases       map[string]unitycli.ReleasePage // by releasesKey; a page not here fails
	plan           unitycli.InstallPlan
	frames         []unitycli.Frame // what an install reports before it ends
	cache          unitycli.CacheInfo
	auth           unitycli.AuthInfo
	update         unitycli.UpdateInfo
	license        json.RawMessage
	log            []unitycli.LogEntry
	err            map[string]error
}

// useFakeCLI gives the model an in-memory CLI that answers nothing until told.
func useFakeCLI(t *testing.T, m *Model) *fakeCLI {
	t.Helper()
	f := &fakeCLI{err: map[string]error{}}
	m.cli = f
	return f
}

// call is how a call is recorded: the method, then each argument as %+v.
func call(method string, args ...any) string {
	parts := []string{method}
	for _, a := range args {
		parts = append(parts, fmt.Sprintf("%+v", a))
	}
	return strings.Join(parts, " ")
}

func (f *fakeCLI) record(method string, args ...any) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, call(method, args...))
	return f.err[method]
}

// Calls lists the calls made so far.
func (f *fakeCLI) Calls() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.calls...)
}

// count is how many times the method was called with these arguments.
func (f *fakeCLI) count(method string, args ...any) int {
	want, n := call(method, args...), 0
	for _, c := range f.Calls() {
		if c == want {
			n++
		}
	}
	return n
}

func (f *fakeCLI) called(method string, args ...any) bool { return f.count(method, args...) > 0 }

func releasesKey(stream string, offset, limit int) string {
	return fmt.Sprintf("stream=%s offset=%d limit=%d", stream, offset, limit)
}

// fixture reads the items of a recorded CLI answer in testdata/cli.
func fixture[T any](t *testing.T, name string) []T {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "testdata", "cli", name))
	if err != nil {
		t.Fatal(err)
	}
	var env struct {
		Data struct {
			Items []T `json:"items"`
		} `json:"data"`
	}
	if err := json.Unmarshal(b, &env); err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	return env.Data.Items
}

// cliEditors is what the CLI lists when it knows exactly the Editors the model
// found on disk.
func cliEditors(m *Model) []unitycli.Editor {
	var out []unitycli.Editor
	for _, e := range m.inv.editors() {
		out = append(out, unitycli.Editor{Version: e.Version, Architecture: e.Arch, Location: e.Location, Default: e.Default})
	}
	return out
}

func cliError(code int, message string) *unitycli.Error {
	return &unitycli.Error{ExitCode: code, Message: message}
}

// ---- unityCLI ------------------------------------------------------------------

func (f *fakeCLI) Projects(context.Context) ([]unitycli.Project, error) {
	return f.projects, f.record("Projects")
}

func (f *fakeCLI) ProjectInfo(_ context.Context, path string) (unitycli.ProjectInfo, error) {
	return f.projectInfo, f.record("ProjectInfo", path)
}

func (f *fakeCLI) AddProjects(_ context.Context, paths ...string) error {
	return f.record("AddProjects", paths)
}

func (f *fakeCLI) RemoveProjects(_ context.Context, paths ...string) error {
	return f.record("RemoveProjects", paths)
}

func (f *fakeCLI) SetPinned(_ context.Context, path string, pinned bool) error {
	return f.record("SetPinned", path, pinned)
}

func (f *fakeCLI) CleanProject(_ context.Context, path string) error {
	return f.record("CleanProject", path)
}

func (f *fakeCLI) Open(_ context.Context, o unitycli.OpenOptions) (unitycli.OpenResult, error) {
	return f.opened, f.record("Open", o)
}

func (f *fakeCLI) UpgradeProject(_ context.Context, path, to string, cloud unitycli.CloudChoice) error {
	return f.record("UpgradeProject", path, to, cloud)
}

func (f *fakeCLI) CreateProject(_ context.Context, o unitycli.CreateOptions) error {
	return f.record("CreateProject", o)
}

func (f *fakeCLI) Templates(_ context.Context, editorVersion string) ([]unitycli.Template, error) {
	return f.templates, f.record("Templates", editorVersion)
}

func (f *fakeCLI) Editors(context.Context) ([]unitycli.Editor, error) {
	return f.editors, f.record("Editors")
}

func (f *fakeCLI) AddEditors(_ context.Context, paths ...string) error {
	return f.record("AddEditors", paths)
}

func (f *fakeCLI) SetDefaultEditor(_ context.Context, version string) error {
	return f.record("SetDefaultEditor", version)
}

func (f *fakeCLI) Releases(_ context.Context, stream string, offset, limit int) (unitycli.ReleasePage, error) {
	if err := f.record("Releases", stream, offset, limit); err != nil {
		return unitycli.ReleasePage{}, err
	}
	page, ok := f.releases[releasesKey(stream, offset, limit)]
	if !ok {
		return page, fmt.Errorf("fake CLI: no releases page %s", releasesKey(stream, offset, limit))
	}
	return page, nil
}

func (f *fakeCLI) InstallPath(context.Context) (string, error) {
	return f.installPath, f.record("InstallPath")
}

func (f *fakeCLI) SetInstallPath(_ context.Context, path string) error {
	err := f.record("SetInstallPath", path)
	if err == nil {
		f.installPath = path
	}
	return err
}

func (f *fakeCLI) Modules(_ context.Context, version, arch string) ([]unitycli.Module, error) {
	return f.modules, f.record("Modules", version, arch)
}

func (f *fakeCLI) ReleaseModules(_ context.Context, version string) ([]unitycli.Module, error) {
	return f.releaseModules, f.record("ReleaseModules", version)
}

func (f *fakeCLI) PlanInstall(_ context.Context, o unitycli.InstallOptions) (unitycli.InstallPlan, error) {
	return f.plan, f.record("PlanInstall", o)
}

func (f *fakeCLI) stream(method string, args ...any) (*unitycli.Stream, error) {
	if err := f.record(method, args...); err != nil {
		return nil, err
	}
	return unitycli.FinishedStream(nil, f.frames...), nil
}

func (f *fakeCLI) Install(_ context.Context, o unitycli.InstallOptions) (*unitycli.Stream, error) {
	return f.stream("Install", o)
}

func (f *fakeCLI) InstallModules(_ context.Context, version, arch string, modules []string) (*unitycli.Stream, error) {
	return f.stream("InstallModules", version, arch, modules)
}

func (f *fakeCLI) RemoveModules(_ context.Context, version, arch string, modules []string) error {
	return f.record("RemoveModules", version, arch, modules)
}

func (f *fakeCLI) Uninstall(_ context.Context, version, arch string) error {
	return f.record("Uninstall", version, arch)
}

func (f *fakeCLI) UpgradeEditor(_ context.Context, version string) (*unitycli.Stream, error) {
	return f.stream("UpgradeEditor", version)
}

func (f *fakeCLI) DownloadCache(context.Context) (unitycli.CacheInfo, error) {
	return f.cache, f.record("DownloadCache")
}

func (f *fakeCLI) CleanDownloadCache(context.Context) error { return f.record("CleanDownloadCache") }

func (f *fakeCLI) AuthStatus(context.Context) (unitycli.AuthInfo, error) {
	return f.auth, f.record("AuthStatus")
}

func (f *fakeCLI) Login(context.Context) error  { return f.record("Login") }
func (f *fakeCLI) Logout(context.Context) error { return f.record("Logout") }

func (f *fakeCLI) LicenseStatus(context.Context) (json.RawMessage, error) {
	return f.license, f.record("LicenseStatus")
}

func (f *fakeCLI) ActivatePersonalLicense(context.Context) error {
	return f.record("ActivatePersonalLicense")
}

func (f *fakeCLI) ReturnLicense(context.Context) error { return f.record("ReturnLicense") }

func (f *fakeCLI) SelfUpdate(_ context.Context, channel string) error {
	return f.record("SelfUpdate", channel)
}

func (f *fakeCLI) CheckUpdate(context.Context) (unitycli.UpdateInfo, error) {
	return f.update, f.record("CheckUpdate")
}

func (f *fakeCLI) Log() []unitycli.LogEntry { return f.log }

var _ unityCLI = (*fakeCLI)(nil)
