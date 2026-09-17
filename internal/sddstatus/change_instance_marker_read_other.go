//go:build !windows

package sddstatus

import "os"

func readChangeInstanceMarkerFile(path string) ([]byte, error) {
	return os.ReadFile(path)
}
