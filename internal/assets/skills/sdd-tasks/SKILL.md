---
name: sdd-tasks
description: "Break an SDD change into implementation tasks. Trigger: orchestrator launches task planning for a change."
disable-model-invocation: true
user-invocable: false
license: MIT
metadata:
  author: gentleman-programming
  version: "2.0"
  delegate_only: true
---

## Execution Role

Confirm your role before acting. You are the dedicated `sdd-tasks` sub-agent unless you loaded this skill directly through the `skill()` tool.

- If you are the `sdd-tasks` sub-agent, continue with the phase work below. Do not delegate. Do not call the Skill tool.
- If you loaded this skill through the `skill()` tool, you are the orchestrator. Stop here and delegate to the dedicated `sdd-tasks` sub-agent using your platform's delegation primitive (for example, `task(...)` or a sub-agent invocation).


## Language Domain Contract

Generated technical artifacts default to English. Do not inherit the user's conversational language or the active persona's regional voice for SDD artifacts unless the user explicitly requests that artifact language or the project convention requires it.

If technical artifacts are explicitly requested in another language, use a neutral/professional register unless the user explicitly requests a different tone or regional variant.

Public/contextual comments follow the target context language by default. Explicit user language or tone overrides win; otherwise use a neutral/professional register unless the target context clearly calls for another tone or regional variant.

## Purpose

You are a sub-agent responsible for creating the TASK BREAKDOWN. You take the proposal, specs, and design, then produce a `tasks.md` with concrete, actionable implementation steps organized by phase.

## What You Receive

From the orchestrator:
- Change name
- Artifact store mode (`engram | openspec | hybrid | none`)
- Resolved delivery route and its route-conditioned `delivery_strategy`

## Execution and Persistence Contract

> Follow **Section B** (retrieval) and **Section C** (persistence) from `skills/_shared/sdd-phase-common.md`.

- **engram**: Read `sdd/{change-name}/proposal` (required), `sdd/{change-name}/spec` (required), `sdd/{change-name}/design` (required). Save as `sdd/{change-name}/tasks`.
- **openspec**: Read and follow `skills/_shared/openspec-convention.md`.
- **hybrid**: Follow BOTH conventions — persist to Engram AND write `tasks.md` to filesystem. Retrieve dependencies from Engram (primary) with filesystem fallback.
- **none**: Return result only. Never create or modify project files.

## What to Do

### Step 1: Load Skills
Follow **Section A** from `skills/_shared/sdd-phase-common.md`.

### Step 2: Analyze the Design

From the design document, identify:
- All files that need to be created/modified/deleted
- The dependency order (what must come first)
- Testing requirements per component
- Every applicable threat-matrix case and its planned RED test; ignore rows explicitly marked `N/A`

### Step 3: Write tasks.md

**IF mode is `openspec` or `hybrid`:** Create the task file:

```
openspec/changes/{change-name}/
├── proposal.md
├── specs/
├── design.md
└── tasks.md               ← You create this
```

**IF mode is `engram` or `none`:** Do NOT create any `openspec/` directories or files. Compose the tasks content in memory — you will persist it in Step 4.

#### Task File Format

```markdown
# Tasks: {Change Title}

## Review Workload Forecast

| Field | Value |
|-------|-------|
| Estimated changed lines | <rough estimate or range> |
| 400-line budget risk | Low / Medium / High |
| Chained PRs recommended | Yes / No |
| Suggested split | <single PR or autonomous work-unit slices> |

## Delivery Route Fact (NON-AUTHORITATIVE)

| Required field | Value |
|-------|-------|
| Repository identity | <exact repository identity> |
| Remote identity | <exact remote/host identity> |
| Provider/capability/route | <observed provider, capability, and route> |
| Pinned revision | <exact candidate or runtime revision> |
| Command-help evidence/digest | <verifiable output or digest> |
| Postcondition evidence/digest | <verifiable observed state or digest> |
| Observed/freshness marker | <observation time or equivalent marker> |

### GitHub-native schema

- Selectable `delivery_strategy`: `ask-on-risk | auto-chain | single-pr`.
- `chain_strategy`: not applicable. Portable exception fields: not applicable.
- Emit `Decision needed before apply`, `Chained PRs recommended`, and `400-line budget risk`; an over-budget `single-pr` stops.

### Portable schema

- Selectable `delivery_strategy`: `ask-on-risk | auto-chain | single-pr | exception-ok`.
- `chain_strategy`: `stacked-to-main` or `feature-branch-chain`; manual chaining is permitted.
- `size:exception` requires the explicit portable approval evidence from the shared protocol.

### Blocked schema

- Stop before a forecast decision. Do not emit a delivery selection, chain choice, or exception field.

This record does not authorize remote create/submit/sync/update/merge.

### Suggested Work Units

| Unit | Goal | Likely PR | Focused test command | Runtime harness | Rollback boundary |
|------|------|-----------|----------------------|-----------------|-------------------|
| 1 | <standalone deliverable> | PR 1 | <smallest proving command> | <real scenario/command or N/A with reason> | <files/behavior removable without unrelated rollback> |
| 2 | <standalone deliverable> | PR 2 | <smallest proving command> | <real scenario/command or N/A with reason> | <independent revert boundary> |

## Phase 1: {Phase Name} (e.g., Infrastructure / Foundation)

- [ ] 1.1 {Concrete action — what file, what change}
- [ ] 1.2 {Concrete action}
- [ ] 1.3 {Concrete action}

## Phase 2: {Phase Name} (e.g., Core Implementation)

- [ ] 2.1 {Concrete action}
- [ ] 2.2 {Concrete action}
- [ ] 2.3 {Concrete action}
- [ ] 2.4 {Concrete action}

## Phase 3: {Phase Name} (e.g., Testing / Verification)

- [ ] 3.1 {Write tests for ...}
- [ ] 3.2 {Write tests for ...}
- [ ] 3.3 {Verify integration between ...}

## Phase 4: {Phase Name} (e.g., Cleanup / Documentation)

- [ ] 4.1 {Update docs/comments}
- [ ] 4.2 {Remove temporary code}
```

### Task Writing Rules

Each task MUST be:

| Criteria | Example ✅ | Anti-example ❌ |
|----------|-----------|----------------|
| **Specific** | "Create `internal/auth/middleware.go` with JWT validation" | "Add auth" |
| **Actionable** | "Add `ValidateToken()` method to `AuthService`" | "Handle tokens" |
| **Verifiable** | "Test: `POST /login` returns 401 without token" | "Make sure it works" |
| **Small** | One file or one logical unit of work | "Implement the feature" |

Every applicable threat-matrix case MUST become an explicit RED-test task before its production task. Preserve the concrete case and expected safe/failure behavior from design; rows marked `N/A` stay omitted.

### Review Workload Forecast Rules

Load `gentle-ai-chained-pr` as the single provider resolver; SDD must not duplicate provider detection. Before finalizing tasks, resolve the provider route through that skill and record the Delivery Route Fact above as NON-AUTHORITATIVE continuity evidence. A GitHub unavailable/unproven or unknown/ambiguous result is blocked; do not invent an install, manual, or guessed fallback.

Before finalizing tasks, estimate whether implementation is likely to exceed the **400 changed-line review budget** (`additions + deletions`). This is a planning guard, not an exact diff count.

Use available signals: number of files, phases, integration points, tests, docs, generated artifacts, migrations, and how many concerns the change crosses.

If the estimate is **High** or likely above 400 lines:

1. Mark `Chained PRs recommended` as `Yes`.
2. Split tasks into **work units** that can become chained or stacked PRs.
3. Each suggested PR must have a clear start, clear finish, verification, autonomous scope, focused test command, runtime harness, and rollback boundary.
4. Select only the matching route schema: GitHub-native permits `ask-on-risk`, `auto-chain`, or `single-pr`; portable non-GitHub permits all four strategies, manual chaining, feature-branch chains, and a size exception only with the recorded approval evidence; blocked routes stop without a delivery selection.
5. Set `Decision needed before apply` from the selected schema; `ask-on-risk` asks, `auto-chain` applies the first slice, GitHub over-budget `single-pr` stops, and portable exceptions require the exact recorded evidence.

Do not bury this in prose. Put the forecast near the top of the tasks artifact so the user sees it before implementation starts.

The route-applicable forecast MUST include these exact plain-text lines so downstream guards can match them literally:

```text
Decision needed before apply: Yes|No
Chained PRs recommended: Yes|No
400-line budget risk: Low|Medium|High
```

Portable forecasts additionally emit `Chain strategy: stacked-to-main|feature-branch-chain`; GitHub-native records `Chain strategy: not-applicable`; blocked routes emit none of these forecast decision lines. You may keep the table for readability, but the plain-text lines are the guard contract.

Under the portable `feature-branch-chain` schema, suggested work units SHOULD name the intended base boundary: PR #1 base = feature/tracker branch; PR #2 base = PR #1 branch; PR #3 base = PR #2 branch. If a child PR would show previous PR changes, the base is wrong and must be retargeted/rebased before review.

### Phase Organization Guidelines

```
Phase 1: Foundation / Infrastructure
  └─ New types, interfaces, database changes, config
  └─ Things other tasks depend on

Phase 2: Core Implementation
  └─ Main logic, business rules, core behavior
  └─ The meat of the change

Phase 3: Integration / Wiring
  └─ Connect components, routes, UI wiring
  └─ Make everything work together

Phase 4: Testing
  └─ Unit tests, integration tests, e2e tests
  └─ Verify against spec scenarios

Phase 5: Cleanup (if needed)
  └─ Documentation, remove dead code, polish
```

### Step 4: Persist Artifact

**This step is MANDATORY — do NOT skip it.**

Follow **Section C** from `skills/_shared/sdd-phase-common.md`.
- artifact: `tasks`
- topic_key: `sdd/{change-name}/tasks`
- type: `architecture`

### Step 5: Return Summary

Return to the orchestrator:

```markdown
## Tasks Created

**Change**: {change-name}
**Location**: `openspec/changes/{change-name}/tasks.md` (openspec/hybrid) | Engram `sdd/{change-name}/tasks` (engram) | inline (none)

### Breakdown
| Phase | Tasks | Focus |
|-------|-------|-------|
| Phase 1 | {N} | {Phase name} |
| Phase 2 | {N} | {Phase name} |
| Phase 3 | {N} | {Phase name} |
| Total | {N} | |

### Implementation Order
{Brief description of the recommended order and why}

### Review Workload Forecast
- Estimated changed lines: {estimate or range}
- 400-line budget risk: {Low | Medium | High}
- Chained PRs recommended: {Yes | No}
- Delivery strategy: {route-conditioned `delivery_strategy` value from the selected schema; omit when blocked}
- Decision needed before apply: {Yes | No}
- Suggested work-unit PR split: {brief list or "Not needed"}

### Next Step
{Ready for implementation (sdd-apply) OR ask the user whether to use chained PRs before sdd-apply.}
```

## Rules

- ALWAYS reference concrete file paths in tasks
- Tasks MUST be ordered by dependency — Phase 1 tasks shouldn't depend on Phase 2
- Testing tasks should reference specific scenarios from the specs
- Each task should be completable in ONE session (if a task feels too big, split it)
- Use hierarchical numbering: 1.1, 1.2, 2.1, 2.2, etc.
- NEVER include vague tasks like "implement feature" or "add tests"
- Apply any `rules.tasks` from `openspec/config.yaml`
- If the project uses TDD, integrate test-first tasks: RED task (write failing test) → GREEN task (make it pass) → REFACTOR task (clean up)
- **Size budget**: Tasks artifact MUST be under 530 words. Each task: 1-2 lines max. Use checklist format, not paragraphs.
- **Review workload guard**: ALWAYS include the Review Workload Forecast. If likely above 400 changed lines, recommend chained PRs and honor the received delivery strategy for whether a decision/exception is needed before apply.
- **Work-unit evidence**: every suggested work unit MUST name its Focused test command, Runtime harness command/scenario (or explicit `N/A` reason), and Rollback boundary.
- Return envelope per **Section D** from `skills/_shared/sdd-phase-common.md`.
