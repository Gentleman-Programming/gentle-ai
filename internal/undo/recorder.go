// Package undo provides file-level undo tracking for transactional config writes.
//
// A Recorder captures the state of files before they are overwritten, allowing
// granular rollback of individual component steps without restoring the entire
// backup snapshot.
//
// Usage:
//
//	rec := undo.NewRecorder(undoDir)
//	// Before each WriteFileAtomic call, save the original:
//	if err := rec.BeforeWrite(path); err != nil { ... }
//	// ... write the file ...
//	rec.RecordCreated(path) // if the file did not exist before
//	// On failure:
//	if err := rec.Rollback(); err != nil { ... }
//	// On success:
//	if err := rec.Commit(); err != nil { ... }
package undo

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// Recorder tracks file modifications and creations for a single component step.
type Recorder struct {
	// Dir is the directory where original files are backed up.
	Dir string

	// modified maps original file path -> backup file path.
	modified map[string]string

	// created lists files that did not exist before being written.
	created []string
}

// NewRecorder creates a new Recorder that stores backups under dir.
// The directory is created on demand.
func NewRecorder(dir string) *Recorder {
	return &Recorder{
		Dir:      dir,
		modified: make(map[string]string),
	}
}

// BeforeWrite saves the existing file at path to the recorder's backup
// directory before it is overwritten. If the file does not exist, it is
// recorded as a created file. If the file is a directory, it is ignored.
func (r *Recorder) BeforeWrite(path string) error {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		r.created = append(r.created, path)
		return nil
	}
	if err != nil {
		return fmt.Errorf("stat %q for undo: %w", path, err)
	}
	if info.IsDir() {
		return nil
	}

	// Use a deterministic backup path based on the absolute path hash.
	backupPath := filepath.Join(r.Dir, "modified", hashPath(path))
	if err := os.MkdirAll(filepath.Dir(backupPath), 0o755); err != nil {
		return fmt.Errorf("create undo dir for %q: %w", path, err)
	}
	if err := copyFile(path, backupPath); err != nil {
		return fmt.Errorf("save undo for %q: %w", path, err)
	}
	r.modified[path] = backupPath
	return nil
}

// RecordCreated explicitly marks path as a file that was created during the
// step (did not exist before). This is useful when the creation was detected
// externally rather than through BeforeWrite.
func (r *Recorder) RecordCreated(path string) {
	r.created = append(r.created, path)
}

// Rollback restores all modified files from their backups and removes all
// created files. It stops at the first error but reports the total number
// of failed operations.
func (r *Recorder) Rollback() error {
	var errs []error

	for orig, backup := range r.modified {
		if err := copyFile(backup, orig); err != nil {
			errs = append(errs, fmt.Errorf("restore %q: %w", orig, err))
		}
	}

	for _, path := range r.created {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			errs = append(errs, fmt.Errorf("remove created %q: %w", path, err))
		}
	}

	if len(errs) > 0 {
		// Return the first error but include the total count.
		return fmt.Errorf("rollback encountered %d error(s): %w", len(errs), errs[0])
	}

	return nil
}

// Commit removes the undo directory, signaling that the step succeeded and
// backups are no longer needed. Commit is idempotent.
func (r *Recorder) Commit() error {
	if r.Dir == "" {
		return nil
	}
	return os.RemoveAll(r.Dir)
}

// copyFile copies src to dst, creating parent directories as needed.
func copyFile(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}

	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}

// hashPath returns a stable, filesystem-safe identifier for path.
// It is used to map arbitrary file paths into the flat undo directory.
func hashPath(path string) string {
	// Simple stable hash: replace path separators with underscores and
	// append the base name to keep it somewhat readable.
	clean := filepath.Clean(path)
	base := filepath.Base(clean)
	hash := fmt.Sprintf("%x", fnvHash(clean))
	return hash + "_" + base
}

// fnvHash is a simple FNV-1a hash for strings.
func fnvHash(s string) uint64 {
	const offset64 = 14695981039346656037
	const prime64 = 1099511628211
	h := uint64(offset64)
	for i := 0; i < len(s); i++ {
		h ^= uint64(s[i])
		h *= prime64
	}
	return h
}
