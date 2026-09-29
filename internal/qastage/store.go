package qastage

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/gentleman-programming/gentle-ai/v3/internal/filecoord"
)

// Record is one immutable, content-addressed entry in a QA change's history.
type Record struct {
	Revision string
	Parent   string
	Data     []byte
}

// RevisionConflictError is returned by Append when expectedRevision does not
// match a change's actual current head — the optimistic concurrency check
// every QAStateMachine mutation relies on (3A.7).
type RevisionConflictError struct {
	Change   string
	Expected string
	Actual   string
}

func (e *RevisionConflictError) Error() string {
	return fmt.Sprintf("qastage: expected-revision %q does not match current head %q for change %q", e.Expected, e.Actual, e.Change)
}

// QAStateStore is QA's persistence contract: a per-change, content-addressed,
// append-only record log with optimistic concurrency via expected-revision.
// QAStateMachine (3A.4+) layers stage semantics on top by storing its own
// JSON encoding in Record.Data — this interface knows nothing about stages,
// approvals, or outcomes.
type QAStateStore interface {
	// Head returns the current record, or a zero Record (empty Revision) if
	// the change has no history yet.
	Head(ctx context.Context, change string) (Record, error)
	// Append writes a new record whose parent is the change's current head.
	// expectedRevision must equal that head's Revision ("" for a brand-new
	// change) or Append returns a *RevisionConflictError and writes nothing.
	Append(ctx context.Context, change string, expectedRevision string, data []byte) (Record, error)
	// History returns every record for change, oldest first. Nothing is ever
	// deleted or rewritten — retries and resets add records, they never
	// erase history (design decision, section 1.1).
	History(ctx context.Context, change string) ([]Record, error)
}

// PersistentQAStateStore is a self-contained, file-based QAStateStore owned
// entirely by the QA domain. It replaces the originally planned
// OpenRuntimeStoreAdapter: sddstatus.RuntimeStore retired its public
// Begin/Finish surface in Gentle-AI 3.7.0 (internal/cli/sdd_attempt.go:15-16),
// so there is no SDD mechanism left to adapt. See
// docs/migration/qa-orchestrator-v3-design.md, section 7 (3A.3).
//
// It uses internal/filecoord for locking — a generic, domain-neutral
// filesystem primitive, not an SDD-specific one — and otherwise depends on
// nothing outside the standard library.
type PersistentQAStateStore struct {
	root string
}

// NewPersistentQAStateStore returns a store rooted at root (typically
// <repo>/.gentle-ai).
func NewPersistentQAStateStore(root string) *PersistentQAStateStore {
	return &PersistentQAStateStore{root: root}
}

var unsafeChangeChars = regexp.MustCompile(`[^a-zA-Z0-9_.-]`)

func (s *PersistentQAStateStore) changeDir(change string) string {
	safe := unsafeChangeChars.ReplaceAllString(change, "_")
	return filepath.Join(s.root, "qa-ledger", safe)
}

func (s *PersistentQAStateStore) headPath(change string) string {
	return filepath.Join(s.changeDir(change), "HEAD")
}

func (s *PersistentQAStateStore) recordPath(change, revision string) string {
	return filepath.Join(s.changeDir(change), "records", strings.TrimPrefix(revision, "sha256:")+".json")
}

// storedRecord persists Data as base64 so the store stays agnostic to
// whatever encoding QAStateMachine chooses to put in it — the store treats
// Data as opaque bytes, not necessarily JSON.
type storedRecord struct {
	Parent string `json:"parent,omitempty"`
	Data   string `json:"data"`
}

// Head implements QAStateStore.
func (s *PersistentQAStateStore) Head(ctx context.Context, change string) (Record, error) {
	headBytes, err := os.ReadFile(s.headPath(change))
	if errors.Is(err, os.ErrNotExist) {
		return Record{}, nil
	}
	if err != nil {
		return Record{}, fmt.Errorf("qastage: read head for %q: %w", change, err)
	}
	revision := strings.TrimSpace(string(headBytes))
	if revision == "" {
		return Record{}, nil
	}
	return s.loadRecord(change, revision)
}

func (s *PersistentQAStateStore) loadRecord(change, revision string) (Record, error) {
	raw, err := os.ReadFile(s.recordPath(change, revision))
	if err != nil {
		return Record{}, fmt.Errorf("qastage: read record %q for %q: %w", revision, change, err)
	}
	var stored storedRecord
	if err := json.Unmarshal(raw, &stored); err != nil {
		return Record{}, fmt.Errorf("qastage: decode record %q for %q: %w", revision, change, err)
	}
	decoded, err := base64.StdEncoding.DecodeString(stored.Data)
	if err != nil {
		return Record{}, fmt.Errorf("qastage: decode record %q data for %q: %w", revision, change, err)
	}
	return Record{Revision: revision, Parent: stored.Parent, Data: decoded}, nil
}

// Append implements QAStateStore.
func (s *PersistentQAStateStore) Append(ctx context.Context, change, expectedRevision string, data []byte) (Record, error) {
	dir := s.changeDir(change)
	if err := os.MkdirAll(filepath.Join(dir, "records"), 0o755); err != nil {
		return Record{}, fmt.Errorf("qastage: prepare change dir for %q: %w", change, err)
	}

	lockRoot := filepath.Join(s.root, "qa-ledger-locks")
	if err := os.MkdirAll(lockRoot, 0o755); err != nil {
		return Record{}, fmt.Errorf("qastage: prepare lock root: %w", err)
	}

	lease, err := acquireWithRetry(ctx, dir, lockRoot)
	if err != nil {
		return Record{}, fmt.Errorf("qastage: acquire ledger lock for %q: %w", change, err)
	}
	defer lease.Release()

	current, err := s.Head(ctx, change)
	if err != nil {
		return Record{}, err
	}
	if current.Revision != expectedRevision {
		return Record{}, &RevisionConflictError{Change: change, Expected: expectedRevision, Actual: current.Revision}
	}

	stored := storedRecord{Parent: current.Revision, Data: base64.StdEncoding.EncodeToString(data)}
	payload, err := json.Marshal(stored)
	if err != nil {
		return Record{}, fmt.Errorf("qastage: encode record for %q: %w", change, err)
	}
	sum := sha256.Sum256(payload)
	revision := "sha256:" + hex.EncodeToString(sum[:])

	if err := atomicWriteFile(s.recordPath(change, revision), payload); err != nil {
		return Record{}, fmt.Errorf("qastage: write record for %q: %w", change, err)
	}
	if err := atomicWriteFile(s.headPath(change), []byte(revision)); err != nil {
		return Record{}, fmt.Errorf("qastage: advance head for %q: %w", change, err)
	}

	return Record{Revision: revision, Parent: current.Revision, Data: data}, nil
}

// History implements QAStateStore.
func (s *PersistentQAStateStore) History(ctx context.Context, change string) ([]Record, error) {
	head, err := s.Head(ctx, change)
	if err != nil {
		return nil, err
	}
	if head.Revision == "" {
		return nil, nil
	}

	chain := []Record{head}
	for revision := head.Parent; revision != ""; {
		record, err := s.loadRecord(change, revision)
		if err != nil {
			return nil, err
		}
		chain = append(chain, record)
		revision = record.Parent
	}
	for i, j := 0, len(chain)-1; i < j; i, j = i+1, j-1 {
		chain[i], chain[j] = chain[j], chain[i]
	}
	return chain, nil
}

// acquireWithRetry retries filecoord.Acquire (a single non-blocking attempt)
// with a short backoff, since concurrent QAStateMachine mutations on the same
// change are expected and should serialize rather than fail outright.
func acquireWithRetry(ctx context.Context, target, lockRoot string) (*filecoord.Lease, error) {
	deadline := time.Now().Add(5 * time.Second)
	for {
		lease, err := filecoord.Acquire(ctx, target, lockRoot)
		if err == nil {
			return lease, nil
		}
		var busy *filecoord.BusyError
		if !errors.As(err, &busy) {
			return nil, err
		}
		if time.Now().After(deadline) {
			return nil, err
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(5 * time.Millisecond):
		}
	}
}

func atomicWriteFile(path string, data []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
