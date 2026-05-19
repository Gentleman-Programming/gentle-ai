//go:build !windows

package engram

import "path/filepath"

func platformVolumes() []string {
	var roots []string
	for _, glob := range []string{"/Volumes/*", "/mnt/*", "/media/*/*"} {
		matches, _ := filepath.Glob(glob)
		for _, m := range matches {
			if dirExists(m) {
				roots = append(roots, m)
			}
		}
	}
	return roots
}
