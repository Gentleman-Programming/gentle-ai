package reviewtransaction

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
)

// ErrCapturedReviewerResultSlotConflict reports an immutable reviewer result
// slot occupied by different canonical bytes.
var ErrCapturedReviewerResultSlotConflict = errors.New("captured reviewer result slot conflicts with different canonical bytes") // refusal:by-design world-action: transaction-layer capture cannot alter an immutable occupied slot

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
	if record.Revision != expectedRevision || state.State != StateReviewing || state.InitialSnapshot.Identity != targetIdentity ||
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
	path := filepath.Join(store.Dir, CompactReviewerResultsDir, fmt.Sprintf("%02d-%s.json", subject.SelectedOrder, subject.Lens))
	if _, err := os.Lstat(path); errors.Is(err, fs.ErrNotExist) {
		return LensResult{}, false, nil
	} else if err != nil {
		return LensResult{}, false, err
	}
	payload, _, err := readCompactReviewerArtifact(path)
	if err != nil {
		return LensResult{}, false, err
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	var envelope compactAdmittedReviewerResult
	if err := decoder.Decode(&envelope); err != nil {
		return LensResult{}, false, fmt.Errorf("decode admitted reviewer result: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF || envelope.Schema != admittedReviewerResultSchemaForSubject(expected) || envelope.Subject != expected || envelope.Admission.Validate(expected) != nil || len(envelope.Result) == 0 || expectedAdmission != nil && !reflect.DeepEqual(envelope.Admission, *expectedAdmission) {
		// refusal:by-design world-action: only an exact provider-admitted artifact can satisfy this immutable slot; conflicting bytes must remain refused
		return LensResult{}, false, errors.New("admitted reviewer result does not match repository authority")
	}
	result, found = reAdmitCompactReviewerResult(ctx, envelope, expected, nativeFrozen)
	if !found {
		return LensResult{}, false, errors.New("admitted reviewer result failed native re-admission")
	}
	return result, true, nil
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
		return CompactAdmittedReviewerCapture{}, errors.New(
			"capture admitted reviewer result requires an exact revision and target",
		)
	}
	if err := ValidateArtifactSubject(request.ArtifactSubject); err != nil {
		return CompactAdmittedReviewerCapture{}, err
	}
	if request.ArtifactSubject.AuthorityRevision != request.ExpectedRevision ||
		request.Result.Lens != request.ArtifactSubject.Lens {
		return CompactAdmittedReviewerCapture{}, errors.New(
			"reviewer result does not bind the requested authority lens",
		)
	}
	if len(request.RawPayload) == 0 ||
		len(request.RawPayload) > compactReviewerResultSizeLimit {
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

	var published CompactReviewerResultSlot
	err = store.captureReviewerResult(
		request.ExpectedRevision,
		request.TargetIdentity,
		request.ArtifactSubject.Lens,
		request.ArtifactSubject.SelectedOrder,
		func(state CompactState) error {
			if err := ctx.Err(); err != nil {
				return err
			}
			builder := SnapshotBuilder{
				Repo: store.repo,
			}
			nativeContext, err := builder.FrozenCandidateContext(ctx, state.InitialSnapshot)
			if err != nil {
				return err
			}
			nativeContext, expected, err := artifactSubjectForSchema(
				ctx, builder, state, request.ExpectedRevision, nativeContext, request.ArtifactSubject.Lens,
				request.ArtifactSubject.SelectedOrder, request.ArtifactSubject.CorrectionTargetIdentity, request.ArtifactSubject.Schema,
			)
			if err != nil {
				return err
			}
			if !reflect.DeepEqual(nativeContext, request.FrozenContext) {
				return errors.New(
					"reviewer frozen context does not match repository authority",
				)
			}
			if expected != request.ArtifactSubject {
				return errors.New(
					"reviewer artifact subject does not match the locked authority",
				)
			}
			_, admission, err := AdmitArtifact(
				ctx,
				ArtifactAdmissionRequest{
					ExpectedSubject:           expected,
					FrozenContext:             nativeContext,
					EchoedSubjectHash:         providerResult.SubjectHash,
					Inspection:                canonicalInspection,
					Result:                    canonicalResult,
					CandidateCausalFindingIDs: request.CandidateCausalFindingIDs,
					RawPayload:                request.RawPayload,
					CanonicalPayload:          canonicalPayload,
				},
			)
			if err != nil {
				return err
			}
			envelopePayload, err := json.Marshal(compactAdmittedReviewerResult{
				Schema:    admittedReviewerResultSchemaForSubject(expected),
				Subject:   expected,
				Admission: admission,
				Result: append(
					json.RawMessage(nil),
					canonicalPayload[:len(canonicalPayload)-1]...,
				),
			})
			if err != nil {
				return err
			}
			envelopePayload = append(envelopePayload, '\n')
			if len(envelopePayload) > compactReviewerResultSizeLimit {
				return errors.New(
					"admitted reviewer result exceeds the native size bound",
				)
			}
			if request.PreparePublication != nil {
				if err := request.PreparePublication(state); err != nil {
					return err
				}
			}
			published, err = publishCompactAdmittedReviewerResult(
				store.Dir,
				expected,
				envelopePayload,
			)
			return err
		},
	)
	if err != nil {
		if errors.Is(err, ErrAuthorityLockTimeout) {
			_, expectedAdmission, admissionErr := AdmitArtifact(ctx, ArtifactAdmissionRequest{
				ExpectedSubject: request.ArtifactSubject, FrozenContext: request.FrozenContext,
				EchoedSubjectHash: request.ArtifactSubject.SubjectHash, Inspection: canonicalInspection,
				Result: request.Result, CandidateCausalFindingIDs: request.CandidateCausalFindingIDs,
				RawPayload: request.RawPayload, CanonicalPayload: canonicalPayload,
			})
			if admissionErr == nil {
				_, found, replayErr := store.resolveAdmittedReviewerResult(
					ctx, request.ExpectedRevision, request.TargetIdentity, request.FrozenContext,
					request.ArtifactSubject, &expectedAdmission,
				)
				if replayErr == nil && found {
					published, replayErr = ReadCompactReviewerResultSlot(store.Dir, request.ArtifactSubject.SelectedOrder, request.ArtifactSubject.Lens)
					if replayErr == nil {
						return compactAdmittedReviewerCaptureFromSlot(ctx, published, request.FrozenContext, request.ArtifactSubject)
					}
				}
			}
		}
		return CompactAdmittedReviewerCapture{}, err
	}
	return compactAdmittedReviewerCaptureFromSlot(ctx, published, request.FrozenContext, request.ArtifactSubject)
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
	decoder := json.NewDecoder(bytes.NewReader(slot.Payload))
	decoder.DisallowUnknownFields()
	var envelope compactAdmittedReviewerResult
	if err := decoder.Decode(&envelope); err != nil {
		return CompactAdmittedReviewerCapture{}, fmt.Errorf("decode admitted reviewer result readback: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF || envelope.Schema != admittedReviewerResultSchemaForSubject(subject) ||
		envelope.Subject != subject || envelope.Admission.Validate(subject) != nil {
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

func publishCompactAdmittedReviewerResult(
	storeDir string,
	subject ArtifactSubject,
	payload []byte,
) (CompactReviewerResultSlot, error) {
	return publishCompactRoleResultSlot(storeDir, compactLensRoleResultSlotKey(subject.SelectedOrder, subject.Lens), payload)
}

func requireCompactReviewerSlotCompatible(
	path string,
	payload []byte,
	limit int,
) error {
	existing, err := readPrivateCompactReviewerFile(path, int64(limit))
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !bytes.Equal(existing, payload) {
		return fmt.Errorf("%w: existing bytes differ from the requested payload", ErrCapturedReviewerResultSlotConflict)
	}
	return nil
}

func publishPrivateCompactReviewerFile(
	path string,
	payload []byte,
	limit int,
) error {
	if len(payload) == 0 || len(payload) > limit {
		return errors.New("reviewer artifact payload size is invalid")
	}
	if err := requireCompactReviewerSlotCompatible(path, payload, limit); err != nil {
		return err
	}
	if err := publishPrivateRARImmutable(path, payload); err != nil {
		return err
	}
	readBack, err := readPrivateCompactReviewerFile(path, int64(limit))
	if err != nil {
		return err
	}
	if !bytes.Equal(readBack, payload) {
		return errors.New("reviewer artifact immutable publication mismatch")
	}
	return nil
}

func readPrivateCompactReviewerFile(path string, limit int64) ([]byte, error) {
	before, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if rarPathUnsafe(path, before) ||
		!before.Mode().IsRegular() ||
		!privateRARPathSafe(path, before) ||
		!compactReviewerFileSingleLink(nil, before) {
		return nil, errUnsafeRARAuthorityPath
	}
	file, err := openRARPathNoFollow(path, false)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil ||
		!opened.Mode().IsRegular() ||
		!os.SameFile(before, opened) ||
		!privateOpenRARPathSafe(file, opened) ||
		!compactReviewerFileSingleLink(file, opened) {
		if err != nil {
			return nil, err
		}
		return nil, errRARAuthorityPathReplaced
	}
	if opened.Size() < 1 || opened.Size() > limit {
		return nil, errors.New(
			"reviewer artifact is unreadable or outside the native size bound",
		)
	}
	payload, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil || int64(len(payload)) > limit {
		return nil, errors.New(
			"reviewer artifact is unreadable or outside the native size bound",
		)
	}
	afterOpen, err := file.Stat()
	if err != nil {
		return nil, err
	}
	current, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !os.SameFile(opened, afterOpen) ||
		!os.SameFile(afterOpen, current) ||
		afterOpen.Size() != int64(len(payload)) ||
		!privateOpenRARPathSafe(file, afterOpen) ||
		!compactReviewerFileSingleLink(file, afterOpen) {
		return nil, errRARAuthorityPathReplaced
	}
	return payload, nil
}

// ReviewerAttemptRecord represents non-admitted attempt evidence.
type ReviewerAttemptRecord struct {
	Schema            string            `json:"schema"` // "gentle-ai.review-attempt-record/v1"
	LineageID         string            `json:"lineage_id"`
	TargetIdentity    string            `json:"target_identity"`
	AuthorityRevision string            `json:"authority_revision"`
	Lens              string            `json:"lens"`
	SelectedOrder     int               `json:"selected_order"`
	SubjectHash       string            `json:"subject_hash"`
	AttemptIndex      int               `json:"attempt_index"`
	Admission         ArtifactAdmission `json:"admission"`
	RawSHA256         string            `json:"raw_sha256"`
	CanonicalSHA256   string            `json:"canonical_sha256"`
}

type CaptureReviewerAttemptRequest struct {
	StoreDir          string
	LineageID         string
	TargetIdentity    string
	AuthorityRevision string
	Lens              string
	SelectedOrder     int
	SubjectHash       string
	Admission         ArtifactAdmission
	RawPayload        []byte
	CanonicalPayload  []byte
}

// CaptureUnachievableReviewerAttempt persists a non-admitted reviewer attempt record.
func (store CompactStore) CaptureUnachievableReviewerAttempt(ctx context.Context, req CaptureReviewerAttemptRequest) (ReviewerAttemptRecord, error) {
	if ctx == nil {
		return ReviewerAttemptRecord{}, errors.New("capture unachievable attempt context is nil")
	}
	if err := ctx.Err(); err != nil {
		return ReviewerAttemptRecord{}, err
	}
	storeDir := req.StoreDir
	if storeDir == "" {
		storeDir = store.Dir
	}
	if !validSHA256(req.AuthorityRevision) || !validSHA256(req.TargetIdentity) {
		return ReviewerAttemptRecord{}, errors.New("capture unachievable attempt requires exact revision and target")
	}
	if req.SelectedOrder < 0 || !isSupportedLens(req.Lens) {
		return ReviewerAttemptRecord{}, errors.New("capture unachievable attempt requires valid lens and order")
	}
	if len(req.RawPayload) == 0 || len(req.RawPayload) > compactReviewerResultSizeLimit ||
		len(req.CanonicalPayload) == 0 || len(req.CanonicalPayload) > compactReviewerResultSizeLimit {
		return ReviewerAttemptRecord{}, errors.New("attempt payload is empty or exceeds size limit")
	}
	if req.Admission.Decision != ArtifactAdmissionUnachievable {
		return ReviewerAttemptRecord{}, errors.New("capture unachievable attempt requires unachievable admission decision")
	}

	attemptsDir := filepath.Join(storeDir, CompactReviewerAttemptsDir)
	if err := createCompactRoleResultDirectories(storeDir, attemptsDir); err != nil {
		return ReviewerAttemptRecord{}, fmt.Errorf("create reviewer attempts directory: %w", err)
	}
	if err := SyncReviewDirectory(storeDir); err != nil {
		return ReviewerAttemptRecord{}, fmt.Errorf("sync reviewer attempts parent directory: %w", err)
	}

	existing, err := ReadCompactReviewerAttempts(storeDir, req.SelectedOrder, req.Lens)
	if err != nil {
		return ReviewerAttemptRecord{}, err
	}
	attemptIndex := len(existing) + 1

	rawSHA := payloadSHA256(req.RawPayload)
	canonicalSHA := payloadSHA256(req.CanonicalPayload)

	admission := req.Admission
	admission.Schema = ArtifactAdmissionSchema
	admission.Decision = ArtifactAdmissionUnachievable
	admission.SubjectHash = req.SubjectHash
	admission.RawSHA256 = rawSHA
	admission.CanonicalSHA256 = canonicalSHA

	record := ReviewerAttemptRecord{
		Schema:            ReviewerAttemptRecordSchema,
		LineageID:         req.LineageID,
		TargetIdentity:    req.TargetIdentity,
		AuthorityRevision: req.AuthorityRevision,
		Lens:              req.Lens,
		SelectedOrder:     req.SelectedOrder,
		SubjectHash:       req.SubjectHash,
		AttemptIndex:      attemptIndex,
		Admission:         admission,
		RawSHA256:         rawSHA,
		CanonicalSHA256:   canonicalSHA,
	}

	payload, err := json.Marshal(record)
	if err != nil {
		return ReviewerAttemptRecord{}, err
	}
	payload = append(payload, '\n')
	digestPayload := []byte(compactPreservedPayloadDigest(payload) + "\n")

	fileName := fmt.Sprintf("%02d-%s-%02d.json", req.SelectedOrder, req.Lens, attemptIndex)
	filePath := filepath.Join(attemptsDir, fileName)

	if err := requireCompactRoleResultSlotCompatible(filePath, payload); err != nil {
		return ReviewerAttemptRecord{}, err
	}
	if err := requireCompactRoleResultSlotCompatible(filePath+".sha256", digestPayload); err != nil {
		return ReviewerAttemptRecord{}, err
	}
	if err := publishPrivateCompactReviewerFile(filePath, payload, compactReviewerResultSizeLimit); err != nil {
		return ReviewerAttemptRecord{}, fmt.Errorf("publish attempt artifact: %w", err)
	}
	if err := publishPrivateCompactReviewerFile(filePath+".sha256", digestPayload, 256); err != nil {
		return ReviewerAttemptRecord{}, fmt.Errorf("publish attempt digest sidecar: %w", err)
	}

	return record, nil
}

// ReadCompactReviewerAttempts reads all unachievable attempt records for a given slot.
func ReadCompactReviewerAttempts(storeDir string, order int, lens string) ([]ReviewerAttemptRecord, error) {
	if order < 0 || !isSupportedLens(lens) {
		return nil, errors.New("invalid lens or order for reviewer attempts")
	}
	dir := filepath.Join(storeDir, CompactReviewerAttemptsDir)
	if _, err := os.Stat(dir); errors.Is(err, fs.ErrNotExist) {
		return []ReviewerAttemptRecord{}, nil
	} else if err != nil {
		return nil, err
	}
	var records []ReviewerAttemptRecord
	for index := 1; ; index++ {
		filename := fmt.Sprintf("%02d-%s-%02d.json", order, lens, index)
		path := filepath.Join(dir, filename)
		if _, err := os.Lstat(path); errors.Is(err, fs.ErrNotExist) {
			break
		} else if err != nil {
			return nil, err
		}
		payload, _, err := readCompactReviewerArtifact(path)
		if err != nil {
			return nil, fmt.Errorf("read attempt artifact %s: %w", filename, err)
		}
		var record ReviewerAttemptRecord
		decoder := json.NewDecoder(bytes.NewReader(payload))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&record); err != nil {
			return nil, fmt.Errorf("decode attempt record %s: %w", filename, err)
		}
		var extra any
		if err := decoder.Decode(&extra); err != io.EOF || record.Schema != ReviewerAttemptRecordSchema ||
			record.SelectedOrder != order || record.Lens != lens || record.AttemptIndex != index {
			return nil, fmt.Errorf("attempt record %s does not match expected schema or index", filename)
		}
		records = append(records, record)
	}
	return records, nil
}

// DiscoverReviewerSlotAttempts groups all matching attempt records by selected lens slot order.
func DiscoverReviewerSlotAttempts(storeDir string, state CompactState, revision string) (map[int][]ReviewerAttemptRecord, error) {
	result := make(map[int][]ReviewerAttemptRecord)
	for order, lens := range state.SelectedLenses {
		attempts, err := ReadCompactReviewerAttempts(storeDir, order, lens)
		if err != nil {
			return nil, err
		}
		var matching []ReviewerAttemptRecord
		for _, att := range attempts {
			if att.LineageID == state.LineageID && att.AuthorityRevision == revision &&
				att.TargetIdentity == state.InitialSnapshot.Identity && att.Lens == lens && att.SelectedOrder == order {
				matching = append(matching, att)
			}
		}
		if len(matching) > 0 {
			result[order] = matching
		}
	}
	return result, nil
}
