# Unity CLI surface used by lazyunity

Every Unity CLI command, flag, output field and exit code that lazyunity relies on.
If a Unity CLI update changes anything listed here, lazyunity may break. Check this page against each new CLI release.

| | Version |
|---|---|
| lazyunity | **0.1.0** |
| Unity CLI it was checked against | **1.0.0** (`unitycli.MinVersion`, `internal/unitycli/version.go`) |

All calls live in `internal/unitycli`. No other package runs `unity`.

> **Keep this page up to date.** Update it whenever a lazyunity release adds, removes or changes a CLI call or a decoded field, and whenever a new Unity CLI version changes any of them. Bump the versions in the table above when you do.

## Global flags

`globalFlags` in `client.go` puts these flags **before** the command on every call:

| Flag | Value | Why |
|---|---|---|
| `--format` | `json` (queries and actions), `ndjson` (streams) | Machine-readable output |
| `--no-banner` | | Keeps stray text out of stdout |
| `--no-pager` | | Stops the CLI from blocking on a pager |
| `--no-color` | | No ANSI codes in the output |
| `--non-interactive` | left out only for `auth login` | Stops the CLI from asking questions |

Flags are used instead of the `UNITY_*` / `NO_COLOR` environment variables on purpose: the CLI's children (Editors started by `unity open`) would inherit those variables.

## Commands

**Kind**: *query* means read-only and limited by `QueryTimeout` (1 min). *action* changes state and has no deadline. *stream* is an action whose `--format ndjson` progress is read frame by frame.

### Projects

| Command (after global flags) | Kind | Go function | lazyunity uses it to | Output fields read |
|---|---|---|---|---|
| `projects --all` | query | `Projects` | List projects (panel [1]) | `data.items[]`: `title`, `path`, `containingFolderPath`, `version`, `architecture`, `changeset`, `lastModified` (ms epoch), `isFavorite`, `isCustomEditor`, `buildTarget`, `renderPipeline`, `cloudEnabled` |
| `projects info <path>` | query | `ProjectInfo` | Show project details | Everything `projects --all` returns, plus `data.scriptingBackend` (map), `data.packages` (map) |
| `projects add <path>...` | action | `AddProjects` | Register existing folders | none |
| `projects remove <path>... --force` | action | `RemoveProjects` | Remove a project from the list (files stay) | none |
| `projects pin <glob>` / `projects unpin <glob>` | action | `SetPinned` | Pin or unpin a project. The path is glob-escaped (`[`, `*`, `?`, `{`, `}` are each wrapped in `[x]`) because the argument is a glob pattern | none |
| `projects clean <path> --yes` | action | `CleanProject` | Delete Library, Temp and the other regenerable folders | none |
| `projects upgrade <path> --to <ver> --yes --cloud\|--no-cloud` | action | `UpgradeProject` | Move a project to another Editor version | none |
| `projects create <name> --path <dir> --editor-version <ver> [--template <id>] --cloud\|--no-cloud [--bare] [--open]` | action | `CreateProject` | Create a new project | none |
| `templates --installed --brief --editor-version <ver>` | query | `Templates` | List the offline templates for the New Project form | `data.items[]`: `name`, `displayName`, `type`, `version`, `status` |
| `open <path> [--editor-version <ver>] [--build-target <target>]` | action | `Open` | Open a project, signed in | `data.alreadyOpen` |

### Editors and releases

| Command | Kind | Go function | lazyunity uses it to | Output fields read |
|---|---|---|---|---|
| `editors --installed` | query | `Editors` | List installed Editors (panel [2]) | `data.items[]`: `version`, `alias`, `architecture`, `location` (path to the executable), `moduleNames[]`, `default` |
| `editors add <path>...` | action | `AddEditors` | Register Editors installed some other way | none |
| `editors default <ver>` | action | `SetDefaultEditor` | Set the default Editor | none |
| `editors info <ver>` | query | `ReleaseModules` | List a release's modules **before** it is installed | `data.modules[]`: `id`, `name`, `downloadSize.value`, `installedSize.value`, `hidden`. There is no category field, so lazyunity works it out from `id` |
| `editors upgrade <ver> --yes --accept-eula` | stream | `UpgradeEditor` | Move to the newest patch of a release line | ndjson frames |
| `editors module remove <ver> --module <id>... [--architecture <a>] --yes` | action | `RemoveModules` | Remove modules | none |
| `releases --limit <n> --offset <n> [--stream lts\|tech\|beta\|alpha]` | query | `Releases` | List releases page by page (the Releases tab) | `data.items[]`: `version`, `architecture`, `stream` (`LTS`/`TECH`/`SUPPORTED`/`BETA`/`ALPHA`), `lts`, `installed`, `revisionHash`; `data.nextPage` |
| `install-path --get` | query | `InstallPath` | Show the Editors folder | `data.path` |
| `install-path --set <path>` | action | `SetInstallPath` | Change the Editors folder | none |

### Modules and installs

| Command | Kind | Go function | lazyunity uses it to | Output fields read |
|---|---|---|---|---|
| `modules <ver> [--architecture <a>]` | query | `Modules` | List an **installed** Editor's modules | `data.items[]`: `id`, `aliases`, `name`, `category`, `status`, `downloadBytes`, `installedBytes`, `isInstalled` |
| `install <ver> [--changeset <c>] [--architecture <a>] [--module <id>]... --dry-run` | query | `PlanInstall` | Show the download size before installing | `data.alreadyInstalled`, `data.editor.{version,architecture,downloadSize}`, `data.modules[].{id,name,downloadSize}`, `data.totalDownloadSize` |
| `install <ver> [--changeset <c>] [--architecture <a>] [--module <id>]... --child-modules --accept-eula --yes [--set-default]` | stream | `Install` | Install an Editor | ndjson frames |
| `install-modules --editor-version <ver> [--module <id>]... [--architecture <a>] --child-modules --accept-eula --yes` | stream | `InstallModules` | Add modules to an installed Editor | ndjson frames |
| `uninstall <ver> [--architecture <a>] --yes` | action | `Uninstall` | Uninstall an Editor | none |
| `cache info` | query | `DownloadCache` | Show the download cache location and size | `data.path`, `data.sizeBytes`, `data.fileCount` |
| `cache clean --yes` | action | `CleanDownloadCache` | Empty the download cache | none |

### Account, license and the CLI itself

| Command | Kind | Go function | lazyunity uses it to | Output fields read |
|---|---|---|---|---|
| `auth status` | query | `AuthStatus` | Show who is signed in. Exit code 3 means signed out, which is not an error | `data.loggedIn`, `data.sessionState`, `data.mode`, `data.user.{name,email}` |
| `auth login` | action, **interactive** (no `--non-interactive`) | `Login` | Sign in through the browser | none |
| `auth logout --yes` | action | `Logout` | Sign out | none |
| `license status` | action | `LicenseStatus` | Show the license. `data` is printed as indented JSON, unparsed. Exit code 4 means no license | `data` (raw) |
| `license activate --personal --accept-eula` | action | `ActivatePersonalLicense` | Activate the free Personal license | none |
| `license return --yes` | action | `ReturnLicense` | Return the license | none |
| `self-update --check [--channel stable\|beta]` | query | `CheckUpdate` | Check for a newer CLI | `data.currentVersion`, `data.latestVersion` |
| `self-update --yes [--channel stable\|beta]` | action | `SelfUpdate` | Update the CLI, or switch it to another channel | none |
| `--version` (no global flags, 5 s limit) | probe | `runVersion` (`locate.go`) | Find the CLI version and compare it with `MinVersion` | The last whitespace-separated field on any line that parses as `X.Y.Z` or `X.Y.Z-tag.N` |

## Output contract

### JSON envelope (`--format json`), in `envelope.go`

| Field | How it is used |
|---|---|
| `success` | The call succeeded only if this is true **and** the exit code is 0 |
| `command` | Used in decode-error messages |
| `data` | Holds the payload. List commands use `data.items[]` |
| `errors[0].code`, `errors[0].message` | Shown to the user when a call fails |
| `warnings`, `notifications[].{code,message}` | Decoded but not acted on yet |

Any text before the first `{` is skipped.

### ndjson frames (`--format ndjson`), in `stream.go`

| Field | Frame | How it is used |
|---|---|---|
| `type` | all | `"result"` marks the last frame. Any other value is treated as progress |
| `phase` | progress | `download` or `install`. The progress bar looks for `install` |
| `pct` | progress | Percentage, 0–100 |
| `msg` | progress | Status text (the key is `msg`, not `message`) |
| `uid`, `parentItemUid`, `name` | progress | Decoded |
| `success`, `command`, `data`, `errors[]` | result | Same as the envelope |

Lines that are not JSON are skipped. Reading continues for 500 ms after the process exits, because the CLI's helper processes can keep the pipe open.

### Exit codes, in `envelope.go`

| Code | Meaning | lazyunity reaction |
|---|---|---|
| 0 | OK | |
| 1 | Error | Shows the message |
| 2 | Bad arguments | Shows the message |
| 3 | Auth needed | `NotSignedIn()`: asks the user to sign in; for `auth status`, means signed out |
| 4 | Needs config | `NeedsConfig()`: "No active license" hint |
| 6 | Command failed | Shows the message |
| 7 | Network | `Network()` |
| 9 | Install locked | `InstallLocked()`: says another install is running |
| 11 | Not found | `NotFound()` |
| 130 / 143 | Interrupted / terminated | `Cancelled()`. 143 is also what a query reports when it times out |

## Installing the CLI (`install_cli.go`)

lazyunity calls no CLI command here. It runs Unity's official installer:

| Platform | What runs |
|---|---|
| Linux / macOS | `https://unity.com/install.sh` with `UNITY_NON_INTERACTIVE=1` |
| Windows | `irm https://unity.com/install.ps1 \| iex` |
| Both | Checks `https://public-cdn.cloud.unity3d.com/hub/prod/cli/latest.json` first. If it is missing, sets `UNITY_CLI_CHANNEL=beta` |

The CLI is looked for in `~/.unity/bin/unity[.exe]`, then on `PATH` (`locate.go`).

## Known quirks in CLI 1.0.0

These are things lazyunity works around. A CLI update may fix them, and then lazyunity's workaround may break.

| Quirk | Workaround |
|---|---|
| `releases` → `data.nextPage` is the next **offset**, not a page number | Passed back as `--offset` |
| `install <ver> --list-modules` and `modules <ver>` fail with "Editor X is not installed" for a release that is not installed | `editors info <ver>` is used instead |
| `editors info` modules have no `category`, and sizes are `{value, unit}` objects | Category is worked out from `id` (`moduleCategory`); `value` is read as bytes |
| The `releases --stream tech` items come back with `stream` set to `SUPPORTED` | `StreamLabel` maps both to "Supported" |
| `projects pin/unpin` take a glob, not a path | `globEscape` |
| `unity open` and installs start helper processes that keep stdout open | Output goes to temp files, or the pipe is read for 500 ms after exit |
| `projects info` takes ~1 s, while `projects --all` takes ~20 ms | Info is loaded only on demand |

## How to check a new CLI version

1. Install the new CLI next to the old one, or on a test machine, and run `unity <cmd> --help` for every command in the tables above. Look for renamed or removed flags and changed defaults.
2. Run each query in the tables with `--format json` and compare the field names in the output with the "Output fields read" column.
3. Run one small install with `--format ndjson` and check the frame keys (`type`, `phase`, `pct`, `msg`).
4. Run `make check`. Then run the read-only checks against the real CLI: `LAZYUNITY_REAL_CLI=1 go test ./internal/unitycli -run TestRealCLI`.
5. Recheck the "Known quirks" table and drop any workaround the CLI made unnecessary.
6. Update this page and the versions at the top. Raise `MinVersion` if lazyunity starts to need the new CLI.
