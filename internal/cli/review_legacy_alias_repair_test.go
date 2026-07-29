package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestReviewRepairLegacyAliasRequiresExactBoundAuthorization(t *testing.T) {
	var output bytes.Buffer
	err := RunReview([]string{"repair-legacy-alias"}, &output)
	if err == nil || !strings.Contains(err.Error(), "requires --lineage, --expected-revision, --diagnostic, --disposition, --reason, --actor, and --maintainer-authorization") {
		t.Fatalf("missing repair input error = %v", err)
	}
}

func TestReviewRepairLegacyAliasHelpDocumentsNarrowContract(t *testing.T) {
	var help bytes.Buffer
	if err := RunReview([]string{"repair-legacy-alias", "--help"}, &help); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"approved historical operation aliases", "exact eight-line LF-only binding", "idempotently"} {
		if !strings.Contains(help.String(), want) {
			t.Fatalf("repair help missing %q: %s", want, help.String())
		}
	}
}
