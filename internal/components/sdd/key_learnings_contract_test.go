package sdd

import (
	"regexp"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/assets"
	"github.com/gentleman-programming/gentle-ai/v2/internal/components/filemerge"
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
		"standalone factual sentence",
		"≥4 words and ≥20 characters",
		"enables engram passive capture",
	} {
		if !strings.Contains(content, clause) {
			t.Errorf("sdd-orchestrator-workflow.md missing required clause %q", clause)
		}
	}
}

// keyLearningsSection extracts the "## F. Key Learnings Closing" section of
// sdd-phase-common.md (from its header to the next lettered section header,
// e.g. "## G. ...", or EOF).
func keyLearningsSection(t *testing.T, content string) string {
	t.Helper()
	const header = "## F. Key Learnings Closing"
	start := strings.Index(content, header)
	if start < 0 {
		t.Fatalf("sdd-phase-common.md missing section %q", header)
	}
	section := content[start:]
	// Stop at the next lettered section header (e.g. "## G. ..."); a plain
	// "\n## " boundary would truncate at the `## Key Learnings` line inside
	// the fenced example.
	if loc := regexp.MustCompile(`\n## [A-Z]\. `).FindStringIndex(section[len(header):]); loc != nil {
		section = section[:len(header)+loc[0]]
	}
	return section
}

// passiveCaptureSection extracts the passive-capture section body of
// engram/protocol.md using the same extractor as production. The extractor
// silently returns the full content when markers are missing or malformed,
// so assert that a real extraction happened.
func passiveCaptureSection(t *testing.T, content string) string {
	t.Helper()
	section := filemerge.ExtractHTMLCommentSection(content, "passive-capture")
	if section == content {
		t.Fatalf("engram/protocol.md missing or malformed passive-capture section markers")
	}
	return section
}

func TestKeyLearningsHeaderAndWordingConsistencyWithEngramProtocol(t *testing.T) {
	sddSection := keyLearningsSection(t, assets.MustRead("skills/_shared/sdd-phase-common.md"))
	engramSection := passiveCaptureSection(t, assets.MustRead("engram/protocol.md"))

	// Both contract blocks must use the same header token; the engram
	// extractor treats the trailing colon as optional.
	headerToken := regexp.MustCompile("## Key Learnings:?")
	if !headerToken.MatchString(sddSection) {
		t.Errorf("sdd-phase-common.md Key Learnings section does not use header token '## Key Learnings'")
	}
	if !headerToken.MatchString(engramSection) {
		t.Errorf("engram/protocol.md passive-capture section does not use header token '## Key Learnings'")
	}

	// The SDD contract block must document the minimum length/word rules.
	hasLengthRule := regexp.MustCompile(`(?i)(\d+\s+character|\d+\s+word)`)
	if !hasLengthRule.MatchString(sddSection) {
		t.Errorf("sdd-phase-common.md Key Learnings section missing minimum-length rules for items")
	}

	// Both contract blocks must describe automatic extraction by engram.
	for name, section := range map[string]string{
		"sdd-phase-common.md": sddSection,
		"engram/protocol.md":  engramSection,
	} {
		if !strings.Contains(section, "automatically extract") {
			t.Errorf("%s contract block missing automatic-extraction wording", name)
		}
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
