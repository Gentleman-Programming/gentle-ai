# Design: Qwen Code Agent Integration

## Technical Approach

Mirror the Gemini CLI adapter pattern with minimal deviations. Qwen Code's config model (`~/.qwen/`, `settings.json` with `mcpServers`, `QWEN.md`) is structurally identical to Gemini CLI's, allowing reuse of `StrategyFileReplace` and `StrategyMergeIntoSettings`. The only behavioral difference is Qwen Code's support for slash commands (`SupportsSlashCommands() = true`), which Gemini CLI does not have.

## Architecture Decisions

| Decision | Choice | Alternatives | Rationale |
|----------|--------|--------------|-----------|
| System prompt strategy | `StrategyFileReplace` | `StrategyMarkdownSections` | Matches Gemini CLI; persona writes base file, SDD/engram append via markers. Proven pattern for agents with single-file system prompts. |
| MCP strategy | `StrategyMergeIntoSettings` | `StrategySeparateMCPFiles` | Qwen uses `mcpServers` key inside `settings.json` — same as Gemini CLI. Deep merge handles incremental additions without clobbering user config. |
| Install method | npm global (`@qwen-code/qwen-code@latest`) | Homebrew, direct binary download | Official installation method; follows existing pattern for Claude, Gemini, Codex. No standalone binary available. |
| Permission mode | `auto_edit` | `bypassPermissions`, `default` | Matches Qwen CLI's native model — auto-approve file edits, manual shell approval. Balanced between productivity and safety. |
| Engram slug | `"qwen-code"` | `"qwen"`, `"qwen-cli"` | Matches `AgentID` value for consistency. Dashes follow existing convention (`claude-code`, `gemini-cli`). |
| Slash command support | `true` | `false` (like Gemini CLI) | Qwen natively supports `~/.qwen/commands/*.md` — leaving this unexposed would waste a capability. Opens future SDD slash command opportunities. |
| SDD orchestrator asset | Dedicated `qwen/sdd-orchestrator.md` | Reuse `gemini/sdd-orchestrator.md` | Dedicated asset allows Qwen-specific path references (`~/.qwen/skills/`) and future customization. Follows precedent of Gemini, Codex, Windsurf, Cursor each having their own. |

## Data Flow

```
TUI: User selects "qwen-code" in agent selection screen
  │
  ▼
CLI: --agent qwen-code flag parsed by validate.go
  │
  ▼
Planner: Graph resolves component order:
  context7 → persona → engram → gga → permissions → sdd → skills
  (soft ordering: persona before engram, persona before sdd)
  │
  ▼
Pipeline: Prepare → Apply
  │
  ├── persona: writes QWEN.md (StrategyFileReplace)
  ├── engram: merges engram MCP into settings.json + injects protocol into QWEN.md
  ├── permissions: merges auto_edit overlay into settings.json
  ├── context7: merges Context7 MCP server into settings.json
  ├── sdd: injects orchestrator into QWEN.md + writes skill files to ~/.qwen/skills/
  └── skills: writes selected skill files to ~/.qwen/skills/
  │
  ▼
Verify: Post-install check confirms QWEN.md contains sdd-orchestrator marker
```

## File Changes

| File | Action | Description |
|------|--------|-------------|
| `internal/agents/qwen/adapter.go` | Create | Qwen Code adapter: detection, install, config paths, strategies, capabilities |
| `internal/agents/qwen/adapter_test.go` | Create | Table-driven tests for detection, install, config paths, capabilities |
| `internal/assets/qwen/sdd-orchestrator.md` | Create | SDD orchestrator prompt with `~/.qwen/skills/` path references |
| `internal/model/types.go` | Modify | Add `AgentQwenCode AgentID = "qwen-code"` constant |
| `internal/agents/factory.go` | Modify | Import qwen package, register in `NewAdapter()` and `NewDefaultRegistry()` |
| `internal/catalog/agents.go` | Modify | Add `{ID: AgentQwenCode, Name: "Qwen Code", Tier: TierFull, ConfigPath: "~/.qwen"}` |
| `internal/assets/assets.go` | Modify | Add `all:qwen` to `//go:embed` directive |
| `internal/components/sdd/inject.go` | Modify | Add `case model.AgentQwenCode: return "qwen/sdd-orchestrator.md"` in `sddOrchestratorAsset()` |
| `internal/components/permissions/inject.go` | Modify | Add `qwenCodeOverlayJSON` variable and `case model.AgentQwenCode` in `agentOverlay()` |
| `internal/components/engram/setup.go` | Modify | Add `case model.AgentQwenCode: return "qwen-code", true` in `SetupAgentSlug()` |
| `internal/system/config_scan.go` | Modify | Add `{Agent: "qwen-code", Path: "~/.qwen"}` in `knownAgentConfigDirs()` |
| `internal/cli/validate.go` | Modify | Add `case string(model.AgentQwenCode)` in agent validation switch |
| `internal/tui/model.go` | Modify | Add `case string(model.AgentQwenCode)` in `loadSelection()` switch |

## Test File Changes

| File | Modification |
|------|-------------|
| `internal/agents/qwen/adapter_test.go` | New — 4 test suites, 17+ subtests |
| `internal/components/sdd/inject_test.go` | Add `TestInjectQwenCodeWritesSDDOrchestratorAndSkills` + asset selection case |
| `internal/components/engram/setup_test.go` | Add `{AgentQwenCode, "qwen-code", true}` test case |
| `internal/cli/install_test.go` | Add `AgentQwenCode` to default selection + agent mapping + known agents list |
| `internal/agents/registry_test.go` | Add `AgentQwenCode` to default registry test |
| `internal/tui/model_test.go` | Add `"qwen-code"` to `makeDetectionWithAgents()` known agents |

## Testing Strategy

| Layer | What to Test | Approach |
|-------|-------------|----------|
| Unit | `Detect()` — binary found/missing, stat error | Table-driven with injected `lookPath` and `statPath` mocks |
| Unit | `InstallCommand()` — darwin, linux+sudo, linux+nvm, windows | Table-driven with `PlatformProfile` structs |
| Unit | Config paths — all methods return correct paths | Table-driven with name/expected pairs |
| Unit | Capabilities — all boolean flags | Table-driven with name/expected pairs |
| Unit | Strategies — `SystemPromptStrategy`, `MCPStrategy` | Table-driven |
| Integration | SDD injection — orchestrator written to QWEN.md, skill files created | Temp home directory, call `Inject()`, assert file contents |
| Integration | Engram setup — slug mapping correct | `SetupAgentSlug()` table-driven test |
| Registry | Default registry includes Qwen Code | `TestDefaultRegistryIncludesAllAgents` extended |
| CLI | Agent validation accepts `"qwen-code"` | `TestDefaultAgentsFromDetection_AllAgentsMappedCorrectly` extended |
| TUI | State restoration includes qwen-code | `makeDetectionWithAgents()` known agents extended |

## Migration / Rollout

No migration required. All changes are additive:
- New adapter package — no existing code modified
- New asset directory — no existing files modified
- New entries in switch statements — no existing cases changed
- New config scan entry — existing agents unaffected
- New catalog entry — existing catalog entries unchanged

## Agent Comparison: Qwen Code vs Gemini CLI

| Aspect | Gemini CLI | Qwen Code | Same? |
|--------|-----------|-----------|-------|
| Config root | `~/.gemini/` | `~/.qwen/` | ✅ Pattern match |
| System prompt | `GEMINI.md` | `QWEN.md` | ✅ Pattern match |
| Settings | `settings.json` | `settings.json` | ✅ Same file |
| MCP key | `mcpServers` | `mcpServers` | ✅ Same key |
| Skills dir | `~/.gemini/skills/` | `~/.qwen/skills/` | ✅ Pattern match |
| System prompt strategy | `StrategyFileReplace` | `StrategyFileReplace` | ✅ Same |
| MCP strategy | `StrategyMergeIntoSettings` | `StrategyMergeIntoSettings` | ✅ Same |
| Auto-install | Yes (npm) | Yes (npm) | ✅ Same |
| Slash commands | No | **Yes** | ❌ Qwen adds capability |
| Output styles | No | No | ✅ Same |
