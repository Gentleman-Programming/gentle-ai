# Tasks: Kimi Code v0.11+ Plugin Integration Completion

## Review Workload Forecast

| Field | Value |
|-------|-------|
| Estimated changed lines | ~120–180 |
| 400-line budget risk | Low |
| Chained PRs recommended | No |
| Suggested split | Single PR |
| Delivery strategy | ask-on-risk |
| Chain strategy | pending |

Decision needed before apply: Yes
Chained PRs recommended: No
Chain strategy: single-pr
400-line budget risk: Low

### Suggested Work Units

| Unit | Goal | Likely PR | Notes |
|------|------|-----------|-------|
| 1 | All changes | Single PR | TDD cycle per unit; tests included |

## Phase 1: Foundation (Version Constant + Plugin Scaffolding)

- [x] 1.1 **RED**: Add failing test `TestInstallPlugin_CreatesSkillsSubdir` in `internal/agents/kimi/plugin_test.go` — call `InstallPlugin`, assert `skills/` dir exists and is empty
- [x] 1.2 **GREEN**: In `internal/agents/kimi/plugin.go`, create `skills/` subdirectory via `os.MkdirAll` after plugin dir creation in `InstallPlugin`
- [x] 1.3 **RED**: Add failing test `TestInstallPlugin_SkillsSubdirIdempotent` — call `InstallPlugin` twice, verify no error and existing contents preserved
- [x] 1.4 **GREEN**: Verify `os.MkdirAll` is already idempotent (no extra code needed)
- [x] 1.5 Add `const GentleAI = "0.1.0"` to `internal/versions/versions.go`
- [x] 1.6 **RED**: Add compile-time assertion `var _ string = versions.GentleAI` in `internal/agents/kimi/adapter_test.go`
- [x] 1.7 **GREEN**: No implementation needed — const resolves at compile time

## Phase 2: BootstrapTemplate → InstallPlugin Wiring

- [x] 2.1 **RED**: Add failing test `TestBootstrapTemplate_CallsInstallPlugin_V11` in `adapter_test.go` — set up v0.11+ adapter, call `BootstrapTemplate`, verify `kimi.plugin.json` exists at `PluginManifestPath`
- [x] 2.2 **GREEN**: In `adapter.go` `BootstrapTemplate`, after writing config.toml/KIMI.md/AGENTS.md, call `a.InstallPlugin(homeDir, versions.GentleAI)` guarded by `a.isV11Plus(homeDir)`, log warning on error without failing bootstrap
- [x] 2.3 **RED**: Add test `TestBootstrapTemplate_NoInstallPlugin_Legacy` — legacy adapter, verify no `kimi.plugin.json` created
- [x] 2.4 **GREEN**: Verify existing `isV11Plus` guard prevents call (no extra code)
- [x] 2.5 Import `github.com/gentleman-programming/gentle-ai/internal/versions` in `adapter.go`

## Phase 3: config.toml Enrichment (Hooks + extra_skill_dirs)

- [x] 3.1 **RED**: Add failing test `TestBootstrapTemplate_ConfigTOML_Hooks_V11` — read written config.toml, assert contains `[[hooks]]` and `sessionStart` referencing `sdd-init`
- [x] 3.2 **RED**: Add failing test `TestBootstrapTemplate_ConfigTOML_ExtraSkillDirs_V11` — assert contains `extra_skill_dirs` with `~/.config/agents/skills` and `~/.agents/skills`
- [x] 3.3 **GREEN**: In `adapter.go` `BootstrapTemplate`, after writing base config.toml for v0.11+, append hooks block and extra_skill_dirs to the content before writing (or post-append to the file)
- [x] 3.4 **RED**: Verify existing test `TestAdapter_BootstrapTemplate_ExistingConfigNotOverwritten` still passes — existing config.toml must NOT be modified
- [x] 3.5 **GREEN**: Ensure enrichment only happens on fresh install (inside the `os.IsNotExist` branch)

## Phase 4: Skill-Registry Automation (Kimi Case)

- [x] 4.1 **RED**: Add failing test `TestInstallSkillRegistryAutomation_Kimi` in `internal/components/sdd/inject_test.go` — call with Kimi adapter, verify TOML `[[hooks]]` block with `gentle-ai skill-registry refresh` written to config.toml
- [x] 4.2 **GREEN**: In `inject.go` `installSkillRegistryAutomation`, add `model.AgentKimi` case before the `AgentClaudeCode` guard — read/parse config.toml as TOML, append `[[hooks]]` sessionStart block, write back
- [x] 4.3 **RED**: Add test `TestInstallSkillRegistryAutomation_Kimi_Idempotent` — call twice, verify hook not duplicated
- [x] 4.4 **GREEN**: Add deduplication check — parse existing TOML, skip if hook with same command already present
- [x] 4.5 **RED**: Add test `TestInstallSkillRegistryAutomation_NonKimiUnaffected` — verify Codex/Claude adapters unchanged

## Phase 5: Verification

- [x] 5.1 Run `go test ./internal/agents/kimi/... -v` — all plugin and adapter tests pass
- [x] 5.2 Run `go test ./internal/components/sdd/... -v` — inject tests pass including new Kimi case
- [x] 5.3 Run `go vet ./...` — no issues
- [x] 5.4 Run `go test ./...` — full suite passes, no regressions
- [x] 5.5 Verify Windows path compatibility: `filepath.Join` used consistently, no hardcoded `/` in path construction

## Relevant Files

- `internal/versions/versions.go` — Add `GentleAI` version constant
- `internal/agents/kimi/plugin.go` — Add `skills/` subdirectory creation in `InstallPlugin`
- `internal/agents/kimi/adapter.go` — Wire `BootstrapTemplate` → `InstallPlugin`; append hooks + extra_skill_dirs
- `internal/agents/kimi/plugin_test.go` — Tests for `skills/` subdirectory and idempotency
- `internal/agents/kimi/adapter_test.go` — Tests for version const, BootstrapTemplate wiring, config.toml enrichment
- `internal/components/sdd/inject.go` — Add `model.AgentKimi` case to `installSkillRegistryAutomation`
- `internal/components/sdd/inject_test.go` — Tests for Kimi skill-registry automation
