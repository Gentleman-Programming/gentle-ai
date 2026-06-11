# Tasks: Hermes Agent Support

All tasks are **STRICT TDD** ordered: the failing test comes immediately before the
implementation that makes it pass. Tasks within a phase that have no data dependency
on each other are noted as parallelizable.

---

## Phase 1: Foundation — Types & Constants

- [x] T-01 `internal/model/types.go` — Add `AgentHermes AgentID = "hermes"` constant (modify)
- [x] T-02 `internal/model/types.go` — Add `StrategyMergeIntoYAML MCPStrategy = 4` constant; must not change existing iota values (modify)

*T-01 and T-02 touch the same file; apply sequentially. No test file needed for constants
— compilation + later adapter tests provide coverage.*

---

## Phase 2: YAML File-Merge Helpers (net-new; largest novel component)

- [x] T-03 `internal/components/filemerge/yaml_test.go` — Write table-driven golden tests for
  `UpsertYAMLMCPServerBlock` covering the full matrix (net-new test):
  - #1 empty/absent content → engram block created from scratch
  - #2 `mcp_servers:` absent, other top-level keys present → keys/comments preserved; section appended
  - #3 `mcp_servers:` present, no engram entry → user server preserved, engram appended as sibling
  - #4 idempotency — output of #3 fed back in → byte-identical result
  - #5 upsert replaces stale engram block (old args) → old block removed, fresh block appended, siblings intact
  - #6 user comments outside managed block → all comments preserved verbatim
  - #7 two managed servers coexist (engram then context7) → both present, both at 2-space indent, both idempotent
  - #8 CRLF input → normalized to `\n`, single trailing `\n`
  - #9 env map rendered → `env:` sub-block with 2-space-deeper KV pairs
  - #10 `UpsertHermesContext7Block` on empty → pinned `versions.Context7MCP` args emitted

- [x] T-04 `internal/components/filemerge/yaml_test.go` — Add table-driven unit tests for
  `ReadYAMLMCPServerCommand` (append to same test file, net-new):
  - scalar command: `command: engram` → `("engram", true)`
  - list command: `command:` + `- /path/engram` items → `("/path/engram", true)` (first element)
  - server absent under `mcp_servers:` → `("", false)`
  - `mcp_servers:` key absent entirely → `("", false)`
  - comment lines (`# ...`) inside/around block ignored without breaking recovery

- [x] T-05 `internal/components/filemerge/yaml.go` — Implement `UpsertYAMLMCPServerBlock`,
  `UpsertHermesEngramBlock`, `UpsertHermesContext7Block`, and `ReadYAMLMCPServerCommand`
  (net-new file; hand-rolled string helpers, NO `gopkg.in/yaml.v3`; mirrors `toml.go`
  semantics — normalize `\r\n`→`\n`, strip+re-append managed block, 2-space indent, single
  trailing `\n`, indent-aware block boundary detection for `mcp_servers:` nested table)

*T-03 and T-04 can be written in parallel (both test-only). T-05 must come after T-03 and T-04.*

---

## Phase 3: Adapter (net-new)

- [x] T-06 `internal/agents/hermes/adapter_test.go` — Write table-driven unit tests (net-new):
  - `TestDetect`: binary found → `(true, resolvedPath, configFound, configPath, nil)`;
    binary not found → `(false, "", …, nil)`; stat returns permission error → error propagated;
    config dir exists → `configFound=true`; config dir absent → `configFound=false`
  - `TestInstallCommand`: returns non-nil `AgentNotInstallableError` and nil/empty command slice
  - `TestSupportsAutoInstall`: returns `false`
  - `TestConfigPaths`: table of `homeDir="/home/test"` → expected return for each path method
    (`GlobalConfigDir`, `SystemPromptDir`, `SystemPromptFile`, `SkillsDir`, `SettingsPath`, `MCPConfigPath`)
  - `TestCapabilities`: all boolean flags and strategy methods verified against expected values
    (`Tier()=TierFull`, `SupportsOutputStyles()=false`, `OutputStyleDir()=""`, `SupportsSlashCommands()=false`,
    `SupportsSkills()=true`, `SupportsSystemPrompt()=true`, `SupportsMCP()=true`,
    `SystemPromptStrategy()=StrategyMarkdownSections`, `MCPStrategy()=StrategyMergeIntoYAML`)

- [x] T-07 `internal/agents/hermes/adapter.go` — Implement `Adapter` struct with injectable
  `lookPath`/`statPath`, `NewAdapter()` constructor, `AgentNotInstallableError`, package-level
  `LookPathOverride = exec.LookPath`, `defaultStat`, and `ConfigPath(homeDir)` helper.
  Implement all interface methods per the Adapter Method Table in design.md.
  Global-only — NO `resolveWorkspaceDir` or workspace-path validation.

*T-06 before T-07 (strict TDD).*

---

## Phase 4: Registration & Wiring

Tasks T-08 through T-13 are independent of each other and can be applied in parallel
(each touches a different file), but all depend on T-01/T-02 (types) and T-07 (adapter).

- [x] T-08 `internal/agents/factory.go` — Import `hermes` package; add `case model.AgentHermes`
  to `NewAdapter()`; add `model.AgentHermes` to `defaultAgentIDs` slice (modify)

- [x] T-09 `internal/catalog/agents.go` — Add entry `{ID: model.AgentHermes, Name: "Hermes",
  Tier: model.TierFull, ConfigPath: "~/.hermes"}` (modify)

- [x] T-10 `internal/system/config_scan.go` — Add `{Agent: "hermes", Path: filepath.Join(homeDir, ".hermes")}`
  to `knownAgentConfigDirs()` (modify)

- [x] T-11 `internal/cli/validate.go` — Add `case string(model.AgentHermes)` that appends
  `model.AgentHermes` to the agents list (modify)

- [x] T-12 `internal/tui/model.go` — Add `case string(model.AgentHermes)` in `loadSelection()`
  switch that appends `model.AgentHermes` to selected agents (modify)

- [x] T-13 `internal/components/engram/setup.go` — Add `case model.AgentHermes: return "", false`
  to `SetupAgentSlug()` with comment explaining MCP is injected via YAML directly (modify)

---

## Phase 5: Assets (net-new files)

Tasks T-14 through T-16 are independent of each other (separate files).

- [x] T-14 `internal/assets/hermes/sdd-orchestrator.md` — Create SDD orchestrator asset for
  Hermes: copy of the existing generic/claude orchestrator prompt with ALL skill path references
  rewritten to `~/.hermes/skills/`; remove `<available_skills>` system-prompt block assumption;
  include strict-TDD markers awareness; reference `~/.hermes/skills/` for skill registry
  (net-new asset file)

- [x] T-15 `internal/assets/hermes/persona-gentleman.md` — Create Hermes gentleman persona asset:
  copy of `generic/persona-gentleman.md` with the `## Contextual Skill Loading (MANDATORY)` block
  rewritten to reference `~/.hermes/skills/` by category; remove `<available_skills>` injection
  mechanism; add a short subsection documenting the complementary relationship between engram
  (cross-agent, cross-session memory protocol) and Hermes's native memory and skill-learning loop
  (net-new asset file)

- [x] T-16 `internal/assets/hermes/persona-neutral.md` — Create Hermes neutral persona asset:
  copy of `generic/persona-neutral.md` with the `## Contextual Skill Loading (MANDATORY)` block
  rewritten for `~/.hermes/skills/` by category; remove `<available_skills>` injection mechanism;
  add the same engram-vs-native-memory complementary note as in T-15 (net-new asset file)

- [x] T-17 `internal/assets/assets.go` — Add `//go:embed all:hermes` directive to include all
  files under `internal/assets/hermes/` in the binary (modify)

*T-14, T-15, T-16 can be written in parallel. T-17 must come after T-14–T-16 exist on disk.*

---

## Phase 6: Component Injectors — Tests First

### 6a. Permissions (simplest; good warm-up)

- [ ] T-18 `internal/components/permissions/inject_test.go` — Add test case: `Inject(homeDir, hermesAdapter)`
  returns `nil` and writes no file (add case to existing test file)

- [ ] T-19 `internal/components/permissions/inject.go` — Add `case model.AgentHermes: return nil`
  to the permissions dispatch; Hermes format is undocumented — skip (modify)

### 6b. Engram Setup Slug

- [ ] T-20 `internal/components/engram/setup_test.go` — Add table case:
  `{model.AgentHermes, "", false}` to `TestSetupAgentSlug` (add case to existing test)

*(T-13 and T-20 both touch the engram/setup path; T-13 is the implementation change, T-20
adds the test case. Apply T-20 before T-13 to follow strict TDD — the test will fail until
T-13's `case model.AgentHermes` is applied.)*

### 6c. SDD Injection

- [ ] T-21 `internal/components/sdd/inject_test.go` — Add tests (add to existing test file):
  - `TestSDDOrchestratorAssetSelection` extended with case `{agent: model.AgentHermes, want: "hermes/sdd-orchestrator.md"}`
  - `TestInjectHermesWritesSDDOrchestratorToSOULMD`: fresh `t.TempDir()` home dir; call
    `sdd.Inject(homeDir, hermesAdapter, "")` with `StrategyMergeIntoYAML` for MCP strategy
    awareness; assert `~/.hermes/SOUL.md` created with `<!-- gentle-ai:sdd-orchestrator -->` markers
    and content; assert user content outside markers preserved on re-run
  - `TestInjectHermesStrategyMergeIntoYAML`: assert StrategyMergeIntoYAML dispatches correctly
    (no panic, returns InjectionResult with Changed=true on first run)

- [ ] T-22 `internal/components/sdd/inject.go` — Add `case model.AgentHermes: return "hermes/sdd-orchestrator.md"`
  to `sddOrchestratorAsset()`; add `case model.StrategyMergeIntoYAML:` handling in sdd inject
  dispatch if needed (verify existing `StrategyMarkdownSections` path already covers SOUL.md
  injection via the adapter's `SystemPromptStrategy()`; add only what's missing) (modify)

### 6d. MCP Injection

- [ ] T-23 `internal/components/mcp/inject_test.go` — Add tests (add to existing test file):
  - `TestInjectHermesContext7IntoYAML`: `t.TempDir()` fresh dir; call `mcp.Inject(homeDir, hermesAdapter)`;
    assert `~/.hermes/config.yaml` contains `context7:` entry under `mcp_servers:`
  - `TestInjectHermesContext7Idempotent`: call twice; assert exactly one `context7:` entry
  - `TestStrategyMergeIntoYAMLDispatches`: verifies no error and `Changed=true` on first run

- [ ] T-24 `internal/components/mcp/inject.go` — Add `case model.StrategyMergeIntoYAML:` to
  the strategy switch; implement `injectYAMLFile(homeDir, adapter)` function (mirrors `injectTOMLFile`):
  read config via `osReadFile`; call `filemerge.UpsertHermesContext7Block(existing)`;
  write via `filemerge.WriteFileAtomic`; return `InjectionResult` (modify)

### 6e. Engram Injection

- [ ] T-25 `internal/components/engram/inject_test.go` — Add tests (add to existing test file):
  - `TestInjectEngramHermesYAMLOverlay`: `t.TempDir()`; call `engram.Inject(homeDir, hermesAdapter, opts)`;
    assert `~/.hermes/config.yaml` contains `engram:` under `mcp_servers:`, idempotent on re-run
  - `TestEngramYAMLCommandRecoveryCustomPath`: `t.TempDir()` with a `config.yaml` whose
    `mcp_servers.engram.command` is `/custom/path/engram`; call `Inject`; assert command preserved
    (not clobbered with bare `engram`)
  - `TestEngramYAMLCommandRecoveryVersionedCellar`: `t.TempDir()` with a cellar-versioned
    command; call `Inject`; assert command is stabilized to `engram`/stable path
  - `TestEngramYAMLCommandRecoveryAbsent`: `t.TempDir()` with no prior engram entry;
    call `Inject`; assert stable fallback `engram` command used
  - `TestEngramYAMLCommandRecoveryListShape`: `config.yaml` with `command:` as YAML list;
    assert first element recovered

- [ ] T-26 `internal/components/engram/inject.go` — Add `case model.StrategyMergeIntoYAML:`
  to the MCP-strategy switch inside `injectWithOptions` (step 1): read config via `readFileOrEmpty`;
  call `stableEngramCommandForMergedConfig`; call `filemerge.UpsertHermesEngramBlock(existing, engramCmd)`;
  write via `filemerge.WriteFileAtomic`; set `changed` and append to `files` (modify)

- [ ] T-27 `internal/components/engram/inject.go` — Add `model.AgentHermes` to `isStandardAgent()`
  switch so Hermes gets the stable `engram` command when no prior command is recovered (modify;
  can be a single-line addition in same commit as T-26)

- [ ] T-28 `internal/components/engram/inject.go` — Add YAML recovery early branch in
  `existingMergedEngramCommand`: after `len(raw)==0` guard, before `MergeJSONObjects` call:
  `if agentID == model.AgentHermes { return filemerge.ReadYAMLMCPServerCommand(string(raw), "engram") }`
  (modify; same commit as T-26/T-27 acceptable since all three are in `inject.go`)

*T-25 must precede T-26/T-27/T-28. T-26, T-27, T-28 are all changes to `inject.go` and
should be applied together in one commit.*

### 6f. Persona Injection

- [ ] T-29 `internal/components/persona/inject_test.go` — Add table-driven test cases
  (add to existing test file):
  - Hermes + `gentleman` → returns content from `hermes/persona-gentleman.md`; does NOT contain `<available_skills>`
  - Hermes + `gentleman-neutral-artifacts` → same as gentleman (same asset)
  - Hermes + `neutral` → returns content from `hermes/persona-neutral.md`; does NOT contain `<available_skills>`
  - Hermes + `custom` → returns empty string, no persona injected
  - Non-Hermes agent + `neutral` → returns byte-identical `generic/persona-neutral.md` (no regression)
  - Hermes persona injected into SOUL.md → `<!-- gentle-ai:persona -->` markers present; engram
    and SDD sections coexist without duplication

- [ ] T-30 `internal/components/persona/inject.go` — Refactor `personaContent()`:
  - Change `PersonaNeutral` case from single `assets.MustRead("generic/persona-neutral.md")` to
    a per-agent inner switch: `case model.AgentHermes: return assets.MustRead("hermes/persona-neutral.md")`;
    `default: return assets.MustRead("generic/persona-neutral.md")`
  - Add `case model.AgentHermes: return assets.MustRead("hermes/persona-gentleman.md")` inside
    the gentleman (default) branch's inner agent switch
  - No other changes; all non-Hermes agents are byte-identical to before (modify)

---

## Phase 7: Registry & Cross-Cutting Tests

Tasks T-31 through T-36 verify the additive registration wiring. They depend on T-07–T-13
being applied but are otherwise independent of each other.

- [ ] T-31 `internal/agents/registry_test.go` — Extend `TestDefaultRegistryIncludesAllAgents`
  (or equivalent) to include `model.AgentHermes`; update expected agent count to N+1 (modify)

- [ ] T-32 `internal/system/config_scan_test.go` — Update total agent count to include `"hermes"`;
  add `"hermes"` to known agents list if present (modify)

- [ ] T-33 `internal/tui/model_test.go` — Add `"hermes"` to `makeDetectionWithAgents()` known
  agents slice; add `loadSelection` restoration case for hermes (modify)

- [ ] T-34 `internal/cli/validate.go` test — Add `{"hermes", model.AgentHermes}` case to the
  CLI agent validation mapping test (modify existing test file)

- [ ] T-35 `internal/assets/assets_test.go` (or `internal/assets/skills_frontmatter_test.go`) —
  Add assertion: `assets.ReadFile("hermes/sdd-orchestrator.md")` returns non-empty content
  without error; assert `hermes/persona-gentleman.md` and `hermes/persona-neutral.md` readable
  (add cases to existing test)

- [ ] T-36 `internal/skillregistry/registry.go` — Verify/add `~/.hermes/skills/` to the list of
  user+project skill directories that the skill registry scans; add `AgentHermes` to any
  agent-dir mapping if the registry is agent-keyed (modify if needed; no-op if already generic)

---

## Phase 8: Build & Final Verification

- [ ] T-37 `go build ./...` — Must compile with zero errors
- [ ] T-38 `go vet ./...` — Must pass with zero issues
- [ ] T-39 `go test ./internal/components/filemerge/...` — All YAML helper tests pass (T-03–T-05)
- [ ] T-40 `go test ./internal/agents/hermes/...` — All adapter tests pass (T-06–T-07)
- [ ] T-41 `go test ./internal/agents/...` — Registry test passes (T-31); factory wiring correct
- [ ] T-42 `go test ./internal/components/permissions/...` — Permissions nil-return test passes
- [ ] T-43 `go test ./internal/components/engram/...` — All engram inject + setup + recovery tests pass
- [ ] T-44 `go test ./internal/components/mcp/...` — All MCP YAML inject tests pass
- [ ] T-45 `go test ./internal/components/sdd/...` — SDD inject tests pass
- [ ] T-46 `go test ./internal/components/persona/...` — All persona content tests pass
- [ ] T-47 `go test ./internal/system/...` — Config scan tests pass
- [ ] T-48 `go test ./internal/tui/...` — TUI state restoration test passes
- [ ] T-49 `go test ./internal/assets/...` — Asset embed test passes
- [ ] T-50 `go test ./...` — Full suite green (no regressions across all packages)

---

## Dependency Order Summary

```
T-01, T-02 (types)
    │
    ├─→ T-03, T-04 (yaml tests) → T-05 (yaml impl)
    │
    ├─→ T-06 (adapter test) → T-07 (adapter impl)
    │       │
    │       └─→ T-08 (factory) T-09 (catalog) T-10 (config_scan)
    │           T-11 (cli)     T-12 (tui)      T-13 (engram setup)
    │
    ├─→ T-14, T-15, T-16 (assets) → T-17 (assets.go embed)
    │
    └─→ (T-05 + T-07 + T-17 all required before phase 6)
            │
            ├─→ T-18 → T-19 (permissions)
            ├─→ T-20 (setup slug test) ← verified by T-13
            ├─→ T-21 → T-22 (sdd inject)
            ├─→ T-23 → T-24 (mcp inject)
            ├─→ T-25 → T-26/T-27/T-28 (engram inject)
            └─→ T-29 → T-30 (persona inject)
                    │
                    └─→ T-31 … T-36 (cross-cutting tests)
                                │
                                └─→ T-37 … T-50 (build & full suite)
```

---

## Review Workload Forecast

| Metric | Estimate |
|--------|----------|
| **Total new/modified files** | ~30 files |
| **Estimated changed lines** | ~1,200–1,500 lines |
| **New source lines (non-test)** | ~400–500 |
| **New test lines** | ~500–600 |
| **New asset/content lines** | ~200–300 |
| **400-line budget risk** | **High** |
| **Chained PRs recommended** | **Yes** |
| **Decision needed before apply** | **Yes — chain strategy** |

### Proposed PR Slice Breakdown

```
PR 1 — Foundation + YAML Helpers
  Scope: T-01, T-02, T-03, T-04, T-05
  Files:
    internal/model/types.go (add AgentHermes + StrategyMergeIntoYAML)
    internal/components/filemerge/yaml.go (net-new)
    internal/components/filemerge/yaml_test.go (net-new)
  Estimated lines: ~350–450 (helper impl ~200, test matrix ~150–200, 2 type lines)
  Independent: Yes — purely additive; no adapter dependency
  Chained PRs risk addressed: introduces StrategyMergeIntoYAML that all later PRs depend on

PR 2 — Adapter + Registration + Assets
  Scope: T-06, T-07, T-08, T-09, T-10, T-11, T-12, T-13, T-14, T-15, T-16, T-17
  Files:
    internal/agents/hermes/adapter.go (net-new)
    internal/agents/hermes/adapter_test.go (net-new)
    internal/agents/factory.go (add hermes case)
    internal/catalog/agents.go (add hermes entry)
    internal/system/config_scan.go (add hermes)
    internal/cli/validate.go (add case)
    internal/tui/model.go (add case)
    internal/components/engram/setup.go (add case)
    internal/assets/hermes/sdd-orchestrator.md (net-new)
    internal/assets/hermes/persona-gentleman.md (net-new)
    internal/assets/hermes/persona-neutral.md (net-new)
    internal/assets/assets.go (add embed)
  Estimated lines: ~450–550 (adapter ~200, tests ~150, assets ~150–200, wiring ~30–50)
  Dependency: PR 1 (needs AgentHermes + StrategyMergeIntoYAML defined)
  Chained PRs risk: This PR may be at budget limit; asset files can be split out if needed

PR 3 — Component Injectors + Tests
  Scope: T-18, T-19, T-20, T-21, T-22, T-23, T-24, T-25, T-26, T-27, T-28, T-29, T-30
  Files:
    internal/components/permissions/inject.go (add nil case)
    internal/components/permissions/inject_test.go (add case)
    internal/components/engram/setup_test.go (add case)
    internal/components/sdd/inject.go (add hermes case + StrategyMergeIntoYAML)
    internal/components/sdd/inject_test.go (add hermes tests)
    internal/components/mcp/inject.go (add StrategyMergeIntoYAML case + injectYAMLFile)
    internal/components/mcp/inject_test.go (add hermes tests)
    internal/components/engram/inject.go (add StrategyMergeIntoYAML case + isStandardAgent + recovery branch)
    internal/components/engram/inject_test.go (add hermes + recovery tests)
    internal/components/persona/inject.go (refactor personaContent neutral + add hermes gentleman)
    internal/components/persona/inject_test.go (add hermes persona tests)
  Estimated lines: ~400–500
  Dependency: PR 1 (yaml helpers) + PR 2 (adapter + assets)
  Note: Most test-heavy PR; all injector tests exercise real file writes via t.TempDir()

PR 4 — Cross-Cutting Tests + Build Verification
  Scope: T-31, T-32, T-33, T-34, T-35, T-36, T-37 … T-50
  Files:
    internal/agents/registry_test.go (update count)
    internal/system/config_scan_test.go (update count)
    internal/tui/model_test.go (add hermes to known agents)
    internal/cli/validate.go test file (add hermes mapping case)
    internal/assets/assets_test.go (add embed assertions)
    internal/skillregistry/registry.go (verify/add hermes skills dir if needed)
  Estimated lines: ~100–150
  Dependency: PR 1 + PR 2 + PR 3
  Can be merged quickly once PR 3 passes CI; mostly test count adjustments
```

**PR dependency chain:** PR 1 → PR 2 → PR 3 → PR 4

**Chain strategy to choose before apply:** `stacked-to-main` (each PR merges to main in order,
each independently reviewable) OR `feature-branch-chain` (tracker branch accumulates; child PRs
target tracker; only tracker merges to main). This decision is required before launching `sdd-apply`.
