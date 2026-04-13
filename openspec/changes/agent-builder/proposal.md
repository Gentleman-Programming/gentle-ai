# Proposal: Agent Builder — Create Custom Sub-Agents from the TUI

## Intent

Users who need specialized AI agents (code reviewer, doc generator, migration validator) must today write SKILL.md files by hand, understand each agent's config structure, and copy files to multiple directories. This friction blocks adoption of the skill ecosystem. The Agent Builder removes that barrier: describe what you want in plain language, pick an installed AI agent to generate it, preview the result, and install it across all configured agents in one step.

## Scope

### In Scope
- 8 new TUI screens for the builder flow (engine → prompt → SDD → generating → preview → installing → complete, plus SDD phase picker)
- `internal/agentbuilder/` package: GenerationEngine interface, prompt composition, output parsing, skill installer, custom-agent registry, SDD integration
- Welcome screen: add "Create your own Agent" menu option (disabled when no agents installed)
- Model extensions: `AgentBuilderState` embedded in `Model`, new `Screen` constants
- Router extensions: agent builder sub-flow routes
- Custom-agent registry at `~/.config/gentle-ai/custom-agents.json`
- SDD integration: standalone, phase-support (marker injection), new-phase (pipeline update)
- Generation engine implementations: Claude Code (`--print`), OpenCode (`run`), Gemini CLI (`-p`), Codex (`exec`)
- Multi-line text input using `charmbracelet/textarea`

### Out of Scope
- Agent management screen (list/edit/delete custom agents)
- Marketplace / sharing
- Templates / starter kits
- Multi-model generation pipeline
- Agent testing sandbox
- Version control for custom agents

## Capabilities

### New Capabilities
- `agent-builder-tui`: 8-screen Bubbletea flow for creating custom agents from natural language
- `generation-engine`: Interface + implementations wrapping installed AI agent CLIs for non-interactive generation
- `custom-agent-registry`: JSON registry tracking created agents with metadata and SDD config
- `skill-installer`: Cross-agent SKILL.md writer that installs to all configured agents atomically

### Modified Capabilities
- `gga`: None (spec unchanged)

## Approach

1. **Infrastructure first**: Define `GenerationEngine` interface and `AgentBuilderState` struct. Add Screen constants and router entries. Wire "Create your own Agent" into the Welcome menu.
2. **Core engine**: Implement engine adapters (Claude, OpenCode, Gemini, Codex) using `exec.CommandContext`. Build prompt composer (system prompt + user input + SDD context). Build output parser with section validation and metadata extraction.
3. **TUI screens**: Build screens in flow order — engine selection (list of detected agents), prompt input (textarea component), SDD integration (radio select), generating (spinner + goroutine), preview (scrollable rendered output), installing (progress), complete.
4. **Installation**: Skill installer walks `agents.Registry`, writes SKILL.md to each agent's `SkillsDir()`. Registry manager writes/reads `custom-agents.json`. SDD integrator injects marker sections into system prompts via existing `StrategyMarkdownSections`.
5. **Testing**: Table-driven unit tests for parser, prompt composer, registry. TUI tests via direct `Model.Update()` keypress simulation. Integration test for engine with mock binary.

## Affected Areas

| Area | Impact | Description |
|------|--------|-------------|
| `internal/agentbuilder/` | New | Core package: engine, parser, installer, registry, prompt, sdd |
| `internal/tui/model.go` | Modified | Add `AgentBuilderState`, 8 Screen constants, message types |
| `internal/tui/router.go` | Modified | Add agent builder sub-flow routes |
| `internal/tui/screens/welcome.go` | Modified | Add "Create your own Agent" menu option |
| `internal/tui/screens/agent_builder_*.go` | New | 6 screen render files (engine, prompt, sdd, generating, preview, complete) |
| `internal/agents/interface.go` | Read-only | Reuse Adapter for detection and SkillsDir |
| `internal/model/types.go` | Read-only | Reuse AgentID for engine selection |
| `go.mod` | Modified | Add `charmbracelet/textarea` dependency |

## Risks

| Risk | Likelihood | Mitigation |
|------|------------|------------|
| AI engine CLI interface changes break generation | Med | Version-pin tested CLI flags; engine.Available() validates before use |
| Generated SKILL.md quality varies wildly by engine | High | Strict output parser with required-section validation; "Regenerate" button; user can edit before install |
| Model.go complexity explosion (already 1500+ lines) | Med | AgentBuilderState as separate struct; screen handlers in dedicated files; minimize Update() changes |
| Textarea dependency conflicts with existing bubbletea version | Low | charmbracelet/textarea is maintained by same org; pin compatible version |
| SDD system prompt injection corrupts existing content | Low | Reuse proven StrategyMarkdownSections marker approach; add rollback for failed installs |
| Generation timeout blocks TUI | Low | Goroutine + context.WithTimeout (120s default); spinner stays responsive |

## Rollback Plan

1. Revert the `feature/agent-builder` branch merge — all changes are additive (new package, new screens, new menu option)
2. Custom-agent registry (`~/.config/gentle-ai/custom-agents.json`) can be deleted manually
3. Installed custom skills are standalone SKILL.md files — deleting the directory removes the agent
4. SDD marker injections use unique `<!-- gentle-ai:custom-agent:{name} -->` markers — can be grep'd and removed

## Dependencies

- `charmbracelet/textarea` — multi-line text input component for the prompt screen
- Existing `agents.Registry` + `Adapter` interface — detection, SkillsDir, SystemPromptStrategy
- At least one AI agent binary installed on the user's system (Claude Code, OpenCode, Gemini CLI, or Codex)
- Existing `model.StrategyMarkdownSections` — marker-based system prompt injection for SDD integration

## Success Criteria

- [ ] `go test ./...` passes with all new tests (engine, parser, registry, TUI screens)
- [ ] `go vet ./...` reports no issues
- [ ] Full 6-step flow completes end-to-end: select engine → describe → choose SDD mode → generate → preview → install
- [ ] Generated SKILL.md installs to all configured agents' skill directories
- [ ] Custom-agent registry persists across sessions
- [ ] SDD phase-support mode injects marker into system prompt without corrupting existing content
- [ ] Esc navigates back at every step; empty prompt disables Continue
- [ ] TUI remains responsive during generation (spinner animates, cancel works)
