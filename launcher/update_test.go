package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestCompareVersions(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"v1.5.0-2", "v1.5.0", 1},
		{"v1.5.0", "v1.5.0-1", 0},
		{"v1.6.0", "v1.5.0-9", 1},
		{"v1.10.0", "v1.9.0", 1},
		{"v2.0.0", "v1.99.99-99", 1},
		{"v1.5.0-2", "v1.5.0-2", 0},
		{"v1.5.0", "dev", 1},
		{"unknown", "v0.0.1", -1},
		// dated tags vX.Y.Z.YYMMDD
		{"v1.5.0.261110", "v1.5.0-2", 1},
		{"v1.5.0.261110", "v1.5.0", 1},
		{"v1.5.0.270101", "v1.5.0.261231", 1}, // new year still sorts later
		{"v1.5.0.261201", "v1.5.0.261130", 1}, // new month too
		{"v1.5.1.261110", "v1.5.0.261231", 1}, // pi version wins over date
		{"v1.5.0.261110", "v1.5.0.261110", 0},
		{"v1.6.0-2", "v1.5.0.261231", 1},
	}
	for _, c := range cases {
		if got := compareVersions(c.a, c.b); got != c.want {
			t.Errorf("compareVersions(%q, %q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
	for _, bad := range []string{"1.5.0", "v1.5", "v1.5.0-", "v1.5.0-0", "v1.x.0", "v1.5.0-rc1",
		"v1.5.0.2611", "v1.5.0.26111", "v1.5.0.2611100", "v1.5.0.261310", "v1.5.0.261100", "v1.5.0.261132",
		"v1.5.0.261110-2", "v1.5.0.261110.1", "v01.5.0.261110", "v1.5.0-261110", "1.5.0.261110", "v1.5.0.+61110", "v1.5.0.-61110", "v+1.5.0.261110"} {
		if _, ok := parseVersion(bad); ok {
			t.Errorf("parseVersion(%q) accepted", bad)
		}
	}
}

func TestSafeJoin(t *testing.T) {
	dest := t.TempDir()
	for _, ok := range []string{"PiPortable", "PiPortable/", "PiPortable/app/pi", "PiPortable/home/.pi/agent/settings.json"} {
		if _, err := safeJoin(dest, ok); err != nil {
			t.Errorf("safeJoin(%q) rejected: %v", ok, err)
		}
	}
	for _, bad := range []string{"../evil", "PiPortable/../../evil", "/etc/passwd", "other/file", "PiPortableX/a", `PiPortable\..\..\evil`} {
		if _, err := safeJoin(dest, bad); err == nil {
			t.Errorf("safeJoin(%q) accepted", bad)
		}
	}
}

func TestMergeSettings(t *testing.T) {
	dir := t.TempDir()
	user := filepath.Join(dir, "user.json")
	next := filepath.Join(dir, "new.json")
	os.WriteFile(user, []byte(`{"theme":"mine","defaultModel":"x","packages":["./old"],"defaultTools":["bash"],"lastChangelogVersion":"1.5.0"}`), 0o644)
	os.WriteFile(next, []byte(`{"theme":"default","packages":["./a","./b"],"lastChangelogVersion":"1.6.0","extensionChangelogVersions":{"k":"2"}}`), 0o644)
	if err := mergeSettings(user, next); err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	data, _ := os.ReadFile(user)
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got["theme"] != "mine" || got["defaultModel"] != "x" {
		t.Errorf("user keys not kept: %v", got)
	}
	if p := got["packages"].([]any); len(p) != 2 || p[0] != "./a" {
		t.Errorf("packages not taken from new release: %v", got["packages"])
	}
	if _, ok := got["defaultTools"]; ok {
		t.Errorf("defaultTools should be removed when the new release has none")
	}
	if got["lastChangelogVersion"] != "1.6.0" || got["extensionChangelogVersions"] == nil {
		t.Errorf("version keys not updated: %v", got)
	}
}
