package tui

import (
	"testing"

	"github.com/ManeFunction/lazyunity/internal/state"
	"github.com/ManeFunction/lazyunity/internal/unitycli"
)

func TestPlanOpen(t *testing.T) {
	project := func(version string, pending state.Pending) projectRow {
		r := projectRow{Pending: pending}
		r.Path, r.Version, r.Changeset, r.BuildTarget = "/p", version, "abc123", "Android"
		return r
	}
	ed := &editorRow{Version: "6000.6.8f1", Location: "/e/Unity"}
	cases := []struct {
		name     string
		r        projectRow
		ed       *editorRow
		cli      bool
		route    openRoute
		openWith unitycli.OpenOptions
		target   string
		after    state.Pending
	}{
		{name: "the project's own version, through the CLI",
			r: project("6000.6.8f1", state.Pending{}), ed: ed, cli: true,
			route: openCLI, openWith: unitycli.OpenOptions{Project: "/p"}},
		{name: "a platform choice becomes -buildTarget",
			r: project("6000.6.8f1", state.Pending{BuildTarget: "StandaloneLinux64"}), ed: ed, cli: true,
			route: openCLI, openWith: unitycli.OpenOptions{Project: "/p", BuildTarget: "linux64"}, target: "linux64"},
		{name: "the current platform chosen again changes nothing",
			r: project("6000.6.8f1", state.Pending{BuildTarget: "Android"}), ed: ed, cli: true,
			route: openCLI, openWith: unitycli.OpenOptions{Project: "/p"}},
		{name: "another version asks first",
			r: project("2022.3.62f1", state.Pending{Version: "6000.6.8f1"}), ed: ed, cli: true,
			route: openConfirm, openWith: unitycli.OpenOptions{Project: "/p", EditorVersion: "6000.6.8f1"}},
		{name: "crossing into 6000.7 is an upgrade, which keeps the platform choice",
			r: project("6000.6.8f1", state.Pending{Version: "6000.7.0b8", BuildTarget: "iOS"}), ed: &editorRow{Version: "6000.7.0b8"}, cli: true,
			route: openUpgrade, target: "ios", after: state.Pending{BuildTarget: "iOS"}},
		{name: "staying within 6000.7 is a plain open",
			r: project("6000.7.0b8", state.Pending{Version: "6000.7.1f1"}), ed: &editorRow{Version: "6000.7.1f1"}, cli: true,
			route: openConfirm, openWith: unitycli.OpenOptions{Project: "/p", EditorVersion: "6000.7.1f1"}},
		{name: "without the CLI the Editor starts directly",
			r: project("6000.6.8f1", state.Pending{BuildTarget: "StandaloneLinux64"}), ed: ed, cli: false,
			route: openDirect, target: "linux64"},
		{name: "the CLI cannot sign in an Editor older than 2019.1",
			r: project("2018.4.36f1", state.Pending{}), ed: &editorRow{Version: "2018.4.36f1"}, cli: true,
			route: openDirect},
		{name: "a missing Editor is installed first",
			r: project("2022.3.62f1", state.Pending{}), cli: true, route: openInstall},
	}
	for _, c := range cases {
		p := planOpen(c.r, c.ed, c.cli)
		if p.route != c.route || p.cli != c.openWith || p.buildTarget != c.target || p.after != c.after {
			t.Errorf("%s: route %d, open %+v, target %q, after %+v", c.name, p.route, p.cli, p.buildTarget, p.after)
		}
		if c.route == openDirect && p.why == "" {
			t.Errorf("%s: a direct launch says why", c.name)
		}
	}

	r := project("2022.3.62f1", state.Pending{})
	if p := planOpen(r, nil, true); p.changeset != "abc123" {
		t.Errorf("the project's own version is installed with its changeset: %q", p.changeset)
	}
	if p := planOpen(project("2022.3.62f1", state.Pending{Version: "2021.3.1f1"}), nil, true); p.changeset != "" {
		t.Error("another version's changeset is not known")
	}
	r.Missing = true
	if p := planOpen(r, ed, true); p.route != openMissing {
		t.Errorf("a missing folder cannot be opened: route %d", p.route)
	}
}
