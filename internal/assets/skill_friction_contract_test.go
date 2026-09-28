package assets

import (
	"os"
	"strings"
	"testing"
)

func TestBranchPRAndCollaborationDecisionBoundaries(t *testing.T) {
	public, err := os.ReadFile("../../skills/branch-pr/SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	collab, err := os.ReadFile("../../skills/gentle-ai-collab-perfect/SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	for name, text := range map[string]string{
		"public branch PR":   string(public),
		"embedded branch PR": MustRead("skills/branch-pr/SKILL.md"),
		"collaboration":      string(collab),
	} {
		t.Run(name, func(t *testing.T) {
			for _, marker := range []string{
				"remote destination", "credential/session", "Before any target-host read",
				"fresh target-bound", "Refs #N", "human-selected", "REQUIRED",
				"rulesets/branch protection", "CodeRabbit", "MAINTAIN", "ADMIN",
				"current direct human instruction", "size:exception", "comparable isolated clean base",
				"native RDD consent",
			} {
				if !strings.Contains(text, marker) {
					t.Errorf("missing decision boundary %q", marker)
				}
			}
			for _, contradiction := range []string{
				"Refs #N does NOT satisfy", "Never `Refs`", "git stash", "git pull",
				"git push -u origin", "gh pr create --title", "All automated checks must pass",
			} {
				if strings.Contains(text, contradiction) {
					t.Errorf("unsafe instruction %q", contradiction)
				}
			}
		})
	}
}

func TestBranchPRDraftCountsAreProvisionalUntilAuthorizedReadback(t *testing.T) {
	content, err := os.ReadFile("../../skills/gentle-ai-collab-perfect/SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	text := string(content)
	start := strings.Index(text, "4. **Draft the PR body before PR creation.**")
	end := strings.Index(text, "5. **For chained PRs**")
	if start < 0 || end <= start {
		t.Fatal("missing pre-creation drafting step")
	}
	draft := text[start:end]
	for _, phrase := range []string{"## 🤖 AI Assistance", "None", "Material assistance used", "tool/model", "material scope", "verification performed", "AI-assistance option"} {
		if !strings.Contains(draft, phrase) {
			t.Errorf("PR draft omits required AI declaration guidance %q", phrase)
		}
	}
	if strings.Contains(draft, "[x] **None**") || strings.Contains(draft, "[x] **Material assistance used**") {
		t.Error("PR draft must not preselect an unverified AI-assistance declaration")
	}
	if strings.Contains(draft, "line counts from `gh pr view") {
		t.Error("pre-creation draft cannot rely on a nonexistent PR API response")
	}
	for _, phrase := range []string{"local diff", "provisional", "post-publication", "separately authorized edit", "never invent"} {
		if !strings.Contains(text, phrase) {
			t.Errorf("missing lifecycle guidance %q", phrase)
		}
	}
}

func TestBranchPRChecklistUsesEvidenceAppropriateToClaim(t *testing.T) {
	content, err := os.ReadFile("../../skills/gentle-ai-collab-perfect/SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	text := string(content)
	section := strings.SplitN(text, "## PR body honesty", 2)[0]
	if !strings.Contains(section, "Changes, AI Assistance, Test Plan") {
		t.Error("template section inventory must name AI Assistance")
	}
	start := strings.Index(text, "## PR body honesty")
	end := strings.Index(text, "2. **Counts follow the lifecycle.")
	if start < 0 || end <= start {
		t.Fatal("missing checklist evidence guidance")
	}
	guidance := text[start:end]
	if strings.Contains(guidance, "only when the assertion is true against the API") {
		t.Error("API evidence cannot establish local tests or an AI declaration")
	}
	for _, phrase := range []string{"API-backed PR state", "locally observed test evidence", "honest human/agent AI declaration", "Do not precheck"} {
		if !strings.Contains(guidance, phrase) {
			t.Errorf("missing distinct checklist evidence guidance %q", phrase)
		}
	}
}

func TestBranchPRGuidanceDoesNotOverrideHumanOrEvidence(t *testing.T) {
	public, err := os.ReadFile("../../skills/branch-pr/SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	collab, err := os.ReadFile("../../skills/gentle-ai-collab-perfect/SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	texts := map[string]string{
		"public":        string(public),
		"embedded":      MustRead("skills/branch-pr/SKILL.md"),
		"collaboration": string(collab),
	}
	for name, text := range texts {
		t.Run(name, func(t *testing.T) {
			for _, forbidden := range []string{
				"linked PR uses `Closes/Fixes/Resolves`", "rationale and verified policy authority",
				"All boxes must be checked:", "- [x] Scripts run without errors",
				"gh pr create \\\n", "gh pr view <N> --json", "gh issue view <N> --json",
			} {
				if strings.Contains(text, forbidden) {
					t.Errorf("unconditional or untargeted guidance %q", forbidden)
				}
			}
		})
	}
	for _, text := range []string{string(public), string(collab)} {
		if !strings.Contains(text, "verified policy authority means") || !strings.Contains(text, "not separate target-host proof") {
			t.Error("protected-label authority must define its actor/instruction evidence without an extra identity gate")
		}
	}
	if strings.Contains(string(public), "- [x] PR is linked") {
		t.Error("public template cannot precheck issue approval")
	}
	if !strings.Contains(string(collab), "before PR creation") {
		t.Error("collaboration workflow must require authorization before PR creation")
	}
	if !strings.Contains(string(collab), "human-selected closing or non-closing") {
		t.Error("collaboration checklist must preserve non-closing intent")
	}
}
