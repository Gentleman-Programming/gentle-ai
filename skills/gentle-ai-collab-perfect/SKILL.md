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

## Source of truth — inspect the repo, don't infer

Before any target-host read, obtain explicit authorization for the remote destination (exact target), read operation and credential/session; never probe ambient credentials. Locally, before recommending any contribution action, inspect the relevant current source. The repo documents every constraint; pulling rules verbatim beats guessing.

| Source | What it tells you |
|---|---|
| `CONTRIBUTING.md` | Issue-first workflow, label taxonomy, branch naming regex `^(feat\|fix\|chore\|docs\|style\|refactor\|perf\|test\|build\|ci\|revert)\/[a-z0-9._-]+$`, Conventional Commits format, 400-line review budget |
| `.github/PULL_REQUEST_TEMPLATE.md` | Required PR body sections (Linked Issue, PR Type, Summary, Changes, AI Assistance, Test Plan, Automated Checks, Contributor Checklist, Notes for Reviewers) |
| `.github/ISSUE_TEMPLATE` | Current issue templates, forms, and routing policy |
| Discovered GitHub labels | Current label names and availability; do not infer them from this skill |
| `.github/workflows/pr-check.yml` | Automated gates: `Check Issue Reference`, `Check Issue Has status:approved`, `Check PR Has type:* Label`, `Check PR Cognitive Load` |
| `skills/branch-pr/SKILL.md` | Branch + PR creation mechanics |
| `skills/chained-pr/SKILL.md` | Chained vs Stacked PR strategy mechanics |
| `internal/assets/skills/issue-creation/SKILL.md` | Canonical issue discovery, drafting, privacy review, and publication authority |
| `skills/cognitive-doc-design/SKILL.md` | Doc-writing principles |
| `skills/comment-writer/SKILL.md` | Tone for comment replies |

These files evolve. Re-read them at the start of every contribution.

---

## Hard rules (do not negotiate)

1. **Issue-first is mandatory.** No PR opens without an issue that already has `status:approved` under the canonical issue-creation workflow contract. Enforced by `pr-check.yml` and CONTRIBUTING.md.
2. **Ask the human whether the PR should close the approved issue on merge.** Preserve the human-selected `Closes/Fixes/Resolves #N` closing intent or `Refs #N` non-closing intent; both can satisfy `Check Issue Reference`. Never infer or change closing intent.
3. **Ordinary `type:*` categorization** — zero or multiple labels fail the check. Route it through the canonical issue-creation workflow contract: a current direct human instruction binds the exact target/action, target-host capability is verified, and it uses one bounded mutation and target-host readback; otherwise wait without mutation.
4. **Protected policy labels** — adding or removing `status:approved` or `size:exception` requires authenticated actor target-host `viewerPermission` `MAINTAIN` or `ADMIN` and a current direct human instruction binding the exact target/action; here verified policy authority means that actor permission and exact direct instruction, not separate target-host proof of the instruction-giver's identity. Do not mutate automatically. `size:exception` additionally requires documented over-budget rationale and human choice.
5. **400-line budget per PR** (`additions + deletions`). Above that, `size:exception` additionally requires documented over-budget rationale.
6. **No `Co-Authored-By` trailers** on commits. AI attribution is not acceptable in this repo.
7. **No force-push to `main`.** It is protected.
8. **PR body checkboxes must reflect API state.** If `gh pr view --json labels` shows `labels: []`, do not check the "type:* added" box — record the pending canonical PR-label action instead.
9. **PR titles follow `^(type)(\(scope\))?!?: <description>`** with exactly **one scope** (no comma). See `skills/branch-pr/SKILL.md` for the regex.
10. **Pre-existing test failures require proof.** Compare the same failing command/environment against a comparable isolated clean base without disturbing user changes; otherwise report baseline unverified. Do not assume particular packages fail or claim all tests pass when they fail.

---

## Contributor vs maintainer scope

This split catches external contributors most often. Verify with `gh` before recommending any action that requires elevated permissions.

| Action | Contributor | Maintainer |
|---|---|---|
| Open / comment on issues | ✅ | — |
| Open a PR from a working branch (wherever they push from) | ✅ | — |
| Edit own PR body | ✅ | — |
| Push commits to own branches | ✅ | — |
| Apply/remove ordinary existing issue labels | Only under the canonical issue-creation workflow contract and target-host capability grant | Same; verify the target host grants the action |
| Apply/remove ordinary `type:*` PR categorization | Only under the canonical issue-creation workflow contract: current direct human instruction, exact target/action, target-host capability, one bounded mutation and target-host readback | Same; verify the target host grants the action |
| Add/remove protected `status:approved` or `size:exception` | Current direct human instruction for the exact target/action plus actor `MAINTAIN`/`ADMIN`; `size:exception` also needs documented rationale and human choice | Same exact direct instruction, actor capability, human choice and rationale |
| Approve `action_required` fork-PR workflows (fork approval gate) | ❌ | ✅ |
| Review a PR (approve / request changes) | ❌ | ✅ |
| Merge a PR | ❌ | ✅ |
| Push slice/* branches to upstream so cross-fork base refs work | ❌ | ✅ |

If a PR-label action lacks a current direct instruction or verified target-host capability, wait without mutation. Other maintainer-only actions remain maintainer-only.

---

## Issue workflow

Use the canonical `issue-creation` skill at `internal/assets/skills/issue-creation/SKILL.md` for duplicate discovery, template handling, privacy review, and publication. Apply Gentle AI's current repository policy from `CONTRIBUTING.md`, `.github/ISSUE_TEMPLATE`, and discovered GitHub labels rather than copying form fields, label names, or commands here.

After submission, return to this collaboration workflow for the contributor/maintainer boundary and the approved-issue gate before PR work. If a maintainer requests technical sub-slices, keep them within the approved issue structure required by the current repository policy and checks.

---

## PR workflow

End-to-end steps once the issue (or chain of sub-issues) is approved.

1. **Branch.**
   ```
   # Resolve the authorized target's default/base branch from current metadata.
   # Obtain human authorization before checkout or branch creation.
   ```
   Branch name matches the regex in `CONTRIBUTING.md`. `<short-description>` is kebab-case, max a few words.

2. **Implement.** Work-unit commits: each commit is one deliverable unit with its code + tests + docs. Keep rollback reasonable — reverting one commit should not remove unrelated work.

3. **Local validation.**
   - `go build ./...` clean
   - `go vet ./...` clean
   - `go test ./internal/...`: report observed failures. Attribute failures to the base only after the same command/environment reproduces on a comparable isolated clean base; otherwise mark baseline unverified.

4. **Draft the PR body before PR creation.** Obtain explicit human authorization for the PR creation operation, exact remote destination and credential/session before any target-host read or publication; drafting is not permission to publish. When authorized, use the current `.github/PULL_REQUEST_TEMPLATE.md`:
   - `## 🔗 Linked Issue` → human-selected closing (`Closes/Fixes/Resolves #N`) or non-closing (`Refs #N`) reference.
   - `## 🏷️ PR Type` → exactly one `[x] type:*` matching the actual type
   - `## 📝 Summary` → one paragraph: what + why
   - `## 📂 Changes` → before PR creation, use local diff counts for the intended base/head and label them provisional; do not claim post-publication API numbers yet.
   - `## 🤖 AI Assistance` → select exactly one of `None` or `Material assistance used` based on actual use; if material, complete applicable tool/model (if known), material scope and verification performed fields. Do not precheck either choice without evidence; follow `AI_POLICY.md`.
   - `## 🧪 Test Plan` → every `go test` command actually run + result; distinguish clean-base-proven failures from baseline unverified.
   - `## ✅ Contributor Checklist` → `[x]` only where verifiable, including the required AI-assistance option and applicable declaration fields; `[ ] pending maintainer` for the rest
   - `## 💬 Notes for Reviewers` → chain position, merge order, blockers

5. **For chained PRs**, add a brief dependency diagram showing your sibling PRs and the merge order. Mark the current PR with `📍`.

6. **After opening, only if PR creation was actually confirmed:** under explicit authorization for the exact remote destination, post-publication read operation and credential/session, read the PR's `additions,deletions,changedFiles` from the target API and reconcile the provisional local diff counts. A body correction needs a separately authorized edit for that exact PR; never invent a successful remote read, edit or creation. Reuse fresh target-bound issue approval, default branch, type-label and check observations. Read target branch rulesets/branch protection and current run status to identify REQUIRED CI. CodeRabbit pending is optional unless target policy requires it. Unknown requiredness is not merge-ready. Commit, push, PR, merge, chain strategy/exception and native RDD consent remain human-owned.

---

## PR body honesty — the single most often-abused rule

Every `[x]` in the Contributor Checklist is a public claim requiring evidence appropriate to that claim. Use API-backed PR state for issue linkage, labels and published counts; locally observed test evidence for test claims; and an honest human/agent AI declaration for the AI Assistance selection and applicable fields. Do not precheck any claim without its evidence. Three rules:

1. **For API-backed PR state, mark `[x]` only when the assertion is true against the authorized target API.**
   ```bash
   gh pr view <N> --repo "$TARGET" --json labels,closingIssuesReferences,additions,deletions,changedFiles
   ```
   If `labels: []`, do not check the "type:* added" box. Instead:
   ```markdown
   ## Pending repository workflow actions

   The following remain pending:

   - [ ] Ordinary `type:feature` categorization requires the canonical issue-creation workflow contract
   - [ ] Protected `size:exception` requires human choice, documented rationale, exact direct instruction and actor `MAINTAIN`/`ADMIN`
   - [ ] Fork workflow approval — 4 runs in `action_required` awaiting a maintainer
   ```

2. **Counts follow the lifecycle.** Before publication, calculate a local diff for the intended base/head (`git diff --stat <base>..<head>`) and label counts provisional. Only after a confirmed PR and separately authorized target-host read compare its API numbers; if different, request separate authorization before editing the body. If the API is unavailable, report the discrepancy or unknown state rather than claiming an API match.

3. **Pre-existing failures must be proven.** Compare the same command and environment against a comparable isolated clean base. Without comparison, state the observed failure and `baseline unverified`, not a presumed non-blocking base failure.

Honest rewrites often look like **adding** content, not removing. Adding `## Pending repository workflow actions` and a quoted callout for pre-existing failures is normal and expected.

### Standard honest-rewrite pattern

When the contributor checks a box that doesn't reflect reality:
- For a PR label, record its pending canonical workflow action; keep other maintainer-only actions distinct
- Update diff numbers to match the API exactly; if a qualifier is needed (e.g. "slice-specific commits only"), state it explicitly
- For test claims, state the exact local commands run and the count of PASS/SKIP observed; don't aggregate to "all pass" if any were skipped on Windows
- Name only clean-base-proven pre-existing failures with their comparable command/environment; otherwise mark baseline unverified.
- For process claims (e.g. "blind dual review approved"), reference the artifact (round-1 → round-2 polish commits ARE that evidence)

---

## Chained PR strategy

This repo supports two strategies via `gentle-ai-chained-pr`:

### Stacked to main

Use when **each slice can land independently**. Branches are independent stacks that each target the verified default/base branch. The diff "pollutes" with previous-slice commits because GitHub does not allow cross-fork base refs (slice branches live only in the contributor's working repo).

- Pros: simple, each PR is its own atomic change.
- Cons: large chained diffs; reviewers must mentally isolate slice-specific changes; needs `size:exception` for anything past 400 lines of slice-specific additions.

### Feature Branch Chain (tracker PR)

Use when **the feature integrates as one atomic unit**. The maintainer pushes the slice/* branches to the upstream as `branch` (not PR); child PR #1 targets the tracker branch, child #2 targets the child #1 branch, child #3 targets child #2. The tracker PR stays draft / no-merge until all children are reviewed and merged.

- Pros: clean per-slice diffs; atomic integration; reversible as a unit.
- Cons: requires maintainer cooperation to push slice branches upstream; slower.

**Critical limitation:** GitHub does NOT support cross-fork base refs. If your slice branches live only in your working repo, the only options are:
- Stacked to the verified default branch (with polluted diffs) — accept the reality
- Ask the maintainer to push slice branches upstream so you can do Feature Branch Chain

The maintainer is the only one who can make Feature Branch Chain work; the contributor alone cannot force it.

---

## Verification protocol

Before any target-host read in these examples, obtain explicit authorization for the exact destination, operation and credential/session. Set `$TARGET` to that verified destination; never infer it from the cwd or probe ambient credentials. Reuse fresh target-bound evidence. Before recommending any action that touches permissions, label state, or commit history:

1. **Verify authority before a PR-label action.** Use the canonical issue-creation workflow contract; do not probe a mutation to discover authority.

2. **Cross-check PR body claims against the GitHub API.**
   ```bash
   gh pr view <N> --repo "$TARGET" --json \
     labels,closingIssuesReferences,additions,deletions,changedFiles,\
     headRefName,baseRefName,isCrossRepository,headRepository,maintainerCanModify,\
     reviewDecision,statusCheckRollup
   ```

3. **Cross-check the linked issue state.**
   ```bash
   gh issue view <N> --repo "$TARGET" --json number,title,state,labels,comments
   ```

4. **After an authorized body rewrite**, round-trip the body on the authorized target. For closing keywords, confirm the issue appears in `closingIssuesReferences`; for non-closing `Refs #N`, verify the visible well-formed body reference and the approved base-repository issue. A non-closing reference need not appear in `closingIssuesReferences`. If linkage cannot be verified, report unknown; never replace `Refs` to manufacture proof.

5. **Trust the contributor's lived permissions over inferred defaults.** If they say "I can only do X", route everything else to the maintainer — don't waste their PR review budget on GraphQL 403s.

6. **Always run the actual test command before claiming it passes.** "Tests pass" must reflect `go test ./path/to/pkg -v` output, not hope.

---

## Pre-PR self-audit checklist

Run this in your head (or print and tick) before requesting review:

- [ ] Linked base-repository issue has `status:approved` and the PR preserves the human-selected closing or non-closing reference
- [ ] PR title follows `^(type)(\(single-scope\))?!?: <description>` — no comma in scope
- [ ] Body preserves the human-selected closing or non-closing reference
- [ ] Line counts in `## 📂 Changes` match `gh pr view --json additions,deletions,changedFiles`
- [ ] No `[x]` claims contradict what the API shows; moves pending actions to `## Pending repository workflow actions`
- [ ] Pre-existing failures named with verification method
- [ ] Conventional Commits in title and commit messages
- [ ] No `Co-Authored-By` trailers
- [ ] Branch name matches the regex in CONTRIBUTING.md
- [ ] Commits are work-unit-sized (one deliverable per commit)
- [ ] Chained-slice strategy agreed with the maintainer (Stacked vs Feature Branch Chain)
- [ ] Docstring coverage checked when applicable; CodeRabbit is not REQUIRED unless target policy says so
- [ ] `go build ./...` clean
- [ ] `go vet ./...` clean
- [ ] Local test run with pre-existing failures acknowledged

---

## Anti-patterns

| Anti-pattern | Symptom | Fix |
|---|---|---|
| "type:* added" checkbox while `labels: []` | CodeRabbit or maintainer catches the lie on first read | Record the pending canonical PR-label action |
| Assuming `Refs #N` fails | Overrides human non-closing intent | Preserve the human-selected form accepted by the parser |
| `[x] PR stays within 400 changed lines` for a 3,200-line PR | `Check PR Cognitive Load` fails; `size:exception` not requested | Compute real totals, document the human-selected exception rationale, actor `MAINTAIN`/`ADMIN` and current direct human instruction for the exact target/action in Pending repository workflow actions |
| `feat(tui,cli): wire...` title | Title fails the single-scope regex | Use one of `feat(tui): ...`, `feat(cli): ...`, `feat(tui-cli): ...` (dash, not comma) |
| Slice branches all base on the default branch with stale carry-over commits | Reviewers can't isolate slice-specific changes; `size:exception` needed | Accept Stacked to main (request exception) OR ask maintainer to push slice branches upstream and use Feature Branch Chain |
| Mutating a PR label without canonical authority | No current direct instruction or verified capability | Wait without mutation |
| Burning reviewer attention on smoke-test green | Low-cardinality tests pass without exercising the behavior; reviewer flags in CodeRabbit | Each test asserts specific behavior, not just non-panicking |
| Pretending local test run = `go test ./...` clean | Observed failures do not establish base causality | Compare in an isolated clean base or mark baseline unverified |
| Calling a working repo a "fork" in code or docs | Misrepresents the contributor's relationship to the upstream | Use neutral language: "your working branch", "the contributor's push location", not "your fork" |

---

## How to apply this skill — workflow for the AI assistant

When you (the AI assistant) are helping a contributor with anything that touches this repo:

1. **Before recommending any action**, look up the relevant `CONTRIBUTING.md` / template / workflow section and cite it.
2. **Before any label, status, merge, or fork-workflow approval**, check the contributor-vs-maintainer scope table. Route PR labels only through the canonical issue-creation workflow contract; route other maintainer-only actions to a maintainer without suggesting an unauthorized attempt.
3. **Before recommending body rewrites**, fetch the PR's current state and cross-check every claim against the API.
4. **Before recommending chained-PR structure**, ask the contributor whether each slice can land independently (Stacked) or needs atomic integration (Feature Branch Chain). Be explicit about the cross-fork base-ref limitation.
5. **Always** use Conventional Commits in title and commit messages.

When in doubt, the right move is more verification, less action.

---

## References

- `CONTRIBUTING.md` — full workflow, label taxonomy, branch naming, commit format, review budget.
- `.github/PULL_REQUEST_TEMPLATE.md` — PR body structure.
- `.github/ISSUE_TEMPLATE` — current issue templates, forms, and routing policy.
- `.github/workflows/pr-check.yml` — automated gates.
- `skills/branch-pr/SKILL.md` — branch + PR creation mechanics in detail.
- `skills/chained-pr/SKILL.md` — chained vs stacked PR strategy mechanics in detail.
- `internal/assets/skills/issue-creation/SKILL.md` — canonical issue-creation authority.
- `skills/cognitive-doc-design/SKILL.md` — doc-writing principles (low cognitive load).
- `skills/comment-writer/SKILL.md` — tone and structure for PR comments and issue replies.
- `skills/work-unit-commits/SKILL.md` — splitting commits for review-friendly PRs.

GitHub Actions docs for the `action_required` gate used in fork PRs: https://docs.github.com/actions/managing-workflow-runs/approving-workflow-runs-from-public-forks
