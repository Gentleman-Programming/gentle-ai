package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/gentleman-programming/gentle-ai/internal/reviewtransaction"
)

type ReviewLegacyAliasRepairResult struct {
	Operation string                                 `json:"operation"`
	Record    reviewtransaction.CompactReclaimRecord `json:"record"`
}

func RunReviewLegacyAliasRepair(args []string, stdout io.Writer) error {
	flags := newReviewFlagSet("review repair-legacy-alias", stdout, "Quarantine one immutable legacy-v1 lineage rejected solely because of approved historical operation aliases. The chain is revalidated against its exact approved historical semantics, moved whole into audited quarantine, and never rewritten or accepted as valid. Unknown aliases, unrelated corruption, mixed authority, stale revisions, and active ownership are refused. Exact committed retries converge idempotently.")
	cwd := flags.String("cwd", ".", "repository path")
	lineage := flags.String("lineage", "", "legacy-v1 lineage to repair by quarantine")
	expected := flags.String("expected-revision", "", "exact current legacy HEAD revision")
	diagnostic := flags.String("diagnostic", "", "exact inventory diagnostic")
	disposition := flags.String("disposition", "", "exact supported quarantine disposition")
	reason := flags.String("reason", "", "non-empty repair reason")
	actor := flags.String("actor", "", "repair actor")
	authorization := flags.String("maintainer-authorization", "", "exact eight-line LF-only binding: gentle-ai.review-legacy-alias-repair-authorization/v1, repository, lineage, revision, diagnostic, disposition, actor, reason")
	if err := parseReviewFlags(flags, args); err != nil {
		return err
	}
	if reviewHelpRequested(args) {
		return nil
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected review repair-legacy-alias argument %q", flags.Arg(0))
	}
	for _, required := range []string{*lineage, *expected, *diagnostic, *disposition, *reason, *actor, *authorization} {
		if strings.TrimSpace(required) == "" {
			return errors.New("review repair-legacy-alias requires --lineage, --expected-revision, --diagnostic, --disposition, --reason, --actor, and --maintainer-authorization")
		}
	}
	root, err := (reviewtransaction.SnapshotBuilder{Repo: *cwd}).ResolveRepositoryRoot(context.Background())
	if err != nil {
		return fmt.Errorf("resolve review repository root: %w", err)
	}
	record, err := reviewtransaction.RepairHistoricalLegacyAlias(context.Background(), root, reviewtransaction.LegacyAliasRepairRequest{
		LineageID: *lineage, ExpectedRevision: *expected, ExpectedDiagnostic: *diagnostic,
		Disposition: *disposition, Reason: *reason, Actor: *actor, MaintainerAuthorization: *authorization,
	})
	if err != nil {
		if record.QuarantinePath != "" {
			_ = encodeReviewJSON(stdout, ReviewLegacyAliasRepairResult{Operation: "review/repair-legacy-alias", Record: record})
		}
		return err
	}
	return encodeReviewJSON(stdout, ReviewLegacyAliasRepairResult{Operation: "review/repair-legacy-alias", Record: record})
}
