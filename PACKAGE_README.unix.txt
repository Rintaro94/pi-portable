Pi Portable (pi 1.5.0, Linux / macOS)
=====================================

Run
  - Extract with tar so the executable bits are kept:
      tar xzf PiPortable-<os>-<arch>.tar.gz
  - Start:  ./PiPortable/pi        (or put PiPortable/ on PATH)
  - First run: type /login and pick a provider (e.g. cliproxyapi).
  - macOS: if Gatekeeper blocks the files, clear the quarantine flag once:
      xattr -dr com.apple.quarantine PiPortable

Layout
  pi            launcher: sets environment variables, then execs app/pi
  app/          official pi release, unmodified
  home/         fake HOME, XDG_CONFIG_HOME, XDG_CACHE_HOME, XDG_DATA_HOME, XDG_STATE_HOME
    .pi/agent/  all settings, auth.json, sessions, plugins
  tmp/          TMPDIR

Bundled plugins: pi-themes, pi-droid-styling, pi-vcc, pi-cliproxyapi-provider.
The install / remove / update commands are disabled. To upgrade, download a
new package and copy home/.pi/agent/auth.json and sessions/ across.

The model uses the bash tool. Pi uses /bin/bash, or bash on PATH, or sh.

Remove: delete the PiPortable folder.

Notes
  - home/.pi/agent/auth.json and cliproxyapi.json hold your API key in plain
    text. Anyone with a copy of the folder can use your account.
  - The bash tool runs as your user with HOME pointing into the portable
    folder. It can still read and write anywhere your user can.
  - OAuth logins open your default browser; its cookies live in the browser
    profile, outside the portable folder.
  - The OS may still log that the program ran (shell history of the shell
    you launched it from, macOS unified log, auditd if enabled).
