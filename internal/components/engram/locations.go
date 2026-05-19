package engram

import (
	"os"
	"path/filepath"

	"github.com/gentleman-programming/gentle-ai/internal/storage"
)

// Location is a candidate Engram data directory the user can choose from.
type Location struct {
	Path      string
	Label     string
	Available int64
	IsCurrent bool
}

// SuggestLocations returns ordered candidate data directories.
func SuggestLocations(homeDir, currentDir string) []Location {
	return SuggestLocationsWithKnown(homeDir, currentDir, nil)
}

// SuggestLocationsWithKnown adds remembered non-active dirs after the default.
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
		out = append(out, Location{
			Path:      clean,
			Label:     buildLabel(clean, homeDir, avail),
			Available: avail,
			IsCurrent: isCurrent,
		})
	}

	if currentDir != "" {
		add(currentDir, true)
	}
	add(DefaultDir(homeDir), currentDir == "")
	for _, dir := range knownDirs {
		add(dir, false)
	}
	for _, vol := range platformVolumes() {
		add(filepath.Join(vol, "Engram"), false)
	}
	return out
}

func buildLabel(path, homeDir string, available int64) string {
	display := path
	if rel, err := filepath.Rel(homeDir, path); err == nil && len(rel) < len(path) {
		display = filepath.Join("~", rel)
	}
	if available < 0 {
		return display
	}
	return display + "  (" + storage.FormatBytes(available) + " free)"
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
