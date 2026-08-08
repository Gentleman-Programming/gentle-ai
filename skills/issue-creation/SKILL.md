---
name: gentle-ai-issue-creation
description: "Create Gentle AI issues with issue-first checks. Trigger: creating GitHub issues, bug reports, or feature requests."
license: Apache-2.0
metadata:
  author: gentleman-programming
  version: "1.1"
---

# Gentle AI — Issue Creation Skill

## When to Use

Load this skill whenever you need to:
- Report a bug in `gga`
- Request a new feature or enhancement
- Open any GitHub issue on the [Gentleman-Programming/gentle-ai](https://github.com/Gentleman-Programming/gentle-ai) repository

## Critical Rules

1. **Blank issues are DISABLED** — `blank_issues_enabled: false` in `.github/ISSUE_TEMPLATE/config.yml`. Web-form submissions MUST use a template; CLI submissions MUST include every required template field in the body.
2. **Template auto-labels require the web form** — use the web-form URLs below to receive the template's type label and `status:needs-review`. GitHub CLI 2.96.0 supports Markdown issue templates, not YAML issue forms, so CLI-created issues do not inherit these labels. CLI submitters must list the missing labels in the issue body for a maintainer.
3. **`status:approved` is REQUIRED before ANY work begins** — a maintainer must label the issue before you or anyone opens a PR.
4. **Questions go to Discussions** — use [GitHub Discussions](https://github.com/Gentleman-Programming/gentle-ai/discussions), NOT issues, for questions and general conversation.
5. **No Co-Authored-By trailers** — never add AI attribution to commits.
6. **Pre-submission privacy review is MANDATORY** — before `gh issue create`, replace private project names, usernames, home paths, hostnames, secrets/credentials, and environment-specific identifiers with explicit placeholders (`<project-name>`, `<user>`, `<hostname>`, `<token>`). Keep reproduction structure with placeholders — never redact an example into nothingness. Do NOT redact intentionally public identifiers like `gentle-ai`, `engram`, `go`. A final body scan happens immediately before publish.

## Pre-submission Privacy Review

Every issue body is scanned immediately before `gh issue create`. The scan replaces — never deletes — environment-specific data with explicit placeholders so the reproduction still teaches:

| Category | Replace with | Example (before → after) |
|----------|---------------|---------------------------|
| Private project names | `<project-name>` | `my-private-project-b` → `<project-name>` |
| Usernames | `<user>` | `C:\Users\my-real-username\go\bin` → `C:\Users\<user>\go\bin` |
| Hostnames | `<hostname>` | `devbox-macbook.local` → `<hostname>` |
| Home paths | `/home/<user>` or `C:\Users\<user>` | (covered above) |
| API keys, tokens, passwords | `<token>` / `<password>` | `ghp_abc123...` → `<token>` |
| Internal ports / hostnames | `<host>:<port>` | `10.0.0.42:5432` → `<host>:<port>` |

Intentionally public identifiers are NOT redacted: tool names (`gentle-ai`, `engram`, `go`, `node`, `python`), package names, public documentation URLs, generic example domains (`example.com`, `localhost`).

**Rule of thumb:** if the reader can run the reproduction step after you replace every identifier with its placeholder, the sanitization is correct. If a step becomes impossible (because the placeholder consumed a needed value), that step needs the value — and you should mark it `<value-required>` and explain in the body what the user should fill in.

## Workflow

```
1. Search existing issues → confirm it's not a duplicate
   https://github.com/Gentleman-Programming/gentle-ai/issues

2. Choose the correct template:
   - Bug   → .github/ISSUE_TEMPLATE/bug_report.yml
   - Feat  → .github/ISSUE_TEMPLATE/feature_request.yml

3. Submit through the web form → template labels are applied automatically

4. Wait — a maintainer reviews and adds status:approved (or closes)

5. Only AFTER status:approved → open a PR referencing this issue
```

> ⚠️ **STOP after step 3.** Do NOT open a PR until the issue has `status:approved`.

---

## Bug Report

**Template path**: `.github/ISSUE_TEMPLATE/bug_report.yml`
**Web-form auto-labels**: `bug`, `status:needs-review`

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

### Recommended Web Form

Use the web form when the template's auto-labels matter:

```text
https://github.com/Gentleman-Programming/gentle-ai/issues/new?template=bug_report.yml
```

### Example CLI Command

CLI filing does not load the YAML issue form or apply its labels. Prepare a body with every required field and list `bug` and `status:needs-review` as maintainer-owed labels.

```bash
gh issue create \
  --repo Gentleman-Programming/gentle-ai \
  --body-file bug-report.md \
  --title "fix(agent): Claude Code not detected on Linux Arch"
```

---

## Feature Request

**Template path**: `.github/ISSUE_TEMPLATE/feature_request.yml`
**Web-form auto-labels**: `enhancement`, `status:needs-review`

### Required Fields

| Field | Description |
|-------|-------------|
| Pre-flight Checklist | Confirm no duplicate exists; confirm PR-approval understanding |
| Affected Area | Which area of `gga` this feature affects |
| Problem Statement | Describe the problem this feature solves |
| Proposed Solution | Specific description — include example `gga` command/output if relevant |
| Alternatives Considered | (optional) Other approaches you thought about |
| Additional Context | (optional) Screenshots, config files, etc. |

### Recommended Web Form

Use the web form when the template's auto-labels matter:

```text
https://github.com/Gentleman-Programming/gentle-ai/issues/new?template=feature_request.yml
```

### Example CLI Command

CLI filing does not load the YAML issue form or apply its labels. Prepare a body with every required field and list `enhancement` and `status:needs-review` as maintainer-owed labels.

```bash
gh issue create \
  --repo Gentleman-Programming/gentle-ai \
  --body-file feature-request.md \
  --title "feat(tui): add keyboard shortcut help overlay"
```

---

## Label System

### Status Labels (applied to Issues)

| Label | Description | Who Applies |
|-------|-------------|-------------|
| `status:needs-review` | Newly opened, awaiting maintainer review | **Auto** (web form only); CLI submitters list it for a maintainer |
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

The body must include the required template fields and identify `bug` and `status:needs-review` as maintainer-owed labels.

```bash
gh issue create \
  --repo Gentleman-Programming/gentle-ai \
  --body-file bug-report.md \
  --title "fix(<scope>): <short description>"
```

### Create a Feature Request

The body must include the required template fields and identify `enhancement` and `status:needs-review` as maintainer-owed labels.

```bash
gh issue create \
  --repo Gentleman-Programming/gentle-ai \
  --body-file feature-request.md \
  --title "feat(<scope>): <short description>"
```

### Check Issue Status

```bash
gh issue view <number> --repo Gentleman-Programming/gentle-ai
```

### Valid Scopes for Issue Titles

`tui`, `cli`, `installer`, `catalog`, `system`, `agent`, `e2e`, `ci`, `docs`
