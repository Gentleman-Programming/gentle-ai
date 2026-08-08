package reviewtransaction

// This file used to also hold ReconcileInvalidRecoveryEdge -- the provider
// behind the `review reconcile-authority` CLI verb -- and its own
// authorization-binding renderers. Wave 7 S3a retired the verb (no
// production caller remained); S3b retires the provider itself, since
// nothing else called it once the verb was gone (confirmed by the deadcode
// ratchet's own +4 uptick in S3a's commit, which named exactly these four
// symbols). What remains here is LIVE, RETAINED code that happened to share
// this file: the anomaly classifier (three other production callers:
// authority_disposition_plan.go, compact_inspect.go,
// compact_batch_reconcile_plan.go) and the abandon-command-text renderers
// (compact_reclaim.go, compact_inspect.go).

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v2/internal/pathquote"
)

const compactRecoveryEdgeUnchangedTarget = "unchanged_target"
const compactRecoveryEdgeMalformedAuthorization = "malformed_recovery_authorization"
const compactCombinedRecoveryAnomalies = compactRecoveryEdgeUnchangedTarget + "," + compactRecoveryEdgeMalformedAuthorization

type compactRecoveryEdgeClassification struct {
	Valid                       bool
	Anomalies                   []string
	ValidationError             error
	RecordedAuthorizationSHA256 string
	NonReconcilableError        error
	// DispositionClass names the one closed anomaly class Wave 2's
	// AuthorityDispositionPlan derivation targets
	// (authority_disposition_plan.go): a schema-prefixed maintainer
	// authorization bound to different content than the successor's own
	// recorded fields. It is deliberately NOT surfaced on
	// CompactRecoveryEdgeInspection.AnomalyClasses -- that would advertise a
	// `review reconcile-authority` continuation this edge would then refuse
	// (design decision 2) -- so CompactRecoveryEdgeInspection's JSON stays
	// byte-identical. A caller that needs it re-derives classification
	// directly from records, exactly as deriveAuthorityDispositionPlan does.
	DispositionClass string
}

// CompactInvalidRecoveryEdgeProof records a natively re-derived edge
// invalidity, historically inside a reconciliation quarantine audit record.
// The type itself is retained (compact_reclaim.go's CompactReclaimRecord
// still carries an optional field of this shape for historical/forensic
// records already on disk) even though nothing constructs a new one anymore.
type CompactInvalidRecoveryEdgeProof struct {
	PredecessorLineageID string `json:"predecessor_lineage_id"`
	PredecessorRevision  string `json:"predecessor_revision"`
	SuccessorRevision    string `json:"successor_revision"`
	ValidationError      string `json:"validation_error"`
}

// CompactMalformedRecoveryAuthorizationProof records a natively re-derived
// pre-contract authorization anomaly, historically inside a reconciliation
// quarantine audit record. Retained for the same reason as
// CompactInvalidRecoveryEdgeProof above.
type CompactMalformedRecoveryAuthorizationProof struct {
	PredecessorLineageID        string `json:"predecessor_lineage_id"`
	PredecessorRevision         string `json:"predecessor_revision"`
	SuccessorRevision           string `json:"successor_revision"`
	RecordedAuthorizationSHA256 string `json:"recorded_authorization_sha256"`
	ValidationError             string `json:"validation_error"`
}

// classifyCompactRecoveryEdgeAnomalies re-derives one recovery edge and
// admits only the two historical anomaly classes reconciliation used to
// support. It is pure and records a malformed authorization only by
// SHA-256 digest.
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
			classification.DispositionClass = compactContentMismatchedRecoveryAuthorizationClass
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
			classification.DispositionClass = compactContentMismatchedRecoveryAuthorizationClass
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

// compactAbandonCommandText renders the exact runnable abandonment invocation
// for one eligible lineage, with the persisted values the abandonment gate
// verifies and the authorization template it accepts. Only a caller whose own
// read-only prediction (InspectCompactPristineAbandonment) accepted the
// lineage may render this, so the command printed is the command that runs.
func compactAbandonCommandText(repo, lineage string, eligibility CompactAbandonEligibility) string {
	return fmt.Sprintf("`gentle-ai review abandon --cwd %s --lineage %q --expected-revision %q --reason %q --actor \"<actor>\" --maintainer-authorization \"<maintainer-authorization>\"`;"+
		" the abandonment moves the entry into the audited quarantine and rewrites nothing, so the recorded authorization bytes survive exactly as persisted."+
		" --maintainer-authorization is exactly these nine lines, joined by LF, with no trailing newline, using the same --actor:\n%s",
		pathquote.Quote(repo), lineage, eligibility.Revision, CompactAbandonReasonOperatorDisposition,
		RenderCompactAbandonAuthorization(lineage, eligibility.Revision, eligibility.SnapshotIdentity, "<actor>", CompactAbandonReasonOperatorDisposition, eligibility.DiscardedWork))
}

// compactAbandonBlockerText names which abandonment rule blocks one lineage,
// so a refusal that cannot name `review abandon` says exactly why instead of
// inventing a continuation.
func compactAbandonBlockerText(state CompactState) string {
	if compactAbandonTerminalState(state.State) {
		return fmt.Sprintf("it holds terminal %q authority", state.State)
	}
	return "the abandonment gate does not accept it: its discarded-work summary cannot be read, it has a same-lineage legacy-v1 entry, a successor of its own, or removal would break the remaining graph"
}
