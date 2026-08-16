package reviewtransaction

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewerprovider"
)

const (
	compactRefuterResultsDir           = "refuter-results"
	compactRefuterResultFile           = "batch.json"
	compactTargetedValidatorResultsDir = "targeted-validator-results"
	compactTargetedValidatorResultFile = "result.json"
)

type compactRoleResultSlotKind string

const (
	compactRoleResultSlotLens              compactRoleResultSlotKind = compactRoleResultSlotKind(reviewerprovider.RoleLens)
	compactRoleResultSlotRefuter           compactRoleResultSlotKind = compactRoleResultSlotKind(reviewerprovider.RoleRefuter)
	compactRoleResultSlotTargetedValidator compactRoleResultSlotKind = compactRoleResultSlotKind(reviewerprovider.RoleTargetedValidator)
)

// compactRoleResultSlotKey has typed constructors below. It is intentionally
// not caller-authored input: role-specific admission selects a known immutable
// slot before generic storage sees any bytes.
type compactRoleResultSlotKey struct {
	kind              compactRoleResultSlotKind
	selectedOrder     int
	lens              string
	authorityRevision string
	targetIdentity    string
}

func compactLensRoleResultSlotKey(order int, lens string) compactRoleResultSlotKey {
	return compactRoleResultSlotKey{kind: compactRoleResultSlotLens, selectedOrder: order, lens: lens}
}

func compactRefuterRoleResultSlotKey() compactRoleResultSlotKey {
	return compactRoleResultSlotKey{kind: compactRoleResultSlotRefuter}
}

func compactTargetedValidatorRoleResultSlotKey(revision, target string) compactRoleResultSlotKey {
	return compactRoleResultSlotKey{
		kind: compactRoleResultSlotTargetedValidator, authorityRevision: revision, targetIdentity: target,
	}
}

// CompactRoleResultSlot is the shared immutable representation for all
// provider roles. Typed role wrappers never expose a generic semantic value.
type CompactRoleResultSlot struct {
	Occupied bool
	Payload  []byte
	Digest   string
}

// CompactAdmittedRefuterResultRequest contains Go-admitted bytes for exactly
// one transaction-wide inferential batch. The caller re-derives the request
// under the role capture lock before publication.
type CompactAdmittedRefuterResultRequest struct {
	ExpectedRevision   string
	TargetIdentity     string
	Payload            []byte
	PreparePublication func(CompactState) error
}

// CompactRefuterResultSlot is the typed readback wrapper for the one refuter
// batch belonging to a reviewing authority.
type CompactRefuterResultSlot = CompactRoleResultSlot

// CompactAdmittedTargetedValidatorResultRequest contains Go-admitted bytes
// for one corrected target. Publication alone never advances correction state.
type CompactAdmittedTargetedValidatorResultRequest struct {
	ExpectedRequest    TargetedValidationRequest
	Payload            []byte
	PreparePublication func(CompactState, TargetedValidationRequest) error
}

// CompactTargetedValidatorResultSlot is the typed readback wrapper for a
// correction-bound validator result.
type CompactTargetedValidatorResultSlot = CompactRoleResultSlot

// CaptureAdmittedRefuterResult captures one complete inferential batch without
// changing compact authority. Exact replay repairs a missing digest sidecar;
// different bytes fail closed.
func (store CompactStore) CaptureAdmittedRefuterResult(ctx context.Context, request CompactAdmittedRefuterResultRequest) error {
	return store.captureAdmittedRoleResult(ctx, "review/capture-refuter-result", request.ExpectedRevision, request.TargetIdentity, request.Payload,
		func(state CompactState) (compactRoleResultSlotKey, error) {
			if state.State != StateReviewing || state.InitialSnapshot.Identity != request.TargetIdentity {
				return compactRoleResultSlotKey{}, errors.New("refuter capture binding does not match the current reviewing authority") // refusal:by-design operator-knowledge: refresh the current reviewing authority before capturing the transaction-wide refuter batch
			}
			return compactRefuterRoleResultSlotKey(), nil
		}, request.PreparePublication)
}

// ReadCompactRefuterResultSlot reads the one typed refuter wrapper.
func ReadCompactRefuterResultSlot(storeDir string) (CompactRefuterResultSlot, error) {
	return ReadCompactRoleResultSlot(storeDir, compactRefuterRoleResultSlotKey())
}

// CaptureAdmittedTargetedValidatorResult captures a validator verdict against
// an open correction without consuming the correction attempt.
func (store CompactStore) CaptureAdmittedTargetedValidatorResult(ctx context.Context, request CompactAdmittedTargetedValidatorResultRequest) error {
	if err := ValidateTargetedValidationRequest(request.ExpectedRequest); err != nil {
		return err
	}
	if request.PreparePublication == nil {
		return errors.New("targeted validator capture requires lock-held native request revalidation") // refusal:by-design world-action: only native revalidation can publish a correction-bound validator result
	}
	return store.captureAdmittedRoleResult(ctx, "review/capture-targeted-validator-result", request.ExpectedRequest.ExpectedRevision, request.ExpectedRequest.CorrectionTargetIdentity, request.Payload,
		func(state CompactState) (compactRoleResultSlotKey, error) {
			authoritative, err := BuildTargetedValidationRequest(ctx, store.repo, state, request.ExpectedRequest.ExpectedRevision)
			if err != nil || !reflect.DeepEqual(authoritative, request.ExpectedRequest) {
				return compactRoleResultSlotKey{}, errors.New("targeted validator capture binding does not match the current open correction authority") // refusal:by-design operator-knowledge: refresh the open correction before capturing its validator result
			}
			return compactTargetedValidatorRoleResultSlotKey(authoritative.ExpectedRevision, authoritative.CorrectionTargetIdentity), nil
		}, func(state CompactState) error {
			authoritative, err := BuildTargetedValidationRequest(ctx, store.repo, state, request.ExpectedRequest.ExpectedRevision)
			if err != nil || !reflect.DeepEqual(authoritative, request.ExpectedRequest) {
				return errors.New("targeted validator capture request changed while awaiting the authority lock") // refusal:by-design operator-knowledge: refresh the exact native validator request before capture
			}
			return request.PreparePublication(state, authoritative)
		})
}

// ReadCompactTargetedValidatorResultSlot reads one correction-bound typed
// validator wrapper without changing correction accounting.
func ReadCompactTargetedValidatorResultSlot(storeDir string, request TargetedValidationRequest) (CompactTargetedValidatorResultSlot, error) {
	if err := ValidateTargetedValidationRequest(request); err != nil {
		return CompactTargetedValidatorResultSlot{}, err
	}
	return ReadCompactRoleResultSlot(storeDir, compactTargetedValidatorRoleResultSlotKey(request.ExpectedRevision, request.CorrectionTargetIdentity))
}

// captureAdmittedRoleResult is the common lock, replay, state predicate, and
// durable publication path used by non-lens role wrappers. Lens capture keeps
// its historical typed admission path but uses the same slot publication code.
func (store CompactStore) captureAdmittedRoleResult(
	ctx context.Context,
	operation, expectedRevision, targetIdentity string,
	payload []byte,
	keyFor func(CompactState) (compactRoleResultSlotKey, error),
	prepare func(CompactState) error,
) error {
	if ctx == nil {
		return errors.New("capture admitted provider role result context is nil") // refusal:by-design world-action: durable role capture requires a live native context
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if !validSHA256(expectedRevision) || !validSHA256(targetIdentity) {
		return errors.New("capture admitted provider role result requires exact revision and target") // refusal:by-design operator-knowledge: refresh the exact current authority before capture
	}
	if len(payload) == 0 || len(payload) > compactReviewerResultSizeLimit {
		return errors.New("admitted provider role result is empty or outside the native size bound") // refusal:by-design world-action: provider output is never truncated to fit immutable storage
	}
	if keyFor == nil || prepare == nil {
		return errors.New("capture admitted provider role result requires native request revalidation") // refusal:by-design world-action: only a freshly re-derived Go request can authorize immutable provider evidence
	}

	publish := func(state CompactState) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		key, err := keyFor(state)
		if err != nil {
			return err
		}
		if err := prepare(state); err != nil {
			return err
		}
		_, err = publishCompactRoleResultSlot(store.Dir, key, payload)
		return err
	}
	err := store.captureRoleResult(expectedRevision, operation, publish)
	if !errors.Is(err, ErrAuthorityLockTimeout) {
		return err
	}

	// A timed-out writer can still be an exact replay that another writer has
	// durably completed. Re-admit the request and read the typed immutable slot.
	record, loadErr := store.LoadContext(ctx)
	if loadErr != nil || record.HistoricalCompat || record.Revision != expectedRevision {
		return err
	}
	key, keyErr := keyFor(record.State)
	if keyErr != nil || prepare(record.State) != nil {
		return err
	}
	slot, readErr := ReadCompactRoleResultSlot(store.Dir, key)
	if readErr == nil && slot.Occupied && bytes.Equal(slot.Payload, payload) {
		return nil
	}
	return err
}

func (store CompactStore) captureRoleResult(expectedRevision, operation string, publish func(CompactState) error) error {
	deadline := time.NewTimer(maintenanceLockTimeout)
	defer deadline.Stop()
	var lock *storeLock
	var err error
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
		return NewLegacyReadOnlyError(operation, record.State.LineageID)
	}
	if record.Revision != expectedRevision {
		return errors.New("provider role capture revision does not match current authority") // refusal:by-design operator-knowledge: refresh the provider role request from the current authority
	}
	return publish(record.State)
}

func compactRoleResultSlotPath(storeDir string, key compactRoleResultSlotKey) (string, error) {
	contract, err := reviewerprovider.ContractFor(reviewerprovider.Role(key.kind))
	if err != nil {
		return "", fmt.Errorf("resolve provider role storage: %w", err)
	}
	switch contract.StorageSlot {
	case "selected-lens":
		if key.selectedOrder < 0 || !isSupportedLens(key.lens) {
			return "", errors.New("lens role result slot requires a selected order and lens") // refusal:by-design world-action: only frozen selected-lens authority may address this slot
		}
		return filepath.Join(storeDir, CompactReviewerResultsDir, fmt.Sprintf("%02d-%s.json", key.selectedOrder, key.lens)), nil
	case "transaction-refuter-batch":
		return filepath.Join(storeDir, compactRefuterResultsDir, compactRefuterResultFile), nil
	case "correction-targeted-validator":
		if !validSHA256(key.authorityRevision) || !validSHA256(key.targetIdentity) {
			return "", errors.New("targeted validator role result identity is invalid") // refusal:by-design world-action: only a current correction authority can define a validator slot
		}
		return filepath.Join(storeDir, compactTargetedValidatorResultsDir, strings.TrimPrefix(key.targetIdentity, "sha256:"),
			strings.TrimPrefix(key.authorityRevision, "sha256:"), compactTargetedValidatorResultFile), nil
	default:
		return "", fmt.Errorf("provider role %q has unsupported storage binding %q", contract.Role, contract.StorageSlot) // refusal:by-design world-action: every role must have a compiled storage binding
	}
}

// ReadCompactRoleResultSlot verifies immutable payload and digest sidecars for
// every role. A partial pair is never replayed as a result.
func ReadCompactRoleResultSlot(storeDir string, key compactRoleResultSlotKey) (CompactRoleResultSlot, error) {
	path, err := compactRoleResultSlotPath(storeDir, key)
	if err != nil {
		return CompactRoleResultSlot{}, err
	}
	payload, payloadErr := readCompactReviewerResultSlotFile(path, compactReviewerResultSizeLimit)
	digestPayload, digestErr := readCompactReviewerResultSlotFile(path+".sha256", 256)
	payloadMissing := errors.Is(payloadErr, fs.ErrNotExist)
	digestMissing := errors.Is(digestErr, fs.ErrNotExist)
	if payloadErr != nil && !payloadMissing {
		return CompactRoleResultSlot{}, fmt.Errorf("read provider role result payload: %w", payloadErr)
	}
	if digestErr != nil && !digestMissing {
		return CompactRoleResultSlot{}, fmt.Errorf("read provider role result digest: %w", digestErr)
	}
	if payloadMissing && digestMissing {
		return CompactRoleResultSlot{}, nil
	}
	if payloadMissing || digestMissing {
		return CompactRoleResultSlot{}, errors.New("provider role result slot is partially published") // refusal:by-design human-authority: interrupted immutable evidence requires maintainer inspection
	}
	digest := strings.TrimSpace(string(digestPayload))
	if !validSHA256(digest) || compactPreservedPayloadDigest(payload) != digest {
		return CompactRoleResultSlot{}, errors.New("provider role result digest does not match its bytes") // refusal:by-design human-authority: mismatched immutable evidence requires maintainer inspection
	}
	return CompactRoleResultSlot{Occupied: true, Payload: append([]byte(nil), payload...), Digest: digest}, nil
}

// publishCompactRoleResultSlot is the one durable publication path for all
// provider role results. Existing equal bytes replay; conflicting bytes leave
// the occupied slot untouched.
func publishCompactRoleResultSlot(storeDir string, key compactRoleResultSlotKey, payload []byte) (CompactRoleResultSlot, error) {
	path, err := compactRoleResultSlotPath(storeDir, key)
	if err != nil {
		return CompactRoleResultSlot{}, err
	}
	if len(payload) == 0 || len(payload) > compactReviewerResultSizeLimit {
		return CompactRoleResultSlot{}, errors.New("provider role result is empty or outside the native size bound") // refusal:by-design world-action: provider results are never truncated to fit immutable storage
	}
	if err := createCompactRoleResultDirectories(storeDir, filepath.Dir(path)); err != nil {
		return CompactRoleResultSlot{}, fmt.Errorf("create provider role result directory: %w", err)
	}
	if err := SyncReviewDirectory(storeDir); err != nil {
		return CompactRoleResultSlot{}, fmt.Errorf("sync provider role result parent directory: %w", err)
	}
	digestPayload := []byte(compactPreservedPayloadDigest(payload) + "\n")
	if err := requireCompactRoleResultSlotCompatible(path, payload); err != nil {
		return CompactRoleResultSlot{}, err
	}
	if err := requireCompactRoleResultSlotCompatible(path+".sha256", digestPayload); err != nil {
		return CompactRoleResultSlot{}, err
	}
	if err := publishPrivateCompactReviewerFile(path, payload, compactReviewerResultSizeLimit); err != nil {
		return CompactRoleResultSlot{}, fmt.Errorf("publish provider role result: %w", err)
	}
	if err := publishPrivateCompactReviewerFile(path+".sha256", digestPayload, 256); err != nil {
		return CompactRoleResultSlot{}, fmt.Errorf("publish provider role result digest: %w", err)
	}
	readBack, err := ReadCompactRoleResultSlot(storeDir, key)
	if err != nil {
		return CompactRoleResultSlot{}, fmt.Errorf("read back provider role result: %w", err)
	}
	if !readBack.Occupied || !bytes.Equal(readBack.Payload, payload) || readBack.Digest != compactPreservedPayloadDigest(payload) {
		return CompactRoleResultSlot{}, errors.New("provider role result readback mismatch") // refusal:by-design human-authority: immutable evidence storage failed readback verification
	}
	return readBack, nil
}

func createCompactRoleResultDirectories(storeDir, directory string) error {
	relative, err := filepath.Rel(storeDir, directory)
	if err != nil || relative == "." || filepath.IsAbs(relative) || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || relative == ".." {
		return errors.New("provider role result directory escapes compact store") // refusal:by-design world-action: compiled role keys must remain under compact authority
	}
	current := storeDir
	for _, part := range strings.Split(relative, string(filepath.Separator)) {
		current = filepath.Join(current, part)
		if _, err := createPrivateRARDirectory(current); err != nil {
			return err
		}
	}
	return nil
}

func requireCompactRoleResultSlotCompatible(path string, payload []byte) error {
	existing, err := readCompactReviewerResultSlotFile(path, compactReviewerResultSizeLimit)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !bytes.Equal(existing, payload) {
		return fmt.Errorf("%w: existing bytes differ from requested provider role result", ErrCapturedReviewerResultSlotConflict)
	}
	return nil
}
