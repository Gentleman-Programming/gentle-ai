package engram

import (
	"path/filepath"
)

// DataDirRef is the persisted reference to an Engram data directory.
// Currently resolves to a local filesystem path.
// Future URI schemes (e.g. "provider://...") can be added inside Resolve
// without changing the JSON format or breaking existing installations.
type DataDirRef string

// Resolve returns the effective local filesystem path for the data directory.
// An empty ref resolves to the platform default (~/.engram).
func (r DataDirRef) Resolve(homeDir string) string {
	if r == "" {
		return DefaultDir(homeDir)
	}
	// filepath.FromSlash normalises Windows path separators on any input,
	// filepath.Clean removes redundant separators and dots.
	return filepath.Clean(filepath.FromSlash(string(r)))
}

// DefaultDir returns the default Engram data directory path (~/.engram).
func DefaultDir(homeDir string) string {
	return filepath.Join(homeDir, ".engram")
}

// DBPath returns the path to the Engram SQLite database inside dataDir.
func DBPath(dataDir string) string {
	return filepath.Join(dataDir, "engram.db")
}
