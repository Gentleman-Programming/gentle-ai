package reviewtransaction

import "testing"

// TestReviewerContextLevelIsClosedOnInputAndOpenOnRead pins the asymmetry the
// whole forward-compatibility design rests on.
func TestReviewerContextLevelIsClosedOnInputAndOpenOnRead(t *testing.T) {
	tests := []struct {
		name              string
		level             ReviewerContextLevel
		accepted          bool
		wellFormed        bool
		declaredNowReason string
	}{
		{name: "provider command", level: ReviewerContextLevelProviderCommand, accepted: true, wellFormed: true},
		{name: "runtime interception", level: ReviewerContextLevelRuntimeInterception, accepted: true, wellFormed: true},
		{name: "unknown future mechanism", level: "signed_attestation", accepted: false, wellFormed: true,
			declaredNowReason: "a level this release cannot produce must not be declarable, but must stay readable"},
		{name: "empty", level: "", accepted: false, wellFormed: false},
		{name: "path shaped", level: "a/b", accepted: false, wellFormed: false},
		{name: "newline", level: "provider_command\nx", accepted: false, wellFormed: false},
		{name: "uppercase", level: "Provider_Command", accepted: false, wellFormed: false},
		{name: "trailing underscore", level: "provider_", accepted: false, wellFormed: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := ReviewerContextLevelAccepted(test.level); got != test.accepted {
				t.Fatalf("accepted(%q) = %v, want %v (%s)", test.level, got, test.accepted, test.declaredNowReason)
			}
			if got := ReviewerContextLevelWellFormed(test.level); got != test.wellFormed {
				t.Fatalf("wellFormed(%q) = %v, want %v", test.level, got, test.wellFormed)
			}
		})
	}
}
