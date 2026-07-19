package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/gentleman-programming/gentle-ai/internal/reviewtransaction"
)

type ReviewLegacyQuarantineResult struct {
	Operation string                                 `json:"operation"`
	Record    reviewtransaction.CompactReclaimRecord `json:"record"`
}

func RunReviewLegacyQuarantine(args []string, stdout io.Writer) error {
	flags := newReviewFlagSet("review quarantine-legacy", stdout, "Quarantine one structurally intact legacy-v1 lineage whose historical review/freeze-findings event fails semantic replay only because it changed unrelated transaction state. The exact content-addressed history moves whole into audited quarantine and is never rewritten or accepted as valid. Other diagnostics, mixed-store identity, stale revisions, and active ownership are refused. Exact committed retries converge idempotently. On partial failure the prepared audit record JSON is emitted and the command exits non-zero.")
	cwd := flags.String("cwd", ".", "repository path")
	lineage := flags.String("lineage", "", "malformed legacy-v1 lineage to quarantine")
	expected := flags.String("expected-revision", "", "exact current legacy HEAD revision")
	diagnostic := flags.String("diagnostic", "", "exact inventory diagnostic")
	disposition := flags.String("disposition", "", "exact supported quarantine disposition")
	reason := flags.String("reason", "", "non-empty quarantine reason")
	actor := flags.String("actor", "", "quarantine actor")
	authorization := flags.String("maintainer-authorization", "", "exact eight-line LF-only binding: gentle-ai.review-legacy-quarantine-authorization/v1, repository, lineage, revision, diagnostic, disposition, actor, reason")
	if err := parseReviewFlags(flags, args); err != nil {
		return err
	}
	if reviewHelpRequested(args) {
		return nil
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected review quarantine-legacy argument %q", flags.Arg(0))
	}
	for _, required := range []string{*lineage, *expected, *diagnostic, *disposition, *reason, *actor, *authorization} {
		if strings.TrimSpace(required) == "" {
			return errors.New("review quarantine-legacy requires --lineage, --expected-revision, --diagnostic, --disposition, --reason, --actor, and --maintainer-authorization")
		}
	}
	root, err := (reviewtransaction.SnapshotBuilder{Repo: *cwd}).ResolveRepositoryRoot(context.Background())
	if err != nil {
		return fmt.Errorf("resolve review repository root: %w", err)
	}
	record, err := reviewtransaction.QuarantineMalformedLegacyFreeze(context.Background(), root, reviewtransaction.LegacyQuarantineRequest{
		LineageID: *lineage, ExpectedRevision: *expected, ExpectedDiagnostic: *diagnostic,
		Disposition: *disposition, Reason: *reason, Actor: *actor, MaintainerAuthorization: *authorization,
	})
	if err != nil {
		if record.QuarantinePath != "" {
			_ = encodeReviewJSON(stdout, ReviewLegacyQuarantineResult{Operation: "review/quarantine-legacy", Record: record})
		}
		return err
	}
	return encodeReviewJSON(stdout, ReviewLegacyQuarantineResult{Operation: "review/quarantine-legacy", Record: record})
}
