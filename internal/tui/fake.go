package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"
	"gopkg.in/yaml.v3"

	"github.com/ManeFunction/lazyunity/internal/config"
	"github.com/ManeFunction/lazyunity/internal/state"
	"github.com/ManeFunction/lazyunity/internal/unity"
	"github.com/ManeFunction/lazyunity/internal/unitycli"
)

// The fake mode (the undocumented --fake flag) shows a made-up machine: the
// projects, Editors, releases, tools and tasks of a YAML file, for
// screenshots. Nothing is read from the Unity CLI or the disk, and what is
// done in the interface changes only the made-up data.

// Fake is the file the fake mode shows.
type Fake struct {
	CLI struct {
		Version string `yaml:"version"` // "" or "missing": not installed
		Latest  string `yaml:"latest"`  // newer than version: an update is offered
		Channel string `yaml:"channel"` // stable or beta
	} `yaml:"cli"`
	Account struct {
		SignedIn bool   `yaml:"signedIn"`
		Email    string `yaml:"email"`
		Name     string `yaml:"name"`
	} `yaml:"account"`
	OpenUPM        FakeOpenUPM `yaml:"openupm"`
	EditorsFolder  string      `yaml:"editorsFolder"`
	ProjectsFolder string      `yaml:"projectsFolder"`

	Projects []FakeProject `yaml:"projects"`
	Editors  []FakeEditor  `yaml:"editors"`
	Releases []FakeRelease `yaml:"releases"`
	Tasks    []FakeTask    `yaml:"tasks"`
}

// FakeOpenUPM is the OpenUPM CLI of the fake mode. With openupm.repo set in
// config.yml, it is a build of that repository, at commit; latestCommit
// newer than it offers an update. Otherwise it is the npm release.
type FakeOpenUPM struct {
	Version      string `yaml:"version"` // "": not installed
	Latest       string `yaml:"latest"`  // the newest npm release
	Commit       string `yaml:"commit"`
	LatestCommit string `yaml:"latestCommit"`
}

// fakeCommit is the commit of a build when the file names none.
const fakeCommit = "3f9c2a71d4e8b05a6c1f7e92d3b84a0c5e6f1d27"

// FakeProject is a project of the fake mode.
type FakeProject struct {
	Title    string `yaml:"title"`
	Path     string `yaml:"path"`
	Version  string `yaml:"version"`
	Platform string `yaml:"platform"` // a platform id, label or BuildTarget: linux, Android, StandaloneOSX ...
	Opened   string `yaml:"opened"`   // how long ago: 45m, 2h, 3d, 2w
	Pinned   bool   `yaml:"pinned"`
	Missing  bool   `yaml:"missing"`
	Pipeline string `yaml:"renderPipeline"`
	// Pending is a version or platform chosen but not applied yet.
	Pending struct {
		Version  string `yaml:"version"`
		Platform string `yaml:"platform"`
	} `yaml:"pending"`
}

// FakeEditor is an installed Editor of the fake mode.
type FakeEditor struct {
	Version   string   `yaml:"version"`
	Arch      string   `yaml:"arch"`
	LTS       bool     `yaml:"lts"`
	Default   bool     `yaml:"default"`
	Platforms []string `yaml:"platforms"`
}

// FakeRelease is a release of the fake mode's Releases tab.
type FakeRelease struct {
	Version string `yaml:"version"`
	Arch    string `yaml:"arch"`
	Stream  string `yaml:"stream"` // LTS, SUPPORTED, BETA, ALPHA
}

// FakeTask is a task of the fake mode. The first one runs, stuck where it
// says; the others wait in the queue.
type FakeTask struct {
	Title   string   `yaml:"title"`
	Pct     *float64 `yaml:"pct"`
	Phase   string   `yaml:"phase"`
	Message string   `yaml:"message"`
}

// LoadFake reads a fake mode file.
func LoadFake(path string) (*Fake, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var f Fake
	dec := yaml.NewDecoder(strings.NewReader(string(b)))
	dec.KnownFields(true)
	if err := dec.Decode(&f); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	for _, p := range f.Projects {
		if _, err := agoDuration(p.Opened); err != nil {
			return nil, fmt.Errorf("%s: project %q: opened: %w", path, p.Title, err)
		}
		for _, pl := range []string{p.Platform, p.Pending.Platform} {
			if _, ok := fakePlatform(pl); pl != "" && !ok {
				return nil, fmt.Errorf("%s: project %q: unknown platform %q", path, p.Title, pl)
			}
		}
	}
	for _, e := range f.Editors {
		for _, pl := range e.Platforms {
			if _, ok := fakePlatform(pl); !ok {
				return nil, fmt.Errorf("%s: Editor %s: unknown platform %q", path, e.Version, pl)
			}
		}
	}
	return &f, nil
}

// Status is the Unity CLI the fake mode shows.
func (f *Fake) Status() unitycli.Status {
	if f.CLI.Version == "" || f.CLI.Version == "missing" {
		return unitycli.Status{State: unitycli.Missing}
	}
	v, ok := unitycli.ParseVersion(f.CLI.Version)
	if !ok {
		return unitycli.Status{State: unitycli.Outdated, Path: "unity"}
	}
	st := unitycli.Ready
	if v.Compare(unitycli.MinVersion) < 0 {
		st = unitycli.Outdated
	}
	return unitycli.Status{State: st, Path: "unity", Version: v}
}

// InstallRoot is the Editors folder the fake mode shows.
func (f *Fake) InstallRoot(goos string) string {
	if f.EditorsFolder != "" {
		return filepath.Clean(config.ExpandHome(f.EditorsFolder))
	}
	return unity.DefaultInstallRoot(goos, homeDir(), os.Getenv)
}

// agoDuration reads how long ago something was: Go's 2h30m, or a number of
// days (3d) or weeks (2w). Empty: never.
func agoDuration(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, nil
	}
	for suffix, unit := range map[string]time.Duration{"d": 24 * time.Hour, "w": 7 * 24 * time.Hour} {
		if n, ok := strings.CutSuffix(s, suffix); ok {
			v, err := strconv.ParseFloat(n, 64)
			if err != nil {
				return 0, fmt.Errorf("bad duration %q", s)
			}
			return time.Duration(v * float64(unit)), nil
		}
	}
	return time.ParseDuration(s)
}

// fakePlatform finds a platform by its id, its label or its BuildTarget.
func fakePlatform(s string) (unity.Platform, bool) {
	for _, p := range unity.Platforms {
		if strings.EqualFold(s, p.ID) || strings.EqualFold(s, p.Label) {
			return p, true
		}
	}
	return unity.PlatformByBuildTarget(s)
}

func fakeBuildTarget(s string) string {
	if p, ok := fakePlatform(s); ok {
		return p.BuildTarget
	}
	return s
}

// useFake fills the model from f instead of the CLI and the disk.
func (m *Model) useFake(f *Fake) {
	m.fake = f
	now := m.now()
	root := f.InstallRoot(m.goos)
	d := &demoCLI{modules: map[string][]unitycli.Module{}, auth: unitycli.AuthInfo{LoggedIn: f.Account.SignedIn}, installPath: root,
		update: unitycli.UpdateInfo{Current: f.CLI.Version, Latest: f.CLI.Latest}}
	d.auth.User.Email, d.auth.User.Name = f.Account.Email, f.Account.Name
	if d.update.Latest == "" {
		d.update.Latest = f.CLI.Version
	}

	missing := map[string]bool{}
	for _, p := range f.Projects {
		path := filepath.Clean(config.ExpandHome(p.Path))
		ago, _ := agoDuration(p.Opened)
		var opened int64
		if p.Opened != "" {
			opened = now.Add(-ago).UnixMilli()
		}
		d.projects = append(d.projects, unitycli.Project{
			Title: p.Title, Path: path, ContainingFolderPath: filepath.Dir(path),
			Version: p.Version, LastModified: opened, IsFavorite: p.Pinned,
			BuildTarget: fakeBuildTarget(p.Platform), RenderPipeline: p.Pipeline,
		})
		if p.Missing {
			missing[path] = true
		}
		if p.Pending.Version != "" || p.Pending.Platform != "" {
			m.st.SetPending(path, state.Pending{Version: p.Pending.Version, BuildTarget: fakeBuildTarget(p.Pending.Platform)})
		}
	}

	var found []foundEditor
	for _, e := range f.Editors {
		arch := e.Arch
		if arch == "" {
			arch = "x86_64"
		}
		dir := filepath.Join(root, e.Version)
		loc := filepath.Join(dir, unity.ExecutableRel(m.goos))
		fe := foundEditor{Installed: unity.Installed{Version: e.Version, Dir: dir, Location: loc, Root: root,
			Architecture: arch, LTS: e.LTS}}
		for _, pl := range e.Platforms {
			p, _ := fakePlatform(pl)
			fe.platforms = append(fe.platforms, p)
		}
		found = append(found, fe)
		d.editors = append(d.editors, unitycli.Editor{Version: e.Version, Architecture: arch, Location: loc, Default: e.Default})
		d.modules[e.Version] = fakeModules(fe.platforms)
	}
	for _, r := range f.Releases {
		arch := r.Arch
		if arch == "" {
			arch = "x86_64"
		}
		stream := strings.ToUpper(r.Stream)
		d.releases = append(d.releases, unitycli.Release{Version: r.Version, Architecture: arch, Stream: stream,
			LTS: stream == "LTS"})
	}

	if len(f.Releases) == 0 && m.cli != nil {
		d.real = m.cli // a real Unity CLI, asked for the releases only
	}
	m.cli, m.cliStatus = d, f.Status()
	switch f.CLI.Channel {
	case unitycli.ChannelStable, unitycli.ChannelBeta:
		m.st.CLIChannel = f.CLI.Channel
	}
	m.host = demoHost{host: m.host, openupm: f.OpenUPM}
	if src := m.cfg.OpenUPM; !src.FromRegistry() && f.OpenUPM.Version != "" {
		// what an install from the configured repository would have recorded
		m.st.OpenUPMBuild = &state.OpenUPMBuild{Repo: src.Repo, Ref: src.Ref,
			Commit: f.OpenUPM.commit(), Version: f.OpenUPM.Version}
	}
	if f.ProjectsFolder != "" {
		m.st.DefaultProjectDir = filepath.Clean(config.ExpandHome(f.ProjectsFolder))
	}
	m.exists = func(path string) bool { return !missing[path] }
	m.scan = func(string, string) []foundEditor { return found }
	m.inv.setInstallRoot(root)
	m.inv.setFound(found)
	m.rawProjects, _ = d.Projects(context.Background())
	m.missing = missingPaths(m.rawProjects, m.exists)

	for _, t := range f.Tasks {
		frame := unitycli.Frame{Type: "progress", Pct: t.Pct, Phase: t.Phase, Message: t.Message}
		m.startCmd = tea.Batch(m.startCmd, m.enqueue(&task{title: t.Title, works: []string{"fake:" + t.Title},
			open: func(ctx context.Context) (*unitycli.Stream, error) { return unitycli.HeldStream(ctx, frame), nil }}))
	}
}

// refuseFake tells the user that the fake mode does not do what, and reports
// true, in the fake mode: it is for what would change the real machine.
func (m *Model) refuseFake(what string) bool {
	if m.fake == nil {
		return false
	}
	m.toastf(toastInfo, "Not in the fake mode: %s", what)
	return true
}

// fakeModules are the platform modules of an Editor with these platforms.
func fakeModules(have []unity.Platform) []unitycli.Module {
	installed := map[string]bool{}
	for _, p := range have {
		installed[p.ID] = true
	}
	var out []unitycli.Module
	for _, p := range unity.Platforms {
		out = append(out, unitycli.Module{ID: p.Modules[0], Name: p.Label + " Build Support", Category: "Platforms",
			DownloadBytes: 350 << 20, InstalledBytes: 1200 << 20, IsInstalled: installed[p.ID]})
	}
	return out
}

// demoHost is the machine of the fake mode: the OpenUPM CLI is what the file
// says, and nothing it starts reaches the disk.
type demoHost struct {
	host
	openupm FakeOpenUPM
}

func (o FakeOpenUPM) commit() string {
	if o.Commit != "" {
		return o.Commit
	}
	return fakeCommit
}

func (h demoHost) FindOpenUPM(context.Context) (string, string) {
	if h.openupm.Version == "" {
		return "", ""
	}
	return "openupm", h.openupm.Version
}

// LatestOpenUPM is the newest npm release, or the newest commit of the
// configured repository; without one in the file, the installed one.
func (h demoHost) LatestOpenUPM(_ context.Context, src config.OpenUPM) (string, error) {
	o := h.openupm
	switch {
	case !src.FromRegistry() && o.LatestCommit != "":
		return o.LatestCommit, nil
	case !src.FromRegistry():
		return o.commit(), nil
	case o.Latest != "":
		return o.Latest, nil
	}
	return o.Version, nil
}

func (h demoHost) StartEditor(string, string, string) error { return nil }

func (h demoHost) AddPackages(context.Context, string, string, []string) error { return nil }

// demoCLI is the Unity CLI of the fake mode. It answers from the file, and
// what changes the list of projects or Editors changes only its own copy.
type demoCLI struct {
	mu          sync.Mutex
	projects    []unitycli.Project
	editors     []unitycli.Editor
	modules     map[string][]unitycli.Module // by Editor version
	releases    []unitycli.Release
	auth        unitycli.AuthInfo
	update      unitycli.UpdateInfo
	installPath string
	// real is the machine's own Unity CLI, when the file lists no releases: it
	// lists them instead, and is asked nothing else.
	real unityCLI
}

var _ unityCLI = (*demoCLI)(nil)

func (d *demoCLI) Projects(context.Context) ([]unitycli.Project, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]unitycli.Project(nil), d.projects...), nil
}

func (d *demoCLI) ProjectInfo(_ context.Context, path string) (unitycli.ProjectInfo, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, p := range d.projects {
		if p.Path == path {
			return unitycli.ProjectInfo{Project: p}, nil
		}
	}
	return unitycli.ProjectInfo{}, nil
}

func (d *demoCLI) AddProjects(context.Context, ...string) error { return nil }

func (d *demoCLI) RemoveProjects(_ context.Context, paths ...string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	gone := map[string]bool{}
	for _, p := range paths {
		gone[p] = true
	}
	kept := d.projects[:0]
	for _, p := range d.projects {
		if !gone[p.Path] {
			kept = append(kept, p)
		}
	}
	d.projects = kept
	return nil
}

func (d *demoCLI) SetPinned(_ context.Context, path string, pinned bool) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	for i := range d.projects {
		if d.projects[i].Path == path {
			d.projects[i].IsFavorite = pinned
		}
	}
	return nil
}

func (d *demoCLI) CleanProject(context.Context, string) error { return nil }

func (d *demoCLI) Open(context.Context, unitycli.OpenOptions) (unitycli.OpenResult, error) {
	return unitycli.OpenResult{}, nil
}

func (d *demoCLI) UpgradeProject(context.Context, string, string, unitycli.CloudChoice) error {
	return nil
}

func (d *demoCLI) CreateProject(context.Context, unitycli.CreateOptions) error { return nil }

func (d *demoCLI) Templates(context.Context, string) ([]unitycli.Template, error) {
	return []unitycli.Template{
		{Name: "com.unity.template.universal-3d", DisplayName: "Universal 3D", Type: "template"},
		{Name: "com.unity.template.universal-2d", DisplayName: "Universal 2D", Type: "template"},
		{Name: "com.unity.template.hdrp-blank", DisplayName: "High Definition 3D", Type: "template"},
	}, nil
}

func (d *demoCLI) Editors(context.Context) ([]unitycli.Editor, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]unitycli.Editor(nil), d.editors...), nil
}

func (d *demoCLI) AddEditors(context.Context, ...string) error { return nil }

func (d *demoCLI) SetDefaultEditor(_ context.Context, version string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	for i := range d.editors {
		d.editors[i].Default = d.editors[i].Version == version
	}
	return nil
}

func (d *demoCLI) Releases(ctx context.Context, stream string, offset, limit int) (unitycli.ReleasePage, error) {
	if d.real != nil {
		page, err := d.real.Releases(ctx, stream, offset, limit)
		installed := d.installed()
		for i := range page.Items { // installed on the made-up machine, not the real one
			page.Items[i].Installed = installed[page.Items[i].Version]
		}
		return page, err
	}
	installed := d.installed()
	d.mu.Lock()
	defer d.mu.Unlock()
	var all []unitycli.Release
	for _, r := range d.releases {
		s := strings.ToLower(r.Stream)
		switch stream = strings.ToLower(stream); {
		case stream == "",
			stream == s,
			stream == unitycli.StreamSupported && (s == "supported" || s == "lts"):
		default:
			continue
		}
		r.Installed = installed[r.Version]
		all = append(all, r)
	}
	if offset >= len(all) {
		return unitycli.ReleasePage{}, nil
	}
	end, next := offset+limit, 0
	if end < len(all) {
		next = end
	} else {
		end = len(all)
	}
	return unitycli.ReleasePage{Items: all[offset:end], NextOffset: next}, nil
}

// installed are the versions of the made-up Editors.
func (d *demoCLI) installed() map[string]bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	installed := map[string]bool{}
	for _, e := range d.editors {
		installed[e.Version] = true
	}
	return installed
}

func (d *demoCLI) InstallPath(context.Context) (string, error) { return d.installPath, nil }

func (d *demoCLI) SetInstallPath(_ context.Context, path string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.installPath = path
	return nil
}

func (d *demoCLI) Modules(_ context.Context, version, _ string) ([]unitycli.Module, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]unitycli.Module(nil), d.modules[version]...), nil
}

func (d *demoCLI) ReleaseModules(context.Context, string) ([]unitycli.Module, error) {
	return fakeModules(nil), nil
}

func (d *demoCLI) PlanInstall(_ context.Context, o unitycli.InstallOptions) (unitycli.InstallPlan, error) {
	var p unitycli.InstallPlan
	p.Editor.Version, p.Editor.Architecture, p.Editor.DownloadSize = o.Version, o.Architecture, 2800<<20
	p.TotalDownloadSize = p.Editor.DownloadSize
	return p, nil
}

// held is a stream that reports nothing and never ends, until it is cancelled.
func held(ctx context.Context) (*unitycli.Stream, error) { return unitycli.HeldStream(ctx), nil }

func (d *demoCLI) Install(ctx context.Context, _ unitycli.InstallOptions) (*unitycli.Stream, error) {
	return held(ctx)
}

func (d *demoCLI) InstallModules(ctx context.Context, _, _ string, _ []string) (*unitycli.Stream, error) {
	return held(ctx)
}

func (d *demoCLI) RemoveModules(context.Context, string, string, []string) error { return nil }
func (d *demoCLI) Uninstall(context.Context, string, string) error               { return nil }

func (d *demoCLI) UpgradeEditor(ctx context.Context, _ string) (*unitycli.Stream, error) {
	return held(ctx)
}

func (d *demoCLI) DownloadCache(context.Context) (unitycli.CacheInfo, error) {
	return unitycli.CacheInfo{Path: filepath.Join(homeDir(), ".cache", "unity"), SizeBytes: 4300 << 20, FileCount: 12}, nil
}

func (d *demoCLI) CleanDownloadCache(context.Context) error { return nil }

func (d *demoCLI) AuthStatus(context.Context) (unitycli.AuthInfo, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.auth, nil
}

func (d *demoCLI) Login(context.Context) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.auth.LoggedIn = true
	return nil
}

func (d *demoCLI) Logout(context.Context) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.auth.LoggedIn = false
	return nil
}

func (d *demoCLI) LicenseStatus(context.Context) (json.RawMessage, error) {
	return json.RawMessage(`{"activated":true,"license":"Unity Personal"}`), nil
}

func (d *demoCLI) ActivatePersonalLicense(context.Context) error { return nil }
func (d *demoCLI) ReturnLicense(context.Context) error           { return nil }
func (d *demoCLI) SelfUpdate(context.Context, string) error      { return nil }

func (d *demoCLI) CheckUpdate(context.Context, string) (unitycli.UpdateInfo, error) {
	return d.update, nil
}

// Log is what the real Unity CLI ran, when it lists the releases.
func (d *demoCLI) Log() []unitycli.LogEntry {
	if d.real != nil {
		return d.real.Log()
	}
	return nil
}
