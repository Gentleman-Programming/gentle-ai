package reviewassets

import (
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v3/internal/assets"
	"github.com/gentleman-programming/gentle-ai/v3/internal/model"
)

func TestRuntimeContracts(t *testing.T) {
	for _, tc := range []struct {
		agent              model.AgentID
		required, excluded string
	}{
		{model.AgentPi, "`gentle_review_capture_group`", "gentle-ai review status"},
		{model.AgentClaudeCode, "gentle-ai review status --cwd <repo> --contract gentle-ai.review-integration/v2 --agent claude-code --next-transition", "gentle_review_capture_group"},
		{model.AgentOpenCode, "### OpenCode Concurrent Reviewer Group", "gentle_review_capture_group"},
	} {
		t.Run(string(tc.agent), func(t *testing.T) {
			got, err := ReviewExecutionContractFor(tc.agent)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(got, tc.required) || strings.Contains(got, tc.excluded) || strings.Contains(got, identityPlaceholder) {
				t.Fatalf("incorrect runtime contract for %s", tc.agent)
			}
		})
	}
	if _, err := ReviewExecutionContractFor(model.AgentKilocode); err == nil {
		t.Fatal("unsupported runtime accepted")
	}
}

func TestRenderedLensAssets(t *testing.T) {
	for _, path := range []string{
		"claude/agents/review-risk.md", "cursor/agents/review-reliability.md",
		"kimi/agents/review-readability.md", "kiro/agents/review-resilience.md",
	} {
		t.Run(path, func(t *testing.T) {
			source := assets.MustRead(path)
			got, ok := RenderReviewerAsset(path, source)
			if !ok || !strings.HasPrefix(got, source[:strings.Index(source, "\n---\n")+5]) {
				t.Fatal("review frontmatter changed")
			}
			for _, want := range []string{"## Candidate-Causal Admission", "## Scope", "## Output", "subject_hash", "GENTLE_AI_REVIEW_BINDING"} {
				if !strings.Contains(got, want) {
					t.Errorf("missing %q", want)
				}
			}
		})
	}
	source := "---\nname: jd-judge-a\n---\noriginal"
	if got, ok := RenderReviewerAsset("kiro/agents/jd-judge-a.md", source); ok || got != source {
		t.Fatal("Judgment Day unexpectedly rendered as review lens")
	}
	if got, ok := RenderReviewerAsset("claude/agents/review-refuter.md", source); ok || got != source {
		t.Fatal("refuter unexpectedly rendered as lens")
	}
}

func TestInspectionCommandsIndependent(t *testing.T) {
	first, second := InspectionCommands(), InspectionCommands()
	first[0] = "changed"
	if second[0] == "changed" {
		t.Fatal("inspection command slices share storage")
	}
}
