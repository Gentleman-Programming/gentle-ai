package trace

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTraceabilityManager_ValidTransition(t *testing.T) {
	tempDir := t.TempDir()
	sourcePath := filepath.Join(tempDir, "source.md")
	destPath := filepath.Join(tempDir, "dest.md")

	err := os.WriteFile(sourcePath, []byte("---\nid: BS-42@2026-08-13T15:12:01Z\n---\nContent"), 0644)
	if err != nil {
		t.Fatal(err)
	}

	err = os.WriteFile(destPath, []byte("---\nid: PROP-018\nimplements:\n  - BS-42@2026-08-13T15:12:01Z\n---\nContent"), 0644)
	if err != nil {
		t.Fatal(err)
	}

	manager := NewManager()
	err = manager.ValidatePhaseTransition(sourcePath, destPath)
	if err != nil {
		t.Errorf("Expected nil error for valid transition, got: %v", err)
	}
}

func TestTraceabilityManager_InvalidTransition_MissingImplements(t *testing.T) {
	tempDir := t.TempDir()
	sourcePath := filepath.Join(tempDir, "source.md")
	destPath := filepath.Join(tempDir, "dest.md")

	err := os.WriteFile(sourcePath, []byte("---\nid: BS-42\n---\nContent"), 0644)
	if err != nil {
		t.Fatal(err)
	}

	err = os.WriteFile(destPath, []byte("---\nid: PROP-018\n---\nContent"), 0644)
	if err != nil {
		t.Fatal(err)
	}

	manager := NewManager()
	err = manager.ValidatePhaseTransition(sourcePath, destPath)

	if err == nil {
		t.Fatal("Expected error for invalid transition, got nil")
	}

	if _, ok := err.(*TraceabilityError); !ok {
		t.Errorf("Expected error to be of type *TraceabilityError, got %T", err)
	}
}

func TestTraceabilityManager_InvalidTransition_MismatchedID(t *testing.T) {
	tempDir := t.TempDir()
	sourcePath := filepath.Join(tempDir, "source.md")
	destPath := filepath.Join(tempDir, "dest.md")

	err := os.WriteFile(sourcePath, []byte("---\nid: BS-42\n---\nContent"), 0644)
	if err != nil {
		t.Fatal(err)
	}

	err = os.WriteFile(destPath, []byte("---\nid: PROP-018\nimplements:\n  - BS-99\n---\nContent"), 0644)
	if err != nil {
		t.Fatal(err)
	}

	manager := NewManager()
	err = manager.ValidatePhaseTransition(sourcePath, destPath)

	if err == nil {
		t.Fatal("Expected error for mismatched ID, got nil")
	}
}
