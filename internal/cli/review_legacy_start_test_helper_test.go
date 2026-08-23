package cli

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

// runLegacyFacadeStartForTest preserves the historical test-helper signature
// while building its authority through the current exact atomic START boundary.
// It parses the retained fixture flags and emits ReviewFacadeStartResult so
// callers can keep exercising status, finalize, correction, and capture flows.
//
// This is the fixture-construction pattern established (and individually
// verified) on finalizeApprovedFacadeReview, approveDiscoveryMarkdownProjection,
// and startFacadeReview -- generalized here to cover every flag combination
// those three narrower helpers didn't need, for the many remaining test
// files with their own ad hoc RunReviewFacadeStart call sites.
//
// Not supported (none of the migrated call sites use them; the negotiated
// contract/consent surface is switch-independent and untouched by WU18, so
// tests needing it can and do keep calling the real RunReviewFacadeStart):
// --contract, --target, --agent, --consent, --locale, --trace.
func runLegacyFacadeStartForTest(t *testing.T, args []string, stdout io.Writer) error {
	t.Helper()
	flags := flag.NewFlagSet("review start (legacy test fixture)", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	cwd := flags.String("cwd", ".", "")
	lineage := flags.String("lineage", "", "")
	policySource := flags.String("policy", "", "")
	focus := flags.String("focus", "reliability", "")
	baseRef := flags.String("base-ref", "", "")
	projectionFlag := flags.String("projection", string(reviewtransaction.ProjectionWorkspace), "")
	committedOnly := flags.Bool("committed-only", false, "")
	workspaceOverlay := flags.Bool("workspace-overlay", false, "")
	allowDuplicateAuthority := flags.Bool("allow-duplicate-authority", false, "")
	untrackedScope := reviewSingleValueFlag{}
	intendedUntracked := reviewRepeatedPathFlag{}
	expectedUntrackedInventory := reviewSingleValueFlag{}
	flags.Var(&untrackedScope, "untracked-scope", "")
	flags.Var(&intendedUntracked, "intended-untracked", "")
	flags.Var(&expectedUntrackedInventory, "expected-untracked-inventory", "")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected review start argument %q", flags.Arg(0))
	}

	ctx := context.Background()
	builder := reviewtransaction.SnapshotBuilder{Repo: *cwd}
	root, err := builder.ResolveRepositoryRoot(ctx)
	if err != nil {
		return fmt.Errorf("resolve review repository root: %w", err)
	}
	rootBuilder := reviewtransaction.SnapshotBuilder{Repo: root}
	selectedProjection := reviewtransaction.Projection(strings.TrimSpace(*projectionFlag))
	if selectedProjection != reviewtransaction.ProjectionWorkspace && selectedProjection != reviewtransaction.ProjectionStaged {
		return fmt.Errorf("unsupported review projection %q", *projectionFlag)
	}
	if *workspaceOverlay && (strings.TrimSpace(*baseRef) == "" || *committedOnly || selectedProjection != reviewtransaction.ProjectionWorkspace) {
		return errors.New("--workspace-overlay requires --base-ref with workspace projection and is incompatible with --committed-only")
	}
	if selectedProjection == reviewtransaction.ProjectionStaged && strings.TrimSpace(*baseRef) != "" && !*workspaceOverlay {
		return fmt.Errorf("review start with --projection staged and --base-ref is refused because intent is ambiguous: for a staged-index review rerun with %q; for a base-diff review rerun with %q",
			"gentle-ai review start --projection staged",
			fmt.Sprintf("gentle-ai review start --base-ref %s --committed-only", strings.TrimSpace(*baseRef)))
	}
	if strings.TrimSpace(*baseRef) != "" && !*workspaceOverlay {
		dirtyTracked, dirtyErr := rootBuilder.HasDirtyTrackedChanges(ctx)
		if dirtyErr != nil {
			return fmt.Errorf("detect dirty tracked changes for committed review: %w", dirtyErr)
		}
		if dirtyTracked && !*committedOnly {
			return errors.New("review start with --base-ref omits dirty tracked changes; rerun with --committed-only to acknowledge committed-only review scope")
		}
	}
	target := reviewtransaction.Target{Kind: reviewtransaction.TargetCurrentChanges, Projection: selectedProjection, IntendedUntracked: []string{}}
	if strings.TrimSpace(*baseRef) != "" {
		target.Kind = reviewtransaction.TargetBaseDiff
		target.BaseRef = strings.TrimSpace(*baseRef)
	}
	if *workspaceOverlay {
		target.Kind = reviewtransaction.TargetBaseWorkspaceOverlay
	}
	intendedScope, err := reviewIntendedUntrackedScopeForTarget(ctx, rootBuilder, untrackedScope, intendedUntracked, expectedUntrackedInventory)
	if err != nil {
		return err
	}
	if intendedScope.NeedsSelection {
		return reviewIntendedUntrackedSelectionRequired(intendedScope)
	}
	target.IntendedUntracked = intendedScope.Intended
	snapshot, err := rootBuilder.Build(ctx, target)
	if err != nil {
		return fmt.Errorf("build facade review target: %w", err)
	}
	assessment, err := rootBuilder.AssessSnapshotRisk(ctx, snapshot)
	if err != nil {
		return fmt.Errorf("classify facade review target: %w", err)
	}
	lenses, err := facadeSelectedLenses(assessment, *focus)
	if err != nil {
		return err
	}
	trimmedLineage := strings.TrimSpace(*lineage)
	if *allowDuplicateAuthority && trimmedLineage == "" {
		return errors.New("--allow-duplicate-authority requires --lineage")
	}
	request, err := prepareReviewFacadeCompactAtomicStart(ctx, root, trimmedLineage, *policySource, target, snapshot, assessment, assessment.ChangedLines, lenses)
	if err != nil {
		return fmt.Errorf("prepare compact atomic test fixture: %w", err)
	}
	started, err := runReviewFacadeCompactAtomicStart(ctx, root, request)
	if err != nil {
		return fmt.Errorf("start compact atomic test fixture: %w", err)
	}
	action := "created"
	if started.Replayed {
		action = "replayed"
	}
	result := reviewFacadeStartResultFor(action, len(started.Record.State.SelectedLenses) > 0, started.Record.State)
	if started.Record.State.InitialSnapshot.Identity == snapshot.Identity {
		result.RiskEvidence = reviewConsentRiskEvidence(assessment)
		switch {
		case result.ChangedFiles == 0 && target.Kind == reviewtransaction.TargetCurrentChanges:
			result.Hint = reviewStartEmptyCandidateHint
		case result.LensesRequired:
			result.Hint = "this response's selected lenses require the frozen Git trees, changed-path manifest, and artifact subjects, which only the negotiated contract form returns; rerun with `" +
				reviewNegotiatedStartCommand(started.Record.State.InitialSnapshot, "") + "` to receive them"
		}
	}
	return encodeReviewJSON(stdout, result)
}

// runLegacyFacadeStartForTestBytes is the []byte-returning convenience form
// for the (more common) call sites that decode from a bytes.Buffer.
func runLegacyFacadeStartForTestBytes(t *testing.T, args []string) ([]byte, error) {
	t.Helper()
	var output bytes.Buffer
	err := runLegacyFacadeStartForTest(t, args, &output)
	return output.Bytes(), err
}

// finalizeHistoricalFacadeReviewForRepo creates a historical compact approval
// directly through reviewtransaction fixture seams. It is only for evaluators
// whose premise is a receipt produced before current FINALIZE burned approvals.
func finalizeHistoricalFacadeReviewForRepo(t *testing.T, repo, lineage string, startExtra ...string) historicalCompatibilityCompactReceiptFixture {
	t.Helper()
	return seedHistoricalCompatibilityApprovedCompactReceipt(t, repo, lineage, historicalFacadeReviewTarget(t, startExtra))
}

func historicalFacadeReviewTarget(t *testing.T, args []string) reviewtransaction.Target {
	t.Helper()
	flags := flag.NewFlagSet("historical facade review fixture", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	baseRef := flags.String("base-ref", "", "")
	projection := flags.String("projection", string(reviewtransaction.ProjectionWorkspace), "")
	committedOnly := flags.Bool("committed-only", false, "")
	workspaceOverlay := flags.Bool("workspace-overlay", false, "")
	if err := flags.Parse(args); err != nil {
		t.Fatalf("parse historical facade review fixture flags: %v", err)
	}
	if flags.NArg() != 0 {
		t.Fatalf("unexpected historical facade review fixture argument %q", flags.Arg(0))
	}
	selectedProjection := reviewtransaction.Projection(strings.TrimSpace(*projection))
	if selectedProjection != reviewtransaction.ProjectionWorkspace && selectedProjection != reviewtransaction.ProjectionStaged {
		t.Fatalf("unsupported historical facade review fixture projection %q", *projection)
	}
	if *workspaceOverlay && (strings.TrimSpace(*baseRef) == "" || *committedOnly || selectedProjection != reviewtransaction.ProjectionWorkspace) {
		t.Fatal("--workspace-overlay requires --base-ref with workspace projection and is incompatible with --committed-only")
	}
	if strings.TrimSpace(*baseRef) != "" && !*workspaceOverlay && !*committedOnly {
		t.Fatal("historical base-diff fixture requires --committed-only")
	}
	target := reviewtransaction.Target{Kind: reviewtransaction.TargetCurrentChanges, Projection: selectedProjection, IntendedUntracked: []string{}}
	if strings.TrimSpace(*baseRef) != "" {
		target.Kind = reviewtransaction.TargetBaseDiff
		target.BaseRef = strings.TrimSpace(*baseRef)
	}
	if *workspaceOverlay {
		target.Kind = reviewtransaction.TargetBaseWorkspaceOverlay
	}
	return target
}

// historicalCompatibilityCompactReceiptFixture is test-only persisted compact
// authority that represents an approval minted before v3 started burning stale
// predecessor records. It is created through reviewtransaction fixture seams,
// never through production FINALIZE or post-approval gate routing.
type historicalCompatibilityCompactReceiptFixture struct {
	Record reviewtransaction.CompactRecord
	Store  reviewtransaction.CompactStore
}

// seedHistoricalCompatibilityApprovedCompactReceipt persists an approved
// compact authority and receipt directly for compatibility tests. Callers must
// prepare the exact candidate before invoking it; this helper deliberately does
// not dispatch production FINALIZE.
func seedHistoricalCompatibilityApprovedCompactReceipt(t *testing.T, repo, lineage string, target reviewtransaction.Target) historicalCompatibilityCompactReceiptFixture {
	t.Helper()
	ctx := t.Context()
	builder := reviewtransaction.SnapshotBuilder{Repo: repo}
	root, err := builder.ResolveRepositoryRoot(ctx)
	if err != nil {
		t.Fatalf("resolve historical compatibility repository root: %v", err)
	}
	rootBuilder := reviewtransaction.SnapshotBuilder{Repo: root}
	snapshot, err := rootBuilder.Build(ctx, target)
	if err != nil {
		t.Fatalf("build historical compatibility target: %v", err)
	}
	assessment, err := rootBuilder.AssessSnapshotRisk(ctx, snapshot)
	if err != nil {
		t.Fatalf("classify historical compatibility target: %v", err)
	}
	lenses, err := facadeSelectedLenses(assessment, "reliability")
	if err != nil {
		t.Fatalf("select historical compatibility lenses: %v", err)
	}
	policy, err := facadePolicyBytes("")
	if err != nil {
		t.Fatalf("read historical compatibility policy: %v", err)
	}
	state, err := reviewtransaction.NewCompactState(reviewtransaction.Start{
		LineageID: lineage, Mode: reviewtransaction.ModeOrdinaryBounded, Generation: 1,
		Snapshot: snapshot, PolicyHash: facadePayloadHash(policy), RiskLevel: assessment.Level,
		SelectedLenses: lenses, OriginalChangedLines: &assessment.ChangedLines,
	})
	if err != nil {
		t.Fatalf("create historical compatibility compact state: %v", err)
	}
	store, err := reviewtransaction.CompactAuthoritativeStore(ctx, root, lineage)
	if err != nil {
		t.Fatalf("open historical compatibility compact store: %v", err)
	}
	if _, err := store.Replace("", "review/start", state); err != nil {
		t.Fatalf("persist historical compatibility review start: %v", err)
	}
	return finalizeHistoricalFacadeReviewForLineage(t, root, lineage)
}

// finalizeHistoricalFacadeReviewForLineage advances a reviewing compact
// authority directly to its matching historical receipt. It deliberately does
// not call current FINALIZE, so approval-burn tests retain their production path.
func finalizeHistoricalFacadeReviewForLineage(t *testing.T, repo, lineage string) historicalCompatibilityCompactReceiptFixture {
	t.Helper()
	ctx := t.Context()
	store, err := reviewtransaction.CompactAuthoritativeStore(ctx, repo, lineage)
	if err != nil {
		t.Fatalf("open historical compatibility review store: %v", err)
	}
	record, err := store.Load()
	if err != nil {
		t.Fatalf("load historical compatibility reviewing authority: %v", err)
	}
	if record.State.LineageID != lineage || record.State.State != reviewtransaction.StateReviewing {
		t.Fatalf("historical compatibility reviewing precondition failed: %#v", record.State)
	}
	state := record.State
	results := make([]reviewtransaction.LensResult, len(state.SelectedLenses))
	for index, lens := range state.SelectedLenses {
		results[index] = reviewtransaction.LensResult{Lens: lens, Findings: []reviewtransaction.Finding{}, Evidence: []string{"historical compatibility review completed"}}
	}
	if err := state.CompleteReview(reviewtransaction.CompactReviewInput{LensResults: results}); err != nil {
		t.Fatalf("complete historical compatibility review: %v", err)
	}
	revision, err := store.Replace(record.Revision, "review/complete-review", state)
	if err != nil {
		t.Fatalf("persist historical compatibility completed review: %v", err)
	}
	if err := state.CompleteVerification([]byte("historical compatibility verification passed\n"), true); err != nil {
		t.Fatalf("complete historical compatibility verification: %v", err)
	}
	return persistHistoricalCompatibilityCompactApproval(t, store, reviewtransaction.CompactRecord{Revision: revision, State: state}, "review/complete-verification", state)
}

// persistHistoricalCompatibilityCompactApproval writes one approved compact
// record and matching receipt directly. It is intentionally limited to test
// fixtures for authority that predates current FINALIZE's approval burn.
func persistHistoricalCompatibilityCompactApproval(t *testing.T, store reviewtransaction.CompactStore, expected reviewtransaction.CompactRecord, operation string, state reviewtransaction.CompactState) historicalCompatibilityCompactReceiptFixture {
	t.Helper()
	if state.State != reviewtransaction.StateApproved {
		t.Fatalf("historical compatibility terminal state = %q, want approved", state.State)
	}
	if _, err := os.Stat(store.ReceiptPath()); err == nil {
		t.Fatalf("historical compatibility receipt already exists; refusing to overwrite %q", store.ReceiptPath())
	} else if !os.IsNotExist(err) {
		t.Fatalf("inspect historical compatibility receipt: %v", err)
	}
	targetIdentity := expected.State.InitialSnapshot.Identity
	predecessor := expected.State.Recovery
	revision, err := store.Replace(expected.Revision, operation, state)
	if err != nil {
		t.Fatalf("persist historical compatibility approved review: %v", err)
	}
	receipt, err := state.Receipt()
	if err != nil {
		t.Fatalf("create historical compatibility receipt: %v", err)
	}
	if err := reviewtransaction.WriteCompactReceiptAtomic(store.ReceiptPath(), receipt); err != nil {
		t.Fatalf("persist historical compatibility receipt: %v", err)
	}
	persistedReceiptBytes, err := os.ReadFile(store.ReceiptPath())
	if err != nil {
		t.Fatalf("read historical compatibility receipt: %v", err)
	}
	persistedReceipt, err := reviewtransaction.ParseCompactReceipt(persistedReceiptBytes)
	if err != nil || !reviewtransaction.CompactReceiptEqual(receipt, persistedReceipt) {
		t.Fatalf("historical compatibility receipt binding = %#v, %v", persistedReceipt, err)
	}
	approved, err := store.Load()
	if err != nil || approved.Revision != revision || approved.State.State != reviewtransaction.StateApproved || approved.State.InitialSnapshot.Identity != targetIdentity {
		t.Fatalf("historical compatibility approved record = %#v, %v", approved, err)
	}
	if predecessor == nil && approved.State.Recovery != nil {
		t.Fatalf("historical compatibility direct approval acquired unexpected recovery provenance: %#v", approved.State.Recovery)
	}
	if predecessor != nil && (approved.State.Recovery == nil ||
		approved.State.Recovery.PredecessorLineageID != predecessor.PredecessorLineageID ||
		approved.State.Recovery.PredecessorRevision != predecessor.PredecessorRevision) {
		t.Fatalf("historical compatibility recovery provenance changed: %#v", approved.State.Recovery)
	}
	return historicalCompatibilityCompactReceiptFixture{Record: approved, Store: store}
}

// finalizeHistoricalCapturedFacadeReviewForLineage preserves the current
// START/lens-context/capture-result setup, then persists its terminal compact
// receipt directly so the receipt can remain available to the provenance test.
func finalizeHistoricalCapturedFacadeReviewForLineage(t *testing.T, repo, lineage string) historicalCompatibilityCompactReceiptFixture {
	t.Helper()
	ctx := t.Context()
	store, err := reviewtransaction.CompactAuthoritativeStore(ctx, repo, lineage)
	if err != nil {
		t.Fatalf("open historical captured review store: %v", err)
	}
	record, err := store.Load()
	if err != nil {
		t.Fatalf("load historical captured review authority: %v", err)
	}
	if record.State.State != reviewtransaction.StateReviewing {
		t.Fatalf("historical captured review precondition = %#v", record.State)
	}
	results, err := readCapturedReviewerResults(ctx, repo, store.Dir, record.State, record.Revision)
	if err != nil {
		t.Fatalf("read captured historical reviewer results: %v", err)
	}
	input, err := prepareCompactReviewerResults(record.State, results, facadeRefuterResult{}, facadeRepositoryEvidence{ctx: ctx, repo: repo})
	if err != nil {
		t.Fatalf("prepare captured historical reviewer results: %v", err)
	}
	state := record.State
	state.ReviewerContextLevel = discoverReviewerContextLevel(ctx, repo, store.Dir, state, record.Revision)
	if err := state.CompleteReview(input); err != nil {
		t.Fatalf("complete captured historical review: %v", err)
	}
	revision, err := store.Replace(record.Revision, "review/complete-review", state)
	if err != nil {
		t.Fatalf("persist captured historical review: %v", err)
	}
	if err := state.CompleteVerification([]byte("historical captured review verification passed\n"), true); err != nil {
		t.Fatalf("complete captured historical verification: %v", err)
	}
	return persistHistoricalCompatibilityCompactApproval(t, store, reviewtransaction.CompactRecord{Revision: revision, State: state}, "review/complete-verification", state)
}

// finalizeHistoricalCorrectionFacadeReviewForLineage preserves a production
// correction forecast and evidence capture, then directly persists the matching
// historical approval and receipt for compatibility-gate evaluation.
func finalizeHistoricalCorrectionFacadeReviewForLineage(t *testing.T, repo, lineage string, validation facadeValidationResult) historicalCompatibilityCompactReceiptFixture {
	t.Helper()
	ctx := t.Context()
	store, err := reviewtransaction.CompactAuthoritativeStore(ctx, repo, lineage)
	if err != nil {
		t.Fatalf("open historical correction review store: %v", err)
	}
	record, err := store.Load()
	if err != nil {
		t.Fatalf("load historical correction review authority: %v", err)
	}
	fix, err := facadeVerificationEvidenceTarget(ctx, repo, record.State, record.Revision)
	if err != nil {
		t.Fatalf("build historical correction verification target: %v", err)
	}
	captured, err := reviewtransaction.ReadCapturedVerificationEvidence(store.Dir, lineage, record.Revision, fix)
	if err != nil {
		t.Fatalf("read captured historical correction evidence: %v", err)
	}
	request, err := reviewtransaction.BuildTargetedValidationRequestFromSnapshot(ctx, repo, record.State, record.Revision, fix)
	if err != nil {
		t.Fatalf("build historical correction validation request: %v", err)
	}
	native, err := validation.compact(reviewtransaction.FixDeltaHashForSnapshot(fix), record.State.FixFindingIDs, request)
	if err != nil {
		t.Fatalf("bind historical correction validation: %v", err)
	}
	builder := reviewtransaction.SnapshotBuilder{Repo: repo}
	actual, err := builder.ChangedLines(ctx, fix)
	if err != nil {
		t.Fatalf("measure historical correction: %v", err)
	}
	complete, err := builder.BuildCorrectedCandidate(ctx, record.State.InitialSnapshot, fix)
	if err != nil {
		t.Fatalf("build historical corrected candidate: %v", err)
	}
	state := record.State
	if err := state.CompleteCorrectionVerification(fix, actual, native, captured.Record, captured.Payload, complete); err != nil {
		t.Fatalf("complete historical correction verification: %v", err)
	}
	return persistHistoricalCompatibilityCompactApproval(t, store, record, "review/complete-correction-verification", state)
}

// completeHistoricalCompatibilityApprovedRecoveryReceipt advances one existing
// recovery successor through the shared historical completion seam.
func completeHistoricalCompatibilityApprovedRecoveryReceipt(t *testing.T, repo, lineage string) {
	t.Helper()
	fixture := finalizeHistoricalFacadeReviewForLineage(t, repo, lineage)
	if fixture.Record.State.Recovery == nil || fixture.Record.State.Recovery.PredecessorLineageID == "" {
		t.Fatalf("historical compatibility recovery precondition failed: %#v", fixture.Record.State)
	}
}
