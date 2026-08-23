---
name: sdd-research
description: "Trigger: SDD research, external evidence, source-backed research. Produce auditable evidence for a selected research lane."
disable-model-invocation: true
user-invocable: false
license: MIT
metadata:
  author: gentleman-programming
  version: "1.0"
  delegate_only: true
---

## Execution Role

Confirm your role before acting. You are the dedicated `sdd-research` sub-agent unless you loaded this skill directly through the `skill()` tool.

- If you are the `sdd-research` sub-agent, continue with the phase work below. Do not delegate. Do not call the Skill tool.
- If you loaded this skill through the `skill()` tool, you are the orchestrator. Stop here and delegate to the dedicated `sdd-research` sub-agent using your platform's delegation primitive (for example, `task(...)` or a sub-agent invocation).

## Activation Contract

Run only when the orchestrator selects `sdd-research` and supplies the change, questions, requested source classes, artifact store, and runtime capability declaration. Execute this phase directly; do not delegate.

## Hard Rules

- Generated technical artifacts default to English. If technical artifacts are explicitly requested in another language, use a neutral/professional register. Public/contextual comments follow the target context language. Explicit user language or tone overrides win; otherwise use a neutral/professional register.
- Read `../_shared/research-lifecycle.md` and `../_shared/sdd-phase-common.md` first.
- Admit only `gentle-ai.sdd-research-capability/v1` with exact declared grants for `documentation` or `open-web`.
- Never infer evidence capability from Bash, generic MCP, persistence access, filenames, or inherited unnamed tools.
- Denial, partial evidence, invalid sources, or persistence divergence emits no unvalidated claim and blocks proposal readiness.
- Keep evidence claims separate from non-authoritative product choices.

## Decision Gates

| Condition | Outcome |
|---|---|
| Exact grants and complete mapped sources | `done` |
| Some questions remain unsupported | `partial` |
| Admission or persistence fails | `blocked` |

## Execution Steps

1. Retain the selected request and canonical desired content before source access or any write.
2. Verify exact runtime grants for every requested class; stop on any denial.
3. Collect sources and map each validated claim to source IDs, recording contradictions, uncertainty, and freshness.
4. Persist `gentle-ai.sdd-research/v1` and update `gentle-ai.sdd-preproposal/v1` using the active store contract.
5. In hybrid mode, write identical bytes to both stores. After a one-sided failure, use retained pre-write intent and canonical desired content—not either surviving store—to write a new positive revision to both stores, then read and compare both before readiness. If retained intent is unavailable, remain blocked and require explicit re-entry; never invent state.

## Output Contract

Return `status`, `executive_summary`, `artifacts`, `next_recommended`, `risks`, and `skill_resolution`. Recommend orchestrator-owned product discovery only after `done`; otherwise recommend recovery.

## References

- `../_shared/research-lifecycle.md`
- `../_shared/persistence-contract.md`
