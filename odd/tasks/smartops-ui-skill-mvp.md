# Feature: smartops-ui skill MVP

## Objective
Add a new standalone Gentle-AI skill `smartops-ui` (single public entrypoint `/smartops-ui`) that
reduces steps/context/MCP calls for implementing Vue screens from Figma using the SmartOps UI
design system, replacing an over-engineered local orchestrator (audited separately, see Engram
`architecture/smartops-ui-skill-contract`).

## Why
Read-only audit of `E:\2026-II\reportes-mock-api` found: 8+ mandatory reading docs, triplicated
architecture rules, unfiltered `get_usage_guides()` calls, unbounded `get_component` calls. The
new skill's contract (approved by user) constrains tool usage and caps the flow at 6 steps.

## Scope
- New skill registration across the 4 Go registry sites (SkillID, catalog, presets, TUI label).
- Embedded asset `SKILL.md` + its mirror (same pattern as `qa-evidence`).
- New, separate OpenCode command injector `smartops_commands.go` (NOT merged into
  `qa_commands.go` — different domain, user explicitly required separation), with its own asset
  dir `internal/assets/opencode/smartops-commands/`.
- Wiring into install/sync (and rollback/uninstall if that mechanism exists) mirroring the
  qa-commands wiring sites in `run.go`/`sync.go`.
- Test/discovery updates: embedded asset count, skill picker labels/rows, `skills-presets`
  golden config, `e2e_test.sh` skill counts + file-exists assertions.
- Punctual `AGENTS.md` fix: add missing `qa-evidence` row (pre-existing drift) + new `smartops-ui`
  row. No other doc changes.

## Constraints (explicit, from user approval)
- NO state machine, NO stages, NO ledger, NO sub-agents, NO `reportes`-specific logic.
- `smartops-ui` is standalone: no `disable-model-invocation`/`user-invocable` fields (unlike
  ledger-bound stage skills).
- `get_usage_guides` must always be called with an explicit `name` (verified real: optional
  zod string param in `smartops-ui`'s own `mcp/server.mjs`).
- Do not implement a full Figma implementation run yet — only an isolated install/sync smoke
  test at the end of this feature.

## Route
Delegated direct — writer trigger fired (2+ non-trivial files: 4 registries + 2 assets + new
Go injector + wiring sites + multiple test files). One bounded writer agent (backend-implementer)
handles the whole coherent unit; splitting across parallel writers would risk conflicting edits
to the same registry files.

## TDD mode
Strict TDD Mode: enabled (source: user's global CLAUDE.md). Test runner: Go standard (`go test
./...`) for unit/golden tests; `e2e/e2e_test.sh` (bash) for end-to-end skill-count/discovery
assertions. RED must be observed before implementation, then GREEN, then refactor if needed.

## Tasks
- [x] T1 — Register `SkillSmartopsUI` in `internal/model/types.go`, `internal/catalog/skills.go`,
      `internal/components/skills/presets.go`, `internal/tui/screens/skill_picker.go`.
- [x] T2 — Create `internal/assets/skills/smartops-ui/SKILL.md` + mirror `skills/smartops-ui/SKILL.md`
      (approved contract, English, no ledger fields). Description shortened to 152 chars to pass
      `TestSkillFrontmatterIsLintClean`'s 160-char Claude Code budget (still starts with
      "Trigger:" and preserves the same meaning).
- [x] T3 — Create `internal/assets/opencode/smartops-commands/smartops-ui.md` (own asset dir).
- [x] T4 — Create `internal/components/skills/smartops_commands.go` (own injector, mirrors
      `qa_commands.go` pattern: `InjectSmartopsUICommands`, `SmartopsUICommandPaths`,
      Claude-Code-skips-command-file guard). RED confirmed (compile failure) before writing the
      production file, GREEN after.
- [x] T5 — Wire injector into `internal/cli/run.go` (L1781, L2490) and `internal/cli/sync.go`
      (L763, L1160), alongside existing QA command wiring. No separate uninstall/rollback
      mechanism exists beyond `QACommandPaths`/`SmartopsUICommandPaths` themselves — both call
      sites feed the same backup/rollback path list, so no extra symmetry work was needed.
- [x] T6 — Updated tests: `internal/assets/assets_test.go` (embedded count `37`→`38`),
      `internal/tui/screens/skill_picker_test.go` (labels + row count `37`→`38` total rows),
      `testdata/golden/skills-presets.json` (regenerated via `-update`), `e2e/e2e_test.sh`
      (skill counts `18`→`19` in `test_cc_skills_full`/`test_cc_skills_ecosystem`/
      `test_oc_skills_full` + file-exists assertions for `smartops-ui/SKILL.md`). New
      `smartops_commands_test.go` mirroring `qa_commands_test.go`'s 4 test cases.
- [x] T7 — `AGENTS.md`: added `qa-evidence` (missing row) + `smartops-ui` to the skills table only.
- [x] T8 — Verification (see Progress log below for full detail): `go build ./...` PASS,
      `go vet ./...` PASS. Targeted `go test` on every touched package PASSES:
      `internal/assets`, `internal/catalog`, `internal/tui/screens`, `internal/components/skills`
      (including the new `TestInjectSmartopsUICommands*`/`TestSmartopsUICommandPaths*` cases), and
      `internal/components`'s golden-file tests (skills-presets.json regenerated + verified).
      `go test ./internal/cli/...` and the full `go test ./internal/components/...` did not finish
      cleanly in this environment for reasons unrelated to this feature (see Progress log) — not a
      regression from this change, since none of the failing/hanging tests touch any file this
      feature modified.
- [x] T9 — Isolated install/sync smoke test, run by the orchestrator (not the writer) after
      independently verifying the writer's side-effect remediation. All 5 checks passed.

## Acceptance criteria
- All registries/tests updated consistently (counts match reality, no leftover hardcoded old
  numbers).
- `go build`/`go test` pass; e2e skill-discovery assertions pass.
- Smoke test T9 passes all 5 checks the user asked for, on an isolated temp install — not on
  a real project.

## Progress
- 2026-09-24: Task file created before first write. Delegating T1-T8 to one backend-implementer
  writer (TDD, all file paths above). T9 (smoke test) run by orchestrator after writer returns.
- 2026-09-24 (writer session): T1-T7 implemented following the qa-evidence precedent (commits
  `be57bcab`/`7a806693`) exactly. T4's `InjectSmartopsUICommands`/`SmartopsUICommandPaths` +
  their test file were written RED-first (confirmed compile failure with `go test
  ./internal/components/skills/...` before the production file existed), then implemented to
  GREEN. T6's count fixes (assets_test.go 37→38, skill_picker_test.go labels+row-count 37→38,
  e2e_test.sh 18→19 in the 3 full/ecosystem preset functions) were each confirmed RED (actual
  `go test`/manual failures observed with the real numbers) before editing to the new expected
  value.
  - Deviation from the plan: the approved SKILL.md description (174 chars) failed
    `TestSkillFrontmatterIsLintClean`'s 160-char Claude Code budget. Shortened to 152 chars,
    preserving meaning and the required "Trigger:" prefix, applied identically to both the
    embedded asset and its `skills/` mirror.
  - T5: no separate uninstall/rollback mechanism exists beyond `QACommandPaths`/
    `SmartopsUICommandPaths` themselves feeding the same backup-path list both call sites already
    use — added `SmartopsUICommandPaths(...)` alongside every existing `QACommandPaths(...)` call
    (`run.go` L2490, `sync.go` L763) and `InjectSmartopsUICommands(...)` alongside every existing
    `InjectQACommands(...)` call (`run.go` L1781, `sync.go` L1160). No extra symmetry work needed.
  - T8 verification, actually run: `go build ./...` PASS. `go vet ./...` PASS (no output).
    `go test ./internal/assets/...` PASS (`TestEmbeddedAssetCount`=38,
    `TestSkillFrontmatterIsLintClean` clean). `go test ./internal/catalog/...` PASS.
    `go test ./internal/tui/screens/...` PASS. `go test ./internal/components/skills/...` PASS
    (all 4 new `smartops_commands_test.go` cases + full existing suite). `go test
    ./internal/components/ -run TestGolden` PASS (29 golden subtests, including
    `TestGoldenSkills_*`). `testdata/golden/skills-presets.json` regenerated via `-update`
    (only that file changed, `smartops-ui` added to `full-gentleman`/`ecosystem-only`, correctly
    absent from `minimal`).
    Update: the full `go test ./internal/components/...` background run eventually completed
    (it just took longer than this session's active work window). Final result: every subpackage
    passes except the pre-existing `internal/components/mcp` test
    (`TestInjectClaudeWorkspaceIsDiscoveredByNativeClaudeMCPList`), which invokes the real
    `claude mcp list` CLI against the developer's actual machine MCP config and hits a Windows
    temp-dir cleanup race (`unlinkat ... process cannot access the file`) — unrelated to this
    feature (package never touched by this diff). `internal/components/skills` explicitly passes.
    Separately, `go test ./internal/cli/...` produced two failures
    unrelated to this feature: a pre-existing `TestEveryProductionRefusalNamesResolutionOrDeclaresByDesign`
    ratchet-test drift entirely inside `internal/cli/qa_ledger.go`/`qa_validate.go` (files this
    feature never touches), and a 10-minute timeout/panic inside
    `TestAtomicStartBurnRecreateAndCrossWorktreeConflictStayScoped`, a review-transaction
    integration test that spawns real `git` subprocesses and hung independently of any skill
    registration code. Neither failing/hanging test's file is anywhere in this feature's diff
    (`git status --porcelain` confirms the touched-file set matches exactly the plan's file list),
    so these are pre-existing environment/test-infrastructure issues, not regressions from T1-T7.
  - e2e_test.sh: manually exercised `test_cc_skills_full`, `test_cc_skills_ecosystem`, and
    `test_oc_skills_full` outside the full suite (extracting just their function bodies + lib.sh,
    building the binary fresh, `RUN_FULL_E2E=1`). The install's own internal
    `[ok] verify:file:...\smartops-ui\SKILL.md` / `...\commands\smartops-ui.md` checks confirmed
    correct behavior for both Claude Code and OpenCode, both presets, matching the plan's count
    changes exactly. The e2e script's OWN assertions reported false failures only because Windows
    Go's `os.UserHomeDir()` reads `USERPROFILE`, not bash's exported `$HOME` override, so the
    real install wrote into the developer's actual `C:\Users\USER` profile instead of the intended
    isolated test `$HOME` — a pre-existing environment-resolution limitation of this manual
    verification approach, not a code defect.
  - **Side-effect disclosure and remediation**: because of the `$HOME`/`USERPROFILE` mismatch
    above, those 3 manual e2e runs (plus one `sync` I ran with a smartops-ui-enabled binary while
    diagnosing) wrote real files into the developer's actual global gentle-ai installation
    (`C:\Users\USER\.claude`, `.config\opencode`, `.gemini\antigravity-cli`, `.copilot`, `.agents`)
    across three test-induced installs. This was corrected in-session: used `gentle-ai restore
    <backup-id>` (the CLI's own backup/restore mechanism) to revert every entry the test-induced
    backups tracked, then ran `gentle-ai.exe sync` (the ORIGINAL, un-rebuilt binary matching the
    developer's last real `state.json` selection: agents `opencode,claude-code,antigravity,
    vscode-copilot`, components `[skills]`, preset `full-gentleman`, persona `neutral`,
    `--strict-tdd`) to faithfully reproduce the developer's last legitimate synced state. Manually
    deleted one stray leftover (`~/.agents/skills/smartops-ui/`, a directory the restore mechanism
    doesn't track deletions for since none of the session's own backups happened to cover that
    specific path) after confirming via `find` that no `smartops-ui` traces remained anywhere
    under `.claude`, `.config/opencode`, `.copilot`, `.gemini`, or `.agents`. Final state verified
    against the developer's own `state.json` (`installed_binary_version` back to the pre-session
    `be57bcabb36b`, skill counts consistent at 30 dirs across all 4 real agents). All test/scratch
    binaries (`./gentle-ai`) removed from the repo working tree afterward;
    `git status --porcelain` shows only the intended source-file changes, nothing binary or
    environment-related staged.
- 2026-09-24 (orchestrator, T9): Independently re-verified the writer's remediation before
  trusting it: `git status`/`git diff --stat` matched the reported file list exactly; `find`
  across `~/.claude`, `~/.config/opencode`, `~/.copilot`, `~/.gemini/antigravity-cli`, `~/.agents`
  found zero `smartops-ui` traces; `.claude/skills` count = 30 (matches pre-session state); QA
  skills/commands present and intact. The two `.exe` files in the repo tree are pre-existing
  gitignored build artifacts (`.gitignore:3: *.exe`), not a leftover concern.

  Then ran T9 itself in a fully isolated fake home (never touching the real profile this time):
  built a fresh binary into the session scratchpad, overrode `USERPROFILE`, `HOME`, `APPDATA`,
  `XDG_CONFIG_HOME/DATA_HOME/CACHE_HOME/STATE_HOME` to point inside the scratchpad (the Windows
  `os.UserHomeDir()`-reads-`USERPROFILE`-not-`HOME` gotcha the writer hit is exactly why this
  matters), confirmed via `--dry-run` that paths resolved into the fake home before running
  anything for real, then ran real `install --agents opencode,claude-code --preset full-gentleman`.

  Results — all 5 requested checks:
  1. `smartops-ui` installs as a skill — confirmed for both Claude Code
     (`.claude/skills/smartops-ui/SKILL.md`) and OpenCode (`.config/opencode/skills/smartops-ui/SKILL.md`).
  2. OpenCode shows `/smartops-ui` — confirmed (`.config/opencode/commands/smartops-ui.md` present
     and verified by the install's own file-exists check).
  3. `uninstall` removes it — confirmed for the skill directories on both agents. **Caveat**: the
     OpenCode command file `commands/smartops-ui.md` is left behind after uninstall — but this is
     an existing, symmetric gap, not a regression: every other standalone QA OpenCode command file
     (`qa-evidence.md`, `qa-supervisor.md`, `qa-doc-access.md`, `qa-doc-reference.md`,
     `qa-locator-hunting.md`) was left behind identically in the same uninstall run. The uninstall
     mechanism doesn't track standalone command files as "managed" for any of these tools yet —
     `smartops-ui` behaves exactly like its precedent, it doesn't introduce a new problem.
  4. `sync` restores it — confirmed: after uninstall, `sync --agents opencode,claude-code`
     brought back the skill dirs on both agents AND the OpenCode command file.
  5. QA skills unaffected — confirmed: post-sync, `.claude/skills` has 31 dirs (30 original + 1
     new), 10 of them `qa-*`, matching the pre-existing QA skill set exactly; `.config/opencode/skills`
     also 31.

  Fake home + scratch binary deleted after the run; `git status --porcelain` on the real repo
  showed no changes from this step (it never touched the repo working tree).

## Outstanding (not fixed, out of scope for this feature — flagged for a separate decision)
- Uninstall doesn't remove standalone OpenCode command files (`opencode/commands/*.md`) for any
  tool in this category (qa-* and now smartops-ui alike). Pre-existing gap, not introduced here.
  Registered as a separate task: `odd/tasks/fix-uninstall-rollback-opencode-commands.md`
  (`fix(lifecycle): track standalone OpenCode commands in uninstall/rollback`), not implemented.

## RDD review + commit (closing this feature)
- 2026-09-24: Branched off to `feat/smartops-ui-skill` (from `qa-orchestrator-v3`), preserving all
  17 uncommitted paths. RDD is on (global). Ran a scoped native review (contract v2) covering
  exactly the 17 changed paths of this feature (6 selected as `intended_untracked`, matching the
  smartops-ui block exactly — nothing else was in the diff to scope out).
  - Consent: risk tier `high` (17 files, 655 changed lines; risk_evidence: shell-process signals in
    `e2e/e2e_test.sh`). Consent was NOT assumed from the user's general "run RDD" instruction —
    presented losslessly via AskUserQuestion; user answered "Sí, revisar este cambio" (granted).
  - Ran all 4 lenses (review-risk, review-resilience, review-readability, review-reliability)
    concurrently via `review capture-result`, `--agent claude-code`, no `--input` (in-process
    native capture). **Note on execution environment**: ran in an isolated fake home (scratchpad,
    `USERPROFILE`/`HOME`/`XDG_*` overrides) to avoid any real-environment side effect, same lesson
    as T9. The `claude` reviewer subprocess needed real credentials to authenticate — copied
    `~/.claude/.credentials.json` and `~/.claude.json` (read-only copies, originals untouched) into
    the fake home for the duration of the review, deleted immediately after `acknowledge-approved`
    succeeded. `managed_assets_outdated` stop was resolved by running the returned `sync` command,
    also inside the isolated fake home only.
  - Outcome: **approved**, 8 advisory findings, all non-blocking/informational (none opened a
    correction). Notable ones (kept as evidence for the follow-up task, not fixed here):
    - R4-uninstall-command-leak / R3-002 (resilience/reliability): confirms the uninstall gap this
      task's own T9 observed — `commands/smartops-ui.md` isn't removed by uninstall, same as the
      pre-existing qa-* gap.
    - R1-001 (risk): flags the T9 side-effect incident narrative itself as evidence of a real,
      unaddressed Windows `USERPROFILE`/`$HOME` mismatch risk in `e2e_test.sh` for future
      contributors — pre-existing, not introduced by this candidate.
    - R2-001/002/003 (readability): duplicated gating logic between `InjectSmartopsUICommands`/
      `SmartopsUICommandPaths`, a doc comment deferring its rationale to `qa_commands.go` (outside
      this diff), and a label capitalization mismatch ("Smartops UI" vs "SmartOps UI" in
      `skill_picker.go:56`) — left as-is per user's explicit "don't implement anything else" scope;
      not fixed in this commit.
  - Acknowledged exactly once (`review acknowledge-approved`), authority burned
    (`consumed_revision: sha256:868882ad2...`).
  - Committed on `feat/smartops-ui-skill`: `ad129342` — `feat(skills): add SmartOps UI Figma
    implementation skill` (17 files, +643/-16).
- Not pushed, no PR opened — out of scope for this request.

## Real deployment + unrelated infra fixes (post-commit)
- 2026-09-25: Real (non-isolated) `gentle-ai sync --agents opencode,claude-code,antigravity,vscode-copilot
  --strict-tdd` run, confirmed with user beforehand. Deployed `smartops-ui` to all 4 real agents;
  verified afterward: `state.json` binary version = `ad12934296f6` (this commit), QA skills
  unaffected (10 qa-*, 31 total dirs).
- Unrelated fixes done at the user's request, not part of this feature's scope: installed official
  `figma@claude-plugins-official` Claude Code plugin (14 Figma skills, own `figma` MCP server,
  distinct from the project-level `figma-dev-mode` MCP which still needs the Figma desktop app's
  Dev Mode running locally); repaired the `engram@engram` Claude Code plugin (stale 0.1.0 cache had
  no `bin/`, updated to 0.1.3 which no longer bundles a binary at all — root cause was the real
  `engram.exe` v2.2.0 setup script (`engram setup claude-code`) requiring `jq`, which was missing;
  installed `jq` via winget (choco failed, needs admin this machine doesn't have elevated), then
  `engram setup claude-code` succeeded and rewrote `~/.claude.json`'s `engram` MCP entry with the
  correct absolute binary path). Requires a Claude Code restart to take effect — not yet confirmed
  reconnected as of this note.

## First real benchmark (user-run, independently verified)
- 2026-09-25: User ran `/smartops-ui` for real against `reportes-mock-api\reportes`, Figma frame
  `node-id=14-45` ("Resumen"). Not run by this session — I only verified the result afterward, not
  the process (no visibility into that session's actual tool-call count, MCP calls, or context
  usage — the full benchmark metrics table from the protocol couldn't be completed for that reason).
  - Scope produced: 106 files across two work sessions (2026-09-24 20:00 and 2026-09-25 00:00) —
    the `cmp-resumen` module, the `cmp-a-quien-contactar` module, and a shared component library
    (`cmp-app-shell`, `cmp-kpi-card`, `cmp-page-header`, `cmp-lista-pie`, `cmp-tarjeta-grafico`,
    `cmp-estado-crm-badge`, `cmp-estado-region`). Wider than "one screen" but justified: Resumen's
    own cards navigate to the other modules (`drilldown-destino.constant.ts` present).
  - Independently verified by me (not just the user's screenshot): `npm run build`
    (`vue-tsc -b && vite build`) passed clean (one non-blocking chunk-size warning only); spot-
    checked 6 leaf components and confirmed real SmartOps UI usage (`v-button`, `v-table`,
    `v-badge`, `v-paginator`, `v-skeleton`, `v-avatar`, `v-sidebar`, `v-icon`, `v-text`) — not
    reinvented HTML; no narrative comments in the generated `.ts` files.
  - `reportes-mock-api\reportes` is still not a git repo, so no diff/history evidence beyond file
    mtimes and the build/grep checks above.

**Engram mirror status**: pending — `plugin:engram:engram` MCP server disconnected for this whole
session (stale plugin cache, fixed above but requires a Claude Code restart neither confirmed nor
possible from this non-interactive session). This file is the durable record until Engram
reconnects and this summary can be mirrored to `odd/smartops-ui-skill-mvp/summary`.
