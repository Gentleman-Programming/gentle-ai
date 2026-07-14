# Proposal: Kimi Code v0.11+ Plugin Integration Completion

## Intent

The Kimi plugin infrastructure is 80% built — `PluginInstaller` interface, manifest generation, plugin directory structure, and skill file routing all exist. Six gaps remain that prevent the plugin from actually being installed during `gentle-ai sync`, making the v0.11+ integration non-functional at runtime.

## Scope

### In Scope
- Call `InstallPlugin` from `BootstrapTemplate` when v0.11+ is detected
- Expose `AppVersion` from `internal/cli/run.go` into `internal/versions/versions.go` so the adapter can read it
- Add `[[hooks]]` sessionStart block to `config.toml` for sdd-init auto-trigger
- Add `extra_skill_dirs` to `config.toml` if Kimi Code supports it
- Extend `installSkillRegistryAutomation` in `inject.go` to handle Kimi (currently only Codex and Claude)
- Create `skills/` subdirectory during plugin scaffolding so skill injection has a target

### Out of Scope
- Kimi Code runtime hook execution (Kimi handles that)
- Plugin marketplace or auto-update
- Changes to other agents
- Migration of existing flat skills

## Capabilities

### New Capabilities
- `kimi-plugin-completion`: Completes the v0.11+ plugin lifecycle — bootstrap calls install, version resolved, config.toml has hooks, skill-registry automation covers Kimi

### Modified Capabilities
- `kimi-plugin-installer`: `BootstrapTemplate` must call `InstallPlugin` for v0.11+; `resolveConfigTOMLContent` must include `[[hooks]]` and `extra_skill_dirs`
- `adapter`: `installSkillRegistryAutomation` must handle `model.AgentKimi`

## Approach

1. **Version constant**: Add `GentleAI` version constant to `internal/versions/versions.go` (e.g. `const GentleAI = "0.1.0"`). Adapter imports `versions.GentleAI`.
2. **Bootstrap calls install**: In `BootstrapTemplate`, after writing KIMI.md/config.toml/AGENTS.md, call `a.InstallPlugin(homeDir, versions.GentleAI)` when `a.isV11Plus(homeDir)`.
3. **Config.toml hooks**: Extend `resolveConfigTOMLContent()` to include `[[hooks]]` with `sessionStart` referencing `sdd-init`.
4. **Config.toml extra_skill_dirs**: Add `extra_skill_dirs` pointing to `~/.config/agents/skills` and `~/.agents/skills` for cross-tool skill discovery.
5. **Skill-registry automation**: Add `model.AgentKimi` case to `installSkillRegistryAutomation` that writes a sessionStart hook to `config.toml` running `gentle-ai skill-registry refresh`.
6. **Plugin scaffolding**: `InstallPlugin` creates `skills/` subdirectory so skill injection has a target directory ready.

## Affected Areas

| Area | Impact | Description |
|------|--------|-------------|
| `internal/versions/versions.go` | Modified | Add `GentleAI` version constant |
| `internal/agents/kimi/adapter.go` | Modified | `BootstrapTemplate` calls `InstallPlugin`; `resolveConfigTOMLContent` adds hooks + extra_skill_dirs |
| `internal/agents/kimi/plugin.go` | Modified | `InstallPlugin` creates `skills/` subdirectory |
| `internal/components/sdd/inject.go` | Modified | `installSkillRegistryAutomation` handles Kimi |
| `internal/agents/kimi/adapter_test.go` | Modified | Tests for BootstrapTemplate calling InstallPlugin, config.toml hooks |
| `internal/agents/kimi/plugin_test.go` | Modified | Test for skills/ subdirectory creation |

## Risks

| Risk | Likelihood | Mitigation |
|------|------------|------------|
| Kimi Code ignores unknown config.toml keys | Low | `extra_skill_dirs` is optional; omit if unsupported |
| Version constant drifts from goreleaser ldflags | Med | Single source of truth in `versions.go`; goreleaser can inject at build |
| `installSkillRegistryAutomation` hook format differs per agent | Low | Follow existing Codex/Claude pattern; Kimi uses TOML hooks not JSON |

## Rollback Plan

1. Remove `InstallPlugin` call from `BootstrapTemplate`
2. Revert `resolveConfigTOMLContent` to original (no hooks, no extra_skill_dirs)
3. Remove Kimi case from `installSkillRegistryAutomation`
4. `git revert` the feature branch
5. User runs `gentle-ai sync` to restore pre-plugin state

## Dependencies

- Existing `PluginInstaller` interface and `InstallPlugin` implementation
- Existing `installSkillRegistryAutomation` pattern from Codex/Claude
- `internal/versions/versions.go` for version constant

## Success Criteria

- [ ] `gentle-ai sync` on v0.11+ creates `~/.kimi-code/plugins/managed/gentle-ai/kimi.plugin.json` with correct version
- [ ] `config.toml` contains `[[hooks]]` section with `sessionStart` referencing `sdd-init`
- [ ] `config.toml` contains `extra_skill_dirs` with cross-tool skill paths
- [ ] `skills/` subdirectory exists under plugin root after install
- [ ] `installSkillRegistryAutomation` writes Kimi sessionStart hook to config.toml
- [ ] All existing tests pass; new tests cover Bootstrap→InstallPlugin flow and config.toml hooks
