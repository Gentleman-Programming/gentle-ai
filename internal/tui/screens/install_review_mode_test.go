package screens

import (
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
		"frozen change candidate",
		"additional review time and potential model cost",
		"RDD ON",
		"RDD OFF",
		"global setting",
		"clone-local overrides",
		"does not authorize commits, pushes, pull requests, or releases",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("RenderInstallReviewMode() missing %q:\n%s", want, out)
		}
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
