package vscode

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDetectVSCodeProfiles_HappyPath(t *testing.T) {
	agentsDir := t.TempDir()

	// Write 10 sdd-*-cheap.agent.md + 10 sdd-*-fast.agent.md files
	for _, phase := range sddPhases {
		for _, profileName := range []string{"cheap", "fast"} {
			fname := phase + "-" + profileName + ".agent.md"
			if err := os.WriteFile(filepath.Join(agentsDir, fname), []byte("content"), 0o644); err != nil {
				t.Fatalf("WriteFile(%q) error = %v", fname, err)
			}
		}
	}

	profiles, err := DetectVSCodeProfiles(agentsDir)
	if err != nil {
		t.Fatalf("DetectVSCodeProfiles() error = %v", err)
	}
	if len(profiles) != 2 {
		t.Fatalf("DetectVSCodeProfiles() returned %d profiles, want 2", len(profiles))
	}
	// Must be sorted by name
	if profiles[0].Name != "cheap" {
		t.Errorf("profiles[0].Name = %q, want %q", profiles[0].Name, "cheap")
	}
	if profiles[1].Name != "fast" {
		t.Errorf("profiles[1].Name = %q, want %q", profiles[1].Name, "fast")
	}
}

func TestDetectVSCodeProfiles_EmptyDir(t *testing.T) {
	agentsDir := t.TempDir()

	profiles, err := DetectVSCodeProfiles(agentsDir)
	if err != nil {
		t.Fatalf("DetectVSCodeProfiles() on empty dir error = %v", err)
	}
	if len(profiles) != 0 {
		t.Fatalf("DetectVSCodeProfiles() returned %d profiles on empty dir, want 0", len(profiles))
	}
}

func TestDetectVSCodeProfiles_MissingDir(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "does-not-exist")

	profiles, err := DetectVSCodeProfiles(missing)
	if err != nil {
		t.Fatalf("DetectVSCodeProfiles() on missing dir error = %v (want nil)", err)
	}
	if len(profiles) != 0 {
		t.Fatalf("DetectVSCodeProfiles() returned %d profiles on missing dir, want 0", len(profiles))
	}
}

func TestDetectVSCodeProfiles_IgnoresNonSDDFiles(t *testing.T) {
	agentsDir := t.TempDir()

	// Write non-SDD files that must be ignored
	noiseFiles := []string{
		"my-custom.agent.md",
		".DS_Store",
		"notes.txt",
		"readme.md",
	}
	for _, f := range noiseFiles {
		if err := os.WriteFile(filepath.Join(agentsDir, f), []byte("noise"), 0o644); err != nil {
			t.Fatalf("WriteFile(%q) error = %v", f, err)
		}
	}
	// Write one valid sdd profile file
	if err := os.WriteFile(filepath.Join(agentsDir, "sdd-apply-myprofile.agent.md"), []byte("content"), 0o644); err != nil {
		t.Fatalf("WriteFile error = %v", err)
	}

	profiles, err := DetectVSCodeProfiles(agentsDir)
	if err != nil {
		t.Fatalf("DetectVSCodeProfiles() error = %v", err)
	}
	if len(profiles) != 1 {
		t.Fatalf("DetectVSCodeProfiles() returned %d profiles, want 1", len(profiles))
	}
	if profiles[0].Name != "myprofile" {
		t.Errorf("profiles[0].Name = %q, want %q", profiles[0].Name, "myprofile")
	}
}

func TestDetectVSCodeProfiles_DefaultFilesExcluded(t *testing.T) {
	agentsDir := t.TempDir()

	// Unsuffixed default files must NOT be counted as profiles
	defaultFiles := []string{
		"sdd-apply.agent.md",
		"sdd-verify.agent.md",
		"sdd-init.agent.md",
	}
	for _, f := range defaultFiles {
		if err := os.WriteFile(filepath.Join(agentsDir, f), []byte("default"), 0o644); err != nil {
			t.Fatalf("WriteFile(%q) error = %v", f, err)
		}
	}

	profiles, err := DetectVSCodeProfiles(agentsDir)
	if err != nil {
		t.Fatalf("DetectVSCodeProfiles() error = %v", err)
	}
	if len(profiles) != 0 {
		t.Fatalf("DetectVSCodeProfiles() returned %d profiles for default files only, want 0", len(profiles))
	}
}
