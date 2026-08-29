package reviewtransaction

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
)

const (
	AdmittedReviewerResultSchemaV1         = "gentle-ai.review-admitted-result/v1"
	AdmittedReviewerResultSchema           = "gentle-ai.review-admitted-result/v2"
	CompactResultReopenOperation           = "review/reopen-results"
	compactResultReopenAuthorizationSchema = "gentle-ai.review-result-reopen-authorization/v1"
	compactReviewerResultSizeLimit         = 4 << 20
)

func admittedReviewerResultSchemaForSubject(subject ArtifactSubject) string {
	if subject.Schema == ArtifactSubjectSchemaV1 {
		return AdmittedReviewerResultSchemaV1
	}
	return AdmittedReviewerResultSchema
}

// CompactResultReopenRequest names one maintainer-authorized selected lens whose
// current captured value must be invalidated. The refuter from the same phase is
// dependent evidence and is removed by the same CAS when present.
type CompactResultReopenRequest struct {
	LineageID               string
	ExpectedRevision        string
	TargetIdentity          string
	Reason                  string
	Actor                   string
	QuarantineLenses        []string
	MaintainerAuthorization string
	ReopenedAt              time.Time
}

// CompactResultReopenReference is payload-free audit metadata for one removed
// canonical role value. It is never an active capture slot and cannot replay or
// materialize its removed value.
type CompactResultReopenReference struct {
	Role                 CompactRole `json:"role"`
	Lens                 string      `json:"lens,omitempty"`
	SelectedOrder        int         `json:"selected_order,omitempty"`
	TargetIdentity       string      `json:"target_identity"`
	CapturePhaseRevision string      `json:"capture_phase_revision"`
	RequestHash          string      `json:"request_hash,omitempty"`
	ArtifactDigest       string      `json:"artifact_digest"`
	ResultHash           string      `json:"result_hash,omitempty"`
}

type CompactResultReopenPlan struct {
	LineageID                       string                         `json:"lineage_id"`
	Revision                        string                         `json:"revision"`
	TargetIdentity                  string                         `json:"target_identity"`
	QuarantineLenses                []string                       `json:"quarantine_lenses"`
	Removed                         []CompactResultReopenReference `json:"removed"`
	RequiredMaintainerAuthorization string                         `json:"required_maintainer_authorization"`
}

type CompactResultReopenRecord struct {
	LineageID        string                         `json:"lineage_id"`
	PreviousRevision string                         `json:"previous_revision"`
	Revision         string                         `json:"revision"`
	State            State                          `json:"state"`
	Removed          []CompactResultReopenReference `json:"removed"`
	Replayed         bool                           `json:"replayed"`
}

// CompactResultReopenAuthorization binds exactly the canonical quarantine set
// and each payload-free removed reference. It intentionally has no retained-slot
// or path projection because authority derives retained captures from the record.
func CompactResultReopenAuthorization(repository string, request CompactResultReopenRequest, quarantineLenses []string, removed []CompactResultReopenReference) string {
	entries := make([]string, len(removed))
	for index, reference := range removed {
		entries[index] = strings.Join([]string{
			string(reference.Role), reference.Lens, fmt.Sprintf("%d", reference.SelectedOrder),
			reference.TargetIdentity, reference.CapturePhaseRevision, reference.RequestHash,
			reference.ArtifactDigest, reference.ResultHash,
		}, ":")
	}
	return compactResultReopenAuthorizationSchema +
		"\nrepository=" + repository +
		"\nlineage=" + request.LineageID +
		"\nrevision=" + request.ExpectedRevision +
		"\ntarget_identity=" + request.TargetIdentity +
		"\nquarantine_lenses=" + strings.Join(quarantineLenses, ",") +
		"\nremoved=" + strings.Join(entries, ",") +
		"\nactor=" + strings.TrimSpace(request.Actor) +
		"\nreason=" + strings.TrimSpace(request.Reason)
}

func PrepareCompactResultReopen(ctx context.Context, repo string, request CompactResultReopenRequest) (CompactResultReopenPlan, error) {
	if err := validateCompactResultReopenRequest(request, false); err != nil {
		return CompactResultReopenPlan{}, err
	}
	_, repository, err := reviewAuthorityRoot(ctx, repo)
	if err != nil {
		return CompactResultReopenPlan{}, err
	}
	store, err := CompactAuthoritativeStore(ctx, repository, request.LineageID)
	if err != nil {
		return CompactResultReopenPlan{}, err
	}
	record, err := store.LoadContext(ctx)
	if err != nil {
		return CompactResultReopenPlan{}, err
	}
	return buildCompactResultReopenPlan(repository, record, request)
}

func ReopenCompactReviewerResults(ctx context.Context, repo string, request CompactResultReopenRequest) (CompactResultReopenRecord, error) {
	if err := validateCompactResultReopenRequest(request, true); err != nil {
		return CompactResultReopenRecord{}, err
	}
	_, repository, err := reviewAuthorityRoot(ctx, repo)
	if err != nil {
		return CompactResultReopenRecord{}, err
	}
	store, err := CompactAuthoritativeStore(ctx, repository, request.LineageID)
	if err != nil {
		return CompactResultReopenRecord{}, err
	}
	record, err := store.LoadContext(ctx)
	if err != nil {
		return CompactResultReopenRecord{}, err
	}
	if replay, ok := replayCompactResultReopen(record, request); ok {
		return replay, nil
	}
	plan, err := buildCompactResultReopenPlan(repository, record, request)
	if err != nil {
		return CompactResultReopenRecord{}, err
	}
	if request.MaintainerAuthorization != plan.RequiredMaintainerAuthorization {
		return CompactResultReopenRecord{}, fmt.Errorf("review reopen-results requires the exact maintainer authorization binding emitted by --prepare (schema %s)", compactResultReopenAuthorizationSchema)
	}
	if request.ReopenedAt.IsZero() {
		request.ReopenedAt = time.Now().UTC()
	}
	next, removed, err := reopenCompactAdmittedRoleResults(record.State, plan.QuarantineLenses)
	if err != nil {
		return CompactResultReopenRecord{}, err
	}
	removedReferences := compactResultReopenReferences(removed)
	if !equalCompactResultReopenReferences(removedReferences, plan.Removed) {
		return CompactResultReopenRecord{}, errors.New("review reopen-results no longer matches the selected canonical role results") // refusal:by-design world-action: this structural compact-authority invariant requires a provider code fix; no operator command can safely repair it
	}
	next.ResultReopens = append(append([]CompactResultReopen(nil), record.State.ResultReopens...), CompactResultReopen{
		PreviousRevision: record.Revision, TargetIdentity: record.State.InitialSnapshot.Identity,
		QuarantineLenses: append([]string(nil), plan.QuarantineLenses...), Removed: removedReferences,
		Reason: strings.TrimSpace(request.Reason), Actor: strings.TrimSpace(request.Actor),
		ReopenedAt: request.ReopenedAt.UTC(), MaintainerAuthorization: request.MaintainerAuthorization,
	})
	revision, err := store.ReplaceContext(ctx, record.Revision, CompactResultReopenOperation, next)
	if err != nil {
		return CompactResultReopenRecord{}, err
	}
	return CompactResultReopenRecord{
		LineageID: request.LineageID, PreviousRevision: record.Revision, Revision: revision,
		State: StateReviewing, Removed: removedReferences,
	}, nil
}

func validateCompactResultReopenRequest(request CompactResultReopenRequest, apply bool) error {
	if err := validateLineageID(request.LineageID); err != nil {
		return err
	}
	if !validSHA256(request.ExpectedRevision) || !validSHA256(request.TargetIdentity) ||
		strings.TrimSpace(request.Reason) == "" || strings.TrimSpace(request.Actor) == "" {
		return errors.New("review reopen-results requires lineage, exact revision, target identity, reason, and actor")
	}
	if apply && strings.TrimSpace(request.MaintainerAuthorization) == "" {
		return errors.New("review reopen-results requires the exact maintainer authorization emitted by --prepare")
	}
	return nil
}

func canonicalReopenQuarantineLenses(state CompactState, lenses []string) ([]string, error) {
	if len(lenses) == 0 {
		return nil, errors.New("review reopen-results requires at least one --quarantine-lens") // refusal:by-design world-action: this structural compact-authority invariant requires a provider code fix; no operator command can safely repair it
	}
	requested := make(map[string]bool, len(lenses))
	for _, lens := range lenses {
		if strings.TrimSpace(lens) != lens || stringIndex(state.SelectedLenses, lens) < 0 || requested[lens] {
			return nil, fmt.Errorf("review reopen-results --quarantine-lens %q does not name one unique selected lens", lens) // refusal:by-design world-action: this structural compact-authority invariant requires a provider code fix; no operator command can safely repair it
		}
		requested[lens] = true
	}
	canonical := make([]string, 0, len(lenses))
	for _, lens := range state.SelectedLenses {
		if requested[lens] {
			canonical = append(canonical, lens)
		}
	}
	return canonical, nil
}

func compactResultReopenAuditQuarantineLenses(state CompactState, reopen CompactResultReopen) ([]string, error) {
	if len(reopen.QuarantineLenses) == 0 {
		if reopen.SelectedLens == "" {
			return nil, errors.New("reviewer result reopen audit omits its quarantine selection") // refusal:by-design world-action: this structural compact-authority invariant requires a provider code fix; no operator command can safely repair it
		}
		return canonicalReopenQuarantineLenses(state, []string{reopen.SelectedLens})
	}
	if reopen.SelectedLens != "" {
		return nil, errors.New("reviewer result reopen audit mixes singular and set quarantine selections") // refusal:by-design world-action: this structural compact-authority invariant requires a provider code fix; no operator command can safely repair it
	}
	lenses, err := canonicalReopenQuarantineLenses(state, reopen.QuarantineLenses)
	if err != nil || !equalStrings(lenses, reopen.QuarantineLenses) {
		return nil, errors.New("reviewer result reopen audit quarantine selection is not canonical") // refusal:by-design world-action: this structural compact-authority invariant requires a provider code fix; no operator command can safely repair it
	}
	return lenses, nil
}

func compactResultReopenStateEligible(state CompactState) bool {
	if !snapshotsEqual(state.CurrentSnapshot, state.InitialSnapshot) ||
		len(state.CorrectionAttempts) != 0 || state.ActualCorrectionLines != nil ||
		state.OriginalCriteria != nil || state.CorrectionRegression != nil || state.EvidenceHash != "" {
		return false
	}
	switch state.State {
	case StateValidating:
		return len(state.FixFindingIDs) == 0 && state.ProposedCorrectionLines == nil
	case StateCorrectionRequired:
		return true
	default:
		return false
	}
}

func buildCompactResultReopenPlan(repository string, record CompactRecord, request CompactResultReopenRequest) (CompactResultReopenPlan, error) {
	if record.Revision != request.ExpectedRevision {
		return CompactResultReopenPlan{}, fmt.Errorf("%w: expected compact revision %q, current %q", ErrConcurrentUpdate, request.ExpectedRevision, record.Revision)
	}
	state := record.State
	if state.InitialSnapshot.Identity != request.TargetIdentity || !compactResultReopenStateEligible(state) {
		return CompactResultReopenPlan{}, errors.New("review reopen-results requires an uncorrected validating or correction-required authority on the exact frozen target") // refusal:by-design world-action: this structural compact-authority invariant requires a provider code fix; no operator command can safely repair it
	}
	quarantineLenses, err := canonicalReopenQuarantineLenses(state, request.QuarantineLenses)
	if err != nil {
		return CompactResultReopenPlan{}, err
	}
	for _, lens := range quarantineLenses {
		order := stringIndex(state.SelectedLenses, lens)
		active, found, activeErr := state.ActiveAdmittedLensResult(order)
		if activeErr != nil || !found || active.Lens != lens || active.TargetIdentity != state.InitialSnapshot.Identity {
			return CompactResultReopenPlan{}, errors.New("review reopen-results selected lens is not admitted in the active capture batch") // refusal:by-design world-action: an active lens tuple mismatch requires provider-owned authority replacement
		}
	}
	_, removed, err := reopenCompactAdmittedRoleResults(state, quarantineLenses)
	if err != nil {
		return CompactResultReopenPlan{}, err
	}
	references := compactResultReopenReferences(removed)
	plan := CompactResultReopenPlan{
		LineageID: request.LineageID, Revision: record.Revision, TargetIdentity: state.InitialSnapshot.Identity,
		QuarantineLenses: quarantineLenses, Removed: references,
	}
	plan.RequiredMaintainerAuthorization = CompactResultReopenAuthorization(repository, request, quarantineLenses, references)
	return plan, nil
}

func compactResultReopenReferences(removed []CompactAdmittedRoleResult) []CompactResultReopenReference {
	references := make([]CompactResultReopenReference, len(removed))
	for index, entry := range removed {
		references[index] = CompactResultReopenReference{
			Role: entry.Role, Lens: entry.Lens, SelectedOrder: entry.SelectedOrder,
			TargetIdentity: entry.TargetIdentity, CapturePhaseRevision: entry.CapturePhaseRevision,
			RequestHash: entry.RequestHash, ArtifactDigest: entry.ArtifactDigest, ResultHash: entry.ResultHash,
		}
	}
	return references
}

func equalCompactResultReopenReferences(left, right []CompactResultReopenReference) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func replayCompactResultReopen(record CompactRecord, request CompactResultReopenRequest) (CompactResultReopenRecord, bool) {
	requested, err := canonicalReopenQuarantineLenses(record.State, request.QuarantineLenses)
	if err != nil {
		return CompactResultReopenRecord{}, false
	}
	for _, reopen := range record.State.ResultReopens {
		audited, auditErr := compactResultReopenAuditQuarantineLenses(record.State, reopen)
		if auditErr != nil || reopen.PreviousRevision != request.ExpectedRevision || reopen.TargetIdentity != request.TargetIdentity ||
			!equalStrings(audited, requested) || reopen.Reason != strings.TrimSpace(request.Reason) || reopen.Actor != strings.TrimSpace(request.Actor) ||
			reopen.MaintainerAuthorization != request.MaintainerAuthorization {
			continue
		}
		return CompactResultReopenRecord{
			LineageID: record.State.LineageID, PreviousRevision: reopen.PreviousRevision, Revision: record.Revision,
			State: record.State.State, Removed: append([]CompactResultReopenReference(nil), reopen.Removed...), Replayed: true,
		}, true
	}
	return CompactResultReopenRecord{}, false
}

func reAdmitCompactReviewerResult(ctx context.Context, envelope compactAdmittedReviewerResult, expected ArtifactSubject, frozen FrozenCandidateContext) (LensResult, bool) {
	decoder := json.NewDecoder(bytes.NewReader(envelope.Result))
	decoder.DisallowUnknownFields()
	var provider compactProviderReviewerResult
	if err := decoder.Decode(&provider); err != nil {
		return LensResult{}, false
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF || provider.Findings == nil || provider.Evidence == nil {
		return LensResult{}, false
	}
	if !compactProviderLensMatches(provider.Lens, expected.Lens) {
		return LensResult{}, false
	}
	canonicalPayload, err := json.Marshal(provider)
	if err != nil {
		return LensResult{}, false
	}
	canonicalPayload = append(canonicalPayload, '\n')
	result, admission, err := AdmitArtifact(ctx, ArtifactAdmissionRequest{
		ExpectedSubject: expected, FrozenContext: frozen, EchoedSubjectHash: provider.SubjectHash,
		Inspection: provider.Inspection, Result: LensResult{Lens: expected.Lens, Findings: provider.Findings, Evidence: provider.Evidence},
		CandidateCausalFindingIDs: envelope.Admission.CandidateCausalFindingIDs,
		RawPayload:                canonicalPayload, CanonicalPayload: canonicalPayload,
	})
	if err != nil || admission.Decision != ArtifactAdmissionCompleted ||
		admission.CanonicalSHA256 != envelope.Admission.CanonicalSHA256 || admission.ResultHash != envelope.Admission.ResultHash {
		return LensResult{}, false
	}
	return result, true
}

func compactProviderLensMatches(provided, expected string) bool {
	if provided == "" || provided == expected {
		return true
	}
	return map[string]string{
		"risk": LensRisk, "resilience": LensResilience,
		"readability": LensReadability, "reliability": LensReliability,
	}[provided] == expected
}
