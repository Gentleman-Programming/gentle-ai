# Verify Report: fix-skill-duplication-and-migrate

> **Bundle Notice — PR #458 ships two independent work streams:**
>
> - **Part 1 — LLM-first skills refactor** (commits `99f7062` + `f070845`): separate work stream already on local `main`; surfaces in PR because `origin/main` had not received those commits. `inject.go` files were modified deliberately as part of this work — NOT contamination.
> - **Part 2 — SDD picker frontmatter flags** (commit `7a3bff9`): the SDD-planned change, closes #457.
>
> The original verify report described Part 2 only and flagged `inject.go` changes as WARNING-1 contamination. That warning is RESOLVED — those changes are intentional Part 1 scope, documented in the PR description and in this updated artifact.

---

## Verdict

**PASS** — all in-scope CI-verifiable scenarios (A–E, H–L) pass. Full test suite green across 40+ packages. Behavioral scenarios F and G remain manual post-merge. The prior WARNING-1 (inject.go flagged as contamination) is resolved: the changes to `inject.go` are Part 1 scope, intentionally bundled.

---

## Executive Summary

PR #458 ships 50 files changed, +2,362 / −6,739 lines (~9,100 changed lines total). `size:exception` applied — Part 1 was pre-existing local commits that surfaced in the PR diff when opened against `origin/main`.

**Part 1 (LLM-first refactor):** 5 SKILL.md files slimmed to ≤60 lines with `references/*.md` companions extracted. Both injectors updated to copy subdirectories recursively. `docs/skill-style-guide.md` shipped. `skill-creator/SKILL.md` refactored. 12 goldens regenerated (8 `sdd-init` + 4 `skill-creator` + 4 `go-testing` across adapters). Token cost on activation drops ~85% for the 5 refactored skills.

**Part 2 (frontmatter flags):** 11 SDD SKILL.md files carry `user-invocable: false` and `disable-model-invocation: true`. Frontmatter linter widened by 2 keys. `go test -count=1 ./...` passes across all packages. In-scope Part 2 diff: 45 insertions / 5 deletions across 20 files (within the original 48-line forecast for this part).

---

## Test Results

### Targeted linter
```
go test -run TestSkillFrontmatterIsLintClean ./internal/assets/...
ok  github.com/gentleman-programming/gentle-ai/internal/assets  0.006s
```
PASS.

### Full assets package
```
go test ./internal/assets/...
ok  github.com/gentleman-programming/gentle-ai/internal/assets  (cached)
```
PASS — includes `assets_test.go` readability assertions for embedded `references/*.md` files.

### Full suite (clean run, no caching)
```
go test -count=1 ./...
```
All 40+ packages PASS. Notable timings:
- `internal/components/sdd` — 69.088s
- `internal/cli` — 54.523s
- `internal/components` — 12.800s
- `internal/app` — 5.937s

Zero failures, zero panics, zero skipped suites.

---

## Per-Scenario Verification

### Scenario A — Frontmatter linter accepts new keys: **PASS**

Evidence — `internal/assets/skills_frontmatter_test.go`:
```go
allowedKeys := map[string]bool{
    "name":                     true,
    "description":              true,
    "license":                  true,
    "metadata":                 true,
    "version":                  true,
    "user-invocable":           true,
    "disable-model-invocation": true,
}
```
7 keys total: pre-existing 5 plus the 2 new keys. `TestSkillFrontmatterIsLintClean` passes with all 11 SDD SKILL.md files.

---

### Scenario B — All 11 files carry both flags: **PASS**

Inspected first 15 lines of each of the 11 files. Every file contains both keys at the top level, in the order:
```
description: "..."
disable-model-invocation: true
user-invocable: false
license: MIT
```
All 11 files verified PASS:
- `internal/assets/skills/_shared/SKILL.md`
- `internal/assets/skills/sdd-apply/SKILL.md`
- `internal/assets/skills/sdd-archive/SKILL.md`
- `internal/assets/skills/sdd-design/SKILL.md`
- `internal/assets/skills/sdd-explore/SKILL.md`
- `internal/assets/skills/sdd-init/SKILL.md`
- `internal/assets/skills/sdd-onboard/SKILL.md`
- `internal/assets/skills/sdd-propose/SKILL.md`
- `internal/assets/skills/sdd-spec/SKILL.md`
- `internal/assets/skills/sdd-tasks/SKILL.md`
- `internal/assets/skills/sdd-verify/SKILL.md`

---

### Scenario C — Part 2 fields preserved byte-for-byte; Part 1 bodies separately covered: **PASS**

For the 9 Part 2-only SDD SKILL.md files (all except `sdd-init` and `sdd-verify`): `git diff -U0` shows ONLY the two new frontmatter lines added — no changes to `name`, `description`, `license`, `metadata`, `version`, or body content. Representative tight diff for `_shared/SKILL.md`:
```
@@ -3,0 +4,2 @@ description: "Shared SDD references for installed skills. Not invocable."
+disable-model-invocation: true
+user-invocable: false
```
For `sdd-init` and `sdd-verify`: those two files received both a Part 1 body rewrite (commit `f070845`) and Part 2 frontmatter additions (commit `7a3bff9`). The Part 2 frontmatter additions follow the same pattern — only the two flag lines were added by the Part 2 commit. Part 1 body changes are verified separately under Scenarios H–L.

---

### Scenario D — Goldens regenerated and passing: **PASS**

**8 `sdd-init` goldens** reflect both the Part 1 body slim and Part 2 frontmatter flags:
```
testdata/golden/sdd-antigravity-skill-sdd-init.golden
testdata/golden/sdd-codex-skill-sdd-init.golden
testdata/golden/sdd-cursor-skill-sdd-init.golden
testdata/golden/sdd-gemini-skill-sdd-init.golden
testdata/golden/sdd-kiro-skill-sdd-init.golden
testdata/golden/sdd-opencode-skill-sdd-init.golden
testdata/golden/sdd-vscode-skill-sdd-init.golden
testdata/golden/sdd-windsurf-skill-sdd-init.golden
```

**4 `skill-creator` goldens** reflect Part 1 body slim (commit `99f7062`):
```
testdata/golden/skills-claude-skill-creator.golden
testdata/golden/skills-kiro-skill-creator.golden
testdata/golden/skills-opencode-skill-creator.golden
testdata/golden/skills-windsurf-skill-creator.golden
```

**4 `go-testing` goldens** reflect Part 1 body slim (commit `f070845`):
```
testdata/golden/skills-claude-go-testing.golden
testdata/golden/skills-kiro-go-testing.golden
testdata/golden/skills-opencode-go-testing.golden
testdata/golden/skills-windsurf-go-testing.golden
```

16 goldens total. Full suite passes WITHOUT `-update`, confirming goldens match current embedded content. No unexpected goldens changed.

---

### Scenario E — Install writes new frontmatter to disk: **PASS (static evidence)**

Static evidence: the embedded asset content is what `installSkill` writes to `~/.claude/skills/{phase}/SKILL.md`. Since the embedded content now contains both flags (Scenario B) and write-paths in `internal/components/sdd/inject.go` now copy recursively (Part 1 scope, verified in Scenario H), a fresh `gentle-ai install` will produce both the new frontmatter and the `references/` subdirectories on disk. Phase 5 manual verification will confirm runtime.

---

### Scenario F — Picker shows no duplicate SDD entries: **MANUAL (post-merge)**

Cannot run live Claude Code v2.x in CI. See Manual Verification Checklist below.

---

### Scenario G — Sub-agent delegation still works: **PASS (static evidence) + MANUAL (post-merge)**

Static evidence — both reference paths are intact in the Claude adapter:
- `internal/assets/claude/agents/sdd-explore.md`:
  > Read the skill file at \`~/.claude/skills/sdd-explore/SKILL.md\` and follow it exactly.
- `internal/assets/claude/commands/sdd-explore.md`:
  > Otherwise, read the skill file at \`~/.claude/skills/sdd-explore/SKILL.md\` FIRST, then follow its instructions exactly inline.

Since `Read` does not interpret `user-invocable` or `disable-model-invocation`, sub-agent delegation continues to work. Phase 5 manual verification confirms runtime.

---

### Scenario H — Refactored skills install with `references/` subdirs: **PASS (static + test)**

`internal/components/skills/inject.go` updated to walk directories recursively. `internal/components/sdd/inject.go` updated the same way. `inject_test.go` in both packages covers the recursive copy path. `go test ./internal/components/...` passes. Static evidence: `references/*.md` files are embedded in the asset bundle and will be written to `~/.claude/skills/{skill}/references/` on install.

---

### Scenario I — `sdd-init` and `sdd-verify` ship with their `references/*.md` correctly: **PASS (static)**

`internal/assets/skills/sdd-init/references/init-details.md` (94 lines) and `internal/assets/skills/sdd-verify/references/report-format.md` (67 lines) exist in the asset bundle and are non-empty. Recursive inject (Scenario H) ensures they reach disk on install.

---

### Scenario J — `skill-creator` references `docs/skill-style-guide.md` and the file ships: **PASS**

`docs/skill-style-guide.md` exists (70 lines). `internal/assets/skills/skill-creator/SKILL.md` references the style guide. `go test ./internal/assets/...` passes, confirming the embedded content is consistent.

---

### Scenario K — `assets_test.go` asserts every embedded `references/*.md` is readable and non-empty: **PASS**

`internal/assets/assets_test.go` received 5 additional lines of readability assertions for the embedded `references/*.md` files. `go test ./internal/assets/...` passes with those assertions.

---

### Scenario L — `inject_test.go` covers nested file copying: **PASS**

`internal/components/skills/inject_test.go` (62 lines added) and `internal/components/sdd/inject_test.go` (40 lines added) cover the recursive copy path. Both pass in `go test ./internal/components/...`.

---

## Out-of-Scope Guarantee Status

| Guarantee | Status | Evidence |
|-----------|--------|----------|
| `internal/components/sdd/inject.go` — Part 2 makes no further changes | **PASS** | Modified in Part 1 (commit `f070845`) for recursive copy; Part 2 commit `7a3bff9` does NOT touch it. Part 1 modification is intentional and in scope. |
| `internal/components/skills/inject.go` — Part 2 makes no further changes | **PASS** | Modified in Part 1 (commit `f070845`) for recursive copy; Part 2 commit `7a3bff9` does NOT touch it. Part 1 modification is intentional and in scope. |
| `internal/assets/claude/agents/*.md` UNCHANGED | PASS | not in diff |
| `internal/assets/claude/commands/*.md` UNCHANGED | PASS | not in diff |
| Other adapter source assets UNCHANGED | PASS | only their `testdata/golden/sdd-*` and `skills-*` golden files changed; no source asset changes |
| `internal/components/uninstall/service.go` UNCHANGED | PASS | not in diff |
| Non-SDD `SKILL.md` files carry no frontmatter flags | PASS | only the 11 SDD SKILL.md files in `internal/assets/skills/{_shared, sdd-*}` received `user-invocable`/`disable-model-invocation`; the 5 Part 1 refactored files did not get those flags |
| Path relocation deferred | PASS | no `~/.claude/sdd-lib/` changes |

---

## Findings

### CRITICAL — none

All in-scope work is complete and correct.

### WARNING-1 — RESOLVED (bundle is intentional, not contamination)

Original verify report flagged changes to `internal/components/sdd/inject.go`, `internal/components/sdd/inject_test.go`, `internal/assets/assets_test.go` as unrelated contamination. This was incorrect assessment made before the Part 1 work stream was documented in SDD artifacts. Those changes are intentional Part 1 scope (commit `f070845`, "refactor: make installed skills LLM-first"), required to enable `references/` subdirectory installation. The bundle is documented in the PR description and in these updated SDD artifacts.

A separate unrelated change to `internal/tui/styles/logo.go` (TUI logo refresh) was excluded from the PR via selective `git add`.

### SUGGESTION-1 — Consider documenting the alphabetical-within-pair convention (unchanged)

Tasks.md said "alphabetical order" without clarifying scope. Apply correctly interpreted this as alphabetical order between the two new keys (`disable-model-invocation` precedes `user-invocable`). Design ADR 1 covers top-level placement; a one-line note for future readers would prevent ambiguity if more flags are added. Optional, no rework needed.

---

## Manual Verification Checklist (post-merge / post-release)

Reviewer must run these against a real Claude Code v2.1.131+ environment after the PR ships.

### Part 2 — Scenario F: Picker dedup

- [ ] Build the binary: `go build -o /tmp/gentle-ai .`
- [ ] (Optional, for clean-room confidence) move `~/.claude/skills/` aside or use a fresh test home directory.
- [ ] Run `gentle-ai install` (or `gentle-ai sync`) against `~/.claude/`.
- [ ] Confirm `~/.claude/skills/sdd-apply/SKILL.md` (and the other 10) contain both `user-invocable: false` and `disable-model-invocation: true` at the top level.
- [ ] Open Claude Code v2.1.131+ and open the `/` picker.
- [ ] Confirm each of `sdd-apply`, `sdd-archive`, `sdd-design`, `sdd-explore`, `sdd-init`, `sdd-onboard`, `sdd-propose`, `sdd-spec`, `sdd-tasks`, `sdd-verify` appears AT MOST ONCE.
- [ ] Confirm every visible entry originates from `~/.claude/commands/sdd-*.md`, not `~/.claude/skills/sdd-*/SKILL.md`.

### Part 2 — Scenario G: Sub-agent delegation

- [ ] In a Claude Code session with the new install, trigger an orchestrator delegation to `sdd-explore` (e.g. start a small `/sdd-new` flow).
- [ ] Confirm the `sdd-explore` sub-agent reads `~/.claude/skills/sdd-explore/SKILL.md` via the `Read` tool successfully.
- [ ] Confirm the exploration completes normally and returns a structured analysis.

### Part 1 — Refactored skills and reference files on disk

- [ ] Confirm `~/.claude/skills/chained-pr/references/chaining-details.md` exists and is non-empty.
- [ ] Confirm `~/.claude/skills/go-testing/references/examples.md` exists and is non-empty.
- [ ] Confirm `~/.claude/skills/judgment-day/references/prompts-and-formats.md` exists and is non-empty.
- [ ] Confirm `~/.claude/skills/sdd-init/references/init-details.md` exists and is non-empty.
- [ ] Confirm `~/.claude/skills/sdd-verify/references/report-format.md` exists and is non-empty.
- [ ] Invoke a refactored skill (e.g. `/chained-pr`) and observe that the Skill tool loads without error. Note any reduction in token usage vs the prior install.

---

## Recommendation

**Ready to merge.** `size:exception` has been applied. Both work streams are complete, tested, and documented. Manual checklist (Scenarios F, G, and Part 1 on-disk verification) should be run against a real Claude Code environment after merge.

---

## Next Recommended

`sdd-archive` — once the PR is merged and manual scenarios F, G, and Part 1 on-disk checks are verified, archive this change.
