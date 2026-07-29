package cli

import (
	"context"
	"fmt"
	"io"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

const ReviewInspectAuthoritySchema = "gentle-ai.review-authority-inspection/v1"

type ReviewInspectAuthorityResult struct {
	Schema           string                                             `json:"schema"`
	Operation        string                                             `json:"operation"`
	RepositoryRoot   string                                             `json:"repository_root"`
	Complete         bool                                               `json:"complete"`
	Valid            bool                                               `json:"valid"`
	Totals           reviewtransaction.CompactRecoveryInspectionTotals  `json:"totals"`
	Edges            []reviewtransaction.CompactRecoveryEdgeInspection  `json:"edges"`
	EntryDiagnostics []reviewtransaction.CompactRecoveryEntryDiagnostic `json:"entry_diagnostics"`
	// SanctionedExits names, per invalid edge, the operation that would accept
	// it right now. It is reported beside the edges rather than inside them
	// because the edge shape is the binding identity batch reconciliation
	// re-derives and compares, while this is derived against the live store.
	SanctionedExits []reviewtransaction.CompactRecoverySanctionedExit `json:"sanctioned_exits"`
}

var inspectCompactRecoveryEdges = reviewtransaction.InspectCompactRecoveryEdges

func RunReviewInspectAuthority(args []string, stdout io.Writer) error {
	return runReviewInspectAuthority(context.Background(), args, stdout)
}

func runReviewInspectAuthority(ctx context.Context, args []string, stdout io.Writer) error {
	flags := newReviewFlagSet("review inspect-authority", stdout, "Inspect every compact-v2 recovery edge and entry diagnostic without acquiring locks or mutating review authority.")
	cwd := flags.String("cwd", ".", "repository path")
	if err := parseReviewFlags(flags, args); err != nil {
		return err
	}
	if reviewHelpRequested(args) {
		return nil
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected review inspect-authority argument %q", flags.Arg(0))
	}
	root, err := (reviewtransaction.SnapshotBuilder{Repo: *cwd}).ResolveRepositoryRoot(ctx)
	if err != nil {
		return fmt.Errorf("resolve review repository root: %w", err)
	}
	report, err := inspectCompactRecoveryEdges(ctx, root)
	if err != nil {
		return fmt.Errorf("inspect review authority: %w", err)
	}
	exits, err := reviewtransaction.SanctionedCompactRecoveryExits(ctx, root, report)
	if err != nil {
		return fmt.Errorf("resolve review authority exits: %w", err)
	}
	return encodeReviewJSON(stdout, ReviewInspectAuthorityResult{
		Schema: ReviewInspectAuthoritySchema, Operation: "review/inspect-authority", RepositoryRoot: root,
		Complete: report.Complete, Valid: report.Valid, Totals: report.Totals,
		Edges:            append([]reviewtransaction.CompactRecoveryEdgeInspection{}, report.Edges...),
		EntryDiagnostics: append([]reviewtransaction.CompactRecoveryEntryDiagnostic{}, report.EntryDiagnostics...),
		SanctionedExits:  exits,
	})
}
