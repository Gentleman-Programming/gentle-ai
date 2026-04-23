package platform

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestPersistEngramEnv_WritesToProfile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix-only test")
	}

	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("SHELL", "/bin/bash")

	profile := filepath.Join(home, ".bashrc")
	_ = os.WriteFile(profile, []byte("# existing\nalias ll='ls -la'\n"), 0o644)

	err := PersistEngramEnv("/custom/engram")
	if err != nil {
		t.Fatalf("PersistEngramEnv: %v", err)
	}

	data, err := os.ReadFile(profile)
	if err != nil {
		t.Fatalf("read profile: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, "export ENGRAM_DATA_DIR=\"/custom/engram\"") {
		t.Errorf("profile missing ENGRAM_DATA_DIR entry:\n%s", content)
	}
	if !strings.Contains(content, "alias ll='ls -la'") {
		t.Errorf("profile lost existing content:\n%s", content)
	}
}

func TestPersistEngramEnv_ReplacesExisting(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix-only test")
	}

	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("SHELL", "/bin/bash")

	profile := filepath.Join(home, ".bashrc")
	_ = os.WriteFile(profile, []byte("export ENGRAM_DATA_DIR=\"/old\"\n"), 0o644)

	err := PersistEngramEnv("/new")
	if err != nil {
		t.Fatalf("PersistEngramEnv: %v", err)
	}

	data, _ := os.ReadFile(profile)
	content := string(data)

	if strings.Contains(content, "/old") {
		t.Errorf("old entry not removed:\n%s", content)
	}
	if !strings.Contains(content, "export ENGRAM_DATA_DIR=\"/new\"") {
		t.Errorf("new entry not found:\n%s", content)
	}
}

func TestRemoveEngramEnv_RemovesFromProfile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix-only test")
	}

	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("SHELL", "/bin/bash")

	profile := filepath.Join(home, ".bashrc")
	_ = os.WriteFile(profile, []byte("# start\nexport ENGRAM_DATA_DIR=\"/old\"\n# end\n"), 0o644)

	err := RemoveEngramEnv()
	if err != nil {
		t.Fatalf("RemoveEngramEnv: %v", err)
	}

	data, _ := os.ReadFile(profile)
	content := string(data)

	if strings.Contains(content, "ENGRAM_DATA_DIR") {
		t.Errorf("entry not removed:\n%s", content)
	}
	if !strings.Contains(content, "# start") || !strings.Contains(content, "# end") {
		t.Errorf("surrounding content lost:\n%s", content)
	}
}

func TestPersistEngramEnv_FishFormat(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix-only test")
	}

	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("SHELL", "/usr/bin/fish")

	profile := filepath.Join(home, ".config", "fish", "config.fish")
	_ = os.MkdirAll(filepath.Dir(profile), 0o755)
	_ = os.WriteFile(profile, []byte("# fish config\n"), 0o644)

	err := PersistEngramEnv("/custom/engram")
	if err != nil {
		t.Fatalf("PersistEngramEnv: %v", err)
	}

	data, _ := os.ReadFile(profile)
	content := string(data)

	if !strings.Contains(content, "set -gx ENGRAM_DATA_DIR \"/custom/engram\"") {
		t.Errorf("fish format missing:\n%s", content)
	}
}
