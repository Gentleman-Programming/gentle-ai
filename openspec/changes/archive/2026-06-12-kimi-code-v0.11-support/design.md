# Design: Kimi Code v0.11+ Comprehensive Integration

## Technical Approach

Extend the existing v0.11+ path-detection adapter with four additional capabilities: environment variable override for `KIMI_CODE_HOME`, real `config.toml` content with permissions and skills config, project AGENTS.md injection into the `${KIMI_AGENTS_MD}` Jinja variable, and complete skills scan directory enumeration. The approach reuses existing patterns (`filemerge.WriteFileAtomic`, `filemerge.UpsertTOMLTableKey`, `assets.MustRead`) and follows the `StrategyJinjaModules` convention of writing standalone module files alongside the hub template.

## Architecture Decisions

| Decision | Choice | Alternatives | Rationale |
|----------|--------|-------------|-----------|
| Config dir resolution | `KIMI_CODE_HOME` env → `~/.kimi-code` → `~/.kimi` | Ignore env var; require manual path | Kimi Code v0.11+ officially supports `KIMI_CODE_HOME` for data root relocation. Users with custom paths get correct paths without adapter changes. |
| config.toml content | String-based TOML upserts via `filemerge` helpers | Full TOML parser dependency | Follows existing Codex `injectCodexPermissions` pattern. No new dependencies. Kimi config.toml schema is small and stable. |
| Permission format | `[[permission.rules]]` array-of-tables (Kimi native) | Codex `[permissions.gentle-dev]` table format | Kimi uses `[[permission.rules]]` with `decision`/`pattern` fields. Codex format is incompatible. Must match Kimi's documented schema. |
| AGENTS.md injection | Post-process rendered KIMI.md via `strings.ReplaceAll` on `${KIMI_AGENTS_MD}` | Jinja variable resolver; separate include module | Kimi's `{% include %}` directives handle module inclusion. `${KIMI_AGENTS_MD}` is a shell-style variable, not a Jinja tag — it requires string substitution after rendering. Simple and deterministic. |
| Skills scan dirs | New `AllSkillsDirs(homeDir)` method on adapter | Single `SkillsDir` return value | v0.11+ scans three directories. Returning a slice enables complete skill discovery documentation and future use by the skill-registry component. |
| Permission defaults | Auto-approve Read/Grep/Glob/Write; ask for Bash | Bypass all permissions; deny all writes | SDD workflow requires frequent file reads/writes. Auto-approving safe tools reduces friction. Bash remains manual for safety (destructive commands). |

## Data Flow

### KIMI_CODE_HOME Resolution
```
resolveConfigDir(homeDir)
  → os.Getenv("KIMI_CODE_HOME")
    → non-empty AND dir exists? return $KIMI_CODE_HOME
  → pathExists(~/.kimi-code) ? return ~/.kimi-code : return ~/.kimi
```

### BootstrapTemplate (config.toml + KIMI.md)
```
BootstrapTemplate(homeDir)
  → resolveConfigDir(homeDir) → configDir
  → os.MkdirAll(configDir)
  → WriteFileAtomic(configDir/KIMI.md, assets.MustRead("kimi/KIMI.md"))
  → resolveProjectAGENTSMD(homeDir) → agentsMDContent
  → strings.ReplaceAll(kimiMD, "${KIMI_AGENTS_MD}", agentsMDContent)
  → strings.ReplaceAll(kimiMD, "${KIMI_SKILLS}", skillsContent)
  → WriteFileAtomic(configDir/KIMI.md, processedContent)
  → resolveConfigTOMLContent() → TOML string
  → WriteFileAtomic(configDir/config.toml, TOML)  [only if missing]
```

### Permissions Injection
```
permissions.Inject(homeDir, adapter)
  → adapter.Agent() == AgentKimi? 
    → NO: return nil (Kimi permissions handled by BootstrapTemplate)
  → (existing Codex/Claude/OpenCode path unchanged)
```

### Skills Directory Discovery
```
AllSkillsDirs(homeDir)
  → isV11Plus(homeDir)?
    → YES: [$KIMI_CODE_HOME/skills, ~/.agents/skills, .kimi-code/skills]
    → NO: [~/.config/agents/skills]
```

## File Changes

| File | Action | Description |
|------|--------|-------------|
| `internal/agents/kimi/adapter.go` | Modify | (1) `resolveConfigDir`: add `KIMI_CODE_HOME` env check before `~/.kimi-code`. (2) `BootstrapTemplate`: write real config.toml with permissions and skills; resolve `${KIMI_AGENTS_MD}` and `${KIMI_SKILLS}` in KIMI.md. (3) Add `AllSkillsDirs(homeDir) []string`. (4) Add `resolveProjectAGENTSMD(homeDir) string` helper. (5) Add `resolveConfigTOMLContent() string` helper. (6) Update `ConfigPath` standalone to also check `KIMI_CODE_HOME`. |
| `internal/agents/kimi/adapter_test.go` | Modify | Add tests: `TestAdapter_KIMI_CODE_HOME_Override`, `TestAdapter_BootstrapTemplate_ConfigTOML`, `TestAdapter_BootstrapTemplate_AGENTSMD_Resolution`, `TestAdapter_AllSkillsDirs_V11Plus`, `TestAdapter_AllSkillsDirs_Legacy`. Update existing tests for `KIMI_CODE_HOME` env. |
| `internal/components/permissions/inject.go` | Modify | Add `case model.AgentKimi: return nil` to `agentOverlay()` — Kimi permissions are written by `BootstrapTemplate`, not the permissions component. |

## Interfaces / Contracts

### New Methods on Adapter

```go
// AllSkillsDirs returns all skills directories the agent discovers.
// For v0.11+ Kimi: [$KIMI_CODE_HOME/skills, ~/.agents/skills, .kimi-code/skills]
// For legacy Kimi: [~/.config/agents/skills]
func (a *Adapter) AllSkillsDirs(homeDir string) []string
```

### config.toml Schema (Kimi v0.11+)

```toml
default_permission_mode = "manual"
merge_all_available_skills = true

[[permission.rules]]
decision = "allow"
pattern = "Read"

[[permission.rules]]
decision = "allow"
pattern = "Grep"

[[permission.rules]]
decision = "allow"
pattern = "Glob"

[[permission.rules]]
decision = "allow"
pattern = "Write"

[[permission.rules]]
decision = "allow"
pattern = "Edit"

[[permission.rules]]
decision = "allow"
pattern = "Agent"

[[permission.rules]]
decision = "ask"
pattern = "Bash"
```

### KIMI.md Variable Resolution

```go
func (a *Adapter) resolveProjectAGENTSMD(homeDir string) string {
    // Search upward from homeDir for AGENTS.md (project root detection)
    // If found, return content wrapped in markdown block
    // If not found, return "<!-- No project AGENTS.md found -->"
}
```

## Testing Strategy

| Layer | What to Test | Approach |
|-------|-------------|----------|
| Unit | `resolveConfigDir` with `KIMI_CODE_HOME` set/unset | `t.Setenv("KIMI_CODE_HOME", ...)` + temp dirs |
| Unit | `BootstrapTemplate` writes valid TOML | Read written config.toml, parse with `toml.Unmarshal` or string contains |
| Unit | `BootstrapTemplate` resolves `${KIMI_AGENTS_MD}` | Create temp AGENTS.md, run bootstrap, verify KIMI.md content |
| Unit | `AllSkillsDirs` returns correct paths | Mock `pathExists` + `isV11Plus` |
| Unit | `resolveProjectAGENTSMD` with/without AGENTS.md | Temp dir with/without file |
| Unit | `ConfigPath` respects `KIMI_CODE_HOME` | `t.Setenv` + verify return value |
| Integration | `permissions.Inject` returns no-op for Kimi | Verify `agentOverlay(AgentKimi)` returns nil |
| E2E | Full install pipeline writes config.toml | Mock home dir, run pipeline, verify files exist |

## Migration / Rollout

**Zero behavioral change for legacy users**: if `KIMI_CODE_HOME` is unset and `~/.kimi-code` does not exist, every path returns exactly what it returned before.

**For v0.11+ users**: on first `gentle-ai install` or `sync`, the adapter writes a meaningful `config.toml` with `merge_all_available_skills = true` and permission rules. Existing empty `# Kimi Code Config` files are NOT overwritten (only written when missing). Users who already have a `config.toml` with their own settings are unaffected — `BootstrapTemplate` only creates the file if it does not exist.

**AGENTS.md resolution**: the project-root AGENTS.md is read at bootstrap time and injected into KIMI.md. If the user later changes their project AGENTS.md, they must re-run `gentle-ai sync` to update KIMI.md. This is consistent with how other agents (OpenCode, Claude) handle AGENTS.md — it is a one-time injection, not a live link.

## Open Questions

- [ ] Should `BootstrapTemplate` overwrite an existing empty `# Kimi Code Config` config.toml with real content? (Current design: NO — only writes if missing. Alternative: overwrite if content is exactly the legacy placeholder.)
- [ ] Should `resolveProjectAGENTSMD` search upward from `homeDir` or from `os.Getwd()`? (Current design: from `homeDir` since that's the adapter convention. But project AGENTS.md lives at project root, not home root.)
- [ ] Should the `[[permission.rules]]` block include `scope = "project"` or omit it (defaults to `user`)? (Current design: omit — user scope is broader and safer for SDD workflows.)
