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
const CompactTransportSchema = "gentle-ai.review-transport/v2"
const LegacyReadOnlyErrorCode = "legacy_v1_read_only"

var ErrLegacyReadOnly = errors.New("legacy v1 review lineage is read-only")

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
}

type CompactStore struct {
	Dir       string
	lineageID string
	repo      string
	lockPath  string
	TracePath string
}

type CompactStartAction string

const (
	CompactStartCreated      CompactStartAction = "created"
	CompactStartResumed      CompactStartAction = "resumed"
	CompactStartReuseReceipt CompactStartAction = "reuse-receipt"
	CompactStartBlocked      CompactStartAction = "blocked-scope-action"
)

type CompactStartRequest struct {
	State     CompactState
	TracePath string
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
	if predecessor.State.InitialSnapshot.Projection != request.Successor.InitialSnapshot.Projection {
		return CompactRecord{}, errors.New("recovery successor must retain the predecessor projection")
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
	switch request.Disposition {
	case RecoveryScopeChanged:
		if predecessor.State.State != StateApproved {
			return CompactRecord{}, errors.New("scope-changed recovery requires an approved predecessor")
		}
		if !compactRecoveryScopeChanged(predecessor.State.CurrentSnapshot, request.Successor.InitialSnapshot) {
			return CompactRecord{}, errors.New("approved predecessor scope has not changed")
		}
	case RecoveryInvalidated:
		if predecessor.State.State != StateInvalidated {
			return CompactRecord{}, errors.New("recovery requires an invalidated predecessor")
		}
	case RecoveryEscalated:
		if predecessor.State.State != StateEscalated {
			return CompactRecord{}, errors.New("recovery requires an escalated predecessor")
		}
		if strings.TrimSpace(request.MaintainerAuthorization) == "" {
			return CompactRecord{}, errors.New("escalated recovery requires explicit maintainer authorization")
		}
	default:
		return CompactRecord{}, errors.New("unsupported recovery disposition")
	}
	if request.Successor.State != StateReviewing {
		return CompactRecord{}, errors.New("recovery successor must start in reviewing state")
	}
	if request.Successor.Generation != predecessor.State.Generation+1 {
		return CompactRecord{}, errors.New("recovery successor generation must follow predecessor")
	}
	if err := request.Successor.Validate(); err != nil {
		return CompactRecord{}, err
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

func compactRecoveryScopeChanged(previous, next Snapshot) bool {
	return previous.CandidateTree != next.CandidateTree || previous.PathsDigest != next.PathsDigest || previous.Kind == next.Kind && previous.BaseTree != next.BaseTree
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
	return CompactStore{Dir: dir, lineageID: lineageID, repo: root, lockPath: filepath.Join(versionRoot, "LOCK")}, nil
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
		if _, statErr := os.Stat(filepath.Join(dir, "review-state.json")); os.IsNotExist(statErr) {
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
			lockPath: filepath.Join(versionRoot, "LOCK"),
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
	requestedClaims := false
	for _, store := range leaves {
		if compactStartClaimsTarget(ctx, requestedStore.repo, records[store.lineageID].State, request.State) {
			claimants = append(claimants, store)
			requestedClaims = requestedClaims || store.lineageID == request.State.LineageID
		}
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
			if !compactStartScopeEqual(record.State, request.State) {
				return CompactStartResult{Record: record, Action: CompactStartBlocked}, nil
			}
		case StateCorrectionRequired:
			if !compactStartCorrectionResume(ctx, requestedStore.repo, record.State, request.State) {
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

func acquireCompactStartLock(ctx context.Context, path string) (*storeLock, error) {
	for {
		lock, err := acquireStoreLock(path)
		if err == nil || !errors.Is(err, ErrConcurrentUpdate) {
			return lock, err
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(5 * time.Millisecond):
		}
	}
}

func compactStartClaimsTarget(ctx context.Context, repo string, existing, requested CompactState) bool {
	if (SnapshotBuilder{Repo: repo}).ValidateEvidence(ctx, requested.InitialSnapshot) != nil {
		return false
	}
	if existing.State == StateCorrectionRequired && compactStartCorrectionResume(ctx, repo, existing, requested) {
		return true
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
	return existing.Generation == requested.Generation &&
		compactStartInitialSnapshotsEqual(existing, requested) &&
		equalStrings(existing.GenesisPaths, requested.GenesisPaths) &&
		existing.PolicyHash == requested.PolicyHash && existing.RiskLevel == requested.RiskLevel &&
		equalStrings(existing.SelectedLenses, requested.SelectedLenses) &&
		existing.OriginalChangedLines == requested.OriginalChangedLines &&
		existing.CorrectionBudget == requested.CorrectionBudget && reflect.DeepEqual(existing.Recovery, requested.Recovery)
}

func (store CompactStore) StatePath() string { return filepath.Join(store.Dir, "review-state.json") }

func (store CompactStore) ReceiptPath() string {
	return filepath.Join(store.Dir, "review-receipt.json")
}

func (store CompactStore) Load() (CompactRecord, error) {
	payload, err := os.ReadFile(store.StatePath())
	if err != nil {
		return CompactRecord{}, err
	}
	return parseCompactRecord(payload, store.lineageID)
}

func (store CompactStore) Replace(expectedRevision, operation string, next CompactState) (string, error) {
	if strings.TrimSpace(operation) == "" {
		return "", errors.New("compact review operation is required")
	}
	if err := next.Validate(); err != nil {
		return "", fmt.Errorf("%w: %v", ErrInvalidSuccessor, err)
	}
	if store.lineageID != "" && next.LineageID != store.lineageID {
		return "", fmt.Errorf("%w: compact lineage does not match store", ErrInvalidSuccessor)
	}
	lock, err := acquireStoreLock(store.lockPath)
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
		if err := validateCompactRepositoryEvidence(context.Background(), store.repo, current, next, operation); err != nil {
			return "", fmt.Errorf("%w: %v", ErrInvalidSuccessor, err)
		}
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

func parseCompactRecord(payload []byte, lineageID string) (CompactRecord, error) {
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	var record CompactRecord
	if err := decoder.Decode(&record); err != nil {
		return CompactRecord{}, err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return CompactRecord{}, errors.New("multiple JSON values in compact review state")
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
	want, _, err := makeCompactRecord(record.State)
	if err != nil || want.Revision != record.Revision {
		return CompactRecord{}, errors.New("compact review state checksum mismatch")
	}
	return record, nil
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
	if err := store.installTransportRecord(ctx, validated.Record); err != nil {
		return CompactRecord{}, err
	}
	if validated.Receipt != nil {
		if err := WriteCompactReceiptAtomic(store.ReceiptPath(), *validated.Receipt); err != nil {
			return CompactRecord{}, err
		}
	}
	return store.Load()
}

func (store CompactStore) installTransportRecord(ctx context.Context, record CompactRecord) error {
	lock, err := acquireStoreLock(store.lockPath)
	if err != nil {
		return err
	}
	defer lock.release()
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
