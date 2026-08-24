package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

func TestNegotiatedCorrectionPlanningExposesProviderOwnedFindings(t *testing.T) {
	// Not parallel: opting in writes the user's global mode through t.Setenv,
	// which Go forbids in a test that also calls t.Parallel.
	reviewEnabledHome(t)

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
			writeReviewStartCandidate(t, repo, tt.path, tt.content, 0o644)
			started := runNegotiatedReviewStart(t, repo, "correction-findings-"+strings.ReplaceAll(tt.name, " ", "-"))
			if started.RiskLevel != tt.wantRisk || len(started.SelectedLenses) != tt.wantSelectedLens {
				t.Fatalf("review routing = risk %q lenses %v", started.RiskLevel, started.SelectedLenses)
			}

			resultPaths := make([]string, 0, len(started.SelectedLenses))
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
				resultPaths = append(resultPaths, resultPath)
			}
			if err := captureReviewCLIResultFiles(t, repo, started.LineageID, resultPaths); err != nil {
				t.Fatal(err)
			}
			if tt.forecast > 0 {
				captureCorrectionPlanFromCurrentStatus(t, repo, started.LineageID, tt.forecast)
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
				"status", "--cwd", repo, "--contract", ReviewIntegrationContractV2,
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
			validateAgainstPublishedNextTransitionSchemaV5(t, transitionPayload)
			after, err := os.ReadFile(store.StatePath())
			if err != nil || !bytes.Equal(before, after) || len(record.State.CorrectionAttempts) != 0 || record.State.CumulativeCorrectionLines != 0 {
				t.Fatalf("read-only correction request consumed authority or budget: %v", err)
			}
		})
	}
}

func captureCorrectionPlanFromCurrentStatus(t *testing.T, cwd, lineage string, correctionLines int) {
	t.Helper()
	root, err := (reviewtransaction.SnapshotBuilder{Repo: cwd}).ResolveRepositoryRoot(context.Background())
	if err != nil {
		t.Fatalf("resolve correction repository root for STATUS: %v", err)
	}
	_, record, err := discoverCompactFacadeReview(context.Background(), root, lineage, false)
	if err != nil {
		t.Fatalf("discover correction authority for STATUS: %v", err)
	}
	args := []string{
		"status", "--cwd", cwd, "--contract", ReviewIntegrationContractV2,
		"--next-transition", "--lineage", lineage,
	}
	switch record.State.InitialSnapshot.Kind {
	case reviewtransaction.TargetCurrentChanges:
		if record.State.InitialSnapshot.Projection == reviewtransaction.ProjectionStaged {
			args = append(args, "--projection", string(reviewtransaction.ProjectionStaged))
		}
	case reviewtransaction.TargetBaseDiff:
		args = append(args, "--base-ref", record.State.InitialSnapshot.BaseTree, "--committed-only")
	case reviewtransaction.TargetBaseWorkspaceOverlay:
		args = append(args, "--base-ref", record.State.InitialSnapshot.BaseTree, "--workspace-overlay")
		if record.State.InitialSnapshot.Projection == reviewtransaction.ProjectionStaged {
			args = append(args, "--projection", string(reviewtransaction.ProjectionStaged))
		}
	default:
		t.Fatalf("unsupported correction authority target kind %q", record.State.InitialSnapshot.Kind)
	}
	builder := reviewtransaction.SnapshotBuilder{Repo: root}
	inventory, inventoryDigest, err := builder.IntendedUntrackedInventory(context.Background())
	if err != nil {
		t.Fatalf("discover correction untracked inventory for STATUS: %v", err)
	}
	if len(inventory) > 0 {
		mode := "exclude"
		if len(record.State.InitialSnapshot.IntendedUntracked) > 0 {
			mode = "select"
		}
		args = append(args, "--untracked-scope", mode, "--expected-untracked-inventory", inventoryDigest)
		for _, path := range record.State.InitialSnapshot.IntendedUntracked {
			args = append(args, "--intended-untracked", path)
		}
	}
	var output bytes.Buffer
	if err := RunReview(args, &output); err != nil {
		t.Fatalf("read correction-plan STATUS: %v\n%s", err, output.String())
	}
	var status ReviewTargetStatusResult
	decodeStrictReviewJSON(t, output.Bytes(), &status)
	if err := status.Validate(); err != nil {
		t.Fatal(err)
	}
	transition := status.NextTransition
	if transition == nil || transition.Kind != reviewNextTransitionCollect || transition.ReasonCode != "correction_plan_required" ||
		transition.CorrectionRequest == nil || transition.Collect == nil || len(transition.Collect.Inputs) != 1 ||
		transition.Collect.Inputs[0].CaptureOperation != reviewCaptureCorrectionPlanOperation {
		t.Fatalf("correction-plan STATUS = %#v", transition)
	}
	captureArgs := reviewTransitionInputTokens(t, transition.Collect.Inputs[0])
	captureArgs = append(captureArgs, "--correction-lines", strconv.Itoa(correctionLines))
	if err := RunReviewCaptureCorrectionPlan(captureArgs, &bytes.Buffer{}); err != nil {
		t.Fatalf("capture current correction plan: %v", err)
	}
}

func reviewTransitionInputTokens(t *testing.T, input ReviewTransitionInput) []string {
	t.Helper()
	args := make([]string, 0, len(input.Arguments))
	for _, argument := range input.Arguments {
		if argument.Token == "" {
			t.Fatalf("transition argument %q has no exact token", argument.Name)
		}
		args = append(args, argument.Token)
	}
	return args
}
