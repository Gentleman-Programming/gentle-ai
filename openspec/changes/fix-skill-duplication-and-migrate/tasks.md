# Tasks: fix-skill-duplication-and-migrate

> **Bundle Notice — PR #458 ships two independent work streams:**
>
> - **Part 1 — LLM-first skills refactor** (commits `99f7062` + `f070845`): separate work stream already on local `main`; surfaces in PR because `origin/main` had not received those commits.
> - **Part 2 — SDD picker frontmatter flags** (commit `7a3bff9`): the SDD-planned change, closes #457.
>
> The original forecast below covered Part 2 only. Part 1 is documented here as historical/completed scope because the PR ships both.

---

## Review Workload Forecast

| Field | Value |
|-------|-------|
| Estimated changed lines (Part 2 only, original forecast) | 48–90 |
| Actual changed lines (full PR, both parts) | ~9,100 (+2,362 / −6,739) |
| Files changed | 50 |
| 400-line budget risk | **High — `size:exception` applied** |
| Chained PRs recommended | Post-hoc: would have been yes if forecast had included Part 1 |
| Suggested split | N/A — already shipped as single PR with `size:exception` |
| Delivery strategy | exception-ok (Part 1 was pre-existing local commits; not splittable after the fact) |

**Forecast discrepancy note:** The original 48-line estimate was accurate for Part 2 in isolation. Part 1 (commits `99f7062` + `f070845`) added ~2,314 insertions / −6,734 deletions across 44 additional files. The difference surfaced only when the PR was opened against `origin/main` and those prior commits were included in the diff.

---

## Part 1 — LLM-first Refactor (already shipped on local main, pre-PR)

These tasks are **completed/historical** — they describe work shipped via commits `99f7062` and `f070845` before this SDD was executed.

### P1 Task A — Slim the 5 heaviest SKILL.md files (completed)

- [x] Rewrite `internal/assets/skills/chained-pr/SKILL.md` from ~371 → 50 lines. Extract verbose content to `internal/assets/skills/chained-pr/references/chaining-details.md`.
- [x] Rewrite `internal/assets/skills/judgment-day/SKILL.md` from ~345 → 52 lines. Extract to `internal/assets/skills/judgment-day/references/prompts-and-formats.md`.
- [x] Rewrite `internal/assets/skills/sdd-init/SKILL.md` from ~358 → 57 lines. Extract to `internal/assets/skills/sdd-init/references/init-details.md`.
- [x] Rewrite `internal/assets/skills/go-testing/SKILL.md` from ~353 → 51 lines. Extract to `internal/assets/skills/go-testing/references/examples.md`.
- [x] Rewrite `internal/assets/skills/sdd-verify/SKILL.md` from ~342 → 59 lines. Extract to `internal/assets/skills/sdd-verify/references/report-format.md`.
- Done criteria: each slim file ≤60 lines; each `references/*.md` non-empty; operational contract (triggers, usage, key instructions) preserved in `SKILL.md`.

**Actual commit:** `f070845 refactor: make installed skills LLM-first`

---

### P1 Task B — Add style guide and refactor `skill-creator` (completed)

- [x] Create `docs/skill-style-guide.md` documenting the LLM-first authoring pattern (70 lines).
- [x] Refactor `internal/assets/skills/skill-creator/SKILL.md` from ~171 lines to reference the style guide; add `skills/chained-pr/` mirror.
- [x] Regenerate `testdata/golden/skills-{claude,kiro,opencode,windsurf}-skill-creator.golden` (4 goldens, each slimmed).
- Done criteria: style guide ships at correct path; `skill-creator/SKILL.md` references it; goldens green.

**Actual commit:** `99f7062 docs: add LLM-first skill style guide`

---

### P1 Task C — Make injectors recursive (completed, in `f070845`)

- [x] Update `internal/components/skills/inject.go` to walk and copy subdirectories recursively (was: `SKILL.md` only).
- [x] Update `internal/components/sdd/inject.go` to walk and copy subdirectories recursively.
- [x] Add `internal/components/skills/inject_test.go` coverage for nested file copy.
- [x] Add `internal/components/sdd/inject_test.go` coverage for nested file copy.
- [x] Add readability assertions in `internal/assets/assets_test.go` for all embedded `references/*.md` files.
- Done criteria: `go test ./internal/components/...` and `go test ./internal/assets/...` pass.

---

## Part 2 — Frontmatter Flags (SDD-planned, closes #457)

### Phase 1: Frontmatter Edits (Foundation)

- [x] 1.1 Add `user-invocable: false` and `disable-model-invocation: true` as top-level YAML frontmatter keys to all 11 SDD SKILL.md files. Insert after `description:`, before `license:` (alphabetical order). Files: `internal/assets/skills/_shared/SKILL.md`, `internal/assets/skills/sdd-apply/SKILL.md`, `internal/assets/skills/sdd-archive/SKILL.md`, `internal/assets/skills/sdd-design/SKILL.md`, `internal/assets/skills/sdd-explore/SKILL.md`, `internal/assets/skills/sdd-init/SKILL.md`, `internal/assets/skills/sdd-onboard/SKILL.md`, `internal/assets/skills/sdd-propose/SKILL.md`, `internal/assets/skills/sdd-spec/SKILL.md`, `internal/assets/skills/sdd-tasks/SKILL.md`, `internal/assets/skills/sdd-verify/SKILL.md`. Done criteria: each file's frontmatter block contains both new keys; `name`, `description`, `license`, `metadata`, `version` values byte-for-byte unchanged (Part 2 files only; `sdd-init` and `sdd-verify` also received Part 1 body rewrites).

**Actual commit:** `7a3bff9 fix(sdd): hide SDD SKILL.md files from Claude Code / picker`

---

### Phase 2: Linter Widening (Unblock CI)

- [x] 2.1 Edit `internal/assets/skills_frontmatter_test.go` — extend `allowedKeys` map to add `"user-invocable": true` and `"disable-model-invocation": true`. Also accommodate Part 1's `references/` subdirectory checks. Done criteria: `go test ./internal/assets/...` passes with zero `non-standard top-level frontmatter key` failures.

---

### Phase 3: Golden Regeneration (Mechanical)

- [x] 3.1 Regenerate 8 `sdd-init` goldens after frontmatter flags + Part 1 body slim. Files: `testdata/golden/sdd-{antigravity,codex,cursor,gemini,kiro,opencode,vscode,windsurf}-skill-sdd-init.golden`. Done criteria: each golden reflects the slim body + 2 frontmatter flag lines; `go test ./...` passes without `-update`.
- [x] 3.2 Regenerate 4 `skill-creator` goldens (Part 1): `testdata/golden/skills-{claude,kiro,opencode,windsurf}-skill-creator.golden`. Done criteria: goldens match slimmed `skill-creator/SKILL.md`.
- [x] 3.3 Regenerate 4 `go-testing` goldens (Part 1): `testdata/golden/skills-{claude,kiro,opencode,windsurf}-go-testing.golden`. Done criteria: goldens match slimmed `go-testing/SKILL.md`.

---

### Phase 4: Full Test Suite (Verification)

- [x] 4.1 Run `go test ./...` (no `-update` flag). All packages pass. See verify-report for full results.

---

### Phase 5: Manual Verification (Human-in-the-loop)

- [ ] 5.1 Build the binary: `go build -o /tmp/gentle-ai .` from repo root.
- [ ] 5.2 Run `gentle-ai install` (or `gentle-ai sync`) against your `~/.claude/` directory.
- [ ] 5.3 Confirm `~/.claude/skills/sdd-apply/SKILL.md` and each of the other 10 SDD files contain both `user-invocable: false` and `disable-model-invocation: true`.
- [ ] 5.4 Confirm `~/.claude/skills/chained-pr/references/chaining-details.md`, `go-testing/references/examples.md`, `judgment-day/references/prompts-and-formats.md`, `sdd-init/references/init-details.md`, and `sdd-verify/references/report-format.md` exist on disk (Part 1 check).
- [ ] 5.5 Open Claude Code v2.1.131+ and open the `/` picker. Each SDD phase appears AT MOST ONCE.
- [ ] 5.6 Trigger an orchestrator delegation to `sdd-explore`. Confirm the sub-agent reads `SKILL.md` successfully and completes normally.

*Steps 5.1–5.6 are a human reviewer checklist, not automated CI.*

---

## Recommended PR

**Title:** `fix: hide SDD SKILL.md files from / picker; ship LLM-first skill refactor`

**Body summary:**
Closes #457 (duplicate `/sdd-*` picker entries). Bundles two related work streams:

- **LLM-first refactor (Part 1):** Slims 5 heavy SKILL.md files (chained-pr, judgment-day, sdd-init, go-testing, sdd-verify) from 340–375 lines to ≤60 lines each. Extracts verbose content to `references/*.md` companions. Updates both injectors to copy subdirectories recursively so reference files install alongside their skill. Ships `docs/skill-style-guide.md`. Token cost on skill activation drops ~85% for these 5 skills.
- **Frontmatter flags (Part 2):** Adds `user-invocable: false` and `disable-model-invocation: true` to all 11 SDD SKILL.md files. Widens the frontmatter linter allowlist by 2 keys. Regenerates 16 affected goldens across adapters.

`size:exception` applied — Part 1 was a pre-existing local commit, not splittable from Part 2 after the PR diff is resolved.

**Checklist:**
- [ ] All 11 SDD SKILL.md files carry both new frontmatter flags
- [ ] 5 skill SKILL.md files slim to ≤60 lines with `references/*.md` companions
- [ ] `go test ./internal/assets/...` passes (linter + readability)
- [ ] `go test ./...` passes (full suite)
- [ ] 16 golden files regenerated and passing
- [ ] Manual picker verification done (Scenario F)
- [ ] Manual delegation smoke test done (Scenario G)
- [ ] Part 1: refactored skills install with `references/` copied (Scenario H)

---

## Spec Scenarios Covered

| Task | Spec Scenario |
|------|---------------|
| 1.1 (Part 2 frontmatter) | Scenario B (both flags present), Scenario C (Part 2 fields preserved), Scenario E (install writes flags) |
| 2.1 (linter widening) | Scenario A (linter accepts new keys) |
| 3.1–3.3 (golden regen) | Scenario D (goldens regenerated and passing) |
| 4.1 (full suite) | Scenario D (full suite passes) |
| P1 Task A (slim SKILL.md + references) | Scenario H (install with refs), Scenario I (sdd-init + sdd-verify refs), Scenario K (assets_test.go) |
| P1 Task B (style guide + skill-creator) | Scenario J (skill-creator references style guide) |
| P1 Task C (recursive inject) | Scenario H, Scenario L (inject_test.go nested copy) |
| 5.* (manual) | Scenario F (picker dedup), Scenario G (delegation still works) |
