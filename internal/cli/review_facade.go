package cli

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
	"sync/atomic"
	"time"

	"github.com/gentleman-programming/gentle-ai/internal/reviewtransaction"
	"github.com/gentleman-programming/gentle-ai/internal/sddstatus"
)

const facadeReviewPolicy = `Gentle AI native bounded review policy.

Only candidate-caused BLOCKER or CRITICAL findings may require correction. Pre-existing and base-only findings are follow-ups. One correction is bounded by the frozen original scope, and delivery gates validate the terminal receipt against live Git evidence.
`

type ReviewFacadeStartResult struct {
	Operation        string                       `json:"operation"`
	Action           string                       `json:"action"`
	LensesRequired   bool                         `json:"lenses_required"`
	LineageID        string                       `json:"lineage_id"`
	State            reviewtransaction.State      `json:"state"`
	RiskLevel        reviewtransaction.RiskLevel  `json:"risk_level"`
	SelectedLenses   []string                     `json:"selected_lenses"`
	LensBindings     []ReviewFacadeLensBinding    `json:"lens_bindings"`
	Projection       reviewtransaction.Projection `json:"projection"`
	TargetMode       reviewtransaction.TargetKind `json:"target_mode,omitempty"`
	TargetIdentity   string                       `json:"target_identity,omitempty"`
	BaseTree         string                       `json:"base_tree,omitempty"`
	CandidateTree    string                       `json:"candidate_tree,omitempty"`
	ChangedFiles     int                          `json:"changed_files"`
	ChangedLines     int                          `json:"changed_lines"`
	CorrectionBudget int                          `json:"correction_budget"`
}

// ReviewFacadeLensBinding pairs one selected lens with its frozen zero-based
// order so orchestrators build capture bindings exclusively from START output.
type ReviewFacadeLensBinding struct {
	Lens  string `json:"lens"`
	Order int    `json:"order"`
}

func facadeLensBindings(lenses []string) []ReviewFacadeLensBinding {
	bindings := make([]ReviewFacadeLensBinding, len(lenses))
	for order, lens := range lenses {
		bindings[order] = ReviewFacadeLensBinding{Lens: lens, Order: order}
	}
	return bindings
}

func facadeProjection(projection reviewtransaction.Projection) reviewtransaction.Projection {
	if projection == "" {
		return reviewtransaction.ProjectionWorkspace
	}
	return projection
}

type ReviewFacadeFinalizeResult struct {
	Operation     string                  `json:"operation"`
	LineageID     string                  `json:"lineage_id"`
	State         reviewtransaction.State `json:"state"`
	Action        string                  `json:"action"`
	StoreRevision string                  `json:"store_revision"`
	ReceiptPath   string                  `json:"receipt_path,omitempty"`
}

type ReviewReceiptDiscoveryKind string

const (
	ReviewReceiptMissing      ReviewReceiptDiscoveryKind = "receipt_missing"
	ReviewReceiptUnrelated    ReviewReceiptDiscoveryKind = "receipt_unrelated"
	ReviewReceiptScopeChanged ReviewReceiptDiscoveryKind = "receipt_scope_changed"
	ReviewReceiptAmbiguous    ReviewReceiptDiscoveryKind = "receipt_ambiguous"
	ReviewAuthorityCorrupted  ReviewReceiptDiscoveryKind = "authority_corrupted"
)

type ReviewReceiptDiscoveryError struct {
	Kind       ReviewReceiptDiscoveryKind
	Category   string
	Candidates []string
	Context    *reviewtransaction.GateContext
}

func (err *ReviewReceiptDiscoveryError) Error() string {
	switch err.Kind {
	case ReviewReceiptMissing:
		return "no terminal review receipt exists for gate validation"
	case ReviewReceiptUnrelated:
		return "terminal review receipts exist only for unrelated targets"
	case ReviewReceiptScopeChanged:
		return "terminal review receipts do not exactly match the live gate target"
	case ReviewReceiptAmbiguous:
		return "multiple terminal review receipts require explicit target selection"
	case ReviewAuthorityCorrupted:
		return "complete review authority inventory is unavailable or corrupted"
	default:
		return "review receipt discovery failed"
	}
}

// ReviewFacadeReceiptPublicationError reports the only safe interpretation of
// a terminal authority whose derived receipt could not be materialized.
type ReviewFacadeReceiptPublicationError struct {
	MutationOutcome string `json:"mutation_outcome"`
	Replayability   string `json:"replayability"`
	LineageID       string `json:"lineage_id"`
	RequestDigest   string `json:"request_digest"`
	Cause           error  `json:"-"`
}

func (err *ReviewFacadeReceiptPublicationError) Error() string {
	return fmt.Sprintf(
		"write compact review receipt: %v (mutation_outcome: %s, replayability: %s, lineage: %s, request_digest: %s)",
		err.Cause, err.MutationOutcome, err.Replayability, err.LineageID, err.RequestDigest,
	)
}

func (err *ReviewFacadeReceiptPublicationError) Unwrap() error { return err.Cause }

type reviewFacadeOperationProgressError struct {
	LineageID            string
	StoreRevision        string
	CommittedTransitions int
	Cause                error
	committed            *atomic.Pointer[reviewFacadeOperationProgressError]
}

func (err *reviewFacadeOperationProgressError) Error() string {
	return fmt.Sprintf("review finalize failed after %d committed native transition(s) for lineage %q at revision %s: %v",
		err.CommittedTransitions, err.LineageID, err.StoreRevision, err.Cause)
}

func (err *reviewFacadeOperationProgressError) Unwrap() error { return err.Cause }

func (err *reviewFacadeOperationProgressError) record(lineage, revision string) {
	err.LineageID = lineage
	err.StoreRevision = revision
	err.CommittedTransitions++
	if err.committed != nil {
		snapshot := *err
		err.committed.Store(&snapshot)
	}
}

var writeCompactFacadeReceipt = reviewtransaction.WriteCompactReceiptAtomic
var reviewFacadeSyncDirectory = reviewtransaction.SyncReviewDirectory
var reviewRecoverBeforePersist = func() {}

type ReviewInvalidateResult struct {
	Operation     string                  `json:"operation"`
	LineageID     string                  `json:"lineage_id"`
	State         reviewtransaction.State `json:"state"`
	StoreRevision string                  `json:"store_revision"`
}

type ReviewRecoverResult struct {
	Operation      string                                      `json:"operation"`
	LineageID      string                                      `json:"lineage_id"`
	State          reviewtransaction.State                     `json:"state"`
	StoreRevision  string                                      `json:"store_revision"`
	Projection     reviewtransaction.Projection                `json:"projection"`
	TargetIdentity string                                      `json:"target_identity"`
	Recovery       reviewtransaction.CompactRecoveryProvenance `json:"recovery"`
}

type facadeFinding struct {
	ID                string                              `json:"id,omitempty"`
	Lens              string                              `json:"lens,omitempty"`
	Location          string                              `json:"location,omitempty"`
	Severity          string                              `json:"severity,omitempty"`
	Claim             string                              `json:"claim,omitempty"`
	ProofRefs         []string                            `json:"proof_refs,omitempty"`
	EvidenceClass     reviewtransaction.EvidenceClass     `json:"evidence_class,omitempty"`
	CausalDisposition reviewtransaction.CausalDisposition `json:"causal_disposition,omitempty"`
}

type facadeReviewerResult struct {
	Lens     string          `json:"lens,omitempty"`
	Findings []facadeFinding `json:"findings"`
	Evidence []string        `json:"evidence"`
}

type facadeValidationCheck struct {
	Passed   bool     `json:"passed"`
	Evidence []string `json:"evidence"`
}

type facadeValidationResult struct {
	OriginalCriteria     facadeValidationCheck        `json:"original_criteria"`
	CorrectionRegression facadeValidationCheck        `json:"correction_regression"`
	FollowUps            []reviewtransaction.FollowUp `json:"follow_ups"`
}

type facadeRefuterResult struct {
	Results []facadeRefuterOutcome `json:"results"`
}

type facadeRefuterOutcome struct {
	FindingID string                            `json:"finding_id"`
	Outcome   reviewtransaction.EvidenceOutcome `json:"outcome"`
	ProofRefs []string                          `json:"proof_refs"`
}

type facadeArtifacts struct {
	policy, ledger, evidence, fixDelta, receipt string
}

var reviewFacadeOperationTimeout = 25 * time.Second
var reviewFacadeCommandRunner = runReviewCommandContext
var reviewFacadePlannedTransitionHook = func(context.Context, string, string, string) error { return nil }
var reviewFacadeCommittedTransitionHook = func(context.Context, string, string, string) error { return nil }

func RunReview(args []string, stdout io.Writer) error {
	if len(args) == 0 || args[0] == "help" || args[0] == "-h" || args[0] == "--help" {
		_, _ = fmt.Fprintln(stdout, "Usage: gentle-ai review <capabilities|start|finalize|validate|status|invalidate|recover|reclaim|reconcile-authority|schema|bind-sdd> [flags]\n\nOrdinary review facade; repository scope, authority, canonical artifacts, and lifecycle transitions are derived by Go.")
		_, _ = fmt.Fprintln(stdout, "Additive headless capability: gentle-ai review capture-result.")
		return nil
	}
	operation, negotiated, preflightFailure := reviewIntegrationFailureRoute(args)
	if preflightFailure != nil {
		if err := emitReviewIntegrationFailure(stdout, *preflightFailure); err != nil {
			return err
		}
		return newReviewIntegrationFailureError(*preflightFailure, nil)
	}
	if !negotiated {
		return runReviewCommand(args, stdout)
	}
	ctx, cancel := context.WithTimeout(context.Background(), reviewFacadeOperationTimeout)
	defer cancel()
	var committed atomic.Pointer[reviewFacadeOperationProgressError]
	ctx = context.WithValue(ctx, reviewFacadeOperationProgressError{}, &committed)
	var output bytes.Buffer
	result := make(chan error, 1)
	go func(runner func(context.Context, []string, io.Writer) error) { result <- runner(ctx, args, &output) }(reviewFacadeCommandRunner)
	var runErr error
	select {
	case runErr = <-result:
		if runErr == nil && operation != ReviewIntegrationOperationBindSDD {
			runErr = ctx.Err()
		}
	case <-ctx.Done():
		if operation == ReviewIntegrationOperationBindSDD {
			runErr = <-result
		} else if progress := committed.Load(); progress != nil {
			progress.Cause = &reviewtransaction.GitCommandTimeoutError{Timeout: reviewFacadeOperationTimeout, Aggregate: true, Cause: ctx.Err()}
			runErr = progress
		} else {
			runErr = ctx.Err()
		}
	}
	if runErr == nil {
		_, err := io.Copy(stdout, &output)
		return err
	}
	failure := newReviewIntegrationFailure(operation, args[1:], runErr)
	if err := emitReviewIntegrationFailure(stdout, failure); err != nil {
		return err
	}
	return newReviewIntegrationFailureError(failure, runErr)
}

func runReviewCommandContext(ctx context.Context, args []string, stdout io.Writer) error {
	switch args[0] {
	case "start":
		return runReviewFacadeStart(ctx, args[1:], stdout)
	case "status":
		return runReviewStatus(ctx, args[1:], stdout)
	case "finalize":
		return runReviewFacadeFinalize(ctx, args[1:], stdout)
	case "validate":
		return runReviewFacadeValidate(ctx, args[1:], stdout)
	case "bind-sdd":
		return runReviewBindSDD(ctx, args[1:], stdout)
	default:
		return runReviewCommand(args, stdout)
	}
}

func runReviewCommand(args []string, stdout io.Writer) error {
	switch args[0] {
	case "capture-result":
		return RunReviewCaptureResult(args[1:], stdout)
	case "capabilities":
		return RunReviewCapabilities(args[1:], stdout)
	case "start":
		return RunReviewFacadeStart(args[1:], stdout)
	case "finalize":
		return RunReviewFacadeFinalize(args[1:], stdout)
	case "validate":
		return RunReviewFacadeValidate(args[1:], stdout)
	case "status":
		return RunReviewStatus(args[1:], stdout)
	case "invalidate":
		return RunReviewInvalidate(args[1:], stdout)
	case "recover":
		return RunReviewRecover(args[1:], stdout)
	case "reclaim":
		return RunReviewReclaim(args[1:], stdout)
	case "reconcile-authority":
		return RunReviewReconcileAuthority(args[1:], stdout)
	case "schema":
		return RunReviewSchema(args[1:], stdout)
	case "bind-sdd":
		return RunReviewBindSDD(args[1:], stdout)
	default:
		return fmt.Errorf("unknown review command %q", args[0])
	}
}

func RunReviewStatus(args []string, stdout io.Writer) error {
	return runReviewStatus(context.Background(), args, stdout)
}

func runReviewStatus(ctx context.Context, args []string, stdout io.Writer) error {
	flags := newReviewFlagSet("review status", stdout, "Read every compact-v2 and shipped legacy-v1 authority from the shared Git common directory without mutation.")
	cwd := flags.String("cwd", ".", "repository path")
	contract := flags.String("contract", "", "optional negotiated review integration contract")
	lineage := flags.String("lineage", "", "optional explicit lineage selector for negotiated target status")
	projection := flags.String("projection", string(reviewtransaction.ProjectionWorkspace), "negotiated target projection: workspace or staged")
	baseRef := flags.String("base-ref", "", "optional negotiated immutable base-to-HEAD target")
	baseTree := flags.String("base-tree", "", "optional negotiated resolved immutable overlay base tree")
	workspaceOverlay := flags.Bool("workspace-overlay", false, "select a negotiated base-ref workspace overlay target")
	if err := parseReviewFlags(flags, args); err != nil {
		return err
	}
	if reviewHelpRequested(args) {
		return nil
	}
	if flags.NArg() != 0 {
		return reviewPreflightError(fmt.Errorf("unexpected review status argument %q", flags.Arg(0)))
	}
	if *contract != "" {
		if err := validateReviewIntegrationContract(*contract); err != nil {
			return err
		}
		selectedProjection := reviewtransaction.Projection(strings.TrimSpace(*projection))
		if selectedProjection != reviewtransaction.ProjectionWorkspace && selectedProjection != reviewtransaction.ProjectionStaged {
			return fmt.Errorf("unsupported review projection %q", *projection)
		}
		selectedBaseRef := strings.TrimSpace(*baseRef)
		selectedBaseTree := strings.TrimSpace(*baseTree)
		if *workspaceOverlay && ((selectedBaseRef == "") == (selectedBaseTree == "") || selectedProjection != reviewtransaction.ProjectionWorkspace) {
			return errors.New("--workspace-overlay requires exactly one of --base-ref or --base-tree with workspace projection")
		}
		if !*workspaceOverlay && selectedBaseTree != "" {
			return errors.New("--base-tree requires --workspace-overlay")
		}
		if selectedBaseTree != "" && !validReviewGitTree(selectedBaseTree) {
			return errors.New("--base-tree requires an exact Git tree object ID")
		}
		builder := reviewtransaction.SnapshotBuilder{Repo: *cwd}
		root, err := builder.ResolveRepositoryRoot(ctx)
		if err != nil {
			return fmt.Errorf("resolve negotiated review repository root: %w", err)
		}
		intended := []string{}
		if selectedProjection != reviewtransaction.ProjectionStaged {
			intended, err = (reviewtransaction.SnapshotBuilder{Repo: root}).DiscoverIntendedUntracked(ctx)
			if err != nil {
				return fmt.Errorf("discover negotiated review target: %w", err)
			}
		}
		target := reviewtransaction.Target{Kind: reviewtransaction.TargetCurrentChanges, Projection: selectedProjection, IntendedUntracked: intended}
		if selectedBaseRef != "" {
			target.Kind, target.BaseRef = reviewtransaction.TargetBaseDiff, selectedBaseRef
		}
		if *workspaceOverlay {
			target.Kind = reviewtransaction.TargetBaseWorkspaceOverlay
			if selectedBaseTree != "" {
				target.BaseRef = selectedBaseTree
			}
		}
		native, err := reviewtransaction.AssessTargetStatus(ctx, root, reviewtransaction.TargetStatusRequest{
			Target: target, LineageID: *lineage,
		})
		if err != nil {
			return fmt.Errorf("assess negotiated review target: %w", err)
		}
		if selectedBaseTree != "" && native.Projection.BaseTree != selectedBaseTree {
			return errors.New("--base-tree does not identify an exact Git tree object")
		}
		result := newReviewTargetStatusResult(native)
		if err := result.Validate(); err != nil {
			return fmt.Errorf("validate negotiated review status: %w", err)
		}
		return encodeReviewJSON(stdout, result)
	}
	if strings.TrimSpace(*lineage) != "" || strings.TrimSpace(*baseRef) != "" || strings.TrimSpace(*baseTree) != "" || *workspaceOverlay || *projection != string(reviewtransaction.ProjectionWorkspace) {
		return errors.New("review status target selectors require --contract")
	}
	report, err := reviewtransaction.InventoryAuthority(ctx, *cwd)
	if err != nil {
		return fmt.Errorf("inventory review authority: %w", err)
	}
	return encodeReviewJSON(stdout, report)
}

func RunReviewRecover(args []string, stdout io.Writer) error {
	flags := newReviewFlagSet("review recover", stdout, "Create an auditable successor authority without changing its predecessor.")
	cwd := flags.String("cwd", ".", "repository path")
	predecessor := flags.String("predecessor-lineage", "", "explicit predecessor lineage")
	expected := flags.String("expected-predecessor-revision", "", "exact predecessor revision")
	successor := flags.String("successor-lineage", "", "distinct successor lineage")
	disposition := flags.String("disposition", "", "scope_changed, invalidated, or escalated")
	reason := flags.String("reason", "", "recovery reason")
	actor := flags.String("actor", "", "recovery actor")
	projectionFlag := flags.String("projection", "", "successor projection: workspace or staged (default: predecessor projection)")
	authorization := flags.String("maintainer-authorization", "", "exact six-line LF-only binding: gentle-ai.review-recovery-authorization/v1, predecessor_lineage, predecessor_revision, target_identity, actor, reason")
	policySource := flags.String("policy", "", "optional review policy file")
	focus := flags.String("focus", "reliability", "dominant standard-risk focus; large pure documentation always uses readability")
	baseRef := flags.String("base-ref", "", "optional base revision for immutable base-to-HEAD review")
	committedOnly := flags.Bool("committed-only", false, "acknowledge that --base-ref excludes dirty tracked changes")
	releaseScope := flags.Bool("release-scope", false, "recover an approved current-changes review into the immutable HEAD first-parent release scope")
	if err := parseReviewFlags(flags, args); err != nil {
		return err
	}
	if reviewHelpRequested(args) {
		return nil
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected review recover argument %q", flags.Arg(0))
	}
	if strings.TrimSpace(*predecessor) == "" || strings.TrimSpace(*expected) == "" || strings.TrimSpace(*successor) == "" || strings.TrimSpace(*reason) == "" || strings.TrimSpace(*actor) == "" || strings.TrimSpace(*disposition) == "" {
		return errors.New("review recover requires --predecessor-lineage, --expected-predecessor-revision, --successor-lineage, --disposition, --reason, and --actor")
	}
	builder := reviewtransaction.SnapshotBuilder{Repo: *cwd}
	root, err := builder.ResolveRepositoryRoot(context.Background())
	if err != nil {
		return fmt.Errorf("resolve review repository root: %w", err)
	}
	predecessorStore, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), root, *predecessor)
	if err != nil {
		return err
	}
	predecessorRecord, err := predecessorStore.Load()
	if err != nil {
		return fmt.Errorf("load recovery predecessor: %w", err)
	}
	base := strings.TrimSpace(*baseRef)
	baseDiff := predecessorRecord.State.InitialSnapshot.Kind == reviewtransaction.TargetBaseDiff
	overlay := predecessorRecord.State.InitialSnapshot.Kind == reviewtransaction.TargetBaseWorkspaceOverlay
	if *releaseScope && (base != "" || *committedOnly) {
		return errors.New("--release-scope cannot be combined with --base-ref or --committed-only")
	}
	if *releaseScope && reviewtransaction.RecoveryDisposition(*disposition) != reviewtransaction.RecoveryScopeChanged {
		return errors.New("--release-scope requires --disposition scope_changed")
	}
	if *releaseScope && predecessorRecord.State.InitialSnapshot.Kind != reviewtransaction.TargetCurrentChanges {
		return errors.New("--release-scope requires a current-changes predecessor")
	}
	if !*releaseScope && (*committedOnly != (base != "") || baseDiff != *committedOnly) {
		return errors.New("base-diff recovery requires matching --base-ref and --committed-only")
	}
	projection := predecessorRecord.State.InitialSnapshot.Projection
	if selected := strings.TrimSpace(*projectionFlag); selected != "" {
		projection = reviewtransaction.Projection(selected)
		if projection != reviewtransaction.ProjectionWorkspace && projection != reviewtransaction.ProjectionStaged {
			return fmt.Errorf("unsupported review recovery projection %q", selected)
		}
	}
	intended := []string{}
	if projection != reviewtransaction.ProjectionStaged {
		intended, err = builder.DiscoverIntendedUntracked(context.Background())
		if err != nil {
			return err
		}
	}
	target := reviewtransaction.Target{Kind: reviewtransaction.TargetCurrentChanges, Projection: projection, IntendedUntracked: intended}
	if *committedOnly {
		target.Kind, target.BaseRef = reviewtransaction.TargetBaseDiff, base
	} else if overlay {
		target.Kind, target.BaseRef = reviewtransaction.TargetBaseWorkspaceOverlay, predecessorRecord.State.InitialSnapshot.BaseTree
	}
	var snapshot reviewtransaction.Snapshot
	if *releaseScope {
		snapshot, err = reviewtransaction.BuildReleaseScopeSnapshot(context.Background(), root)
	} else {
		snapshot, err = builder.Build(context.Background(), target)
	}
	if err != nil {
		return err
	}
	if !*releaseScope && (baseDiff || overlay) && snapshot.BaseTree != predecessorRecord.State.InitialSnapshot.BaseTree {
		return errors.New("recovery base-ref does not match predecessor base")
	}
	if !*releaseScope && (baseDiff || overlay) && snapshot.Identity == predecessorRecord.State.InitialSnapshot.Identity {
		return errors.New("recovery scope has not changed")
	}
	assessment, err := (reviewtransaction.SnapshotBuilder{Repo: root}).AssessSnapshotRisk(context.Background(), snapshot)
	if err != nil {
		return err
	}
	risk, changedLines := assessment.Level, assessment.ChangedLines
	lenses, err := facadeSelectedLenses(assessment, *focus)
	if err != nil {
		return err
	}
	policy, err := facadePolicyBytes(*policySource)
	if err != nil {
		return err
	}
	state, err := reviewtransaction.NewCompactState(reviewtransaction.Start{
		LineageID: *successor, Mode: reviewtransaction.ModeOrdinaryBounded, Generation: predecessorRecord.State.Generation + 1,
		Snapshot: snapshot, PolicyHash: facadePayloadHash(policy), RiskLevel: risk, SelectedLenses: lenses, OriginalChangedLines: &changedLines,
	})
	if err != nil {
		return err
	}
	if *releaseScope {
		*authorization = reviewtransaction.ReleaseScopeRecoveryAuthorization
	}
	reviewRecoverBeforePersist()
	record, err := reviewtransaction.RecoverCompactAuthority(context.Background(), root, reviewtransaction.CompactRecoveryRequest{
		PredecessorLineageID: *predecessor, ExpectedPredecessorRevision: *expected, Successor: state,
		Disposition: reviewtransaction.RecoveryDisposition(*disposition), Reason: *reason, Actor: *actor, MaintainerAuthorization: *authorization,
	})
	if err != nil {
		return err
	}
	return encodeReviewJSON(stdout, ReviewRecoverResult{Operation: "review/recover", LineageID: record.State.LineageID, State: record.State.State,
		StoreRevision: record.Revision, Projection: facadeProjection(snapshot.Projection), TargetIdentity: snapshot.Identity, Recovery: *record.State.Recovery})
}

func RunReviewBindSDD(args []string, stdout io.Writer) error {
	return runReviewBindSDD(context.Background(), args, stdout)
}

func runReviewBindSDD(ctx context.Context, args []string, stdout io.Writer) error {
	flags := newReviewFlagSet("review bind-sdd", stdout, "Bind an explicit approved compact lineage to an OpenSpec change.")
	cwd := flags.String("cwd", "", "repository path")
	contract := flags.String("contract", "", "optional negotiated review integration contract")
	change := flags.String("change", "", "OpenSpec change")
	lineage := flags.String("lineage", "", "approved lineage")
	expected := flags.String("expected-binding-revision", "", "binding revision")
	if err := parseReviewFlags(flags, args); err != nil {
		return err
	}
	if reviewHelpRequested(args) {
		return nil
	}
	if flags.NArg() != 0 {
		return reviewPreflightError(fmt.Errorf("unexpected review bind-sdd argument %q", flags.Arg(0)))
	}
	negotiated, err := reviewIntegrationNegotiation(flags, *contract)
	if err != nil {
		return err
	}
	hasExpected := false
	for _, arg := range args {
		hasExpected = hasExpected || arg == "--expected-binding-revision" || strings.HasPrefix(arg, "--expected-binding-revision=")
	}
	if strings.TrimSpace(*cwd) == "" || strings.TrimSpace(*change) == "" || strings.TrimSpace(*lineage) == "" || !hasExpected {
		return errors.New("review bind-sdd requires --cwd, --change, --lineage, and --expected-binding-revision")
	}
	binding, err := sddstatus.BindApprovedReview(ctx, *cwd, *change, *lineage, *expected)
	if err != nil {
		return err
	}
	return encodeReviewIntegrationOperation(stdout, negotiated, ReviewIntegrationOperationBindSDD, binding, binding)
}

func RunReviewInvalidate(args []string, stdout io.Writer) error {
	flags := newReviewFlagSet("review invalidate", stdout, "Terminally invalidate one explicit pristine reviewing authority.")
	cwd := flags.String("cwd", "", "repository path")
	lineage := flags.String("lineage", "", "explicit review lineage identifier")
	expected := flags.String("expected-revision", "", "exact current authority revision")
	reason := flags.String("reason", "", "non-empty terminal invalidation reason")
	if err := parseReviewFlags(flags, args); err != nil {
		return err
	}
	if reviewHelpRequested(args) {
		return nil
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected review invalidate argument %q", flags.Arg(0))
	}
	if strings.TrimSpace(*cwd) == "" || strings.TrimSpace(*lineage) == "" || strings.TrimSpace(*expected) == "" || strings.TrimSpace(*reason) == "" {
		return errors.New("review invalidate requires --cwd, --lineage, --expected-revision, and --reason")
	}
	root, err := (reviewtransaction.SnapshotBuilder{Repo: *cwd}).ResolveRepositoryRoot(context.Background())
	if err != nil {
		return fmt.Errorf("resolve review repository root: %w", err)
	}
	compact, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), root, *lineage)
	if err != nil {
		return err
	}
	record, loadErr := compact.Load()
	if loadErr == nil {
		legacy, legacyErr := reviewtransaction.AuthoritativeStore(context.Background(), root, *lineage)
		if legacyErr == nil {
			if _, legacyLoadErr := legacy.LoadChain(); legacyLoadErr == nil {
				return errors.New("review authority is ambiguous across compact v2 and legacy v1 stores")
			}
		}
		state := record.State
		if state.State != reviewtransaction.StateInvalidated || state.InvalidationReason != strings.TrimSpace(*reason) {
			if err := state.Invalidate(*reason); err != nil {
				return err
			}
		}
		revision, err := compact.Replace(*expected, "review/invalidate", state)
		if err != nil {
			return err
		}
		return encodeReviewJSON(stdout, ReviewInvalidateResult{Operation: "review/invalidate", LineageID: state.LineageID, State: state.State, StoreRevision: revision})
	}
	if !errors.Is(loadErr, os.ErrNotExist) {
		return fmt.Errorf("load explicit compact review lineage: %w", loadErr)
	}
	legacy, err := reviewtransaction.AuthoritativeStore(context.Background(), root, *lineage)
	if err != nil {
		return err
	}
	chain, err := legacy.LoadChain()
	if err != nil {
		return fmt.Errorf("load explicit review lineage: %w", err)
	}
	revision, err := legacy.InvalidatePristine(*expected, *reason, chain.Records[len(chain.Records)-1].Transaction.Snapshot)
	if err != nil {
		return err
	}
	return encodeReviewJSON(stdout, ReviewInvalidateResult{Operation: "review/invalidate", LineageID: *lineage, State: reviewtransaction.StateInvalidated, StoreRevision: revision})
}

func RunReviewFacadeStart(args []string, stdout io.Writer) error {
	return runReviewFacadeStart(context.Background(), args, stdout)
}

func runReviewFacadeStart(ctx context.Context, args []string, stdout io.Writer) error {
	flags := newReviewFlagSet("review start", stdout, "Freeze live Git scope and derive the bounded review tier, lenses, and correction budget.")
	cwd := flags.String("cwd", ".", "repository path")
	contract := flags.String("contract", "", "optional negotiated review integration contract")
	lineage := flags.String("lineage", "", "optional explicit review lineage identifier")
	policySource := flags.String("policy", "", "optional review policy file; the native bounded policy is used by default")
	focus := flags.String("focus", "reliability", "dominant standard-risk focus: risk, resilience, readability, or reliability; large pure documentation always uses readability")
	baseRef := flags.String("base-ref", "", "optional base revision for immutable base-to-HEAD review")
	projection := flags.String("projection", string(reviewtransaction.ProjectionWorkspace), "candidate projection: workspace or staged; staged base-diff records post-commit delivery provenance")
	committedOnly := flags.Bool("committed-only", false, "acknowledge that --base-ref excludes dirty tracked changes")
	workspaceOverlay := flags.Bool("workspace-overlay", false, "include branch commits and the live workspace over --base-ref")
	tracePath := flags.String("trace", "", "optional diagnostic operation metadata trace path")
	if err := parseReviewFlags(flags, args); err != nil {
		return err
	}
	if reviewHelpRequested(args) {
		return nil
	}
	if flags.NArg() != 0 {
		return reviewPreflightError(fmt.Errorf("unexpected review start argument %q", flags.Arg(0)))
	}
	negotiated, err := reviewIntegrationNegotiation(flags, *contract)
	if err != nil {
		return err
	}
	builder := reviewtransaction.SnapshotBuilder{Repo: *cwd}
	root, err := builder.ResolveRepositoryRoot(ctx)
	if err != nil {
		return fmt.Errorf("resolve review repository root: %w", err)
	}
	selectedProjection := reviewtransaction.Projection(strings.TrimSpace(*projection))
	if selectedProjection != reviewtransaction.ProjectionWorkspace && selectedProjection != reviewtransaction.ProjectionStaged {
		return fmt.Errorf("unsupported review projection %q", *projection)
	}
	if *workspaceOverlay && (strings.TrimSpace(*baseRef) == "" || *committedOnly || selectedProjection != reviewtransaction.ProjectionWorkspace) {
		return errors.New("--workspace-overlay requires --base-ref with workspace projection and is incompatible with --committed-only")
	}
	if strings.TrimSpace(*baseRef) != "" && !*workspaceOverlay {
		dirtyTracked, dirtyErr := (reviewtransaction.SnapshotBuilder{Repo: root}).HasDirtyTrackedChanges(ctx)
		if dirtyErr != nil {
			return fmt.Errorf("detect dirty tracked changes for committed review: %w", dirtyErr)
		}
		if dirtyTracked && !*committedOnly {
			return errors.New("review start with --base-ref omits dirty tracked changes; rerun with --committed-only to acknowledge committed-only review scope")
		}
	}
	intended := []string{}
	if selectedProjection != reviewtransaction.ProjectionStaged {
		intended, err = builder.DiscoverIntendedUntracked(ctx)
		if err != nil {
			return fmt.Errorf("discover intended untracked files: %w", err)
		}
	}
	target := reviewtransaction.Target{Kind: reviewtransaction.TargetCurrentChanges, Projection: selectedProjection, IntendedUntracked: intended}
	if strings.TrimSpace(*baseRef) != "" {
		target.Kind = reviewtransaction.TargetBaseDiff
		target.BaseRef = strings.TrimSpace(*baseRef)
	}
	if *workspaceOverlay {
		target.Kind = reviewtransaction.TargetBaseWorkspaceOverlay
	}
	snapshot, err := (reviewtransaction.SnapshotBuilder{Repo: root}).Build(ctx, target)
	if err != nil {
		return fmt.Errorf("build facade review target: %w", err)
	}
	assessment, err := (reviewtransaction.SnapshotBuilder{Repo: root}).AssessSnapshotRisk(ctx, snapshot)
	if err != nil {
		return fmt.Errorf("classify facade review target: %w", err)
	}
	risk, changedLines := assessment.Level, assessment.ChangedLines
	lenses, err := facadeSelectedLenses(assessment, *focus)
	if err != nil {
		return err
	}
	explicitLineage := strings.TrimSpace(*lineage) != ""
	if !explicitLineage {
		*lineage = "review-" + strings.TrimPrefix(snapshot.Identity, "sha256:")[:16]
	}
	legacy, err := reviewtransaction.AuthoritativeStore(ctx, root, *lineage)
	if err == nil {
		if _, loadErr := legacy.LoadChain(); loadErr == nil {
			return fmt.Errorf("%w: choose a new lineage for compact authority", reviewtransaction.NewLegacyReadOnlyError("review/start", *lineage))
		}
	}
	policy, err := facadePolicyBytes(*policySource)
	if err != nil {
		return err
	}
	state, err := reviewtransaction.NewCompactState(reviewtransaction.Start{
		LineageID: *lineage, Mode: reviewtransaction.ModeOrdinaryBounded, Generation: 1,
		Snapshot: snapshot, PolicyHash: facadePayloadHash(policy), RiskLevel: risk,
		SelectedLenses: lenses, OriginalChangedLines: &changedLines,
	})
	if err != nil {
		return fmt.Errorf("create compact facade review: %w", err)
	}
	started, err := reviewtransaction.StartCompactAuthority(ctx, root, reviewtransaction.CompactStartRequest{
		State: state, TracePath: strings.TrimSpace(*tracePath), ExplicitLineage: explicitLineage,
	})
	if err != nil {
		return fmt.Errorf("start compact facade review: %w", err)
	}
	authority := started.Record.State
	legacyResult := ReviewFacadeStartResult{
		Operation: "review/start", Action: string(started.Action), LensesRequired: started.LensesRequired,
		LineageID: authority.LineageID, State: authority.State, RiskLevel: authority.RiskLevel,
		SelectedLenses: append([]string{}, authority.SelectedLenses...), LensBindings: facadeLensBindings(authority.SelectedLenses),
		Projection:   facadeProjection(authority.InitialSnapshot.Projection),
		ChangedFiles: len(authority.InitialSnapshot.Paths), TargetIdentity: authority.InitialSnapshot.Identity,
		ChangedLines: authority.OriginalChangedLines, CorrectionBudget: authority.CorrectionBudget,
	}
	if authority.InitialSnapshot.Kind == reviewtransaction.TargetBaseWorkspaceOverlay {
		legacyResult.TargetMode = authority.InitialSnapshot.Kind
		legacyResult.BaseTree = authority.InitialSnapshot.BaseTree
		legacyResult.CandidateTree = authority.InitialSnapshot.CandidateTree
	}
	if !negotiated {
		return encodeReviewJSON(stdout, legacyResult)
	}
	if started.Action == reviewtransaction.CompactStartRecover {
		legacyResult.Action = string(reviewtransaction.CompactStartBlocked)
	}
	if authority.InitialSnapshot.Identity != snapshot.Identity {
		assessment, err = (reviewtransaction.SnapshotBuilder{Repo: root}).AssessSnapshotRisk(ctx, authority.InitialSnapshot)
		if err != nil {
			return fmt.Errorf("classify authoritative negotiated START target: %w", err)
		}
	}
	negotiatedResult, err := newReviewIntegrationStartResult(legacyResult, assessment, authority.InitialSnapshot.Kind)
	if err != nil {
		return err
	}
	return encodeReviewJSON(stdout, negotiatedResult)
}

func RunReviewFacadeFinalize(args []string, stdout io.Writer) error {
	return runReviewFacadeFinalize(context.Background(), args, stdout)
}

func runReviewFacadeFinalize(ctx context.Context, args []string, stdout io.Writer) (returnErr error) {
	committed, _ := ctx.Value(reviewFacadeOperationProgressError{}).(*atomic.Pointer[reviewFacadeOperationProgressError])
	progress := reviewFacadeOperationProgressError{committed: committed}
	defer func() {
		if returnErr == nil || progress.CommittedTransitions == 0 {
			return
		}
		var alreadyWrapped *reviewFacadeOperationProgressError
		if errors.As(returnErr, &alreadyWrapped) {
			return
		}
		wrapped := progress
		wrapped.Cause = returnErr
		returnErr = &wrapped
	}()
	flags := newReviewFlagSet("review finalize", stdout, "Canonicalize reviewer output and evidence, perform required native transitions, and materialize the terminal receipt.")
	cwd := flags.String("cwd", ".", "repository path")
	contract := flags.String("contract", "", "optional negotiated review integration contract")
	lineage := flags.String("lineage", "", "optional lineage override when discovery is ambiguous")
	validationPath := flags.String("validation", "", "targeted correction validation JSON file or - for stdin")
	refuterPath := flags.String("refuter", "", "optional refuter outcomes JSON file or - for stdin")
	evidencePath := flags.String("evidence", "", "final test or verification evidence file or - for stdin")
	correctionLines := flags.Int("correction-lines", 0, "positive predicted correction changed lines before editing")
	failed := flags.Bool("failed", false, "bind supplied final evidence as a failed verification")
	tracePath := flags.String("trace", "", "optional diagnostic operation metadata trace path")
	var resultPaths repeatedString
	flags.Var(&resultPaths, "result", "reviewer result JSON file or - for stdin; repeat in selected-lens order")
	var resultArtifacts repeatedString
	flags.Var(&resultArtifacts, "result-artifact", "native reviewer artifact manifest JSON; repeat in selected-lens order")
	if err := parseReviewFlags(flags, args); err != nil {
		return err
	}
	if reviewHelpRequested(args) {
		return nil
	}
	if flags.NArg() != 0 {
		return reviewPreflightError(fmt.Errorf("unexpected review finalize argument %q", flags.Arg(0)))
	}
	negotiated, err := reviewIntegrationNegotiation(flags, *contract)
	if err != nil {
		return err
	}
	if countFacadeStdin(resultPaths, *validationPath, *refuterPath, *evidencePath) > 1 {
		return reviewPreflightError(errors.New("review finalize accepts stdin for only one input"))
	}
	if len(resultPaths) != 0 && len(resultArtifacts) != 0 {
		return reviewPreflightError(errors.New("review finalize cannot mix --result and --result-artifact"))
	}
	root, err := (reviewtransaction.SnapshotBuilder{Repo: *cwd}).ResolveRepositoryRoot(ctx)
	if err != nil {
		return fmt.Errorf("resolve review repository root: %w", err)
	}
	store, record, err := discoverCompactFacadeReview(ctx, root, *lineage, false)
	if err != nil {
		if _, chain, _, legacyErr := discoverFacadeReview(ctx, root, *lineage, false); legacyErr == nil {
			legacyLineage := chain.Records[len(chain.Records)-1].Transaction.LineageID
			return reviewtransaction.NewLegacyReadOnlyError("review/finalize", legacyLineage)
		}
		return err
	}
	store.TracePath = strings.TrimSpace(*tracePath)
	state := record.State
	if strings.TrimSpace(*lineage) != "" {
		leaves, err := reviewtransaction.CompactAuthorityLeaves(ctx, root)
		if err != nil {
			return err
		}
		current := false
		for _, leaf := range leaves {
			current = current || leaf.StatePath() == store.StatePath()
		}
		if !current {
			return fmt.Errorf("review lineage %q is superseded", *lineage)
		}
	}
	terminalAtEntry := facadeTerminalState(state.State)
	if state.State != reviewtransaction.StateReviewing && (len(resultArtifacts) != 0 || len(resultPaths) != 0) {
		pending, pendingErr := store.PendingFinalizeAttempt()
		if pendingErr != nil {
			return pendingErr
		}
		if terminalAtEntry || pending == nil {
			return reviewPreflightError(errors.New("reviewer results are accepted only while the authority is reviewing"))
		}
	}
	var terminalReceipt reviewtransaction.CompactReceipt
	terminalReceiptExists := false
	var terminalPending *reviewtransaction.FinalizeAttempt
	terminalComplete := false
	if terminalAtEntry {
		terminalReceipt, err = state.Receipt()
		if err != nil {
			return err
		}
		terminalPending, err = store.PendingFinalizeAttempt()
		if err != nil {
			return err
		}
		terminalReceiptExists, err = inspectCompactFacadeReceipt(store.ReceiptPath(), terminalReceipt)
		if err != nil {
			requestDigest := ""
			if terminalPending != nil {
				requestDigest = terminalPending.Request.RequestDigest
			}
			return newFacadeReceiptPublicationError(state.LineageID, requestDigest, err)
		}
		if terminalReceiptExists {
			if terminalPending == nil {
				terminalComplete = true
			}
			if terminalPending != nil && !facadeFinalizeReplayInputsEmpty(resultPaths, resultArtifacts, *validationPath, *refuterPath, *evidencePath, *correctionLines, *failed, *tracePath) {
				return errors.New("terminal review finalize accepts no review inputs; exact replay requires only --lineage")
			}
		}
		if !terminalReceiptExists {
			if !facadeFinalizeReplayInputsEmpty(resultPaths, resultArtifacts, *validationPath, *refuterPath, *evidencePath, *correctionLines, *failed, *tracePath) {
				return errors.New("terminal review finalize accepts no review inputs; exact receipt replay requires only --lineage")
			}
			if *lineage != state.LineageID || strings.TrimSpace(*lineage) != *lineage {
				return errors.New("receipt publication replay requires the exact explicit --lineage")
			}
		}
	}
	reviewerResults, err := readFacadeReviewerResults(resultPaths)
	if err != nil {
		return reviewPreflightError(err)
	}
	if len(resultArtifacts) != 0 {
		reviewerResults, err = readFacadeReviewerArtifacts(resultArtifacts, store.Dir, state)
		if err != nil {
			return reviewPreflightError(err)
		}
	}
	var validation *facadeValidationResult
	if strings.TrimSpace(*validationPath) != "" {
		validation = &facadeValidationResult{}
		if err := readFacadeJSON(*validationPath, validation); err != nil {
			return reviewPreflightError(fmt.Errorf("read targeted validation: %w", err))
		}
	}
	var refuter facadeRefuterResult
	if strings.TrimSpace(*refuterPath) != "" {
		if err := readFacadeJSON(*refuterPath, &refuter); err != nil {
			return reviewPreflightError(fmt.Errorf("read refuter outcomes: %w", err))
		}
	}
	var evidence []byte
	if strings.TrimSpace(*evidencePath) != "" {
		evidence, err = readFacadeBytes(*evidencePath)
		if err != nil {
			return reviewPreflightError(fmt.Errorf("read final review evidence: %w", err))
		}
	}
	if terminalComplete {
		if err := reviewFacadeSyncDirectory(filepath.Dir(store.FinalizeAttemptJournalPath())); err != nil {
			return fmt.Errorf("sync completed finalize journal directory: %w", err)
		}
		return encodeCompactFacadeFinalize(stdout, negotiated, state, record.Revision, store, "validate delivery with gentle-ai review validate --gate <gate>")
	}
	var attempt reviewtransaction.FinalizeAttempt
	attemptLoaded := false
	if !terminalAtEntry {
		pending, pendingErr := store.PendingFinalizeAttempt()
		if pendingErr != nil {
			return pendingErr
		}
		if index := facadeFinalizeTransitionIndex(pending, record.Revision); index >= 0 {
			replayEvidence := evidence
			if len(replayEvidence) == 0 && facadeNativeLowRiskCandidate(state) {
				replayEvidence, err = prepareFacadeNativeLowRiskVerification(ctx, root, state)
				if err != nil {
					return reviewPreflightError(err)
				}
			}
			replayRequest := facadeFinalizeAttemptRequestForCandidate(record, state.CurrentSnapshot, reviewerResults, validation, refuter, replayEvidence, *correctionLines, *failed)
			attempt, attemptLoaded, err = store.ReconcileFinalizeAttempt(ctx, replayRequest)
			if err != nil {
				return err
			}
			if index == len(attempt.Transitions)-1 {
				if err := store.CompleteFinalizeAttempt(attempt.Request.RequestDigest); err != nil {
					return err
				}
				return encodeCompactFacadeFinalize(stdout, negotiated, state, record.Revision, store, "continue the current review state")
			}
		}
	}
	plan, err := prepareFacadeFinalizePlan(ctx, root, state, reviewerResults, refuter, validation, evidence, *correctionLines, *failed)
	if err != nil {
		return reviewPreflightError(err)
	}
	request := facadeFinalizeAttemptRequestForCandidate(record, plan.Candidate, reviewerResults, validation, refuter, plan.Evidence, *correctionLines, *failed)
	if terminalAtEntry {
		if terminalPending != nil {
			attempt = *terminalPending
		} else {
			attempt, err = facadePendingFinalizeAttempt(store, request)
		}
	} else if !attemptLoaded {
		attempt, _, err = store.ReconcileFinalizeAttempt(ctx, request)
	}
	if err != nil {
		return err
	}
	requestDigest := attempt.Request.RequestDigest
	defer func() {
		if returnErr == nil {
			completionErr := store.CompleteFinalizeAttempt(requestDigest)
			if completionErr != nil && facadeTerminalState(state.State) {
				returnErr = newFacadeReceiptPublicationError(state.LineageID, requestDigest, completionErr)
			} else {
				returnErr = completionErr
			}
		}
	}()
	plannedRevisions := make([]string, len(plan.Transitions))
	expectedRevision := record.Revision
	for index, transition := range plan.Transitions {
		planned, err := store.PlanFinalizeAttemptTransition(requestDigest, transition.Operation, expectedRevision, transition.State)
		if err != nil {
			return err
		}
		plannedRevisions[index] = planned
		expectedRevision = planned
	}
	for index, transition := range plan.Transitions {
		planned := plannedRevisions[index]
		if err := reviewFacadePlannedTransitionHook(ctx, root, transition.Operation, planned); err != nil {
			return err
		}
		revision, err := store.ReplaceContext(ctx, record.Revision, transition.Operation, transition.State)
		if err != nil {
			return err
		}
		if revision != planned {
			return errors.New("compact finalize transition did not match its planned revision")
		}
		progress.record(transition.State.LineageID, revision)
		if err := reviewFacadeCommittedTransitionHook(ctx, root, transition.Operation, revision); err != nil {
			return err
		}
		record.Revision, record.State, state = revision, transition.State, transition.State
	}

	if state.State != reviewtransaction.StateApproved && state.State != reviewtransaction.StateEscalated {
		return encodeCompactFacadeFinalize(stdout, negotiated, state, record.Revision, store, "continue the current review state")
	}
	if terminalAtEntry && terminalReceiptExists {
		return encodeCompactFacadeFinalize(stdout, negotiated, state, record.Revision, store, "validate delivery with gentle-ai review validate --gate <gate>")
	}
	receipt := terminalReceipt
	if !terminalAtEntry {
		receipt, err = state.Receipt()
		if err != nil {
			return err
		}
	}
	if err := writeCompactFacadeReceipt(store.ReceiptPath(), receipt); err != nil {
		return newFacadeReceiptPublicationError(state.LineageID, requestDigest, err)
	}
	published, err := inspectCompactFacadeReceipt(store.ReceiptPath(), receipt)
	if err != nil {
		return newFacadeReceiptPublicationError(state.LineageID, requestDigest, err)
	}
	if !published {
		return newFacadeReceiptPublicationError(state.LineageID, requestDigest, errors.New("receipt writer did not materialize the derived receipt"))
	}
	if err := store.MarkFinalizeAttemptReceiptPublished(requestDigest); err != nil {
		return err
	}
	return encodeCompactFacadeFinalize(stdout, negotiated, state, record.Revision, store, "validate delivery with gentle-ai review validate --gate <gate>")
}

func facadeFinalizeTransitionIndex(attempt *reviewtransaction.FinalizeAttempt, revision string) int {
	if attempt == nil {
		return -1
	}
	for index, transition := range attempt.Transitions {
		if transition.Revision == revision {
			return index
		}
	}
	return -1
}

func facadePendingFinalizeAttempt(store reviewtransaction.CompactStore, request reviewtransaction.FinalizeAttemptRequest) (reviewtransaction.FinalizeAttempt, error) {
	pending, err := store.PendingFinalizeAttempt()
	if err != nil {
		return reviewtransaction.FinalizeAttempt{}, err
	}
	if pending != nil {
		return *pending, nil
	}
	attempt, _, err := store.BeginFinalizeAttempt(context.Background(), request)
	return attempt, err
}

func facadeFinalizeAttemptRequest(record reviewtransaction.CompactRecord, results []facadeReviewerResult, validation *facadeValidationResult, refuter facadeRefuterResult, evidence []byte, correctionLines int, failed bool) reviewtransaction.FinalizeAttemptRequest {
	return facadeFinalizeAttemptRequestForCandidate(record, record.State.CurrentSnapshot, results, validation, refuter, evidence, correctionLines, failed)
}

func facadeFinalizeAttemptRequestForCandidate(record reviewtransaction.CompactRecord, candidate reviewtransaction.Snapshot, results []facadeReviewerResult, validation *facadeValidationResult, refuter facadeRefuterResult, evidence []byte, correctionLines int, failed bool) reviewtransaction.FinalizeAttemptRequest {
	request := reviewtransaction.FinalizeAttemptRequest{
		LineageID: record.State.LineageID, ExpectedRevision: record.Revision,
		CandidateDigest:          reviewtransaction.FinalizeAttemptValueDigest("candidate", candidate),
		ReviewerResultsDigest:    reviewtransaction.FinalizeAttemptValueDigest("reviewer-results", results),
		CorrectionForecastDigest: reviewtransaction.FinalizeAttemptValueDigest("correction-forecast", correctionLines),
		ValidationDigest:         reviewtransaction.FinalizeAttemptValueDigest("validation", validation),
		RefuterDigest:            reviewtransaction.FinalizeAttemptValueDigest("refuter", refuter),
		EvidenceDigest:           reviewtransaction.FinalizeAttemptValueDigest("evidence", evidence),
		FailedDigest:             reviewtransaction.FinalizeAttemptValueDigest("failed", failed),
	}
	request.RequestDigest = reviewtransaction.FinalizeAttemptRequestDigest(request)
	return request
}

type facadeFinalizeTransition struct {
	Operation string
	State     reviewtransaction.CompactState
}
type facadeFinalizePlan struct {
	Transitions []facadeFinalizeTransition
	Candidate   reviewtransaction.Snapshot
	Evidence    []byte
}

// prepareFacadeFinalizePlan performs every deterministic validation before the
// attempt journal exists. Its states are the only states later admitted and
// written through the write-ahead journal.
func prepareFacadeFinalizePlan(ctx context.Context, repo string, state reviewtransaction.CompactState, results []facadeReviewerResult, refuter facadeRefuterResult, validation *facadeValidationResult, evidence []byte, correctionLines int, failed bool) (facadeFinalizePlan, error) {
	entryState, entryProposed := state.State, state.ProposedCorrectionLines != nil
	plan := facadeFinalizePlan{Transitions: []facadeFinalizeTransition{}, Candidate: state.CurrentSnapshot, Evidence: evidence}
	appendState := func(operation string) {
		plan.Transitions = append(plan.Transitions, facadeFinalizeTransition{Operation: operation, State: state})
	}
	if state.State == reviewtransaction.StateReviewing {
		input, err := prepareCompactReviewerResults(state, results, refuter, facadeRepositoryEvidence{ctx: ctx, repo: repo})
		if err != nil {
			return plan, err
		}
		if err := state.CompleteReview(input); err != nil {
			return plan, err
		}
		appendState("review/complete-review")
	}
	if state.State == reviewtransaction.StateCorrectionRequired && state.ProposedCorrectionLines == nil && correctionLines > 0 {
		if err := state.BeginCorrection(correctionLines); err != nil {
			return plan, err
		}
		appendState("review/begin-fix")
	}
	if state.State == reviewtransaction.StateCorrectionRequired && validation != nil && entryState == reviewtransaction.StateCorrectionRequired && entryProposed {
		if err := rejectFacadeCorrectionUntracked(ctx, repo, state); err != nil {
			return plan, err
		}
		fix, err := (reviewtransaction.SnapshotBuilder{Repo: repo}).Build(ctx, reviewtransaction.Target{Kind: reviewtransaction.TargetFixDiff, Projection: state.InitialSnapshot.Projection, BaseRef: state.CurrentSnapshot.CandidateTree, IntendedUntracked: state.InitialSnapshot.IntendedUntracked, LedgerIDs: state.FixFindingIDs})
		if err != nil {
			return plan, err
		}
		actual, err := (reviewtransaction.SnapshotBuilder{Repo: repo}).ChangedLines(ctx, fix)
		if err != nil {
			return plan, err
		}
		native, err := validation.compact(reviewtransaction.FixDeltaHashForSnapshot(fix), state.FixFindingIDs)
		if err != nil {
			return plan, err
		}
		if err := state.CompleteCorrection(fix, actual, native); err != nil {
			return plan, err
		}
		plan.Candidate = fix
		appendState("review/complete-fix")
	}
	if state.State == reviewtransaction.StateValidating {
		if len(plan.Evidence) == 0 && facadeNativeLowRiskCandidate(state) {
			generated, err := prepareFacadeNativeLowRiskVerification(ctx, repo, state)
			if err != nil {
				return plan, err
			}
			plan.Evidence = generated
		}
		if len(plan.Evidence) > 0 {
			if err := state.CompleteVerification(plan.Evidence, !failed); err != nil {
				return plan, err
			}
			appendState("review/complete-verification")
		}
	}
	if state.State == reviewtransaction.StateValidating && len(plan.Evidence) == 0 {
		return plan, nil
	}
	return plan, nil
}

func facadeNativeLowRiskCandidate(state reviewtransaction.CompactState) bool {
	return (state.State == reviewtransaction.StateReviewing || state.State == reviewtransaction.StateValidating) &&
		state.RiskLevel == reviewtransaction.RiskLow && len(state.SelectedLenses) == 0
}

func prepareFacadeNativeLowRiskVerification(ctx context.Context, repo string, state reviewtransaction.CompactState) ([]byte, error) {
	if err := (reviewtransaction.SnapshotBuilder{Repo: repo}).ValidateEvidence(ctx, state.InitialSnapshot); err != nil {
		return nil, fmt.Errorf("revalidate frozen low-risk snapshot: %w", err)
	}
	target := reviewtransaction.Target{
		Kind: state.InitialSnapshot.Kind, Projection: state.InitialSnapshot.Projection,
		IntendedUntracked: append([]string{}, state.InitialSnapshot.IntendedUntracked...),
	}
	switch target.Kind {
	case reviewtransaction.TargetCurrentChanges:
	case reviewtransaction.TargetBaseDiff, reviewtransaction.TargetBaseWorkspaceOverlay:
		target.BaseRef = state.InitialSnapshot.BaseTree
	default:
		return nil, fmt.Errorf("native low-risk verification does not support target kind %q", target.Kind)
	}
	live, err := (reviewtransaction.SnapshotBuilder{Repo: repo}).Build(ctx, target)
	if err != nil {
		return nil, fmt.Errorf("rebuild frozen low-risk projection: %w", err)
	}
	if live.Identity != state.InitialSnapshot.Identity || live.Identity != state.CurrentSnapshot.Identity {
		return nil, errors.New("live low-risk projection no longer matches the frozen authority")
	}
	assessment, err := (reviewtransaction.SnapshotBuilder{Repo: repo}).AssessSnapshotRisk(ctx, live)
	if err != nil {
		return nil, fmt.Errorf("reclassify frozen low-risk projection: %w", err)
	}
	return reviewtransaction.NativeLowRiskVerificationEvidence(state, assessment)
}

func facadeTerminalState(state reviewtransaction.State) bool {
	return state == reviewtransaction.StateApproved || state == reviewtransaction.StateEscalated
}

func facadeFinalizeReplayInputsEmpty(results, artifacts []string, validation, refuter, evidence string, correctionLines int, failed bool, trace string) bool {
	return len(results) == 0 && len(artifacts) == 0 && strings.TrimSpace(validation) == "" && strings.TrimSpace(refuter) == "" &&
		strings.TrimSpace(evidence) == "" && correctionLines == 0 && !failed && strings.TrimSpace(trace) == ""
}

func inspectCompactFacadeReceipt(path string, expected reviewtransaction.CompactReceipt) (bool, error) {
	payload, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, &reviewtransaction.ImmutablePublicationConflictError{Cause: errors.New("existing receipt cannot be read")}
	}
	existing, err := reviewtransaction.ParseCompactReceipt(payload)
	if err != nil {
		return false, &reviewtransaction.ImmutablePublicationConflictError{Cause: errors.New("existing receipt is invalid")}
	}
	if !reflect.DeepEqual(existing, expected) {
		return false, &reviewtransaction.ImmutablePublicationConflictError{Cause: errors.New("existing receipt differs from terminal authority")}
	}
	return true, nil
}

func newFacadeReceiptPublicationError(lineage, requestDigest string, cause error) error {
	replayability := string(reviewtransaction.ReplayabilityExactReplaySafe)
	var conflict *reviewtransaction.ImmutablePublicationConflictError
	if errors.As(cause, &conflict) {
		replayability = string(reviewtransaction.ReplayabilityManualActionRequired)
	}
	return &ReviewFacadeReceiptPublicationError{
		MutationOutcome: "committed", Replayability: replayability,
		LineageID: lineage, RequestDigest: requestDigest, Cause: cause,
	}
}

func facadeFinalizeReplayRequestDigest(lineage, revision string, receipt reviewtransaction.CompactReceipt) string {
	return facadeValueHash("finalize-replay-request", struct {
		Schema        string                           `json:"schema"`
		Operation     string                           `json:"operation"`
		LineageID     string                           `json:"lineage_id"`
		StoreRevision string                           `json:"store_revision"`
		Receipt       reviewtransaction.CompactReceipt `json:"receipt"`
	}{
		Schema: "gentle-ai.review-finalize-replay-request/v1", Operation: "review/finalize",
		LineageID: lineage, StoreRevision: revision, Receipt: receipt,
	})
}

func RunReviewFacadeValidate(args []string, stdout io.Writer) error {
	return runReviewFacadeValidate(context.Background(), args, stdout)
}

func runReviewFacadeValidate(ctx context.Context, args []string, stdout io.Writer) error {
	flags := newReviewFlagSet("review validate", stdout, "Auto-discover authoritative review state and receipt, then validate them against live Git evidence.")
	cwd := flags.String("cwd", ".", "repository path")
	contract := flags.String("contract", "", "optional negotiated review integration contract")
	lineage := flags.String("lineage", "", "optional lineage override when discovery is ambiguous")
	gate := flags.String("gate", "", "lifecycle gate: post-apply, pre-commit, pre-push, pre-pr, or release")
	baseRef := flags.String("base-ref", "", "optional expected remote publication base for pre-pr")
	ciAttestation := flags.String("pre-pr-ci-attestation", "", "signed exact-merged-tree CI attestation for a compatible base advance")
	policy := flags.String("policy", "", "explicit custom policy containing compatible-base CI trust")
	releaseConfiguration := flags.String("release-configuration", "", "release configuration artifact")
	releaseGenerated := flags.String("release-generated", "", "generated artifact manifest")
	releaseProvenance := flags.String("release-provenance", "", "release provenance artifact")
	releaseBoundary := flags.String("release-publication-boundary", "", "sealed publication boundary artifact")
	releaseFreshness := flags.String("release-evidence-freshness", "", "current release evidence freshness artifact")
	if err := parseReviewFlags(flags, args); err != nil {
		return err
	}
	if reviewHelpRequested(args) {
		return nil
	}
	if flags.NArg() != 0 {
		return reviewPreflightError(fmt.Errorf("unexpected review validate argument %q", flags.Arg(0)))
	}
	negotiated, err := reviewIntegrationNegotiation(flags, *contract)
	if err != nil {
		return err
	}
	if strings.TrimSpace(*gate) == "" {
		return errors.New("review validate requires --gate")
	}
	root, err := (reviewtransaction.SnapshotBuilder{Repo: *cwd}).ResolveRepositoryRoot(ctx)
	if err != nil {
		return fmt.Errorf("resolve review repository root: %w", err)
	}
	gateInput := reviewtransaction.NativeGateRequestInput{
		Gate: reviewtransaction.GateKind(*gate), BaseRef: *baseRef, PrePRCIAttestation: *ciAttestation,
		ReleaseConfiguration: *releaseConfiguration, ReleaseGenerated: *releaseGenerated,
		ReleaseProvenance: *releaseProvenance, ReleasePublicationBoundary: *releaseBoundary,
		ReleaseEvidenceFreshness: *releaseFreshness,
	}
	if strings.TrimSpace(*ciAttestation) != "" {
		gateInput.PolicyArtifact = *policy
	}
	compactStore, compactRecord, compactErr := discoverCompactFacadeGateReview(ctx, root, *lineage, gateInput)
	if compactErr == nil {
		if strings.TrimSpace(*lineage) != "" {
			if _, _, _, legacyErr := discoverFacadeReview(ctx, root, *lineage, true); legacyErr == nil {
				return errors.New("review authority is ambiguous across compact v2 and legacy v1 stores; specify and clean up the intended lineage")
			}
		} else if legacyExactFacadeGateLineages(ctx, root, gateInput) > 0 {
			return errors.New("review authority is ambiguous across compact v2 and legacy v1 stores; specify and clean up the intended lineage")
		}
		payload, err := os.ReadFile(compactStore.ReceiptPath())
		if err != nil {
			return errors.New("facade review receipt is not available")
		}
		receipt, err := reviewtransaction.ParseCompactReceipt(payload)
		if err != nil {
			return fmt.Errorf("parse compact review receipt: %w", err)
		}
		input := gateInput
		input.LineageID = compactRecord.State.LineageID
		input.IntendedUntracked = append([]string(nil), compactRecord.State.InitialSnapshot.IntendedUntracked...)
		evaluation := reviewtransaction.EvaluateCompactGate(ctx, root, receipt, input)
		if gateInput.Gate == reviewtransaction.GatePrePR && strings.TrimSpace(*lineage) == "" &&
			evaluation.Context.Denial != nil && evaluation.Context.Denial.Stage == "receipt-binding" && evaluation.Context.Denial.Code == "base-mismatch" {
			if composed, attempted := reviewtransaction.EvaluateCompactPrePRChain(ctx, root, gateInput); attempted {
				return emitFacadeGateEvaluationNegotiated(stdout, composed, negotiated)
			}
		}
		return emitFacadeGateEvaluationNegotiated(stdout, evaluation, negotiated)
	}
	var compactDiscovery *ReviewReceiptDiscoveryError
	if gateInput.Gate == reviewtransaction.GatePrePR && strings.TrimSpace(*lineage) == "" &&
		errors.As(compactErr, &compactDiscovery) && compactDiscovery.Kind != ReviewAuthorityCorrupted && compactDiscovery.Kind != ReviewReceiptMissing {
		if evaluation, attempted := reviewtransaction.EvaluateCompactPrePRChain(ctx, root, gateInput); attempted {
			return emitFacadeGateEvaluationNegotiated(stdout, evaluation, negotiated)
		}
	}
	if !negotiated {
		var discovery *ReviewReceiptDiscoveryError
		if errors.As(compactErr, &discovery) {
			result := reviewtransaction.GateInvalidated
			reason := discovery.Error()
			context := reviewtransaction.GateContext{
				Gate: gateInput.Gate, Denial: &reviewtransaction.GateDenial{Stage: "receipt-discovery", Code: string(discovery.Kind)},
			}
			if discovery.Kind == ReviewReceiptScopeChanged {
				result = reviewtransaction.GateScopeChanged
				if discovery.Context != nil {
					context = *discovery.Context
				}
			}
			return emitFacadeGateEvaluationNegotiated(stdout, reviewtransaction.NativeGateEvaluation{
				Result: result, Reason: reason, Context: context,
			}, false)
		}
	}

	_, chain, artifacts, legacyErr := discoverFacadeReview(ctx, root, *lineage, true)
	if legacyErr != nil {
		return compactErr
	}
	tx := chain.Records[len(chain.Records)-1].Transaction
	validateArgs := []string{"--cwd", root, "--receipt", artifacts.receipt, "--lineage", tx.LineageID, "--gate", *gate}
	if strings.TrimSpace(*baseRef) != "" {
		validateArgs = append(validateArgs, "--base-ref", *baseRef)
	}
	if strings.TrimSpace(*ciAttestation) != "" {
		validateArgs = append(validateArgs, "--pre-pr-ci-attestation", *ciAttestation)
		if _, err := os.Stat(artifacts.policy); err == nil {
			validateArgs = append(validateArgs, "--policy", artifacts.policy)
		}
	}
	for _, item := range [][2]string{{"--release-configuration", *releaseConfiguration}, {"--release-generated", *releaseGenerated}, {"--release-provenance", *releaseProvenance}, {"--release-publication-boundary", *releaseBoundary}, {"--release-evidence-freshness", *releaseFreshness}} {
		if strings.TrimSpace(item[1]) != "" {
			validateArgs = append(validateArgs, item[0], item[1])
		}
	}
	for _, path := range tx.Snapshot.IntendedUntracked {
		validateArgs = append(validateArgs, "--intended-untracked", path)
	}
	return runFacadeLegacyValidateNegotiated(ctx, validateArgs, stdout, negotiated)
}

func discoverCompactFacadeGateReview(ctx context.Context, repo, lineage string, input reviewtransaction.NativeGateRequestInput) (reviewtransaction.CompactStore, reviewtransaction.CompactRecord, error) {
	if strings.TrimSpace(lineage) != "" {
		return discoverCompactFacadeReview(ctx, repo, lineage, true)
	}
	report, err := reviewtransaction.InventoryAuthority(ctx, repo)
	if (err != nil || !report.Complete || !report.Authoritative) && !reviewAuthorityCorruptionConfinedToLegacyEntries(report, err) {
		return reviewtransaction.CompactStore{}, reviewtransaction.CompactRecord{}, &ReviewReceiptDiscoveryError{Kind: ReviewAuthorityCorrupted, Category: reviewAuthorityCauseCategory(report, err)}
	}
	stores, err := reviewtransaction.CompactAuthorityLeaves(ctx, repo)
	if err != nil {
		return reviewtransaction.CompactStore{}, reviewtransaction.CompactRecord{}, &ReviewReceiptDiscoveryError{Kind: ReviewAuthorityCorrupted, Category: "record_or_graph_invalid"}
	}
	type candidate struct {
		store      reviewtransaction.CompactStore
		record     reviewtransaction.CompactRecord
		assessment reviewtransaction.CompactGateTargetAssessment
		context    reviewtransaction.GateContext
	}
	exact := []candidate{}
	scopeChanged := []candidate{}
	scopeWithoutContext := []string{}
	assessmentUnknown := []string{}
	terminalCount := 0
	allLineages := []string{}
	for _, store := range stores {
		record, loadErr := store.Load()
		if loadErr != nil {
			return reviewtransaction.CompactStore{}, reviewtransaction.CompactRecord{}, &ReviewReceiptDiscoveryError{Kind: ReviewAuthorityCorrupted}
		}
		if !facadeTerminalState(record.State.State) {
			continue
		}
		payload, readErr := os.ReadFile(store.ReceiptPath())
		if readErr != nil {
			return reviewtransaction.CompactStore{}, reviewtransaction.CompactRecord{}, &ReviewReceiptDiscoveryError{Kind: ReviewAuthorityCorrupted}
		}
		receipt, parseErr := reviewtransaction.ParseCompactReceipt(payload)
		derived, deriveErr := record.State.Receipt()
		if parseErr != nil || deriveErr != nil || !reviewtransaction.CompactReceiptEqual(receipt, derived) {
			return reviewtransaction.CompactStore{}, reviewtransaction.CompactRecord{}, &ReviewReceiptDiscoveryError{Kind: ReviewAuthorityCorrupted}
		}
		terminalCount++
		allLineages = append(allLineages, record.State.LineageID)
		candidateInput := input
		candidateInput.LineageID = record.State.LineageID
		candidateInput.IntendedUntracked = append([]string(nil), record.State.CurrentSnapshot.IntendedUntracked...)
		assessment, assessErr := reviewtransaction.AssessCompactGateTarget(ctx, repo, record.State, candidateInput)
		if assessErr != nil {
			assessmentUnknown = append(assessmentUnknown, record.State.LineageID)
			continue
		}
		switch assessment.Applicability {
		case reviewtransaction.CompactGateTargetExact:
			exact = append(exact, candidate{store: store, record: record, assessment: assessment})
		case reviewtransaction.CompactGateTargetScopeChanged:
			diagnostics, diagnosticsErr := reviewtransaction.CompactScopeChangeDiagnostics(ctx, repo, record.State, record.Revision, assessment.Actual)
			if diagnosticsErr != nil {
				scopeWithoutContext = append(scopeWithoutContext, record.State.LineageID)
				continue
			}
			scopeChanged = append(scopeChanged, candidate{
				store: store, record: record, assessment: assessment,
				context: reviewtransaction.GateContext{
					Gate: input.Gate, LineageID: record.State.LineageID, Generation: record.State.Generation,
					StoreRevision: record.Revision, GenesisRevision: record.Revision, ChainIdentity: record.Revision, BundleDigest: record.Revision,
					BaseTree: assessment.Actual.BaseTree, CandidateTree: assessment.Actual.CandidateTree, PathsDigest: assessment.Actual.PathsDigest,
					FixDeltaHash: record.State.FixDeltaHash, PolicyHash: record.State.PolicyHash,
					LedgerHash: reviewtransaction.EmptyFixDeltaHash, EvidenceHash: record.State.EvidenceHash,
					Denial: &reviewtransaction.GateDenial{Stage: "receipt-binding", Code: "candidate-or-paths-mismatch"}, ScopeChange: &diagnostics,
				},
			})
		}
	}
	if len(exact) == 1 {
		return exact[0].store, exact[0].record, nil
	}
	if len(exact) > 1 {
		lineages := make([]string, len(exact))
		for index := range exact {
			lineages[index] = exact[index].record.State.LineageID
		}
		sort.Strings(lineages)
		return reviewtransaction.CompactStore{}, reviewtransaction.CompactRecord{}, &ReviewReceiptDiscoveryError{Kind: ReviewReceiptAmbiguous, Candidates: lineages}
	}
	if terminalCount == 0 {
		return reviewtransaction.CompactStore{}, reviewtransaction.CompactRecord{}, &ReviewReceiptDiscoveryError{Kind: ReviewReceiptMissing}
	}
	scopeCandidateCount := len(scopeChanged) + len(scopeWithoutContext)
	if scopeCandidateCount > 0 && scopeCandidateCount+len(assessmentUnknown) > 1 {
		lineages := make([]string, 0, scopeCandidateCount+len(assessmentUnknown))
		for index := range scopeChanged {
			lineages = append(lineages, scopeChanged[index].record.State.LineageID)
		}
		lineages = append(lineages, scopeWithoutContext...)
		lineages = append(lineages, assessmentUnknown...)
		sort.Strings(lineages)
		return reviewtransaction.CompactStore{}, reviewtransaction.CompactRecord{}, &ReviewReceiptDiscoveryError{Kind: ReviewReceiptAmbiguous, Candidates: lineages}
	}
	if len(scopeChanged) == 1 && len(scopeWithoutContext) == 0 && len(assessmentUnknown) == 0 {
		context := scopeChanged[0].context
		return reviewtransaction.CompactStore{}, reviewtransaction.CompactRecord{}, &ReviewReceiptDiscoveryError{
			Kind: ReviewReceiptScopeChanged, Candidates: []string{scopeChanged[0].record.State.LineageID}, Context: &context,
		}
	}
	if len(scopeWithoutContext) > 0 || len(assessmentUnknown) > 0 {
		return reviewtransaction.CompactStore{}, reviewtransaction.CompactRecord{}, &ReviewReceiptDiscoveryError{Kind: ReviewAuthorityCorrupted}
	}
	sort.Strings(allLineages)
	return reviewtransaction.CompactStore{}, reviewtransaction.CompactRecord{}, &ReviewReceiptDiscoveryError{Kind: ReviewReceiptUnrelated, Candidates: allLineages}
}

// reviewAuthorityCorruptionConfinedToLegacyEntries reports whether every cause
// of a non-authoritative inventory is an invalid legacy-v1 entry, which can
// never resolve as a compact discovery candidate. Inventory IO/layout
// diagnostics, ambiguous locks, reset residue, mixed-store collisions, and any
// compact-v2 problem keep lineage-less discovery fail-closed.
func reviewAuthorityCorruptionConfinedToLegacyEntries(report reviewtransaction.AuthorityStatusReport, inventoryErr error) bool {
	if inventoryErr != nil || len(report.Diagnostics) > 0 {
		return false
	}
	for _, lock := range report.Locks {
		if lock.Status == reviewtransaction.AuthorityLockAmbiguous {
			return false
		}
	}
	confined := false
	for _, entry := range report.Entries {
		switch entry.Status {
		case reviewtransaction.AuthorityStatusInvalid:
			if entry.Version != reviewtransaction.AuthorityVersionLegacy {
				return false
			}
			confined = true
		case reviewtransaction.AuthorityStatusReset, reviewtransaction.AuthorityStatusCollision:
			return false
		}
	}
	return confined
}

func reviewAuthorityCauseCategory(report reviewtransaction.AuthorityStatusReport, inventoryErr error) string {
	if inventoryErr != nil || len(report.Diagnostics) > 0 {
		return "inventory_io_or_layout"
	}
	for _, lock := range report.Locks {
		if lock.Status == reviewtransaction.AuthorityLockAmbiguous {
			return "lock_ambiguous"
		}
	}
	for _, entry := range report.Entries {
		switch entry.Status {
		case reviewtransaction.AuthorityStatusReset:
			return "reset_residue"
		case reviewtransaction.AuthorityStatusInvalid, reviewtransaction.AuthorityStatusCollision:
			return "record_or_graph_invalid"
		}
	}
	for _, entry := range report.Entries {
		if entry.Status == reviewtransaction.AuthorityStatusIncomplete {
			return "incomplete_store_entry"
		}
	}
	return "inventory_incomplete"
}

func legacyExactFacadeGateLineages(ctx context.Context, repo string, input reviewtransaction.NativeGateRequestInput) int {
	stores, err := reviewtransaction.DiscoverAuthoritativeStores(ctx, repo)
	if err != nil {
		return 0
	}
	exact := 0
	for _, store := range stores {
		chain, loadErr := store.LoadChain()
		if loadErr != nil {
			continue
		}
		tx := chain.Records[len(chain.Records)-1].Transaction
		if !facadeTerminalState(tx.State) {
			continue
		}
		candidateInput := input
		candidateInput.LineageID = tx.LineageID
		candidateInput.IntendedUntracked = append([]string(nil), tx.Snapshot.IntendedUntracked...)
		request, requestErr := reviewtransaction.BuildNativeGateRequest(ctx, repo, candidateInput)
		if requestErr != nil {
			continue
		}
		payload, readErr := os.ReadFile(filepath.Join(store.Dir, "artifacts", "receipt.json"))
		if readErr != nil {
			continue
		}
		receipt, parseErr := reviewtransaction.ParseReceipt(payload)
		if parseErr != nil {
			continue
		}
		authoritative, deriveErr := tx.Receipt()
		if deriveErr != nil || !reflect.DeepEqual(receipt, authoritative) {
			continue
		}
		evaluation := reviewtransaction.EvaluateNativeGate(ctx, repo, receipt, request)
		if evaluation.Result == reviewtransaction.GateAllow ||
			evaluation.Context.LineageID == receipt.LineageID && evaluation.Context.CandidateTree == receipt.FinalCandidateTree && evaluation.Result != reviewtransaction.GateScopeChanged {
			exact++
		}
	}
	return exact
}

func facadeSelectedLenses(assessment reviewtransaction.RiskAssessment, focus string) ([]string, error) {
	if assessment.DominantLens != "" {
		if assessment.Level != reviewtransaction.RiskMedium || assessment.DominantLens != reviewtransaction.LensReadability {
			return nil, fmt.Errorf("unsupported dominant review lens %q for risk %q", assessment.DominantLens, assessment.Level)
		}
		if _, ok := facadeFocusLens(focus); !ok {
			return nil, fmt.Errorf("unsupported review focus %q", focus)
		}
		return []string{assessment.DominantLens}, nil
	}
	switch assessment.Level {
	case reviewtransaction.RiskLow:
		return []string{}, nil
	case reviewtransaction.RiskHigh:
		return []string{reviewtransaction.LensRisk, reviewtransaction.LensResilience, reviewtransaction.LensReadability, reviewtransaction.LensReliability}, nil
	case reviewtransaction.RiskMedium:
		lens, ok := facadeFocusLens(focus)
		if !ok {
			return nil, fmt.Errorf("unsupported review focus %q", focus)
		}
		return []string{lens}, nil
	default:
		return nil, fmt.Errorf("unsupported review risk %q", assessment.Level)
	}
}

func facadeFocusLens(focus string) (string, bool) {
	lens, ok := map[string]string{
		"risk": reviewtransaction.LensRisk, "resilience": reviewtransaction.LensResilience,
		"readability": reviewtransaction.LensReadability, "reliability": reviewtransaction.LensReliability,
	}[strings.TrimSpace(focus)]
	return lens, ok
}

func (result facadeReviewerResult) nativeLensResult() (reviewtransaction.LensResult, []facadeFinding) {
	findings := make([]reviewtransaction.Finding, len(result.Findings))
	for index, finding := range result.Findings {
		findings[index] = reviewtransaction.Finding{
			ID: finding.ID, Lens: finding.Lens, Location: finding.Location, Severity: finding.Severity,
			Claim: finding.Claim, ProofRefs: append([]string(nil), finding.ProofRefs...),
		}
	}
	return reviewtransaction.LensResult{Lens: result.Lens, Findings: findings, Evidence: result.Evidence}, result.Findings
}

func (result facadeValidationResult) native(tx reviewtransaction.Transaction) (reviewtransaction.ScopedValidationResult, error) {
	if len(result.OriginalCriteria.Evidence) == 0 || len(result.CorrectionRegression.Evidence) == 0 {
		return reviewtransaction.ScopedValidationResult{}, errors.New("targeted validation requires original_criteria and correction_regression evidence")
	}
	if result.FollowUps == nil {
		result.FollowUps = []reviewtransaction.FollowUp{}
	}
	return reviewtransaction.ScopedValidationResult{
		LedgerIDs: tx.FixFindingIDs, FixCausedFindings: []reviewtransaction.Finding{}, FollowUps: result.FollowUps,
		OriginalCriteria: reviewtransaction.ValidationCheck{
			EvidenceHash: facadeValueHash("original-criteria", result.OriginalCriteria), FixDeltaHash: tx.FixDeltaHash, Passed: result.OriginalCriteria.Passed,
		},
		CorrectionRegression: reviewtransaction.ValidationCheck{
			EvidenceHash: facadeValueHash("correction-regression", result.CorrectionRegression), FixDeltaHash: tx.FixDeltaHash, Passed: result.CorrectionRegression.Passed,
		},
	}, nil
}

func (result facadeValidationResult) compact(fixDeltaHash string, findingIDs []string) (reviewtransaction.ScopedValidationResult, error) {
	if len(result.OriginalCriteria.Evidence) == 0 || len(result.CorrectionRegression.Evidence) == 0 {
		return reviewtransaction.ScopedValidationResult{}, errors.New("targeted validation requires original_criteria and correction_regression evidence")
	}
	if result.FollowUps == nil {
		result.FollowUps = []reviewtransaction.FollowUp{}
	}
	return reviewtransaction.ScopedValidationResult{
		LedgerIDs: append([]string(nil), findingIDs...), FixCausedFindings: []reviewtransaction.Finding{}, FollowUps: result.FollowUps,
		OriginalCriteria: reviewtransaction.ValidationCheck{
			EvidenceHash: facadeValueHash("original-criteria", result.OriginalCriteria), FixDeltaHash: fixDeltaHash, Passed: result.OriginalCriteria.Passed,
		},
		CorrectionRegression: reviewtransaction.ValidationCheck{
			EvidenceHash: facadeValueHash("correction-regression", result.CorrectionRegression), FixDeltaHash: fixDeltaHash, Passed: result.CorrectionRegression.Passed,
		},
	}, nil
}

func (result facadeRefuterResult) native() []reviewtransaction.EvidenceResult {
	outcomes := make([]reviewtransaction.EvidenceResult, len(result.Results))
	for index, item := range result.Results {
		outcomes[index] = reviewtransaction.EvidenceResult{
			FindingID: item.FindingID, Outcome: item.Outcome, Proof: strings.Join(item.ProofRefs, "; "),
		}
	}
	return outcomes
}

type facadeRepositoryEvidence struct {
	ctx  context.Context
	repo string
}

func prepareCompactReviewerResults(state reviewtransaction.CompactState, results []facadeReviewerResult, refuter facadeRefuterResult, repository ...facadeRepositoryEvidence) (reviewtransaction.CompactReviewInput, error) {
	if len(results) != len(state.SelectedLenses) {
		return reviewtransaction.CompactReviewInput{}, fmt.Errorf("review finalize requires all %d original reviewer result(s)", len(state.SelectedLenses))
	}
	lensResults := make([]reviewtransaction.LensResult, len(results))
	classifications := make([]reviewtransaction.FindingEvidence, 0)
	for index, reviewer := range results {
		lensResult, rawFindings := reviewer.nativeLensResult()
		expectedLens := state.SelectedLenses[index]
		if reviewer.Lens != "" {
			providedLens, err := nativeFacadeReviewerLens(reviewer.Lens)
			if err != nil {
				return reviewtransaction.CompactReviewInput{}, fmt.Errorf("reviewer result %d: %w", index+1, err)
			}
			if providedLens != expectedLens {
				return reviewtransaction.CompactReviewInput{}, fmt.Errorf(
					"reviewer result %d lens %q does not match selected lens %q",
					index+1, reviewer.Lens, expectedLens,
				)
			}
		}
		lensResult.Lens = expectedLens
		canonical, err := reviewtransaction.CanonicalCompactLensResult(lensResult)
		if err != nil {
			return reviewtransaction.CompactReviewInput{}, fmt.Errorf("canonicalize reviewer result %d: %w", index+1, err)
		}
		lensResults[index] = canonical
		for findingIndex, finding := range canonical.Findings {
			if !facadeSevere(finding.Severity) {
				continue
			}
			raw := rawFindings[findingIndex]
			switch raw.CausalDisposition {
			case reviewtransaction.CausalIntroduced, reviewtransaction.CausalBehaviorActivated, reviewtransaction.CausalWorsened:
				if len(repository) == 1 {
					changed, err := (reviewtransaction.SnapshotBuilder{Repo: repository[0].repo}).CandidateLocationSupportsCausality(repository[0].ctx, state.InitialSnapshot, finding.Location, raw.CausalDisposition)
					if err != nil {
						return reviewtransaction.CompactReviewInput{}, fmt.Errorf("verify candidate causality for finding %q: %w", finding.ID, err)
					}
					if !changed {
						raw.CausalDisposition = reviewtransaction.CausalUnknown
					}
				}
			}
			classifications = append(classifications, reviewtransaction.FindingEvidence{
				FindingID: finding.ID, Class: raw.EvidenceClass, Causality: raw.CausalDisposition,
				Proof: strings.Join(raw.ProofRefs, "; "),
			})
		}
	}
	return reviewtransaction.CompactReviewInput{
		LensResults: lensResults, Classifications: classifications, RefuterOutcomes: refuter.native(),
	}, nil
}

func nativeFacadeReviewerLens(lens string) (string, error) {
	switch lens {
	case "risk", reviewtransaction.LensRisk:
		return reviewtransaction.LensRisk, nil
	case "resilience", reviewtransaction.LensResilience:
		return reviewtransaction.LensResilience, nil
	case "readability", reviewtransaction.LensReadability:
		return reviewtransaction.LensReadability, nil
	case "reliability", reviewtransaction.LensReliability:
		return reviewtransaction.LensReliability, nil
	default:
		return "", fmt.Errorf("unsupported reviewer lens %q", lens)
	}
}

func discoverCompactFacadeReview(ctx context.Context, repo, lineage string, terminal bool) (reviewtransaction.CompactStore, reviewtransaction.CompactRecord, error) {
	if strings.TrimSpace(lineage) != "" {
		store, err := reviewtransaction.CompactAuthoritativeStore(ctx, repo, lineage)
		if err != nil {
			return reviewtransaction.CompactStore{}, reviewtransaction.CompactRecord{}, err
		}
		record, err := store.Load()
		if err != nil {
			legacy, legacyErr := reviewtransaction.AuthoritativeStore(ctx, repo, lineage)
			if legacyErr == nil {
				if _, loadErr := legacy.LoadChain(); loadErr == nil {
					return reviewtransaction.CompactStore{}, reviewtransaction.CompactRecord{}, reviewtransaction.ErrLegacyReadOnly
				}
			}
			return reviewtransaction.CompactStore{}, reviewtransaction.CompactRecord{}, fmt.Errorf("load compact facade review lineage: %w", err)
		}
		if terminal {
			if _, err := os.Stat(store.ReceiptPath()); err != nil {
				return reviewtransaction.CompactStore{}, reviewtransaction.CompactRecord{}, errors.New("facade review receipt is not available")
			}
		}
		return store, record, nil
	}
	stores, err := reviewtransaction.CompactAuthorityLeaves(ctx, repo)
	if err != nil {
		return reviewtransaction.CompactStore{}, reviewtransaction.CompactRecord{}, err
	}
	type candidate struct {
		store  reviewtransaction.CompactStore
		record reviewtransaction.CompactRecord
	}
	candidates := []candidate{}
	for _, store := range stores {
		record, loadErr := store.Load()
		if loadErr != nil {
			continue
		}
		isTerminal := record.State.State == reviewtransaction.StateApproved || record.State.State == reviewtransaction.StateEscalated
		if terminal {
			if !isTerminal {
				continue
			}
			if _, statErr := os.Stat(store.ReceiptPath()); statErr != nil {
				continue
			}
		}
		candidates = append(candidates, candidate{store: store, record: record})
	}
	if !terminal && len(candidates) > 1 {
		active := candidates[:0]
		for _, candidate := range candidates {
			if candidate.record.State.State != reviewtransaction.StateApproved && candidate.record.State.State != reviewtransaction.StateEscalated {
				active = append(active, candidate)
			}
		}
		if len(active) > 0 {
			candidates = active
		}
	}
	if len(candidates) > 1 {
		matching := candidates[:0]
		for _, candidate := range candidates {
			snapshot, buildErr := (reviewtransaction.SnapshotBuilder{Repo: repo}).Build(ctx, reviewtransaction.Target{
				Kind: reviewtransaction.TargetCurrentChanges, Projection: candidate.record.State.InitialSnapshot.Projection,
				IntendedUntracked: candidate.record.State.InitialSnapshot.IntendedUntracked,
			})
			if buildErr == nil && snapshot.CandidateTree == candidate.record.State.CurrentSnapshot.CandidateTree {
				matching = append(matching, candidate)
			}
		}
		if len(matching) > 0 {
			candidates = matching
		}
	}
	if len(candidates) == 0 {
		return reviewtransaction.CompactStore{}, reviewtransaction.CompactRecord{}, errors.New("no discoverable compact facade review lineage found")
	}
	if len(candidates) != 1 {
		return reviewtransaction.CompactStore{}, reviewtransaction.CompactRecord{}, errors.New("multiple compact facade review lineages found; specify --lineage")
	}
	return candidates[0].store, candidates[0].record, nil
}

func discoverFacadeReview(ctx context.Context, repo, lineage string, terminal bool) (reviewtransaction.Store, reviewtransaction.ValidatedChain, facadeArtifacts, error) {
	if strings.TrimSpace(lineage) != "" {
		store, err := reviewtransaction.AuthoritativeStore(ctx, repo, lineage)
		if err != nil {
			return reviewtransaction.Store{}, reviewtransaction.ValidatedChain{}, facadeArtifacts{}, err
		}
		chain, err := store.LoadChain()
		if err != nil {
			return reviewtransaction.Store{}, reviewtransaction.ValidatedChain{}, facadeArtifacts{}, fmt.Errorf("load facade review lineage: %w", err)
		}
		artifacts := facadeArtifactPaths(store)
		if terminal {
			if _, err := os.Stat(artifacts.receipt); err != nil {
				return reviewtransaction.Store{}, reviewtransaction.ValidatedChain{}, facadeArtifacts{}, errors.New("facade review receipt is not available")
			}
		}
		return store, chain, artifacts, nil
	}
	stores, err := reviewtransaction.DiscoverAuthoritativeStores(ctx, repo)
	if err != nil {
		return reviewtransaction.Store{}, reviewtransaction.ValidatedChain{}, facadeArtifacts{}, fmt.Errorf("discover authoritative review stores: %w", err)
	}
	type candidate struct {
		store     reviewtransaction.Store
		chain     reviewtransaction.ValidatedChain
		artifacts facadeArtifacts
	}
	candidates := []candidate{}
	for _, store := range stores {
		artifacts := facadeArtifactPaths(store)
		if terminal {
			if _, err := os.Stat(artifacts.receipt); err != nil {
				continue
			}
		}
		chain, err := store.LoadChain()
		if err != nil {
			continue
		}
		tx := chain.Records[len(chain.Records)-1].Transaction
		isTerminal := tx.State == reviewtransaction.StateApproved || tx.State == reviewtransaction.StateEscalated
		if terminal && !isTerminal {
			continue
		}
		candidates = append(candidates, candidate{store: store, chain: chain, artifacts: artifacts})
	}
	if len(candidates) == 0 {
		return reviewtransaction.Store{}, reviewtransaction.ValidatedChain{}, facadeArtifacts{}, errors.New("no discoverable facade review lineage found")
	}
	if !terminal && len(candidates) > 1 {
		nonterminal := candidates[:0]
		for _, candidate := range candidates {
			tx := candidate.chain.Records[len(candidate.chain.Records)-1].Transaction
			if tx.State != reviewtransaction.StateApproved && tx.State != reviewtransaction.StateEscalated {
				nonterminal = append(nonterminal, candidate)
			}
		}
		if len(nonterminal) > 0 {
			candidates = nonterminal
		}
	}
	if len(candidates) > 1 {
		matching := candidates[:0]
		for _, candidate := range candidates {
			tx := candidate.chain.Records[len(candidate.chain.Records)-1].Transaction
			snapshot, err := (reviewtransaction.SnapshotBuilder{Repo: repo}).Build(ctx, reviewtransaction.Target{
				Kind: reviewtransaction.TargetCurrentChanges, Projection: tx.Snapshot.Projection,
				IntendedUntracked: tx.Snapshot.IntendedUntracked,
			})
			if err == nil && snapshot.CandidateTree == tx.FinalCandidateTree {
				matching = append(matching, candidate)
			}
		}
		if len(matching) > 0 {
			candidates = matching
		}
	}
	if len(candidates) != 1 {
		return reviewtransaction.Store{}, reviewtransaction.ValidatedChain{}, facadeArtifacts{}, errors.New("multiple facade review lineages found; specify --lineage")
	}
	selected := candidates[0]
	return selected.store, selected.chain, selected.artifacts, nil
}

func facadeArtifactPaths(store reviewtransaction.Store) facadeArtifacts {
	dir := filepath.Join(store.Dir, "artifacts")
	return facadeArtifacts{
		policy: filepath.Join(dir, "policy.md"), ledger: filepath.Join(dir, "ledger.json"),
		evidence: filepath.Join(dir, "evidence"), fixDelta: filepath.Join(dir, "fix-delta.json"),
		receipt: filepath.Join(dir, "receipt.json"),
	}
}

func encodeCompactFacadeFinalize(stdout io.Writer, negotiated bool, state reviewtransaction.CompactState, revision string, store reviewtransaction.CompactStore, action string) error {
	result := ReviewFacadeFinalizeResult{
		Operation: "review/finalize", LineageID: state.LineageID, State: state.State, Action: action, StoreRevision: revision,
	}
	if state.State == reviewtransaction.StateApproved || state.State == reviewtransaction.StateEscalated {
		result.ReceiptPath = store.ReceiptPath()
	}
	public := ReviewIntegrationFinalizeResult{
		Operation: result.Operation, LineageID: result.LineageID, State: result.State,
		Action: result.Action, StoreRevision: result.StoreRevision,
	}
	return encodeReviewIntegrationOperation(stdout, negotiated, ReviewIntegrationOperationFinalize, result, public)
}

func rejectFacadeCorrectionUntracked(ctx context.Context, repo string, state reviewtransaction.CompactState) error {
	if state.InitialSnapshot.Projection == reviewtransaction.ProjectionStaged {
		return nil
	}
	live, err := (reviewtransaction.SnapshotBuilder{Repo: repo}).DiscoverIntendedUntracked(ctx)
	if err != nil {
		return fmt.Errorf("discover correction untracked paths: %w", err)
	}
	allowed := make(map[string]struct{}, len(state.CurrentSnapshot.IntendedUntracked))
	for _, path := range state.CurrentSnapshot.IntendedUntracked {
		allowed[path] = struct{}{}
	}
	unexpected := make([]string, 0)
	for _, path := range live {
		if _, ok := allowed[path]; !ok {
			unexpected = append(unexpected, path)
		}
	}
	if len(unexpected) != 0 {
		return fmt.Errorf("correction contains untracked paths outside the frozen review scope: %s", strings.Join(unexpected, ", "))
	}
	return nil
}

func emitFacadeGateEvaluation(stdout io.Writer, evaluation reviewtransaction.NativeGateEvaluation) error {
	return emitFacadeGateEvaluationNegotiated(stdout, evaluation, false)
}

func emitFacadeGateEvaluationNegotiated(stdout io.Writer, evaluation reviewtransaction.NativeGateEvaluation, negotiated bool) error {
	result := ReviewValidateResult{
		Schema: ReviewValidateSchema, Result: evaluation.Result, Allowed: evaluation.Result == reviewtransaction.GateAllow,
		Action: reviewGateAction(evaluation.Result), Reason: evaluation.Reason, Context: evaluation.Context,
	}
	if err := encodeReviewIntegrationOperation(stdout, negotiated, ReviewIntegrationOperationValidate, result, result); err != nil {
		return err
	}
	if !result.Allowed {
		return ReviewGateDeniedError{Result: result.Result, Context: result.Context, Cause: evaluation.Cause}
	}
	return nil
}

func runFacadeLegacyValidateNegotiated(ctx context.Context, args []string, stdout io.Writer, negotiated bool) error {
	if !negotiated {
		return runReviewValidate(ctx, args, stdout)
	}
	var output bytes.Buffer
	runErr := runReviewValidate(ctx, args, &output)
	if output.Len() == 0 {
		return runErr
	}
	var result ReviewValidateResult
	if err := decodeStrictReviewIntegrationResult(output.Bytes(), &result); err != nil {
		return err
	}
	if err := encodeReviewIntegrationOperation(stdout, true, ReviewIntegrationOperationValidate, result, result); err != nil {
		return err
	}
	return runErr
}

func facadePolicyBytes(path string) ([]byte, error) {
	if strings.TrimSpace(path) == "" {
		return []byte(facadeReviewPolicy), nil
	}
	payload, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read facade review policy: %w", err)
	}
	return payload, nil
}

func readFacadeReviewerResults(paths []string) ([]facadeReviewerResult, error) {
	results := make([]facadeReviewerResult, len(paths))
	for index, path := range paths {
		if err := readFacadeJSON(path, &results[index]); err != nil {
			return nil, fmt.Errorf("read reviewer result %d: %w", index+1, err)
		}
		if results[index].Findings == nil || results[index].Evidence == nil {
			return nil, fmt.Errorf("reviewer result %d requires explicit findings and evidence arrays", index+1)
		}
	}
	return results, nil
}

func readFacadeJSON(path string, value any) error {
	payload, err := readFacadeBytes(path)
	if err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return errors.New("input contains multiple JSON values")
	}
	return nil
}

func readFacadeBytes(path string) ([]byte, error) {
	if path == "-" {
		return io.ReadAll(os.Stdin)
	}
	return os.ReadFile(path)
}

func countFacadeStdin(resultPaths []string, paths ...string) int {
	count := 0
	for _, path := range append(append([]string{}, resultPaths...), paths...) {
		if path == "-" {
			count++
		}
	}
	return count
}

func facadeValueHash(domain string, value any) string {
	payload, _ := json.Marshal(value)
	sum := sha256.Sum256(append([]byte("gentle-ai.facade-"+domain+"/v1\x00"), payload...))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func facadePayloadHash(payload []byte) string {
	sum := sha256.Sum256(payload)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func facadeSevere(severity string) bool {
	switch strings.ToUpper(strings.TrimSpace(severity)) {
	case "BLOCKER", "CRITICAL":
		return true
	default:
		return false
	}
}
