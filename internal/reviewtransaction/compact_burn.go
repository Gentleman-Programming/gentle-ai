package reviewtransaction

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// ReviewAuthorityBurnStateError reports an exact authority whose state cannot be
// discarded by the requested burn operation.
type ReviewAuthorityBurnStateError struct {
	LineageID string
	Version   string
	State     string
	Required  string
}

func (err *ReviewAuthorityBurnStateError) Error() string {
	return fmt.Sprintf("review authority burn refused for %s lineage %q: state %q is not %q", err.Version, err.LineageID, err.State, err.Required)
}

// ReviewAuthorityBurnIncompleteError reports an exact burn that could not prove
// every owned path absent. It never authorizes a replay or reconstruction.
type ReviewAuthorityBurnIncompleteError struct {
	LineageID string
	Residue   []string
	Cause     error
}

func (err *ReviewAuthorityBurnIncompleteError) Error() string {
	return fmt.Sprintf("review authority burn for lineage %q is incomplete: owned residue remains at %s: %v", err.LineageID, strings.Join(err.Residue, ", "), err.Cause)
}

func (err *ReviewAuthorityBurnIncompleteError) Unwrap() error { return err.Cause }

const compactApprovedAcknowledgementTokenBytes = 32

// compactAcknowledgementRandomReader remains a package-local seam so tests can
// prove that random-token failure occurs before any authority mutation.
var compactAcknowledgementRandomReader io.Reader = rand.Reader

// ApprovedCompactAcknowledgement binds the one pending acknowledgement to its
// approved compact authority. The raw token is returned only through the active
// v2 owner and remains solely in the authority until it burns.
type ApprovedCompactAcknowledgement struct {
	LineageID        string
	TargetIdentity   string
	ExpectedRevision string
	Token            string
}

func validCompactAcknowledgementToken(token string) bool {
	if len(token) != compactApprovedAcknowledgementTokenBytes*2 {
		return false
	}
	decoded, err := hex.DecodeString(token)
	return err == nil && len(decoded) == compactApprovedAcknowledgementTokenBytes && hex.EncodeToString(decoded) == token
}

func newCompactApprovedAcknowledgementToken() (string, error) {
	value := make([]byte, compactApprovedAcknowledgementTokenBytes)
	if _, err := io.ReadFull(compactAcknowledgementRandomReader, value); err != nil {
		return "", fmt.Errorf("read approved acknowledgement randomness: %w", err)
	}
	return hex.EncodeToString(value), nil
}

func approvedCompactAcknowledgementForRecord(record CompactRecord) ApprovedCompactAcknowledgement {
	return ApprovedCompactAcknowledgement{
		LineageID:        record.State.LineageID,
		TargetIdentity:   record.State.CurrentSnapshot.Identity,
		ExpectedRevision: record.Revision,
		Token:            record.State.ApprovedAckToken,
	}
}

// PendingApprovedCompactAcknowledgement reports the committed acknowledgement
// without issuing entropy or mutating authority. It is absent for historical
// approvals that predate the v2 acknowledgement route.
func PendingApprovedCompactAcknowledgement(record CompactRecord) (ApprovedCompactAcknowledgement, bool) {
	if record.State.State != StateApproved || !validCompactAcknowledgementToken(record.State.ApprovedAckToken) {
		return ApprovedCompactAcknowledgement{}, false
	}
	return approvedCompactAcknowledgementForRecord(record), true
}

// CommitApprovedCompactAcknowledgement publishes an approved terminal state and
// its sole pending acknowledgement in one existing Compact CAS transition. The
// token is generated before taking the mutation lock, so a successful approval
// is never visible without the exact replayable continuation that owns its burn.
func CommitApprovedCompactAcknowledgement(ctx context.Context, store CompactStore, expectedRevision, operation string, next CompactState) (ApprovedCompactAcknowledgement, error) {
	if next.State != StateApproved || next.ApprovedAckToken != "" {
		return ApprovedCompactAcknowledgement{}, errors.New("approved acknowledgement commit requires a tokenless approved successor") // refusal:by-design world-action: only the final approved Compact successor may atomically own a fresh acknowledgement token
	}
	token, err := newCompactApprovedAcknowledgementToken()
	if err != nil {
		return ApprovedCompactAcknowledgement{}, err
	}
	next.ApprovedAckToken = token
	revision, err := store.ReplaceContext(ctx, expectedRevision, operation, next)
	if err != nil {
		return ApprovedCompactAcknowledgement{}, err
	}
	return ApprovedCompactAcknowledgement{
		LineageID:        next.LineageID,
		TargetIdentity:   next.CurrentSnapshot.Identity,
		ExpectedRevision: revision,
		Token:            token,
	}, nil
}

// ErrApprovedAcknowledgementAuthorityAbsent reports that the acknowledgement
// names no live authority in this repository. The ordinary way to reach it is a
// replay: the caller's own earlier acknowledgement already burned the lineage it
// names. It is deliberately path-free, like every other refusal on this surface,
// because a caller learns nothing useful from the store layout and a relayed
// error should not carry it.
var ErrApprovedAcknowledgementAuthorityAbsent = errors.New("approved acknowledgement names no live compact authority; it was already acknowledged and burned, and no further lifecycle operation applies to it") // refusal:by-design operator-knowledge: an acknowledged lineage is terminal, so start a new review instead of replaying its acknowledgement

// AcknowledgeApprovedCompactAuthority verifies one exact pending acknowledgement
// and burns the authority while the existing maintenance and version locks remain
// held. It never writes an acknowledged state, so a failure leaves the pending
// authority replayable and a success leaves no active authority behind.
func AcknowledgeApprovedCompactAuthority(ctx context.Context, repo, lineageID, targetIdentity, expectedRevision, token string) error {
	if err := validateLineageID(lineageID); err != nil {
		return err
	}
	if !validSHA256(targetIdentity) || !validSHA256(expectedRevision) {
		return errors.New("approved acknowledgement requires canonical target and live compact revision") // refusal:by-design operator-knowledge: use the exact target and expected revision returned by the pending acknowledgement
	}
	if !validCompactAcknowledgementToken(token) {
		return errors.New("approved acknowledgement token is malformed") // refusal:by-design operator-knowledge: use the exact opaque token returned by the pending acknowledgement
	}
	base, _, err := reviewAuthorityRoot(ctx, repo)
	if err != nil {
		return err
	}
	lockCtx, cancel := context.WithTimeout(ctx, storeResetLockTimeout)
	defer cancel()
	maintenance, err := storeResetAcquireLease(lockCtx, repo)
	if err != nil {
		return fmt.Errorf("acquire review maintenance lease: %w", err)
	}
	defer maintenance.Release()
	if err := ensureNoPreparedCompactBatchReconciliation(base); err != nil {
		return err
	}
	versionLock, err := acquireLocalStoreLock(filepath.Join(base, "v2", "LOCK"))
	if err != nil {
		return fmt.Errorf("acquire compact authority version lock: %w", err)
	}
	defer versionLock.release()

	store := CompactStore{Dir: filepath.Join(base, "v2", lineageID), lineageID: lineageID}
	record, err := store.loadCompactRecordLocked()
	if err != nil {
		// Narrow on the fact, not on the error class: the absent-authority
		// refusal is only honest when this lineage's state file is the thing
		// that is missing. An ErrNotExist raised anywhere else inside the load
		// keeps its wrapped cause rather than being reported as an already
		// acknowledged lineage.
		if _, statErr := os.Lstat(store.StatePath()); errors.Is(err, fs.ErrNotExist) && errors.Is(statErr, fs.ErrNotExist) {
			return ErrApprovedAcknowledgementAuthorityAbsent
		}
		return fmt.Errorf("load compact authority: %w", err)
	}
	if record.Revision != expectedRevision {
		return &CompactRevisionConflictError{LineageID: lineageID, Expected: expectedRevision, Current: record.Revision}
	}
	if record.State.State != StateApproved {
		return &ReviewAuthorityBurnStateError{LineageID: lineageID, Version: "v2", State: string(record.State.State), Required: string(StateApproved)}
	}
	if record.State.CurrentSnapshot.Identity != targetIdentity {
		return errors.New("approved acknowledgement target does not match active compact authority") // refusal:by-design operator-knowledge: use the exact target returned by the pending acknowledgement
	}
	if !validCompactAcknowledgementToken(record.State.ApprovedAckToken) {
		return errors.New("approved compact authority has no valid pending acknowledgement") // refusal:by-design human-authority: only an authority with its exact pending acknowledgement can be burned by this operation
	}
	if subtle.ConstantTimeCompare([]byte(record.State.ApprovedAckToken), []byte(token)) != 1 {
		return errors.New("approved acknowledgement token does not match active compact authority") // refusal:by-design operator-knowledge: use the exact opaque token returned by the pending acknowledgement
	}
	return burnApprovedCompactAuthorityLocked(base, lineageID, store)
}

func burnApprovedCompactAuthorityLocked(base, lineageID string, store CompactStore) error {
	// The effect marker is non-authoritative. Remove it first so a failure keeps
	// the approved authority available for maintainer inspection. The authority
	// directory is the final direct deletion and contains every captured result.
	for _, path := range []string{
		filepath.Join(base, "effect-markers", "v1", lineageID),
		store.Dir,
	} {
		if err := removeExactCompactBurnPath(lineageID, path); err != nil {
			return err
		}
	}
	return nil
}

func removeExactCompactBurnPath(lineageID, path string) error {
	if err := storeResetRemoveTree(path); err != nil {
		return &ReviewAuthorityBurnIncompleteError{LineageID: lineageID, Residue: []string{path}, Cause: err}
	}
	if _, err := os.Lstat(path); err == nil {
		return &ReviewAuthorityBurnIncompleteError{
			LineageID: lineageID,
			Residue:   []string{path},
			Cause:     errors.New("owned burn path remains after deletion"), // refusal:by-design world-action: a remaining authority path cannot be reported as burned
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return &ReviewAuthorityBurnIncompleteError{LineageID: lineageID, Residue: []string{path}, Cause: err}
	}
	return nil
}
