#!/usr/bin/env python3
"""Keep the pinned build inputs in sources.lock up to date.

    python3 scripts/upstream.py update [--dry-run] [--summary FILE] [--now ISO-8601]
    python3 scripts/upstream.py verify-tag TAG

update
    Checks every upstream source, then updates sources.lock,
    config/settings.json, the version mentions in the READMEs and CHANGELOG.md.
    - pi: the latest GitHub release, taken as soon as it is out. pi-ai must
      exist on npm with the same version; pi-telemetry and partial-json follow
      the versions pi-ai pins.
    - Plugins on npm, pi-themes (git main), fd and ripgrep (GitHub releases):
      the newest version that has been public for at least 3 days.
    The release tag is v<pi version>.<YYMMDD of today, UTC>.
    Writes state=changed|none|skipped and tag=... to $GITHUB_OUTPUT.

verify-tag
    Exits non-zero unless TAG looks like vX.Y.Z.YYMMDD, X.Y.Z is the pi
    version in sources.lock, and TAG is newer than the latest release.

release-plan
    Reads the top section of CHANGELOG.md. If its tag is not released yet,
    checks it like verify-tag and writes tag=<tag> to $GITHUB_OUTPUT;
    otherwise writes tag= (nothing to release).

Set GITHUB_TOKEN to avoid the GitHub API rate limit.
"""
import argparse
import datetime as dt
import hashlib
import json
import os
import pathlib
import re
import subprocess
import sys
import urllib.error
import urllib.request

REPO = pathlib.Path(__file__).resolve().parent.parent
LOCK = REPO / "sources.lock"
SETTINGS = REPO / "config" / "settings.json"
CHANGELOG = REPO / "CHANGELOG.md"
PORTABLE_REPO = os.environ.get("GITHUB_REPOSITORY", "Rintaro94/pi-portable")

DELAY = dt.timedelta(days=3)
TARGETS = ("windows-x64", "linux-x64", "linux-arm64", "darwin-x64", "darwin-arm64")
AUTO_MARKER = "<!-- upstream:auto -->"

PI_REPO = "earendil-works/pi"
THEMES_REPO = "sting8k/pi-themes"
# lock name -> (npm package, source key in extensionChangelogVersions or None)
PLUGINS = {
    "pi-droid-styling.tgz": ("@sting8k/pi-droid-styling", "./git/github.com/sting8k/pi-droid-styling"),
    "pi-vcc.tgz": ("@sting8k/pi-vcc", "./git/github.com/sting8k/pi-vcc"),
    "pi-cliproxyapi-provider.tgz": ("@router-for-me/pi-cliproxyapi-provider", None),
}
PI_AI = ("pi-ai.tgz", "@earendil-works/pi-ai")
PI_AI_DEPS = {"pi-telemetry.tgz": "@earendil-works/pi-telemetry", "partial-json.tgz": "partial-json"}
NPM = {**{k: v[0] for k, v in PLUGINS.items()}, PI_AI[0]: PI_AI[1], **PI_AI_DEPS}
# lock name prefix -> (GitHub repo, display name)
TOOLS = {"fd": ("sharkdp/fd", "fd"), "rg": ("BurntSushi/ripgrep", "ripgrep")}
# peer dependencies whose version equals the pi version
PI_PEERS = ("@earendil-works/pi-coding-agent", "@earendil-works/pi-tui")

ORDER = ["pi", "pi-droid-styling", "pi-vcc", "pi-cliproxyapi-provider", "pi-themes",
         "pi-ai", "pi-telemetry", "partial-json", "fd", "ripgrep"]

DOWNLOADS = """### Downloads

| File | Platform | Shell tool for the model |
|---|---|---|
| `PiPortable-windows-x64.zip` | Windows x64 | `powershell` |
| `PiPortable-linux-x64.tar.gz` | Linux x86_64 (glibc) | `bash` |
| `PiPortable-linux-arm64.tar.gz` | Linux aarch64 (glibc) | `bash` |
| `PiPortable-darwin-arm64.tar.gz` | macOS, Apple Silicon | `bash` |
| `PiPortable-darwin-x64.tar.gz` | macOS, Intel | `bash` |

Check downloads against `SHA256SUMS`. `SHA256SUMS.sig` is its ed25519 signature."""


# ---- versions ----

def parse_tag(tag):
    """Mirror of parseVersion in launcher/update.go."""
    m = re.fullmatch(r"v(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(?:\.(\d{6})|-([1-9]\d*))?", tag)
    if not m:
        return None
    head = tuple(int(x) for x in m.group(1, 2, 3))
    if m.group(4):
        n = int(m.group(4))
        if not (1 <= n // 100 % 100 <= 12 and 1 <= n % 100 <= 31):
            return None
    elif m.group(5):
        n = int(m.group(5))
        if n >= 260101:
            return None
    else:
        n = 1
    return head + (n,)


def compare_tags(a, b):
    pa, pb = parse_tag(a), parse_tag(b)
    if pa is None or pb is None:
        return (pa is not None) - (pb is not None)
    return (pa > pb) - (pa < pb)


def semver(v):
    """(major, minor, patch, is_prerelease) or None."""
    m = re.fullmatch(r"v?(\d+)\.(\d+)\.(\d+)(-[0-9A-Za-z.-]+)?(\+[0-9A-Za-z.-]+)?", v or "")
    if not m:
        return None
    return int(m.group(1)), int(m.group(2)), int(m.group(3)), bool(m.group(4))


def newer(a, b):
    sa, sb = semver(a), semver(b)
    return sa is not None and (sb is None or sa[:3] > sb[:3])


def satisfies(version, rng):
    """Enough of npm semver ranges for peerDependencies. Raises ValueError."""
    v = semver(version)[:3]
    for alt in rng.split("||"):
        alt = alt.strip()
        if alt in ("", "*", "x", "latest"):
            return True
        ok = True
        for c in alt.split():
            m = re.fullmatch(r"(>=|<=|>|<|=|\^|~)?v?(\d+|x|\*)(?:\.(\d+|x|\*))?(?:\.(\d+|x|\*))?", c)
            if not m:
                raise ValueError(f"unsupported range {rng!r}")
            op = m.group(1) or "="
            parts = [None if p in (None, "x", "*") else int(p) for p in m.group(2, 3, 4)]
            if parts[0] is None:
                continue
            lo = tuple(p or 0 for p in parts)
            given = len([p for p in parts if p is not None])
            if op == "^":
                a, b, c3 = lo
                hi = (a + 1, 0, 0) if a or given == 1 else (0, b + 1, 0) if b or given == 2 else (0, 0, c3 + 1)
                ok &= lo <= v < hi
            elif op == "~":
                hi = (lo[0] + 1, 0, 0) if given == 1 else (lo[0], lo[1] + 1, 0)
                ok &= lo <= v < hi
            elif op == "=":
                hi = (lo[0] + 1, 0, 0) if given == 1 else (lo[0], lo[1] + 1, 0) if given == 2 else (lo[0], lo[1], lo[2] + 1)
                ok &= lo <= v < hi
            else:
                ok &= {">=": v >= lo, "<=": v <= lo, ">": v > lo, "<": v < lo}[op]
        if ok:
            return True
    return False


# ---- network ----

def _request(url):
    req = urllib.request.Request(url, headers={"User-Agent": "pi-portable-upstream"})
    token = os.environ.get("GITHUB_TOKEN")
    if token and url.startswith("https://api.github.com/"):
        req.add_header("Authorization", f"Bearer {token}")
        req.add_header("Accept", "application/vnd.github+json")
    return urllib.request.urlopen(req, timeout=300)


def get_json(url):
    with _request(url) as r:
        return json.load(r)


def gh(path):
    return get_json("https://api.github.com/" + path)


def npm_meta(pkg):
    return get_json("https://registry.npmjs.org/" + pkg)


def sha256_url(url):
    h = hashlib.sha256()
    with _request(url) as r:
        for chunk in iter(lambda: r.read(1 << 20), b""):
            h.update(chunk)
    return "sha256:" + h.hexdigest()


def asset_sum(asset):
    d = asset.get("digest") or ""
    return d if re.fullmatch(r"sha256:[0-9a-f]{64}", d) else sha256_url(asset["browser_download_url"])


def npm_entry(info):
    dist = info["dist"]
    if not dist.get("integrity", "").startswith("sha512-"):
        raise RuntimeError(f"{info['name']}@{info['version']} has no sha512 integrity")
    return dist["tarball"], dist["integrity"]


def when(s):
    return dt.datetime.fromisoformat(s.replace("Z", "+00:00"))


# ---- sources.lock ----

class Lock:
    def __init__(self, text):
        self.lines = text.splitlines()
        self.idx = {}
        for i, line in enumerate(self.lines):
            s = line.strip()
            if s and not s.startswith("#"):
                self.idx[s.split()[0]] = i

    def url(self, name):
        return self.lines[self.idx[name]].split()[1]

    def set(self, name, url, checksum):
        self.lines[self.idx[name]] = f"{name} {url} {checksum}"

    def text(self):
        return "\n".join(self.lines) + "\n"


def display(lockname):
    return lockname.removesuffix(".tgz")


def versions(lock):
    """Display name -> version (pi-themes: commit sha). Missing entries are skipped."""
    out = {}
    try:
        out["pi"] = re.search(r"/download/v([^/]+)/", lock.url("pi@linux-x64")).group(1)
    except (KeyError, AttributeError):
        pass
    for name, pkg in NPM.items():
        base = re.escape(pkg.rsplit("/", 1)[-1])
        try:
            out[display(name)] = re.search(rf"/-/{base}-(.+)\.tgz$", lock.url(name)).group(1)
        except (KeyError, AttributeError):
            pass
    try:
        out["pi-themes"] = lock.url("pi-themes.tgz").rsplit("/", 1)[1]
    except KeyError:
        pass
    for prefix, (_, disp) in TOOLS.items():
        try:
            out[disp] = re.search(r"/download/([^/]+)/", lock.url(f"{prefix}@linux-x64")).group(1).lstrip("v")
        except (KeyError, AttributeError):
            pass
    return out


# ---- checks ----

class Report:
    def __init__(self):
        self.waiting, self.warnings = [], []

    def wait(self, what, published):
        self.waiting.append(f"{what}: published {published:%Y-%m-%d %H:%M} UTC, "
                            f"eligible from {published + DELAY:%Y-%m-%d %H:%M} UTC")


def update_pi(lock, cur, rep):
    rel = gh(f"repos/{PI_REPO}/releases/latest")
    ver = rel["tag_name"].removeprefix("v")
    if not newer(ver, cur) or semver(ver)[3]:
        return cur
    ai_meta = npm_meta(PI_AI[1])
    if ver not in ai_meta["versions"]:
        rep.warnings.append(f"pi {ver} is out, but {PI_AI[1]} {ver} is not on npm yet. pi stays at {cur}.")
        return cur
    assets = {a["name"]: a for a in rel["assets"]}
    entries = {}
    for t in TARGETS:
        fname = lock.url(f"pi@{t}").rsplit("/", 1)[1]
        if fname not in assets:
            rep.warnings.append(f"pi {ver} has no {fname}. pi stays at {cur}.")
            return cur
        entries[f"pi@{t}"] = (assets[fname]["browser_download_url"], asset_sum(assets[fname]))

    ai = ai_meta["versions"][ver]
    entries[PI_AI[0]] = npm_entry(ai)
    deps = ai.get("dependencies") or {}
    for lockname, pkg in PI_AI_DEPS.items():
        want = deps.get(pkg)
        if want is None:
            rep.warnings.append(f"pi-ai {ver} no longer depends on {pkg}; it is still bundled. See MAINTAINING.md 4.1.")
            continue
        if not re.fullmatch(r"\d+\.\d+\.\d+", want):
            rep.warnings.append(f"pi-ai {ver} wants {pkg} {want!r}, not an exact version; kept as is. Pin it by hand.")
            continue
        meta = npm_meta(pkg)
        if want not in meta["versions"]:
            rep.warnings.append(f"{pkg} {want} (needed by pi-ai {ver}) is not on npm; kept as is.")
            continue
        entries[lockname] = npm_entry(meta["versions"][want])
    old_ai = ai_meta["versions"].get(versions(lock).get("pi-ai"), {})
    added = sorted(set(deps) - set(old_ai.get("dependencies") or {}))
    if added:
        rep.warnings.append(f"pi-ai {ver} adds dependencies: {', '.join(added)}. If "
                            "dist/api/openai-codex-responses.js imports one of them, bundle it (MAINTAINING.md 4.1).")
    for name, (url, checksum) in entries.items():
        lock.set(name, url, checksum)
    return ver


def update_plugin(lock, lockname, cur, pi_ver, now, rep):
    pkg = PLUGINS[lockname][0]
    meta = npm_meta(pkg)
    latest = semver(meta["dist-tags"]["latest"])
    best = cur
    young = []
    for v, info in meta["versions"].items():
        sv = semver(v)
        if sv is None or sv[3] or info.get("deprecated") or (latest and sv[:3] > latest[:3]):
            continue
        if not newer(v, cur):
            continue
        t = when(meta["time"][v])
        if now - t < DELAY:
            young.append((sv, v, t))
        elif newer(v, best):
            best = v
    for sv, v, t in sorted(young):
        if newer(v, best):
            rep.wait(f"{display(lockname)} {v}", t)
    if best != cur:
        lock.set(lockname, *npm_entry(meta["versions"][best]))
    peers = meta["versions"].get(best, {}).get("peerDependencies") or {}
    for p in PI_PEERS:
        if p in peers:
            try:
                if not satisfies(pi_ver, peers[p]):
                    rep.warnings.append(f"{display(lockname)} {best} declares {p} {peers[p]!r}, which excludes pi {pi_ver}.")
            except ValueError as e:
                rep.warnings.append(f"{display(lockname)} {best}: cannot check {p} {peers[p]!r} ({e}).")
    return best


def update_themes(lock, cur, now, rep):
    acts = gh(f"repos/{THEMES_REPO}/activity?ref=refs/heads/main&per_page=30")
    acts.sort(key=lambda a: a["timestamp"], reverse=True)
    chosen = cur
    for a in acts:
        if a["after"] == cur:
            break  # what we have is at least as new as anything eligible
        t = when(a["timestamp"])
        if now - t >= DELAY:
            chosen = a["after"]
            break
        if a is acts[0]:
            rep.wait(f"pi-themes {a['after'][:7]}", t)
    if chosen != cur:
        url = f"https://codeload.github.com/{THEMES_REPO}/tar.gz/{chosen}"
        lock.set("pi-themes.tgz", url, sha256_url(url))
    return chosen


def update_tool(lock, prefix, cur, now, rep):
    repo, disp = TOOLS[prefix]
    best = None
    for r in gh(f"repos/{repo}/releases?per_page=20"):
        v = r["tag_name"].removeprefix("v")
        sv = semver(v)
        if r["draft"] or r["prerelease"] or sv is None or sv[3] or not newer(v, cur):
            continue
        t = when(r["published_at"])
        if now - t < DELAY:
            if best is None or newer(v, best[0]):
                rep.wait(f"{disp} {v}", t)
            continue
        if best is None or newer(v, best[0]):
            best = (v, r)
    if best is None:
        return cur
    v, r = best
    assets = {a["name"]: a for a in r["assets"]}
    entries = {}
    for t in TARGETS:
        name = f"{prefix}@{t}"
        fname = lock.url(name).rsplit("/", 1)[1].replace(cur, v)
        if fname not in assets:
            rep.warnings.append(f"{disp} {v} has no {fname}. {disp} stays at {cur}.")
            return cur
        entries[name] = (assets[fname]["browser_download_url"], asset_sum(assets[fname]))
    for name, (url, checksum) in entries.items():
        lock.set(name, url, checksum)
    return v


# ---- files ----

def update_settings(text, new):
    s = json.loads(text)
    s["lastChangelogVersion"] = new["pi"]
    ecv = s.get("extensionChangelogVersions") or {}
    for lockname, (_, source) in PLUGINS.items():
        key = json.dumps({"source": source, "scope": "user"}, separators=(",", ":"))
        if source and key in ecv:
            ecv[key] = new[display(lockname)]
    return json.dumps(s, indent=2) + "\n"


def update_readmes(new):
    """Return {path: new text} for files whose version mentions changed."""
    out = {}
    rules = {
        REPO / "README.md": [(r"(\[pi\]\(https://github\.com/earendil-works/pi\) )\d+\.\d+\.\d+", new["pi"])]
        + [(rf"({re.escape(p)} )\d+\.\d+\.\d+", new[p]) for p in ("pi-droid-styling", "pi-vcc", "pi-cliproxyapi-provider")],
        REPO / "package" / "README.windows.txt": [(r"(\(pi )\d+\.\d+\.\d+", new["pi"])],
        REPO / "package" / "README.unix.txt": [(r"(\(pi )\d+\.\d+\.\d+", new["pi"])],
    }
    for path, subs in rules.items():
        text = path.read_text(encoding="utf-8")
        new_text = text
        for pattern, value in subs:
            new_text = re.sub(pattern, lambda m, v=value: m.group(1) + v, new_text)
        if new_text != text:
            out[path] = new_text
    return out


def change_lines(old, new, lock):
    lines = []
    for c in ORDER:
        a, b = old.get(c), new.get(c)
        if a == b or b is None:
            continue
        if c == "pi-themes":
            text = f"pi-themes {a[:7] if a else '(new)'} → {b[:7]}"
            link = f"https://github.com/{THEMES_REPO}/compare/{a}...{b}" if a else None
            label = "changes"
        else:
            text = f"{c} {a} → {b}" if a else f"{c} {b} (new)"
            if c == "pi":
                link, label = f"https://github.com/{PI_REPO}/releases/tag/v{b}", "release notes"
            elif c in ("fd", "ripgrep"):
                prefix = next(k for k, v in TOOLS.items() if v[1] == c)
                tag = re.search(r"/download/([^/]+)/", lock.url(f"{prefix}@linux-x64")).group(1)
                link, label = f"https://github.com/{TOOLS[prefix][0]}/releases/tag/{tag}", "release notes"
            else:
                link, label = f"https://www.npmjs.com/package/{NPM[c + '.tgz']}/v/{b}", "npm"
            if c == "pi-ai":
                text = "pi-ai (copy used by pi-cliproxyapi-provider) " + text.removeprefix("pi-ai ")
            elif c in ("pi-telemetry", "partial-json"):
                text += " (needed by pi-ai)"
        lines.append(f"- {text}" + (f" ([{label}]({link}))." if link else "."))
    return lines


def changelog_section(tag, date, lines):
    return "\n".join([
        f"## {tag} - {date:%Y-%m-%d}",
        "",
        AUTO_MARKER,
        "Updates the bundled components. Existing installs (v1.5.0.261110 or later) can run `pi update` "
        "(Windows: `update.bat`, Linux and macOS: `update.sh`).",
        "",
        DOWNLOADS,
        "",
        "### Changed",
        "",
        *lines,
    ]) + "\n"


def insert_section(text, section, released):
    """Put section above the first release. An unreleased automatic section at
    the top is replaced, so two unmerged-then-merged updates make one release."""
    m = re.search(r"^## (\S+).*$", text, re.M)
    if not m:
        return text.rstrip("\n") + "\n\n" + section
    start = m.start()
    nxt = re.search(r"^## ", text[m.end():], re.M)
    end = m.end() + nxt.start() if nxt else len(text)
    if m.group(1) not in released and AUTO_MARKER in text[start:end]:
        return text[:start] + section + "\n" + text[end:]
    return text[:start] + section + "\n" + text[start:]


def releases():
    try:
        return {r["tag_name"] for r in gh(f"repos/{PORTABLE_REPO}/releases?per_page=100")}
    except urllib.error.HTTPError as e:
        if e.code == 404:
            return set()
        raise


def latest_release():
    try:
        return gh(f"repos/{PORTABLE_REPO}/releases/latest")["tag_name"]
    except urllib.error.HTTPError as e:
        if e.code == 404:
            return None
        raise


def output(**kv):
    path = os.environ.get("GITHUB_OUTPUT")
    if path:
        with open(path, "a", encoding="utf-8") as f:
            for k, v in kv.items():
                f.write(f"{k}={v}\n")


# ---- commands ----

def cmd_update(args):
    now = when(args.now) if args.now else dt.datetime.now(dt.timezone.utc)
    lock_text = LOCK.read_text(encoding="utf-8")
    settings_text = SETTINGS.read_text(encoding="utf-8")
    lock = Lock(lock_text)
    cur = versions(lock)
    rep = Report()

    pi_ver = update_pi(lock, cur["pi"], rep)
    for lockname in PLUGINS:
        update_plugin(lock, lockname, cur[display(lockname)], pi_ver, now, rep)
    update_themes(lock, cur["pi-themes"], now, rep)
    for prefix, (_, disp) in TOOLS.items():
        update_tool(lock, prefix, cur[disp], now, rep)

    new = versions(lock)
    new_settings = update_settings(settings_text, new)
    tag = f"v{new['pi']}.{now:%y%m%d}"
    released = releases()
    base_tag = latest_release()
    base = cur
    if base_tag:
        try:
            base = versions(Lock(subprocess.run(["git", "-C", str(REPO), "show", f"{base_tag}:sources.lock"],
                                                check=True, capture_output=True, text=True).stdout))
        except subprocess.CalledProcessError:
            pass
    lines = change_lines(base, new, lock)

    if lock.text() == lock_text and new_settings == settings_text:
        state = "none"
    elif tag in released:
        state = "skipped"
        rep.warnings.insert(0, f"{tag} is already released today; the update waits for tomorrow's run.")
    else:
        state = "changed"

    summary = [f"**State:** {state}" + (f", release tag `{tag}`" if state == "changed" else ""), ""]
    if state != "none":
        summary += [f"### Changes since {base_tag or 'the current lock'}", "", *(lines or ["- (none)"]), ""]
    if rep.waiting:
        summary += ["### Waiting (public for less than 3 days)", "", *[f"- {w}" for w in rep.waiting], ""]
    if rep.warnings:
        summary += ["### Check before merging", "", *[f"- {w}" for w in rep.warnings], ""]
    summary_text = "\n".join(summary)
    print(summary_text)
    if args.summary:
        pathlib.Path(args.summary).write_text(summary_text, encoding="utf-8")

    if state == "changed" and not args.dry_run:
        LOCK.write_text(lock.text(), encoding="utf-8")
        SETTINGS.write_text(new_settings, encoding="utf-8")
        for path, text in update_readmes(new).items():
            path.write_text(text, encoding="utf-8")
        CHANGELOG.write_text(insert_section(CHANGELOG.read_text(encoding="utf-8"),
                                            changelog_section(tag, now, lines), released), encoding="utf-8")
    output(state=state, tag=tag)


def cmd_verify_tag(args):
    tag = args.tag
    errors = verify_tag(tag)
    if errors:
        sys.exit("\n".join(errors))
    print(f"{tag}: ok")


def verify_tag(tag):
    errors = []
    if not re.fullmatch(r"v\d+\.\d+\.\d+\.\d{6}", tag) or parse_tag(tag) is None:
        errors.append(f"{tag}: release tags must look like vX.Y.Z.YYMMDD (pi version, then build date)")
    else:
        pi = versions(Lock(LOCK.read_text(encoding="utf-8"))).get("pi")
        if tag[1:].rsplit(".", 1)[0] != pi:
            errors.append(f"{tag}: X.Y.Z must be the pi version in sources.lock ({pi})")
        latest = latest_release()
        if latest and compare_tags(tag, latest) <= 0:
            errors.append(f"{tag} is not newer than the latest release {latest}")
    return errors


def cmd_release_plan(args):
    m = re.search(r"^## (\S+)", CHANGELOG.read_text(encoding="utf-8"), re.M)
    if not m:
        sys.exit("CHANGELOG.md has no release section")
    tag = m.group(1)
    if tag in releases():
        print(f"{tag} is already released; nothing to do")
        output(tag="")
        return
    errors = verify_tag(tag)
    if errors:
        sys.exit("\n".join(errors))
    print(f"{tag} is ready to release")
    output(tag=tag)


def main():
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    sub = ap.add_subparsers(dest="cmd", required=True)
    u = sub.add_parser("update")
    u.add_argument("--dry-run", action="store_true", help="report only, change no files")
    u.add_argument("--summary", help="also write the markdown summary to this file")
    u.add_argument("--now", help="pretend the current time is this ISO-8601 timestamp (tests)")
    v = sub.add_parser("verify-tag")
    v.add_argument("tag")
    sub.add_parser("release-plan")
    args = ap.parse_args()
    {"update": cmd_update, "verify-tag": cmd_verify_tag, "release-plan": cmd_release_plan}[args.cmd](args)


if __name__ == "__main__":
    main()
