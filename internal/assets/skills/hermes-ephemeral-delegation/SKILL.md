---
name: hermes-ephemeral-delegation
description: "Trigger: broad exploration, multi-file reads, tests/builds, fresh review, or multi-step debug. Orchestrate complex work via delegate_task to protect context."
license: Apache-2.0
metadata:
  author: gentleman-programming
  version: "1.0"
---

## Activation Contract

Load this skill when you are acting as the parent orchestrator and the work ahead falls into any of these categories:

- Broad exploration (4+ files to understand, codebase mapping, approach comparison)
- Multi-file implementation (touching 2+ non-trivial files)
- Test or build execution
- Fresh adversarial review (diffs, PR readiness, incident audit)
- Multi-step debugging that would flood the parent context

Do NOT load this skill if you are already inside a delegated child task — you are the executor, not the orchestrator.

## Hard Rules

- Use `delegate_task` for all complex work listed above. Do NOT execute it inline.
- Workers are EPHEMERAL: each `delegate_task` call creates a fresh context. Do NOT request persistent agent files or profiles.
- Pass a self-contained mission. Workers have no memory of the parent conversation.
- Treat worker output as self-report: verify file writes, test pass/fail, URLs, and IDs before reporting success to the user.
- Batch parallel calls only for INDEPENDENT workstreams. Sequential dependencies must run sequentially.

## Decision Gates

| Situation | Action |
|-----------|--------|
| Need to read 4+ files to understand | Delegate a narrow exploration worker |
| Need to write 2+ non-trivial files | Delegate a single writer with the full mission |
| Need to run tests or builds | Delegate an executor; do not run inline |
| Need an adversarial review of a diff | Delegate a fresh-context reviewer |
| Multi-step debug that grows the context | Delegate a debug worker; feed results back inline |
| Simple 1-file edit you already understand | Do it inline; no delegation needed |
| Quick git/state check | Do it inline; no delegation needed |

## Execution Steps

1. Identify which gate applies. If none applies, skip delegation.
2. Draft a self-contained mission for the worker — include:
   - Exact goal (one sentence)
   - File paths or targets to act on
   - Relevant prior context the worker needs (decisions, conventions, prior findings)
   - Constraints (style, test runner, budget)
   - Expected evidence to return (e.g., file written, test output, URL found)
   - Allowed toolsets/MCP/skills the worker should use
   - Any `SKILL.md` paths to load before work
3. Call `delegate_task` with that mission.
4. Wait for the worker summary.
5. Verify the claimed output (check file existence, test result, side effect).
6. Synthesize the verified result into your orchestrator reply.

## Output Contract

After synthesizing worker results, return:

- What was delegated and to how many workers
- What each worker returned (verified, not just claimed)
- Any discrepancy between worker self-report and verified evidence
- Final answer or next step for the user

## References

- [references/tuning-knobs.md](references/tuning-knobs.md) — Full table of `delegate_task` configuration parameters and the explicit toolset/MCP/skill checklist for worker missions.
- [../../hermes/sdd-orchestrator.md](../../hermes/sdd-orchestrator.md) — SDD orchestrator protocol that uses this delegation standard for SDD phase work.
