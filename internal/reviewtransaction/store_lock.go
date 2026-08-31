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

// MaintenanceLock is an advisory authority-maintenance lease. It may be shared
// by review writers or exclusive for approved maintenance. Acquisition is
// bounded by maintenanceLockTimeout, including context.Background callers.
// Callers must always Release the lease.
type MaintenanceLock struct{ lock *storeLock }

// AuthorityFileLock exposes the hardened, platform-specific no-follow lock
// primitive to adjacent provider-owned authority stores. It preserves kernel
// advisory ownership as truth and never treats PID metadata as authorization.
type AuthorityFileLock struct{ lock *storeLock }

// AcquireAuthorityFileLock opens and exclusively locks one private authority
// lock path using the same no-follow implementation as the review stores.
func AcquireAuthorityFileLock(path string) (*AuthorityFileLock, error) {
	lock, err := acquireLocalStoreLock(path)
	if err != nil {
		return nil, err
	}
	return &AuthorityFileLock{lock: lock}, nil
}

// Release relinquishes an AuthorityFileLock.
func (lock *AuthorityFileLock) Release() error {
	if lock == nil || lock.lock == nil {
		return nil
	}
	err := lock.lock.release()
	lock.lock = nil
	return err
}

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

func (err *AuthorityLockCancelledError) Unwrap() []error {
	if err.Cause == nil {
		return []error{ErrAuthorityLockCancelled}
	}
	return []error{ErrAuthorityLockCancelled, err.Cause}
}

// ErrStoreLockContended reports the one refusal that proves, from where it is
// produced rather than from its text, that nothing was mutated: the advisory
// authority lock was already held, so the non-blocking acquisition syscall
// refused and the guarded body never ran (1861).
//
// It is deliberately narrower than ErrConcurrentUpdate, which every
// compare-and-swap precondition in this package also reports. Those describe
// authority that moved under a caller and may need a re-derivation; this one
// describes a caller that never got in. Only this sentinel is safe to treat as
// a proven non-mutation, and only when nothing in the same operation committed
// a native transition first — that remains genuinely unknown.
var ErrStoreLockContended = errors.New("authoritative review store advisory lock is held")

type storeLockBusyError struct{}

func (err storeLockBusyError) Error() string {
	return fmt.Sprintf("%v: authoritative review store advisory lock is held; persisted PID and host metadata are not current-holder proof", ErrConcurrentUpdate)
}

// Unwrap keeps ErrConcurrentUpdate in the chain so every existing bounded
// retry helper still recognizes contention, and adds ErrStoreLockContended so
// a caller can tell "never acquired the lock" apart from "lost a CAS".
func (err storeLockBusyError) Unwrap() []error {
	return []error{ErrStoreLockContended, ErrConcurrentUpdate}
}

// StoreLockPreAcquisitionError reports a failure that happened before the
// authoritative store lock was acquired: directory creation, the secure
// open walk, or the advisory lock syscall itself all fail before any review
// authority mutation, so callers must classify this as not-started rather
// than an unknown-mutation outcome (1781).
type StoreLockPreAcquisitionError struct {
	Err error
}

func (err *StoreLockPreAcquisitionError) Error() string {
	if err == nil || err.Err == nil {
		// refusal:by-design world-action: this sentence renders only for a degenerately constructed error with no cause, which no operator command can reach; the exit is a code fix at the construction site.
		return "review store lock could not be acquired"
	}
	return fmt.Sprintf("review store lock could not be acquired: %v", err.Err)
}

func (err *StoreLockPreAcquisitionError) Unwrap() error {
	if err == nil {
		return nil
	}
	return err.Err
}

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
		return nil, &StoreLockPreAcquisitionError{Err: err}
	}
	file, err := secureOpenLocalStoreLock(path)
	if err != nil {
		return nil, &StoreLockPreAcquisitionError{Err: err}
	}
	locked, err := tryLockFile(file)
	if err != nil {
		_ = file.Close()
		return nil, &StoreLockPreAcquisitionError{Err: fmt.Errorf("acquire bounded review store lock: %w", err)}
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
	return acquireMaintenanceLockInternal(ctx, path, mode, false)
}

func acquireMaintenanceLockInternal(ctx context.Context, path string, mode maintenanceLockMode, allowPreparedBatch bool) (*MaintenanceLock, error) {
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
			lock := &MaintenanceLock{lock: &storeLock{file: file}}
			if !allowPreparedBatch && filepath.Base(path) == "REVIEW-MAINTENANCE.lock" {
				base := filepath.Join(filepath.Dir(path), "review-transactions")
				if err := ensureNoPreparedCompactBatchReconciliation(base); err != nil {
					_ = lock.Release()
					return nil, err
				}
			}
			return lock, nil
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
	if !filepath.IsAbs(path) {
		return fmt.Errorf("maintenance lock path %q must be absolute", path)
	}
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
	// #2504: clear the owner payload while the advisory lock is still held,
	// so no observer ever reads an exited process as the recorded holder.
	// Kernel ownership stays the truth; the payload is only ever a hint.
	truncateErr := lock.file.Truncate(0)
	unlockErr := unlockFile(lock.file)
	closeErr := lock.file.Close()
	lock.file = nil
	var maintenanceErr error
	if lock.maintenance != nil {
		maintenanceErr = lock.maintenance.Release()
		lock.maintenance = nil
	}
	return errors.Join(truncateErr, unlockErr, closeErr, maintenanceErr)
}

// secureLockRoot locates the repository's Git common directory that this
// store lock path is derived from — the parent of the fixed "gentle-ai"
// authority marker — so the secure open walk can anchor there instead of at
// the filesystem root (1781). It mirrors the authority-root walk in
// maintenanceLockPathForStoreLock, but absence or ambiguity is not an error
// here: the caller falls back to the root-anchored walk, which is always
// safe and matches today's behavior verbatim.
func secureLockRoot(path string) (string, bool) {
	cleanPath := filepath.Clean(path)
	var authorityRoot string
	for current := filepath.Dir(cleanPath); ; current = filepath.Dir(current) {
		if filepath.Base(current) == "review-transactions" && filepath.Base(filepath.Dir(current)) == "gentle-ai" {
			if authorityRoot != "" {
				return "", false
			}
			authorityRoot = current
		}
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
	}
	if authorityRoot == "" {
		return "", false
	}
	return filepath.Dir(filepath.Dir(authorityRoot)), true
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
