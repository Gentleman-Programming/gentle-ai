package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

type ReviewLegacyFixScopeQuarantineResult struct {
	Operation string                                 `json:"operation"`
	Record    reviewtransaction.CompactReclaimRecord `json:"record"`
}

func RunReviewLegacyFixScopeQuarantine(args []string, stdout io.Writer) error {
	flags := newReviewFlagSet("review quarantine-legacy-fix-scope", stdout, "Quarantine one immutable legacy-v1 ordinary_4r lineage for exactly one authorized anomaly set: complete_fix_scope_expansion, or complete_fix_scope_expansion,validate_fix_alias with its exact historical review/validate-fix event. History is never rewritten or accepted as valid. Exact committed retries converge idempotently.")
	cwd := flags.String("cwd", ".", "repository path")
	lineage := flags.String("lineage", "", "legacy-v1 lineage to quarantine")
	expected := flags.String("expected-revision", "", "exact current legacy HEAD revision")
	diagnostic := flags.String("diagnostic", "", "exact inventory diagnostic")
	disposition := flags.String("disposition", "", "must be quarantine-historical-complete-fix-scope-expansion")
	anomalySet := flags.String("anomaly-set", "", "exact canonical anomaly set")
	aliasRevision := flags.String("validate-fix-alias-revision", "", "exact historical review/validate-fix event revision")
	aliasOperation := flags.String("validate-fix-alias-operation", "", "exact historical alias operation")
	reason := flags.String("reason", "", "non-empty quarantine reason")
	actor := flags.String("actor", "", "quarantine actor")
	authorization := flags.String("maintainer-authorization", "", "exact eleven-line LF-only binding: gentle-ai.review-legacy-fix-scope-quarantine-authorization/v2, repository, lineage, revision, diagnostic, disposition, anomaly_set, validate_fix_alias_revision, validate_fix_alias_operation, actor, reason")
	if err := parseReviewFlags(flags, args); err != nil {
		return err
	}
	if reviewHelpRequested(args) {
		return nil
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected review quarantine-legacy-fix-scope argument %q", flags.Arg(0))
	}
	for _, required := range []string{*lineage, *expected, *diagnostic, *disposition, *anomalySet, *reason, *actor, *authorization} {
		if strings.TrimSpace(required) == "" {
			return errors.New("review quarantine-legacy-fix-scope requires --lineage, --expected-revision, --diagnostic, --disposition, --anomaly-set, --reason, --actor, and --maintainer-authorization")
		}
	}
	root, err := resolveReviewMutationRoot(context.Background(), *cwd)
	if err != nil {
		return fmt.Errorf("resolve review repository root: %w", err)
	}
	record, err := reviewtransaction.QuarantineHistoricalLegacyFixScope(context.Background(), root, reviewtransaction.LegacyFixScopeQuarantineRequest{LineageID: *lineage, ExpectedRevision: *expected, ExpectedDiagnostic: *diagnostic, Disposition: *disposition, ExpectedAnomalySet: *anomalySet, ExpectedValidateFixAliasRevision: *aliasRevision, ExpectedValidateFixAliasOperation: *aliasOperation, Reason: *reason, Actor: *actor, MaintainerAuthorization: *authorization})
	if err != nil {
		if record.QuarantinePath != "" {
			_ = encodeReviewJSON(stdout, ReviewLegacyFixScopeQuarantineResult{Operation: "review/quarantine-legacy-fix-scope", Record: record})
		}
		return err
	}
	return encodeReviewJSON(stdout, ReviewLegacyFixScopeQuarantineResult{Operation: "review/quarantine-legacy-fix-scope", Record: record})
}
