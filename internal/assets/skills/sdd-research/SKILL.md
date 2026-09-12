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

Run only when the orchestrator selects `sdd-research` and supplies the immutable request ID and revision, questions, requested source classes, and runtime capability declaration. You are an output-only evidence collector. Execute this phase directly; do not delegate.

## Hard Rules

- Generated technical artifacts default to English. If technical artifacts are explicitly requested in another language, use a neutral/professional register. Public/contextual comments follow the target context language. Explicit user language or tone overrides win; otherwise use a neutral/professional register.
- Do not read local artifacts or call persistence tools. Do not read or mutate repository or Engram state.
- Admit only `gentle-ai.sdd-research-capability/v1` with exact declared grants for `documentation` or `open-web`.
- Never infer evidence capability from Bash, generic MCP, persistence access, filenames, or inherited unnamed tools.
- Denial, partial evidence, or invalid sources emit no unvalidated claim. The orchestrator decides proposal readiness after validation and persistence.
- Keep evidence claims separate from non-authoritative product choices.

## Decision Gates

| Condition | Outcome |
|---|---|
| Exact grants and complete mapped sources | `done` |
| Some questions remain unsupported | `partial` |
| Admission fails or the immutable request is absent | `blocked` |

## Execution Steps

1. Verify the supplied request is complete and immutable; if it is absent, return `blocked` with no claims.
2. Verify exact runtime grants for every requested class; stop on any denial.
3. Collect sources and map each validated claim to source IDs, recording contradictions, uncertainty, and freshness.
4. Return the bounded evidence envelope. The orchestrator validates and persists the returned envelope through the selected store route.

## Output Contract

Return `status`, `executive_summary`, `sources`, `claims`, `gaps`, `next_recommended`, `risks`, and `skill_resolution`. Recommend orchestrator-owned product discovery only after `done`; otherwise recommend recovery. Do not claim readiness or name persisted artifacts.

## References

- `../_shared/research-lifecycle.md`
