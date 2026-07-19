package reviewtransaction

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const CompactReclaimRecordSchema = "gentle-ai.review-reclaim-record/v1"

const (
	CompactReclaimPrepared  = "prepared"
	CompactReclaimCommitted = "committed"
)

var reclaimQuarantineResidue = os.Rename
var persistReclaimRecord = persistCompactReclaimRecord

// CompactReclaimRequest identifies one explicit incomplete compact-v2 store
// entry to quarantine, together with the maintainer audit inputs.
type CompactReclaimRequest struct {
	LineageID   string
	Reason      string
	Actor       string
	ReclaimedAt time.Time
}

// CompactReclaimRecord is the persisted audit record for one quarantined
// incomplete store entry; it mirrors the recovery provenance shape. Status is
// "committed" only after the residue rename landed and the record rewrite
// succeeded. A record stuck at "prepared" means the rename may or may not
// have run; the discriminator is the residue/ subdirectory next to the
// record: absent means the residue was never moved, present means it was
// moved but the committed rewrite did not land.
type CompactReclaimRecord struct {
	Schema         string    `json:"schema"`
	Status         string    `json:"status"`
	LineageID      string    `json:"lineage_id"`
	Reason         string    `json:"reason"`
	Actor          string    `json:"actor"`
	ReclaimedAt    time.Time `json:"reclaimed_at"`
	SourcePath     string    `json:"source_path"`
	QuarantinePath string    `json:"quarantine_path"`
	Residue        []string  `json:"residue"`
	// InvalidRecoveryEdge carries the natively re-derived edge-invalidity
	// proof; it is set only for reconcile-authority quarantines.
	InvalidRecoveryEdge *CompactInvalidRecoveryEdgeProof `json:"invalid_recovery_edge,omitempty"`
}

// compactAuthoritativeArtifact reports whether a store-entry name carries
// review authority: state, receipt, finalize journal, captured reviewer
// results, or interrupted atomic-write payloads that may hold any of them.
// It must cover every file the compact store writes under a lineage
// directory; the shared names live beside CompactStore in compact_store.go.
func compactAuthoritativeArtifact(name string) bool {
	switch name {
	case compactStateFileName, compactReceiptFileName, compactFinalizeJournalFileName, CompactReviewerResultsDir:
		return true
	}
	return strings.HasPrefix(name, ".atomic-")
}

func compactStoreHoldsAuthority(items []os.DirEntry) bool {
	for _, item := range items {
		if compactAuthoritativeArtifact(item.Name()) {
			return true
		}
	}
	return false
}

// ReclaimIncompleteCompactStore moves one incomplete compact-v2 store entry —
// no review-state.json and no other authoritative artifact — into an audited
// quarantine directory under the review authority root. It never deletes
// residue and never fabricates authority state. On partial failure after the
// prepared record is persisted, it returns the populated prepared record
// (QuarantinePath and Residue set) alongside a non-nil error; callers must
// not discard the record on error — it locates the quarantine or orphan for
// reconciliation.
func ReclaimIncompleteCompactStore(ctx context.Context, repo string, request CompactReclaimRequest) (CompactReclaimRecord, error) {
	if err := ctx.Err(); err != nil {
		return CompactReclaimRecord{}, err
	}
	if err := validateLineageID(request.LineageID); err != nil {
		return CompactReclaimRecord{}, err
	}
	if strings.TrimSpace(request.Reason) == "" || strings.TrimSpace(request.Actor) == "" {
		return CompactReclaimRecord{}, errors.New("review reclaim requires a non-empty reason and actor")
	}
	base, _, err := reviewAuthorityRoot(ctx, repo)
	if err != nil {
		return CompactReclaimRecord{}, err
	}
	versionRoot := filepath.Join(base, "v2")
	dir := filepath.Join(versionRoot, request.LineageID)
	lock, err := acquireStoreLock(filepath.Join(versionRoot, "LOCK"))
	if err != nil {
		return CompactReclaimRecord{}, err
	}
	defer lock.release()
	items, err := os.ReadDir(dir)
	if err != nil {
		return CompactReclaimRecord{}, fmt.Errorf("inspect reclaim target: %w", err)
	}
	residue := make([]string, 0, len(items))
	for _, item := range items {
		if compactAuthoritativeArtifact(item.Name()) {
			return CompactReclaimRecord{}, fmt.Errorf("review reclaim refused: store entry %q holds authoritative artifact %q; an invalid recovery successor is quarantined with review reconcile-authority", request.LineageID, item.Name())
		}
		residue = append(residue, item.Name())
	}
	sort.Strings(residue)
	if request.ReclaimedAt.IsZero() {
		request.ReclaimedAt = time.Now().UTC()
	}
	return quarantineCompactStoreEntry(base, dir, CompactReclaimRecord{
		Schema: CompactReclaimRecordSchema, Status: CompactReclaimPrepared, LineageID: request.LineageID,
		Reason: strings.TrimSpace(request.Reason), Actor: strings.TrimSpace(request.Actor),
		ReclaimedAt: request.ReclaimedAt.UTC(), SourcePath: dir, Residue: residue,
	})
}

// quarantineCompactStoreEntry runs the shared two-phase audited move for one
// store entry: create the quarantine directory, persist the prepared record,
// rename the entry into residue/, then rewrite the record as committed. On
// partial failure after the prepared record persisted, it returns the
// populated prepared record alongside a non-nil error.
func quarantineCompactStoreEntry(base, dir string, record CompactReclaimRecord) (CompactReclaimRecord, error) {
	quarantineRoot := filepath.Join(base, "quarantine")
	if err := os.MkdirAll(quarantineRoot, 0o755); err != nil {
		return CompactReclaimRecord{}, err
	}
	quarantineRootInfo, err := os.Lstat(quarantineRoot)
	if err != nil {
		return CompactReclaimRecord{}, err
	}
	if quarantineRootInfo.Mode()&os.ModeSymlink != 0 || !quarantineRootInfo.IsDir() {
		return CompactReclaimRecord{}, fmt.Errorf("review quarantine refused unsafe quarantine root %q", quarantineRoot)
	}
	if err := SyncReviewDirectory(base); err != nil {
		return CompactReclaimRecord{}, fmt.Errorf("sync review authority root after quarantine creation: %w", err)
	}
	quarantineDir, err := os.MkdirTemp(quarantineRoot, record.LineageID+"-")
	if err != nil {
		return CompactReclaimRecord{}, err
	}
	cleanupUnprepared := func(cause error) error {
		if removeErr := os.RemoveAll(quarantineDir); removeErr != nil {
			return errors.Join(cause, fmt.Errorf("remove unidentified quarantine directory %q: %w", quarantineDir, removeErr))
		}
		if syncErr := SyncReviewDirectory(quarantineRoot); syncErr != nil {
			return errors.Join(cause, fmt.Errorf("sync quarantine cleanup for %q: %w", quarantineDir, syncErr))
		}
		return cause
	}
	if err := SyncReviewDirectory(quarantineRoot); err != nil {
		return CompactReclaimRecord{}, cleanupUnprepared(fmt.Errorf("sync quarantine root after reclaim directory creation: %w", err))
	}
	record.QuarantinePath = quarantineDir
	if err := persistReclaimRecord(record); err != nil {
		return CompactReclaimRecord{}, cleanupUnprepared(err)
	}
	if err := reclaimQuarantineResidue(dir, filepath.Join(quarantineDir, "residue")); err != nil {
		return record, fmt.Errorf("reclaim was prepared at %s but quarantining the residue failed: %w", quarantineDir, err)
	}
	if err := SyncReviewDirectory(filepath.Dir(dir)); err != nil {
		return record, fmt.Errorf("residue was quarantined at %s but syncing the source version root failed: %w", quarantineDir, err)
	}
	if err := SyncReviewDirectory(quarantineDir); err != nil {
		return record, fmt.Errorf("residue was quarantined at %s but syncing the quarantine directory failed: %w", quarantineDir, err)
	}
	committed := record
	committed.Status = CompactReclaimCommitted
	if err := persistReclaimRecord(committed); err != nil {
		return record, fmt.Errorf("residue was quarantined at %s but the reclaim audit record could not be marked committed: %w", quarantineDir, err)
	}
	return committed, nil
}

func persistCompactReclaimRecord(record CompactReclaimRecord) error {
	payload, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return err
	}
	if err := writeAtomic(filepath.Join(record.QuarantinePath, "reclaim-record.json"), append(payload, '\n'), 0o644); err != nil {
		return fmt.Errorf("persist %s reclaim audit record: %w", record.Status, err)
	}
	return nil
}
