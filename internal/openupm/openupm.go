// Package openupm drives the OpenUPM CLI (openupm-cli), which adds packages
// from the OpenUPM registry to a Unity project. It is a Node.js program,
// installed with npm: the official release from the npm registry, or, when
// config.yml names another repository (a fork with fixes not merged yet, say),
// built from that repository's source.
package openupm

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/ManeFunction/lazyunity/internal/config"
)

// Find looks for the OpenUPM CLI on the PATH, then in ~/.local/bin, where the
// installer puts it when the system-wide npm folder is not writable.
func Find(lookPath func(string) (string, error), home string) (string, error) {
	path, err := lookPath("openupm")
	if err == nil {
		return path, nil
	}
	if home != "" {
		if p, e := lookPath(filepath.Join(home, ".local", "bin", "openupm")); e == nil {
			return p, nil
		}
	}
	return "", err
}

// Version asks the CLI at exe for its version. It is "" when the CLI does not
// say.
func Version(ctx context.Context, exe string) string {
	out, _ := exec.CommandContext(ctx, exe, "--version").Output()
	if f := strings.Fields(string(out)); len(f) > 0 {
		return f[0]
	}
	return ""
}

// Add adds packages to the project in dir.
func Add(ctx context.Context, exe, dir string, packages []string) error {
	cmd := exec.CommandContext(ctx, exe, append([]string{"add"}, packages...)...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("openupm add %s: %w: %s", strings.Join(packages, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}

// InstallCommand is the process that installs the OpenUPM CLI from src, to be
// run with the terminal handed over: it says how the install went and waits
// for Enter.
func InstallCommand(goos string, src config.OpenUPM) *exec.Cmd {
	var cmd *exec.Cmd
	if goos == "windows" {
		cmd = exec.Command("powershell", "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", psScript)
	} else {
		cmd = exec.Command("/bin/sh", "-c", shScript)
	}
	repo, ref := "", ""
	if !src.FromRegistry() {
		repo, ref = strings.TrimSpace(src.Repo), strings.TrimSpace(src.Ref)
	}
	cmd.Env = append(os.Environ(), "LAZYUNITY_OPENUPM_REPO="+repo, "LAZYUNITY_OPENUPM_REF="+ref)
	return cmd
}

// The install scripts take the repository and ref from the environment, so that
// nothing from config.yml is ever parsed by a shell.
const shScript = `# npm_global installs globally. When npm's own folder is not writable (a
# system-wide Node.js owns /usr/local), it installs into ~/.local instead, so
# that no sudo is needed.
npm_global() {
  prefix=$(npm prefix -g 2>/dev/null)
  lib="$prefix/lib/node_modules"
  if [ -z "$prefix" ] || [ "$(id -u)" -eq 0 ] || [ -w "$lib" ] || { [ ! -e "$lib" ] && [ -w "$prefix/lib" ]; }; then
    npm install -g "$@"
    return
  fi
  echo "npm cannot write to $prefix: installing into $HOME/.local instead."
  npm install -g --prefix "$HOME/.local" "$@" || return
  case ":$PATH:" in
    *":$HOME/.local/bin:"*) ;;
    *) echo "Add $HOME/.local/bin to your PATH to run openupm from the shell." ;;
  esac
}
install() {
  if [ -z "$LAZYUNITY_OPENUPM_REPO" ]; then
    npm_global openupm-cli
    return
  fi
  tmp=$(mktemp -d) || return
  trap 'rm -rf "$tmp"' EXIT
  if [ -n "$LAZYUNITY_OPENUPM_REF" ]; then
    git clone --depth 1 --branch="$LAZYUNITY_OPENUPM_REF" -- "$LAZYUNITY_OPENUPM_REPO" "$tmp/src" || return
  else
    git clone --depth 1 -- "$LAZYUNITY_OPENUPM_REPO" "$tmp/src" || return
  fi
  (cd "$tmp/src" && npm ci && npm run build && npm pack --pack-destination "$tmp") || return
  npm_global "$tmp"/*.tgz
}
install
status=$?
echo
if [ $status -eq 0 ]; then echo "OpenUPM CLI installed."; else echo "The install failed (exit code $status)."; fi
printf '\033[34mPress enter to return to lazyunity...\033[0m '
read _
exit $status
`

const psScript = `$code = 0
try {
  $repo = $env:LAZYUNITY_OPENUPM_REPO
  $ref = $env:LAZYUNITY_OPENUPM_REF
  function Run($exe, [string[]]$params) { & $exe @params; if ($LASTEXITCODE) { throw "$exe failed (exit code $LASTEXITCODE)" } }
  if (-not $repo) {
    Run 'npm' @('install', '-g', 'openupm-cli')
  } else {
    $tmp = Join-Path ([IO.Path]::GetTempPath()) ('lazyunity-openupm-' + [guid]::NewGuid())
    New-Item -ItemType Directory $tmp | Out-Null
    try {
      $src = Join-Path $tmp 'src'
      if ($ref) { Run 'git' @('clone', '--depth', '1', "--branch=$ref", '--', $repo, $src) }
      else { Run 'git' @('clone', '--depth', '1', '--', $repo, $src) }
      Push-Location $src
      try {
        Run 'npm' @('ci')
        Run 'npm' @('run', 'build')
        Run 'npm' @('pack', '--pack-destination', $tmp)
      } finally { Pop-Location }
      $tgz = Get-ChildItem $tmp -Filter *.tgz | Select-Object -First 1
      Run 'npm' @('install', '-g', $tgz.FullName)
    } finally { Remove-Item -Recurse -Force $tmp -ErrorAction SilentlyContinue }
  }
  Write-Host ''
  Write-Host 'OpenUPM CLI installed.'
} catch {
  Write-Host ''
  Write-Host "The install failed: $_"
  $code = 1
}
Write-Host 'Press enter to return to lazyunity... ' -ForegroundColor Blue -NoNewline
Read-Host | Out-Null
exit $code
`
