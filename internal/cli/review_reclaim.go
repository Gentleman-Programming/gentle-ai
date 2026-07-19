package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/gentleman-programming/gentle-ai/internal/reviewtransaction"
)

type ReviewReclaimResult struct {
	Operation string                                 `json:"operation"`
	Record    reviewtransaction.CompactReclaimRecord `json:"record"`
}

func RunReviewReclaim(args []string, stdout io.Writer) error {
	flags := newReviewFlagSet("review reclaim", stdout, "Quarantine one explicit incomplete compact-v2 store entry with a persisted audit record; entries holding any authoritative artifact are refused — an invalid recovery successor is quarantined with review reconcile-authority instead. On partial failure the prepared audit record JSON is still emitted to stdout and the command exits non-zero.")
	cwd := flags.String("cwd", ".", "repository path")
	lineage := flags.String("lineage", "", "explicit incomplete compact store lineage")
	reason := flags.String("reason", "", "non-empty reclaim reason")
	actor := flags.String("actor", "", "reclaim actor")
	if err := parseReviewFlags(flags, args); err != nil {
		return err
	}
	if reviewHelpRequested(args) {
		return nil
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected review reclaim argument %q", flags.Arg(0))
	}
	if strings.TrimSpace(*lineage) == "" || strings.TrimSpace(*reason) == "" || strings.TrimSpace(*actor) == "" {
		return errors.New("review reclaim requires --lineage, --reason, and --actor")
	}
	root, err := (reviewtransaction.SnapshotBuilder{Repo: *cwd}).ResolveRepositoryRoot(context.Background())
	if err != nil {
		return fmt.Errorf("resolve review repository root: %w", err)
	}
	record, err := reviewtransaction.ReclaimIncompleteCompactStore(context.Background(), root, reviewtransaction.CompactReclaimRequest{
		LineageID: *lineage, Reason: *reason, Actor: *actor,
	})
	if err != nil {
		// A partial reclaim persisted the prepared audit record and may have
		// moved the residue; surface the quarantine location for reconciliation.
		if record.QuarantinePath != "" {
			_ = encodeReviewJSON(stdout, ReviewReclaimResult{Operation: "review/reclaim", Record: record})
		}
		return err
	}
	return encodeReviewJSON(stdout, ReviewReclaimResult{Operation: "review/reclaim", Record: record})
}
