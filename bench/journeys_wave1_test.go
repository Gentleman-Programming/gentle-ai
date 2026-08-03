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

func TestRequireCandidateDeclinedGate(t *testing.T) {
	sandbox := &Sandbox{Scratch: map[string]string{
		"decline-base":  "1111111111111111111111111111111111111111",
		"decline-tree":  "2222222222222222222222222222222222222222",
		"decline-paths": "sha256:3333333333333333333333333333333333333333333333333333333333333333",
	}}
	observation := Observation{Stdout: `{"result":"invalidated","allowed":false,"action":"repository-policy","delivery":"candidate_declined/unmanaged","context":{"lineage_id":"","base_tree":"1111111111111111111111111111111111111111","candidate_tree":"2222222222222222222222222222222222222222","paths_digest":"sha256:3333333333333333333333333333333333333333333333333333333333333333","denial":{"stage":"candidate-decline","code":"exact_candidate"}}}`}
	if err := requireCandidateDeclinedGate(sandbox, observation); err != nil {
		t.Fatal(err)
	}
	observation.Stdout = strings.Replace(observation.Stdout, `"lineage_id":""`, `"lineage_id":"fabricated"`, 1)
	if err := requireCandidateDeclinedGate(sandbox, observation); err == nil {
		t.Fatal("accepted candidate-declined gate with fabricated lineage")
	}
}
