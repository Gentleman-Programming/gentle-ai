# Proposal: contextual-skill-loading

**Change ID**: `contextual-skill-loading`
**Worktree**: `/Users/alanbuscaglia/work/gentle-ai-claude-skills`
**Branch**: `feat/claude-contextual-skill-loading`
**Phase**: propose
**Strict TDD**: enabled (`go test ./...`)

---

## 1. Intent

Users who install gentle-ai expect their authored skills under `~/.claude/skills/` to be invoked proactively by Claude Code — that is the harness's whole promise. Today the skills are correctly listed in the model's `<available_skills>` system block (verified live: 50/52 user-installed `SKILL.md` use `description: >` block scalars and Claude Code parses them fine, Trigger phrases preserved), so this is **not a parsing bug**. The model still under-invokes them because gentle-ai's emitted persona file actively **competes** with `<available_skills>`: `internal/assets/claude/persona-gentleman.md:49-58` injects a hardcoded "Skills (Auto-load based on context)" table that names only `go-testing` and `skill-creator`, narrowing the model's "motivated universe" to those two. There is also no general MANDATORY-style directive instructing the model to consult `<available_skills>` and invoke matching skills via the built-in `Skill` tool before responding — the maintainer has such a directive in their personal `~/.claude/CLAUDE.md`, but it never ships to users. Success is binary and observable: a fresh gentle-ai install + a user request matching any installed skill triggers the `Skill` tool without the user editing any config or maintaining any trigger table.

Two prior root-cause hypotheses (multi-layer runtime stack with `skill(name)` tool; YAML block-scalar parsing failure) were investigated and refuted. This proposal supersedes them. See `sdd/contextual-skill-loading/decision` and `sdd/contextual-skill-loading/opencode-mechanism` in engram for the full chain.

---

## 2. Scope (four pieces — A central, B central, C+D supporting)

| Piece | Type | Files | Central? |
|-------|------|-------|----------|
| A | Remove competing trigger table | `internal/assets/claude/persona-gentleman.md` (+ propose for opencode/generic) | Supporting |
| B | Inject generic mandatory skill directive | Same persona files | **CENTRAL FIX** |
| C | Frontmatter hygiene | 2 SKILL.md files | Minor |
| D | Block-scalar → single-line description | All 21 SKILL.md files | Compatibility insurance |

## Success criteria

- A user installs gentle-ai into a fresh Claude Code home dir, makes a request matching any installed skill (e.g. `react-19`, `typescript`, `playwright`), and observes the `Skill` tool being invoked **without** the user editing any config or maintaining any trigger table.
- All Go tests pass (`go test ./...`), including the new frontmatter linter.
- All golden files reflect the new persona content.
- No regression in existing skill invocation paths (`go-testing`, `skill-creator`, SDD skills) — these still get invoked via `<available_skills>` discovery, just without a hardcoded persona table backing them.
