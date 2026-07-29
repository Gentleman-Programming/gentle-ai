package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

const ReviewReconcileAuthorityBatchPreparationInputSchema = "gentle-ai.review-batch-reconcile-preparation-input/v1"
const ReviewReconcileAuthorityBatchResultSchema = "gentle-ai.review-batch-reconcile-result/v1"

// ReviewReconcileAuthorityBatchPreparationInput declares the complete invalid
// edge set and the maintainer identity used to derive an exact authorization.
type ReviewReconcileAuthorityBatchPreparationInput struct {
	Schema               string                                            `json:"schema"`
	DeclaredInvalidEdges []reviewtransaction.CompactRecoveryEdgeInspection `json:"declared_invalid_edges"`
	Actor                string                                            `json:"actor"`
	Reason               string                                            `json:"reason"`
}

// ReviewReconcileAuthorityBatchResult carries either a read-only preparation
// or the durable journal returned by an apply or exact replay.
type ReviewReconcileAuthorityBatchResult struct {
	Schema      string                                              `json:"schema"`
	Operation   string                                              `json:"operation"`
	Mode        string                                              `json:"mode"`
	Preparation *reviewtransaction.CompactBatchReconcilePreparation `json:"preparation,omitempty"`
	Journal     *reviewtransaction.CompactBatchReconcileJournal     `json:"journal,omitempty"`
}

func RunReviewReconcileAuthorityBatch(args []string, stdout io.Writer) error {
	return runReviewReconcileAuthorityBatch(context.Background(), args, stdout)
}

func runReviewReconcileAuthorityBatch(ctx context.Context, args []string, stdout io.Writer) error {
	flags := newReviewFlagSet(
		"review reconcile-authority-batch",
		stdout,
		"Prepare or atomically apply one exact batch reconciliation for the complete declared set of supported invalid compact-v2 recovery edges. Preparation performs no authority mutation and emits the byte-exact maintainer authorization. Apply revalidates the locked authority snapshot, durably journals every whole-entry quarantine move, fails closed after partial progress, and permits only exact replay.",
	)
	cwd := flags.String("cwd", ".", "repository path")
	input := flags.String("input", "", "strict JSON input file, or - for stdin")
	prepare := flags.Bool("prepare", false, "derive the exact plan and maintainer authorization without mutating authority")
	if err := parseReviewFlags(flags, args); err != nil {
		return err
	}
	if reviewHelpRequested(args) {
		return nil
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected review reconcile-authority-batch argument %q", flags.Arg(0))
	}
	if strings.TrimSpace(*input) == "" {
		return errors.New("review reconcile-authority-batch requires --input")
	}
	root, err := resolveReviewMutationRoot(ctx, *cwd)
	if err != nil {
		return fmt.Errorf("resolve review repository root: %w", err)
	}

	if *prepare {
		var preparationInput ReviewReconcileAuthorityBatchPreparationInput
		if err := readFacadeJSON(*input, &preparationInput); err != nil {
			return fmt.Errorf("read review reconcile-authority-batch preparation input: %w", err)
		}
		if preparationInput.Schema != ReviewReconcileAuthorityBatchPreparationInputSchema {
			return errors.New("review reconcile-authority-batch preparation requires the exact input schema")
		}
		if preparationInput.DeclaredInvalidEdges == nil {
			return errors.New("review reconcile-authority-batch preparation requires declared invalid edges")
		}
		preparation, err := reviewtransaction.PrepareCompactBatchReconciliation(
			ctx,
			root,
			preparationInput.DeclaredInvalidEdges,
			preparationInput.Actor,
			preparationInput.Reason,
		)
		if err != nil {
			return err
		}
		return encodeReviewJSON(stdout, ReviewReconcileAuthorityBatchResult{
			Schema: ReviewReconcileAuthorityBatchResultSchema, Operation: "review/reconcile-authority-batch",
			Mode: "prepared", Preparation: &preparation,
		})
	}

	var request reviewtransaction.CompactBatchReconcileRequest
	if err := readFacadeJSON(*input, &request); err != nil {
		return fmt.Errorf("read review reconcile-authority-batch request: %w", err)
	}
	journal, err := reviewtransaction.ReconcileInvalidRecoveryEdges(ctx, root, request)
	if err != nil {
		if journal.Status != "" {
			_ = encodeReviewJSON(stdout, ReviewReconcileAuthorityBatchResult{
				Schema: ReviewReconcileAuthorityBatchResultSchema, Operation: "review/reconcile-authority-batch",
				Mode: "prepared", Journal: &journal,
			})
		}
		return err
	}
	return encodeReviewJSON(stdout, ReviewReconcileAuthorityBatchResult{
		Schema: ReviewReconcileAuthorityBatchResultSchema, Operation: "review/reconcile-authority-batch",
		Mode: "committed", Journal: &journal,
	})
}
