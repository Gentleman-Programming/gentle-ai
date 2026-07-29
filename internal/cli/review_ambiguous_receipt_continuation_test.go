package cli

import (
	"strings"
	"testing"
)

// A tester with reviews switched OFF hit this through a pre-commit hook and
// had to delete the hook to commit at all, because the refusal named nothing
// they could run. The sibling branch of this same switch already lists the
// contributing lineages; this one named neither them nor the flag that selects
// one, even though `review validate --lineage <id>` is exactly that selection.
func TestAmbiguousReceiptRefusalNamesHowToSelectATarget(t *testing.T) {
	err := &ReviewReceiptDiscoveryError{
		Kind:       ReviewReceiptAmbiguous,
		Candidates: []string{"review-aaa1", "review-bbb2"},
	}
	message := err.Error()

	if !strings.Contains(message, "--lineage") {
		t.Errorf("refusal names no way to select a target.\n got: %s", message)
	}
	for _, candidate := range []string{"review-aaa1", "review-bbb2"} {
		if !strings.Contains(message, candidate) {
			t.Errorf("refusal does not list candidate %s.\n got: %s", candidate, message)
		}
	}
}
