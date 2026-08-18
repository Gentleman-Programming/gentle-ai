---
name: database-specialist
description: "Trigger: database specialist, database-specialist, db specialist. Handles complex database migrations, schema changes, and high-risk DB tasks."
license: Apache-2.0
metadata:
  author: Jhunior Gutierrez
  version: "1.0"
---

## Activation Contract
Use this skill when a task involves risky database changes such as PK modifications, massive migrations, new large indexes, partitioning, or cross-database changes.

## Hard Rules
- Focus on backward compatibility, zero-downtime migrations, locking, and performance.
- MUST provide a rollback strategy for every migration.

## Decision Gates
| Need | Action |
|------|--------|
| Task requires downtime | Escalate for architecture gate approval |

## Execution Steps
1. Ingest the Task and database impact analysis.
2. Load database-specific skills (e.g., `postgresql`).
3. Write the migration scripts.
4. Verify index impacts and query performance.
5. Provide data migration scripts if needed.

## Artifact Contract
Follow **Section B** (retrieval) and **Section C** (persistence) from your agent's own `_shared/sdd-phase-common.md` (e.g. `~/.claude/skills/_shared/...` for Claude Code, `~/.gemini/skills/_shared/...` for Gemini, `~/.codex/skills/_shared/...` for Codex — resolve via your own agent's skills root, not necessarily Claude's).
- Reads: `sdd/{change-name}/design` (required, for the data model/transaction section); the `apply-progress/{repo-slug}` of the implementer that flagged the DB risk (if any).
- Writes: `sdd/{change-name}/apply-progress/{repo-slug}`, where `{repo-slug}` is the repository owning the schema you are migrating. Treat this role as a specialized implementer for persistence purposes — same multi-repo scoping rule as `backend-implementer`/`frontend-implementer`: never write the bare unscoped `apply-progress` key.

## Output Contract
Return the structured envelope from **Section D** of `sdd-phase-common.md`. The `executive_summary`/inline body must include: the migration files, rollback scripts, and a risk assessment summary, scoped to the owning repository.
