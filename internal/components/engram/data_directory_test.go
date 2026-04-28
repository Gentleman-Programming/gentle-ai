package engram

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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

func TestConfirmTitle(t *testing.T) {
	tests := []struct {
		action Action
		want   string
	}{
		{ActionKeepDefault, "CONFIRM ACTION"},
		{ActionMigrate, "CONFIRM MIGRATION"},
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
