package assets

import (
	"strings"
	"testing"
)

// These assertions inspect the shipped instruction contract, not model obedience.
func TestPortableBranchPRRepositoryPolicy(t *testing.T) {
	text := MustRead("skills/branch-pr/SKILL.md")
	for _, rule := range []struct {
		name  string
		terms []string
	}{
		{"policy ownership", []string{"destination repository's contribution policy", "If policy does not require an issue, proceed without one"}},
		{"fork destination", []string{"PR base repository", "not the fork's origin"}},
		{"conditional labels and conventions", []string{"Never invent a label", "branch and commit conventions", "soft defaults, not gates"}},
		{"destination template", []string{"destination's current PR template", "Keep each required template checklist item separate"}},
	} {
		t.Run(rule.name, func(t *testing.T) {
			taxonomyTerms(t, text, rule.terms...)
		})
	}
	for _, universal := range []string{
		"Every PR MUST visibly link an approved base-repository issue",
		"Every PR MUST have exactly one `type:*` label",
		"Blank PRs without issue linkage will be blocked",
		"Branch names MUST match this regex",
		"Commit messages MUST match this regex",
	} {
		if strings.Contains(text, universal) {
			t.Errorf("portable skill imposes repository-specific gate %q", universal)
		}
	}
	taxonomyTerms(t, text, "name: branch-pr\n", "current direct human instruction", "native RDD consent")
}

func TestIssueCreationRepositoryFormats(t *testing.T) {
	text := MustRead("skills/issue-creation/SKILL.md")
	for _, scenario := range []struct {
		name  string
		terms []string
	}{
		{"required YAML form", []string{"When the target uses YAML Issue Forms", "Never bypass a required form", "omit `markdown` guidance"}},
		{"Markdown template", []string{"When the target uses a Markdown template", "preserve its required structure"}},
		{"no issue template", []string{"When the target has no issue template", "do not block publication solely because YAML forms are absent", "labels permitted by repository policy"}},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			taxonomyTerms(t, text, scenario.terms...)
		})
	}
	if strings.Contains(text, "Never use Markdown, a blank body") {
		t.Error("portable issue creation must not reject the target's supported format")
	}
}

func TestRepositoryPRPolicyPreserved(t *testing.T) {
	text := taxonomyRead(t, "skills/branch-pr/SKILL.md")
	taxonomyTerms(t, text, "name: gentle-ai-branch-pr\n",
		"Every PR MUST visibly link an approved base-repository issue",
		"status:approved", "CI rejects zero or multiple type labels",
		"Branch names **must** match this pattern", "Commit messages **must** match this pattern")
}
