package reviewtransaction

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sort"
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

// ReviewAuthorityBurnIncompleteError reports a deletion that moved the lineage
// out of its authoritative path but left non-authoritative staging residue.
type ReviewAuthorityBurnIncompleteError struct {
	LineageID string
	Residue   []string
	Cause     error
}

func (err *ReviewAuthorityBurnIncompleteError) Error() string {
	return fmt.Sprintf("review authority burn for lineage %q is incomplete: non-authoritative residue remains at %s: %v", err.LineageID, strings.Join(err.Residue, ", "), err.Cause)
}

func (err *ReviewAuthorityBurnIncompleteError) Unwrap() error { return err.Cause }

// BurnApprovedCompactAuthority irreversibly removes one approved compact-v2
// lineage and its exact delivery artifacts. It never removes candidate views or
// another lineage.
func BurnApprovedCompactAuthority(ctx context.Context, repo, lineageID, expectedRevision string) error {
	return burnCompactAuthority(ctx, repo, lineageID, expectedRevision, StateApproved, nil)
}

func burnCompactAuthority(ctx context.Context, repo, lineageID, expectedRevision string, required State, revalidate func(CompactState) error) error {
	return burnExactLineage(ctx, repo, lineageID, expectedRevision, func(base string) error {
		store := CompactStore{Dir: filepath.Join(base, "v2", lineageID), lineageID: lineageID}
		record, err := store.loadCompactRecordLocked()
		if err != nil {
			return fmt.Errorf("load compact authority: %w", err)
		}
		if record.Revision != expectedRevision {
			return &CompactRevisionConflictError{LineageID: lineageID, Expected: expectedRevision, Current: record.Revision}
		}
		if record.State.State != required {
			return &ReviewAuthorityBurnStateError{LineageID: lineageID, Version: "v2", State: string(record.State.State), Required: string(required)}
		}
		if revalidate != nil {
			if err := revalidate(record.State); err != nil {
				return fmt.Errorf("revalidate stale compact review target: %w", err)
			}
		}
		return nil
	})
}

// burnExactLineage is the compact-only exact-path deletion seam. It owns no
// caller-supplied paths: every source is derived from the validated lineage.
func burnExactLineage(ctx context.Context, repo, lineageID, expectedRevision string, validate func(string) error) error {
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
	completed := false
	if err := recoverCompactBurnStaging(base, lineageID, expectedRevision, &completed); err != nil || completed {
		return err
	}
	if err := validate(base); err != nil {
		return err
	}
	return burnExactLineagePaths(base, lineageID, expectedRevision)
}

type lineageBurnMove struct{ source, staged string }

var compactBurnStagingNames = []string{"authority", "effect-marker", "incident"}

func compactBurnTargets(base, lineageID string) []string {
	return []string{filepath.Join(base, "v2", lineageID), filepath.Join(base, "effect-markers", "v1", lineageID), filepath.Join(base, "incidents", lineageID)}
}

func compactBurnPaths(base, lineageID string) ([]string, []string) {
	return compactBurnTargets(base, lineageID), compactBurnStagingNames
}

func compactBurnStagingPath(base, lineageID, expectedRevision string) string {
	digest := sha256.Sum256([]byte(expectedRevision))
	return filepath.Join(base, fmt.Sprintf(".review-burn-v2-%s-%x-prepared", lineageID, digest))
}

// discoverCompactBurnStagedStores observes approved compact authorities that are
// between their live location and irrevocable deletion. It is deliberately
// read-only: STATUS needs to route an exact FINALIZE retry without restoring or
// deleting staged bytes itself. Every accepted stage is complete enough to burn
// immediately, while every ambiguous shape stays an error rather than falling
// through as a fresh target.
func discoverCompactBurnStagedStoresForLineage(ctx context.Context, repo, requestedLineageID string) ([]CompactStore, error) {
	base, root, err := reviewAuthorityRoot(ctx, repo)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(base)
	if errors.Is(err, fs.ErrNotExist) {
		return []CompactStore{}, nil
	}
	if err != nil {
		return nil, err
	}
	stores := make([]CompactStore, 0)
	seen := make(map[string]string)
	for _, entry := range entries {
		lineageID, revisionDigest, phase, staged := compactBurnStagingIdentity(entry.Name())
		if !staged {
			continue
		}
		if requestedLineageID != "" && lineageID != "" && lineageID != requestedLineageID {
			continue
		}
		if lineageID == "" || revisionDigest == "" || phase == "" {
			return nil, fmt.Errorf("malformed compact burn staging %q", entry.Name()) // refusal:by-design world-action: malformed burn staging cannot prove an authoritative recovery path; maintainers must repair the authority store
		}
		if prior, duplicate := seen[lineageID]; duplicate {
			return nil, fmt.Errorf("compact burn staging for lineage %q has multiple phases: %s and %s", lineageID, prior, phase) // refusal:by-design world-action: competing burn phases cannot prove which bytes are authoritative; maintainers must repair the authority store
		}
		store, err := inspectCompactBurnStagedStore(ctx, base, root, entry.Name(), lineageID, revisionDigest)
		if err != nil {
			return nil, err
		}
		seen[lineageID] = phase
		stores = append(stores, store)
	}
	sort.Slice(stores, func(i, j int) bool { return stores[i].lineageID < stores[j].lineageID })
	return stores, nil
}

// CompactStatusStore returns the compact store ordinary STATUS may inspect for
// one exact lineage. A valid staged approved authority stays read-only at its
// staging path; otherwise the ordinary live v2 store is returned unchanged.
func CompactStatusStore(ctx context.Context, repo, lineageID string) (CompactStore, error) {
	if err := validateLineageID(lineageID); err != nil {
		return CompactStore{}, err
	}
	staged, err := discoverCompactBurnStagedStoresForLineage(ctx, repo, lineageID)
	if err != nil {
		return CompactStore{}, err
	}
	for _, store := range staged {
		if store.lineageID == lineageID {
			return store, nil
		}
	}
	return CompactAuthoritativeStore(ctx, repo, lineageID)
}

// InspectStagedApprovedCompactBurn reads the exact approved compact burn stage
// STATUS may route to FINALIZE. It never restores or deletes stage contents.
func InspectStagedApprovedCompactBurn(ctx context.Context, repo, lineageID string) (CompactRecord, bool, error) {
	if err := validateLineageID(lineageID); err != nil {
		return CompactRecord{}, false, err
	}
	stores, err := discoverCompactBurnStagedStoresForLineage(ctx, repo, lineageID)
	if err != nil {
		return CompactRecord{}, false, err
	}
	for _, store := range stores {
		if store.lineageID != lineageID {
			continue
		}
		record, err := store.LoadContext(ctx)
		if err != nil {
			return CompactRecord{}, false, fmt.Errorf("load staged compact authority: %w", err)
		}
		return record, true, nil
	}
	return CompactRecord{}, false, nil
}

// RecoverStagedApprovedCompactBurn performs the one mutating half of a
// previously observed staged compact burn. Callers must supply the exact lineage
// STATUS bound into its FINALIZE retry; a selector-free operation never guesses
// which staged authority to delete.
func RecoverStagedApprovedCompactBurn(ctx context.Context, repo, lineageID string) (CompactRecord, bool, error) {
	record, found, err := InspectStagedApprovedCompactBurn(ctx, repo, lineageID)
	if err != nil || !found {
		return CompactRecord{}, found, err
	}
	if err := BurnApprovedCompactAuthority(ctx, repo, lineageID, record.Revision); err != nil {
		return CompactRecord{}, false, err
	}
	return record, true, nil
}

func RecoverDeletingCompactBurnWithoutAuthority(ctx context.Context, repo, lineageID, expectedRevision string) (bool, error) {
	if err := validateLineageID(lineageID); err != nil {
		return false, err
	}
	base, _, err := reviewAuthorityRoot(ctx, repo)
	if err != nil {
		return false, err
	}
	deleting := strings.TrimSuffix(compactBurnStagingPath(base, lineageID, expectedRevision), "-prepared") + "-deleting"
	info, err := os.Lstat(deleting)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !info.IsDir() || info.Mode()&fs.ModeSymlink != 0 {
		return false, nil
	}
	_, names := compactBurnPaths(base, lineageID)
	entries, err := os.ReadDir(deleting)
	if err != nil {
		return false, err
	}
	for _, entry := range entries {
		if !slices.Contains(names, entry.Name()) {
			return false, nil
		}
		entryInfo, statErr := os.Lstat(filepath.Join(deleting, entry.Name()))
		if statErr != nil || !entryInfo.IsDir() || entryInfo.Mode()&fs.ModeSymlink != 0 {
			return false, statErr
		}
		if entry.Name() == "authority" {
			return false, nil
		}
	}
	if err := burnExactLineage(ctx, repo, lineageID, expectedRevision, func(string) error {
		return errors.New("deleting compact burn staging disappeared before recovery") // refusal:by-design world-action: disappeared staged authority bytes cannot be safely reconstructed; maintainers must inspect and repair the authority store
	}); err != nil {
		return false, err
	}
	return true, nil
}

func compactBurnStagingIdentity(name string) (lineageID, revisionDigest, phase string, staged bool) {
	const prefix = ".review-burn-v2-"
	if !strings.HasPrefix(name, prefix) {
		return "", "", "", false
	}
	staged = true
	for _, candidate := range []string{"prepared", "deleting"} {
		suffix := "-" + candidate
		if !strings.HasSuffix(name, suffix) {
			continue
		}
		body := strings.TrimSuffix(strings.TrimPrefix(name, prefix), suffix)
		if len(body) < 66 {
			return "", "", "", true
		}
		separator := len(body) - 65
		if body[separator] != '-' {
			return "", "", "", true
		}
		lineageID = body[:separator]
		revisionDigest = body[separator+1:]
		if validateLineageID(lineageID) != nil || !validSHA256("sha256:"+revisionDigest) {
			return "", "", "", true
		}
		return lineageID, revisionDigest, candidate, true
	}
	return "", "", "", true
}

func inspectCompactBurnStagedStore(ctx context.Context, base, root, stageName, lineageID, revisionDigest string) (CompactStore, error) {
	staging := filepath.Join(base, stageName)
	info, err := os.Lstat(staging)
	if err != nil {
		return CompactStore{}, fmt.Errorf("inspect compact burn staging %q: %w", stageName, err)
	}
	if !info.IsDir() || info.Mode()&fs.ModeSymlink != 0 {
		return CompactStore{}, fmt.Errorf("compact burn staging %q is not a directory", stageName) // refusal:by-design world-action: malformed staging cannot be safely recovered or deleted by an ordinary lifecycle command; maintainers must repair the authority store
	}
	targets, names := compactBurnPaths(base, lineageID)
	allowed := make(map[string]int, len(names))
	for index, name := range names {
		allowed[name] = index
	}
	entries, err := os.ReadDir(staging)
	if err != nil {
		return CompactStore{}, fmt.Errorf("read compact burn staging %q: %w", stageName, err)
	}
	hasAuthority := false
	for _, entry := range entries {
		index, known := allowed[entry.Name()]
		if !known {
			return CompactStore{}, fmt.Errorf("compact burn staging %q has unexpected target %q", stageName, entry.Name()) // refusal:by-design world-action: unknown staged bytes cannot be safely discarded; maintainers must inspect and repair the authority store
		}
		path := filepath.Join(staging, entry.Name())
		entryInfo, statErr := os.Lstat(path)
		if statErr != nil {
			return CompactStore{}, fmt.Errorf("inspect staged compact target %q: %w", entry.Name(), statErr)
		}
		if !entryInfo.IsDir() || entryInfo.Mode()&fs.ModeSymlink != 0 {
			return CompactStore{}, fmt.Errorf("staged compact target %q is not a directory", entry.Name()) // refusal:by-design world-action: malformed staged authority bytes cannot be safely recovered or deleted; maintainers must repair the authority store
		}
		if _, sourceErr := os.Lstat(targets[index]); sourceErr == nil {
			return CompactStore{}, fmt.Errorf("live and staged compact burn target %q conflict", entry.Name()) // refusal:by-design world-action: conflicting live and staged authority bytes cannot be resolved without a maintainer integrity decision
		} else if !errors.Is(sourceErr, fs.ErrNotExist) {
			return CompactStore{}, fmt.Errorf("inspect live compact burn target %q: %w", entry.Name(), sourceErr)
		}
		hasAuthority = hasAuthority || entry.Name() == "authority"
	}
	if !hasAuthority {
		return CompactStore{}, fmt.Errorf("compact burn staging %q is missing authority", stageName) // refusal:by-design world-action: missing authoritative bytes cannot be recreated by an ordinary lifecycle command; maintainers must repair the authority store
	}
	store := CompactStore{
		Dir: filepath.Join(staging, "authority"), lineageID: lineageID, repo: root,
		lockPath: filepath.Join(base, "v2", "LOCK"), maintenanceLockPath: compactMaintenanceLockPath(base),
	}
	record, err := store.LoadContext(ctx)
	if err != nil {
		return CompactStore{}, fmt.Errorf("load staged compact authority: %w", err)
	}
	expectedPrepared := filepath.Base(compactBurnStagingPath(base, lineageID, record.Revision))
	expectedDeleting := strings.TrimSuffix(expectedPrepared, "-prepared") + "-deleting"
	if stageName != expectedPrepared && stageName != expectedDeleting {
		return CompactStore{}, errors.New("compact burn staging revision digest does not match its authority") // refusal:by-design world-action: a staging digest mismatch is an authority-integrity failure that requires maintainer repair
	}
	if record.State.State != StateApproved {
		return CompactStore{}, &ReviewAuthorityBurnStateError{LineageID: lineageID, Version: "v2", State: string(record.State.State), Required: string(StateApproved)}
	}
	receipt, err := inspectCompactTargetReceipt(store, record.State)
	if err != nil {
		return CompactStore{}, fmt.Errorf("inspect staged compact receipt: %w", err)
	}
	if !receipt.published || !receipt.artifact.exists || !bytes.Equal(receipt.artifact.content, receipt.artifact.canonical) {
		return CompactStore{}, errors.New("staged compact authority has no canonical published receipt") // refusal:by-design world-action: a staged authority without its canonical receipt cannot be delivered or safely burned; maintainers must repair the authority store
	}
	return store, nil
}

func recoverCompactBurnStaging(base, lineageID, expectedRevision string, completed *bool) error {
	staging := compactBurnStagingPath(base, lineageID, expectedRevision)
	targets, names := compactBurnPaths(base, lineageID)
	deleting := strings.TrimSuffix(staging, "-prepared") + "-deleting"
	if info, err := os.Lstat(deleting); err == nil {
		if _, conflict := os.Lstat(staging); conflict == nil {
			return burnIncomplete(lineageID, staging, errors.New("prepared and deleting burn phases conflict")) // refusal:by-design world-action: competing phase roots cannot prove which bytes are authoritative
		}
		if !info.IsDir() || info.Mode()&fs.ModeSymlink != 0 {
			return burnIncomplete(lineageID, deleting, errors.New("deleting compact burn staging is not a directory")) // refusal:by-design world-action: recovery must not remove malformed deletion staging
		}
		entries, err := os.ReadDir(deleting)
		if err != nil {
			return burnIncomplete(lineageID, deleting, err)
		}
		for _, entry := range entries {
			if !slices.Contains(names, entry.Name()) {
				return burnIncomplete(lineageID, deleting, fmt.Errorf("unexpected deleting burn target %q", entry.Name())) // refusal:by-design world-action: recovery must not discard unknown staged authority bytes
			}
		}
		if err := completeCompactBurn(lineageID, deleting, targets); err != nil {
			return err
		}
		*completed = true
		return nil
	} else if !errors.Is(err, fs.ErrNotExist) {
		return burnIncomplete(lineageID, deleting, err)
	}
	info, err := os.Lstat(staging)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return burnIncomplete(lineageID, staging, err)
	}
	if !info.IsDir() || info.Mode()&fs.ModeSymlink != 0 {
		return burnIncomplete(lineageID, staging, errors.New("compact burn staging is not a directory")) // refusal:by-design world-action: recovery must not trust malformed on-disk staging
	}
	entries, err := os.ReadDir(staging)
	if err != nil {
		return burnIncomplete(lineageID, staging, err)
	}
	for _, entry := range entries {
		if !slices.Contains(names, entry.Name()) {
			return burnIncomplete(lineageID, staging, fmt.Errorf("unexpected compact burn staging target %q", entry.Name())) // refusal:by-design world-action: recovery must not discard an unknown staged target
		}
	}
	moved := make([]lineageBurnMove, 0, len(targets))
	for index, source := range targets {
		staged := filepath.Join(staging, names[index])
		info, err := os.Lstat(staged)
		if errors.Is(err, fs.ErrNotExist) {
			if index == 0 {
				if _, sourceErr := os.Lstat(source); errors.Is(sourceErr, fs.ErrNotExist) {
					return burnIncomplete(lineageID, staging, errors.New("compact burn staging is missing authority")) // refusal:by-design world-action: recovery cannot recreate missing authority bytes
				} else if sourceErr != nil {
					return burnIncomplete(lineageID, staging, sourceErr)
				}
			}
			continue
		}
		if err != nil {
			return burnIncomplete(lineageID, staging, err)
		}
		if !info.IsDir() || info.Mode()&fs.ModeSymlink != 0 {
			return burnIncomplete(lineageID, staging, fmt.Errorf("staged compact target %q is not a directory", names[index])) // refusal:by-design world-action: recovery must not restore malformed staged targets
		}
		if _, sourceErr := os.Lstat(source); sourceErr == nil {
			return burnIncomplete(lineageID, staging, fmt.Errorf("live and staged compact burn target %q conflict", names[index])) // refusal:by-design world-action: recovery must not overwrite a live target
		} else if !errors.Is(sourceErr, fs.ErrNotExist) {
			return burnIncomplete(lineageID, staging, sourceErr)
		}
		moved = append(moved, lineageBurnMove{source: source, staged: staged})
	}
	if err := validateCompactBurnStaging(staging, lineageID, expectedRevision); err != nil {
		return burnIncomplete(lineageID, staging, err)
	}
	for _, move := range moved {
		if err := storeResetRename(move.staged, move.source); err != nil {
			return burnIncomplete(lineageID, staging, err)
		}
	}
	if err := storeResetRemoveTree(staging); err != nil {
		return burnIncomplete(lineageID, staging, err)
	}
	return nil
}

func validateCompactBurnStaging(staging, lineageID, expectedRevision string) error {
	record, err := (CompactStore{Dir: filepath.Join(staging, "authority"), lineageID: lineageID}).loadCompactRecordLocked()
	if err != nil {
		return err
	}
	if record.Revision != expectedRevision {
		return &CompactRevisionConflictError{LineageID: lineageID, Expected: expectedRevision, Current: record.Revision}
	}
	return nil
}

func burnExactLineagePaths(base, lineageID, expectedRevision string) error {
	targets, names := compactBurnPaths(base, lineageID)
	staging := compactBurnStagingPath(base, lineageID, expectedRevision)
	if err := os.Mkdir(staging, 0o700); err != nil {
		return fmt.Errorf("create lineage burn staging: %w", err)
	}
	moved := make([]lineageBurnMove, 0, len(targets))
	for index, source := range targets {
		if _, err := os.Lstat(source); errors.Is(err, fs.ErrNotExist) {
			continue
		} else if err != nil {
			return burnRollback(lineageID, staging, moved, err)
		}
		destination := filepath.Join(staging, names[index])
		if err := storeResetRename(source, destination); err != nil {
			return burnRollback(lineageID, staging, moved, err)
		}
		moved = append(moved, lineageBurnMove{source: source, staged: destination})
	}
	deleting := strings.TrimSuffix(staging, "-prepared") + "-deleting"
	if err := storeResetRename(staging, deleting); err != nil {
		return burnRollback(lineageID, staging, moved, err)
	}
	return completeCompactBurn(lineageID, deleting, targets)
}

func completeCompactBurn(lineageID, staging string, targets []string) error {
	for _, path := range targets {
		if _, err := os.Lstat(path); !errors.Is(err, fs.ErrNotExist) {
			return burnIncomplete(lineageID, path, err)
		}
	}
	cause := storeResetRemoveTree(staging)
	if _, err := os.Lstat(staging); !errors.Is(err, fs.ErrNotExist) {
		if err == nil {
			err = errors.New("burn staging remains") // refusal:by-design world-action: a supposedly removed authority staging path is not safe to classify as burned
		}
		return burnIncomplete(lineageID, staging, errors.Join(cause, err))
	}
	return nil
}

func burnRollback(lineageID, staging string, moved []lineageBurnMove, cause error) error {
	failures := []error{cause}
	rollbackComplete := true
	for index := len(moved) - 1; index >= 0; index-- {
		if err := storeResetRename(moved[index].staged, moved[index].source); err != nil {
			failures = append(failures, err)
			rollbackComplete = false
		}
	}
	if !rollbackComplete {
		return burnIncomplete(lineageID, staging, errors.Join(failures...))
	}
	if err := storeResetRemoveTree(staging); err != nil && !errors.Is(err, fs.ErrNotExist) {
		failures = append(failures, err)
	}
	return errors.Join(failures...)
}

func burnIncomplete(lineageID, residue string, cause error) error {
	return &ReviewAuthorityBurnIncompleteError{LineageID: lineageID, Residue: []string{residue}, Cause: cause}
}
