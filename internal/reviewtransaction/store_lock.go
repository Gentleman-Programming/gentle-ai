package reviewtransaction

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const storeLockSchema = "gentle-ai.review-store-lock/v1"

type maintenanceLockMode bool

const (
	maintenanceShared    maintenanceLockMode = false
	maintenanceExclusive maintenanceLockMode = true
)

const maintenanceLockTimeout = 2 * time.Second

type storeLockOwner struct {
	Schema     string    `json:"schema"`
	OwnerID    string    `json:"owner_id"`
	PID        int       `json:"pid"`
	Host       string    `json:"host"`
	AcquiredAt time.Time `json:"acquired_at"`
}

type storeLock struct {
	file        *os.File
	owner       storeLockOwner
	maintenance *MaintenanceLock
}

// MaintenanceLock is an advisory exclusive authority-maintenance lease.
// Callers must supply a bounded context and always Release the lease.
type MaintenanceLock struct{ lock *storeLock }

func (lock *MaintenanceLock) Release() error {
	if lock == nil {
		return nil
	}
	return lock.lock.release()
}

var ErrAuthorityLockTimeout = errors.New("authority lock acquisition timed out")
var ErrAuthorityLockCancelled = errors.New("authority lock acquisition cancelled")

type AuthorityLockTimeoutError struct {
	Timeout time.Duration
}

func (err *AuthorityLockTimeoutError) Error() string {
	return fmt.Sprintf("%v after %s", ErrAuthorityLockTimeout, err.Timeout)
}

func (err *AuthorityLockTimeoutError) Unwrap() error { return ErrAuthorityLockTimeout }

type AuthorityLockCancelledError struct {
	Cause error
}

func (err *AuthorityLockCancelledError) Error() string {
	if err.Cause == nil {
		return ErrAuthorityLockCancelled.Error()
	}
	return fmt.Sprintf("%v: %v", ErrAuthorityLockCancelled, err.Cause)
}

func (err *AuthorityLockCancelledError) Unwrap() error { return ErrAuthorityLockCancelled }

type storeLockBusyError struct{}

func (err storeLockBusyError) Error() string {
	return fmt.Sprintf("%v: authoritative review store advisory lock is held; persisted PID and host metadata are not current-holder proof", ErrConcurrentUpdate)
}

func (err storeLockBusyError) Unwrap() error { return ErrConcurrentUpdate }

func acquireStoreLock(path string) (*storeLock, error) {
	maintenancePath, err := maintenanceLockPathForStoreLock(path)
	if err != nil {
		return nil, err
	}
	if maintenancePath != "" {
		maintenance, err := acquireMaintenanceLock(context.Background(), maintenancePath, maintenanceShared)
		if err != nil {
			return nil, err
		}
		lock, err := acquireLocalStoreLock(path)
		if err != nil {
			_ = maintenance.Release()
			return nil, err
		}
		lock.maintenance = maintenance
		return lock, nil
	}
	return acquireLocalStoreLock(path)
}

func acquireLocalStoreLock(path string) (*storeLock, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, err
	}
	locked, err := tryLockFile(file)
	if err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("acquire bounded review store lock: %w", err)
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
	if err != nil {
		_ = unlockFile(file)
		_ = file.Close()
		return nil, err
	}
	payload = append(payload, '\n')
	if err := file.Truncate(0); err == nil {
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
		return nil, fmt.Errorf("write review store lock owner: %w", err)
	}
	return &storeLock{file: file, owner: owner}, nil
}

func acquireMaintenanceLock(ctx context.Context, path string, mode maintenanceLockMode) (*MaintenanceLock, error) {
	if err := ensureMaintenanceLockPath(path); err != nil {
		return nil, err
	}
	deadline := time.NewTimer(maintenanceLockTimeout)
	defer deadline.Stop()
	for {
		if err := ctx.Err(); err != nil {
			return nil, &AuthorityLockCancelledError{Cause: err}
		}
		info, err := os.Lstat(path)
		if err != nil && !os.IsNotExist(err) {
			return nil, err
		}
		if err == nil && (info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular()) {
			return nil, errors.New("maintenance lock is not a regular file")
		}
		file, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
		if err != nil {
			return nil, err
		}
		locked, err := tryLockFileMode(file, bool(mode))
		if err != nil {
			_ = file.Close()
			return nil, err
		}
		if locked {
			return &MaintenanceLock{lock: &storeLock{file: file}}, nil
		}
		_ = file.Close()
		select {
		case <-ctx.Done():
			return nil, &AuthorityLockCancelledError{Cause: ctx.Err()}
		case <-deadline.C:
			return nil, &AuthorityLockTimeoutError{Timeout: maintenanceLockTimeout}
		case <-time.After(10 * time.Millisecond):
		}
	}
}

// AcquireReviewMaintenanceExclusive gives an approved maintenance tool exclusive
// advisory access. It refuses unbounded contexts so a failed tool cannot wait forever.
func AcquireReviewMaintenanceExclusive(ctx context.Context, repo string) (*MaintenanceLock, error) {
	if _, ok := ctx.Deadline(); !ok {
		return nil, errors.New("maintenance lock requires a context deadline")
	}
	root, _, err := reviewAuthorityRoot(ctx, repo)
	if err != nil {
		return nil, err
	}
	return acquireMaintenanceLock(ctx, filepath.Join(filepath.Dir(root), "REVIEW-MAINTENANCE.lock"), maintenanceExclusive)
}

func ensureMaintenanceLockPath(path string) error {
	root := filepath.Dir(path)
	for current := root; ; current = filepath.Dir(current) {
		info, err := os.Lstat(current)
		if err == nil {
			if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
				return fmt.Errorf("maintenance authority component %q is not a directory", current)
			}
			break
		}
		if !os.IsNotExist(err) {
			return err
		}
		parent := filepath.Dir(current)
		if parent == current {
			return err
		}
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return err
	}
	for current := root; ; current = filepath.Dir(current) {
		info, err := os.Lstat(current)
		if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return fmt.Errorf("maintenance authority component %q is unsafe", current)
		}
		if current == filepath.VolumeName(current)+string(filepath.Separator) {
			return nil
		}
	}
}

func (lock *storeLock) release() error {
	if lock == nil || lock.file == nil {
		return nil
	}
	unlockErr := unlockFile(lock.file)
	closeErr := lock.file.Close()
	lock.file = nil
	var maintenanceErr error
	if lock.maintenance != nil {
		maintenanceErr = lock.maintenance.Release()
		lock.maintenance = nil
	}
	return errors.Join(unlockErr, closeErr, maintenanceErr)
}

func maintenanceLockPathForStoreLock(path string) (string, error) {
	cleanPath := filepath.Clean(path)
	var authorityRoot string
	for current := filepath.Dir(cleanPath); ; current = filepath.Dir(current) {
		if filepath.Base(current) == "review-transactions" && filepath.Base(filepath.Dir(current)) == "gentle-ai" {
			if authorityRoot != "" {
				return "", errors.New("review store lock path has ambiguous authority roots")
			}
			authorityRoot = current
		}
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
	}
	if authorityRoot == "" {
		return "", nil
	}
	relative, err := filepath.Rel(authorityRoot, cleanPath)
	if err != nil || relative == "." || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", errors.New("review store lock path escapes canonical authority")
	}
	return filepath.Join(filepath.Dir(authorityRoot), "REVIEW-MAINTENANCE.lock"), nil
}

func newStoreLockOwner() (storeLockOwner, error) {
	var token [16]byte
	if _, err := rand.Read(token[:]); err != nil {
		return storeLockOwner{}, fmt.Errorf("generate review store lock owner: %w", err)
	}
	host, err := os.Hostname()
	if err != nil {
		host = "unknown"
	}
	return storeLockOwner{
		Schema: storeLockSchema, OwnerID: hex.EncodeToString(token[:]), PID: os.Getpid(),
		Host: host, AcquiredAt: time.Now().UTC(),
	}, nil
}
