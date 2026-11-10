# Hướng dẫn bảo trì Pi Portable

Tài liệu này dành cho người (hoặc agent) cập nhật repo sau này mà không có ngữ cảnh của lần làm đầu tiên. Đọc hết mục 1 trước khi sửa gì.

## 1. Thiết kế và các giả định không được phá

Mục tiêu: giải nén, chạy `pi` (`pi.exe` trên Windows), mọi dữ liệu nằm trong thư mục `PiPortable/`, xóa thư mục là sạch. Không cài gì, không ghi registry.

Có 5 target: `windows-x64`, `linux-x64`, `linux-arm64`, `darwin-x64`, `darwin-arm64`. Windows đóng gói `.zip`, các target còn lại đóng `.tar.gz` (để giữ quyền thực thi).

Cách đạt được:

- Launcher (`launcher/`) đặt biến môi trường rồi chạy `app/pi`:
  - Chung: `PI_CODING_AGENT_DIR=home/.pi/agent`, `HOME=home/`, `TMPDIR=tmp/`, `PI_SKIP_VERSION_CHECK=1`, `PI_TELEMETRY=0`.
  - Windows (`platform_windows.go`): thêm `USERPROFILE`, `HOMEDRIVE`, `HOMEPATH`, `APPDATA` → `home\AppData\Roaming`, `LOCALAPPDATA` → `home\AppData\Local`, `TEMP`, `TMP`. Chạy pi làm tiến trình con, bỏ qua Ctrl+C để chờ và trả đúng exit code.
  - Linux/macOS (`platform_unix.go`): thêm `XDG_CONFIG_HOME`, `XDG_CACHE_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME` trỏ vào `home/`. Dùng `exec` thay tiến trình launcher bằng pi, nên tín hiệu và job control đi thẳng tới pi.
- Launcher chặn `install`, `remove`, `uninstall`, `update`. Plugin cố định theo gói, nâng cấp bằng cách phát hành gói mới.
- `app/` là nguyên bản release chính thức của pi cho từng nền tảng, không sửa. Đây là binary Bun, đã nhúng sẵn pi-ai, pi-tui, typebox. Riêng plugin, config và pi-ai đi kèm là giống nhau ở mọi target.
- Plugin nằm đúng chỗ một bản cài thường đặt chúng (`home\.pi\agent\git\...` và `home\.pi\agent\npm\node_modules\...`) nhưng được khai báo trong `config/settings.json` bằng đường dẫn tương đối `./...`. Pi coi chúng là local package nên không bao giờ gọi git hay npm.
- Windows: build script thêm `defaultTools: [read, powershell, edit, write]` vào settings, vì máy Windows thường không có bash. Không đóng gói Git Bash. Linux/macOS dùng mặc định của pi (tool `bash`). Không đặt `defaultTools` trong `config/settings.json`, vì file này dùng chung.

Những chỗ dễ vỡ khi cập nhật (đã kiểm chứng ở bản 1.5.0):

| Phụ thuộc | Vì sao quan trọng |
|---|---|
| `pi-droid-styling` đọc config ở `homedir()/.pi/agent/...` và quét theme ở `homedir()/.pi/agent/git` | Giữ plugin dưới `home\.pi\agent\git\github.com\sting8k\`. Nếu đổi chỗ, theme và config của plugin không tìm thấy. |
| `pi-cliproxyapi-provider` patch file `openai-codex-responses.js` của pi-ai **trên đĩa** | Binary Bun không có file này, nên zip kèm bản `@earendil-works/pi-ai` cùng version với pi, đặt ở `npm\node_modules\@earendil-works\pi-ai`. Plugin tìm thấy qua đường dẫn `HOME/.pi/agent/npm/node_modules/...`. **Version pi-ai phải bằng version pi.** |
| pi-ai cần `partial-json` và `@earendil-works/pi-telemetry` | Hai gói này đi kèm cạnh pi-ai. Khi đổi version pi-ai, kiểm tra lại `dependencies` trong `package.json` của nó (mục 4). |
| `patches/cliproxy_prompt.py` sửa `extensions/index.ts` của cliproxyapi theo chuỗi chính xác | Nếu plugin đổi code, script sẽ dừng với lỗi `patch anchor not found`. Cập nhật lại chuỗi trong script. |
| `pi-droid-styling` cần `node_modules/pi-themes` | Build script giải nén `pi-themes.tgz` vào đó. Nếu plugin đổi phụ thuộc, kiểm tra `dependencies` của nó. |

## 2. Cấu trúc repo

```
.github/workflows/build.yml   CI: build mọi target, smoke test trên từng OS, tag v* thì tạo release
scripts/build.sh              tải nguồn theo sources.lock, kiểm checksum, build, đóng gói
scripts/release-notes.py      in mục CHANGELOG.md của một tag (CI dùng làm release notes)
CHANGELOG.md                  ghi chú từng release, viết cho người dùng
sources.lock                  <tên file> <url> <checksum>, mọi đầu vào đều ghim
launcher/main.go              logic chung của launcher
launcher/platform_*.go        phần riêng Windows và Unix
config/                       settings.json + config mặc định của plugin, chép vào home/.pi/agent/
patches/cliproxy_prompt.py    đổi mẫu URL khi /login cliproxyapi thành https://example.com
PACKAGE_README.windows.txt    README đi kèm bản Windows (đổi tên thành README.txt)
PACKAGE_README.unix.txt       README đi kèm bản Linux/macOS
```

Trong `sources.lock`, file có dạng `<tên>@<target>` (ví dụ `pi@linux-arm64`) chỉ dùng cho target đó. File không có `@` (plugin, pi-ai) dùng chung.

## 3. Build ở máy local

Cần: bash, curl, tar, unzip, python3, Go (đúng version trong `launcher/go.mod`). Chạy được trên Linux hoặc WSL.

```bash
bash scripts/build.sh                  # mọi target
bash scripts/build.sh linux-x64        # một hoặc vài target
# -> dist/PiPortable-<target>.zip|.tar.gz, dist/SHA256SUMS
```

Gói `.tar.gz` được đặt quyền theo danh sách cố định trong `package_tar` (launcher, `app/pi`, fd, rg, file `.node`), không lấy từ filesystem. Lý do: trên WSL, thư mục `/mnt/<ổ đĩa>` báo mọi file là `0777`. Nếu thêm binary mới cần quyền chạy, thêm vào danh sách `EXEC` trong `scripts/build.sh`.

Thư mục làm việc mặc định là `.work/` (đổi bằng `WORK_DIR=...`). File tải về được giữ lại ở `.work/dl/` và chỉ tải lại khi checksum không khớp.

## 4. Quy trình cập nhật

### 4.1 Cập nhật pi lên version mới

1. Lấy URL và sha256 của cả 5 bản:
   ```bash
   V=1.6.0
   curl -s https://api.github.com/repos/earendil-works/pi/releases/tags/v$V \
     | python3 -c "import json,sys;[print(a['browser_download_url'],a['digest']) for a in json.load(sys.stdin)['assets'] if a['name'].startswith(('pi-windows-x64','pi-linux-','pi-darwin-'))]"
   ```
2. Sửa 5 dòng `pi@<target>` trong `sources.lock`. Nếu pi bỏ hoặc thêm nền tảng, sửa `ALL_TARGETS` trong `scripts/build.sh` và ma trận `smoke` trong workflow.
3. **Cập nhật pi-ai cùng version** (bắt buộc, xem mục 1):
   ```bash
   npm view @earendil-works/pi-ai@$V dist.tarball dist.integrity dependencies
   npm view @earendil-works/pi-telemetry@$V dist.tarball dist.integrity
   ```
   Sửa các dòng `pi-ai.tgz`, `pi-telemetry.tgz`. Nếu `dependencies` của pi-ai đổi version `partial-json` hoặc thêm gói mới mà `api/openai-codex-responses.js` import tới, thêm hoặc sửa dòng tương ứng và thêm lệnh `untar` trong `scripts/build.sh`.
   Cách tìm các gói mà file đó thực sự import: giải nén pi-ai rồi lần theo các `import` từ `dist/api/openai-codex-responses.js` và `dist/api/openai-responses-shared.js`. Ở 1.5.0 chỉ có `partial-json` và `@earendil-works/pi-telemetry`.
4. Sửa `"lastChangelogVersion"` trong `config/settings.json` thành version mới (để pi không hiện changelog lần đầu chạy).
5. Đọc `CHANGELOG.md` của pi tìm thay đổi liên quan tới: `PI_CODING_AGENT_DIR`, cách tìm `settings.json`, local package, `defaultTools`, tool `powershell`, lệnh `install/update`. Nếu có, sửa launcher hoặc config cho phù hợp.
6. Build và kiểm tra theo mục 5.

### 4.2 Cập nhật một plugin

npm:
```bash
npm view @sting8k/pi-vcc@latest version dist.tarball dist.integrity peerDependencies dependencies
```
Sửa URL và checksum trong `sources.lock` (giữ nguyên tên file đích, ví dụ `pi-vcc.tgz`).

pi-themes lấy theo commit trên GitHub:
```bash
SHA=$(git ls-remote https://github.com/sting8k/pi-themes HEAD | cut -f1)
curl -fsSL -o /tmp/t.tgz https://codeload.github.com/sting8k/pi-themes/tar.gz/$SHA && sha256sum /tmp/t.tgz
```
Ghi URL có `$SHA` và `sha256:<hex>` vào `sources.lock`.

Sau khi đổi version plugin:
- Kiểm tra `peerDependencies`: plugin phải hỗ trợ version pi đang dùng.
- Kiểm tra `dependencies`: pi chỉ cung cấp sẵn `@earendil-works/pi-ai`, `pi-agent-core`, `pi-coding-agent`, `pi-tui`, `typebox`. Gói nào khác phải được đóng kèm trong `node_modules` của plugin (xem cách làm với `pi-themes` trong `scripts/build.sh`).
- Plugin không được có native module (`.node`), vì không build lại cho Windows.
- Với `pi-droid-styling` và `pi-vcc`, sửa version trong `extensionChangelogVersions` của `config/settings.json`.
- Với `pi-cliproxyapi-provider`, chạy build. Nếu `cliproxy_prompt.py` báo không tìm thấy anchor, mở `extensions/index.ts`, hàm `promptConnection`, cập nhật chuỗi `old` trong script cho khớp.

### 4.3 Thêm plugin mới

1. Thêm dòng vào `sources.lock`.
2. Thêm lệnh `untar` trong `scripts/build.sh`, đặt vào `$G/<repo>` (plugin từ git) hoặc `$NM/<tên npm>` (plugin npm).
3. Thêm đường dẫn tương đối vào `packages` trong `config/settings.json`.
4. Nếu plugin ghi file, xác nhận nó ghi vào `getAgentDir()`, `homedir()` hoặc `tmpdir()`. Ba chỗ này đều đã được chuyển vào thư mục portable. Nếu plugin dùng đường dẫn tuyệt đối khác, không đưa vào.

### 4.4 Cập nhật Go, fd, rg

- Go: sửa dòng `go` trong `launcher/go.mod`. CI đọc version từ đây.
- fd/rg: mỗi công cụ có 5 dòng trong `sources.lock`. Lấy URL và `digest`:
  ```bash
  for r in sharkdp/fd BurntSushi/ripgrep; do
    curl -s https://api.github.com/repos/$r/releases/latest | python3 -c "
  import json,sys
  want=('x86_64-pc-windows-msvc.zip','x86_64-unknown-linux-musl.tar.gz','aarch64-unknown-linux-musl.tar.gz','x86_64-apple-darwin.tar.gz','aarch64-apple-darwin.tar.gz')
  [print(a['browser_download_url'],a['digest']) for a in json.load(sys.stdin)['assets'] if a['name'].endswith(want)]"
  done
  ```
  Bản Linux dùng `musl` (static), chạy được trên mọi distro. Build script tìm file `fd`/`rg` (hoặc `.exe`) ở bất kỳ đâu trong archive, nên đổi cấu trúc thư mục bên trong không ảnh hưởng.

## 5. Kiểm tra trước khi phát hành

CI tự chạy các bước cơ bản dưới đây (job `smoke`) trên Windows, Linux x64, Linux arm64 và macOS arm64. macOS Intel (`darwin-x64`) không được chạy thử trên CI, chỉ được build. Xem tab Actions phải xanh hết.

Kiểm tra thủ công một bản (ví dụ Linux, chạy trên filesystem Linux, không qua `/mnt/<ổ>`):

```bash
mkdir -p /tmp/pt && tar xzf dist/PiPortable-linux-x64.tar.gz -C /tmp/pt
cd /tmp/pt/PiPortable
./pi --version                      # đúng version pi mới
./pi list                           # đủ 4 package, đường dẫn trong home/.pi/agent
./pi install npm:x; echo $?         # bị chặn, exit 1
./pi --offline --list-models        # không có lỗi nạp extension; cliproxyapi báo "not configured" là đúng
ls tmp/pi-cliproxyapi-provider/     # có openai-codex-responses-cpa-*.mjs → patch codex thành công
```

Bản Windows: giải nén `PiPortable-windows-x64.zip` rồi chạy các lệnh tương tự với `./pi.exe` (trong PowerShell, Git Bash hoặc WSL).

Nên làm khi đổi version pi hoặc launcher:
- Mở pi, `/login` cliproxyapi, kiểm tra màn hình nhập URL hiện `https://example.com`. Chat thử và nhờ model chạy một lệnh (PowerShell trên Windows, bash trên Linux/macOS).
- Kiểm tra không ghi ra ngoài:
  - Windows: ghi lại thời điểm, chạy một phiên, rồi tìm file mới trong `C:\Users\<user>` và `C:\ProgramData`. Không được có file nào do pi tạo ngoài `PiPortable\`. Dấu vết của Windows (Prefetch, Amcache, Jump List, CryptnetUrlCache, WebCache) là bình thường.
  - Linux/macOS: chạy với một `HOME` và `TMPDIR` giả rồi kiểm tra hai thư mục đó vẫn rỗng:
    ```bash
    mkdir -p /tmp/fh /tmp/ft
    env -i PATH=/usr/bin:/bin TERM=xterm HOME=/tmp/fh TMPDIR=/tmp/ft ./pi --offline --list-models
    find /tmp/fh /tmp/ft -mindepth 1      # phải không in gì
    ```
- Kiểm tra zip không chứa dữ liệu cá nhân:
  ```bash
  python3 - <<'EOF'
  import glob, tarfile, zipfile
  bad = ('auth.json', 'cliproxyapi.json', 'cliproxyapi-models.json')
  for p in glob.glob('dist/PiPortable-*'):
      names = zipfile.ZipFile(p).namelist() if p.endswith('.zip') else tarfile.open(p).getnames()
      print(p, [n for n in names if n.split('/')[-1] in bad or '/sessions/' in n])   # phải là []
  EOF
  ```

## 6. Phát hành

Trước khi tag, thêm mục mới lên đầu `CHANGELOG.md` với tiêu đề đúng dạng `## v1.6.0 - YYYY-MM-DD`. CI lấy nguyên mục này làm release notes (`scripts/release-notes.py`). Nếu thiếu, CI dừng ngay và không tạo release. Viết cho người dùng: có gói gì, thay đổi gì, cần làm gì khi nâng cấp, còn giới hạn gì. Xem `python3 scripts/release-notes.py v1.6.0` để kiểm tra trước.

```bash
git add -A && git commit -m "Update pi to 1.6.0"
git push origin main            # CI build, kiểm tra tab Actions xanh
git tag -a v1.6.0 -m "Pi Portable 1.6.0"
git push origin v1.6.0          # CI build, smoke test, rồi tạo GitHub Release kèm mọi gói + SHA256SUMS
```

Quy ước tag: dùng version pi (`v1.6.0`). Nếu chỉ đổi plugin hoặc launcher mà giữ version pi, thêm hậu tố: `v1.6.0-2`, `v1.6.0-3`.

Gói build trên CI có hash khác gói build ở máy khác nếu một bên không build trong git repo: Go nhúng commit vào launcher (`go version -m pi` để xem). Các file còn lại giống hệt nhau.

Release trước v1.5.0-2 chỉ có `PiPortable.zip` (Windows). Từ v1.5.0-2, tên file theo target: `PiPortable-windows-x64.zip`, `PiPortable-linux-x64.tar.gz`, ...

## 7. Quyền push

Repo dùng **deploy key** chỉ có quyền với repo này (Settings → Deploy keys, có write access). Ở máy hiện tại:
- Private key: `/root/.ssh/pi-portable_deploy` (không passphrase).
- `/root/.ssh/config` có host `github-pi-portable` trỏ tới key đó.
- Remote: `git@github-pi-portable:Rintaro94/pi-portable.git`.

Sang máy khác: tạo key mới (`ssh-keygen -t ed25519 -N '' -f ~/.ssh/pi-portable_deploy`), thêm vào Deploy keys với write access, thêm khối `Host` tương tự vào `~/.ssh/config`. Không copy private key giữa các máy. Thu hồi bằng cách xóa key trong Deploy keys.

## 8. Không được commit

- `auth.json`, `cliproxyapi.json`, `cliproxyapi-models.json`, `sessions/` từ một lần chạy thử. Chúng chứa API key, URL proxy và lịch sử chat. `.gitignore` đã chặn, nhưng vẫn phải kiểm tra `git status` trước khi commit.
- `.work/`, `dist/`.
- URL hoặc key của proxy cá nhân trong `config/` hoặc `patches/`.
