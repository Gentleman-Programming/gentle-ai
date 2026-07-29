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
const compactRecoveryEdgeUnchangedTarget = "unchanged_target"
const compactRecoveryEdgeMalformedAuthorization = "malformed_recovery_authorization"
const compactCombinedRecoveryAnomalies = compactRecoveryEdgeUnchangedTarget + "," + compactRecoveryEdgeMalformedAuthorization

type compactRecoveryEdgeClassification struct {
	Valid                       bool
	Anomalies                   []string
	ValidationError             error
	RecordedAuthorizationSHA256 string
	NonReconcilableError        error
}

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

// classifyCompactRecoveryEdgeAnomalies re-derives one recovery edge and
// admits only the two historical anomaly classes supported by reconciliation.
// It is pure and records a malformed authorization only by SHA-256 digest.
func classifyCompactRecoveryEdgeAnomalies(predecessor, successor CompactRecord) compactRecoveryEdgeClassification {
	edgeErr := validateCompactRecoveryEdge(predecessor, successor.State)
	if edgeErr == nil {
		return compactRecoveryEdgeClassification{Valid: true}
	}
	classification := compactRecoveryEdgeClassification{ValidationError: edgeErr}
	recovery := successor.State.Recovery
	switch {
	case errors.Is(edgeErr, errCompactRecoveryTargetUnchanged):
		classification.Anomalies = []string{compactRecoveryEdgeUnchangedTarget}
		exactBinding := compactRecoveryAuthorizationBinding(
			predecessor.State.LineageID, predecessor.Revision, successor.State.InitialSnapshot.Identity,
			recovery.Actor, recovery.Reason)
		if recovery.MaintainerAuthorization == exactBinding {
			return classification
		}
		if strings.HasPrefix(recovery.MaintainerAuthorization, compactRecoveryAuthorizationSchema) {
			classification.Anomalies = nil
			classification.NonReconcilableError = fmt.Errorf("unchanged target is not the sole anomaly; successor %q records a %s binding bound to different content, which is corruption rather than a pre-contract authorization", successor.State.LineageID, compactRecoveryAuthorizationSchema)
			return classification
		}
		repaired := successor.State
		provenance := *recovery
		provenance.MaintainerAuthorization = exactBinding
		repaired.Recovery = &provenance
		if residualErr := validateCompactRecoveryEdge(predecessor, repaired); !errors.Is(residualErr, errCompactRecoveryTargetUnchanged) {
			classification.Anomalies = nil
			classification.NonReconcilableError = fmt.Errorf("combined recovery anomalies do not re-derive exactly: %v", residualErr)
			return classification
		}
		classification.Anomalies = append(classification.Anomalies, compactRecoveryEdgeMalformedAuthorization)
		recorded := sha256.Sum256([]byte(recovery.MaintainerAuthorization))
		classification.RecordedAuthorizationSHA256 = "sha256:" + hex.EncodeToString(recorded[:])
		return classification
	case errors.Is(edgeErr, errCompactRecoveryAuthorizationInexact):
		if strings.HasPrefix(recovery.MaintainerAuthorization, compactRecoveryAuthorizationSchema) {
			classification.NonReconcilableError = fmt.Errorf("successor %q records a %s binding bound to different content; that is corruption, not a pre-contract authorization", successor.State.LineageID, compactRecoveryAuthorizationSchema)
			return classification
		}
		exactBinding := compactRecoveryAuthorizationBinding(
			predecessor.State.LineageID, predecessor.Revision, successor.State.InitialSnapshot.Identity,
			recovery.Actor, recovery.Reason)
		repaired := successor.State
		provenance := *recovery
		provenance.MaintainerAuthorization = exactBinding
		repaired.Recovery = &provenance
		if residualErr := validateCompactRecoveryEdge(predecessor, repaired); residualErr != nil {
			classification.NonReconcilableError = fmt.Errorf("the pre-contract authorization is not the sole edge anomaly: %v", residualErr)
			return classification
		}
		classification.Anomalies = []string{compactRecoveryEdgeMalformedAuthorization}
		recorded := sha256.Sum256([]byte(recovery.MaintainerAuthorization))
		classification.RecordedAuthorizationSHA256 = "sha256:" + hex.EncodeToString(recorded[:])
		return classification
	default:
		classification.NonReconcilableError = fmt.Errorf("recovery edge fails outside the unchanged-target class and the pre-contract authorization class: %v", edgeErr)
		return classification
	}
}

// compactNonReconcilableContinuation renders what an operator can actually run
// after reconciliation declines an edge. Reconciliation is right to refuse a
// forged binding — admitting it would let a corrupt attestation be rewritten
// into a reconcilable class — but a refusal that names nothing leaves the
// store unrecoverable, so the refusal resolves the one exit that does exist.
//
// `review abandon` is named only when the abandonment gate's own read-only
// prediction accepts the successor, and it is rendered with the exact
// persisted values and the exact authorization template that gate verifies, so
// the command printed here is the command that runs. When the successor is not
// abandonable the refusal says which rule blocks it and names the diagnostic
// instead of inventing a continuation.
func compactNonReconcilableContinuation(ctx context.Context, repo string, successor CompactRecord) string {
	lineage := successor.State.LineageID
	eligibility, err := InspectCompactPristineAbandonment(ctx, repo, lineage)
	if err == nil && eligibility.Eligible {
		return fmt.Sprintf(" The edge cannot be reconciled, but successor %q is pristine, so it can be quarantined whole: %s",
			lineage, compactAbandonCommandText(repo, lineage, eligibility))
	}
	return fmt.Sprintf(" No advertised operation admits this edge: reconciliation refuses it as corruption, and `review abandon` refuses successor %q because %s."+
		" Nothing quarantines this shape today, so no command here will clear it; the entry and its recorded authorization stay exactly as persisted."+
		" Capture the complete machine-readable diagnosis for every affected lineage with `gentle-ai review inspect-authority --cwd %q` and escalate that report — it is the artifact a maintainer needs to decide whether this corruption class becomes admissible.",
		lineage, compactAbandonBlockerText(successor.State), repo)
}

// compactAbandonCommandText renders the exact runnable abandonment invocation
// for one pristine lineage, with the persisted values the abandonment gate
// verifies and the authorization template it accepts. Only a caller whose own
// read-only prediction (InspectCompactPristineAbandonment) accepted the
// lineage may render this, so the command printed is the command that runs.
func compactAbandonCommandText(repo, lineage string, eligibility CompactAbandonEligibility) string {
	return fmt.Sprintf("`gentle-ai review abandon --cwd %q --lineage %q --expected-revision %q --reason \"<why-it-is-abandoned>\" --actor \"<actor>\" --maintainer-authorization \"<maintainer-authorization>\"`;"+
		" the abandonment moves the entry into the audited quarantine and rewrites nothing, so the recorded authorization bytes survive exactly as persisted."+
		" --maintainer-authorization is exactly these six lines, joined by LF, with no trailing newline, using the same --actor and --reason with surrounding whitespace trimmed:\n%s",
		repo, lineage, eligibility.Revision,
		RenderCompactAbandonAuthorization(lineage, eligibility.Revision, eligibility.SnapshotIdentity, "<actor>", "<why-it-is-abandoned>"))
}

// compactAbandonBlockerText names which abandonment rule blocks one lineage,
// so a refusal that cannot name `review abandon` says exactly why instead of
// inventing a continuation.
func compactAbandonBlockerText(state CompactState) string {
	if !compactAbandonablePristineState(state) {
		return fmt.Sprintf("it holds %q authority carrying captured review or correction data, and review results are never discarded to clear an edge", state.State)
	}
	return "the abandonment gate does not accept it: a same-lineage legacy-v1 entry, an authoritative artifact beside its state, a successor of its own, or a removal that would break the remaining graph"
}

// compactReconcileCommandText renders the exact runnable reconciliation for
// one recovery edge already classified into a reconcilable anomaly class,
// with the persisted revisions the operation compare-and-swaps against and
// the authorization template it verifies. Only a caller that classified the
// edge (classifyCompactRecoveryEdgeAnomalies admitted it) may render this, so
// the command printed is the command that runs.
func compactReconcileCommandText(repo, predecessorLineage, predecessorRevision, successorLineage, successorRevision string, anomalies []string) string {
	binding := compactReconcileAuthorizationBinding(
		predecessorLineage, predecessorRevision, successorLineage, successorRevision, "<actor>", "<why-it-is-reconciled>")
	lineCount := "seven"
	if strings.Join(anomalies, ",") == compactCombinedRecoveryAnomalies {
		binding = compactCombinedReconcileAuthorizationBinding(
			predecessorLineage, predecessorRevision, successorLineage, successorRevision, "<actor>", "<why-it-is-reconciled>")
		lineCount = "eight"
	}
	return fmt.Sprintf("`gentle-ai review reconcile-authority --cwd %q --predecessor-lineage %q --expected-predecessor-revision %q --successor-lineage %q --expected-successor-revision %q --reason \"<why-it-is-reconciled>\" --actor \"<actor>\" --maintainer-authorization \"<maintainer-authorization>\"`;"+
		" the reconciliation moves the successor whole — never deleted — into the audited quarantine together with the natively re-derived proof."+
		" --maintainer-authorization is exactly these %s lines, joined by LF, with no trailing newline, using the same --actor and --reason with surrounding whitespace trimmed:\n%s",
		repo, predecessorLineage, predecessorRevision, successorLineage, successorRevision, lineCount, binding)
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
	base, root, err := reviewAuthorityRoot(ctx, repo)
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
		// A record that exists but does not load — a write interrupted between
		// opening the file and finishing it — is never answered with the bare
		// decoder error. Reconciliation proves an edge invalid by re-deriving
		// it from readable state, so an unreadable record is outside both
		// anomaly classes by construction, and admitting bytes that can prove
		// nothing for byte-preserving quarantine is a maintainer policy
		// decision, not a repair this operation may improvise.
		return CompactReclaimRecord{}, fmt.Errorf("review reconcile-authority refused: successor %q holds a compact state record that cannot be loaded (%v);"+
			" inspection classifies it %s. Reconciliation quarantines only a recovery edge it can natively re-derive from readable state, so it cannot admit this record,"+
			" and admitting the damage class is a maintainer policy decision, not a repair."+
			" Capture the complete machine-readable diagnosis with `gentle-ai review inspect-authority --cwd %q` and escalate that report — it is the artifact a maintainer needs to decide whether this class becomes admissible",
			request.SuccessorLineageID, err, compactRecoveryEntryProblem(err), root)
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
	classification := classifyCompactRecoveryEdgeAnomalies(predecessor, successor)
	if classification.Valid {
		return CompactReclaimRecord{}, fmt.Errorf("review reconcile-authority refused: recovery edge for %q validates; the successor remains authoritative", request.SuccessorLineageID)
	}
	if classification.NonReconcilableError != nil {
		return CompactReclaimRecord{}, fmt.Errorf("review reconcile-authority refused: %v.%s",
			classification.NonReconcilableError, compactNonReconcilableContinuation(ctx, root, successor))
	}
	expectedReconcileAuthorization := compactReconcileAuthorizationBinding(
		request.PredecessorLineageID, predecessor.Revision, request.SuccessorLineageID, successor.Revision,
		request.Actor, request.Reason)
	var invalidEdgeProof *CompactInvalidRecoveryEdgeProof
	var malformedAuthorizationProof *CompactMalformedRecoveryAuthorizationProof
	for _, anomaly := range classification.Anomalies {
		switch anomaly {
		case compactRecoveryEdgeUnchangedTarget:
			invalidEdgeProof = &CompactInvalidRecoveryEdgeProof{
				PredecessorLineageID: request.PredecessorLineageID, PredecessorRevision: predecessor.Revision,
				SuccessorRevision: successor.Revision, ValidationError: classification.ValidationError.Error(),
			}
		case compactRecoveryEdgeMalformedAuthorization:
			malformedAuthorizationProof = &CompactMalformedRecoveryAuthorizationProof{
				PredecessorLineageID: request.PredecessorLineageID, PredecessorRevision: predecessor.Revision,
				SuccessorRevision:           successor.Revision,
				RecordedAuthorizationSHA256: classification.RecordedAuthorizationSHA256,
				ValidationError:             compactRecoveryAuthorizationError(successor.State.InitialSnapshot).Error(),
			}
		}
	}
	if strings.Join(classification.Anomalies, ",") == compactCombinedRecoveryAnomalies {
		expectedReconcileAuthorization = compactCombinedReconcileAuthorizationBinding(
			request.PredecessorLineageID, predecessor.Revision, request.SuccessorLineageID, successor.Revision,
			request.Actor, request.Reason)
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
	for _, store := range stores {
		record, loadErr := store.Load()
		if loadErr != nil {
			return CompactReclaimRecord{}, fmt.Errorf("review reconcile-authority refused: related compact authority %q does not load: %w", store.lineageID, loadErr)
		}
		records[record.State.LineageID] = record
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
	if err := compactAuthorityRemovalRegression(records, compactRecordsWithout(records, request.SuccessorLineageID)); err != nil {
		return CompactReclaimRecord{}, fmt.Errorf("review reconcile-authority refused: %w", err)
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
	return quarantineCompactStoreEntry(ctx, base, dir, CompactReclaimRecord{
		Schema: CompactReclaimRecordSchema, Status: CompactReclaimPrepared, LineageID: request.SuccessorLineageID,
		Reason: strings.TrimSpace(request.Reason), Actor: strings.TrimSpace(request.Actor),
		ReclaimedAt: request.ReconciledAt.UTC(), SourcePath: dir, Residue: residue,
		InvalidRecoveryEdge:            invalidEdgeProof,
		MalformedRecoveryAuthorization: malformedAuthorizationProof,
	})
}
