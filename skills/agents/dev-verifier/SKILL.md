---
name: dev-verifier
description: "Trigger: dev verifier, dev-verifier, integration verifier. Validates implementations strictly against tasks and specs."
license: Apache-2.0
metadata:
  author: Jhunior Gutierrez
  version: "1.0"
---

## Activation Contract
Use this skill to independently verify that the completed work meets the specifications and design.

## Hard Rules
- The Verifier MUST NOT inherit the reasoning from the Implementer. It must act independently.
- Check explicitly: `SPEC-001 PASS`, `SPEC-002 FAIL`. Do not output vague statements like "looks good".

## Decision Gates
| Need | Action |
|------|--------|
| Spec fails verification | Reject and send back to Implementer with failure details |

## Execution Steps
1. Ingest Specs, Design, Tasks, Code Diff, Build results, and Test results.
2. Methodically check each Acceptance Criterion from the Specs.
3. Run integration tests or validation commands if applicable.
4. Output a strict PASS/FAIL for each spec.

## Artifact Contract
Follow **Section B** (retrieval) and **Section C** (persistence) from your agent's own `_shared/sdd-phase-common.md` (e.g. `~/.claude/skills/_shared/...` for Claude Code, `~/.gemini/skills/_shared/...` for Gemini, `~/.codex/skills/_shared/...` for Codex — resolve via your own agent's skills root, not necessarily Claude's).
- Reads: `sdd/{change-name}/spec`, `sdd/{change-name}/tasks` (both required), and **every** `sdd/{change-name}/apply-progress/{repo-slug}` for repositories touched by this change (multi-repo aggregation — search the prefix, do not assume a single repo).
- Writes: `sdd/{change-name}/verify-report` — same topic_key `sdd-verify` uses.

## Output Contract
Return the structured envelope from **Section D** of `sdd-phase-common.md`. The `executive_summary`/inline body must include: PASS/FAIL per spec and per task, along with any warnings or blockers, broken down by repository when the change is multi-repo.
