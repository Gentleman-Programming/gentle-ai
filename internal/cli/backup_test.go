package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gentleman-programming/gentle-ai/v2/internal/backup"
)

func createTestBackup(t *testing.T, homeDir string, id string, createdAt time.Time, source backup.BackupSource, desc string, pinned bool) {
	t.Helper()
	setupTestBackupEnv(t, homeDir)
	bDir := filepath.Join(homeDir, ".gentle-ai", "backups", id)
	m := backup.Manifest{
		ID:          id,
		CreatedAt:   createdAt,
		RootDir:     bDir,
		Source:      source,
		Description: desc,
		FileCount:   1,
		Pinned:      pinned,
	}
	if err := backup.WriteManifest(filepath.Join(bDir, backup.ManifestFilename), m); err != nil {
		t.Fatalf("WriteManifest(%s) error = %v", id, err)
	}
	if err := os.WriteFile(filepath.Join(bDir, "test.dat"), []byte("sample-data"), 0o644); err != nil {
		t.Fatalf("WriteFile test.dat error = %v", err)
	}
}

func setupTestBackupEnv(t *testing.T, home string) {
	t.Helper()
	orig := backup.BackupRootFn
	t.Cleanup(func() { backup.BackupRootFn = orig })
	backup.BackupRootFn = func() (string, error) {
		return filepath.Join(home, ".gentle-ai", "backups"), nil
	}
}

func TestRunBackup_Help(t *testing.T) {
	var out bytes.Buffer
	err := RunBackupWithInput([]string{"--help"}, &out, strings.NewReader(""), t.TempDir())
	if err != nil {
		t.Fatalf("RunBackupWithInput(--help) error = %v", err)
	}
	output := out.String()
	if !strings.Contains(output, "Usage: gentle-ai backup") {
		t.Errorf("expected help usage in output, got: %s", output)
	}
	if !strings.Contains(output, "list, ls") || !strings.Contains(output, "clean") {
		t.Errorf("expected subcommands in output, got: %s", output)
	}
}

func TestRunBackup_UnknownSubcommand(t *testing.T) {
	var out bytes.Buffer
	err := RunBackupWithInput([]string{"unknown-cmd"}, &out, strings.NewReader(""), t.TempDir())
	if err == nil {
		t.Fatalf("expected error for unknown subcommand, got nil")
	}
	if !strings.Contains(err.Error(), "unknown backup subcommand") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestRunBackupList_Empty(t *testing.T) {
	home := t.TempDir()
	var out bytes.Buffer
	err := RunBackupWithInput([]string{"list"}, &out, strings.NewReader(""), home)
	if err != nil {
		t.Fatalf("RunBackupWithInput(list) error = %v", err)
	}
	if !strings.Contains(out.String(), "No backups found") {
		t.Errorf("expected 'No backups found' message, got: %s", out.String())
	}
}

func TestRunBackupList_WithBackups(t *testing.T) {
	home := t.TempDir()
	t1 := time.Date(2026, 8, 1, 10, 0, 0, 0, time.UTC)
	t2 := time.Date(2026, 8, 2, 10, 0, 0, 0, time.UTC)

	createTestBackup(t, home, "backup-1", t1, backup.BackupSourceInstall, "pre-install", false)
	createTestBackup(t, home, "backup-2", t2, backup.BackupSourceUpgrade, "upgrade-v2", true)

	var out bytes.Buffer
	err := RunBackupWithInput([]string{"list"}, &out, strings.NewReader(""), home)
	if err != nil {
		t.Fatalf("RunBackupWithInput(list) error = %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "TIMESTAMP") || !strings.Contains(output, "NAME / REASON") {
		t.Errorf("expected table header in output, got: %s", output)
	}
	if !strings.Contains(output, "backup-1") || !strings.Contains(output, "backup-2") {
		t.Errorf("expected backup names in output, got: %s", output)
	}
	if !strings.Contains(output, "[pinned]") {
		t.Errorf("expected [pinned] in output for backup-2, got: %s", output)
	}
	if !strings.Contains(output, "Total: 2 backups occupying") {
		t.Errorf("expected summary total in output, got: %s", output)
	}
}

func TestRunBackupList_JSON(t *testing.T) {
	home := t.TempDir()
	t1 := time.Date(2026, 8, 1, 10, 0, 0, 0, time.UTC)
	createTestBackup(t, home, "backup-1", t1, backup.BackupSourceInstall, "pre-install", false)

	var out bytes.Buffer
	err := RunBackupWithInput([]string{"list", "--json"}, &out, strings.NewReader(""), home)
	if err != nil {
		t.Fatalf("RunBackupWithInput(list --json) error = %v", err)
	}

	var report backup.BackupReport
	if err := json.Unmarshal(out.Bytes(), &report); err != nil {
		t.Fatalf("json.Unmarshal error = %v, output = %s", err, out.String())
	}

	if report.TotalCount != 1 {
		t.Errorf("report.TotalCount = %d, want 1", report.TotalCount)
	}
	if len(report.Backups) != 1 {
		t.Fatalf("len(report.Backups) = %d, want 1", len(report.Backups))
	}
	if report.Backups[0].Name != "backup-1" {
		t.Errorf("report.Backups[0].Name = %q, want backup-1", report.Backups[0].Name)
	}
	if report.Backups[0].SizeHuman == "" {
		t.Errorf("report.Backups[0].SizeHuman is empty")
	}
}

func TestRunBackupClean_ForceKeep(t *testing.T) {
	home := t.TempDir()
	t1 := time.Date(2026, 8, 1, 10, 0, 0, 0, time.UTC)
	t2 := time.Date(2026, 8, 2, 10, 0, 0, 0, time.UTC)
	t3 := time.Date(2026, 8, 3, 10, 0, 0, 0, time.UTC)

	createTestBackup(t, home, "backup-1", t1, backup.BackupSourceInstall, "b1", false)
	createTestBackup(t, home, "backup-2", t2, backup.BackupSourceSync, "b2", false)
	createTestBackup(t, home, "backup-3", t3, backup.BackupSourceUpgrade, "b3", false)

	var out bytes.Buffer
	// Keep 1 most recent, force skip prompt
	err := RunBackupWithInput([]string{"clean", "--keep", "1", "--force"}, &out, strings.NewReader(""), home)
	if err != nil {
		t.Fatalf("RunBackupWithInput(clean) error = %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "Cleaned 2 backup(s)") {
		t.Errorf("expected cleaned 2 backups, got: %s", output)
	}

	// Verify only backup-3 remains
	report, err := backup.ListBackupsReport(filepath.Join(home, ".gentle-ai", "backups"))
	if err != nil {
		t.Fatalf("ListBackupsReport error = %v", err)
	}
	if report.TotalCount != 1 {
		t.Fatalf("expected 1 remaining backup, got %d", report.TotalCount)
	}
	if report.Backups[0].Name != "backup-3" {
		t.Errorf("expected remaining backup to be backup-3, got %q", report.Backups[0].Name)
	}
}

func TestRunBackupClean_InteractiveConfirmationYes(t *testing.T) {
	home := t.TempDir()
	t1 := time.Date(2026, 8, 1, 10, 0, 0, 0, time.UTC)
	t2 := time.Date(2026, 8, 2, 10, 0, 0, 0, time.UTC)

	createTestBackup(t, home, "backup-1", t1, backup.BackupSourceInstall, "b1", false)
	createTestBackup(t, home, "backup-2", t2, backup.BackupSourceSync, "b2", false)

	var out bytes.Buffer
	stdin := strings.NewReader("yes\n")
	err := RunBackupWithInput([]string{"clean", "--keep", "1"}, &out, stdin, home)
	if err != nil {
		t.Fatalf("RunBackupWithInput(clean interactive yes) error = %v", err)
	}

	if !strings.Contains(out.String(), "Cleaned 1 backup(s)") {
		t.Errorf("expected 1 backup cleaned, got: %s", out.String())
	}
}

func TestRunBackupList_AliasLs(t *testing.T) {
	home := t.TempDir()
	t1 := time.Date(2026, 8, 1, 10, 0, 0, 0, time.UTC)
	createTestBackup(t, home, "backup-ls-1", t1, backup.BackupSourceInstall, "pre-install", false)

	var out bytes.Buffer
	err := RunBackupWithInput([]string{"ls"}, &out, strings.NewReader(""), home)
	if err != nil {
		t.Fatalf("RunBackupWithInput(ls) error = %v", err)
	}

	if !strings.Contains(out.String(), "backup-ls-1") {
		t.Errorf("expected backup-ls-1 in output for 'ls' alias, got: %s", out.String())
	}
}

func TestRunBackupClean_ShortFlagY(t *testing.T) {
	home := t.TempDir()
	t1 := time.Date(2026, 8, 1, 10, 0, 0, 0, time.UTC)
	t2 := time.Date(2026, 8, 2, 10, 0, 0, 0, time.UTC)

	createTestBackup(t, home, "backup-1", t1, backup.BackupSourceInstall, "b1", false)
	createTestBackup(t, home, "backup-2", t2, backup.BackupSourceSync, "b2", false)

	var out bytes.Buffer
	err := RunBackupWithInput([]string{"clean", "--keep=1", "-y"}, &out, strings.NewReader(""), home)
	if err != nil {
		t.Fatalf("RunBackupWithInput(clean -y) error = %v", err)
	}

	if !strings.Contains(out.String(), "Cleaned 1 backup(s)") {
		t.Errorf("expected 1 backup cleaned with -y, got: %s", out.String())
	}
}

func TestRunBackupClean_InteractiveConfirmationNo(t *testing.T) {
	home := t.TempDir()
	t1 := time.Date(2026, 8, 1, 10, 0, 0, 0, time.UTC)
	createTestBackup(t, home, "backup-1", t1, backup.BackupSourceInstall, "b1", false)

	var out bytes.Buffer
	stdin := strings.NewReader("no\n")
	err := RunBackupWithInput([]string{"clean", "--keep", "1"}, &out, stdin, home)
	if err != nil {
		t.Fatalf("RunBackupWithInput(clean interactive no) error = %v", err)
	}

	if !strings.Contains(out.String(), "clean cancelled") {
		t.Errorf("expected 'clean cancelled', got: %s", out.String())
	}
}
