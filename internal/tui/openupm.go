package tui

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/ManeFunction/lazyunity/internal/config"
)

// The OpenUPM CLI (openupm-cli) adds packages from the OpenUPM registry to a
// project. It is a Node.js program, installed with npm: the official release
// from the npm registry, or, when config.yml names another repository (a fork
// with fixes not merged yet, say), built from that repository's source.

// openupmState is what is known about the installed OpenUPM CLI.
type openupmState struct {
	checked bool
	path    string // "" when not installed
	version string
}

type openupmMsg openupmState

type openupmInstalledMsg struct{ err error }

// lookPath finds programs; tests replace it.
var lookPath = exec.LookPath

// probeOpenUPMCmd looks for the OpenUPM CLI and asks it for its version.
func probeOpenUPMCmd() tea.Cmd {
	return func() tea.Msg {
		path, err := findOpenUPM()
		if err != nil {
			return openupmMsg{checked: true}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		out, _ := exec.CommandContext(ctx, path, "--version").Output()
		return openupmMsg{checked: true, path: path, version: firstWord(string(out))}
	}
}

// findOpenUPM looks for the OpenUPM CLI on the PATH, then in ~/.local/bin, where
// the installer puts it when the system-wide npm folder is not writable.
func findOpenUPM() (string, error) {
	path, err := lookPath("openupm")
	if err == nil {
		return path, nil
	}
	if home := homeDir(); home != "" {
		if p, e := lookPath(filepath.Join(home, ".local", "bin", "openupm")); e == nil {
			return p, nil
		}
	}
	return "", err
}

func firstWord(s string) string {
	if f := strings.Fields(s); len(f) > 0 {
		return f[0]
	}
	return ""
}

// openupmStatus is the OpenUPM row of the Tools tab.
func (m *Model) openupmRow() toolRow {
	r := toolRow{id: "openupm", label: "OpenUPM CLI", run: (*Model).openupmTool}
	o := m.openupm
	switch {
	case !o.checked:
		r.status = "checking…"
	case o.path == "":
		r.status, r.tone = "not installed · enter to install", toneWarn
	default:
		v := o.version
		if v == "" {
			v = "installed"
		}
		r.status, r.tone = v+" · enter to reinstall", toneGood
	}
	if src := m.cfg.OpenUPM; !src.FromRegistry() {
		r.status += " · from " + repoName(src.Repo, src.Ref)
	}
	return r
}

// repoName is a repository URL without its scheme, with the ref if there is one.
func repoName(repo, ref string) string {
	s := strings.TrimSuffix(strings.TrimRight(repo, "/"), ".git")
	s = strings.TrimPrefix(strings.TrimPrefix(s, "https://"), "http://")
	s = strings.TrimPrefix(s, "github.com/")
	if ref != "" {
		s += "@" + ref
	}
	return s
}

// openupmTool installs the OpenUPM CLI, after saying how.
func (m *Model) openupmTool() tea.Cmd {
	if m.refuseWhileBusy("installing the OpenUPM CLI") {
		return nil
	}
	src := m.cfg.OpenUPM
	needs := []string{"npm"}
	if !src.FromRegistry() {
		needs = append(needs, "git")
	}
	var missing []string
	for _, p := range needs {
		if _, err := lookPath(p); err != nil {
			missing = append(missing, p)
		}
	}
	if len(missing) > 0 {
		m.push(&infoPopup{title: "OpenUPM CLI", lines: wrapText(
			"Installing the OpenUPM CLI needs "+strings.Join(missing, " and ")+", which "+
				map[bool]string{true: "is", false: "are"}[len(missing) == 1]+" not on the PATH.\n\n"+
				"Install Node.js (it comes with npm) from https://nodejs.org or your package manager"+
				map[bool]string{true: ", and git,", false: ""}[len(needs) > 1]+" then try again.", 70)})
		return nil
	}
	verb := "Install"
	if m.openupm.path != "" {
		verb = "Reinstall"
	}
	var msg string
	if src.FromRegistry() {
		msg = verb + " openupm-cli, the official release from the npm registry (npm install -g openupm-cli)?"
	} else {
		msg = verb + " openupm-cli from " + src.Repo
		if src.Ref != "" {
			msg += " (" + src.Ref + ")"
		}
		msg += "? It is cloned with git, built with npm, and installed with npm install -g."
	}
	msg += "\n\n"
	if src.FromRegistry() {
		msg += "To build it from a git repository instead (a fork, say), set openupm.repo in config.yml."
	} else {
		msg += "The repository is set by openupm.repo in config.yml; without it, the npm release is installed."
	}
	m.push(&confirmPopup{
		title: verb + " the OpenUPM CLI", yes: verb, message: msg,
		onYes: func(m *Model) tea.Cmd {
			return tea.ExecProcess(openupmInstallCommand(m.goos, src), func(err error) tea.Msg { return openupmInstalledMsg{err: err} })
		},
	})
	return nil
}

func (m *Model) onOpenUPMInstalled(msg openupmInstalledMsg) tea.Cmd {
	if msg.err != nil {
		m.toastf(toastError, "The OpenUPM CLI was not installed: %v", msg.err)
	} else {
		m.toastf(toastOK, "Installed the OpenUPM CLI")
	}
	m.openupm.checked = false
	m.refreshTools()
	return probeOpenUPMCmd()
}

// The install scripts take the repository and ref from the environment, so that
// nothing from config.yml is ever parsed by a shell.
const openupmShScript = `# npm_global installs globally. When npm's own folder is not writable (a
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
printf 'Press enter to return to lazyunity... '
read _
exit $status
`

const openupmPSScript = `$code = 0
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
Read-Host 'Press enter to return to lazyunity' | Out-Null
exit $code
`

// openupmInstallCommand is the process that installs the OpenUPM CLI.
func openupmInstallCommand(goos string, src config.OpenUPM) *exec.Cmd {
	var cmd *exec.Cmd
	if goos == "windows" {
		cmd = exec.Command("powershell", "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", openupmPSScript)
	} else {
		cmd = exec.Command("/bin/sh", "-c", openupmShScript)
	}
	repo, ref := "", ""
	if !src.FromRegistry() {
		repo, ref = strings.TrimSpace(src.Repo), strings.TrimSpace(src.Ref)
	}
	cmd.Env = append(os.Environ(), "LAZYUNITY_OPENUPM_REPO="+repo, "LAZYUNITY_OPENUPM_REF="+ref)
	return cmd
}
