# Delta B — Generic Skill-Invocation Directive (CENTRAL)

**Domain**: all 6 persona assets (claude, opencode, generic, kiro, kimi) — per-agent variant wording
**Type**: ADDED
**Status**: SHIPPED — commit 9bd58d9
**Files**:
- `internal/assets/claude/persona-gentleman.md` — Claude variant (names the built-in `Skill` tool)
- `internal/assets/opencode/persona-gentleman.md` — non-Claude variant
- `internal/assets/generic/persona-gentleman.md` — non-Claude variant
- `internal/assets/generic/persona-neutral.md` — non-Claude variant
- `internal/assets/kiro/persona-gentleman.md` — non-Claude variant
- `internal/assets/kimi/persona-gentleman.md` — non-Claude variant

## Context

After Delta A removes the hardcoded trigger table from all 6 personas, each persona MUST acquire a short behavioral directive that tells the model (or agent) to use `<available_skills>` for all skill discovery. This is the CENTRAL behavioral fix. Without it, skills listed in `<available_skills>` remain available but the model has no explicit instruction to consult and act on that list.

**Per-agent variant policy** (design Decision 2, option β'):
- Claude variant names the built-in `Skill` tool explicitly: "invoke it via the built-in `Skill` tool"
- Non-Claude variants substitute: "read the matching SKILL.md (using your agent's read mechanism)"

All variants share the same mandatory-phrasing structure.

---

## ADDED Requirements

### Requirement: Generic Skill-Invocation Directive

All 6 persona sources MUST contain a directive that instructs the model or agent to evaluate `<available_skills>` before responding and invoke (or read) the matching skill BEFORE generating its reply.

The directive MUST:
- Reference `<available_skills>` by that exact token
- Name the invocation mechanism explicitly (Claude: `Skill` tool; non-Claude: "read the matching SKILL.md")
- Use mandatory phrasing (MANDATORY, blocking requirement, discipline failure)
- Instruct action BEFORE generating the reply
- NOT enumerate specific skill names or file paths
- NOT use a markdown table

The section heading MUST be `## Contextual Skill Loading (MANDATORY)`.

#### Scenario: Rendered directive contains all required structural tokens

- GIVEN each of the 6 persona files after Delta B is applied (commit 9bd58d9)
- WHEN the file content is asserted by `TestPersonasContainContextualSkillLoadingDirective`
- THEN the heading `## Contextual Skill Loading (MANDATORY)` is present
- AND the token `<available_skills>` is present
- AND the phrase `Self-check BEFORE every response` is present
- AND the phrase `blocking requirement` is present
- AND for Claude: the phrase `invoke it via the built-in \`Skill\` tool` is present
- AND for non-Claude: the phrase `read the matching SKILL.md` is present

---

### Behavioral Verification (deferred)

Structural-only evidence cannot prove that a live model invokes the `Skill` tool proactively. The scenario above validates that the directive is correctly rendered into the persona asset; it cannot validate runtime model behavior.

Behavioral verification is deferred to a future change that should verify:
- A Claude Code session with at least one skill installed triggers a `Skill` tool call BEFORE any code modification when the user asks a task covered by that skill.
- The invocation occurs without any manual `~/.claude/CLAUDE.md` skill-trigger table (i.e. the directive in the persona asset alone is sufficient).
- Non-Claude variants read the matching SKILL.md via their native read mechanism before generating the response.

Such verification requires either a manual transcript capture or a Claude API automated test. Both were considered and deferred in this change in favor of speed (option C, structural-only).

---

## Test Surface

All 6 personas verified by `TestPersonasContainContextualSkillLoadingDirective` and corresponding golden tests — all green.
