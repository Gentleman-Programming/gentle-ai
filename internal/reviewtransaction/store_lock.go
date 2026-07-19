package reviewtransaction

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const storeLockSchema = "gentle-ai.review-store-lock/v1"

type storeLockOwner struct {
	Schema     string    `json:"schema"`
	OwnerID    string    `json:"owner_id"`
	PID        int       `json:"pid"`
	Host       string    `json:"host"`
	AcquiredAt time.Time `json:"acquired_at"`
}

type storeLock struct {
	file  *os.File
	owner storeLockOwner
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

func (lock *storeLock) release() error {
	if lock == nil || lock.file == nil {
		return nil
	}
	unlockErr := unlockFile(lock.file)
	closeErr := lock.file.Close()
	lock.file = nil
	return errors.Join(unlockErr, closeErr)
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
