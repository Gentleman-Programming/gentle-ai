//go:build unix

package reviewtransaction

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"syscall"
	"testing"
)

func TestBusyStoreLockProbePreservesExistingUnixInodeAndMetadata(t *testing.T) {
	path := filepath.Join(t.TempDir(), "review-store", "LOCK")
	held, err := acquireStoreLock(path)
	if err != nil {
		t.Fatal(err)
	}
	defer held.release()
	beforePayload, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	beforeInfo, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	beforeStat := beforeInfo.Sys().(*syscall.Stat_t)

	if _, err := acquireStoreLock(path); !errors.Is(err, ErrConcurrentUpdate) {
		t.Fatalf("busy acquisition = %v, want ErrConcurrentUpdate", err)
	}
	evidence, exists := inventoryLock(AuthorityVersionCompact, "", path)
	if !exists || evidence.Status != AuthorityLockOwned || evidence.Owner != nil {
		t.Fatalf("busy lock evidence = %#v, exists=%t", evidence, exists)
	}

	afterPayload, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	afterInfo, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	afterStat := afterInfo.Sys().(*syscall.Stat_t)
	if beforeStat.Ino != afterStat.Ino || beforeInfo.ModTime() != afterInfo.ModTime() || !reflect.DeepEqual(beforePayload, afterPayload) {
		t.Fatalf("busy probe mutated LOCK: inode %d/%d mtime %s/%s", beforeStat.Ino, afterStat.Ino, beforeInfo.ModTime(), afterInfo.ModTime())
	}
}
