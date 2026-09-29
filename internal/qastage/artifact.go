// Package qastage implements the QA-Orchestrator v3 core: artifact
// validation, the QA stage state machine, and stage approval. See
// docs/migration/qa-orchestrator-v3-design.md for the architecture this
// package implements (sections 1.1 and 7).
package qastage

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
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

// ArtifactV1 is the canonical QA stage artifact schema (schema version 1).
// This is the ONLY schema the validator accepts — it is not a wrapper with
// separate envelope metadata (schema/change/stage/status/created_at/
// artifact_revision/facts/observations/inferences/recommendations/risks/
// decision). That wrapper shape was what the v2 skills incorrectly
// documented; the real v2 code, and this v3 implementation, only ever
// accepted this body. See ADR note in the design doc, section 1.1.
type ArtifactV1 struct {
	Findings          []Finding         `json:"findings,omitempty"`
	PendingQuestions  []PendingQuestion `json:"pending_questions,omitempty"`
	Scope             Scope             `json:"scope"`
	PredecessorSHA256 string            `json:"predecessor_sha256,omitempty"`
}

// ValidationResult is the outcome of validating a QA stage artifact.
type ValidationResult struct {
	Valid            bool
	Stage            string
	ArtifactRevision string
	Reason           string
}

// QAArtifactValidator validates a QA stage artifact against the canonical
// schema and anti-hallucination rules, and computes its artifact_revision.
type QAArtifactValidator interface {
	Validate(payload []byte, stage string, expectedPredecessorSHA256 string) ValidationResult
}

// CanonicalArtifactValidator implements QAArtifactValidator for schema v1.
type CanonicalArtifactValidator struct{}

// NewCanonicalArtifactValidator returns the current (v1) canonical artifact validator.
func NewCanonicalArtifactValidator() *CanonicalArtifactValidator {
	return &CanonicalArtifactValidator{}
}

func isPlaceholder(s string) bool {
	lower := strings.ToLower(strings.TrimSpace(s))
	return lower == "todo" || lower == "..." || lower == "tbd" || lower == "placeholder" || strings.Contains(lower, "[insert") || strings.Contains(lower, "<insert")
}

// Validate checks payload against the canonical ArtifactV1 schema. On
// success, ArtifactRevision is the sha256 of the exact payload bytes
// (canonicalized by re-marshaling through ArtifactV1, so semantically
// identical payloads with different formatting still match) — this is the
// value qa-finish/qa-approve must require as --evidence-revision (3A.8/3A.9),
// closing the gap where v2 accepted any well-formed but arbitrary hash.
func (v *CanonicalArtifactValidator) Validate(payload []byte, stage string, expectedPredecessorSHA256 string) ValidationResult {
	result := ValidationResult{Stage: stage}

	if len(payload) > 1024*1024 {
		result.Reason = "artifact payload exceeds 1MiB limit"
		return result
	}

	dec := json.NewDecoder(bytes.NewReader(payload))
	dec.DisallowUnknownFields()

	var artifact ArtifactV1
	if err := dec.Decode(&artifact); err != nil {
		if err == io.EOF {
			result.Reason = "empty JSON payload"
			return result
		}
		result.Reason = fmt.Sprintf("invalid JSON or unknown field: %s", err.Error())
		return result
	}

	if err := validateBody(artifact, stage, expectedPredecessorSHA256); err != nil {
		result.Reason = err.Error()
		return result
	}

	canonical, err := json.Marshal(artifact)
	if err != nil {
		result.Reason = fmt.Sprintf("canonicalize artifact: %s", err.Error())
		return result
	}
	sum := sha256.Sum256(canonical)
	result.Valid = true
	result.ArtifactRevision = "sha256:" + hex.EncodeToString(sum[:])
	return result
}

func validateBody(body ArtifactV1, currentStage string, expectedPredecessorSHA256 string) error {
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
		validNext := false
		for _, s := range QAStageVocabulary() {
			if s == body.Scope.NextStage {
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

// QAStageVocabulary returns the fixed, canonical QA stage sequence for v3.
// Unlike v2's configurable StageVocabulary, this is fixed by design decision
// 1.1 — QA has exactly these stages, in this order.
func QAStageVocabulary() []string {
	return []string{"explore", "spec", "apply", "verify", "docs"}
}
