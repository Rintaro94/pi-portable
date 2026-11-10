# pi-portable

Portable builds of [pi](https://github.com/earendil-works/pi) 1.5.0. Extract, run `pi`, `/login`. All state stays inside the `PiPortable` folder; delete the folder to remove it.

| Package | Platform | Model shell tool |
|---|---|---|
| `PiPortable-windows-x64.zip` | Windows x64 | `powershell` |
| `PiPortable-linux-x64.tar.gz` | Linux x86_64 (glibc) | `bash` |
| `PiPortable-linux-arm64.tar.gz` | Linux aarch64 (glibc) | `bash` |
| `PiPortable-darwin-arm64.tar.gz` | macOS Apple Silicon | `bash` |
| `PiPortable-darwin-x64.tar.gz` | macOS Intel | `bash` |

Download from [Releases](../../releases). Check against `SHA256SUMS`.

## What a package contains

- `pi` / `pi.exe`: Go launcher. Points `HOME` and `PI_CODING_AGENT_DIR` into the folder, plus `TMPDIR` and the XDG dirs on Linux/macOS, or `USERPROFILE`, `APPDATA`, `LOCALAPPDATA`, `TEMP`, `TMP` on Windows. Then it runs `app/pi` with the same arguments. Blocks `install`, `remove`, `update`.
- `app/`: the official pi release for that platform, unmodified.
- `home/.pi/agent/`: settings (`config/`), fd and rg, and the bundled plugins:
  pi-themes, pi-droid-styling 3.0.0, pi-vcc 0.11.2, pi-cliproxyapi-provider 1.4.25.

On Linux/macOS extract with `tar xzf` so executable bits are kept. See `PACKAGE_README.windows.txt` and `PACKAGE_README.unix.txt` (shipped as `README.txt`) for end-user notes and limits.

## Build

```bash
bash scripts/build.sh                 # all targets
bash scripts/build.sh linux-x64       # one target
# -> dist/PiPortable-<target>.{zip,tar.gz}, dist/SHA256SUMS
```

Needs bash, curl, tar, unzip, python3 and Go (version in `launcher/go.mod`). Every input is pinned with a checksum in `sources.lock`; the build fails on any mismatch.

Patches applied at build time:
- `patches/cliproxy_prompt.py`: the cliproxyapi login prompt shows `https://example.com` as the sample URL instead of pre-filling the last saved one.

## CI

`.github/workflows/build.yml` builds all packages on every push and PR and uploads them as the `PiPortable` artifact. A smoke test then runs each package on Windows, Linux x64, Linux arm64 and macOS arm64 runners. Pushing a tag `v*` also publishes a GitHub release with every package and `SHA256SUMS`.

## Updating

Follow [MAINTAINING.md](MAINTAINING.md): design constraints, how to bump pi or a plugin, the test checklist, and how to publish a release.
