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

// RecoveryTargetUnchanged reports whether err is the unchanged-target escalated
// recovery refusal.
//
// The sentence itself stays here, unchanged, because it is persisted verbatim
// into reconcile records as InvalidRecoveryEdge.ValidationError -- an authority
// artifact must not carry operator instructions. This predicate lets the command
// surface recognize the refusal and add the continuation there, where the exact
// selectors the operator just supplied are still in hand.
func RecoveryTargetUnchanged(err error) bool {
	return errors.Is(err, errCompactRecoveryTargetUnchanged)
}

// errCompactApprovedRecoveryScopeUnchanged identifies the unchanged-scope
// approved recovery refusal, and errCompactRecoveryPredecessorNotInvalidated
// the invalidated-disposition refusal over a predecessor that is not
// invalidated. Both sentences stay exactly as they are: like
// errCompactRecoveryTargetUnchanged above, they are authority-layer statements
// of fact, and an authority artifact must not carry operator instructions. The
// predicates below let the command surface recognize each refusal and add the
// continuation there, where the selectors the operator just supplied and the
// predecessor's real state are still in hand.
var errCompactApprovedRecoveryScopeUnchanged = errors.New("approved predecessor scope has not changed")

var errCompactRecoveryPredecessorNotInvalidated = errors.New("recovery requires an invalidated predecessor")

// ApprovedRecoveryScopeUnchanged reports whether err is the refusal of a
// scope-changed recovery whose approved predecessor already approved exactly
// the candidate the successor would freeze.
func ApprovedRecoveryScopeUnchanged(err error) bool {
	return errors.Is(err, errCompactApprovedRecoveryScopeUnchanged)
}

// RecoveryPredecessorNotInvalidated reports whether err is the refusal of an
// `--disposition invalidated` recovery whose predecessor is in some other
// state.
func RecoveryPredecessorNotInvalidated(err error) bool {
	return errors.Is(err, errCompactRecoveryPredecessorNotInvalidated)
}

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
// tolerated only at its exact nesting level: top-level entries remain top-level
// and "recovery.review_start" remains nested inside recovery provenance.
// Historical records that carry them load read-only with the retired content
// dropped from the in-memory view only; persisted bytes remain untouched
// because the tolerant read never rewrites authority. New authority state never
// persists these fields.
var compactRetiredStateFieldPaths = map[string]struct{}{
	"candidate_artifact_required": {},
	"zero_edit_escalation":        {},
	// risk_source was the persisted record of how a record's risk level had
	// been decided. Risk is derived from the frozen candidate now, so the
	// field carries nothing the current schema needs; records written before
	// it was dropped still carry it, and their revisions still bind their own
	// bytes exactly (issue 2399).
	"risk_source":           {},
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
	// BeforeCreate runs under the START lock only after existing-authority
	// selection is exhausted and immediately before a new record is built. It
	// may validate derived response material without running for resumes.
	BeforeCreate func() error
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
	// An ordinary one-parent delivery commit (squash or rebase merge) is an
	// accepted release-scope HEAD, not only a two-parent merge commit
	// (issue-1816); a root commit (zero parents) is still refused.
	if _, err := runGit(ctx, root, nil, nil, "rev-parse", "--verify", commit+"^1^{commit}"); err != nil {
		return Snapshot{}, errors.New("release-scope recovery requires HEAD to have at least one parent commit")
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
	if request.Disposition == RecoveryFinalVerificationRetry {
		return CompactRecord{}, errors.New("final_verification_retry is provider-only; use the dedicated final-verification retry operation")
	}
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
	approvedStagedScopeRecovery := request.Disposition == RecoveryScopeChanged &&
		compactApprovedStagedScopeRecoveryShape(predecessor.State, request.Successor.InitialSnapshot)
	correctionStagedScopeRecovery := request.Disposition == RecoveryScopeChanged &&
		compactCorrectionRequiredStagedScopeRecoveryShape(predecessor.State, request.Successor.InitialSnapshot)
	stagedScopeRecovery := approvedStagedScopeRecovery || correctionStagedScopeRecovery
	var predecessorReceipt compactTargetReceiptObservation
	if stagedScopeRecovery {
		if request.MaintainerAuthorization != compactApprovedStagedScopeRecoveryAuthorizationBinding(
			request.PredecessorLineageID, predecessor.Revision, request.Successor.InitialSnapshot.Identity,
			request.Successor.LineageID, request.Actor, request.Reason,
		) {
			return CompactRecord{}, errors.New("staged scope recovery requires an exact successor-bound maintainer authorization") // refusal:by-design human-authority: only a maintainer can authorize this exact successor edge
		}
	}
	if approvedStagedScopeRecovery {
		predecessorReceipt, err = inspectCompactTargetReceipt(predecessorStore, predecessor.State)
		if err != nil || !predecessorReceipt.published ||
			!bytes.Equal(predecessorReceipt.artifact.content, predecessorReceipt.artifact.canonical) {
			return CompactRecord{}, errors.New("approved staged scope recovery requires a canonical published predecessor receipt")
		}
		eligible, eligibilityErr := compactApprovedStagedScopeRecovery(ctx, successorStore.repo, predecessor.State, request.Successor.InitialSnapshot)
		if eligibilityErr != nil {
			return CompactRecord{}, eligibilityErr
		}
		if !eligible {
			return CompactRecord{}, errors.New("approved staged scope recovery is not a complete same-base index expansion")
		}
	}
	correctionLines := 0
	if correctionStagedScopeRecovery {
		var eligible bool
		correctionLines, eligible, err = compactCorrectionRequiredStagedScopeRecovery(ctx, successorStore.repo, predecessor.State, request.Successor.InitialSnapshot)
		if err != nil {
			return CompactRecord{}, err
		}
		if !eligible {
			return CompactRecord{}, errors.New("correction-required staged scope recovery is not a complete same-base index expansion") // refusal:by-design world-action: the operator must restage the exact same-base correction scope before retrying
		}
	}
	if predecessor.State.State == StateCorrectionRequired && request.Disposition != RecoveryEscalated && !correctionStagedScopeRecovery && request.MaintainerAuthorization != compactRecoveryAuthorizationBinding(request.PredecessorLineageID, predecessor.Revision, request.Successor.InitialSnapshot.Identity, request.Actor, request.Reason) {
		return CompactRecord{}, errors.New("correction-required scope recovery requires an exact maintainer authorization binding")
	}
	if !sameRecoveryProjection(predecessor.State.InitialSnapshot.Projection, request.Successor.InitialSnapshot.Projection) &&
		request.Disposition != RecoveryEscalated && !stagedScopeRecovery {
		return CompactRecord{}, errors.New("recovery successor must retain the predecessor projection")
	}
	if !sameRecoveryProjection(predecessor.State.InitialSnapshot.Projection, request.Successor.InitialSnapshot.Projection) &&
		!stagedScopeRecovery &&
		request.MaintainerAuthorization != compactRecoveryAuthorizationBinding(request.PredecessorLineageID, predecessor.Revision, request.Successor.InitialSnapshot.Identity, request.Actor, request.Reason) {
		return CompactRecord{}, compactRecoveryAuthorizationError(request.Successor.InitialSnapshot)
	}
	// Every shape the three comparisons above do not cover still records the
	// caller's authorization verbatim in the provenance below, so a supplied
	// one has to bind here or nothing is written. Absent stays allowed: this
	// is the only asymmetry, and it is the one that keeps the self-minting
	// recovery shapes working.
	if strings.TrimSpace(request.MaintainerAuthorization) != "" &&
		!compactRecoverySuppliedAuthorizationBinds(request.MaintainerAuthorization, request.PredecessorLineageID,
			predecessor.Revision, request.Successor.InitialSnapshot.Identity, request.Successor.LineageID,
			request.Actor, request.Reason) {
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
	if correctionStagedScopeRecovery {
		request.Successor.CorrectionBudget = predecessor.State.CorrectionBudget
		request.Successor.Recovery.ConsumedCorrectionAttempts = MaxCompactCorrectionAttempts
		request.Successor.Recovery.ConsumedCorrectionLines = correctionLines
	}
	if request.Disposition == RecoveryEscalated {
		evidence, eligible, evidenceErr := deriveCompactRecoveredEvidence(ctx, successorStore.repo, predecessorStore, predecessor, request.Successor)
		if evidenceErr != nil {
			return CompactRecord{}, evidenceErr
		}
		if eligible {
			importCompactRecoveredEvidence(&request.Successor, predecessor.State, evidence)
		}
	}
	// The recovery graph is scoped the same way every other authority walk is
	// (#2495, finished here for #2741/#2743): a foreign record nobody can read
	// is ABSENT from the graph, never a repository-wide refusal issued to a
	// healthy, unrelated recovery. scanCompactAuthority still propagates
	// operational failures, and the predecessor's own readability was already
	// proven by its explicit load above.
	scan, err := scanCompactAuthority(ctx, repo)
	if err != nil {
		return CompactRecord{}, err
	}
	if existingErr == nil {
		if compactStateEqual(existing.State, request.Successor) {
			return existing, nil
		}
		return CompactRecord{}, errors.New("recovery successor lineage already exists with different authority")
	}
	for _, record := range scan.records {
		if record.State.Recovery != nil && record.State.Recovery.PredecessorLineageID == request.PredecessorLineageID {
			return CompactRecord{}, errors.New("recovery predecessor already has successor")
		}
	}
	if err := request.Successor.Validate(); err != nil {
		return CompactRecord{}, err
	}
	if request.Successor.Recovery.Evidence == nil {
		if !compactPristineReviewing(request.Successor) || len(request.Successor.CorrectionAttempts) != 0 || request.Successor.CumulativeCorrectionLines != 0 {
			return CompactRecord{}, errors.New("recovery successor must start as a fresh reviewing authority")
		}
	} else if request.Successor.State != StateValidating {
		return CompactRecord{}, errors.New("recovery successor with imported evidence must start in validating state")
	}
	if err := validateCompactRecoveryEdge(predecessor, request.Successor); err != nil {
		return CompactRecord{}, err
	}
	if request.MaintainerAuthorization == ReleaseScopeRecoveryAuthorization {
		if live, liveErr := BuildReleaseScopeSnapshot(ctx, successorStore.repo); liveErr != nil || !snapshotsEqual(live, request.Successor.InitialSnapshot) {
			return CompactRecord{}, fmt.Errorf("%w: live release scope no longer matches successor", ErrInvalidSuccessor)
		}
	}
	if !sameRecoveryProjection(predecessor.State.InitialSnapshot.Projection, request.Successor.InitialSnapshot.Projection) ||
		request.Successor.InitialSnapshot.Kind == TargetBaseWorkspaceOverlay && request.Successor.InitialSnapshot.Projection == ProjectionStaged {
		if err := validateLiveRecoverySuccessor(ctx, successorStore.repo, request.Successor.InitialSnapshot); err != nil {
			return CompactRecord{}, fmt.Errorf("%w: repository evidence for selected recovery projection changed: %v", ErrInvalidSuccessor, err)
		}
	}
	if err := validateCompactRepositoryEvidence(ctx, successorStore.repo, nil, request.Successor, "review/start"); err != nil {
		return CompactRecord{}, fmt.Errorf("%w: %v", ErrInvalidSuccessor, err)
	}
	if approvedStagedScopeRecovery {
		currentReceipt, receiptErr := inspectCompactTargetReceipt(predecessorStore, predecessor.State)
		if receiptErr != nil || currentReceipt.published != predecessorReceipt.published ||
			!compactTargetArtifactObservationsEqual(currentReceipt.artifact, predecessorReceipt.artifact) {
			return CompactRecord{}, fmt.Errorf("%w: predecessor receipt changed during staged scope recovery", ErrConcurrentUpdate)
		}
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
	live, err := (SnapshotBuilder{Repo: repo}).BuildStoredSnapshot(ctx, target)
	if err != nil {
		return err
	}
	if !snapshotsEqual(live, expected) {
		return errors.New("live target no longer matches the prepared successor")
	}
	return nil
}

func compactRecoveryScopeChanged(previous, next Snapshot) bool {
	relation := classifyCompactTargetRelation(previous, next, previous.Paths, compactTargetRelationEvidence{ExplicitScopeChange: true})
	return relation.Kind != compactTargetSame && relation.Kind != compactTargetUnsafe
}

func compactApprovedStagedScopeRecoveryShape(predecessor CompactState, next Snapshot) bool {
	return predecessor.State == StateApproved && compactStagedScopeRecoverySnapshotShape(predecessor, next)
}

func compactCorrectionRequiredStagedScopeRecoveryShape(predecessor CompactState, next Snapshot) bool {
	return predecessor.State == StateCorrectionRequired && predecessor.ProposedCorrectionLines != nil &&
		!predecessor.CorrectionAttemptConsumed() && compactStagedScopeRecoverySnapshotShape(predecessor, next)
}

func compactStagedScopeRecoverySnapshotShape(predecessor CompactState, next Snapshot) bool {
	initial := predecessor.InitialSnapshot
	initialProjection := initial.Projection
	if initialProjection == "" {
		initialProjection = ProjectionWorkspace
	}
	return initial.Kind == TargetBaseDiff &&
		initialProjection == ProjectionWorkspace && next.Kind == TargetBaseWorkspaceOverlay &&
		next.Projection == ProjectionStaged && next.BaseTree == initial.BaseTree &&
		len(initial.IntendedUntracked) == 0 && len(initial.LedgerIDs) == 0 &&
		len(next.IntendedUntracked) == 0 && len(next.LedgerIDs) == 0 &&
		len(predecessor.GenesisPaths) > 0 && len(next.Paths) > len(predecessor.GenesisPaths) &&
		pathsAreSubset(predecessor.GenesisPaths, next.Paths) == nil &&
		next.CandidateTree != predecessor.CurrentSnapshot.CandidateTree
}

func compactApprovedStagedScopeRecovery(ctx context.Context, repo string, predecessor CompactState, next Snapshot) (bool, error) {
	if !compactApprovedStagedScopeRecoveryShape(predecessor, next) {
		return false, nil
	}
	return compactStagedScopeRecovery(ctx, repo, predecessor, next)
}

func compactCorrectionRequiredStagedScopeRecovery(ctx context.Context, repo string, predecessor CompactState, next Snapshot) (int, bool, error) {
	if !compactCorrectionRequiredStagedScopeRecoveryShape(predecessor, next) {
		return 0, false, nil
	}
	eligible, err := compactStagedScopeRecovery(ctx, repo, predecessor, next)
	if err != nil || !eligible {
		return 0, eligible, err
	}
	builder := SnapshotBuilder{Repo: repo}
	paths, err := builder.changedPaths(ctx, predecessor.CurrentSnapshot.CandidateTree, next.CandidateTree)
	if err != nil {
		return 0, false, err
	}
	lines, err := builder.ChangedLines(ctx, Snapshot{
		BaseTree: predecessor.CurrentSnapshot.CandidateTree, CandidateTree: next.CandidateTree, Paths: paths,
	})
	if err != nil {
		return 0, false, err
	}
	if lines > predecessor.CorrectionBudget {
		return 0, false, fmt.Errorf("staged correction is %d changed lines, exceeding the frozen budget of %d", lines, predecessor.CorrectionBudget) // refusal:by-design world-action: the staged correction must be reduced or receive a separate maintainer decision
	}
	return lines, true, nil
}

func compactStagedScopeRecovery(ctx context.Context, repo string, predecessor CompactState, next Snapshot) (bool, error) {
	committed, err := (SnapshotBuilder{Repo: repo}).Build(ctx, Target{
		Kind: TargetBaseDiff, Projection: ProjectionStaged,
		BaseRef: predecessor.InitialSnapshot.BaseTree, IntendedUntracked: []string{},
	})
	if err != nil {
		return false, err
	}
	return committed.CandidateTree == predecessor.CurrentSnapshot.CandidateTree &&
		pathsAreSubset(predecessor.GenesisPaths, committed.Paths) == nil &&
		len(next.Paths) > len(committed.Paths) && pathsAreSubset(committed.Paths, next.Paths) == nil, nil
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
	// forgedSchemaAuthorization reports a recorded authorization that names
	// compactRecoveryAuthorizationSchema — thereby asserting a maintainer bound
	// this exact edge — while binding nothing. Replay must refuse it, or a
	// forged attestation reads as genuine forever after. Pre-contract free-form
	// text makes no such claim, so it stays outside this predicate and keeps the
	// tolerance compact_reconcile.go already classifies it under.
	//
	// It is applied per disposition rather than up front on purpose: the
	// RecoveryEscalated branch already compares the binding exactly, and it does
	// so only after errCompactRecoveryTargetUnchanged, an ordering
	// classifyCompactRecoveryEdgeAnomalies depends on to tell an unchanged
	// target apart from a malformed authorization.
	forgedSchemaAuthorization := func() bool {
		return strings.HasPrefix(recovery.MaintainerAuthorization, compactRecoveryAuthorizationSchema) &&
			!compactRecoverySuppliedAuthorizationBinds(recovery.MaintainerAuthorization, predecessor.State.LineageID,
				predecessor.Revision, successor.InitialSnapshot.Identity, successor.LineageID,
				recovery.Actor, recovery.Reason)
	}
	if successor.Generation != predecessor.State.Generation+1 {
		return errors.New("recovery successor generation must follow predecessor")
	}
	approvedStagedScopeRecovery := recovery.Disposition == RecoveryScopeChanged &&
		compactApprovedStagedScopeRecoveryShape(predecessor.State, successor.InitialSnapshot)
	correctionStagedScopeRecovery := recovery.Disposition == RecoveryScopeChanged &&
		compactCorrectionRequiredStagedScopeRecoveryShape(predecessor.State, successor.InitialSnapshot)
	stagedScopeRecovery := approvedStagedScopeRecovery || correctionStagedScopeRecovery
	if recovery.ConsumedCorrectionAttempts > 0 && !correctionStagedScopeRecovery {
		return errors.New("only correction-required staged recovery may preserve consumed correction accounting") // refusal:by-design world-action: forged cross-edge accounting cannot be rebound to a different recovery shape
	}
	if !sameRecoveryProjection(predecessor.State.InitialSnapshot.Projection, successor.InitialSnapshot.Projection) &&
		recovery.Disposition != RecoveryEscalated && !stagedScopeRecovery {
		return errors.New("recovery successor must retain the predecessor projection")
	}
	switch recovery.Disposition {
	case RecoveryScopeChanged:
		switch predecessor.State.State {
		case StateApproved:
			previous, next := predecessor.State.CurrentSnapshot, successor.InitialSnapshot
			releaseScope := recovery.MaintainerAuthorization == ReleaseScopeRecoveryAuthorization
			if stagedScopeRecovery {
				if recovery.MaintainerAuthorization != compactApprovedStagedScopeRecoveryAuthorizationBinding(
					predecessor.State.LineageID, predecessor.Revision, next.Identity,
					successor.LineageID, recovery.Actor, recovery.Reason,
				) {
					return errors.New("approved staged scope recovery authorization is not successor-bound")
				}
				break
			}
			if releaseScope && !compactReleaseScopeRecovery(predecessor.State, next) {
				return errors.New("approved recovery target-kind transition is not a complete release scope expansion")
			}
			if !releaseScope && !compactRecoveryScopeChanged(previous, next) {
				return errCompactApprovedRecoveryScopeUnchanged
			}
			if forgedSchemaAuthorization() {
				return compactRecoveryAuthorizationError(next)
			}
		case StateCorrectionRequired:
			if strings.TrimSpace(recovery.MaintainerAuthorization) == "" {
				return errors.New("correction-required scope recovery requires explicit maintainer authorization")
			}
			if correctionStagedScopeRecovery {
				if recovery.MaintainerAuthorization != compactApprovedStagedScopeRecoveryAuthorizationBinding(
					predecessor.State.LineageID, predecessor.Revision, successor.InitialSnapshot.Identity,
					successor.LineageID, recovery.Actor, recovery.Reason,
				) {
					return errors.New("correction-required staged scope recovery authorization is not successor-bound") // refusal:by-design human-authority: only a maintainer can issue the exact successor-bound authorization
				}
				if successor.CorrectionBudget != predecessor.State.CorrectionBudget ||
					recovery.ConsumedCorrectionAttempts != MaxCompactCorrectionAttempts {
					return errors.New("correction-required staged scope recovery did not preserve correction accounting") // refusal:by-design world-action: provider-built authority contradicted its predecessor and requires a code fix
				}
			}
			if forgedSchemaAuthorization() {
				return compactRecoveryAuthorizationError(successor.InitialSnapshot)
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
			return errCompactRecoveryPredecessorNotInvalidated
		}
		if forgedSchemaAuthorization() {
			return compactRecoveryAuthorizationError(successor.InitialSnapshot)
		}
	case RecoveryEscalated:
		if recovery.Evidence != nil {
			if recovery.MaintainerAuthorization != compactRecoveryAuthorizationBinding(predecessor.State.LineageID, predecessor.Revision, successor.InitialSnapshot.Identity, recovery.Actor, recovery.Reason) {
				return compactRecoveryAuthorizationError(successor.InitialSnapshot)
			}
			if err := validateCompactRecoveredEvidenceEdge(predecessor, successor); err != nil {
				return err
			}
			break
		}
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
	case RecoveryFinalVerificationRetry:
		return validateCompactFinalVerificationRetryEdge(predecessor, successor)
	default:
		return errors.New("unsupported recovery disposition")
	}
	return nil
}

func compactEscalatedRecoveryTargetChanged(previous, next Snapshot) bool {
	return previous.CandidateTree != next.CandidateTree && previous.Identity != next.Identity
}

func compactHistoricalFailedValidator(state CompactState) bool {
	if state.State != StateCorrectionRequired || !state.CorrectionAttemptConsumed() || state.ActualCorrectionLines != nil ||
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

// compactRecoverySuppliedAuthorizationBinds reports whether a caller-supplied
// maintainer authorization actually binds to this recovery edge.
//
// The asymmetry it enables is deliberate. An absent authorization is honestly
// absent: several recovery shapes legitimately self-mint actor, reason and
// binding (RunReviewRecover in internal/cli/review_facade.go), so demanding one
// unconditionally would refuse a path that is correct today. A supplied one is
// different, because it is copied verbatim into CompactRecoveryProvenance and
// read afterwards as a maintainer attestation. It must therefore bind or be
// refused: a wrong authorization is worse than none, since an absent field
// cannot lie about who approved what.
//
// ReleaseScopeRecoveryAuthorization is a recognized sentinel rather than a
// binding — RecoverCompactAuthority re-derives the live release scope for it
// and validateCompactRecoveryEdge proves the expansion — so it is exempt.
func compactRecoverySuppliedAuthorizationBinds(authorization, lineage, revision, targetIdentity, successor, actor, reason string) bool {
	if authorization == ReleaseScopeRecoveryAuthorization {
		return true
	}
	return authorization == compactRecoveryAuthorizationBinding(lineage, revision, targetIdentity, actor, reason) ||
		authorization == compactApprovedStagedScopeRecoveryAuthorizationBinding(lineage, revision, targetIdentity, successor, actor, reason)
}

func compactApprovedStagedScopeRecoveryAuthorizationBinding(lineage, revision, targetIdentity, successor, actor, reason string) string {
	return compactRecoveryAuthorizationSchema + "\npredecessor_lineage=" + lineage +
		"\npredecessor_revision=" + revision + "\ntarget_identity=" + targetIdentity +
		"\nsuccessor_lineage=" + successor + "\nactor=" + strings.TrimSpace(actor) +
		"\nreason=" + strings.TrimSpace(reason)
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
	if classifyCompactPathSetRelation(predecessor.GenesisPaths, live.Paths) != compactPathsOverlap {
		return false
	}
	reaches := pathsAreSubset(live.Paths, predecessor.GenesisPaths) != nil
	// An expansion must still be the frozen work: it retains at least one
	// genesis path and reaches past the set. A live scope disjoint from genesis
	// is unrelated work, not a wider view of this lineage, so it must not be
	// admitted here. Worktrees of one repository share the review store and a
	// base tree, so without the retention test an unrelated candidate would be
	// captured by whichever stale lineage happened to be enumerated first.
	return reaches
}

// compactRecoveryContractsGenesisPaths reports whether the live repository
// scope is a pure contraction of predecessor genesis scope: a non-empty strict
// subset with no live path outside genesis. Disjoint or overlapping-different
// path sets never qualify; they remain governed by the expansion rule.
func compactRecoveryContractsGenesisPaths(predecessor CompactState, live Snapshot) bool {
	return classifyCompactPathSetRelation(predecessor.GenesisPaths, live.Paths) == compactPathsContraction
}

// compactAuthorityScan is one read of every compact store in a repository,
// split into the records that could be read and the lineages that could not.
// The split is the point: an entry nobody can read is ABSENT from the graph,
// not a verdict on it. Absence already has a meaning the graph knows, because
// a successor naming a lineage that is not there is a dangling predecessor and
// self-excludes; nothing else has to be invented to describe it.
type compactAuthorityScan struct {
	records    map[string]CompactRecord
	stores     map[string]CompactStore
	unreadable map[string]error
}

// scanCompactAuthority reads every discovered store once. It never fails on a
// record it cannot read, because a repository shares one review store across
// every one of its worktrees: a repository-wide refusal derived from one
// damaged entry is a refusal issued to every worktree, for work none of them
// did (issues 1892, 2014, 2167, 2234, 2270, 2399, 2456).
func scanCompactAuthority(ctx context.Context, repo string) (compactAuthorityScan, error) {
	stores, err := DiscoverCompactStores(ctx, repo)
	if err != nil {
		return compactAuthorityScan{}, err
	}
	scan := compactAuthorityScan{
		records:    make(map[string]CompactRecord, len(stores)),
		stores:     make(map[string]CompactStore, len(stores)),
		unreadable: map[string]error{},
	}
	for _, store := range stores {
		record, loadErr := store.LoadContext(ctx)
		if loadErr != nil {
			// An operational failure is not a damaged record: it is this
			// process being unable to see the store at all. Cancellation, a
			// held lock, a Git failure or a filesystem error say nothing about
			// the entry's content, and quarantining them would turn a
			// transient or environmental problem into a permanent verdict
			// about somebody's authority. Those still propagate.
			if IsCompactAuthorityOperationalFailure(loadErr) {
				return compactAuthorityScan{}, loadErr
			}
			scan.unreadable[store.lineageID] = loadErr
			continue
		}
		scan.records[record.State.LineageID], scan.stores[record.State.LineageID] = record, store
	}
	return scan, nil
}

func CompactAuthorityLeaves(ctx context.Context, repo string) ([]CompactStore, error) {
	scan, err := scanCompactAuthority(ctx, repo)
	if err != nil {
		return nil, err
	}
	return compactAuthorityLeaves(scan.records, scan.stores), nil
}

// CompactAuthorityLineageBlocked reports the reason one named lineage cannot
// govern, and nothing about any other lineage. It is the scoped replacement
// for asking whether the whole repository's authority graph validates: an
// entry that does not apply to the operation under way must not be able to
// refuse it.
//
// It answers nil for a lineage the graph does not carry at all, because "no
// authority here" is not damage; the caller's own discovery already decides
// what to do with an absent lineage.
func CompactAuthorityLineageBlocked(ctx context.Context, repo, lineageID string) error {
	if strings.TrimSpace(lineageID) == "" {
		return nil
	}
	scan, err := scanCompactAuthority(ctx, repo)
	if err != nil {
		return err
	}
	return scan.blocked(lineageID)
}

// blocked names the exact entry that stops one lineage from governing, which
// may be the lineage itself or an ancestor it inherits its authority through.
// The refusal names a runnable continuation because a refusal that names none
// is the one shape this project does not ship.
func (scan compactAuthorityScan) blocked(lineageID string) error {
	if loadErr, unreadable := scan.unreadable[lineageID]; unreadable {
		return compactBlockedLineageError(lineageID, lineageID, loadErr)
	}
	if _, known := scan.records[lineageID]; !known {
		return nil
	}
	violations, _ := compactAuthorityGraphViolations(scan.records)
	carrier, cause := compactAuthorityBlockingCause(scan.records, violations, lineageID)
	if cause == nil {
		return nil
	}
	return compactBlockedLineageError(lineageID, carrier, cause)
}

// compactBlockedLineageError names the entry, the defect, and the ONE command
// that is runnable for every shape of damage: the read-only diagnosis, which
// then names each entry's own sanctioned exit if it has one.
//
// It deliberately does not guess a clearing command. `review abandon` refuses
// a successor holding captured review data and cannot load a truncated record
// at all, so printing it here would send an operator to a command that turns
// around and refuses. A refusal that names an exit which does not work is
// worse than one that names only the diagnosis.
func compactBlockedLineageError(lineageID, carrier string, cause error) error {
	if carrier == lineageID {
		return fmt.Errorf(
			"compact authority lineage %q cannot govern: %w. Every other lineage is unaffected; see this entry's own diagnosis and sanctioned exits with `gentle-ai review inspect-authority`",
			lineageID, cause)
	}
	return fmt.Errorf(
		"compact authority lineage %q cannot govern because the entry %q it recovers from carries: %w. Every lineage that does not recover through %q is unaffected; see that entry's own diagnosis and sanctioned exits with `gentle-ai review inspect-authority`",
		lineageID, carrier, cause, carrier)
}

// compactAuthorityBlockingCause walks one lineage's recovery ancestry and
// reports the first entry that carries a graph defect, together with that
// defect. Walking the ancestry rather than the whole record set is the whole
// scoping change: a defect on a branch this lineage never inherits from is
// somebody else's problem.
func compactAuthorityBlockingCause(
	records map[string]CompactRecord,
	violations map[string]error,
	lineageID string,
) (string, error) {
	if cause, carried := violations[lineageID]; carried {
		return lineageID, cause
	}
	seen := map[string]bool{lineageID: true}
	cursor, known := records[lineageID]
	for known && cursor.State.Recovery != nil {
		parent := cursor.State.Recovery.PredecessorLineageID
		if cause, carried := violations[parent]; carried {
			return parent, cause
		}
		if seen[parent] {
			return parent, errors.New("recovery cycle")
		}
		seen[parent] = true
		cursor, known = records[parent]
	}
	return "", nil
}

// compactAuthorityBlockedLineages is compactAuthorityBlockingCause over the
// whole record set at once, for the enumeration that has to decide which
// lineages may be offered as authority.
func compactAuthorityBlockedLineages(records map[string]CompactRecord, violations map[string]error) map[string]bool {
	blocked := make(map[string]bool, len(violations))
	for lineage := range records {
		if carrier, _ := compactAuthorityBlockingCause(records, violations, lineage); carrier != "" {
			blocked[lineage] = true
		}
	}
	return blocked
}

// compactAuthorityGraphViolations enumerates EVERY graph defect in one record
// set, keyed by the lineage that carries it, together with the child count of
// each predecessor. It is the set form of the first-error check
// compactAuthorityLeaves reports, and it exists so a repair operation can prove
// what it removes instead of being handed a graph that is already healthy.
//
// Attribution matters: a defect is recorded against the successor whose edge
// carries it, so removing that successor provably removes that defect and
// nothing else. Forks are attributed to every sibling and counted only over
// edges that already validate, exactly as the leaf selector does, so the two
// surfaces can never disagree about which graph is valid.
func compactAuthorityGraphViolations(records map[string]CompactRecord) (map[string]error, map[string]int) {
	violations := make(map[string]error)
	children := make(map[string]int)
	siblings := make(map[string][]string)
	for lineage, record := range records {
		if record.State.Recovery == nil {
			continue
		}
		predecessor, ok := records[record.State.Recovery.PredecessorLineageID]
		if !ok {
			violations[lineage] = fmt.Errorf("dangling predecessor for %q", lineage)
			continue
		}
		if predecessor.Revision != record.State.Recovery.PredecessorRevision {
			violations[lineage] = fmt.Errorf("predecessor revision mismatch for %q", lineage)
			continue
		}
		if err := validateCompactRecoveryEdge(predecessor, record.State); err != nil {
			violations[lineage] = err
			continue
		}
		children[predecessor.State.LineageID]++
		siblings[predecessor.State.LineageID] = append(siblings[predecessor.State.LineageID], lineage)
		seen := map[string]bool{lineage: true}
		cursor := record
		for cursor.State.Recovery != nil {
			parent := cursor.State.Recovery.PredecessorLineageID
			if seen[parent] {
				violations[lineage] = errors.New("recovery cycle")
				break
			}
			seen[parent] = true
			cursor = records[parent]
		}
	}
	for predecessor, forked := range siblings {
		if len(forked) < 2 {
			continue
		}
		for _, lineage := range forked {
			if _, carried := violations[lineage]; !carried {
				violations[lineage] = fmt.Errorf("fork at %q", predecessor)
			}
		}
	}
	return violations, children
}

// compactAuthorityRemovalRegression reports whether removing entries from an
// authority graph would make it WORSE, which is the invariant a repair
// operation can actually satisfy. Requiring the whole remaining graph to
// validate is a stronger post-condition that no repair can ever establish on a
// graph carrying two independent defects: each removal refuses citing the
// other and neither can go first, so the store stays unrecoverable forever.
//
// This is not a relaxation into permissiveness. Removal can only drop
// constraints, so anything it introduces — a successor left dangling behind a
// removed predecessor, a defect that changes class — is a real regression and
// is still refused. What it stops refusing is a defect the operation did not
// cause and does not touch.
func compactAuthorityRemovalRegression(before, after map[string]CompactRecord) error {
	prior, _ := compactAuthorityGraphViolations(before)
	remaining, _ := compactAuthorityGraphViolations(after)
	lineages := make([]string, 0, len(remaining))
	for lineage := range remaining {
		lineages = append(lineages, lineage)
	}
	sort.Strings(lineages)
	for _, lineage := range lineages {
		carried, existed := prior[lineage]
		// guard:population authority-repair-removal too-tight: legitimate repair removals may leave pre-existing unchanged graph defects but must not introduce or alter a defect
		if existed && carried.Error() == remaining[lineage].Error() {
			continue
		}
		return fmt.Errorf("it would introduce a new authority graph defect at %q: %w", lineage, remaining[lineage])
	}
	return nil
}

// compactRecordsWithout copies a record set without one lineage, so a caller
// can hand compactAuthorityRemovalRegression both the graph it observed and
// the graph its operation would leave behind without mutating either.
func compactRecordsWithout(records map[string]CompactRecord, lineage string) map[string]CompactRecord {
	remaining := make(map[string]CompactRecord, len(records))
	for candidate, record := range records {
		if candidate == lineage {
			continue
		}
		remaining[candidate] = record
	}
	return remaining
}

// compactAuthorityLeaves selects the leaves of the HEALTHY part of the
// authority graph. A lineage carrying a graph defect, and every lineage whose
// recovery ancestry inherits through one, is excluded from the selection.
//
// The removal here is the aggregate verdict this used to return. Reporting the
// first defect in sorted order made every lineage in the repository share the
// fate of whichever one sorted earliest, which is how a single historical edge
// nobody was operating on came to refuse review in every worktree at once.
// Exclusion is strictly more conservative for the damaged entry than the old
// error was: it can never be offered as authority, and an operation that names
// it directly still fails closed through blocked().
func compactAuthorityLeaves(records map[string]CompactRecord, storeByLineage map[string]CompactStore) []CompactStore {
	violations, children := compactAuthorityGraphViolations(records)
	blocked := compactAuthorityBlockedLineages(records, violations)
	leaves := []CompactStore{}
	for lineage, store := range storeByLineage {
		if blocked[lineage] || children[lineage] != 0 {
			continue
		}
		leaves = append(leaves, store)
	}
	sort.Slice(leaves, func(i, j int) bool { return leaves[i].lineageID < leaves[j].lineageID })
	return leaves
}

// CompactLineageSuperseded reports whether any READABLE entry recovers from
// this lineage.
//
// An entry nobody can read supersedes nothing, for the same reason it is not
// in the graph: a record that cannot be parsed states no predecessor. The
// alternative was to treat one unreadable entry as making supersession
// unknowable for every lineage in the repository, which is the exact
// repository-global refusal this whole change removes -- and it disagreed with
// the graph, which already reads an unreadable entry as absent.
//
// The residual case is narrow and named: a lineage whose only successor became
// unreadable stops reporting as superseded, so its own approved receipt can
// govern its own reviewed content again. That content was genuinely reviewed
// and the receipt still binds it exactly; the successor that retired it is
// broken and can deliver nothing itself. Leaving both unusable was the worse
// answer.
func CompactLineageSuperseded(ctx context.Context, repo, lineageID string) (bool, error) {
	scan, err := scanCompactAuthority(ctx, repo)
	if err != nil {
		return false, err
	}
	for _, record := range scan.records {
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
	if err := ensureNoPreparedCompactBatchReconciliation(base); err != nil {
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
	return compactIncidentsDir(ctx, repo, lineageID, false)
}

// EnsureCompactIncidentsDir creates the durable raw-result incident directory
// with the same owner-only safety boundary as compact authority artifacts.
func EnsureCompactIncidentsDir(ctx context.Context, repo, lineageID string) (string, error) {
	return compactIncidentsDir(ctx, repo, lineageID, true)
}

func compactIncidentsDir(ctx context.Context, repo, lineageID string, create bool) (string, error) {
	if err := validateLineageID(lineageID); err != nil {
		return "", err
	}
	base, _, err := reviewAuthorityRoot(ctx, repo)
	if err != nil {
		return "", err
	}
	dir := filepath.Join(base, "incidents", lineageID)
	if create {
		if err := ensureCompactIncidentsDir(dir); err != nil {
			return "", fmt.Errorf("ensure private incident preservation directory: %w", err)
		}
	}
	return dir, nil
}

func ensureCompactIncidentsDir(dir string) error {
	if _, err := createPrivateRARDirectory(filepath.Dir(dir)); err != nil {
		return fmt.Errorf("create private incident root: %w", err)
	}
	if _, err := createPrivateRARDirectory(dir); err != nil {
		return fmt.Errorf("create private incident lineage directory: %w", err)
	}
	return nil
}

func DiscoverCompactStores(ctx context.Context, repo string) ([]CompactStore, error) {
	base, root, err := reviewAuthorityRoot(ctx, repo)
	if err != nil {
		return nil, err
	}
	if err := ensureNoPreparedCompactBatchReconciliation(base); err != nil {
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
			if onlyUnpublishedCompactCrashResidue(residue, readErr) {
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

func onlyUnpublishedCompactCrashResidue(entries []os.DirEntry, readErr error) bool {
	if readErr != nil {
		return false
	}
	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), ".atomic-") && !strings.HasPrefix(entry.Name(), ".publish-") {
			return false
		}
	}
	return true
}

// StartCompactAuthority serializes compact start discovery, equivalence
// decisions, and the initial write under the repository-wide v2 lock.
func StartCompactAuthority(ctx context.Context, repo string, request CompactStartRequest) (CompactStartResult, error) {
	if err := request.State.Validate(); err != nil {
		return CompactStartResult{}, fmt.Errorf("validate compact start: %w", err)
	}
	if request.State.InitialSnapshot.Kind == TargetBaseWorkspaceOverlay && request.State.InitialSnapshot.Projection == ProjectionStaged {
		return CompactStartResult{}, errors.New("staged workspace-overlay authority requires approved recovery")
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

	// A damaged entry is skipped rather than fatal: starting a review of new
	// work has nothing to do with an entry that new work does not inherit
	// from, and refusing every start in every worktree until somebody repairs
	// unrelated history was the dead end reported as 1892, 2014 and 2167.
	scan, err := scanCompactAuthority(ctx, requestedStore.repo)
	if err != nil {
		return CompactStartResult{}, err
	}
	records, storeByLineage := scan.records, scan.stores
	if blocked := scan.blocked(request.State.LineageID); blocked != nil {
		return CompactStartResult{}, compactStartInvalidGraphRefusal(ctx, requestedStore.repo, records, blocked)
	}
	leaves := compactAuthorityLeaves(records, storeByLineage)
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
			claim, claimErr := classifyCompactCorrectionTargetForStart(ctx, requestedStore.repo, existing, request.State)
			if claimErr != nil {
				return CompactStartResult{}, fmt.Errorf("classify compact correction target: %w", claimErr)
			}
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
		rebasedRecovery, rebasedErr := compactApprovedRebasedScopeRecovery(ctx, requestedStore.repo, existing, request.State.InitialSnapshot)
		if rebasedErr != nil {
			return CompactStartResult{}, fmt.Errorf("classify approved rebased scope recovery: %w", rebasedErr)
		}
		if rebasedRecovery {
			receipt, receiptErr := inspectCompactTargetReceipt(store, existing)
			if receiptErr != nil {
				return CompactStartResult{}, fmt.Errorf("inspect approved rebased scope recovery receipt: %w", receiptErr)
			}
			if receipt.published && bytes.Equal(receipt.artifact.content, receipt.artifact.canonical) {
				recoveryCandidates = append(recoveryCandidates, store)
				continue
			}
		}
		if compactApprovedScopeChangedRecovery(existing, request.State.InitialSnapshot) {
			recoveryCandidates = append(recoveryCandidates, store)
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
	if request.BeforeCreate != nil {
		if err := request.BeforeCreate(); err != nil {
			return CompactStartResult{}, err
		}
	}
	record, payload, err := makeCompactRecord(request.State)
	if err != nil {
		return CompactStartResult{}, err
	}
	if err := writeAtomic(requestedStore.StatePath(), payload, 0o644); err != nil {
		return CompactStartResult{}, err
	}
	if request.TracePath != "" {
		recordCompactTrace(request.TracePath, CompactTraceEntry{
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

// compactApprovedScopeChangedRecovery reports the approved-predecessor shape
// START routes to recovery instead of a fresh lineage: the live candidate
// keeps the exact frozen delivery scope while its candidate tree changed.
// Target STATUS classifies with this same predicate on purpose — when the two
// surfaces used different rules, negotiated status emitted a START the store
// then refused, the closed loop confirmed on issue #1826.
func compactApprovedScopeChangedRecovery(existing CompactState, live Snapshot) bool {
	if existing.State != StateApproved {
		return false
	}
	requested := existing
	requested.InitialSnapshot = live
	return compactStartDeliveryScopeMatches(existing, requested) &&
		existing.CurrentSnapshot.CandidateTree != live.CandidateTree
}

// compactApprovedRebasedScopeRecovery recognizes one narrow committed-delivery
// transition: an approved current-changes patch was committed, its base advanced
// on disjoint paths, and the feature was rebased without changing its patch.
func compactApprovedRebasedScopeRecovery(ctx context.Context, repo string, existing CompactState, live Snapshot) (bool, error) {
	original, approved := existing.InitialSnapshot, existing.CurrentSnapshot
	if existing.State != StateApproved || original.Kind != TargetCurrentChanges || approved.Kind != TargetCurrentChanges ||
		live.Kind != TargetBaseDiff || !sameApprovedRebasedRecoveryProjection(original.Projection, live.Projection) ||
		original.BaseTree != approved.BaseTree || original.BaseTree == live.BaseTree ||
		approved.CandidateTree == live.CandidateTree || approved.Identity == live.Identity ||
		len(existing.GenesisPaths) == 0 || !equalStrings(approved.Paths, existing.GenesisPaths) ||
		!equalStrings(live.Paths, existing.GenesisPaths) ||
		!equalStrings(original.IntendedUntracked, approved.IntendedUntracked) ||
		original.IntendedUntrackedProof != approved.IntendedUntrackedProof || len(live.IntendedUntracked) != 0 ||
		len(original.LedgerIDs) != 0 || len(approved.LedgerIDs) != 0 || len(live.LedgerIDs) != 0 {
		return false, nil
	}
	for _, path := range original.IntendedUntracked {
		if stringIndex(existing.GenesisPaths, path) < 0 {
			return false, nil
		}
	}
	builder := SnapshotBuilder{Repo: repo}
	baseAdvancePaths, err := builder.changedPaths(ctx, original.BaseTree, live.BaseTree)
	if err != nil {
		return false, fmt.Errorf("derive rebased predecessor base-advance paths: %w", err)
	}
	if !disjointPaths(baseAdvancePaths, existing.GenesisPaths) {
		return false, nil
	}
	originalPatch, err := patchIdentity(ctx, repo, original.BaseTree, approved.CandidateTree)
	if err != nil {
		candidateType, inspectErr := runGit(ctx, repo, nil, []byte(approved.CandidateTree+"\n"), "cat-file", "--batch-check=%(objectname) %(objecttype)")
		if inspectErr != nil {
			return false, fmt.Errorf("inspect approved predecessor candidate tree: %w", inspectErr)
		}
		candidateFields := strings.Fields(string(candidateType))
		if len(candidateFields) == 2 && candidateFields[0] == approved.CandidateTree && candidateFields[1] == "missing" {
			return false, nil
		}
		if len(candidateFields) != 2 || candidateFields[0] != approved.CandidateTree || candidateFields[1] != "tree" {
			return false, fmt.Errorf("inspect approved predecessor candidate tree: unexpected git cat-file response %q", strings.TrimSpace(string(candidateType))) // refusal:by-design world-action: Git returned an unrecognized plumbing response, so only a code or Git-environment repair can define a trustworthy continuation
		}
		return false, fmt.Errorf("derive approved predecessor patch identity: %w", err)
	}
	livePatch, err := patchIdentity(ctx, repo, live.BaseTree, live.CandidateTree)
	if err != nil {
		return false, fmt.Errorf("derive live rebased patch identity: %w", err)
	}
	return originalPatch == livePatch, nil
}

// A base-diff has no index-backed staged execution mode. STATUS therefore
// projects this narrow approved staged-current-changes rebase to its executable
// committed-only base-diff form before identity derivation. No other mixed
// projection is admitted here.
func sameApprovedRebasedRecoveryProjection(original, live Projection) bool {
	if live == "" {
		live = ProjectionWorkspace
	}
	return sameRecoveryProjection(original, live) || original == ProjectionStaged && live == ProjectionWorkspace
}

// compactStartDeliveryScopeMatches compares the immutable delivery boundary
// without Snapshot.Identity because current-changes and base-diff have distinct
// representations for the same base-to-candidate tree range.
func compactStartDeliveryScopeMatches(existing, requested CompactState) bool {
	original, live := existing.InitialSnapshot, requested.InitialSnapshot
	return compactTargetProjectionsCompatible(original.Kind, original.Projection, live.Kind, live.Projection) &&
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

func compactTargetProjectionsCompatible(existingKind TargetKind, existingProjection Projection, requestedKind TargetKind, requestedProjection Projection) bool {
	if existingProjection == "" {
		existingProjection = ProjectionWorkspace
	}
	if requestedProjection == "" {
		requestedProjection = ProjectionWorkspace
	}
	if existingProjection == requestedProjection {
		return true
	}
	// Staged/workspace representations are safe only for this content-equivalent
	// kind class because surrounding predicates still bind one content boundary;
	// workspace-overlay remains excluded.
	return (existingKind == TargetCurrentChanges || existingKind == TargetBaseDiff) &&
		(requestedKind == TargetCurrentChanges || requestedKind == TargetBaseDiff) &&
		(existingProjection == ProjectionStaged && requestedProjection == ProjectionWorkspace ||
			existingProjection == ProjectionWorkspace && requestedProjection == ProjectionStaged)
}

type compactCorrectionTargetClaim uint8

const (
	compactCorrectionTargetUnclaimed compactCorrectionTargetClaim = iota
	compactCorrectionTargetResume
	compactCorrectionTargetBlocked
	compactCorrectionTargetRecover
)

func classifyCompactCorrectionTargetForStart(ctx context.Context, repo string, existing, requested CompactState) (compactCorrectionTargetClaim, error) {
	return classifyCompactCorrectionTarget(ctx, repo, existing, requested, false)
}

// classifyCompactCorrectionTarget is the single START/STATUS correction
// ownership evaluator. It keeps correction ownership bound to the original
// delivery boundary even when in-genesis bytes change. Callers that already
// built the request-scoped live snapshot may skip duplicate evidence reads;
// every target relationship and operational error still follows this path.
func classifyCompactCorrectionTarget(ctx context.Context, repo string, existing, requested CompactState, liveAlreadyValidated bool) (compactCorrectionTargetClaim, error) {
	live := requested.InitialSnapshot
	if existing.State != StateCorrectionRequired ||
		existing.InitialSnapshot.Projection != live.Projection ||
		!compactStartTargetKindsCompatible(existing.InitialSnapshot.Kind, live.Kind) ||
		existing.InitialSnapshot.BaseTree != live.BaseTree || len(live.LedgerIDs) != 0 {
		return compactCorrectionTargetUnclaimed, nil
	}
	if !liveAlreadyValidated {
		if err := (SnapshotBuilder{Repo: repo}).ValidateEvidence(ctx, live); err != nil {
			if IsCompactAuthorityOperationalFailure(err) {
				return compactCorrectionTargetUnclaimed, err
			}
			return compactCorrectionTargetUnclaimed, nil
		}
	}
	if compactHistoricalFailedValidator(existing) {
		if compactEscalatedRecoveryTargetChanged(existing.CurrentSnapshot, live) {
			return compactCorrectionTargetRecover, nil
		}
		return compactCorrectionTargetBlocked, nil
	}
	if compactRecoveryAddsGenesisPath(existing, live) {
		return compactCorrectionTargetRecover, nil
	}
	if pathsAreSubset(live.Paths, existing.GenesisPaths) != nil {
		return compactCorrectionTargetUnclaimed, nil
	}
	if compactStartLiveTargetMatchesValidated(existing, requested, false) {
		if live.CandidateTree == existing.CurrentSnapshot.CandidateTree {
			return compactCorrectionTargetResume, nil
		}
		matches, err := compactCorrectionCandidateMatches(ctx, repo, existing, requested)
		if err != nil {
			if IsCompactAuthorityOperationalFailure(err) {
				return compactCorrectionTargetUnclaimed, err
			}
		} else if matches {
			return compactCorrectionTargetResume, nil
		}
	}
	if compactRecoveryContractsGenesisPaths(existing, live) {
		return compactCorrectionTargetRecover, nil
	}
	return compactCorrectionTargetBlocked, nil
}

// IsCompactAuthorityOperationalFailure reports errors that prevent observing
// authority at all rather than describing a quarantinable authority record.
func IsCompactAuthorityOperationalFailure(err error) bool {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, ErrConcurrentUpdate) {
		return true
	}
	var lockTimeout *AuthorityLockTimeoutError
	var lockCancelled *AuthorityLockCancelledError
	if errors.As(err, &lockTimeout) || errors.As(err, &lockCancelled) {
		return true
	}
	var timeout *GitCommandTimeoutError
	var command *GitCommandError
	var processControl *GitProcessControlError
	if errors.As(err, &timeout) || errors.As(err, &command) || errors.As(err, &processControl) {
		return true
	}
	var pathErr *os.PathError
	return errors.As(err, &pathErr) && !errors.Is(err, os.ErrNotExist)
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

func compactCorrectionCandidateMatches(ctx context.Context, repo string, existing, requested CompactState) (bool, error) {
	if existing.ProposedCorrectionLines == nil {
		return false, nil
	}
	fix, err := (SnapshotBuilder{Repo: repo}).Build(ctx, Target{Kind: TargetFixDiff,
		Projection: existing.InitialSnapshot.Projection, BaseRef: existing.CurrentSnapshot.CandidateTree,
		IntendedUntracked: existing.InitialSnapshot.IntendedUntracked, LedgerIDs: existing.FixFindingIDs})
	if err != nil {
		return false, err
	}
	if fix.CandidateTree != requested.InitialSnapshot.CandidateTree || pathsAreSubset(fix.Paths, existing.GenesisPaths) != nil {
		return false, nil
	}
	lines, err := (SnapshotBuilder{Repo: repo}).ChangedLines(ctx, fix)
	if err != nil {
		return false, err
	}
	remaining, err := compactCorrectionRemainingBudget(existing)
	if err != nil {
		return false, err
	}
	return lines <= remaining, nil
}

func compactCorrectionRemainingBudget(state CompactState) (int, error) {
	if state.CorrectionBudget < 0 || state.CumulativeCorrectionLines < 0 || state.CumulativeCorrectionLines > state.CorrectionBudget {
		return 0, errors.New("compact correction accounting cannot derive a remaining budget") // refusal:by-design world-action: invalid persisted correction accounting cannot authorize another correction candidate
	}
	return state.CorrectionBudget - state.CumulativeCorrectionLines, nil
}

func compactStartLiveTargetMatches(ctx context.Context, repo string, existing, requested CompactState, requireCurrentCandidate bool) bool {
	return compactStartLiveTargetMatchesValidated(existing, requested, requireCurrentCandidate) &&
		(SnapshotBuilder{Repo: repo}).ValidateEvidence(ctx, requested.InitialSnapshot) == nil
}

func compactStartLiveTargetMatchesValidated(existing, requested CompactState, requireCurrentCandidate bool) bool {
	if existing.Generation != requested.Generation || existing.PolicyHash != requested.PolicyHash ||
		!reflect.DeepEqual(existing.Recovery, requested.Recovery) {
		return false
	}
	return compactLiveTargetMatchesValidatedSnapshot(existing, requested.InitialSnapshot, requireCurrentCandidate)
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
	return store.LoadContext(context.Background())
}

// LoadContext reads one compact record while preserving the caller's bounded
// cancellation through shared maintenance-lock acquisition.
func (store CompactStore) LoadContext(ctx context.Context) (CompactRecord, error) {
	maintenance, err := store.acquireReadMaintenance(ctx)
	if err != nil {
		return CompactRecord{}, err
	}
	if maintenance != nil {
		defer maintenance.Release()
	}
	return store.loadCompactRecordLocked()
}

// acquireReadMaintenance prevents a stale CompactStore handle from observing
// a partially applied authority-maintenance transaction. The first marker
// check refuses an already-prepared batch without waiting on its exclusive
// lease; the maintenance acquisition closes the race with a batch that starts
// after that check and repeats the marker check once shared access is held.
func (store CompactStore) acquireReadMaintenance(ctx context.Context) (*MaintenanceLock, error) {
	if store.maintenanceLockPath == "" {
		return nil, nil
	}
	authorityRoot := filepath.Join(filepath.Dir(store.maintenanceLockPath), "review-transactions")
	if err := ensureNoPreparedCompactBatchReconciliation(authorityRoot); err != nil {
		return nil, err
	}
	// Preserve the historical read-only behavior for a handle whose compact
	// authority record does not exist. There is no batch-owned record to
	// coordinate in that case, and creating REVIEW-MAINTENANCE.lock would make
	// a failed legacy fallback observably mutate authority metadata.
	if _, err := os.Lstat(store.StatePath()); os.IsNotExist(err) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	return acquireMaintenanceLock(ctx, store.maintenanceLockPath, maintenanceShared)
}

// loadCompactRecordLocked is the uncoordinated record read for callers that
// already hold the required maintenance/store coordination. It is also used by
// batch reconciliation while its exclusive maintenance lease is held.
func (store CompactStore) loadCompactRecordLocked() (CompactRecord, error) {
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
		// Typed rather than anonymous: this is the compare-and-set that gates
		// the write below, so losing it proves this call mutated nothing. A
		// caller cannot recover that proof from an untyped ErrConcurrentUpdate,
		// which several non-write preconditions in this package also report.
		return "", &CompactRevisionConflictError{
			LineageID: store.lineageID, Expected: expectedRevision, Current: currentRevision,
		}
	}
	if current == nil {
		if operation != "review/start" || next.State != StateReviewing {
			return "", fmt.Errorf("%w: compact authority must start in reviewing state", ErrInvalidSuccessor)
		}
		if next.InitialSnapshot.Kind == TargetBaseWorkspaceOverlay && next.InitialSnapshot.Projection == ProjectionStaged {
			return "", fmt.Errorf("%w: staged workspace-overlay authority requires an approved recovery predecessor", ErrInvalidSuccessor)
		}
	} else if err := validateCompactSuccessor(current.Revision, current.State, next, operation); err != nil {
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
		recordCompactTrace(store.TracePath, CompactTraceEntry{
			Operation: operation, PreviousRevision: currentRevision, Revision: record.Revision,
			State: next.State, RecordedAt: time.Now().UTC().Format(time.RFC3339Nano),
		})
	}
	return record.Revision, nil
}

// captureReviewerResult revalidates the reviewing binding while holding shared
// maintenance access and the compact version lock before publishing an artifact.
func (store CompactStore) captureReviewerResult(expectedRevision, target, lens string, order int, publish func(CompactState) error) error {
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
		return NewLegacyReadOnlyError(
			"review/capture-result",
			record.State.LineageID,
		)
	}
	state := record.State
	if record.Revision != expectedRevision || state.State != StateReviewing || state.InitialSnapshot.Identity != target || order < 0 || order >= len(state.SelectedLenses) || state.SelectedLenses[order] != lens {
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
	if operation == "review/complete-fix" || operation == "review/complete-correction-verification" {
		attempt := next.CorrectionAttempts[len(next.CorrectionAttempts)-1]
		if err := builder.ValidateEvidence(ctx, attempt.Snapshot); err != nil {
			return errors.New("compact correction snapshot is not repository-derived")
		}
		lines, err := builder.ChangedLines(ctx, attempt.Snapshot)
		if err != nil || lines != attempt.ActualLines {
			return errors.New("compact correction size does not match repository evidence")
		}
		if !snapshotsEqual(next.CurrentSnapshot, attempt.Snapshot) {
			if err := builder.ValidateEvidence(ctx, next.CurrentSnapshot); err != nil {
				return errors.New("complete corrected candidate is not repository-derived") // refusal:by-design world-action: repository drift requires rebuilding the candidate-bound finalize request
			}
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
	if operation == CompactResultReopenOperation {
		if current == nil || len(next.ResultReopens) != len(current.State.ResultReopens)+1 {
			return errors.New("reviewer result reopen lacks an exact predecessor authority")
		}
		reopen := next.ResultReopens[len(next.ResultReopens)-1]
		request := CompactResultReopenRequest{
			LineageID:        current.State.LineageID,
			ExpectedRevision: current.Revision,
			TargetIdentity:   current.State.InitialSnapshot.Identity,
			Reason:           reopen.Reason,
			Actor:            reopen.Actor,
		}
		if reopen.MaintainerAuthorization != CompactResultReopenAuthorization(repo, request, reopen.Quarantined, reopen.Retained, reopen.AuthorizedLenses) {
			return errors.New("reviewer result reopen does not carry the exact maintainer authorization")
		}
	}
	if operation == "review/invalidate" {
		if err := rebuildCurrentSnapshotEvidence(ctx, repo, next.InitialSnapshot); err != nil {
			return err
		}
	}
	return nil
}

func validateCompactSuccessor(previousRevision string, previous, next CompactState, operation string) error {
	if previous.LineageID != next.LineageID || previous.Generation != next.Generation ||
		!snapshotsEqual(previous.InitialSnapshot, next.InitialSnapshot) || !equalStrings(previous.GenesisPaths, next.GenesisPaths) ||
		previous.PolicyHash != next.PolicyHash || previous.RiskLevel != next.RiskLevel ||
		!equalStrings(previous.SelectedLenses, next.SelectedLenses) || previous.OriginalChangedLines != next.OriginalChangedLines ||
		previous.CorrectionBudget != next.CorrectionBudget || previous.CorrectionBudgetPolicy != next.CorrectionBudgetPolicy {
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
		if previous.CorrectionAttemptConsumed() {
			return fmt.Errorf("%w: %w", ErrInvalidSuccessor, ErrCompactCorrectionConsumed)
		}
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
		if previous.CorrectionAttemptConsumed() {
			return fmt.Errorf("%w: %w", ErrInvalidSuccessor, ErrCompactCorrectionConsumed)
		}
		if previous.State != StateCorrectionRequired || previous.ProposedCorrectionLines == nil || next.State != StateValidating && next.State != StateCorrectionRequired && next.State != StateEscalated || len(next.CorrectionAttempts) != len(previous.CorrectionAttempts)+1 {
			return fmt.Errorf("%w: invalid compact correction completion", ErrInvalidSuccessor)
		}
		if len(previous.CorrectionAttempts) > 0 && !reflect.DeepEqual(previous.CorrectionAttempts, next.CorrectionAttempts[:len(previous.CorrectionAttempts)]) {
			return fmt.Errorf("%w: compact correction attempt history is immutable", ErrInvalidSuccessor)
		}
		if !reflectCompactReviewData(previous, next) || previous.EvidenceHash != next.EvidenceHash {
			return fmt.Errorf("%w: compact correction changed frozen review evidence", ErrInvalidSuccessor)
		}
	case "review/complete-correction-verification":
		if previous.CorrectionAttemptConsumed() {
			return fmt.Errorf("%w: %w", ErrInvalidSuccessor, ErrCompactCorrectionConsumed)
		}
		if previous.State != StateCorrectionRequired || previous.ProposedCorrectionLines == nil || next.State != StateApproved ||
			len(next.CorrectionAttempts) != len(previous.CorrectionAttempts)+1 || next.EvidenceOutcome != VerificationOutcomePassed ||
			next.EvidenceAuthorityRevision != previousRevision || next.EvidenceTargetIdentity != next.CorrectionAttempts[len(next.CorrectionAttempts)-1].Snapshot.Identity {
			return fmt.Errorf("%w: invalid atomic compact correction verification", ErrInvalidSuccessor)
		}
		if len(previous.CorrectionAttempts) > 0 && !reflect.DeepEqual(previous.CorrectionAttempts, next.CorrectionAttempts[:len(previous.CorrectionAttempts)]) {
			return fmt.Errorf("%w: compact correction attempt history is immutable", ErrInvalidSuccessor)
		}
		if !reflectCompactReviewData(previous, next) {
			return fmt.Errorf("%w: atomic compact correction verification changed frozen review evidence", ErrInvalidSuccessor)
		}
	case "review/escalate-correction-verification":
		if previous.State != StateCorrectionRequired || previous.ProposedCorrectionLines == nil || next.State != StateEscalated ||
			next.CorrectionVerificationTarget == nil || next.EvidenceOutcome != VerificationOutcomeProceduralFailure ||
			next.EvidenceAuthorityRevision != previousRevision || len(next.CorrectionAttempts) != len(previous.CorrectionAttempts) ||
			next.CumulativeCorrectionLines != previous.CumulativeCorrectionLines {
			return fmt.Errorf("%w: invalid procedural correction verification escalation", ErrInvalidSuccessor)
		}
		expected := previous
		expected.State = StateEscalated
		expected.CorrectionVerificationTarget = next.CorrectionVerificationTarget
		expected.EvidenceHash = next.EvidenceHash
		expected.EvidenceRecordDigest = next.EvidenceRecordDigest
		expected.EvidenceOutcome = next.EvidenceOutcome
		expected.EvidenceTargetIdentity = next.EvidenceTargetIdentity
		expected.EvidenceAuthorityRevision = next.EvidenceAuthorityRevision
		if !compactStateEqual(expected, next) {
			return fmt.Errorf("%w: procedural correction verification escalation changed unrelated state", ErrInvalidSuccessor)
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
	case CompactResultReopenOperation:
		// Validating keeps its historical eligibility; correction-required is
		// additionally eligible only while uncorrected — a completed
		// correction attempt or actual correction accounting closes this door
		// for good, because those prove candidate bytes moved under review.
		reopenablePredecessor := previous.State == StateValidating ||
			previous.State == StateCorrectionRequired && len(previous.CorrectionAttempts) == 0 && previous.ActualCorrectionLines == nil
		if !reopenablePredecessor || next.State != StateReviewing ||
			len(next.ResultReopens) != len(previous.ResultReopens)+1 ||
			len(previous.ResultReopens) > 0 && !reflect.DeepEqual(previous.ResultReopens, next.ResultReopens[:len(previous.ResultReopens)]) {
			return fmt.Errorf("%w: reviewer result reopen must append one audit record returning an uncorrected authority to reviewing", ErrInvalidSuccessor)
		}
		reopen := next.ResultReopens[len(next.ResultReopens)-1]
		if reopen.PreviousRevision != previousRevision || reopen.TargetIdentity != previous.InitialSnapshot.Identity {
			return fmt.Errorf("%w: reviewer result reopen does not bind the exact predecessor authority", ErrInvalidSuccessor)
		}
		expected := previous
		expected.State = StateReviewing
		expected.LensResults = []LensResult{}
		expected.Findings = []Finding{}
		expected.Classifications = map[string]FindingEvidence{}
		expected.Outcomes = map[string]EvidenceOutcome{}
		expected.FixFindingIDs = []string{}
		expected.FollowUps = []FollowUp{}
		expected.ProposedCorrectionLines = nil
		expected.ActualCorrectionLines = nil
		expected.FixDeltaHash = EmptyFixDeltaHash
		expected.OriginalCriteria = nil
		expected.CorrectionRegression = nil
		expected.EvidenceHash = ""
		expected.ResultReopens = next.ResultReopens
		if !compactStateEqual(expected, next) {
			return fmt.Errorf("%w: reviewer result reopen changed frozen scope, budget, or unrelated authority", ErrInvalidSuccessor)
		}
	case "review/complete-verification":
		if previous.State != StateValidating || next.State != StateApproved && next.State != StateEscalated || !validSHA256(next.EvidenceHash) ||
			next.EvidenceRecordDigest != "" && next.EvidenceAuthorityRevision != previousRevision {
			return fmt.Errorf("%w: invalid compact verification completion", ErrInvalidSuccessor)
		}
		expected := previous
		expected.State = next.State
		expected.EvidenceHash = next.EvidenceHash
		expected.EvidenceRecordDigest = next.EvidenceRecordDigest
		expected.EvidenceOutcome = next.EvidenceOutcome
		expected.EvidenceTargetIdentity = next.EvidenceTargetIdentity
		expected.EvidenceAuthorityRevision = next.EvidenceAuthorityRevision
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
		return CompactRecord{}, &CompactSemanticStateError{LineageID: record.State.LineageID, State: record.State.State, Problem: err.Error(),
			OutdatedIdentity: errors.Is(err, errCompactSnapshotIdentityMismatch)}
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

// compactTraceWarn reports a lost diagnostic-trace write (issue #1854). A
// caller that supplies TracePath explicitly asked for that observability; the
// authority mutation has already committed by the time this runs and must
// never be rolled back or fail because of it, so this is report-only. It
// follows the same "WARNING: ..." convention run.go already uses at the CLI
// boundary for other non-fatal, already-succeeded-but-partially-degraded
// operations (e.g. "could not add %s to PATH"). It is a package-level var, in
// keeping with this file's existing test-seam convention (see
// finalCompactInvalidationHook and similar hooks), so tests can observe the
// report without capturing real stderr.
var compactTraceWarn = func(operation, path string, err error) {
	fmt.Fprintf(os.Stderr, "WARNING: review trace for %s was not recorded at %s: %v\n", operation, path, err)
}

// recordCompactTrace appends a diagnostic trace entry and reports rather than
// swallows a write failure. The trace is best-effort diagnostics only: it
// never carries authority, so its failure must never affect an already
// committed mutation's success/failure outcome.
func recordCompactTrace(path string, entry CompactTraceEntry) {
	if err := appendCompactTrace(path, entry); err != nil {
		compactTraceWarn(entry.Operation, path, err)
	}
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
	maintenance, err := store.acquireReadMaintenance(context.Background())
	if err != nil {
		return CompactTransport{}, err
	}
	if maintenance != nil {
		defer maintenance.Release()
	}
	record, err := store.loadCompactRecordLocked()
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
	if validated.Record.State.InitialSnapshot.Kind == TargetBaseWorkspaceOverlay && validated.Record.State.InitialSnapshot.Projection == ProjectionStaged {
		return CompactRecord{}, errors.New("staged workspace-overlay authority cannot be imported")
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
	return store.loadCompactRecordLocked()
}

func (store CompactStore) installTransportRecordLocked(ctx context.Context, record CompactRecord) error {
	if existing, loadErr := store.loadCompactRecordLocked(); loadErr == nil {
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
	// Bounded wait, not instant refusal: the receipt is derived from
	// already-terminal authority and publication is idempotent, so a
	// briefly-held advisory lock is a competitor completing the same
	// publication, not a second writer to refuse.
	lock, err := acquireStoreLockForConvergentCompletion(ctx, store.lockPath)
	if err != nil {
		return err
	}
	defer lock.release()
	return store.writeReceiptLocked(receipt)
}

func (store CompactStore) writeReceiptLocked(receipt CompactReceipt) error {
	record, err := store.loadCompactRecordLocked()
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
