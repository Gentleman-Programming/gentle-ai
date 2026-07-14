# Design: Kimi Code Plugin Architecture

## Technical Approach

Add a `PluginInstaller` optional interface that Kimi's adapter implements for v0.11+. When present, components write skills and manifests into `~/.kimi-code/plugins/managed/gentle-ai/` instead of the flat `~/.kimi-code/skills/`. Legacy installs (Python/uv-based ~/.kimi) continue using the existing flat path unchanged. This follows the established optional-interface pattern (`bootstrapper`, `workflowInjector`, `piEngramProvisioner`) — adapters opt in; components type-assert and branch.

## Architecture Decisions

| Choice | Alternatives | Rationale |
|--------|-------------|-----------|
| Optional interface on `agents.Adapter` | New field, config flag, separate package | Follows existing pattern (bootstrapper, workflowInjector); no breaking changes; adapter controls behavior |
| `~/.kimi-code/plugins/managed/gentle-ai/` | Flat `plugins/gentle-ai/`, nested under skills | Matches Kimi Code v0.11+ official plugin directory layout |
| Manifest version from `app.Version` | Hardcoded constant, git tag | Always reflects build-time version; avoids drift |
| Fallback to flat dir on error | Abort install | Graceful degradation — user gets skills even if plugin dir creation fails |
| Skills + manifest in one manifest call | Separate calls | Single responsibility: `InstallPlugin` owns the complete plugin scaffold |

## Data Flow

```
SDD/Skills Inject
  │
  ├─ adapter.(PluginInstaller) ?
  │    ├─ YES (v0.11+): PluginDir → write skills + manifest
  │    │    └─ BootstrapTemplate → generate kimi.plugin.json
  │    └─ NO (legacy): SkillsDir → flat path (existing behavior)
  │
  └─ MCP Inject (unchanged) → mcp.json in config dir
```

## File Changes

| File | Action | Description |
|------|--------|-------------|
| `internal/agents/kimi/plugin.go` | Create | `PluginInstaller` implementation: `PluginDir`, `PluginManifestPath`, `InstallPlugin`, manifest struct + generation |
| `internal/agents/kimi/adapter.go` | Modify | `SkillsDir()` returns plugin subdir for v0.11+; `BootstrapTemplate` calls `InstallPlugin` |
| `internal/agents/kimi/adapter_test.go` | Modify | Tests for plugin dir paths, manifest content, v0.11+ vs legacy, error fallback |
| `internal/components/sdd/inject.go` | Modify | Check `PluginInstaller` before `SkillsDir` in skill-write loop (lines 471-526) |
| `internal/components/skills/inject.go` | Modify | `Inject` checks `PluginInstaller` before `SkillsDir` (lines 36-39, 125-131) |

### Line-level precision

**`internal/agents/kimi/plugin.go`** (new file, ~120 lines):
- `KimiPluginManifest` struct with JSON tags: `name`, `version`, `description`, `skills`, `hooks`, `mcpServers`
- `PluginDir(homeDir)` → `~/.kimi-code/plugins/managed/gentle-ai/`
- `PluginManifestPath(homeDir)` → `PluginDir + "/kimi.plugin.json"`
- `InstallPlugin(homeDir string) error` — creates plugin dir, writes manifest, delegates skills to `SkillsDir`
- Manifest template references `app.Version` for version field

**`internal/agents/kimi/adapter.go`**:
- Line 182-187 (`SkillsDir`): Change return for v0.11+ from `~/.kimi-code/skills` to `PluginDir + "/skills"`
- Line 365-406 (`BootstrapTemplate`): After writing KIMI.md, call `a.InstallPlugin(homeDir)` if v0.11+

**`internal/components/sdd/inject.go`**:
- Lines 471-473: Before `skillDir := adapter.SkillsDir(homeDir)`, add:
  ```go
  if pi, ok := adapter.(PluginInstaller); ok {
      skillDir = filepath.Join(pi.PluginDir(homeDir), "skills")
  }
  ```

**`internal/components/skills/inject.go`**:
- Lines 36-39: Same pattern — check `PluginInstaller` interface before falling back to `SkillsDir`

## Interfaces / Contracts

```go
// PluginInstaller is an optional adapter capability for agents that support
// plugin-based distribution (e.g. Kimi Code v0.11+). When implemented,
// components write skills into the plugin directory instead of the flat skills dir.
type PluginInstaller interface {
    PluginDir(homeDir string) string
    PluginManifestPath(homeDir string) string
    InstallPlugin(homeDir string) error
}

// kimi.plugin.json manifest (generated, not hand-edited)
type KimiPluginManifest struct {
    Name        string                    `json:"name"`
    Version     string                    `json:"version"`
    Description string                    `json:"description"`
    Skills      KimiPluginSkills          `json:"skills"`
    Hooks       KimiPluginHooks           `json:"hooks,omitempty"`
    MCPServers  map[string]KimiMCPServer  `json:"mcpServers,omitempty"`
}

type KimiPluginSkills struct {
    Path string `json:"path"`
}

type KimiPluginHooks struct {
    SessionStart *KimiHookRef `json:"sessionStart,omitempty"`
}

type KimiHookRef struct {
    Skill string `json:"skill"`
}

type KimiMCPServer struct {
    Command string   `json:"command"`
    Args    []string `json:"args"`
}
```

## Testing Strategy

| Layer | What | Approach |
|-------|------|----------|
| Unit | Manifest generation correctness | `TestPluginManifest_GeneratesValidJSON`, `TestPluginManifest_IncludesVersion` |
| Unit | Plugin dir path resolution | `TestPluginDir_V11Plus`, `TestPluginDir_Legacy`, `TestSkillsDir_PluginPath` |
| Unit | Interface type assertion | `TestAdapter_ImplementsPluginInstaller` |
| Integration | Skills written to plugin dir | `TestInstallPlugin_CreatesDirAndManifest` with `t.TempDir()` |
| Integration | Fallback on error | `TestInstallPlugin_DirCreationFails_FallsBack` |
| E2E | SDD inject uses plugin path | `TestSDDInject_PluginInstaller` — mock adapter implementing `PluginInstaller` |

## Migration / Rollout

No migration required. This is additive:
1. v0.11+ users: new installs place skills in plugin dir automatically
2. Legacy users: unchanged flat path behavior
3. Existing flat skills: user cleans up manually (noted in proposal out-of-scope)
4. The `PluginInstaller` interface is opt-in — other agents unaffected

## Open Questions

- [ ] Should `AllSkillsDirs` also include the plugin skills path so Kimi discovers skills from both locations during transition?
- [ ] Should the manifest declare a `schemaVersion` field for forward compatibility with future Kimi plugin spec changes?
