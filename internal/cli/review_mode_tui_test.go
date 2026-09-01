package cli

import (
	"context"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

func TestReviewModeTUIWrappersResolveAndChangeOnlyGlobalMode(t *testing.T) {
	reviewModeHome(t)
	repo := initReviewCLIRepo(t)

	before, err := ReviewModeStatus(context.Background(), repo)
	if err != nil || before.Effective != reviewtransaction.RDDModeOff {
		t.Fatalf("initial status = %#v, %v", before, err)
	}
	updated, err := SetGlobalReviewMode(context.Background(), repo, true)
	if err != nil {
		t.Fatalf("enable global mode: %v", err)
	}
	if updated.Global != reviewtransaction.RDDModeOn || updated.CloneLocal != reviewtransaction.RDDModeUnset || !updated.Enabled() {
		t.Fatalf("global update = %#v", updated)
	}
	after, err := ReviewModeStatus(context.Background(), repo)
	if err != nil || after != updated {
		t.Fatalf("fresh status = %#v, %v; want %#v", after, err, updated)
	}
}
