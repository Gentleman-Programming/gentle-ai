package sdd

import (
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/assets"
)

func TestReviewFoundationSkillsCarryThreatAndWorkUnitEvidence(t *testing.T) {
	tests := []struct {
		path  string
		wants []string
	}{
		{path: "skills/sdd-design/SKILL.md", wants: []string{"Applicability-Driven Threat Matrix", "references/threat-matrix.md", "explicit `N/A`"}},
		{path: "skills/sdd-tasks/SKILL.md", wants: []string{"Every applicable threat-matrix case", "Focused test command", "Runtime harness", "Rollback boundary"}},
		{path: "skills/sdd-apply/SKILL.md", wants: []string{"Work Unit Evidence", "Focused test command and exact result", "Runtime harness command/scenario and exact result", "Rollback boundary"}},
		{path: "skills/sdd-verify/SKILL.md", wants: []string{"including partial work", "command results and limitations", "do not gate archive", "model/provider/profile"}},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			content := assets.MustRead(tt.path)
			for _, want := range tt.wants {
				if !strings.Contains(content, want) {
					t.Errorf("%s missing %q", tt.path, want)
				}
			}
		})
	}

	matrix := assets.MustRead("skills/sdd-design/references/threat-matrix.md")
	for _, want := range []string{
		"requirements.txt", "CMakeLists.txt", "Markdown/MDX", "README.sh",
		"git -C", "relative paths", "absolute paths", "staged", "commit -a", "empty index",
		"tracking branch", "first push", "explicit refspec", "--head", "environment prefix", "composed commands",
	} {
		if !strings.Contains(matrix, want) {
			t.Errorf("threat matrix missing %q", want)
		}
	}

	statusContract := assets.MustRead("skills/_shared/sdd-status-contract.md")
	for _, want := range []string{"gentle-ai.sdd-status/v2", "SDD status never reads review mode", "optional diagnostic", "not a verdict"} {
		if !strings.Contains(statusContract, want) {
			t.Errorf("status contract missing %q", want)
		}
	}
}

func TestSDDApplyRoutesToArchiveWithOptionalVerify(t *testing.T) {
	content := assets.MustRead("skills/sdd-apply/SKILL.md")
	for _, want := range []string{
		"After all implementation work units finish, return control to the parent orchestrator for archive.",
		"Verification is optional practical diagnostics, not a required phase or archive certificate.",
		"neither executor nor parent offers or launches RDD within SDD.",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("sdd-apply missing ordering rule %q", want)
		}
	}
	if strings.Contains(content, "review/start(target)") {
		t.Fatal("sdd-apply retains a direct post-apply review launch")
	}
}

func TestSDDVerifyRunsWithoutReviewArtifacts(t *testing.T) {
	content := assets.MustRead("skills/sdd-verify/SKILL.md")
	for _, want := range []string{
		"SDD never offers, launches, or consumes RDD.",
		"Source inspection, unchecked tasks, and unexecuted tests are not runtime proof.",
		"Missing tooling means unavailable checks, not PASS.",
		"Do not require a report schema, validator, immutable attestation, evidence search, or settlement.",
		"Findings do not start automatic review, refuter, or correction loops.",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("sdd-verify missing independent verification clause %q", want)
		}
	}
	for _, forbidden := range []string{
		"authoritative preterminal transaction",
		"missing_review_authority",
		"authority_only_failure",
		"exact canonical verification-evidence bytes, not only their hash",
		"complete-final-verification",
	} {
		if strings.Contains(content, forbidden) {
			t.Fatalf("sdd-verify retains a review prerequisite %q", forbidden)
		}
	}
}
