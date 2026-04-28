package engram

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestLocalDataBackend_MigrateData_InsufficientSpaceDoesNotCopyFiles(t *testing.T) {
	backend := NewLocalDataBackend()
	source := t.TempDir()
	target := t.TempDir()

	if err := os.WriteFile(filepath.Join(source, "engram.db"), []byte("sqlite data"), 0o644); err != nil {
		t.Fatal(err)
	}

	orig := requireFreeSpace
	requireFreeSpace = func(path string, minBytes uint64) error {
		return fmt.Errorf("insufficient disk space at \"%s\": need 100 B, have 0 B", path)
	}
	defer func() { requireFreeSpace = orig }()

	_, err := backend.MigrateData(source, target)
	if err == nil {
		t.Fatal("expected error for insufficient disk space, got nil")
	}
	want := "insufficient disk space"
	if !strings.Contains(err.Error(), want) {
		t.Errorf("error = %q, want substring %q", err.Error(), want)
	}

	if _, err := os.Stat(filepath.Join(target, "engram.db")); !os.IsNotExist(err) {
		t.Error("target should not have engram.db when space check fails")
	}
	if _, err := os.Stat(filepath.Join(source, "engram.db")); err != nil {
		t.Error("source file was removed despite migration failure")
	}
}

func TestLocalDataBackend_HardDefaultDataDir_IgnoresEnvVar(t *testing.T) {
	backend := NewLocalDataBackend()
	orig := os.Getenv(DataDirEnvVar)
	defer os.Setenv(DataDirEnvVar, orig)

	os.Setenv(DataDirEnvVar, "/custom/engram")
	got := backend.HardDefaultDataDir()
	if strings.HasSuffix(got, "/custom/engram") || strings.HasSuffix(got, `\custom\engram`) {
		t.Errorf("HardDefaultDataDir() = %q, should ignore ENGRAM_DATA_DIR env var", got)
	}
	home, _ := os.UserHomeDir()
	want := filepath.Join(home, ".engram")
	if got != want {
		t.Errorf("HardDefaultDataDir() = %q, want %q", got, want)
	}
}

func TestLocalDataBackend_DefaultDataDir_RespectsEnvVar(t *testing.T) {
	backend := NewLocalDataBackend()
	orig := os.Getenv(DataDirEnvVar)
	defer os.Setenv(DataDirEnvVar, orig)

	os.Setenv(DataDirEnvVar, "/custom/engram")
	got := backend.DefaultDataDir()
	if !strings.HasSuffix(got, filepath.FromSlash("/custom/engram")) {
		t.Errorf("DefaultDataDir() = %q, want suffix %q", got, filepath.FromSlash("/custom/engram"))
	}
}

func TestLocalDataBackend_DefaultDataDir_FallsBackToHome(t *testing.T) {
	backend := NewLocalDataBackend()
	orig := os.Getenv(DataDirEnvVar)
	defer os.Setenv(DataDirEnvVar, orig)
	os.Unsetenv(DataDirEnvVar)

	home, _ := os.UserHomeDir()
	want := filepath.Join(home, ".engram")
	if got := backend.DefaultDataDir(); got != want {
		t.Errorf("DefaultDataDir() = %q, want %q", got, want)
	}
}

func TestLocalDataBackend_DetectExistingData(t *testing.T) {
	backend := NewLocalDataBackend()
	tmp := t.TempDir()

	if backend.DetectExistingData(tmp) {
		t.Error("DetectExistingData(tmp) = true, want false")
	}

	if err := os.WriteFile(filepath.Join(tmp, "engram.db"), []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}

	if !backend.DetectExistingData(tmp) {
		t.Error("DetectExistingData(tmp) = false, want true")
	}
}

func TestLocalDataBackend_ExistingFiles(t *testing.T) {
	backend := NewLocalDataBackend()
	tmp := t.TempDir()

	if got := backend.ExistingFiles(tmp); len(got) != 0 {
		t.Errorf("ExistingFiles(tmp) = %v, want empty", got)
	}

	files := []string{"engram.db", "engram.db-wal"}
	for _, f := range files {
		if err := os.WriteFile(filepath.Join(tmp, f), []byte(f), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	got := backend.ExistingFiles(tmp)
	if len(got) != 2 {
		t.Errorf("len(ExistingFiles) = %d, want 2", len(got))
	}
}

func TestLocalDataBackend_MigrateData(t *testing.T) {
	backend := NewLocalDataBackend()
	source := t.TempDir()
	target := t.TempDir()

	content := []byte("sqlite data")
	for _, f := range []string{"engram.db", "engram.db-wal"} {
		if err := os.WriteFile(filepath.Join(source, f), content, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	if _, err := backend.MigrateData(source, target); err != nil {
		t.Fatalf("MigrateData() error = %v", err)
	}

	// Target should have the copied files.
	for _, f := range []string{"engram.db", "engram.db-wal"} {
		if _, err := os.Stat(filepath.Join(target, f)); err != nil {
			t.Errorf("target missing %s: %v", f, err)
		}
	}
	// Source should STILL have the files — MigrateData no longer deletes.
	// Deletion is the responsibility of DataDirService.Execute after config
	// persistence succeeds.
	for _, f := range []string{"engram.db", "engram.db-wal"} {
		if _, err := os.Stat(filepath.Join(source, f)); err != nil {
			t.Errorf("source missing %s after copy-only migration", f)
		}
	}
}

func TestLocalDataBackend_MigrateData_CreatesTargetDir(t *testing.T) {
	backend := NewLocalDataBackend()
	source := t.TempDir()
	target := filepath.Join(t.TempDir(), "nested", "engram")

	if err := os.WriteFile(filepath.Join(source, "engram.db"), []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := backend.MigrateData(source, target); err != nil {
		t.Fatalf("MigrateData() error = %v", err)
	}

	if _, err := os.Stat(filepath.Join(target, "engram.db")); err != nil {
		t.Errorf("target file missing: %v", err)
	}
}

func TestLocalDataBackend_MigrateData_NoSourceFiles(t *testing.T) {
	backend := NewLocalDataBackend()
	source := t.TempDir()
	target := t.TempDir()

	if _, err := backend.MigrateData(source, target); err != nil {
		t.Fatalf("MigrateData() error = %v", err)
	}
}

func TestLocalDataBackend_MigrateData_PartialFailureLeavesSource(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("read-only directory test is unreliable on Windows")
	}

	backend := NewLocalDataBackend()
	source := t.TempDir()
	target := t.TempDir()

	if err := os.WriteFile(filepath.Join(source, "engram.db"), []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := os.Chmod(target, 0o555); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(target, 0o755)

	if _, err := backend.MigrateData(source, target); err == nil {
		t.Fatal("expected error for read-only target")
	}

	if _, err := os.Stat(filepath.Join(source, "engram.db")); err != nil {
		t.Error("source file was removed despite migration failure")
	}
}

func TestLocalDataBackend_DetectLockedData_NoFiles(t *testing.T) {
	backend := NewLocalDataBackend()
	dir := t.TempDir()
	locked, err := backend.DetectLockedData(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if locked {
		t.Fatal("expected locked=false when no files exist")
	}
}

func TestLocalDataBackend_DetectLockedData_UnlockedFiles(t *testing.T) {
	backend := NewLocalDataBackend()
	dir := t.TempDir()
	for _, f := range []string{"engram.db", "engram.db-wal"} {
		if err := os.WriteFile(filepath.Join(dir, f), []byte("data"), 0o644); err != nil {
			t.Fatalf("write %s: %v", f, err)
		}
	}
	locked, err := backend.DetectLockedData(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if locked {
		t.Fatal("expected locked=false for unlocked files")
	}
}

func TestLocalDataBackend_ExpandPath(t *testing.T) {
	backend := NewLocalDataBackend()
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
			got, err := backend.ExpandPath(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ExpandPath(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
			if !tt.wantErr && tt.wantSub != "" && got != tt.wantSub {
				t.Errorf("ExpandPath(%q) = %q, want %q", tt.input, got, tt.wantSub)
			}
		})
	}
}

func TestLocalDataBackend_DefaultDataDir_NormalizesRelativePaths(t *testing.T) {
	backend := NewLocalDataBackend()
	orig := os.Getenv(DataDirEnvVar)
	defer os.Setenv(DataDirEnvVar, orig)

	cwd, _ := os.Getwd()
	os.Setenv(DataDirEnvVar, "./engram")

	got := backend.DefaultDataDir()
	want := filepath.Join(cwd, "engram")
	if got != want {
		t.Errorf("DefaultDataDir() = %q, want %q", got, want)
	}
}

func TestLocalDataBackend_DefaultDataDir_AbsoluteUnchanged(t *testing.T) {
	backend := NewLocalDataBackend()
	orig := os.Getenv(DataDirEnvVar)
	defer os.Setenv(DataDirEnvVar, orig)

	os.Setenv(DataDirEnvVar, "/absolute/engram")

	got := backend.DefaultDataDir()
	if !strings.HasSuffix(got, filepath.FromSlash("/absolute/engram")) {
		t.Errorf("DefaultDataDir() = %q, want suffix %q", got, filepath.FromSlash("/absolute/engram"))
	}
}

func TestLocalDataBackend_DetectLockedData_LsofAvailableAndClosed(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix-only test")
	}

	backend := NewLocalDataBackend()
	tmp := t.TempDir()
	_ = os.WriteFile(filepath.Join(tmp, "engram.db"), []byte("data"), 0o644)

	locked, err := backend.DetectLockedData(tmp)
	if err != nil {
		t.Fatalf("DetectLockedData: %v", err)
	}
	if locked {
		t.Error("DetectLockedData = true, want false (no process has file open)")
	}
}

func TestLocalDataBackend_DetectLockedData_LsofMissing(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix-only test")
	}

	backend := NewLocalDataBackend()
	tmp := t.TempDir()
	_ = os.WriteFile(filepath.Join(tmp, "engram.db"), []byte("data"), 0o644)

	locked, err := backend.DetectLockedData(tmp)
	if err != nil {
		t.Fatalf("DetectLockedData: %v", err)
	}
	if locked {
		t.Error("DetectLockedData = true, want false")
	}
}

func TestLocalDataBackend_CleanData_DeletesAllFiles(t *testing.T) {
	backend := NewLocalDataBackend()
	dir := t.TempDir()

	for _, f := range []string{"engram.db", "engram.db-wal", "engram.db-shm"} {
		if err := os.WriteFile(filepath.Join(dir, f), []byte("data"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	if err := backend.CleanData(dir); err != nil {
		t.Fatalf("CleanData() error = %v", err)
	}

	for _, f := range []string{"engram.db", "engram.db-wal", "engram.db-shm"} {
		if _, err := os.Stat(filepath.Join(dir, f)); !os.IsNotExist(err) {
			t.Errorf("%s should have been deleted", f)
		}
	}
}

func TestLocalDataBackend_CleanData_MissingFilesIsNoop(t *testing.T) {
	backend := NewLocalDataBackend()
	dir := t.TempDir()

	if err := backend.CleanData(dir); err != nil {
		t.Fatalf("CleanData() on empty dir error = %v, want nil", err)
	}
}

func TestLocalDataBackend_CleanData_PartialFiles(t *testing.T) {
	backend := NewLocalDataBackend()
	dir := t.TempDir()

	if err := os.WriteFile(filepath.Join(dir, "engram.db"), []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := backend.CleanData(dir); err != nil {
		t.Fatalf("CleanData() error = %v", err)
	}

	if _, err := os.Stat(filepath.Join(dir, "engram.db")); !os.IsNotExist(err) {
		t.Error("engram.db should have been deleted")
	}
}

func TestLocalDataBackend_CleanData_PreservesOtherFiles(t *testing.T) {
	backend := NewLocalDataBackend()
	dir := t.TempDir()

	if err := os.WriteFile(filepath.Join(dir, "engram.db"), []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "other.txt"), []byte("keep me"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := backend.CleanData(dir); err != nil {
		t.Fatalf("CleanData() error = %v", err)
	}

	if _, err := os.Stat(filepath.Join(dir, "other.txt")); err != nil {
		t.Error("other.txt should not have been deleted")
	}
}

func TestLocalDataBackend_CleanData_NonExistentDir(t *testing.T) {
	backend := NewLocalDataBackend()
	err := backend.CleanData(filepath.Join(t.TempDir(), "does-not-exist"))
	if err == nil {
		t.Fatal("CleanData() on non-existent dir = nil, want error")
	}
}

func TestLocalDataBackend_EstimateMigration(t *testing.T) {
	backend := NewLocalDataBackend()
	source := t.TempDir()

	if err := os.WriteFile(filepath.Join(source, "engram.db"), []byte("main data"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "engram.db-wal"), []byte("wal data"), 0o644); err != nil {
		t.Fatal(err)
	}

	infos, total, err := backend.EstimateMigration(source)
	if err != nil {
		t.Fatalf("EstimateMigration() error = %v", err)
	}

	if len(infos) != 2 {
		t.Errorf("len(infos) = %d, want 2", len(infos))
	}
	if total != 17 {
		t.Errorf("total = %d, want 17", total)
	}

	found := make(map[string]bool)
	for _, fi := range infos {
		found[fi.Name] = true
	}
	if !found["engram.db"] {
		t.Error("missing engram.db in estimate")
	}
	if !found["engram.db-wal"] {
		t.Error("missing engram.db-wal in estimate")
	}
}

func TestLocalDataBackend_EstimateMigration_EmptySource(t *testing.T) {
	backend := NewLocalDataBackend()
	source := t.TempDir()

	infos, total, err := backend.EstimateMigration(source)
	if err != nil {
		t.Fatalf("EstimateMigration() error = %v", err)
	}

	if len(infos) != 0 {
		t.Errorf("len(infos) = %d, want 0", len(infos))
	}
	if total != 0 {
		t.Errorf("total = %d, want 0", total)
	}
}
