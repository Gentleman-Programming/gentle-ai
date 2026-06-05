# Spec: fix-skill-duplication-and-migrate

> **Bundle Notice — PR #458 ships two independent work streams:**
>
> - **Part 1 — LLM-first skills refactor** (commits `99f7062` + `f070845`): slims 5 heavy SKILL.md files to ≤60 lines, ships `references/*.md` companions, adds `docs/skill-style-guide.md`, makes injectors recursive. This was a separate work stream already on local `main` before this SDD was planned; it surfaces in PR #458 because `origin/main` had not received those commits when the PR was opened.
> - **Part 2 — SDD picker frontmatter flags** (commit `7a3bff9`): hides 11 SDD SKILL.md files from the Claude Code `/` picker via YAML frontmatter flags. This is the change the SDD process planned and tracked — closes #457.
>
> The artifact originally described Part 2 only. It is updated here to honestly describe both parts because the PR ships both.

---

## Goal

**Part 2 (SDD-planned):** Add `user-invocable: false` and `disable-model-invocation: true` to the YAML frontmatter of the 11 SDD `SKILL.md` files so Claude Code v2.x stops showing duplicate entries in the `/` picker, while keeping the files readable by agents and commands via `Read`.

**Part 1 (bundled):** Slim the 5 heaviest non-SDD SKILL.md files to ≤60 lines each by extracting verbose content into `references/*.md` companion files. Update injectors to copy subdirectories recursively. Ship a style guide for future skill authoring.

---

## Part 2 — Frontmatter Flags (SDD-planned)

### Functional Requirements

- The 11 SDD `SKILL.md` files MUST contain `user-invocable: false` and `disable-model-invocation: true` as **top-level** YAML frontmatter keys (not nested under `metadata:`).
- The 11 files are: `internal/assets/skills/_shared/SKILL.md` plus `sdd-apply`, `sdd-archive`, `sdd-design`, `sdd-explore`, `sdd-init`, `sdd-onboard`, `sdd-propose`, `sdd-spec`, `sdd-tasks`, `sdd-verify`.
- All pre-existing top-level fields (`name`, `description`, `license`, `metadata`, `version`) of the **Part 2 files** MUST retain their values byte-for-byte after the change. (See Scenario C for Part 1 scope clarification.)
- Each of the 10 phase files MUST keep `name:` equal to its directory basename (e.g. `name: sdd-apply`).
- `_shared/SKILL.md` MUST keep `name: _shared`.
- The frontmatter linter at `internal/assets/skills_frontmatter_test.go` MUST accept `user-invocable` and `disable-model-invocation` as valid top-level keys — the existing allowlist `{name, description, license, metadata, version}` MUST be widened to include them.
- No other non-SDD `SKILL.md` files MUST carry the new flags (non-SDD skills are intentionally user-invocable).
- Golden test files under `testdata/golden/` that embed the affected SKILL.md content MUST be regenerated so that `go test ./...` passes without `-update`.

### Behavioral Requirements

- After `gentle-ai install` or `gentle-ai sync` on any `~/.claude/` directory, none of the 11 SDD SKILL.md entries MUST appear as user-invocable items in the Claude Code `/` picker.
- The `~/.claude/commands/sdd-*.md` entries MUST still appear in the `/` picker — they are the single canonical user-facing entry point for each phase.
- Each of `sdd-apply`, `sdd-archive`, `sdd-design`, `sdd-explore`, `sdd-init`, `sdd-onboard`, `sdd-propose`, `sdd-spec`, `sdd-tasks`, `sdd-verify` MUST appear AT MOST ONCE in the `/` picker after install.
- `~/.claude/agents/sdd-*.md` sub-agents MUST continue to function; orchestrator delegation MUST NOT be affected by the frontmatter flags.
- The `Read` tool MUST continue to load `~/.claude/skills/sdd-{phase}/SKILL.md` successfully — `Read` does not interpret `user-invocable` or `disable-model-invocation`.
- Claude Code MUST NOT auto-load the SDD SKILL.md files as contextual skills via description-based matching (suppressed by `disable-model-invocation: true`).

### Test Scenarios (Part 2)

#### Scenario A — Frontmatter linter accepts new keys

- GIVEN the 11 embedded SDD SKILL.md files carry `user-invocable: false` and `disable-model-invocation: true` at the top level of their YAML frontmatter
- WHEN `go test ./internal/assets/...` runs
- THEN `TestSkillFrontmatterIsLintClean` passes with zero "non-standard top-level frontmatter key" failures
- AND the two new keys are present in the widened `allowedKeys` map in `skills_frontmatter_test.go`

---

#### Scenario B — All 11 files carry both flags

- GIVEN the change has been applied to the embedded assets
- WHEN each of the 11 `SKILL.md` files is read and the YAML frontmatter block is parsed
- THEN every file contains `user-invocable: false` at the top level
- AND every file contains `disable-model-invocation: true` at the top level

---

#### Scenario C — Part 2 fields preserved byte-for-byte; Part 1 bodies separately covered

- GIVEN the 11 Part 2 SKILL.md files (the SDD phase files + `_shared`)
- WHEN parsed as YAML
- THEN `name`, `description`, `license`, `metadata`, and `version` retain values identical to their pre-change state
- AND no other lines in the file body (below the closing `---`) of those 11 files are altered
- NOTE: The 5 Part 1 SKILL.md files (`chained-pr`, `judgment-day`, `sdd-init`, `go-testing`, `sdd-verify`) have intentional body rewrites — those are covered under Part 1 Scenarios H–L, not this scenario.
- NOTE: `sdd-init` and `sdd-verify` appear in both parts: Part 2 adds frontmatter flags only; Part 1 rewrites the body. The net diff for those two files includes both changes.

---

#### Scenario D — Golden tests regenerated and passing

- GIVEN the embedded SKILL.md files now include the two new frontmatter flags (and Part 1 body changes for affected files)
- WHEN `go test -run TestGolden ./... -update` is executed and the resulting golden files are committed
- THEN `go test ./...` passes without `-update`
- AND the diff on each `sdd-init` golden shows the frontmatter additions plus the body slim (Part 1 and Part 2 changes combined for that file)

---

#### Scenario E — Install writes new frontmatter to disk

- GIVEN a fresh `~/.claude/skills/` directory with no prior gentle-ai install
- WHEN `gentle-ai install` runs with the Claude adapter selected
- THEN `~/.claude/skills/sdd-apply/SKILL.md` (and each of the other 10 skill files) contains `user-invocable: false` at the top level of its YAML frontmatter
- AND contains `disable-model-invocation: true` at the top level

---

#### Scenario F — Picker shows no duplicate SDD entries (manual verification)

- GIVEN a Claude Code v2.1.131+ session with the new SKILL.md files installed via `gentle-ai sync`
- WHEN the user opens the `/` skill picker
- THEN each of `sdd-apply`, `sdd-archive`, `sdd-design`, `sdd-explore`, `sdd-init`, `sdd-onboard`, `sdd-propose`, `sdd-spec`, `sdd-tasks`, `sdd-verify` appears AT MOST ONCE
- AND every visible entry originates from `~/.claude/commands/sdd-*.md`, not from `~/.claude/skills/sdd-*/SKILL.md`
- NOTE: behavioral verification performed manually post-merge; not an automated Go test

---

#### Scenario G — Sub-agent delegation still works (manual verification)

- GIVEN a Claude Code session with the new SKILL.md files installed
- WHEN the SDD orchestrator delegates to the `sdd-explore` sub-agent
- THEN the sub-agent executes `Read` on `~/.claude/skills/sdd-explore/SKILL.md` without error
- AND the sub-agent completes the exploration phase normally
- NOTE: behavioral verification performed manually post-merge; not an automated Go test

---

## Part 1 — LLM-first Refactor (bundled, pre-planned separately)

### Functional Requirements

- The 5 heaviest SKILL.md files MUST slim to ≤60 lines after refactoring: `chained-pr` (was ~371 lines → ≤60), `judgment-day` (was ~345 → ≤60), `sdd-init` (was ~358 → ≤60), `go-testing` (was ~353 → ≤60), `sdd-verify` (was ~342 → ≤60).
- Each refactored skill MUST ship a `references/` subdirectory with at least one `*.md` companion file containing the extracted verbose content.
- `docs/skill-style-guide.md` MUST exist in the repository and document the LLM-first authoring pattern.
- `internal/assets/skills/skill-creator/SKILL.md` MUST reference `docs/skill-style-guide.md` so future skill authors can find the guide.
- `internal/components/skills/inject.go` MUST copy skill directories recursively (including subdirectories), not just the top-level `SKILL.md`.
- `internal/components/sdd/inject.go` MUST copy SDD skill directories recursively.
- `internal/assets/assets_test.go` MUST assert that every embedded `references/*.md` file is readable and non-empty.

### Behavioral Requirements

- Token cost on activation MUST drop approximately 85% for the 5 refactored skills (from ~18–25k tokens to ~3–5k tokens).
- After `gentle-ai install`, each refactored skill directory on disk MUST contain both `SKILL.md` and its `references/` subdirectory.
- Existing skill consumers (orchestrator, sub-agents reading SKILL.md via `Read`) MUST continue to work because `SKILL.md` retains the full operational contract.
- Reference files MUST install alongside their parent skill on every supported adapter that installs skills.

### Test Scenarios (Part 1)

#### Scenario H — Refactored skills install with their `references/` subdirs

- GIVEN `internal/components/skills/inject.go` has been updated to copy recursively
- WHEN `gentle-ai install` runs for the Claude adapter
- THEN `~/.claude/skills/chained-pr/references/chaining-details.md` exists on disk
- AND `~/.claude/skills/go-testing/references/examples.md` exists on disk
- AND each of the other 3 refactored skills has its companion `references/*.md` present

---

#### Scenario I — `sdd-init` and `sdd-verify` ship with their `references/*.md` correctly

- GIVEN `internal/components/sdd/inject.go` has been updated to copy recursively
- WHEN `gentle-ai install` runs
- THEN `~/.claude/skills/sdd-init/references/init-details.md` exists on disk
- AND `~/.claude/skills/sdd-verify/references/report-format.md` exists on disk
- AND both files are non-empty

---

#### Scenario J — `skill-creator` references `docs/skill-style-guide.md` and the file ships embedded

- GIVEN `internal/assets/skills/skill-creator/SKILL.md` has been updated
- WHEN `skill-creator/SKILL.md` is read
- THEN it contains a reference to `docs/skill-style-guide.md`
- AND `docs/skill-style-guide.md` exists in the repository at that path

---

#### Scenario K — `assets_test.go` asserts every embedded `references/*.md` is readable and non-empty

- GIVEN the 5 refactored skills have `references/*.md` files embedded in the Go asset bundle
- WHEN `go test ./internal/assets/...` runs
- THEN the readability assertions in `assets_test.go` pass for all embedded reference files
- AND no reference file returns zero bytes

---

#### Scenario L — `inject_test.go` covers nested file copying

- GIVEN `internal/components/skills/inject_test.go` and `internal/components/sdd/inject_test.go` have been updated
- WHEN `go test ./internal/components/...` runs
- THEN tests covering the recursive copy path pass
- AND a synthetic skill with a `references/` subdir installs its nested file to the correct destination path

---

## Out of Scope

- Path relocation to `~/.claude/sdd-lib/` (Option C) — rejected in proposal.
- Migration logic — existing installs pick up new content automatically on next `gentle-ai sync`.
- Changes to `~/.claude/commands/sdd-*.md` or `~/.claude/agents/sdd-*.md` reference paths.
- Cross-adapter changes for Cursor, Kiro, OpenCode, Windsurf for the frontmatter flags — no duplication in those adapters.
- Frontmatter changes to non-SDD skills (`judgment-day`, `branch-pr`, `chained-pr`, `cognitive-doc-design`, etc.) — those remain user-invocable.
- Changes to `internal/components/uninstall/service.go`.
- Recursive directory copy was originally listed as out of scope; it is **now IN scope** as Part 1 shipped it as a required enabler for `references/` propagation.

---

## References

- Proposal (file): `openspec/changes/archive/2026-05-07-fix-skill-duplication-and-migrate/proposal.md`
- Design (file): `openspec/changes/archive/2026-05-07-fix-skill-duplication-and-migrate/design.md`
- Frontmatter linter: `internal/assets/skills_frontmatter_test.go`
- Style guide: `docs/skill-style-guide.md`
- Claude Code skill schema docs: https://code.claude.com/docs/en/skills
- Claude Code slash commands docs: https://code.claude.com/docs/en/slash-commands
