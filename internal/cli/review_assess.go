package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v3/internal/model"
	"github.com/gentleman-programming/gentle-ai/v3/internal/reviewtransaction"
)

// ReviewAssessmentSchema is the typed envelope gentle-ai review assess prints
// with --json. It is a read-only projection of the same candidate risk
// assessment START uses to choose lenses (reviewtransaction.AssessSnapshotRisk),
// so a host can gate delegated verification on it before deciding whether to
// start a review at all, with or without receipt-driven development enabled
// (issue #4295).
const ReviewAssessmentSchema = "gentle-ai.review-assessment/v1"
const ReviewAssessmentSchemaID = "https://gentle-ai.dev/contracts/review-integration/v2/schemas/assess.schema.json"

// ReviewAssessmentReason is the public projection of one
// reviewtransaction.RiskReason: the same evidence code and path START already
// carries in its own risk_reasons, plus an optional human-readable detail for
// the signal or mode-change evidence a bare code does not spell out.
type ReviewAssessmentReason struct {
	Code   string `json:"code"`
	Path   string `json:"path,omitempty"`
	Detail string `json:"detail,omitempty"`
}

// ReviewAssessmentCandidate names the exact candidate the assessment was
// computed over, so a caller can tell a current-changes candidate from a named
// base comparison without re-deriving it. Consumed reports whether this exact
// candidate's terminal review authority was already acknowledged -- the same
// evidence STATUS itself consults (reviewtransaction.CompactTargetConsumed)
// before ever offering a fresh START for the identical identity -- so a host
// never re-derives review_due from a tier that a burned lineage has already
// settled.
type ReviewAssessmentCandidate struct {
	Kind     string `json:"kind"`
	BaseRef  string `json:"base_ref,omitempty"`
	Consumed bool   `json:"consumed"`
}

// ReviewAssessmentNextTransition is the exact, literally runnable
// `gentle-ai review status ... --next-transition` preflight continuation for
// a review_due=true candidate, rendered with the same ReviewTransitionArgument
// rows and builder conventions review_next_transition.go uses for every other
// negotiated continuation this product emits. Command is "gentle-ai " plus
// every argument's token joined by single spaces, so a caller never
// hand-assembles the invocation from prose (issue: orchestrators skipped the
// RDD preflight after a slice closed because the ODD rule was prose only).
type ReviewAssessmentNextTransition struct {
	Operation string                     `json:"operation"`
	Command   string                     `json:"command"`
	Arguments []ReviewTransitionArgument `json:"arguments"`
}

// Public review_due_reason vocabulary. Exactly one of these accompanies every
// review_due value, in the precedence order reviewAssessDue evaluates: a
// consumed candidate always reports already_reviewed regardless of its risk
// tier, high risk always reports true, a medium candidate reports true only
// once its changed_lines reaches the same reviewtransaction.LargeChangeLines
// slice boundary the ODD delivery-budget rule already names, and passive
// never needs review at all.
const (
	reviewAssessDueReasonHighRisk           = "high_risk"
	reviewAssessDueReasonSliceBudgetReached = "slice_budget_reached"
	reviewAssessDueReasonPassive            = "passive"
	reviewAssessDueReasonUnderBudget        = "under_budget"
	reviewAssessDueReasonAlreadyReviewed    = "already_reviewed"
)

// ReviewAssessmentResult is the complete gentle-ai.review-assessment/v1
// envelope. Risk is the public vocabulary: "passive" is exactly the tier
// reviewtransaction.RiskLow names -- the same zero-lens, structural-readback
// tier START selects for it -- and "medium"/"high" are unchanged so this
// projection can never disagree with START's own classification of the same
// candidate. ReviewDue and ReviewDueReason turn that tier into the one
// consequence orchestrators actually need (issue: the ~400-line ODD slice
// rule was prose only, and it was skipped in practice); NextTransition is the
// literal preflight command to run when ReviewDue is true, and is absent
// otherwise.
type ReviewAssessmentResult struct {
	Schema          string                          `json:"schema"`
	Risk            string                          `json:"risk"`
	Reasons         []ReviewAssessmentReason        `json:"reasons"`
	ChangedPaths    int                             `json:"changed_paths"`
	ChangedLines    int                             `json:"changed_lines"`
	Candidate       ReviewAssessmentCandidate       `json:"candidate"`
	ReviewDue       bool                            `json:"review_due"`
	ReviewDueReason string                          `json:"review_due_reason"`
	NextTransition  *ReviewAssessmentNextTransition `json:"next_transition,omitempty"`
}

// reviewAssessPublicRisk maps the internal risk tier to the public
// gentle-ai.review-assessment/v1 vocabulary. RiskLow is renamed to "passive"
// because that is exactly the condition it names: every authored path proven
// passive documentation by its own frozen bytes, the one tier START selects
// zero reviewer lenses for. Medium and high keep their internal spelling
// unchanged.
func reviewAssessPublicRisk(level reviewtransaction.RiskLevel) (string, error) {
	switch level {
	case reviewtransaction.RiskLow:
		return "passive", nil
	case reviewtransaction.RiskMedium:
		return "medium", nil
	case reviewtransaction.RiskHigh:
		return "high", nil
	default:
		return "", fmt.Errorf("review assess computed an unsupported risk level %q; this is a defect in gentle-ai itself, not a request error -- file it and retry with gentle-ai review assess --help", level)
	}
}

// reviewAssessmentReasons projects the internal risk reasons onto the public
// envelope shape, folding the signal and mode-change evidence a bare code
// does not carry into an optional human-readable detail string.
func reviewAssessmentReasons(reasons []reviewtransaction.RiskReason) []ReviewAssessmentReason {
	public := make([]ReviewAssessmentReason, 0, len(reasons))
	for _, reason := range reasons {
		var detail []string
		if reason.Signal != "" {
			detail = append(detail, "signal: "+string(reason.Signal))
		}
		if reason.OldMode != "" || reason.NewMode != "" {
			detail = append(detail, fmt.Sprintf("mode %s -> %s", reason.OldMode, reason.NewMode))
		}
		public = append(public, ReviewAssessmentReason{
			Code: string(reason.Code), Path: reason.Path, Detail: strings.Join(detail, "; "),
		})
	}
	return public
}

// reviewAssessDue derives review_due and its reason from the same evidence
// START itself never disagrees with: whether this exact candidate identity
// was already acknowledged, and the public risk tier reviewAssessPublicRisk
// already computed. Order matters and is deliberate: a consumed candidate
// reports already_reviewed even when its content would otherwise classify as
// high or over-budget medium, because the terminal authority already settled
// it and a fresh START for the identical identity would only replay the same
// acknowledgement STATUS itself would offer.
func reviewAssessDue(consumed bool, publicRisk string, changedLines int) (bool, string) {
	switch {
	case consumed:
		return false, reviewAssessDueReasonAlreadyReviewed
	case publicRisk == "high":
		return true, reviewAssessDueReasonHighRisk
	case publicRisk == "medium" && changedLines >= reviewtransaction.LargeChangeLines:
		return true, reviewAssessDueReasonSliceBudgetReached
	case publicRisk == "medium":
		return false, reviewAssessDueReasonUnderBudget
	default:
		return false, reviewAssessDueReasonPassive
	}
}

// reviewAssessNextTransitionFor renders the exact `review status
// --next-transition` preflight continuation for a review_due=true candidate.
// Argument order is fixed: --cwd, --contract, the optional --agent the caller
// declared, --next-transition, and -- only for a named base comparison -- the
// caller's own --base-ref echoed verbatim plus --committed-only. A
// current-changes candidate (no --base-ref) omits the selector pair entirely;
// the preflight STATUS this continuation runs collects the current-changes
// scope itself. root is the absolute repository root review assess already
// resolved (reviewtransaction.PrepareReviewRepositoryRoot), never the raw
// --cwd the caller passed, so the printed command runs unchanged from any
// working directory.
func reviewAssessNextTransitionFor(root string, runtime model.AgentID, baseRef string) *ReviewAssessmentNextTransition {
	arguments := []ReviewTransitionArgument{
		{Name: "cwd", Value: root},
		{Name: "contract", Value: ReviewIntegrationContractV2},
	}
	if runtime != "" {
		arguments = append(arguments, ReviewTransitionArgument{Name: "agent", Value: string(runtime)})
	}
	arguments = append(arguments, ReviewTransitionArgument{Name: "next-transition", Value: "true"})
	if baseRef != "" {
		arguments = append(arguments,
			ReviewTransitionArgument{Name: "base-ref", Value: baseRef},
			ReviewTransitionArgument{Name: "committed-only", Value: "true"},
		)
	}
	tokenized := reviewTokenizedTransitionArguments(arguments)
	return &ReviewAssessmentNextTransition{
		Operation: "review.status",
		Command:   reviewTransitionCommandLine("review.status", tokenized),
		Arguments: tokenized,
	}
}

func reviewFlagProvided(args []string, flag string) bool {
	for _, arg := range args {
		if arg == flag {
			return true
		}
		if strings.HasPrefix(arg, flag+"=") {
			val := strings.TrimPrefix(arg, flag+"=")
			if val == "false" || val == "0" {
				return false
			}
			return true
		}
	}
	return false
}

// RunReviewAssess is the read-only `gentle-ai review assess` command. It
// builds the exact same candidate review start would (current changes, or a
// named --base-ref comparison), runs the shared risk assessment, and prints
// it: no authority, no lineage, no store mutation, and no lock beyond an
// ordinary read. It works identically whether or not receipt-driven
// development is enabled, so a host can gate delegated verification on the
// result before ever calling review start (issue #4295).
//
// When the candidate cannot be built or assessed, this command fails closed:
// every returned error names a runnable `gentle-ai review assess ...`
// continuation (or an unambiguous %w propagation of the underlying native
// failure). Hosts that cannot resolve the named continuation should treat the
// failure exactly as they would treat a "high" result.
func RunReviewAssess(args []string, stdout io.Writer) error {
	ctx := context.Background()
	jsonRequested := reviewFlagProvided(args, "--json")
	var failClosedCandidate ReviewAssessmentCandidate
	failClosedCandidate.Kind = string(reviewtransaction.TargetCurrentChanges)

	failClosed := func(err error, candidate ReviewAssessmentCandidate) error {
		if !jsonRequested {
			return err
		}
		res := ReviewAssessmentResult{
			Schema:          ReviewAssessmentSchema,
			Risk:            "high",
			Reasons:         []ReviewAssessmentReason{{Code: "unassessable", Detail: reviewScrubDefectReportField(err.Error())}},
			ChangedPaths:    0,
			ChangedLines:    0,
			Candidate:       candidate,
			ReviewDue:       true,
			ReviewDueReason: reviewAssessDueReasonHighRisk,
		}
		_ = encodeReviewJSON(stdout, res)
		return err
	}

	flags := newReviewFlagSet("review assess", stdout,
		"Print the read-only candidate risk assessment review start would use to select lenses, without creating any review authority.")
	cwd := flags.String("cwd", ".", "repository path")
	baseRef := flags.String("base-ref", "", "optional base revision for an immutable base-to-HEAD assessment")
	committedOnly := flags.Bool("committed-only", false, "acknowledge that --base-ref excludes dirty tracked changes")
	agent := flags.String("agent", "", "optional generated active runtime identity to carry on the next_transition preflight")
	jsonOutput := flags.Bool("json", false, "print the gentle-ai.review-assessment/v1 envelope as JSON instead of human-readable text")
	untrackedScope := reviewSingleValueFlag{}
	intendedUntracked := reviewRepeatedPathFlag{}
	expectedUntrackedInventory := reviewSingleValueFlag{}
	flags.Var(&untrackedScope, "untracked-scope", "explicit untracked scope: exclude or select")
	flags.Var(&intendedUntracked, "intended-untracked", "repo-relative untracked path to include; repeat for each path")
	flags.Var(&expectedUntrackedInventory, "expected-untracked-inventory", "sha256 inventory digest from review status")
	if err := parseReviewFlags(flags, args); err != nil {
		return failClosed(err, failClosedCandidate)
	}
	if reviewHelpRequested(args) {
		return nil
	}
	if flags.NArg() != 0 {
		return failClosed(reviewPreflightError(fmt.Errorf("unexpected review assess argument %q; run `gentle-ai review assess --help` for the closed command form", flags.Arg(0))), failClosedCandidate)
	}

	trimmedBaseRef := strings.TrimSpace(*baseRef)
	if trimmedBaseRef != "" {
		failClosedCandidate.Kind = string(reviewtransaction.TargetBaseDiff)
		failClosedCandidate.BaseRef = trimmedBaseRef
	}

	// The declared runtime identity is validated exactly as `review status`
	// validates its own --agent (reviewRuntimeWithImmutableTransport), so an
	// unsupported transport refuses the same way here before any repository
	// work: assess is read-only, but the runtime it will hand to
	// next_transition must still be one review status itself would accept.
	var runtimeAgent model.AgentID
	if strings.TrimSpace(*agent) != "" {
		var runtimeErr error
		runtimeAgent, runtimeErr = reviewRuntimeWithImmutableTransport(*agent)
		if runtimeErr != nil {
			return failClosed(reviewPreflightRefusal(reviewImmutableTransportUnsupportedReason, runtimeErr), failClosedCandidate)
		}
	}

	root, err := reviewtransaction.PrepareReviewRepositoryRoot(ctx, *cwd)
	if err != nil {
		return failClosed(fmt.Errorf("review assess: resolve repository root: %w", err), failClosedCandidate)
	}
	builder := reviewtransaction.SnapshotBuilder{Repo: root}

	target := reviewtransaction.Target{
		Kind: reviewtransaction.TargetCurrentChanges, Projection: reviewtransaction.ProjectionWorkspace,
		IntendedUntracked: []string{},
	}
	if trimmedBaseRef != "" {
		target.Kind = reviewtransaction.TargetBaseDiff
		target.BaseRef = trimmedBaseRef
		dirtyTracked, dirtyErr := builder.HasDirtyTrackedChanges(ctx)
		if dirtyErr != nil {
			return failClosed(fmt.Errorf("review assess: detect dirty tracked changes: %w", dirtyErr), failClosedCandidate)
		}
		if dirtyTracked && !*committedOnly {
			return failClosed(reviewPreflightError(fmt.Errorf(
				"review assess with --base-ref omits dirty tracked changes; rerun `gentle-ai review assess --base-ref %s --committed-only` to acknowledge committed-only scope",
				trimmedBaseRef)), failClosedCandidate)
		}
	}

	intendedScope, err := intendedUntrackedScopeForTarget(ctx, builder, untrackedScope, intendedUntracked, expectedUntrackedInventory,
		reviewIntendedUntrackedInventoryCommand, "gentle-ai review assess")
	if err != nil {
		return failClosed(reviewPreflightError(err), failClosedCandidate)
	}
	if intendedScope.NeedsSelection {
		return failClosed(reviewPreflightError(intendedUntrackedSelectionRequired(intendedScope, reviewIntendedUntrackedInventoryCommand, "gentle-ai review assess")), failClosedCandidate)
	}
	target.IntendedUntracked = intendedScope.Intended

	snapshot, err := builder.Build(ctx, target)
	if err != nil {
		return failClosed(fmt.Errorf("review assess could not build the candidate; correct --cwd or --base-ref and retry with `gentle-ai review assess --help`: %w", err), failClosedCandidate)
	}
	if reviewStartEmptyCandidateScope(snapshot) {
		return failClosed(reviewPreflightError(errors.New(
			"the review assess candidate has no pending changes; already-committed work can be assessed by rerunning `gentle-ai review assess --base-ref <commit>` naming the base to compare against")), failClosedCandidate)
	}

	assessment, err := builder.AssessSnapshotRisk(ctx, snapshot)
	if err != nil {
		return failClosed(fmt.Errorf("review assess could not classify the candidate; retry with `gentle-ai review assess --help` or a narrower --base-ref: %w", err), failClosedCandidate)
	}
	publicRisk, err := reviewAssessPublicRisk(assessment.Level)
	if err != nil {
		return failClosed(err, failClosedCandidate)
	}

	// Computed the same way STATUS itself computes it, so a lineage STATUS
	// would already report consumed can never disagree with what assess
	// reports here (reviewtransaction.CompactTargetConsumed never inverts a
	// tombstone; it matches by identity re-derivation only).
	consumed, err := reviewtransaction.CompactTargetConsumed(ctx, root, snapshot.Identity)
	if err != nil {
		return failClosed(fmt.Errorf("review assess could not read terminal consumption evidence; retry with `gentle-ai review assess --help`: %w", err), failClosedCandidate)
	}
	reviewDue, reviewDueReason := reviewAssessDue(consumed, publicRisk, assessment.ChangedLines)
	var nextTransition *ReviewAssessmentNextTransition
	if reviewDue {
		nextTransition = reviewAssessNextTransitionFor(root, runtimeAgent, trimmedBaseRef)
	}

	result := ReviewAssessmentResult{
		Schema: ReviewAssessmentSchema, Risk: publicRisk, Reasons: reviewAssessmentReasons(assessment.Reasons),
		ChangedPaths: len(snapshot.Paths), ChangedLines: assessment.ChangedLines,
		Candidate:       ReviewAssessmentCandidate{Kind: string(snapshot.Kind), BaseRef: trimmedBaseRef, Consumed: consumed},
		ReviewDue:       reviewDue,
		ReviewDueReason: reviewDueReason,
		NextTransition:  nextTransition,
	}

	if *jsonOutput {
		return encodeReviewJSON(stdout, result)
	}
	return writeReviewAssessmentHuman(stdout, result)
}

// writeReviewAssessmentHuman renders the same assessment as readable text,
// for a caller that has not asked for the JSON envelope.
func writeReviewAssessmentHuman(stdout io.Writer, result ReviewAssessmentResult) error {
	candidate := result.Candidate.Kind
	if result.Candidate.BaseRef != "" {
		candidate = fmt.Sprintf("%s (base-ref %s)", candidate, result.Candidate.BaseRef)
	}
	if _, err := fmt.Fprintf(stdout, "Risk: %s\nCandidate: %s\nChanged paths: %d\nChanged lines: %d\n",
		result.Risk, candidate, result.ChangedPaths, result.ChangedLines); err != nil {
		return err
	}
	due := "no"
	if result.ReviewDue {
		due = "yes"
	}
	dueLine := fmt.Sprintf("review due: %s (%s)", due, result.ReviewDueReason)
	if result.NextTransition != nil {
		dueLine += " -> " + result.NextTransition.Command
	}
	if _, err := fmt.Fprintln(stdout, dueLine); err != nil {
		return err
	}
	if len(result.Reasons) == 0 {
		return nil
	}
	if _, err := fmt.Fprintln(stdout, "Reasons:"); err != nil {
		return err
	}
	for _, reason := range result.Reasons {
		line := "  - " + reason.Code
		if reason.Path != "" {
			line += " (" + reason.Path + ")"
		}
		if reason.Detail != "" {
			line += ": " + reason.Detail
		}
		if _, err := fmt.Fprintln(stdout, line); err != nil {
			return err
		}
	}
	return nil
}
