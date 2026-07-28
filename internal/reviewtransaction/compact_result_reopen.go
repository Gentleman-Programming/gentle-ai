package reviewtransaction

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"time"
)

const (
	AdmittedReviewerResultSchema           = "gentle-ai.review-admitted-result/v1"
	CompactResultReopenOperation           = "review/reopen-results"
	CompactQuarantinedReviewerResultsDir   = "quarantined-reviewer-results"
	compactResultReopenAuthorizationSchema = "gentle-ai.review-result-reopen-authorization/v1"
	compactReviewerResultSizeLimit         = 4 << 20
)

type CompactResultReopenRequest struct {
	LineageID        string
	ExpectedRevision string
	TargetIdentity   string
	Reason           string
	Actor            string
	// QuarantineLenses names the admitted reviewer results the maintainer
	// explicitly overrides. Native detection never quarantines an admitted
	// slot; only a lens named here — and named again inside the emitted
	// maintainer authorization — may move an admitted result into quarantine.
	QuarantineLenses        []string
	MaintainerAuthorization string
	ReopenedAt              time.Time
}

type CompactResultReopenPlan struct {
	LineageID                       string                    `json:"lineage_id"`
	Revision                        string                    `json:"revision"`
	TargetIdentity                  string                    `json:"target_identity"`
	Quarantined                     []CompactResultReopenSlot `json:"quarantined"`
	Retained                        []CompactResultReopenSlot `json:"retained"`
	AuthorizedLenses                []string                  `json:"authorized_lenses,omitempty"`
	RequiredMaintainerAuthorization string                    `json:"required_maintainer_authorization"`
}

type CompactResultReopenRecord struct {
	LineageID        string                    `json:"lineage_id"`
	PreviousRevision string                    `json:"previous_revision"`
	Revision         string                    `json:"revision"`
	State            State                     `json:"state"`
	Quarantined      []CompactResultReopenSlot `json:"quarantined"`
	Retained         []CompactResultReopenSlot `json:"retained"`
	Replayed         bool                      `json:"replayed"`
}

type compactAdmittedReviewerResult struct {
	Schema    string            `json:"schema"`
	Subject   ArtifactSubject   `json:"subject"`
	Admission ArtifactAdmission `json:"admission"`
	Result    json.RawMessage   `json:"result"`
}

type compactProviderReviewerResult struct {
	SubjectHash string             `json:"subject_hash"`
	Inspection  ArtifactInspection `json:"inspection"`
	Lens        string             `json:"lens,omitempty"`
	Findings    []Finding          `json:"findings"`
	Evidence    []string           `json:"evidence"`
}

// CompactResultReopenAuthorization derives the exact maintainer binding for
// one reopen. authorizedLenses must already be canonical (selected order,
// deduplicated); when a maintainer overrides admitted slots the binding names
// them on their own line, so an authorization emitted for one override set can
// never be replayed to sweep a different one, and a binding emitted for a
// purely native plan (no line) can never quarantine an admitted slot. The
// empty case stays byte-identical to the historical binding so persisted
// audits keep re-deriving exactly.
func CompactResultReopenAuthorization(repository string, request CompactResultReopenRequest, quarantined, retained []CompactResultReopenSlot, authorizedLenses []string) string {
	entries := func(slots []CompactResultReopenSlot) string {
		values := make([]string, len(slots))
		for index, slot := range slots {
			values[index] = strconv.Itoa(slot.SelectedOrder) + ":" + slot.Lens + ":" + slot.ArtifactDigest + ":" + slot.ResultHash
		}
		return strings.Join(values, ",")
	}
	authorized := ""
	if len(authorizedLenses) > 0 {
		authorized = "\nauthorized_lenses=" + strings.Join(authorizedLenses, ",")
	}
	return compactResultReopenAuthorizationSchema +
		"\nrepository=" + repository +
		"\nlineage=" + request.LineageID +
		"\nrevision=" + request.ExpectedRevision +
		"\ntarget_identity=" + request.TargetIdentity +
		"\nquarantined=" + entries(quarantined) +
		"\nretained=" + entries(retained) +
		authorized +
		"\nactor=" + strings.TrimSpace(request.Actor) +
		"\nreason=" + strings.TrimSpace(request.Reason)
}

// canonicalReopenQuarantineLenses validates and canonicalizes the maintainer's
// named admitted-slot overrides: every entry must be a frozen selected lens,
// duplicates collapse, and the result is ordered by selected order so the
// binding and audit are deterministic regardless of flag order.
func canonicalReopenQuarantineLenses(state CompactState, lenses []string) ([]string, error) {
	if len(lenses) == 0 {
		return nil, nil
	}
	named := make(map[string]struct{}, len(lenses))
	for _, lens := range lenses {
		lens = strings.TrimSpace(lens)
		if stringIndex(state.SelectedLenses, lens) < 0 {
			return nil, fmt.Errorf("review reopen-results --quarantine-lens %q does not name a selected lens; this review's frozen selection is %s", lens, strings.Join(state.SelectedLenses, ", "))
		}
		named[lens] = struct{}{}
	}
	canonical := make([]string, 0, len(named))
	for _, lens := range state.SelectedLenses {
		if _, ok := named[lens]; ok {
			canonical = append(canonical, lens)
		}
	}
	return canonical, nil
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
	record, err := store.Load()
	if err != nil {
		return CompactResultReopenPlan{}, err
	}
	return buildCompactResultReopenPlan(ctx, repository, store, record, request)
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
	record, err := store.Load()
	if err != nil {
		return CompactResultReopenRecord{}, err
	}
	if replay, ok := replayCompactResultReopen(record, request); ok {
		return replay, nil
	}
	plan, err := buildCompactResultReopenPlan(ctx, repository, store, record, request)
	if err != nil {
		return CompactResultReopenRecord{}, err
	}
	if request.MaintainerAuthorization != plan.RequiredMaintainerAuthorization {
		return CompactResultReopenRecord{}, fmt.Errorf("review reopen-results requires the exact maintainer authorization binding emitted by --prepare (schema %s)", compactResultReopenAuthorizationSchema)
	}
	frozen, err := (SnapshotBuilder{Repo: repository}).FrozenCandidateContext(ctx, record.State.InitialSnapshot)
	if err != nil {
		return CompactResultReopenRecord{}, err
	}
	if request.ReopenedAt.IsZero() {
		request.ReopenedAt = time.Now().UTC()
	}
	audit := CompactResultReopen{
		PreviousRevision: record.Revision, TargetIdentity: record.State.InitialSnapshot.Identity,
		Quarantined:      append([]CompactResultReopenSlot(nil), plan.Quarantined...),
		Retained:         append([]CompactResultReopenSlot(nil), plan.Retained...),
		AuthorizedLenses: append([]string(nil), plan.AuthorizedLenses...),
		Reason:           strings.TrimSpace(request.Reason), Actor: strings.TrimSpace(request.Actor),
		ReopenedAt: request.ReopenedAt.UTC(), MaintainerAuthorization: request.MaintainerAuthorization,
	}
	next := record.State
	next.State = StateReviewing
	next.LensResults = []LensResult{}
	next.Findings = []Finding{}
	next.Classifications = map[string]FindingEvidence{}
	next.Outcomes = map[string]EvidenceOutcome{}
	next.FixFindingIDs = []string{}
	next.FollowUps = []FollowUp{}
	next.ProposedCorrectionLines = nil
	next.ActualCorrectionLines = nil
	next.FixDeltaHash = EmptyFixDeltaHash
	next.OriginalCriteria = nil
	next.CorrectionRegression = nil
	next.EvidenceHash = ""
	next.ResultReopens = append(append([]CompactResultReopen(nil), record.State.ResultReopens...), audit)
	revision, err := store.replaceContextGuarded(ctx, record.Revision, CompactResultReopenOperation, next, func() error {
		if _, statErr := os.Stat(store.ReceiptPath()); statErr == nil {
			return errors.New("review reopen-results refuses an authority that already has a receipt")
		} else if !os.IsNotExist(statErr) {
			return statErr
		}
		quarantined, retained, inspectErr := classifyCompactResultReopenSlots(store.Dir, record.State, frozen, plan.AuthorizedLenses)
		if inspectErr != nil {
			return inspectErr
		}
		if !reflect.DeepEqual(quarantined, plan.Quarantined) || !reflect.DeepEqual(retained, plan.Retained) {
			return errors.New("reviewer result artifacts changed after reopen authorization")
		}
		return nil
	})
	if err != nil {
		return CompactResultReopenRecord{}, err
	}
	return CompactResultReopenRecord{
		LineageID: request.LineageID, PreviousRevision: record.Revision, Revision: revision, State: StateReviewing,
		Quarantined: plan.Quarantined, Retained: plan.Retained,
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

// compactResultReopenStateEligible gates which authorities may reopen
// reviewer results. Validating keeps its historical full-pristineness
// requirements. Correction-required is additionally eligible — that is where
// admitted results carrying blocking findings land — but only while the
// authority is still uncorrected: no completed correction attempt, no actual
// correction accounting, no validation criteria, and no delivery evidence. A
// correction forecast alone (proposed lines with zero byte movement) does not
// close the door, because the product's own collect transition asks for that
// forecast before the operator can even see the reopen route. Everything the
// gate tolerates here is derived review state that the reopen transition
// resets and audits; the frozen scope, tier, budget, and candidate identity
// stay untouched either way.
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

func buildCompactResultReopenPlan(ctx context.Context, repository string, store CompactStore, record CompactRecord, request CompactResultReopenRequest) (CompactResultReopenPlan, error) {
	if record.Revision != request.ExpectedRevision {
		return CompactResultReopenPlan{}, fmt.Errorf("%w: expected compact revision %q, current %q", ErrConcurrentUpdate, request.ExpectedRevision, record.Revision)
	}
	state := record.State
	if state.InitialSnapshot.Identity != request.TargetIdentity || !compactResultReopenStateEligible(state) {
		return CompactResultReopenPlan{}, errors.New("review reopen-results requires an uncorrected validating or correction-required authority on the exact frozen target; run `gentle-ai review status --cwd <repo>` to see where this review actually is")
	}
	if _, err := os.Stat(store.ReceiptPath()); err == nil {
		return CompactResultReopenPlan{}, errors.New("review reopen-results refuses an authority that already has a receipt")
	} else if !os.IsNotExist(err) {
		return CompactResultReopenPlan{}, err
	}
	authorizedLenses, err := canonicalReopenQuarantineLenses(state, request.QuarantineLenses)
	if err != nil {
		return CompactResultReopenPlan{}, err
	}
	frozen, err := (SnapshotBuilder{Repo: repository}).FrozenCandidateContext(ctx, state.InitialSnapshot)
	if err != nil {
		return CompactResultReopenPlan{}, err
	}
	quarantined, retained, err := classifyCompactResultReopenSlots(store.Dir, state, frozen, authorizedLenses)
	if err != nil {
		return CompactResultReopenPlan{}, err
	}
	if len(quarantined) == 0 {
		return CompactResultReopenPlan{}, errors.New("review reopen-results found no unusable or unadmitted reviewer result to quarantine; to override admitted results whose reviewer input was wrong, name each one with `--quarantine-lens <lens>` and re-run `--prepare`")
	}
	plan := CompactResultReopenPlan{
		LineageID: request.LineageID, Revision: record.Revision, TargetIdentity: state.InitialSnapshot.Identity,
		Quarantined: quarantined, Retained: retained, AuthorizedLenses: authorizedLenses,
	}
	plan.RequiredMaintainerAuthorization = CompactResultReopenAuthorization(repository, request, quarantined, retained, authorizedLenses)
	return plan, nil
}

// classifyCompactResultReopenSlots splits every selected lens slot into
// quarantined and retained. Native detection quarantines only what the store
// can prove unusable: an artifact that fails exact re-admission or evidence
// reporting that candidate inspection was unavailable. authorizedLenses adds
// the maintainer's explicitly named admitted slots — the one and only path by
// which a structurally valid, provider-admitted result leaves its slot.
func classifyCompactResultReopenSlots(storeDir string, state CompactState, frozen FrozenCandidateContext, authorizedLenses []string) ([]CompactResultReopenSlot, []CompactResultReopenSlot, error) {
	authorized := make(map[string]struct{}, len(authorizedLenses))
	for _, lens := range authorizedLenses {
		authorized[lens] = struct{}{}
	}
	quarantined := make([]CompactResultReopenSlot, 0)
	retained := make([]CompactResultReopenSlot, 0)
	for order, lens := range state.SelectedLenses {
		slot, trusted, err := inspectCompactResultReopenSlot(storeDir, state, frozen, order, lens)
		if err != nil {
			return nil, nil, err
		}
		_, named := authorized[lens]
		unusable := !trusted || named
		for _, evidence := range state.LensResults[order].Evidence {
			unusable = unusable || evidenceReportsUnavailableInspection(evidence)
		}
		if unusable {
			quarantined = append(quarantined, slot)
		} else {
			retained = append(retained, slot)
		}
	}
	return quarantined, retained, nil
}

func inspectCompactResultReopenSlot(storeDir string, state CompactState, frozen FrozenCandidateContext, order int, lens string) (CompactResultReopenSlot, bool, error) {
	if order >= len(state.LensResults) {
		return CompactResultReopenSlot{}, false, errors.New("validating authority does not contain every selected lens result")
	}
	path := filepath.Join(storeDir, CompactReviewerResultsDir, fmt.Sprintf("%02d-%s.json", order, lens))
	payload, digest, err := readCompactReviewerArtifact(path)
	if err != nil {
		return CompactResultReopenSlot{}, false, fmt.Errorf("inspect admitted reviewer result %d: %w", order, err)
	}
	slot := CompactResultReopenSlot{
		Lens: lens, SelectedOrder: order, ArtifactDigest: digest, ResultHash: state.LensResults[order].ResultHash,
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	var envelope compactAdmittedReviewerResult
	if err := decoder.Decode(&envelope); err != nil {
		return slot, false, nil
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF || envelope.Schema != AdmittedReviewerResultSchema || len(envelope.Result) == 0 {
		return slot, false, nil
	}
	expected, err := NewArtifactSubject(state, envelope.Subject.AuthorityRevision, frozen, lens, order, envelope.Subject.CorrectionTargetIdentity)
	if err != nil || envelope.Subject != expected || envelope.Admission.Validate(expected) != nil {
		return slot, false, nil
	}
	result, admitted := reAdmitCompactReviewerResult(envelope, expected, frozen)
	if !admitted || result.ResultHash != state.LensResults[order].ResultHash {
		return slot, false, nil
	}
	slot.SubjectHash, slot.AuthorityRevision = expected.SubjectHash, expected.AuthorityRevision
	return slot, true, nil
}

func reAdmitCompactReviewerResult(envelope compactAdmittedReviewerResult, expected ArtifactSubject, frozen FrozenCandidateContext) (LensResult, bool) {
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
	result, admission, err := AdmitArtifact(ArtifactAdmissionRequest{
		ExpectedSubject: expected, FrozenContext: frozen, EchoedSubjectHash: provider.SubjectHash,
		Inspection:                provider.Inspection,
		Result:                    LensResult{Lens: expected.Lens, Findings: provider.Findings, Evidence: provider.Evidence},
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

func readCompactReviewerArtifact(path string) ([]byte, string, error) {
	payload, err := readPrivateCompactReviewerFile(
		path,
		compactReviewerResultSizeLimit,
	)
	if err != nil {
		return nil, "", errors.New("reviewer artifact is unreadable or outside the native size bound")
	}
	digestPayload, err := readPrivateCompactReviewerFile(path+".sha256", 256)
	if err != nil {
		return nil, "", errors.New("reviewer artifact digest sidecar is unavailable")
	}
	digest := strings.TrimSpace(string(digestPayload))
	if !validSHA256(digest) || compactPreservedPayloadDigest(payload) != digest {
		return nil, "", errors.New("reviewer artifact digest does not match its bytes")
	}
	return payload, digest, nil
}

func replayCompactResultReopen(record CompactRecord, request CompactResultReopenRequest) (CompactResultReopenRecord, bool) {
	for _, reopen := range record.State.ResultReopens {
		if reopen.PreviousRevision != request.ExpectedRevision || reopen.TargetIdentity != request.TargetIdentity ||
			reopen.Reason != strings.TrimSpace(request.Reason) || reopen.Actor != strings.TrimSpace(request.Actor) ||
			reopen.MaintainerAuthorization != request.MaintainerAuthorization {
			continue
		}
		return CompactResultReopenRecord{
			LineageID: record.State.LineageID, PreviousRevision: reopen.PreviousRevision, Revision: record.Revision,
			State: record.State.State, Quarantined: reopen.Quarantined, Retained: reopen.Retained, Replayed: true,
		}, true
	}
	return CompactResultReopenRecord{}, false
}

func ReviewerResultDigestIsQuarantined(state CompactState, order int, digest string) bool {
	for index := len(state.ResultReopens) - 1; index >= 0; index-- {
		for _, slot := range state.ResultReopens[index].Quarantined {
			if slot.SelectedOrder == order && slot.ArtifactDigest == digest {
				return true
			}
		}
	}
	return false
}

func ReviewerResultAdmissionWasRetained(state CompactState, order int, digest, subjectHash, authorityRevision, resultHash string) bool {
	for index := len(state.ResultReopens) - 1; index >= 0; index-- {
		for _, slot := range state.ResultReopens[index].Retained {
			if slot.SelectedOrder == order && slot.ArtifactDigest == digest && slot.SubjectHash == subjectHash &&
				slot.AuthorityRevision == authorityRevision && slot.ResultHash == resultHash {
				return true
			}
		}
	}
	return false
}

func ReviewerResultQuarantinePath(storeDir string, state CompactState, order int, digest string) (string, bool) {
	for index := len(state.ResultReopens) - 1; index >= 0; index-- {
		reopen := state.ResultReopens[index]
		for _, slot := range reopen.Quarantined {
			if slot.SelectedOrder == order && slot.ArtifactDigest == digest {
				name := fmt.Sprintf("%02d-%s.json", slot.SelectedOrder, slot.Lens)
				return filepath.Join(storeDir, CompactQuarantinedReviewerResultsDir, strings.TrimPrefix(reopen.PreviousRevision, "sha256:")[:16], name), true
			}
		}
	}
	return "", false
}
