---
name: gentle-ai-collab-perfect
description: "Trigger: contributing to Gentleman-Programming/gentle-ai as an external collaborator. Strict issue-first workflow, honest PR bodies, contributor-vs-maintainer scope, chained-PR strategy, verification protocol, docstring coverage. Load whenever the active repo is Gentleman-Programming/gentle-ai and any part of the contribution flow is in scope: opening an issue, drafting or editing a PR body, splitting a change into chained/stacked PRs, or auditing a PR before requesting review."
license: Apache-2.0
metadata:
  author: ardelperal
  version: "0.1"
---

## When to use

Use this skill when the active repo is `Gentleman-Programming/gentle-ai` and the contributor is an external collaborator (not the maintainer). Scope of the skill:

- Opening or commenting on issues
- Drafting, editing, or auditing PR bodies
- Choosing a chained-PR strategy (Stacked vs Feature Branch Chain)
- Cross-checking PR claims against the GitHub API
- Deciding whether an action is contributor-scope or maintainer-scope

Do NOT load this skill for:

- Using gentle-ai as an installer (Gentleman-Programming/gentle-ai is the installer itself; not this skill)
- Reading docs, debugging tests, or reviewing the codebase in general
- Tasks on a different repository

The skill assumes the contributor is working from wherever they push — a fork, a personal working repo, or anywhere they have write access. **It deliberately does not assume a fork.** Use it whether you push to `ardelperal/gentle-ai`, to a personal fork, or to a contributor org.

---

## Source of truth — read the repo, don't infer

Before recommending any contribution action, read the relevant local file. The repo documents every constraint; pulling rules verbatim beats guessing.

| File | What it tells you |
|---|---|
| `CONTRIBUTING.md` | Issue-first workflow, label taxonomy, branch naming regex `^(feat\|fix\|chore\|docs\|style\|refactor\|perf\|test\|build\|ci\|revert)\/[a-z0-9._-]+$`, Conventional Commits format, 400-line review budget |
| `.github/PULL_REQUEST_TEMPLATE.md` | Required PR body sections (Linked Issue, PR Type, Summary, Changes, Test Plan, Automated Checks, Contributor Checklist, Notes for Reviewers) |
| `.github/ISSUE_TEMPLATE/bug_report.yml` | Bug report structure + `status:needs-review` auto-label + `status:approved` blocking gate |
| `.github/ISSUE_TEMPLATE/feature_request.yml` | Feature request structure + same gates |
| `.github/workflows/pr-check.yml` | Automated gates: `Check Issue Reference`, `Check Issue Has status:approved`, `Check PR Has type:* Label`, `Check PR Cognitive Load` |
| `.github/ISSUE_TEMPLATE/config.yml` | Issue-template routing rules; do not bypass |
| `skills/branch-pr/SKILL.md` | Branch + PR creation mechanics |
| `skills/chained-pr/SKILL.md` | Chained vs Stacked PR strategy mechanics |
| `skills/issue-creation/SKILL.md` | Issue creation mechanics |
| `skills/cognitive-doc-design/SKILL.md` | Doc-writing principles |
| `skills/comment-writer/SKILL.md` | Tone for comment replies |

These files evolve. Re-read them at the start of every contribution.

---

## Hard rules (do not negotiate)

1. **Issue-first is mandatory.** No PR opens without an issue that already has `status:approved` from a maintainer. Enforced by `pr-check.yml` and CONTRIBUTING.md.
2. **Use `Closes/Fixes/Resolves #N`** in the PR body. `Refs #N` does NOT satisfy `Check Issue Reference`. Verified empirically on this repo.
3. **Exactly one `type:*` label per PR.** Two `type:*` labels fail the check. No `type:*` label fails the check.
4. **400-line budget per PR** (`additions + deletions`). Above that, request `size:exception` from a maintainer with rationale documented in the PR body.
5. **No `Co-Authored-By` trailers** on commits. AI attribution is not acceptable in this repo.
6. **No force-push to `main`.** It is protected.
7. **PR body checkboxes must reflect API state.** If `gh pr view --json labels` shows `labels: []`, do not check the "type:* added" box — write a `## Pending maintainer actions` section instead.
8. **PR titles follow `^(type)(\(scope\))?!?: <description>`** with exactly **one scope** (no comma). See `skills/branch-pr/SKILL.md` for the regex.
9. **Pre-existing test failures are named honestly.** This repo has pre-existing failures in `pi_codegraph`, `tui/sync`, and similar packages. Acknowledging them with the verification method (e.g. `git stash` baseline) is mandatory. Claiming "all tests pass" without that context is dishonest.

---

## Contributor vs maintainer scope

This split catches external contributors most often. Verify with `gh` before recommending any action that requires elevated permissions.

| Action | Contributor | Maintainer |
|---|---|---|
| Open / comment on issues | ✅ | — |
| Open a PR from a working branch (wherever they push from) | ✅ | — |
| Edit own PR body | ✅ | — |
| Push commits to own branches | ✅ | — |
| Add `status:approved` to an issue | ❌ | ✅ |
| Apply `type:*` label to a PR | ❌ | ✅ |
| Apply `size:exception` label | ❌ | ✅ |
| Approve `action_required` fork-PR workflows (fork approval gate) | ❌ | ✅ |
| Review a PR (approve / request changes) | ❌ | ✅ |
| Merge a PR | ❌ | ✅ |
| Push slice/* branches to upstream so cross-fork base refs work | ❌ | ✅ |

If the contributor's PR is "stuck" on something, the next step is almost always a maintainer action — not the contributor trying it. Trying it surfaces GraphQL 403 (`AddLabelsToLabelable`, `requestReviewsByLogin`, deployment-protection API) and makes the contributor look unaware of the workflow.

**Verified empirically** in this repo (July 2026): `gh pr edit --add-label type:feature` returns **403** `ardelperal does not have the correct permissions to execute AddLabelsToLabelable`. Treat that error as policy, not as a bug.

---

## Issue workflow

End-to-end steps to open an issue that becomes a viable PR.

1. **Search first.** Confirm the issue does not already exist (`gh search issues` or the issue list). Avoid duplicates — `CONTRIBUTING.md` says PRs on duplicates will be rejected.
2. **Pick the template.** `.github/ISSUE_TEMPLATE/bug_report.yml` for bugs, `feature_request.yml` for changes. Each auto-applies `status:needs-review` via the template's `labels:` field.
3. **Fill every required field.** Don't skip the Pre-flight Checklist. The two self-attestations (not a duplicate; understand `status:approved` is required) are present for a reason.
4. **After submission, watch for `status:approved`.** Comment clarifications promptly. This is the gate before any PR opens.
5. **If the maintainer asks for technical sub-slices** (e.g. `slice/journal-cas`, `slice/four-layer-engine`, `slice/tui-cli`), keep them as technical slices of one approved issue. The umbrella issue keeps `status:approved`; the sub-issues should also get approved if you want `Check Issue Has status:approved` to pass per slice.

---

## PR workflow

End-to-end steps once the issue (or chain of sub-issues) is approved.

1. **Branch.**
   ```
   git checkout main && git pull
   git checkout -b <type>/<short-description>
   ```
   Branch name matches the regex in `CONTRIBUTING.md`. `<short-description>` is kebab-case, max a few words.

2. **Implement.** Work-unit commits: each commit is one deliverable unit with its code + tests + docs. Keep rollback reasonable — reverting one commit should not remove unrelated work.

3. **Local validation.**
   - `go build ./...` clean
   - `go vet ./...` clean
   - `go test ./internal/...` with pre-existing failures acknowledged via `git stash` baseline. The repo has known pre-existing failures in `internal/components/communitytool/pi_codegraph`, `internal/tui/sync`, and similar — confirm they exist with `git stash` AND `git stash pop`, then name them in the PR body's Test Plan.

4. **Open the PR with `gh pr create`.** Body matches `.github/PULL_REQUEST_TEMPLATE.md`:
   - `## 🔗 Linked Issue` → `Closes #N` (or `Fixes`/`Resolves`). Never `Refs`.
   - `## 🏷️ PR Type` → exactly one `[x] type:*` matching the actual type
   - `## 📝 Summary` → one paragraph: what + why
   - `## 📂 Changes` → file table with line counts from `gh pr view --json additions,deletions,changedFiles`
   - `## 🧪 Test Plan` → every `go test` command actually run + result, **including pre-existing failures with verification method**
   - `## ✅ Contributor Checklist` → `[x]` only where verifiable; `[ ] pending maintainer` for the rest
   - `## 💬 Notes for Reviewers` → chain position, merge order, blockers

5. **For chained PRs**, add a brief dependency diagram showing your sibling PRs and the merge order. Mark the current PR with `📍`.

6. **After opening:** check `gh pr checks --json` and address any `pending contributor follow-up` items you documented in the body.

---

## PR body honesty — the single most often-abused rule

Every `[x]` in the Contributor Checklist is a public claim that `pr-check.yml` and the maintainer will verify. Three rules:

1. **Mark `[x]` only when the assertion is true against the API.**
   ```bash
   gh pr view <N> --json labels,closingIssuesReferences,additions,deletions,changedFiles
   ```
   If `labels: []`, do not check the "type:* added" box. Instead:
   ```markdown
   ## Pending maintainer actions

   The following are **maintainer-applied per `pr-check.yml` and CONTRIBUTING.md** in this repo — not within contributor scope:

   - [ ] `type:feature` label applied to this PR — pending Alan
   - [ ] `size:exception` consideration — see rationale below
   - [ ] Fork workflow approval — 4 runs in `action_required` awaiting Alan
   ```

2. **Numbers and file counts must match the API.** If you can't trust the API, recount locally: `git diff --stat <base>..<head>`.

3. **Pre-existing failures must be named and methodology stated.** "Tests pass" is dishonest if four packages fail in the `git stash`-absent baseline. State them:
   ```markdown
   > **Known pre-existing failures (not blocking this PR):** `internal/components/communitytool/pi_codegraph`, `internal/tui/sync`, `internal/tui/sddmode` clusters present on `main` are not introduced by this slice; verified identical via `git stash` baseline.
   ```

Honest rewrites often look like **adding** content, not removing. Adding `## Pending maintainer actions` and a quoted callout for pre-existing failures is normal and expected.

### Standard honest-rewrite pattern

When the contributor checks a box that doesn't reflect reality:
- Move the unfulfillable contributor-side check into `## Pending maintainer actions` with `pending Alan` suffix
- Update diff numbers to match the API exactly; if a qualifier is needed (e.g. "slice-specific commits only"), state it explicitly
- For test claims, state the exact local commands run and the count of PASS/SKIP observed; don't aggregate to "all pass" if any were skipped on Windows
- Always name pre-existing repo failures that the contributor observed but did not fix, with the verification method (`git stash` baseline)
- For process claims (e.g. "blind dual review approved"), reference the artifact (round-1 → round-2 polish commits ARE that evidence)

---

## Chained PR strategy

This repo supports two strategies via `gentle-ai-chained-pr`:

### Stacked to main

Use when **each slice can land independently**. Branches are independent stacks that each target `main`. The diff "pollutes" with previous-slice commits because GitHub does not allow cross-fork base refs (slice branches live only in the contributor's working repo).

- Pros: simple, each PR is its own atomic change.
- Cons: large chained diffs; reviewers must mentally isolate slice-specific changes; needs `size:exception` for anything past 400 lines of slice-specific additions.

### Feature Branch Chain (tracker PR)

Use when **the feature integrates as one atomic unit**. The maintainer pushes the slice/* branches to the upstream as `branch` (not PR); child PR #1 targets the tracker branch, child #2 targets the child #1 branch, child #3 targets child #2. The tracker PR stays draft / no-merge until all children are reviewed and merged.

- Pros: clean per-slice diffs; atomic integration; reversible as a unit.
- Cons: requires maintainer cooperation to push slice branches upstream; slower.

**Critical limitation:** GitHub does NOT support cross-fork base refs. If your slice branches live only in your working repo, the only options are:
- Stacked to main (with polluted diffs) — accept the reality
- Ask the maintainer to push slice branches upstream so you can do Feature Branch Chain

The maintainer is the only one who can make Feature Branch Chain work; the contributor alone cannot force it.

---

## Verification protocol

Before recommending any action that touches permissions, label state, or commit history:

1. **Dry-verify with `gh` first.** Attempt the action and surface the error if it fails. Example:
   ```
   $ gh pr edit 1132 --add-label type:feature
   → GraphQL: ardelperal does not have the correct permissions to execute `AddLabelsToLabelable`
   ```
   That output IS your answer. Don't try to work around it.

2. **Cross-check PR body claims against the GitHub API.**
   ```bash
   gh pr view <N> --json \
     labels,closingIssuesReferences,additions,deletions,changedFiles,\
     headRefName,baseRefName,isCrossRepository,headRepository,maintainerCanModify,\
     reviewDecision,statusCheckRollup
   ```

3. **Cross-check the linked issue state.**
   ```bash
   gh issue view <N> --json number,title,state,labels,comments
   ```

4. **After applying a body rewrite**, round-trip the body and confirm `closingIssuesReferences` is populated for the linked issue:
   ```bash
   gh pr view <N> --json body --jq '.body'             # round-trip
   gh pr view <N> --json closingIssuesReferences        # confirm linkage parsed
   ```

5. **Trust the contributor's lived permissions over inferred defaults.** If they say "I can only do X", route everything else to the maintainer — don't waste their PR review budget on GraphQL 403s.

6. **Always run the actual test command before claiming it passes.** "Tests pass" must reflect `go test ./path/to/pkg -v` output, not hope.

---

## Pre-PR self-audit checklist

Run this in your head (or print and tick) before requesting review:

- [ ] Linked issue has `status:approved` (and the linked PR uses `Closes/Fixes/Resolves`)
- [ ] PR title follows `^(type)(\(single-scope\))?!?: <description>` — no comma in scope
- [ ] Body uses `Closes/Fixes/Resolves #N`, not `Refs`
- [ ] Line counts in `## 📂 Changes` match `gh pr view --json additions,deletions,changedFiles`
- [ ] No `[x]` claims contradict what the API shows; moves maintainer-applied actions to `## Pending maintainer actions`
- [ ] Pre-existing failures named with verification method
- [ ] Conventional Commits in title and commit messages
- [ ] No `Co-Authored-By` trailers
- [ ] Branch name matches the regex in CONTRIBUTING.md
- [ ] Commits are work-unit-sized (one deliverable per commit)
- [ ] Chained-slice strategy agreed with the maintainer (Stacked vs Feature Branch Chain)
- [ ] Docstring coverage on exported items in the diff ≥80% (CodeRabbit pre-merge check)
- [ ] `go build ./...` clean
- [ ] `go vet ./...` clean
- [ ] Local test run with pre-existing failures acknowledged

---

## Anti-patterns

| Anti-pattern | Symptom | Fix |
|---|---|---|
| "type:* added" checkbox while `labels: []` | CodeRabbit or maintainer catches the lie on first read | Move to `## Pending maintainer actions` with `pending Alan` |
| `Refs #N` instead of `Closes #N` | `Check Issue Reference` fails; PR auto-rejected | Use `Closes`/`Fixes`/`Resolves` keyword |
| `[x] PR stays within 400 changed lines` for a 3,200-line PR | `Check PR Cognitive Load` fails; `size:exception` not requested | Compute real totals, document `size:exception` rationale in Pending maintainer section |
| `feat(tui,cli): wire...` title | Title fails the single-scope regex | Use one of `feat(tui): ...`, `feat(cli): ...`, `feat(tui-cli): ...` (dash, not comma) |
| Slice branches all base on `main` with stale carry-over commits | Reviewers can't isolate slice-specific changes; `size:exception` needed | Accept Stacked to main (request exception) OR ask maintainer to push slice branches upstream and use Feature Branch Chain |
| Trying `gh pr edit --add-label` as contributor | GraphQL 403 `AddLabelsToLabelable` | Stop, ask the maintainer via comment on the issue |
| Burning reviewer attention on smoke-test green | Low-cardinality tests pass without exercising the behavior; reviewer flags in CodeRabbit | Each test asserts specific behavior, not just non-panicking |
| Pretending local test run = `go test ./...` clean | Repo has pre-existing failures (`pi_codegraph`, `tui/sync`); saying "all tests pass" is dishonest | Run with `git stash` baseline, name pre-existing failures explicitly |
| Calling a working repo a "fork" in code or docs | Misrepresents the contributor's relationship to the upstream | Use neutral language: "your working branch", "the contributor's push location", not "your fork" |

---

## How to apply this skill — workflow for the AI assistant

When you (the AI assistant) are helping a contributor with anything that touches this repo:

1. **Before recommending any action**, look up the relevant `CONTRIBUTING.md` / template / workflow section and cite it.
2. **Before any label, status, merge, or fork-workflow approval**, check the contributor-vs-maintainer scope table; if maintainer-only, route to the maintainer with a comment or note — do NOT suggest the contributor try it.
3. **Before recommending body rewrites**, fetch the PR's current state and cross-check every claim against the API.
4. **Before recommending chained-PR structure**, ask the contributor whether each slice can land independently (Stacked) or needs atomic integration (Feature Branch Chain). Be explicit about the cross-fork base-ref limitation.
5. **Always** use Conventional Commits in title and commit messages.

When in doubt, the right move is more verification, less action.

---

## References

- `CONTRIBUTING.md` — full workflow, label taxonomy, branch naming, commit format, review budget.
- `.github/PULL_REQUEST_TEMPLATE.md` — PR body structure.
- `.github/ISSUE_TEMPLATE/bug_report.yml`, `feature_request.yml` — issue body structures.
- `.github/workflows/pr-check.yml` — automated gates.
- `skills/branch-pr/SKILL.md` — branch + PR creation mechanics in detail.
- `skills/chained-pr/SKILL.md` — chained vs stacked PR strategy mechanics in detail.
- `skills/issue-creation/SKILL.md` — issue creation mechanics in detail.
- `skills/cognitive-doc-design/SKILL.md` — doc-writing principles (low cognitive load).
- `skills/comment-writer/SKILL.md` — tone and structure for PR comments and issue replies.
- `skills/work-unit-commits/SKILL.md` — splitting commits for review-friendly PRs.

GitHub Actions docs for the `action_required` gate used in fork PRs: https://docs.github.com/actions/managing-workflow-runs/approving-workflow-runs-from-public-forks
