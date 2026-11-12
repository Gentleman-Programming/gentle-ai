package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/reviewtransaction"
)

// TestStatusReportsUnbuildableCorrectionRequestTruthfully pins issue #6086
// step 1 end to end. A mode-only plus golden-only candidate counts zero
// changed lines, so its authority freezes a zero correction budget that
// CompactState.Validate accepts but the correction-plan request contract
// refuses. STATUS must name that stop for what it is, not as corrupted
// authority, and must not move the lineage.
func TestStatusReportsUnbuildableCorrectionRequestTruthfully(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the mode-only half of the candidate needs a filesystem executable bit")
	}
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	writeReviewStartCandidate(t, repo, "candidate.go", "package candidate\n\nfunc value() int { return 1 }\n", 0o644)
	writeReviewStartCandidate(t, repo, "testdata/golden/output.golden", "before\n", 0o644)
	runReviewCLIGit(t, repo, "commit", "-qm", "add candidate")
	// A mode-only executable change keeps the tier above passive content,
	// and a regenerated golden carries the changed hunk the finding cites;
	// both count zero lines.
	if err := os.Chmod(filepath.Join(repo, "candidate.go"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "testdata", "golden", "output.golden"), []byte("after\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	started := runNegotiatedReviewStart(t, repo, "correction-request-unavailable")
	resultPaths := make([]string, 0, len(started.SelectedLenses))
	for index, lens := range started.SelectedLenses {
		findings := []facadeFinding{}
		if index == 0 {
			findings = []facadeFinding{{
				Location: "testdata/golden/output.golden:1", Severity: "CRITICAL", Claim: "candidate records the wrong output",
				ProofRefs:     []string{"exact changed hunk", "reproduced candidate failure"},
				EvidenceClass: reviewtransaction.EvidenceDeterministic, CausalDisposition: reviewtransaction.CausalIntroduced,
			}}
		}
		resultPath := filepath.Join(t.TempDir(), fmt.Sprintf("result-%d.json", index))
		writeReviewCLIJSON(t, resultPath, facadeReviewerResult{Lens: lens, Findings: findings, Evidence: []string{"inspected the exact frozen candidate"}})
		resultPaths = append(resultPaths, resultPath)
	}
	if err := captureReviewCLIResultFiles(t, repo, started.LineageID, resultPaths); err != nil {
		t.Fatal(err)
	}
	corroborateRefuterClaimsForTest(t, repo, started.LineageID)

	store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, started.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	before, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if before.State.State != reviewtransaction.StateCorrectionRequired || before.State.CorrectionBudget != 0 || before.State.Validate() != nil {
		t.Fatalf("fixture authority = state %q budget %d, want valid correction_required with a zero budget", before.State.State, before.State.CorrectionBudget)
	}
	if _, buildErr := reviewtransaction.BuildCorrectionPlanRequest(before.State, before.State.CapturePhaseRevision); !errors.Is(buildErr, reviewtransaction.ErrCorrectionPlanRequestUnbuildable) {
		t.Fatalf("fixture builder error = %v, want an unbuildable correction request", buildErr)
	}

	status := negotiatedReviewStatusForLineage(t, repo, started.LineageID)
	transition, err := json.Marshal(status.NextTransition)
	if err != nil {
		t.Fatal(err)
	}
	if want := `{"kind":"stop","reason_code":"correction_request_unavailable"}`; string(transition) != want {
		t.Fatalf("next_transition = %s, want %s", transition, want)
	}
	if status.Forecast == nil || status.Forecast.Horizon != ForecastHorizonTerminal || len(status.Forecast.Steps) != 1 ||
		status.Forecast.Steps[0].ReasonCode != "correction_request_unavailable" || status.ValidationRequest != nil {
		t.Fatalf("forecast = %#v validation_request = %#v", status.Forecast, status.ValidationRequest)
	}
	// The stop invents no route: the authority is not repairable, and no
	// recovery or other action is offered beside the stop.
	if status.Repair.Status != reviewtransaction.AuthorityRepairUnsupported || status.Repair.Candidate != nil || status.Disposition != nil {
		t.Fatalf("repair = %#v disposition = %#v, want the unsupported assessment every non-repairable authority reports", status.Repair, status.Disposition)
	}
	after, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if after.Revision != before.Revision {
		t.Fatalf("read-only STATUS moved the authority from %q to %q", before.Revision, after.Revision)
	}
}

// TestCorrectionRequiredStopNamesOnlyAnUnbuildableRequest pins the routing
// split: only a builder refusal classified as an unbuildable request becomes
// correction_request_unavailable; every other missing request, with or without
// an accepted correction forecast, stays corrupted_or_unverifiable_authority.
func TestCorrectionRequiredStopNamesOnlyAnUnbuildableRequest(t *testing.T) {
	t.Parallel()

	unbuildable := fmt.Errorf("build: %w", reviewtransaction.ErrCorrectionPlanRequestUnbuildable)
	for _, forecasted := range []bool{false, true} {
		for _, tt := range []struct {
			name string
			err  error
			want string
		}{
			{name: "unbuildable request", err: unbuildable, want: "correction_request_unavailable"},
			{name: "authority refusal", err: errors.New("correction plan request findings are not satisfied by admitted compact authority"), want: "corrupted_or_unverifiable_authority"},
			{name: "no recorded cause", err: nil, want: "corrupted_or_unverifiable_authority"},
		} {
			t.Run(fmt.Sprintf("%s forecasted=%v", tt.name, forecasted), func(t *testing.T) {
				status := ReviewTargetStatusResult{
					Applicability:  reviewtransaction.TargetApplicabilityCurrent,
					TargetIdentity: "sha256:" + strings.Repeat("b", 64),
					Authority:      &ReviewTargetStatusAuthority{LineageID: "correction-request-split", Revision: "sha256:" + strings.Repeat("a", 64), State: reviewtransaction.StateCorrectionRequired},
				}
				status.repositoryRoot = "/review-next-transition-repo"
				got := newReviewNextTransition(status, nil, nil, tt.err, reviewNextTransitionInput{CorrectionForecasted: forecasted})
				if got != reviewStopTransition(tt.want) {
					t.Fatalf("next transition = %#v, want stop %q", got, tt.want)
				}
				if forecast := newReviewForecast(got); forecast.Horizon != ForecastHorizonTerminal {
					t.Fatalf("forecast horizon = %q, want terminal", forecast.Horizon)
				}
			})
		}
	}
}
