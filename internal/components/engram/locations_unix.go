//go:build !windows

package engram

import "path/filepath"

// maxPlatformVolumes caps the number of suggested external volumes to avoid
// flooding the location picker on systems with many mount points.
const maxPlatformVolumes = 20

// platformVolumes returns mount-point directories to suggest as alternative
// Engram data locations on macOS (/Volumes/*) and Linux (/mnt/*, /media/*/*).
// At most maxPlatformVolumes results are returned.
func platformVolumes() []string {
	var roots []string

	for _, glob := range []string{
		"/Volumes/*",       // macOS external drives
		"/mnt/*",           // Linux manual mounts
		"/media/*/*",       // Linux user-automounted drives (udisks2)
	} {
		if len(roots) >= maxPlatformVolumes {
			break
		}
		matches, _ := filepath.Glob(glob)
		for _, m := range matches {
			if len(roots) >= maxPlatformVolumes {
				break
			}
			if dirExists(m) {
				roots = append(roots, m)
			}
		}
	}
	return roots
}
