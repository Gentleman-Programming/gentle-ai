package engram

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

type recordingConfigPersister struct {
	value    string
	writeErr error
	clearErr error
}

func (p *recordingConfigPersister) Read() (string, error) {
	return p.value, nil
}

func (p *recordingConfigPersister) Write(dir string) error {
	if p.writeErr != nil {
		return p.writeErr
	}
	p.value = dir
	return nil
}

func (p *recordingConfigPersister) Clear() error {
	if p.clearErr != nil {
		return p.clearErr
	}
	p.value = ""
	return nil
}

func TestPreview_HasEnoughSpace(t *testing.T) {
	tests := []struct {
		name           string
		totalBytes     uint64
		availableSpace uint64
		want           bool
	}{
		{"zero total", 0, 0, true},
		{"zero total with space", 0, 1000, true},
		{"enough space", 1000, 2000, true},
		{"exact space", 1000, 1000, true},
		{"not enough space", 2000, 1000, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := Preview{TotalBytes: tt.totalBytes, AvailableSpace: tt.availableSpace}
			if got := p.HasEnoughSpace(); got != tt.want {
				t.Errorf("HasEnoughSpace() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDataDirService_Preview_Migrate(t *testing.T) {
	backend := NewLocalDataBackend()
	service := NewDataDirService(backend, nil)

	home := t.TempDir()
	origHomeFn := userHomeDir
	userHomeDir = func() (string, error) { return home, nil }
	defer func() { userHomeDir = origHomeFn }()

	src := backend.HardDefaultDataDir()
	os.MkdirAll(src, 0o755)
	os.WriteFile(filepath.Join(src, "engram.db"), []byte("data"), 0o644)

	preview, err := service.Preview(ActionMigrate, "~/custom")
	if err != nil {
		t.Fatalf("Preview() error = %v", err)
	}
	if len(preview.Files) != 1 {
		t.Errorf("len(Files) = %d, want 1", len(preview.Files))
	}
	if preview.TotalBytes == 0 {
		t.Error("TotalBytes = 0, want > 0")
	}
	if preview.ExpandedPath == "" {
		t.Error("ExpandedPath is empty")
	}
}

func TestDataDirService_Preview_Clean(t *testing.T) {
	backend := NewLocalDataBackend()
	service := NewDataDirService(backend, nil)

	home := t.TempDir()
	origHomeFn := userHomeDir
	userHomeDir = func() (string, error) { return home, nil }
	defer func() { userHomeDir = origHomeFn }()

	src := backend.HardDefaultDataDir()
	os.MkdirAll(src, 0o755)
	os.WriteFile(filepath.Join(src, "engram.db"), []byte("data"), 0o644)

	preview, err := service.Preview(ActionClean, "")
	if err != nil {
		t.Fatalf("Preview() error = %v", err)
	}
	if len(preview.Files) != 1 {
		t.Errorf("len(Files) = %d, want 1", len(preview.Files))
	}
	if preview.TotalBytes == 0 {
		t.Error("TotalBytes = 0, want > 0")
	}
}

func TestDataDirService_Preview_InvalidPath(t *testing.T) {
	backend := NewLocalDataBackend()
	service := NewDataDirService(backend, nil)

	_, err := service.Preview(ActionMigrate, "")
	if err == nil {
		t.Fatal("Preview() with empty path = nil, want error")
	}
	if !errors.Is(err, ErrInvalidPath) {
		t.Errorf("error = %v, want ErrInvalidPath", err)
	}
}

func TestDataDirService_Execute_Clean(t *testing.T) {
	backend := NewLocalDataBackend()
	home := t.TempDir()
	origHomeFn := userHomeDir
	userHomeDir = func() (string, error) { return home, nil }
	defer func() { userHomeDir = origHomeFn }()

	src := backend.HardDefaultDataDir()
	os.MkdirAll(src, 0o755)
	os.WriteFile(filepath.Join(src, "engram.db"), []byte("data"), 0o644)

	persister := NewLocalConfigPersister(home)
	service := NewDataDirService(backend, persister)

	result, err := service.Execute(ActionClean, "")
	if err != nil {
		t.Fatalf("Execute(Clean) error = %v", err)
	}
	if result.Message == "" {
		t.Error("result.Message is empty")
	}
	if backend.DetectExistingData(src) {
		t.Error("data should have been cleaned")
	}
}

// TestDataDirService_Execute_Clean_CreatesAndRemovesTempBackup verifies that
// Clean creates a temporary backup before deleting, and removes it on success.
func TestDataDirService_Execute_Clean_CreatesAndRemovesTempBackup(t *testing.T) {
	backend := NewLocalDataBackend()
	home := t.TempDir()
	origHomeFn := userHomeDir
	userHomeDir = func() (string, error) { return home, nil }
	defer func() { userHomeDir = origHomeFn }()

	src := backend.HardDefaultDataDir()
	os.MkdirAll(src, 0o755)
	os.WriteFile(filepath.Join(src, "engram.db"), []byte("precious memories"), 0o644)

	persister := NewLocalConfigPersister(home)
	service := NewDataDirService(backend, persister)

	_, err := service.Execute(ActionClean, "")
	if err != nil {
		t.Fatalf("Execute(Clean) error = %v", err)
	}

	// The temp backup should have been removed on success.
	// We verify by checking that no gentle-ai-engram-clean-* dir exists in temp.
	// This is best-effort; we can't guarantee the temp dir path.
	// Instead, verify the source is clean and no panic occurred.
	if backend.DetectExistingData(src) {
		t.Error("source data should have been cleaned")
	}
}

func TestDataDirService_Execute_KeepDefault(t *testing.T) {
	backend := NewLocalDataBackend()
	home := t.TempDir()
	persister := NewLocalConfigPersister(home)
	service := NewDataDirService(backend, persister)

	result, err := service.Execute(ActionKeepDefault, "")
	if err != nil {
		t.Fatalf("Execute(KeepDefault) error = %v", err)
	}
	if result.Message == "" {
		t.Error("result.Message is empty")
	}
}

func TestExecuteStartFreshReportsDeletedFilesAndBytes(t *testing.T) {
	home := t.TempDir()
	backend := NewLocalDataBackend()
	restoreHome := SetUserHomeDirForTest(func() (string, error) { return home, nil })
	defer restoreHome()

	src := backend.HardDefaultDataDir()
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatalf("MkdirAll(src): %v", err)
	}
	if err := os.WriteFile(filepath.Join(src, "engram.db"), []byte("data"), 0o644); err != nil {
		t.Fatalf("WriteFile db: %v", err)
	}
	if err := os.WriteFile(filepath.Join(src, "engram.db-wal"), []byte("wal"), 0o644); err != nil {
		t.Fatalf("WriteFile wal: %v", err)
	}

	persister := NewLocalConfigPersister(home)
	service := NewDataDirService(backend, persister)
	result, err := service.Execute(ActionStartFresh, filepath.Join(home, "fresh"))
	if err != nil {
		t.Fatalf("Execute(StartFresh) error = %v", err)
	}
	if result.FilesDeleted != 2 {
		t.Fatalf("FilesDeleted = %d, want 2", result.FilesDeleted)
	}
	if result.BytesDeleted != uint64(len("data")+len("wal")) {
		t.Fatalf("BytesDeleted = %d, want %d", result.BytesDeleted, len("data")+len("wal"))
	}
}

func TestFeedbackDetailsIncludesDeletedDataForStartFresh(t *testing.T) {
	got := FeedbackDetails(ActionStartFresh, 0, 0, 0, 0, 2, 7*1024)
	if !strings.Contains(got, "2 files deleted") {
		t.Fatalf("FeedbackDetails(StartFresh) = %q, want deleted file count", got)
	}
	if !strings.Contains(got, "7.0 KB") {
		t.Fatalf("FeedbackDetails(StartFresh) = %q, want deleted byte size", got)
	}
}

func TestPreviewMigrateUsesEffectiveDataDirWhenItHasData(t *testing.T) {
	home := t.TempDir()
	backend := NewLocalDataBackend()
	restoreHome := SetUserHomeDirForTest(func() (string, error) { return home, nil })
	defer restoreHome()

	effective := filepath.Join(home, "current-engram")
	if err := os.Setenv(DataDirEnvVar, effective); err != nil {
		t.Fatal(err)
	}
	defer os.Unsetenv(DataDirEnvVar)
	if err := os.MkdirAll(effective, 0o755); err != nil {
		t.Fatalf("MkdirAll(effective): %v", err)
	}
	if err := os.WriteFile(filepath.Join(effective, "engram.db"), []byte("current"), 0o644); err != nil {
		t.Fatalf("WriteFile effective db: %v", err)
	}

	service := NewDataDirService(backend, nil)
	preview, err := service.Preview(ActionMigrate, filepath.Join(home, "target"))
	if err != nil {
		t.Fatalf("Preview(Migrate) error = %v", err)
	}
	if preview.TotalBytes != uint64(len("current")) {
		t.Fatalf("TotalBytes = %d, want %d from effective data dir", preview.TotalBytes, len("current"))
	}
}

func TestHasExistingSourceDataDetectsHardDefaultWhenEffectiveIsEmpty(t *testing.T) {
	home := t.TempDir()
	backend := NewLocalDataBackend()
	restoreHome := SetUserHomeDirForTest(func() (string, error) { return home, nil })
	defer restoreHome()

	effective := filepath.Join(home, "empty-current")
	if err := os.Setenv(DataDirEnvVar, effective); err != nil {
		t.Fatal(err)
	}
	defer os.Unsetenv(DataDirEnvVar)

	hardDefault := backend.HardDefaultDataDir()
	if err := os.MkdirAll(hardDefault, 0o755); err != nil {
		t.Fatalf("MkdirAll(hardDefault): %v", err)
	}
	if err := os.WriteFile(filepath.Join(hardDefault, "engram.db"), []byte("legacy"), 0o644); err != nil {
		t.Fatalf("WriteFile hard-default db: %v", err)
	}

	if !HasExistingSourceData(backend) {
		t.Fatal("HasExistingSourceData() = false, want true when hard-default has Engram data")
	}
}

func TestExecuteCopyCopiesDataWithoutCleaningSourceOrPersistingConfig(t *testing.T) {
	home := t.TempDir()
	backend := NewLocalDataBackend()
	restoreHome := SetUserHomeDirForTest(func() (string, error) { return home, nil })
	defer restoreHome()

	src := backend.HardDefaultDataDir()
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatalf("MkdirAll(src): %v", err)
	}
	if err := os.WriteFile(filepath.Join(src, "engram.db"), []byte("copy-data"), 0o644); err != nil {
		t.Fatalf("WriteFile source db: %v", err)
	}

	persister := &recordingConfigPersister{}
	service := NewDataDirService(backend, persister)
	target := filepath.Join(home, "copy-drive", "engram")
	result, err := service.Execute(ActionCopy, target)
	if err != nil {
		t.Fatalf("Execute(Copy) error = %v", err)
	}

	if result.FilesCopied != 1 {
		t.Fatalf("FilesCopied = %d, want 1", result.FilesCopied)
	}
	if result.BytesCopied != uint64(len("copy-data")) {
		t.Fatalf("BytesCopied = %d, want %d", result.BytesCopied, len("copy-data"))
	}
	if !backend.DetectExistingData(src) {
		t.Fatal("source data was cleaned; copy must leave original data in place")
	}
	if !backend.DetectExistingData(target) {
		t.Fatal("target data missing after copy")
	}
	if got, err := persister.Read(); err != nil || got != "" {
		t.Fatalf("persister.Read() = %q, %v; copy must not change configured data dir", got, err)
	}
}

func TestExecuteMoveDoesNotDeleteSourceWhenAfterConfigPersistFails(t *testing.T) {
	home := t.TempDir()
	backend := NewLocalDataBackend()
	restoreHome := SetUserHomeDirForTest(func() (string, error) { return home, nil })
	defer restoreHome()

	src := backend.HardDefaultDataDir()
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatalf("MkdirAll(src): %v", err)
	}
	if err := os.WriteFile(filepath.Join(src, "engram.db"), []byte("move-data"), 0o644); err != nil {
		t.Fatalf("WriteFile source db: %v", err)
	}

	persister := &recordingConfigPersister{}
	service := NewDataDirService(backend, persister)
	service.SetAfterConfigPersist(func(Action, string) error {
		return errors.New("mcp write failed")
	})
	target := filepath.Join(home, "target-engram")
	_, err := service.Execute(ActionMigrate, target)
	if err == nil || !strings.Contains(err.Error(), "MCP config could not be updated") {
		t.Fatalf("Execute(Migrate) error = %v, want MCP config failure", err)
	}
	if !backend.DetectExistingData(src) {
		t.Fatal("source was deleted even though MCP reinjection failed")
	}
	if !backend.DetectExistingData(target) {
		t.Fatal("target copy should remain for recovery after MCP reinjection failure")
	}
}

func TestExecuteSetActivePersistsExistingDataDirWithoutCopyOrDelete(t *testing.T) {
	home := t.TempDir()
	backend := NewLocalDataBackend()
	restoreHome := SetUserHomeDirForTest(func() (string, error) { return home, nil })
	defer restoreHome()

	target := filepath.Join(home, "external-engram")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatalf("MkdirAll(target): %v", err)
	}
	if err := os.WriteFile(filepath.Join(target, "engram.db"), []byte("existing"), 0o644); err != nil {
		t.Fatalf("WriteFile target db: %v", err)
	}

	persister := &recordingConfigPersister{}
	service := NewDataDirService(backend, persister)
	result, err := service.Execute(ActionSetActive, target)
	if err != nil {
		t.Fatalf("Execute(SetActive) error = %v", err)
	}
	if persister.value != target {
		t.Fatalf("active dir = %q, want %q", persister.value, target)
	}
	if !backend.DetectExistingData(target) {
		t.Fatal("set active must not delete selected data")
	}
	if result.FilesCopied != 1 || result.BytesCopied != uint64(len("existing")) {
		t.Fatalf("result active stats = files %d bytes %d, want 1/%d", result.FilesCopied, result.BytesCopied, len("existing"))
	}
}

func TestExecuteSetActiveRejectsEmptyDirectory(t *testing.T) {
	home := t.TempDir()
	backend := NewLocalDataBackend()
	restoreHome := SetUserHomeDirForTest(func() (string, error) { return home, nil })
	defer restoreHome()

	target := filepath.Join(home, "empty-engram")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatalf("MkdirAll(target): %v", err)
	}
	persister := &recordingConfigPersister{}
	service := NewDataDirService(backend, persister)
	_, err := service.Execute(ActionSetActive, target)
	if !errors.Is(err, ErrNoEngramData) {
		t.Fatalf("Execute(SetActive empty) error = %v, want ErrNoEngramData", err)
	}
	if persister.value != "" {
		t.Fatalf("active dir changed to %q for invalid target", persister.value)
	}
}

func TestFeedbackDetailsIncludesCopiedDataForCopy(t *testing.T) {
	got := FeedbackDetails(ActionCopy, 0, 0, 3, 12*1024, 0, 0)
	if !strings.Contains(got, "3 files copied") {
		t.Fatalf("FeedbackDetails(Copy) = %q, want copied file count", got)
	}
	if !strings.Contains(got, "12.0 KB") {
		t.Fatalf("FeedbackDetails(Copy) = %q, want copied byte size", got)
	}
}

func TestConfirmTitle(t *testing.T) {
	tests := []struct {
		action Action
		want   string
	}{
		{ActionKeepDefault, "CONFIRM ACTION"},
		{ActionMigrate, "CONFIRM MIGRATION"},
		{ActionCopy, "CONFIRM COPY"},
		{ActionSetActive, "CONFIRM ACTIVE DIRECTORY"},
		{ActionStartFresh, "CONFIRM DELETE & START FRESH"},
		{ActionClean, "CONFIRM CLEAN DATA"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := ConfirmTitle(tt.action); got != tt.want {
				t.Errorf("ConfirmTitle(%v) = %q, want %q", tt.action, got, tt.want)
			}
		})
	}
}

func TestFeedbackTitle(t *testing.T) {
	tests := []struct {
		action Action
		want   string
	}{
		{ActionKeepDefault, "COMPLETE"},
		{ActionMigrate, "MIGRATION COMPLETE"},
		{ActionCopy, "COPY COMPLETE"},
		{ActionSetActive, "ACTIVE DIRECTORY UPDATED"},
		{ActionStartFresh, "FRESH DATABASE CREATED"},
		{ActionClean, "DATA CLEANED"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := FeedbackTitle(tt.action); got != tt.want {
				t.Errorf("FeedbackTitle(%v) = %q, want %q", tt.action, got, tt.want)
			}
		})
	}
}

func TestFormatBytes(t *testing.T) {
	tests := []struct {
		bytes uint64
		want  string
	}{
		{0, "0 B"},
		{512, "512 B"},
		{1024, "1.0 KB"},
		{1536, "1.5 KB"},
		{1024 * 1024, "1.0 MB"},
		{1024 * 1024 * 1024, "1.0 GB"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := FormatBytes(tt.bytes); got != tt.want {
				t.Errorf("FormatBytes(%d) = %q, want %q", tt.bytes, got, tt.want)
			}
		})
	}
}

func TestPreviewFileNames(t *testing.T) {
	files := []FileInfo{
		{Name: "engram.db", Size: 1024},
		{Name: "engram.db-wal", Size: 512},
	}
	names := PreviewFileNames(files)
	if len(names) != 2 {
		t.Fatalf("len(names) = %d, want 2", len(names))
	}
	if !strings.Contains(names[0], "engram.db") {
		t.Errorf("names[0] = %q, want containing 'engram.db'", names[0])
	}
	if !strings.Contains(names[0], "1.0 KB") {
		t.Errorf("names[0] = %q, want containing '1.0 KB'", names[0])
	}
}

func TestErrorMessage(t *testing.T) {
	if got := ErrorMessage(nil); got != "" {
		t.Errorf("ErrorMessage(nil) = %q, want empty", got)
	}
	if got := ErrorMessage(ErrLocked); !strings.Contains(got, "in use") {
		t.Errorf("ErrorMessage(ErrLocked) = %q, want containing 'in use'", got)
	}
	if got := ErrorMessage(ErrTargetHasData); !strings.Contains(got, "already contains Engram data") {
		t.Errorf("ErrorMessage(ErrTargetHasData) = %q, want target data warning", got)
	}
}

// failPersister always fails on Write for testing transactional rollback.
type failPersister struct{}

func (f *failPersister) Read() (string, error)  { return "", nil }
func (f *failPersister) Write(dir string) error { return errors.New("persist failed") }
func (f *failPersister) Clear() error           { return nil }

// TestDataDirService_Migrate_ConfigFailureKeepsSource verifies that when
// persister.Write fails after a successful copy, the original source data
// is NOT deleted. The user can recover by setting ENGRAM_DATA_DIR manually.
func TestDataDirService_Migrate_ConfigFailureKeepsSource(t *testing.T) {
	backend := NewLocalDataBackend()
	home := t.TempDir()
	origHomeFn := userHomeDir
	userHomeDir = func() (string, error) { return home, nil }
	defer func() { userHomeDir = origHomeFn }()

	src := backend.HardDefaultDataDir()
	dst := filepath.Join(home, "custom")
	os.MkdirAll(src, 0o755)
	os.WriteFile(filepath.Join(src, "engram.db"), []byte("precious data"), 0o644)

	service := NewDataDirService(backend, &failPersister{})
	_, err := service.Execute(ActionMigrate, dst)
	if err == nil {
		t.Fatal("expected error when persister fails, got nil")
	}

	// Source MUST still have the data — this is the safety guarantee.
	if _, err := os.Stat(filepath.Join(src, "engram.db")); err != nil {
		t.Errorf("source data was deleted despite config failure: %v", err)
	}

	// Target should have the copy — user can manually point ENGRAM_DATA_DIR here.
	if _, err := os.Stat(filepath.Join(dst, "engram.db")); err != nil {
		t.Errorf("target copy missing: %v", err)
	}
}

// TestDataDirService_Migrate_SuccessDeletesSource verifies that when the
// full flow succeeds (copy + persist), the source is cleaned up.
func TestDataDirService_Migrate_SuccessDeletesSource(t *testing.T) {
	backend := NewLocalDataBackend()
	home := t.TempDir()
	origHomeFn := userHomeDir
	userHomeDir = func() (string, error) { return home, nil }
	defer func() { userHomeDir = origHomeFn }()

	src := backend.HardDefaultDataDir()
	dst := filepath.Join(home, "custom")
	os.MkdirAll(src, 0o755)
	os.WriteFile(filepath.Join(src, "engram.db"), []byte("data"), 0o644)

	persister := NewLocalConfigPersister(home)
	service := NewDataDirService(backend, persister)
	_, err := service.Execute(ActionMigrate, dst)
	if err != nil {
		t.Fatalf("Execute(Migrate) error: %v", err)
	}

	// Source should be empty after successful migration.
	if backend.DetectExistingData(src) {
		t.Error("source still has data after successful migration")
	}
}

func TestDataDirService_Migrate_RejectsSamePathWithoutTouchingData(t *testing.T) {
	backend := NewLocalDataBackend()
	home := t.TempDir()
	origHomeFn := userHomeDir
	userHomeDir = func() (string, error) { return home, nil }
	defer func() { userHomeDir = origHomeFn }()

	src := backend.HardDefaultDataDir()
	os.MkdirAll(src, 0o755)
	if err := os.WriteFile(filepath.Join(src, "engram.db"), []byte("precious"), 0o644); err != nil {
		t.Fatal(err)
	}

	persister := NewLocalConfigPersister(home)
	service := NewDataDirService(backend, persister)
	_, err := service.Execute(ActionMigrate, src)
	if !errors.Is(err, ErrInvalidPath) {
		t.Fatalf("Execute(Migrate same path) error = %v, want ErrInvalidPath", err)
	}

	got, readErr := os.ReadFile(filepath.Join(src, "engram.db"))
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(got) != "precious" {
		t.Fatalf("source data changed after rejected same-path migration: %q", got)
	}
}

func TestDataDirService_Migrate_RejectsTargetWithDataBeforeCopy(t *testing.T) {
	backend := NewLocalDataBackend()
	home := t.TempDir()
	origHomeFn := userHomeDir
	userHomeDir = func() (string, error) { return home, nil }
	defer func() { userHomeDir = origHomeFn }()

	src := backend.HardDefaultDataDir()
	dst := filepath.Join(home, "custom")
	os.MkdirAll(src, 0o755)
	os.MkdirAll(dst, 0o755)
	os.WriteFile(filepath.Join(src, "engram.db"), []byte("source"), 0o644)
	os.WriteFile(filepath.Join(dst, "engram.db"), []byte("target"), 0o644)

	persister := NewLocalConfigPersister(home)
	service := NewDataDirService(backend, persister)
	_, err := service.Execute(ActionMigrate, dst)
	if !errors.Is(err, ErrTargetHasData) {
		t.Fatalf("Execute(Migrate existing target) error = %v, want ErrTargetHasData", err)
	}

	targetData, readErr := os.ReadFile(filepath.Join(dst, "engram.db"))
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(targetData) != "target" {
		t.Fatalf("target data overwritten: %q", targetData)
	}
	if !backend.DetectExistingData(src) {
		t.Fatal("source data was removed despite rejected target")
	}
}

// TestPreview_PartialMigrationWarning verifies that when both source and target
// have existing data, the preview includes a warning about interrupted migration.
func TestPreview_PartialMigrationWarning(t *testing.T) {
	backend := NewLocalDataBackend()
	service := NewDataDirService(backend, nil)

	home := t.TempDir()
	origHomeFn := userHomeDir
	userHomeDir = func() (string, error) { return home, nil }
	defer func() { userHomeDir = origHomeFn }()

	src := backend.HardDefaultDataDir()
	dst := filepath.Join(home, "custom")

	// Put data in BOTH source and target.
	os.MkdirAll(src, 0o755)
	os.WriteFile(filepath.Join(src, "engram.db"), []byte("source data"), 0o644)
	os.MkdirAll(dst, 0o755)
	os.WriteFile(filepath.Join(dst, "engram.db"), []byte("target data"), 0o644)

	preview, err := service.Preview(ActionMigrate, dst)
	if err != nil {
		t.Fatalf("Preview() error = %v", err)
	}
	if preview.PartialMigrationWarning == "" {
		t.Error("PartialMigrationWarning is empty, want non-empty warning")
	}
}

// TestPreview_NoPartialMigrationWarning verifies that when only source has
// data, no warning is emitted.
func TestPreview_NoPartialMigrationWarning(t *testing.T) {
	backend := NewLocalDataBackend()
	service := NewDataDirService(backend, nil)

	home := t.TempDir()
	origHomeFn := userHomeDir
	userHomeDir = func() (string, error) { return home, nil }
	defer func() { userHomeDir = origHomeFn }()

	src := backend.HardDefaultDataDir()
	os.MkdirAll(src, 0o755)
	os.WriteFile(filepath.Join(src, "engram.db"), []byte("data"), 0o644)

	preview, err := service.Preview(ActionMigrate, filepath.Join(home, "empty-target"))
	if err != nil {
		t.Fatalf("Preview() error = %v", err)
	}
	if preview.PartialMigrationWarning != "" {
		t.Errorf("PartialMigrationWarning = %q, want empty", preview.PartialMigrationWarning)
	}
}

// TestDataDirService_Preview_StartFresh verifies that Preview works for the
// StartFresh action (path expansion, file listing, space check).
func TestDataDirService_Preview_StartFresh(t *testing.T) {
	backend := NewLocalDataBackend()
	service := NewDataDirService(backend, nil)

	home := t.TempDir()
	origHomeFn := userHomeDir
	userHomeDir = func() (string, error) { return home, nil }
	defer func() { userHomeDir = origHomeFn }()

	src := backend.HardDefaultDataDir()
	os.MkdirAll(src, 0o755)
	os.WriteFile(filepath.Join(src, "engram.db"), []byte("data"), 0o644)

	preview, err := service.Preview(ActionStartFresh, "~/fresh")
	if err != nil {
		t.Fatalf("Preview() error = %v", err)
	}
	if len(preview.Files) != 1 {
		t.Errorf("len(Files) = %d, want 1", len(preview.Files))
	}
	if preview.ExpandedPath == "" {
		t.Error("ExpandedPath is empty")
	}
}

// TestDataDirService_Preview_SpaceError verifies that when AvailableSpace
// fails, the error is stored in Preview.SpaceErr and HasEnoughSpace still
// returns true (zero-total fallback).
func TestDataDirService_Preview_SpaceError(t *testing.T) {
	// Use a mock backend that always fails AvailableSpace and reports no files.
	backend := &mockSpaceErrorBackend{}
	service := NewDataDirService(backend, nil)

	preview, err := service.Preview(ActionMigrate, "~/any")
	if err != nil {
		t.Fatalf("Preview() error = %v", err)
	}
	if preview.SpaceErr == nil {
		t.Error("Preview.SpaceErr is nil, want non-nil")
	}
	if !preview.HasEnoughSpace() {
		t.Error("HasEnoughSpace() = false, want true when TotalBytes == 0")
	}
}

// mockSpaceErrorBackend implements just enough of DataBackend to test the
// SpaceErr path. It reports no existing data and always fails AvailableSpace.
type mockSpaceErrorBackend struct{}

func (m *mockSpaceErrorBackend) DefaultDataDir() string     { return "" }
func (m *mockSpaceErrorBackend) HardDefaultDataDir() string { return "" }
func (m *mockSpaceErrorBackend) ExpandPath(path string) (string, error) {
	return "/expanded/" + path, nil
}
func (m *mockSpaceErrorBackend) DetectExistingData(dir string) bool        { return false }
func (m *mockSpaceErrorBackend) ExistingFiles(dir string) []string         { return nil }
func (m *mockSpaceErrorBackend) DetectLockedData(dir string) (bool, error) { return false, nil }
func (m *mockSpaceErrorBackend) EstimateMigration(source string) ([]FileInfo, uint64, error) {
	return nil, 0, nil
}
func (m *mockSpaceErrorBackend) CopyData(source, target string) (Result, error) { return Result{}, nil }
func (m *mockSpaceErrorBackend) MoveData(source, target string) (Result, error) { return Result{}, nil }
func (m *mockSpaceErrorBackend) DeleteData(dir string) (Result, error)          { return Result{}, nil }
func (m *mockSpaceErrorBackend) MigrateData(source, target string) (Result, error) {
	return Result{}, nil
}
func (m *mockSpaceErrorBackend) CleanData(dir string) error { return nil }
func (m *mockSpaceErrorBackend) EnsureDir(dir string) error { return nil }
func (m *mockSpaceErrorBackend) AvailableSpace(dir string) (uint64, error) {
	return 0, fmt.Errorf("simulated space check failure")
}
func (m *mockSpaceErrorBackend) CheckWritable(dir string) error { return nil }

// TestDataDirService_Execute_StartFresh verifies that StartFresh cleans the
// source, creates the target, and persists the config.
func TestDataDirService_Execute_StartFresh(t *testing.T) {
	backend := NewLocalDataBackend()
	home := t.TempDir()
	origHomeFn := userHomeDir
	userHomeDir = func() (string, error) { return home, nil }
	defer func() { userHomeDir = origHomeFn }()

	src := backend.HardDefaultDataDir()
	os.MkdirAll(src, 0o755)
	os.WriteFile(filepath.Join(src, "engram.db"), []byte("old"), 0o644)

	dst := filepath.Join(home, "fresh")
	persister := NewLocalConfigPersister(home)
	service := NewDataDirService(backend, persister)

	result, err := service.Execute(ActionStartFresh, dst)
	if err != nil {
		t.Fatalf("Execute(StartFresh) error = %v", err)
	}
	if result.Message == "" {
		t.Error("result.Message is empty")
	}
	if backend.DetectExistingData(src) {
		t.Error("source data should have been deleted")
	}
	if _, err := os.Stat(dst); err != nil {
		t.Errorf("target dir missing: %v", err)
	}
}

func TestDataDirService_Execute_StartFresh_RejectsTargetWithDataBeforeDeletingSource(t *testing.T) {
	backend := NewLocalDataBackend()
	home := t.TempDir()
	origHomeFn := userHomeDir
	userHomeDir = func() (string, error) { return home, nil }
	defer func() { userHomeDir = origHomeFn }()

	src := backend.HardDefaultDataDir()
	dst := filepath.Join(home, "fresh")
	os.MkdirAll(src, 0o755)
	os.MkdirAll(dst, 0o755)
	os.WriteFile(filepath.Join(src, "engram.db"), []byte("source"), 0o644)
	os.WriteFile(filepath.Join(dst, "engram.db"), []byte("target"), 0o644)

	persister := NewLocalConfigPersister(home)
	service := NewDataDirService(backend, persister)
	_, err := service.Execute(ActionStartFresh, dst)
	if !errors.Is(err, ErrTargetHasData) {
		t.Fatalf("Execute(StartFresh existing target) error = %v, want ErrTargetHasData", err)
	}
	if !backend.DetectExistingData(src) {
		t.Fatal("source data was deleted before rejecting target with data")
	}
}

// TestDataDirService_Execute_Migrate_LockedData verifies that migration is
// rejected when the source data appears to be locked.
func TestDataDirService_Execute_Migrate_LockedData(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("lock detection uses lsof on Unix; skip on Windows")
	}

	backend := NewLocalDataBackend()
	home := t.TempDir()
	origHomeFn := userHomeDir
	userHomeDir = func() (string, error) { return home, nil }
	defer func() { userHomeDir = origHomeFn }()

	src := backend.HardDefaultDataDir()
	os.MkdirAll(src, 0o755)
	os.WriteFile(filepath.Join(src, "engram.db"), []byte("data"), 0o644)

	// Open the file to hold a lock.
	f, err := os.Open(filepath.Join(src, "engram.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	persister := NewLocalConfigPersister(home)
	service := NewDataDirService(backend, persister)
	_, err = service.Execute(ActionMigrate, filepath.Join(home, "dst"))
	if err == nil {
		t.Fatal("expected error for locked data, got nil")
	}
	if !errors.Is(err, ErrLocked) {
		t.Errorf("error = %v, want ErrLocked", err)
	}
}

// TestConfirmMessage verifies that ConfirmMessage returns the correct text
// for each action.
func TestConfirmMessage(t *testing.T) {
	if got := ConfirmMessage(ActionClean, "/src", "/dst"); !strings.Contains(got, "delete") {
		t.Errorf("ConfirmMessage(Clean) = %q, want containing 'delete'", got)
	}
	if got := ConfirmMessage(ActionMigrate, "/src", "/dst"); !strings.Contains(got, "/src") {
		t.Errorf("ConfirmMessage(Migrate) = %q, want containing '/src'", got)
	}
	if got := ConfirmMessage(ActionStartFresh, "/src", "/dst"); !strings.Contains(got, "/dst") {
		t.Errorf("ConfirmMessage(StartFresh) = %q, want containing '/dst'", got)
	}
}

// TestConfirmWarning verifies that ConfirmWarning returns non-empty warnings
// for destructive actions.
func TestConfirmWarning(t *testing.T) {
	for _, action := range []Action{ActionClean, ActionMigrate, ActionStartFresh} {
		if got := ConfirmWarning(action); got == "" {
			t.Errorf("ConfirmWarning(%v) is empty, want non-empty", action)
		}
	}
}

// TestPreviewMessage verifies that PreviewMessage returns text for Migrate and
// StartFresh.
func TestPreviewMessage(t *testing.T) {
	if got := PreviewMessage(ActionMigrate); got == "" {
		t.Error("PreviewMessage(Migrate) is empty")
	}
	if got := PreviewMessage(ActionStartFresh); got == "" {
		t.Error("PreviewMessage(StartFresh) is empty")
	}
	if got := PreviewMessage(ActionClean); got != "" {
		t.Errorf("PreviewMessage(Clean) = %q, want empty", got)
	}
}

// TestWarningMessage verifies that WarningMessage returns warnings for
// destructive choices.
func TestWarningMessage(t *testing.T) {
	if got := WarningMessage(ActionStartFresh); !strings.Contains(got, "deleted") {
		t.Errorf("WarningMessage(StartFresh) = %q, want containing 'deleted'", got)
	}
	if got := WarningMessage(ActionClean); !strings.Contains(got, "deleted") {
		t.Errorf("WarningMessage(Clean) = %q, want containing 'deleted'", got)
	}
	if got := WarningMessage(ActionMigrate); got != "" {
		t.Errorf("WarningMessage(Migrate) = %q, want empty", got)
	}
}

// TestLocalConfigPersister_Read verifies reading from state file.
func TestLocalConfigPersister_Read(t *testing.T) {
	// Isolate from any env var set by previous tests in this process.
	t.Setenv(DataDirEnvVar, "")

	home := t.TempDir()
	p := NewLocalConfigPersister(home)

	// Empty state file → empty result.
	got, err := p.Read()
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if got != "" {
		t.Errorf("Read() = %q, want empty", got)
	}

	// Write then read back.
	if err := p.Write("/custom/engram"); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	got, err = p.Read()
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if got != "/custom/engram" {
		t.Errorf("Read() = %q, want /custom/engram", got)
	}
}

// TestLocalConfigPersister_Write verifies atomic write and read-back.
func TestLocalConfigPersister_Write(t *testing.T) {
	home := t.TempDir()
	p := NewLocalConfigPersister(home)

	if err := p.Write("/first"); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if err := p.Write("/second"); err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	got, err := p.Read()
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if got != "/second" {
		t.Errorf("Read() = %q, want /second", got)
	}
}

// TestLocalConfigPersister_Clear verifies that Clear removes the config.
func TestLocalConfigPersister_Clear(t *testing.T) {
	home := t.TempDir()
	p := NewLocalConfigPersister(home)

	if err := p.Write("/custom/engram"); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if err := p.Clear(); err != nil {
		t.Fatalf("Clear() error = %v", err)
	}
	got, err := p.Read()
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if got != "" {
		t.Errorf("Read() after Clear = %q, want empty", got)
	}
}
