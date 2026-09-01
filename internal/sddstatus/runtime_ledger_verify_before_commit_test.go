package sddstatus

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// #3816 / #2833: the store used to commit before it verified. publishRecord
// wrote the record, publishHead ADVANCED HEAD, and only then did the chain
// replay -- so a record the store's own validator rejects was already on the
// chain, and every later read walked into it. That is the wedge class: a drift
// bug became a dead end rather than a refusal.
//
// The record must now replay as a candidate BEFORE HEAD moves.

// corruptRecordsOnSync overwrites every published record with valid but wrong
// JSON at the moment the records directory is synced, so the candidate replay
// must reject what was just written. It uses the seam the ledger already
// exposes rather than adding one.
//
// It deliberately does not single out the newest record: the ledger names
// records by content address, so there is no ordering to select on, and this
// test publishes exactly one.
func corruptRecordsOnSync(t *testing.T, store RuntimeStore) {
	corruptRecordsOnSyncWith(t, store, []byte("{}\n"))
}

func corruptRecordsOnSyncWith(t *testing.T, store RuntimeStore, replacement []byte) {
	t.Helper()
	recordsDir := filepath.Join(store.Dir, "records")
	original := runtimeSyncDirectory
	runtimeSyncDirectory = func(path string) error {
		if filepath.Clean(path) == filepath.Clean(recordsDir) {
			entries, err := os.ReadDir(recordsDir)
			if err != nil {
				return err
			}
			for _, entry := range entries {
				if !strings.HasSuffix(entry.Name(), ".json") {
					continue
				}
				full := filepath.Join(recordsDir, entry.Name())
				info, statErr := os.Stat(full)
				if statErr != nil || info.Size() == 0 {
					continue
				}
				if writeErr := os.WriteFile(full, replacement, 0o600); writeErr != nil {
					return writeErr
				}
			}
		}
		return original(path)
	}
	t.Cleanup(func() { runtimeSyncDirectory = original })
}

func readRuntimeHeadRevision(t *testing.T, store RuntimeStore) string {
	t.Helper()
	head, exists, err := readRuntimeHead(filepath.Join(store.Dir, "HEAD"))
	if err != nil {
		t.Fatalf("read HEAD: %v", err)
	}
	if !exists {
		return ""
	}
	return head
}

// TestRejectedRecordNeverReachesHead pins the reordering: when the candidate
// chain does not replay, HEAD stays exactly where it was and the failure is
// not reported as committed.
func TestRejectedRecordNeverReachesHead(t *testing.T) {
	repo := initRuntimeLedgerRepo(t)
	store, err := OpenRuntimeStore(context.Background(), repo, "verify-before-commit")
	if err != nil {
		t.Fatal(err)
	}

	before := readRuntimeHeadRevision(t, store)
	corruptRecordsOnSync(t, store)

	_, err = store.Begin(context.Background(), BeginAttemptRequest{
		ExpectedRevision: before, RequestID: "verify-first-begin", WorkUnit: "work",
		EvidenceGoal: "prove the candidate is verified before HEAD moves",
		MaxAttempts:  2, MaxChangedLines: 90,
	})
	if err == nil {
		t.Fatal("a record that cannot replay was committed")
	}

	// Unconditional: this path must produce no publication outcome at all,
	// because nothing was published. Gating this on errors.As would let the
	// assertion pass silently for exactly the error type this path returns.
	var publication *RuntimePublicationError
	if errors.As(err, &publication) {
		t.Errorf("rejected record produced a publication error (committed=%v): %v", publication.Committed, err)
	}
	if after := readRuntimeHeadRevision(t, store); after != before {
		t.Errorf("HEAD advanced to %q despite a rejected record (was %q)", after, before)
	}
}

// TestUnreadableCandidateRecordNeverReachesHead covers the other new branch:
// a record whose bytes do not parse at all fails on loadRevision itself rather
// than on the revision comparison. The first test corrupts to valid-but-wrong
// JSON and therefore only exercises the mismatch branch.
func TestUnreadableCandidateRecordNeverReachesHead(t *testing.T) {
	repo := initRuntimeLedgerRepo(t)
	store, err := OpenRuntimeStore(context.Background(), repo, "verify-before-commit-unreadable")
	if err != nil {
		t.Fatal(err)
	}

	before := readRuntimeHeadRevision(t, store)
	corruptRecordsOnSyncWith(t, store, []byte("not json at all\n"))

	_, err = store.Begin(context.Background(), BeginAttemptRequest{
		ExpectedRevision: before, RequestID: "verify-first-unreadable", WorkUnit: "work",
		EvidenceGoal: "prove an unreadable candidate never advances HEAD",
		MaxAttempts:  2, MaxChangedLines: 90,
	})
	if err == nil {
		t.Fatal("an unreadable record was committed")
	}
	if after := readRuntimeHeadRevision(t, store); after != before {
		t.Errorf("HEAD advanced to %q despite an unreadable record (was %q)", after, before)
	}
}
