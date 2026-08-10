package main

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestWaveReviewInvocationArgs(t *testing.T) {
	got, err := waveReviewInvocationArgs("gentle-ai review start --contract=gentle-ai.review-integration/v2 --consent=relay")
	if err != nil {
		t.Fatal(err)
	}
	if joined := strings.Join(got, "\x00"); joined != "review\x00start\x00--contract=gentle-ai.review-integration/v2\x00--consent=relay" {
		t.Fatalf("review invocation args = %q", joined)
	}
	for _, invalid := range []string{"", "other review start", "gentle-ai status"} {
		if _, err := waveReviewInvocationArgs(invalid); err == nil {
			t.Fatalf("accepted invalid review invocation %q", invalid)
		}
	}
}

func TestCorrectionSubmissionArgumentsRejectsShortDescriptor(t *testing.T) {
	var status waveCorrectionStatus
	if err := json.Unmarshal([]byte(`{
  "authority": {"lineage_id": "lineage"},
  "next_transition": {
    "kind": "collect",
    "reason_code": "correction_plan_required",
    "collect": {"inputs": [{
      "name": "correction_lines",
      "submission": {
        "operation_token": "finalize",
        "argument_tokens": [
          "--contract=gentle-ai.review-integration/v2",
          "--lineage=lineage",
          "--expected-revision=sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
          "--target=sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
        ],
        "value": {"slot": "correction_lines", "substitution_location": 3}
      }
    }]}
  }
}`), &status); err != nil {
		t.Fatal(err)
	}
	run := &journeyRun{sandbox: &Sandbox{Root: t.TempDir()}}
	if _, err := correctionSubmissionArguments(run, status, "correction_plan_required", "correction_lines", "2"); err == nil || !strings.Contains(err.Error(), "has 4 tokens") {
		t.Fatalf("short descriptor error = %v", err)
	}
}

// TestRequireCandidateDeclineGateDeniesGenerically supersedes
// TestRequireCandidateDeclinedGate (Wave 5 Slice 6, design decision 6):
// requireCandidateDeclinedGate asserted the OLD decline-specific unmanaged
// shape, deleted along with j50's old form.
func TestRequireCandidateDeclineGateDeniesGenerically(t *testing.T) {
	observation := Observation{ExitCode: 1, Stdout: `{"result":"invalidated","allowed":false,"action":"explicit-maintainer-action","reason":"no terminal review receipt exists for gate validation","context":{"lineage_id":"","denial":{"stage":"receipt-discovery","code":"receipt_missing"}}}`}
	if err := requireCandidateDeclineGateDeniesGenerically(nil, observation); err != nil {
		t.Fatal(err)
	}
	allowed := Observation{Stdout: `{"result":"allow","allowed":true,"action":"continue"}`}
	if err := requireCandidateDeclineGateDeniesGenerically(nil, allowed); err == nil {
		t.Fatal("accepted an allowed candidate-declined gate")
	}
}
