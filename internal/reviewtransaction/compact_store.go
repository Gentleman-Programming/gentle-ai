package reviewtransaction

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"time"
)

const compactRecordSchema = "gentle-ai.review-state-record/v2"

// Compact store entry artifact names. Every file the compact store writes
// under a lineage directory must be named here so the reclaim authority
// predicate stays in sync with the store layout.
const (
	compactStateFileName           = "review-state.json"
	compactReceiptFileName         = "review-receipt.json"
	compactFinalizeJournalFileName = "finalize-attempt-journal.json"
	// CompactReviewerResultsDir holds captured reviewer result artifacts.
	CompactReviewerResultsDir = "reviewer-results"
)
const CompactTransportSchema = "gentle-ai.review-transport/v2"
const LegacyReadOnlyErrorCode = "legacy_v1_read_only"

var compactStartLockTimeout = 2 * time.Second
var compactStartLockPollInterval = 25 * time.Millisecond

var ErrLegacyReadOnly = errors.New("legacy v1 review lineage is read-only")

// errCompactRecoveryTargetUnchanged identifies the unchanged-target recovery
// anomaly so reconcile-authority can gate quarantine to exactly this class.
var errCompactRecoveryTargetUnchanged = errors.New("escalated recovery successor target has not changed")

// errCompactRecoveryAuthorizationInexact identifies the escalated-recovery
// authorization-binding anomaly so reconcile-authority can gate quarantine of
// historical pre-contract free-form authorizations to exactly this class.
var errCompactRecoveryAuthorizationInexact = errors.New("escalated recovery requires an exact maintainer authorization binding")

// compactRecoveryAuthorizationSchema is the first line of the exact six-line
// escalated-recovery maintainer authorization binding.
const compactRecoveryAuthorizationSchema = "gentle-ai.review-recovery-authorization/v1"

// ErrHistoricalCompatReadOnly denies ordinary mutation of authority loaded
// through the retired-field compatibility path.
var ErrHistoricalCompatReadOnly = errors.New("historical compatibility authority is read-only")

// compactRetiredStateFieldPaths lists dot-separated compact state field paths
// persisted by older builds and removed from the current schema. Each path is
// tolerated only at its exact nesting level: "zero_edit_escalation" at the
// state top level and "recovery.review_start" nested inside the recovery
// provenance object. Historical records that carry them load read-only with
// the retired content dropped from the in-memory view only; the persisted
// bytes, including the retired recovered-start provenance, remain untouched
// on disk because the tolerant read never rewrites authority. New authority
// state never persists these fields.
var compactRetiredStateFieldPaths = map[string]struct{}{
	"zero_edit_escalation":  {},
	"recovery.review_start": {},
}

// LegacyReadOnlyError is the typed ordinary-mutation denial for historical
// legacy-v1 authority. Legacy authority remains available for read-only
// compatibility and explicit maintenance transport operations only.
type LegacyReadOnlyError struct {
	Operation string
	LineageID string
}

func (err *LegacyReadOnlyError) Error() string {
	return fmt.Sprintf("%s: %s for lineage %q", ErrLegacyReadOnly, err.Operation, err.LineageID)
}

func (err *LegacyReadOnlyError) Unwrap() error { return ErrLegacyReadOnly }

func (err *LegacyReadOnlyError) Code() string { return LegacyReadOnlyErrorCode }

func NewLegacyReadOnlyError(operation, lineageID string) error {
	return &LegacyReadOnlyError{Operation: strings.TrimSpace(operation), LineageID: strings.TrimSpace(lineageID)}
}

type CompactRecord struct {
	Schema   string       `json:"schema"`
	Revision string       `json:"revision"`
	State    CompactState `json:"state"`
	// HistoricalCompat marks a record loaded through the retired-field
	// compatibility path; such authority is read-only.
	HistoricalCompat bool `json:"-"`
}

type CompactStore struct {
	Dir                 string
	lineageID           string
	repo                string
	lockPath            string
	maintenanceLockPath string
	TracePath           string
}

type CompactStartAction string

const (
	CompactStartCreated      CompactStartAction = "created"
	CompactStartResumed      CompactStartAction = "resumed"
	CompactStartReuseReceipt CompactStartAction = "reuse-receipt"
	CompactStartBlocked      CompactStartAction = "blocked-scope-action"
	CompactStartRecover      CompactStartAction = "recover"
)

type CompactStartRequest struct {
	State           CompactState
	TracePath       string
	ExplicitLineage bool
}

type CompactStartResult struct {
	Record         CompactRecord
	Action         CompactStartAction
	LensesRequired bool
}

type CompactTraceEntry struct {
	Operation        string `json:"operation"`
	PreviousRevision string `json:"previous_revision,omitempty"`
	Revision         string `json:"revision"`
	State            State  `json:"state"`
	RecordedAt       string `json:"recorded_at"`
}

type CompactTransport struct {
	Schema       string          `json:"schema"`
	Record       CompactRecord   `json:"record"`
	Receipt      *CompactReceipt `json:"receipt,omitempty"`
	BundleDigest string          `json:"bundle_digest"`
}

type CompactRecoveryRequest struct {
	PredecessorLineageID        string
	ExpectedPredecessorRevision string
	Successor                   CompactState
	Disposition                 RecoveryDisposition
	Reason                      string
	Actor                       string
	RecoveredAt                 time.Time
	MaintainerAuthorization     string
}

const ReleaseScopeRecoveryAuthorization = "gentle-ai.release-scope-recovery/v1"

func BuildReleaseScopeSnapshot(ctx context.Context, repo string) (Snapshot, error) {
	builder := SnapshotBuilder{Repo: repo}
	root, err := builder.repositoryRoot(ctx)
	if err != nil {
		return Snapshot{}, err
	}
	commitOutput, err := runGit(ctx, root, nil, nil, "rev-parse", "--verify", "HEAD^{commit}")
	if err != nil {
		return Snapshot{}, err
	}
	commit := strings.TrimSpace(string(commitOutput))
	if _, err := runGit(ctx, root, nil, nil, "rev-parse", "--verify", commit+"^2^{commit}"); err != nil {
		return Snapshot{}, errors.New("release-scope recovery requires HEAD to be a merge commit")
	}
	snapshot, err := (SnapshotBuilder{Repo: root}).Build(ctx, Target{Kind: TargetExactRevision, Revision: commit})
	if err != nil {
		return Snapshot{}, err
	}
	snapshot.Kind = TargetBaseDiff
	snapshot.Identity = snapshotIdentityForProjection(snapshot.Kind, snapshot.Projection, snapshot.BaseTree, snapshot.CandidateTree, snapshot.PathsDigest, snapshot.IntendedUntrackedProof, snapshot.IntendedUntracked, snapshot.LedgerIDs)
	return snapshot, nil
}

func RecoverCompactAuthority(ctx context.Context, repo string, request CompactRecoveryRequest) (CompactRecord, error) {
	predecessorStore, err := CompactAuthoritativeStore(ctx, repo, request.PredecessorLineageID)
	if err != nil {
		return CompactRecord{}, err
	}
	successorStore, err := CompactAuthoritativeStore(ctx, repo, request.Successor.LineageID)
	if err != nil {
		return CompactRecord{}, err
	}
	if request.PredecessorLineageID == request.Successor.LineageID {
		return CompactRecord{}, errors.New("recovery requires a distinct successor lineage")
	}
	lock, err := acquireStoreLock(predecessorStore.lockPath)
	if err != nil {
		return CompactRecord{}, err
	}
	defer lock.release()
	predecessor, err := predecessorStore.Load()
	if err != nil {
		return CompactRecord{}, fmt.Errorf("load recovery predecessor: %w", err)
	}
	if predecessor.Revision != request.ExpectedPredecessorRevision {
		return CompactRecord{}, fmt.Errorf("%w: expected predecessor revision %q, current %q", ErrConcurrentUpdate, request.ExpectedPredecessorRevision, predecessor.Revision)
	}
	if predecessor.State.State == StateCorrectionRequired && request.Disposition != RecoveryEscalated && request.MaintainerAuthorization != compactRecoveryAuthorizationBinding(request.PredecessorLineageID, predecessor.Revision, request.Successor.InitialSnapshot.Identity, request.Actor, request.Reason) {
		return CompactRecord{}, errors.New("correction-required scope recovery requires an exact maintainer authorization binding")
	}
	if !sameRecoveryProjection(predecessor.State.InitialSnapshot.Projection, request.Successor.InitialSnapshot.Projection) && request.Disposition != RecoveryEscalated {
		return CompactRecord{}, errors.New("recovery successor must retain the predecessor projection")
	}
	if !sameRecoveryProjection(predecessor.State.InitialSnapshot.Projection, request.Successor.InitialSnapshot.Projection) &&
		request.MaintainerAuthorization != compactRecoveryAuthorizationBinding(request.PredecessorLineageID, predecessor.Revision, request.Successor.InitialSnapshot.Identity, request.Actor, request.Reason) {
		return CompactRecord{}, compactRecoveryAuthorizationError(request.Successor.InitialSnapshot)
	}
	existing, existingErr := successorStore.Load()
	if existingErr != nil && !os.IsNotExist(existingErr) {
		return CompactRecord{}, existingErr
	}
	if request.RecoveredAt.IsZero() && existingErr == nil && existing.State.Recovery != nil {
		request.RecoveredAt = existing.State.Recovery.RecoveredAt
	}
	if request.RecoveredAt.IsZero() {
		request.RecoveredAt = time.Now().UTC()
	}
	request.Successor.Recovery = &CompactRecoveryProvenance{
		PredecessorLineageID: request.PredecessorLineageID, PredecessorRevision: predecessor.Revision,
		Disposition: request.Disposition, Reason: strings.TrimSpace(request.Reason), Actor: strings.TrimSpace(request.Actor),
		RecoveredAt: request.RecoveredAt.UTC(), MaintainerAuthorization: strings.TrimSpace(request.MaintainerAuthorization),
	}
	stores, err := DiscoverCompactStores(ctx, repo)
	if err != nil {
		return CompactRecord{}, err
	}
	if _, err := CompactAuthorityLeaves(ctx, repo); err != nil {
		return CompactRecord{}, err
	}
	if existingErr == nil {
		if compactStateEqual(existing.State, request.Successor) {
			return existing, nil
		}
		return CompactRecord{}, errors.New("recovery successor lineage already exists with different authority")
	}
	for _, store := range stores {
		record, loadErr := store.Load()
		if loadErr != nil {
			return CompactRecord{}, fmt.Errorf("validate recovery graph: %w", loadErr)
		}
		if record.State.Recovery != nil && record.State.Recovery.PredecessorLineageID == request.PredecessorLineageID {
			return CompactRecord{}, errors.New("recovery predecessor already has successor")
		}
	}
	if err := request.Successor.Validate(); err != nil {
		return CompactRecord{}, err
	}
	if !compactPristineReviewing(request.Successor) || len(request.Successor.CorrectionAttempts) != 0 || request.Successor.CumulativeCorrectionLines != 0 {
		return CompactRecord{}, errors.New("recovery successor must start as a fresh reviewing authority")
	}
	if err := validateCompactRecoveryEdge(predecessor, request.Successor); err != nil {
		return CompactRecord{}, err
	}
	if request.MaintainerAuthorization == ReleaseScopeRecoveryAuthorization {
		if live, liveErr := BuildReleaseScopeSnapshot(ctx, successorStore.repo); liveErr != nil || !snapshotsEqual(live, request.Successor.InitialSnapshot) {
			return CompactRecord{}, fmt.Errorf("%w: live release scope no longer matches successor", ErrInvalidSuccessor)
		}
	}
	if !sameRecoveryProjection(predecessor.State.InitialSnapshot.Projection, request.Successor.InitialSnapshot.Projection) {
		if err := validateLiveRecoverySuccessor(ctx, successorStore.repo, request.Successor.InitialSnapshot); err != nil {
			return CompactRecord{}, fmt.Errorf("%w: repository evidence for selected recovery projection changed: %v", ErrInvalidSuccessor, err)
		}
	}
	if err := validateCompactRepositoryEvidence(ctx, successorStore.repo, nil, request.Successor, "review/start"); err != nil {
		return CompactRecord{}, fmt.Errorf("%w: %v", ErrInvalidSuccessor, err)
	}
	record, payload, err := makeCompactRecord(request.Successor)
	if err != nil {
		return CompactRecord{}, err
	}
	if err := writeAtomic(successorStore.StatePath(), payload, 0o644); err != nil {
		return CompactRecord{}, err
	}
	return record, nil
}

func validateLiveRecoverySuccessor(ctx context.Context, repo string, expected Snapshot) error {
	target := Target{
		Kind: expected.Kind, Projection: expected.Projection, IntendedUntracked: expected.IntendedUntracked,
		LedgerIDs: expected.LedgerIDs,
	}
	if expected.Kind == TargetBaseDiff || expected.Kind == TargetBaseWorkspaceOverlay || expected.Kind == TargetFixDiff {
		target.BaseRef = expected.BaseTree
	}
	live, err := (SnapshotBuilder{Repo: repo}).Build(ctx, target)
	if err != nil {
		return err
	}
	if !snapshotsEqual(live, expected) {
		return errors.New("live target no longer matches the prepared successor")
	}
	return nil
}

func compactRecoveryScopeChanged(previous, next Snapshot) bool {
	return previous.CandidateTree != next.CandidateTree || previous.PathsDigest != next.PathsDigest || previous.Kind == next.Kind && previous.BaseTree != next.BaseTree
}

func compactReleaseScopeRecovery(predecessor CompactState, next Snapshot) bool {
	previous := predecessor.CurrentSnapshot
	if predecessor.InitialSnapshot.Kind != TargetCurrentChanges ||
		(previous.Kind != TargetCurrentChanges && previous.Kind != TargetFixDiff) || next.Kind != TargetBaseDiff ||
		previous.Projection != next.Projection || previous.CandidateTree != next.CandidateTree ||
		len(next.Paths) <= len(predecessor.GenesisPaths) {
		return false
	}
	return pathsAreSubset(predecessor.GenesisPaths, next.Paths) == nil
}

func validateCompactRecoveryEdge(predecessor CompactRecord, successor CompactState) error {
	recovery := successor.Recovery
	if recovery == nil || recovery.PredecessorLineageID != predecessor.State.LineageID {
		return errors.New("recovery successor does not name its predecessor")
	}
	if recovery.PredecessorRevision != predecessor.Revision {
		return errors.New("recovery predecessor revision mismatch")
	}
	if successor.Generation != predecessor.State.Generation+1 {
		return errors.New("recovery successor generation must follow predecessor")
	}
	if !sameRecoveryProjection(predecessor.State.InitialSnapshot.Projection, successor.InitialSnapshot.Projection) && recovery.Disposition != RecoveryEscalated {
		return errors.New("recovery successor must retain the predecessor projection")
	}
	switch recovery.Disposition {
	case RecoveryScopeChanged:
		switch predecessor.State.State {
		case StateApproved:
			previous, next := predecessor.State.CurrentSnapshot, successor.InitialSnapshot
			releaseScope := recovery.MaintainerAuthorization == ReleaseScopeRecoveryAuthorization
			if releaseScope && !compactReleaseScopeRecovery(predecessor.State, next) {
				return errors.New("approved recovery target-kind transition is not a complete release scope expansion")
			}
			if !releaseScope && !compactRecoveryScopeChanged(previous, next) {
				return errors.New("approved predecessor scope has not changed")
			}
		case StateCorrectionRequired:
			if strings.TrimSpace(recovery.MaintainerAuthorization) == "" {
				return errors.New("correction-required scope recovery requires explicit maintainer authorization")
			}
			if !compactRecoveryAddsGenesisPath(predecessor.State, successor.InitialSnapshot) &&
				!compactRecoveryContractsGenesisPaths(predecessor.State, successor.InitialSnapshot) {
				return errors.New("correction-required scope recovery requires repository-derived path expansion or pure genesis-scope contraction")
			}
		default:
			return errors.New("scope-changed recovery requires an approved or correction-required predecessor")
		}
	case RecoveryInvalidated:
		if predecessor.State.State != StateInvalidated {
			return errors.New("recovery requires an invalidated predecessor")
		}
	case RecoveryEscalated:
		historicalFailedValidator := compactHistoricalFailedValidator(predecessor.State)
		if predecessor.State.State != StateEscalated && !historicalFailedValidator {
			return errors.New("recovery requires an escalated predecessor")
		}
		if !compactEscalatedRecoveryTargetChanged(predecessor.State.CurrentSnapshot, successor.InitialSnapshot) {
			return errCompactRecoveryTargetUnchanged
		}
		if recovery.MaintainerAuthorization != compactRecoveryAuthorizationBinding(predecessor.State.LineageID, predecessor.Revision, successor.InitialSnapshot.Identity, recovery.Actor, recovery.Reason) {
			return compactRecoveryAuthorizationError(successor.InitialSnapshot)
		}
	default:
		return errors.New("unsupported recovery disposition")
	}
	return nil
}

func compactEscalatedRecoveryTargetChanged(previous, next Snapshot) bool {
	return previous.CandidateTree != next.CandidateTree && previous.Identity != next.Identity
}

func compactHistoricalFailedValidator(state CompactState) bool {
	if state.State != StateCorrectionRequired || len(state.CorrectionAttempts) == 0 || state.ProposedCorrectionLines != nil || state.ActualCorrectionLines != nil ||
		state.FixDeltaHash != EmptyFixDeltaHash || state.OriginalCriteria != nil || state.CorrectionRegression != nil {
		return false
	}
	last := state.CorrectionAttempts[len(state.CorrectionAttempts)-1]
	return !last.OriginalCriteria.Passed || !last.CorrectionRegression.Passed
}

func compactRecoveryAuthorizationBinding(lineage, revision, targetIdentity, actor, reason string) string {
	return compactRecoveryAuthorizationSchema + "\npredecessor_lineage=" + lineage +
		"\npredecessor_revision=" + revision + "\ntarget_identity=" + targetIdentity +
		"\nactor=" + strings.TrimSpace(actor) + "\nreason=" + strings.TrimSpace(reason)
}

func sameRecoveryProjection(left, right Projection) bool {
	if left == "" {
		left = ProjectionWorkspace
	}
	if right == "" {
		right = ProjectionWorkspace
	}
	return left == right
}

func compactRecoveryAuthorizationError(snapshot Snapshot) error {
	projection := snapshot.Projection
	if projection == "" {
		projection = ProjectionWorkspace
	}
	return fmt.Errorf("%w (projection=%s target_identity=%s)", errCompactRecoveryAuthorizationInexact, projection, snapshot.Identity)
}

func compactRecoveryAddsGenesisPath(predecessor CompactState, live Snapshot) bool {
	paths, pathErr := canonicalPaths(live.Paths)
	genesis, genesisErr := canonicalPaths(predecessor.GenesisPaths)
	if pathErr != nil || genesisErr != nil || !equalStrings(paths, live.Paths) || !equalStrings(genesis, predecessor.GenesisPaths) {
		return false
	}
	known := make(map[string]struct{}, len(genesis))
	for _, path := range genesis {
		known[path] = struct{}{}
	}
	for _, path := range paths {
		if _, exists := known[path]; !exists {
			return true
		}
	}
	return false
}

// compactRecoveryContractsGenesisPaths reports whether the live repository
// scope is a pure contraction of predecessor genesis scope: a non-empty strict
// subset with no live path outside genesis. Disjoint or overlapping-different
// path sets never qualify; they remain governed by the expansion rule.
func compactRecoveryContractsGenesisPaths(predecessor CompactState, live Snapshot) bool {
	paths, pathErr := canonicalPaths(live.Paths)
	genesis, genesisErr := canonicalPaths(predecessor.GenesisPaths)
	if pathErr != nil || genesisErr != nil || !equalStrings(paths, live.Paths) || !equalStrings(genesis, predecessor.GenesisPaths) {
		return false
	}
	if len(paths) == 0 || len(paths) >= len(genesis) {
		return false
	}
	return pathsAreSubset(paths, genesis) == nil
}

func CompactAuthorityLeaves(ctx context.Context, repo string) ([]CompactStore, error) {
	stores, err := DiscoverCompactStores(ctx, repo)
	if err != nil {
		return nil, err
	}
	records := make(map[string]CompactRecord, len(stores))
	storeByLineage := make(map[string]CompactStore, len(stores))
	for _, store := range stores {
		record, loadErr := store.Load()
		if loadErr != nil {
			return nil, fmt.Errorf("invalid compact authority graph: %w", loadErr)
		}
		records[record.State.LineageID], storeByLineage[record.State.LineageID] = record, store
	}
	return compactAuthorityLeaves(records, storeByLineage)
}

func compactAuthorityLeaves(records map[string]CompactRecord, storeByLineage map[string]CompactStore) ([]CompactStore, error) {
	children := make(map[string]int)
	for lineage, record := range records {
		if record.State.Recovery == nil {
			continue
		}
		predecessor, ok := records[record.State.Recovery.PredecessorLineageID]
		if !ok {
			return nil, fmt.Errorf("invalid compact authority graph: dangling predecessor for %q", lineage)
		}
		if predecessor.Revision != record.State.Recovery.PredecessorRevision {
			return nil, fmt.Errorf("invalid compact authority graph: predecessor revision mismatch for %q", lineage)
		}
		if err := validateCompactRecoveryEdge(predecessor, record.State); err != nil {
			return nil, fmt.Errorf("invalid compact authority graph: %w", err)
		}
		children[predecessor.State.LineageID]++
		if children[predecessor.State.LineageID] > 1 {
			return nil, fmt.Errorf("invalid compact authority graph: fork at %q", predecessor.State.LineageID)
		}
		seen := map[string]bool{lineage: true}
		cursor := record
		for cursor.State.Recovery != nil {
			parent := cursor.State.Recovery.PredecessorLineageID
			if seen[parent] {
				return nil, errors.New("invalid compact authority graph: recovery cycle")
			}
			seen[parent] = true
			cursor = records[parent]
		}
	}
	leaves := []CompactStore{}
	for lineage, store := range storeByLineage {
		if children[lineage] == 0 {
			leaves = append(leaves, store)
		}
	}
	sort.Slice(leaves, func(i, j int) bool { return leaves[i].lineageID < leaves[j].lineageID })
	return leaves, nil
}

func CompactLineageSuperseded(ctx context.Context, repo, lineageID string) (bool, error) {
	stores, err := DiscoverCompactStores(ctx, repo)
	if err != nil {
		return false, err
	}
	for _, store := range stores {
		record, loadErr := store.Load()
		if loadErr != nil {
			return false, loadErr
		}
		if record.State.Recovery != nil && record.State.Recovery.PredecessorLineageID == lineageID {
			return true, nil
		}
	}
	return false, nil
}

func CompactAuthoritativeStore(ctx context.Context, repo, lineageID string) (CompactStore, error) {
	if err := validateLineageID(lineageID); err != nil {
		return CompactStore{}, err
	}
	base, root, err := reviewAuthorityRoot(ctx, repo)
	if err != nil {
		return CompactStore{}, err
	}
	versionRoot := filepath.Join(base, "v2")
	dir := filepath.Join(versionRoot, lineageID)
	return CompactStore{Dir: dir, lineageID: lineageID, repo: root, lockPath: filepath.Join(versionRoot, "LOCK"), maintenanceLockPath: compactMaintenanceLockPath(base)}, nil
}

func compactMaintenanceLockPath(authorityRoot string) string {
	return filepath.Join(filepath.Dir(authorityRoot), "REVIEW-MAINTENANCE.lock")
}

// CompactIncidentsDir returns the durable raw-result incident directory for
// one lineage beside the compact authority root. It validates only the
// lineage shape and never requires the lineage to hold authority under repo,
// so incident preservation still works when capture was attempted from a
// repository that does not own the reviewing lineage.
func CompactIncidentsDir(ctx context.Context, repo, lineageID string) (string, error) {
	if err := validateLineageID(lineageID); err != nil {
		return "", err
	}
	base, _, err := reviewAuthorityRoot(ctx, repo)
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "incidents", lineageID), nil
}

func DiscoverCompactStores(ctx context.Context, repo string) ([]CompactStore, error) {
	base, root, err := reviewAuthorityRoot(ctx, repo)
	if err != nil {
		return nil, err
	}
	versionRoot := filepath.Join(base, "v2")
	entries, err := os.ReadDir(versionRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return []CompactStore{}, nil
		}
		return nil, err
	}
	stores := make([]CompactStore, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() || validateLineageID(entry.Name()) != nil {
			continue
		}
		dir := filepath.Join(versionRoot, entry.Name())
		if _, statErr := os.Stat(filepath.Join(dir, compactStateFileName)); os.IsNotExist(statErr) {
			residue, readErr := os.ReadDir(dir)
			unpublished := readErr == nil
			for _, item := range residue {
				unpublished = unpublished && strings.HasPrefix(item.Name(), ".atomic-")
			}
			if unpublished {
				continue
			}
		}
		stores = append(stores, CompactStore{
			Dir: dir, lineageID: entry.Name(), repo: root,
			lockPath: filepath.Join(versionRoot, "LOCK"), maintenanceLockPath: compactMaintenanceLockPath(base),
		})
	}
	sort.Slice(stores, func(i, j int) bool { return stores[i].lineageID < stores[j].lineageID })
	return stores, nil
}

// StartCompactAuthority serializes compact start discovery, equivalence
// decisions, and the initial write under the repository-wide v2 lock.
func StartCompactAuthority(ctx context.Context, repo string, request CompactStartRequest) (CompactStartResult, error) {
	if err := request.State.Validate(); err != nil {
		return CompactStartResult{}, fmt.Errorf("validate compact start: %w", err)
	}
	requestedStore, err := CompactAuthoritativeStore(ctx, repo, request.State.LineageID)
	if err != nil {
		return CompactStartResult{}, err
	}
	lock, err := acquireCompactStartLock(ctx, requestedStore.lockPath)
	if err != nil {
		return CompactStartResult{}, err
	}
	defer lock.release()
	if request.ExplicitLineage {
		record, loadErr := requestedStore.Load()
		if loadErr == nil {
			hasSuccessor, successorErr := explicitCompactSuccessor(ctx, requestedStore.repo, record)
			if successorErr != nil {
				return CompactStartResult{}, fmt.Errorf("validate explicit compact start successor: %w", successorErr)
			}
			if hasSuccessor {
				return CompactStartResult{Record: record, Action: CompactStartBlocked}, nil
			}
			return resumeExplicitCompactStart(ctx, requestedStore, record, request.State)
		}
		if !errors.Is(loadErr, os.ErrNotExist) {
			return CompactStartResult{}, fmt.Errorf("load explicit compact start authority: %w", loadErr)
		}
	}

	stores, err := DiscoverCompactStores(ctx, requestedStore.repo)
	if err != nil {
		return CompactStartResult{}, err
	}
	records := make(map[string]CompactRecord, len(stores))
	storeByLineage := make(map[string]CompactStore, len(stores))
	for _, store := range stores {
		record, loadErr := store.Load()
		if loadErr != nil {
			return CompactStartResult{}, fmt.Errorf("load compact start authority: %w", loadErr)
		}
		records[record.State.LineageID], storeByLineage[record.State.LineageID] = record, store
	}
	leaves, err := compactAuthorityLeaves(records, storeByLineage)
	if err != nil {
		return CompactStartResult{}, err
	}
	claimants := make([]CompactStore, 0, len(leaves))
	recoveryCandidates := make([]CompactStore, 0, 1)
	correctionClaims := make(map[string]compactCorrectionTargetClaim)
	requestedClaims := false
	for _, store := range leaves {
		existing := records[store.lineageID].State
		if existing.State == StateEscalated && compactStartDeliveryScopeMatches(existing, request.State) {
			if compactEscalatedRecoveryTargetChanged(existing.CurrentSnapshot, request.State.InitialSnapshot) {
				recoveryCandidates = append(recoveryCandidates, store)
			} else {
				claimants = append(claimants, store)
				requestedClaims = requestedClaims || store.lineageID == request.State.LineageID
			}
			continue
		}
		if existing.State == StateCorrectionRequired {
			claim := classifyCompactCorrectionTarget(ctx, requestedStore.repo, existing, request.State)
			switch claim {
			case compactCorrectionTargetResume, compactCorrectionTargetBlocked:
				claimants = append(claimants, store)
				correctionClaims[store.lineageID] = claim
				requestedClaims = requestedClaims || store.lineageID == request.State.LineageID
			case compactCorrectionTargetRecover:
				recoveryCandidates = append(recoveryCandidates, store)
			}
			continue
		}
		if compactStartClaimsTarget(ctx, requestedStore.repo, existing, request.State) {
			claimants = append(claimants, store)
			requestedClaims = requestedClaims || store.lineageID == request.State.LineageID
		}
	}
	if len(recoveryCandidates) > 0 {
		if len(recoveryCandidates) == 1 && len(claimants) == 0 {
			return CompactStartResult{Record: records[recoveryCandidates[0].lineageID], Action: CompactStartRecover}, nil
		}
		return CompactStartResult{Record: records[recoveryCandidates[0].lineageID], Action: CompactStartBlocked}, nil
	}
	if existing, exists := records[request.State.LineageID]; exists && !requestedClaims {
		return CompactStartResult{Record: existing, Action: CompactStartBlocked}, nil
	}
	if len(claimants) > 1 {
		return CompactStartResult{Record: records[claimants[0].lineageID], Action: CompactStartBlocked}, nil
	}
	for _, store := range claimants {
		record := records[store.lineageID]
		switch record.State.State {
		case StateReviewing:
			if record.State.InitialSnapshot.CandidateTree != request.State.InitialSnapshot.CandidateTree {
				continue
			}
			if !compactStartScopeCompatible(ctx, requestedStore.repo, record.State, request.State) {
				return CompactStartResult{Record: record, Action: CompactStartBlocked}, nil
			}
		case StateCorrectionRequired:
			if correctionClaims[store.lineageID] != compactCorrectionTargetResume {
				return CompactStartResult{Record: record, Action: CompactStartBlocked}, nil
			}
		case StateValidating:
			if !compactStartLiveTargetMatches(ctx, requestedStore.repo, record.State, request.State, true) {
				return CompactStartResult{Record: record, Action: CompactStartBlocked}, nil
			}
		case StateApproved:
			if !compactStartLiveTargetMatches(ctx, requestedStore.repo, record.State, request.State, true) {
				return CompactStartResult{Record: record, Action: CompactStartBlocked}, nil
			}
			payload, readErr := os.ReadFile(store.ReceiptPath())
			if readErr != nil {
				if os.IsNotExist(readErr) {
					return CompactStartResult{Record: record, Action: CompactStartBlocked}, nil
				}
				return CompactStartResult{}, fmt.Errorf("load compact approved receipt: %w", readErr)
			}
			receipt, parseErr := ParseCompactReceipt(payload)
			want, receiptErr := record.State.Receipt()
			if parseErr != nil || receiptErr != nil || !compactReceiptEqual(receipt, want) {
				return CompactStartResult{Record: record, Action: CompactStartBlocked}, nil
			}
			return CompactStartResult{Record: record, Action: CompactStartReuseReceipt}, nil
		default:
			return CompactStartResult{Record: record, Action: CompactStartBlocked}, nil
		}
		return CompactStartResult{Record: record, Action: CompactStartResumed,
			LensesRequired: len(record.State.LensResults) < len(record.State.SelectedLenses)}, nil
	}
	if err := validateCompactRepositoryEvidence(ctx, requestedStore.repo, nil, request.State, "review/start"); err != nil {
		return CompactStartResult{}, fmt.Errorf("validate compact start repository evidence: %w", err)
	}
	record, payload, err := makeCompactRecord(request.State)
	if err != nil {
		return CompactStartResult{}, err
	}
	if err := writeAtomic(requestedStore.StatePath(), payload, 0o644); err != nil {
		return CompactStartResult{}, err
	}
	if request.TracePath != "" {
		_ = appendCompactTrace(request.TracePath, CompactTraceEntry{
			Operation: "review/start", Revision: record.Revision, State: request.State.State,
			RecordedAt: time.Now().UTC().Format(time.RFC3339Nano),
		})
	}
	return CompactStartResult{
		Record: record, Action: CompactStartCreated,
		LensesRequired: len(request.State.SelectedLenses) > 0,
	}, nil
}

func explicitCompactSuccessor(ctx context.Context, repo string, predecessor CompactRecord) (bool, error) {
	stores, err := DiscoverCompactStores(ctx, repo)
	if err != nil {
		return false, err
	}
	needle := []byte(`"` + predecessor.State.LineageID + `"`)
	children := 0
	for _, store := range stores {
		if store.lineageID == predecessor.State.LineageID {
			continue
		}
		payload, readErr := os.ReadFile(store.StatePath())
		if readErr != nil || !bytes.Contains(payload, needle) {
			continue
		}
		record, parseErr := parseCompactRecord(payload, store.lineageID)
		if parseErr != nil {
			return false, fmt.Errorf("invalid related compact authority %q: %w", store.lineageID, parseErr)
		}
		if record.State.Recovery == nil || record.State.Recovery.PredecessorLineageID != predecessor.State.LineageID {
			continue
		}
		if err := validateCompactRecoveryEdge(predecessor, record.State); err != nil {
			return false, fmt.Errorf("invalid related compact authority %q: %w", store.lineageID, err)
		}
		children++
		if children > 1 {
			return false, fmt.Errorf("invalid compact authority graph: fork at %q", predecessor.State.LineageID)
		}
	}
	return children == 1, nil
}

func resumeExplicitCompactStart(ctx context.Context, store CompactStore, record CompactRecord, requested CompactState) (CompactStartResult, error) {
	existing := record.State
	blocked := func() (CompactStartResult, error) {
		return CompactStartResult{Record: record, Action: CompactStartBlocked}, nil
	}
	if existing.State == StateEscalated || compactHistoricalFailedValidator(existing) {
		if compactStartDeliveryScopeMatches(existing, requested) &&
			compactEscalatedRecoveryTargetChanged(existing.CurrentSnapshot, requested.InitialSnapshot) {
			return CompactStartResult{Record: record, Action: CompactStartRecover}, nil
		}
		return blocked()
	}
	currentTarget := existing.State == StateCorrectionRequired || existing.State == StateValidating || existing.State == StateApproved
	want := existing.InitialSnapshot
	if currentTarget {
		want = existing.CurrentSnapshot
	}
	if existing.PolicyHash != requested.PolicyHash || want.Identity != requested.InitialSnapshot.Identity ||
		(SnapshotBuilder{Repo: store.repo}).ValidateEvidence(ctx, requested.InitialSnapshot) != nil {
		return blocked()
	}
	switch existing.State {
	case StateReviewing, StateCorrectionRequired, StateValidating:
		return CompactStartResult{Record: record, Action: CompactStartResumed,
			LensesRequired: len(existing.LensResults) < len(existing.SelectedLenses)}, nil
	case StateApproved:
		payload, err := os.ReadFile(store.ReceiptPath())
		if err != nil {
			if os.IsNotExist(err) {
				return blocked()
			}
			return CompactStartResult{}, fmt.Errorf("load compact approved receipt: %w", err)
		}
		receipt, parseErr := ParseCompactReceipt(payload)
		wantReceipt, receiptErr := existing.Receipt()
		if parseErr != nil || receiptErr != nil || !compactReceiptEqual(receipt, wantReceipt) {
			return blocked()
		}
		return CompactStartResult{Record: record, Action: CompactStartReuseReceipt}, nil
	default:
		return blocked()
	}
}

func acquireCompactStartLock(ctx context.Context, path string) (*storeLock, error) {
	timer := time.NewTimer(compactStartLockTimeout)
	defer timer.Stop()
	ticker := time.NewTicker(compactStartLockPollInterval)
	defer ticker.Stop()
	for {
		if err := ctx.Err(); err != nil {
			return nil, &AuthorityLockCancelledError{Cause: err}
		}
		lock, err := acquireStoreLock(path)
		if err == nil || !errors.Is(err, ErrConcurrentUpdate) {
			return lock, err
		}
		select {
		case <-ctx.Done():
			return nil, &AuthorityLockCancelledError{Cause: ctx.Err()}
		case <-timer.C:
			return nil, &AuthorityLockTimeoutError{Timeout: compactStartLockTimeout}
		case <-ticker.C:
		}
	}
}

func compactStartClaimsTarget(ctx context.Context, repo string, existing, requested CompactState) bool {
	if (SnapshotBuilder{Repo: repo}).ValidateEvidence(ctx, requested.InitialSnapshot) != nil {
		return false
	}
	if (existing.State == StateValidating || existing.State == StateApproved) &&
		compactStartLiveTargetMatches(ctx, repo, existing, requested, true) {
		return true
	}
	if !compactStartDeliveryScopeMatches(existing, requested) {
		return false
	}
	candidate := requested.InitialSnapshot.CandidateTree
	return candidate == existing.InitialSnapshot.CandidateTree || candidate == existing.CurrentSnapshot.CandidateTree
}

// compactStartDeliveryScopeMatches compares the immutable delivery boundary
// without Snapshot.Identity because current-changes and base-diff have distinct
// representations for the same base-to-candidate tree range.
func compactStartDeliveryScopeMatches(existing, requested CompactState) bool {
	original, live := existing.InitialSnapshot, requested.InitialSnapshot
	return original.Projection == live.Projection &&
		compactStartTargetKindsCompatible(original.Kind, live.Kind) &&
		live.BaseTree == original.BaseTree &&
		live.PathsDigest == original.PathsDigest &&
		equalStrings(live.Paths, existing.GenesisPaths) &&
		equalStrings(live.IntendedUntracked, original.IntendedUntracked) &&
		live.IntendedUntrackedProof == original.IntendedUntrackedProof &&
		equalStrings(live.LedgerIDs, original.LedgerIDs)
}

func compactStartTargetKindsCompatible(existing, requested TargetKind) bool {
	if existing == requested {
		return true
	}
	return existing == TargetCurrentChanges && requested == TargetBaseDiff ||
		existing == TargetBaseDiff && requested == TargetCurrentChanges
}

type compactCorrectionTargetClaim uint8

const (
	compactCorrectionTargetUnclaimed compactCorrectionTargetClaim = iota
	compactCorrectionTargetResume
	compactCorrectionTargetBlocked
	compactCorrectionTargetRecover
)

// classifyCompactCorrectionTarget keeps correction ownership bound to the
// original delivery boundary even when in-genesis bytes change. START may only
// resume an authorized continuation; otherwise the existing authority blocks a
// fresh budget. Repository-derived path expansion, and a pure non-empty
// contraction of genesis scope, remain an explicit recovery.
func classifyCompactCorrectionTarget(ctx context.Context, repo string, existing, requested CompactState) compactCorrectionTargetClaim {
	live := requested.InitialSnapshot
	if existing.State != StateCorrectionRequired ||
		existing.InitialSnapshot.Projection != live.Projection ||
		!compactStartTargetKindsCompatible(existing.InitialSnapshot.Kind, live.Kind) ||
		existing.InitialSnapshot.BaseTree != live.BaseTree || len(live.LedgerIDs) != 0 ||
		(SnapshotBuilder{Repo: repo}).ValidateEvidence(ctx, live) != nil {
		return compactCorrectionTargetUnclaimed
	}
	if compactHistoricalFailedValidator(existing) {
		if compactEscalatedRecoveryTargetChanged(existing.CurrentSnapshot, live) {
			return compactCorrectionTargetRecover
		}
		return compactCorrectionTargetBlocked
	}
	if compactRecoveryAddsGenesisPath(existing, live) {
		return compactCorrectionTargetRecover
	}
	if pathsAreSubset(live.Paths, existing.GenesisPaths) != nil {
		return compactCorrectionTargetUnclaimed
	}
	if compactStartCorrectionResume(ctx, repo, existing, requested) {
		return compactCorrectionTargetResume
	}
	if compactRecoveryContractsGenesisPaths(existing, live) {
		return compactCorrectionTargetRecover
	}
	return compactCorrectionTargetBlocked
}

// compactCorrectionRecoveryDisposition names the `review recover --disposition`
// value the recovery rules accept for a correction-required predecessor that
// classifyCompactCorrectionTarget already classified as
// compactCorrectionTargetRecover. It re-evaluates the very predicates that
// authorize each recovery — compactHistoricalFailedValidator for the escalated
// disposition, and the genesis-scope expansion/contraction pair for the
// scope-changed disposition — in the same order, so status can never name a
// disposition ValidateCompactRecovery would reject. It authorizes nothing on
// its own and returns "" when no disposition applies.
func compactCorrectionRecoveryDisposition(existing CompactState, live Snapshot) RecoveryDisposition {
	if existing.State != StateCorrectionRequired {
		return ""
	}
	if compactHistoricalFailedValidator(existing) {
		if compactEscalatedRecoveryTargetChanged(existing.CurrentSnapshot, live) {
			return RecoveryEscalated
		}
		return ""
	}
	if compactRecoveryAddsGenesisPath(existing, live) || compactRecoveryContractsGenesisPaths(existing, live) {
		return RecoveryScopeChanged
	}
	return ""
}

func compactStartInitialSnapshotsEqual(existing, requested CompactState) bool {
	return compactStartDeliveryScopeMatches(existing, requested) &&
		existing.InitialSnapshot.CandidateTree == requested.InitialSnapshot.CandidateTree
}

func compactStartCorrectionResume(ctx context.Context, repo string, existing, requested CompactState) bool {
	return compactStartLiveTargetMatches(ctx, repo, existing, requested, false) &&
		(requested.InitialSnapshot.CandidateTree == existing.CurrentSnapshot.CandidateTree || compactStartCorrectionCandidateMatches(ctx, repo, existing, requested))
}

func compactStartCorrectionCandidateMatches(ctx context.Context, repo string, existing, requested CompactState) bool {
	if existing.ProposedCorrectionLines == nil {
		return false
	}
	fix, err := (SnapshotBuilder{Repo: repo}).Build(ctx, Target{Kind: TargetFixDiff,
		Projection: existing.InitialSnapshot.Projection, BaseRef: existing.CurrentSnapshot.CandidateTree,
		IntendedUntracked: existing.InitialSnapshot.IntendedUntracked, LedgerIDs: existing.FixFindingIDs})
	if err != nil || fix.CandidateTree != requested.InitialSnapshot.CandidateTree || pathsAreSubset(fix.Paths, existing.GenesisPaths) != nil {
		return false
	}
	lines, err := (SnapshotBuilder{Repo: repo}).ChangedLines(ctx, fix)
	return err == nil && lines <= existing.CorrectionBudget-existing.CumulativeCorrectionLines
}

func compactStartLiveTargetMatches(ctx context.Context, repo string, existing, requested CompactState, requireCurrentCandidate bool) bool {
	if existing.Generation != requested.Generation || existing.PolicyHash != requested.PolicyHash ||
		!reflect.DeepEqual(existing.Recovery, requested.Recovery) {
		return false
	}
	return compactLiveTargetMatchesSnapshot(ctx, repo, existing, requested.InitialSnapshot, requireCurrentCandidate)
}

func compactStartScopeEqual(existing, requested CompactState) bool {
	return compactStartScopeIdentityEqual(existing, requested) &&
		existing.RiskLevel == requested.RiskLevel && equalStrings(existing.SelectedLenses, requested.SelectedLenses)
}

func compactStartScopeIdentityEqual(existing, requested CompactState) bool {
	return existing.Generation == requested.Generation &&
		compactStartInitialSnapshotsEqual(existing, requested) &&
		equalStrings(existing.GenesisPaths, requested.GenesisPaths) &&
		existing.PolicyHash == requested.PolicyHash &&
		existing.OriginalChangedLines == requested.OriginalChangedLines &&
		existing.CorrectionBudget == requested.CorrectionBudget && reflect.DeepEqual(existing.Recovery, requested.Recovery)
}

func compactStartScopeCompatible(ctx context.Context, repo string, existing, requested CompactState) bool {
	if compactStartScopeEqual(existing, requested) {
		return true
	}
	full4R := []string{LensRisk, LensResilience, LensReadability, LensReliability}
	if !compactStartScopeIdentityEqual(existing, requested) || existing.RiskLevel != RiskHigh ||
		!equalStrings(existing.SelectedLenses, full4R) || requested.RiskLevel != RiskMedium ||
		!equalStrings(requested.SelectedLenses, []string{LensReadability}) {
		return false
	}
	builder := SnapshotBuilder{Repo: repo}
	if err := builder.ValidateEvidence(ctx, requested.InitialSnapshot); err != nil {
		return false
	}
	assessment, err := builder.AssessSnapshotRisk(ctx, requested.InitialSnapshot)
	return err == nil && assessment.Level == RiskMedium && assessment.DominantLens == LensReadability &&
		assessment.ChangedLines == requested.OriginalChangedLines
}

func (store CompactStore) StatePath() string { return filepath.Join(store.Dir, compactStateFileName) }

func (store CompactStore) ReceiptPath() string {
	return filepath.Join(store.Dir, compactReceiptFileName)
}

func (store CompactStore) Load() (CompactRecord, error) {
	payload, err := os.ReadFile(store.StatePath())
	if err != nil {
		return CompactRecord{}, err
	}
	return parseCompactRecord(payload, store.lineageID)
}

func (store CompactStore) Replace(expectedRevision, operation string, next CompactState) (string, error) {
	return store.ReplaceContext(context.Background(), expectedRevision, operation, next)
}

func (store CompactStore) ReplaceContext(ctx context.Context, expectedRevision, operation string, next CompactState) (string, error) {
	return store.replaceContextGuarded(ctx, expectedRevision, operation, next, nil)
}

// replaceContextGuarded commits exactly like ReplaceContext, but runs guard
// inside the same critical section that publishes the successor, immediately
// before the state file is written and after the revision CAS has passed.
//
// It exists because the revision CAS alone cannot see every relevant change:
// CaptureReviewerResult publishes its artifact under the reviewer-results
// directory while holding this same store lock and never bumps the authority
// revision, so a precondition an operation derived from that directory before
// taking the lock is stale by the time the CAS succeeds. A guard re-derives
// such a precondition from the authoritative on-disk state while the lock is
// held, which makes the check atomic with the commit.
//
// The guard runs with the store lock already held and must never acquire it
// again — acquireStoreLock and acquireLocalStoreLock take an exclusive advisory
// lock on the same file, and a second acquisition from this process would be
// refused rather than granted. Guards are therefore restricted to lock-free
// reads of the authority directory.
func (store CompactStore) replaceContextGuarded(ctx context.Context, expectedRevision, operation string, next CompactState, guard func() error) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if strings.TrimSpace(operation) == "" {
		return "", errors.New("compact review operation is required")
	}
	if err := next.Validate(); err != nil {
		return "", fmt.Errorf("%w: %v", ErrInvalidSuccessor, err)
	}
	if store.lineageID != "" && next.LineageID != store.lineageID {
		return "", fmt.Errorf("%w: compact lineage does not match store", ErrInvalidSuccessor)
	}
	var maintenance *MaintenanceLock
	var err error
	if store.maintenanceLockPath != "" {
		maintenance, err = acquireMaintenanceLock(ctx, store.maintenanceLockPath, maintenanceShared)
		if err != nil {
			return "", err
		}
		defer maintenance.Release()
	}
	lock, err := acquireLocalStoreLock(store.lockPath)
	if err != nil {
		return "", err
	}
	defer lock.release()

	var current *CompactRecord
	payload, err := os.ReadFile(store.StatePath())
	if err == nil {
		loaded, parseErr := parseCompactRecord(payload, store.lineageID)
		if parseErr != nil {
			return "", parseErr
		}
		if loaded.HistoricalCompat {
			return "", fmt.Errorf("%w: %s for lineage %q", ErrHistoricalCompatReadOnly, operation, loaded.State.LineageID)
		}
		current = &loaded
	} else if !os.IsNotExist(err) {
		return "", err
	}
	record, payload, err := makeCompactRecord(next)
	if err != nil {
		return "", err
	}
	if current != nil && current.Revision == record.Revision && compactStateEqual(current.State, next) {
		return record.Revision, nil
	}
	currentRevision := ""
	if current != nil {
		currentRevision = current.Revision
	}
	if currentRevision != expectedRevision {
		return "", fmt.Errorf("%w: expected compact revision %q, current %q", ErrConcurrentUpdate, expectedRevision, currentRevision)
	}
	if current == nil {
		if operation != "review/start" || next.State != StateReviewing {
			return "", fmt.Errorf("%w: compact authority must start in reviewing state", ErrInvalidSuccessor)
		}
	} else if err := validateCompactSuccessor(current.State, next, operation); err != nil {
		return "", err
	}
	if store.repo != "" {
		if err := validateCompactRepositoryEvidence(ctx, store.repo, current, next, operation); err != nil {
			return "", fmt.Errorf("%w: %v", ErrInvalidSuccessor, err)
		}
	}
	if guard != nil {
		if err := guard(); err != nil {
			return "", err
		}
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if err := writeAtomic(store.StatePath(), payload, 0o644); err != nil {
		return "", err
	}
	if store.TracePath != "" {
		_ = appendCompactTrace(store.TracePath, CompactTraceEntry{
			Operation: operation, PreviousRevision: currentRevision, Revision: record.Revision,
			State: next.State, RecordedAt: time.Now().UTC().Format(time.RFC3339Nano),
		})
	}
	return record.Revision, nil
}

// CaptureReviewerResult revalidates the reviewing binding while holding shared
// maintenance access and the compact version lock before publishing an artifact.
func (store CompactStore) CaptureReviewerResult(target, lens string, order int, publish func(CompactState) error) error {
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
	state := record.State
	if state.State != StateReviewing || state.InitialSnapshot.Identity != target || order < 0 || order >= len(state.SelectedLenses) || state.SelectedLenses[order] != lens {
		return errors.New("capture binding does not match the current reviewing authority")
	}
	return publish(state)
}

func validateCompactRepositoryEvidence(ctx context.Context, repo string, current *CompactRecord, next CompactState, operation string) error {
	builder := SnapshotBuilder{Repo: repo}
	if current == nil {
		if err := builder.ValidateEvidence(ctx, next.InitialSnapshot); err != nil {
			return errors.New("initial compact snapshot is not repository-derived")
		}
		risk, lines, err := builder.ClassifySnapshotRisk(ctx, next.InitialSnapshot)
		if err != nil || risk != next.RiskLevel || lines != next.OriginalChangedLines {
			return errors.New("compact risk inputs do not match repository evidence")
		}
	}
	if operation == "review/complete-fix" {
		attempt := next.CorrectionAttempts[len(next.CorrectionAttempts)-1]
		if err := builder.ValidateEvidence(ctx, attempt.Snapshot); err != nil {
			return errors.New("compact correction snapshot is not repository-derived")
		}
		lines, err := builder.ChangedLines(ctx, attempt.Snapshot)
		if err != nil || lines != attempt.ActualLines {
			return errors.New("compact correction size does not match repository evidence")
		}
	}
	if operation == "review/complete-review" {
		for _, finding := range next.Findings {
			classification := next.Classifications[finding.ID]
			switch classification.Causality {
			case CausalIntroduced, CausalBehaviorActivated, CausalWorsened:
				changed, err := builder.CandidateLocationSupportsCausality(ctx, next.InitialSnapshot, finding.Location, classification.Causality)
				if err != nil || !changed {
					return errors.New("candidate-causal compact finding is not on a repository-derived changed line")
				}
			}
		}
	}
	if operation == "review/invalidate" {
		if err := rebuildCurrentSnapshotEvidence(ctx, repo, next.InitialSnapshot); err != nil {
			return err
		}
	}
	return nil
}

func validateCompactSuccessor(previous, next CompactState, operation string) error {
	if previous.LineageID != next.LineageID || previous.Generation != next.Generation ||
		!snapshotsEqual(previous.InitialSnapshot, next.InitialSnapshot) || !equalStrings(previous.GenesisPaths, next.GenesisPaths) ||
		previous.PolicyHash != next.PolicyHash || previous.RiskLevel != next.RiskLevel ||
		!equalStrings(previous.SelectedLenses, next.SelectedLenses) || previous.OriginalChangedLines != next.OriginalChangedLines ||
		previous.CorrectionBudget != next.CorrectionBudget {
		return fmt.Errorf("%w: compact review scope, tier, policy, and budget are immutable", ErrInvalidSuccessor)
	}
	switch operation {
	case "review/invalidate":
		expected := previous
		if err := expected.Invalidate(next.InvalidationReason); err != nil || !compactStateEqual(expected, next) {
			return fmt.Errorf("%w: invalidation must retain a pristine reviewing authority", ErrInvalidSuccessor)
		}
	case "review/complete-review":
		if previous.State != StateReviewing || next.State != StateCorrectionRequired && next.State != StateValidating && next.State != StateEscalated {
			return fmt.Errorf("%w: invalid compact review completion", ErrInvalidSuccessor)
		}
		if !snapshotsEqual(previous.CurrentSnapshot, next.CurrentSnapshot) || next.ProposedCorrectionLines != nil || next.ActualCorrectionLines != nil || next.FixDeltaHash != EmptyFixDeltaHash || next.OriginalCriteria != nil || next.EvidenceHash != "" {
			return fmt.Errorf("%w: compact review completion changed correction or delivery state", ErrInvalidSuccessor)
		}
	case "review/begin-fix":
		if previous.State != StateCorrectionRequired || next.State != StateCorrectionRequired && next.State != StateEscalated || previous.ProposedCorrectionLines != nil || next.ProposedCorrectionLines == nil {
			return fmt.Errorf("%w: invalid compact correction start", ErrInvalidSuccessor)
		}
		expected := previous
		expected.State = next.State
		expected.ProposedCorrectionLines = next.ProposedCorrectionLines
		if !compactStateEqual(expected, next) {
			return fmt.Errorf("%w: compact correction start changed unrelated state", ErrInvalidSuccessor)
		}
	case "review/complete-fix":
		if previous.State != StateCorrectionRequired || previous.ProposedCorrectionLines == nil || next.State != StateValidating && next.State != StateCorrectionRequired && next.State != StateEscalated || len(next.CorrectionAttempts) != len(previous.CorrectionAttempts)+1 {
			return fmt.Errorf("%w: invalid compact correction completion", ErrInvalidSuccessor)
		}
		if len(previous.CorrectionAttempts) > 0 && !reflect.DeepEqual(previous.CorrectionAttempts, next.CorrectionAttempts[:len(previous.CorrectionAttempts)]) {
			return fmt.Errorf("%w: compact correction attempt history is immutable", ErrInvalidSuccessor)
		}
		if !reflectCompactReviewData(previous, next) || previous.EvidenceHash != next.EvidenceHash {
			return fmt.Errorf("%w: compact correction changed frozen review evidence", ErrInvalidSuccessor)
		}
	case CompactResultDispositionOperation:
		// reviewing -> escalated. The disposition may only append its own audit
		// record and flip the terminal state; freezing every other field here is
		// what keeps captured lens results, findings, and evidence untouched and
		// makes it impossible to launder a refused payload into an admitted one.
		if previous.State != StateReviewing || next.State != StateEscalated {
			return fmt.Errorf("%w: a reviewer result disposition terminally escalates a reviewing authority only", ErrInvalidSuccessor)
		}
		if len(next.ResultDispositions) != len(previous.ResultDispositions)+1 {
			return fmt.Errorf("%w: a reviewer result disposition records exactly one disposition", ErrInvalidSuccessor)
		}
		expected := previous
		expected.State = StateEscalated
		expected.ResultDispositions = next.ResultDispositions
		if !compactStateEqual(expected, next) {
			return fmt.Errorf("%w: reviewer result disposition changed unrelated state", ErrInvalidSuccessor)
		}
	case "review/complete-verification":
		if previous.State != StateValidating || next.State != StateApproved && next.State != StateEscalated || !validSHA256(next.EvidenceHash) {
			return fmt.Errorf("%w: invalid compact verification completion", ErrInvalidSuccessor)
		}
		expected := previous
		expected.State = next.State
		expected.EvidenceHash = next.EvidenceHash
		if !compactStateEqual(expected, next) {
			return fmt.Errorf("%w: compact verification changed unrelated state", ErrInvalidSuccessor)
		}
	default:
		return fmt.Errorf("%w: unsupported compact operation %q", ErrInvalidSuccessor, operation)
	}
	return nil
}

func reflectCompactReviewData(previous, next CompactState) bool {
	return reflect.DeepEqual(previous.LensResults, next.LensResults) &&
		reflect.DeepEqual(previous.Findings, next.Findings) &&
		reflect.DeepEqual(previous.Classifications, next.Classifications) &&
		reflect.DeepEqual(previous.Outcomes, next.Outcomes) &&
		equalStrings(previous.FixFindingIDs, next.FixFindingIDs) &&
		len(previous.FollowUps) <= len(next.FollowUps) && reflect.DeepEqual(previous.FollowUps, next.FollowUps[:len(previous.FollowUps)])
}

func makeCompactRecord(state CompactState) (CompactRecord, []byte, error) {
	statePayload, err := json.Marshal(state)
	if err != nil {
		return CompactRecord{}, nil, err
	}
	sum := sha256.Sum256(append([]byte("gentle-ai.review-state/v2\x00"), statePayload...))
	record := CompactRecord{Schema: compactRecordSchema, Revision: "sha256:" + hex.EncodeToString(sum[:]), State: state}
	payload, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return CompactRecord{}, nil, err
	}
	return record, append(payload, '\n'), nil
}

// CompactRevisionForState derives the exact content-addressed revision without
// writing authority. FINALIZE uses it for write-ahead successor planning.
func CompactRevisionForState(state CompactState) (string, error) {
	record, _, err := makeCompactRecord(state)
	return record.Revision, err
}

func parseCompactRecord(payload []byte, lineageID string) (CompactRecord, error) {
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	var record CompactRecord
	if strictErr := decoder.Decode(&record); strictErr != nil {
		if !retiredCompactFieldError(strictErr) {
			return CompactRecord{}, strictErr
		}
		historical, historicalErr := parseHistoricalCompactRecord(payload)
		if historicalErr != nil {
			// Preserve the original strict decode wording for callers.
			return CompactRecord{}, strictErr
		}
		record = historical
	} else {
		var extra any
		if err := decoder.Decode(&extra); err != io.EOF {
			return CompactRecord{}, errors.New("multiple JSON values in compact review state")
		}
	}
	if record.Schema != compactRecordSchema || !validSHA256(record.Revision) {
		return CompactRecord{}, errors.New("invalid compact review state record")
	}
	if err := record.State.Validate(); err != nil {
		return CompactRecord{}, err
	}
	if lineageID != "" && record.State.LineageID != lineageID {
		return CompactRecord{}, errors.New("compact state lineage does not match its directory")
	}
	if !record.HistoricalCompat {
		want, _, err := makeCompactRecord(record.State)
		if err != nil || want.Revision != record.Revision {
			return CompactRecord{}, errors.New("compact review state checksum mismatch")
		}
	}
	return record, nil
}

// retiredCompactFieldError reports whether a strict decode failure names a
// retired compatibility field, so only genuine historical records pay the
// tolerant second parse. The decoder error only carries the leaf field name;
// the tolerant parse then enforces the exact nesting level of each path.
func retiredCompactFieldError(err error) bool {
	message := err.Error()
	if !strings.Contains(message, "unknown field") {
		return false
	}
	for path := range compactRetiredStateFieldPaths {
		segments := strings.Split(path, ".")
		if strings.Contains(message, `"`+segments[len(segments)-1]+`"`) {
			return true
		}
	}
	return false
}

// parseHistoricalCompactRecord tolerates only retired state field paths from
// older builds, each removed at its exact nesting level. The persisted
// revision must bind the exact historical state bytes, so loading preserves
// revisions and provenance without ever rewriting or re-hashing persisted
// authority; retired content such as recovery.review_start stays intact on
// disk and is only dropped from the decoded in-memory view.
func parseHistoricalCompactRecord(payload []byte) (CompactRecord, error) {
	var envelope struct {
		Schema   string          `json:"schema"`
		Revision string          `json:"revision"`
		State    json.RawMessage `json:"state"`
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&envelope); err != nil {
		return CompactRecord{}, err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return CompactRecord{}, errors.New("multiple JSON values in compact review state")
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(envelope.State, &fields); err != nil {
		return CompactRecord{}, err
	}
	retired := false
	for path := range compactRetiredStateFieldPaths {
		deleted, deleteErr := deleteRetiredCompactField(fields, strings.Split(path, "."))
		if deleteErr != nil {
			return CompactRecord{}, deleteErr
		}
		if deleted {
			retired = true
		}
	}
	if !retired {
		return CompactRecord{}, errors.New("compact review state has no tolerated retired fields")
	}
	remaining, err := json.Marshal(fields)
	if err != nil {
		return CompactRecord{}, err
	}
	stateDecoder := json.NewDecoder(bytes.NewReader(remaining))
	stateDecoder.DisallowUnknownFields()
	var state CompactState
	if err := stateDecoder.Decode(&state); err != nil {
		return CompactRecord{}, err
	}
	var compacted bytes.Buffer
	if err := json.Compact(&compacted, envelope.State); err != nil {
		return CompactRecord{}, err
	}
	// makeCompactRecord hashes json.Marshal(state) while the record file is
	// written with json.MarshalIndent, which is marshal-then-indent; Compact
	// only inverts the added whitespace, so this reproduces the historical
	// writer's exact revision preimage without re-marshaling the struct.
	sum := sha256.Sum256(append([]byte(CompactStateSchema+"\x00"), compacted.Bytes()...))
	if envelope.Revision != "sha256:"+hex.EncodeToString(sum[:]) {
		return CompactRecord{}, errors.New("compact review state checksum mismatch")
	}
	return CompactRecord{Schema: envelope.Schema, Revision: envelope.Revision, State: state, HistoricalCompat: true}, nil
}

// deleteRetiredCompactField removes one retired field at the exact nesting
// level its dot-path names, mutating only the in-memory field view used for
// the tolerant re-decode. A retired leaf name appearing at any other level
// stays in place and keeps failing strict decoding.
func deleteRetiredCompactField(fields map[string]json.RawMessage, path []string) (bool, error) {
	name := path[0]
	raw, exists := fields[name]
	if !exists {
		return false, nil
	}
	if len(path) == 1 {
		delete(fields, name)
		return true, nil
	}
	var nested map[string]json.RawMessage
	if err := json.Unmarshal(raw, &nested); err != nil {
		return false, err
	}
	deleted, err := deleteRetiredCompactField(nested, path[1:])
	if err != nil || !deleted {
		return deleted, err
	}
	updated, err := json.Marshal(nested)
	if err != nil {
		return false, err
	}
	fields[name] = updated
	return true, nil
}

func appendCompactTrace(path string, entry CompactTraceEntry) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer file.Close()
	payload, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	if _, err := file.Write(append(payload, '\n')); err != nil {
		return err
	}
	return file.Sync()
}

func (store CompactStore) ExportTransport() (CompactTransport, error) {
	record, err := store.Load()
	if err != nil {
		return CompactTransport{}, err
	}
	if record.HistoricalCompat {
		// Transport re-marshals the typed record, which cannot reproduce the
		// retired historical bytes or their revision; refuse before a
		// checksum failure would mask the cause.
		return CompactTransport{}, fmt.Errorf("%w: lineage %q cannot be exported as compact transport", ErrHistoricalCompatReadOnly, record.State.LineageID)
	}
	transport := CompactTransport{Schema: CompactTransportSchema, Record: record}
	if payload, readErr := os.ReadFile(store.ReceiptPath()); readErr == nil {
		receipt, parseErr := ParseCompactReceipt(payload)
		authoritative, authorityErr := record.State.Receipt()
		if parseErr != nil || authorityErr != nil || !compactReceiptEqual(receipt, authoritative) {
			return CompactTransport{}, errors.New("compact receipt does not match authority")
		}
		transport.Receipt = &receipt
	} else if !os.IsNotExist(readErr) {
		return CompactTransport{}, readErr
	}
	transport.BundleDigest = compactTransportDigest(transport)
	return transport, nil
}

func ParseCompactTransport(payload []byte) (CompactTransport, error) {
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	var transport CompactTransport
	if err := decoder.Decode(&transport); err != nil {
		return CompactTransport{}, err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return CompactTransport{}, errors.New("multiple JSON values in compact review transport")
	}
	if transport.Schema != CompactTransportSchema || transport.BundleDigest != compactTransportDigest(transport) {
		return CompactTransport{}, errors.New("compact review transport checksum mismatch")
	}
	recordPayload, _ := json.Marshal(transport.Record)
	if _, err := parseCompactRecord(recordPayload, transport.Record.State.LineageID); err != nil {
		return CompactTransport{}, err
	}
	if transport.Receipt != nil {
		authoritative, err := transport.Record.State.Receipt()
		if err != nil || transport.Receipt.Validate() != nil || !compactReceiptEqual(*transport.Receipt, authoritative) {
			return CompactTransport{}, errors.New("compact transport receipt does not match state")
		}
	}
	return transport, nil
}

func WriteCompactTransportAtomic(path string, transport CompactTransport) error {
	transport.BundleDigest = compactTransportDigest(transport)
	payload, err := json.MarshalIndent(transport, "", "  ")
	if err != nil {
		return err
	}
	validated, err := ParseCompactTransport(append(payload, '\n'))
	if err != nil || validated.BundleDigest != transport.BundleDigest {
		return errors.New("invalid compact review transport")
	}
	return writeAtomic(path, append(payload, '\n'), 0o644)
}

func ImportCompactTransport(ctx context.Context, repo string, transport CompactTransport) (CompactRecord, error) {
	payload, _ := json.Marshal(transport)
	validated, err := ParseCompactTransport(payload)
	if err != nil {
		return CompactRecord{}, err
	}
	store, err := CompactAuthoritativeStore(ctx, repo, validated.Record.State.LineageID)
	if err != nil {
		return CompactRecord{}, err
	}
	if legacy, legacyErr := AuthoritativeStore(ctx, repo, validated.Record.State.LineageID); legacyErr == nil {
		if _, loadErr := legacy.LoadChain(); loadErr == nil {
			return CompactRecord{}, errors.New("cannot import compact authority over an existing legacy v1 lineage")
		}
	}
	if recovery := validated.Record.State.Recovery; recovery != nil {
		predecessorStore, predecessorErr := CompactAuthoritativeStore(ctx, repo, recovery.PredecessorLineageID)
		if predecessorErr != nil {
			return CompactRecord{}, predecessorErr
		}
		predecessor, predecessorErr := predecessorStore.Load()
		if predecessorErr != nil {
			return CompactRecord{}, fmt.Errorf("load imported recovery predecessor: %w", predecessorErr)
		}
		if err := validateCompactRecoveryEdge(predecessor, validated.Record.State); err != nil {
			return CompactRecord{}, fmt.Errorf("validate imported recovery edge: %w", err)
		}
	}
	lock, err := acquireStoreLock(store.lockPath)
	if err != nil {
		return CompactRecord{}, err
	}
	defer lock.release()
	if err := store.installTransportRecordLocked(ctx, validated.Record); err != nil {
		return CompactRecord{}, err
	}
	if validated.Receipt != nil {
		if err := store.writeReceiptLocked(*validated.Receipt); err != nil {
			return CompactRecord{}, err
		}
	}
	return store.Load()
}

func (store CompactStore) installTransportRecordLocked(ctx context.Context, record CompactRecord) error {
	if existing, loadErr := store.Load(); loadErr == nil {
		if existing.Revision == record.Revision && compactStateEqual(existing.State, record.State) {
			return nil
		}
		return ErrConcurrentUpdate
	} else if !os.IsNotExist(loadErr) {
		return loadErr
	}
	if err := validateCompactTransportDelivery(ctx, store.repo, record.State); err != nil {
		return err
	}
	want, payload, err := makeCompactRecord(record.State)
	if err != nil || want.Revision != record.Revision {
		return errors.New("imported compact record checksum changed")
	}
	return writeAtomic(store.StatePath(), payload, 0o644)
}

// WriteReceipt validates the receipt against authoritative compact state while
// holding maintenance shared access before the compact version lock.
func (store CompactStore) WriteReceipt(ctx context.Context, receipt CompactReceipt) error {
	lock, err := acquireStoreLock(store.lockPath)
	if err != nil {
		return err
	}
	defer lock.release()
	return store.writeReceiptLocked(receipt)
}

func (store CompactStore) writeReceiptLocked(receipt CompactReceipt) error {
	record, err := store.Load()
	if err != nil {
		return err
	}
	want, err := record.State.Receipt()
	if err != nil || !compactReceiptEqual(receipt, want) {
		return errors.New("compact receipt does not match authority")
	}
	return WriteCompactReceiptAtomic(store.ReceiptPath(), receipt)
}

func validateCompactTransportDelivery(ctx context.Context, repo string, state CompactState) error {
	builder := SnapshotBuilder{Repo: repo}
	headTree, err := builder.resolveTree(ctx, "HEAD")
	if err != nil || headTree != state.CurrentSnapshot.CandidateTree {
		return errors.New("imported compact authority does not match the current delivered tree")
	}
	paths, err := builder.changedPaths(ctx, state.InitialSnapshot.BaseTree, state.CurrentSnapshot.CandidateTree)
	if err != nil {
		return fmt.Errorf("derive imported compact delivered scope: %w", err)
	}
	if !equalStrings(paths, state.GenesisPaths) || digestPaths(paths) != state.InitialSnapshot.PathsDigest {
		return errors.New("imported compact authority does not match the original base-to-final path scope")
	}
	proof, err := builder.untrackedProof(ctx, state.CurrentSnapshot.CandidateTree, state.CurrentSnapshot.IntendedUntracked)
	if err != nil || proof != state.CurrentSnapshot.IntendedUntrackedProof {
		return errors.New("imported compact authority does not match delivered intended-untracked content")
	}
	return nil
}

func compactTransportDigest(transport CompactTransport) string {
	copy := transport
	copy.BundleDigest = ""
	payload, _ := json.Marshal(copy)
	sum := sha256.Sum256(append([]byte("gentle-ai.review-transport/v2\x00"), payload...))
	return "sha256:" + hex.EncodeToString(sum[:])
}
