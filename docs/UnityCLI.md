# Unity CLI surface used by lazyunity

Every Unity CLI command, flag, output field and exit code that lazyunity relies on.
If a Unity CLI update changes anything listed here, lazyunity may break. Check this page against each new CLI release.

| | Version |
|---|---|
| lazyunity | **0.1.0**, plus the unreleased changes in `CHANGELOG.md` |
| Unity CLI it was checked against | **1.0.0** and **1.0.1** |
| Verified range | **1.0.*** (`MinVersion` 1.0.0 to `MaxLine` 1.0, in `internal/unitycli/version.go`) |

All calls live in `internal/unitycli`. No other package runs `unity`.

> **Keep this page up to date.** Update it whenever a lazyunity release adds, removes or changes a CLI call or a decoded field, and whenever a new Unity CLI version changes any of them. Bump the versions in the table above when you do.

## Verified range and update rules

Every lazyunity release has a **verified range** of Unity CLI versions: from `MinVersion` up to every patch of `MaxLine`.

- **Patch releases** (N.M.\*) are trusted not to change the commands above, so a whole release line is verified at once.
- **A new minor or major line** may change them. It counts as verified only once it has been checked against this page and `MaxLine` has been moved up in a lazyunity release.

What the user sees on the **Unity CLI** row (Tools tab, `3`):

| Installed CLI | Row | Updates offered |
|---|---|---|
| Missing | yellow: `not installed` | The official installer, pinned to the newest verified release on Linux/macOS |
| Below `MinVersion` | **red**: `update required` | The same installer |
| Reports no version | **red**: `not working` | The same installer |
| Inside the range, no newer **verified** version | **green**: `ready · <version>`, even if a newer unverified version is out | Only in the menu |
| Inside the range, a newer **verified** version is out | **yellow**: `update available: <version>` | That verified version |
| Beyond the range | **yellow**: `not verified` | In the menu: go back to the newest verified version |
| Beyond the range and it broke a launch load | **red**: `safe mode` | Downgrade to the newest verified version, roll back, or reinstall with the installer |

- An update **beyond** the range never reaches the main interface: not the row, the bottom line or a toast. The user only sees it after opening the Unity CLI menu, where it appears as `Not verified update X available`. Installing it asks for confirmation first, with a warning that lazyunity may break. It shows up on the row only once a lazyunity release moves `MaxLine` up to its line.
- Switching to the beta feed also asks first, because the newest beta may be beyond the range.

**Safe mode.** With a CLI beyond the range, lazyunity goes into safe mode as soon as one of these launch loads fails:

- `projects --all`
- `editors --installed`
- `auth status`
- `install-path --get`

Network errors (exit 7), sign-in errors (exit 3), install locks (exit 9) and cancels or time-outs (exit 130/143) do not count, because they don't mean the CLI changed. In safe mode only the Tools panel is shown, cut down to the Unity CLI, Config file, Report an issue (opens the GitHub issues page) and About rows. Every other CLI use is refused until a probe finds a working CLI again.

**`--safe`** starts lazyunity in safe mode whatever the CLI, for one that breaks lazyunity without any of those loads failing (a hang, wrong data, a crash). The projects are not loaded before the first frame, and nothing but the Unity CLI tools touches the CLI. The menu moves the CLI to the newest verified version (when it is on another one), rolls it back, reinstalls it with the installer, or leaves safe mode and loads everything with the CLI as it is.

**The newest verified release** comes from Unity's CDN, because the CLI cannot list its own versions. lazyunity asks for `<CDNBase><version>/latest.json` (HEAD requests) for each patch of `MaxLine`, from `.0` up. It stops after 3 missing patches in a row, and falls back to older verified lines if `MaxLine` has no releases yet (`unitycli.LatestVerified`). This takes about 1.4 s and runs in the background at launch. Until it answers, `MinVersion` is used as the target.

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
| `editors upgrade --all --dry-run` | query | `EditorUpdates` | Find the newest patch of each installed Editor's release line (the "X available" label and `U`). lazyunity runs the update itself, as an `install` | `data[]` (an array, not `data.items`): `current`, `architecture`, `target` (`null` when none), `upgradeAvailable` |
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
| `self-update --check [--channel stable\|beta]` | query | `CheckUpdate` | Check for a newer CLI on the feed | `data.currentVersion`, `data.latestVersion` |
| `self-update --yes [--channel stable\|beta] [--target <ver>]` | action | `SelfUpdate` | Update the CLI to an exact version, including downgrades back into the verified range, or switch it to another channel. `--target` takes a full semver (`1.0.1`), not a line (`1.0`) | none |
| `self-update --rollback --yes` | action | `Rollback` | Put back the binary the last update replaced (offered only in safe mode, as the version it restores may be unverified too). It cannot be combined with `--dry-run` | none |
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
| Linux / macOS | `https://unity.com/install.sh` with `UNITY_NON_INTERACTIVE=1` and `UNITY_CLI_VERSION=<newest verified>`, which pins the release |
| Windows | `irm https://unity.com/install.ps1 \| iex`. `install.ps1` has no `UNITY_CLI_VERSION`, so it always installs the newest release. If that is beyond the range, the row says so and the menu can move it back with `self-update --target` |
| Both | Checks `https://public-cdn.cloud.unity3d.com/hub/prod/cli/latest.json` first. If it is missing, sets `UNITY_CLI_CHANNEL=beta` (only matters when unpinned) |

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
| The CDN has no list of CLI versions (`index.json` and the like return 404) | Patch numbers are probed one by one (`LatestVerified`) |
| `editors upgrade` installs without letting the user pick modules, and reports a `target` that may already be installed | Only its `--dry-run` is used, to learn the newest patch; a target that is installed is not offered |
| The `--rollback` flag cannot be dry-run | Nothing to work around; it is only offered as a choice in the menu |

## How to check a new CLI version

1. Install the new CLI next to the old one, or on a test machine, and run `unity <cmd> --help` for every command in the tables above. Look for renamed or removed flags and changed defaults.
2. Run each query in the tables with `--format json` and compare the field names in the output with the "Output fields read" column.
3. Run one small install with `--format ndjson` and check the frame keys (`type`, `phase`, `pct`, `msg`).
4. Run `make check`. Then run the read-only checks against the real CLI: `LAZYUNITY_REAL_CLI=1 go test ./internal/unitycli -run TestRealCLI`.
5. Recheck the "Known quirks" table and drop any workaround the CLI made unnecessary.
6. Update this page and the versions at the top. Raise `MinVersion` if lazyunity starts to need the new CLI.
7. Once a new minor or major line passes these checks, move `MaxLine` up to it in the next lazyunity release. Until then, users see that line as not verified. A new patch of a verified line needs no change, but run steps 2–4 against it when you can.
