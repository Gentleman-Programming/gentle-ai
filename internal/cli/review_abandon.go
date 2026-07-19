package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/gentleman-programming/gentle-ai/internal/reviewtransaction"
)

type ReviewAbandonResult struct {
	Operation string                                 `json:"operation"`
	Record    reviewtransaction.CompactReclaimRecord `json:"record"`
}

func RunReviewAbandon(args []string, stdout io.Writer) error {
	flags := newReviewFlagSet("review abandon", stdout, "Quarantine one pristine compact-v2 review lineage — a reviewing authority that never captured lens results or a pristine invalidated authority — with a persisted audit record carrying the natively re-derived pristineness proof. Pristineness is proven from persisted bytes and store topology, never the live worktree, so stale lineages remain abandonable. Terminal, corrected, artifact-holding, and superseded lineages are refused; an exact replay of a committed abandonment converges idempotently. On partial failure the prepared audit record JSON is still emitted to stdout and the command exits non-zero.")
	cwd := flags.String("cwd", ".", "repository path")
	lineage := flags.String("lineage", "", "pristine compact store lineage to abandon")
	expected := flags.String("expected-revision", "", "exact current authority revision")
	reason := flags.String("reason", "", "non-empty abandonment reason")
	actor := flags.String("actor", "", "abandonment actor")
	authorization := flags.String("maintainer-authorization", "", "exact six-line LF-only binding: gentle-ai.review-abandon-authorization/v1, lineage, revision, snapshot_identity, actor, reason")
	if err := parseReviewFlags(flags, args); err != nil {
		return err
	}
	if reviewHelpRequested(args) {
		return nil
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected review abandon argument %q", flags.Arg(0))
	}
	for _, required := range []string{*lineage, *expected, *reason, *actor, *authorization} {
		if strings.TrimSpace(required) == "" {
			return errors.New("review abandon requires --lineage, --expected-revision, --reason, --actor, and --maintainer-authorization")
		}
	}
	root, err := (reviewtransaction.SnapshotBuilder{Repo: *cwd}).ResolveRepositoryRoot(context.Background())
	if err != nil {
		return fmt.Errorf("resolve review repository root: %w", err)
	}
	record, err := reviewtransaction.AbandonPristineCompactStore(context.Background(), root, reviewtransaction.CompactAbandonRequest{
		LineageID: *lineage, ExpectedRevision: *expected,
		Reason: *reason, Actor: *actor, MaintainerAuthorization: *authorization,
	})
	if err != nil {
		// A partial abandonment persisted the prepared audit record and may have
		// moved the entry; surface the quarantine location for reconciliation.
		if record.QuarantinePath != "" {
			_ = encodeReviewJSON(stdout, ReviewAbandonResult{Operation: "review/abandon", Record: record})
		}
		return err
	}
	return encodeReviewJSON(stdout, ReviewAbandonResult{Operation: "review/abandon", Record: record})
}
