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
	ClassifiedAuthorityRepairAuditSchema = "gentle-ai.review-classified-authority-repair-audit/v1"
	ClassifiedAuthorityRepairRoute       = "classified_authority_repair"
)

const (
	CompactReclaimPrepared  = "prepared"
	CompactReclaimCommitted = "committed"
)

var reclaimQuarantineResidue = os.Rename
var persistReclaimRecord = persistCompactReclaimRecord
var compactReclaimPhaseHook = func(ctx context.Context, _ string, _ CompactReclaimRecord) error { return ctx.Err() }

const (
	compactReclaimPhasePrepared  = "prepared"
	compactReclaimPhaseRenamed   = "renamed"
	compactReclaimPhaseCommitted = "committed"
)

type ClassifiedAuthorityRepairAudit struct {
	Schema           string                    `json:"schema"`
	Route            string                    `json:"route"`
	Assessment       AuthorityRepairAssessment `json:"assessment"`
	AssessmentDigest string                    `json:"assessment_digest"`
	RequestDigest    string                    `json:"request_digest"`
}

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
	// proof; it is set only for reconcile-authority quarantines of the
	// unchanged-target class.
	InvalidRecoveryEdge *CompactInvalidRecoveryEdgeProof `json:"invalid_recovery_edge,omitempty"`
	// MalformedRecoveryAuthorization carries the natively re-derived
	// pre-contract authorization proof; it is set only for reconcile-authority
	// quarantines of the malformed-recovery-authorization class.
	MalformedRecoveryAuthorization *CompactMalformedRecoveryAuthorizationProof `json:"malformed_recovery_authorization,omitempty"`
	// PristineAbandonment carries the natively re-derived pristineness proof;
	// it is set only for review-abandon quarantines of pristine reviewing or
	// pristine invalidated lineages.
	PristineAbandonment *CompactPristineAbandonmentProof `json:"pristine_abandonment,omitempty"`
	// IncompleteAbandonment carries the natively re-derived captured/uncaptured
	// lens partition; it is set only for review-abandon quarantines of a
	// reviewing lineage whose selected plan never finished reporting.
	IncompleteAbandonment *CompactIncompleteAbandonmentProof `json:"incomplete_abandonment,omitempty"`
	// MalformedLegacyFreeze carries the natively re-derived semantic replay
	// failure for a shipped legacy-v1 findings-freeze event.
	MalformedLegacyFreeze *LegacyMalformedFreezeProof `json:"malformed_legacy_freeze,omitempty"`
	// LegacyAliasRepair carries the natively re-derived proof for the narrow
	// historical v1 operation-alias quarantine.
	LegacyAliasRepair *LegacyAliasRepairProof `json:"legacy_alias_repair,omitempty"`
	// ClassifiedAuthorityRepair binds a generic review.repair quarantine to the
	// exact provider assessment and authorization-free request digest. Records
	// created by the compatibility command intentionally omit this proof.
	ClassifiedAuthorityRepair *ClassifiedAuthorityRepairAudit `json:"classified_authority_repair,omitempty"`
	// LegacyFixScopeQuarantine carries the proof for the narrowly authorized
	// complete-fix scope-expansion quarantine.
	LegacyFixScopeQuarantine *LegacyFixScopeQuarantineProof `json:"legacy_fix_scope_quarantine,omitempty"`
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
	base, root, err := reviewAuthorityRoot(ctx, repo)
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
			return CompactReclaimRecord{}, compactReclaimAuthorityRefusal(ctx, root, dir, request.LineageID, item.Name())
		}
		residue = append(residue, item.Name())
	}
	sort.Strings(residue)
	if request.ReclaimedAt.IsZero() {
		request.ReclaimedAt = time.Now().UTC()
	}
	return quarantineCompactStoreEntry(ctx, base, dir, CompactReclaimRecord{
		Schema: CompactReclaimRecordSchema, Status: CompactReclaimPrepared, LineageID: request.LineageID,
		Reason: strings.TrimSpace(request.Reason), Actor: strings.TrimSpace(request.Actor),
		ReclaimedAt: request.ReclaimedAt.UTC(), SourcePath: dir, Residue: residue,
	})
}

// compactReclaimAuthorityRefusal explains why reclaim does not apply to a
// store entry holding an authoritative artifact, and names the operation that
// actually admits THIS entry's shape. Reclaim only quarantines entries that
// never held authority, so the historical one-size pointer at
// review reconcile-authority named an operation that refuses (a forged
// binding), or cannot even load its target (a half-written record) — a named
// dead end, which is worse than naming nothing. The honest continuation is
// derived read-only from the entry itself:
//
//   - a recovery successor whose edge classifies into a reconcilable anomaly
//     class is admitted by `review reconcile-authority`, rendered runnably
//     with the exact persisted revisions;
//   - a pristine entry the abandonment gate's own read-only prediction accepts
//     is quarantined whole with `review abandon`;
//   - an unreadable record, and every other shape no advertised operation
//     admits, gets the precise diagnosis and the inspection to capture, never
//     a command that would then refuse.
func compactReclaimAuthorityRefusal(ctx context.Context, repo, dir, lineageID, artifact string) error {
	refused := fmt.Sprintf("review reclaim refused: store entry %q holds authoritative artifact %q, and reclaim only quarantines entries that never held authority.", lineageID, artifact)
	record, loadErr := (CompactStore{Dir: dir, lineageID: lineageID}).Load()
	if loadErr != nil {
		if os.IsNotExist(loadErr) {
			return fmt.Errorf("%s The entry holds no readable review-state.json beside that artifact, so nothing can prove the artifact never carried authority, and no advertised operation admits this shape today."+
				" Capture the complete machine-readable diagnosis with `gentle-ai review inspect-authority --cwd %q` and escalate that report", refused, repo)
		}
		return fmt.Errorf("%s Its record cannot be loaded (%v) — inspection classifies it %s, which an interrupted write leaves behind — and no advertised operation admits an unreadable record:"+
			" reconciliation re-derives its proof from readable state, and admitting bytes that can prove nothing is a maintainer policy decision, not a repair."+
			" Capture the complete machine-readable diagnosis with `gentle-ai review inspect-authority --cwd %q` and escalate that report",
			refused, loadErr, compactRecoveryEntryProblem(loadErr), repo)
	}
	if recovery := record.State.Recovery; recovery != nil {
		predecessor, predecessorErr := (CompactStore{Dir: filepath.Join(filepath.Dir(dir), recovery.PredecessorLineageID), lineageID: recovery.PredecessorLineageID}).Load()
		if predecessorErr == nil {
			classification := classifyCompactRecoveryEdgeAnomalies(predecessor, record)
			if !classification.Valid && len(classification.Anomalies) > 0 {
				return fmt.Errorf("%s Its recovery edge classifies as %s, which `gentle-ai review reconcile-authority` admits: %s",
					refused, strings.Join(classification.Anomalies, ","),
					compactReconcileCommandText(repo, recovery.PredecessorLineageID, predecessor.Revision, record.State.LineageID, record.Revision, classification.Anomalies))
			}
		}
	}
	eligibility, eligibilityErr := InspectCompactPristineAbandonment(ctx, repo, lineageID)
	if eligibilityErr == nil && eligibility.Eligible {
		return fmt.Errorf("%s The entry is pristine, so `gentle-ai review abandon` quarantines it whole: %s",
			refused, compactAbandonCommandText(repo, lineageID, eligibility))
	}
	return fmt.Errorf("%s No advertised operation admits it: reconciliation does not classify it into a supported anomaly class, and `review abandon` refuses because %s."+
		" Nothing quarantines this shape today; the entry stays exactly as persisted."+
		" Capture the complete machine-readable diagnosis with `gentle-ai review inspect-authority --cwd %q` and escalate that report",
		refused, compactAbandonBlockerText(record.State), repo)
}

// quarantineCompactStoreEntry runs the shared two-phase audited move for one
// store entry: create the quarantine directory, persist the prepared record,
// rename the entry into residue/, then rewrite the record as committed. On
// partial failure after the prepared record persisted, it returns the
// populated prepared record alongside a non-nil error.
func quarantineCompactStoreEntry(ctx context.Context, base, dir string, record CompactReclaimRecord) (CompactReclaimRecord, error) {
	if err := ctx.Err(); err != nil {
		return CompactReclaimRecord{}, err
	}
	quarantineRoot := filepath.Join(base, "quarantine")
	if err := ensureCanonicalReviewQuarantineRoot(base, quarantineRoot); err != nil {
		return CompactReclaimRecord{}, err
	}
	if err := os.MkdirAll(quarantineRoot, 0o755); err != nil {
		return CompactReclaimRecord{}, err
	}
	if err := ensureCanonicalReviewQuarantineRoot(base, quarantineRoot); err != nil {
		return CompactReclaimRecord{}, err
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
	if err := compactReclaimPhaseHook(ctx, compactReclaimPhasePrepared, record); err != nil {
		return record, fmt.Errorf("reclaim prepared before residue mutation: %w", err)
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
	if err := compactReclaimPhaseHook(ctx, compactReclaimPhaseRenamed, record); err != nil {
		return record, fmt.Errorf("residue was quarantined before audit commit: %w", err)
	}
	committed := record
	committed.Status = CompactReclaimCommitted
	if err := persistReclaimRecord(committed); err != nil {
		return record, fmt.Errorf("residue was quarantined at %s but the reclaim audit record could not be marked committed: %w", quarantineDir, err)
	}
	if err := compactReclaimPhaseHook(ctx, compactReclaimPhaseCommitted, committed); err != nil {
		return committed, fmt.Errorf("reclaim audit committed before readback: %w", err)
	}
	return committed, nil
}

// ensureCanonicalReviewQuarantineRoot rejects symlinked authority components
// before a quarantine operation can create, inspect, or rename inside them.
func ensureCanonicalReviewQuarantineRoot(base, quarantineRoot string) error {
	commonDir := filepath.Dir(filepath.Dir(base))
	if filepath.Clean(base) != filepath.Join(commonDir, "gentle-ai", "review-transactions") ||
		filepath.Clean(quarantineRoot) != filepath.Join(base, "quarantine") {
		return errors.New("review quarantine refused noncanonical authority root")
	}
	for _, component := range []string{"gentle-ai", "review-transactions", "quarantine"} {
		path := filepath.Join(commonDir, component)
		if component == "review-transactions" {
			path = base
		}
		if component == "quarantine" {
			path = quarantineRoot
		}
		info, err := os.Lstat(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return fmt.Errorf("inspect review quarantine root component %q: %w", component, err)
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return fmt.Errorf("review quarantine refused unsafe authority root component %q", component)
		}
		canonical, err := filepath.EvalSymlinks(path)
		if err != nil || filepath.Clean(canonical) != path {
			return fmt.Errorf("review quarantine refused noncanonical authority root component %q", component)
		}
	}
	return nil
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
