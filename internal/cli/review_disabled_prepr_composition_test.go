package cli

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

// reviewPrePRHistoricalAuthorityRepo builds the exact shape issue-1878 reports:
// a repository that already carries compact review authority from earlier work,
// plus a later commit no receipt covers. Selector-free pre-PR reaches compact
// chain composition on this shape, which is what makes it the useful fixture
// for both halves of the kill-switch decision.
func reviewPrePRHistoricalAuthorityRepo(t *testing.T) string {
	t.Helper()
	repo := initReviewCLIRepo(t)
	branch := strings.TrimSpace(runReviewCLIGit(t, repo, "symbolic-ref", "--short", "HEAD"))
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("published base\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runReviewCLIGit(t, repo, "add", "tracked.txt")
	runReviewCLIGit(t, repo, "commit", "-qm", "published base")
	// The remote is pinned here, so pre-PR derives its publication base from
	// this commit.
	configureCLIReviewPublicationRemote(t, repo, branch)

	// Two reviewed lineages. Composition is only attempted from two compact
	// stores upward (compact_chain.go: len(stores) < 2 is not applicable), so a
	// single receipt would never exercise the path this issue is about.
	for _, step := range []string{"first reviewed slice", "second reviewed slice"} {
		base := strings.TrimSpace(runReviewCLIGit(t, repo, "rev-parse", "HEAD"))
		if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte(step+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		runReviewCLIGit(t, repo, "add", "tracked.txt")
		runReviewCLIGit(t, repo, "commit", "-qm", step)
		finalizeFacadeReviewForRepo(t, repo, "--base-ref", base, "--committed-only")
	}
	return repo
}

// TestReviewValidateReportsDisabledUnmanagedDeliveryAtPrePRWithHistoricalAuthority
// closes issue-1878. pre-PR was the one gate that vetoed delivery while the
// user's kill switch was off: selector-free composition ran before the disabled
// branch, so an `invalid-chain` denial returned before `disabled/unmanaged`
// could ever be reported. pre-commit and pre-push on this same repository
// already report; pre-PR must now agree with them.
func TestReviewValidateReportsDisabledUnmanagedDeliveryAtPrePRWithHistoricalAuthority(t *testing.T) {
	reviewModeHome(t)
	repo := reviewPrePRHistoricalAuthorityRepo(t)

	disableReviewForClone(t, repo)

	// Work authored and committed while disabled. No receipt can cover it, so
	// the composed chain cannot reach HEAD — the exact denial the user hits.
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("work authored while disabled\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runReviewCLIGit(t, repo, "add", "tracked.txt")
	runReviewCLIGit(t, repo, "commit", "-qm", "authored while disabled")

	var output bytes.Buffer
	err := RunReviewFacadeValidate([]string{"--cwd", repo, "--gate", string(reviewtransaction.GatePrePR)}, &output)
	// The gate reports; it does not veto.
	if err != nil {
		t.Fatalf("disabled pre-PR vetoed delivery instead of reporting it: %v\n%s", err, output.String())
	}
	var result ReviewValidateResult
	decodeStrictReviewJSON(t, output.Bytes(), &result)
	if result.Schema != ReviewValidateSchema {
		t.Fatalf("disabled pre-PR left the typed gate schema = %q", result.Schema)
	}
	if result.Delivery != reviewtransaction.RDDDeliveryDisabledUnmanaged {
		t.Fatalf("disabled pre-PR delivery = %q, want %q", result.Delivery, reviewtransaction.RDDDeliveryDisabledUnmanaged)
	}
	// Unmanaged by choice is neither an approval nor a fault.
	if result.Allowed || result.Result == reviewtransaction.GateAllow {
		t.Fatalf("disabled pre-PR fabricated an approval: %#v", result)
	}
	var denied ReviewGateDeniedError
	if errors.As(err, &denied) {
		t.Fatalf("disabled pre-PR was reported as a denial: %#v", denied)
	}
	// The defect signature must be gone: composition never ran, so no
	// receipt-composition denial can be what the user is shown.
	if result.Context.Denial != nil && result.Context.Denial.Stage == "receipt-composition" {
		t.Fatalf("disabled pre-PR still entered receipt composition: %#v", result.Context.Denial)
	}
	if strings.Contains(result.Reason, "compact receipt graph contains a cycle") {
		t.Fatalf("disabled pre-PR reported a composition cycle: %q", result.Reason)
	}

	// The report is an observation: replaying returns the same bytes and
	// composition never mutated authority.
	var replay bytes.Buffer
	if err := RunReviewFacadeValidate([]string{"--cwd", repo, "--gate", string(reviewtransaction.GatePrePR)}, &replay); err != nil {
		t.Fatalf("replayed disabled pre-PR gate: %v\n%s", err, replay.String())
	}
	if !bytes.Equal(replay.Bytes(), output.Bytes()) {
		t.Fatalf("disabled pre-PR report is not replay stable:\nfirst:\n%s\nreplay:\n%s", output.String(), replay.String())
	}
}

// TestReviewValidatePrePRKeepsComposingHistoricalAuthorityWhileEnabled is the
// enabled half. The fix must remove composition only while the switch is off:
// with reviews on, the identical repository keeps today's composition, today's
// denial, today's exit status and today's exact field set.
func TestReviewValidatePrePRKeepsComposingHistoricalAuthorityWhileEnabled(t *testing.T) {
	reviewModeHome(t)
	repo := reviewPrePRHistoricalAuthorityRepo(t)

	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("unreviewed work\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runReviewCLIGit(t, repo, "add", "tracked.txt")
	runReviewCLIGit(t, repo, "commit", "-qm", "unreviewed work")

	var output bytes.Buffer
	err := RunReviewFacadeValidate([]string{"--cwd", repo, "--gate", string(reviewtransaction.GatePrePR)}, &output)
	var denied ReviewGateDeniedError
	if !errors.As(err, &denied) {
		t.Fatalf("enabled pre-PR over uncovered work error = %T %v\n%s", err, err, output.String())
	}
	if fields := strictReviewJSONFields(t, output.Bytes()); !reflect.DeepEqual(fields, wantEnabledReviewGateFields) {
		t.Fatalf("enabled pre-PR gate fields = %v, want %v", fields, wantEnabledReviewGateFields)
	}
	var result ReviewValidateResult
	decodeStrictReviewJSON(t, output.Bytes(), &result)
	if result.Delivery != "" {
		t.Fatalf("an enabled switch reported a delivery disposition: %#v", result)
	}
	if result.Allowed || result.Result == reviewtransaction.GateAllow {
		t.Fatalf("enabled pre-PR approved uncovered work: %#v", result)
	}
	// Composition still runs with the switch on: this is the exact denial the
	// disabled half must no longer produce, so asserting it here is what proves
	// the fix removed composition only while reviews are off.
	if result.Context.Denial == nil || result.Context.Denial.Stage != "receipt-composition" ||
		result.Context.Denial.Code != "invalid-chain" {
		t.Fatalf("enabled pre-PR denial = %#v, want receipt-composition/invalid-chain", result.Context.Denial)
	}
}
