//go:build windows

package opencode

import (
	"bytes"
	"fmt"
	"io"
	"os"

	"golang.org/x/sys/windows"
)

// RemoveManagedLauncher validates and removes a regular, canonical managed
// launcher through its opened Windows handle. Deletion is requested on that
// handle, never on a subsequently resolved path.
func RemoveManagedLauncher(path string) (ManagedLauncherRemovalResult, error) {
	initial, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return ManagedLauncherRemovalResult{Status: ManagedLauncherRemovalAbsent}, nil
	}
	if err != nil {
		return ManagedLauncherRemovalResult{}, err
	}
	if !initial.Mode().IsRegular() {
		return ManagedLauncherRemovalResult{Status: ManagedLauncherRemovalRefused}, nil
	}

	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return ManagedLauncherRemovalResult{}, fmt.Errorf("encode managed launcher %q: %w", path, err)
	}
	handle, err := windows.CreateFile(
		name,
		windows.GENERIC_READ|windows.DELETE,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil,
		windows.OPEN_EXISTING,
		windows.FILE_ATTRIBUTE_NORMAL|windows.FILE_FLAG_OPEN_REPARSE_POINT,
		0,
	)
	if os.IsNotExist(err) {
		return ManagedLauncherRemovalResult{Status: ManagedLauncherRemovalRefused}, nil
	}
	if err != nil {
		return ManagedLauncherRemovalResult{}, fmt.Errorf("open managed launcher %q: %w", path, err)
	}
	file := os.NewFile(uintptr(handle), path)
	if file == nil {
		_ = windows.CloseHandle(handle)
		return ManagedLauncherRemovalResult{}, fmt.Errorf("open managed launcher %q: create file handle", path)
	}
	defer file.Close()

	var information windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(handle, &information); err != nil {
		return ManagedLauncherRemovalResult{}, fmt.Errorf("inspect managed launcher %q: %w", path, err)
	}
	fileType, err := windows.GetFileType(handle)
	if err != nil {
		return ManagedLauncherRemovalResult{}, fmt.Errorf("identify managed launcher %q: %w", path, err)
	}
	if fileType != windows.FILE_TYPE_DISK || information.FileAttributes&(windows.FILE_ATTRIBUTE_DIRECTORY|windows.FILE_ATTRIBUTE_DEVICE|windows.FILE_ATTRIBUTE_REPARSE_POINT) != 0 {
		return ManagedLauncherRemovalResult{Status: ManagedLauncherRemovalRefused}, nil
	}

	opened, err := file.Stat()
	if err != nil {
		return ManagedLauncherRemovalResult{}, fmt.Errorf("stat managed launcher %q: %w", path, err)
	}
	if !opened.Mode().IsRegular() || !os.SameFile(initial, opened) {
		return ManagedLauncherRemovalResult{Status: ManagedLauncherRemovalRefused}, nil
	}
	data, err := io.ReadAll(file)
	if err != nil {
		return ManagedLauncherRemovalResult{}, fmt.Errorf("read managed launcher %q: %w", path, err)
	}
	if !IsManagedLauncher(path, data) {
		return ManagedLauncherRemovalResult{Status: ManagedLauncherRemovalNotOwned}, nil
	}

	managedLauncherRemovalBeforeDelete(path)

	current, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return ManagedLauncherRemovalResult{Status: ManagedLauncherRemovalRefused}, nil
	}
	if err != nil {
		return ManagedLauncherRemovalResult{}, err
	}
	if !current.Mode().IsRegular() || !os.SameFile(opened, current) {
		return ManagedLauncherRemovalResult{Status: ManagedLauncherRemovalRefused}, nil
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return ManagedLauncherRemovalResult{}, fmt.Errorf("rewind managed launcher %q: %w", path, err)
	}
	currentData, err := io.ReadAll(file)
	if err != nil {
		return ManagedLauncherRemovalResult{}, fmt.Errorf("re-read managed launcher %q: %w", path, err)
	}
	if !bytes.Equal(data, currentData) || !IsManagedLauncher(path, currentData) {
		return ManagedLauncherRemovalResult{Status: ManagedLauncherRemovalRefused}, nil
	}

	managedLauncherRemovalBeforeUnlink(path)

	deleteFile := byte(1)
	var status windows.IO_STATUS_BLOCK
	if err := windows.NtSetInformationFile(handle, &status, &deleteFile, 1, windows.FileDispositionInformation); err != nil {
		return ManagedLauncherRemovalResult{}, fmt.Errorf("remove managed launcher %q: %w", path, err)
	}
	return ManagedLauncherRemovalResult{Status: ManagedLauncherRemovalRemoved}, nil
}
