// Pi Portable launcher.
//
// Sets up an environment that keeps all Pi state inside the portable folder,
// then runs app\pi.exe in the same console and returns its exit code.
package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
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
		fail("'%s' is disabled in Pi Portable. Plugins are fixed; to upgrade, download a new zip.", args[0])
	}

	core := filepath.Join(root, "app", "pi.exe")
	if _, err := os.Stat(core); err != nil {
		fail("missing %s", core)
	}

	home := filepath.Join(root, "home")
	agent := filepath.Join(home, ".pi", "agent")
	appData := filepath.Join(home, "AppData", "Roaming")
	localAppData := filepath.Join(home, "AppData", "Local")
	tmp := filepath.Join(root, "tmp")
	for _, dir := range []string{agent, appData, localAppData, tmp} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			fail("cannot create %s: %v", dir, err)
		}
	}

	vol := filepath.VolumeName(home)
	overrides := map[string]string{
		"PI_CODING_AGENT_DIR":   agent,
		"HOME":                  home,
		"USERPROFILE":           home,
		"HOMEDRIVE":             vol,
		"HOMEPATH":              strings.TrimPrefix(home, vol),
		"APPDATA":               appData,
		"LOCALAPPDATA":          localAppData,
		"TEMP":                  tmp,
		"TMP":                   tmp,
		"TMPDIR":                tmp,
		"PI_SKIP_VERSION_CHECK": "1",
		"PI_TELEMETRY":          "0",
		"PI_PORTABLE_ROOT":      root,
		"npm_config_cache":      filepath.Join(localAppData, "npm-cache"),
		"npm_config_userconfig": filepath.Join(home, ".npmrc"),
		"GIT_CONFIG_NOSYSTEM":   "1",
	}

	// Windows environment names are case-insensitive.
	replaced := make(map[string]bool, len(overrides))
	for k := range overrides {
		replaced[strings.ToUpper(k)] = true
	}
	env := make([]string, 0, len(os.Environ())+len(overrides))
	for _, kv := range os.Environ() {
		name, _, _ := strings.Cut(kv, "=")
		if name != "" && replaced[strings.ToUpper(name)] {
			continue
		}
		env = append(env, kv)
	}
	for k, v := range overrides {
		env = append(env, k+"="+v)
	}

	cmd := exec.Command(core, args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	cmd.Env = env

	// Ctrl+C is delivered to every process on the console. Pi handles it;
	// the launcher must survive so it can wait and forward the exit code.
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt)
	go func() {
		for range sig {
		}
	}()

	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			os.Exit(exitErr.ExitCode())
		}
		fail("cannot start %s: %v", core, err)
	}
}
