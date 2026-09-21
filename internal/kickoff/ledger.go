package kickoff

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/gentleman-programming/gentle-ai/v3/internal/reviewtransaction"
	"gopkg.in/yaml.v3"
)

// GatesFileName is the append-only gate ledger's file name inside a change root.
const GatesFileName = "gates.yaml"

const gatesLockFileName = ".gates.lock"

// gateLockAcquireAttempts and gateLockRetryDelay bound how long AppendGate
// spins against a busy advisory lock before giving up. AcquireAuthorityFileLock
// is non-blocking (store_lock.go): a contended lock returns immediately, so
// the caller — not the primitive — owns retrying. A single AppendGate holds
// the lock only for a short read-append-write cycle, so a bounded, short-sleep
// retry is enough to serialize concurrent recorders without losing a record.
const (
	gateLockAcquireAttempts = 200
	gateLockRetryDelay      = 2 * time.Millisecond
)

// Test seams over the reviewtransaction primitives, mirroring the pattern
// seal.go (publishKickoff) and sddstatus/runtime_ledger.go already use.
var (
	acquireGateLedgerLock = reviewtransaction.AcquireAuthorityFileLock
	replaceGateLedgerFile = reviewtransaction.ReplaceFileAtomic
	syncGateLedgerDir     = reviewtransaction.SyncReviewDirectory
	sleepBeforeLockRetry  = time.Sleep
)

func gatesPath(changeRoot string) string {
	return filepath.Join(changeRoot, GatesFileName)
}

// LoadGates reads the append-only gate ledger for a change. An absent file
// is not an error: it means no gate decision has ever been recorded, so
// LoadGates returns an empty, but schema-tagged, ledger.
func LoadGates(changeRoot string) (GateLedger, error) {
	path := gatesPath(changeRoot)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return GateLedger{Schema: GateLedgerSchemaV1, Change: filepath.Base(changeRoot)}, nil
		}
		return GateLedger{}, fmt.Errorf("leer gates.yaml en %q: %w", path, err)
	}
	ledger, err := ParseGateLedger(data)
	if err != nil {
		return GateLedger{}, fmt.Errorf("gates.yaml invalido en %q: %w", path, err)
	}
	return ledger, nil
}

// AppendGate appends one decision to the change's gate ledger under an
// exclusive advisory lock, so concurrent recorders never lose a record. It
// never edits or removes an existing record (REQ-21.12): rejection history
// is evidence, and this is the only writer of gates.yaml.
func AppendGate(changeRoot string, rec GateRecord) error {
	if err := rec.Validate(); err != nil {
		return err
	}

	lockPath := filepath.Join(changeRoot, gatesLockFileName)
	lock, err := acquireGateLedgerLockWithRetry(lockPath)
	if err != nil {
		return fmt.Errorf("adquirir cerrojo de gates.yaml en %q: %w", lockPath, err)
	}
	defer func() { _ = lock.Release() }()

	ledger, err := LoadGates(changeRoot)
	if err != nil {
		return err
	}
	if ledger.Schema == "" {
		ledger.Schema = GateLedgerSchemaV1
	}
	if ledger.Change == "" {
		ledger.Change = filepath.Base(changeRoot)
	}
	ledger.Records = append(ledger.Records, rec)

	payload, err := yaml.Marshal(ledger)
	if err != nil {
		return fmt.Errorf("serializar gates.yaml: %w", err)
	}

	temporary, err := os.CreateTemp(changeRoot, ".gates-*.yaml")
	if err != nil {
		return fmt.Errorf("crear publicacion temporal de gates.yaml: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)

	if _, err := temporary.Write(payload); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("escribir publicacion temporal de gates.yaml: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("cerrar publicacion temporal de gates.yaml: %w", err)
	}

	if err := replaceGateLedgerFile(temporaryPath, gatesPath(changeRoot)); err != nil {
		return fmt.Errorf("publicar gates.yaml: %w", err)
	}
	if err := syncGateLedgerDir(changeRoot); err != nil {
		return fmt.Errorf("sincronizar directorio de gates.yaml: %w", err)
	}
	return nil
}

// acquireGateLedgerLockWithRetry retries a busy (but not otherwise broken)
// advisory lock a bounded number of times. Any error other than contention
// is returned immediately: contention is the only condition this function
// treats as "try again shortly".
func acquireGateLedgerLockWithRetry(lockPath string) (*reviewtransaction.AuthorityFileLock, error) {
	var lastErr error
	for attempt := 0; attempt < gateLockAcquireAttempts; attempt++ {
		lock, err := acquireGateLedgerLock(lockPath)
		if err == nil {
			return lock, nil
		}
		if !errors.Is(err, reviewtransaction.ErrStoreLockContended) {
			return nil, err
		}
		lastErr = err
		sleepBeforeLockRetry(gateLockRetryDelay)
	}
	return nil, fmt.Errorf("cerrojo de gates.yaml ocupado tras %d intentos: %w", gateLockAcquireAttempts, lastErr)
}
