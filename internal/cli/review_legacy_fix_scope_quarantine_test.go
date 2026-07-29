package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestReviewQuarantineLegacyFixScopeContract(t *testing.T) {
	var output bytes.Buffer
	if err := RunReview([]string{"quarantine-legacy-fix-scope"}, &output); err == nil || !strings.Contains(err.Error(), "requires --lineage") {
		t.Fatalf("missing input error = %v", err)
	}
	if err := RunReview([]string{"quarantine-legacy-fix-scope", "--help"}, &output); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"complete_fix_scope_expansion", "validate_fix_alias", "quarantine-historical-complete-fix-scope-expansion", "gentle-ai.review-legacy-fix-scope-quarantine-authorization/v2, repository, lineage, revision, diagnostic, disposition, anomaly_set, validate_fix_alias_revision, validate_fix_alias_operation, actor, reason"} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("help missing %q", want)
		}
	}
}
