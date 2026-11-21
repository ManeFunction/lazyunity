# lazyunity

a lazygit-style terminal interface for Unity projects and Editors, born of an urge to get rid of the slow, bloated, half-a-gig official Unity Hub.
It replaces the Unity Hub completely: you do not need to install the Hub at all.

lazyunity is a thin, fast front end for the official [Unity CLI](https://docs.unity.com/en-us/unity-cli). Every change goes through the CLI, so it shares the Hub's project list and install location, and Editors you open from it start **signed in**, without the Hub running.

```
╭─[1]─Projects────────────────────────────────╮╭─[2]─Installed Editors - Releases──────────────────────╮
│★ Space Shooter 2h ago    6000.6.8f1  Linux  ││  6000.6.8f1      x86_64 Linux                         │
│  Old Platform… 3d ago    2022.3.62f1 Android││  2022.3.62f1 LTS x86_64 Linux, Android                │
│  Ancient Thing ✗ missing 2019.4.40f1 Linux  ││                                                       │
│                                             ││                                                       │
│                                             ││                                                       │
│                                             ││                                                       │
│                                             │╰────────────────────────────────────────────────1 of 2─╯
│                                             │╭─[3]─Tools─────────────────────────────────────────────╮
│                                             ││Unity CLI                ready · 1.0.0                 │
│                                             ││Account                  signed in                     │
│                                             ││License                  press enter to check          │
│                                             ││ · · · · · · · · · · · · · · · · · · · · · · · · · · · │
╰──────────────────────────────────────1 of 3─╯╰────────────────────────────────────────────────1 of 9─╯
╭─Tasks────────────────────────────────────────────────────────────────────────────────────────────────╮
│ ⠋ Installing 6000.7.0b8 + Web Build Support [■■■■■■■·········]  42% download · Downloading Web Buil… │
╰──────────────────────────────────────────────────────────────────────────────────────────────────────╯
Open: <enter> | Version: v | Platform: p | Pin: f | New: n | Add: a | Remove: d      ● signed in · 0.1.0
```

## Requirements

- The Unity CLI, version 1.0.0 or newer. If it is missing, lazyunity offers to install it with Unity's official installer (Tools tab, `3`). [docs/UnityCLI.md](docs/UnityCLI.md) lists every CLI command, flag and output field lazyunity relies on, and the CLI version they were checked against.
- Linux, macOS or Windows. A terminal with colours and, ideally, mouse support.

## Installation

**lazyunity** is available from a variety of sources.
`brew`, `scoop` or `winget` is recommended, because they have a convenient way to manage updates automatically.

1) **brew (Recommended for Mac and Linux users)**
    - Type `brew tap manefunction/tap` in your Terminal to add my custom tap (app source) to your brew sources,
      if you haven't already.
    - Type `brew install lazyunity` to install the application itself.
    - Verify the installation with `lazyunity --version`.
    - You are perfect, you can use the app with `lazyunity` from any folder in your system.
2) **Scoop (Recommended for Windows users)**
    - If you don't have Scoop yet, install it as [scoop.sh](https://scoop.sh) shows: one line in PowerShell.
    - Type `scoop bucket add manefunction https://github.com/ManeFunction/scoop-bucket` in PowerShell to add my
      bucket (app source) to your Scoop sources, if you haven't already.
    - Type `scoop install lazyunity` to install the application itself.
    - Verify the installation with `lazyunity --version`.
    - You are perfect, you can use the app with `lazyunity` from any folder in your system.
3) **WinGet (built into Windows 10 and 11)**
    - Type `winget install ManeFunction.lazyunity` in PowerShell or cmd. A new package waits for Microsoft's review
      before it shows up in WinGet, so the newest version can be a few days behind the other options.
    - Verify the installation with `lazyunity --version` in a new terminal window.
    - You are perfect, you can use the app with `lazyunity` from any folder in your system.
    - Update it with `winget upgrade ManeFunction.lazyunity`.
4) **go install (for Go developers)**
    - Type `go install github.com/ManeFunction/lazyunity/cmd/lazyunity@latest`. It needs Go 1.27.3 or newer and
      puts the app into `$(go env GOPATH)/bin`, which should be on your `PATH`.
    - Verify the installation with `lazyunity --version`.
5) **Ready-to-use binaries (manual installation)**
    - Download the archive for your system from the [GitHub Releases](https://github.com/ManeFunction/lazyunity/releases)
      page: `darwin` is macOS, `arm64` is Apple Silicon and Windows on ARM, `amd64` is Intel and AMD.
    - Unpack it and put `lazyunity` (`lazyunity.exe` on Windows) into a folder on your `PATH`.
    - The binaries are not code-signed. On macOS a downloaded binary is blocked until you run
      `xattr -d com.apple.quarantine lazyunity` once.
    - Verify the installation with `lazyunity --version`.
6) **From source (for advanced users)**
    - Clone the repository or download the source code from GitHub.
    - Go to the folder with the code in your Terminal and run `make build` (Go 1.27.3 or newer).
    - Run `bin/lazyunity --version` to verify the build.

## Layout

| Panel | What it shows |
|---|---|
| **[1] Projects** | Your projects with the last time each was opened, its Unity version and its build platform. |
| **[2] Editors** | **Installed Editors** with their platforms, and **Releases** you can install. `enter` on an Editor shows its modules. |
| **[3] Options** | **Tools**, in groups: the Unity CLI, your account and the license; the default project folder, the editors folder and the config file; the OpenUPM CLI; About and Donate. |
| **Tasks** (bottom) | A progress frame that appears **only while something is installing**, then goes away. Tasks run one at a time; the same Editor or module cannot be queued twice. |

`+` and `_` switch the screen mode: **full** (every panel), **projects** (only Projects) and **editors** (Editors and Options). Jumping to a panel that is not on screen (`1`, `2`, `3`) switches to the mode that shows it.

Changing a project's Unity version or platform is remembered and applied the next time you open it. A pending change is shown in yellow, as `6000.6.8f1→2022.3.62f1` or `Linux→Android`.

## Keys

Everything follows lazygit where lazygit has an equivalent. `?` shows the keys that apply right now, and filters as you type. Keys can be changed in `config.yml`.

**Everywhere**

| Key | |
|---|---|
| `?` | keybindings menu |
| `q`, `ctrl+c` | quit |
| `tab`, `l`, `→` / `shift+tab`, `h`, `←` | next / previous panel |
| `1` `2` `3` | jump to a panel (again: next tab) |
| `[` `]` | previous / next tab |
| `j` `k`, `↓` `↑` | next / previous item |
| `,` `.` (or `pgup` `pgdn`) | previous / next page |
| `<` `>` (or `home` `end`) | top / bottom |
| `/` | filter the list; `esc` clears it |
| `+` `_` | next / previous screen mode (full, projects, editors) |
| `R` | refresh |
| `@` | command log |
| `:` | run a shell command |
| `ctrl+o` | copy the selected path |
| `X` | cancel the running task |
| `ctrl+z` | suspend to the shell (not on Windows) |
| `esc` | clear a filter, or leave an Editor's modules |

**Projects**

| Key | |
|---|---|
| `enter`, `space` | open in Unity, signed in |
| `v` | change the Unity version (a list of installed Editors) |
| `p` | change the build platform (the platforms installed for that Editor) |
| `f` | pin or unpin |
| `n` | new project (name, folder, version, template, options) |
| `a` | add an existing project |
| `ctrl+a` | add an OpenUPM package to the project (needs the OpenUPM CLI) |
| `d` | remove from the list, or delete the folder (asks twice) |
| `s` | sort: last opened, name, Unity version |
| `o` / `e` | open the folder / edit in a code editor |
| `i` | project details |
| `y` | copy the path, name or version |
| `C` | clean caches (Library, Temp ...) |

**Editors, Installed Editors**: `enter` modules · `a` add modules · `n` install a new Editor · `d` uninstall · `space` set default · `u` upgrade to the newest patch · `o` folder · `y` copy.
**Modules of an Editor**: `space`, `a` or `enter` install · `d` remove.
**Releases**: `enter` or `space` install · `/` filter by version, in the filter row at the bottom of the tab (every page of releases is searched, not only the ones loaded) · `ctrl+s` release stream: Supported (the default), LTS, Beta, Alpha or All · `o` release notes.
**Anywhere in the Editors panel**: `C` cleans the download cache, where installers and the partial downloads of interrupted installs are kept (asks first, with its size; not while a task runs).

**Tools**: `enter` runs the selected tool, and `e` edits `config.yml` from any row. On the **Unity CLI** row, `enter` installs the CLI, or, once it is ready, opens a menu to update it, switch it between the release and beta feeds (the row says `beta feed` while you follow betas), or reinstall it with the official installer. The **Editors folder** row is where new Editors install and installed Editors are looked for; `enter` on it changes it through the CLI. When the editor quits with the config changed, lazyunity restarts to apply it; if a task is running, the Config file row says a restart is required instead, and `enter` on it restarts (asking first while a task would be cancelled). **OpenUPM CLI** installs [openupm-cli](https://github.com/openupm/openupm-cli) with npm (Node.js is needed): the official release from the npm registry, unless `openupm.repo` is set in `config.yml`: then that repository (the official one or a fork) is cloned and built, and git is needed too. Like the Unity CLI row, it says `ready · <version>` for the npm release and `update available: <version>` once a newer one is out. A build from a repository reads `<version> · from <repo>@<branch> #<commit>`, and the row offers an update whenever the branch has newer commits, even when the version did not change. **Donate** opens the GitHub Sponsors page. The bottom line ends with the lazyunity version.

## Configuration

`config.yml` lives in `~/.config/lazyunity/` (`~/Library/Application Support/lazyunity/` on macOS, `%AppData%\lazyunity\` on Windows), or wherever `--config-dir` or `$LAZYUNITY_CONFIG_DIR` points. As with lazygit, a set `$XDG_CONFIG_HOME` puts it in `$XDG_CONFIG_HOME/lazyunity/` on every system. lazyunity only reads it. What the interface remembers (pending choices, sort order) goes in `state.yml` next to it. The first time you edit it from the Tools tab, lazyunity writes every setting into it, commented out and set to its default, keybindings included.

```yaml
gui:
  border: rounded            # rounded, single, double, bold, hidden
  sidePanelWidth: 0.45
  mouseEvents: true
  showPanelJumps: true
  showListFooter: true       # "1 of 20" on a list's bottom border
  screenMode: full           # full, projects, editors
  portraitMode: auto         # one column of panels: auto, always, never
  portraitModeAutoMaxWidth: 84   # auto: when the window is at most this wide
  portraitModeAutoMinHeight: 46  # and at least this tall
  theme:                     # a colour and modifiers: [brightblue, bold], '#5c9dff', ...
    activeBorderColor: [brightblue, bold]
    selectedLineBgColor: [blue]
keybinding:                  # lazygit's notation works: <c-s>, <space>, <enter>
  universal:
    quit: [q, <c-c>]
    refresh: <disabled>
  projects:
    open: [<enter>, O]
os:
  edit: code {{dir}}         # for `e`; default is $VISUAL / $EDITOR
  openFolder: ''             # default is xdg-open, open or explorer
  shell: ''                  # for `:`; default is $SHELL, or cmd on Windows (pwsh works too)
unity:
  cliPath: ''                # default: found automatically
projects:
  defaultLocation: ~/Projects
# openupm:                   # commented out: the Tools tab installs the official npm release
#   repo: https://github.com/ManeFunction/openupm-cli   # set: clone, build and install from this git repo
#   ref: no-more-legacy      # a branch or tag to build; empty: the default branch
quitAfterOpen: false
```

**Command lines** (`os.edit`, `os.openFolder`, `$VISUAL` / `$EDITOR`) are cut into words as a shell would, with quotes grouping words, but nothing in them is expanded. On Windows a backslash is part of the path, so `C:\Tools\nvim.exe {{dir}}` works as written; on Unix it escapes the next character.

**OpenUPM CLI fork.** Until [openupm/openupm-cli#430](https://github.com/openupm/openupm-cli/pull/430) is merged (it is not accepted yet), the fork in the `openupm` block above is the recommended build. It drops deprecated dependencies, so npm no longer floods the install with deprecation warnings. It also cuts the `npm audit` findings from 19 vulnerable production packages (one critical) to 5, and fixes `openupm login` with a username and password, which sent the email as the password and logged in to registry.npmjs.org instead of the registry you asked for. It needs Node.js 20.17+ or 22.9+. Uncomment the block to use it; leave it commented out for the official npm release.

## Safety

- Nothing is installed, removed or deleted without a question first. Deleting a project's folder asks twice and refuses anything that is not a Unity project, or the home folder.
- Quitting during a task asks first.
- lazyunity never writes the Hub's files itself. It asks the CLI.

## Notes

- Opening an Editor that is older than 2019.1, or opening without the CLI, launches it directly. That works, but the Editor starts signed out.
- A project moving to Unity 6.7 or later is upgraded with `unity projects upgrade`, since `unity open` refuses to do that.

## Author

lazyunity is made by Ilia Petrov-Komotskii ([Mane Function](https://github.com/ManeFunction)). If it saves you time, you can [support it on GitHub Sponsors](https://github.com/sponsors/ManeFunction).

## Repository info

This repo follows the [Conventional Commits](https://www.conventionalcommits.org/) specification.

[![GitHub release (latest by date)](https://img.shields.io/github/v/release/ManeFunction/lazyunity)](https://github.com/ManeFunction/lazyunity/releases/latest)
[![GitHub All Releases](https://img.shields.io/github/downloads/ManeFunction/lazyunity/total)](https://github.com/ManeFunction/lazyunity/releases)
[![GitHub Sponsors](https://img.shields.io/github/sponsors/ManeFunction?label=Sponsor&logo=GitHubSponsors&style=flat)](https://github.com/sponsors/ManeFunction)
