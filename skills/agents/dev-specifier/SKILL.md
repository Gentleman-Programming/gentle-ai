---
name: dev-specifier
description: "Trigger: dev specifier, dev-specifier, write specs. Creates verifiable functional criteria using Given/When/Then format."
license: Apache-2.0
metadata:
  author: Jhunior Gutierrez
  version: "1.0"
---

## Activation Contract
Use this skill to define WHAT must happen based on the approved proposal.

## Hard Rules
- Focus ONLY on what must happen, not HOW to implement it.
- Must use `Given / When / Then` format for acceptance criteria.
- Must include business rules, edge cases, error scenarios, and non-functional constraints.

## Decision Gates
| Need | Action |
|------|--------|
| Edge cases found | Add explicit Given/When/Then scenarios |

## Execution Steps
1. Ingest the approved proposal.
2. Draft functional requirements.
3. Specify all testable scenarios in Given/When/Then.
4. Detail error handling expectations.

## Artifact Contract
Follow **Section B** (retrieval) and **Section C** (persistence) from your agent's own `_shared/sdd-phase-common.md` (e.g. `~/.claude/skills/_shared/...` for Claude Code, `~/.gemini/skills/_shared/...` for Gemini, `~/.codex/skills/_shared/...` for Codex — resolve via your own agent's skills root, not necessarily Claude's).
- Reads: `sdd/{change-name}/proposal` (required).
- Writes: `sdd/{change-name}/spec` — same topic_key `sdd-spec` uses.

## Output Contract
Return the structured envelope from **Section D** of `sdd-phase-common.md`. The `executive_summary`/inline body must include: functional requirements, business rules, edge cases, error scenarios, non-functional constraints, and traceability links.
