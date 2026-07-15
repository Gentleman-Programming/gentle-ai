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
	"strings"

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
	Projection       reviewtransaction.Projection `json:"projection"`
	ChangedFiles     int                          `json:"changed_files"`
	ChangedLines     int                          `json:"changed_lines"`
	CorrectionBudget int                          `json:"correction_budget"`
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

var writeCompactFacadeReceipt = reviewtransaction.WriteCompactReceiptAtomic

type ReviewInvalidateResult struct {
	Operation     string                  `json:"operation"`
	LineageID     string                  `json:"lineage_id"`
	State         reviewtransaction.State `json:"state"`
	StoreRevision string                  `json:"store_revision"`
}

type ReviewRecoverResult struct {
	Operation     string                                      `json:"operation"`
	LineageID     string                                      `json:"lineage_id"`
	State         reviewtransaction.State                     `json:"state"`
	StoreRevision string                                      `json:"store_revision"`
	Recovery      reviewtransaction.CompactRecoveryProvenance `json:"recovery"`
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

func RunReview(args []string, stdout io.Writer) error {
	if len(args) == 0 || args[0] == "help" || args[0] == "-h" || args[0] == "--help" {
		_, _ = fmt.Fprintln(stdout, "Usage: gentle-ai review <capabilities|start|finalize|validate|status|invalidate|recover|schema|bind-sdd> [flags]\n\nOrdinary review facade; repository scope, authority, canonical artifacts, and lifecycle transitions are derived by Go.")
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
	var output bytes.Buffer
	runErr := runReviewCommand(args, &output)
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

func runReviewCommand(args []string, stdout io.Writer) error {
	switch args[0] {
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
	case "schema":
		return RunReviewSchema(args[1:], stdout)
	case "bind-sdd":
		return RunReviewBindSDD(args[1:], stdout)
	default:
		return fmt.Errorf("unknown review command %q", args[0])
	}
}

func RunReviewStatus(args []string, stdout io.Writer) error {
	flags := newReviewFlagSet("review status", stdout, "Read every compact-v2 and shipped legacy-v1 authority from the shared Git common directory without mutation.")
	cwd := flags.String("cwd", ".", "repository path")
	contract := flags.String("contract", "", "optional negotiated review integration contract")
	lineage := flags.String("lineage", "", "optional explicit lineage selector for negotiated target status")
	projection := flags.String("projection", string(reviewtransaction.ProjectionWorkspace), "negotiated target projection: workspace or staged")
	baseRef := flags.String("base-ref", "", "optional negotiated immutable base-to-HEAD target")
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
		builder := reviewtransaction.SnapshotBuilder{Repo: *cwd}
		root, err := builder.ResolveRepositoryRoot(context.Background())
		if err != nil {
			return fmt.Errorf("resolve negotiated review repository root: %w", err)
		}
		intended := []string{}
		if selectedProjection != reviewtransaction.ProjectionStaged {
			intended, err = (reviewtransaction.SnapshotBuilder{Repo: root}).DiscoverIntendedUntracked(context.Background())
			if err != nil {
				return fmt.Errorf("discover negotiated review target: %w", err)
			}
		}
		target := reviewtransaction.Target{Kind: reviewtransaction.TargetCurrentChanges, Projection: selectedProjection, IntendedUntracked: intended}
		if strings.TrimSpace(*baseRef) != "" {
			target.Kind, target.BaseRef = reviewtransaction.TargetBaseDiff, strings.TrimSpace(*baseRef)
		}
		native, err := reviewtransaction.AssessTargetStatus(context.Background(), root, reviewtransaction.TargetStatusRequest{
			Target: target, LineageID: *lineage,
		})
		if err != nil {
			return fmt.Errorf("assess negotiated review target: %w", err)
		}
		result := newReviewTargetStatusResult(native)
		if err := result.Validate(); err != nil {
			return fmt.Errorf("validate negotiated review status: %w", err)
		}
		return encodeReviewJSON(stdout, result)
	}
	if strings.TrimSpace(*lineage) != "" || strings.TrimSpace(*baseRef) != "" || *projection != string(reviewtransaction.ProjectionWorkspace) {
		return errors.New("review status target selectors require --contract")
	}
	report, err := reviewtransaction.InventoryAuthority(context.Background(), *cwd)
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
	authorization := flags.String("maintainer-authorization", "", "explicit authorization required for escalated recovery")
	policySource := flags.String("policy", "", "optional review policy file")
	focus := flags.String("focus", "reliability", "dominant standard-risk focus")
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
	projection := predecessorRecord.State.InitialSnapshot.Projection
	intended := []string{}
	if projection != reviewtransaction.ProjectionStaged {
		intended, err = builder.DiscoverIntendedUntracked(context.Background())
		if err != nil {
			return err
		}
	}
	snapshot, err := (reviewtransaction.SnapshotBuilder{Repo: root}).Build(context.Background(), reviewtransaction.Target{Kind: reviewtransaction.TargetCurrentChanges, Projection: projection, IntendedUntracked: intended})
	if err != nil {
		return err
	}
	risk, changedLines, err := (reviewtransaction.SnapshotBuilder{Repo: root}).ClassifySnapshotRisk(context.Background(), snapshot)
	if err != nil {
		return err
	}
	lenses, err := facadeSelectedLenses(risk, *focus)
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
	record, err := reviewtransaction.RecoverCompactAuthority(context.Background(), root, reviewtransaction.CompactRecoveryRequest{
		PredecessorLineageID: *predecessor, ExpectedPredecessorRevision: *expected, Successor: state,
		Disposition: reviewtransaction.RecoveryDisposition(*disposition), Reason: *reason, Actor: *actor, MaintainerAuthorization: *authorization,
	})
	if err != nil {
		return err
	}
	return encodeReviewJSON(stdout, ReviewRecoverResult{Operation: "review/recover", LineageID: record.State.LineageID, State: record.State.State, StoreRevision: record.Revision, Recovery: *record.State.Recovery})
}

func RunReviewBindSDD(args []string, stdout io.Writer) error {
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
	binding, err := sddstatus.BindApprovedReview(context.Background(), *cwd, *change, *lineage, *expected)
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
	flags := newReviewFlagSet("review start", stdout, "Freeze live Git scope and derive the bounded review tier, lenses, and correction budget.")
	cwd := flags.String("cwd", ".", "repository path")
	contract := flags.String("contract", "", "optional negotiated review integration contract")
	lineage := flags.String("lineage", "", "optional explicit review lineage identifier")
	policySource := flags.String("policy", "", "optional review policy file; the native bounded policy is used by default")
	focus := flags.String("focus", "reliability", "dominant standard-risk focus: risk, resilience, readability, or reliability")
	baseRef := flags.String("base-ref", "", "optional base revision for immutable base-to-HEAD review")
	projection := flags.String("projection", string(reviewtransaction.ProjectionWorkspace), "candidate projection: workspace or staged; staged base-diff records post-commit delivery provenance")
	committedOnly := flags.Bool("committed-only", false, "acknowledge that --base-ref excludes dirty tracked changes")
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
	root, err := builder.ResolveRepositoryRoot(context.Background())
	if err != nil {
		return fmt.Errorf("resolve review repository root: %w", err)
	}
	selectedProjection := reviewtransaction.Projection(strings.TrimSpace(*projection))
	if selectedProjection != reviewtransaction.ProjectionWorkspace && selectedProjection != reviewtransaction.ProjectionStaged {
		return fmt.Errorf("unsupported review projection %q", *projection)
	}
	if strings.TrimSpace(*baseRef) != "" {
		dirtyTracked, dirtyErr := (reviewtransaction.SnapshotBuilder{Repo: root}).HasDirtyTrackedChanges(context.Background())
		if dirtyErr != nil {
			return fmt.Errorf("detect dirty tracked changes for committed review: %w", dirtyErr)
		}
		if dirtyTracked && !*committedOnly {
			return errors.New("review start with --base-ref omits dirty tracked changes; rerun with --committed-only to acknowledge committed-only review scope")
		}
	}
	intended := []string{}
	if selectedProjection != reviewtransaction.ProjectionStaged {
		intended, err = builder.DiscoverIntendedUntracked(context.Background())
		if err != nil {
			return fmt.Errorf("discover intended untracked files: %w", err)
		}
	}
	target := reviewtransaction.Target{Kind: reviewtransaction.TargetCurrentChanges, Projection: selectedProjection, IntendedUntracked: intended}
	if strings.TrimSpace(*baseRef) != "" {
		target.Kind = reviewtransaction.TargetBaseDiff
		target.BaseRef = strings.TrimSpace(*baseRef)
	}
	snapshot, err := (reviewtransaction.SnapshotBuilder{Repo: root}).Build(context.Background(), target)
	if err != nil {
		return fmt.Errorf("build facade review target: %w", err)
	}
	assessment, err := (reviewtransaction.SnapshotBuilder{Repo: root}).AssessSnapshotRisk(context.Background(), snapshot)
	if err != nil {
		return fmt.Errorf("classify facade review target: %w", err)
	}
	risk, changedLines := assessment.Level, assessment.ChangedLines
	lenses, err := facadeSelectedLenses(risk, *focus)
	if err != nil {
		return err
	}
	if strings.TrimSpace(*lineage) == "" {
		*lineage = "review-" + strings.TrimPrefix(snapshot.Identity, "sha256:")[:16]
	}
	legacy, err := reviewtransaction.AuthoritativeStore(context.Background(), root, *lineage)
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
	started, err := reviewtransaction.StartCompactAuthority(context.Background(), root, reviewtransaction.CompactStartRequest{
		State: state, TracePath: strings.TrimSpace(*tracePath),
	})
	if err != nil {
		return fmt.Errorf("start compact facade review: %w", err)
	}
	authority := started.Record.State
	legacyResult := ReviewFacadeStartResult{
		Operation: "review/start", Action: string(started.Action), LensesRequired: started.LensesRequired,
		LineageID: authority.LineageID, State: authority.State, RiskLevel: authority.RiskLevel,
		SelectedLenses: authority.SelectedLenses, Projection: facadeProjection(authority.InitialSnapshot.Projection),
		ChangedFiles: len(authority.InitialSnapshot.Paths),
		ChangedLines: authority.OriginalChangedLines, CorrectionBudget: authority.CorrectionBudget,
	}
	if !negotiated {
		return encodeReviewJSON(stdout, legacyResult)
	}
	if authority.InitialSnapshot.Identity != snapshot.Identity {
		assessment, err = (reviewtransaction.SnapshotBuilder{Repo: root}).AssessSnapshotRisk(context.Background(), authority.InitialSnapshot)
		if err != nil {
			return fmt.Errorf("classify authoritative negotiated START target: %w", err)
		}
	}
	negotiatedResult, err := newReviewIntegrationStartResult(legacyResult, assessment)
	if err != nil {
		return err
	}
	return encodeReviewJSON(stdout, negotiatedResult)
}

func RunReviewFacadeFinalize(args []string, stdout io.Writer) error {
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
		return errors.New("review finalize accepts stdin for only one input")
	}
	root, err := (reviewtransaction.SnapshotBuilder{Repo: *cwd}).ResolveRepositoryRoot(context.Background())
	if err != nil {
		return fmt.Errorf("resolve review repository root: %w", err)
	}
	store, record, err := discoverCompactFacadeReview(context.Background(), root, *lineage, false)
	if err != nil {
		if _, chain, _, legacyErr := discoverFacadeReview(context.Background(), root, *lineage, false); legacyErr == nil {
			legacyLineage := chain.Records[len(chain.Records)-1].Transaction.LineageID
			return reviewtransaction.NewLegacyReadOnlyError("review/finalize", legacyLineage)
		}
		return err
	}
	store.TracePath = strings.TrimSpace(*tracePath)
	state := record.State
	terminalAtEntry := facadeTerminalState(state.State)
	var terminalReceipt reviewtransaction.CompactReceipt
	terminalReceiptExists := false
	if terminalAtEntry {
		terminalReceipt, err = state.Receipt()
		if err != nil {
			return err
		}
		terminalReceiptExists, err = inspectCompactFacadeReceipt(store.ReceiptPath(), terminalReceipt)
		if err != nil {
			return err
		}
		if !terminalReceiptExists {
			if !facadeFinalizeReplayInputsEmpty(resultPaths, *validationPath, *refuterPath, *evidencePath, *correctionLines, *failed, *tracePath) {
				return errors.New("terminal review finalize accepts no review inputs; exact receipt replay requires only --lineage")
			}
			if *lineage != state.LineageID || strings.TrimSpace(*lineage) != *lineage {
				return errors.New("receipt publication replay requires the exact explicit --lineage")
			}
		}
	}
	reviewerResults, err := readFacadeReviewerResults(resultPaths)
	if err != nil {
		return err
	}
	var validation *facadeValidationResult
	if strings.TrimSpace(*validationPath) != "" {
		validation = &facadeValidationResult{}
		if err := readFacadeJSON(*validationPath, validation); err != nil {
			return fmt.Errorf("read targeted validation: %w", err)
		}
	}
	var refuter facadeRefuterResult
	if strings.TrimSpace(*refuterPath) != "" {
		if err := readFacadeJSON(*refuterPath, &refuter); err != nil {
			return fmt.Errorf("read refuter outcomes: %w", err)
		}
	}
	var evidence []byte
	if strings.TrimSpace(*evidencePath) != "" {
		evidence, err = readFacadeBytes(*evidencePath)
		if err != nil {
			return fmt.Errorf("read final review evidence: %w", err)
		}
	}

	if state.State == reviewtransaction.StateReviewing {
		input, err := prepareCompactReviewerResults(state, reviewerResults, refuter)
		if err != nil {
			return err
		}
		if err := state.CompleteReview(input); err != nil {
			return fmt.Errorf("complete compact review: %w", err)
		}
		revision, err := store.Replace(record.Revision, "review/complete-review", state)
		if err != nil {
			return err
		}
		record.Revision, record.State = revision, state
	}
	if state.State == reviewtransaction.StateCorrectionRequired && state.ProposedCorrectionLines == nil && *correctionLines > 0 {
		if err := state.BeginCorrection(*correctionLines); err != nil {
			return fmt.Errorf("begin bounded compact correction: %w", err)
		}
		revision, err := store.Replace(record.Revision, "review/begin-fix", state)
		if err != nil {
			return err
		}
		record.Revision, record.State = revision, state
	}
	if state.State == reviewtransaction.StateCorrectionRequired && state.ProposedCorrectionLines == nil {
		return encodeCompactFacadeFinalize(stdout, negotiated, state, record.Revision, store, "rerun with --correction-lines before editing")
	}
	if state.State == reviewtransaction.StateCorrectionRequired {
		if validation == nil {
			return encodeCompactFacadeFinalize(stdout, negotiated, state, record.Revision, store, "apply the bounded correction, then rerun with --validation and --evidence")
		}
		if err := rejectFacadeCorrectionUntracked(context.Background(), root, state); err != nil {
			return err
		}
		fixSnapshot, err := (reviewtransaction.SnapshotBuilder{Repo: root}).Build(context.Background(), reviewtransaction.Target{
			Kind: reviewtransaction.TargetFixDiff, Projection: state.InitialSnapshot.Projection,
			BaseRef: state.CurrentSnapshot.CandidateTree, IntendedUntracked: state.InitialSnapshot.IntendedUntracked,
			LedgerIDs: state.FixFindingIDs,
		})
		if err != nil {
			return fmt.Errorf("derive facade correction snapshot: %w", err)
		}
		actual, err := (reviewtransaction.SnapshotBuilder{Repo: root}).ChangedLines(context.Background(), fixSnapshot)
		if err != nil {
			return fmt.Errorf("derive facade correction size: %w", err)
		}
		nativeValidation, err := validation.compact(reviewtransaction.FixDeltaHashForSnapshot(fixSnapshot), state.FixFindingIDs)
		if err != nil {
			return err
		}
		if err := state.CompleteCorrection(fixSnapshot, actual, nativeValidation); err != nil {
			return fmt.Errorf("complete compact correction: %w", err)
		}
		revision, err := store.Replace(record.Revision, "review/complete-fix", state)
		if err != nil {
			return err
		}
		record.Revision, record.State = revision, state
	}
	if state.State == reviewtransaction.StateValidating {
		if len(evidence) == 0 {
			return encodeCompactFacadeFinalize(stdout, negotiated, state, record.Revision, store, "rerun with --evidence")
		}
		if err := state.CompleteVerification(evidence, !*failed); err != nil {
			return fmt.Errorf("complete compact final verification: %w", err)
		}
		revision, err := store.Replace(record.Revision, "review/complete-verification", state)
		if err != nil {
			return err
		}
		record.Revision, record.State = revision, state
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
	digest := facadeFinalizeReplayRequestDigest(state.LineageID, record.Revision, receipt)
	if err := writeCompactFacadeReceipt(store.ReceiptPath(), receipt); err != nil {
		return newFacadeReceiptPublicationError(state.LineageID, digest, err)
	}
	published, err := inspectCompactFacadeReceipt(store.ReceiptPath(), receipt)
	if err != nil {
		return newFacadeReceiptPublicationError(state.LineageID, digest, err)
	}
	if !published {
		return newFacadeReceiptPublicationError(state.LineageID, digest, errors.New("receipt writer did not materialize the derived receipt"))
	}
	return encodeCompactFacadeFinalize(stdout, negotiated, state, record.Revision, store, "validate delivery with gentle-ai review validate --gate <gate>")
}

func facadeTerminalState(state reviewtransaction.State) bool {
	return state == reviewtransaction.StateApproved || state == reviewtransaction.StateEscalated
}

func facadeFinalizeReplayInputsEmpty(results []string, validation, refuter, evidence string, correctionLines int, failed bool, trace string) bool {
	return len(results) == 0 && strings.TrimSpace(validation) == "" && strings.TrimSpace(refuter) == "" &&
		strings.TrimSpace(evidence) == "" && correctionLines == 0 && !failed && strings.TrimSpace(trace) == ""
}

func inspectCompactFacadeReceipt(path string, expected reviewtransaction.CompactReceipt) (bool, error) {
	payload, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("inspect compact review receipt: %w", err)
	}
	existing, err := reviewtransaction.ParseCompactReceipt(payload)
	if err != nil {
		return false, fmt.Errorf("existing compact review receipt is unsafe for replay: %w", err)
	}
	if !reflect.DeepEqual(existing, expected) {
		return false, errors.New("existing compact review receipt is unsafe for replay: receipt does not equal terminal authority")
	}
	return true, nil
}

func newFacadeReceiptPublicationError(lineage, requestDigest string, cause error) error {
	return &ReviewFacadeReceiptPublicationError{
		MutationOutcome: "committed", Replayability: "exact_replay_safe",
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
	root, err := (reviewtransaction.SnapshotBuilder{Repo: *cwd}).ResolveRepositoryRoot(context.Background())
	if err != nil {
		return fmt.Errorf("resolve review repository root: %w", err)
	}
	compactStore, compactRecord, compactErr := discoverCompactFacadeReview(context.Background(), root, *lineage, true)
	if compactErr == nil {
		if _, _, _, legacyErr := discoverFacadeReview(context.Background(), root, *lineage, true); legacyErr == nil {
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
		input := reviewtransaction.NativeGateRequestInput{
			Gate: reviewtransaction.GateKind(*gate), LineageID: compactRecord.State.LineageID,
			IntendedUntracked: append([]string(nil), compactRecord.State.InitialSnapshot.IntendedUntracked...),
			BaseRef:           *baseRef, PrePRCIAttestation: *ciAttestation,
			ReleaseConfiguration: *releaseConfiguration, ReleaseGenerated: *releaseGenerated,
			ReleaseProvenance: *releaseProvenance, ReleasePublicationBoundary: *releaseBoundary,
			ReleaseEvidenceFreshness: *releaseFreshness,
		}
		if strings.TrimSpace(*ciAttestation) != "" {
			input.PolicyArtifact = *policy
		}
		evaluation := reviewtransaction.EvaluateCompactGate(context.Background(), root, receipt, input)
		return emitFacadeGateEvaluationNegotiated(stdout, evaluation, negotiated)
	}

	_, chain, artifacts, legacyErr := discoverFacadeReview(context.Background(), root, *lineage, true)
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
	return runFacadeLegacyValidateNegotiated(validateArgs, stdout, negotiated)
}

func facadeSelectedLenses(risk reviewtransaction.RiskLevel, focus string) ([]string, error) {
	switch risk {
	case reviewtransaction.RiskLow:
		return []string{}, nil
	case reviewtransaction.RiskHigh:
		return []string{reviewtransaction.LensRisk, reviewtransaction.LensResilience, reviewtransaction.LensReadability, reviewtransaction.LensReliability}, nil
	case reviewtransaction.RiskMedium:
		lens, ok := map[string]string{
			"risk": reviewtransaction.LensRisk, "resilience": reviewtransaction.LensResilience,
			"readability": reviewtransaction.LensReadability, "reliability": reviewtransaction.LensReliability,
		}[strings.TrimSpace(focus)]
		if !ok {
			return nil, fmt.Errorf("unsupported review focus %q", focus)
		}
		return []string{lens}, nil
	default:
		return nil, fmt.Errorf("unsupported review risk %q", risk)
	}
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

func prepareCompactReviewerResults(state reviewtransaction.CompactState, results []facadeReviewerResult, refuter facadeRefuterResult) (reviewtransaction.CompactReviewInput, error) {
	if len(results) != len(state.SelectedLenses) {
		return reviewtransaction.CompactReviewInput{}, fmt.Errorf("review finalize requires all %d original reviewer result(s)", len(state.SelectedLenses))
	}
	lensResults := make([]reviewtransaction.LensResult, len(results))
	classifications := make([]reviewtransaction.FindingEvidence, 0)
	for index, reviewer := range results {
		lensResult, rawFindings := reviewer.nativeLensResult()
		lensResult.Lens = state.SelectedLenses[index]
		canonical, err := reviewtransaction.CanonicalLensResult(lensResult)
		if err != nil {
			return reviewtransaction.CompactReviewInput{}, fmt.Errorf("canonicalize reviewer result %d: %w", index+1, err)
		}
		lensResults[index] = canonical
		for findingIndex, finding := range canonical.Findings {
			if !facadeSevere(finding.Severity) {
				continue
			}
			raw := rawFindings[findingIndex]
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
		return ReviewGateDeniedError{Result: result.Result}
	}
	return nil
}

func runFacadeLegacyValidateNegotiated(args []string, stdout io.Writer, negotiated bool) error {
	if !negotiated {
		return RunReviewValidate(args, stdout)
	}
	var output bytes.Buffer
	runErr := RunReviewValidate(args, &output)
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
