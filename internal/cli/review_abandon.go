package cli

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

type ReviewAbandonResult struct {
	Operation string                                 `json:"operation"`
	Record    reviewtransaction.CompactReclaimRecord `json:"record"`
}

// reviewAbandonInputsRefusal names, for every value the gate demands, the one
// command that publishes it and the exact field to read it from, then prints
// the authorization template as its own final six lines so the reader can
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
You choose --actor and --reason; the binding uses them with surrounding whitespace trimmed.
--maintainer-authorization is exactly these final six lines, joined by LF, with no trailing newline:
%s`, cwd, reviewtransaction.RenderCompactAbandonAuthorization(
		"<--lineage>", "<--expected-revision>", "<entries[].snapshot_identity>", "<--actor>", "<--reason>"))
}

func RunReviewAbandon(args []string, stdout io.Writer) error {
	flags := newReviewFlagSet("review abandon", stdout, "Quarantine one pristine compact-v2 review lineage — a reviewing authority that never captured lens results or a pristine invalidated authority — with a persisted audit record carrying the natively re-derived pristineness proof. Pristineness is proven from persisted bytes and store topology, never the live worktree, so stale lineages remain abandonable. Terminal, corrected, artifact-holding, and superseded lineages are refused; an exact replay of a committed abandonment converges idempotently. On partial failure the prepared audit record JSON is still emitted to stdout and the command exits non-zero.")
	cwd := flags.String("cwd", ".", "repository path")
	lineage := flags.String("lineage", "", "pristine compact store lineage to abandon")
	expected := flags.String("expected-revision", "", "exact current authority revision")
	reason := flags.String("reason", "", "non-empty abandonment reason")
	actor := flags.String("actor", "", "abandonment actor")
	authorization := flags.String("maintainer-authorization", "", "exact six-line LF-only binding; run review abandon with no flags to print the template and where every value is read")
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
	root, err := resolveReviewMutationRoot(context.Background(), *cwd)
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
