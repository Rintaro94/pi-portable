//go:build windows

package main

import (
	"errors"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
)

const coreName = "pi.exe"

// Windows environment names are case-insensitive.
func envKey(name string) string { return strings.ToUpper(name) }

func platformEnv(home, tmp string) (map[string]string, []string) {
	appData := filepath.Join(home, "AppData", "Roaming")
	localAppData := filepath.Join(home, "AppData", "Local")
	vol := filepath.VolumeName(home)
	return map[string]string{
		"USERPROFILE":      home,
		"HOMEDRIVE":        vol,
		"HOMEPATH":         strings.TrimPrefix(home, vol),
		"APPDATA":          appData,
		"LOCALAPPDATA":     localAppData,
		"TEMP":             tmp,
		"TMP":              tmp,
		"npm_config_cache": filepath.Join(localAppData, "npm-cache"),
	}, []string{appData, localAppData}
}

// run starts pi as a child in the same console and forwards its exit code.
func run(core string, args, env []string) {
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
