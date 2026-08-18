---
name: dev-designer
description: "Trigger: dev designer, dev-designer, system design. Defines technical design and architecture to fulfill specifications."
license: Apache-2.0
metadata:
  author: Jhunior Gutierrez
  version: "1.0"
---

## Activation Contract
Use this skill to define HOW to technically build what was specified.

## Hard Rules
- Must analyze architecture, components, contracts, events, data persistence, transactions, and security.
- Must plan migrations, backwards compatibility, and rollback strategies.

## Decision Gates
| Need | Action |
|------|--------|
| Database changes required | Flag for `database-specialist` task |
| Breaking changes | Mandate a backward compatibility plan |

## Execution Steps
1. Ingest the proposal and specifications.
2. Design the technical architecture and component interactions.
3. Define API contracts and data models.
4. Formulate rollback and rollout strategies.

## Artifact Contract
Follow **Section B** (retrieval) and **Section C** (persistence) from your agent's own `_shared/sdd-phase-common.md` (e.g. `~/.claude/skills/_shared/...` for Claude Code, `~/.gemini/skills/_shared/...` for Gemini, `~/.codex/skills/_shared/...` for Codex — resolve via your own agent's skills root, not necessarily Claude's).
- Reads: `sdd/{change-name}/proposal` (required).
- Writes: `sdd/{change-name}/design` — same topic_key `sdd-design` uses.

## Output Contract
Return the structured envelope from **Section D** of `sdd-phase-common.md`. The `executive_summary`/inline body must include: architecture, repositories, components, contracts, database, transactions, failure modes, security, observability, rollout, and rollback.
