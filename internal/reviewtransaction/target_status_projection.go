package reviewtransaction

import (
	"bytes"
	"context"
	"errors"
	"fmt"
)

// targetStatusCompactAuthorityReadHook is a deterministic test seam around
// the read-only compact authority observation. Production leaves it inert.
var targetStatusCompactAuthorityReadHook = func(string, string, int) {}

const targetStatusCompactAuthorityReadAttempts = 3

// targetStatusAuthorityView is the immutable authority side of one status
// request. Compact records, graph edges, receipts, and finalize journals are
// observed coherently before projection. Legacy stores remain available so an
// approved receipt is inspected only after pure target matching selects it.
type targetStatusAuthorityView struct {
	compact map[string]targetStatusCandidate
	legacy  map[string]targetStatusCandidate
}

func loadTargetStatusAuthorityView(ctx context.Context, repo string, request TargetStatusRequest) (targetStatusAuthorityView, error) {
	compact, err := loadCompactTargetStatusCandidates(ctx, repo, request.LineageID)
	if err != nil {
		return targetStatusAuthorityView{}, fmt.Errorf("load compact target status authority: %w", err)
	}
	legacy, err := loadLegacyTargetStatusCandidates(ctx, repo, request.LineageID)
	if err != nil {
		return targetStatusAuthorityView{}, fmt.Errorf("load legacy target status authority: %w", err)
	}
	for lineage := range compact {
		if _, mixed := legacy[lineage]; mixed {
			return targetStatusAuthorityView{}, fmt.Errorf("lineage %q has mixed compact and legacy authority", lineage)
		}
	}
	return targetStatusAuthorityView{compact: compact, legacy: legacy}, nil
}

func loadCompactTargetStatusCandidates(ctx context.Context, repo, lineageID string) (map[string]targetStatusCandidate, error) {
	stores, err := DiscoverCompactStores(ctx, repo)
	if err != nil {
		return nil, err
	}
	storeByLineage := make(map[string]CompactStore, len(stores))
	for _, store := range stores {
		storeByLineage[store.lineageID] = store
	}

	records := make(map[string]CompactRecord, len(stores))
	selected := []CompactStore{}
	if lineageID == "" {
		for _, store := range stores {
			record, loadErr := store.LoadContext(ctx)
			if loadErr != nil {
				return nil, loadErr
			}
			records[record.State.LineageID] = record
		}
		selected, err = compactAuthorityLeaves(records, storeByLineage)
		if err != nil {
			return nil, err
		}
	} else if store, ok := storeByLineage[lineageID]; ok {
		// An explicit selector keeps unrelated inventory isolated, while still
		// validating every recovery edge in the selected lineage's ancestry.
		cursor := store
		selected = append(selected, store)
		for {
			if _, seen := records[cursor.lineageID]; seen {
				return nil, errors.New("invalid compact authority graph: recovery cycle")
			}
			record, loadErr := cursor.LoadContext(ctx)
			if loadErr != nil {
				return nil, loadErr
			}
			records[record.State.LineageID] = record
			if record.State.Recovery == nil {
				break
			}
			predecessor, exists := storeByLineage[record.State.Recovery.PredecessorLineageID]
			if !exists {
				return nil, fmt.Errorf("invalid compact authority graph: dangling predecessor for %q", record.State.LineageID)
			}
			cursor = predecessor
		}
		chainStores := make(map[string]CompactStore, len(records))
		for lineage := range records {
			chainStores[lineage] = storeByLineage[lineage]
		}
		if _, graphErr := compactAuthorityLeaves(records, chainStores); graphErr != nil {
			return nil, graphErr
		}
	}

	candidates := make(map[string]targetStatusCandidate, len(selected))
	for _, store := range selected {
		record := records[store.lineageID]
		candidate, loadErr := loadStableCompactTargetStatusCandidate(ctx, store, record)
		if loadErr != nil {
			return nil, loadErr
		}
		candidates[candidate.lineage] = candidate
	}
	return candidates, nil
}

func loadStableCompactTargetStatusCandidate(ctx context.Context, store CompactStore, initial CompactRecord) (targetStatusCandidate, error) {
	record := initial
	var lastSemanticError error
	for attempt := 0; attempt < targetStatusCompactAuthorityReadAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return targetStatusCandidate{}, err
		}
		targetStatusCompactAuthorityReadHook(store.lineageID, "after-state", attempt)
		if err := ctx.Err(); err != nil {
			return targetStatusCandidate{}, err
		}

		first, firstErr := inspectCompactTargetArtifacts(ctx, store, record.State, "first", attempt)
		if err := ctx.Err(); err != nil {
			return targetStatusCandidate{}, err
		}
		observed, loadErr := store.LoadContext(ctx)
		if loadErr != nil {
			return targetStatusCandidate{}, loadErr
		}
		if !compactTargetStatusRecordsEqual(record, observed) {
			record = observed
			lastSemanticError = nil
			continue
		}

		if firstErr != nil && compactAuthorityOperationalFailure(firstErr) {
			return targetStatusCandidate{}, firstErr
		}
		second, secondErr := inspectCompactTargetArtifacts(ctx, store, observed.State, "second", attempt)
		if err := ctx.Err(); err != nil {
			return targetStatusCandidate{}, err
		}
		if secondErr != nil && compactAuthorityOperationalFailure(secondErr) {
			return targetStatusCandidate{}, secondErr
		}
		// The first receipt/journal pair precedes the second state observation,
		// while the second pair follows it. Equal raw identities, canonical
		// content, and existence therefore make that state observation the
		// linearization point for the complete authority view.
		if !compactTargetArtifactSetsEqual(first, second) {
			record = observed
			lastSemanticError = nil
			continue
		}

		observationErr := errors.Join(firstErr, secondErr)
		if observationErr != nil {
			lastSemanticError = observationErr
			record = observed
			continue
		}

		copy := observed
		return targetStatusCandidate{
			version: AuthorityVersionCompact, lineage: observed.State.LineageID, compact: &copy,
			receiptIdentity: second.receipt.artifact.identity, receiptPublished: second.receipt.published,
			receiptCanonical:  bytes.Equal(second.receipt.artifact.content, second.receipt.artifact.canonical),
			receiptReplayable: second.receipt.replayable, pendingFinalize: second.journal.pending,
		}, nil
	}
	if lastSemanticError != nil {
		return targetStatusCandidate{}, lastSemanticError
	}
	return targetStatusCandidate{}, fmt.Errorf(
		"%w: compact target status authority %q did not stabilize after %d reads",
		ErrConcurrentUpdate, store.lineageID, targetStatusCompactAuthorityReadAttempts,
	)
}

type compactTargetFinalizeJournalObservation struct {
	artifact compactTargetArtifactObservation
	pending  bool
}

type compactTargetArtifactSetObservation struct {
	receipt compactTargetReceiptObservation
	journal compactTargetFinalizeJournalObservation
}

func inspectCompactTargetArtifacts(
	ctx context.Context,
	store CompactStore,
	state CompactState,
	pass string,
	attempt int,
) (compactTargetArtifactSetObservation, error) {
	receipt, receiptErr := inspectCompactTargetReceipt(store, state)
	targetStatusCompactAuthorityReadHook(store.lineageID, "after-"+pass+"-receipt", attempt)
	if err := ctx.Err(); err != nil {
		return compactTargetArtifactSetObservation{}, err
	}
	journal, journalErr := inspectCompactTargetFinalizeJournal(ctx, store)
	targetStatusCompactAuthorityReadHook(store.lineageID, "after-"+pass+"-journal", attempt)
	if pass == "first" {
		// Preserve the original test seam while the more precise hooks bind the
		// transition between individual artifact reads.
		targetStatusCompactAuthorityReadHook(store.lineageID, "after-artifacts", attempt)
	}
	if err := ctx.Err(); err != nil {
		return compactTargetArtifactSetObservation{}, err
	}
	return compactTargetArtifactSetObservation{receipt: receipt, journal: journal}, errors.Join(receiptErr, journalErr)
}

func inspectCompactTargetFinalizeJournal(ctx context.Context, store CompactStore) (compactTargetFinalizeJournalObservation, error) {
	payload, journal, exists, err := store.loadFinalizeAttemptJournalReadOnly(ctx)
	if !exists {
		return compactTargetFinalizeJournalObservation{}, err
	}
	observation := compactTargetFinalizeJournalObservation{
		artifact: newCompactTargetArtifactObservation(payload, nil),
	}
	if err != nil {
		return observation, err
	}
	canonical, marshalErr := marshalFinalizeAttemptJournal(journal)
	if marshalErr != nil {
		return observation, marshalErr
	}
	observation.artifact.canonical = canonical
	observation.pending = latestPendingFinalizeAttempt(journal) != nil
	return observation, nil
}

func compactTargetArtifactSetsEqual(left, right compactTargetArtifactSetObservation) bool {
	return compactTargetArtifactObservationsEqual(left.receipt.artifact, right.receipt.artifact) &&
		left.receipt.published == right.receipt.published && left.receipt.replayable == right.receipt.replayable &&
		compactTargetArtifactObservationsEqual(left.journal.artifact, right.journal.artifact) &&
		left.journal.pending == right.journal.pending
}

func compactTargetStatusRecordsEqual(left, right CompactRecord) bool {
	return left.Revision == right.Revision &&
		left.State.InitialSnapshot.Identity == right.State.InitialSnapshot.Identity &&
		left.State.CurrentSnapshot.Identity == right.State.CurrentSnapshot.Identity &&
		compactStateEqual(left.State, right.State)
}

func loadLegacyTargetStatusCandidates(ctx context.Context, repo, lineageID string) (map[string]targetStatusCandidate, error) {
	stores, err := DiscoverAuthoritativeStores(ctx, repo)
	if err != nil {
		return nil, err
	}
	candidates := make(map[string]targetStatusCandidate, len(stores))
	for _, store := range stores {
		if lineageID != "" && store.lineageID != lineageID {
			continue
		}
		chain, loadErr := store.LoadChain()
		if loadErr != nil {
			return nil, loadErr
		}
		transaction := chain.Records[len(chain.Records)-1].Transaction
		copy := chain
		storeCopy := store
		candidates[transaction.LineageID] = targetStatusCandidate{
			version: AuthorityVersionLegacy, lineage: transaction.LineageID, legacy: &copy, legacyStore: &storeCopy,
		}
	}
	return candidates, nil
}

func compactLiveTargetMatchesValidatedSnapshot(state CompactState, live Snapshot, requireCurrentCandidate bool) bool {
	initial := state.InitialSnapshot
	proof := initial.IntendedUntrackedProof
	if requireCurrentCandidate {
		proof = state.CurrentSnapshot.IntendedUntrackedProof
	}
	return initial.Projection == live.Projection && compactStartTargetKindsCompatible(initial.Kind, live.Kind) &&
		initial.BaseTree == live.BaseTree && (!requireCurrentCandidate || state.CurrentSnapshot.CandidateTree == live.CandidateTree) &&
		pathsAreSubset(live.Paths, state.GenesisPaths) == nil && equalStrings(initial.IntendedUntracked, live.IntendedUntracked) &&
		proof == live.IntendedUntrackedProof && len(live.LedgerIDs) == 0
}

func legacyLiveTargetMatchesValidatedSnapshot(transaction Transaction, live Snapshot) bool {
	genesis := transaction.GenesisPaths
	if len(genesis) == 0 {
		genesis = transaction.Snapshot.Paths
	}
	kindsMatch := compactStartTargetKindsCompatible(transaction.Snapshot.Kind, live.Kind) ||
		transaction.Snapshot.Kind == TargetFixDiff && (live.Kind == TargetCurrentChanges || live.Kind == TargetBaseDiff)
	return transaction.Snapshot.Projection == live.Projection && kindsMatch && transaction.BaseTree == live.BaseTree &&
		transaction.FinalCandidateTree == live.CandidateTree && pathsAreSubset(live.Paths, genesis) == nil &&
		equalStrings(transaction.Snapshot.IntendedUntracked, live.IntendedUntracked) &&
		transaction.Snapshot.IntendedUntrackedProof == live.IntendedUntrackedProof && len(live.LedgerIDs) == 0
}

func classifyCompactCorrectionTargetForStatus(ctx context.Context, repo string, existing CompactState, live Snapshot) (compactCorrectionTargetClaim, error) {
	requested := existing
	requested.InitialSnapshot = live
	return classifyCompactCorrectionTarget(ctx, repo, existing, requested, true)
}

func targetStatusFailure(base TargetStatusResult, err error) (TargetStatusResult, error) {
	if compactAuthorityOperationalFailure(err) {
		return TargetStatusResult{}, err
	}
	return corruptedTargetStatus(base), nil
}
