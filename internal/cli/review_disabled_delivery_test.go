package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

// TestReviewValidateReportsDisabledUnmanagedDeliveryWithoutAuthority keeps the
// disabled delivery contract independent of review receipts: every ordinary
// delivery gate reports disabled/unmanaged, is replay-safe, and never creates
// review authority.
func TestReviewValidateReportsDisabledUnmanagedDeliveryWithoutAuthority(t *testing.T) {
	reviewModeHome(t)
	repo := initReviewCLIRepo(t)
	disableReviewForClone(t, repo)

	for _, gate := range []reviewtransaction.GateKind{
		reviewtransaction.GatePostApply,
		reviewtransaction.GatePreCommit,
	} {
		t.Run(string(gate), func(t *testing.T) {
			var output bytes.Buffer
			err := RunReviewFacadeValidate([]string{"--cwd", repo, "--gate", string(gate)}, &output)
			assertDisabledUnmanagedGate(t, err, output.Bytes())

			var replay bytes.Buffer
			if err := RunReviewFacadeValidate([]string{"--cwd", repo, "--gate", string(gate)}, &replay); err != nil {
				t.Fatalf("replayed disabled %s delivery gate: %v\n%s", gate, err, replay.String())
			}
			if !bytes.Equal(replay.Bytes(), output.Bytes()) {
				t.Fatalf("disabled %s delivery is not replay stable:\nfirst:\n%s\nreplay:\n%s", gate, output.String(), replay.String())
			}
		})
	}

	authorityRoot := filepath.Join(repo, ".git", "gentle-ai", "review-transactions", "v2")
	if _, err := os.Stat(authorityRoot); !os.IsNotExist(err) {
		t.Fatalf("disabled delivery validation created review authority: %v", err)
	}
}

// TestReviewValidateReportsDisabledUnmanagedDeliveryWhenNoUpstreamConfigured
// proves the pre-push gate short-circuits to ordinary delivery before it tries
// to derive a publication boundary. initReviewCLIRepo deliberately configures
// neither a remote nor a branch upstream.
func TestReviewValidateReportsDisabledUnmanagedDeliveryWhenNoUpstreamConfigured(t *testing.T) {
	reviewModeHome(t)
	repo := initReviewCLIRepo(t)
	disableReviewForClone(t, repo)

	var output bytes.Buffer
	err := RunReviewFacadeValidate([]string{"--cwd", repo, "--gate", string(reviewtransaction.GatePrePush)}, &output)
	assertDisabledUnmanagedGate(t, err, output.Bytes())

	var replay bytes.Buffer
	if err := RunReviewFacadeValidate([]string{"--cwd", repo, "--gate", string(reviewtransaction.GatePrePush)}, &replay); err != nil {
		t.Fatalf("replayed disabled pre-push delivery gate without upstream: %v\n%s", err, replay.String())
	}
	if !bytes.Equal(replay.Bytes(), output.Bytes()) {
		t.Fatalf("disabled pre-push delivery without upstream is not replay stable:\nfirst:\n%s\nreplay:\n%s", output.String(), replay.String())
	}

	authorityRoot := filepath.Join(repo, ".git", "gentle-ai", "review-transactions", "v2")
	if _, err := os.Stat(authorityRoot); !os.IsNotExist(err) {
		t.Fatalf("disabled pre-push validation without upstream created review authority: %v", err)
	}
}

func disableReviewForClone(t *testing.T, repo string) {
	t.Helper()
	var output bytes.Buffer
	if err := RunReviewMode([]string{"disable", "--cwd", repo, "--scope", "clone", "--json"}, &output); err != nil {
		t.Fatalf("disable receipt-driven development: %v\n%s", err, output.String())
	}
	if status := decodeReviewModeResult(t, output.Bytes()).Status; status.Effective != reviewtransaction.RDDModeOff {
		t.Fatalf("kill switch did not take effect: %#v", status)
	}
}
