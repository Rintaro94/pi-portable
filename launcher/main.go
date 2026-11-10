// Pi Portable launcher.
//
// Sets up an environment that keeps all Pi state inside the portable folder,
// then runs app/pi (app\pi.exe on Windows) with the same arguments and console.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Package-management subcommands are disabled: the portable build only ships
// the bundled plugins and must not touch npm/git or replace its own binary.
var blockedCommands = map[string]bool{
	"install":   true,
	"remove":    true,
	"uninstall": true,
	"update":    true,
}

func fail(format string, a ...any) {
	fmt.Fprintf(os.Stderr, "pi-portable: "+format+"\n", a...)
	os.Exit(1)
}

func main() {
	exe, err := os.Executable()
	if err != nil {
		fail("cannot locate launcher: %v", err)
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	root := filepath.Dir(exe)

	args := os.Args[1:]
	if len(args) > 0 && blockedCommands[strings.ToLower(args[0])] {
		fail("'%s' is disabled in Pi Portable. Plugins are fixed; to upgrade, download a new package.", args[0])
	}

	core := filepath.Join(root, "app", coreName)
	if _, err := os.Stat(core); err != nil {
		fail("missing %s", core)
	}

	home := filepath.Join(root, "home")
	agent := filepath.Join(home, ".pi", "agent")
	tmp := filepath.Join(root, "tmp")

	overrides := map[string]string{
		"PI_CODING_AGENT_DIR":   agent,
		"HOME":                  home,
		"TMPDIR":                tmp,
		"PI_SKIP_VERSION_CHECK": "1",
		"PI_TELEMETRY":          "0",
		"PI_PORTABLE_ROOT":      root,
		"npm_config_userconfig": filepath.Join(home, ".npmrc"),
		"GIT_CONFIG_NOSYSTEM":   "1",
	}
	dirs := []string{agent, tmp}
	extra, extraDirs := platformEnv(home, tmp)
	for k, v := range extra {
		overrides[k] = v
	}
	dirs = append(dirs, extraDirs...)

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			fail("cannot create %s: %v", dir, err)
		}
	}

	replaced := make(map[string]bool, len(overrides))
	for k := range overrides {
		replaced[envKey(k)] = true
	}
	env := make([]string, 0, len(os.Environ())+len(overrides))
	for _, kv := range os.Environ() {
		name, _, _ := strings.Cut(kv, "=")
		if name != "" && replaced[envKey(name)] {
			continue
		}
		env = append(env, kv)
	}
	for k, v := range overrides {
		env = append(env, k+"="+v)
	}

	run(core, args, env)
}
