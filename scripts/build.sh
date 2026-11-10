#!/usr/bin/env bash
# Build dist/PiPortable.zip from pinned, checksum-verified sources.
# Requirements: bash, curl, tar, unzip, python3, go (version in launcher/go.mod).
# Env: WORK_DIR (default ./.work) for downloads and temp files.
set -euo pipefail

REPO=$(cd "$(dirname "$0")/.." && pwd)
WORK=${WORK_DIR:-$REPO/.work}
DL=$WORK/dl
STAGE=$WORK/stage
DIST=$REPO/dist
export TMPDIR=$WORK/tmp GOTELEMETRY=off
mkdir -p "$DL" "$TMPDIR" "$DIST"

# 1. Download + verify
verify() { # file checksum
  local f=$1 sum=$2
  case $sum in
    sha256:*) echo "${sum#sha256:}  $f" | sha256sum -c --quiet - ;;
    sha512-*) python3 - "$f" "${sum#sha512-}" <<'EOF'
import base64, hashlib, sys
d = base64.b64encode(hashlib.sha512(open(sys.argv[1], "rb").read()).digest()).decode()
sys.exit(0 if d == sys.argv[2] else f"integrity mismatch: {sys.argv[1]}")
EOF
    ;;
    *) echo "unknown checksum format: $sum" >&2; return 1 ;;
  esac
}
grep -vE '^\s*(#|$)' "$REPO/sources.lock" | while read -r name url sum; do
  f=$DL/$name
  if [ ! -f "$f" ] || ! verify "$f" "$sum" 2>/dev/null; then
    echo "download $name"
    curl -fsSL --retry 3 -o "$f.part" "$url"
    mv "$f.part" "$f"
  fi
  verify "$f" "$sum"
done

# 2. Stage
rm -rf "$STAGE"
OUT=$STAGE/PiPortable
AGENT=$OUT/home/.pi/agent
G=$AGENT/git/github.com/sting8k
NM=$AGENT/npm/node_modules
mkdir -p "$OUT/app" "$OUT/tmp" "$AGENT/bin" "$G" "$NM"

(cd "$REPO/launcher" && GOOS=windows GOARCH=amd64 CGO_ENABLED=0 \
  go build -trimpath -ldflags="-s -w" -o "$OUT/pi.exe" .)

unzip -q "$DL/pi-windows-x64.zip" -d "$OUT/app"
unzip -q -j "$DL/fd.zip" '*/fd.exe' -d "$AGENT/bin"
unzip -q -j "$DL/rg.zip" '*/rg.exe' -d "$AGENT/bin"

untar() { mkdir -p "$2" && tar xzf "$1" -C "$2" --strip-components=1; }
untar "$DL/pi-themes.tgz" "$G/pi-themes"
untar "$DL/pi-droid-styling.tgz" "$G/pi-droid-styling"
untar "$DL/pi-themes.tgz" "$G/pi-droid-styling/node_modules/pi-themes"
untar "$DL/pi-vcc.tgz" "$G/pi-vcc"
rm -f "$G/pi-vcc/demo.gif"
untar "$DL/pi-cliproxyapi-provider.tgz" "$NM/@router-for-me/pi-cliproxyapi-provider"
python3 "$REPO/patches/cliproxy_prompt.py" "$NM/@router-for-me/pi-cliproxyapi-provider/extensions/index.ts"
# cliproxyapi patches pi-ai's openai-codex-responses.js from disk; the Bun
# binary embeds pi-ai, so ship the physical copy where the plugin looks.
untar "$DL/pi-ai.tgz" "$NM/@earendil-works/pi-ai"
untar "$DL/pi-telemetry.tgz" "$NM/@earendil-works/pi-telemetry"
untar "$DL/partial-json.tgz" "$NM/partial-json"

cp "$REPO"/config/*.json "$AGENT/"
cp "$REPO/PACKAGE_README.txt" "$OUT/README.txt"

# 3. Zip (deterministic order and timestamps)
ZIP=$DIST/PiPortable.zip
rm -f "$ZIP"
python3 - "$STAGE" "$ZIP" <<'EOF'
import os, sys, zipfile
src, dst = sys.argv[1], sys.argv[2]
def add(z, path, arc, is_dir=False):
    info = zipfile.ZipInfo(arc + ("/" if is_dir else ""), date_time=(2026, 1, 1, 0, 0, 0))
    info.external_attr = (0o40755 << 16 | 0x10) if is_dir else (0o100644 << 16)
    info.compress_type = zipfile.ZIP_DEFLATED
    z.writestr(info, b"" if is_dir else open(path, "rb").read(), compresslevel=9)
with zipfile.ZipFile(dst, "w") as z:
    for base, dirs, files in os.walk(os.path.join(src, "PiPortable")):
        dirs.sort()
        rel = os.path.relpath(base, src).replace(os.sep, "/")
        if not files and not dirs:
            add(z, base, rel, is_dir=True)
        for f in sorted(files):
            add(z, os.path.join(base, f), f"{rel}/{f}")
EOF
(cd "$DIST" && sha256sum PiPortable.zip > PiPortable.zip.sha256 && cat PiPortable.zip.sha256)
du -sh "$ZIP"
