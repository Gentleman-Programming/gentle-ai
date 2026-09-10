package cli

import (
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

func descriptorTestSHA(char string) string { return "sha256:" + strings.Repeat(char, 64) }

func TestCorrectionPlanDescriptorBindsOnlyCurrentCapture(t *testing.T) {
	binding := ReviewTransitionBinding{
		LineageID: "descriptor-correction", Revision: descriptorTestSHA("a"), TargetIdentity: descriptorTestSHA("b"),
		RepositoryContext: "rctx1_" + strings.Repeat("c", 64),
	}
	request := reviewtransaction.CorrectionPlanRequest{
		LineageID: binding.LineageID, ExpectedRevision: binding.Revision, TargetIdentity: binding.TargetIdentity,
		RequestHash: descriptorTestSHA("d"), CorrectionBudget: 7,
	}
	descriptor := reviewCorrectionPlanSubmission(ReviewIntegrationContractV2, binding, request)
	if descriptor == nil || descriptor.OperationToken != "capture-correction-plan" || descriptor.Value == nil ||
		descriptor.Value.Slot != "correction_lines" || descriptor.Value.Maximum != request.CorrectionBudget {
		t.Fatalf("correction-plan descriptor = %#v", descriptor)
	}
	if err := descriptor.validateCorrectionPlan(); err != nil {
		t.Fatalf("current correction-plan descriptor = %v", err)
	}
}

func TestReviewerCaptureDescriptorUsesCaptureResult(t *testing.T) {
	t.Setenv(reviewPiHostRelayContractEnvironment, reviewPiHostRelayContract)
	binding := ReviewTransitionBinding{
		LineageID: "descriptor-reviewer", Revision: descriptorTestSHA("a"), TargetIdentity: descriptorTestSHA("b"),
		RepositoryContext: "rctx1_" + strings.Repeat("c", 64),
	}
	input := reviewCaptureInput(binding, reviewtransaction.LensReliability, 0, nil, model.AgentPi)
	if input.CaptureOperation != reviewCaptureResultCaptureOperation || input.Submission != nil {
		t.Fatalf("reviewer capture descriptor = %#v, want provider-owned execute route", input)
	}
	arguments, err := reviewTransitionArgumentMap(input.Arguments)
	if err != nil || arguments["agent"] != string(model.AgentPi) || arguments["execute"] != "true" {
		t.Fatalf("reviewer capture arguments = %#v, want agent=pi and execute=true", input.Arguments)
	}
}
