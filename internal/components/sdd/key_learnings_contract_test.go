package sdd

import (
	"regexp"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/internal/assets"
)

func TestKeyLearningsClosingContractExistsInSddPhaseCommon(t *testing.T) {
	content := assets.MustRead("skills/_shared/sdd-phase-common.md")
	for _, clause := range []string{
		"## F. Key Learnings Closing",
		"## Key Learnings",
		"1–5 items",
		"standalone factual sentence",
		"≥20 characters and ≥4 words",
	} {
		if !strings.Contains(content, clause) {
			t.Errorf("sdd-phase-common.md missing required clause %q", clause)
		}
	}
}

func TestKeyLearningsClosingRequiredForGenericDelegations(t *testing.T) {
	content := assets.MustRead("claude/sdd-orchestrator-workflow.md")
	for _, clause := range []string{
		"Key Learnings closing (generic delegations)",
		"generic agents (Explore, general-purpose)",
		"close its final message with a `## Key Learnings` section",
		"1–5 numbered items",
		"≥4 words and ≥20 characters",
		"enables engram passive capture",
	} {
		if !strings.Contains(content, clause) {
			t.Errorf("sdd-orchestrator-workflow.md missing required clause %q", clause)
		}
	}
}

func TestKeyLearningsContractConsistencyWithEngramProtocol(t *testing.T) {
	sddCommon := assets.MustRead("skills/_shared/sdd-phase-common.md")
	engramProtocol := assets.MustRead("engram/protocol.md")

	// Both must use the exact header token "## Key Learnings"
	if !strings.Contains(sddCommon, "## Key Learnings") {
		t.Errorf("sdd-phase-common.md does not use header token '## Key Learnings'")
	}
	if !strings.Contains(engramProtocol, "## Key Learnings:") && !strings.Contains(engramProtocol, "## Key Learnings") {
		t.Errorf("engram/protocol.md does not contain 'Key Learnings' header variant")
	}

	// sdd-phase-common.md must document the minimum length/word requirement for items
	hasLengthRule := regexp.MustCompile(`(?i)(\d+\s+character|\d+\s+word)`)
	if !hasLengthRule.MatchString(sddCommon) {
		t.Errorf("sdd-phase-common.md missing minimum-length rules for items")
	}

	// engram/protocol.md section must describe passive capture with examples
	if !strings.Contains(engramProtocol, "PASSIVE CAPTURE") && !strings.Contains(engramProtocol, "passive-capture") {
		t.Errorf("engram/protocol.md missing passive-capture section")
	}
}

func TestKeyLearningsReferenceIsConsistentAcrossAssets(t *testing.T) {
	sddCommon := assets.MustRead("skills/_shared/sdd-phase-common.md")
	orchestratorWorkflow := assets.MustRead("claude/sdd-orchestrator-workflow.md")

	// Orchestrator workflow should reference that phase agents load from sdd-phase-common.md
	if !strings.Contains(orchestratorWorkflow, "sdd-phase-common.md") {
		t.Errorf("sdd-orchestrator-workflow.md does not cross-reference sdd-phase-common.md for Key Learnings")
	}

	// Both should agree on the header token
	if !strings.Contains(sddCommon, "## Key Learnings") {
		t.Errorf("sdd-phase-common.md Key Learnings header mismatch")
	}
	if !strings.Contains(orchestratorWorkflow, "## Key Learnings") {
		t.Errorf("sdd-orchestrator-workflow.md Key Learnings header mismatch")
	}
}
