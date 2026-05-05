# Delta D — Block-Scalar Flattening (Compatibility Insurance)

**Domain**: skills/frontmatter — all 21 embedded SKILL.md files
**Type**: MODIFIED (21 files)
**Status**: SHIPPED — commit 31ca188
**Files**: ALL `internal/assets/skills/*/SKILL.md`

## Context

Every one of the 21 embedded SKILL.md files used YAML block-scalar syntax for `description:` with the `>` (folded) indicator. This folds newlines into spaces, creating a single logical string.

While this works correctly in Claude Code today, flattening to a single-line scalar eliminates a fragility class documented in YAML parser edge cases (issue class #9716). Defense in depth: no parser-ambiguity, no folded-scalar interaction bugs, maximum portability.

---

## MODIFIED Requirements

### Requirement: Single-Line Description Scalar

Every `description:` field in every `internal/assets/skills/*/SKILL.md` MUST be a plain single-line scalar — no `>`, no `|`, no indented continuation lines.

The full description content (including the `Trigger:` sub-phrase) MUST be preserved verbatim.

#### Scenario: All descriptions are single-line after flattening

- GIVEN all 21 files under `internal/assets/skills/*/SKILL.md`
- WHEN each file's frontmatter is parsed
- THEN the `description` field resolves to a single string (no newlines)
- AND the string is fully represented on one line in the source file

#### Scenario: Trigger phrase preserved after flattening

- GIVEN any SKILL.md description containing `Trigger: ...`
- WHEN the `>` block scalar is flattened
- THEN the resulting description string contains the same `Trigger:` phrase
- AND the `<available_skills>` listing seen by the model shows the same Trigger phrase as before

#### Scenario: Frontmatter linter catches any remaining block scalars

- GIVEN the `TestSkillFrontmatterIsLintClean` test
- WHEN a SKILL.md has a `description:` starting with `>` or `|`
- THEN the test fails
- AND the test passes only when ALL 21 files use plain single-line descriptions

---

## Test Surface

All tests green: `TestSkillFrontmatterIsLintClean` (no block-scalar descriptions), ~12+ skill goldens updated to reflect flattened form.
