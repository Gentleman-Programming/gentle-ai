# Chained PR Details

## Strategy Notes

| | Stacked PRs to main | Feature Branch Chain |
|---|---|---|
| Speed | Each slice can ship in order | Full feature waits for tracker merge |
| Rollback | Revert individual main PRs | Revert/hold the whole feature branch |
| Risk | Partial behavior may land | Nothing lands until the chain completes |
| Complexity | Simpler retarget/rebase flow | Requires tracker and strict diff hygiene |

## Feature Branch Chain

Use when the feature branch accumulates the final integration while child PRs are reviewed as focused slices.

```text
main
 └── feat/my-feature              ← tracker/final integration branch
      ↑ PR #1 base: feat/my-feature
      └── feat/my-feature-01-core
           ↑ PR #2 base: feat/my-feature-01-core
           └── feat/my-feature-02-shared
                ↑ PR #3 base: feat/my-feature-02-shared
                └── feat/my-feature-03-slice
```

Steps:

1. Create the feature/tracker branch from `main`.
2. Open the tracker PR to `main`; mark it draft/no-merge.
3. Create PR #1 from a child branch and target it to the tracker branch.
4. Create each later child branch from the previous PR branch and target it to that parent branch.
5. Merge/integrate children in order; merge the tracker only after the chain is complete.

## Stacked PRs to Main

Use when each slice can land on `main` in order.

```text
main <- PR 1: foundation
          └── PR 2: feature slice built on PR 1
                └── PR 3: docs/tests built on PR 2
```

After a parent PR merges, rebase/retarget the next PR so GitHub shows only the current slice.

## Chain Context Section

Append this section to the repo PR template; do not replace required issue/checklist sections.

```markdown
## Chain Context

| Field | Value |
|-------|-------|
| Chain | <feature or stack name> |
| Tracker PR | <#NNN or "Not needed"> |
| Position | <N of total> |
| Base | `<target branch>` |
| Depends on | <PR/issue/link or "None"> |
| Follow-up | <next PR or "None"> |
| Review budget | <changed lines> / 400 |
| Starts at | <branch, PR, or state this builds on> |
| Ends with | <standalone result delivered by this PR> |

### Chain Overview

```text
main
 └── #NNN Previous PR
      └── 📍 #NNN This PR
           └── #NNN Next PR
```

### Scope
- Includes: <focused unit>
- Excludes: <deferred work>

### Autonomy
- [ ] CI is expected to pass for this PR branch
- [ ] This PR has one deliverable scope
- [ ] This PR can be rolled back without unrelated changes
- [ ] Tests, docs, or manual verification cover this unit
```

## Commands

```bash
gh pr view <PR_NUMBER> --json additions,deletions,changedFiles,title,url
gh pr create --base feat/my-feature --title "feat(scope): focused slice" --body-file pr-body.md
gh pr create --base feat/my-feature-01-core --title "feat(scope): next focused slice" --body-file pr-body.md
```

## Reviewer Guidance

- Ask for a split when a PR exceeds 400 changed lines without `size:exception`.
- Recommend Feature Branch Chain when work must integrate before `main`.
- Recommend stacked PRs when each slice can merge independently.
- Review child PRs against immediate parent branches; a polluted diff is a branching bug.
