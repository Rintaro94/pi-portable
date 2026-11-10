package main

// `pi update`: replace this portable folder with the latest signed release,
// keeping the user's login, sessions and settings.

import (
	"archive/tar"
	"archive/zip"
	"bufio"
	"compress/gzip"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const repo = "Rintaro94/pi-portable"

// Release signing key (ed25519, base64 raw 32 bytes). SHA256SUMS of every
// release is signed with the matching private key in CI. Overridable at
// build time with -X main.releasePublicKey=... for tests only.
var releasePublicKey = "y/Cbb9GFblXZzSRzhW8EKgijueqICvD/vtq+7VZ1two="

// Paths owned by the package. They are swapped as a whole on update; the old
// copy goes to backup/. Everything else under home/ belongs to the user.
func managedPaths() []string {
	return []string{
		"pi" + exeSuffix, updateScript, "VERSION", "README.txt", "app",
		"home/.pi/agent/git", "home/.pi/agent/npm", "home/.pi/agent/bin",
	}
}

// settings.json keys that describe the package rather than the user.
var managedSettings = []string{"packages", "defaultTools", "lastChangelogVersion", "extensionChangelogVersions"}

var errPiRunning = errors.New("pi is running from this folder; close it and run the update again")

func runUpdate(root string, args []string) int {
	checkOnly, assumeYes := false, false
	for _, a := range args {
		switch a {
		case "--check":
			checkOnly = true
		case "-y", "--yes":
			assumeYes = true
		case "-h", "--help":
			fmt.Println("Usage: pi update [--check] [--yes]\n\n  --check  only report whether a newer release exists\n  --yes    do not ask for confirmation")
			return 0
		default:
			fmt.Fprintf(os.Stderr, "pi-portable: unknown option %q (see pi update --help)\n", a)
			return 2
		}
	}
	if err := update(root, checkOnly, assumeYes); err != nil {
		fmt.Fprintf(os.Stderr, "\nUpdate failed: %v\nNothing was changed unless stated above.\n", err)
		return 1
	}
	return 0
}

func update(root string, checkOnly, assumeYes bool) error {
	current := readVersion(root)
	src := newSource()
	fmt.Printf("Pi Portable %s (%s). Checking %s ...\n", current, target(), src.describe())

	latest, err := src.latest()
	if err != nil {
		return fmt.Errorf("cannot get the latest release: %w", err)
	}
	if _, ok := parseVersion(latest); !ok {
		return fmt.Errorf("latest release has an unexpected tag %q", latest)
	}
	if compareVersions(latest, current) <= 0 {
		fmt.Printf("Already up to date (latest release is %s).\n", latest)
		return nil
	}
	fmt.Printf("New version available: %s -> %s\nChanges: %s\n", current, latest, src.notesURL(latest))
	if checkOnly {
		return nil
	}
	if !assumeYes && !confirm("Download and install it now? [y/N] ") {
		fmt.Println("Cancelled.")
		return nil
	}

	unlock, err := lockExclusive(root)
	if err != nil {
		return err
	}
	defer unlock()

	work := filepath.Join(root, "tmp", "update")
	if err := os.RemoveAll(work); err != nil {
		return err
	}
	if err := os.MkdirAll(work, 0o755); err != nil {
		return err
	}
	defer os.RemoveAll(work)

	pkg := packageFile()
	sums := filepath.Join(work, "SHA256SUMS")
	sig := filepath.Join(work, "SHA256SUMS.sig")
	archive := filepath.Join(work, pkg)
	for _, f := range []struct{ name, dst string }{{"SHA256SUMS", sums}, {"SHA256SUMS.sig", sig}, {pkg, archive}} {
		if err := download(src.fileURL(latest, f.name), f.dst); err != nil {
			return fmt.Errorf("download %s: %w", f.name, err)
		}
	}

	if err := verifySignature(sums, sig); err != nil {
		return err
	}
	if err := verifyChecksum(sums, archive, pkg); err != nil {
		return err
	}
	fmt.Println("Signature and checksum OK.")

	extracted := filepath.Join(work, "new")
	if err := extract(archive, extracted); err != nil {
		return fmt.Errorf("extract: %w", err)
	}
	newRoot := filepath.Join(extracted, "PiPortable")
	if v := readVersion(newRoot); v != latest {
		return fmt.Errorf("package reports version %q, expected %q", v, latest)
	}

	backup, err := backupDir(root, current)
	if err != nil {
		return err
	}
	if err := install(root, newRoot, backup); err != nil {
		return err
	}
	pruneBackups(root, backup)

	fmt.Printf("\nUpdated to %s.\nPrevious files: %s\nDelete that folder once the new version works.\n", latest, backup)
	return nil
}

// install swaps the managed paths and merges settings. Any failure undoes
// the moves made so far.
func install(root, newRoot, backup string) (err error) {
	type move struct{ from, to string }
	var done []move
	mv := func(from, to string) error {
		if err := os.MkdirAll(filepath.Dir(to), 0o755); err != nil {
			return err
		}
		if err := os.Rename(from, to); err != nil {
			return err
		}
		done = append(done, move{from, to})
		return nil
	}
	defer func() {
		if err == nil {
			return
		}
		for i := len(done) - 1; i >= 0; i-- {
			if rerr := os.Rename(done[i].to, done[i].from); rerr != nil {
				fmt.Fprintf(os.Stderr, "rollback: cannot move %s back to %s: %v\n", done[i].to, done[i].from, rerr)
			}
		}
		err = fmt.Errorf("%w (changes rolled back)", err)
	}()

	for _, p := range managedPaths() {
		rel := filepath.FromSlash(p)
		cur, old, nw := filepath.Join(root, rel), filepath.Join(backup, rel), filepath.Join(newRoot, rel)
		if exists(cur) {
			if err := mv(cur, old); err != nil {
				if os.IsPermission(err) || isInUse(err) {
					return errPiRunning
				}
				return fmt.Errorf("move %s to backup: %w", p, err)
			}
		}
		if exists(nw) {
			if err := mv(nw, cur); err != nil {
				return fmt.Errorf("install %s: %w", p, err)
			}
		}
	}

	agent := filepath.Join("home", ".pi", "agent")
	userSettings := filepath.Join(root, agent, "settings.json")
	if exists(userSettings) {
		if err := copyFile(userSettings, filepath.Join(backup, agent, "settings.json")); err != nil {
			return err
		}
	}
	if err := mergeSettings(userSettings, filepath.Join(newRoot, agent, "settings.json")); err != nil {
		return fmt.Errorf("merge settings.json: %w", err)
	}

	// New default config files (e.g. for a new plugin) are added; existing
	// user files are left alone.
	entries, _ := os.ReadDir(filepath.Join(newRoot, agent))
	for _, e := range entries {
		if e.IsDir() || e.Name() == "settings.json" {
			continue
		}
		dst := filepath.Join(root, agent, e.Name())
		if !exists(dst) {
			if err := copyFile(filepath.Join(newRoot, agent, e.Name()), dst); err != nil {
				return err
			}
		}
	}
	return nil
}

// mergeSettings keeps the user's settings and takes the package-owned keys
// from the new release.
func mergeSettings(userPath, newPath string) error {
	newData, err := os.ReadFile(newPath)
	if err != nil {
		return err
	}
	var next map[string]json.RawMessage
	if err := json.Unmarshal(newData, &next); err != nil {
		return err
	}
	merged := map[string]json.RawMessage{}
	if data, err := os.ReadFile(userPath); err == nil {
		if err := json.Unmarshal(data, &merged); err != nil {
			return fmt.Errorf("%s is not valid JSON: %w", userPath, err)
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	for _, k := range managedSettings {
		if v, ok := next[k]; ok {
			merged[k] = v
		} else {
			delete(merged, k)
		}
	}
	out, err := json.MarshalIndent(merged, "", "  ")
	if err != nil {
		return err
	}
	tmp := userPath + ".tmp"
	if err := os.WriteFile(tmp, append(out, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, userPath)
}

// ---- release source ----

type source struct {
	base   string // custom source: <base>/latest.txt and <base>/<tag>/<file>
	github bool
}

// PI_PORTABLE_UPDATE_SOURCE points the updater at another server, for
// testing. Releases from it must still be signed with releasePublicKey.
func newSource() source {
	if b := strings.TrimRight(os.Getenv("PI_PORTABLE_UPDATE_SOURCE"), "/"); b != "" {
		return source{base: b}
	}
	return source{github: true}
}

func (s source) describe() string {
	if s.github {
		return "github.com/" + repo
	}
	return s.base
}

func (s source) latest() (string, error) {
	if !s.github {
		body, err := httpGet(s.base + "/latest.txt")
		return strings.TrimSpace(string(body)), err
	}
	body, err := httpGet("https://api.github.com/repos/" + repo + "/releases/latest")
	if err != nil {
		return "", err
	}
	var rel struct {
		TagName string `json:"tag_name"`
	}
	if err := json.Unmarshal(body, &rel); err != nil {
		return "", err
	}
	return rel.TagName, nil
}

func (s source) fileURL(tag, name string) string {
	if s.github {
		return "https://github.com/" + repo + "/releases/download/" + tag + "/" + name
	}
	return s.base + "/" + tag + "/" + name
}

func (s source) notesURL(tag string) string {
	if s.github {
		return "https://github.com/" + repo + "/releases/tag/" + tag
	}
	return s.base + "/" + tag + "/"
}

var client = &http.Client{Timeout: 30 * time.Minute}

func request(url string) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "pi-portable-updater")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		if resp.StatusCode == http.StatusNotFound {
			return nil, fmt.Errorf("%s: not found (release may not be signed or not built for %s)", url, target())
		}
		return nil, fmt.Errorf("%s: HTTP %d", url, resp.StatusCode)
	}
	return resp, nil
}

func httpGet(url string) ([]byte, error) {
	resp, err := request(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return io.ReadAll(io.LimitReader(resp.Body, 1<<20))
}

func download(url, dst string) error {
	resp, err := request(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	f, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer f.Close()
	name := path.Base(dst)
	var r io.Reader = resp.Body
	if resp.ContentLength > 1<<20 {
		r = &progress{r: resp.Body, total: resp.ContentLength, name: name}
	}
	if _, err := io.Copy(f, r); err != nil {
		return err
	}
	if resp.ContentLength > 1<<20 {
		fmt.Println()
	}
	return f.Close()
}

type progress struct {
	r          io.Reader
	total, n   int64
	name       string
	lastReport int64
}

func (p *progress) Read(b []byte) (int, error) {
	n, err := p.r.Read(b)
	p.n += int64(n)
	if pct := p.n * 100 / p.total; pct/10 != p.lastReport/10 || err == io.EOF {
		p.lastReport = pct
		fmt.Printf("\rDownloading %s: %3d%% of %d MB", p.name, pct, p.total>>20)
	}
	return n, err
}

// ---- verification ----

func verifySignature(sumsPath, sigPath string) error {
	pub, err := base64.StdEncoding.DecodeString(releasePublicKey)
	if err != nil || len(pub) != ed25519.PublicKeySize {
		return errors.New("launcher has an invalid release public key")
	}
	sums, err := os.ReadFile(sumsPath)
	if err != nil {
		return err
	}
	sig, err := os.ReadFile(sigPath)
	if err != nil {
		return err
	}
	if !ed25519.Verify(ed25519.PublicKey(pub), sums, sig) {
		return errors.New("SHA256SUMS signature is not valid; refusing to install this release")
	}
	return nil
}

func verifyChecksum(sumsPath, file, name string) error {
	data, err := os.ReadFile(sumsPath)
	if err != nil {
		return err
	}
	want := ""
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && strings.TrimPrefix(fields[1], "*") == name {
			want = strings.ToLower(fields[0])
		}
	}
	if want == "" {
		return fmt.Errorf("%s is not listed in SHA256SUMS", name)
	}
	f, err := os.Open(file)
	if err != nil {
		return err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}
	if got := hex.EncodeToString(h.Sum(nil)); got != want {
		return fmt.Errorf("checksum mismatch for %s", name)
	}
	return nil
}

// ---- archives ----

// safeJoin maps an archive entry to a path under dest. Only entries inside
// the top-level PiPortable/ folder are accepted.
func safeJoin(dest, name string) (string, error) {
	clean := path.Clean(strings.ReplaceAll(name, "\\", "/"))
	if clean != "PiPortable" && !strings.HasPrefix(clean, "PiPortable/") {
		return "", fmt.Errorf("unexpected entry %q", name)
	}
	return filepath.Join(dest, filepath.FromSlash(clean)), nil
}

func extract(archive, dest string) error {
	if strings.HasSuffix(archive, ".zip") {
		return extractZip(archive, dest)
	}
	return extractTarGz(archive, dest)
}

func extractZip(archive, dest string) error {
	z, err := zip.OpenReader(archive)
	if err != nil {
		return err
	}
	defer z.Close()
	for _, f := range z.File {
		p, err := safeJoin(dest, f.Name)
		if err != nil {
			return err
		}
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(p, 0o755); err != nil {
				return err
			}
			continue
		}
		if !f.Mode().IsRegular() {
			return fmt.Errorf("unsupported entry type: %s", f.Name)
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		err = writeFile(p, rc, 0o644)
		rc.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

func extractTarGz(archive, dest string) error {
	f, err := os.Open(archive)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		if hdr.Typeflag == tar.TypeXGlobalHeader {
			continue
		}
		p, err := safeJoin(dest, hdr.Name)
		if err != nil {
			return err
		}
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(p, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := writeFile(p, tr, os.FileMode(hdr.Mode)&0o755); err != nil {
				return err
			}
		default:
			return fmt.Errorf("unsupported entry type in %s", hdr.Name)
		}
	}
}

func writeFile(p string, r io.Reader, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode|0o600)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, r); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// ---- versions ----

func readVersion(root string) string {
	data, err := os.ReadFile(filepath.Join(root, "VERSION"))
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(data))
}

// parseVersion reads vMAJOR.MINOR.PATCH.YYMMDD, where MAJOR.MINOR.PATCH is
// the pi version and YYMMDD the build date. It also reads the older
// vMAJOR.MINOR.PATCH[-N] tags (v1.5.0, v1.5.0-2); a missing N counts as 1.
// N is always below 260101, so v1.5.0 < v1.5.0-2 < v1.5.0.261110 < v1.6.0.261101.
func parseVersion(v string) ([4]int, bool) {
	var out [4]int
	s, ok := strings.CutPrefix(v, "v")
	if !ok {
		return out, false
	}
	core, build, hasBuild := strings.Cut(s, "-")
	parts := strings.Split(core, ".")
	dated := len(parts) == 4
	if (len(parts) != 3 && !dated) || (dated && hasBuild) {
		return out, false
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return out, false
		}
		// No leading zeros or signs in MAJOR.MINOR.PATCH. YYMMDD is checked below.
		if i < 3 && p != strconv.Itoa(n) {
			return out, false
		}
		out[i] = n
	}
	switch {
	case dated:
		d := parts[3]
		month, day := out[3]/100%100, out[3]%100
		if len(d) != 6 || strings.Trim(d, "0123456789") != "" || month < 1 || month > 12 || day < 1 || day > 31 {
			return out, false
		}
	case hasBuild:
		n, err := strconv.Atoi(build)
		if err != nil || n < 1 || n >= 260101 {
			return out, false
		}
		out[3] = n
	default:
		out[3] = 1
	}
	return out, true
}

// compareVersions returns -1, 0 or 1. Unparseable versions (dev builds)
// sort before every release.
func compareVersions(a, b string) int {
	va, okA := parseVersion(a)
	vb, okB := parseVersion(b)
	switch {
	case !okA && !okB:
		return 0
	case !okA:
		return -1
	case !okB:
		return 1
	}
	for i := range va {
		if va[i] != vb[i] {
			if va[i] < vb[i] {
				return -1
			}
			return 1
		}
	}
	return 0
}

// ---- helpers ----

func target() string {
	arch := runtime.GOARCH
	if arch == "amd64" {
		arch = "x64"
	}
	return runtime.GOOS + "-" + arch
}

func packageFile() string {
	if runtime.GOOS == "windows" {
		return "PiPortable-" + target() + ".zip"
	}
	return "PiPortable-" + target() + ".tar.gz"
}

func backupDir(root, version string) (string, error) {
	dir := filepath.Join(root, "backup", sanitize(version))
	if exists(dir) {
		dir += "-" + time.Now().Format("20060102-150405")
	}
	return dir, os.MkdirAll(dir, 0o755)
}

// pruneBackups keeps only the backup just made, so repeated updates do not
// fill the disk with old copies of app/ (~150 MB each).
func pruneBackups(root, keep string) {
	entries, err := os.ReadDir(filepath.Join(root, "backup"))
	if err != nil {
		return
	}
	for _, e := range entries {
		p := filepath.Join(root, "backup", e.Name())
		if e.IsDir() && p != keep {
			os.RemoveAll(p)
		}
	}
}

func sanitize(s string) string {
	return strings.Map(func(r rune) rune {
		if r == '.' || r == '-' || r == '_' || r >= '0' && r <= '9' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' {
			return r
		}
		return '_'
	}, s)
}

func confirm(prompt string) bool {
	fmt.Print(prompt)
	line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes":
		return true
	}
	return false
}

func exists(p string) bool {
	_, err := os.Lstat(p)
	return err == nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	return writeFile(dst, in, 0o644)
}
