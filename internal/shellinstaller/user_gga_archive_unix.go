//go:build linux || darwin

package shellinstaller

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"syscall"

	"golang.org/x/sys/unix"
)

func userGGAOpenArchive(path string) (*os.File, error) {
	return os.OpenFile(path, os.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
}

// Authenticate a fixed-size descriptor, not a second unbounded pathname read.
// The open seam permits controlled replacement tests on owned fixture files.
func userGGAReadArchive(ctx context.Context, path string, open func(string) (*os.File, error)) (data []byte, err error) {
	if ctx == nil || open == nil {
		return nil, privateError("source", errors.New("missing GGA archive reader context/opener"))
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	before, err := privatePhysical(path)
	if err != nil {
		return nil, err
	}
	stat := before.Sys().(*syscall.Stat_t)
	if before.Mode() != 0600 || before.Size() != 81841 || stat.Uid != uint32(os.Getuid()) || stat.Nlink != 1 {
		return nil, privateError("source", errors.New("GGA archive metadata differs"))
	}
	file, err := open(path)
	if err != nil {
		return nil, err
	}
	defer func() {
		err = errors.Join(err, file.Close(), ctx.Err())
		if err != nil {
			data = nil
		}
	}()
	opened, err := file.Stat()
	if err != nil || !os.SameFile(before, opened) || privateStamp(before) != privateStamp(opened) {
		return nil, privateError("source", errors.Join(err, errors.New("GGA archive changed before descriptor open")))
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	data, err = io.ReadAll(io.LimitReader(file, 81842))
	if err != nil || len(data) != 81841 || fmt.Sprintf("%x", sha256.Sum256(data)) != userGGASourceSHA {
		return nil, privateError("source", errors.Join(err, errors.New("GGA archive bytes differ")))
	}
	fresh, err := privatePhysical(path)
	if err != nil || !os.SameFile(before, fresh) || privateStamp(before) != privateStamp(fresh) || fresh.Sys().(*syscall.Stat_t).Nlink != 1 {
		return nil, privateError("source", errors.Join(err, errors.New("GGA archive changed during read")))
	}
	return data, nil
}
