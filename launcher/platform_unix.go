//go:build !windows

package main

import (
	"path/filepath"
	"syscall"
)

const coreName = "pi"

func envKey(name string) string { return name }

func platformEnv(home, tmp string) (map[string]string, []string) {
	config := filepath.Join(home, ".config")
	cache := filepath.Join(home, ".cache")
	data := filepath.Join(home, ".local", "share")
	state := filepath.Join(home, ".local", "state")
	return map[string]string{
		"XDG_CONFIG_HOME":  config,
		"XDG_CACHE_HOME":   cache,
		"XDG_DATA_HOME":    data,
		"XDG_STATE_HOME":   state,
		"npm_config_cache": filepath.Join(cache, "npm"),
	}, []string{config, cache, data, state}
}

// run replaces the launcher with pi, so signals, job control and the exit
// code go straight to pi without a wrapper process in between.
func run(core string, args, env []string) {
	argv := append([]string{core}, args...)
	err := syscall.Exec(core, argv, env)
	fail("cannot start %s: %v", core, err)
}
