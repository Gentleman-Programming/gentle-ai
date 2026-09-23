//go:build !windows

package filecoord

import (
	"fmt"
	"io"
	"io/fs"
	"os"

	"golang.org/x/sys/unix"
)

func readPlatformSnapshot(path string, info fs.FileInfo) (*Snapshot, error) {
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		if err == unix.ELOOP || err == unix.EMLINK {
			return nil, ErrSymlinkTarget
		}
		if err == unix.ENOENT {
			return &Snapshot{Path: path, Exists: false}, nil
		}
		return nil, fmt.Errorf("open target %q with nofollow: %w", path, err)
	}

	file := os.NewFile(uintptr(fd), path)
	defer file.Close()

	var stat unix.Stat_t
	if err := unix.Fstat(fd, &stat); err != nil {
		return nil, fmt.Errorf("fstat target %q: %w", path, err)
	}

	if stat.Mode&unix.S_IFMT != unix.S_IFREG {
		return nil, ErrNonRegularTarget
	}

	data, err := io.ReadAll(io.LimitReader(file, maxSnapshotFileSize+1))
	if err != nil {
		return nil, fmt.Errorf("read target %q: %w", path, err)
	}
	if int64(len(data)) > maxSnapshotFileSize {
		return nil, ErrOversizedTarget
	}

	identity := FileIdentity{
		Dev:     uint64(stat.Dev),
		Ino:     uint64(stat.Ino),
		Size:    stat.Size,
		ModTime: stat.Mtim.Nano(),
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
