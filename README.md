# pi-portable

Portable build of [pi](https://github.com/earendil-works/pi) 1.5.0 for Windows x64. Unzip, run `pi.exe`, `/login`. All state stays inside the folder; delete the folder to remove it.

## What the zip contains

- `pi.exe`: Go launcher. Points `HOME`, `USERPROFILE`, `APPDATA`, `LOCALAPPDATA`, `TEMP`, `TMP` and `PI_CODING_AGENT_DIR` into the folder, then runs `app\pi.exe` with the same arguments and console. Blocks `install`, `remove`, `update`.
- `app\`: official `pi-windows-x64.zip`, unmodified.
- `home\.pi\agent\`: settings (`config/`), fd and rg, and the bundled plugins:
  pi-themes, pi-droid-styling 3.0.0, pi-vcc 0.11.2, pi-cliproxyapi-provider 1.4.25.
- The model uses the `powershell` tool instead of `bash`.

See `PACKAGE_README.txt` (shipped as `README.txt`) for end-user notes and limits.

## Build

```bash
bash scripts/build.sh   # -> dist/PiPortable.zip, dist/PiPortable.zip.sha256
```

Needs bash, curl, tar, unzip, python3 and Go (version in `launcher/go.mod`). Every input is pinned with a checksum in `sources.lock`; the build fails on any mismatch.

Patches applied at build time:
- `patches/cliproxy_prompt.py`: the cliproxyapi login prompt shows `https://example.com` as the sample URL instead of pre-filling the last saved one.

## CI

`.github/workflows/build.yml` builds the zip on every push and PR and uploads it as the `PiPortable` artifact. Pushing a tag `v*` also publishes a GitHub release with the zip and its sha256.

## Updating

Follow [MAINTAINING.md](MAINTAINING.md): design constraints, how to bump pi or a plugin, the test checklist, and how to publish a release.
