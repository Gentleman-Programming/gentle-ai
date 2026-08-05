package cli

// A kill switch that a corrupted store can override is not a kill switch.
//
// Issue #2270 is the sharpest report in the class: receipt-driven development
// was globally OFF, the operator ran an ordinary `git push`, and the push hook
// blocked because authority inspection had already failed closed over a stored
// record it could not trust. Nothing about that record had anything to do with
// the commit being pushed, and nothing about the operator's choice to turn
// reviews off had been consulted before the inspection refused.
//
// This file pins the shape from both sides. With the switch off, an ordinary
// pre-push over a store holding a damaged historical entry reports ordinary
// unmanaged delivery and exits zero. With the switch on, the same store still
// serves the healthy candidate rather than answering with a repository-wide
// corruption verdict — because the scope of a damaged entry is that entry.

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

// TestDisabledDeliveryIsNeverBlockedByADamagedAuthorityEntry is property 3.
// It runs the damage twice — a forged recovery binding and a record truncated
// mid-write — because the two reach the gate through completely different code
// (a graph-violation verdict and a decode failure) and #2270's own evidence
// names the decode shape.
func TestDisabledDeliveryIsNeverBlockedByADamagedAuthorityEntry(t *testing.T) {
	damages := []struct {
		name   string
		damage func(t *testing.T, repo, lineage string)
	}{
		{"forged recovery binding", func(t *testing.T, repo, lineage string) {
			forgeDamagedStoreRecoveryReason(t, repo, lineage)
		}},
		{"record truncated mid-write", func(t *testing.T, repo, lineage string) {
			truncateDamagedStoreRecord(t, repo, lineage)
		}},
	}
	for _, damage := range damages {
		t.Run(damage.name, func(t *testing.T) {
			reviewModeHome(t)
			repo := initReviewCLIRepo(t)
			branch := strings.TrimSpace(runReviewCLIGit(t, repo, "symbolic-ref", "--short", "HEAD"))
			configureCLIReviewPublicationRemote(t, repo, branch)

			_, successor := mintDamagedStoreRecoveryPair(t, repo)
			damage.damage(t, repo, successor)

			// Ordinary work, committed, with reviews off. This is the operator
			// in #2270: local tests, type-check, lint and commit all passed,
			// and then the push hook refused.
			disableReviewForClone(t, repo)
			if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("ordinary work authored while reviews are off\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			runReviewCLIGit(t, repo, "add", "tracked.txt")
			runReviewCLIGit(t, repo, "commit", "-qm", "ordinary work")

			var output bytes.Buffer
			err := RunReviewFacadeValidate([]string{"--cwd", repo, "--gate", string(reviewtransaction.GatePrePush)}, &output)
			if err != nil {
				t.Fatalf("a damaged historical entry blocked ordinary delivery while the kill switch was off: %v\n%s", err, output.String())
			}
			var result ReviewValidateResult
			decodeStrictReviewJSON(t, output.Bytes(), &result)
			if result.Delivery != reviewtransaction.RDDDeliveryDisabledUnmanaged {
				t.Fatalf("delivery under a disabled switch = %q, want %q\n%s",
					result.Delivery, reviewtransaction.RDDDeliveryDisabledUnmanaged, output.String())
			}
			if result.Context.Denial != nil {
				t.Fatalf("the disabled report carried a denial derived from authority it must never have read: %#v", result.Context.Denial)
			}

			// The damage is still reported, not hidden: the switch changes who
			// is refused, never whether the store tells the truth about itself.
			var inspection bytes.Buffer
			if err := RunReviewInspectAuthority([]string{"--cwd", repo}, &inspection); err != nil {
				t.Fatalf("inspect-authority over a damaged store: %v\n%s", err, inspection.String())
			}
			if !strings.Contains(inspection.String(), successor) {
				t.Fatalf("inspect-authority no longer names the damaged entry:\n%s", inspection.String())
			}

			// The surface #2270 actually blocked on. The push hook asked the
			// harness for an authorization; the harness asks for the next
			// transition; and this call answered that authority inspection was
			// unavailable, with the kill switch never consulted on the way. A
			// switched-off system has no implications, so it must route the
			// live target the same as it would with no store at all.
			var status bytes.Buffer
			if err := RunReviewStatus([]string{"--cwd", repo, "--contract", ReviewIntegrationContractV2, "--next-transition"}, &status); err != nil {
				t.Fatalf("negotiated status was unavailable over a damaged entry while reviews were off: %v\n%s", err, status.String())
			}
			var routed struct {
				Applicability  string `json:"applicability"`
				NextTransition *struct {
					Kind       string `json:"kind"`
					ReasonCode string `json:"reason_code"`
				} `json:"next_transition"`
			}
			if err := json.Unmarshal(status.Bytes(), &routed); err != nil {
				t.Fatalf("parse negotiated status: %v\n%s", err, status.String())
			}
			if routed.Applicability == string(reviewtransaction.TargetApplicabilityCorrupted) {
				t.Fatalf("a damaged entry made an unrelated target report corrupted authority while reviews were off:\n%s", status.String())
			}
		})
	}
}

// TestEnabledDeliveryOverADamagedEntryStillServesTheHealthyCandidate is the
// other half, and the one that proves the disabled result above is not merely
// the kill switch hiding a repository-wide refusal. With reviews ON, a store
// holding one damaged historical entry must still be able to start, finalize
// and validate a review of unrelated work.
//
// This is the shape #2167 reported (a documentation-only pre-commit target
// denied `authority_corrupted` against unrelated historical authority) and the
// shape #1892 and #2014 reported for `review start`.
func TestEnabledDeliveryOverADamagedEntryStillServesTheHealthyCandidate(t *testing.T) {
	// Two damages that reach the gate through completely different doors. A
	// forged binding leaves a readable record carrying an invalid edge, so the
	// graph reports it. A truncated record states nothing at all, so it is
	// absent from the graph and reaches the gate through supersession, which
	// used to sweep every entry and fail on the first one it could not read.
	// Both must end in the same place: blocked for itself, invisible to
	// everybody else.
	damages := []struct {
		name   string
		damage func(t *testing.T, repo, lineage string)
	}{
		{"forged recovery binding", func(t *testing.T, repo, lineage string) {
			forgeDamagedStoreRecoveryReason(t, repo, lineage)
		}},
		{"record truncated mid-write", func(t *testing.T, repo, lineage string) {
			truncateDamagedStoreRecord(t, repo, lineage)
		}},
	}
	for _, damage := range damages {
		t.Run(damage.name, func(t *testing.T) {
			reviewModeHome(t)
			repo := initReviewCLIRepo(t)

			_, successor := mintDamagedStoreRecoveryPair(t, repo)
			damage.damage(t, repo, successor)

			// Unrelated new work, reviewed from scratch with the damaged entry
			// still sitting in the same store.
			if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("unrelated new behavior\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			finalizeFacadeReviewForRepo(t, repo)
			runReviewCLIGit(t, repo, "add", "tracked.txt")

			var output bytes.Buffer
			if err := RunReviewFacadeValidate([]string{"--cwd", repo, "--gate", string(reviewtransaction.GatePreCommit)}, &output); err != nil {
				t.Fatalf("an unrelated damaged entry denied a healthy reviewed candidate: %v\n%s", err, output.String())
			}
			var result ReviewValidateResult
			decodeStrictReviewJSON(t, output.Bytes(), &result)
			if !result.Allowed || result.Result != reviewtransaction.GateAllow {
				t.Fatalf("healthy reviewed candidate over a damaged store = %#v\n%s", result, output.String())
			}

			// The damaged entry never becomes governable, and says so by name.
			blocked := reviewtransaction.CompactAuthorityLineageBlocked(context.Background(), repo, successor)
			if blocked == nil {
				t.Fatalf("the damaged successor %q reports no block of its own", successor)
			}
			if !strings.Contains(blocked.Error(), successor) {
				t.Fatalf("the block does not name the entry it refuses: %v", blocked)
			}
			if !strings.Contains(blocked.Error(), "gentle-ai review inspect-authority") {
				t.Fatalf("the block names no runnable diagnosis: %v", blocked)
			}
		})
	}
}

// TestNegotiatedStatusOverADamagedEntryStillRoutesTheHealthyTarget pins the
// surface #2234, #2270 and #2456 all name: the negotiated status envelope the
// agent harness drives before every delivery. It reported
// `native-status-unavailable` with `inventory_complete: false` because one
// entry could not be read, which left the harness with no transition to take
// and the operator with a blocked push and no exit.
func TestNegotiatedStatusOverADamagedEntryStillRoutesTheHealthyTarget(t *testing.T) {
	reviewModeHome(t)
	repo := initReviewCLIRepo(t)

	_, successor := mintDamagedStoreRecoveryPair(t, repo)
	truncateDamagedStoreRecord(t, repo, successor)

	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("unrelated target while an entry is unreadable\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var output bytes.Buffer
	if err := RunReviewStatus([]string{"--cwd", repo, "--contract", ReviewIntegrationContractV2, "--next-transition"}, &output); err != nil {
		t.Fatalf("negotiated status was unavailable because one unrelated entry could not be read: %v\n%s", err, output.String())
	}
	var envelope struct {
		Applicability  string `json:"applicability"`
		NextTransition *struct {
			Kind       string `json:"kind"`
			ReasonCode string `json:"reason_code"`
		} `json:"next_transition"`
	}
	if err := json.Unmarshal(output.Bytes(), &envelope); err != nil {
		t.Fatalf("parse negotiated status: %v\n%s", err, output.String())
	}
	if envelope.NextTransition == nil {
		t.Fatalf("negotiated status published no transition at all:\n%s", output.String())
	}
	if envelope.Applicability == string(reviewtransaction.TargetApplicabilityCorrupted) {
		t.Fatalf("one unreadable entry made an unrelated target report corrupted authority:\n%s", output.String())
	}
	if envelope.NextTransition.Kind == "stop" {
		t.Fatalf("negotiated status stopped an unrelated target over an unreadable entry: reason=%q\n%s",
			envelope.NextTransition.ReasonCode, output.String())
	}
}
