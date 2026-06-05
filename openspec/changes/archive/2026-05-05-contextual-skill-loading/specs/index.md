# Spec Index: contextual-skill-loading

**Change**: contextual-skill-loading
**Branch**: feat/claude-contextual-skill-loading
**Worktree**: /Users/alanbuscaglia/work/gentle-ai-claude-skills
**Date**: 2026-05-05
**Status**: SHIPPED — 6 commits, all tests green (reconciled post-apply)

## Summary

Root cause: all six persona assets across `claude/`, `opencode/`, `generic/`, `kiro/`, and `kimi/` contained a hardcoded "Skills (Auto-load based on context)" table that names only two skills, narrowing the model's motivated universe and competing with the native `<available_skills>` block. No generic mandatory directive existed telling any agent to consult `<available_skills>` before responding. Result: skills installed by gentle-ai were listed in `<available_skills>` but not invoked proactively by users who install gentle-ai without the maintainer's personal CLAUDE.md.

## Scope — 6 persona files (shipped)

| File | Variant | Directive wording |
|------|---------|-------------------|
| `internal/assets/claude/persona-gentleman.md` | Claude | Names the built-in `Skill` tool |
| `internal/assets/opencode/persona-gentleman.md` | non-Claude | "read the matching SKILL.md (using your agent's read mechanism)" |
| `internal/assets/generic/persona-gentleman.md` | non-Claude | same as opencode |
| `internal/assets/generic/persona-neutral.md` | non-Claude | same as opencode |
| `internal/assets/kiro/persona-gentleman.md` | non-Claude | same as opencode |
| `internal/assets/kimi/persona-gentleman.md` | non-Claude | same as opencode |

## Work Units

| ID | Title | Type | File |
|----|-------|------|------|
| A | Persona competing-table removal | MODIFIED + REMOVED | [A-persona-table.md](A-persona-table.md) |
| B | Generic skill-invocation directive | ADDED | [B-skill-directive.md](B-skill-directive.md) |
| C | Frontmatter hygiene | MODIFIED | [C-frontmatter-hygiene.md](C-frontmatter-hygiene.md) |
| D | Block-scalar flattening | MODIFIED | [D-block-scalar-flatten.md](D-block-scalar-flatten.md) |

## Out of Scope

- Skill-registry bootstrap at install time
- Agents without SkillsDir (windsurf/kimi/qwen/kiro/codex) for skill-invocation logic — they inherit the directive from generic personas but cannot act on `<available_skills>` if the host does not populate it; no worse off than before
- CLAUDE.md trigger-table emission (user territory, constraint confirmed)
- Description content rewriting for clarity
