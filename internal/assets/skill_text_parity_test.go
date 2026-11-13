package assets

import (
	"strings"
	"testing"
)

// The work-unit review budget is about 400 authored changed lines; no
// session setting overrides it, so the skill must not name one.
func TestWorkUnitCommitsBudgetNamesNoSessionSetting(t *testing.T) {
	skill := MustRead("skills/work-unit-commits/SKILL.md")
	if !strings.Contains(skill, "to fit the review budget (about 400 authored changed lines)") {
		t.Error("work-unit-commits does not state the review budget as about 400 authored changed lines")
	}
	if strings.Contains(skill, "review_budget_lines") {
		t.Error("work-unit-commits still names the nonexistent review_budget_lines setting")
	}
}

// work-unit-commits loads for commit planning, not for every implementation.
func TestWorkUnitCommitsTriggerOmitsBareImplementation(t *testing.T) {
	const want = `description: "Plan commits as reviewable work units. Trigger: commit splitting, chained PRs, or keeping tests and docs with code."`
	for name, skill := range map[string]string{
		"embedded": MustRead("skills/work-unit-commits/SKILL.md"),
		"root":     readRepoRootFile(t, "skills/work-unit-commits/SKILL.md"),
	} {
		if !strings.Contains(skill, want+"\n") {
			t.Errorf("%s work-unit-commits description is not %s", name, want)
		}
		if strings.Contains(skill, "Trigger: implementation") {
			t.Errorf("%s work-unit-commits still triggers on bare implementation", name)
		}
	}
}

// The size example defers to the target repository's size policy instead of
// presenting a 400-line budget and size:exception as universal.
func TestCommentWriterSplitExampleDefersToRepoSizePolicy(t *testing.T) {
	skill := MustRead("skills/comment-writer/SKILL.md")
	if !strings.Contains(skill, "This PR is large for one review (about 600 changed lines). Could we split it, or follow the repo's size policy if it allows one PR?") {
		t.Error("comment-writer split example does not defer to the repo's size policy")
	}
	for _, old := range []string{"exceeds the 400-line budget", "justify `size:exception`"} {
		if strings.Contains(skill, old) {
			t.Errorf("comment-writer still contains %q", old)
		}
	}
}

// rdd-defect-workflow is for fixing RDD defects, not for using or toggling RDD.
func TestRDDDefectWorkflowDescriptionIsNarrow(t *testing.T) {
	skill := MustRead("skills/rdd-defect-workflow/SKILL.md")
	want := "name: rdd-defect-workflow\ndescription: \"Trigger: fixing RDD defects (review authority, receipts/lineage, correction/recovery, delivery gate). Not for using or toggling RDD.\"\n"
	if !strings.Contains(skill, want) {
		t.Error("rdd-defect-workflow lacks the narrow fixing-RDD-defects description under its own name")
	}
	for _, old := range []string{"Trigger: RDD, receipt-driven development", "Guide work."} {
		if strings.Contains(skill, old) {
			t.Errorf("rdd-defect-workflow still contains %q", old)
		}
	}
}

// The public comment-writer copy must match the embedded distribution asset.
// rdd-defect-workflow is already compared by TestRDDDefectWorkflowSkillContract.
func TestCommentWriterRootMatchesEmbedded(t *testing.T) {
	if readRepoRootFile(t, "skills/comment-writer/SKILL.md") != MustRead("skills/comment-writer/SKILL.md") {
		t.Fatal("skills/comment-writer/SKILL.md and the embedded asset differ")
	}
}
