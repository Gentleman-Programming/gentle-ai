package engram

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// TestLitmus_MigrateFlow verifies the complete happy path:
// 1. Existing data at ~/.engram
// 2. User chooses Migrate to custom path
// 3. Data moves to custom path
// 4. engramEnvMap() returns the custom path for MCP configs
// 5. Re-running doesn't self-copy from already-migrated location
func TestLitmus_MigrateFlow(t *testing.T) {
	backend := NewLocalDataBackend()
	home := t.TempDir()
	// Override home dir so HardDefaultDataDir() returns our test dir.
	origHomeFn := userHomeDir
	userHomeDir = func() (string, error) { return home, nil }
	defer func() { userHomeDir = origHomeFn }()

	src := backend.HardDefaultDataDir() // = filepath.Join(home, ".engram")
	dst := filepath.Join(home, "custom-engram")

	// Simulate existing data at the hard default.
	os.MkdirAll(src, 0o755)
	for _, f := range []string{"engram.db", "engram.db-wal"} {
		if err := os.WriteFile(filepath.Join(src, f), []byte(f+" data"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// 1. Execute the full service flow (copy → persist → clean source).
	persister := NewLocalConfigPersister(home)
	service := NewDataDirService(backend, persister)
	result, err := service.Execute(ActionMigrate, dst)
	if err != nil {
		t.Fatalf("Execute(Migrate) error: %v", err)
	}
	if result.FilesMoved != 2 {
		t.Errorf("FilesMoved = %d, want 2", result.FilesMoved)
	}
	if result.BytesMoved == 0 {
		t.Error("BytesMoved = 0, want > 0")
	}

	// 2. Destination should have the files.
	for _, f := range []string{"engram.db", "engram.db-wal"} {
		if _, err := os.Stat(filepath.Join(dst, f)); err != nil {
			t.Errorf("destination missing %s: %v", f, err)
		}
	}

	// 3. Source should NOT have the files (source cleaned only after config persisted).
	for _, f := range []string{"engram.db", "engram.db-wal"} {
		if _, err := os.Stat(filepath.Join(src, f)); !os.IsNotExist(err) {
			t.Errorf("source still has %s after migration", f)
		}
	}

	// 4. Set ENGRAM_DATA_DIR to the new location (as the installer/TUI would).
	origEnv := os.Getenv(DataDirEnvVar)
	os.Setenv(DataDirEnvVar, dst)
	defer os.Setenv(DataDirEnvVar, origEnv)

	// 5. engramEnvMap() should return the custom dir.
	env := engramEnvMap()
	if env == nil {
		t.Fatal("engramEnvMap() = nil, want non-nil after migration")
	}
	if got := env["ENGRAM_DATA_DIR"]; got != dst {
		t.Errorf("engramEnvMap()[ENGRAM_DATA_DIR] = %q, want %q", got, dst)
	}

	// 6. DefaultDataDir() now returns the custom dir (for reading).
	if got := backend.DefaultDataDir(); got != dst {
		t.Errorf("DefaultDataDir() = %q, want %q", got, dst)
	}

	// 7. HardDefaultDataDir() still returns the ORIGINAL location.
	// This is the critical safety valve — re-running install won't
	// try to migrate from the already-migrated location.
	if got := backend.HardDefaultDataDir(); got == dst {
		t.Errorf("HardDefaultDataDir() = %q, should NOT match migrated dir %q", got, dst)
	}
	if got := backend.HardDefaultDataDir(); got != src {
		t.Errorf("HardDefaultDataDir() = %q, want %q", got, src)
	}

	// 8. DetectExistingData on HardDefaultDataDir should be FALSE
	// because the source was cleaned up after migration.
	if backend.DetectExistingData(backend.HardDefaultDataDir()) {
		t.Error("DetectExistingData(HardDefaultDataDir()) = true after migration; source should be empty")
	}
}

// TestLitmus_ReMigrationSafety verifies that after a first migration,
// a second run cannot accidentally self-copy from the already-migrated
// location. This was a real bug where DefaultDataDir() (which respects
// ENGRAM_DATA_DIR) was used as the migration source.
func TestLitmus_ReMigrationSafety(t *testing.T) {
	backend := NewLocalDataBackend()
	home := t.TempDir()
	origHomeFn := userHomeDir
	userHomeDir = func() (string, error) { return home, nil }
	defer func() { userHomeDir = origHomeFn }()

	src := backend.HardDefaultDataDir()
	dst := filepath.Join(home, "migrated-engram")

	// First migration: ~/.engram → ~/migrated-engram
	os.MkdirAll(src, 0o755)
	os.WriteFile(filepath.Join(src, "engram.db"), []byte("data"), 0o644)
	persister := NewLocalConfigPersister(home)
	service := NewDataDirService(backend, persister)
	if _, err := service.Execute(ActionMigrate, dst); err != nil {
		t.Fatal(err)
	}

	// Now ENGRAM_DATA_DIR points to migrated location.
	origEnv := os.Getenv(DataDirEnvVar)
	os.Setenv(DataDirEnvVar, dst)
	defer os.Setenv(DataDirEnvVar, origEnv)

	// Simulate user running installer again and choosing Migrate.
	// The installer MUST use HardDefaultDataDir() as source.
	// If it used DefaultDataDir(), it would copy from dst to dst2.
	// Since src is now empty, DetectExistingData(src) is false →
	// the Migrate option should NOT be shown.
	if backend.DetectExistingData(backend.HardDefaultDataDir()) {
		t.Error("Migrate option should NOT be shown after first migration (source is empty)")
	}

	// Even if somehow Migrate was triggered, the source is empty
	// so MigrateData would be a no-op (no files to copy).
	dst2 := filepath.Join(home, "another-location")
	if _, err := backend.MigrateData(backend.HardDefaultDataDir(), dst2); err != nil {
		t.Fatalf("re-migration error: %v", err)
	}
	// dst2 should NOT have engram.db because source was empty.
	if _, err := os.Stat(filepath.Join(dst2, "engram.db")); !os.IsNotExist(err) {
		t.Error("re-migration copied files from empty source — this should not happen")
	}
}

// TestLitmus_KeepDefaultProducesNoEnv verifies that when the user
// keeps the default location, engramEnvMap() returns nil so that
// no env key is injected into agent configs.
func TestLitmus_KeepDefaultProducesNoEnv(t *testing.T) {
	origEnv := os.Getenv(DataDirEnvVar)
	os.Unsetenv(DataDirEnvVar)
	defer os.Setenv(DataDirEnvVar, origEnv)

	if env := engramEnvMap(); env != nil {
		t.Errorf("engramEnvMap() = %v, want nil when using default dir", env)
	}
}

// TestLitmus_EngramServerJSONWithEnv verifies that the JSON output
// includes the env key when a custom directory is set.
func TestLitmus_EngramServerJSONWithEnv(t *testing.T) {
	home := t.TempDir()
	customDir := filepath.Join(home, "custom-engram")

	// Set custom dir.
	origEnv := os.Getenv(DataDirEnvVar)
	os.Setenv(DataDirEnvVar, customDir)
	defer os.Setenv(DataDirEnvVar, origEnv)

	jsonBytes := engramServerJSONWithCmd("engram")

	var cfg map[string]any
	if err := json.Unmarshal(jsonBytes, &cfg); err != nil {
		t.Fatalf("unmarshal JSON: %v", err)
	}

	// Must have env key.
	env, ok := cfg["env"].(map[string]any)
	if !ok {
		t.Fatalf("JSON missing env key; got: %s", string(jsonBytes))
	}
	if got := env["ENGRAM_DATA_DIR"]; got != customDir {
		t.Errorf("env.ENGRAM_DATA_DIR = %q, want %q", got, customDir)
	}
}

// TestLitmus_EngramServerJSONNoEnvForDefault verifies that the JSON
// output does NOT include env when using the default directory.
func TestLitmus_EngramServerJSONNoEnvForDefault(t *testing.T) {
	origEnv := os.Getenv(DataDirEnvVar)
	os.Unsetenv(DataDirEnvVar)
	defer os.Setenv(DataDirEnvVar, origEnv)

	jsonBytes := engramServerJSONWithCmd("engram")

	var cfg map[string]any
	if err := json.Unmarshal(jsonBytes, &cfg); err != nil {
		t.Fatalf("unmarshal JSON: %v", err)
	}

	if _, hasEnv := cfg["env"]; hasEnv {
		t.Errorf("JSON should NOT have env key for default dir; got: %s", string(jsonBytes))
	}
}

// TestLitmus_DiskSpaceBlocksMigration verifies that when disk space
// check fails, no files are copied and source remains intact.
func TestLitmus_DiskSpaceBlocksMigration(t *testing.T) {
	backend := NewLocalDataBackend()
	src := t.TempDir()
	dst := t.TempDir()

	os.MkdirAll(src, 0o755)
	os.WriteFile(filepath.Join(src, "engram.db"), []byte("sqlite data"), 0o644)

	// Mock the space checker to always fail.
	orig := requireFreeSpace
	requireFreeSpace = func(path string, minBytes uint64) error {
		return os.ErrInvalid // any error works
	}
	defer func() { requireFreeSpace = orig }()

	_, err := backend.MigrateData(src, dst)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	// Target should NOT have the file.
	if _, err := os.Stat(filepath.Join(dst, "engram.db")); !os.IsNotExist(err) {
		t.Error("target should not have engram.db when space check fails")
	}

	// Source should still have the file.
	if _, err := os.Stat(filepath.Join(src, "engram.db")); err != nil {
		t.Error("source file was removed despite migration failure")
	}
}
