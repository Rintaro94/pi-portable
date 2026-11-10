#!/usr/bin/env python3
"""Patch pi-cliproxyapi-provider 1.4.25 login prompt.

The stock prompt pre-fills the base URL with the last saved value (or
http://127.0.0.1:8317), which shows the user's real proxy host on screen.
Show a neutral https://example.com sample and require the user to type it.

Usage: cliproxy_prompt.py <path-to>/extensions/index.ts
"""
import sys

path = sys.argv[1]
src = open(path, encoding="utf-8").read()

replacements = [
    (
        'callbacks.onProgress?.("Configure CLIProxyAPI. Preferred baseUrl form: host:port (e.g. http://127.0.0.1:8317).");',
        'callbacks.onProgress?.("Configure CLIProxyAPI. Enter the full base URL, e.g. https://example.com");',
    ),
    (
        """		message: `CLIProxyAPI base URL [${defaults.baseUrl}]:`,
		placeholder: defaults.baseUrl,
		allowEmpty: true,
	});
	const baseUrlInput = firstNonEmpty(baseUrlRaw, defaults.baseUrl)!;""",
        """		message: "CLIProxyAPI base URL:",
		placeholder: "https://example.com",
		allowEmpty: false,
	});
	const baseUrlInput = baseUrlRaw.trim();
	if (!baseUrlInput) {
		throw new Error("Base URL cannot be empty.");
	}""",
    ),
]

marker = 'placeholder: "https://example.com"'
if marker in src:
    print(f"already patched: {path}")
    sys.exit(0)
for old, new in replacements:
    if src.count(old) != 1:
        sys.exit(f"patch anchor not found exactly once in {path}:\n{old[:80]}")
    src = src.replace(old, new)
open(path, "w", encoding="utf-8", newline="\n").write(src)
print(f"patched: {path}")
