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
)

func privateNativeFile(ctx context.Context, path string, mode os.FileMode, size int64, pin string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	info, err := privatePhysical(path)
	if err != nil || info.Sys().(*syscall.Stat_t).Uid != uint32(os.Getuid()) || (mode != 0 && info.Mode() != mode) || (mode == 0 && info.Mode() != 0600 && info.Mode() != 0644 && info.Mode() != 0700 && info.Mode() != 0755) || info.Size() > 268435456 || (size >= 0 && info.Size() != size) {
		return "", privateError("source", errors.Join(err, fmt.Errorf("native file refused: %s metadata=%v", path, info)))
	}
	file, err := os.Open(path)
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

func privateNativeEntries(path string) ([]os.DirEntry, error) {
	directory, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	entries, err := directory.ReadDir(17)
	if errors.Is(err, io.EOF) {
		err = nil
	}
	return entries, errors.Join(err, directory.Close())
}
