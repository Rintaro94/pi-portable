#!/usr/bin/env python3
"""Print the CHANGELOG.md section for one release tag.

    python3 scripts/release-notes.py v1.5.0-2 > notes.md
    python3 scripts/release-notes.py --list        # tags that have a section

A section starts at a line "## <tag> - <date>" and ends at the next "## ".
Exits non-zero if the tag has no section, so a release cannot go out
without written notes.
"""
import pathlib
import re
import sys

CHANGELOG = pathlib.Path(__file__).resolve().parent.parent / "CHANGELOG.md"
HEADER = re.compile(r"^## (\S+)(?:\s+-\s+.*)?$")


def sections():
    current, lines = None, []
    for line in CHANGELOG.read_text(encoding="utf-8").splitlines():
        m = HEADER.match(line)
        if m:
            if current:
                yield current, lines
            current, lines = m.group(1), []
        elif current:
            lines.append(line)
    if current:
        yield current, lines


def main():
    if len(sys.argv) != 2:
        sys.exit(__doc__)
    if sys.argv[1] == "--list":
        for tag, _ in sections():
            print(tag)
        return
    for tag, lines in sections():
        if tag == sys.argv[1]:
            print("\n".join(lines).strip())
            return
    sys.exit(f"CHANGELOG.md has no section for {sys.argv[1]}")


if __name__ == "__main__":
    main()
