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

func TestParseBackupFlags(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		want    BackupFlags
		wantErr bool
	}{
		{
			name: "default no args",
			args: nil,
			want: BackupFlags{Subcommand: "list"},
		},
		{
			name: "list sub",
			args: []string{"list"},
			want: BackupFlags{Subcommand: "list"},
		},
		{
			name: "ls alias with json",
			args: []string{"ls", "--json"},
			want: BackupFlags{Subcommand: "list", JSON: true},
		},
		{
			name: "clean default keep",
			args: []string{"clean"},
			want: BackupFlags{Subcommand: "clean", KeepCount: 5},
		},
		{
			name: "clean with keep and force",
			args: []string{"clean", "--keep", "3", "--force"},
			want: BackupFlags{Subcommand: "clean", KeepCount: 3, Force: true},
		},
		{
			name:    "clean with invalid keep count",
			args:    []string{"clean", "--keep", "-1"},
			wantErr: true,
		},
		{
			name:    "unknown subcommand",
			args:    []string{"invalid"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseBackupFlags(tt.args)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ParseBackupFlags(%v) error = %v, wantErr %v", tt.args, err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if got != tt.want {
				t.Errorf("ParseBackupFlags(%v) = %+v; want %+v", tt.args, got, tt.want)
			}
		})
	}
}

func TestRunBackupListAndClean(t *testing.T) {
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)

	backupDir := filepath.Join(tempHome, ".gentle-ai", "backups")
	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		t.Fatalf("mkdir backupDir error = %v", err)
	}

	// Override BackupRootFn for safety
	defer func(orig func() (string, error)) { backup.BackupRootFn = orig }(backup.BackupRootFn)
	backup.BackupRootFn = func() (string, error) { return backupDir, nil }

	now := time.Now()
	// Create test manifest
	mDir := filepath.Join(backupDir, "upgrade-test")
	if err := os.MkdirAll(mDir, 0o755); err != nil {
		t.Fatalf("mkdir mDir error = %v", err)
	}
	m := backup.Manifest{
		ID:        "upgrade-test",
		CreatedAt: now.Add(-1 * time.Hour),
		RootDir:   mDir,
		Source:    backup.BackupSourceUpgrade,
		FileCount: 1,
	}
	if err := backup.WriteManifest(filepath.Join(mDir, backup.ManifestFilename), m); err != nil {
		t.Fatalf("WriteManifest error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(mDir, "data.txt"), []byte("data"), 0o644); err != nil {
		t.Fatalf("Write data error = %v", err)
	}

	// Test list human table
	var bufHuman bytes.Buffer
	if err := RunBackup([]string{"list"}, &bufHuman); err != nil {
		t.Fatalf("RunBackup(list) error = %v", err)
	}
	outHuman := bufHuman.String()
	if !strings.Contains(outHuman, "upgrade") || !strings.Contains(outHuman, "Total: 1 backup") {
		t.Errorf("RunBackup(list) human output unexpected: %s", outHuman)
	}

	// Test list JSON
	var bufJSON bytes.Buffer
	if err := RunBackup([]string{"list", "--json"}, &bufJSON); err != nil {
		t.Fatalf("RunBackup(list --json) error = %v", err)
	}
	var report backup.BackupListReport
	if err := json.Unmarshal(bufJSON.Bytes(), &report); err != nil {
		t.Fatalf("json.Unmarshal report error = %v; raw = %s", err, bufJSON.String())
	}
	if report.TotalCount != 1 || len(report.Backups) != 1 {
		t.Errorf("JSON report total_count = %d; want 1", report.TotalCount)
	}

	// Test clean
	var bufClean bytes.Buffer
	if err := RunBackup([]string{"clean", "--keep", "5"}, &bufClean); err != nil {
		t.Fatalf("RunBackup(clean) error = %v", err)
	}
	if !strings.Contains(bufClean.String(), "No stale backups to clean") {
		t.Errorf("RunBackup(clean) output unexpected: %s", bufClean.String())
	}
}

func TestPrintBackupHelp(t *testing.T) {
	var buf bytes.Buffer
	PrintBackupHelp(&buf)
	if !strings.Contains(buf.String(), "gentle-ai backup — Inspect and manage stored safety backups") {
		t.Errorf("PrintBackupHelp output unexpected: %s", buf.String())
	}
}
