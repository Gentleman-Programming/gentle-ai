//go:build linux && amd64

package shellinstaller

import (
	"context"
	"errors"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

// userPNPMSyncTree flushes copied regular files through the bounded queue.
// Links are deliberately not followed: generated links can name the not-yet-
// published destination. The subsequent inventory still checks their exact text.
func userPNPMSyncTree(ctx context.Context, reader *userPNPMInventoryReader, submit func(userPNPMSyncFile) error) error {
	if reader == nil {
		// Older kernels retain the immediate copy-and-sync path.
		return nil
	}
	verifyRoot := func() error {
		held, err := reader.directory.Stat()
		current, currentErr := privateDirectory(reader.root)
		if err != nil || currentErr != nil || !os.SameFile(held, current) {
			return privateError("source", errors.Join(err, currentErr))
		}
		return nil
	}
	if err := verifyRoot(); err != nil {
		return err
	}
	err := filepath.WalkDir(reader.root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return nil
		}
		if !info.Mode().IsRegular() {
			return privateError("source", nil)
		}
		// Root-relative openat2 refuses replaced ancestors, links and escapes.
		// NONBLOCK prevents a raced-in FIFO from hanging before regularity checks.
		file, err := reader.open(path, unix.O_RDONLY|unix.O_NONBLOCK)
		if err != nil {
			return err
		}
		opened, err := file.Stat()
		if err != nil || !opened.Mode().IsRegular() || privateStamp(info) != privateStamp(opened) {
			return privateError("source", errors.Join(err, file.Close()))
		}
		return submit(file)
	})
	if err != nil {
		return err
	}
	return verifyRoot()
}
