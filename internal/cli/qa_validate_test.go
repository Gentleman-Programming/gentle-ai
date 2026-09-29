package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestRunQAValidate_ValidArtifactReturnsArtifactRevision(t *testing.T) {
	stdin := strings.NewReader(`{"scope":{"status":"complete","next_stage":"spec"}}`)
	var stdout bytes.Buffer

	err := RunQAValidateFromReader([]string{"--input", "-", "--change", "x", "--stage", "explore"}, stdin, &stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result QAValidateResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("failed to decode output %q: %v", stdout.String(), err)
	}
	if !result.Valid {
		t.Fatalf("expected valid=true, got reason %q", result.Reason)
	}
	if result.ArtifactRevision == "" {
		t.Fatal("expected qa-validate to return a non-empty artifact_revision for a valid artifact")
	}
	if result.Stage != "explore" || result.Change != "x" {
		t.Fatalf("expected stage/change to be echoed back, got %+v", result)
	}
}

func TestRunQAValidate_InvalidArtifactHasNoArtifactRevision(t *testing.T) {
	// Same legacy-envelope shape confirmed rejected by the baseline.
	stdin := strings.NewReader(`{"schema":"gentle-ai.qa-stage-artifact/v1","scope":{"status":"complete","next_stage":"spec"}}`)
	var stdout bytes.Buffer

	err := RunQAValidateFromReader([]string{"--input", "-", "--change", "x", "--stage", "explore"}, stdin, &stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result QAValidateResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("failed to decode output %q: %v", stdout.String(), err)
	}
	if result.Valid {
		t.Fatal("expected valid=false for the legacy envelope")
	}
	if result.ArtifactRevision != "" {
		t.Fatalf("expected no artifact_revision on an invalid artifact, got %q", result.ArtifactRevision)
	}
	if result.Reason == "" {
		t.Fatal("expected a non-empty reason on rejection")
	}
}
