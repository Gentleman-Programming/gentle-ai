package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/gentleman-programming/gentle-ai/internal/reviewtransaction"
)

type ReviewDisposeResultResult struct {
	Operation string                                           `json:"operation"`
	Record    reviewtransaction.CompactResultDispositionRecord `json:"record"`
}

// RunReviewDisposeResult exposes the audited native disposition of one
// preserved reviewer result that cannot be replayed through capture-result.
// It is fail-closed by construction: the preserved bytes are read and
// digest-verified but never rewritten, the candidate-inapplicability evidence
// is re-derived natively rather than trusted, and the only outcome is terminal
// escalation of the lineage with every valid captured result retained.
func RunReviewDisposeResult(args []string, stdout io.Writer) error {
	flags := newReviewFlagSet("review dispose-result", stdout, "Disposition one preserved reviewer result as candidate-inapplicable and terminally escalate its lineage, retaining every valid captured lens result. Binds repository, lineage, target identity, lens, selected order, current authority revision, and preserved-artifact digest; requires natively re-derived inapplicability evidence and an exact maintainer authorization binding. The preserved raw payload is never mutated or deleted, and no path here turns a refused payload into an admitted result. An exact replay of a committed disposition converges idempotently. The supported forward path afterwards is review recover --disposition escalated.")
	cwd := flags.String("cwd", ".", "repository path")
	lineage := flags.String("lineage", "", "exact reviewing lineage identifier")
	expected := flags.String("expected-revision", "", "exact current authority revision")
	target := flags.String("target", "", "exact frozen target identity")
	lens := flags.String("lens", "", "exact selected lens of the preserved result")
	order := flags.Int("order", -1, "zero-based selected lens order of the preserved result")
	digest := flags.String("artifact-digest", "", "sha256 digest of the preserved incident artifact reported by review preserve-result")
	class := flags.String("class", "", "inapplicability class: transport_syntax or wrong_target")
	diagnostic := flags.String("diagnostic", "", "non-empty diagnostic describing why the preserved result cannot be admitted")
	var absent repeatedString
	flags.Var(&absent, "absent-path", "repository-relative path cited by the preserved output and absent from the frozen candidate; required and repeatable for wrong_target")
	reason := flags.String("reason", "", "non-empty disposition reason")
	actor := flags.String("actor", "", "disposition actor")
	authorization := flags.String("maintainer-authorization", "", "exact eleven-line LF-only binding: gentle-ai.review-result-disposition-authorization/v1, repository, lineage, revision, target_identity, lens, order, artifact_digest, class, actor, reason")
	if err := parseReviewFlags(flags, args); err != nil {
		return err
	}
	if reviewHelpRequested(args) {
		return nil
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected review dispose-result argument %q", flags.Arg(0))
	}
	for _, required := range []string{*lineage, *expected, *target, *lens, *digest, *class, *diagnostic, *reason, *actor, *authorization} {
		if strings.TrimSpace(required) == "" {
			return errors.New("review dispose-result requires --lineage, --expected-revision, --target, --lens, --order, --artifact-digest, --class, --diagnostic, --reason, --actor, and --maintainer-authorization")
		}
	}
	if *order < 0 {
		return errors.New("review dispose-result requires a zero-based --order")
	}
	ctx := context.Background()
	root, err := (reviewtransaction.SnapshotBuilder{Repo: *cwd}).ResolveRepositoryRoot(ctx)
	if err != nil {
		return fmt.Errorf("resolve review repository root: %w", err)
	}
	record, err := reviewtransaction.DisposeUnreplayablePreservedResult(ctx, root, reviewtransaction.CompactResultDispositionRequest{
		LineageID: *lineage, ExpectedRevision: *expected, TargetIdentity: *target,
		Lens: *lens, SelectedOrder: *order, ArtifactDigest: *digest,
		Class: reviewtransaction.ResultDispositionClass(*class), Diagnostic: *diagnostic, AbsentPaths: absent,
		Reason: *reason, Actor: *actor, MaintainerAuthorization: *authorization,
	})
	if err != nil {
		return err
	}
	return encodeReviewJSON(stdout, ReviewDisposeResultResult{
		Operation: reviewtransaction.CompactResultDispositionOperation, Record: record,
	})
}
