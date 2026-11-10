package tui

import (
	"fmt"

	"github.com/ManeFunction/lazyunity/internal/state"
	"github.com/ManeFunction/lazyunity/internal/unity"
	"github.com/ManeFunction/lazyunity/internal/unitycli"
)

// openRoute is the way a project opens.
type openRoute int

const (
	openCLI     openRoute = iota // through the CLI, which signs the Editor in
	openConfirm                  // through the CLI with another version, once the user agrees
	openUpgrade                  // moved to Unity 6.7 or later by an upgrade, which `open` refuses to do
	openDirect                   // the Editor started directly, signed out
	openInstall                  // not yet: its Editor is not installed
	openMissing                  // not at all: its folder is gone
)

// openPlan is how a project opens. It is decided from the project and its
// pending change, the installed Editor of the version it opens with, and
// whether the CLI is there; the Model only turns it into popups and commands.
type openPlan struct {
	route openRoute
	// version is the Editor version the project opens with.
	version string
	// changeset is the version's changeset, when known, to install it with.
	changeset string
	// editor is the Editor that opens it; nil when it is not installed.
	editor *editorRow
	// why says, for openDirect, why the CLI is not used.
	why string
	// cli is what to ask `unity open` for, on the CLI routes.
	cli unitycli.OpenOptions
	// buildTarget is the -buildTarget value of the platform to switch to, or
	// "" to leave the project's alone.
	buildTarget string
	// after is what is still pending once the project has opened this way.
	after state.Pending
}

func planOpen(r projectRow, ed *editorRow, cliReady bool) openPlan {
	p := openPlan{version: r.effVersion(), editor: ed, buildTarget: pendingBuildTarget(r)}
	switch {
	case r.Missing:
		p.route = openMissing
		return p
	case ed == nil:
		p.route = openInstall
		if p.version == r.Version {
			p.changeset = r.Changeset
		}
		return p
	case !unity.AtLeast(p.version, 2019, 1):
		p.route, p.why = openDirect, fmt.Sprintf("The Unity CLI cannot sign in Editors older than 2019.1, and %s is one.", p.version)
		return p
	case !cliReady:
		p.route, p.why = openDirect, "The Unity CLI is not available."
		return p
	case r.versionChanged() && unity.AtLeast(p.version, 6000, 7) && !unity.AtLeast(r.Version, 6000, 7):
		// the upgrade moves the project only: a platform choice waits for the
		// next open
		p.route, p.after = openUpgrade, state.Pending{BuildTarget: r.Pending.BuildTarget}
		return p
	case r.versionChanged():
		p.route = openConfirm
	}
	p.cli = unitycli.OpenOptions{Project: r.Path, BuildTarget: p.buildTarget}
	if r.versionChanged() {
		p.cli.EditorVersion = r.Pending.Version
	}
	return p
}

// pendingBuildTarget is the -buildTarget value of the platform chosen for a
// project, or "" when none is.
func pendingBuildTarget(r projectRow) string {
	if !r.targetChanged() {
		return ""
	}
	if p, ok := unity.PlatformByBuildTarget(r.Pending.BuildTarget); ok {
		return p.CLITarget
	}
	return ""
}
