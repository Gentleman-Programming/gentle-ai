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
)

const compactApprovedAcknowledgementTokenBytes = 32

// compactAcknowledgementRandomReader remains a package-local seam so tests can
// prove that random-token failure occurs before any authority mutation.
var compactAcknowledgementRandomReader io.Reader = rand.Reader

// ApprovedCompactAcknowledgement binds the one pending acknowledgement to its
// approved compact authority. The raw token is returned only through the active
// v2 owner and remains solely in the authority until acknowledgement consumes it.
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
// is never visible without the exact replayable continuation that owns token consumption.
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
// names no live authority in this repository. It is deliberately path-free,
// because a caller learns nothing useful from the store layout and a relayed
// error should not carry it.
var ErrApprovedAcknowledgementAuthorityAbsent = errors.New("approved acknowledgement names no live compact authority in this repository") // refusal:by-design operator-knowledge: refresh review status for the repository-bound continuation instead of replaying an absent lineage

// AcknowledgeApprovedCompactAuthority verifies one exact pending acknowledgement
// and consumes only its transient token in a Compact CAS transition. The approved
// authority and its exact candidate binding remain durable for delivery gates.
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
	store, err := CompactAuthoritativeStore(ctx, repo, lineageID)
	if err != nil {
		return err
	}
	record, err := store.LoadContext(ctx)
	if err != nil {
		if _, statErr := os.Lstat(store.StatePath()); errors.Is(err, fs.ErrNotExist) && errors.Is(statErr, fs.ErrNotExist) {
			return ErrApprovedAcknowledgementAuthorityAbsent
		}
		return fmt.Errorf("load compact authority: %w", err)
	}
	if err := validatePendingApprovedCompactAcknowledgement(record, lineageID, targetIdentity, expectedRevision, token); err != nil {
		return err
	}

	next := record.State
	next.ApprovedAckToken = ""
	authorityRoot := filepath.Join(filepath.Dir(store.maintenanceLockPath), "review-transactions")
	_, err = store.replaceContextGuarded(ctx, expectedRevision, "review/acknowledge-approved", next, func() error {
		return ensureNoPreparedCompactBatchReconciliation(authorityRoot)
	})
	return err
}

func validatePendingApprovedCompactAcknowledgement(record CompactRecord, lineageID, targetIdentity, expectedRevision, token string) error {
	if record.Revision != expectedRevision {
		return &CompactRevisionConflictError{LineageID: lineageID, Expected: expectedRevision, Current: record.Revision}
	}
	if record.State.State != StateApproved {
		return fmt.Errorf("approved acknowledgement requires approved compact authority; lineage %q is %q", lineageID, record.State.State) // refusal:by-design operator-knowledge: refresh review status and run only the continuation emitted for an approved authority
	}
	if record.State.CurrentSnapshot.Identity != targetIdentity {
		return errors.New("approved acknowledgement target does not match active compact authority") // refusal:by-design operator-knowledge: use the exact target returned by the pending acknowledgement
	}
	if !validCompactAcknowledgementToken(record.State.ApprovedAckToken) {
		return errors.New("approved compact authority has no valid pending acknowledgement") // refusal:by-design operator-knowledge: refresh review status; an acknowledged approval remains authoritative but has no acknowledgement to replay
	}
	if subtle.ConstantTimeCompare([]byte(record.State.ApprovedAckToken), []byte(token)) != 1 {
		return errors.New("approved acknowledgement token does not match active compact authority") // refusal:by-design operator-knowledge: use the exact opaque token returned by the pending acknowledgement
	}
	return nil
}
