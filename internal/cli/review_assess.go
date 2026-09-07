package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
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
// base comparison without re-deriving it.
type ReviewAssessmentCandidate struct {
	Kind    string `json:"kind"`
	BaseRef string `json:"base_ref,omitempty"`
}

// ReviewAssessmentResult is the complete gentle-ai.review-assessment/v1
// envelope. Risk is the public vocabulary: "passive" is exactly the tier
// reviewtransaction.RiskLow names -- the same zero-lens, structural-readback
// tier START selects for it -- and "medium"/"high" are unchanged so this
// projection can never disagree with START's own classification of the same
// candidate.
type ReviewAssessmentResult struct {
	Schema       string                    `json:"schema"`
	Risk         string                    `json:"risk"`
	Reasons      []ReviewAssessmentReason  `json:"reasons"`
	ChangedPaths int                       `json:"changed_paths"`
	ChangedLines int                       `json:"changed_lines"`
	Candidate    ReviewAssessmentCandidate `json:"candidate"`
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
	flags := newReviewFlagSet("review assess", stdout,
		"Print the read-only candidate risk assessment review start would use to select lenses, without creating any review authority.")
	cwd := flags.String("cwd", ".", "repository path")
	baseRef := flags.String("base-ref", "", "optional base revision for an immutable base-to-HEAD assessment")
	committedOnly := flags.Bool("committed-only", false, "acknowledge that --base-ref excludes dirty tracked changes")
	jsonOutput := flags.Bool("json", false, "print the gentle-ai.review-assessment/v1 envelope as JSON instead of human-readable text")
	untrackedScope := reviewSingleValueFlag{}
	intendedUntracked := reviewRepeatedPathFlag{}
	expectedUntrackedInventory := reviewSingleValueFlag{}
	flags.Var(&untrackedScope, "untracked-scope", "explicit untracked scope: exclude or select")
	flags.Var(&intendedUntracked, "intended-untracked", "repo-relative untracked path to include; repeat for each path")
	flags.Var(&expectedUntrackedInventory, "expected-untracked-inventory", "sha256 inventory digest from review status")
	if err := parseReviewFlags(flags, args); err != nil {
		return err
	}
	if reviewHelpRequested(args) {
		return nil
	}
	if flags.NArg() != 0 {
		return reviewPreflightError(fmt.Errorf("unexpected review assess argument %q; run `gentle-ai review assess --help` for the closed command form", flags.Arg(0)))
	}

	root, err := reviewtransaction.PrepareReviewRepositoryRoot(ctx, *cwd)
	if err != nil {
		return fmt.Errorf("review assess: resolve repository root: %w", err)
	}
	builder := reviewtransaction.SnapshotBuilder{Repo: root}

	trimmedBaseRef := strings.TrimSpace(*baseRef)
	target := reviewtransaction.Target{
		Kind: reviewtransaction.TargetCurrentChanges, Projection: reviewtransaction.ProjectionWorkspace,
		IntendedUntracked: []string{},
	}
	if trimmedBaseRef != "" {
		target.Kind = reviewtransaction.TargetBaseDiff
		target.BaseRef = trimmedBaseRef
		dirtyTracked, dirtyErr := builder.HasDirtyTrackedChanges(ctx)
		if dirtyErr != nil {
			return fmt.Errorf("review assess: detect dirty tracked changes: %w", dirtyErr)
		}
		if dirtyTracked && !*committedOnly {
			return reviewPreflightError(fmt.Errorf(
				"review assess with --base-ref omits dirty tracked changes; rerun `gentle-ai review assess --base-ref %s --committed-only` to acknowledge committed-only scope",
				trimmedBaseRef))
		}
	}

	intendedScope, err := intendedUntrackedScopeForTarget(ctx, builder, untrackedScope, intendedUntracked, expectedUntrackedInventory,
		reviewIntendedUntrackedInventoryCommand, "gentle-ai review assess")
	if err != nil {
		return reviewPreflightError(err)
	}
	if intendedScope.NeedsSelection {
		return reviewPreflightError(intendedUntrackedSelectionRequired(intendedScope, reviewIntendedUntrackedInventoryCommand, "gentle-ai review assess"))
	}
	target.IntendedUntracked = intendedScope.Intended

	snapshot, err := builder.Build(ctx, target)
	if err != nil {
		return fmt.Errorf("review assess could not build the candidate; correct --cwd or --base-ref and retry with `gentle-ai review assess --help`: %w", err)
	}
	if reviewStartEmptyCandidateScope(snapshot) {
		return reviewPreflightError(errors.New(
			"the review assess candidate has no pending changes; already-committed work can be assessed by rerunning `gentle-ai review assess --base-ref <commit>` naming the base to compare against"))
	}

	assessment, err := builder.AssessSnapshotRisk(ctx, snapshot)
	if err != nil {
		return fmt.Errorf("review assess could not classify the candidate; retry with `gentle-ai review assess --help` or a narrower --base-ref: %w", err)
	}
	publicRisk, err := reviewAssessPublicRisk(assessment.Level)
	if err != nil {
		return err
	}

	result := ReviewAssessmentResult{
		Schema: ReviewAssessmentSchema, Risk: publicRisk, Reasons: reviewAssessmentReasons(assessment.Reasons),
		ChangedPaths: len(snapshot.Paths), ChangedLines: assessment.ChangedLines,
		Candidate: ReviewAssessmentCandidate{Kind: string(snapshot.Kind), BaseRef: trimmedBaseRef},
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
