package unity

// Platform is a build target a project can be switched to.
type Platform struct {
	// ID is a stable key; it is what lazyunity stores.
	ID string
	// Label is shown to the user.
	Label string
	// BuildTarget is Unity's BuildTarget enum name. The project database uses
	// it, e.g. StandaloneLinux64.
	BuildTarget string
	// CLITarget is the value for the Editor's -buildTarget argument.
	CLITarget string
	// Modules are the module ids that provide the platform. Any one is enough
	// for the platform to be usable.
	Modules []string
	// PlaybackEngine is the folder under PlaybackEngines that the platform's
	// support lives in.
	PlaybackEngine string
	// HostOS is the operating system whose Editor ships the platform built in
	// (and so needs no module); empty for platforms that always need one.
	HostOS string
}

// Platforms is every platform lazyunity can switch a project to, in the order
// it lists them.
var Platforms = []Platform{
	{ID: "windows", Label: "Windows", BuildTarget: "StandaloneWindows64", CLITarget: "win64",
		Modules: []string{"windows-mono", "windows-il2cpp", "windows-server"}, PlaybackEngine: "WindowsStandaloneSupport", HostOS: "windows"},
	{ID: "macos", Label: "macOS", BuildTarget: "StandaloneOSX", CLITarget: "osxuniversal",
		Modules: []string{"mac-mono", "mac-il2cpp", "mac-server"}, PlaybackEngine: "MacStandaloneSupport", HostOS: "darwin"},
	{ID: "linux", Label: "Linux", BuildTarget: "StandaloneLinux64", CLITarget: "linux64",
		Modules: []string{"linux-mono", "linux-il2cpp", "linux-server"}, PlaybackEngine: "LinuxStandaloneSupport", HostOS: "linux"},
	{ID: "android", Label: "Android", BuildTarget: "Android", CLITarget: "android",
		Modules: []string{"android"}, PlaybackEngine: "AndroidPlayer"},
	{ID: "ios", Label: "iOS", BuildTarget: "iOS", CLITarget: "ios",
		Modules: []string{"ios"}, PlaybackEngine: "iOSSupport"},
	{ID: "webgl", Label: "WebGL", BuildTarget: "WebGL", CLITarget: "webgl",
		Modules: []string{"webgl"}, PlaybackEngine: "WebGLSupport"},
	{ID: "tvos", Label: "tvOS", BuildTarget: "tvOS", CLITarget: "tvos",
		Modules: []string{"appletv"}, PlaybackEngine: "AppleTVSupport"},
	{ID: "visionos", Label: "visionOS", BuildTarget: "VisionOS", CLITarget: "visionos",
		Modules: []string{"visionos"}, PlaybackEngine: "VisionOSPlayer"},
	{ID: "uwp", Label: "Universal Windows Platform", BuildTarget: "WSAPlayer", CLITarget: "windowsstoreapps",
		Modules: []string{"universal-windows-platform"}, PlaybackEngine: "MetroSupport"},
}

// PlatformByBuildTarget finds a platform from its BuildTarget enum name. The
// 32-bit and older standalone names map to their 64-bit platform.
func PlatformByBuildTarget(target string) (Platform, bool) {
	switch target {
	case "StandaloneWindows":
		target = "StandaloneWindows64"
	case "StandaloneLinux", "StandaloneLinuxUniversal":
		target = "StandaloneLinux64"
	case "StandaloneOSXUniversal", "StandaloneOSXIntel", "StandaloneOSXIntel64":
		target = "StandaloneOSX"
	}
	for _, p := range Platforms {
		if p.BuildTarget == target {
			return p, true
		}
	}
	return Platform{}, false
}

// PlatformByID finds a platform from its ID.
func PlatformByID(id string) (Platform, bool) {
	for _, p := range Platforms {
		if p.ID == id {
			return p, true
		}
	}
	return Platform{}, false
}

// PlatformLabel names a BuildTarget for display. Targets lazyunity does not
// know (consoles, new platforms) are shown as Unity spells them.
func PlatformLabel(buildTarget string) string {
	if buildTarget == "" {
		return ""
	}
	if p, ok := PlatformByBuildTarget(buildTarget); ok {
		return p.Label
	}
	return buildTarget
}

// PlatformForModule returns the platform a module id provides, if any.
func PlatformForModule(moduleID string) (Platform, bool) {
	for _, p := range Platforms {
		for _, m := range p.Modules {
			if m == moduleID {
				return p, true
			}
		}
	}
	return Platform{}, false
}
