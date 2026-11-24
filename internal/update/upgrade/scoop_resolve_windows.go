//go:build windows

package upgrade

import (
	"fmt"
	"os"
	"strings"

	"golang.org/x/sys/windows"
)

func scoopResolvePath(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()

	buffer := make([]uint16, 260)
	for {
		length, err := windows.GetFinalPathNameByHandle(windows.Handle(file.Fd()), &buffer[0], uint32(len(buffer)), 0)
		if err != nil {
			return "", fmt.Errorf("resolve final path %q: %w", path, err)
		}
		if length < uint32(len(buffer)) {
			return normalizeWindowsPath(windows.UTF16ToString(buffer[:length])), nil
		}
		buffer = make([]uint16, length+1)
	}
}

func normalizeWindowsPath(path string) string {
	path = strings.TrimPrefix(path, `\\?\`)
	if strings.HasPrefix(path, `UNC\`) {
		return `\\` + strings.TrimPrefix(path, `UNC\`)
	}
	return path
}
