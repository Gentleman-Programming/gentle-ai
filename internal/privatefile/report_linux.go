//go:build linux

package privatefile

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

// WriteNewReport writes a bounded private report to a new owner-only file.
// A failure may leave a protected partial or complete file at the destination.
func WriteNewReport(absoluteDestination string, payload []byte) error {
	if len(payload) == 0 || len(payload) > maxReportSize {
		return ErrInvalidPayload
	}
	if !filepath.IsAbs(absoluteDestination) || filepath.Clean(absoluteDestination) != absoluteDestination {
		return ErrInvalidDestination
	}
	name := filepath.Base(absoluteDestination)
	if !safeBasename(name) {
		return ErrInvalidDestination
	}

	dirFD, err := openDirectory(filepath.Dir(absoluteDestination))
	if err != nil {
		return ErrUnsafeParent
	}
	defer unix.Close(dirFD)

	var parent unix.Stat_t
	if unix.Fstat(dirFD, &parent) != nil || parent.Uid != uint32(os.Geteuid()) || parent.Mode&0022 != 0 {
		return ErrUnsafeParent
	}

	fileFD, err := unix.Openat(dirFD, name, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0600)
	if errors.Is(err, unix.EEXIST) {
		return ErrDestinationExists
	}
	if err != nil {
		return ErrWriteFailed
	}
	file := os.NewFile(uintptr(fileFD), "private-report")
	if unix.Fchmod(fileFD, 0600) != nil {
		file.Close()
		return ErrWriteFailed
	}
	n, writeErr := file.Write(payload)
	if writeErr == nil && n != len(payload) {
		writeErr = io.ErrShortWrite
	}
	if writeErr == nil {
		writeErr = file.Sync()
	}
	closeErr := file.Close()
	if writeErr != nil || closeErr != nil || unix.Fsync(dirFD) != nil {
		return ErrWriteFailed
	}
	return nil
}

func safeBasename(name string) bool {
	if len(name) == 0 || len(name) > 255 || !asciiAlphaNum(name[0]) {
		return false
	}
	for i := 1; i < len(name); i++ {
		c := name[i]
		if !asciiAlphaNum(c) && c != '.' && c != '_' && c != '-' {
			return false
		}
	}
	return true
}

func asciiAlphaNum(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9'
}

func openDirectory(path string) (int, error) {
	fd, err := unix.Open("/", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		return -1, err
	}
	for _, component := range strings.Split(strings.TrimPrefix(path, "/"), "/") {
		if component == "" {
			continue
		}
		next, openErr := unix.Openat(fd, component, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
		unix.Close(fd)
		if openErr != nil {
			return -1, openErr
		}
		fd = next
	}
	return fd, nil
}
