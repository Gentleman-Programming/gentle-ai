package engram

import (
	"os"
	"path/filepath"

	"github.com/gentleman-programming/gentle-ai/internal/storage"
)

// Location is a candidate Engram data directory the user can choose from.
type Location struct {
	Path      string // absolute, cleaned filesystem path
	Label     string // human-readable: "Default (~/.engram)", "D:\\ (120 GiB free)"
	Available int64  // bytes free on the containing volume; -1 if unknown
	IsCurrent bool   // true when this matches the currently active data dir
}

// SuggestLocations returns an ordered list of candidate data directories.
// The current location is always listed first. The default ~/.engram follows,
// then any additional volumes detected on the platform (see platform files).
// Duplicate paths are deduplicated; labels include free-space info where available.
func SuggestLocations(homeDir, currentDir string) []Location {
	return SuggestLocationsWithKnown(homeDir, currentDir, nil)
}

// SuggestLocationsWithKnown returns suggested locations plus user-known data
// directories, such as custom copy destinations from previous management ops.
func SuggestLocationsWithKnown(homeDir, currentDir string, knownDirs []string) []Location {
	seen := map[string]bool{}
	var out []Location

	add := func(path string, isCurrent bool) {
		clean := filepath.Clean(path)
		if seen[clean] {
			return
		}
		seen[clean] = true
		avail, err := storage.AvailableBytes(filepath.Dir(clean))
		if err != nil {
			avail = -1
		}
		label := buildLabel(clean, homeDir, avail)
		out = append(out, Location{
			Path:      clean,
			Label:     label,
			Available: avail,
			IsCurrent: isCurrent,
		})
	}

	// Current location first (may equal default).
	if currentDir != "" {
		add(currentDir, true)
	}

	// Default location — isCurrent only when no explicit current dir was provided.
	// If currentDir equals the default the seen-map dedup above already handles it;
	// if currentDir is a custom path the default is a non-current alternative.
	add(DefaultDir(homeDir), currentDir == "")

	for _, dir := range knownDirs {
		add(dir, filepath.Clean(dir) == filepath.Clean(currentDir))
	}

	// Platform-specific extra volumes (see locations_unix.go / locations_windows.go).
	for _, vol := range platformVolumes() {
		add(filepath.Join(vol, "Engram"), false)
	}

	return out
}

// buildLabel constructs the display label for a location entry.
func buildLabel(path, homeDir string, available int64) string {
	// Replace home prefix with ~ for readability.
	display := path
	if rel, err := filepath.Rel(homeDir, path); err == nil && len(rel) < len(path) {
		display = filepath.Join("~", rel)
	}

	if available < 0 {
		return display
	}
	return display + "  (" + storage.FormatBytes(available) + " free)"
}

// dirExists is a small helper used by platform files.
func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
