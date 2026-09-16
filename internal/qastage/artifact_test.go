package qastage_test

import (
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/qastage"
)

func TestValidateStageArtifactAdmission(t *testing.T) {
	tests := []struct {
		name              string
		payload           string
		currentStage      string
		expectedSha       string
		expectedError     string
	}{
		{
			name: "valid empty scope",
			payload: `{"scope": {"status": "complete", "next_stage": "spec"}}`,
			currentStage: "explore",
			expectedError: "",
		},
		{
			name: "valid blocked scope",
			payload: `{"scope": {"status": "blocked", "blocked_reason": "waiting for design"}}`,
			currentStage: "explore",
			expectedError: "",
		},
		{
			name: "bad classification",
			payload: `{"findings": [{"classification": "UNKNOWN_CLASS", "url": "http://x"}], "scope": {"status": "complete", "next_stage": "spec"}}`,
			expectedError: "bad classification in finding",
		},
		{
			name: "DOCUMENTED needs URL",
			payload: `{"findings": [{"classification": "DOCUMENTED"}], "scope": {"status": "complete", "next_stage": "spec"}}`,
			expectedError: "DOCUMENTED finding needs URL",
		},
		{
			name: "MISSING needs pending question",
			payload: `{"findings": [{"classification": "MISSING"}], "scope": {"status": "complete", "next_stage": "spec"}}`,
			expectedError: "MISSING finding needs pending question",
		},
		{
			name: "complete/blocked contradiction",
			payload: `{"scope": {"status": "complete", "blocked_reason": "should not exist", "next_stage": "spec"}}`,
			expectedError: "complete/blocked contradiction",
		},
		{
			name: "bad sha256 pattern",
			payload: `{"predecessor_sha256": "bad-hash", "scope": {"status": "complete", "next_stage": "spec"}}`,
			expectedError: "bad sha256 pattern in predecessor_sha256",
		},
		{
			name: "illegal stage successor",
			payload: `{"scope": {"status": "complete", "next_stage": "invalid_stage"}}`,
			expectedError: "illegal stage successor",
		},
		{
			name: "predecessor-binding mismatch",
			payload: `{"predecessor_sha256": "sha256:1111111111111111111111111111111111111111111111111111111111111111", "scope": {"status": "complete", "next_stage": "spec"}}`,
			expectedSha: "sha256:2222222222222222222222222222222222222222222222222222222222222222",
			expectedError: "predecessor-binding mismatch",
		},
		{
			name: "unknown JSON field",
			payload: `{"unknown_field": 123, "scope": {"status": "complete", "next_stage": "spec"}}`,
			expectedError: "unknown field",
		},
		{
			name: "placeholder evidence in finding URL",
			payload: `{"findings": [{"classification": "DOCUMENTED", "url": "[insert URL]"}], "scope": {"status": "complete", "next_stage": "spec"}}`,
			expectedError: "placeholder evidence detected in finding",
		},
		{
			name: "placeholder evidence in pending question",
			payload: `{"pending_questions": [{"question": "TODO"}], "scope": {"status": "complete", "next_stage": "spec"}}`,
			expectedError: "placeholder evidence detected in pending question",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := qastage.ValidateStageArtifactAdmission([]byte(tc.payload), tc.currentStage, tc.expectedSha)
			if tc.expectedError == "" {
				if err != nil {
					t.Errorf("expected success, got error: %v", err)
				}
			} else {
				if err == nil {
					t.Errorf("expected error containing %q, got nil", tc.expectedError)
				} else if !strings.Contains(err.Error(), tc.expectedError) {
					t.Errorf("expected error containing %q, got %v", tc.expectedError, err)
				}
			}
		})
	}
}

func TestValidateStageArtifactAdmissionSizeLimit(t *testing.T) {
	largePayload := make([]byte, 1024*1024+1)
	err := qastage.ValidateStageArtifactAdmission(largePayload, "explore", "")
	if err == nil || !strings.Contains(err.Error(), "exceeds 1MiB limit") {
		t.Errorf("expected 1MiB limit error, got %v", err)
	}
}
