# Design: RTK Integration

## Technical Approach

RTK follows the same binary-component pattern as GGA: install a standalone binary, then configure per-agent hooks. The key insight is that RTK already handles its own hook installation via `rtk init` — so the Gentleman installer delegates hook setup to RTK rather than manually writing hook files.

## Architecture Decisions

### Decision: Delegate hook installation to RTK's native `rtk init`

**Choice**: Call `rtk init -g`, `rtk init -g --opencode`, `rtk init -g --agent cursor`, etc. — one per selected agent.
**Alternatives considered**: Manually write hook files into agent config directories (like Engram does for MCP).
**Rationale**: RTK already maintains hook logic for 10 agents. Duplicating this in Gentle AI would create maintenance burden and risk drift. Delegating ensures we always use the latest RTK hook format.

### Decision: Follow GGA's install pattern (Homebrew + curl fallback)

**Choice**: `resolveRTKInstall()` in `installcmd/resolver.go` with Homebrew preferred, curl/download fallback.
**Alternatives considered**: Use `cargo install --git` (RTK's recommended method).
**Rationale**: Homebrew is the project's preferred binary delivery mechanism. Cargo requires Rust toolchain. GitHub Releases binary approach matches Engram's pattern for non-brew platforms.

### Decision: Graceful per-agent failure

**Choice**: If `rtk init` fails for one agent, log warning and continue with remaining agents.
**Alternatives considered**: Abort entire installation on first failure.
**Rationale**: Users may select agents where RTK support varies. Blocking the entire install because one agent's hook fails is bad UX. The spec already defines this behavior.

## Data Flow

```
gentle-ai install --agents claude-code,opencode --component rtk
    │
    ├─ 1. Resolve install command
    │     └─ Homebrew: brew install rtk
    │     └─ Linux/Windows: download binary from GitHub Releases
    │
    ├─ 2. Execute install (pipeline/stages.go)
    │     └─ Binary lands in PATH
    │
    ├─ 3. Configure hooks per agent
    │     ├─ rtk init -g                    → Claude Code hook
    │     └─ rtk init -g --opencode         → OpenCode hook
    │
    └─ 4. Verify
          ├─ rtk --version                  → binary health
          └─ rtk init --show                → hook status per agent
```

## File Changes

| File | Action | Description |
|------|--------|-------------|
| `internal/model/types.go` | Modify | Add `ComponentRTK ComponentID = "rtk"` |
| `internal/catalog/components.go` | Modify | Add RTK to `mvpComponents` slice |
| `internal/installcmd/resolver.go` | Modify | Add `resolveRTKInstall()` case in `ResolveComponentInstall` |
| `internal/components/rtk/install.go` | Create | `InstallCommand()` + `ShouldInstall()` — mirrors GGA pattern |
| `internal/components/rtk/runtime.go` | Create | Hook configuration: `ConfigureAgentHook(agentID)` — calls `rtk init` with correct flags |
| `internal/components/rtk/verify.go` | Create | `VerifyInstalled()` + `VerifyVersion()` — binary + hook health checks |
| `internal/components/rtk/config.go` | Create | Agent-to-flag mapping: maps `AgentClaudeCode` → `-g`, `AgentOpenCode` → `--opencode`, etc. |
| `docs/components.md` | Modify | Add RTK row to component table |

## Interfaces / Contracts

```go
// internal/components/rtk/config.go
package rtk

// AgentFlags maps an AgentID to the rtk init flags for that agent.
// Returns empty string for agents that don't support RTK hooks.
func AgentFlags(agentID model.AgentID) string

// internal/components/rtk/runtime.go
package rtk

// ConfigureAgentHook runs rtk init with the correct flags for the given agent.
// Returns nil on success, error on failure (caller decides whether to abort).
func ConfigureAgentHook(agentID model.AgentID) error

// ConfigureAllHooks iterates over selected agents and calls ConfigureAgentHook.
// Per-agent failures are logged but do not abort the loop.
func ConfigureAllHooks(selectedAgents []model.AgentID) []AgentHookResult

type AgentHookResult struct {
    AgentID model.AgentID
    Success bool
    Err     error
}
```

## Testing Strategy

| Layer | What to Test | Approach |
|-------|-------------|----------|
| Unit | `AgentFlags()` mapping for all 8 agents | Table-driven test, verify correct flags returned |
| Unit | `resolveRTKInstall()` for brew/apt/pacman/winget | Follow existing `resolver_test.go` patterns |
| Unit | `VerifyInstalled()` with/without rtk in PATH | Mock `exec.LookPath` (same pattern as engram/verify.go) |
| Integration | `ConfigureAgentHook()` runs correct `rtk init` command | Mock `exec.Command`, verify args |
| Integration | Graceful failure: one agent fails, others succeed | Mock first `rtk init` to return error, verify loop continues |
| E2E | Full RTK install + hook + uninstall | Docker test with RTK pre-installed in container |

## Migration / Rollout

No data migration required. RTK is additive:
- Existing installs without RTK are unaffected
- `gentle-ai update` can add RTK component without touching existing configs
- RTK hooks are idempotent (`rtk init -g` on an already-configured agent is a no-op)

## Open Questions

- [ ] Should RTK be opt-in in presets or enabled by default? Proposal says "opt-in" for Ecosystem Only, auto-included in Full Gentleman — need to confirm.
- [ ] Minimum RTK version requirement? Current latest is v0.34.1. Should we pin a minimum (e.g., v0.28+)?
- [ ] RTK telemetry is enabled by default. Should Gentle AI disable it (`RTK_TELEMETRY_DISABLED=1`) or leave it to the user?
