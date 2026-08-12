//go:build !windows

package filemerge

import "os"

func resolveAtomicParentJunction(string, os.FileInfo) (string, os.FileInfo, bool, error) {
	return "", nil, false, nil
}
