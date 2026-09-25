---
name: branch-pr
description: "Create Gentle AI pull requests with issue-first checks. Trigger: creating, opening, or preparing PRs for review."
license: Apache-2.0
metadata:
  author: gentleman-programming
  version: "2.0"
---

## When to Use

Use this skill when:
- Creating a pull request for any change
- Preparing a branch for submission
- Helping a contributor open a PR

---

## Critical Rules

Before any target-host read, obtain explicit authorization for the remote destination (exact target), operation and credential/session; do not probe ambient credentials. Once authorized, reuse fresh target-bound approved issue, default branch, type-label and current check evidence. Commit, push, PR, merge, chain strategy/exception and native RDD consent remain human-owned.

1. **Every PR MUST visibly link an approved base-repository issue** — `Closes/Fixes/Resolves #N` closes it on merge; `Refs #N` is non-closing; malformed, cross-repository, and mixed closing/non-closing references for the same issue are rejected
2. **Every PR MUST have exactly one `type:*` label**. A current direct human instruction for the exact target/action and verified target-host capability are required before its canonical issue-creation workflow mutation; mark checkboxes only after readback.
3. Establish REQUIRED CI from current target branch rulesets/branch protection and run status before declaring merge-ready. CodeRabbit pending is optional unless required by target policy; unknown requiredness is not merge-ready.
4. **Blank PRs without issue linkage will be blocked** by GitHub Actions

---

## Workflow

```
1. After remote read authorization, verify the base-repository issue has `status:approved` and resolve the target's current default/base branch; reuse fresh target-bound evidence
2. Ask the human to select closing (`Closes/Fixes/Resolves #N`) vs non-closing (`Refs #N`) intent; preserve the human-selected choice
3. Implement authorized work; run applicable local checks and report failures honestly
4. Draft the template; do not auto commit, push, create a PR, merge or grant native RDD consent
5. Apply a type label only under the canonical issue-creation action contract
6. Read target policy and status to identify REQUIRED checks; do not infer requiredness from a pending optional run
```

---

## Branch Naming

Branch names MUST match this regex:

```
^(feat|fix|chore|docs|style|refactor|perf|test|build|ci|revert)\/[a-z0-9._-]+$
```

**Format:** `type/description` — lowercase, no spaces, only `a-z0-9._-` in description.

| Type | Branch pattern | Example |
|------|---------------|---------|
| Feature | `feat/<description>` | `feat/user-login` |
| Bug fix | `fix/<description>` | `fix/zsh-glob-error` |
| Chore | `chore/<description>` | `chore/update-ci-actions` |
| Docs | `docs/<description>` | `docs/installation-guide` |
| Style | `style/<description>` | `style/format-scripts` |
| Refactor | `refactor/<description>` | `refactor/extract-shared-logic` |
| Performance | `perf/<description>` | `perf/reduce-startup-time` |
| Test | `test/<description>` | `test/add-setup-coverage` |
| Build | `build/<description>` | `build/update-shellcheck` |
| CI | `ci/<description>` | `ci/add-branch-validation` |
| Revert | `revert/<description>` | `revert/broken-setup-change` |

---

## PR Body Format

Use the current `.github/PULL_REQUEST_TEMPLATE.md` as the body authority, including all required sections. The items below are schematic guidance, not a complete ready-to-publish body; never precheck unsupported claims:

### 1. Linked Issue (REQUIRED)

```markdown
<human-selected Closes/Fixes/Resolves #N or Refs #N>
```

Valid keywords: `Closes #N`, `Fixes #N`, `Resolves #N` (case insensitive) close the issue on merge; `Refs #N` is a non-closing link. Use only visible, well-formed references to approved issues in the base repository.
The linked issue MUST have the `status:approved` label.

### 2. PR Type (REQUIRED)

Check exactly ONE in the template and add the matching label:

| Checkbox | Label to add |
|----------|-------------|
| Bug fix | `type:bug` |
| New feature | `type:feature` |
| Documentation only | `type:docs` |
| Code refactoring | `type:refactor` |
| Maintenance/tooling | `type:chore` |
| Breaking change | `type:breaking-change` |

### 3. Summary

1-3 bullet points of what the PR does.

### 4. Changes Table

```markdown
| File | Change |
|------|--------|
| `path/to/file` | What changed |
```

### 5. Test Plan

```markdown
- [ ] Scripts run without errors: `shellcheck scripts/*.sh` (check only if run and passed)
- [ ] Manually tested the affected functionality (check only if observed)
- [ ] Skills load correctly in target agent (check only if verified)
```

### 6. Contributor Checklist

Mark boxes only with observed evidence; leave pending actions unchecked and describe them. An unchecked required gate is not merge-ready:
- Linked an approved issue using the human-selected closing or non-closing reference
- Added exactly one `type:*` label (confirmed by target-host readback)
- Ran shellcheck on modified scripts where applicable
- Skills tested in at least one agent where applicable
- Docs updated if behavior changed
- Conventional commit format
- No `Co-Authored-By` trailers

---

## Automated Checks (requiredness depends on target policy)

| Check | Job name | What it verifies |
|-------|----------|-----------------|
| PR Validation | `Check Issue Reference` | Body contains a visible, well-formed base-repository `Closes/Fixes/Resolves #N` or `Refs #N` |
| PR Validation | `Check Issue Has status:approved` | Linked issue has `status:approved` |
| PR Validation | `Check PR Has type:* Label` | PR has exactly one `type:*` label |
| CI | `Shellcheck` | Shell scripts pass `shellcheck` |

---

## Conventional Commits

Commit messages MUST match this regex:

```
^(build|chore|ci|docs|feat|fix|perf|refactor|revert|style|test)(\([a-z0-9\._-]+\))?!?: .+
```

**Format:** `type(scope): description` or `type: description`

- `type` — required, one of: `build`, `chore`, `ci`, `docs`, `feat`, `fix`, `perf`, `refactor`, `revert`, `style`, `test`
- `(scope)` — optional, lowercase with `a-z0-9._-`
- `!` — optional, indicates breaking change
- `description` — required, starts after `: `

Type-to-label mapping:

| Commit type | PR label |
|-------------|----------|
| `feat` | `type:feature` |
| `fix` | `type:bug` |
| `docs` | `type:docs` |
| `refactor` | `type:refactor` |
| `chore` | `type:chore` |
| `style` | `type:chore` |
| `perf` | `type:feature` |
| `test` | `type:chore` |
| `build` | `type:chore` |
| `ci` | `type:chore` |
| `revert` | `type:bug` |
| `feat!` / `fix!` | `type:breaking-change` |

Examples:
```
feat(scripts): add Codex support to setup.sh
fix(skills): correct topic key format in sdd-apply
docs(readme): update multi-model configuration guide
refactor(skills): extract shared persistence logic
chore(ci): add shellcheck to PR validation workflow
perf(scripts): reduce setup.sh execution time
style(skills): fix markdown formatting
test(scripts): add setup.sh integration tests
ci(workflows): add branch name validation
revert: undo broken setup change
feat!: redesign skill loading system
```

---

## Commands

Do not assume `main` or execute branch/remote mutations from examples. Resolve the authorized target's default branch from current metadata first. For protected `status:approved` or `size:exception`, require authenticated actor target-host `viewerPermission` `MAINTAIN` or `ADMIN` and a current direct human instruction binding the exact target/action; do not demand separate proof of the instruction-giver's identity. A human-selected `size:exception` additionally requires documented over-budget rationale. Baseline attribution requires reproducing the same failing command/environment on a comparable isolated clean base, without disturbing user changes; otherwise report baseline unverified. Never stash/pop for this purpose.
