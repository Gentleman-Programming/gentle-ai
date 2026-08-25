package devorchestrator

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestFigmaAnalyzerSkillContentAssertsNoAccessStop covers spec requirement
// "Mandatory Stop-and-Report on No Real Design Access" (design D-E): real
// Figma retrieval is deliberately out of scope for this change (D4/D5), so
// an agent handed a design_ref has no way to actually see the referenced
// design. Without an explicit, load-bearing instruction, the agent could
// confidently invent design detail instead of admitting it cannot see the
// design -- a confidently hallucinated spec is strictly worse than an
// honest "I cannot see this design." This test asserts three exact literal
// strings in the shipped skill file, mirroring
// internal/assets/dev_agent_parity_test.go's exact-literal P3 marker
// assertion style: prose/regex matching would silently pass a weakened
// rewrite, so the assertion is deliberately brittle -- a legitimate future
// prose rewrite of SKILL.md is meant to fail this test until it preserves
// (or the test is deliberately updated alongside) all three literals.
func TestFigmaAnalyzerSkillContentAssertsNoAccessStop(t *testing.T) {
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}

	skillPath := filepath.Join(repoRoot, "skills", "technology", "figma-analyzer", "SKILL.md")
	content, err := os.ReadFile(skillPath)
	if err != nil {
		t.Fatalf("read %s: %v", skillPath, err)
	}
	body := string(content)

	requiredLiterals := []string{
		"<!-- contract: no-access-stop -->",
		"If you cannot actually retrieve the referenced design, STOP and report explicitly that you do not have design access.",
		"You MUST NOT infer or invent any design detail — structure, spacing, tokens, states, or components — as a substitute for design you cannot see.",
	}

	for _, literal := range requiredLiterals {
		if !strings.Contains(body, literal) {
			t.Errorf("%s missing required literal: %q", skillPath, literal)
		}
	}
}
