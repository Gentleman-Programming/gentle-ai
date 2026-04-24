package storage

import "fmt"

// CheckAvailableSpace returns the number of free bytes on the volume
// containing the given path.
func CheckAvailableSpace(path string) (uint64, error) {
	return checkAvailableSpace(path)
}

// RequireFreeSpace validates that the volume containing path has at least
// minBytes of free space. Returns a descriptive error if insufficient.
func RequireFreeSpace(path string, minBytes uint64) error {
	available, err := CheckAvailableSpace(path)
	if err != nil {
		return fmt.Errorf("check disk space for %q: %w", path, err)
	}
	if available < minBytes {
		return fmt.Errorf("insufficient disk space at %q: need %s, have %s",
			path, formatBytes(minBytes), formatBytes(available))
	}
	return nil
}