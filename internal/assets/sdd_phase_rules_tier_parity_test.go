package assets

import (
	"regexp"
	"strings"
	"testing"
)

// rulesConfigTokenPattern matches a phase-scoped openspec/config.yaml rules
// reference such as `rules.apply` or `rules.verify`.
var rulesConfigTokenPattern = regexp.MustCompile(`rules\.[a-zA-Z0-9_]+`)

// sddPhaseSkillIDs enumerates every sdd-<phase> skill directory under
// internal/assets/skills. Kept as a literal list (rather than a directory
// walk) so a newly added phase must be added here deliberately.
var sddPhaseSkillIDs = []string{
	"sdd-apply",
	"sdd-archive",
	"sdd-design",
	"sdd-explore",
	"sdd-init",
	"sdd-onboard",
	"sdd-propose",
	"sdd-research",
	"sdd-spec",
	"sdd-tasks",
	"sdd-verify",
}

// extractModelSectionForTest mirrors
// internal/components/filemerge.ExtractHTMLCommentSection's marker semantics
// without importing that package from assets' test suite. When a matching
// section marker pair is absent, the whole content is returned unchanged —
// which is how a single-tier skill (no model-capable/model-small split)
// naturally short-circuits the drift check below.
func extractModelSectionForTest(content, tier string) string {
	openMarker := "<!-- section:model-" + tier + " -->"
	closeMarker := "<!-- /section:model-" + tier + " -->"
	start := strings.Index(content, openMarker)
	end := strings.Index(content, closeMarker)
	if start == -1 || end == -1 || end <= start {
		return content
	}
	return content[start+len(openMarker) : end]
}

// #4114/#4118: sdd-apply and sdd-verify render two model tiers
// (model-capable and model-small) from one SKILL.md. Whenever any tier
// mentions reading a `rules.<phase>` key from openspec/config.yaml, every
// other rendered tier of that same skill must carry the same instruction —
// otherwise a smaller model silently skips the project's configured phase
// rules while the capable tier still applies them.
func TestSDDPhaseSkillsKeepRulesConfigInstructionAcrossModelTiers(t *testing.T) {
	for _, phase := range sddPhaseSkillIDs {
		t.Run(phase, func(t *testing.T) {
			path := "skills/" + phase + "/SKILL.md"
			content := MustRead(path)

			tokens := rulesConfigTokenPattern.FindAllString(content, -1)
			if len(tokens) == 0 {
				t.Skipf("%s does not reference an openspec/config.yaml rules key", path)
			}

			capable := extractModelSectionForTest(content, "capable")
			small := extractModelSectionForTest(content, "small")
			if capable == content && small == content {
				// No model-capable/model-small marker pair found: this skill
				// renders a single tier, so there is nothing to drift.
				return
			}

			seen := map[string]bool{}
			for _, token := range tokens {
				if seen[token] {
					continue
				}
				seen[token] = true

				if !strings.Contains(capable, token) {
					t.Errorf("%s: model-capable section is missing %q", path, token)
				}
				if !strings.Contains(small, token) {
					t.Errorf("%s: model-small section is missing %q", path, token)
				}
			}
		})
	}
}

// #4114: docs/openspec-config.md documents that sdd-verify reads
// `rules.verify` from openspec/config.yaml, the same way sdd-apply already
// reads `rules.apply` — but the sdd-verify skill never actually said so.
func TestSDDApplyAndVerifyReadPhaseScopedConfigRules(t *testing.T) {
	cases := map[string]string{
		"sdd-apply":  "rules.apply",
		"sdd-verify": "rules.verify",
	}
	for phase, token := range cases {
		t.Run(phase, func(t *testing.T) {
			path := "skills/" + phase + "/SKILL.md"
			if content := MustRead(path); !strings.Contains(content, token) {
				t.Fatalf("%s must mention %q (see docs/openspec-config.md phase table)", path, token)
			}
		})
	}
}

// #4114: strict-tdd-verify.md carries its own "Rules (Strict TDD Verify
// specific)" list and is loaded instead of relying solely on the base
// SKILL.md Hard Rules once Strict TDD is active, so it must repeat the
// rules.verify instruction rather than let it apply only outside TDD mode.
func TestStrictTDDVerifyModuleRepeatsRulesConfigInstruction(t *testing.T) {
	path := "skills/sdd-verify/strict-tdd-verify.md"
	if content := MustRead(path); !strings.Contains(content, "rules.verify") {
		t.Fatalf("%s must mention \"rules.verify\" since it carries its own Rules list", path)
	}
}
