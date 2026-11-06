//go:build linux || darwin

package shellinstaller

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"

	"golang.org/x/sys/unix"
)

// Bind the opened descriptor to the physical selection before reading bytes.
// Nonblocking, nofollow opens also refuse a substituted FIFO without waiting.
func privateNativeOpenFile(path string, before os.FileInfo) (*os.File, error) {
	file, err := os.OpenFile(path, os.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}
	opened, err := file.Stat()
	if err != nil || !os.SameFile(before, opened) || privateStamp(before) != privateStamp(opened) {
		return nil, privateError("source", errors.Join(err, file.Close(), errors.New("native file changed before descriptor read")))
	}
	return file, nil
}

func privateNativeFile(ctx context.Context, path string, mode os.FileMode, size int64, pin string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	info, err := privatePhysical(path)
	if err != nil || info.Sys().(*syscall.Stat_t).Uid != uint32(os.Getuid()) || (mode != 0 && info.Mode() != mode) || (mode == 0 && info.Mode() != 0600 && info.Mode() != 0644 && info.Mode() != 0700 && info.Mode() != 0755) || info.Size() > 268435456 || (size >= 0 && info.Size() != size) {
		return "", privateError("source", errors.Join(err, fmt.Errorf("native file refused: %s metadata=%v", path, info)))
	}
	file, err := privateNativeOpenFile(path, info)
	if err != nil {
		return "", err
	}
	hash := sha256.New()
	buffer := make([]byte, 65536)
	var count int64
	for err == nil {
		var n int
		n, err = file.Read(buffer)
		count += int64(n)
		_, _ = hash.Write(buffer[:n])
		if contextErr := ctx.Err(); contextErr != nil {
			err = contextErr
		}
		if count > 268435456 {
			err = privateError("source", nil)
		}
	}
	if errors.Is(err, io.EOF) {
		err = nil
	}
	err = errors.Join(err, file.Close())
	fresh, freshErr := privatePhysical(path)
	digest := fmt.Sprintf("%x", hash.Sum(nil))
	if err != nil || freshErr != nil || privateStamp(info) != privateStamp(fresh) || count != info.Size() || (pin != "" && digest != pin) {
		return "", privateError("source", errors.Join(err, freshErr))
	}
	return privateStamp(info) + ":" + digest, nil
}

func privateNativeDirectory(path string) error {
	info, err := os.Lstat(path)
	canonical, canonicalErr := filepath.EvalSymlinks(path)
	if err != nil || canonicalErr != nil || canonical != path || (info.Mode() != os.ModeDir|0700 && info.Mode() != os.ModeDir|0755) || info.Sys().(*syscall.Stat_t).Uid != uint32(os.Getuid()) {
		return privateError("source", errors.Join(err, canonicalErr))
	}
	if err := privateExtendedMetadata(path, info, true); err != nil {
		return privateError("source", err)
	}
	return nil
}

// privateNativeHome accepts the operator's home when group read/traverse is
// present. It is strictly more private than the already accepted 0755 and is
// never used for owned install directories.
func privateNativeHome(path string) error {
	info, err := os.Lstat(path)
	canonical, canonicalErr := filepath.EvalSymlinks(path)
	if err != nil || canonicalErr != nil || canonical != path || (info.Mode() != os.ModeDir|0700 && info.Mode() != os.ModeDir|0750 && info.Mode() != os.ModeDir|0755) || info.Sys().(*syscall.Stat_t).Uid != uint32(os.Getuid()) {
		return privateError("source", errors.Join(err, canonicalErr))
	}
	if err := privateExtendedMetadata(path, info, true); err != nil {
		return privateError("source", err)
	}
	return nil
}

func privateNativeEntries(path string) ([]os.DirEntry, error) {
	directory, err := os.OpenFile(path, os.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}
	entries, err := directory.ReadDir(17)
	if errors.Is(err, io.EOF) {
		err = nil
	}
	return entries, errors.Join(err, directory.Close())
}
