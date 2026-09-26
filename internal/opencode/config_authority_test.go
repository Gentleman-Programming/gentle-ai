package opencode

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteAuthorityMigrationAndPin(t *testing.T) {
	dir := t.TempDir()
	jsonPath := filepath.Join(dir, "opencode.json")
	jsoncPath := filepath.Join(dir, "opencode.jsonc")
	if err := os.WriteFile(jsonPath, []byte(`{"agent":{"gentle-orchestrator":{"hidden":true,"prompt":"user","permission":{}}}}`), 0600); err != nil {
		t.Fatal(err)
	}
	marked := `{"agent":{"gentle-orchestrator":{"__managed_by":"gentle-ai/sdd"}}}`
	if err := os.WriteFile(jsoncPath, []byte(marked), 0600); err != nil {
		t.Fatal(err)
	}
	got, err := findEffectiveConfigPathChecked("", dir)
	if err != nil || got != jsoncPath {
		t.Fatalf("migration: %q %v", got, err)
	}
	if _, err := os.Lstat(authorityPath(dir)); !os.IsNotExist(err) {
		t.Fatalf("read created authority: %v", err)
	}
	if err := WriteInitialAuthority(dir, jsoncPath); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(jsoncPath, []byte(`{}`), 0600); err != nil {
		t.Fatal(err)
	}
	got, err = findEffectiveConfigPathChecked("", dir)
	if err != nil || got != jsoncPath {
		t.Fatalf("pin: %q %v", got, err)
	}
	other := t.TempDir()
	if err := os.WriteFile(filepath.Join(other, "opencode.json"), []byte(`{}`), 0600); err != nil {
		t.Fatal(err)
	}
	if got, err := findEffectiveConfigPathChecked("", other); err != nil || got != filepath.Join(other, "opencode.json") {
		t.Fatalf("directory isolation: %q %v", got, err)
	}
	if err := os.Remove(jsoncPath); err != nil {
		t.Fatal(err)
	}
	if got, err = findEffectiveConfigPathChecked("", dir); err == nil || got != "" {
		t.Fatalf("missing target: %q %v", got, err)
	}
}

func TestWriteInitialAuthorityRejectsInvalidTargetAndOverwrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "opencode.jsonc")
	if err := os.WriteFile(path, []byte(`{}`), 0600); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{filepath.Join(t.TempDir(), "opencode.jsonc"), filepath.Join(dir, "other.json"), filepath.Join(dir, "opencode.json")} {
		if err := WriteInitialAuthority(dir, bad); err == nil {
			t.Fatalf("accepted %q", bad)
		}
	}
	if _, err := os.Lstat(authorityPath(dir)); !os.IsNotExist(err) {
		t.Fatalf("invalid target created authority: %v", err)
	}
	if err := WriteInitialAuthority(dir, path); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(authorityPath(dir))
	if err != nil {
		t.Fatal(err)
	}
	if err := WriteInitialAuthority(dir, path); err == nil {
		t.Fatal("overwrote authority")
	}
	after, err := os.ReadFile(authorityPath(dir))
	if err != nil || string(before) != string(after) {
		t.Fatalf("record changed: %v", err)
	}
}

func TestWriteAuthorityInvalidFailsClosed(t *testing.T) {
	for _, content := range []string{`{`, `{"version":2,"directory":"wrong","basename":"opencode.json"}`, `{"version":1,"directory":"wrong","basename":"opencode.json"}`} {
		t.Run(content, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "opencode.json")
			if err := os.WriteFile(path, []byte(`{}`), 0600); err != nil {
				t.Fatal(err)
			}
			before, _ := os.ReadFile(path)
			if err := os.WriteFile(filepath.Join(dir, writeAuthorityFile), []byte(content), 0600); err != nil {
				t.Fatal(err)
			}
			if got, err := findEffectiveConfigPathChecked("", dir); err == nil || got != "" {
				t.Fatalf("invalid authority: %q %v", got, err)
			}
			if got := findEffectiveConfigPath("", dir); got != "" {
				t.Fatalf("legacy resolver did not fail closed: %q", got)
			}
			after, _ := os.ReadFile(path)
			if string(after) != string(before) {
				t.Fatal("config changed")
			}
		})
	}
}

func TestWriteAuthoritySymlinkRejected(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "record")
	if err := os.WriteFile(target, []byte(`{}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(dir, writeAuthorityFile)); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if got, err := findEffectiveConfigPathChecked("", dir); err == nil || got != "" {
		t.Fatalf("symlink: %q %v", got, err)
	}
}
