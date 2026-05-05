package engram

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// LocationSuggestion describes a candidate destination for Engram data.
type LocationSuggestion struct {
	Label string
	Path  string
	Space uint64
}

// SuggestedLocations returns a list of candidate Engram data directories
// with their available disk space. It includes well-known paths and
// platform-specific drives / mount points.
func SuggestedLocations(backend DataBackend, homeDir string) []LocationSuggestion {
	var candidates []string

	// 1. Hard default (~/.engram)
	candidates = append(candidates, backend.HardDefaultDataDir())

	// 2. Documents subfolder
	candidates = append(candidates, filepath.Join(homeDir, "Documents", ".engram"))

	// 3. Platform-specific drives / volumes
	candidates = append(candidates, platformCandidates()...)

	// Deduplicate and build suggestions with space info.
	const maxSuggestions = 8
	seen := make(map[string]struct{}, len(candidates))
	suggestions := make([]LocationSuggestion, 0, maxSuggestions)
	for _, path := range candidates {
		clean := filepath.Clean(path)
		if _, ok := seen[clean]; ok {
			continue
		}
		seen[clean] = struct{}{}

		space, _ := backend.AvailableSpace(clean)
		suggestions = append(suggestions, LocationSuggestion{
			Label: labelForPath(clean, homeDir),
			Path:  clean,
			Space: space,
		})
		if len(suggestions) >= maxSuggestions {
			break
		}
	}

	return suggestions
}

func platformCandidates() []string {
	var candidates []string

	switch runtime.GOOS {
	case "windows":
		// Check common drive letters.
		for _, letter := range []string{"C", "D", "E", "F", "G", "H"} {
			drive := letter + ":\\"
			if _, err := os.Stat(drive); err == nil {
				candidates = append(candidates, filepath.Join(drive, "engram"))
			}
		}
	case "darwin":
		// External volumes on macOS.
		if entries, err := os.ReadDir("/Volumes"); err == nil {
			for _, entry := range entries {
				if entry.IsDir() {
					candidates = append(candidates, filepath.Join("/Volumes", entry.Name(), ".engram"))
				}
			}
		}
	default:
		// Linux / Unix mount points.
		for _, root := range []string{"/mnt", "/media"} {
			if entries, err := os.ReadDir(root); err == nil {
				for _, entry := range entries {
					if entry.IsDir() {
						candidates = append(candidates, filepath.Join(root, entry.Name(), ".engram"))
					}
				}
			}
		}
	}

	return candidates
}

func labelForPath(path, homeDir string) string {
	homeClean := filepath.Clean(homeDir)
	engramHome := filepath.Join(homeClean, ".engram")

	if filepath.Clean(path) == filepath.Clean(engramHome) {
		return "Home (~/.engram)"
	}

	if filepath.Clean(path) == filepath.Clean(filepath.Join(homeClean, "Documents", ".engram")) {
		return "Documents"
	}

	// Heuristic: any path ending in .engram under a user home directory.
	parent := filepath.Dir(path)
	if filepath.Base(path) == ".engram" {
		if strings.Contains(parent, `\Users\`) || strings.Contains(parent, `/Users/`) {
			return "Home (~/.engram)"
		}
		if strings.HasPrefix(parent, "/home/") {
			return "Home (~/.engram)"
		}
	}

	// For drive roots on Windows: D:\engram → "D:\"
	if runtime.GOOS == "windows" {
		if parent[len(parent)-1] == filepath.Separator {
			return parent
		}
		return parent + string(filepath.Separator)
	}

	// For mount points: /Volumes/MyDrive/.engram → "MyDrive"
	base := filepath.Base(parent)
	if base != "." && base != "/" {
		return base
	}

	return path
}

// NormalizeTargetPath converts a user-selected drive or mount root into the
// concrete Engram data folder used there. Full folder paths are returned cleanly
// as-is. Examples: C:\ -> C:\engram on Windows, /mnt/usb -> /mnt/usb/.engram
// on Unix-like systems.
func NormalizeTargetPath(path string) string {
	clean := filepath.Clean(strings.TrimSpace(path))
	if clean == "." {
		return clean
	}

	if runtime.GOOS == "windows" {
		volume := filepath.VolumeName(clean)
		if volume != "" {
			rest := strings.TrimPrefix(clean, volume)
			if rest == "" || rest == string(filepath.Separator) || rest == `/` {
				return filepath.Join(volume+string(filepath.Separator), "engram")
			}
		}
		return clean
	}

	if clean == string(filepath.Separator) {
		return filepath.Join(clean, ".engram")
	}
	return clean
}
