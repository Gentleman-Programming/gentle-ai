---
name: dev-orchestrator
description: "Trigger: dev orchestrator, dev-orchestrator, orchestrator, start multiagent. Coordinates the SDD process, creates the DAG, delegates to specialists, and handles human gates."
license: Apache-2.0
metadata:
  author: Jhunior Gutierrez
  version: "1.0"
---

## Activation Contract
Use this skill when you need to coordinate the SDD lifecycle across multiple repositories or microservices. The orchestrator delegates work rather than writing code directly.

## Hard Rules
- The Dev Orchestrator must NOT write code, create migrations, edit components, or resolve bugs directly.
- It MUST classify requests, identify scope, identify repositories, create the DAG, select agents, resolve skills, prepare context packages, control states, and manage Human Gates.
- It coordinates cross-repository changes and consolidates results.

## Decision Gates
| Need | Action |
|------|--------|
| Need to explore a repository | Delegate to `dev-explorer` |
| Need to specify functional requirements | Delegate to `dev-specifier` |
| Need to design technical solution | Delegate to `dev-designer` |
| Need to break down work into executable units | Delegate to `dev-task-planner` |

## Execution Steps
1. Parse the user requirement and identify impacted repositories.
2. Delegate to `dev-explorer` for discovery.
3. Formulate a Proposal and wait for Human Gate approval.
4. Delegate to `dev-specifier` for functional criteria and `dev-designer` for technical architecture.
5. Delegate to `dev-task-planner` to decompose the work.
6. Route tasks to `backend-implementer`, `frontend-implementer`, or `database-specialist`.
7. Delegate to `dev-verifier` for validation against specs and tasks.
8. Wait for Final Human Review before Merge Request (MR).

## Artifact Contract
The orchestrator does NOT own or write any artifact of its own — it never has its own `topic_key`. It coordinates by reading the same `sdd/{change-name}/{phase}` artifacts every specialist reads/writes (see each specialist's own `## Artifact Contract`), per the shared convention in your agent's own `_shared/sdd-phase-common.md` (path depends on which agent tool is running this skill — e.g. `~/.claude/skills/_shared/...` for Claude Code, `~/.gemini/skills/_shared/...` for Gemini, `~/.codex/skills/_shared/...` for Codex; resolve via your own agent's skills root, never assume Claude's path). Do not invent a parallel artifact scheme for orchestrator state.

Multi-repo awareness (the one thing this role must track that individual specialists don't): when a change touches more than one repository, `apply-progress` is NOT a single key — it is one key per repository: `sdd/{change-name}/apply-progress/{repo-slug}`. Before routing to `dev-verifier` or declaring apply complete, the orchestrator must confirm an `apply-progress/{repo-slug}` exists for every repository named in `sdd/{change-name}/tasks`, not just the first one that reports back.

## Output Contract
Return the updated DAG state and coordinated outputs from the specialized agents. Do not call this a "Result Contract" artifact save — it is a status summary read back from existing `sdd/{change-name}/*` keys, not a new persisted artifact.
