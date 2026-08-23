package reviewtransaction

import (
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
	// Ordinary STATUS is compact-only. Historical v1/v3 records remain
	// inspectable through their explicit compatibility owners, but they never
	// compete with, corrupt, or select an ordinary compact lifecycle.
	return targetStatusAuthorityView{compact: compact, legacy: map[string]targetStatusCandidate{}}, nil
}

func loadCompactTargetStatusCandidates(ctx context.Context, repo, lineageID string) (map[string]targetStatusCandidate, error) {
	stores, err := DiscoverCompactStores(ctx, repo)
	if err != nil {
		return nil, err
	}
	storeByLineage := make(map[string]CompactStore, len(stores))
	for _, store := range stores {
		if _, duplicate := storeByLineage[store.lineageID]; duplicate {
			return nil, fmt.Errorf("multiple compact authority locations for lineage %q", store.lineageID) // refusal:by-design world-action: duplicate compact authority roots are an integrity failure that maintainers must repair before any lifecycle command can select one
		}
		storeByLineage[store.lineageID] = store
	}
	records := make(map[string]CompactRecord, len(stores))
	selected := []CompactStore{}
	if lineageID == "" {
		// A separate, locally scoped store map: compactAuthorityLeaves selects
		// leaves from whichever store map it is given, so the shared
		// storeByLineage (needed unfiltered by the explicit-selector branch
		// below, including for a caller naming this exact quarantined lineage)
		// must not be the one used here.
		//
		// A record this read cannot decode is left out of the graph entirely.
		// Status is the surface an operator reaches for when something is
		// wrong, so it is the last surface that may answer "everything is
		// unavailable" because one entry is (2234, 2270, 2456).
		healthyStoreByLineage := make(map[string]CompactStore, len(stores))
		for _, store := range stores {
			record, loadErr := store.LoadContext(ctx)
			if loadErr != nil {
				// Only content failures quarantine. Not being able to READ the
				// store is a different fact from an entry being damaged, and
				// status has to keep saying so.
				if IsCompactAuthorityOperationalFailure(loadErr) {
					return nil, loadErr
				}
				continue
			}
			records[record.State.LineageID] = record
			healthyStoreByLineage[record.State.LineageID] = store
		}
		selected = compactAuthorityLeaves(records, healthyStoreByLineage)
	} else if store, ok := storeByLineage[lineageID]; ok {
		// An explicit selector keeps unrelated inventory isolated, while still
		// validating every recovery edge in the selected lineage's ancestry.
		cursor := store
		selected = append(selected, store)
		priorSchema := map[string]bool{}
		for {
			if _, seen := records[cursor.lineageID]; seen || priorSchema[cursor.lineageID] {
				return nil, errors.New("invalid compact authority graph: recovery cycle")
			}
			record, loadErr := cursor.LoadContext(ctx)
			if loadErr != nil {
				semantic, priorSchemaRecord := compactPriorSchemaLoadFailure(loadErr)
				if !priorSchemaRecord {
					return nil, loadErr
				}
				// A prior-schema record — provably frozen by an earlier
				// release under the retired snapshot-identity formula and
				// still self-consistent under it — is inert history, not
				// damage. It cannot govern this target, so it never fails the
				// selection closed by itself; the walk keeps auditing the
				// rest of the ancestry so any genuinely corrupted deeper
				// record still does.
				priorSchema[cursor.lineageID] = true
				if semantic.PriorSchemaPredecessorLineageID == "" {
					break
				}
				predecessor, exists := storeByLineage[semantic.PriorSchemaPredecessorLineageID]
				if !exists {
					return nil, fmt.Errorf("invalid compact authority graph: dangling predecessor for %q", cursor.lineageID)
				}
				cursor = predecessor
				continue
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
		// The named lineage's OWN ancestry still has to validate completely.
		// Scoping the refusal is not relaxing it: what changes is whose
		// business a defect is, never whether a defect is tolerated.
		violations, _ := compactAuthorityGraphViolations(records)
		for lineage, record := range records {
			// Tolerate exactly the one violation a prior-schema gap explains:
			// the dangling-predecessor defect for that proven absence. Every
			// other violation on the same lineage keeps blocking.
			if violation, carried := violations[lineage]; carried && compactPriorSchemaToleratedViolation(lineage, violation, record, priorSchema) {
				delete(violations, lineage)
			}
		}
		if carrier, cause := compactAuthorityBlockingCause(records, violations, lineageID); cause != nil {
			if carrier == lineageID {
				return nil, fmt.Errorf(
					"compact authority lineage %q cannot govern: %w. Every other lineage is unaffected; see this entry's own diagnosis and sanctioned exits with `gentle-ai review inspect-authority`",
					lineageID, cause)
			}
			return nil, fmt.Errorf(
				"compact authority lineage %q cannot govern because the entry %q it recovers from carries: %w. Every lineage that does not recover through %q is unaffected; see that entry's own diagnosis and sanctioned exits with `gentle-ai review inspect-authority`",
				lineageID, carrier, cause, carrier)
		}
		if priorSchema[lineageID] {
			// The named lineage ITSELF is prior-schema history, so it owns no
			// live authority: report no candidate and let status reclassify
			// the live target through its one lifted-restriction recursion
			// (#2645), which lands on the same fresh-start route any
			// unrelated target takes. A live current-schema lineage whose
			// ancestors are prior-schema falls through and keeps its
			// authority. The prior-schema records stay untouched on disk as
			// inert history.
			return map[string]targetStatusCandidate{}, nil
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
	for attempt := 0; attempt < targetStatusCompactAuthorityReadAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return targetStatusCandidate{}, err
		}
		targetStatusCompactAuthorityReadHook(store.lineageID, "after-state", attempt)
		observed, err := store.LoadContext(ctx)
		if err != nil {
			return targetStatusCandidate{}, err
		}
		if record.Revision != observed.Revision || !compactStateEqual(record.State, observed.State) {
			record = observed
			continue
		}
		copy := observed
		return targetStatusCandidate{version: AuthorityVersionCompact, lineage: observed.State.LineageID, compact: &copy}, nil
	}
	return targetStatusCandidate{}, fmt.Errorf(
		"%w: compact target status authority %q did not stabilize after %d reads",
		ErrConcurrentUpdate, store.lineageID, targetStatusCompactAuthorityReadAttempts,
	)
}

/*
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

*/

type compactTerminalHistoryProjection uint8

const (
	compactTerminalHistoryUnrelated compactTerminalHistoryProjection = iota
	compactTerminalHistoryScopeChanged
)

// projectCompactTerminalHistory compares receipt-validated historical
// authority with the one request-scoped live snapshot. Frozen intended paths
// remain historical proof; they are never replayed against current tracking or
// filesystem membership.
func projectCompactTerminalHistory(state CompactState, live Snapshot) compactTerminalHistoryProjection {
	if live.BaseTree == state.CurrentSnapshot.CandidateTree {
		// The reviewed bytes and modes are now the immutable HEAD base. A clean
		// target or a disjoint next slice is not an applicability claim on the
		// historical receipt.
		if len(live.Paths) == 0 || classifyCompactPathSetRelation(state.CorrectionScopePaths(), live.Paths) == compactPathsDisjoint {
			return compactTerminalHistoryUnrelated
		}
		return compactTerminalHistoryScopeChanged
	}

	relation := classifyCompactTargetRelation(state.CurrentSnapshot, live, state.CorrectionScopePaths(), compactTargetRelationEvidence{})
	if relation.Kind != compactTargetUnsafe {
		return compactTerminalHistoryScopeChanged
	}
	// A projection, kind, or base mismatch can make the aggregate relation
	// unsafe even when live work still contracts or overlaps immutable genesis
	// scope. That is related evolution and must not be claimed as unrelated.
	if len(live.Paths) > 0 && relation.Paths != compactPathsDisjoint && relation.Paths != compactPathsInvalid {
		return compactTerminalHistoryScopeChanged
	}
	return compactTerminalHistoryUnrelated
}

func compactLiveTargetMatchesValidatedSnapshot(state CompactState, live Snapshot, requireCurrentCandidate bool) bool {
	initial := state.InitialSnapshot
	sideBandMatches := requireCurrentCandidate ||
		(equalStrings(initial.IntendedUntracked, live.IntendedUntracked) &&
			initial.IntendedUntrackedProof == live.IntendedUntrackedProof)
	return compactTargetProjectionsCompatible(initial.Kind, initial.Projection, live.Kind, live.Projection) &&
		compactStartTargetKindsCompatible(initial.Kind, live.Kind) &&
		initial.BaseTree == live.BaseTree && (!requireCurrentCandidate || state.CurrentSnapshot.CandidateTree == live.CandidateTree) &&
		pathsAreSubset(live.Paths, state.CorrectionScopePaths()) == nil && sideBandMatches && len(live.LedgerIDs) == 0
}

func legacyLiveTargetMatchesValidatedSnapshot(transaction Transaction, live Snapshot) bool {
	genesis := transaction.GenesisPaths
	if len(genesis) == 0 {
		genesis = transaction.Snapshot.Paths
	}
	kindsMatch := compactStartTargetKindsCompatible(transaction.Snapshot.Kind, live.Kind) ||
		transaction.Snapshot.Kind == TargetFixDiff && (live.Kind == TargetCurrentChanges || live.Kind == TargetBaseDiff)
	return compactTargetProjectionsCompatible(transaction.Snapshot.Kind, transaction.Snapshot.Projection, live.Kind, live.Projection) &&
		kindsMatch && transaction.BaseTree == live.BaseTree &&
		transaction.FinalCandidateTree == live.CandidateTree && pathsAreSubset(live.Paths, genesis) == nil &&
		len(live.LedgerIDs) == 0
}

func classifyCompactCorrectionTargetForStatus(ctx context.Context, repo string, existing CompactState, live Snapshot) (compactCorrectionTargetClaim, error) {
	requested := existing
	requested.InitialSnapshot = live
	return classifyCompactCorrectionTarget(ctx, repo, existing, requested, true)
}

// compactPriorSchemaLoadFailure classifies one load failure as provably
// prior-schema: parseCompactRecord's forensic pass (#2743) confirmed the
// stored snapshot identities equal the retired pre-fbb55080 formula's own
// recomputation and the state validates once coherently re-minted. Anything
// else — unrecognized identities, structural damage, IO failure — is not
// prior schema and keeps failing closed exactly as before.
func compactPriorSchemaLoadFailure(err error) (*CompactSemanticStateError, bool) {
	var semantic *CompactSemanticStateError
	if !errors.As(err, &semantic) || !semantic.OutdatedIdentity {
		return nil, false
	}
	return semantic, true
}

// compactPriorSchemaToleratedViolation reports whether one carried violation
// is exactly the dangling-predecessor defect a proven prior-schema gap
// explains: the record's recovery predecessor is prior-schema, and the
// violation is the one compactAuthorityGraphViolations records for that
// missing predecessor. Any other violation on the same lineage keeps
// blocking.
func compactPriorSchemaToleratedViolation(lineage string, violation error, record CompactRecord, priorSchema map[string]bool) bool {
	if record.State.Recovery == nil || !priorSchema[record.State.Recovery.PredecessorLineageID] {
		return false
	}
	return violation != nil && violation.Error() == fmt.Sprintf("dangling predecessor for %q", lineage)
}

func targetStatusFailure(base TargetStatusResult, err error) (TargetStatusResult, error) {
	if IsCompactAuthorityOperationalFailure(err) {
		return TargetStatusResult{}, err
	}
	return corruptedTargetStatus(base), nil
}
