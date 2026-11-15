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

// LatestRelease asks npm for the newest openupm-cli in the registry.
func LatestRelease(ctx context.Context, npm string) (string, error) {
	out, err := exec.CommandContext(ctx, npm, "view", "openupm-cli", "version").Output()
	if err != nil {
		return "", fmt.Errorf("npm view openupm-cli: %w", err)
	}
	v := strings.TrimSpace(string(out))
	if v == "" {
		return "", fmt.Errorf("npm view openupm-cli: no version")
	}
	return v, nil
}

// LatestCommit asks repo for the commit that ref, a branch or a tag, points to
// now; with no ref, the commit of its default branch. git never asks for a
// password: lazyunity owns the terminal.
func LatestCommit(ctx context.Context, git, repo, ref string) (string, error) {
	repo, ref = strings.TrimSpace(repo), strings.TrimSpace(ref)
	want := []string{"HEAD"}
	if ref != "" {
		// a branch first, as git clone --branch does, then the commit a tag points to
		want = []string{"refs/heads/" + ref, "refs/tags/" + ref + "^{}", "refs/tags/" + ref}
	}
	cmd := exec.CommandContext(ctx, git, append([]string{"ls-remote", "--", repo}, want...)...)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	if os.Getenv("GIT_SSH_COMMAND") == "" && os.Getenv("GIT_SSH") == "" {
		cmd.Env = append(cmd.Env, "GIT_SSH_COMMAND=ssh -o BatchMode=yes")
	}
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git ls-remote %s: %w", repo, err)
	}
	refs := map[string]string{}
	for _, l := range strings.Split(string(out), "\n") {
		if f := strings.Fields(l); len(f) == 2 {
			refs[f[1]] = f[0]
		}
	}
	for _, w := range want {
		if c := refs[w]; c != "" {
			return c, nil
		}
	}
	if ref == "" {
		return "", fmt.Errorf("%s has no default branch", repo)
	}
	return "", fmt.Errorf("%s has no branch or tag %s", repo, ref)
}

// InstallCommand is the process that installs the OpenUPM CLI from src, to be
// run with the terminal handed over: it says how the install went and waits
// for Enter. When it builds from a repository and stamp is not empty, it
// writes the commit it installed to the file stamp.
func InstallCommand(goos string, src config.OpenUPM, stamp string) *exec.Cmd {
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
	cmd.Env = append(os.Environ(), "LAZYUNITY_OPENUPM_REPO="+repo, "LAZYUNITY_OPENUPM_REF="+ref, "LAZYUNITY_OPENUPM_STAMP="+stamp)
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
  npm_global "$tmp"/*.tgz || return
  if [ -n "$LAZYUNITY_OPENUPM_STAMP" ]; then
    git -C "$tmp/src" rev-parse HEAD > "$LAZYUNITY_OPENUPM_STAMP"
  fi
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
      if ($env:LAZYUNITY_OPENUPM_STAMP) {
        $commit = & git -C $src rev-parse HEAD
        Set-Content -LiteralPath $env:LAZYUNITY_OPENUPM_STAMP -Value $commit -Encoding ascii
      }
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
