package reviewtransaction

import (
	"context"
	"errors"
	"fmt"
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

// BurnApprovedCompactAuthority physically removes one exact approved compact-v2
// authority after the terminal capture committed it. The lock, revision, and
// state checks make this a direct owner-enforced burn; no receipt, journal, or
// staged FINALIZE replay is involved.
func BurnApprovedCompactAuthority(ctx context.Context, repo, lineageID, expectedRevision string) error {
	if err := validateLineageID(lineageID); err != nil {
		return err
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
		return fmt.Errorf("load compact authority: %w", err)
	}
	if record.Revision != expectedRevision {
		return &CompactRevisionConflictError{LineageID: lineageID, Expected: expectedRevision, Current: record.Revision}
	}
	if record.State.State != StateApproved {
		return &ReviewAuthorityBurnStateError{LineageID: lineageID, Version: "v2", State: string(record.State.State), Required: string(StateApproved)}
	}

	// Companion paths are non-authoritative. Remove them first so a failure keeps
	// the approved authority available for maintainer inspection. The authority
	// directory is the final direct deletion and contains every captured result.
	for _, path := range []string{
		filepath.Join(base, "effect-markers", "v1", lineageID),
		filepath.Join(base, "incidents", lineageID),
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
