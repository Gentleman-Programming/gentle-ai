---
name: gentle-ai-branch-pr
description: "Create Gentle AI pull requests with issue-first checks. Trigger: creating, opening, or preparing PRs for review."
license: Apache-2.0
metadata:
  author: gentleman-programming
  version: "2.0"
---

# Gentle AI — Branch & PR Skill

## When to Use

Load this skill whenever you need to:
- Create a branch for a new fix or feature
- Open a pull request on [Gentleman-Programming/gentle-ai](https://github.com/Gentleman-Programming/gentle-ai)
- Prepare changes for review

## Critical Rules

1. **Every PR MUST visibly link an approved base-repository issue** — `Closes/Fixes/Resolves #<N>` closes it on merge; `Refs #<N>` is non-closing. Every accepted reference MUST have `status:approved`; malformed, cross-repository, or mixed closing/non-closing references for the same issue are rejected by CI.
2. **Ordinary `type:*` categorization** — CI rejects zero or multiple type labels. Route it through the canonical issue-creation workflow contract: a current direct human instruction binds the exact target/action, target-host capability is verified, and it uses one bounded mutation and target-host readback; otherwise wait without mutation.
3. **Protected policy labels** — Adding or removing `status:approved` or `size:exception` requires authenticated actor target-host `viewerPermission` `MAINTAIN` or `ADMIN` and a current direct human instruction binding the exact target/action. Here verified policy authority means that actor permission and exact direct instruction, not separate target-host proof of the instruction-giver's identity; do not mutate automatically. `size:exception` additionally requires documented over-budget rationale and a human-selected exception.
4. **400-line review budget** — keep PRs within 400 changed lines (`additions + deletions`) or document the rationale required for a `size:exception` label.
5. **REQUIRED checks must pass** — establish requiredness from the target branch rulesets/branch protection and current run status; see Automated Checks below.
6. **No `Co-Authored-By` trailers** — never add AI attribution to commits.
7. **No force-push to main/master** — protected branch.

## Workflow

Before any target-host read, obtain explicit authorization for the remote destination (exact target), operation (including metadata/status reads), and credential/session. Do not probe ambient credentials. After authorization reuse fresh target-bound approved-issue, default branch, `type:*` label and check evidence rather than re-asking verified facts. Missing or stale evidence remains unknown.

1. Confirm the base-repository issue has `status:approved` on the authorized target. Resolve its current default/base branch from target metadata; do not assume `main`.
2. Ask the human whether the PR should close the issue on merge. Preserve the human-selected `Closes/Fixes/Resolves #N` closing intent or `Refs #N` non-closing intent; do not substitute one for the other.
3. Implement authorized work and run applicable local checks. Do not auto commit, push, create a PR, merge, select a chain strategy or exception, or give native RDD consent. Each operation needs its own human authority.
4. Draft against the template. Declare one `type:*` result; any label mutation follows the canonical issue-creation workflow contract and exact direct instruction. Mark checkboxes only after observed readback.
5. Determine REQUIRED CI from current target branch rulesets/branch protection and current run status before calling a PR merge-ready. CodeRabbit is optional unless target policy makes it required; a pending optional run is not a blocker. Unknown requiredness is not merge-ready.

For baseline attribution compare the same failing command/environment on a comparable isolated clean base, without disturbing user changes. If not compared, report baseline unverified; do not use stash/pop.

---

## Branch Naming

Branch names **must** match this pattern:

```
^(feat|fix|chore|docs|style|refactor|perf|test|build|ci|revert)\/[a-z0-9._-]+$
```

| Type | Example |
|------|---------|
| `feat/` | `feat/user-login` |
| `fix/` | `fix/duplicate-observation-insert` |
| `docs/` | `docs/api-reference-update` |
| `refactor/` | `refactor/extract-query-sanitizer` |
| `chore/` | `chore/bump-bubbletea-v0.26` |
| `style/` | `style/fix-linter-warnings` |
| `perf/` | `perf/optimize-catalog-loading` |
| `test/` | `test/add-pipeline-coverage` |
| `build/` | `build/update-goreleaser-config` |
| `ci/` | `ci/add-e2e-docker-job` |
| `revert/` | `revert/undo-model-picker-change` |

**Rules:**
- All lowercase
- Use hyphens, dots, or underscores as separators (no spaces, no uppercase)
- Description must be short and descriptive

---

## PR Body Format

Use the current `.github/PULL_REQUEST_TEMPLATE.md` as authority. The following is a non-executable schematic, not a complete PR body or a publication command. Include all sections required by the actual template (including Automated Checks and Notes for Reviewers when present). Fill only observed facts, leave unverified boxes unchecked and record pending actions separately.

```markdown
## 🔗 Linked Issue

<human-selected Closes/Fixes/Resolves #N or Refs #N> (closing vs non-closing intent must be asked, not inferred)

## 🏷️ PR Type

- [ ] `type:bug` — Bug fix (non-breaking change that fixes an issue)
- [ ] `type:feature` — New feature (non-breaking change that adds functionality)
- [ ] `type:docs` — Documentation only
- [ ] `type:refactor` — Code refactoring (no functional changes)
- [ ] `type:chore` — Build, CI, or tooling changes
- [ ] `type:breaking-change` — Breaking change

## 📝 Summary

<!-- Clear description of what this PR does and why. -->

## 📂 Changes

| File / Area | What Changed |
|-------------|-------------|
| `path/to/file` | Brief description |

## 🧪 Test Plan

<!-- Replace examples below with commands actually run and their observed outcomes. -->

**Unit Tests**
\`\`\`bash
go test ./...
\`\`\`

**Go Format**
\`\`\`bash
go run ./internal/gofmtcheck
\`\`\`

**E2E Tests** (Docker required)
\`\`\`bash
cd e2e && ./docker-test.sh
\`\`\`

- [ ] Unit tests pass (`go test ./...`)
- [ ] Go format passes (`go run ./internal/gofmtcheck`)
- [ ] E2E tests pass (`cd e2e && ./docker-test.sh`)
- [ ] Manually tested locally

## ✅ Contributor Checklist

- [ ] PR is linked to an issue with `status:approved`
- [ ] PR stays within 400 changed lines, or the human-selected `size:exception` rationale, current direct human instruction for the exact target/action and actor `MAINTAIN`/`ADMIN` are documented
- [ ] API read-back confirms exactly one appropriate `type:*` label on this PR
- [ ] Unit tests pass (`go test ./...`)
- [ ] E2E tests pass (`cd e2e && ./docker-test.sh`)
- [ ] I have updated documentation if necessary
- [ ] My commits follow Conventional Commits format
- [ ] My commits do not include `Co-Authored-By` trailers

## Automated Checks

<!-- Record current target-required checks and observed statuses only. -->

## Notes for Reviewers

<!-- Describe dependencies or pending actions where applicable. -->
```

---

## Automated Checks

These workflows may run on a PR. Establish which are REQUIRED from current target branch rulesets/branch protection and run status before asserting merge readiness. CodeRabbit is optional unless required by target policy; unknown requiredness blocks a merge-ready claim:

| Check | What It Verifies | How to Fix |
|-------|-----------------|------------|
| **Check PR Cognitive Load** | PR stays within 400 changed lines (`additions + deletions`) or has `size:exception` | Split the PR, or document the human-selected `size:exception` rationale and verify actor `MAINTAIN`/`ADMIN` plus a current direct human instruction for the exact target/action before its canonical workflow action |
| **Check Issue Reference** | PR body contains a visible, well-formed base-repository `Closes/Fixes/Resolves #N` or `Refs #N` | Add one valid reference; malformed, cross-repository, and mixed closing/non-closing references for the same issue fail |
| **Check Issue Has `status:approved`** | Linked issue has the required label | Use the canonical issue-creation workflow contract only when a current direct instruction and target-host capability grant authorize the exact action; otherwise wait |
| **Check PR Has `type:*` Label** | Exactly one `type:*` label is applied to the PR | Use the canonical issue-creation workflow contract only when a current direct instruction and target-host capability authorize the exact action; otherwise wait |
| **Unit Tests** | `go test ./...` passes | Fix failing tests before pushing |
| **Go Format** | `go run ./internal/gofmtcheck` passes | Format malformed Go files before pushing |
| **E2E Tests** | `cd e2e && ./docker-test.sh` passes | Fix failing E2E scenarios before pushing |

---

## Conventional Commits

Commit messages **must** match this pattern:

```
^(build|chore|ci|docs|feat|fix|perf|refactor|revert|style|test)(\([a-z0-9\._-]+\))?!?: .+
```

### Format

```
<type>(<optional-scope>)!: <description>

[optional body]

[optional footer]
```

### Allowed Types

| Type | Purpose | PR Label |
|------|---------|----------|
| `feat` | New feature | `type:feature` |
| `fix` | Bug fix | `type:bug` |
| `docs` | Documentation only | `type:docs` |
| `refactor` | Code change (no behavior change) | `type:refactor` |
| `chore` | Maintenance, dependencies, tooling | `type:chore` |
| `style` | Formatting, linting (no logic change) | `type:chore` |
| `perf` | Performance improvement | `type:feature` |
| `test` | Adding or updating tests | `type:chore` |
| `build` | Build system or external deps | `type:chore` |
| `ci` | CI configuration | `type:chore` |
| `revert` | Reverts a previous commit | matches reverted type |

### Breaking Changes

Add `!` after the type/scope:

```
feat(cli)!: rename --config flag to --config-file

BREAKING CHANGE: the --config flag has been renamed to --config-file.
```

Breaking changes map to `type:breaking-change` label.

### Examples

```
feat(tui): add progress bar to installation steps
fix(agent): correct Claude Code detection on macOS
docs: update contributing guide
chore(deps): bump bubbletea to v0.26
refactor(pipeline): extract step executor
style: fix linter warnings in catalog package
perf(system): cache OS detection result
test(installer): add coverage for catalog step execution
build: update goreleaser config for arm64
ci: split unit and e2e test jobs
revert: undo model picker redesign
feat(cli)!: change default config path
```

---

## Commands

### Setup

```bash
# Only after explicit authorization for remote destination, operation and credential/session,
# confirm approved issue on exact target; reuse fresh target-bound approval evidence.
gh issue view <N> --repo Gentleman-Programming/gentle-ai

# After exact remote read authorization, verify approval and resolve the current target default branch.
# Checkout/branch creation requires separate human authorization; never assume main.
```

### Testing Locally

```bash
# Unit tests
go test ./...

# Go format
go run ./internal/gofmtcheck

# Unit tests — specific package
go test ./internal/tui/...

# Unit tests — verbose
go test -v ./...

# E2E tests (Docker must be running)
cd e2e && ./docker-test.sh
```

### Open a PR

Draft using the current `.github/PULL_REQUEST_TEMPLATE.md`, including every required section. Replace placeholders with observed evidence; leave unsupported checklist claims unchecked, including label readback before the PR exists. Ask the human for the exact closing or non-closing issue reference. PR creation needs separate explicit authorization for the target destination, operation and credential/session; this skill provides no executable creation command or publication permission.

### Check PR Status

```bash
# Only after explicit authorization for these exact target PR status reads.
gh pr checks --repo Gentleman-Programming/gentle-ai <PR-number>
gh pr view --repo Gentleman-Programming/gentle-ai <PR-number>
```
