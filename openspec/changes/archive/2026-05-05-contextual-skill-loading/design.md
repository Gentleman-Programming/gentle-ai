# Design: contextual-skill-loading

**Worktree**: /Users/alanbuscaglia/work/gentle-ai-claude-skills
**Branch**: feat/claude-contextual-skill-loading
**Strict TDD**: enabled (`go test ./...`)

---

## Executive summary

Inject one short MANDATORY directive (Decision 1, B2 wording) into ALL six `persona-gentleman.md` / `persona-neutral.md` assets in place of the hardcoded "Skills (Auto-load based on context)" table. Claude variant references the native `Skill` tool by name; non-Claude variants reference `<available_skills>` discovery in tool-agnostic phrasing (Decision 2, option β'). Guard against future block-scalar regressions with a Go frontmatter linter test under `go test ./...` (Decision 3, option c). Each delta (A, B, C, D) ships as its own work-unit commit; cumulative diff stays within the 400-line review budget — no chained PRs needed.

---

## Key Decisions

**Decision 1 (B2 wording)**: Inject "## Contextual Skill Loading (MANDATORY)" directive into all 6 personas, mirroring the maintainer's personal `~/.claude/CLAUDE.md` style with MANDATORY phrasing ("blocking requirement", "discipline failure"). This wording is empirically known to drive proactive skill invocation.

**Decision 2 (option β')**: Uniform fix to all 6 personas (not Claude only). Claude variant names the `Skill` tool explicitly; non-Claude variants reference "read the matching SKILL.md (using your agent's read mechanism)".

**Decision 3 (option c)**: Go linter test in `internal/assets/skills_frontmatter_test.go` to prevent future contributors from re-introducing block-scalar descriptions.

**Decision 4**: 9 persona goldens affected (not 4 — proposal scope undercount).

**Decision 5**: Strict TDD with two RED-first commits per maintainer's convention (linter test, then persona directive test).

---

## Success Criteria

- Fresh install + skill-matching prompt → host invokes matching skill via native mechanism (Claude `Skill` tool, others read SKILL.md) without user editing config.
- `go test ./...` green: new linter, new persona test, all refreshed goldens.
- No SKILL.md uses `description: >` or `description: |`.
- No persona contains `## Skills (Auto-load based on context)` heading.
- `chained-pr/SKILL.md` name matches directory; skill-creator has no `allowed-tools:`.
- All 6 personas contain `## Contextual Skill Loading (MANDATORY)`, `<available_skills>`, and `Self-check BEFORE every response`.

---

## Work Units (6 commits, 280–330 lines cumulative)

1. `test(assets): add SKILL.md frontmatter linter` (RED)
2. `fix(skills): align chained-pr SKILL.md name with directory` (C-1)
3. `fix(skills): drop non-standard allowed-tools field from skill-creator` (C-2 + 4 goldens)
4. `refactor(skills): flatten SKILL.md description frontmatter` (D + ~12 goldens)
5. `test(assets): require contextual skill loading directive in personas` (RED)
6. `feat(persona): replace skills auto-load table with mandatory contextual directive` (A+B + 9 persona goldens)

All within 400-line review budget — no chained PRs needed.

