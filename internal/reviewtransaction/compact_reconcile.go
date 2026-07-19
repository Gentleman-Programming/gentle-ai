package reviewtransaction

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const compactReconcileAuthorizationSchema = "gentle-ai.review-reconcile-authorization/v1"
const compactCombinedRecoveryAnomalies = "unchanged_target,malformed_recovery_authorization"

// CompactReconcileRequest identifies one recovery successor whose persisted
// recovery edge must natively re-derive as invalid before quarantine, together
// with the exact maintainer authorization binding for that content.
type CompactReconcileRequest struct {
	PredecessorLineageID        string
	ExpectedPredecessorRevision string
	SuccessorLineageID          string
	ExpectedSuccessorRevision   string
	Reason                      string
	Actor                       string
	MaintainerAuthorization     string
	ReconciledAt                time.Time
}

// CompactInvalidRecoveryEdgeProof records the natively re-derived edge
// invalidity inside the quarantine audit record.
type CompactInvalidRecoveryEdgeProof struct {
	PredecessorLineageID string `json:"predecessor_lineage_id"`
	PredecessorRevision  string `json:"predecessor_revision"`
	SuccessorRevision    string `json:"successor_revision"`
	ValidationError      string `json:"validation_error"`
}

// CompactMalformedRecoveryAuthorizationProof records the natively re-derived
// pre-contract authorization anomaly inside the quarantine audit record. The
// recorded free-form authorization stays byte-preserved in the quarantined
// residue; the proof binds it by digest.
type CompactMalformedRecoveryAuthorizationProof struct {
	PredecessorLineageID        string `json:"predecessor_lineage_id"`
	PredecessorRevision         string `json:"predecessor_revision"`
	SuccessorRevision           string `json:"successor_revision"`
	RecordedAuthorizationSHA256 string `json:"recorded_authorization_sha256"`
	ValidationError             string `json:"validation_error"`
}

func compactReconcileAuthorizationBinding(predecessorLineage, predecessorRevision, successorLineage, successorRevision, actor, reason string) string {
	return compactReconcileAuthorizationSchema + "\npredecessor_lineage=" + predecessorLineage +
		"\npredecessor_revision=" + predecessorRevision + "\nsuccessor_lineage=" + successorLineage +
		"\nsuccessor_revision=" + successorRevision +
		"\nactor=" + strings.TrimSpace(actor) + "\nreason=" + strings.TrimSpace(reason)
}

func compactCombinedReconcileAuthorizationBinding(predecessorLineage, predecessorRevision, successorLineage, successorRevision, actor, reason string) string {
	return compactReconcileAuthorizationBinding(predecessorLineage, predecessorRevision, successorLineage, successorRevision, actor, reason) +
		"\nanomalies=" + compactCombinedRecoveryAnomalies
}

// ReconcileInvalidRecoveryEdge quarantines one compact-v2 recovery successor
// whose recovery edge natively re-derives as invalid for either or both of two
// supported classes: the unchanged-target class, and the pre-contract malformed-
// recovery-authorization class in which a historical free-form maintainer
// authorization predates the exact
// gentle-ai.review-recovery-authorization/v1 binding while the edge is
// otherwise structurally consistent. The predecessor and every unrelated
// authority stay untouched; the successor entry moves whole — never deleted —
// into the audited quarantine together with the re-derived proof. Valid
// edges, incomplete entries, non-recovery records, stale revisions, inexact
// authorization, structurally inconsistent edges, and any additional graph
// defect are refused.
func ReconcileInvalidRecoveryEdge(ctx context.Context, repo string, request CompactReconcileRequest) (CompactReclaimRecord, error) {
	if err := ctx.Err(); err != nil {
		return CompactReclaimRecord{}, err
	}
	if err := validateLineageID(request.PredecessorLineageID); err != nil {
		return CompactReclaimRecord{}, err
	}
	if err := validateLineageID(request.SuccessorLineageID); err != nil {
		return CompactReclaimRecord{}, err
	}
	if request.PredecessorLineageID == request.SuccessorLineageID {
		return CompactReclaimRecord{}, errors.New("review reconcile-authority requires distinct predecessor and successor lineages")
	}
	if strings.TrimSpace(request.Reason) == "" || strings.TrimSpace(request.Actor) == "" {
		return CompactReclaimRecord{}, errors.New("review reconcile-authority requires a non-empty reason and actor")
	}
	base, _, err := reviewAuthorityRoot(ctx, repo)
	if err != nil {
		return CompactReclaimRecord{}, err
	}
	versionRoot := filepath.Join(base, "v2")
	dir := filepath.Join(versionRoot, request.SuccessorLineageID)
	lock, err := acquireStoreLock(filepath.Join(versionRoot, "LOCK"))
	if err != nil {
		return CompactReclaimRecord{}, err
	}
	defer lock.release()
	successorStore := CompactStore{Dir: dir, lineageID: request.SuccessorLineageID}
	successor, err := successorStore.Load()
	if err != nil {
		if os.IsNotExist(err) {
			return CompactReclaimRecord{}, fmt.Errorf("review reconcile-authority refused: successor %q holds no compact authority state. If the entry never held authority, quarantine its residue with review reclaim; if a prior reconcile was interrupted after moving the entry, the prepared reclaim-record.json under %s locates the moved residue for manual reconciliation: %w", request.SuccessorLineageID, filepath.Join(base, "quarantine"), err)
		}
		return CompactReclaimRecord{}, fmt.Errorf("load reconcile successor: %w", err)
	}
	recovery := successor.State.Recovery
	if recovery == nil {
		return CompactReclaimRecord{}, fmt.Errorf("review reconcile-authority refused: successor %q is not a recovery successor", request.SuccessorLineageID)
	}
	if recovery.PredecessorLineageID != request.PredecessorLineageID {
		return CompactReclaimRecord{}, fmt.Errorf("review reconcile-authority refused: successor %q names predecessor %q", request.SuccessorLineageID, recovery.PredecessorLineageID)
	}
	predecessorStore := CompactStore{Dir: filepath.Join(versionRoot, request.PredecessorLineageID), lineageID: request.PredecessorLineageID}
	predecessor, err := predecessorStore.Load()
	if err != nil {
		return CompactReclaimRecord{}, fmt.Errorf("load reconcile predecessor: %w", err)
	}
	if predecessor.Revision != request.ExpectedPredecessorRevision {
		return CompactReclaimRecord{}, fmt.Errorf("%w: expected predecessor revision %q, current %q", ErrConcurrentUpdate, request.ExpectedPredecessorRevision, predecessor.Revision)
	}
	if successor.Revision != request.ExpectedSuccessorRevision {
		return CompactReclaimRecord{}, fmt.Errorf("%w: expected successor revision %q, current %q", ErrConcurrentUpdate, request.ExpectedSuccessorRevision, successor.Revision)
	}
	edgeErr := validateCompactRecoveryEdge(predecessor, successor.State)
	if edgeErr == nil {
		return CompactReclaimRecord{}, fmt.Errorf("review reconcile-authority refused: recovery edge for %q validates; the successor remains authoritative", request.SuccessorLineageID)
	}
	exactRecordedBinding := compactRecoveryAuthorizationBinding(predecessor.State.LineageID, predecessor.Revision, successor.State.InitialSnapshot.Identity, recovery.Actor, recovery.Reason)
	expectedReconcileAuthorization := compactReconcileAuthorizationBinding(
		request.PredecessorLineageID, predecessor.Revision, request.SuccessorLineageID, successor.Revision,
		request.Actor, request.Reason)
	var invalidEdgeProof *CompactInvalidRecoveryEdgeProof
	var malformedAuthorizationProof *CompactMalformedRecoveryAuthorizationProof
	switch {
	case errors.Is(edgeErr, errCompactRecoveryTargetUnchanged):
		invalidEdgeProof = &CompactInvalidRecoveryEdgeProof{
			PredecessorLineageID: request.PredecessorLineageID, PredecessorRevision: predecessor.Revision,
			SuccessorRevision: successor.Revision, ValidationError: edgeErr.Error(),
		}
		if recovery.MaintainerAuthorization != exactRecordedBinding {
			if strings.HasPrefix(recovery.MaintainerAuthorization, compactRecoveryAuthorizationSchema) {
				return CompactReclaimRecord{}, fmt.Errorf("review reconcile-authority refused: unchanged target is not the sole anomaly; successor %q records a %s binding bound to different content, which is corruption rather than a pre-contract authorization", request.SuccessorLineageID, compactRecoveryAuthorizationSchema)
			}
			repaired := successor.State
			provenance := *recovery
			provenance.MaintainerAuthorization = exactRecordedBinding
			repaired.Recovery = &provenance
			if residualErr := validateCompactRecoveryEdge(predecessor, repaired); !errors.Is(residualErr, errCompactRecoveryTargetUnchanged) {
				return CompactReclaimRecord{}, fmt.Errorf("review reconcile-authority refused: combined recovery anomalies do not re-derive exactly: %v", residualErr)
			}
			recorded := sha256.Sum256([]byte(recovery.MaintainerAuthorization))
			malformedAuthorizationProof = &CompactMalformedRecoveryAuthorizationProof{
				PredecessorLineageID: request.PredecessorLineageID, PredecessorRevision: predecessor.Revision,
				SuccessorRevision:           successor.Revision,
				RecordedAuthorizationSHA256: "sha256:" + hex.EncodeToString(recorded[:]),
				ValidationError:             compactRecoveryAuthorizationError(successor.State.InitialSnapshot).Error(),
			}
			expectedReconcileAuthorization = compactCombinedReconcileAuthorizationBinding(
				request.PredecessorLineageID, predecessor.Revision, request.SuccessorLineageID, successor.Revision,
				request.Actor, request.Reason)
		}
	case errors.Is(edgeErr, errCompactRecoveryAuthorizationInexact):
		if strings.HasPrefix(recovery.MaintainerAuthorization, compactRecoveryAuthorizationSchema) {
			return CompactReclaimRecord{}, fmt.Errorf("review reconcile-authority refused: successor %q records a %s binding bound to different content; that is corruption, not a pre-contract authorization", request.SuccessorLineageID, compactRecoveryAuthorizationSchema)
		}
		repaired := successor.State
		provenance := *recovery
		provenance.MaintainerAuthorization = exactRecordedBinding
		repaired.Recovery = &provenance
		if residualErr := validateCompactRecoveryEdge(predecessor, repaired); residualErr != nil {
			return CompactReclaimRecord{}, fmt.Errorf("review reconcile-authority refused: the pre-contract authorization is not the sole edge anomaly: %v", residualErr)
		}
		recorded := sha256.Sum256([]byte(recovery.MaintainerAuthorization))
		malformedAuthorizationProof = &CompactMalformedRecoveryAuthorizationProof{
			PredecessorLineageID: request.PredecessorLineageID, PredecessorRevision: predecessor.Revision,
			SuccessorRevision:           successor.Revision,
			RecordedAuthorizationSHA256: "sha256:" + hex.EncodeToString(recorded[:]),
			ValidationError:             edgeErr.Error(),
		}
	default:
		return CompactReclaimRecord{}, fmt.Errorf("review reconcile-authority refused: recovery edge fails outside the unchanged-target class and the pre-contract authorization class: %v", edgeErr)
	}
	if request.MaintainerAuthorization != expectedReconcileAuthorization {
		return CompactReclaimRecord{}, fmt.Errorf("review reconcile-authority requires an exact maintainer authorization binding (schema %s over predecessor %s@%s and successor %s@%s)",
			compactReconcileAuthorizationSchema, request.PredecessorLineageID, predecessor.Revision, request.SuccessorLineageID, successor.Revision)
	}
	stores, err := DiscoverCompactStores(ctx, repo)
	if err != nil {
		return CompactReclaimRecord{}, err
	}
	records := make(map[string]CompactRecord, len(stores))
	storeByLineage := make(map[string]CompactStore, len(stores))
	for _, store := range stores {
		record, loadErr := store.Load()
		if loadErr != nil {
			return CompactReclaimRecord{}, fmt.Errorf("review reconcile-authority refused: related compact authority %q does not load: %w", store.lineageID, loadErr)
		}
		records[record.State.LineageID], storeByLineage[record.State.LineageID] = record, store
	}
	for lineage, record := range records {
		related := record.State.Recovery
		if related == nil {
			continue
		}
		if related.PredecessorLineageID == request.PredecessorLineageID && lineage != request.SuccessorLineageID {
			return CompactReclaimRecord{}, fmt.Errorf("review reconcile-authority refused: predecessor %q has another successor %q", request.PredecessorLineageID, lineage)
		}
		if related.PredecessorLineageID == request.SuccessorLineageID {
			return CompactReclaimRecord{}, fmt.Errorf("review reconcile-authority refused: successor %q has its own successor %q", request.SuccessorLineageID, lineage)
		}
	}
	delete(records, request.SuccessorLineageID)
	delete(storeByLineage, request.SuccessorLineageID)
	if _, err := compactAuthorityLeaves(records, storeByLineage); err != nil {
		return CompactReclaimRecord{}, fmt.Errorf("review reconcile-authority refused: remaining authority graph is invalid: %w", err)
	}
	items, err := os.ReadDir(dir)
	if err != nil {
		return CompactReclaimRecord{}, fmt.Errorf("inspect reconcile target: %w", err)
	}
	residue := make([]string, 0, len(items))
	for _, item := range items {
		residue = append(residue, item.Name())
	}
	sort.Strings(residue)
	if request.ReconciledAt.IsZero() {
		request.ReconciledAt = time.Now().UTC()
	}
	return quarantineCompactStoreEntry(base, dir, CompactReclaimRecord{
		Schema: CompactReclaimRecordSchema, Status: CompactReclaimPrepared, LineageID: request.SuccessorLineageID,
		Reason: strings.TrimSpace(request.Reason), Actor: strings.TrimSpace(request.Actor),
		ReclaimedAt: request.ReconciledAt.UTC(), SourcePath: dir, Residue: residue,
		InvalidRecoveryEdge:            invalidEdgeProof,
		MalformedRecoveryAuthorization: malformedAuthorizationProof,
	})
}
