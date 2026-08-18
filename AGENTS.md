# Gentle AI — Agent Skills Index

When working on this project, load the relevant skill(s) BEFORE writing any code.

Naming convention: `gentle-ai-*` skills are repo-specific workflow skills. Unprefixed skills are portable writing or work-unit skills and intentionally keep their canonical names.

## How to Use

1. Check the trigger column to find skills that match your current task
2. Load the skill by reading the SKILL.md file at the listed path
3. Follow ALL patterns and rules from the loaded skill
4. Multiple skills can apply simultaneously

## Skills

| Skill | Trigger | Path |
|-------|---------|------|
| `issue-creation` | When creating a GitHub issue, reporting a bug, or requesting a feature. | [`internal/assets/skills/issue-creation/SKILL.md`](internal/assets/skills/issue-creation/SKILL.md) |
| `gentle-ai-branch-pr` | When creating a pull request, opening a PR, or preparing changes for review. | [`skills/legacy/branch-pr/SKILL.md`](skills/legacy/branch-pr/SKILL.md) |
| `gentle-ai-chained-pr` | When a change is too large for one review, or when creating chained/stacked pull requests. | [`skills/legacy/chained-pr/SKILL.md`](skills/legacy/chained-pr/SKILL.md) |
| `cognitive-doc-design` | When writing docs that must reduce cognitive load for readers or reviewers. | [`skills/legacy/cognitive-doc-design/SKILL.md`](skills/legacy/cognitive-doc-design/SKILL.md) |
| `comment-writer` | When drafting human comments, PR feedback, issue replies, or async updates. | [`skills/legacy/comment-writer/SKILL.md`](skills/legacy/comment-writer/SKILL.md) |
| `work-unit-commits` | When splitting implementation work into deliverable commits or chained PRs. | [`skills/legacy/work-unit-commits/SKILL.md`](skills/legacy/work-unit-commits/SKILL.md) |
| `rdd-defect-workflow` | When RDD defects involve receipts, authority, recovery, delivery gates, or kill switches. | [`skills/legacy/rdd-defect-workflow/SKILL.md`](skills/legacy/rdd-defect-workflow/SKILL.md) |
| `rdd-advisory-transport` | When changing reviewer transport, adapters, lens prompts/schemas, or transport capability policy. | [`skills/legacy/rdd-advisory-transport/SKILL.md`](skills/legacy/rdd-advisory-transport/SKILL.md) |
| `issue-root-resolution` | When auditing backlog roots, proposing cluster fixes, or closing resolved/outdated issues. | [`skills/legacy/issue-root-resolution/SKILL.md`](skills/legacy/issue-root-resolution/SKILL.md) |
| `systemic-issue-triage` | When triaging issues, bugs, backlogs, root causes, dead ends, or blocked users. | [`skills/legacy/systemic-issue-triage/SKILL.md`](skills/legacy/systemic-issue-triage/SKILL.md) |
| `gentle-ai-bench` | When touching `bench/`, journeys, driven mode, the journey corpus, or bench axes. | [`skills/legacy/gentle-ai-bench/SKILL.md`](skills/legacy/gentle-ai-bench/SKILL.md) |
| `dev-orchestrator` | When coordinating the SDD process, delegating to specialists, or managing human gates. | [`skills/agents/dev-orchestrator/SKILL.md`](skills/agents/dev-orchestrator/SKILL.md) |
| `dev-explorer` | When exploring a repository to analyze structure, dependencies, and risks. | [`skills/agents/dev-explorer/SKILL.md`](skills/agents/dev-explorer/SKILL.md) |
| `dev-proposer` | When proposing technical approaches, alternatives, and architecture decisions. | [`skills/agents/dev-proposer/SKILL.md`](skills/agents/dev-proposer/SKILL.md) |
| `dev-specifier` | When writing verifiable functional criteria using Given/When/Then format. | [`skills/agents/dev-specifier/SKILL.md`](skills/agents/dev-specifier/SKILL.md) |
| `dev-designer` | When defining technical design and architecture for specifications. | [`skills/agents/dev-designer/SKILL.md`](skills/agents/dev-designer/SKILL.md) |
| `dev-task-planner` | When decomposing specs and design into actionable tasks. | [`skills/agents/dev-task-planner/SKILL.md`](skills/agents/dev-task-planner/SKILL.md) |
| `backend-implementer` | When writing backend code for approved specs and design. | [`skills/agents/backend-implementer/SKILL.md`](skills/agents/backend-implementer/SKILL.md) |
| `frontend-implementer` | When writing frontend code for approved specs and design. | [`skills/agents/frontend-implementer/SKILL.md`](skills/agents/frontend-implementer/SKILL.md) |
| `database-specialist` | When handling complex DB migrations, schema changes, and high-risk DB tasks. | [`skills/agents/database-specialist/SKILL.md`](skills/agents/database-specialist/SKILL.md) |
| `dev-verifier` | When validating implementations strictly against tasks and specs. | [`skills/agents/dev-verifier/SKILL.md`](skills/agents/dev-verifier/SKILL.md) |
| `solution-architect` | When starting a new Greenfield project or module. | [`skills/agents/solution-architect/SKILL.md`](skills/agents/solution-architect/SKILL.md) |
| `project-bootstrap` | When initializing a new repository from an approved Blueprint. | [`skills/agents/project-bootstrap/SKILL.md`](skills/agents/project-bootstrap/SKILL.md) |
