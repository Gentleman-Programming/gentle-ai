package catalog

import "github.com/gentleman-programming/gentle-ai/v3/internal/model"

type Skill struct {
	ID       model.SkillID
	Name     string
	Category string
	Priority string
}

var mvpSkills = []Skill{
	// SDD skills
	{ID: model.SkillSDDInit, Name: "sdd-init", Category: "sdd", Priority: "p0"},

	{ID: model.SkillSDDApply, Name: "sdd-apply", Category: "sdd", Priority: "p0"},
	{ID: model.SkillSDDVerify, Name: "sdd-verify", Category: "sdd", Priority: "p0"},
	{ID: model.SkillSDDExplore, Name: "sdd-explore", Category: "sdd", Priority: "p0"},
	{ID: model.SkillSDDResearch, Name: "sdd-research", Category: "sdd", Priority: "p0"},
	{ID: model.SkillSDDPropose, Name: "sdd-propose", Category: "sdd", Priority: "p0"},
	{ID: model.SkillSDDSpec, Name: "sdd-spec", Category: "sdd", Priority: "p0"},
	{ID: model.SkillSDDDesign, Name: "sdd-design", Category: "sdd", Priority: "p0"},
	{ID: model.SkillSDDTasks, Name: "sdd-tasks", Category: "sdd", Priority: "p0"},
	{ID: model.SkillSDDArchive, Name: "sdd-archive", Category: "sdd", Priority: "p0"},
	{ID: model.SkillSDDOnboard, Name: "sdd-onboard", Category: "sdd", Priority: "p0"},
	// Foundation skills
	{ID: model.SkillGoTesting, Name: "go-testing", Category: "testing", Priority: "p0"},
	{ID: model.SkillGentleAIBench, Name: "gentle-ai-bench", Category: "testing", Priority: "p0"},
	{ID: model.SkillCreator, Name: "skill-creator", Category: "workflow", Priority: "p0"},
	{ID: model.SkillImprover, Name: "skill-improver", Category: "workflow", Priority: "p0"},
	{ID: model.SkillJudgmentDay, Name: "judgment-day", Category: "workflow", Priority: "p0"},
	{ID: model.SkillBranchPR, Name: "branch-pr", Category: "workflow", Priority: "p0"},
	{ID: model.SkillIssueCreation, Name: "issue-creation", Category: "workflow", Priority: "p0"},
	{ID: model.SkillSkillRegistry, Name: "skill-registry", Category: "workflow", Priority: "p0"},
	// Sustainable review skills
	{ID: model.SkillChainedPR, Name: "chained-pr", Category: "workflow", Priority: "p0"},
	{ID: model.SkillCognitiveDoc, Name: "cognitive-doc-design", Category: "workflow", Priority: "p0"},
	{ID: model.SkillCommentWriter, Name: "comment-writer", Category: "workflow", Priority: "p0"},
	{ID: model.SkillWorkUnitCommits, Name: "work-unit-commits", Category: "workflow", Priority: "p0"},
	{ID: model.SkillRDDDefectWorkflow, Name: "rdd-defect-workflow", Category: "workflow", Priority: "p0"},
	{ID: model.SkillSystemicIssueTriage, Name: "systemic-issue-triage", Category: "workflow", Priority: "p0"},
	// QA-Orchestrator skills
	{ID: model.SkillQASupervisor, Name: "qa-supervisor", Category: "qa", Priority: "p0"},
	{ID: model.SkillQAExplore, Name: "qa-explore", Category: "qa", Priority: "p0"},
	{ID: model.SkillQASpec, Name: "qa-spec", Category: "qa", Priority: "p0"},
	{ID: model.SkillQAApply, Name: "qa-apply", Category: "qa", Priority: "p0"},
	{ID: model.SkillQAVerify, Name: "qa-verify", Category: "qa", Priority: "p0"},
	{ID: model.SkillQADocs, Name: "qa-docs", Category: "qa", Priority: "p0"},
	{ID: model.SkillQALocatorHunting, Name: "qa-locator-hunting", Category: "qa", Priority: "p0"},
	{ID: model.SkillQADocReference, Name: "qa-doc-reference", Category: "qa", Priority: "p0"},
	{ID: model.SkillQADocAccess, Name: "qa-doc-access", Category: "qa", Priority: "p0"},
	{ID: model.SkillQAEvidence, Name: "qa-evidence", Category: "qa", Priority: "p0"},
}

func MVPSkills() []Skill {
	skills := make([]Skill, len(mvpSkills))
	copy(skills, mvpSkills)
	return skills
}
