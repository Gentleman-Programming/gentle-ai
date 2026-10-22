//go:build linux || darwin

package shellinstaller

import (
	"context"
	"errors"
	"io"
	"os"
	"syscall"

	"golang.org/x/sys/unix"
)

// Shared fixed-size source custody reader; caller authenticates the bytes.
// Reused from the CodeGraph retained-part contract, without widening its bound.
func userRetainedSource(ctx context.Context, path string, size int64) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	before, err := privatePhysical(path)
	if err != nil || size < 0 || size > 32<<20 || before.Mode() != 0600 || before.Size() != size || before.Sys().(*syscall.Stat_t).Uid != uint32(os.Getuid()) || before.Sys().(*syscall.Stat_t).Nlink != 1 {
		return nil, privateError("source", errors.Join(err, errors.New("CodeGraph source part custody differs")))
	}
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(fd), path)
	opened, err := file.Stat()
	if err != nil || !os.SameFile(before, opened) {
		return nil, errors.Join(err, file.Close(), errors.New("CodeGraph opened part differs"))
	}
	data, readErr := io.ReadAll(io.LimitReader(file, size+1))
	closeErr := file.Close()
	after, statErr := privatePhysical(path)
	if err := errors.Join(readErr, closeErr, statErr, ctx.Err()); err != nil || int64(len(data)) != size || privateStamp(before) != privateStamp(after) {
		return nil, privateError("source", errors.Join(err, errors.New("CodeGraph retained source changed")))
	}
	return data, nil
}
