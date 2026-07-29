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
		{path: "skills/sdd-verify/SKILL.md", wants: []string{"all tasks are complete", "actual requirements and scenarios", "test_output_hash", "build_output_hash", "model/provider/profile"}},
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
	for _, want := range []string{"reviewReceipt", "scope-changed", "exact persisted transaction", "bare envelope never passes"} {
		if !strings.Contains(statusContract, want) {
			t.Errorf("status contract missing %q", want)
		}
	}
}

func TestSDDVerifyConsumesPreterminalInputsWithoutCircularTerminalArtifacts(t *testing.T) {
	content := assets.MustRead("skills/sdd-verify/SKILL.md")
	for _, want := range []string{
		"authoritative preterminal transaction plus the preserved policy and canonical ledger preimages",
		"Do not require `receipt.json`, `chain-bundle.json`, `gate-context.json`",
		"exact canonical verification-evidence bytes, not only their hash",
		"hashes cannot reconstruct artifact content",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("sdd-verify missing non-circular final-verification clause %q", want)
		}
	}
	if strings.Contains(content, "(`reviews/transaction.json`, `ledger.json`, `receipt.json`, `gate-context.json`") {
		t.Fatal("sdd-verify still requires terminal receipt and gate context before final verification")
	}
}
