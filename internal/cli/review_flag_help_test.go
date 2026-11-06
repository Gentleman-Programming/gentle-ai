package cli

import (
	"strings"
	"testing"
)

// A tester followed the scope-changed recovery, reached `review recover`, and
// stopped: nothing said where the mandatory successor lineage comes from. They
// were right not to invent one -- for all the output told them, it had to be
// obtained rather than chosen. It is caller-minted, and only the code knew.
func TestRecoverSuccessorLineageHelpSaysWhereItComesFrom(t *testing.T) {
	t.Parallel()

	var out strings.Builder
	_ = RunReviewRecover([]string{"--help"}, &out)
	help := out.String()
	if !strings.Contains(help, "--successor-lineage") {
		t.Fatalf("recover help does not document --successor-lineage: %s", help)
	}
	if !strings.Contains(help, "choose") && !strings.Contains(help, "new identifier") {
		t.Fatalf("--successor-lineage help never says the caller mints it: %s", help)
	}
}

// The collect tokens carry --cwd beside --repository-context because the
// context is a digest over that exact repository and binding. The help has to
// say so: a caller who drops --cwd gets a refusal whose cause is the pairing,
// and the maintainer made this exact mistake while writing the guide.
func TestCaptureResultHelpSaysRepositoryContextIsVerifiedAgainstCwd(t *testing.T) {
	t.Parallel()

	var out strings.Builder
	_ = RunReviewCaptureResult([]string{"--help"}, &out)
	help := out.String()
	if !strings.Contains(help, "--repository-context") {
		t.Fatalf("capture-result help omits --repository-context: %s", help)
	}
	for _, want := range []string{"--cwd", "verified against --cwd"} {
		if !strings.Contains(help, want) {
			t.Fatalf("capture-result help never says the context is checked against the repository (missing %q): %s", want, help)
		}
	}
}
