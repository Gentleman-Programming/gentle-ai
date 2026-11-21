package cli

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v4/internal/reviewtransaction"
)

// ReviewAbandonResult is the JSON `review abandon` prints. Record is the
// requested lineage's own quarantine. Cascaded is present only when the
// abandonment moved more entries (recovery successors, deepest first, and a
// same-lineage legacy-v1 entry); the authorization binding names exactly those,
// so a caller that could not have authorized them never receives the field.
type ReviewAbandonResult struct {
	Operation string                                   `json:"operation"`
	Record    reviewtransaction.CompactReclaimRecord   `json:"record"`
	Cascaded  []reviewtransaction.CompactReclaimRecord `json:"cascaded,omitempty"`
}

// reviewAbandonInputsRefusal names, for every value the gate demands, the one
// command that publishes it and the exact field to read it from, then prints
// the authorization template as its own final eight lines so the reader can
// fill it in without opening the source. The lookup command is the authority
// inventory rather than the negotiated target status on purpose: abandonment
// is defined over persisted bytes, so it must keep working for a lineage that
// has gone stale, and the negotiated status recomputes its snapshot identity
// from the live worktree and drops the authority block once the target drifts,
// so following it there would produce a token this gate rejects.
//
// Deliberately NOT a --prepare emitter, unlike review reopen-results. The
// values reopen-results derives are ones the maintainer already supplied as
// flags; its --prepare only spares them the string formatting. Abandonment
// has no --snapshot-identity flag, so the binding is the sole carrier of the
// claim "I read this specific lineage's frozen state". A mode that minted the
// token from --lineage alone would let a caller abandon a lineage it never
// looked at, which is the one thing this gate exists to prevent. Naming the
// field to read keeps the human act; emitting the token would remove it.
// (openspec/changes/organic-dx task 6.6 generalizes the emitter to the eight
// gated commands lacking one; for abandon that task must first add the
// snapshot identity as an operator-supplied input, or it weakens the gate.)
func reviewAbandonInputsRefusal(cwd string) error {
	return fmt.Errorf(`review abandon requires --lineage, --expected-revision, --reason, --actor, and --maintainer-authorization.
Read every value you cannot invent from the persisted authority, in the entries[] row for the lineage you are abandoning, with:
gentle-ai review status --cwd %s
--lineage = entries[].lineage_id
--expected-revision = entries[].revision
snapshot_identity = entries[].snapshot_identity
discarded_work = entries[].discarded_work; use captured_lens_results comma-joined with "," in listed order
--reason = %q or %q
You choose --actor; the binding uses it with surrounding whitespace trimmed.
For an entry whose record cannot be read (unreadable or missing review-state.json, or a legacy-v1 entry) revision and snapshot_identity are "unavailable".
If abandoning the lineage also moves recovery successors or a legacy-v1 entry, or its discarded work cannot be verified, the binding gains extra lines after actor= (discarded_work=unverifiable, one cascade=<lineage>@<revision> ... line per successor, legacy_v1_entry=<lineage>); a refusal for a wrong binding prints the exact one, so the human reads and approves exactly what moves.
--maintainer-authorization is exactly these final eight lines, joined by LF, with no trailing newline:
%s`, cwd, reviewtransaction.CompactAbandonReasonOperatorDisposition, reviewtransaction.CompactAbandonReasonRetiredSchema,
		strings.NewReplacer("captured_lens_results=", "captured_lens_results=<entries[].discarded_work.captured_lens_results comma-joined with \",\" in listed order>", "findings_present=false", "findings_present=<entries[].discarded_work.findings_present>", "evidence_records_present=false", "evidence_records_present=<entries[].discarded_work.evidence_records_present>").Replace(
			reviewtransaction.RenderCompactAbandonAuthorization("<--lineage>", "<--expected-revision>", "<entries[].snapshot_identity>", "<--actor>", "<--reason>", reviewtransaction.CompactDiscardedWorkSummary{})))
}

func RunReviewAbandon(args []string, stdout io.Writer) error {
	flags := newReviewFlagSet("review abandon", stdout, "Declare one review lineage lost and move it to the audited quarantine, bound to the maintainer's exact authorization of what moves. Accepts every state: terminal (approved, escalated, invalidated), unreadable or missing review-state.json, and a legacy-v1 entry. Recovery successors of the lineage are abandoned with it, deepest first, so no orphan recovery edge remains; ancestors and unrelated lineages are never touched. Nothing is deleted. The authorization binding names the lineage and every successor and legacy-v1 entry that moves with it. The v2 store lock is taken without waiting: a live holder fails the command with the lock-busy error and nothing moves. An exact replay of a committed abandonment converges idempotently. On partial failure the prepared audit record JSON is still emitted to stdout and the command exits non-zero.")
	cwd := flags.String("cwd", ".", "repository path")
	lineage := flags.String("lineage", "", "compact store lineage to abandon")
	expected := flags.String("expected-revision", "", "exact current authority revision")
	reason := flags.String("reason", "", "abandonment reason: operator_disposition or retired_schema")
	actor := flags.String("actor", "", "abandonment actor")
	authorization := flags.String("maintainer-authorization", "", "exact LF-only v2 binding (eight lines, plus one line per extra entry that moves); run review abandon with no flags to print the template and where every value is read")
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
			return reviewAbandonInputsRefusal(*cwd)
		}
	}
	root, err := resolveReviewOperationRoot(context.Background(), *cwd, reviewtransaction.RDDOperationAbandon)
	if err != nil {
		return withPlainReviewContinuation(fmt.Errorf("resolve review repository root: %w", err))
	}
	result, err := reviewtransaction.AbandonCompactStore(context.Background(), root, reviewtransaction.CompactAbandonRequest{
		LineageID: *lineage, ExpectedRevision: *expected,
		Reason: *reason, Actor: *actor, MaintainerAuthorization: *authorization,
	})
	if err != nil {
		// A partial abandonment persisted the prepared audit record and may have
		// moved the entry; surface the quarantine location for reconciliation.
		if output := (ReviewAbandonResult{Operation: "review/abandon", Record: result.CompactReclaimRecord, Cascaded: result.Cascaded}); result.QuarantinePath != "" || len(result.Cascaded) != 0 {
			_ = encodeReviewJSON(stdout, output)
		}
		return err
	}
	return encodeReviewJSON(stdout, ReviewAbandonResult{Operation: "review/abandon", Record: result.CompactReclaimRecord, Cascaded: result.Cascaded})
}
