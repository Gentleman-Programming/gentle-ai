# Contributing to Gentle AI™

Thank you for your interest in contributing to **Gentle AI** (`gentle-ai`) — a Go CLI/TUI ecosystem configurator for AI coding agents.

Before you dive in, please read this guide fully. We have a structured workflow to keep the project organized and maintainable.

---

## Table of Contents

- [Issue-First Workflow](#issue-first-workflow)
- [AI-Assisted Contributions](#ai-assisted-contributions)
- [Label System](#label-system)
- [Development Setup](#development-setup)
- [Testing](#testing)
- [Running the Cross-Lane Battery](#running-the-cross-lane-battery)
- [Commit Convention](#commit-convention)
- [Delivery Strategy for SDD Changes](#delivery-strategy-for-sdd-changes)
- [Pull Request Rules](#pull-request-rules)
- [Code of Conduct](#code-of-conduct)

---

## Issue-First Workflow

**No PR without an issue. No exceptions.**

This project follows a strict issue-first workflow:

1. **Open an issue** using the appropriate template ([Bug Report](https://github.com/Gentleman-Programming/gentle-ai/issues/new?template=bug_report.yml) or [Feature Request](https://github.com/Gentleman-Programming/gentle-ai/issues/new?template=feature_request.yml))
2. **Wait for approval** — work may begin only when the issue has `status:approved` under the canonical issue-creation workflow contract. Without a current direct instruction and target-host capability granting the exact action, comment and wait.
3. **Comment on the issue** to let others know you're working on it
4. **Open a PR** referencing the approved issue

PRs that are not linked to an approved issue will be **automatically rejected** by CI.

---

## Looking for something to work on?

Start at the **[Community Roadmap](docs/community-roadmap.md)**.

Everything labelled [`up-for-grabs`](https://github.com/Gentleman-Programming/gentle-ai/issues?q=is%3Aissue+is%3Aopen+label%3Aup-for-grabs) is scoped, carries `status:approved` so a PR can be opened, and is unclaimed. Comment that you are taking it and go.

An issue **without** that label is usually waiting on information (`status:needs-info`) or on an architectural decision (`status:needs-design`). Those want discussion first — implementing before the decision lands means the work gets thrown away.

## AI-Assisted Contributions

**AI assistance is allowed, but you must understand and own the complete submission.** Before opening a PR:

- [ ] Confirm the change matches the approved issue scope.
- [ ] Inspect every changed line.
- [ ] Remove invented, unverifiable, or unrelated output.
- [ ] Identify the responsible cause or invariant; confirm the fix resolves it rather than masking or shifting the symptom.
- [ ] Remove duplicate authority, unnecessary abstractions, and unrelated complexity; keep the fix proportionate.
- [ ] Run applicable tests and report the actual outcomes.
- [ ] Be ready to explain the design and tradeoffs.
- [ ] Disclose material AI assistance in the PR.

For disclosure boundaries, required details, attribution rules, and reviewer expectations, see the canonical [AI-Assisted Contribution Policy](AI_POLICY.md).

## Label System

### Type Labels (applied to PRs)

| Label | Description |
|-------|-------------|
| `type:bug` | Bug fix |
| `type:feature` | New feature or enhancement |
| `type:docs` | Documentation only |
| `type:refactor` | Code refactoring, no functional changes |
| `type:chore` | Build, CI, tooling changes |
| `type:breaking-change` | Breaking change |

### Size Labels (applied to PRs)

| Label | Description |
|-------|-------------|
| `size:exception` | Maintainer-approved exception for PRs above the 400 changed-line review budget |

### Status Labels (applied to Issues)

| Label | Description |
|-------|-------------|
| `status:needs-review` | Newly opened, awaiting maintainer review |
| `status:approved` | Approved for implementation — work can begin |
| `status:in-progress` | Being worked on |
| `status:blocked` | Blocked by another issue or external dependency |
| `status:wont-fix` | Out of scope or won't be addressed |

### Priority Labels

| Label | Description |
|-------|-------------|
| `priority:critical` | Blocking issues, security vulnerabilities |
| `priority:high` | Important, affects many users |
| `priority:medium` | Normal priority |
| `priority:low` | Nice to have |

---

## Development Setup

### Prerequisites

- Go 1.25.10+
- Docker (for E2E tests)
- Git 2.38+

### Clone and Build

```bash
git clone https://github.com/Gentleman-Programming/gentle-ai.git
cd gentle-ai
go build -o gentle-ai ./cmd/gentle-ai
```

### Run Locally

```bash
./gentle-ai
```

---

## Testing

### Unit Tests

Run the full unit test suite:

```bash
go test ./...
```

Run tests for a specific package:

```bash
go test ./internal/tui/...
```

Run with verbose output:

```bash
go test -v ./...
```

### E2E Tests

E2E tests are Docker-based shell scripts. Docker must be running.

```bash
cd e2e
chmod +x docker-test.sh
./docker-test.sh
```

> ⚠️ E2E tests spin up containers to simulate real installation environments. They may take a few minutes to complete.

### Running the Cross-Lane Battery

The cross-lane battery ([`scripts/cross-lane-battery.sh`](scripts/cross-lane-battery.sh), implemented in [`scripts/crosslane/`](scripts/crosslane/)) is a local, out-of-CI regression net. It drives one real `gentle-ai` binary end to end across the supported agent-host review integration boundaries. It is deliberately not wired into CI because its optional tiers spend real reviewer model runs and real host sessions.

Build a binary first, then run the tier you can afford:

```bash
go build -o /tmp/gentle-ai ./cmd/gentle-ai
./scripts/cross-lane-battery.sh --binary /tmp/gentle-ai [--with-model] [--with-host] [--keep-work]
```

| Tier | Flags | Cost profile | What it covers |
|------|-------|--------------|----------------|
| Deterministic | none (always runs) | Free and fast | The real OpenCode transport plugin bytes through an emulated Task hook surface (host-frame emulation), one full Claude-lane lifecycle plus a medium-candidate consent round-trip, and schema conformance of every captured envelope against `contracts/review-integration/`. |
| Model | `--with-model` | Real reviewer model runs (model spend) | Additionally runs the real compiled claude-code reviewer runtime. |
| Host | `--with-host` | Real host sessions plus model spend | Spawns real host applications: `codex exec` through the compiled Codex adapter, the installed `gentle-pi` print-mode Pi relay, and a headless `opencode run` session in a sandboxed HOME with the real transport plugin. |

Behavior to expect:

- Every host command is bounded (12 minutes per host command, 20 minutes per non-host command), so a hung host surfaces as a bounded lane failure instead of hanging the battery.
- The run prints a PASS/FAIL/SKIP table per check and the real model runs spent; any failing check makes the battery exit non-zero. Known-red checks still fail — red at the exact seam where a defect escaped is the battery working.
- The scratch work root is removed on every exit, including failing ones; pass `--keep-work` to keep it for inspection.

Run the battery before merging changes that touch a review-lifecycle surface (facade, transports, contracts, host adapters) and after building a new binary you intend to exercise. Running it and reporting red checks is itself a valuable contribution — open an issue with the PASS/FAIL/SKIP table and the binary/commit you tested.

The sibling `gentle-pi` repository carries its own battery: `pnpm test:cross-lane` in [Gentleman-Programming/gentle-pi](https://github.com/Gentleman-Programming/gentle-pi).

### Benchmark Validation

[`bench/`](bench/README.md) is a separate Go module, so root-module tests do not validate it. For benchmark-module changes, run these commands from `bench/`:

```bash
go build ./...
go vet ./...
go test ./...
```

The `model-picker` axis (`j97`) and damaged-store crash-recovery journeys use
the `bench_fixture` product build tag. Build that product binary from the
repository root only when running those opt-in axes; their exact driven commands
are documented in [`bench/README.md`](bench/README.md).

Benchmark validation applies to review-lifecycle, gate, recovery, delivery, benchmark implementation/corpus/classifier, and benchmark-claim changes. For measured product-behavior changes, use driven mode and report the command, tested binary or commit, selected subset or axes, and result summary. Compare before and after only when claiming a measured friction change. For unrelated changes, mark benchmark validation `N/A` with a brief reason.

### Windows — Known Test Limitations

Some unit tests require OS-level capabilities that are restricted on Windows by default.

#### Symlink tests (`SeCreateSymbolicLinkPrivilege`)

Tests that create symbolic links (e.g. in `internal/components/filemerge`) will be **skipped automatically** on Windows builds where the process lacks `SeCreateSymbolicLinkPrivilege` (`ERROR_PRIVILEGE_NOT_HELD`, errno 1314). This is a Windows security policy, not a bug in the code.

To run these tests without restrictions, choose one of:

- **Enable Developer Mode** — Settings → System → For developers → Developer Mode. This grants symlink creation to all processes without admin rights.
- **Run as Administrator** — open your terminal as Administrator before running `go test ./...`.
- **Grant the privilege explicitly** via Group Policy: `Local Security Policy → User Rights Assignment → Create symbolic links`.

> On Linux and macOS these tests always run without any extra setup.

---

## Commit Convention

This project uses [Conventional Commits](https://www.conventionalcommits.org/).

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

| Type | Purpose |
|------|---------|
| `feat` | New feature |
| `fix` | Bug fix |
| `docs` | Documentation only |
| `refactor` | Code change (no behavior change) |
| `chore` | Maintenance, dependencies, tooling |
| `style` | Formatting, linting (no logic change) |
| `perf` | Performance improvement |
| `test` | Adding or updating tests |
| `build` | Build system or external deps |
| `ci` | CI configuration |
| `revert` | Reverts a previous commit |

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
```

### Breaking Changes

Add `!` after the type/scope and include a `BREAKING CHANGE:` footer:

```
feat(cli)!: rename --config flag to --config-file

BREAKING CHANGE: the --config flag has been renamed to --config-file.
Update your scripts and aliases accordingly.
```

Breaking changes map to the `type:breaking-change` label.

---

## Branch Naming

Branch names **must** match this pattern:

```
^(feat|fix|chore|docs|style|refactor|perf|test|build|ci|revert)\/[a-z0-9._-]+$
```

**Rules:**
- All lowercase
- Use hyphens, dots, or underscores as separators (no spaces, no uppercase)
- Description must be short and descriptive

**Examples:** `feat/user-login`, `fix/crash-on-startup`, `docs/api-reference`, `ci/add-e2e-job`

---

## Pull Request Rules

### Delivery Strategy for SDD Changes

Before `sdd-apply` starts, the SDD conductor checks the **Review Workload Forecast** from `sdd-tasks`. This protects reviewers from one giant, exhausting PR when the work should be split.

| Strategy | Use when | What happens before apply |
|---|---|---|
| `ask-on-risk` | Default. You want the conductor to pause only when the forecast is risky. | If the forecast is high or above 400 changed lines, it asks whether to split or proceed with `size:exception`. |
| `auto-chain` | You already know the change should be reviewed in slices. | The apply phase implements the next chained/stacked PR slice using work-unit commits. |
| `single-pr` | The change is small or must land atomically. | If the forecast exceeds 400 changed lines, apply stops until a maintainer approves `size:exception`. |
| `exception-ok` | A maintainer already accepted a large PR. | Apply continues and records that the PR has maintainer-approved `size:exception`. |

**Decision checklist:**

- [ ] Can one reviewer understand this in about 60 minutes?
- [ ] Is the PR at or below 400 changed lines?
- [ ] Does each work-unit commit include its code, tests, and docs together?
- [ ] If the answer is “no” to any item, choose `auto-chain` or get explicit `size:exception` approval.

**Mental model:** work-unit commits are the bricks; chained PRs are the wall sections. Don’t make reviewers inspect the whole building in one sitting.

### PR Size Budget

Keep PRs at or below **400 changed lines** (`additions + deletions`). This is a deliberate cognitive-load limit: a PR should be reviewable in roughly **60 minutes** without pushing reviewers into fatigue.

If your change cannot fit that budget, split it into **chained or stacked PRs** so each review remains focused. Large generated/vendor/migration diffs may use the `size:exception` label, but only when a maintainer agrees the large diff is unavoidable.

### Work-Unit Commits

Structure commits by deliverable unit, not by file type. A good commit includes the code, tests, and docs needed to understand and verify one behavior or workflow.

- Prefer `feat(auth): validate tokens at login` over separate `models`, `services`, and `tests` commits.
- Keep rollback reasonable: reverting one commit should not remove unrelated work.
- When a PR grows near 400 changed lines, promote work-unit commits into chained or stacked PRs.

### Review Comments

Review feedback should be warm, direct, and useful quickly. Start with the actionable point, explain why when needed, and avoid recapping the PR before giving feedback.

### Before Opening a PR

- [ ] There is a linked approved issue (`Closes #<N>`)
- [ ] The PR is at or below 400 changed lines, or a maintainer approved `size:exception`
- [ ] Commits are organized by deliverable work unit
- [ ] All unit tests pass (`go test ./...`)
- [ ] E2E tests pass (`cd e2e && ./docker-test.sh`)
- [ ] Benchmark validation completed, or this change is not applicable to the benchmark (explain why in the Test Plan).
- [ ] Commits follow Conventional Commits format
- [ ] Code is self-reviewed
- [ ] I understand and take responsibility for the complete submission, and have disclosed any material AI assistance in the PR

### PR Title

Use the same Conventional Commits format as commit messages:

```
feat(tui): add keyboard shortcut help overlay
fix(agent): handle missing HOME env var gracefully
```

### Automated PR Checks

All PRs go through automated checks:

| Check | What It Verifies |
|-------|-----------------|
| **Check PR Cognitive Load** | PR stays within 400 changed lines (`additions + deletions`) unless labelled `size:exception` |
| **Check Issue Reference** | PR body contains `Closes/Fixes/Resolves #N` |
| **Check Issue Has status:approved** | The linked issue has `status:approved` under the canonical issue-creation workflow contract |
| **Check PR Has type:* Label** | Exactly one `type:*` label is applied |
| **Unit Tests** | `go test ./...` passes |
| **E2E Tests** | `cd e2e && ./docker-test.sh` passes |

**All checks must pass** before a PR can be merged.

### Linking Your Issue

In the PR body, include one of:

```
Closes #42
Fixes #42
Resolves #42
```

---

## Code of Conduct

Be respectful. We're building something together.

- Critique code, not people
- Be constructive in reviews
- Welcome newcomers

Violations may result in removal from the project.

---

## Questions?

Use [GitHub Discussions](https://github.com/Gentleman-Programming/gentle-ai/discussions) — not issues — for questions, ideas, and general conversation.
