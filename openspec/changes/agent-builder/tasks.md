# Tasks: Agent Builder — Create Custom Sub-Agents from the TUI

## Phase 1: Foundation — Types, Interfaces, Constants

- [ ] T-01 Create `internal/agentbuilder/types.go` — define `GeneratedAgent`, `SDDIntegration`, `SDDIntegrationMode` (standalone/new-phase/phase-support), `RegistryEntry`, `InstallResult` structs
- [ ] T-02 Create `internal/agentbuilder/engine.go` — define `GenerationEngine` interface (`Agent()`, `Generate()`, `Available()`); implement `ClaudeEngine`, `OpenCodeEngine`, `GeminiEngine`, `CodexEngine` via `exec.CommandContext`; add `NewEngine(agentID, binaryPath) GenerationEngine` factory
- [ ] T-03 Add 8 `Screen` constants to `internal/tui/model.go` after `ScreenModelConfig` (iota block); add `AgentBuilderState` struct with all fields including `textarea.Model`; add `AgentBuilderGeneratedMsg` and `AgentBuilderInstallDoneMsg` message types
- [ ] T-04 Update `go.mod` — add `github.com/charmbracelet/bubbles` dependency (textarea component)

## Phase 2: Core Engine — Generation Logic

- [ ] T-05 Create `internal/agentbuilder/prompt.go` — implement `ComposePrompt(userInput string, sddConfig *SDDIntegration, installedAgents []model.AgentID) string`; include system prompt template, user description, conditional SDD context block, installed agents list
- [ ] T-06 Create `internal/agentbuilder/parser.go` — implement `Parse(raw string) (*GeneratedAgent, error)`; strip code fences, validate required sections (Description, Trigger, Instructions), extract Name (title→kebab-case), Title, Description, Trigger into `GeneratedAgent`
- [ ] T-07 Create `internal/agentbuilder/registry.go` — implement `Registry` struct with `Load(path)`, `Save(path)`, `Add(entry)`, `List()` methods; JSON at `~/.config/gentle-ai/custom-agents.json`; `version: 1` field; conflict detection for built-in and custom name collisions
- [ ] T-08 Create `internal/agentbuilder/installer.go` — implement `Install(agent, targets, homeDir)` writing `SKILL.md` to each agent's `SkillsDir()`; atomic rollback on partial failure (clean up already-written files on any error)
- [ ] T-09 Create `internal/agentbuilder/sdd.go` — implement `InjectSDDReference(agent, adapter, homeDir)` injecting `<!-- gentle-ai:custom-agent:{name} -->` marker blocks into agent system prompts; handle phase-support (augment existing) and new-phase (insert into pipeline graph); no duplication on re-run

## Phase 3: TUI Screens

- [ ] T-10 Create `internal/tui/screens/agent_builder_engine.go` — `RenderABEngine(availableEngines []model.AgentID, cursor int) string`; list detected engines with j/k navigation, Enter to select, Esc to Welcome
- [ ] T-11 Create `internal/tui/screens/agent_builder_prompt.go` — `RenderABPrompt(ta textarea.Model) string`; render textarea with helper text, example prompts, disabled Continue hint when empty
- [ ] T-12 Create `internal/tui/screens/agent_builder_sdd.go` — `RenderABSDD(mode SDDIntegrationMode, cursor int) string`; 3-option radio (Standalone / New SDD Phase / Support existing phase); `RenderABSDDPhase(phases []string, cursor int) string` for phase picker sub-screen
- [ ] T-13 Create `internal/tui/screens/agent_builder_generating.go` — `RenderABGenerating(engineName string, spinnerFrame int) string`; spinner animation with engine name, cancel hint
- [ ] T-14 Create `internal/tui/screens/agent_builder_preview.go` — `RenderABPreview(agent *GeneratedAgent, targets []string, scroll int) string`; metadata header (Name, Description, Trigger, SDD), scrollable SKILL.md pane, Install/Edit/Regenerate/Back action bar
- [ ] T-15 Create `internal/tui/screens/agent_builder_complete.go` — `RenderABComplete(agent *GeneratedAgent, results []InstallResult) string`; per-agent install status (✓/✗), usage instruction referencing trigger, Done → Welcome

## Phase 4: Integration — Wiring

- [ ] T-16 Modify `internal/tui/router.go` — add 8 entries to `linearRoutes` map for agent builder sub-flow (`ScreenABEngine` → `ScreenABPrompt` → … → `ScreenABComplete`); `PreviousScreen()` handles Esc correctly
- [ ] T-17 Modify `internal/tui/screens/welcome.go` — insert "Create your own Agent" at index 5; update `WelcomeOptions()` to disable (append "(no agents)") when no generation-capable engine is detected
- [ ] T-18 Modify `internal/tui/model.go` `Update()` — add `AgentBuilderGeneratedMsg`/`AgentBuilderInstallDoneMsg` handlers; delegate `tea.KeyMsg` to `textarea.Update()` on `ScreenABPrompt`; add `startGeneration()` and `startInstallation()` goroutine cmd methods (120s timeout)
- [ ] T-19 Modify `internal/tui/model.go` `View()` / `confirmSelection()` / `goBack()` / `handleKeyPress()` — add cases for all 8 agent builder screens; Esc calls `goBack()` preserving `AgentBuilderState`; empty prompt blocks Enter on `ScreenABPrompt`
- [ ] T-20 Modify `internal/tui/model.go` `optionCount()` — add agent builder screen cases returning correct item counts for j/k cursor bounds

## Phase 5: Testing

- [ ] T-21 Write `internal/agentbuilder/parser_test.go` — table-driven: valid full output, missing Trigger section, missing Instructions section, code-fence stripping, kebab-case name generation
- [ ] T-22 Write `internal/agentbuilder/prompt_test.go` — assert SDD context block present for phase-support mode, absent for standalone; assert installed agents list in output; assert output starts with system prompt header
- [ ] T-23 Write `internal/agentbuilder/registry_test.go` — JSON round-trip (write → read → compare), first-install creates file with `version:1`, second install appends entry, version preserved, conflict detection for built-in and custom names
- [ ] T-24 Write `internal/agentbuilder/installer_test.go` — temp dirs simulating agent skill dirs; happy path writes SKILL.md to all targets; atomic rollback: inject failure at 2nd write, assert 1st file removed
- [ ] T-25 Write `internal/agentbuilder/sdd_test.go` — inject marker into empty file, inject into file with existing content (no duplication), phase-support adds correct reference, new-phase updates dependency graph string
- [ ] T-26 Write `internal/agentbuilder/engine_test.go` — `MockEngine` struct implementing `GenerationEngine`; `Available()` returns false when binary missing; verify CLI flag assembly per engine type
- [ ] T-27 Write `internal/tui/screens/agent_builder_*_test.go` (one per screen file) — render smoke tests: non-empty string output, key strings present (engine name, "Continue", "Esc")
- [ ] T-28 Write TUI navigation tests in `internal/tui/model_test.go` (or dedicated file) — `Model.Update(tea.KeyMsg{Type: tea.KeyEnter})` on each AB screen asserts correct `Model.Screen` transition; Esc from each screen returns to prior; empty textarea blocks Enter on `ScreenABPrompt`
- [ ] T-29 Write integration test — `MockEngine` returns canned valid SKILL.md; call `startGeneration()` cmd, send resulting msg to model, assert screen → Preview; then `startInstallation()` to temp dirs, assert files written and registry updated
