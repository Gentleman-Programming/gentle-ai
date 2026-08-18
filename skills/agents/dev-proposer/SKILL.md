---
name: dev-proposer
description: "Trigger: dev proposer, dev-proposer, propose solution. Proposes technical approaches, alternatives, and architecture decisions based on exploration."
license: Apache-2.0
metadata:
  author: Jhunior Gutierrez
  version: "1.0"
---

## Activation Contract
Use this skill after the exploration phase to define the high-level approach and technical solution to meet the requirement.

## Hard Rules
- Must provide clear alternatives if the solution is ambiguous.
- Must evaluate pros, cons, and risks of the proposed approach.
- The proposal must be reviewed by the Human Gate before proceeding.

## Decision Gates
| Need | Action |
|------|--------|
| Solution has multiple viable paths | Present options with trade-offs |
| Greenfield project | Request Architecture Gate review |

## Execution Steps
1. Ingest the requirement and the `dev-explorer` output.
2. Outline the high-level architecture changes.
3. List the affected components and databases.
4. Prepare the proposal document for the Orchestrator to present to the user.

## Artifact Contract
Follow **Section B** (retrieval) and **Section C** (persistence) from your agent's own `_shared/sdd-phase-common.md` (e.g. `~/.claude/skills/_shared/...` for Claude Code, `~/.gemini/skills/_shared/...` for Gemini, `~/.codex/skills/_shared/...` for Codex — resolve via your own agent's skills root, not necessarily Claude's).
- Reads: `sdd/{change-name}/explore` (optional, if `dev-explorer` ran first).
- Writes: `sdd/{change-name}/proposal` — same topic_key `sdd-propose` uses. Injected repo-profile/domain context changes the CONTENT of the proposal, not where it lives.

## Output Contract
Return the structured envelope from **Section D** of `sdd-phase-common.md`. The `executive_summary`/inline body must include: the proposal detailing technical approach, alternatives considered, and risks.
