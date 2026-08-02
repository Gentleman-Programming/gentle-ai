package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

func TestNegotiatedCorrectionPlanningExposesProviderOwnedFindings(t *testing.T) {
	for _, tt := range []struct {
		name             string
		path             string
		content          string
		forecast         int
		wantRisk         reviewtransaction.RiskLevel
		wantKind         string
		wantReason       string
		wantBudget       int
		wantSelectedLens int
	}{
		{
			name: "medium risk before correction forecast", path: "candidate.go",
			content: "package candidate\n\nfunc value() int { return 1 }\n", wantRisk: reviewtransaction.RiskMedium,
			wantKind: reviewNextTransitionCollect, wantReason: "correction_plan_required", wantSelectedLens: 1,
		},
		{
			name: "high risk after accepted correction forecast", path: "service-token.ts",
			content: strings.Repeat("export const candidate = 1;\n", 400), forecast: 120,
			wantRisk: reviewtransaction.RiskHigh, wantKind: reviewNextTransitionStop,
			wantReason: "corrected_candidate_unavailable", wantBudget: 200, wantSelectedLens: 4,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			repo := initReviewCLIRepo(t)
			if err := os.WriteFile(filepath.Join(repo, tt.path), []byte(tt.content), 0o644); err != nil {
				t.Fatal(err)
			}
			started := runNegotiatedReviewStart(t, repo, "correction-findings-"+strings.ReplaceAll(tt.name, " ", "-"))
			if started.RiskLevel != tt.wantRisk || len(started.SelectedLenses) != tt.wantSelectedLens {
				t.Fatalf("review routing = risk %q lenses %v", started.RiskLevel, started.SelectedLenses)
			}

			finalizeArgs := []string{"--cwd", repo, "--lineage", started.LineageID}
			for index, lens := range started.SelectedLenses {
				resultPath := filepath.Join(t.TempDir(), fmt.Sprintf("result-%d.json", index))
				findings := []facadeFinding{}
				if index == 0 {
					findings = []facadeFinding{{
						Location: tt.path + ":1", Severity: "CRITICAL", Claim: "candidate exposes the wrong behavior",
						ProofRefs:     []string{"exact changed hunk", "reproduced candidate failure"},
						EvidenceClass: reviewtransaction.EvidenceDeterministic, CausalDisposition: reviewtransaction.CausalIntroduced,
					}}
				}
				writeReviewCLIJSON(t, resultPath, facadeReviewerResult{
					Lens: lens, Findings: findings, Evidence: []string{"inspected the exact frozen candidate"},
				})
				finalizeArgs = append(finalizeArgs, "--result", resultPath)
			}
			if err := finalizeReviewCLIArgs(t, repo, finalizeArgs, &bytes.Buffer{}); err != nil {
				t.Fatal(err)
			}
			if tt.forecast > 0 {
				if err := RunReviewFacadeFinalize([]string{
					"--cwd", repo, "--lineage", started.LineageID, "--correction-lines", fmt.Sprint(tt.forecast),
				}, &bytes.Buffer{}); err != nil {
					t.Fatal(err)
				}
			}

			store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, started.LineageID)
			if err != nil {
				t.Fatal(err)
			}
			record, err := store.Load()
			if err != nil {
				t.Fatal(err)
			}
			if tt.wantBudget != 0 && record.State.CorrectionBudget != tt.wantBudget {
				t.Fatalf("correction budget = %d, want %d", record.State.CorrectionBudget, tt.wantBudget)
			}
			before, err := os.ReadFile(store.StatePath())
			if err != nil {
				t.Fatal(err)
			}

			args := []string{
				"status", "--cwd", repo, "--contract", ReviewIntegrationContractV2, "--agent", "claude-code",
				"--next-transition", "--lineage", started.LineageID,
			}
			var first, restarted bytes.Buffer
			if err := RunReview(args, &first); err != nil {
				t.Fatal(err)
			}
			if err := RunReview(args, &restarted); err != nil {
				t.Fatal(err)
			}
			var status ReviewTargetStatusResult
			decodeStrictReviewJSON(t, first.Bytes(), &status)
			if err := status.Validate(); err != nil {
				t.Fatal(err)
			}
			transition := status.NextTransition
			if transition == nil || transition.Kind != tt.wantKind || transition.ReasonCode != tt.wantReason || transition.CorrectionRequest == nil {
				t.Fatalf("correction transition = %#v", transition)
			}
			request := transition.CorrectionRequest
			classification := record.State.Classifications[record.State.FixFindingIDs[0]]
			if request.LineageID != record.State.LineageID || request.ExpectedRevision != record.Revision ||
				request.TargetIdentity != record.State.CurrentSnapshot.Identity || request.CorrectionBudget != record.State.CorrectionBudget ||
				!reflect.DeepEqual(request.FixFindingIDs, record.State.FixFindingIDs) || len(request.Findings) != 1 ||
				request.Findings[0].ID != record.State.FixFindingIDs[0] || request.Findings[0].Location != tt.path+":1" ||
				request.Findings[0].Claim != "candidate exposes the wrong behavior" || request.Findings[0].Evidence != classification.Proof ||
				request.Findings[0].EvidenceClass != classification.Class || request.Findings[0].CausalDisposition != classification.Causality ||
				reviewtransaction.ValidateCorrectionPlanRequest(*request) != nil {
				t.Fatalf("correction request = %#v", request)
			}
			tampered := *request
			tampered.Findings = append([]reviewtransaction.CorrectionPlanFinding(nil), request.Findings...)
			tampered.Findings[0].Claim = "different claim"
			if reviewtransaction.ValidateCorrectionPlanRequest(tampered) == nil {
				t.Fatal("content-bound correction request accepted a changed finding")
			}
			if !bytes.Equal(first.Bytes(), restarted.Bytes()) {
				t.Fatalf("restarted correction request changed:\nfirst=%s\nrestarted=%s", first.String(), restarted.String())
			}
			transitionPayload, err := json.Marshal(transition)
			if err != nil {
				t.Fatal(err)
			}
			validateAgainstPublishedNextTransitionSchemaV4(t, transitionPayload)
			after, err := os.ReadFile(store.StatePath())
			if err != nil || !bytes.Equal(before, after) || len(record.State.CorrectionAttempts) != 0 || record.State.CumulativeCorrectionLines != 0 {
				t.Fatalf("read-only correction request consumed authority or budget: %v", err)
			}
		})
	}
}
