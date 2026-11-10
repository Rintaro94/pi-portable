# Hướng dẫn bảo trì Pi Portable

Tài liệu này dành cho người (hoặc agent) cập nhật repo sau này mà không có ngữ cảnh của lần làm đầu tiên. Đọc hết mục 1 trước khi sửa gì.

## 1. Thiết kế và các giả định không được phá

Mục tiêu: giải nén zip, chạy `pi.exe`, mọi dữ liệu nằm trong thư mục `PiPortable\`, xóa thư mục là sạch. Không cài gì, không ghi registry.

Cách đạt được:

- `launcher/main.go` (build thành `pi.exe`) đặt các biến môi trường sau rồi chạy `app\pi.exe`:
  `PI_CODING_AGENT_DIR=home\.pi\agent`, `HOME`, `USERPROFILE`, `HOMEDRIVE`, `HOMEPATH` → `home\`,
  `APPDATA` → `home\AppData\Roaming`, `LOCALAPPDATA` → `home\AppData\Local`,
  `TEMP`, `TMP`, `TMPDIR` → `tmp\`, cùng `PI_SKIP_VERSION_CHECK=1`, `PI_TELEMETRY=0`.
- Launcher chặn `install`, `remove`, `uninstall`, `update`. Plugin cố định theo zip, nâng cấp bằng cách phát hành zip mới.
- `app\` là nguyên bản `pi-windows-x64.zip` chính thức, không sửa. Đây là binary Bun, đã nhúng sẵn pi-ai, pi-tui, typebox.
- Plugin nằm đúng chỗ một bản cài thường đặt chúng (`home\.pi\agent\git\...` và `home\.pi\agent\npm\node_modules\...`) nhưng được khai báo trong `config/settings.json` bằng đường dẫn tương đối `./...`. Pi coi chúng là local package nên không bao giờ gọi git hay npm.
- Model dùng tool `powershell` (`defaultTools` trong `config/settings.json`). Không đóng gói Git Bash.

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
.github/workflows/build.yml   CI: build mỗi lần push main/PR; tag v* thì tạo release
scripts/build.sh              tải nguồn theo sources.lock, kiểm checksum, build, đóng zip
sources.lock                  <tên file> <url> <checksum>, mọi đầu vào đều ghim
launcher/                     mã nguồn pi.exe (Go, version trong go.mod)
config/                       settings.json + config mặc định của plugin, chép vào home\.pi\agent\
patches/cliproxy_prompt.py    đổi mẫu URL khi /login cliproxyapi thành https://example.com
PACKAGE_README.txt            README đi kèm trong zip (đổi tên thành README.txt)
```

## 3. Build ở máy local

Cần: bash, curl, tar, unzip, python3, Go (đúng version trong `launcher/go.mod`). Chạy được trên Linux hoặc WSL.

```bash
bash scripts/build.sh
# -> dist/PiPortable.zip, dist/PiPortable.zip.sha256
```

Thư mục làm việc mặc định là `.work/` (đổi bằng `WORK_DIR=...`). File tải về được giữ lại ở `.work/dl/` và chỉ tải lại khi checksum không khớp.

## 4. Quy trình cập nhật

### 4.1 Cập nhật pi lên version mới

1. Lấy URL và sha256 của bản Windows:
   ```bash
   V=1.6.0
   curl -s https://api.github.com/repos/earendil-works/pi/releases/tags/v$V \
     | python3 -c "import json,sys;[print(a['browser_download_url'],a['digest']) for a in json.load(sys.stdin)['assets'] if a['name']=='pi-windows-x64.zip']"
   ```
2. Sửa dòng `pi-windows-x64.zip` trong `sources.lock`.
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
- fd/rg: lấy asset `*-x86_64-pc-windows-msvc.zip` và trường `digest` từ API release GitHub của `sharkdp/fd`, `BurntSushi/ripgrep`. Build script giải nén bằng mẫu `'*/fd.exe'` và `'*/rg.exe'`, nếu cấu trúc zip đổi thì sửa lại.

## 5. Kiểm tra trước khi phát hành

Bắt buộc, trên Windows (hoặc WSL gọi `.exe` qua interop):

```bash
python3 -c "import zipfile;zipfile.ZipFile('dist/PiPortable.zip').extractall('.work/test')"
cd .work/test/PiPortable
./pi.exe --version                  # đúng version pi mới
./pi.exe list                       # đủ 4 package, đường dẫn trong home\.pi\agent
./pi.exe install npm:x; echo $?     # bị chặn, exit 1
./pi.exe --offline --list-models    # không có lỗi nạp extension; cliproxyapi báo "not configured" là đúng
ls tmp/pi-cliproxyapi-provider/     # có openai-codex-responses-cpa-*.mjs → patch codex thành công
```

Nên làm khi đổi version pi hoặc launcher:
- Mở `pi.exe` trong Windows Terminal, `/login` cliproxyapi, kiểm tra màn hình nhập URL hiện `https://example.com`. Chat thử và nhờ model chạy một lệnh PowerShell.
- Ghi lại thời điểm, chạy một phiên, rồi tìm file mới trong `C:\Users\<user>` và `C:\ProgramData`. Không được có file nào do pi tạo ngoài `PiPortable\`. Dấu vết của Windows (Prefetch, Amcache, Jump List, CryptnetUrlCache, WebCache) là bình thường.
- Kiểm tra zip không chứa dữ liệu cá nhân:
  ```bash
  python3 -c "import zipfile;print([n for n in zipfile.ZipFile('dist/PiPortable.zip').namelist() if n.split('/')[-1] in ('auth.json','cliproxyapi.json','cliproxyapi-models.json') or '/sessions/' in n])"
  # phải in ra []
  ```

## 6. Phát hành

```bash
git add -A && git commit -m "Update pi to 1.6.0"
git push origin main            # CI build, kiểm tra tab Actions xanh
git tag -a v1.6.0 -m "Pi Portable 1.6.0"
git push origin v1.6.0          # CI build lại và tạo GitHub Release kèm zip + sha256
```

Quy ước tag: dùng version pi (`v1.6.0`). Nếu chỉ đổi plugin hoặc launcher mà giữ version pi, thêm hậu tố: `v1.6.0-2`, `v1.6.0-3`.

Zip trên CI có hash khác zip build ở máy khác nếu một bên không build trong git repo: Go nhúng commit vào `pi.exe` (`go version -m pi.exe` để xem). Các file còn lại giống hệt nhau.

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
