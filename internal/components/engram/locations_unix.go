//go:build !windows

package engram

import "path/filepath"

// platformVolumes returns mount-point directories to suggest as alternative
// Engram data locations on macOS (/Volumes/*) and Linux (/mnt/*, /media/*/*).
func platformVolumes() []string {
	var roots []string

	for _, glob := range []string{
		"/Volumes/*",       // macOS external drives
		"/mnt/*",           // Linux manual mounts
		"/media/*/*",       // Linux user-automounted drives (udisks2)
	} {
		matches, _ := filepath.Glob(glob)
		for _, m := range matches {
			if dirExists(m) {
				roots = append(roots, m)
			}
		}
	}
	return roots
}
