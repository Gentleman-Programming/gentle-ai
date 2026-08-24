<!-- section:model-capable -->
---
name: sdd-verify
description: "Trigger: SDD verification phase, verify change. Execute tests and prove implementation matches specs, design, and tasks."
disable-model-invocation: true
user-invocable: false
license: MIT
metadata:
  author: gentleman-programming
  version: "3.0"
  delegate_only: true
---

## Execution Role

Confirm your role before acting. You are the dedicated `sdd-verify` sub-agent unless you loaded this skill directly through the `skill()` tool.

- If you are the `sdd-verify` sub-agent, continue with the phase work below. Do not delegate. Do not call the Skill tool.
- If you loaded this skill through the `skill()` tool, you are the orchestrator. Stop here and delegate to the dedicated `sdd-verify` sub-agent using your platform's delegation primitive (for example, `task(...)` or a sub-agent invocation).

## Language Domain Contract

Generated technical artifacts default to English. Do not inherit the user's conversational language or the active persona's regional voice for SDD artifacts unless the user explicitly requests that artifact language or the project convention requires it.

If technical artifacts are explicitly requested in another language, use a neutral/professional register unless the user explicitly requests a different tone or regional variant.

Public/contextual comments follow the target context language by default. Explicit user language or tone overrides win; otherwise use a neutral/professional register unless the target context clearly calls for another tone or regional variant.

## Activation Contract

Run when the orchestrator launches verification for an SDD change. You are the quality gate: prove completion with source inspection plus real execution evidence.

The orchestrator should provide structured status from `skills/_shared/sdd-status-contract.md`. Use its `schemaName`, `planningHome`, `changeRoot`, `artifactPaths`, `contextFiles`, task progress, dependency states, and `actionContext` before judging artifacts.

## Hard Rules

- Read all available status `contextFiles` before judging implementation. Full spec-driven verification reads proposal, specs, design, and tasks; partial artifact sets degrade as described below.
- Run full verification only after all tasks are complete. If any task is pending, return `blocked` without running the full suite.
- Execute relevant tests; static analysis alone is never verification.
- A spec scenario is compliant only when a covering test passed at runtime.
- Compare specs first, design second, task completion third.
- Do not fix issues; report them for the orchestrator/user.
- Build the complete report as exact candidate bytes, then run `gentle-ai sdd-verify-validate` with authoritative spec counts before any OpenSpec or Engram write. If the validator is unavailable or denies admission, make zero writes and leave the prior report untouched; otherwise persist the same bytes, including a valid `fail`.
- Persist `verify-report` according to mode: Engram, openspec file, hybrid both, or inline-only for `none`.
- For the final OpenSpec `verify` work unit, persist the canonical passing `openspec/changes/{change}/verify-report.md` before settlement. Native settlement reads, strictly admits, and immutably attests the exact report bytes and resulting candidate tree; never provide a caller digest.
- If Strict TDD is active, load `strict-tdd-verify.md` from this skill directory; if inactive, never load it.
- Return the Section D envelope from `../_shared/sdd-phase-common.md`.
- Count the actual requirements and scenarios from the retrieved specs; never invent envelope totals.
- Record current test/build commands, exit codes, and `test_output_hash` / `build_output_hash` values in the strict envelope.
- Model/provider/profile/effort selection remains user-owned and is never changed by verification.
- This is the one independent requirements/runtime final verification. A contradiction or new failing check returns FAIL/escalation; it never starts 4R, Judgment Day, a refuter, another correction, or scoped validation.
- Review state is informational and never a verification prerequisite.
- A missing, pending, invalid, or non-allow review state never suppresses tests or builds.
- Native review artifacts, when present, are review-context evidence only. Do not require a transaction, policy, ledger, receipt, bundle, or gate-context artifact to begin or complete independent SDD verification.
- Exit `125` is reserved for an actual verification prerequisite or unavailable verification tooling, never missing review authority.
- Return ordinary verification evidence with the result. Terminal reviewer closure is capture-owned and informational; it is never a verification completion prerequisite.

## Decision Gates

| Condition | Action |
|---|---|
| Orchestrator says `STRICT TDD MODE IS ACTIVE` | Treat as authoritative. |
| Cached/config `strict_tdd: true` and runner exists | Strict TDD verify; load module. |
| Strict TDD false or no runner | Standard verify; skip TDD checks. |
| `actionContext.mode: workspace-planning` | STOP; full workspace implementation verification is not supported in this slice. |
| Only tasks artifact exists | Verify task completion only; skip spec/design correctness and record skipped checks. |
| Tasks + specs exist | Verify completeness and correctness; skip design coherence and record skipped checks. |
| Proposal/specs/design/tasks exist | Verify all dimensions. |
| Task incomplete | CRITICAL for core task, WARNING for cleanup task. |
| Test command exits non-zero | CRITICAL. |
| Spec scenario has no passing covering test | CRITICAL `UNTESTED` or `FAILING`. |
| Design deviation exists | WARNING unless it breaks a spec. |

## Execution Steps

1. Load relevant skills via shared SDD Section A.
2. Retrieve artifacts via shared Section B for the active persistence mode, or read the concrete `contextFiles` from structured status.
3. Resolve testing/TDD mode from cached capabilities, config, or project files.
4. Count completed and incomplete tasks. Any unchecked task blocks full verification; focused checks remain an apply work-unit responsibility.
5. If specs exist, map each spec requirement/scenario to implementation evidence and tests.
6. If design exists, check design decisions against changed code. If design is missing, skip design coherence and record why.
7. Run test, build/type-check, and coverage commands when available. For full spec verification, preserve gentle-ai's stricter runtime evidence: source inspection alone does not prove spec scenario compliance.
8. Build the behavioral compliance matrix from actual test results when specs/scenarios exist.
9. Persist and return the verification report, including skipped dimensions for missing artifacts.

## Output Contract

Return `## Verification Report` with change, mode, completeness table, build/tests/coverage evidence, spec compliance matrix, correctness table, design coherence table, issues grouped as CRITICAL/WARNING/SUGGESTION, and final verdict `PASS`, `PASS WITH WARNINGS`, or `FAIL`.

## Graceful Artifact Handling

- **Tasks only**: verify objective task completion only. Do not claim spec correctness or design coherence. If all tasks are checked and no runtime evidence is available, verdict may be `PASS WITH WARNINGS` for task completion only.
- **Tasks + specs**: verify task completeness and requirement/scenario correctness. Runtime test evidence is still required for full spec scenario compliance; missing covering tests are CRITICAL for required scenarios unless project config explicitly allows manual verification.
- **Full artifacts**: verify completeness, correctness, and coherence.
- **Unchecked tasks**: always remain CRITICAL, even when other artifacts are missing or warnings-only.

## References

- [references/report-format.md](references/report-format.md) — full report template, compliance statuses, and command evidence fields.
- [strict-tdd-verify.md](strict-tdd-verify.md) — load only when Strict TDD is active.
- `../_shared/sdd-phase-common.md` — skill loading, retrieval, persistence, and return envelope.
<!-- /section:model-capable -->

<!-- section:model-small -->
---
name: sdd-verify
description: "Trigger: SDD verification phase, verify change. Execute tests and prove implementation matches specs, design, and tasks."
disable-model-invocation: true
user-invocable: false
license: MIT
metadata:
  author: gentleman-programming
  version: "3.0"
  delegate_only: true
---

> **ORCHESTRATOR GATE**: If you loaded this skill via the `skill()` tool, you are the ORCHESTRATOR — STOP. Do NOT execute these instructions inline. Do NOT delegate, do NOT call task/delegate, do NOT launch sub-agents. Read this SKILL.md and follow it exactly.


## Language Domain Contract

Generated technical artifacts default to English. Do not inherit the user's conversational language or the active persona's regional voice for SDD artifacts unless the user explicitly requests that artifact language or the project convention requires it.

If technical artifacts are explicitly requested in another language, use a neutral/professional register unless the user explicitly requests a different tone or regional variant.

Public/contextual comments follow the target context language by default. Explicit user language or tone overrides win; otherwise use a neutral/professional register unless the target context clearly calls for another tone or regional variant.

## Purpose

You are a VERIFY sub-agent. Your job: check implemented changes match spec acceptance criteria. Do NOT delegate.

## Hard Rules

- Read spec acceptance criteria only
- Count actual requirements and scenarios from the spec instead of copying example totals.
- Inspect changed files listed in apply-progress (or tasks) — limit to those files
- Use structured status when provided; stop on workspace-planning action context
- Run the provided test and build/type-check commands even when `strict_tdd` is inactive; verification requires current evidence.
- Include command, exit code, `test_output_hash`, and `build_output_hash` fields in the strict result envelope.
- Preserve user-owned model/provider/profile/effort selection; do not prescribe or override it.
- Do not fix issues; report them for the orchestrator/user
- A contradiction or failing check escalates; never start another review/fix loop.
- Review state is informational and never a verification prerequisite.
- A missing, pending, invalid, or non-allow review state never suppresses tests or builds.
- Do not require a review transaction, policy, ledger, receipt, bundle, or gate context to begin or complete independent SDD verification.
- Exit `125` is reserved for an actual verification prerequisite or unavailable verification tooling, never missing review authority.
- Return ordinary verification evidence with the result. Terminal reviewer closure remains capture-owned and informational.
- Build the complete report as exact candidate bytes, then run `gentle-ai sdd-verify-validate` with authoritative spec counts before any OpenSpec or Engram write. If the validator is unavailable or denies admission, make zero writes and leave the prior report untouched; otherwise persist the same bytes, including a valid `fail`.
- For the final OpenSpec `verify` work unit, persist the canonical passing `openspec/changes/{change}/verify-report.md` before settlement. Native settlement reads, strictly admits, and immutably attests the exact report bytes and resulting candidate tree; never provide a caller digest.
- Return minimal report

## Return Minimal Report

```json
{
  "status": "pass|fail|warning",
  "checks": [{"criterion": "text", "result": "pass|fail", "evidence": "one-line"}],
  "next": "ready-for-archive|fixes-required"
}
```
<!-- /section:model-small -->
