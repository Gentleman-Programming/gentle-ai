---
name: workflow-governance
description: "Trigger: authoring a Workflow under ultracode/xhigh, dynamic orchestration, code fan-out. Carry Agent-tool governance onto the Workflow agent() surface."
license: Apache-2.0
metadata:
  author: gentleman-programming
  version: "1.0"
---

## Activation Contract

Apply when ultracode/xhigh (dynamic workflow orchestration) is ON and you are about to author a Workflow whose `agent()` calls do real work: writing or reviewing code, running SDD phases, or touching engram. Skip for read-only exploration fan-out and trivial probes.

## Hard Rules

- NEVER call bare `agent()` for substantive work. Route every such call through `launchSubagent(role, task, ...)` from `assets/governance-preamble.js`.
- `opts.model` is MANDATORY on every delegation — Agent tool OR Workflow `agent()`. Resolve the alias from the Model Assignments table; a bare `agent()` silently inherits the session model (Opus under xhigh), inverting the cost tiering.
- For SDD/judgment roles, set `opts.agentType` to the registered agent (`sdd-apply`, `sdd-verify`, …) so the SKILL.md hard-gates (strict TDD, delivery hard-stop, engram read/write rules) are inherited, not bypassed.
- Populate `SKILL_PATHS` from `.atl/skill-registry.md` before first use — do NOT ship hardcoded paths; they break for every other user.
- Inject resolved `## Skills to load before work` paths from `.atl/skill-registry.md`. Pass paths, not summaries.
- Inject the engram contract per call: prior context + topic_key (MERGE, never overwrite) + a `mem_save`-before-returning directive. The MAIN agent (which owns the human turn) still runs `mem_save_prompt` + `mem_session_summary` over the returned results.
- For `sdd-apply`/`sdd-verify`, inject the literal `STRICT TDD MODE IS ACTIVE. Test runner: {cmd}` block ONLY when `strictTdd` is true; resolve `{cmd}` from `sdd-init/{project}`.
- ASSERT governance in the deterministic JS (`assertGoverned`) and FAIL the workflow on any missing model / skill / TDD marker.
- Pin a BACKED engram project for the mem_save/mem_search directives — resolve via mem_current_project; if cwd is ambiguous, pass an explicit project or add .engram/config.json. Do NOT assume cwd resolves to a project.

## Decision Gates

| Role | model | agentType | TDD block |
|------|-------|-----------|-----------|
| sdd-propose / sdd-design | opus | yes | no |
| sdd-explore / spec / tasks / apply / verify | sonnet | yes | apply + verify only |
| sdd-archive | haiku | yes | no |
| jd-judge-a / jd-judge-b / jd-fix-agent | sonnet | yes | no |
| read-only probe / explore | sonnet (set haiku explicitly for cheap probes) | Explore | no |

## Execution Steps

1. Copy `assets/governance-preamble.js` to the TOP of the workflow script (Workflow scripts cannot import files).
2. Populate `SKILL_PATHS` by reading `.atl/skill-registry.md` (Path column = per-user absolute paths).
3. Resolve once up front: `strictTdd` + `testCommand` via `mem_search("sdd-init/{project}")`; matching skill paths per the registry loading protocol.
4. Replace every substantive `agent(...)` with `launchSubagent(role, task, { ctx, schema, phase })`.
5. After fan-out, assert there are no `assertGoverned` errors; on failure, re-resolve and relaunch rather than continuing.

## Output Contract

Report: roles launched, model + agentType per role, skill paths injected, whether the TDD and engram blocks were present, and the `assertGoverned` result (pass / fail per collision: model, skills, engram, TDD).

## References

- `assets/governance-preamble.js` — drop-in helper (MODEL_BY_ROLE, AGENTTYPE_BY_ROLE, SKILL_PATHS, buildGoverned, launchSubagent, assertGoverned).
