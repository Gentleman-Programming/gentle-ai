package sddstatus

import "testing"

func TestRetiredSDDReceiptFixturesAreAbsent(t *testing.T) {
	repoRoot := retiredLegacyFixtureRepositoryRoot(t)

	for _, fixture := range []string{
		"internal/reviewtransaction/receipt_ref_test.go",
		"internal/sddstatus/runtime_receipt_test.go",
	} {
		requireRetiredLegacyFixtureAbsent(t, repoRoot, fixture)
	}
}
