package openrecord

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
)

// ManifestName and manifestVersion match openrecord's own emit manifest
// (copia/open-record/internal/emit/emit.go) verbatim — gentle-ai reads this
// format, it does not define one.
const ManifestName = ".openrecord-emitted.json"
const manifestName = ManifestName
const manifestVersion = 1

type manifestEntry struct {
	Path string `json:"path"`
	Hash string `json:"hash"`
}

type manifest struct {
	Version int             `json:"version"`
	Emitter string          `json:"emitter"`
	WithQmd bool            `json:"with_qmd"`
	Entries []manifestEntry `json:"entries"`
}

// loadManifest reads a target directory's previous emit record. An absent or
// unreadable manifest reads as empty, matching openrecord's own tolerance: a
// stale file nobody prunes is a smaller cost than deleting from a record that
// cannot be trusted.
func loadManifest(dir string) manifest {
	raw, err := os.ReadFile(filepath.Join(dir, manifestName))
	if err != nil {
		return manifest{}
	}
	var m manifest
	if err := json.Unmarshal(raw, &m); err != nil || m.Version != manifestVersion {
		return manifest{}
	}
	return m
}

// EmittedPaths reports the relative paths the manifest in dir lists, for a
// caller (uninstall) that needs to remove exactly what was fanned out here —
// never a gentle-ai-owned skill list, since gentle-ai does not decide what
// openrecord ships. An absent or unreadable manifest reports no paths.
func EmittedPaths(dir string) []string {
	m := loadManifest(dir)
	paths := make([]string, 0, len(m.Entries))
	for _, entry := range m.Entries {
		paths = append(paths, entry.Path)
	}
	return paths
}

func hashContent(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}

// pruneStaleEntries removes files this target directory's previous emit wrote
// that the new manifest no longer lists, as long as the on-disk content still
// matches what was recorded. A locally edited file is left alone — the same
// "kept" outcome openrecord's own emit reports for a stale, hand-edited file.
func pruneStaleEntries(dir string, shipped manifest) error {
	previous := loadManifest(dir)
	if len(previous.Entries) == 0 {
		return nil
	}
	stillShipped := make(map[string]bool, len(shipped.Entries))
	for _, entry := range shipped.Entries {
		stillShipped[entry.Path] = true
	}
	for _, entry := range previous.Entries {
		if stillShipped[entry.Path] {
			continue
		}
		target := filepath.Join(dir, filepath.FromSlash(entry.Path))
		content, err := os.ReadFile(target)
		if err != nil {
			continue
		}
		if hashContent(content) != entry.Hash {
			continue
		}
		if err := os.Remove(target); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}
