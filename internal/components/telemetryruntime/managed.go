package telemetryruntime

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"

	"github.com/gentleman-programming/gentle-ai/v2/internal/assets"
	"github.com/gentleman-programming/gentle-ai/v2/internal/components/mutationjournal"
)

const ownershipSchema = "gentle-ai.telemetry-runtime-ownership/v1"
const ownershipMarker = "// gentle-ai:managed telemetry-runtime/v1\n"

// approvedPriorPluginDigests is an append-only provenance allowlist. Each
// managed plugin change must add the immediately previous embedded asset digest.
// Commit a9cab7dd embedded telemetry-runtime.ts with SHA-256
// 902fe09299c0bb04590f12ba993a196b838e1fbbd8a94e96100ea3d5f30182b7.
const priorPluginDigestA9cab7dd = "902fe09299c0bb04590f12ba993a196b838e1fbbd8a94e96100ea3d5f30182b7"

var approvedPriorPluginDigests = map[string]struct{}{
	priorPluginDigestA9cab7dd: {},
}

// Windows exposes writable regular files as 0666 regardless of the requested
// POSIX permission bits. Retain exact modes elsewhere and never equate a
// read-only file with a writable managed file.
func managedModeMatches(expected, observed os.FileMode) bool {
	return managedModeMatchesForOS(runtime.GOOS, expected, observed)
}

func managedModeMatchesForOS(goos string, expected, observed os.FileMode) bool {
	if expected == observed {
		return true
	}
	return goos == "windows" &&
		expected.Perm()&0222 != 0 &&
		observed.Perm() == 0666 &&
		expected&^os.ModePerm == observed&^os.ModePerm
}

type managedManifest struct {
	Schema string                    `json:"schema"`
	File   mutationjournal.OwnedFile `json:"file"`
}

// ManagedPaths accepts the adapter's resolved config directory, including XDG.
// The manifest lives outside plugins; no user JSON or exporter is modified.
func ManagedPaths(configDir string) []string {
	return []string{filepath.Join(configDir, "plugins", "telemetry-runtime.ts"), filepath.Join(configDir, ".gentle-ai-telemetry-runtime.json")}
}

// Refuse indirection inside the caller-resolved configuration root as well as
// leaf symlinks. The generic journal only refuses a leaf or a root escape.
func checkManagedPath(configDir, path string) error {
	for _, candidate := range []string{configDir, filepath.Dir(path), path} {
		info, err := os.Lstat(candidate)
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		if err == nil && info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("telemetry runtime symlink conflict: %s", candidate)
		}
	}
	return nil
}

func readManaged(path string) ([]byte, error) {
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > 65536 {
		return nil, fmt.Errorf("telemetry runtime ownership conflict: %s", path)
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, 65537))
	if len(data) > 65536 {
		return nil, fmt.Errorf("telemetry runtime manifest size conflict")
	}
	return data, err
}

// inspect refuses adoption, even when an unowned file matches this release.
// A missing owned asset is repairable; altered bytes or metadata are not.
func inspect(configDir string) ([][]byte, error) {
	paths := ManagedPaths(configDir)
	current := make([][]byte, 2)
	journal := mutationjournal.New(configDir)
	for i, path := range paths {
		if err := checkManagedPath(configDir, path); err != nil {
			return nil, err
		}
		if err := journal.Validate(path); err != nil {
			return nil, err
		}
		data, err := readManaged(path)
		if err != nil {
			return nil, err
		}
		current[i] = data
	}
	conflict := fmt.Errorf("telemetry runtime ownership conflict in %s; custom files preserved", configDir)
	if current[1] == nil {
		if current[0] != nil {
			return nil, conflict
		}
		return current, nil
	}
	var manifest managedManifest
	root, err := manifestObject(current[1], "schema", "file")
	if err != nil {
		return nil, conflict
	}
	file, err := manifestObject(root["file"], "after", "afterHash", "overlay", "mode")
	if err != nil || !bytes.Equal(bytes.TrimSpace(file["overlay"]), []byte("false")) || json.Unmarshal(current[1], &manifest) != nil || manifest.Schema != ownershipSchema ||
		(!managedModeMatches(0600, os.FileMode(manifest.File.Mode)) && !managedModeMatches(0644, os.FileMode(manifest.File.Mode))) || manifest.File.AfterHash != fmt.Sprintf("%x", sha256.Sum256([]byte(manifest.File.After))) {
		return nil, conflict
	}
	embedded, err := assets.Read("opencode/plugins/telemetry-runtime.ts")
	embeddedDigest := fmt.Sprintf("%x", sha256.Sum256([]byte(embedded)))
	_, approvedPrior := approvedPriorPluginDigests[manifest.File.AfterHash]
	if err != nil || (manifest.File.AfterHash != embeddedDigest && !approvedPrior) {
		return nil, conflict
	}
	for i, path := range paths {
		if current[i] == nil {
			continue
		}
		info, err := os.Lstat(path)
		expected := os.FileMode(0600)
		if i == 0 {
			expected = os.FileMode(manifest.File.Mode)
		}
		if err != nil || !managedModeMatches(expected, info.Mode()) {
			return nil, conflict
		}
	}
	if current[0] != nil && string(current[0]) != manifest.File.After {
		return nil, conflict
	}
	return current, nil
}

// Decode exact required keys before struct decoding: Go structs alone accept
// aliases, duplicates and omitted/null scalar fields. Each object is bounded.
func manifestObject(raw []byte, keys ...string) (map[string]json.RawMessage, error) {
	d := json.NewDecoder(bytes.NewReader(raw))
	token, err := d.Token()
	if err != nil || token != json.Delim('{') {
		return nil, errors.New("invalid ownership object")
	}
	fields := map[string]json.RawMessage{}
	for d.More() {
		token, err = d.Token()
		key, ok := token.(string)
		known := false
		for _, expected := range keys {
			known = known || key == expected
		}
		if err != nil || !ok || !known || fields[key] != nil {
			return nil, errors.New("invalid ownership key")
		}
		var value json.RawMessage
		if err := d.Decode(&value); err != nil {
			return nil, err
		}
		fields[key] = value
	}
	if _, err = d.Token(); err != nil {
		return nil, err
	}
	if _, err = d.Token(); err != io.EOF || len(fields) != len(keys) {
		return nil, errors.New("incomplete ownership object")
	}
	return fields, nil
}

// CheckManaged refuses unsafe/conflicting targets before a lifecycle snapshot.
// Reconcile and removal also recheck at execution time.
func CheckManaged(configDir string) error {
	_, err := inspect(configDir)
	return err
}

// Reconcile installs or refreshes only this managed asset. Permission is checked
// by the plugin at runtime, never by installation: disabled installs stay inert.
func Reconcile(configDir string) ([]string, error) {
	changed, _, err := ReconcileWithRollback(configDir)
	return changed, err
}

type guardedFile struct {
	configDir string
	path      string
	journal   *mutationjournal.Journal
	expected  []byte
	mode      os.FileMode
}

func (f *guardedFile) restore() error {
	if f.expected == nil {
		return nil
	}
	// Do not infer permission from the editable on-disk ownership manifest.
	// Each file must match its independently retained in-memory post-image.
	if err := checkManagedPath(f.configDir, f.path); err != nil {
		return fmt.Errorf("telemetry runtime rollback conflict: %w", err)
	}
	if err := f.journal.Validate(f.path); err != nil {
		return fmt.Errorf("telemetry runtime rollback conflict: %w", err)
	}
	current, err := readManaged(f.path)
	info, statErr := os.Lstat(f.path)
	if err != nil || statErr != nil || !managedModeMatches(f.mode, info.Mode()) || !bytes.Equal(current, f.expected) {
		return fmt.Errorf("telemetry runtime rollback conflict: %s; edited file preserved", f.path)
	}
	if err := f.journal.Restore(); err != nil {
		return err
	}
	f.expected = nil
	return nil
}

// ReconcileWithRollback retains per-file journals for the outer lifecycle. The
// caller must exclude this pair from unconditional snapshot restoration, even
// if reconcile fails or never runs. Safe members restore despite other conflicts.
func ReconcileWithRollback(configDir string) (changed []string, rollback func() error, err error) {
	current, err := inspect(configDir)
	if err != nil {
		return nil, nil, err
	}
	content, err := assets.Read("opencode/plugins/telemetry-runtime.ts")
	if err != nil {
		return nil, nil, err
	}
	paths := ManagedPaths(configDir)
	files := make([]guardedFile, 2)
	for i, path := range paths {
		files[i] = guardedFile{configDir: configDir, path: path, journal: mutationjournal.New(configDir), mode: 0600}
		if i == 0 {
			files[i].mode = 0644
		}
		if current[i] != nil {
			info, statErr := os.Lstat(path)
			if statErr != nil {
				return nil, nil, statErr
			}
			files[i].mode = info.Mode()
		}
		if err = files[i].journal.Capture(path); err != nil {
			return nil, nil, err
		}
	}
	rollback = func() error {
		var conflicts []error
		for i := range files {
			conflicts = append(conflicts, files[i].restore())
		}
		return errors.Join(conflicts...)
	}
	defer func() {
		if err != nil {
			err = errors.Join(err, rollback())
		}
	}()
	if _, err = inspect(configDir); err != nil {
		return nil, rollback, err
	}
	files[0].expected = []byte(content)
	owned, err := files[0].journal.WriteWithMode(paths[0], []byte(content), files[0].mode)
	if err != nil {
		return nil, rollback, err
	}
	info, err := os.Lstat(paths[0])
	if err != nil {
		return nil, rollback, err
	}
	if !managedModeMatches(files[0].mode, info.Mode()) {
		return nil, rollback, fmt.Errorf("telemetry runtime mode conflict: %s", paths[0])
	}
	owned.Before = nil
	owned.Mode = uint32(info.Mode().Perm()) // The writer preserves existing modes.
	metadata, err := json.MarshalIndent(managedManifest{ownershipSchema, owned}, "", "  ")
	if err != nil {
		return nil, rollback, err
	}
	metadata = append(metadata, '\n')
	files[1].expected = metadata
	if _, err = files[1].journal.WriteWithMode(paths[1], metadata, files[1].mode); err != nil {
		return nil, rollback, err
	}
	if _, err = inspect(configDir); err != nil {
		return nil, rollback, err
	}
	for i, data := range [][]byte{[]byte(content), metadata} {
		if !bytes.Equal(current[i], data) {
			changed = append(changed, paths[i])
		}
	}
	return changed, rollback, nil
}

// RemoveManaged validates the pair immediately before transactional removal.
// Drift is an explicit conflict, not permission to remove a user-modified asset.
func RemoveManaged(configDir string) (removed []string, err error) {
	if _, err = inspect(configDir); err != nil {
		return nil, err
	}
	journal := mutationjournal.New(configDir)
	defer func() {
		if err != nil {
			err = errors.Join(err, journal.Restore())
		}
	}()
	paths := ManagedPaths(configDir)
	for _, path := range paths {
		if err = journal.Capture(path); err != nil {
			return nil, err
		}
	}
	if _, err = inspect(configDir); err != nil {
		return nil, err
	}
	for _, path := range paths {
		var changed bool
		changed, err = journal.Remove(path)
		if err != nil {
			return nil, err
		}
		if changed {
			removed = append(removed, path)
		}
	}
	return removed, nil
}
