//go:build !windows

package upgrade

import "path/filepath"

func scoopResolvePath(path string) (string, error) {
	return filepath.EvalSymlinks(path)
}
