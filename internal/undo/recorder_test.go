package undo

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRecorderBeforeWriteSavesExistingFile(t *testing.T) {
	dir := t.TempDir()
	undoDir := filepath.Join(dir, "undo")

	origPath := filepath.Join(dir, "config.json")
	if err := os.WriteFile(origPath, []byte("original"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	rec := NewRecorder(undoDir)
	if err := rec.BeforeWrite(origPath); err != nil {
		t.Fatalf("BeforeWrite error = %v", err)
	}

	// Modify the original file.
	if err := os.WriteFile(origPath, []byte("modified"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// Rollback should restore the original.
	if err := rec.Rollback(); err != nil {
		t.Fatalf("Rollback error = %v", err)
	}

	got, err := os.ReadFile(origPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(got) != "original" {
		t.Errorf("after rollback content = %q, want %q", string(got), "original")
	}
}

func TestRecorderBeforeWriteTracksCreatedFiles(t *testing.T) {
	dir := t.TempDir()
	undoDir := filepath.Join(dir, "undo")

	newPath := filepath.Join(dir, "new.json")
	// Do NOT create the file — simulate a creation.

	rec := NewRecorder(undoDir)
	if err := rec.BeforeWrite(newPath); err != nil {
		t.Fatalf("BeforeWrite error = %v", err)
	}

	// Create the file after tracking.
	if err := os.WriteFile(newPath, []byte("created"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// Rollback should remove the created file.
	if err := rec.Rollback(); err != nil {
		t.Fatalf("Rollback error = %v", err)
	}

	if _, err := os.Stat(newPath); !os.IsNotExist(err) {
		t.Errorf("created file %q still exists after rollback", newPath)
	}
}

func TestRecorderRollbackRestoresMultipleFiles(t *testing.T) {
	dir := t.TempDir()
	undoDir := filepath.Join(dir, "undo")

	paths := []string{
		filepath.Join(dir, "a.json"),
		filepath.Join(dir, "b.json"),
		filepath.Join(dir, "c.json"),
	}
	for i, p := range paths {
		if err := os.WriteFile(p, []byte(string(rune('a'+i))), 0o644); err != nil {
			t.Fatalf("WriteFile %q: %v", p, err)
		}
	}

	rec := NewRecorder(undoDir)
	for _, p := range paths {
		if err := rec.BeforeWrite(p); err != nil {
			t.Fatalf("BeforeWrite %q: %v", p, err)
		}
	}

	// Modify all files.
	for _, p := range paths {
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatalf("WriteFile %q: %v", p, err)
		}
	}

	if err := rec.Rollback(); err != nil {
		t.Fatalf("Rollback error = %v", err)
	}

	for i, p := range paths {
		got, err := os.ReadFile(p)
		if err != nil {
			t.Fatalf("ReadFile %q: %v", p, err)
		}
		want := string(rune('a' + i))
		if string(got) != want {
			t.Errorf("%q after rollback = %q, want %q", p, string(got), want)
		}
	}
}

func TestRecorderCommitCleansUpUndoDir(t *testing.T) {
	dir := t.TempDir()
	undoDir := filepath.Join(dir, "undo")

	origPath := filepath.Join(dir, "config.json")
	if err := os.WriteFile(origPath, []byte("x"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	rec := NewRecorder(undoDir)
	if err := rec.BeforeWrite(origPath); err != nil {
		t.Fatalf("BeforeWrite error = %v", err)
	}

	if err := rec.Commit(); err != nil {
		t.Fatalf("Commit error = %v", err)
	}

	if _, err := os.Stat(undoDir); !os.IsNotExist(err) {
		t.Errorf("undo dir %q still exists after commit", undoDir)
	}
}

func TestRecorderCommitIsIdempotent(t *testing.T) {
	rec := NewRecorder("")
	if err := rec.Commit(); err != nil {
		t.Fatalf("Commit on empty dir error = %v", err)
	}
}

func TestRecorderRecordCreatedExplicit(t *testing.T) {
	dir := t.TempDir()
	undoDir := filepath.Join(dir, "undo")

	newPath := filepath.Join(dir, "explicit.json")
	if err := os.WriteFile(newPath, []byte("data"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	rec := NewRecorder(undoDir)
	rec.RecordCreated(newPath)

	if err := rec.Rollback(); err != nil {
		t.Fatalf("Rollback error = %v", err)
	}

	if _, err := os.Stat(newPath); !os.IsNotExist(err) {
		t.Errorf("explicitly created file %q still exists after rollback", newPath)
	}
}

func TestRecorderBeforeWriteIgnoresDirectory(t *testing.T) {
	dir := t.TempDir()
	undoDir := filepath.Join(dir, "undo")

	subDir := filepath.Join(dir, "subdir")
	if err := os.MkdirAll(subDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	rec := NewRecorder(undoDir)
	if err := rec.BeforeWrite(subDir); err != nil {
		t.Fatalf("BeforeWrite directory error = %v", err)
	}

	// Directory should still exist after rollback.
	if err := rec.Rollback(); err != nil {
		t.Fatalf("Rollback error = %v", err)
	}
	if _, err := os.Stat(subDir); err != nil {
		t.Errorf("directory %q was removed unexpectedly", subDir)
	}
}

func TestRecorderRollbackReportsErrors(t *testing.T) {
	dir := t.TempDir()
	undoDir := filepath.Join(dir, "undo")

	origPath := filepath.Join(dir, "config.json")
	if err := os.WriteFile(origPath, []byte("x"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	rec := NewRecorder(undoDir)
	if err := rec.BeforeWrite(origPath); err != nil {
		t.Fatalf("BeforeWrite error = %v", err)
	}

	// Delete the backup to force a rollback error.
	backupPath := rec.modified[origPath]
	if err := os.Remove(backupPath); err != nil {
		t.Fatalf("Remove backup: %v", err)
	}

	err := rec.Rollback()
	if err == nil {
		t.Fatal("Rollback expected error when backup is missing")
	}
	if !strings.Contains(err.Error(), "error(s)") {
		t.Errorf("error = %q, want error count message", err.Error())
	}
}
