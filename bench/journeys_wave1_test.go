package main

import (
	"encoding/json"
	"os"
	"path/filepath"
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

const correctionPlanStatusJSON = `{
  "schema": "gentle-ai.review-integration/v2",
  "authority": {
    "lineage_id": "carried-correction-plan",
    "state": "correction_required",
    "revision": "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
  },
  "target_identity": "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
  "receipt": {"status": "not_applicable", "identity": "receipt-identity"},
  "action": "capture the bounded correction plan",
  "validation_request": {
    "request_hash": "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc",
    "correction_target_identity": "sha256:dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd",
    "correction_paths_digest": "sha256:eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"
  },
  "next_transition": {
    "kind": "collect",
    "reason_code": "correction_plan_required",
    "collect": {"inputs": [{
      "name": "correction_lines",
      "capture_operation": "review.capture-correction-plan",
      "arguments": [{"name": "lineage", "value": "carried-correction-plan", "token": "--lineage=carried-correction-plan"}],
      "submission": {
        "operation_token": "finalize",
        "argument_tokens": ["--lineage=carried-correction-plan", "--request-hash=sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc", "--correction-lines={{value}}"],
        "value": {"slot": "correction_lines", "substitution_location": 2}
      }
    }]}
  }
}
`

func correctionPlanStatusTestRun(t *testing.T) (*journeyRun, Observation) {
	t.Helper()
	root := t.TempDir()
	binary := filepath.Join(root, "status-continuation")
	script := "#!/bin/sh\ncat <<'STATUS'\n" + correctionPlanStatusJSON + "STATUS\n"
	if err := os.WriteFile(binary, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	run := &journeyRun{
		sandbox: &Sandbox{
			Binary:  binary,
			Root:    root,
			Home:    root,
			Repo:    root,
			Scratch: map[string]string{},
		},
		accumulator: newAccumulator(),
	}
	closure := Observation{ExitCode: 0, Stdout: `{
  "lineage_id": "carried-correction-plan",
  "state": "correction_required",
  "status_continuation": {
    "operation": "review.status",
    "arguments": [{"token": "--lineage=carried-correction-plan"}]
  }
}`}
	return run, closure
}

func rememberCorrectionPlanStatusForTest(t *testing.T) *journeyRun {
	t.Helper()
	run, closure := correctionPlanStatusTestRun(t)
	status, continued, err := correctionStatusFromLastEventCapture(run, closure)
	if err != nil || !continued {
		t.Fatalf("correction status continuation = continued %t, err %v", continued, err)
	}
	if err := rememberCorrectionStatusContinuation(run, status.Authority.LineageID, status); err != nil {
		t.Fatal(err)
	}
	return run
}

func TestRememberCorrectionStatusContinuationPreservesExactRawStatusJSON(t *testing.T) {
	run := rememberCorrectionPlanStatusForTest(t)
	payload := run.sandbox.Scratch[correctionPlanStatusContinuationKeyPrefix+"carried-correction-plan"]
	if payload != correctionPlanStatusJSON {
		t.Fatalf("carried correction STATUS changed bytes:\n got: %q\nwant: %q", payload, correctionPlanStatusJSON)
	}

	var status waveCorrectionStatus
	if err := json.Unmarshal([]byte(payload), &status); err != nil {
		t.Fatal(err)
	}
	if status.Schema != "gentle-ai.review-integration/v2" || status.Receipt.Identity != "receipt-identity" ||
		status.Action != "capture the bounded correction plan" || status.ValidationRequest == nil ||
		status.ValidationRequest.RequestHash == "" || status.NextTransition == nil ||
		len(status.NextTransition.Collect.Inputs) != 1 || status.NextTransition.Collect.Inputs[0].Submission == nil {
		t.Fatalf("load-bearing correction STATUS fields were not preserved: %#v", status)
	}
}

func TestCarriedCorrectionPlanStatusSurvivesInspectionAndPlanReads(t *testing.T) {
	lineage := "carried-correction-plan"
	t.Run("inspection first", func(t *testing.T) {
		run := rememberCorrectionPlanStatusForTest(t)
		if _, err := readCorrectionStatusForContract(run, lineage, reviewContractV2); err != nil {
			t.Fatal(err)
		}
		if payload := run.sandbox.Scratch[correctionPlanStatusContinuationKeyPrefix+lineage]; payload != correctionPlanStatusJSON {
			t.Fatalf("inspection consumed the correction-plan binding: %q", payload)
		}
	})
	t.Run("plan read first", func(t *testing.T) {
		run := rememberCorrectionPlanStatusForTest(t)
		if _, found, err := takeCorrectionStatusContinuation(run, lineage); err != nil || !found {
			t.Fatalf("plan read = found %t, err %v", found, err)
		}
		if payload := run.sandbox.Scratch[correctionPlanStatusContinuationKeyPrefix+lineage]; payload != correctionPlanStatusJSON {
			t.Fatalf("plan read consumed the exact correction-plan binding: %q", payload)
		}
		if _, err := readCorrectionStatusForContract(run, lineage, reviewContractV2); err != nil {
			t.Fatal(err)
		}
	})
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
