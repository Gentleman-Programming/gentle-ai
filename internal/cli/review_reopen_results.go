package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

const ReviewResultReopenSchema = "gentle-ai.review-result-reopen-result/v1"

type ReviewResultReopenResult struct {
	Schema    string                                       `json:"schema"`
	Operation string                                       `json:"operation"`
	Prepared  bool                                         `json:"prepared"`
	Plan      *reviewtransaction.CompactResultReopenPlan   `json:"plan,omitempty"`
	Record    *reviewtransaction.CompactResultReopenRecord `json:"record,omitempty"`
}

// RunReviewReopenResults exposes the only path returning an uncorrected
// validating or correction-required authority to reviewing. Native detection
// covers reviewer slots that were historically accepted without provider
// admission or whose own evidence proves that repository inspection was
// unavailable; --quarantine-lens additionally lets a maintainer override
// named admitted slots when the reviewers' input, not the candidate, was
// wrong. The overridden bytes are preserved in quarantine and the audit
// record names each authorized lens.
func RunReviewReopenResults(args []string, stdout io.Writer) error {
	flags := newReviewFlagSet("review reopen-results", stdout, "Prepare or apply an exact-revision, maintainer-authorized same-lineage quarantine of unusable reviewer results. Frozen scope, lenses, risk, and correction budget are preserved; provider-admitted usable slots are retained unless a maintainer names them with --quarantine-lens because the reviewer's input, not the candidate, was wrong.")
	cwd := flags.String("cwd", ".", "repository path")
	lineage := flags.String("lineage", "", "exact review lineage identifier")
	revision := flags.String("expected-revision", "", "exact uncorrected validating or correction-required authority revision")
	target := flags.String("target", "", "exact frozen target identity")
	reason := flags.String("reason", "", "maintainer reason for reopening reviewer results")
	actor := flags.String("actor", "", "maintainer actor authorizing the operation")
	authorization := flags.String("maintainer-authorization", "", "exact authorization emitted by --prepare")
	var quarantineLenses repeatedString
	flags.Var(&quarantineLenses, "quarantine-lens", "selected lens whose admitted result the maintainer overrides into quarantine; repeatable, bound verbatim into the emitted authorization")
	prepare := flags.Bool("prepare", false, "derive the immutable quarantine plan and required authorization without mutation")
	if err := parseReviewFlags(flags, args); err != nil {
		return err
	}
	if reviewHelpRequested(args) {
		return nil
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected review reopen-results argument %q", flags.Arg(0))
	}
	if strings.TrimSpace(*lineage) == "" || strings.TrimSpace(*revision) == "" || strings.TrimSpace(*target) == "" ||
		strings.TrimSpace(*reason) == "" || strings.TrimSpace(*actor) == "" {
		return errors.New("review reopen-results requires --cwd, --lineage, --expected-revision, --target, --reason, and --actor")
	}
	if *prepare && strings.TrimSpace(*authorization) != "" {
		return errors.New("review reopen-results --prepare derives maintainer authorization and does not accept --maintainer-authorization")
	}
	request := reviewtransaction.CompactResultReopenRequest{
		LineageID: *lineage, ExpectedRevision: *revision, TargetIdentity: *target,
		Reason: *reason, Actor: *actor, QuarantineLenses: quarantineLenses,
		MaintainerAuthorization: *authorization,
	}
	ctx := context.Background()
	if _, err := resolveReviewMutationRoot(ctx, *cwd); err != nil {
		return err
	}
	if *prepare {
		plan, err := reviewtransaction.PrepareCompactResultReopen(ctx, *cwd, request)
		if err != nil {
			return fmt.Errorf("prepare reviewer result reopen: %w", err)
		}
		return encodeReviewJSON(stdout, ReviewResultReopenResult{
			Schema: ReviewResultReopenSchema, Operation: reviewtransaction.CompactResultReopenOperation,
			Prepared: true, Plan: &plan,
		})
	}
	record, err := reviewtransaction.ReopenCompactReviewerResults(ctx, *cwd, request)
	if err != nil {
		return fmt.Errorf("reopen reviewer results: %w", err)
	}
	return encodeReviewJSON(stdout, ReviewResultReopenResult{
		Schema: ReviewResultReopenSchema, Operation: reviewtransaction.CompactResultReopenOperation,
		Record: &record,
	})
}
