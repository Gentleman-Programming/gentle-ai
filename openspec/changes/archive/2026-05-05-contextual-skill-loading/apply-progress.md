# Apply Progress — contextual-skill-loading

**Worktree**: /Users/alanbuscaglia/work/gentle-ai-claude-skills
**Branch**: feat/claude-contextual-skill-loading
**Mode**: Strict TDD (`go test ./...`)
**Delivery strategy**: ask-on-risk — single PR within budget (Low risk per tasks.md forecast)
**Status**: All 7 tasks complete — ready for `sdd-verify`

---

## Phase Summary

| Phase | Status | Commits |
|-------|--------|---------|
| Phase 1 — Frontmatter linter foundation (T1) | done | `540d32b` |
| Phase 2 — Frontmatter hygiene (T2, T3) | done | `1b2b374`, `45dc833` |
| Phase 3 — Block-scalar flatten (T4) | done | `31ca188` |
| Phase 4 — Persona directive injection (T5, T6) | done | `131707f`, `9bd58d9` |
| Phase 5 — Verification (T7) | done | n/a (verify-only) |

---

## TDD Cycle Evidence

| Task | Test File | Layer | Safety Net | RED | GREEN | TRIANGULATE | REFACTOR |
|------|-----------|-------|------------|-----|-------|-------------|----------|
| T1 | `internal/assets/skills_frontmatter_test.go` (new) | Unit | green baseline (`go test ./...` ok before commit) | written, executed: 21/21 sub-tests FAIL — block scalars + name mismatch + allowed-tools surfaced | n/a (T1 is RED-only by design) | n/a (T1 is the test itself) | n/a |
| T2 | reuses `TestSkillFrontmatterIsLintClean/skills/chained-pr` | Unit | linter RED across 21 files | n/a (uses existing RED) | name assertion now passes for chained-pr; description block-scalar still RED (intentional, fixed in T4) | n/a (single fix) | n/a |
| T3 | reuses `TestSkillFrontmatterIsLintClean/skills/skill-creator` + 4 skill-creator goldens | Unit + Golden | linter RED + 4 goldens RED after edit | n/a | allowed-tools assertion now passes for skill-creator; 4 goldens regenerated; full suite green except linter (still RED on block scalars) | n/a (single fix) | n/a |
| T4 | reuses `TestSkillFrontmatterIsLintClean` (all 21) + 16 installed-skill goldens | Unit + Golden | linter still RED on block scalars (T1+T2+T3 partial) | n/a | linter fully GREEN (21/21); 16 goldens regenerated; `go test ./...` fully green | n/a (single mechanical refactor across 21 files) | n/a |
| T5 | `internal/assets/assets_test.go` — `TestPersonasContainContextualSkillLoadingDirective` (new function) | Unit | full suite green pre-commit | written, executed: 6/6 sub-tests FAIL — old table present + new directive absent on every persona | n/a (T5 is RED-only) | n/a (T5 is the test itself) | n/a |
| T6 | reuses `TestPersonasContainContextualSkillLoadingDirective` + 9 persona/combined goldens + cursor smoke test | Unit + Golden | persona test RED, all other tests green | n/a | persona directive test GREEN (6/6); 9 goldens regenerated; cursor smoke assertion updated; full `go test ./...` green | n/a (atomic A+B; per design Decision 5 step 6, A and B not independently testable) | n/a |
| T7 | full `go test ./...` | Suite | n/a | n/a | full suite GREEN (no failures, no skips); manual `rg` checks confirm: no `description: >`/`|` in any of 21 SKILL.md frontmatters; no `Skills (Auto-load based on context)` heading in any of 6 personas; chained-pr name = chained-pr; skill-creator has no `allowed-tools`; all 6 personas contain MANDATORY directive + `<available_skills>` + Self-check directive. | n/a (verify-only) | n/a |

### Test Summary

- **Total tests written this change**: 2 (`TestSkillFrontmatterIsLintClean`, `TestPersonasContainContextualSkillLoadingDirective`)
- **Total tests passing**: full `go test ./...` green
- **Layers used**: Unit (2), Golden (29 regenerated)
- **Approval tests**: none (no refactoring of existing behavior — only mechanical asset edits with golden refresh)
- **Pure functions created**: linter helpers (`extractSkillFrontmatter`, `unquote`, `skillDirBasename`) — pure, deterministic, no side effects

---

## Files Changed

| File | Action | What |
|------|--------|------|
| `internal/assets/skills_frontmatter_test.go` | created | new linter (frontmatter parses, name == basename, plain scalar description, Trigger present, whitelist) |
| `internal/assets/skills/chained-pr/SKILL.md` | modified | `name: gentle-ai-chained-pr` → `name: chained-pr` (T2); description flattened (T4) |
| `internal/assets/skills/skill-creator/SKILL.md` | modified | dropped `allowed-tools:` top-level key (T3); description flattened (T4) |
| `internal/assets/skills/{_shared, branch-pr, cognitive-doc-design, comment-writer, go-testing, issue-creation, judgment-day, sdd-apply, sdd-archive, sdd-design, sdd-explore, sdd-init, sdd-onboard, sdd-propose, sdd-spec, sdd-tasks, sdd-verify, skill-registry, work-unit-commits}/SKILL.md` | modified | description block-scalar → plain double-quoted single-line scalar (T4) |
| `internal/assets/assets_test.go` | modified | added `TestPersonasContainContextualSkillLoadingDirective` covering 6 personas (T5) |
| `internal/assets/claude/persona-gentleman.md` | modified | removed `## Skills (Auto-load based on context)` table; injected verbatim B2 directive naming `Skill` tool (T6) |
| `internal/assets/{opencode, generic, kiro, kimi}/persona-gentleman.md` + `internal/assets/generic/persona-neutral.md` | modified | removed table; injected non-Claude variant of B2 directive ("read the matching SKILL.md (using your agent's read mechanism)") across 5 files (T6) |
| `internal/components/persona/inject_test.go` | modified | updated cursor persona smoke assertion to match the new directive heading (T6) |
| `testdata/golden/skills-{claude, opencode, windsurf, kiro}-skill-creator.golden` | regenerated | T3 + T4 (allowed-tools removal + description flatten) |
| `testdata/golden/skills-{claude, opencode, windsurf, kiro}-go-testing.golden` | regenerated | T4 (description flatten) |
| `testdata/golden/sdd-{antigravity, codex, cursor, gemini, kiro, opencode, vscode, windsurf}-skill-sdd-init.golden` | regenerated | T4 (description flatten — 8 files; claude SDD goldens do not include installed SKILL.md) |
| `testdata/golden/persona-{claude-gentleman, claude-neutral, opencode-gentleman, opencode-neutral, windsurf-gentleman, kiro-gentleman, antigravity-gentleman}.golden` + `testdata/golden/combined-{claude-claudemd, windsurf-global-rules}.golden` | regenerated | T6 (table → directive across 9 goldens) |
| `openspec/changes/contextual-skill-loading/tasks.md` | modified | marked T1–T7 `[x]` |
| `openspec/changes/contextual-skill-loading/apply-progress.md` | created | this artifact |

---

## Commits (chronological)

1. `540d32b` — `test(assets): add SKILL.md frontmatter linter`
2. `1b2b374` — `fix(skills): align chained-pr SKILL.md name with directory`
3. `45dc833` — `fix(skills): drop non-standard allowed-tools field from skill-creator`
4. `31ca188` — `refactor(skills): flatten SKILL.md description frontmatter`
5. `131707f` — `test(assets): require contextual skill loading directive in personas`
6. `9bd58d9` — `feat(persona): replace skills auto-load table with mandatory contextual directive`

Six work-unit commits, conventional, no Co-Authored-By trailers, no `--no-verify`. Branch is NOT pushed.

---

## Deviations from Design

1. **Cursor persona smoke test fix (extra)**: `internal/components/persona/inject_test.go` had an existing assertion `strings.Contains(text, "Skills")` to confirm the persona content (vs neutral fallback) was used. The previous heading included the literal substring "Skills"; the new heading is "Contextual Skill Loading". I updated the assertion to look for "Contextual Skill Loading" — that's a behavioral confirmation of the same idea (persona has the skill section). This was not in the design's enumerated golden list but is a single-line, semantically equivalent fix and was committed as part of T6 (where the change forced it). Documented here for the verify phase.

2. **Goldens regenerated count**: design Decision 4 said "all `skills-{agent}-go-testing.golden` files (D)" and "~9× `sdd-{agent}-skill-sdd-init.golden` files (D, mechanical regen)". Actual counts during apply: 4 go-testing goldens + 8 sdd-skill-sdd-init goldens (no claude variant — `TestGoldenSDD_Claude` does not check installed SKILL.md, only commands and agents). Within the design's range; no semantic deviation. The design said "Total ~21 golden updates from C+D alone" — actual was 16 (4 skill-creator + 4 go-testing + 8 sdd-init). Likely the design counted skill-creator and go-testing per agent (4+4=8) plus 9 sdd-init = 17 expected; actual 16 fits this within rounding. No issue.

3. **Persona variant wording**: I used the exact phrase **"read the matching SKILL.md (using your agent's read mechanism)"** for non-Claude variants, per design Decision 2. Verified by `TestPersonasContainContextualSkillLoadingDirective` GREEN.

No semantic deviations from the design — all behavior-bearing decisions (B2 wording verbatim, β' per-agent variant, D quoting style with `"..."`, drop-not-move for `allowed-tools`) followed exactly.

---

## Issues Found

None blocking. The cursor persona smoke test (item 1 in deviations above) was a pre-existing brittle assertion the design did not flag — it was caught immediately at full-suite green-check time, fixed in the same commit (T6) since it logically belongs there.

---

## Remaining Tasks

None. All 7 tasks complete. Ready for `sdd-verify`.

---

## Workload / PR Boundary

- Mode: single PR
- Current work unit: full change (6 commits)
- Boundary: starts from `5f6af97` (origin/feat/claude-contextual-skill-loading head before apply) and ends at `9bd58d9` — six commits, ~280-330 changed lines (within 400-line review budget per tasks.md forecast)
- Estimated review budget impact: Low — single PR, no chain needed
