//go:build windows

package reviewtransaction

import (
	"errors"
	"io/fs"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows"
)

func publishNoReplace(source, destination string) error {
	if err := moveFile(source, destination, windows.MOVEFILE_WRITE_THROUGH); err != nil {
		if errors.Is(err, windows.ERROR_ALREADY_EXISTS) || errors.Is(err, windows.ERROR_FILE_EXISTS) {
			return fs.ErrExist
		}
		return err
	}
	return nil
}

func replaceFileAtomic(source, destination string) error {
	return moveFile(source, destination, windows.MOVEFILE_REPLACE_EXISTING|windows.MOVEFILE_WRITE_THROUGH)
}

func moveFile(source, destination string, flags uint32) error {
	source, err := extendedWindowsPath(source)
	if err != nil {
		return err
	}
	destination, err = extendedWindowsPath(destination)
	if err != nil {
		return err
	}
	from, err := windows.UTF16PtrFromString(source)
	if err != nil {
		return err
	}
	to, err := windows.UTF16PtrFromString(destination)
	if err != nil {
		return err
	}
	return windows.MoveFileEx(from, to, flags)
}

func extendedWindowsPath(path string) (string, error) {
	if strings.HasPrefix(path, `\\?\`) {
		return filepath.Clean(path), nil
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	absolute = filepath.Clean(absolute)
	if strings.HasPrefix(absolute, `\\`) {
		return `\\?\UNC\` + strings.TrimPrefix(absolute, `\\`), nil
	}
	return `\\?\` + absolute, nil
}
