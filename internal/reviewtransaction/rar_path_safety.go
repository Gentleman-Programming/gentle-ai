package reviewtransaction

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var (
	// refusal:by-design world-action: the exit is a filesystem repair (chown, takeown, icacls /setowner, or deleting the offending link) that only the operator can perform; no gentle-ai command may rewrite ownership of paths it refuses to trust
	errUnsafeRARAuthorityPath = errors.New(
		"unsafe RAR authority path; restore trusted ownership of the reported " +
			"path (chown on POSIX; takeown or icacls /setowner on Windows) or " +
			"remove the symlink/reparse point, then rerun the command",
	)
	errRARAuthorityPathReplaced = errors.New("RAR authority path changed during access")
)

// UnsafeRARPathError preserves the unsafe private RAR path for recovery.
type UnsafeRARPathError struct {
	Path      string
	Directory bool
	Cause     error
}

func (err *UnsafeRARPathError) Error() string {
	kind := "file"
	if err.Directory {
		kind = "directory"
	}
	return fmt.Sprintf("unsafe private RAR %s %q: %v", kind, err.Path, err.Cause)
}

func (err *UnsafeRARPathError) Unwrap() error { return err.Cause }

func unsafeRARPathError(path string, directory bool) error {
	return &UnsafeRARPathError{Path: path, Directory: directory, Cause: errUnsafeRARAuthorityPath}
}

func ensureRARRepositoryRoot(commonDir, root string, create bool) error {
	want := filepath.Join(
		"gentle-ai",
		"review-transactions",
		rarAuthorityDirectory,
		rarAuthorityVersion,
	)
	// rar-authority and every descendant are owner-only; gentle-ai and
	// review-transactions are the shared ancestors above it.
	return ensureRARDirectoryChain(commonDir, root, want, 2, create)
}

// ensureRARSwitchRoot validates the kill switch's own root, a sibling of
// review-transactions under gentle-ai. It reuses this file's walk, permission
// rules, and private-directory helpers unchanged; the only thing it does not
// reuse is the authority tree itself, because #2882 showed the switch must not
// be unreachable whenever that tree is damaged.
func ensureRARSwitchRoot(commonDir, root string, create bool) error {
	// The shape mirrors the authority path exactly -- two shared ancestors,
	// then owner-only from rar-authority down -- so the switch inherits the
	// proven permission layout and differs only in its second component.
	want := filepath.Join("gentle-ai", rddModeSwitchDirectory, rarAuthorityDirectory, rarAuthorityVersion)
	return ensureRARDirectoryChain(commonDir, root, want, 2, create)
}

func ensureRARDirectoryChain(commonDir, root, want string, privateFrom int, create bool) error {
	commonDir = filepath.Clean(commonDir)
	root = filepath.Clean(root)
	relative, err := filepath.Rel(commonDir, root)
	if err != nil || relative == "." || relative == ".." ||
		strings.HasPrefix(relative, ".."+string(filepath.Separator)) ||
		filepath.IsAbs(relative) {
		return errors.New("RAR authority root escapes the Git common directory")
	}
	if relative != want {
		return errors.New("RAR authority root is not the canonical Git-common-dir path")
	}
	if err := validateRARRepositoryParent(commonDir); err != nil {
		return fmt.Errorf("validate RAR Git common directory %q: %w", commonDir, err)
	}
	current := commonDir
	parts := strings.Split(relative, string(filepath.Separator))
	for index, part := range parts {
		if part == "" || part == "." || part == ".." {
			return errUnsafeRARAuthorityPath
		}
		parent := current
		current = filepath.Join(current, part)
		private := index >= privateFrom
		_, statErr := os.Lstat(current)
		if errors.Is(statErr, fs.ErrNotExist) && create {
			if private {
				created, createErr := createPrivateRARDirectory(current)
				if createErr != nil {
					return createErr
				}
				if created {
					if err := SyncReviewDirectory(parent); err != nil {
						return err
					}
				}
				_, statErr = os.Lstat(current)
			} else {
				// Pre-execution plan authority exists before a native terminal
				// receipt. Create missing owner-only parents without weakening
				// already-existing native review directory permissions.
				created, createErr := createPrivateRARDirectory(current)
				if createErr != nil {
					return createErr
				}
				if created {
					if err := SyncReviewDirectory(parent); err != nil {
						return err
					}
				}
				_, statErr = os.Lstat(current)
			}
		}
		if statErr != nil {
			return statErr
		}
		if private {
			if err := validatePrivateRARDirectory(current); err != nil {
				return fmt.Errorf("validate private RAR directory %q: %w", current, err)
			}
		} else if err := validateRARRepositoryParent(current); err != nil {
			return fmt.Errorf("validate RAR repository parent %q: %w", current, err)
		}
	}
	return nil
}

func ensurePrivateRARDirectoryTree(base, dir string, create bool) error {
	base = filepath.Clean(base)
	dir = filepath.Clean(dir)
	relative, err := filepath.Rel(base, dir)
	if err != nil || relative == ".." ||
		strings.HasPrefix(relative, ".."+string(filepath.Separator)) ||
		filepath.IsAbs(relative) {
		return errors.New("RAR private directory escapes repository root")
	}
	if err := validatePrivateRARDirectory(base); err != nil {
		return fmt.Errorf("validate RAR directory root %q: %w", base, err)
	}
	if relative == "." {
		return nil
	}
	current := base
	for _, part := range strings.Split(relative, string(filepath.Separator)) {
		if part == "" || part == "." || part == ".." {
			return errUnsafeRARAuthorityPath
		}
		parent := current
		current = filepath.Join(current, part)
		info, statErr := os.Lstat(current)
		if errors.Is(statErr, fs.ErrNotExist) && create {
			created, createErr := createPrivateRARDirectory(current)
			if createErr != nil {
				return createErr
			}
			if created {
				if err := SyncReviewDirectory(parent); err != nil {
					return err
				}
			}
			info, statErr = os.Lstat(current)
		}
		if statErr != nil {
			return statErr
		}
		if rarPathUnsafe(current, info) || !info.IsDir() {
			return unsafeRARPathError(current, true)
		}
		if err := validatePrivateRARDirectory(current); err != nil {
			return fmt.Errorf("validate nested private RAR directory %q: %w", current, err)
		}
	}
	return nil
}

func validatePrivateRARDirectory(path string) error {
	return validatePrivateRARPath(path, true)
}

func validatePrivateRARFile(path string) error {
	return validatePrivateRARPath(path, false)
}

func validatePrivateRARPath(path string, directory bool) error {
	before, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if rarPathUnsafe(path, before) || before.IsDir() != directory ||
		!privateRARPathSafe(path, before) {
		return unsafeRARPathError(path, directory)
	}
	file, err := openRARPathNoFollow(path, directory)
	if err != nil {
		return err
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil {
		return err
	}
	current, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !os.SameFile(before, opened) || !os.SameFile(opened, current) ||
		!privateOpenRARPathSafe(file, opened) {
		return errRARAuthorityPathReplaced
	}
	return nil
}

func readPrivateRARFile(path string) ([]byte, error) {
	before, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if rarPathUnsafe(path, before) || !before.Mode().IsRegular() ||
		!privateRARPathSafe(path, before) {
		return nil, unsafeRARPathError(path, false)
	}
	file, err := openRARPathNoFollow(path, false)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !opened.Mode().IsRegular() ||
		!os.SameFile(before, opened) ||
		!privateOpenRARPathSafe(file, opened) {
		if err != nil {
			return nil, err
		}
		return nil, errRARAuthorityPathReplaced
	}
	if opened.Size() < 1 || opened.Size() > rarAuthorityMaxBytes {
		return nil, errors.New("RAR authority file size is invalid")
	}
	payload, err := io.ReadAll(io.LimitReader(file, rarAuthorityMaxBytes+1))
	if err != nil {
		return nil, err
	}
	if len(payload) > rarAuthorityMaxBytes {
		return nil, errors.New("RAR authority file exceeds size limit")
	}
	afterOpen, err := file.Stat()
	if err != nil {
		return nil, err
	}
	current, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !os.SameFile(opened, afterOpen) || !os.SameFile(afterOpen, current) ||
		afterOpen.Size() != int64(len(payload)) ||
		!privateOpenRARPathSafe(file, afterOpen) {
		return nil, errRARAuthorityPathReplaced
	}
	return payload, nil
}

func publishPrivateRARImmutable(path string, payload []byte) error {
	if len(payload) == 0 || len(payload) > rarAuthorityMaxBytes {
		return errors.New("RAR immutable payload size is invalid")
	}
	dir := filepath.Dir(path)
	if err := validatePrivateRARDirectory(dir); err != nil {
		return err
	}
	existing, err := readPrivateRARFile(path)
	if err == nil {
		if !bytes.Equal(existing, payload) {
			return &RARAuthorityConflictError{Slot: path}
		}
		return SyncReviewDirectory(dir)
	}
	if !errors.Is(err, fs.ErrNotExist) {
		return err
	}

	temp, err := createPrivateRARTempFile(dir)
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	if _, err := temp.Write(payload); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Sync(); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if err := validatePrivateRARFile(tempPath); err != nil {
		return err
	}
	if err := publishNoReplace(tempPath, path); err != nil {
		if !errors.Is(err, fs.ErrExist) {
			return err
		}
		existing, readErr := readPrivateRARFile(path)
		if readErr != nil {
			return readErr
		}
		if !bytes.Equal(existing, payload) {
			return &RARAuthorityConflictError{Slot: path}
		}
	}
	if err := validatePrivateRARFile(path); err != nil {
		return err
	}
	return SyncReviewDirectory(dir)
}

func createPrivateRARTempFile(dir string) (*os.File, error) {
	for attempt := 0; attempt < 100; attempt++ {
		token, err := randomAuthorityToken()
		if err != nil {
			return nil, err
		}
		path := filepath.Join(dir, ".rar-"+token)
		file, err := createPrivateRARFile(path)
		if errors.Is(err, fs.ErrExist) {
			continue
		}
		return file, err
	}
	return nil, errors.New("cannot allocate private RAR temporary file")
}

func randomAuthorityToken() (string, error) {
	owner, err := newStoreLockOwner()
	if err != nil {
		return "", err
	}
	return owner.OwnerID, nil
}

func acquireRARAuthorityLock(ctx context.Context, path string) (*storeLock, error) {
	if err := validatePrivateRARDirectory(filepath.Dir(path)); err != nil {
		return nil, err
	}
	if _, err := os.Lstat(path); errors.Is(err, fs.ErrNotExist) {
		file, createErr := createPrivateRARFile(path)
		if createErr != nil && !errors.Is(createErr, fs.ErrExist) {
			return nil, createErr
		}
		if file != nil {
			if syncErr := file.Sync(); syncErr != nil {
				_ = file.Close()
				return nil, syncErr
			}
			if closeErr := file.Close(); closeErr != nil {
				return nil, closeErr
			}
			if syncErr := SyncReviewDirectory(filepath.Dir(path)); syncErr != nil {
				return nil, syncErr
			}
		}
	} else if err != nil {
		return nil, err
	}
	if err := validatePrivateRARFile(path); err != nil {
		return nil, err
	}
	lock, err := acquireBoundedRARLock(ctx, func() (*storeLock, error) {
		return acquirePrivateRARLocalStoreLock(path)
	})
	if err != nil {
		return nil, err
	}
	info, err := lock.file.Stat()
	current, currentErr := os.Lstat(path)
	if err != nil || currentErr != nil ||
		!os.SameFile(info, current) ||
		!privateOpenRARPathSafe(lock.file, info) {
		_ = lock.release()
		if err != nil {
			return nil, err
		}
		if currentErr != nil {
			return nil, currentErr
		}
		return nil, errRARAuthorityPathReplaced
	}
	return lock, nil
}

func acquirePrivateRARLocalStoreLock(path string) (*storeLock, error) {
	file, err := secureOpenLocalStoreLock(path)
	if err != nil {
		return nil, err
	}
	opened, err := file.Stat()
	current, currentErr := os.Lstat(path)
	if err != nil || currentErr != nil ||
		!opened.Mode().IsRegular() ||
		!os.SameFile(opened, current) ||
		!privateOpenRARPathSafe(file, opened) {
		_ = file.Close()
		if err != nil {
			return nil, err
		}
		if currentErr != nil {
			return nil, currentErr
		}
		return nil, errRARAuthorityPathReplaced
	}
	locked, err := tryLockFile(file)
	if err != nil {
		_ = file.Close()
		return nil, err
	}
	if !locked {
		_ = file.Close()
		return nil, storeLockBusyError{}
	}
	owner, err := newStoreLockOwner()
	if err != nil {
		_ = unlockFile(file)
		_ = file.Close()
		return nil, err
	}
	payload, err := json.Marshal(owner)
	if err == nil {
		payload = append(payload, '\n')
		err = file.Truncate(0)
	}
	if err == nil {
		_, err = file.Seek(0, 0)
	}
	if err == nil {
		_, err = file.Write(payload)
	}
	if err == nil {
		err = file.Sync()
	}
	if err != nil {
		_ = unlockFile(file)
		_ = file.Close()
		return nil, fmt.Errorf("write RAR authority lock owner: %w", err)
	}
	return &storeLock{file: file, owner: owner}, nil
}

func acquireBoundedRARLock(
	ctx context.Context,
	acquire func() (*storeLock, error),
) (*storeLock, error) {
	deadline := time.NewTimer(maintenanceLockTimeout)
	defer deadline.Stop()
	for {
		lock, err := acquire()
		if err == nil {
			return lock, nil
		}
		if !errors.Is(err, ErrConcurrentUpdate) {
			return nil, err
		}
		select {
		case <-ctx.Done():
			return nil, &AuthorityLockCancelledError{Cause: ctx.Err()}
		case <-deadline.C:
			return nil, &AuthorityLockTimeoutError{Timeout: maintenanceLockTimeout}
		case <-time.After(10 * time.Millisecond):
		}
	}
}
