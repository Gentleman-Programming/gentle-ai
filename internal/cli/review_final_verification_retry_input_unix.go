//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package cli

import (
	"context"
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

func openFinalVerificationIncidentInput(ctx context.Context, path string) (*os.File, func() error, error) {
	if path == "-" {
		return openFacadeArtifactManifestInput(ctx, path)
	}
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return nil, nil, err
	}
	if !info.Mode().IsRegular() && info.Mode()&os.ModeNamedPipe == 0 {
		return nil, nil, errors.New("final-verification incident input must be a regular file or FIFO")
	}
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_NONBLOCK, 0)
	if err != nil {
		return nil, nil, err
	}
	unix.CloseOnExec(fd)
	file := os.NewFile(uintptr(fd), path)
	if file == nil {
		_ = unix.Close(fd)
		return nil, nil, errors.New("open final-verification incident input")
	}
	opened, err := file.Stat()
	if err != nil || !os.SameFile(info, opened) ||
		opened.Mode().IsRegular() != info.Mode().IsRegular() ||
		(opened.Mode()&os.ModeNamedPipe != 0) != (info.Mode()&os.ModeNamedPipe != 0) {
		_ = file.Close()
		return nil, nil, errors.New("final-verification incident input changed during open")
	}
	if err := ctx.Err(); err != nil {
		_ = file.Close()
		return nil, nil, err
	}
	return file, func() error { return nil }, nil
}
