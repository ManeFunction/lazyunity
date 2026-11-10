package unity

import "testing"

func TestPlatformTableIsConsistent(t *testing.T) {
	ids, targets, cli, mods := map[string]bool{}, map[string]bool{}, map[string]bool{}, map[string]string{}
	hosts := map[string]int{}
	for _, p := range Platforms {
		if p.ID == "" || p.Label == "" || p.BuildTarget == "" || p.CLITarget == "" || len(p.Modules) == 0 {
			t.Errorf("incomplete platform: %+v", p)
		}
		if ids[p.ID] || targets[p.BuildTarget] || cli[p.CLITarget] {
			t.Errorf("duplicate id, build target or CLI target: %+v", p)
		}
		ids[p.ID], targets[p.BuildTarget], cli[p.CLITarget] = true, true, true
		for _, m := range p.Modules {
			if other, dup := mods[m]; dup {
				t.Errorf("module %q belongs to both %s and %s", m, other, p.ID)
			}
			mods[m] = p.ID
		}
		if p.HostOS != "" {
			hosts[p.HostOS]++
		}
	}
	for _, os := range []string{"windows", "darwin", "linux"} {
		if hosts[os] != 1 {
			t.Errorf("%d platforms are built in on %s, want exactly 1", hosts[os], os)
		}
	}
}

func TestPlatformLookups(t *testing.T) {
	cases := map[string]string{
		"StandaloneLinux64":        "linux",
		"StandaloneWindows64":      "windows",
		"StandaloneWindows":        "windows",
		"StandaloneLinux":          "linux",
		"StandaloneLinuxUniversal": "linux",
		"StandaloneOSX":            "macos",
		"StandaloneOSXUniversal":   "macos",
		"Android":                  "android",
		"iOS":                      "ios",
		"WebGL":                    "webgl",
		"WSAPlayer":                "uwp",
	}
	for target, id := range cases {
		p, ok := PlatformByBuildTarget(target)
		if !ok || p.ID != id {
			t.Errorf("PlatformByBuildTarget(%q) = %q, %v; want %q", target, p.ID, ok, id)
		}
	}
	if _, ok := PlatformByBuildTarget("PS5"); ok {
		t.Error("a console target is not known")
	}

	for mod, id := range map[string]string{
		"android": "android", "windows-mono": "windows", "windows-server": "windows", "mac-il2cpp": "macos",
		"linux-il2cpp": "linux", "webgl": "webgl", "ios": "ios", "visionos": "visionos", "appletv": "tvos",
	} {
		if p, ok := PlatformForModule(mod); !ok || p.ID != id {
			t.Errorf("PlatformForModule(%q) = %q, %v; want %q", mod, p.ID, ok, id)
		}
	}
	// sub-modules of a platform are not platforms
	for _, mod := range []string{"android-open-jdk-17.0.18+8", "documentation", "language-ja"} {
		if _, ok := PlatformForModule(mod); ok {
			t.Errorf("%q should not be a platform", mod)
		}
	}
}

func TestPlatformLabel(t *testing.T) {
	if got := PlatformLabel("StandaloneLinux64"); got != "Linux" {
		t.Errorf("got %q", got)
	}
	if got := PlatformLabel("PS5"); got != "PS5" {
		t.Errorf("unknown targets are shown as Unity spells them, got %q", got)
	}
	if got := PlatformLabel(""); got != "" {
		t.Errorf("got %q", got)
	}
}
