package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/gentleman-programming/gentle-ai/internal/reviewtransaction"
)

type ReviewReconcileAuthorityResult struct {
	Operation string                                 `json:"operation"`
	Record    reviewtransaction.CompactReclaimRecord `json:"record"`
}

func RunReviewReconcileAuthority(args []string, stdout io.Writer) error {
	flags := newReviewFlagSet("review reconcile-authority", stdout, "Quarantine one compact-v2 recovery successor whose recovery edge natively re-derives as invalid for either or both supported classes: an unchanged target and a historical pre-contract free-form maintainer authorization on an otherwise structurally consistent edge. The persisted audit record carries every re-derived proof; valid edges, incomplete entries, non-recovery records, and structurally inconsistent edges are refused. On partial failure the prepared audit record JSON is still emitted to stdout and the command exits non-zero.")
	cwd := flags.String("cwd", ".", "repository path")
	predecessor := flags.String("predecessor-lineage", "", "exact recovery predecessor lineage; it stays untouched")
	expectedPredecessor := flags.String("expected-predecessor-revision", "", "exact predecessor revision")
	successor := flags.String("successor-lineage", "", "invalid recovery successor lineage to quarantine")
	expectedSuccessor := flags.String("expected-successor-revision", "", "exact successor revision")
	reason := flags.String("reason", "", "non-empty reconcile reason")
	actor := flags.String("actor", "", "reconcile actor")
	authorization := flags.String("maintainer-authorization", "", "exact LF-only binding: gentle-ai.review-reconcile-authorization/v1, predecessor_lineage, predecessor_revision, successor_lineage, successor_revision, actor, reason; combined repair appends anomalies=unchanged_target,malformed_recovery_authorization")
	if err := parseReviewFlags(flags, args); err != nil {
		return err
	}
	if reviewHelpRequested(args) {
		return nil
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected review reconcile-authority argument %q", flags.Arg(0))
	}
	for _, required := range []string{*predecessor, *expectedPredecessor, *successor, *expectedSuccessor, *reason, *actor, *authorization} {
		if strings.TrimSpace(required) == "" {
			return errors.New("review reconcile-authority requires --predecessor-lineage, --expected-predecessor-revision, --successor-lineage, --expected-successor-revision, --reason, --actor, and --maintainer-authorization")
		}
	}
	root, err := (reviewtransaction.SnapshotBuilder{Repo: *cwd}).ResolveRepositoryRoot(context.Background())
	if err != nil {
		return fmt.Errorf("resolve review repository root: %w", err)
	}
	record, err := reviewtransaction.ReconcileInvalidRecoveryEdge(context.Background(), root, reviewtransaction.CompactReconcileRequest{
		PredecessorLineageID: *predecessor, ExpectedPredecessorRevision: *expectedPredecessor,
		SuccessorLineageID: *successor, ExpectedSuccessorRevision: *expectedSuccessor,
		Reason: *reason, Actor: *actor, MaintainerAuthorization: *authorization,
	})
	if err != nil {
		// A partial reconcile persisted the prepared audit record and may have
		// moved the successor; surface the quarantine location for reconciliation.
		if record.QuarantinePath != "" {
			_ = encodeReviewJSON(stdout, ReviewReconcileAuthorityResult{Operation: "review/reconcile-authority", Record: record})
		}
		return err
	}
	return encodeReviewJSON(stdout, ReviewReconcileAuthorityResult{Operation: "review/reconcile-authority", Record: record})
}
