# Changelog

Each release is named after the pi version it ships. A `-N` suffix marks a later build of the same pi version, for changes to packaging, plugins or the launcher.

## v1.5.0-2 - 2026-11-10

Adds Linux and macOS packages. The Windows package works the same as in v1.5.0.

### Downloads

| File | Platform | Shell tool for the model |
|---|---|---|
| `PiPortable-windows-x64.zip` | Windows x64 | `powershell` |
| `PiPortable-linux-x64.tar.gz` | Linux x86_64 (glibc) | `bash` |
| `PiPortable-linux-arm64.tar.gz` | Linux aarch64 (glibc) | `bash` |
| `PiPortable-darwin-arm64.tar.gz` | macOS, Apple Silicon | `bash` |
| `PiPortable-darwin-x64.tar.gz` | macOS, Intel | `bash` |

Check downloads against `SHA256SUMS`.

### Added

- Linux x64 and arm64 packages.
- macOS x64 and arm64 packages.
- On Linux and macOS, the launcher points `HOME`, `TMPDIR`, `XDG_CONFIG_HOME`, `XDG_CACHE_HOME`, `XDG_DATA_HOME` and `XDG_STATE_HOME` into the `PiPortable` folder. It then replaces itself with pi, so Ctrl+C, Ctrl+Z and the exit code go straight to pi.
- Every package is test-run on its own OS before release (Windows, Linux x64, Linux arm64, macOS arm64). The test checks that pi starts, all 4 plugins load, and `install` is blocked.

### Changed

- The Windows download is renamed from `PiPortable.zip` to `PiPortable-windows-x64.zip`.
- Checksums for all packages are in one `SHA256SUMS` file. It replaces `PiPortable.zip.sha256`.
- The message shown for a blocked `install`, `remove` or `update` now says "download a new package" instead of "download a new zip".

### Upgrading from v1.5.0

Extract the new package to a new folder. To keep your login and chat history, copy `home\.pi\agent\auth.json` and the `home\.pi\agent\sessions\` folder from the old folder into the new one.

### Known limitations

- The macOS Intel package was built but not test-run.
- The macOS binaries are not signed. If Gatekeeper blocks them, run `xattr -dr com.apple.quarantine PiPortable` once.
- Extract the Linux and macOS packages with `tar xzf`. Tools that drop file permissions will make `pi` fail to start.

## v1.5.0 - 2026-11-10

First release. Windows x64 only.

### Contents

- pi 1.5.0: the official `pi-windows-x64.zip`, unmodified, in `app\`.
- `pi.exe` launcher. It points `HOME`, `USERPROFILE`, `APPDATA`, `LOCALAPPDATA`, `TEMP`, `TMP` and `PI_CODING_AGENT_DIR` into the `PiPortable` folder, then runs `app\pi.exe` with the same arguments.
- Bundled plugins, loaded from the folder without npm or git:
  - pi-themes (commit `9df900d`)
  - pi-droid-styling 3.0.0
  - pi-vcc 0.11.2
  - pi-cliproxyapi-provider 1.4.25, with a copy of `@earendil-works/pi-ai` 1.5.0 that the plugin needs on disk
- fd 10.5.2 and ripgrep 15.2.0, so pi does not download them on first run.
- The model runs commands with the `powershell` tool, because a stock Windows machine has no bash.

### Behavior

- Everything pi writes stays in the `PiPortable` folder: settings, `auth.json`, sessions, plugin caches, temp files. Delete the folder to remove it all.
- `install`, `remove`, `uninstall` and `update` are blocked. The plugins are fixed per release.
- The version check and install telemetry are turned off.
- The cliproxyapi login prompt shows `https://example.com` as the sample URL. It no longer pre-fills the last URL you used.

### Known limitations

- `auth.json` and `cliproxyapi.json` store your API key in plain text. Anyone with a copy of the folder can use your account.
- The model's shell tool runs as your user. It can read and write anywhere your user can, not only inside the portable folder.
- OAuth logins open your default browser. Its cookies live in the browser profile, outside the portable folder.
- Windows still logs that the program ran (Prefetch, Amcache, ShimCache, Jump Lists, and `Zone.Identifier` on the downloaded zip).
- The `!` command in the pi editor needs bash. Without Git for Windows it fails; everything else works.
