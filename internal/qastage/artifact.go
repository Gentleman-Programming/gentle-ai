package qastage

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
)

var sha256Pattern = regexp.MustCompile(`^sha256:[a-f0-9]{64}$`)

// Finding represents an analysis finding during a QA stage.
type Finding struct {
	Classification string `json:"classification"`
	URL            string `json:"url,omitempty"`
	Description    string `json:"description,omitempty"`
}

// PendingQuestion represents a blocked or missing detail needing resolution.
type PendingQuestion struct {
	Question string `json:"question"`
}

// Scope defines the termination state of a QA stage.
type Scope struct {
	Status        string `json:"status"`
	BlockedReason string `json:"blocked_reason,omitempty"`
	NextStage     string `json:"next_stage,omitempty"`
}

// stageBody represents the JSON payload of a QA artifact.
type stageBody struct {
	Findings          []Finding         `json:"findings,omitempty"`
	PendingQuestions  []PendingQuestion `json:"pending_questions,omitempty"`
	Scope             Scope             `json:"scope"`
	PredecessorSHA256 string            `json:"predecessor_sha256,omitempty"`
}

// isPlaceholder returns true if the text matches common hallucinated or placeholder values.
func isPlaceholder(s string) bool {
	lower := strings.ToLower(strings.TrimSpace(s))
	return lower == "todo" || lower == "..." || lower == "tbd" || lower == "placeholder" || strings.Contains(lower, "[insert") || strings.Contains(lower, "<insert")
}

// ValidateStageArtifactAdmission verifies a QA stage artifact against strict anti-hallucination rules.
func ValidateStageArtifactAdmission(payload []byte, currentStage string, expectedPredecessorSHA256 string) error {
	if len(payload) > 1024*1024 {
		return errors.New("artifact payload exceeds 1MiB limit")
	}

	dec := json.NewDecoder(bytes.NewReader(payload))
	dec.DisallowUnknownFields()

	var body stageBody
	if err := dec.Decode(&body); err != nil {
		if err == io.EOF {
			return errors.New("empty JSON payload")
		}
		return fmt.Errorf("invalid JSON or unknown field: %w", err)
	}

	if body.PredecessorSHA256 != "" && !sha256Pattern.MatchString(body.PredecessorSHA256) {
		return errors.New("bad sha256 pattern in predecessor_sha256")
	}
	if expectedPredecessorSHA256 != "" && body.PredecessorSHA256 != expectedPredecessorSHA256 {
		return errors.New("predecessor-binding mismatch")
	}

	for _, f := range body.Findings {
		if isPlaceholder(f.URL) || isPlaceholder(f.Description) {
			return errors.New("placeholder evidence detected in finding")
		}
		if f.Classification != "DOCUMENTED" && f.Classification != "MISSING" && f.Classification != "NOT_APPLICABLE" {
			return errors.New("bad classification in finding")
		}
		if f.Classification == "DOCUMENTED" && strings.TrimSpace(f.URL) == "" {
			return errors.New("DOCUMENTED finding needs URL")
		}
		if f.Classification == "MISSING" && len(body.PendingQuestions) == 0 {
			return errors.New("MISSING finding needs pending question")
		}
	}

	for _, q := range body.PendingQuestions {
		if isPlaceholder(q.Question) {
			return errors.New("placeholder evidence detected in pending question")
		}
	}

	if body.Scope.Status != "complete" && body.Scope.Status != "blocked" {
		return errors.New("scope status must be complete or blocked")
	}
	if body.Scope.Status == "blocked" && strings.TrimSpace(body.Scope.BlockedReason) == "" {
		return errors.New("blocked scope needs blocked_reason")
	}
	if body.Scope.Status == "complete" && strings.TrimSpace(body.Scope.BlockedReason) != "" {
		return errors.New("complete/blocked contradiction: complete scope cannot have blocked_reason")
	}
	if body.Scope.Status == "complete" && strings.TrimSpace(body.Scope.NextStage) == "" && currentStage != "docs" {
		return errors.New("complete scope needs next_stage")
	}

	if body.Scope.NextStage != "" {
		vocab := VocabularyV1()
		validNext := false
		for _, s := range vocab.Stages {
			if s.Label == body.Scope.NextStage {
				validNext = true
				break
			}
		}
		if !validNext {
			return errors.New("illegal stage successor")
		}
	}

	return nil
}
