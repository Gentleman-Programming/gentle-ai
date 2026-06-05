# Delta A — Persona Competing-Table Removal

**Domain**: all 6 persona assets (claude, opencode, generic, kiro, kimi)
**Type**: MODIFIED (section replaced) + REMOVED (table content)
**Status**: SHIPPED — commit 9bd58d9
**Files**:
- `internal/assets/claude/persona-gentleman.md`
- `internal/assets/opencode/persona-gentleman.md`
- `internal/assets/generic/persona-gentleman.md`
- `internal/assets/generic/persona-neutral.md`
- `internal/assets/kiro/persona-gentleman.md`
- `internal/assets/kimi/persona-gentleman.md`

## Context

The "Skills (Auto-load based on context)" block appeared in all 6 persona assets with a two-row markdown table naming only `go-testing` and `skill-creator` with absolute `~/.claude/skills/` paths. This block competed with the native `<available_skills>` discovery mechanism by anchoring model attention to two specific skills and ignoring all others installed by the user.

The original spec covered Claude only; the design phase (Decision 2, option β') expanded scope to all 6 personas with per-agent variant wording. All 6 were updated in a single commit (9bd58d9).

The block MUST be removed from all 6 persona assets and replaced by the directive in Delta B.

---

## REMOVED Requirements

### Requirement: Hardcoded Skill Trigger Table

(Reason: The table names only `go-testing` and `skill-creator`, limiting the model's motivated universe to two skills. It competes with and overrides the native `<available_skills>` mechanism, which already lists ALL installed skills. Users who install additional skills via `gentle-ai install` will not see them invoked because the hardcoded table anchors model attention to two specific entries. The table is also a maintenance liability — it goes stale when new skills are added.)

---

## MODIFIED Requirements

### Requirement: Persona Section Boundary

All 6 persona assets MUST NOT contain the `## Skills (Auto-load based on context)` section or any markdown table with hardcoded skill names or file paths. No skill-name references (`go-testing`, `skill-creator`, etc.), trigger tables, or absolute `~/.claude/skills/` paths SHALL appear in any persona source.

(Previously: all 6 persona assets ended with a `## Skills (Auto-load based on context)` section containing a two-row markdown table naming `go-testing` and `skill-creator` with absolute `~/.claude/skills/` paths.)

#### Scenario: Persona sources contain no skill names

- GIVEN each of the 6 persona files after Delta A+B applied (commit 9bd58d9)
- WHEN a reviewer scans each file for any occurrence of `go-testing` or `skill-creator`
- THEN no matches are found in any of the 6 files

#### Scenario: Persona sources contain no trigger table

- GIVEN each of the 6 persona files
- WHEN a reviewer searches for markdown table syntax (`| Context |`) in each file
- THEN no table with skill names or file paths is found in any persona

#### Scenario: Test enforces directive presence in all 6 personas

- GIVEN the test `TestPersonasContainContextualSkillLoadingDirective` (commit 131707f, package `assets`)
- WHEN it runs against all 6 persona paths
- THEN all 6 pass the assertion for `## Contextual Skill Loading (MANDATORY)`, `<available_skills>`, and `Self-check BEFORE every response`

---

## Test Surface

All 6 personas verified by `TestPersonasContainContextualSkillLoadingDirective` and corresponding golden tests — all green.
