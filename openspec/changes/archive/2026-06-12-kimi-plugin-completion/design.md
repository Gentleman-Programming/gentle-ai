# Design: Kimi Code v0.11+ Plugin Integration Completion

## Technical Approach

Complete six gaps in the Kimi plugin lifecycle so `gentle-ai sync` actually installs the plugin at runtime. The change touches four files (plus tests): add a version constant, wire `BootstrapTemplate` → `InstallPlugin`, enrich `config.toml` with hooks and skill dirs, handle Kimi in skill-registry automation, and scaffold the `skills/` subdirectory.

## Architecture Decisions

### Decision: Version Constant Location

**Choice**: Add `const GentleAI = "0.1.0"` to `internal/versions/versions.go`.

| Option | Tradeoff | Decision |
|--------|----------|----------|
| `versions.go` const | No runtime override; goreleaser ldflags must be wired later. Avoids circular imports. | ✅ chosen |
| `cli.AppVersion` var | Already runtime-set via ldflags, but kimi package cannot import `cli` (circular). | rejected |
| `Adapter.Version` method | Every adapter would need implementation; version is project-wide, not adapter-specific. | rejected |

**Rationale**: `versions.go` is the existing canonical source for pinned versions. Adding `GentleAI` const here keeps the single-source-of-truth pattern. Goreleaser can later convert it to a var set via ldflags without changing call sites.

### Decision: Bootstrap Failure on Plugin Install Error

**Choice**: Log warning, do NOT fail bootstrap.

| Option | Tradeoff | Decision |
|--------|----------|----------|
| Fail bootstrap | Safe but aggressive — users lose all config if plugin dir is unwritable. | rejected |
| Log + continue | Bootstrap completes (config.toml, KIMI.md written). Plugin is a best-effort enhancement. | ✅ chosen |

**Rationale**: `BootstrapTemplate` already writes KIMI.md and config.toml successfully before `InstallPlugin` is called. Failing the entire bootstrap for a plugin directory permission issue is disproportionate. The existing pattern in inject.go already logs and continues for non-critical steps.

### Decision: config.toml Enrichment Strategy

**Choice**: `resolveConfigTOMLContent()` returns base TOML; `BootstrapTemplate` appends hooks and extra_skill_dirs conditionally for v0.11+.

| Option | Tradeoff | Decision |
|--------|----------|----------|
| Parameterize `resolveConfigTOMLContent(isV11)` | Clean but changes function signature; all callers must update. | rejected |
| Append in `BootstrapTemplate` | No signature change; conditional logic stays in the one place that knows the version. | ✅ chosen |

**Rationale**: The function is package-private and called from one place. Keeping it pure and doing conditional assembly in `BootstrapTemplate` avoids touching unrelated code paths.

### Decision: Kimi Skill-Registry Hook Format

**Choice**: TOML `[[hooks]]` block in `config.toml` (Kimi's native format), not JSON.

| Option | Tradeoff | Decision |
|--------|----------|----------|
| JSON hooks.json | Wrong format — Kimi uses TOML config, not JSON hooks. | rejected |
| TOML [[hooks]] in config.toml | Native format; same file enriched by BootstrapTemplate. | ✅ chosen |

**Rationale**: Kimi Code v0.11+ reads `config.toml` for all configuration. The `[[hooks]]` TOML array-of-tables syntax is the correct way to declare lifecycle hooks.

## Data Flow

```
gentle-ai sync
  └─ BootstrapTemplate(homeDir)
       ├─ 1. Write KIMI.md (existing)
       ├─ 2. Write config.toml (existing)
       │     └─ append [[hooks]] + extra_skill_dirs if v0.11+
       ├─ 3. Write AGENTS.md (existing)
       └─ 4. InstallPlugin(homeDir, versions.GentleAI) [NEW — v0.11+ only]
             ├─ MkdirAll plugin dir + skills/
             └─ Write kimi.plugin.json

  └─ installSkillRegistryAutomation(homeDir, adapter)
       └─ if AgentKimi: append [[hooks]] sessionStart → config.toml
```

## File Changes

| File | Action | Description |
|------|--------|-------------|
| `internal/versions/versions.go` | Modify | Add `const GentleAI = "0.1.0"` |
| `internal/agents/kimi/adapter.go` | Modify | `BootstrapTemplate`: call `InstallPlugin` for v0.11+; `resolveConfigTOMLContent`: no change (base TOML); append hooks+extra_skill_dirs in `BootstrapTemplate` |
| `internal/agents/kimi/plugin.go` | Modify | `InstallPlugin`: create `skills/` subdirectory alongside manifest |
| `internal/components/sdd/inject.go` | Modify | `installSkillRegistryAutomation`: add `model.AgentKimi` case writing TOML hooks to config.toml |
| `internal/agents/kimi/adapter_test.go` | Modify | Test BootstrapTemplate calls InstallPlugin for v0.11+; test config.toml contains hooks and extra_skill_dirs |
| `internal/agents/kimi/plugin_test.go` | Modify | Test `skills/` subdirectory is created; test idempotency |

## Interfaces / Contracts

No interface changes. The existing `PluginInstaller` interface (`InstallPlugin(homeDir, version string) error`) is sufficient. The version parameter is populated from `versions.GentleAI`.

New constant:
```go
// internal/versions/versions.go
const GentleAI = "0.1.0"
```

config.toml hooks format (appended for v0.11+):
```toml
[[hooks]]
event = "sessionStart"
command = "gentle-ai skill-registry refresh --quiet --no-gitignore --cwd \"$PWD\" || true"
```

config.toml extra_skill_dirs (appended for v0.11+):
```toml
extra_skill_dirs = ["~/.config/agents/skills", "~/.agents/skills"]
```

Plugin manifest `skills/` directory creation (in `InstallPlugin`):
```go
skillsDir := filepath.Join(pluginDir, "skills")
if err := os.MkdirAll(skillsDir, 0o755); err != nil {
    return fmt.Errorf("create plugin skills dir: %w", err)
}
```

## Testing Strategy

| Layer | What to Test | Approach |
|-------|-------------|----------|
| Unit | `versions.GentleAI` is non-empty | Compile-time check in adapter_test.go |
| Unit | `BootstrapTemplate` calls `InstallPlugin` for v0.11+ | Mock adapter with v0.11+ detection; verify `InstallPlugin` called |
| Unit | `BootstrapTemplate` does NOT call `InstallPlugin` for legacy | Mock adapter without v0.11+; verify no plugin files created |
| Unit | config.toml contains `[[hooks]]` and `extra_skill_dirs` for v0.11+ | Read written config.toml; assert TOML sections present |
| Unit | `InstallPlugin` creates `skills/` subdirectory | Call `InstallPlugin`; stat `skills/` dir |
| Unit | `installSkillRegistryAutomation` handles Kimi | Call with `model.AgentKimi`; verify TOML hooks in config.toml |
| Unit | Idempotency: re-running `InstallPlugin` preserves existing dirs | Call twice; verify no error, existing contents preserved |

## Migration / Rollout

No migration required. This change adds functionality to existing code paths. Users on legacy Kimi (< v0.11) are unaffected — the `isV11Plus` guard prevents any behavioral change.

## Open Questions

- [ ] Should `extra_skill_dirs` paths be platform-aware (Windows `%USERPROFILE%` vs Unix `~`)? Kimi Code likely handles `~` expansion, but worth verifying.
- [ ] Should the skill-registry hook use the same `gentle-ai skill-registry refresh` command as Codex/Claude, or a Kimi-specific variant?
