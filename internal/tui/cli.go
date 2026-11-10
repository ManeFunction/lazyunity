package tui

import (
	"context"
	"encoding/json"

	"github.com/ManeFunction/lazyunity/internal/unitycli"
)

// unityCLI is what the interface asks of the Unity CLI: the operations, not
// how they are spelled on a command line. *unitycli.Client is the real one;
// the tests use an in-memory one. A Model without a usable CLI has none (nil),
// and its flows go through needCLI, withCLI or cliCmd.
type unityCLI interface {
	// projects
	Projects(ctx context.Context) ([]unitycli.Project, error)
	ProjectInfo(ctx context.Context, path string) (unitycli.ProjectInfo, error)
	AddProjects(ctx context.Context, paths ...string) error
	RemoveProjects(ctx context.Context, paths ...string) error
	SetPinned(ctx context.Context, path string, pinned bool) error
	CleanProject(ctx context.Context, path string) error
	Open(ctx context.Context, o unitycli.OpenOptions) (unitycli.OpenResult, error)
	UpgradeProject(ctx context.Context, path, to string, cloud unitycli.CloudChoice) error
	CreateProject(ctx context.Context, o unitycli.CreateOptions) error
	Templates(ctx context.Context, editorVersion string) ([]unitycli.Template, error)

	// Editors, modules, releases
	Editors(ctx context.Context) ([]unitycli.Editor, error)
	AddEditors(ctx context.Context, paths ...string) error
	SetDefaultEditor(ctx context.Context, version string) error
	Releases(ctx context.Context, stream string, offset, limit int) (unitycli.ReleasePage, error)
	InstallPath(ctx context.Context) (string, error)
	SetInstallPath(ctx context.Context, path string) error
	Modules(ctx context.Context, version, arch string) ([]unitycli.Module, error)
	ReleaseModules(ctx context.Context, version string) ([]unitycli.Module, error)
	PlanInstall(ctx context.Context, o unitycli.InstallOptions) (unitycli.InstallPlan, error)
	Install(ctx context.Context, o unitycli.InstallOptions) (*unitycli.Stream, error)
	InstallModules(ctx context.Context, version, arch string, modules []string) (*unitycli.Stream, error)
	RemoveModules(ctx context.Context, version, arch string, modules []string) error
	Uninstall(ctx context.Context, version, arch string) error
	UpgradeEditor(ctx context.Context, version string) (*unitycli.Stream, error)
	DownloadCache(ctx context.Context) (unitycli.CacheInfo, error)
	CleanDownloadCache(ctx context.Context) error

	// account, license, the CLI itself
	AuthStatus(ctx context.Context) (unitycli.AuthInfo, error)
	Login(ctx context.Context) error
	Logout(ctx context.Context) error
	LicenseStatus(ctx context.Context) (json.RawMessage, error)
	ActivatePersonalLicense(ctx context.Context) error
	ReturnLicense(ctx context.Context) error
	SelfUpdate(ctx context.Context, channel string) error
	// Log is the commands run so far, oldest first, for the command log.
	Log() []unitycli.LogEntry
}

var _ unityCLI = (*unitycli.Client)(nil)
