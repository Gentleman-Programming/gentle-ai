package screens

import (
	"slices"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

func TestRenderInstallReviewModeExplainsChoiceAndGlobalScope(t *testing.T) {
	out := RenderInstallReviewMode(reviewtransaction.RDDModeStatus{
		Schema: reviewtransaction.RDDModeStatusSchema,
		Global: reviewtransaction.RDDModeUnset,
	}, nil, 1)

	for _, want := range []string{
		"RDD adds an independent review of your code changes to help catch bugs and regressions before they reach your project.",
		"It records review findings and verifies corrections, helping you understand what was checked and build confidence in your changes.",
		"Would you like to enable RDD?",
		"Your choice applies globally after installation succeeds. Existing project-specific settings are preserved.",
		"Enable RDD",
		"Disable RDD",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("RenderInstallReviewMode() missing %q:\n%s", want, out)
		}
	}
}

func TestInstallReviewModeOptionsUseApprovedLabels(t *testing.T) {
	if got, want := InstallReviewModeOptions(nil), []string{"Enable RDD", "Disable RDD", "Back"}; !slices.Equal(got, want) {
		t.Fatalf("InstallReviewModeOptions() = %v, want %v", got, want)
	}
}

func TestInstallReviewModeStatusLabelsRemainTruthful(t *testing.T) {
	tests := []struct {
		name   string
		global reviewtransaction.RDDMode
		want   string
	}{
		{name: "on", global: reviewtransaction.RDDModeOn, want: "RDD is currently ON."},
		{name: "off", global: reviewtransaction.RDDModeOff, want: "RDD is currently OFF."},
		{name: "unset", global: reviewtransaction.RDDModeUnset, want: "No global RDD preference is configured. RDD defaults to OFF until you explicitly choose otherwise."},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := installReviewModeStatusLabel(reviewtransaction.RDDModeStatus{Global: tt.global}); got != tt.want {
				t.Fatalf("installReviewModeStatusLabel() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestInstallReviewModeOptionsFailClosedOnStatusError(t *testing.T) {
	options := InstallReviewModeOptions(assertiveError{})
	if len(options) != 1 || options[0] != "Back" {
		t.Fatalf("InstallReviewModeOptions() = %v, want only Back when status is unreadable", options)
	}
}

type assertiveError struct{}

func (assertiveError) Error() string { return "cannot read configured RDD mode" }
