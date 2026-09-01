package reviewtransaction

import "testing"

// TestReviewerContextLevelIsClosedOnInput pins that callers cannot declare a
// delivery mechanism this release does not implement.
func TestReviewerContextLevelIsClosedOnInput(t *testing.T) {
	tests := []struct {
		name              string
		level             ReviewerContextLevel
		accepted          bool
		declaredNowReason string
	}{
		{name: "provider command", level: ReviewerContextLevelProviderCommand, accepted: true},
		{name: "runtime interception", level: ReviewerContextLevelRuntimeInterception, accepted: true},
		{name: "unknown future mechanism", level: "signed_attestation", accepted: false,
			declaredNowReason: "a level this release cannot produce must not be declarable"},
		{name: "empty", level: "", accepted: false},
		{name: "path shaped", level: "a/b", accepted: false},
		{name: "newline", level: "provider_command\nx", accepted: false},
		{name: "uppercase", level: "Provider_Command", accepted: false},
		{name: "trailing underscore", level: "provider_", accepted: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := ReviewerContextLevelAccepted(test.level); got != test.accepted {
				t.Fatalf("accepted(%q) = %v, want %v (%s)", test.level, got, test.accepted, test.declaredNowReason)
			}
		})
	}
}
