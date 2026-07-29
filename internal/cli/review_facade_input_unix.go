//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package cli

import (
	"context"
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

func openFacadeArtifactManifestInput(ctx context.Context, path string) (*os.File, func() error, error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	if path != "-" {
		return openFacadeArtifactManifestRegularFile(ctx, path)
	}

	source := os.Stdin
	raw, err := source.SyscallConn()
	if err != nil {
		return nil, nil, err
	}
	flags, fd := 0, -1
	var controlErr error
	if err := raw.Control(func(sourceFD uintptr) {
		flags, controlErr = unix.FcntlInt(sourceFD, unix.F_GETFL, 0)
		if controlErr == nil {
			fd, controlErr = unix.Dup(int(sourceFD))
		}
	}); err != nil || controlErr != nil {
		if fd >= 0 {
			_ = unix.Close(fd)
		}
		if err != nil {
			return nil, nil, err
		}
		return nil, nil, controlErr
	}
	restore := func() error {
		var restoreErr error
		if err := raw.Control(func(sourceFD uintptr) {
			_, restoreErr = unix.FcntlInt(sourceFD, unix.F_SETFL, flags)
		}); err != nil {
			return err
		}
		return restoreErr
	}
	if err := unix.SetNonblock(fd, true); err != nil {
		_ = unix.Close(fd)
		_ = restore()
		return nil, nil, err
	}
	unix.CloseOnExec(fd)
	file := os.NewFile(uintptr(fd), source.Name())
	if file == nil {
		_ = restore()
		_ = unix.Close(fd)
		return nil, nil, errors.New("duplicate facade input")
	}
	if err := ctx.Err(); err != nil {
		_ = file.Close()
		_ = restore()
		return nil, nil, err
	}
	return file, restore, nil
}

func openFacadeArtifactManifestRegularFile(ctx context.Context, path string) (*os.File, func() error, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, nil, errFacadeArtifactManifestInputNotRegular
	}
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}

	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_NONBLOCK, 0)
	if err != nil {
		if current, statErr := os.Stat(path); statErr == nil && !current.Mode().IsRegular() {
			return nil, nil, errFacadeArtifactManifestInputNotRegular
		}
		return nil, nil, err
	}
	unix.CloseOnExec(fd)
	file := os.NewFile(uintptr(fd), path)
	if file == nil {
		_ = unix.Close(fd)
		return nil, nil, errors.New("open facade artifact manifest input")
	}
	descriptorInfo, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, nil, err
	}
	if !descriptorInfo.Mode().IsRegular() {
		_ = file.Close()
		return nil, nil, errFacadeArtifactManifestInputNotRegular
	}
	if err := ctx.Err(); err != nil {
		_ = file.Close()
		return nil, nil, err
	}
	return file, func() error { return nil }, nil
}

func cancelFacadeArtifactManifestInput(*os.File) {}
