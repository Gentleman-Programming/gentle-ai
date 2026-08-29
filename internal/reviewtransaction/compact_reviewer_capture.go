package reviewtransaction

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"sort"
	"time"
)

// ErrCapturedReviewerResultSlotConflict reports an immutable reviewer result
// slot occupied by different canonical bytes.
var ErrCapturedReviewerResultSlotConflict = errors.New("captured reviewer result slot conflicts with different canonical bytes") // refusal:by-design world-action: transaction-layer capture cannot alter an immutable occupied slot

// compactAdmittedReviewerResult is the canonical lens envelope retained in the
// sole admitted-role owner. It is decoded by readback and derived review views.
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

type compactAdmittedRefuterValue struct {
	Results []EvidenceResult `json:"results"`
}

type compactAdmittedTargetedValidatorValue struct {
	Outcome string `json:"outcome"`
}

func decodeCompactAdmittedReviewerValue(value []byte) (compactAdmittedReviewerResult, error) {
	canonical, err := canonicalCompactRoleValue(value)
	if err != nil {
		return compactAdmittedReviewerResult{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(canonical))
	decoder.DisallowUnknownFields()
	var envelope compactAdmittedReviewerResult
	if err := decoder.Decode(&envelope); err != nil {
		return compactAdmittedReviewerResult{}, err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return compactAdmittedReviewerResult{}, errors.New("admitted reviewer value must contain exactly one object") // refusal:by-design world-action: an immutable role value that is not one canonical envelope requires authority replacement
	}
	encoded, err := json.Marshal(envelope)
	if err != nil || !bytes.Equal(canonical, encoded) {
		return compactAdmittedReviewerResult{}, errors.New("admitted reviewer value is not canonical") // refusal:by-design world-action: an immutable role value that is not canonical requires authority replacement
	}
	return envelope, nil
}

func decodeCompactAdmittedRefuterValue(value []byte) ([]EvidenceResult, error) {
	var payload compactAdmittedRefuterValue
	if err := decodeCompactAdmittedRoleValue(value, &payload); err != nil || payload.Results == nil {
		return nil, errors.New("admitted refuter value is invalid") // refusal:by-design world-action: an immutable refuter value must be replaced by its provider
	}
	return append([]EvidenceResult(nil), payload.Results...), nil
}

func decodeCompactAdmittedTargetedValidatorValue(value []byte) (string, error) {
	var payload compactAdmittedTargetedValidatorValue
	if err := decodeCompactAdmittedRoleValue(value, &payload); err != nil || payload.Outcome != "passed" && payload.Outcome != "failed" {
		return "", errors.New("admitted targeted-validator value is invalid") // refusal:by-design world-action: an immutable validator value must be replaced by its provider
	}
	return payload.Outcome, nil
}

func decodeCompactAdmittedRoleValue(value []byte, target any) error {
	canonical, err := canonicalCompactRoleValue(value)
	if err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(canonical))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return errors.New("admitted role value must contain exactly one object") // refusal:by-design world-action: an immutable role value that is not one canonical object requires authority replacement
	}
	encoded, err := json.Marshal(target)
	if err != nil || !bytes.Equal(canonical, encoded) {
		return errors.New("admitted role value is not canonical") // refusal:by-design world-action: an immutable role value that is not canonical requires authority replacement
	}
	return nil
}

// CompactAdmittedReviewerResultRequest contains one provider-observed reviewer
// result and the exact native authority preimages that result must bind.
type CompactAdmittedReviewerResultRequest struct {
	ExpectedRevision          string
	TargetIdentity            string
	FrozenContext             FrozenCandidateContext
	ArtifactSubject           ArtifactSubject
	Inspection                ArtifactInspection
	Result                    LensResult
	CandidateCausalFindingIDs []string
	RawPayload                []byte
	// PreparePublication performs caller-owned quarantine work under the authority lock.
	PreparePublication func(CompactState) error
}

// CompactAdmittedReviewerCapture is the canonical readback from one durable
// selected-lens slot. Its subject, admission, payload, and digest originate
// from immutable storage, never from the provider's in-memory host output.
type CompactAdmittedReviewerCapture struct {
	LensResult
	Slot      CompactReviewerResultSlot
	Subject   ArtifactSubject
	Admission ArtifactAdmission
}

// ResolveAdmittedReviewerResult reads and re-admits one already captured
// reviewer slot without publishing or changing compact authority.
func (store CompactStore) ResolveAdmittedReviewerResult(ctx context.Context, expectedRevision, targetIdentity string, frozen FrozenCandidateContext, subject ArtifactSubject) (result LensResult, found bool, err error) {
	return store.resolveAdmittedReviewerResult(ctx, expectedRevision, targetIdentity, frozen, subject, nil)
}

func (store CompactStore) resolveAdmittedReviewerResult(ctx context.Context, expectedRevision, targetIdentity string, frozen FrozenCandidateContext, subject ArtifactSubject, expectedAdmission *ArtifactAdmission) (result LensResult, found bool, err error) {
	if ctx == nil {
		return LensResult{}, false, errors.New("resolve admitted reviewer result context is nil")
	}
	if err = ctx.Err(); err != nil {
		return LensResult{}, false, err
	}
	if !validSHA256(expectedRevision) || !validSHA256(targetIdentity) || targetIdentity != subject.TargetIdentity || subject.AuthorityRevision != expectedRevision {
		return LensResult{}, false, errors.New("resolve admitted reviewer result requires an exact revision and target")
	}
	record, err := store.LoadContext(ctx)
	if err != nil {
		return LensResult{}, false, err
	}
	state := record.State
	if record.HistoricalCompat {
		return LensResult{}, false, NewLegacyReadOnlyError("review/capture-result", state.LineageID)
	}
	if state.CapturePhaseRevision != expectedRevision || state.State != StateReviewing || state.InitialSnapshot.Identity != targetIdentity ||
		subject.SelectedOrder < 0 || subject.SelectedOrder >= len(state.SelectedLenses) || state.SelectedLenses[subject.SelectedOrder] != subject.Lens {
		// refusal:by-design operator-knowledge: the provider must refresh the exact current revision, target, lens, and order before resolving this slot
		return LensResult{}, false, errors.New("resolve binding does not match the current reviewing authority")
	}
	builder := SnapshotBuilder{Repo: store.repo}
	nativeFrozen, err := builder.FrozenCandidateContext(ctx, state.InitialSnapshot)
	if err != nil {
		return LensResult{}, false, err
	}
	nativeFrozen, expected, err := artifactSubjectForSchema(ctx, builder, state, expectedRevision, nativeFrozen, subject.Lens, subject.SelectedOrder, subject.CorrectionTargetIdentity, subject.Schema)
	if err != nil {
		return LensResult{}, false, err
	}
	if !reflect.DeepEqual(nativeFrozen, frozen) || expected != subject {
		return LensResult{}, false, errors.New("resolved reviewer result does not match repository authority")
	}
	for _, entry := range state.AdmittedRoleResults {
		if !compactAdmittedRoleResultCanSatisfyActiveCapture(state, entry) || entry.Role != CompactRoleLens || entry.CapturePhaseRevision != expectedRevision ||
			entry.TargetIdentity != targetIdentity || entry.SelectedOrder != subject.SelectedOrder || entry.Lens != subject.Lens {
			continue
		}
		slot := CompactReviewerResultSlot{Occupied: true, Payload: append(append([]byte(nil), entry.Value...), '\n'), Digest: entry.ArtifactDigest}
		capture, captureErr := compactAdmittedReviewerCaptureFromSlot(ctx, slot, nativeFrozen, expected)
		if captureErr != nil {
			return LensResult{}, false, captureErr
		}
		if expectedAdmission != nil && !reflect.DeepEqual(capture.Admission, *expectedAdmission) {
			return LensResult{}, false, errors.New("admitted reviewer result does not match repository authority") // refusal:by-design world-action: this structural compact-authority invariant requires a provider code fix; no operator command can safely repair it
		}
		return capture.LensResult, true, nil
	}
	return LensResult{}, false, nil
}

// CaptureAdmittedReviewerResult admits, durably publishes, and reads back one
// real reviewer result while the compact reviewing authority and selected lens
// slot remain locked to the request's exact revision and target.
func (store CompactStore) CaptureAdmittedReviewerResult(
	ctx context.Context,
	request CompactAdmittedReviewerResultRequest,
) (CompactAdmittedReviewerCapture, error) {
	if ctx == nil {
		return CompactAdmittedReviewerCapture{}, errors.New("capture admitted reviewer result context is nil")
	}
	if err := ctx.Err(); err != nil {
		return CompactAdmittedReviewerCapture{}, err
	}
	if !validSHA256(request.ExpectedRevision) ||
		!validSHA256(request.TargetIdentity) ||
		request.TargetIdentity != request.ArtifactSubject.TargetIdentity {
		return CompactAdmittedReviewerCapture{}, errors.New( // refusal:by-design world-action: this structural compact-authority invariant requires a provider code fix; no operator command can safely repair it
			"capture admitted reviewer result requires an exact capture phase and target",
		)
	}
	if err := ValidateArtifactSubject(request.ArtifactSubject); err != nil {
		return CompactAdmittedReviewerCapture{}, err
	}
	if request.ArtifactSubject.AuthorityRevision != request.ExpectedRevision ||
		request.Result.Lens != request.ArtifactSubject.Lens {
		return CompactAdmittedReviewerCapture{}, errors.New( // refusal:by-design world-action: this structural compact-authority invariant requires a provider code fix; no operator command can safely repair it
			"reviewer result does not bind the requested capture phase lens",
		)
	}
	if len(request.RawPayload) == 0 || len(request.RawPayload) > compactReviewerResultSizeLimit {
		return CompactAdmittedReviewerCapture{}, errors.New(
			"raw reviewer result is empty or outside the native size bound",
		)
	}

	canonicalInspection := request.Inspection
	canonicalInspectionPaths, err := canonicalPaths(request.Inspection.Paths)
	if err != nil {
		return CompactAdmittedReviewerCapture{}, err
	}
	canonicalInspection.Paths = canonicalInspectionPaths
	canonicalResult, err := CanonicalCompactLensResult(request.Result)
	if err != nil {
		return CompactAdmittedReviewerCapture{}, err
	}
	providerResult := compactProviderReviewerResult{
		SubjectHash: request.ArtifactSubject.SubjectHash,
		Inspection:  canonicalInspection,
		Lens:        request.ArtifactSubject.Lens,
		Findings:    canonicalResult.Findings,
		Evidence:    canonicalResult.Evidence,
	}
	canonicalPayload, err := json.Marshal(providerResult)
	if err != nil {
		return CompactAdmittedReviewerCapture{}, err
	}
	canonicalPayload = append(canonicalPayload, '\n')

	// Admission is deliberately outside the lock. The only facts it consumes are
	// frozen into Pn, so sibling captures can validate in parallel before each
	// performs its short lock-owned CAS merge.
	record, err := store.LoadContext(ctx)
	if err != nil {
		return CompactAdmittedReviewerCapture{}, err
	}
	if record.HistoricalCompat {
		return CompactAdmittedReviewerCapture{}, NewLegacyReadOnlyError("review/capture-result", record.State.LineageID)
	}
	state := record.State
	if state.State != StateReviewing || state.CapturePhaseRevision != request.ExpectedRevision ||
		state.InitialSnapshot.Identity != request.TargetIdentity || request.ArtifactSubject.SelectedOrder < 0 ||
		request.ArtifactSubject.SelectedOrder >= len(state.SelectedLenses) ||
		state.SelectedLenses[request.ArtifactSubject.SelectedOrder] != request.ArtifactSubject.Lens {
		return CompactAdmittedReviewerCapture{}, errors.New("capture binding does not match the current reviewing authority") // refusal:by-design world-action: this structural compact-authority invariant requires a provider code fix; no operator command can safely repair it
	}
	builder := SnapshotBuilder{Repo: store.repo}
	nativeContext, err := builder.FrozenCandidateContext(ctx, state.InitialSnapshot)
	if err != nil {
		return CompactAdmittedReviewerCapture{}, err
	}
	nativeContext, expected, err := artifactSubjectForSchema(
		ctx, builder, state, request.ExpectedRevision, nativeContext, request.ArtifactSubject.Lens,
		request.ArtifactSubject.SelectedOrder, request.ArtifactSubject.CorrectionTargetIdentity, request.ArtifactSubject.Schema,
	)
	if err != nil {
		return CompactAdmittedReviewerCapture{}, err
	}
	if !reflect.DeepEqual(nativeContext, request.FrozenContext) {
		return CompactAdmittedReviewerCapture{}, errors.New("reviewer frozen context does not match repository authority")
	}
	if expected != request.ArtifactSubject {
		return CompactAdmittedReviewerCapture{}, errors.New("reviewer artifact subject does not match the frozen authority") // refusal:by-design world-action: this structural compact-authority invariant requires a provider code fix; no operator command can safely repair it
	}
	admitted, admission, err := AdmitArtifact(ctx, ArtifactAdmissionRequest{
		ExpectedSubject:           expected,
		FrozenContext:             nativeContext,
		EchoedSubjectHash:         providerResult.SubjectHash,
		Inspection:                canonicalInspection,
		Result:                    canonicalResult,
		CandidateCausalFindingIDs: request.CandidateCausalFindingIDs,
		RawPayload:                request.RawPayload,
		CanonicalPayload:          canonicalPayload,
	})
	if err != nil {
		return CompactAdmittedReviewerCapture{}, err
	}
	envelopePayload, err := json.Marshal(compactAdmittedReviewerResult{
		Schema:    admittedReviewerResultSchemaForSubject(expected),
		Subject:   expected,
		Admission: admission,
		Result:    append(json.RawMessage(nil), canonicalPayload[:len(canonicalPayload)-1]...),
	})
	if err != nil {
		return CompactAdmittedReviewerCapture{}, err
	}
	envelopePayload = append(envelopePayload, '\n')
	if len(envelopePayload) > compactReviewerResultSizeLimit {
		return CompactAdmittedReviewerCapture{}, errors.New("admitted reviewer result exceeds the native size bound")
	}
	value, err := canonicalCompactRoleValue(envelopePayload[:len(envelopePayload)-1])
	if err != nil {
		return CompactAdmittedReviewerCapture{}, err
	}
	entry := CompactAdmittedRoleResult{
		Role:                 CompactRoleLens,
		Lens:                 expected.Lens,
		SelectedOrder:        expected.SelectedOrder,
		TargetIdentity:       expected.TargetIdentity,
		CapturePhaseRevision: request.ExpectedRevision,
		ArtifactDigest:       compactPreservedPayloadDigest(append(append([]byte(nil), value...), '\n')),
		ResultHash:           admitted.ResultHash,
		Value:                value,
	}
	return store.mergeAdmittedLensResult(ctx, request, expected, admitted, admission, entry)
}

// mergeAdmittedLensResult owns the brief CAS section after reviewer admission.
// It reloads current Rn, verifies the stable Pn tuple, accepts exact replay
// without writing, and appends only a previously empty lens tuple.
func (store CompactStore) mergeAdmittedLensResult(
	ctx context.Context,
	request CompactAdmittedReviewerResultRequest,
	subject ArtifactSubject,
	admitted LensResult,
	admission ArtifactAdmission,
	entry CompactAdmittedRoleResult,
) (CompactAdmittedReviewerCapture, error) {
	maintenance, err := store.acquireReadMaintenance(ctx)
	if err != nil {
		return CompactAdmittedReviewerCapture{}, err
	}
	if maintenance != nil {
		defer maintenance.Release()
	}
	deadline := time.NewTimer(maintenanceLockTimeout)
	defer deadline.Stop()
	var lock *storeLock
	for {
		lock, err = acquireStoreLock(store.lockPath)
		if !errors.Is(err, ErrConcurrentUpdate) {
			break
		}
		select {
		case <-deadline.C:
			return CompactAdmittedReviewerCapture{}, &AuthorityLockTimeoutError{Timeout: maintenanceLockTimeout}
		case <-time.After(10 * time.Millisecond):
		}
	}
	if err != nil {
		return CompactAdmittedReviewerCapture{}, err
	}
	defer lock.release()

	record, err := store.loadCompactRecordLocked()
	if err != nil {
		return CompactAdmittedReviewerCapture{}, err
	}
	if record.HistoricalCompat {
		return CompactAdmittedReviewerCapture{}, NewLegacyReadOnlyError("review/capture-result", record.State.LineageID)
	}
	state := record.State
	if state.State != StateReviewing || state.CapturePhaseRevision != request.ExpectedRevision ||
		state.InitialSnapshot.Identity != request.TargetIdentity || subject.AuthorityRevision != request.ExpectedRevision ||
		subject.SelectedOrder < 0 || subject.SelectedOrder >= len(state.SelectedLenses) ||
		state.SelectedLenses[subject.SelectedOrder] != subject.Lens {
		return CompactAdmittedReviewerCapture{}, errors.New("capture binding does not match the current reviewing authority") // refusal:by-design world-action: this structural compact-authority invariant requires a provider code fix; no operator command can safely repair it
	}
	for _, existing := range state.AdmittedRoleResults {
		if !compactAdmittedRoleResultCanSatisfyActiveCapture(state, existing) || existing.Role != CompactRoleLens || existing.SelectedOrder != subject.SelectedOrder || existing.Lens != subject.Lens {
			continue
		}
		existingValue, err := canonicalCompactRoleValue(existing.Value)
		if err != nil {
			return CompactAdmittedReviewerCapture{}, fmt.Errorf("decode stored admitted role value: %w", err)
		}
		if existing.TargetIdentity == entry.TargetIdentity &&
			existing.CapturePhaseRevision == entry.CapturePhaseRevision &&
			existing.RequestHash == entry.RequestHash &&
			existing.ArtifactDigest == entry.ArtifactDigest &&
			existing.ResultHash == entry.ResultHash && bytes.Equal(existingValue, entry.Value) {
			return CompactAdmittedReviewerCapture{
				LensResult: admitted,
				Slot: CompactReviewerResultSlot{
					Occupied: true,
					Payload:  append(existingValue, '\n'),
					Digest:   existing.ArtifactDigest,
				},
				Subject: subject, Admission: admission,
			}, nil
		}
		return CompactAdmittedReviewerCapture{}, ErrCapturedReviewerResultSlotConflict
	}
	if request.PreparePublication != nil {
		if err := request.PreparePublication(state); err != nil {
			return CompactAdmittedReviewerCapture{}, err
		}
	}
	next := cloneCompactStateInitialAtomicStart(state)
	next.AdmittedRoleResults = append(next.AdmittedRoleResults, entry)
	sort.Slice(next.AdmittedRoleResults, func(left, right int) bool {
		leftEntry, rightEntry := next.AdmittedRoleResults[left], next.AdmittedRoleResults[right]
		if leftEntry.Role != rightEntry.Role {
			return leftEntry.Role < rightEntry.Role
		}
		if leftEntry.Role == CompactRoleLens && leftEntry.SelectedOrder != rightEntry.SelectedOrder {
			return leftEntry.SelectedOrder < rightEntry.SelectedOrder
		}
		return leftEntry.Lens < rightEntry.Lens
	})
	_, payload, err := makeCompactRecord(next)
	if err != nil {
		return CompactAdmittedReviewerCapture{}, err
	}
	if err := writeAtomic(store.StatePath(), payload, 0o644); err != nil {
		return CompactAdmittedReviewerCapture{}, err
	}
	return CompactAdmittedReviewerCapture{
		LensResult: admitted,
		Slot: CompactReviewerResultSlot{
			Occupied: true,
			Payload:  append(append([]byte(nil), entry.Value...), '\n'),
			Digest:   entry.ArtifactDigest,
		},
		Subject: subject, Admission: admission,
	}, nil
}

func canonicalCompactRoleValue(value []byte) (json.RawMessage, error) {
	var canonical bytes.Buffer
	if err := json.Compact(&canonical, value); err != nil {
		return nil, err
	}
	if canonical.Len() == 0 {
		return nil, errors.New("admitted role value is empty") // refusal:by-design world-action: this structural compact-authority invariant requires a provider code fix; no operator command can safely repair it
	}
	return append(json.RawMessage(nil), canonical.Bytes()...), nil
}

// mergeAdmittedNonLensRoleResult persists one refuter or targeted-validator
// value under the same brief lock-owned merge contract as lens capture. The
// request's Pn stays stable while this write advances only live Rn.
func (store CompactStore) mergeAdmittedNonLensRoleResult(
	ctx context.Context,
	role CompactRole,
	expectedPhase, targetIdentity, requestHash string,
	payload []byte,
	prepare func(CompactState) error,
	validateCurrent func(CompactState) error,
) error {
	if ctx == nil {
		return errors.New("capture admitted provider role result context is nil") // refusal:by-design world-action: this structural compact-authority invariant requires a provider code fix; no operator command can safely repair it
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if role != CompactRoleRefuter && role != CompactRoleTargetedValidator ||
		!validSHA256(expectedPhase) || !validSHA256(targetIdentity) || !validSHA256(requestHash) ||
		len(payload) == 0 || len(payload) > compactReviewerResultSizeLimit || validateCurrent == nil {
		return errors.New("capture admitted provider role result requires an exact role, phase, target, request, and bounded payload") // refusal:by-design world-action: this structural compact-authority invariant requires a provider code fix; no operator command can safely repair it
	}
	value, err := canonicalCompactRoleValue(payload)
	if err != nil {
		return err
	}
	entry := CompactAdmittedRoleResult{
		Role: role, TargetIdentity: targetIdentity, CapturePhaseRevision: expectedPhase,
		RequestHash: requestHash, ArtifactDigest: compactPreservedPayloadDigest(append(append([]byte(nil), value...), '\n')),
		Value: value,
	}

	maintenance, err := store.acquireReadMaintenance(ctx)
	if err != nil {
		return err
	}
	if maintenance != nil {
		defer maintenance.Release()
	}
	deadline := time.NewTimer(maintenanceLockTimeout)
	defer deadline.Stop()
	var lock *storeLock
	for {
		lock, err = acquireStoreLock(store.lockPath)
		if !errors.Is(err, ErrConcurrentUpdate) {
			break
		}
		select {
		case <-deadline.C:
			return &AuthorityLockTimeoutError{Timeout: maintenanceLockTimeout}
		case <-time.After(10 * time.Millisecond):
		}
	}
	if err != nil {
		return err
	}
	defer lock.release()

	record, err := store.loadCompactRecordLocked()
	if err != nil {
		return err
	}
	if record.HistoricalCompat {
		return NewLegacyReadOnlyError("review/capture-provider-role", record.State.LineageID)
	}
	state := record.State
	if state.CapturePhaseRevision != expectedPhase {
		return errors.New("provider role capture binding does not match the current authority") // refusal:by-design world-action: this structural compact-authority invariant requires a provider code fix; no operator command can safely repair it
	}
	if err := validateCurrent(state); err != nil {
		return errors.New("provider role capture binding does not match the current authority") // refusal:by-design world-action: this structural compact-authority invariant requires a provider code fix; no operator command can safely repair it
	}
	for _, existing := range state.AdmittedRoleResults {
		if !compactAdmittedRoleResultCanSatisfyActiveCapture(state, existing) || existing.Role != role || existing.CapturePhaseRevision != expectedPhase ||
			existing.TargetIdentity != targetIdentity || existing.RequestHash != requestHash {
			continue
		}
		existingValue, valueErr := canonicalCompactRoleValue(existing.Value)
		if valueErr != nil {
			return fmt.Errorf("decode stored admitted provider role value: %w", valueErr)
		}
		if existing.ArtifactDigest == entry.ArtifactDigest && bytes.Equal(existingValue, entry.Value) {
			return nil
		}
		return ErrCapturedReviewerResultSlotConflict
	}
	if prepare != nil {
		if err := prepare(state); err != nil {
			return err
		}
	}
	next := cloneCompactStateInitialAtomicStart(state)
	next.AdmittedRoleResults = append(next.AdmittedRoleResults, entry)
	sort.Slice(next.AdmittedRoleResults, func(left, right int) bool {
		leftEntry, rightEntry := next.AdmittedRoleResults[left], next.AdmittedRoleResults[right]
		if leftEntry.Role != rightEntry.Role {
			return leftEntry.Role < rightEntry.Role
		}
		return leftEntry.SelectedOrder < rightEntry.SelectedOrder
	})
	_, recordPayload, err := makeCompactRecord(next)
	if err != nil {
		return err
	}
	return writeAtomic(store.StatePath(), recordPayload, 0o644)
}

func compactAdmittedReviewerCaptureFromSlot(
	ctx context.Context,
	slot CompactReviewerResultSlot,
	frozen FrozenCandidateContext,
	subject ArtifactSubject,
) (CompactAdmittedReviewerCapture, error) {
	if !slot.Occupied {
		// refusal:by-design human-authority: a successful immutable publication without a verifiable readback requires storage inspection, not a retry that could misstate evidence
		return CompactAdmittedReviewerCapture{}, errors.New("admitted reviewer result readback is missing")
	}
	envelope, err := decodeCompactAdmittedReviewerValue(slot.Payload)
	if err != nil {
		return CompactAdmittedReviewerCapture{}, fmt.Errorf("decode admitted reviewer result readback: %w", err)
	}
	if envelope.Schema != admittedReviewerResultSchemaForSubject(subject) || envelope.Subject != subject || envelope.Admission.Validate(subject) != nil {
		// refusal:by-design human-authority: immutable readback bytes that no longer bind the admitted authority require storage inspection rather than replacement
		return CompactAdmittedReviewerCapture{}, errors.New("admitted reviewer result readback does not match repository authority")
	}
	result, found := reAdmitCompactReviewerResult(ctx, envelope, subject, frozen)
	if !found {
		// refusal:by-design human-authority: immutable evidence that fails native re-admission must be inspected or quarantined by an authorized maintainer
		return CompactAdmittedReviewerCapture{}, errors.New("admitted reviewer result readback failed native re-admission")
	}
	return CompactAdmittedReviewerCapture{LensResult: result, Slot: slot, Subject: envelope.Subject, Admission: envelope.Admission}, nil
}

func artifactSubjectForSchema(
	ctx context.Context,
	builder SnapshotBuilder,
	state CompactState,
	revision string,
	frozen FrozenCandidateContext,
	lens string,
	order int,
	correctionTargetIdentity string,
	schema string,
) (FrozenCandidateContext, ArtifactSubject, error) {
	if schema == ArtifactSubjectSchemaV1 {
		legacy, err := builder.WithLegacyCandidateDiff(ctx, state.InitialSnapshot, frozen)
		if err != nil {
			return FrozenCandidateContext{}, ArtifactSubject{}, err
		}
		subject, err := NewLegacyArtifactSubject(state, revision, legacy, lens, order, correctionTargetIdentity)
		return legacy, subject, err
	}
	subject, err := NewArtifactSubject(state, revision, frozen, lens, order, correctionTargetIdentity)
	return frozen, subject, err
}

// CompactReviewerResultSlot is a transient capture readback. It is not a
// persisted slot: its bytes come from the one admitted record value.
type CompactReviewerResultSlot struct {
	Occupied bool
	Payload  []byte
	Digest   string
}

// CaptureAdmittedRefuterResult merges a canonical refuter value into the one
// compact record. The stable capture phase binds the role while the record CAS
// advances the live revision.
func (store CompactStore) CaptureAdmittedRefuterResult(ctx context.Context, request CompactAdmittedRefuterResultRequest) error {
	return store.mergeAdmittedNonLensRoleResult(ctx, CompactRoleRefuter, request.ExpectedRevision, request.TargetIdentity, request.RequestHash, request.Payload,
		request.PreparePublication,
		func(state CompactState) error {
			if state.State != StateReviewing || state.InitialSnapshot.Identity != request.TargetIdentity {
				return errors.New("refuter capture binding does not match the current reviewing authority") // refusal:by-design world-action: this structural compact-authority invariant requires a provider code fix; no operator command can safely repair it
			}
			return nil
		},
	)
}

// CompactAdmittedRefuterResultRequest contains the one Go-admitted refuter batch.
type CompactAdmittedRefuterResultRequest struct {
	ExpectedRevision   string
	TargetIdentity     string
	RequestHash        string
	Payload            []byte
	PreparePublication func(CompactState) error
}

// CompactAdmittedTargetedValidatorResultRequest contains one conclusive
// targeted-validator value. Rejected values are represented only by the bounded
// hash-only attempt ledger.
type CompactAdmittedTargetedValidatorResultRequest struct {
	ExpectedRequest    TargetedValidationRequest
	Payload            []byte
	PreparePublication func(CompactState, TargetedValidationRequest) error
}

func (store CompactStore) CaptureAdmittedTargetedValidatorResult(ctx context.Context, request CompactAdmittedTargetedValidatorResultRequest) error {
	if err := ValidateTargetedValidationRequest(request.ExpectedRequest); err != nil {
		return err
	}
	return store.mergeAdmittedNonLensRoleResult(ctx, CompactRoleTargetedValidator,
		request.ExpectedRequest.ExpectedRevision, request.ExpectedRequest.CorrectionTargetIdentity, request.ExpectedRequest.RequestHash, request.Payload,
		func(state CompactState) error {
			if request.PreparePublication == nil {
				return nil
			}
			return request.PreparePublication(state, request.ExpectedRequest)
		},
		func(state CompactState) error {
			if state.State != StateCorrectionRequired || state.ProposedCorrectionLines == nil ||
				state.CapturePhaseRevision != request.ExpectedRequest.ExpectedRevision ||
				state.InitialSnapshot.Identity != request.ExpectedRequest.TargetIdentity {
				return errors.New("targeted validator capture binding does not match the current open correction authority") // refusal:by-design world-action: this structural compact-authority invariant requires a provider code fix; no operator command can safely repair it
			}
			return nil
		},
	)
}

// RecordInconclusiveTargetedValidatorAttempt appends one hash-only non-verdict
// under the authority lock. Exact replay does not write and a fourth digest is
// refused without mutation.
func (store CompactStore) RecordInconclusiveTargetedValidatorAttempt(ctx context.Context, request TargetedValidationRequest, attemptDigest string) (bool, error) {
	if ctx == nil {
		return false, errors.New("targeted validator attempt context is nil") // refusal:by-design world-action: this structural compact-authority invariant requires a provider code fix; no operator command can safely repair it
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if err := ValidateTargetedValidationRequest(request); err != nil || !validSHA256(attemptDigest) {
		return false, errors.New("targeted validator attempt requires a valid request and digest") // refusal:by-design world-action: this structural compact-authority invariant requires a provider code fix; no operator command can safely repair it
	}
	maintenance, err := store.acquireReadMaintenance(ctx)
	if err != nil {
		return false, err
	}
	if maintenance != nil {
		defer maintenance.Release()
	}
	deadline := time.NewTimer(maintenanceLockTimeout)
	defer deadline.Stop()
	var lock *storeLock
	for {
		lock, err = acquireStoreLock(store.lockPath)
		if !errors.Is(err, ErrConcurrentUpdate) {
			break
		}
		select {
		case <-deadline.C:
			return false, &AuthorityLockTimeoutError{Timeout: maintenanceLockTimeout}
		case <-time.After(10 * time.Millisecond):
		}
	}
	if err != nil {
		return false, err
	}
	defer lock.release()
	record, err := store.loadCompactRecordLocked()
	if err != nil {
		return false, err
	}
	if record.HistoricalCompat {
		return false, NewLegacyReadOnlyError("review/capture-validation", record.State.LineageID)
	}
	state := record.State
	if state.State != StateCorrectionRequired || state.ProposedCorrectionLines == nil ||
		state.CapturePhaseRevision != request.ExpectedRevision || state.InitialSnapshot.Identity != request.TargetIdentity {
		return false, errors.New("targeted validator attempt does not match the current correction phase") // refusal:by-design world-action: this structural compact-authority invariant requires a provider code fix; no operator command can safely repair it
	}
	for _, existing := range state.TargetedValidatorAttempts {
		if existing.AttemptDigest == attemptDigest {
			return true, nil
		}
	}
	if len(state.TargetedValidatorAttempts) >= maxCompactTargetedValidatorAttempts {
		return false, ErrCompactTargetedValidatorAttemptsExhausted
	}
	next := cloneCompactStateInitialAtomicStart(state)
	next.TargetedValidatorAttempts = append(next.TargetedValidatorAttempts, CompactTargetedValidatorAttempt{
		CapturePhaseRevision: request.ExpectedRevision, TargetIdentity: request.CorrectionTargetIdentity,
		RequestHash: request.RequestHash, AttemptDigest: attemptDigest, Outcome: compactTargetedValidatorAttemptInconclusive,
	})
	_, payload, err := makeCompactRecord(next)
	if err != nil {
		return false, err
	}
	if err := writeAtomic(store.StatePath(), payload, 0o644); err != nil {
		return false, err
	}
	return false, nil
}
