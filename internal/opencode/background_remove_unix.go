//go:build unix

package opencode

import (
	"bytes"
	cryptorand "crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

// RemoveManagedLauncher validates a launcher before atomically capturing the
// current directory entry into an implementation-owned quarantine name. The
// captured entry is then validated by identity and bytes before only that
// quarantine name is unlinked. A replacement at the public launcher path can
// therefore never be removed by the cleanup mutation.
func RemoveManagedLauncher(path string) (ManagedLauncherRemovalResult, error) {
	initial, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return ManagedLauncherRemovalResult{Status: ManagedLauncherRemovalAbsent}, nil
	}
	if err != nil {
		return ManagedLauncherRemovalResult{}, err
	}
	if !initial.Mode().IsRegular() {
		return ManagedLauncherRemovalResult{Status: ManagedLauncherRemovalRefused}, nil
	}

	parent, err := os.Open(filepath.Dir(path))
	if err != nil {
		return ManagedLauncherRemovalResult{}, fmt.Errorf("open managed launcher parent %q: %w", path, err)
	}
	defer parent.Close()

	parentFD := int(parent.Fd())
	name := filepath.Base(path)
	fd, err := unix.Openat(
		parentFD,
		name,
		unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK,
		0,
	)
	if err != nil {
		if errors.Is(err, unix.ENOENT) || errors.Is(err, unix.ELOOP) {
			return ManagedLauncherRemovalResult{Status: ManagedLauncherRemovalRefused}, nil
		}
		return ManagedLauncherRemovalResult{}, fmt.Errorf("open managed launcher %q: %w", path, err)
	}
	file := os.NewFile(uintptr(fd), path)
	if file == nil {
		_ = unix.Close(fd)
		return ManagedLauncherRemovalResult{}, fmt.Errorf("open managed launcher %q: create file handle", path)
	}
	defer file.Close()

	opened, err := file.Stat()
	if err != nil {
		return ManagedLauncherRemovalResult{}, fmt.Errorf("stat managed launcher %q: %w", path, err)
	}
	if !opened.Mode().IsRegular() || !os.SameFile(initial, opened) {
		return ManagedLauncherRemovalResult{Status: ManagedLauncherRemovalRefused}, nil
	}

	data, err := io.ReadAll(file)
	if err != nil {
		return ManagedLauncherRemovalResult{}, fmt.Errorf("read managed launcher %q: %w", path, err)
	}
	if !IsManagedLauncher(path, data) {
		return ManagedLauncherRemovalResult{Status: ManagedLauncherRemovalNotOwned}, nil
	}

	// Preserve the existing pre-capture test seam. In production this is a
	// no-op; any replacement made here is captured and restored below.
	managedLauncherRemovalBeforeDelete(path)

	quarantine, err := createLauncherQuarantine(parentFD)
	if err != nil {
		return ManagedLauncherRemovalResult{}, fmt.Errorf("reserve managed launcher quarantine %q: %w", path, err)
	}
	if err := unix.Renameat(parentFD, name, parentFD, quarantine); err != nil {
		_ = unix.Unlinkat(parentFD, quarantine, 0)
		if errors.Is(err, unix.ENOENT) {
			return ManagedLauncherRemovalResult{Status: ManagedLauncherRemovalRefused}, nil
		}
		return ManagedLauncherRemovalResult{}, fmt.Errorf("capture managed launcher %q: %w", path, err)
	}

	capturedFD, err := unix.Openat(
		parentFD,
		quarantine,
		unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK,
		0,
	)
	if err != nil {
		if restoreErr := restoreLauncherQuarantine(parentFD, quarantine, name); restoreErr != nil {
			return ManagedLauncherRemovalResult{}, errors.Join(
				fmt.Errorf("inspect captured managed launcher %q: %w", path, err),
				fmt.Errorf("restore captured managed launcher %q: %w", path, restoreErr),
			)
		}
		return ManagedLauncherRemovalResult{Status: ManagedLauncherRemovalRefused}, nil
	}
	captured := os.NewFile(uintptr(capturedFD), filepath.Join(filepath.Dir(path), quarantine))
	if captured == nil {
		_ = unix.Close(capturedFD)
		if restoreErr := restoreLauncherQuarantine(parentFD, quarantine, name); restoreErr != nil {
			return ManagedLauncherRemovalResult{}, fmt.Errorf("restore captured managed launcher %q after opening: %w", path, restoreErr)
		}
		return ManagedLauncherRemovalResult{Status: ManagedLauncherRemovalRefused}, nil
	}
	defer captured.Close()

	capturedInfo, err := captured.Stat()
	if err != nil {
		if restoreErr := restoreLauncherQuarantine(parentFD, quarantine, name); restoreErr != nil {
			return ManagedLauncherRemovalResult{}, errors.Join(
				fmt.Errorf("stat captured managed launcher %q: %w", path, err),
				fmt.Errorf("restore captured managed launcher %q: %w", path, restoreErr),
			)
		}
		return ManagedLauncherRemovalResult{Status: ManagedLauncherRemovalRefused}, nil
	}
	if !capturedInfo.Mode().IsRegular() {
		if restoreErr := restoreLauncherQuarantine(parentFD, quarantine, name); restoreErr != nil {
			return ManagedLauncherRemovalResult{}, fmt.Errorf("restore non-regular managed launcher %q: %w", path, restoreErr)
		}
		return ManagedLauncherRemovalResult{Status: ManagedLauncherRemovalRefused}, nil
	}

	capturedData, err := io.ReadAll(captured)
	if err != nil {
		if restoreErr := restoreLauncherQuarantine(parentFD, quarantine, name); restoreErr != nil {
			return ManagedLauncherRemovalResult{}, errors.Join(
				fmt.Errorf("read captured managed launcher %q: %w", path, err),
				fmt.Errorf("restore captured managed launcher %q: %w", path, restoreErr),
			)
		}
		return ManagedLauncherRemovalResult{Status: ManagedLauncherRemovalRefused}, nil
	}
	if !os.SameFile(initial, capturedInfo) || !bytes.Equal(data, capturedData) || !IsManagedLauncher(path, capturedData) {
		if restoreErr := restoreLauncherQuarantine(parentFD, quarantine, name); restoreErr != nil {
			return ManagedLauncherRemovalResult{}, errors.Join(
				fmt.Errorf("stale managed launcher substitution %q: captured entry is not the validated launcher", path),
				fmt.Errorf("restore captured managed launcher %q: %w", path, restoreErr),
			)
		}
		return ManagedLauncherRemovalResult{Status: ManagedLauncherRemovalRefused}, nil
	}

	// The public name is absent now. This seam inserts a replacement precisely
	// between final captured-entry validation and quarantine removal in tests.
	managedLauncherRemovalBeforeUnlink(path)

	if err := unix.Unlinkat(parentFD, quarantine, 0); err != nil {
		if errors.Is(err, unix.ENOENT) {
			return ManagedLauncherRemovalResult{Status: ManagedLauncherRemovalRefused}, nil
		}
		return ManagedLauncherRemovalResult{}, fmt.Errorf("remove captured managed launcher %q: %w", path, err)
	}
	return ManagedLauncherRemovalResult{Status: ManagedLauncherRemovalRemoved}, nil
}

func createLauncherQuarantine(parentFD int) (string, error) {
	const attempts = 8
	for i := 0; i < attempts; i++ {
		var suffix [16]byte
		if _, err := cryptorand.Read(suffix[:]); err != nil {
			return "", err
		}
		name := ".gentle-ai-remove-" + hex.EncodeToString(suffix[:])
		fd, err := unix.Openat(
			parentFD,
			name,
			unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_CLOEXEC|unix.O_NOFOLLOW,
			0o600,
		)
		if errors.Is(err, unix.EEXIST) {
			continue
		}
		if err != nil {
			return "", err
		}
		if err := unix.Close(fd); err != nil {
			_ = unix.Unlinkat(parentFD, name, 0)
			return "", err
		}
		return name, nil
	}
	return "", errors.New("could not allocate a unique launcher quarantine name")
}

func restoreLauncherQuarantine(parentFD int, quarantine, original string) error {
	// Linkat creates the original directory entry with no-replace semantics and
	// therefore cannot clobber a new entry that appeared while the captured
	// launcher was being validated. The source and destination are in one
	// directory, so this preserves regular files and symlinks without changing
	// their bytes or link target.
	if err := unix.Linkat(parentFD, quarantine, parentFD, original, 0); err != nil {
		return err
	}
	if err := unix.Unlinkat(parentFD, quarantine, 0); err != nil {
		return err
	}
	return nil
}
