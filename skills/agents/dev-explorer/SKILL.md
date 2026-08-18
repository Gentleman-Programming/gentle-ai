---
name: dev-explorer
description: "Trigger: dev explorer, dev-explorer, explore repo. Analyzes the codebase structure, modules, dependencies, and risks without making modifications."
license: Apache-2.0
metadata:
  author: Jhunior Gutierrez
  version: "1.0"
---

## Activation Contract
Use this skill when exploring an existing system or repository to understand its current behavior, architecture, and constraints.

## Hard Rules
- READ ONLY. Do NOT implement, edit, decide architecture, or assume missing information.
- Analyze structure, architecture, modules, dependencies, tests, documentation, contracts, and risks.

## Decision Gates
| Need | Action |
|------|--------|
| Missing context | Request more reads via Orchestrator |
| Conflicting implementations | Document as contradiction in risks |

## Execution Steps
1. Navigate the target repository.
2. Read the main components, modules, and tests.
3. Identify external dependencies and contracts.
4. Note any risks, contradictions, or edge cases.

## Artifact Contract
Follow **Section B** (retrieval) and **Section C** (persistence) from your agent's own `_shared/sdd-phase-common.md` (e.g. `~/.claude/skills/_shared/...` for Claude Code, `~/.gemini/skills/_shared/...` for Gemini, `~/.codex/skills/_shared/...` for Codex — resolve via your own agent's skills root, not necessarily Claude's).
- Reads: none required (optional prior `sdd/{change-name}/explore` if resuming).
- Writes: `sdd/{change-name}/explore` — same topic_key `sdd-explore` uses. This is not a parallel artifact scheme; a domain-aware exploration and a plain `sdd-explore` run are interchangeable to downstream phases.

## Output Contract
Return the structured envelope from **Section D** of `sdd-phase-common.md` (`status`, `executive_summary`, `artifacts`, `next_recommended`, `risks`, `skill_resolution`). The `executive_summary`/inline body must include: `repositories`, `impacted_modules`, `current_behavior`, `dependencies`, `similar_implementations`, `contracts`, `tests`, `risks`, `unknowns`, and `contradictions`.
