#!/usr/bin/env bash
# Build portable packages from pinned, checksum-verified sources.
#
#   bash scripts/build.sh [target ...]
#
# Targets: windows-x64 linux-x64 linux-arm64 darwin-x64 darwin-arm64 (default: all)
# Output:  dist/PiPortable-windows-x64.zip, dist/PiPortable-<os>-<arch>.tar.gz, dist/SHA256SUMS
# Needs:   bash, curl, tar, unzip, python3, go (version in launcher/go.mod)
# Env:     WORK_DIR (default ./.work) for downloads and temp files.
set -euo pipefail

REPO=$(cd "$(dirname "$0")/.." && pwd)
WORK=${WORK_DIR:-$REPO/.work}
DL=$WORK/dl
DIST=$REPO/dist
export TMPDIR=$WORK/tmp GOTELEMETRY=off
mkdir -p "$DL" "$TMPDIR" "$DIST"

ALL_TARGETS="windows-x64 linux-x64 linux-arm64 darwin-x64 darwin-arm64"
TARGETS=${*:-$ALL_TARGETS}
for t in $TARGETS; do
  case " $ALL_TARGETS " in *" $t "*) ;; *) echo "unknown target: $t" >&2; exit 1 ;; esac
done

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

# Download every lock entry that is shared or belongs to a requested target.
grep -vE '^\s*(#|$)' "$REPO/sources.lock" | while read -r name url sum; do
  if [[ $name == *@* ]]; then
    case " $TARGETS " in *" ${name#*@} "*) ;; *) continue ;; esac
  fi
  f=$DL/$name
  if [ ! -f "$f" ] || ! verify "$f" "$sum" 2>/dev/null; then
    echo "download $name"
    curl -fsSL --retry 3 -o "$f.part" "$url"
    mv "$f.part" "$f"
  fi
  verify "$f" "$sum"
done

untar() { mkdir -p "$2" && tar xzf "$1" -C "$2" --strip-components=1; }

# Extract one binary from a release archive (zip or tar.gz) into a directory.
extract_bin() { # archive binary-name dest-dir
  local tmp; tmp=$(mktemp -d)
  if unzip -tq "$1" >/dev/null 2>&1; then unzip -q "$1" -d "$tmp"; else tar xzf "$1" -C "$tmp"; fi
  find "$tmp" -type f -name "$2" -exec install -m 0755 {} "$3/" \;
  rm -rf "$tmp"
  [ -f "$3/$2" ] || { echo "missing $2 in $1" >&2; exit 1; }
}

build_target() {
  local target=$1 os=${1%-*} arch=${1#*-}
  local goos=$os goarch=$arch exe="" stage=$WORK/stage/$target
  [ "$arch" = x64 ] && goarch=amd64
  [ "$os" = windows ] && exe=.exe

  echo "== $target"
  rm -rf "$stage"
  local out=$stage/PiPortable
  local agent=$out/home/.pi/agent
  local g=$agent/git/github.com/sting8k
  local nm=$agent/npm/node_modules
  mkdir -p "$out/app" "$out/tmp" "$agent/bin" "$g" "$nm"

  (cd "$REPO/launcher" && GOOS=$goos GOARCH=$goarch CGO_ENABLED=0 \
    go build -trimpath -ldflags="-s -w" -o "$out/pi$exe" .)

  # Official pi release, unmodified.
  if [ "$os" = windows ]; then
    unzip -q "$DL/pi@$target" -d "$out/app"
  else
    untar "$DL/pi@$target" "$out/app"   # tarball root is pi/
  fi
  [ -f "$out/app/pi$exe" ] || { echo "missing app/pi$exe" >&2; exit 1; }

  extract_bin "$DL/fd@$target" "fd$exe" "$agent/bin"
  extract_bin "$DL/rg@$target" "rg$exe" "$agent/bin"

  # Plugins: same layout as a normal install so plugins that scan <agent>/git
  # or <agent>/npm keep working; declared as local packages in settings.json.
  untar "$DL/pi-themes.tgz" "$g/pi-themes"
  untar "$DL/pi-droid-styling.tgz" "$g/pi-droid-styling"
  untar "$DL/pi-themes.tgz" "$g/pi-droid-styling/node_modules/pi-themes"
  untar "$DL/pi-vcc.tgz" "$g/pi-vcc"
  rm -f "$g/pi-vcc/demo.gif"
  untar "$DL/pi-cliproxyapi-provider.tgz" "$nm/@router-for-me/pi-cliproxyapi-provider"
  python3 "$REPO/patches/cliproxy_prompt.py" "$nm/@router-for-me/pi-cliproxyapi-provider/extensions/index.ts"
  # cliproxyapi patches pi-ai's openai-codex-responses.js from disk; the Bun
  # binary embeds pi-ai, so ship the physical copy where the plugin looks.
  untar "$DL/pi-ai.tgz" "$nm/@earendil-works/pi-ai"
  untar "$DL/pi-telemetry.tgz" "$nm/@earendil-works/pi-telemetry"
  untar "$DL/partial-json.tgz" "$nm/partial-json"

  cp "$REPO"/config/*.json "$agent/"
  if [ "$os" = windows ]; then
    # No bash on a stock Windows machine: give the model PowerShell instead.
    python3 - "$agent/settings.json" <<'EOF'
import json, sys
p = sys.argv[1]; d = json.load(open(p))
d["defaultTools"] = ["read", "powershell", "edit", "write"]
open(p, "w").write(json.dumps(d, indent=2) + "\n")
EOF
    cp "$REPO/PACKAGE_README.windows.txt" "$out/README.txt"
  else
    cp "$REPO/PACKAGE_README.unix.txt" "$out/README.txt"
  fi

  # Package with fixed timestamps and permissions for reproducible output.
  if [ "$os" = windows ]; then
    package_zip "$stage" "$DIST/PiPortable-$target.zip"
  else
    package_tar "$stage" "$DIST/PiPortable-$target.tar.gz"
  fi
}

package_zip() { # stage-dir output
  rm -f "$2"
  python3 - "$1" "$2" <<'EOF'
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
}

package_tar() { # stage-dir output
  rm -f "$2"
  python3 - "$1" "$2" <<'EOF'
import gzip, os, sys, tarfile
src, dst = sys.argv[1], sys.argv[2]
EXEC = {"PiPortable/pi", "PiPortable/app/pi",
        "PiPortable/home/.pi/agent/bin/fd", "PiPortable/home/.pi/agent/bin/rg"}
def norm(ti):
    ti.uid = ti.gid = 0; ti.uname = ti.gname = ""; ti.mtime = 1767225600  # 2026-01-01
    if ti.isdir():
        ti.mode = 0o755
    elif ti.isfile():
        # Mode comes from this list, not the filesystem (DrvFS/NTFS reports 0777).
        executable = ti.name in EXEC or ti.name.endswith(".node")
        ti.mode = 0o755 if executable else 0o644
    return ti
with open(dst, "wb") as raw, gzip.GzipFile(fileobj=raw, mode="wb", mtime=0, compresslevel=9) as gz, \
        tarfile.open(fileobj=gz, mode="w", format=tarfile.PAX_FORMAT) as tar:
    root = os.path.join(src, "PiPortable")
    for base, dirs, files in os.walk(root):
        dirs.sort()
        rel = os.path.relpath(base, src).replace(os.sep, "/")
        tar.add(base, arcname=rel, recursive=False, filter=norm)
        for f in sorted(files):
            p = os.path.join(base, f)
            tar.add(p, arcname=f"{rel}/{f}", recursive=False, filter=norm)
EOF
}

for t in $TARGETS; do build_target "$t"; done

(cd "$DIST" && sha256sum PiPortable-* | grep -v SHA256SUMS > SHA256SUMS && cat SHA256SUMS)
du -sh "$DIST"/PiPortable-*
