# pi-portable

Portable builds of [pi](https://github.com/earendil-works/pi) 1.5.2. Extract, run `pi`, `/login`. All state stays inside the `PiPortable` folder; delete the folder to remove it.

| Package | Platform | Model shell tool |
|---|---|---|
| `PiPortable-windows-x64.zip` | Windows x64 | `powershell` |
| `PiPortable-linux-x64.tar.gz` | Linux x86_64 (glibc) | `bash` |
| `PiPortable-linux-arm64.tar.gz` | Linux aarch64 (glibc) | `bash` |
| `PiPortable-darwin-arm64.tar.gz` | macOS Apple Silicon | `bash` |
| `PiPortable-darwin-x64.tar.gz` | macOS Intel | `bash` |

Download from [Releases](../../releases). Check against `SHA256SUMS`. See [CHANGELOG.md](CHANGELOG.md) for what changed in each release.

## What a package contains

- `pi` / `pi.exe`: Go launcher. Points `HOME` and `PI_CODING_AGENT_DIR` into the folder, plus `TMPDIR` and the XDG dirs on Linux/macOS, or `USERPROFILE`, `APPDATA`, `LOCALAPPDATA`, `TEMP`, `TMP` on Windows. Then it runs `app/pi` with the same arguments. Blocks `install` and `remove`. `pi update` is handled by the launcher (see below).
- `update.bat` / `update.sh`: run `pi update`. `VERSION`: the release tag of the package.
- `app/`: the official pi release for that platform, unmodified.
- `home/.pi/agent/`: settings (`config/`), fd and rg, and the bundled plugins:
  pi-themes, pi-droid-styling 3.0.0, pi-vcc 0.11.2, pi-cliproxyapi-provider 1.4.25.

On Linux/macOS extract with `tar xzf` so executable bits are kept. See `package/README.windows.txt` and `package/README.unix.txt` (shipped as `README.txt`) for end-user notes and limits.

## Updating an installed package

`pi update` (or `update.bat` / `update.sh`) asks the GitHub API for the latest release. If it is newer than `VERSION`, it downloads the package for this platform plus `SHA256SUMS` and `SHA256SUMS.sig`. It refuses to install unless the ed25519 signature matches the public key built into the launcher and the package matches its checksum. It then moves the launcher, `app/` and the bundled plugins to `backup/<old version>/` and puts the new ones in place. `auth.json`, `sessions/`, plugin config and the user's own `settings.json` keys are kept.

A lock file in `tmp/` stops the update while any pi from the folder is running, and stops pi from starting while an update runs. `--check` only reports; `--yes` skips the prompt.

## Build

```bash
bash scripts/build.sh                 # all targets
bash scripts/build.sh linux-x64       # one target
# -> dist/PiPortable-<target>.{zip,tar.gz}, dist/SHA256SUMS
VERSION=v1.5.0.261110 SIGNING_KEY_FILE=key.pem bash scripts/build.sh   # also writes dist/SHA256SUMS.sig
```

Needs bash, curl, tar, unzip, python3 and Go (version in `launcher/go.mod`). Every input is pinned with a checksum in `sources.lock`; the build fails on any mismatch.

Patches applied at build time:
- `patches/cliproxy_prompt.py`: the cliproxyapi login prompt shows `https://example.com` as the sample URL instead of pre-filling the last saved one.

## CI and releases

- `build.yml` builds every package and smoke-tests it on Windows, Linux x64, Linux arm64 and macOS arm64. It runs on pull requests and is called by the two workflows below.
- `upstream.yml` runs daily at 02:17 UTC (`scripts/upstream.py`). A new pi release is taken at once. Plugins, pi-themes, fd and ripgrep are taken once they have been public for 3 days. If anything changed, it commits to the branch `upstream/auto`, builds it, and opens a pull request with the new tag `v<pi version>.<YYMMDD>` and a CHANGELOG section.
- `release.yml` runs on every push to `main`. If the top section of `CHANGELOG.md` names a tag that is not released yet, it builds, waits for approval in the `release` environment, signs `SHA256SUMS` with the environment secret `RELEASE_SIGNING_KEY`, and publishes the GitHub release with the tag.

To release: merge the pull request (or push a commit with a new CHANGELOG section), then approve the waiting deployment in the Actions tab.

## Updating

Follow [MAINTAINING.md](MAINTAINING.md): design constraints, how to bump pi or a plugin, the test checklist, and how to publish a release.
