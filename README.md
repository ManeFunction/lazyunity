# lazyunity

A terminal interface for Unity projects and Editors, driven with [lazygit](https://github.com/jesseduffield/lazygit)'s keys. It replaces the Unity Hub: you do not need to install the Hub at all.

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
│                                             │╭─[3]─Tools - Editor locations──────────────────────────╮
│                                             ││Unity CLI                ready · 1.0.0-beta.16         │
│                                             ││Account                  signed in                     │
│                                             ││License                  press enter to check          │
╰──────────────────────────────────────1 of 3─╯╰────────────────────────────────────────────────1 of 8─╯
╭─Tasks────────────────────────────────────────────────────────────────────────────────────────────────╮
│ ⠋ Installing 6000.7.0b8 + Web Build Support [#######---------]  42% download · Downloading Web Buil… │
╰──────────────────────────────────────────────────────────────────────────────────────────────────────╯
Open: <enter> | Version: v | Platform: p | Pin: f | New: n | Add: a | Remove: d       ● signed in Donate
```

## Requirements

- The Unity CLI, version 1.0.0-beta.16 or newer. If it is missing, lazyunity offers to install it with Unity's official installer (Tools tab, `3`).
- Linux, macOS or Windows. A terminal with colours and, ideally, mouse support.

## Build

```sh
make build        # bin/lazyunity
make check        # vet, gofmt and the tests
```

Go 1.27 or newer. There are no releases yet.

## Layout

| Panel | What it shows |
|---|---|
| **[1] Projects** | Your projects with the last time each was opened, its Unity version and its build platform. |
| **[2] Editors** | **Installed Editors** with their platforms, and **Releases** you can install. `enter` on an Editor shows its modules. |
| **[3] Options** | **Tools**: the Unity CLI, your account, the license, the OpenUPM CLI, the default project folder, the config file, About and Donate. **Editor locations**: the folders Editors are looked for in. |
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
| `ctrl+z` | suspend to the shell |
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
**Editor locations**: `a` add a folder · `d` remove · `space` install new Editors here · `o` open.
**Tools**: `enter` runs the selected tool. **OpenUPM CLI** installs [openupm-cli](https://github.com/openupm/openupm-cli) with npm (Node.js is needed): the official release from the npm registry, unless `openupm.repo` is set in `config.yml`: then that repository (the official one or a fork) is cloned and built, and git is needed too. **Donate** opens the GitHub Sponsors page, as does the Donate link at the end of the bottom line.

## Configuration

`config.yml` lives in `~/.config/lazyunity/` (`~/Library/Application Support/lazyunity/` on macOS, `%AppData%\lazyunity\` on Windows), or wherever `--config-dir` or `$LAZYUNITY_CONFIG_DIR` points. lazyunity only reads it. What the interface remembers (pending choices, extra roots, sort order) goes in `state.yml` next to it.

```yaml
gui:
  border: rounded            # rounded, single, double, bold, hidden
  sidePanelWidth: 0.45
  mouseEvents: true
  showPanelJumps: true
  screenMode: full           # full, projects, editors
  theme:                     # a colour and modifiers: [brightblue, bold], '#5c9dff', ...
    activeBorderColor: [brightblue, bold]
    selectedLineBgColor: [blue]
keybinding:                  # lazygit's notation works: <c-s>, <space>, <enter>
  universal:
    quit: [q, <c-c>]
    refresh: <disabled>
  projects:
    open: [<enter>, o]
os:
  edit: code {{dir}}         # for `e`; default is $VISUAL / $EDITOR
  openFolder: ''             # default is xdg-open, open or explorer
unity:
  cliPath: ''                # default: found automatically
editors:
  extraRoots: [/opt/unity]
projects:
  defaultLocation: ~/Projects
# openupm:                   # commented out: the Tools tab installs the official npm release
#   repo: https://github.com/ManeFunction/openupm-cli   # set: clone, build and install from this git repo
#   ref: no-more-legacy      # a branch or tag to build; empty: the default branch
quitAfterOpen: false
```

**OpenUPM CLI fork.** Until [openupm/openupm-cli#430](https://github.com/openupm/openupm-cli/pull/430) is merged (it is not accepted yet), the fork in the `openupm` block above is the recommended build. It drops deprecated dependencies, so npm no longer floods the install with deprecation warnings. It also cuts the `npm audit` findings from 19 vulnerable production packages (one critical) to 5, and fixes `openupm login` with a username and password, which sent the email as the password and logged in to registry.npmjs.org instead of the registry you asked for. It needs Node.js 20.17+ or 22.9+. Uncomment the block to use it; leave it commented out for the official npm release.

## Safety

- Nothing is installed, removed or deleted without a question first. Deleting a project's folder asks twice and refuses anything that is not a Unity project, or the home folder.
- Quitting during a task asks first.
- lazyunity never writes the Hub's files itself. It asks the CLI.

## Notes

- Opening an Editor that is older than 2019.1, or opening without the CLI, launches it directly. That works, but the Editor starts signed out.
- A project moving to Unity 6.7 or later is upgraded with `unity projects upgrade`, since `unity open` refuses to do that.
- `-buildTarget` values are the ones Unity documents (`linux64`, `android`, `webgl` ...).

## Author

lazyunity is made by Ilia Petrov-Komotskii ([Mane Function](https://github.com/ManeFunction)). If it saves you time, you can [support it on GitHub Sponsors](https://github.com/sponsors/ManeFunction).
