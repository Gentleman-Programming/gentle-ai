---
name: gentle-ai-issue-creation
description: "Create Gentle AI issues with issue-first checks. Trigger: creating GitHub issues, bug reports, or feature requests."
license: Apache-2.0
metadata:
  author: gentleman-programming
  version: "1.0"
---

# Gentle AI — Issue Creation Skill

## When to Use

Load this skill whenever you need to:
- Report a bug in `gga`
- Request a new feature or enhancement
- Open any GitHub issue on the [Gentleman-Programming/gentle-ai](https://github.com/Gentleman-Programming/gentle-ai) repository

## Critical Rules

1. **Blank issues are DISABLED** — `blank_issues_enabled: false` in `.github/ISSUE_TEMPLATE/config.yml`. You MUST use a template.
2. **`status:needs-review` is applied automatically** — every new issue gets this label; you do NOT add it manually.
3. **`status:approved` is REQUIRED before ANY work begins** — a maintainer must label the issue before you or anyone opens a PR.
4. **Questions go to Discussions** — use [GitHub Discussions](https://github.com/Gentleman-Programming/gentle-ai/discussions), NOT issues, for questions and general conversation.
5. **No Co-Authored-By trailers** — never add AI attribution to commits.

## Workflow

```
1. Search existing issues → confirm it's not a duplicate
   https://github.com/Gentleman-Programming/gentle-ai/issues

2. Choose the correct template:
   - Bug   → .github/ISSUE_TEMPLATE/bug_report.yml
   - Feat  → .github/ISSUE_TEMPLATE/feature_request.yml

3. Submit the issue → status:needs-review is applied automatically

4. Wait — a maintainer reviews and adds status:approved (or closes)

5. Only AFTER status:approved → open a PR referencing this issue
```

> ⚠️ **STOP after step 3.** Do NOT open a PR until the issue has `status:approved`.

---

## Bug Report

**Template path**: `.github/ISSUE_TEMPLATE/bug_report.yml`
**Auto-labels**: `bug`, `status:needs-review`

### Required Fields

| Field | Description |
|-------|-------------|
| Pre-flight Checklist | Confirm no duplicate exists; confirm PR-approval understanding |
| Bug Description | Clear description of what the bug is |
| Steps to Reproduce | Numbered steps to reproduce the behavior |
| Expected Behavior | What should happen |
| Actual Behavior | What actually happens |
| Gentle AI Version | Output of `gga version` |
| Operating System | macOS / Linux distro / Windows / WSL |
| AI Agent / Client | Claude Code / OpenCode / Gemini CLI / Cursor / Windsurf / Other |
| Affected Area | See area list below |

### Affected Areas

`CLI (commands, flags)` · `TUI (terminal UI)` · `Installation Pipeline` · `Agent Detection` · `System Detection` · `Catalog/Steps` · `Documentation` · `Other`

### Example CLI Command

```bash
gh issue create \
  --repo Gentleman-Programming/gentle-ai \
  --template bug_report.yml \
  --title "fix(agent): Claude Code not detected on Linux Arch"
```

Or open the web form directly:
```
https://github.com/Gentleman-Programming/gentle-ai/issues/new?template=bug_report.yml
```

---

## Feature Request

**Template path**: `.github/ISSUE_TEMPLATE/feature_request.yml`
**Auto-labels**: `enhancement`, `status:needs-review`

### Required Fields

| Field | Description |
|-------|-------------|
| Pre-flight Checklist | Confirm no duplicate exists; confirm PR-approval understanding |
| Affected Area | Which area of `gga` this feature affects |
| Problem Statement | Describe the problem this feature solves |
| Proposed Solution | Specific description — include example `gga` command/output if relevant |
| Alternatives Considered | (optional) Other approaches you thought about |
| Additional Context | (optional) Screenshots, config files, etc. |

### Example CLI Command

```bash
gh issue create \
  --repo Gentleman-Programming/gentle-ai \
  --template feature_request.yml \
  --title "feat(tui): add keyboard shortcut help overlay"
```

Or open the web form directly:
```
https://github.com/Gentleman-Programming/gentle-ai/issues/new?template=feature_request.yml
```

---

## Label System

### Status Labels (applied to Issues)

| Label | Description | Who Applies |
|-------|-------------|-------------|
| `status:needs-review` | Newly opened, awaiting maintainer review | **Auto** (template) |
| `status:approved` | Approved — work can begin | Maintainer only |
| `status:in-progress` | Being actively worked on | Contributor |
| `status:blocked` | Blocked by another issue or external dependency | Maintainer / Contributor |
| `status:wont-fix` | Out of scope or won't be addressed | Maintainer only |

### Type Labels (applied to Issues and PRs)

| Label | Description |
|-------|-------------|
| `bug` | Defect report |
| `enhancement` | Feature or improvement request |
| `type:bug` | Bug fix (used on PRs) |
| `type:feature` | New feature (used on PRs) |
| `type:docs` | Documentation only (used on PRs) |
| `type:refactor` | Refactoring, no functional changes (used on PRs) |
| `type:chore` | Build, CI, tooling (used on PRs) |
| `type:breaking-change` | Breaking change (used on PRs) |

### Priority Labels

| Label | Description |
|-------|-------------|
| `priority:critical` | Blocking issues, security vulnerabilities |
| `priority:high` | Important, affects many users |
| `priority:medium` | Normal priority |
| `priority:low` | Nice to have |

---

## Maintainer Approval Workflow

```
Issue submitted
      │
      ▼
status:needs-review  ← auto-applied by template
      │
      ▼
Maintainer reviews
      │
  ┌───┴────────────────┐
  │                    │
  ▼                    ▼
status:approved    Closed
(work can begin)   (invalid / duplicate / wont-fix)
      │
      ▼
Contributor comments "I'll work on this"
      │
      ▼
status:in-progress
      │
      ▼
PR opened with `Closes #<N>`
```

---

## Decision Tree

```
Do you have a question or idea to discuss?
├── YES → GitHub Discussions (NOT issues)
│         https://github.com/Gentleman-Programming/gentle-ai/discussions
└── NO  → Is it a defect in gga?
          ├── YES → Bug Report template
          └── NO  → Feature Request template
                    │
                    ▼
          Does a similar issue already exist?
          ├── YES → Comment on existing issue instead
          └── NO  → Submit new issue → wait for status:approved
```

---

## Commands

### Search for Existing Issues

```bash
# Search open issues
gh issue list --repo Gentleman-Programming/gentle-ai --state open --search "your keywords"

# Search all issues including closed
gh issue list --repo Gentleman-Programming/gentle-ai --state all --search "your keywords"
```

### Create a Bug Report

```bash
gh issue create \
  --repo Gentleman-Programming/gentle-ai \
  --template bug_report.yml \
  --title "fix(<scope>): <short description>"
```

### Create a Feature Request

```bash
gh issue create \
  --repo Gentleman-Programming/gentle-ai \
  --template feature_request.yml \
  --title "feat(<scope>): <short description>"
```

### Check Issue Status

```bash
gh issue view <number> --repo Gentleman-Programming/gentle-ai
```

### Valid Scopes for Issue Titles

`tui`, `cli`, `installer`, `catalog`, `system`, `agent`, `e2e`, `ci`, `docs`
