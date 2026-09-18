//go:build windows

package filecoord

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"

	"golang.org/x/sys/windows"
)

func readPlatformSnapshot(path string, info fs.FileInfo) (*Snapshot, error) {
	u16, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, fmt.Errorf("utf16 path %q: %w", path, err)
	}

	handle, err := windows.CreateFile(
		u16,
		windows.GENERIC_READ,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil,
		windows.OPEN_EXISTING,
		windows.FILE_FLAG_OPEN_REPARSE_POINT|windows.FILE_ATTRIBUTE_NORMAL,
		0,
	)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) || errors.Is(err, windows.ERROR_FILE_NOT_FOUND) {
			return &Snapshot{Path: path, Exists: false}, nil
		}
		return nil, fmt.Errorf("createfile target %q: %w", path, err)
	}
	defer windows.CloseHandle(handle)

	var fileInfo windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(handle, &fileInfo); err != nil {
		return nil, fmt.Errorf("get file info %q: %w", path, err)
	}

	if fileInfo.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		return nil, ErrSymlinkTarget
	}
	if fileInfo.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY != 0 {
		return nil, ErrNonRegularTarget
	}

	file := os.NewFile(uintptr(handle), path)

	data, err := io.ReadAll(io.LimitReader(file, maxSnapshotFileSize+1))
	if err != nil {
		return nil, fmt.Errorf("read target %q: %w", path, err)
	}
	if int64(len(data)) > maxSnapshotFileSize {
		return nil, ErrOversizedTarget
	}

	fileIndex := (uint64(fileInfo.FileIndexHigh) << 32) | uint64(fileInfo.FileIndexLow)
	modTime := fileInfo.LastWriteTime.Nanoseconds()

	identity := FileIdentity{
		Dev:     uint64(fileInfo.VolumeSerialNumber),
		Ino:     fileIndex,
		Size:    int64((uint64(fileInfo.FileSizeHigh) << 32) | uint64(fileInfo.FileSizeLow)),
		ModTime: modTime,
		Hash:    computeHash(data),
	}

	return &Snapshot{
		Path:     path,
		Bytes:    data,
		Mode:     info.Mode(),
		Identity: identity,
		Exists:   true,
	}, nil
}
