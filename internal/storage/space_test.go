package storage

import (
	"path/filepath"
	"testing"
)

func TestCheckAvailableSpace_ReturnsPositive(t *testing.T) {
	tmp := t.TempDir()
	space, err := CheckAvailableSpace(tmp)
	if err != nil {
		t.Fatalf("CheckAvailableSpace(%q) error: %v", tmp, err)
	}
	if space == 0 {
		t.Error("CheckAvailableSpace returned 0, expected positive value")
	}
}

func TestRequireFreeSpace_PassesForSmallRequirement(t *testing.T) {
	tmp := t.TempDir()
	err := RequireFreeSpace(tmp, 1024) // 1 KB
	if err != nil {
		t.Errorf("RequireFreeSpace(%q, 1024) error: %v", tmp, err)
	}
}

func TestRequireFreeSpace_FailsForImpossibleRequirement(t *testing.T) {
	tmp := t.TempDir()
	// 1 exabyte — should always fail.
	err := RequireFreeSpace(tmp, 1<<60)
	if err == nil {
		t.Error("RequireFreeSpace expected error for 1 EB requirement, got nil")
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
		{1048576, "1.0 MB"},
		{1073741824, "1.0 GB"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := formatBytes(tt.bytes)
			if got != tt.want {
				t.Errorf("formatBytes(%d) = %q, want %q", tt.bytes, got, tt.want)
			}
		})
	}
}

func TestRequireFreeSpace_NonexistentChildUsesExistingParent(t *testing.T) {
	target := filepath.Join(t.TempDir(), "new", "engram", "data")
	err := RequireFreeSpace(target, 1024)
	if err != nil {
		t.Fatalf("RequireFreeSpace should check the nearest existing parent for creatable paths: %v", err)
	}
}
