//go:build !windows

package filemerge

import "os"

func replaceFileAtomic(sourcePath, destinationPath string) error {
	return os.Rename(sourcePath, destinationPath)
}
