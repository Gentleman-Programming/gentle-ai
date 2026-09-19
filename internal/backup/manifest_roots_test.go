package backup

import (
	"path/filepath"
	"testing"
)

// TestBackupRootFor verifies that BackupRootFor resolves the canonical backup
// root (~/.axiom/backups) for a variety of home directory shapes, including a
// Windows-style path with spaces and an empty string.
func TestBackupRootFor(t *testing.T) {
	tests := []struct {
		name string
		home string
	}{
		{name: "unix-style home", home: "/home/user"},
		{name: "windows-style home with spaces", home: `C:\Users\Jane Doe`},
		{name: "empty home", home: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			want := filepath.Join(tt.home, ".axiom", "backups")
			got := BackupRootFor(tt.home)
			if got != want {
				t.Errorf("BackupRootFor(%q) = %q, want %q", tt.home, got, want)
			}
		})
	}
}

// TestLegacyBackupRootFor verifies that LegacyBackupRootFor resolves the
// legacy backup root (~/.gentle-ai/backups) for the same variety of home
// directory shapes as TestBackupRootFor.
func TestLegacyBackupRootFor(t *testing.T) {
	tests := []struct {
		name string
		home string
	}{
		{name: "unix-style home", home: "/home/user"},
		{name: "windows-style home with spaces", home: `C:\Users\Jane Doe`},
		{name: "empty home", home: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			want := filepath.Join(tt.home, ".gentle-ai", "backups")
			got := LegacyBackupRootFor(tt.home)
			if got != want {
				t.Errorf("LegacyBackupRootFor(%q) = %q, want %q", tt.home, got, want)
			}
		})
	}
}

// TestBackupRoots verifies that BackupRoots returns exactly the canonical
// root followed by the legacy root, in that order. Order is part of the
// contract, not an implementation detail: every reader that scans both roots
// must see the canonical one first.
func TestBackupRoots(t *testing.T) {
	tests := []struct {
		name string
		home string
	}{
		{name: "unix-style home", home: "/home/user"},
		{name: "windows-style home with spaces", home: `C:\Users\Jane Doe`},
		{name: "empty home", home: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			want := []string{BackupRootFor(tt.home), LegacyBackupRootFor(tt.home)}
			got := BackupRoots(tt.home)
			if len(got) != len(want) {
				t.Fatalf("BackupRoots(%q) = %v, want %v", tt.home, got, want)
			}
			for i := range want {
				if got[i] != want[i] {
					t.Errorf("BackupRoots(%q)[%d] = %q, want %q", tt.home, i, got[i], want[i])
				}
			}
		})
	}
}
