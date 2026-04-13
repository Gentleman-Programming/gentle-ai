# Design: Agent Builder — Create Custom Sub-Agents from the TUI

## Technical Approach

Isolate all generation logic in a new `internal/agentbuilder/` package behind clean interfaces. Extend `Model` with an embedded `AgentBuilderState` struct that owns all builder-specific fields, keeping model.go changes surgical. Use `charmbracelet/textarea` for multi-line prompt input — it's the same org as Bubbletea and handles wrapping, scrolling, and cursor natively. Screen constants use sequential iota (no offset) appended after `ScreenModelConfig` to match existing convention.

## Architecture Decisions

| Decision | Choice | Alternatives | Rationale |
|----------|--------|--------------|-----------|
| Screen constants | Sequential iota after `ScreenModelConfig` | Offset block (`iota + 100`) | Codebase uses single contiguous iota block; offset breaks the pattern, complicates `optionCount()` range checks, and gains nothing — no external consumers depend on numeric values |
| Router integration | Extend `linearRoutes` map with 8 new entries | Separate `agentBuilderRoutes` merged at init | Single map is simpler; `PreviousScreen()` already covers all screens via one lookup; separate map adds merge complexity for zero benefit |
| Text input | `charmbracelet/textarea` component | Custom `handleRenameInput`-style logic | textarea gives multi-line, scroll, word-wrap, paste, undo for free; custom code would duplicate what rename already struggles with at single-line level |
| AgentBuilderState isolation | Embedded struct in Model, textarea.Model inside it | Flat fields on Model | Keeps Model diff small (~5 lines); all builder state is zero-value safe — no allocation until user enters the flow |
| Generation engine abstraction | Interface in `agentbuilder/engine.go` with per-agent structs | Switch statement on AgentID | Interface enables test mocking via `MockEngine`; matches existing `Adapter` pattern; new engines = new struct, no switch changes |
| Async generation | Goroutine returning `tea.Cmd` (existing `startInstalling` pattern) | Channel-based, or `tea.Exec` | Follows proven project pattern; `context.WithTimeout` for cancellation; spinner stays responsive via `TickMsg` |
| Installation target detection | `agents.DiscoverInstalled(registry, homeDir)` | Re-run `Adapter.Detect()` per agent | `DiscoverInstalled` already exists, is pure FS check, no subprocess; returns `InstalledAgent` with ConfigDir — exactly what installer needs |
| Custom agent registry | JSON file at `~/.config/gentle-ai/custom-agents.json` | SQLite, or embed in agent config | JSON is human-readable, zero dependencies, matches ecosystem config patterns; versioned schema for forward compat |

## Data Flow

```
User: "Create your own Agent"
  │
  ▼
ScreenABEngine ──(select engine)──▶ ScreenABPrompt ──(textarea input)──▶ ScreenABSDD
  │                                                                         │
  │                                          ┌──────────────────────────────┤
  │                                          ▼ (if new-phase or phase-support)
  │                                   ScreenABSDDPhase ──(select phase)──┐
  │                                                                      │
  ▼◀─────────────────────────────────────────────────────────────────────┘
ScreenABGenerating
  │  goroutine: compose prompt → engine.Generate(ctx, prompt) → parse output
  │  sends AgentBuilderGeneratedMsg{Agent, Err}
  ▼
ScreenABPreview ──(Install / Regenerate / Back)
  │
  ▼ (Install)
ScreenABInstalling
  │  goroutine: for each installed agent → write SKILL.md → update registry → SDD inject
  │  sends AgentBuilderInstallDoneMsg{Results, Err}
  ▼
ScreenABComplete ──(Enter)──▶ ScreenWelcome
```

### Goroutine Boundaries

1. **Generation** (`startGeneration() tea.Cmd`): Composes prompt, calls `engine.Generate(ctx, prompt)`, parses result. Returns `AgentBuilderGeneratedMsg`.
2. **Installation** (`startInstallation() tea.Cmd`): Walks installed agents, writes SKILL.md, updates registry, injects SDD markers. Returns `AgentBuilderInstallDoneMsg`.

### Message Types

| Message | Trigger | Fields |
|---------|---------|--------|
| `AgentBuilderGeneratedMsg` | Generation goroutine completes | `Agent *GeneratedAgent, Err error` |
| `AgentBuilderInstallDoneMsg` | Installation goroutine completes | `Results []InstallResult, Err error` |

## File Changes

| File | Action | Description |
|------|--------|-------------|
| `internal/agentbuilder/engine.go` | Create | `GenerationEngine` interface + `ClaudeEngine`, `OpenCodeEngine`, `GeminiEngine`, `CodexEngine` structs. `NewEngine(agentID, binaryPath) GenerationEngine`. Each wraps `exec.CommandContext` with agent-specific CLI flags. |
| `internal/agentbuilder/prompt.go` | Create | `ComposePrompt(userInput string, sddConfig *SDDIntegration, installedAgents []model.AgentID) string`. Concatenates system prompt template + user description + SDD context + agent list. |
| `internal/agentbuilder/parser.go` | Create | `Parse(raw string) (*GeneratedAgent, error)`. Strips code fences, validates required sections (Description, Trigger, Instructions), extracts metadata, generates kebab-case name from title. |
| `internal/agentbuilder/installer.go` | Create | `Install(agent *GeneratedAgent, targets []agents.InstalledAgent, homeDir string) ([]InstallResult, error)`. Writes `SKILL.md` to each target's `SkillsDir()`. Atomic: on failure, cleans up already-written files. |
| `internal/agentbuilder/registry.go` | Create | `Registry` struct: `Load(path) error`, `Save(path) error`, `Add(entry RegistryEntry) error`, `List() []RegistryEntry`. JSON file at `~/.config/gentle-ai/custom-agents.json` with version field. |
| `internal/agentbuilder/sdd.go` | Create | `InjectSDDReference(agent *GeneratedAgent, adapter agents.Adapter, homeDir string) error`. Reads system prompt file, injects `<!-- gentle-ai:custom-agent:{name} -->` marker section via existing `StrategyMarkdownSections` logic, writes back. |
| `internal/agentbuilder/types.go` | Create | `GeneratedAgent`, `SDDIntegration`, `SDDIntegrationMode`, `RegistryEntry`, `InstallResult` structs. Shared types for the package. |
| `internal/tui/screens/agent_builder_engine.go` | Create | `RenderABEngine(availableEngines []model.AgentID, cursor int) string`. List of detected engines with radio selection. |
| `internal/tui/screens/agent_builder_prompt.go` | Create | `RenderABPrompt(ta textarea.Model) string`. Renders textarea component with helper text and examples. |
| `internal/tui/screens/agent_builder_sdd.go` | Create | `RenderABSDD(mode SDDIntegrationMode, cursor int) string` and `RenderABSDDPhase(phases []string, cursor int) string`. Radio select for SDD mode and phase picker. |
| `internal/tui/screens/agent_builder_generating.go` | Create | `RenderABGenerating(engineName string, spinnerFrame int) string`. Spinner with engine name. |
| `internal/tui/screens/agent_builder_preview.go` | Create | `RenderABPreview(agent *GeneratedAgent, targets []string, scroll int) string`. Scrollable preview with metadata header, rendered SKILL.md, install targets, action buttons. |
| `internal/tui/screens/agent_builder_complete.go` | Create | `RenderABComplete(agent *GeneratedAgent, results []InstallResult) string`. Success message with installed targets. |
| `internal/tui/model.go` | Modify | Add 8 Screen constants after `ScreenModelConfig`. Add `AgentBuilder AgentBuilderState` field. Add `textarea.Model` import. Add `AgentBuilderGeneratedMsg`/`AgentBuilderInstallDoneMsg` handlers in `Update()`. Add cases in `View()`, `optionCount()`, `confirmSelection()`, `goBack()`, `handleKeyPress()`. Add `startGeneration()`, `startInstallation()` methods. |
| `internal/tui/model.go` | Modify | `handleKeyPress`: intercept `tea.KeyMsg` for agent builder textarea screens (delegate to `textarea.Update()` when on `ScreenABPrompt`), similar to existing `ScreenRenameBackup` pattern. |
| `internal/tui/router.go` | Modify | Add 8 entries to `linearRoutes` map for the builder sub-flow. |
| `internal/tui/screens/welcome.go` | Modify | Insert `"Create your own Agent"` at index 5 (before "Manage backups"). Update `WelcomeOptions()` to conditionally disable when no agents have CLI generation support. |
| `go.mod` | Modify | Add `github.com/charmbracelet/bubbles` dependency (textarea lives in bubbles). |

## Interfaces / Contracts

```go
// internal/agentbuilder/engine.go
type GenerationEngine interface {
    Agent() model.AgentID
    Generate(ctx context.Context, prompt string) (string, error)
    Available() bool
}

// internal/agentbuilder/types.go
type GeneratedAgent struct {
    Name        string
    Title       string
    Description string
    Trigger     string
    Content     string
    SDDConfig   *SDDIntegration
}

type SDDIntegrationMode string
const (
    SDDStandalone   SDDIntegrationMode = "standalone"
    SDDNewPhase     SDDIntegrationMode = "new-phase"
    SDDPhaseSupport SDDIntegrationMode = "phase-support"
)

// internal/tui/model.go (embedded in Model)
type AgentBuilderState struct {
    AvailableEngines []model.AgentID
    SelectedEngine   model.AgentID
    Textarea         textarea.Model      // charmbracelet/bubbles textarea
    SDDMode          SDDIntegrationMode
    SDDTargetPhase   string
    Generating       bool
    Generated        *agentbuilder.GeneratedAgent
    GenerationErr    error
    Installing       bool
    InstallResults   []agentbuilder.InstallResult
    InstallErr       error
    PreviewScroll    int
    Targets          []agents.InstalledAgent
}
```

## Testing Strategy

| Layer | What to Test | Approach |
|-------|-------------|----------|
| Unit | `parser.Parse()` — valid/invalid/edge-case SKILL.md outputs | Table-driven tests with raw string inputs; validate extracted fields, error on missing sections |
| Unit | `prompt.ComposePrompt()` — correct concatenation of system + user + SDD context | Assert substrings present, SDD context conditional on mode |
| Unit | `registry.Load/Save/Add` — JSON serialization round-trip | Temp file, write → read → compare; test version migration path |
| Unit | `installer.Install()` — writes to correct paths, atomic cleanup on failure | Temp dirs simulating agent skill dirs; inject failure at Nth write, assert cleanup of previous N-1 |
| Unit | `sdd.InjectSDDReference()` — marker injection without clobbering | Provide file with existing content; assert markers present, existing content preserved |
| Unit | Engine implementations — `Available()`, `Generate()` with mock binary | `MockEngine` struct implementing `GenerationEngine` for TUI tests; real engine tests use `exec.Command` with a test script that echoes expected output |
| TUI | Screen navigation: forward/backward through all 8 screens | Direct `Model.Update(tea.KeyMsg{})` simulation; assert `Model.Screen` transitions |
| TUI | Textarea input: type characters, multi-line, submit | Send `tea.KeyMsg` with runes to model on `ScreenABPrompt`; assert `AgentBuilder.Textarea.Value()` |
| TUI | Generation async: spinner ticks, done message transitions screen | Send `AgentBuilderGeneratedMsg` to model; assert screen moves to preview |
| Integration | Full pipeline: compose → generate (mock) → parse → install (temp dir) | `MockEngine` returns canned SKILL.md; assert files written to temp dirs, registry updated |

## Migration / Rollout

No migration required. All changes are additive:
- New package `internal/agentbuilder/` — no existing code modified
- New screen constants appended to existing iota block
- New menu option on Welcome screen
- Custom agent registry created on first use (no pre-existing file to migrate)

## Open Questions

- [ ] Should "Edit" (open `$EDITOR`) be P0 or deferred to V1.1? It requires `tea.Exec` to suspend the TUI — well-supported by Bubbletea but adds complexity to the preview screen flow.
- [ ] Should generation engines that require API keys (future) have a validation step before generation starts?
