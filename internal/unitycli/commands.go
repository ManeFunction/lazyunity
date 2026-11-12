package unitycli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// ---- projects --------------------------------------------------------------

// Projects lists every project in the Hub's database.
func (c *Client) Projects(ctx context.Context) ([]Project, error) {
	env, err := c.Run(ctx, "projects", "--all")
	if err != nil {
		return nil, err
	}
	return decodeItems[Project](env)
}

// ProjectInfo returns the details of one project.
func (c *Client) ProjectInfo(ctx context.Context, path string) (ProjectInfo, error) {
	var info ProjectInfo
	env, err := c.Run(ctx, "projects", "info", path)
	if err != nil {
		return info, err
	}
	return info, env.decodeData(&info)
}

// AddProjects registers existing project folders.
func (c *Client) AddProjects(ctx context.Context, paths ...string) error {
	_, err := c.Run(ctx, append([]string{"projects", "add"}, paths...)...)
	return err
}

// RemoveProjects removes projects from the list. Files on disk stay.
func (c *Client) RemoveProjects(ctx context.Context, paths ...string) error {
	_, err := c.Run(ctx, append(append([]string{"projects", "remove"}, paths...), "--force")...)
	return err
}

// SetPinned pins or unpins one project.
func (c *Client) SetPinned(ctx context.Context, path string, pinned bool) error {
	verb := "unpin"
	if pinned {
		verb = "pin"
	}
	_, err := c.Run(ctx, "projects", verb, globEscape(path))
	return err
}

// globEscape makes a path match only itself where the CLI takes a glob pattern
// (`projects pin` and `unpin` match every project whose title or path fits):
// each glob character is put in a character class of its own.
func globEscape(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch r {
		case '[', '*', '?', '{', '}':
			b.WriteByte('[')
			b.WriteRune(r)
			b.WriteByte(']')
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// CleanProject deletes a project's regenerable cache folders (Library, Temp ...).
func (c *Client) CleanProject(ctx context.Context, path string) error {
	_, err := c.Run(ctx, "projects", "clean", path, "--yes")
	return err
}

// CacheInfo is what `unity cache info` reports about the download cache.
type CacheInfo struct {
	Path      string `json:"path"`
	SizeBytes int64  `json:"sizeBytes"`
	FileCount int    `json:"fileCount"`
}

// DownloadCache reports where the download cache is and how big it is.
func (c *Client) DownloadCache(ctx context.Context) (CacheInfo, error) {
	var info CacheInfo
	env, err := c.Run(ctx, "cache", "info")
	if err != nil {
		return info, err
	}
	if err := env.decodeData(&info); err != nil {
		return info, fmt.Errorf("decoding %q output: %w", env.Command, err)
	}
	return info, nil
}

// CleanDownloadCache deletes every file in the download cache, including
// what interrupted installs left behind.
func (c *Client) CleanDownloadCache(ctx context.Context) error {
	_, err := c.Run(ctx, "cache", "clean", "--yes")
	return err
}

// OpenOptions says how to open a project.
type OpenOptions struct {
	Project       string // path
	EditorVersion string // empty: the project's own version
	BuildTarget   string // value for -buildTarget, empty to leave it alone
}

// OpenResult is what `unity open` reports.
type OpenResult struct {
	AlreadyOpen bool `json:"alreadyOpen"`
}

// Open opens a project in its Editor. The Editor starts signed in because the
// CLI answers its sign-in requests; it keeps running after the CLI returns.
func (c *Client) Open(ctx context.Context, o OpenOptions) (OpenResult, error) {
	args := []string{"open", o.Project}
	if o.EditorVersion != "" {
		args = append(args, "--editor-version", o.EditorVersion)
	}
	if o.BuildTarget != "" {
		args = append(args, "--build-target", o.BuildTarget)
	}
	var res OpenResult
	env, err := c.Run(ctx, args...)
	if err != nil {
		return res, err
	}
	return res, env.decodeData(&res)
}

// CloudChoice is what to do about Unity Cloud when a project is created or
// moved to Unity 6.7 or later.
type CloudChoice int

const (
	// CloudOff keeps the project out of Unity Cloud.
	CloudOff CloudChoice = iota
	// CloudNew links the project to a new Unity Cloud project.
	CloudNew
)

func (c CloudChoice) flag() string {
	if c == CloudNew {
		return "--cloud"
	}
	return "--no-cloud"
}

// UpgradeProject moves a project to another Editor version.
func (c *Client) UpgradeProject(ctx context.Context, path, to string, cloud CloudChoice) error {
	_, err := c.Run(ctx, "projects", "upgrade", path, "--to", to, "--yes", cloud.flag())
	return err
}

// CreateOptions says how to create a project.
type CreateOptions struct {
	Name          string
	ParentDir     string
	EditorVersion string
	Template      string // package id, or "2d" / "3d"
	Cloud         CloudChoice
	Bare          bool // no extra packages and settings beyond the template
	Open          bool // open it once created
}

// CreateProject creates a project and registers it.
func (c *Client) CreateProject(ctx context.Context, o CreateOptions) error {
	args := []string{"projects", "create", o.Name, "--path", o.ParentDir, "--editor-version", o.EditorVersion}
	if o.Template != "" {
		args = append(args, "--template", o.Template)
	}
	args = append(args, o.Cloud.flag())
	if o.Bare {
		args = append(args, "--bare")
	}
	if o.Open {
		args = append(args, "--open")
	}
	_, err := c.Run(ctx, args...)
	return err
}

// Templates lists the project templates available offline for an Editor.
func (c *Client) Templates(ctx context.Context, editorVersion string) ([]Template, error) {
	env, err := c.Run(ctx, "templates", "--installed", "--brief", "--editor-version", editorVersion)
	if err != nil {
		return nil, err
	}
	return decodeItems[Template](env)
}

// ---- editors ---------------------------------------------------------------

// Editors lists the Editors the CLI knows about.
func (c *Client) Editors(ctx context.Context) ([]Editor, error) {
	env, err := c.Run(ctx, "editors", "--installed")
	if err != nil {
		return nil, err
	}
	return decodeItems[Editor](env)
}

// AddEditors registers Editors that were installed by other means.
func (c *Client) AddEditors(ctx context.Context, paths ...string) error {
	_, err := c.Run(ctx, append([]string{"editors", "add"}, paths...)...)
	return err
}

// SetDefaultEditor makes version the default Editor.
func (c *Client) SetDefaultEditor(ctx context.Context, version string) error {
	_, err := c.Run(ctx, "editors", "default", version)
	return err
}

// Releases lists Editor versions available for download, newest first. stream
// is "", "lts", "tech", "beta" or "alpha". Start with offset 0 and continue
// from ReleasePage.NextOffset.
func (c *Client) Releases(ctx context.Context, stream string, offset, limit int) (ReleasePage, error) {
	args := []string{"releases", "--limit", strconv.Itoa(limit), "--offset", strconv.Itoa(offset)}
	if stream != "" {
		args = append(args, "--stream", stream)
	}
	var out ReleasePage
	env, err := c.Run(ctx, args...)
	if err != nil {
		return out, err
	}
	var d struct {
		Items    []Release `json:"items"`
		NextPage *int      `json:"nextPage"` // really the next offset
	}
	if err := env.decodeData(&d); err != nil {
		return out, err
	}
	out.Items = d.Items
	if d.NextPage != nil {
		out.NextOffset = *d.NextPage
	}
	return out, nil
}

// InstallPath returns the folder new Editors are installed into.
func (c *Client) InstallPath(ctx context.Context) (string, error) {
	env, err := c.Run(ctx, "install-path", "--get")
	if err != nil {
		return "", err
	}
	var d struct {
		Path string `json:"path"`
	}
	return d.Path, env.decodeData(&d)
}

// SetInstallPath changes the folder new Editors are installed into.
func (c *Client) SetInstallPath(ctx context.Context, path string) error {
	_, err := c.Run(ctx, "install-path", "--set", path)
	return err
}

// ---- modules and installs --------------------------------------------------

// Modules lists the modules of an installed Editor, with what is installed.
func (c *Client) Modules(ctx context.Context, version, arch string) ([]Module, error) {
	args := []string{"modules", version}
	if arch != "" {
		args = append(args, "--architecture", arch)
	}
	env, err := c.Run(ctx, args...)
	if err != nil {
		return nil, err
	}
	return decodeItems[Module](env)
}

// ReleaseModules lists the modules a release offers, before it is installed.
//
// It reads `editors info`: `install --list-modules` (despite its help text) and
// `modules` only answer for an Editor that is already installed.
func (c *Client) ReleaseModules(ctx context.Context, version string) ([]Module, error) {
	env, err := c.Run(ctx, "editors", "info", version)
	if err != nil {
		return nil, err
	}
	type size struct {
		Value int64 `json:"value"`
	}
	var d struct {
		Modules []struct {
			ID            string `json:"id"`
			Name          string `json:"name"`
			DownloadSize  size   `json:"downloadSize"`
			InstalledSize size   `json:"installedSize"`
			Hidden        bool   `json:"hidden"`
		} `json:"modules"`
	}
	if err := env.decodeData(&d); err != nil {
		return nil, fmt.Errorf("decoding %q output: %w", env.Command, err)
	}
	out := make([]Module, 0, len(d.Modules))
	for _, x := range d.Modules {
		if x.Hidden {
			continue
		}
		out = append(out, Module{
			ID: x.ID, Name: x.Name, Category: moduleCategory(x.ID),
			DownloadBytes: x.DownloadSize.Value, InstalledBytes: x.InstalledSize.Value,
		})
	}
	return out, nil
}

// moduleCategory groups a module the way `unity modules` does. `editors info`
// leaves the category out, so it is told from the id.
func moduleCategory(id string) string {
	switch {
	case id == "documentation":
		return "Documentation"
	case strings.HasPrefix(id, "language-"):
		return "Language packs (Preview)"
	case strings.HasPrefix(id, "visualstudio") || strings.HasPrefix(id, "vscode") || id == "rider":
		return "Dev tools"
	}
	return "Platforms"
}

// InstallOptions says what to install.
type InstallOptions struct {
	Version      string
	Changeset    string // needed for versions missing from the release feed
	Architecture string
	Modules      []string
	SetDefault   bool
}

func repeated(flag string, values []string) []string {
	var out []string
	for _, v := range values {
		out = append(out, flag, v)
	}
	return out
}

// PlanInstall reports what installing would download, without installing.
func (c *Client) PlanInstall(ctx context.Context, o InstallOptions) (InstallPlan, error) {
	var plan InstallPlan
	args := append(installArgs(o), "--dry-run")
	env, err := c.Run(ctx, args...)
	if err != nil {
		return plan, err
	}
	return plan, env.decodeData(&plan)
}

func installArgs(o InstallOptions) []string {
	args := []string{"install", o.Version}
	if o.Changeset != "" {
		args = append(args, "--changeset", o.Changeset)
	}
	if o.Architecture != "" {
		args = append(args, "--architecture", o.Architecture)
	}
	args = append(args, repeated("--module", o.Modules)...)
	return args
}

// Install installs an Editor with modules. It reports progress as a stream.
func (c *Client) Install(ctx context.Context, o InstallOptions) (*Stream, error) {
	args := append(installArgs(o), "--child-modules", "--accept-eula", "--yes")
	if o.SetDefault {
		args = append(args, "--set-default")
	}
	return c.Stream(ctx, args...)
}

// InstallModules adds modules to an installed Editor.
func (c *Client) InstallModules(ctx context.Context, version, arch string, modules []string) (*Stream, error) {
	args := append([]string{"install-modules", "--editor-version", version}, repeated("--module", modules)...)
	if arch != "" {
		args = append(args, "--architecture", arch)
	}
	args = append(args, "--child-modules", "--accept-eula", "--yes")
	return c.Stream(ctx, args...)
}

// RemoveModules removes modules from an installed Editor.
func (c *Client) RemoveModules(ctx context.Context, version, arch string, modules []string) error {
	args := append([]string{"editors", "module", "remove", version}, repeated("--module", modules)...)
	if arch != "" {
		args = append(args, "--architecture", arch)
	}
	_, err := c.Run(ctx, append(args, "--yes")...)
	return err
}

// Uninstall removes an installed Editor.
func (c *Client) Uninstall(ctx context.Context, version, arch string) error {
	args := []string{"uninstall", version}
	if arch != "" {
		args = append(args, "--architecture", arch)
	}
	_, err := c.Run(ctx, append(args, "--yes")...)
	return err
}

// UpgradeEditor installs the newest patch of an Editor's release line, carrying
// its modules over.
func (c *Client) UpgradeEditor(ctx context.Context, version string) (*Stream, error) {
	return c.Stream(ctx, "editors", "upgrade", version, "--yes", "--accept-eula")
}

// ---- account and license ---------------------------------------------------

// AuthStatus reports whether the CLI is signed in. Being signed out is not an
// error.
func (c *Client) AuthStatus(ctx context.Context) (AuthInfo, error) {
	var info AuthInfo
	env, err := c.Run(ctx, "auth", "status")
	if err != nil {
		var e *Error
		if errors.As(err, &e) && e.NotSignedIn() {
			return info, nil
		}
		return info, err
	}
	return info, env.decodeData(&info)
}

// Login signs in through the browser. It returns when the user is done, and
// stops if ctx is cancelled.
func (c *Client) Login(ctx context.Context) error {
	_, err := c.run(ctx, runOpts{interactive: true}, []string{"auth", "login"})
	return err
}

// Logout signs the CLI out.
func (c *Client) Logout(ctx context.Context) error {
	_, err := c.Run(ctx, "auth", "logout", "--yes")
	return err
}

// LicenseStatus returns the license state as the CLI reports it. The first call
// downloads Unity's licensing client, which is why it is only made on request.
func (c *Client) LicenseStatus(ctx context.Context) (json.RawMessage, error) {
	env, err := c.Run(ctx, "license", "status")
	if env != nil {
		return env.Data, err
	}
	return nil, err
}

// ActivatePersonalLicense activates the free Personal license. The caller must
// have shown the license terms to the user.
func (c *Client) ActivatePersonalLicense(ctx context.Context) error {
	_, err := c.Run(ctx, "license", "activate", "--personal", "--accept-eula")
	return err
}

// ReturnLicense returns the active license.
func (c *Client) ReturnLicense(ctx context.Context) error {
	_, err := c.Run(ctx, "license", "return", "--yes")
	return err
}

// SelfUpdate updates the CLI itself. channel may be empty.
func (c *Client) SelfUpdate(ctx context.Context, channel string) error {
	args := []string{"self-update", "--yes"}
	if channel != "" {
		args = append(args, "--channel", channel)
	}
	_, err := c.Run(ctx, args...)
	return err
}

// UpdateInfo is what `self-update --check` reports.
type UpdateInfo struct {
	Current string `json:"currentVersion"`
	Latest  string `json:"latestVersion"`
}

// Available reports whether Latest is newer than Current.
func (u UpdateInfo) Available() bool {
	cur, ok1 := ParseVersion(u.Current)
	lat, ok2 := ParseVersion(u.Latest)
	if ok1 && ok2 {
		return lat.Compare(cur) > 0
	}
	return u.Latest != "" && u.Latest != u.Current
}

// CheckUpdate asks whether a newer CLI is out, without installing it.
func (c *Client) CheckUpdate(ctx context.Context) (UpdateInfo, error) {
	var u UpdateInfo
	env, err := c.Run(ctx, "self-update", "--check")
	if err != nil {
		return u, err
	}
	err = env.decodeData(&u)
	return u, err
}
