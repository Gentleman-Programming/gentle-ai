package qastage

import (
	"strings"
	"testing"
)

// These tests lock the canonical QA stage artifact schema (v1) — the schema
// the real Go code accepts, not the wrapper envelope (schema/facts/
// observations/.../artifact_revision) that the v2 skills incorrectly
// described. See docs/migration/qa-orchestrator-v2-baseline-checklist.md,
// Escenario 1, for the evidence behind this contract.

func TestValidateArtifact_AcceptsCanonicalSchema(t *testing.T) {
	payload := []byte(`{"findings":[{"classification":"DOCUMENTED","url":"https://example.com/doc","description":"real evidence"}],"scope":{"status":"complete","next_stage":"spec"}}`)

	result := NewCanonicalArtifactValidator().Validate(payload, "explore", "")

	if !result.Valid {
		t.Fatalf("expected valid=true, got false with reason %q", result.Reason)
	}
	if result.ArtifactRevision == "" {
		t.Fatal("expected a non-empty artifact_revision on a valid artifact")
	}
	if !strings.HasPrefix(result.ArtifactRevision, "sha256:") || len(result.ArtifactRevision) != len("sha256:")+64 {
		t.Fatalf("expected artifact_revision to be sha256:<64 hex>, got %q", result.ArtifactRevision)
	}
}

func TestValidateArtifact_RejectsLegacySkillEnvelope(t *testing.T) {
	// This is exactly the envelope shape the v2 skills (qa-apply/SKILL.md etc.)
	// described as required — confirmed by the baseline to be rejected by the
	// real validator. v3 preserves that rejection: the wrapper never becomes
	// part of the canonical schema.
	payload := []byte(`{"schema":"gentle-ai.qa-stage-artifact/v1","change":"x","stage":"explore","status":"complete","created_at":"2026-09-23T00:00:00Z","facts":[],"observations":[],"scope":{"status":"complete","next_stage":"spec"}}`)

	result := NewCanonicalArtifactValidator().Validate(payload, "explore", "")

	if result.Valid {
		t.Fatal("expected the legacy skill-described envelope to be rejected, got valid=true")
	}
	if !strings.Contains(result.Reason, "unknown field") {
		t.Fatalf("expected an unknown-field rejection reason, got %q", result.Reason)
	}
	if result.ArtifactRevision != "" {
		t.Fatalf("expected no artifact_revision on an invalid artifact, got %q", result.ArtifactRevision)
	}
}

func TestValidateArtifact_ArtifactRevisionIsDeterministic(t *testing.T) {
	payload := []byte(`{"scope":{"status":"complete","next_stage":"apply"}}`)

	first := NewCanonicalArtifactValidator().Validate(payload, "spec", "")
	second := NewCanonicalArtifactValidator().Validate(payload, "spec", "")

	if !first.Valid || !second.Valid {
		t.Fatalf("expected both validations to succeed, got %v and %v", first, second)
	}
	if first.ArtifactRevision != second.ArtifactRevision {
		t.Fatalf("expected the same payload to produce the same artifact_revision, got %q and %q", first.ArtifactRevision, second.ArtifactRevision)
	}
}

func TestValidateArtifact_DifferentPayloadsProduceDifferentRevisions(t *testing.T) {
	a := NewCanonicalArtifactValidator().Validate([]byte(`{"scope":{"status":"complete","next_stage":"apply"}}`), "spec", "")
	b := NewCanonicalArtifactValidator().Validate([]byte(`{"scope":{"status":"complete","next_stage":"verify"}}`), "spec", "")

	if a.ArtifactRevision == b.ArtifactRevision {
		t.Fatalf("expected different payloads to produce different artifact_revisions, both got %q", a.ArtifactRevision)
	}
}

func TestValidateArtifact_PredecessorBindingMismatchIsRejected(t *testing.T) {
	payload := []byte(`{"predecessor_sha256":"sha256:` + strings.Repeat("a", 64) + `","scope":{"status":"complete","next_stage":"apply"}}`)

	result := NewCanonicalArtifactValidator().Validate(payload, "spec", "sha256:"+strings.Repeat("b", 64))

	if result.Valid {
		t.Fatal("expected predecessor-binding mismatch to be rejected")
	}
}

func TestValidateArtifact_ScopeCompleteRequiresNextStageUnlessDocs(t *testing.T) {
	payload := []byte(`{"scope":{"status":"complete"}}`)

	result := NewCanonicalArtifactValidator().Validate(payload, "verify", "")
	if result.Valid {
		t.Fatal("expected complete scope without next_stage to be rejected for a non-terminal stage")
	}

	docsResult := NewCanonicalArtifactValidator().Validate(payload, "docs", "")
	if !docsResult.Valid {
		t.Fatalf("expected complete scope without next_stage to be accepted for the terminal 'docs' stage, got reason %q", docsResult.Reason)
	}
}

func TestValidateArtifact_BlockedScopeRequiresReason(t *testing.T) {
	payload := []byte(`{"scope":{"status":"blocked"}}`)

	result := NewCanonicalArtifactValidator().Validate(payload, "explore", "")

	if result.Valid {
		t.Fatal("expected blocked scope without blocked_reason to be rejected")
	}
}

func TestValidateArtifact_PlaceholderFindingIsRejected(t *testing.T) {
	payload := []byte(`{"findings":[{"classification":"DOCUMENTED","url":"TODO","description":"real"}],"scope":{"status":"complete","next_stage":"spec"}}`)

	result := NewCanonicalArtifactValidator().Validate(payload, "explore", "")

	if result.Valid {
		t.Fatal("expected a placeholder value in a finding to be rejected")
	}
}
