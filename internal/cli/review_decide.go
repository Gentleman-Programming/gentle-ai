package cli

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v3/internal/reviewtransaction"
)

type ReviewDecideResult struct {
	Operation string                          `json:"operation"`
	Record    reviewtransaction.CompactRecord `json:"record"`
}

// reviewDecideInputsRefusal names, for every value the decide gate demands,
// the one command that publishes it and the exact field to read it from,
// mirroring the abandon precedent: the human act is choosing --decision and
// supplying --actor/--reason; the gate never invents them.
func reviewDecideInputsRefusal(cwd string) error {
	return fmt.Errorf(`review decide requires --lineage, --expected-revision, --decision, --actor, and --reason.
Read every persisted value you cannot invent from the authority inventory, in the entries[] row for the paused lineage, with:
gentle-ai review status --cwd %s
--lineage = entries[].lineage_id
--expected-revision = entries[].revision
--decision = continue (resume the review) or stop (close it as escalated)
The STATUS decision question carries both runnable invocations with the current expected revision.
You choose --actor and --reason; the decision journal stores them trimmed.`, cwd)
}

// RunReviewDecide resolves one decision_required compact lineage: continue
// resumes the reviewing authority with the answered question cleared; stop
// escalates it with the frozen question and decision journal retained. An
// exact replay of a committed decision converges idempotently; anything else
// on a non-paused authority is refused.
func RunReviewDecide(args []string, stdout io.Writer) error {
	flags := newReviewFlagSet("review decide", stdout, "Resolve one decision_required compact lineage through the closed continue|stop vocabulary. The compare-and-swap bound to the expected revision proves a losing caller mutated nothing; an exact replay of a committed decision converges on the committed record. Refuses every non-paused authority: only the paused question itself can be decided.")
	cwd := flags.String("cwd", ".", "repository path")
	lineage := flags.String("lineage", "", "decision_required compact lineage to resolve")
	expected := flags.String("expected-revision", "", "exact current authority revision")
	decision := flags.String("decision", "", "continue or stop")
	actor := flags.String("actor", "", "deciding actor")
	reason := flags.String("reason", "", "decision reason journaled with the entry")
	if err := parseReviewFlags(flags, args); err != nil {
		return err
	}
	if reviewHelpRequested(args) {
		return nil
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected review decide argument %q; run `gentle-ai review decide --help` for the exact flag set", flags.Arg(0))
	}
	for _, required := range []string{*lineage, *expected, *decision, *actor, *reason} {
		if strings.TrimSpace(required) == "" {
			return reviewDecideInputsRefusal(*cwd)
		}
	}
	root, err := resolveReviewOperationRoot(context.Background(), *cwd, reviewtransaction.RDDOperationMutate)
	if err != nil {
		return fmt.Errorf("resolve review repository root: %w", err)
	}
	record, err := reviewtransaction.DecideCompactStore(context.Background(), root, reviewtransaction.CompactDecisionRequest{
		LineageID: *lineage, ExpectedRevision: *expected, Decision: *decision, Actor: *actor, Reason: *reason,
	})
	if err != nil {
		return err
	}
	return encodeReviewJSON(stdout, ReviewDecideResult{Operation: "review/decide", Record: record})
}
