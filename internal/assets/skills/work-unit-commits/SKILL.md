---
name: work-unit-commits
description: "Plan commits as reviewable work units. Trigger: implementation, commit splitting, chained PRs, or keeping tests and docs with code."
license: Apache-2.0
metadata:
  author: gentleman-programming
  version: "1.0"
---

## When to Use

Load this skill when deciding what belongs in each commit or PR.

Use it for:

- Splitting a feature into reviewable work.
- Preparing commits before opening a PR.
- Turning a large change into chained or stacked PRs.
- Keeping reviewer cognitive load healthy.
- Committing ODD work units in reviewable slices.

## Critical Rules

| Rule | Requirement |
|------|-------------|
| Commit by work unit | A commit represents a deliverable behavior, fix, migration, or docs unit. |
| Do not commit by file type | Avoid `models`, then `services`, then `tests` if none works alone. |
| Keep tests with code | Tests belong in the same commit as the behavior they verify. |
| Keep docs with the user-visible change | Docs belong with the feature or workflow they explain. |
| Tell a story | A reviewer should understand why each commit exists from its diff and message. |
| Future PR-ready | Each commit should be a candidate chained PR when the change grows. |
| Budget is not code-golf | Never shrink a diff by deleting comments, blank lines, docs, or tests, or by compressing code, to fit the review budget (400 by default, or the session `review_budget_lines`). Slice by work unit or report the overage. |

## Work Unit Checklist

Before committing, confirm:

- [ ] The commit has one clear purpose.
- [ ] The repo still makes sense after applying only this commit.
- [ ] Tests or docs for this unit are included when relevant.
- [ ] Rollback is reasonable without reverting unrelated work.
- [ ] Focused test command and exact result are recorded.
- [ ] Runtime harness command/scenario and exact result are recorded, or explicit `N/A` explains why no runtime boundary exists.
- [ ] Rollback boundary names the exact files/behavior removable without unrelated work.
- [ ] The commit message explains the outcome, not the file list.

## Split Examples

| Weak split | Better work-unit split |
|------------|------------------------|
| `add models` | `feat(auth): add token validation domain model and tests` |
| `add services` | `feat(auth): wire token validation into login flow` |
| `add tests` | Tests included with each behavior commit |
| `update docs` | Docs included with the user-facing change they explain |

## PR Relationship

Use work-unit commits as the foundation for chained PRs:

1. Build the smallest independent work unit.
2. Include verification for that unit.
3. Commit it with a Conventional Commit message.
4. If the PR approaches 400 changed lines, promote commits or groups of commits into chained PRs.

## ODD Relationship

Organic Driven Development (ODD) commits substantial work in reviewable slices on the feature branch, not one commit per task:

- Commit work units in reviewable slices of about 400 authored changed lines, branch first when on the default branch, with tests and docs alongside the behavior and a Conventional Commit message. A unit whose Risk line names a high-risk item gets its own commit so native review can take it alone, and the last slice closes the feature.
- The native review candidate is that commit, or the PR slice it belongs to when review is deferred, against the previous reviewed boundary. It is never a TODO checkbox and never the accumulated feature branch.
- The running authored line count from work-unit commits feeds the delivery-strategy vocabulary: `ask-on-risk`, `auto-chain`, `single-pr`, `exception-ok`.
- The ODD feature document records the commit that holds each task as evidence and, once a delivery strategy applies, the chosen chain strategy and slice boundaries (which commits each PR holds).

## Commands

```bash
# Review the story before committing
git diff --stat
git diff --cached --stat

# Check recent commit style
git log --oneline -5
```
