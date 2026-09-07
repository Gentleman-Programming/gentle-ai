# Proposal: Kimi Code Plugin Architecture

## Intent

The current Kimi adapter writes skills to `~/.kimi-code/skills/` as flat files, mixing Gentle AI artifacts with user-created skills. Kimi Code v0.11+ supports a proper plugin architecture at `~/.kimi-code/plugins/managed/{name}/` with a `kimi.plugin.json` manifest — enabling isolation, session hooks, version tracking, and MCP bundling. This change aligns Gentle AI with Kimi's official distribution model.

## Scope

### In Scope
- `PluginInstaller` optional interface for adapters that support plugin-based distribution
- `kimi.plugin.json` manifest generation (name, version, description, skills path, session hooks, mcpServers)
- Plugin directory structure: `~/.kimi-code/plugins/managed/gentle-ai/`
- Skill files written to plugin dir instead of flat `~/.kimi-code/skills/`
- Engram MCP server declared in plugin manifest
- v0.11+ detection to choose plugin vs flat install path
- Tests for manifest generation, directory creation, version detection

### Out of Scope
- Plugin marketplace or auto-update mechanism
- Migration of existing flat skills to plugin dir (user cleans up manually)
- Changes to other agents
- Session hook execution logic (Kimi Code handles that)

## Capabilities

### New Capabilities
- `kimi-plugin-installer`: Optional `PluginInstaller` interface and Kimi implementation for manifest-based plugin distribution

### Modified Capabilities
- `kimi-adapter`: `SkillsDir` returns plugin subdirectory for v0.11+; new `PluginDir`, `PluginManifestPath` methods; `BootstrapTemplate` generates manifest

## Approach

1. Define `PluginInstaller` interface: `PluginDir(homeDir) string`, `PluginManifestPath(homeDir) string`, `InstallPlugin(homeDir, version) error`
2. Implement on Kimi adapter with v0.11+ guard — legacy falls back to flat skills
3. Generate `kimi.plugin.json` with name, version, description, `skills.path`, `hooks.sessionStart.skill`, `mcpServers.engram`
4. Write skills to `~/.kimi-code/plugins/managed/gentle-ai/skills/` instead of `~/.kimi-code/skills/`
5. Component checks `PluginInstaller` interface before `SkillsDir` — opt-in per adapter

## Affected Areas

| Area | Impact | Description |
|------|--------|-------------|
| `internal/agents/interface.go` | Modified | Add `PluginInstaller` optional interface |
| `internal/agents/kimi/adapter.go` | Modified | Implement `PluginInstaller`; update `SkillsDir` for plugin path; add manifest generation |
| `internal/agents/kimi/adapter_test.go` | Modified | Tests for plugin dir, manifest content, v0.11+ vs legacy paths |
| `internal/agents/kimi/manifest.go` | New | `kimi.plugin.json` struct and generation logic |
| `internal/components/sdd/inject.go` | Modified | Check `PluginInstaller` before `SkillsDir` for skill placement |

## Risks

| Risk | Likelihood | Mitigation |
|------|------------|------------|
| Kimi plugin schema changes across versions | Med | Pin manifest to documented v0.11+ fields; version field enables future detection |
| Plugin dir creation fails (permissions) | Low | Fall back to flat skills dir; log warning |
| Engram MCP bundling adds complexity | Low | Manifest declares server; actual bundling is separate concern |

## Rollback Plan

1. Revert adapter changes — `SkillsDir` returns flat path again
2. Remove `manifest.go` and `PluginInstaller` interface
3. `git revert` the feature branch
4. User runs `gentle-ai sync` to restore flat skill layout

## Dependencies

- kimi-code v0.11+ plugin directory layout (`~/.kimi-code/plugins/managed/`)
- Existing `agents.Adapter` interface — `PluginInstaller` is optional, not breaking
- Existing `filemerge.WriteFileAtomic` for manifest writing

## Success Criteria

- [ ] v0.11+ installs place skills in `~/.kimi-code/plugins/managed/gentle-ai/skills/`
- [ ] `kimi.plugin.json` is generated with correct name, version, skills path, hooks, and mcpServers
- [ ] Legacy installs continue using flat `~/.config/agents/skills/` unchanged
- [ ] `PluginInstaller` interface is opt-in — other adapters unaffected
- [ ] All existing tests pass; new tests cover manifest generation and plugin dir creation
