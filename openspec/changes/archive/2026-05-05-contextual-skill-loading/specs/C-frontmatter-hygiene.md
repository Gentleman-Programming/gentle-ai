# Delta C — Frontmatter Hygiene

**Domain**: skills/frontmatter
**Type**: MODIFIED (two specific SKILL.md files) + NEW test (linter)
**Status**: SHIPPED — fixes in commits 1b2b374 and 45dc833; linter in commit 540d32b
**Files**:
- `internal/assets/skills/chained-pr/SKILL.md` — name field fix (commit 1b2b374)
- `internal/assets/skills/skill-creator/SKILL.md` — allowed-tools removal (commit 45dc833)
- `internal/assets/skills_frontmatter_test.go` — regression guardrail (commit 540d32b)

## Context

Two SKILL.md files had frontmatter anomalies:

1. **`chained-pr/SKILL.md`** had `name: gentle-ai-chained-pr` but the directory slug is `chained-pr`. Fixed in commit 1b2b374. The `name:` field now equals `chained-pr`.

2. **`skill-creator/SKILL.md`** had `allowed-tools:` as a top-level frontmatter key outside the documented schema. Fixed in commit 45dc833 — the key is removed.

---

## Requirements (as shipped)

### Requirement: chained-pr Name Field Alignment

The `name:` field in `internal/assets/skills/chained-pr/SKILL.md` MUST equal the directory basename `chained-pr`.

#### Scenario: Name matches directory slug

- GIVEN the file `internal/assets/skills/chained-pr/SKILL.md`
- WHEN the frontmatter is parsed
- THEN `name` equals `"chained-pr"`

---

### Requirement: skill-creator Non-Standard Field Absence

The `allowed-tools:` top-level key MUST NOT appear in the frontmatter of any SKILL.md. If tool information is needed, it MUST be in the skill body or under the `metadata:` sub-key.

#### Scenario: Frontmatter linter rejects unknown top-level keys

- GIVEN `TestSkillFrontmatterIsLintClean` running against all 21 `skills/*/SKILL.md` files
- WHEN any SKILL.md contains a top-level key outside `{name, description, license, metadata, version}`
- THEN the test fails citing the unknown key

---

## Linter Guardrail

The linter `TestSkillFrontmatterIsLintClean` (commit 540d32b) runs as part of `go test ./...` and enforces:

| Assertion | Enforces |
|-----------|---------|
| `name` equals directory basename | C-1 (chained-pr alignment) |
| No top-level keys outside allowed set | C-2 (no allowed-tools) |

> **Note**: `license` and `metadata.author` are in the allowed-keys whitelist but are NOT actively asserted as required fields. Requiring them is deferred to a future linter hardening change.

---

## Test Surface

All tests green: `TestSkillFrontmatterIsLintClean`, skill-creator goldens (no `allowed-tools:`).
