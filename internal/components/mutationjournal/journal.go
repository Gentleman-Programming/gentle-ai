// Package mutationjournal records before-images of files within an
// allow-listed set of roots and restores them on demand. It is intended for
// code that mutates user files as part of an installation or sync step and
// must roll back atomically if a later step fails.
package mutationjournal

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/gentleman-programming/gentle-ai/internal/components/filemerge"
)

// OwnedFile is the persisted before-image record. It mirrors what an external
// manifest can serialize to JSON to track every file a sync step touched.
type OwnedFile struct {
	Before    *string `json:"before,omitempty"`
	After     string  `json:"after"`
	AfterHash string  `json:"afterHash"`
	Overlay   bool    `json:"overlay"`
	Adopted   bool    `json:"adopted,omitempty"`
	Mode      uint32  `json:"mode,omitempty"`
}

type journalEntry struct {
	data *[]byte
	mode os.FileMode
}

// Journal captures before-images and supports atomic writes with restore on
// error. A zero-value Journal is not usable; construct one with New.
type Journal struct {
	before map[string]*journalEntry
	roots  []string
}

// New constructs a journal that only accepts paths within the given roots.
// Roots must be absolute or the path-traversal guard will reject every write.
func New(roots ...string) *Journal {
	return &Journal{before: map[string]*journalEntry{}, roots: append([]string(nil), roots...)}
}

// Capture snapshots a file's current content + mode for later restoration.
// If the file does not exist, records nil (Restore will remove anything
// created later). Capture is idempotent for the same path.
func (j *Journal) Capture(path string) error {
	if err := j.validate(path); err != nil {
		return err
	}
	if _, exists := j.before[path]; exists {
		return nil
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		j.before[path] = nil
		return nil
	}
	if err != nil {
		return fmt.Errorf("read before-image %q: %w", path, err)
	}
	info, statErr := os.Stat(path)
	if statErr != nil {
		return fmt.Errorf("stat before-image %q: %w", path, statErr)
	}
	j.before[path] = &journalEntry{data: &data, mode: info.Mode().Perm()}
	return nil
}

// Write atomically writes data to path. If the file existed before the first
// Capture/Write call for this path, the journal will have its content
// captured automatically; otherwise the file's prior absence is recorded.
// Returns the resulting OwnedFile record (Before/After/Hash populated).
func (j *Journal) Write(path string, data []byte) (OwnedFile, error) {
	return j.WriteWithMode(path, data, 0)
}

// WriteWithMode is like Write but preserves an explicit file mode for new
// files. If the file existed before, the prior mode is reused.
func (j *Journal) WriteWithMode(path string, data []byte, mode os.FileMode) (OwnedFile, error) {
	if err := j.Capture(path); err != nil {
		return OwnedFile{}, err
	}
	effective := mode
	if effective == 0 {
		effective = 0o600
	}
	if previous := j.before[path]; previous != nil {
		effective = previous.mode
	}
	if _, err := filemerge.WriteFileAtomic(path, data, effective); err != nil {
		return OwnedFile{}, fmt.Errorf("write %q: %w", path, err)
	}
	return j.OwnedFile(path, string(data), false, false), nil
}

// OwnedFile builds an OwnedFile record using the current journal state for
// path. Use this for files written externally (e.g. via os.WriteFile) when you
// still need the before/after/hash tuple for a manifest.
func (j *Journal) OwnedFile(path, after string, overlay, adopted bool) OwnedFile {
	entry := j.before[path]
	var snapshot *string
	if entry != nil {
		value := string(*entry.data)
		snapshot = &value
	}
	mode := uint32(0o600)
	if entry != nil {
		mode = uint32(entry.mode)
	}
	return OwnedFile{
		Before:    snapshot,
		After:     after,
		AfterHash: hashBytes([]byte(after)),
		Overlay:   overlay,
		Adopted:   adopted,
		Mode:      mode,
	}
}

// Restore rewrites every captured file to its before-image. For files that
// did not exist before, Restore removes them. Returns errors.Join of any
// individual failures; partial restore is best-effort.
func (j *Journal) Restore() error {
	paths := make([]string, 0, len(j.before))
	for path := range j.before {
		paths = append(paths, path)
	}
	slices.Sort(paths)
	var restoreErrors []error
	for _, path := range paths {
		entry := j.before[path]
		if entry == nil {
			if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
				restoreErrors = append(restoreErrors, fmt.Errorf("remove %q: %w", path, err))
			}
			continue
		}
		if _, err := filemerge.WriteFileAtomic(path, *entry.data, entry.mode); err != nil {
			restoreErrors = append(restoreErrors, fmt.Errorf("restore %q: %w", path, err))
		}
	}
	return errors.Join(restoreErrors...)
}

func (j *Journal) validate(path string) error {
	if path == "" {
		return fmt.Errorf("empty mutation journal path")
	}
	if info, err := os.Lstat(path); err == nil && info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("refuse symlink mutation journal path %q", path)
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("resolve absolute path for %q: %w", path, err)
	}
	for _, root := range j.roots {
		rootAbs, rootErr := filepath.Abs(root)
		if rootErr != nil {
			continue
		}
		rel, relErr := filepath.Rel(rootAbs, abs)
		if relErr == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return nil
		}
	}
	return fmt.Errorf("path %q is outside mutation journal roots", path)
}

func hashBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}