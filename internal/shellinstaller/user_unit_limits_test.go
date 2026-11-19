package shellinstaller

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUserUnitLimitsDefaultWithoutSetting(t *testing.T) {
	home := t.TempDir()
	limits, err := LoadUserUnitLimits(home)
	if err != nil {
		t.Fatalf("missing setting must load the defaults: %v", err)
	}
	if limits != DefaultUserUnitLimits() {
		t.Fatalf("missing setting = %+v, want defaults %+v", limits, DefaultUserUnitLimits())
	}
	want := UserUnitLimits{MemoryMaxBytes: 3221225472, SwapMaxBytes: 0, CPUs: 1, TasksMax: 512}
	if DefaultUserUnitLimits() != want {
		t.Fatalf("defaults changed: %+v, want %+v", DefaultUserUnitLimits(), want)
	}
}

func TestUserUnitLimitsSaveLoadReset(t *testing.T) {
	home := t.TempDir()
	set := UserUnitLimits{MemoryMaxBytes: 12 << 30, SwapMaxBytes: 8 << 30, CPUs: 1, TasksMax: 2048}
	if err := SaveUserUnitLimits(home, set); err != nil {
		t.Fatalf("save: %v", err)
	}
	info, err := os.Stat(UserUnitLimitsPath(home))
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("setting file mode = %v (%v), want 0600", info, err)
	}
	got, err := LoadUserUnitLimits(home)
	if err != nil || got != set {
		t.Fatalf("load = %+v (%v), want %+v", got, err, set)
	}
	if err := ResetUserUnitLimits(home); err != nil {
		t.Fatalf("reset: %v", err)
	}
	if err := ResetUserUnitLimits(home); err != nil {
		t.Fatalf("reset without a setting must succeed: %v", err)
	}
	got, err = LoadUserUnitLimits(home)
	if err != nil || got != DefaultUserUnitLimits() {
		t.Fatalf("after reset = %+v (%v), want defaults", got, err)
	}
}

func TestUserUnitLimitsRejectsInvalidSetting(t *testing.T) {
	for name, content := range map[string]string{
		"not json":      "nope",
		"zero memory":   `{"memory_max_bytes":0,"swap_max_bytes":0,"cpus":1,"tasks_max":512}`,
		"below memory":  `{"memory_max_bytes":1048576,"swap_max_bytes":0,"cpus":1,"tasks_max":512}`,
		"zero cpus":     `{"memory_max_bytes":3221225472,"swap_max_bytes":0,"cpus":0,"tasks_max":512}`,
		"tasks too low": `{"memory_max_bytes":3221225472,"swap_max_bytes":0,"cpus":1,"tasks_max":64}`,
		"unaligned mem": `{"memory_max_bytes":1395864371,"swap_max_bytes":0,"cpus":1,"tasks_max":512}`,
		"unaligned swp": `{"memory_max_bytes":3221225472,"swap_max_bytes":4097,"cpus":1,"tasks_max":512}`,
		"unknown field": `{"memory_max_bytes":3221225472,"swap_max_bytes":0,"cpus":1,"tasks_max":512,"x":1}`,
	} {
		t.Run(name, func(t *testing.T) {
			home := t.TempDir()
			path := UserUnitLimitsPath(home)
			if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := LoadUserUnitLimits(home); err == nil || !strings.Contains(err.Error(), path) {
				t.Fatalf("invalid setting must refuse and name %s, got %v", path, err)
			}
		})
	}
}

func TestUserUnitLimitsEnvironmentRoundTrip(t *testing.T) {
	set := UserUnitLimits{MemoryMaxBytes: 12 << 30, SwapMaxBytes: 8 << 30, CPUs: 6, TasksMax: 2048}
	entry := set.environment()
	if entry != "GENTLE_SHELL_UNIT_LIMITS=12884901888/8589934592/6/2048" {
		t.Fatalf("environment entry = %q", entry)
	}
	got, err := userUnitLimitsFromEnv([]string{"HOME=/h", entry})
	if err != nil || got != set {
		t.Fatalf("parsed %+v (%v), want %+v", got, err, set)
	}
	got, err = userUnitLimitsFromEnv([]string{"HOME=/h"})
	if err != nil || got != DefaultUserUnitLimits() {
		t.Fatalf("absent entry = %+v (%v), want defaults", got, err)
	}
	for _, bad := range []string{"1395864371/0/1/512", "max/0/1/512", "3221225472/0/1", "3221225472/0/0/512", "3221225472/0/1/64", ""} {
		if _, err := userUnitLimitsFromEnv([]string{"GENTLE_SHELL_UNIT_LIMITS=" + bad}); err == nil {
			t.Fatalf("invalid entry %q accepted", bad)
		}
	}
	if set.cpuMax() != "600000 100000" || DefaultUserUnitLimits().cpuMax() != "100000 100000" {
		t.Fatalf("cpu.max = %q / %q", set.cpuMax(), DefaultUserUnitLimits().cpuMax())
	}
}

func TestParseUserUnitSize(t *testing.T) {
	for in, want := range map[string]uint64{"0": 0, "3G": 3 << 30, "12g": 12 << 30, "512M": 512 << 20, "1T": 1 << 40, "4096": 0, "1.5G": 3 << 29, "1.3G": 1331 << 20, "1048577": 1 << 20} {
		got, err := ParseUserUnitSize(in)
		if err != nil || got != want {
			t.Fatalf("ParseUserUnitSize(%q) = %d (%v), want %d", in, got, err, want)
		}
	}
	for _, bad := range []string{"", "max", "-1G", "12X", "G"} {
		if _, err := ParseUserUnitSize(bad); err == nil {
			t.Fatalf("ParseUserUnitSize(%q) accepted", bad)
		}
	}
}
