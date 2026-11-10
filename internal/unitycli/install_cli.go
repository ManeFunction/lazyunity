package unitycli

import (
	"os/exec"
)

// ManifestURL is Unity's release manifest for the stable CLI channel.
const ManifestURL = "https://public-cdn.cloud.unity3d.com/hub/prod/cli/latest.json"

// installScriptUnix downloads and runs Unity's installer. The installer
// defaults to the stable channel; while that has no manifest it falls back to
// beta. $1 is the stable manifest URL.
const installScriptUnix = `stable_manifest="$1"
fetch() {
  if command -v curl >/dev/null 2>&1; then curl -fsSL "$1" -o "$2"
  elif command -v wget >/dev/null 2>&1; then wget -qO "$2" "$1"
  else echo 'Installing the Unity CLI needs curl or wget.' >&2; return 127; fi
}
if [ -z "${UNITY_CLI_CHANNEL:-}" ] && ! fetch "$stable_manifest" /dev/null 2>/dev/null; then
  export UNITY_CLI_CHANNEL=beta
fi
f=$(mktemp) || exit 1
fetch https://unity.com/install.sh "$f" && UNITY_NON_INTERACTIVE=1 bash "$f"
s=$?
rm -f "$f"
exit $s
`

// installScriptWindows does the same in PowerShell.
const installScriptWindows = `if (-not $env:UNITY_CLI_CHANNEL) { try { Invoke-WebRequest -UseBasicParsing -Method Head ` + ManifestURL + ` | Out-Null } catch { $env:UNITY_CLI_CHANNEL = 'beta' } }; irm https://unity.com/install.ps1 | iex`

// InstallCommand returns the command that installs or updates the Unity CLI
// with Unity's official installer. It is meant to be run with the terminal
// handed over (tea.ExecProcess), because the installer may ask for a sudo
// password.
func InstallCommand(goos string) *exec.Cmd {
	if goos == "windows" {
		return exec.Command("powershell.exe", "-NoProfile", "-ExecutionPolicy", "Bypass",
			"-Command", installScriptWindows)
	}
	return exec.Command("/bin/sh", "-c", installScriptUnix, "sh", ManifestURL)
}
