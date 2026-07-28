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
}

// ResolveAdmittedReviewerResult reads and re-admits one already captured
// reviewer slot without publishing or changing compact authority.
func (store CompactStore) ResolveAdmittedReviewerResult(ctx context.Context, expectedRevision, targetIdentity string, frozen FrozenCandidateContext, subject ArtifactSubject) (result LensResult, found bool, err error) {
	if ctx == nil {
		return LensResult{}, false, errors.New("resolve admitted reviewer result context is nil")
	}
	if err = ctx.Err(); err != nil {
		return LensResult{}, false, err
	}
	if !validSHA256(expectedRevision) || !validSHA256(targetIdentity) || targetIdentity != subject.TargetIdentity || subject.AuthorityRevision != expectedRevision {
		return LensResult{}, false, errors.New("resolve admitted reviewer result requires an exact revision and target")
	}
	err = store.CaptureReviewerResult(expectedRevision, targetIdentity, subject.Lens, subject.SelectedOrder, func(state CompactState) error {
		nativeFrozen, err := (SnapshotBuilder{Repo: store.repo}).FrozenCandidateContext(ctx, state.InitialSnapshot)
		if err != nil {
			return err
		}
		expected, err := NewArtifactSubject(state, expectedRevision, nativeFrozen, subject.Lens, subject.SelectedOrder, subject.CorrectionTargetIdentity)
		if err != nil {
			return err
		}
		if !reflect.DeepEqual(nativeFrozen, frozen) || expected != subject {
			return errors.New("resolved reviewer result does not match repository authority")
		}
		path := filepath.Join(store.Dir, CompactReviewerResultsDir, fmt.Sprintf("%02d-%s.json", subject.SelectedOrder, subject.Lens))
		if _, err := os.Lstat(path); errors.Is(err, fs.ErrNotExist) {
			return nil
		} else if err != nil {
			return err
		}
		payload, _, err := readCompactReviewerArtifact(path)
		if err != nil {
			return err
		}
		decoder := json.NewDecoder(bytes.NewReader(payload))
		decoder.DisallowUnknownFields()
		var envelope compactAdmittedReviewerResult
		if err := decoder.Decode(&envelope); err != nil {
			return fmt.Errorf("decode admitted reviewer result: %w", err)
		}
		var extra any
		if err := decoder.Decode(&extra); err != io.EOF || envelope.Schema != AdmittedReviewerResultSchema || envelope.Subject != expected || envelope.Admission.Validate(expected) != nil || len(envelope.Result) == 0 {
			return errors.New("admitted reviewer result does not match locked authority")
		}
		result, found = reAdmitCompactReviewerResult(envelope, expected, nativeFrozen)
		if !found {
			return errors.New("admitted reviewer result failed native re-admission")
		}
		return nil
	})
	return result, found, err
}

// CaptureAdmittedReviewerResult admits and durably publishes one real reviewer
// result while the compact reviewing authority and selected lens slot remain
// locked to the request's exact revision and target.
func (store CompactStore) CaptureAdmittedReviewerResult(
	ctx context.Context,
	request CompactAdmittedReviewerResultRequest,
) (LensResult, error) {
	if ctx == nil {
		return LensResult{}, errors.New("capture admitted reviewer result context is nil")
	}
	if err := ctx.Err(); err != nil {
		return LensResult{}, err
	}
	if !validSHA256(request.ExpectedRevision) ||
		!validSHA256(request.TargetIdentity) ||
		request.TargetIdentity != request.ArtifactSubject.TargetIdentity {
		return LensResult{}, errors.New(
			"capture admitted reviewer result requires an exact revision and target",
		)
	}
	if err := ValidateArtifactSubject(request.ArtifactSubject); err != nil {
		return LensResult{}, err
	}
	if request.ArtifactSubject.AuthorityRevision != request.ExpectedRevision ||
		request.Result.Lens != request.ArtifactSubject.Lens {
		return LensResult{}, errors.New(
			"reviewer result does not bind the requested authority lens",
		)
	}
	if len(request.RawPayload) == 0 ||
		len(request.RawPayload) > compactReviewerResultSizeLimit {
		return LensResult{}, errors.New(
			"raw reviewer result is empty or outside the native size bound",
		)
	}

	canonicalResult, err := CanonicalCompactLensResult(request.Result)
	if err != nil {
		return LensResult{}, err
	}
	providerResult := compactProviderReviewerResult{
		SubjectHash: request.ArtifactSubject.SubjectHash,
		Inspection:  request.Inspection,
		Lens:        request.ArtifactSubject.Lens,
		Findings:    canonicalResult.Findings,
		Evidence:    canonicalResult.Evidence,
	}
	canonicalPayload, err := json.Marshal(providerResult)
	if err != nil {
		return LensResult{}, err
	}
	canonicalPayload = append(canonicalPayload, '\n')

	var admitted LensResult
	err = store.CaptureReviewerResult(
		request.ExpectedRevision,
		request.TargetIdentity,
		request.ArtifactSubject.Lens,
		request.ArtifactSubject.SelectedOrder,
		func(state CompactState) error {
			if err := ctx.Err(); err != nil {
				return err
			}
			nativeContext, err := (SnapshotBuilder{
				Repo: store.repo,
			}).FrozenCandidateContext(ctx, state.InitialSnapshot)
			if err != nil {
				return err
			}
			if !reflect.DeepEqual(nativeContext, request.FrozenContext) {
				return errors.New(
					"reviewer frozen context does not match repository authority",
				)
			}
			expected, err := NewArtifactSubject(
				state,
				request.ExpectedRevision,
				nativeContext,
				request.ArtifactSubject.Lens,
				request.ArtifactSubject.SelectedOrder,
				request.ArtifactSubject.CorrectionTargetIdentity,
			)
			if err != nil {
				return err
			}
			if expected != request.ArtifactSubject {
				return errors.New(
					"reviewer artifact subject does not match the locked authority",
				)
			}
			admissionResult, admission, err := AdmitArtifact(
				ArtifactAdmissionRequest{
					ExpectedSubject:           expected,
					FrozenContext:             nativeContext,
					EchoedSubjectHash:         providerResult.SubjectHash,
					Inspection:                providerResult.Inspection,
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
				Schema:    AdmittedReviewerResultSchema,
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
			admitted = admissionResult
			return publishCompactAdmittedReviewerResult(
				store.Dir,
				expected,
				envelopePayload,
			)
		},
	)
	if err != nil {
		return LensResult{}, err
	}
	return admitted, nil
}

func publishCompactAdmittedReviewerResult(
	storeDir string,
	subject ArtifactSubject,
	payload []byte,
) error {
	dir := filepath.Join(storeDir, CompactReviewerResultsDir)
	_, err := createPrivateRARDirectory(dir)
	if err != nil {
		return fmt.Errorf("create private reviewer result directory: %w", err)
	}
	// Sync on every attempt rather than only the creating attempt. If a prior
	// parent sync was interrupted, exact replay must repair that durability
	// boundary before publishing either child artifact.
	if err := SyncReviewDirectory(storeDir); err != nil {
		return fmt.Errorf(
			"sync reviewer result parent directory: %w",
			err,
		)
	}
	path := filepath.Join(
		dir,
		fmt.Sprintf("%02d-%s.json", subject.SelectedOrder, subject.Lens),
	)
	digestPayload := []byte(compactPreservedPayloadDigest(payload) + "\n")

	// Preflight both immutable slots so a known sidecar conflict cannot leave a
	// newly published payload without its matching digest.
	if err := requireCompactReviewerSlotCompatible(
		path,
		payload,
		compactReviewerResultSizeLimit,
	); err != nil {
		return err
	}
	if err := requireCompactReviewerSlotCompatible(
		path+".sha256",
		digestPayload,
		256,
	); err != nil {
		return err
	}
	if err := publishPrivateCompactReviewerFile(
		path,
		payload,
		compactReviewerResultSizeLimit,
	); err != nil {
		return fmt.Errorf("publish admitted reviewer result: %w", err)
	}
	if err := publishPrivateCompactReviewerFile(
		path+".sha256",
		digestPayload,
		256,
	); err != nil {
		return fmt.Errorf("publish admitted reviewer result digest: %w", err)
	}
	readBack, digest, err := readCompactReviewerArtifact(path)
	if err != nil {
		return fmt.Errorf("read back admitted reviewer result: %w", err)
	}
	if !bytes.Equal(readBack, payload) ||
		digest != compactPreservedPayloadDigest(payload) {
		return errors.New("admitted reviewer result readback mismatch")
	}
	return nil
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
		return errors.New(
			"captured reviewer result already exists with different canonical bytes",
		)
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
