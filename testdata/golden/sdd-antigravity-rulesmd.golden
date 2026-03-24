# Spec-Driven Development (SDD) — Antigravity

> Antigravity runs as a single agent. All SDD phases are executed sequentially
> by you, in the same session. There is no sub-agent delegation.

## Spec-Driven Development (SDD)

SDD is the structured planning layer for substantial changes.

### Artifact Store Policy

| Mode | Behavior |
|------|----------|
| `engram` | Default when available. Persistent memory across sessions. |
| `openspec` | File-based artifacts. Use only when user explicitly requests. |
| `hybrid` | Both backends. Cross-session recovery + local files. More tokens per op. |
| `none` | Return results inline only. Recommend enabling engram or openspec. |

### Commands
- `/sdd-init` → run the sdd-init skill
- `/sdd-explore <topic>` → run the sdd-explore skill
- `/sdd-new <change>` → run sdd-explore, then sdd-propose
- `/sdd-continue [change]` → create next missing artifact in dependency chain
- `/sdd-ff [change]` → run sdd-propose → sdd-spec → sdd-design → sdd-tasks in sequence
- `/sdd-apply [change]` → run sdd-apply in batches
- `/sdd-verify [change]` → run sdd-verify
- `/sdd-archive [change]` → run sdd-archive
- `/sdd-new`, `/sdd-continue`, and `/sdd-ff` are meta-commands handled by YOU directly. Do NOT invoke them as skills.

### Dependency Graph
```
proposal -> specs --> tasks -> apply -> verify -> archive
             ^
             |
           design
```

### Execution Model (Single Agent)

Since Antigravity does not support sub-agent invocation, YOU execute each phase
directly by loading the corresponding skill and following its instructions:

1. Load the skill file for the current phase from `~/.gemini/antigravity/skills/`
2. Execute the phase inline, following all skill instructions
3. Save artifacts to engram before moving to the next phase
4. Confirm with the user before advancing in the dependency chain

### Phase Execution Order

| Phase | Skill to load | Reads | Writes |
|-------|--------------|-------|--------|
| sdd-explore | `sdd-explore/SKILL.md` | nothing | explore artifact |
| sdd-propose | `sdd-propose/SKILL.md` | explore (optional) | proposal artifact |
| sdd-spec | `sdd-spec/SKILL.md` | proposal (required) | spec artifact |
| sdd-design | `sdd-design/SKILL.md` | proposal (required) | design artifact |
| sdd-tasks | `sdd-tasks/SKILL.md` | spec + design (required) | tasks artifact |
| sdd-apply | `sdd-apply/SKILL.md` | tasks + spec + design | apply-progress artifact |
| sdd-verify | `sdd-verify/SKILL.md` | spec + tasks | verify-report artifact |
| sdd-archive | `sdd-archive/SKILL.md` | all artifacts | archive-report artifact |

### Engram Protocol

For each phase, retrieve required artifacts via two steps:
1. `mem_search(query: "{topic_key}", project: "{project}")` → get observation ID
2. `mem_get_observation(id: {id})` → full content (REQUIRED — search results are truncated)

#### Engram Topic Key Format

| Artifact | Topic Key |
|----------|-----------|
| Project context | `sdd-init/{project}` |
| Exploration | `sdd/{change-name}/explore` |
| Proposal | `sdd/{change-name}/proposal` |
| Spec | `sdd/{change-name}/spec` |
| Design | `sdd/{change-name}/design` |
| Tasks | `sdd/{change-name}/tasks` |
| Apply progress | `sdd/{change-name}/apply-progress` |
| Verify report | `sdd/{change-name}/verify-report` |
| Archive report | `sdd/{change-name}/archive-report` |
| DAG state | `sdd/{change-name}/state` |

### State and Conventions

Convention files under `~/.gemini/antigravity/skills/_shared/` (global) or `.agents/skills/_shared/` (workspace): `engram-convention.md`, `persistence-contract.md`, `openspec-convention.md`.

### Recovery Rule

| Mode | Recovery |
|------|----------|
| `engram` | `mem_search(...)` → `mem_get_observation(...)` |
| `openspec` | read `openspec/changes/*/state.yaml` |
| `none` | State not persisted — explain to user |
