package engram

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestDefaultDataDir_RespectsEnvVar(t *testing.T) {
	orig := os.Getenv(DataDirEnvVar)
	defer os.Setenv(DataDirEnvVar, orig)

	os.Setenv(DataDirEnvVar, "/custom/engram")
	got := DefaultDataDir()
	// filepath.Abs may add drive letter on Windows; verify it ends with the right path.
	if !strings.HasSuffix(got, filepath.FromSlash("/custom/engram")) {
		t.Errorf("DefaultDataDir() = %q, want suffix %q", got, filepath.FromSlash("/custom/engram"))
	}
}

func TestDefaultDataDir_FallsBackToHome(t *testing.T) {
	orig := os.Getenv(DataDirEnvVar)
	defer os.Setenv(DataDirEnvVar, orig)
	os.Unsetenv(DataDirEnvVar)

	home, _ := os.UserHomeDir()
	want := filepath.Join(home, ".engram")
	if got := DefaultDataDir(); got != want {
		t.Errorf("DefaultDataDir() = %q, want %q", got, want)
	}
}

func TestDetectExistingData(t *testing.T) {
	tmp := t.TempDir()

	if DetectExistingData(tmp) {
		t.Error("DetectExistingData(tmp) = true, want false")
	}

	if err := os.WriteFile(filepath.Join(tmp, "engram.db"), []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}

	if !DetectExistingData(tmp) {
		t.Error("DetectExistingData(tmp) = false, want true")
	}
}

func TestExistingEngramFiles(t *testing.T) {
	tmp := t.TempDir()

	if got := ExistingEngramFiles(tmp); len(got) != 0 {
		t.Errorf("ExistingEngramFiles(tmp) = %v, want empty", got)
	}

	files := []string{"engram.db", "engram.db-wal"}
	for _, f := range files {
		if err := os.WriteFile(filepath.Join(tmp, f), []byte(f), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	got := ExistingEngramFiles(tmp)
	if len(got) != 2 {
		t.Errorf("len(ExistingEngramFiles) = %d, want 2", len(got))
	}
}

func TestMigrateData(t *testing.T) {
	source := t.TempDir()
	target := t.TempDir()

	// Create source files
	content := []byte("sqlite data")
	for _, f := range []string{"engram.db", "engram.db-wal"} {
		if err := os.WriteFile(filepath.Join(source, f), content, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	if err := MigrateData(source, target); err != nil {
		t.Fatalf("MigrateData() error = %v", err)
	}

	// Target should have files
	for _, f := range []string{"engram.db", "engram.db-wal"} {
		if _, err := os.Stat(filepath.Join(target, f)); err != nil {
			t.Errorf("target missing %s: %v", f, err)
		}
	}

	// Source should NOT have files
	for _, f := range []string{"engram.db", "engram.db-wal"} {
		if _, err := os.Stat(filepath.Join(source, f)); !os.IsNotExist(err) {
			t.Errorf("source still has %s", f)
		}
	}
}

func TestMigrateData_CreatesTargetDir(t *testing.T) {
	source := t.TempDir()
	target := filepath.Join(t.TempDir(), "nested", "engram")

	if err := os.WriteFile(filepath.Join(source, "engram.db"), []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := MigrateData(source, target); err != nil {
		t.Fatalf("MigrateData() error = %v", err)
	}

	if _, err := os.Stat(filepath.Join(target, "engram.db")); err != nil {
		t.Errorf("target file missing: %v", err)
	}
}

func TestMigrateData_NoSourceFiles(t *testing.T) {
	source := t.TempDir()
	target := t.TempDir()

	if err := MigrateData(source, target); err != nil {
		t.Fatalf("MigrateData() error = %v", err)
	}
}

func TestMigrateData_PartialFailureLeavesSource(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("read-only directory test is unreliable on Windows")
	}

	source := t.TempDir()
	target := t.TempDir()

	// Write a file that is readable
	if err := os.WriteFile(filepath.Join(source, "engram.db"), []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Make target read-only so second file would fail
	if err := os.Chmod(target, 0o555); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(target, 0o755)

	// Should fail because target is read-only
	if err := MigrateData(source, target); err == nil {
		t.Fatal("expected error for read-only target")
	}

	// Source should still have the file
	if _, err := os.Stat(filepath.Join(source, "engram.db")); err != nil {
		t.Error("source file was removed despite migration failure")
	}
}

func TestDetectLockedData_NoFiles(t *testing.T) {
	dir := t.TempDir()
	locked, err := DetectLockedData(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if locked {
		t.Fatal("expected locked=false when no files exist")
	}
}

func TestDetectLockedData_UnlockedFiles(t *testing.T) {
	dir := t.TempDir()
	for _, f := range []string{"engram.db", "engram.db-wal"} {
		if err := os.WriteFile(filepath.Join(dir, f), []byte("data"), 0o644); err != nil {
			t.Fatalf("write %s: %v", f, err)
		}
	}
	locked, err := DetectLockedData(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if locked {
		t.Fatal("expected locked=false for unlocked files")
	}
}

func TestExpandDataDir(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("cannot get home dir")
	}

	tests := []struct {
		name    string
		input   string
		wantErr bool
		wantSub string
	}{
		{"empty", "", true, ""},
		{"tilde", "~/engram", false, filepath.Join(home, "engram")},
		{"absolute", filepath.Join(os.TempDir(), "engram"), false, filepath.Join(os.TempDir(), "engram")},
		{"relative", "engram", false, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ExpandDataDir(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ExpandDataDir(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
			if !tt.wantErr && tt.wantSub != "" && got != tt.wantSub {
				t.Errorf("ExpandDataDir(%q) = %q, want %q", tt.input, got, tt.wantSub)
			}
		})
	}
}

func TestDefaultDataDir_NormalizesRelativePaths(t *testing.T) {
	orig := os.Getenv(DataDirEnvVar)
	defer os.Setenv(DataDirEnvVar, orig)

	cwd, _ := os.Getwd()
	os.Setenv(DataDirEnvVar, "./engram")

	got := DefaultDataDir()
	want := filepath.Join(cwd, "engram")
	if got != want {
		t.Errorf("DefaultDataDir() = %q, want %q", got, want)
	}
}

func TestDefaultDataDir_AbsoluteUnchanged(t *testing.T) {
	orig := os.Getenv(DataDirEnvVar)
	defer os.Setenv(DataDirEnvVar, orig)

	os.Setenv(DataDirEnvVar, "/absolute/engram")

	got := DefaultDataDir()
	if !strings.HasSuffix(got, filepath.FromSlash("/absolute/engram")) {
		t.Errorf("DefaultDataDir() = %q, want suffix %q", got, filepath.FromSlash("/absolute/engram"))
	}
}

func TestDetectLockedData_LsofAvailableAndClosed(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix-only test")
	}

	tmp := t.TempDir()
	_ = os.WriteFile(filepath.Join(tmp, "engram.db"), []byte("data"), 0o644)

	locked, err := DetectLockedData(tmp)
	if err != nil {
		t.Fatalf("DetectLockedData: %v", err)
	}
	if locked {
		t.Error("DetectLockedData = true, want false (no process has file open)")
	}
}

func TestDetectLockedData_LsofMissing(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix-only test")
	}

	// This test passes on systems without lsof (returns false, nil).
	// On systems WITH lsof and no open files, it also returns false.
	// We can't easily test the "lsof available + file open" case without
	// actually locking a file in another process.
	tmp := t.TempDir()
	_ = os.WriteFile(filepath.Join(tmp, "engram.db"), []byte("data"), 0o644)

	locked, err := DetectLockedData(tmp)
	if err != nil {
		t.Fatalf("DetectLockedData: %v", err)
	}
	if locked {
		t.Error("DetectLockedData = true, want false")
	}
}
