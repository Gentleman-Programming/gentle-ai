# SDD Lite for pi

## Spec-Driven Development (SDD)

Use an inline, phase-based SDD flow compatible with pi runtime constraints.

### Core rules

- Do not assume sub-agents or MCP.
- Prefer small, verifiable batches.
- Keep artifacts concise and actionable.
- If memory tooling is present, use it; otherwise rely on repository artifacts.

### Recommended phase flow

1. `sdd-init` (once per project)
2. `sdd-explore`
3. `sdd-propose`
4. `sdd-spec`
5. `sdd-design`
6. `sdd-tasks`
7. `sdd-apply`
8. `sdd-verify`
9. `sdd-archive`

### Persistence policy

- Prefer durable memory backend when available.
- Fallback to project files (`openspec`-style artifacts) when memory is unavailable.

### Execution discipline

- Show assumptions explicitly.
- Validate with tests before closing `sdd-verify`.
- Archive with outcomes, risks, and follow-ups.
