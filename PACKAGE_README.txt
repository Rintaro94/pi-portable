Pi Portable (pi 1.5.0, Windows x64)
====================================

Chay
  - Nhap dup pi.exe, hoac trong cmd/PowerShell: D:\duong\dan\PiPortable\pi.exe
  - Lan dau: go /login roi chon provider (vd. cliproxyapi).
  - Nen dung Windows Terminal. Neu dung cmd co dien bi loi hien thi,
    chay: pi.exe --tui-mode regular

Cau truc
  pi.exe          launcher: dat bien moi truong roi chay app\pi.exe
  app\            ban pi chinh thuc (khong sua)
  home\           HOME/USERPROFILE/APPDATA/LOCALAPPDATA gia
    .pi\agent\    toan bo cau hinh, auth.json, sessions, plugin
  tmp\            TEMP/TMP

Plugin co san: pi-themes, pi-droid-styling, pi-vcc, pi-cliproxyapi-provider.
Lenh install / remove / update bi khoa. Muon nang cap: tai zip moi,
chep home\.pi\agent\auth.json va sessions\ sang ban moi.

Model dung tool "powershell" thay cho bash. Lenh "!" trong editor van can
bash (Git for Windows); neu may khong co thi lenh "!" bao loi, phan con lai
van chay binh thuong.

Go bo: xoa thu muc PiPortable.

Luu y
  - home\.pi\agent\auth.json chua token dang plaintext. Ai co thu muc nay la
    dung duoc tai khoan cua ban.
  - Dang nhap OAuth mo trinh duyet mac dinh; cookie nam trong profile trinh
    duyet, ngoai thu muc portable.
  - Windows van tu ghi dau vet thuc thi (Prefetch, Amcache, ShimCache,
    BAM, SRUM, UserAssist neu mo bang Explorer, Zone.Identifier tren file
    zip tai ve). Day la hanh vi cua he dieu hanh, launcher khong chan duoc.
