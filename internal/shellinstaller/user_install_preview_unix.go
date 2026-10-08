//go:build linux || darwin

package shellinstaller

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"syscall"

	"golang.org/x/sys/unix"
)

func userPreviewReadSettings(req UserInstallRequest, inspected string) (data []byte, existing bool, err error) {
	check := func() error {
		token, err := InspectUserInstall(req)
		if err != nil || token != inspected {
			return errors.Join(err, errors.New("shared preview selection changed; inspect again"))
		}
		return nil
	}
	if err = check(); err != nil {
		return
	}
	defer func() {
		if err == nil {
			err = check()
		}
	}()
	if _, statErr := os.Lstat(req.Destination); statErr == nil {
		return nil, true, nil
	}
	fd, openErr := unix.Open(filepath.Join(req.SharedAgent, "settings.json"), unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if errors.Is(openErr, unix.ENOENT) {
		return nil, false, nil
	}
	if openErr != nil {
		return nil, false, openErr
	}
	file := os.NewFile(uintptr(fd), "shared settings")
	defer func() { err = errors.Join(err, file.Close()) }()
	info, err := file.Stat()
	if err != nil {
		return nil, false, err
	}
	const limit = 64 << 10
	if !info.Mode().IsRegular() || info.Mode().Perm()&0022 != 0 || info.Sys().(*syscall.Stat_t).Uid != uint32(os.Getuid()) || info.Size() > limit {
		return nil, false, errors.New("shared settings must be an owned non-writable regular file of at most 64 KiB")
	}
	data, err = io.ReadAll(io.LimitReader(file, limit+1))
	if err == nil && (len(data) > limit || int64(len(data)) != info.Size()) {
		err = errors.New("shared settings byte bound or preimage changed")
	}
	return data, false, err
}
