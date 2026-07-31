package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

const (
	// ReviewRepairRecoveryBindingSchema is a maintenance-tier result schema. Like
	// review reconcile-authority and review inspect-authority it carries no
	// packaged JSON schema; the provider contract surfaces stay untouched.
	ReviewRepairRecoveryBindingSchema    = "gentle-ai.review-recovery-binding-repair/v1"
	reviewRepairRecoveryBindingOperation = "review/repair-recovery-binding"
)

// ReviewRepairRecoveryBindingResult publishes, per admissible edge, every value
// the maintainer authorization binds except actor and reason, so preflight is a
// complete set of required inputs rather than an unsupported dead end.
type ReviewRepairRecoveryBindingResult struct {
	Schema              string                                                    `json:"schema"`
	Operation           string                                                    `json:"operation"`
	Mode                ReviewRepairMode                                          `json:"mode"`
	Counts              *reviewtransaction.CompactRecoveryBindingRepairCounts     `json:"counts,omitempty"`
	Candidates          []reviewtransaction.CompactRecoveryBindingRepairCandidate `json:"candidates,omitempty"`
	RequiredInputs      []string                                                  `json:"required_inputs"`
	AuthorizationSchema string                                                    `json:"authorization_schema,omitempty"`
	AuthorizationLines  []string                                                  `json:"authorization_lines,omitempty"`
	Record              *reviewtransaction.CompactReclaimRecord                   `json:"record,omitempty"`
}

func (result ReviewRepairRecoveryBindingResult) Validate() error {
	if result.Schema != ReviewRepairRecoveryBindingSchema || result.Operation != reviewRepairRecoveryBindingOperation {
		return reviewRepairRecoveryBindingInvariant("result identity is invalid")
	}
	switch result.Mode {
	case ReviewRepairModePreflight:
		if result.Record != nil || result.Counts == nil || result.Candidates == nil ||
			result.AuthorizationSchema != reviewtransaction.AuthorityRepairAuthorizationSchema ||
			!reflect.DeepEqual(result.AuthorizationLines, reviewtransaction.CompactRecoveryBindingRepairAuthorizationLines()) ||
			!reflect.DeepEqual(result.RequiredInputs, []string{"actor", "reason", "maintainer_authorization"}) {
			return reviewRepairRecoveryBindingInvariant("preflight does not publish the complete required inputs")
		}
		for _, candidate := range result.Candidates {
			if candidate.Class != reviewtransaction.AuthorityRepairClassCompactV2RecoveryTargetDrift ||
				candidate.PredecessorLineageID == "" || candidate.ExpectedPredecessorRevision == "" ||
				candidate.SuccessorLineageID == "" || candidate.ExpectedSuccessorRevision == "" ||
				candidate.PersistedTargetIdentity == "" || candidate.AuthorizedTargetIdentity == "" ||
				candidate.RecordedAuthorizationSHA256 == "" || candidate.RepositoryBinding == "" {
				return reviewRepairRecoveryBindingInvariant("preflight candidate is incomplete")
			}
		}
		return nil
	case ReviewRepairModeExecute:
		if result.Record == nil || result.Counts != nil || result.Candidates != nil ||
			result.AuthorizationSchema != "" || len(result.AuthorizationLines) != 0 || len(result.RequiredInputs) != 0 {
			return reviewRepairRecoveryBindingInvariant("execution result is invalid")
		}
		return nil
	}
	return reviewRepairRecoveryBindingInvariant(fmt.Sprintf("mode %q is unknown", result.Mode))
}

// reviewRepairRecoveryBindingInvariant frames a result this file built and
// cannot validate.
func reviewRepairRecoveryBindingInvariant(detail string) error {
	// refusal:by-design world-action: the value is constructed in this file from provider output, so a shape that fails validation is a programmer invariant breach; the exit is an action on the code, not a command an operator could run
	return fmt.Errorf("review repair-recovery-binding %s", detail)
}

func RunReviewRepairRecoveryBinding(args []string, stdout io.Writer) error {
	return runReviewRepairRecoveryBinding(context.Background(), args, stdout)
}

func runReviewRepairRecoveryBinding(ctx context.Context, args []string, stdout io.Writer) error {
	flags := newReviewFlagSet("review repair-recovery-binding", stdout, "Quarantine one compact-v2 recovery successor whose recorded gentle-ai.review-recovery-authorization/v1 binding names a target identity the successor never persisted. Run --preflight first: it lists every admissible edge leaf-first — descendants before ancestors — with both lineage IDs, both expected revisions and every drift value the authorization binds, and never a completed authorization. The repair is single-edge and compare-and-swap guarded; a repository with several drifted edges repairs them sequentially, and an exact replay converges without moving anything twice.")
	cwd := flags.String("cwd", ".", "repository path")
	preflight := flags.Bool("preflight", false, "perform deterministic read-only classification without authority mutation")
	predecessor := flags.String("predecessor-lineage", "", "exact recovery predecessor lineage; it stays untouched")
	expectedPredecessor := flags.String("expected-predecessor-revision", "", "exact predecessor revision")
	successor := flags.String("successor-lineage", "", "drifted recovery successor lineage to quarantine")
	expectedSuccessor := flags.String("expected-successor-revision", "", "exact successor revision")
	actor := flags.String("actor", "", "maintainer actor; never emitted in public output")
	reason := flags.String("reason", "", "maintainer reason; never emitted in public output")
	authorization := flags.String("maintainer-authorization", "", "exact fourteen-line LF-only gentle-ai.review-repair-authorization/v1 binding; never emitted in public output")
	if err := parseReviewFlags(flags, args); err != nil {
		return err
	}
	if reviewHelpRequested(args) {
		return nil
	}
	if flags.NArg() != 0 {
		return reviewPreflightError(fmt.Errorf("unexpected review repair-recovery-binding argument %q; the verb takes flags only, so run `gentle-ai review repair-recovery-binding --preflight --cwd <repo>`", flags.Arg(0)))
	}
	if *preflight {
		if repairExecutionInputPresent(*actor, *reason, *authorization) {
			return reviewPreflightError(errors.New("review repair-recovery-binding --preflight does not accept execution inputs; run `gentle-ai review repair-recovery-binding --preflight --cwd <repo>` to classify, then the same verb without --preflight to execute"))
		}
		root, err := (reviewtransaction.SnapshotBuilder{Repo: *cwd}).ResolveRepositoryRoot(ctx)
		if err != nil {
			return fmt.Errorf("resolve review repository root: %w", err)
		}
		candidates, counts, err := reviewtransaction.InspectCompactRecoveryBindingRepairCandidates(ctx, root,
			reviewtransaction.CompactRecoveryBindingRepairSelector{
				PredecessorLineageID: *predecessor, ExpectedPredecessorRevision: *expectedPredecessor,
				SuccessorLineageID: *successor, ExpectedSuccessorRevision: *expectedSuccessor,
			})
		if err != nil {
			return reviewPreflightError(err)
		}
		result := ReviewRepairRecoveryBindingResult{
			Schema: ReviewRepairRecoveryBindingSchema, Operation: reviewRepairRecoveryBindingOperation,
			Mode: ReviewRepairModePreflight, Counts: &counts, Candidates: candidates,
			RequiredInputs:      []string{"actor", "reason", "maintainer_authorization"},
			AuthorizationSchema: reviewtransaction.AuthorityRepairAuthorizationSchema,
			AuthorizationLines:  reviewtransaction.CompactRecoveryBindingRepairAuthorizationLines(),
		}
		if err := result.Validate(); err != nil {
			return fmt.Errorf("validate review repair-recovery-binding preflight: %w", err)
		}
		return encodeReviewJSON(stdout, result)
	}
	for _, required := range []string{*predecessor, *expectedPredecessor, *successor, *expectedSuccessor, *actor, *reason, *authorization} {
		if strings.TrimSpace(required) == "" {
			return reviewPreflightError(errors.New("review repair-recovery-binding execution requires --predecessor-lineage, --expected-predecessor-revision, --successor-lineage, --expected-successor-revision, --actor, --reason, and --maintainer-authorization; run `gentle-ai review repair-recovery-binding --preflight --cwd <repo>` to publish every value except actor and reason"))
		}
	}
	root, err := resolveReviewMutationRoot(ctx, *cwd)
	if err != nil {
		return fmt.Errorf("resolve review repository root: %w", err)
	}
	if err := authorizeReviewAuthorityMutation(ctx, root); err != nil {
		return err
	}
	record, err := reviewtransaction.RepairCompactRecoveryBinding(ctx, root, reviewtransaction.CompactRecoveryBindingRepairRequest{
		PredecessorLineageID: *predecessor, ExpectedPredecessorRevision: *expectedPredecessor,
		SuccessorLineageID: *successor, ExpectedSuccessorRevision: *expectedSuccessor,
		Actor: *actor, Reason: *reason, MaintainerAuthorization: *authorization,
	})
	result := ReviewRepairRecoveryBindingResult{
		Schema: ReviewRepairRecoveryBindingSchema, Operation: reviewRepairRecoveryBindingOperation,
		Mode: ReviewRepairModeExecute, RequiredInputs: []string{}, Record: &record,
	}
	if err != nil {
		// A partial repair persisted the prepared audit record and may have moved
		// the successor; surface the quarantine location for reconciliation.
		if record.QuarantinePath != "" {
			_ = encodeReviewJSON(stdout, result)
		}
		return err
	}
	if err := result.Validate(); err != nil {
		return fmt.Errorf("validate review repair-recovery-binding execution: %w", err)
	}
	return encodeReviewJSON(stdout, result)
}
