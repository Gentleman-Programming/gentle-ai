# Agent Builder — Full Specification

## Purpose

Define behavior for the 8-screen TUI flow, generation engine interface, skill installer, and custom-agent registry that together allow users to create custom sub-agents from natural language descriptions.

---

## 1. agent-builder-tui

### Requirement: Welcome Menu Entry

The Welcome screen MUST display a "Create your own Agent" option. When no AI agents are installed, the option MUST appear disabled with a "(no agents)" suffix and MUST NOT be selectable.

#### Scenario: Menu entry enabled

- GIVEN at least one AI agent is detected on the system
- WHEN the Welcome screen renders
- THEN "Create your own Agent" appears as a selectable menu item

#### Scenario: Menu entry disabled (no agents)

- GIVEN no AI agent binaries are detected
- WHEN the Welcome screen renders
- THEN "Create your own Agent" appears with "(no agents)" suffix and cannot be selected

---

### Requirement: Engine Selection Screen

The engine selection screen MUST list only AI agents whose binaries are detected. The user MUST be able to select one engine with Enter and navigate back to Welcome with Esc.

#### Scenario: Engine selection happy path

- GIVEN Claude Code and OpenCode are installed
- WHEN the user opens engine selection
- THEN only Claude Code and OpenCode appear as options

#### Scenario: Esc returns to Welcome

- GIVEN the engine selection screen is active
- WHEN the user presses Esc
- THEN the TUI navigates back to the Welcome screen with no state change

---

### Requirement: Prompt Input Screen

The prompt input screen MUST provide a multi-line textarea (charmbracelet/textarea). The Continue button MUST be disabled when the textarea is empty. The screen MUST display example prompts as helper text.

#### Scenario: Empty prompt blocks continuation

- GIVEN the prompt screen is shown
- WHEN the textarea is empty
- THEN the Continue action is disabled and helper text is shown

#### Scenario: Non-empty prompt enables continuation

- GIVEN the user has typed at least one character
- WHEN Continue is activated
- THEN the TUI navigates to the SDD integration screen with the prompt stored in AgentBuilderState

---

### Requirement: SDD Integration Screen

The SDD integration screen MUST offer three radio options: Standalone, New SDD Phase, Support for existing phase. Selecting "New SDD Phase" or "Support for existing phase" MUST navigate to the SDD Phase Picker sub-screen before proceeding.

#### Scenario: Standalone selection skips phase picker

- GIVEN Standalone is selected
- WHEN the user presses Continue
- THEN the TUI navigates directly to the Generating screen

#### Scenario: New SDD Phase triggers phase picker

- GIVEN "New SDD Phase" is selected
- WHEN the user presses Continue
- THEN the TUI navigates to the SDD Phase Picker screen

#### Scenario: Phase support triggers phase picker

- GIVEN "Support for existing phase" is selected
- WHEN the user presses Continue
- THEN the TUI navigates to the SDD Phase Picker screen

---

### Requirement: Generating Screen

The Generating screen MUST run the AI generation in a goroutine with a 120-second context timeout. A spinner MUST animate during generation. The user MUST be able to cancel generation; cancellation returns to the Prompt screen.

#### Scenario: Generation succeeds

- GIVEN generation completes within 120 seconds
- WHEN the AgentBuilderDoneMsg arrives with no error
- THEN the TUI navigates to the Preview screen

#### Scenario: Generation times out

- GIVEN generation exceeds 120 seconds
- WHEN the context deadline is exceeded
- THEN an error message is shown with options to "Retry with longer timeout" (2×) or "Try different engine"

#### Scenario: Generation fails

- GIVEN the engine CLI returns a non-zero exit code
- WHEN the error is received
- THEN the error message and engine's stderr are displayed; the user can Retry or go Back

#### Scenario: TUI remains responsive during generation

- GIVEN generation is in progress
- WHEN the spinner tick fires
- THEN the spinner animation updates without blocking the event loop

---

### Requirement: Preview Screen

The Preview screen MUST show the agent's Name, Description, Trigger, and SDD config as metadata. The full SKILL.md content MUST be displayed in a scrollable pane. The screen MUST provide Install, Edit, Regenerate, and Back actions.

#### Scenario: Preview displays metadata and content

- GIVEN a GeneratedAgent is available
- WHEN the Preview screen renders
- THEN Name, Description, Trigger, SDD integration, and target installation paths are shown

#### Scenario: Edit opens $EDITOR

- GIVEN $EDITOR is set
- WHEN the user selects Edit
- THEN the generated SKILL.md is written to a temp file, $EDITOR is opened, and on close the TUI returns to Preview with updated content

#### Scenario: Edit falls back to vi

- GIVEN $EDITOR is not set and vi is available
- WHEN the user selects Edit
- THEN vi is opened with the skill file

#### Scenario: Regenerate returns to generating

- GIVEN the user selects Regenerate
- WHEN the action fires
- THEN AgentBuilderState retains the current prompt and engine, and the Generating screen restarts

---

### Requirement: Installation and Complete Screens

The Installing screen MUST display per-agent progress. The Complete screen MUST display usage instructions and a Done button that returns to Welcome.

#### Scenario: Multi-agent installation

- GIVEN two agents are configured (Claude Code, OpenCode)
- WHEN Install is confirmed
- THEN both agents show individual status lines (✓ installed or ✗ failed)

#### Scenario: Complete screen shows usage hint

- GIVEN all agents were installed successfully
- WHEN the Complete screen renders
- THEN a usage instruction referencing the agent's trigger is shown, and Done returns to Welcome

---

### Requirement: Esc Navigation at Every Step

At every agent builder screen, pressing Esc MUST navigate to the logically prior screen without resetting the entire AgentBuilderState.

#### Scenario: Esc from Prompt returns to Engine

- GIVEN the Prompt screen is active
- WHEN Esc is pressed
- THEN the TUI shows the Engine screen; previously entered text is preserved in state

---

## 2. generation-engine

### Requirement: GenerationEngine Interface

The system MUST define a `GenerationEngine` interface with three methods: `Agent() model.AgentID`, `Generate(ctx context.Context, prompt string) (string, error)`, and `Available() bool`. Every supported AI CLI MUST have a concrete implementation.

#### Scenario: Available returns false when binary missing

- GIVEN the claude binary is not on PATH
- WHEN `ClaudeEngine.Available()` is called
- THEN it returns false and the engine does not appear on the selection screen

#### Scenario: Generate delegates to CLI subprocess

- GIVEN ClaudeEngine is selected and claude binary exists
- WHEN `Generate(ctx, prompt)` is called
- THEN `exec.CommandContext(ctx, "claude", "--print", "-p", prompt)` is executed and its stdout is returned

#### Scenario: New engine added without core changes

- GIVEN a new AI agent CLI is supported
- WHEN a struct implementing `GenerationEngine` is added
- THEN it can be registered without changes to existing engine code

---

### Requirement: Prompt Composition

The system MUST compose the final prompt from: (1) system prompt with SKILL.md format instructions, (2) user description, (3) SDD context if applicable, (4) installed agents list. The composed prompt MUST instruct the engine to return ONLY the SKILL.md content starting with "# {Name}".

#### Scenario: SDD context appended for phase support

- GIVEN SDDIntegrationMode is `phase-support` with target phase "design"
- WHEN the prompt is composed
- THEN the SDD block references `supports_phase_design` in ADDITIONAL CONTEXT

#### Scenario: Standalone generates no SDD context

- GIVEN SDDIntegrationMode is `standalone`
- WHEN the prompt is composed
- THEN no SDD context block is appended

---

### Requirement: Output Parsing

The parser MUST validate the output contains required sections: Description, Trigger, Instructions. The parser MUST strip code fences and preamble. The parser MUST extract Name (from `# {Title}` → kebab-case), Description, and Trigger into `GeneratedAgent`.

#### Scenario: Valid output parses successfully

- GIVEN the engine returns text starting with "# A11y Reviewer" and containing Description, Trigger, Instructions sections
- WHEN ParseGeneratedAgent is called
- THEN a GeneratedAgent with Name="a11y-reviewer", Title="A11y Reviewer" is returned

#### Scenario: Missing required section fails gracefully

- GIVEN the engine returns text without a "## Trigger" section
- WHEN ParseGeneratedAgent is called
- THEN an error listing missing sections is returned and the Preview screen shows a warning with Edit/Regenerate options

#### Scenario: Code fence stripped

- GIVEN the engine wraps output in triple-backtick fences
- WHEN ParseGeneratedAgent is called
- THEN the fences are removed and Content contains only the SKILL.md text

---

## 3. custom-agent-registry

### Requirement: Registry Persistence

The system MUST read and write a JSON registry at `~/.config/gentle-ai/custom-agents.json`. The registry MUST include a `version` field (integer). Each entry MUST record: name, title, description, created_at (RFC3339), generation_engine, sdd_integration (nullable), installed_agents.

#### Scenario: Registry created on first install

- GIVEN no registry file exists
- WHEN the first custom agent is installed
- THEN the registry file is created with `"version": 1` and the agent entry

#### Scenario: Registry updated on subsequent install

- GIVEN the registry contains one entry
- WHEN a second custom agent is installed
- THEN the new entry is appended and the version field is unchanged

#### Scenario: Registry version field preserved on update

- GIVEN a registry with `"version": 1`
- WHEN an agent is added
- THEN `"version"` remains 1

---

### Requirement: Skill Name Conflict Resolution

When the generated agent name conflicts with a built-in skill in the catalog, the system MUST append `-custom` and warn the user. When the name conflicts with an existing custom agent in the registry, the system MUST ask the user "Agent '{name}' already exists. Replace it?" before overwriting.

#### Scenario: Built-in name conflict

- GIVEN the generated name is "typescript" which exists in the built-in catalog
- WHEN the name is resolved
- THEN it becomes "typescript-custom" and a warning is shown on the Preview screen

#### Scenario: Custom agent name conflict

- GIVEN the registry already contains an agent named "a11y-reviewer"
- WHEN the Preview screen shows
- THEN a confirmation dialog asks the user whether to replace the existing agent

---

## 4. skill-installer

### Requirement: Cross-Agent Installation

The installer MUST write the SKILL.md to every configured agent's `SkillsDir()`. The installer MUST detect configured agents by checking if each agent's skill directory exists on disk. If a skill directory does not exist, the installer MUST create it.

#### Scenario: Skill installed to all configured agents

- GIVEN Claude Code and OpenCode have existing skill directories
- WHEN Install is executed
- THEN SKILL.md is written to `~/.claude/skills/{name}/SKILL.md` and `~/.config/opencode/skills/{name}/SKILL.md`

#### Scenario: Missing skill directory is created

- GIVEN an agent's skill directory does not exist
- WHEN Install runs for that agent
- THEN the directory is created and SKILL.md is written

---

### Requirement: Atomic Installation

Installation MUST be atomic. If writing to any agent fails after partial success, all already-written SKILL.md files MUST be removed before the error is surfaced.

#### Scenario: Rollback on partial failure

- GIVEN Claude Code install succeeds but OpenCode install fails
- WHEN the error is detected
- THEN the Claude Code SKILL.md is deleted and an error is shown listing the failed agent

---

### Requirement: SDD Phase Support Injection

For `phase-support` mode, the installer MUST inject a marker-fenced block into the agent's system prompt file using the `StrategyMarkdownSections` approach. The marker MUST follow the format `<!-- gentle-ai:custom-agent:{name} -->`.

#### Scenario: Phase support marker injected

- GIVEN SDDIntegrationMode is `phase-support` targeting "design"
- WHEN installation completes
- THEN the agent system prompt contains the marker block referencing the custom skill

#### Scenario: Existing marker not duplicated

- GIVEN the system prompt already contains `<!-- gentle-ai:custom-agent:a11y-reviewer -->`
- WHEN installation runs again (replace scenario)
- THEN the existing block is replaced, not duplicated

---

### Requirement: SDD New Phase Injection

For `new-phase` mode, the installer MUST inject a marker block that updates the orchestrator's dependency graph in the system prompt to include the new phase at the specified position.

#### Scenario: New phase inserted after "design"

- GIVEN SDDIntegrationMode is `new-phase` and TargetPhase is "design"
- WHEN installation completes
- THEN the system prompt dependency graph reads `... → design → {phase-name} → tasks → ...`

---

## Interface Contracts

### GenerationEngine Interface

```
Agent()    model.AgentID
Generate(ctx context.Context, prompt string) (string, error)
Available() bool
```

Implementations: ClaudeEngine (`claude --print -p`), OpenCodeEngine (`opencode run`), GeminiEngine (`gemini -p`), CodexEngine (`codex exec`).

### GeneratedAgent Struct

| Field       | Type             | Description                        |
|-------------|------------------|------------------------------------|
| Name        | string           | kebab-case directory name          |
| Title       | string           | Human-readable title from `# ...`  |
| Description | string           | From `## Description` section      |
| Trigger     | string           | From `## Trigger` section          |
| Content     | string           | Full SKILL.md text                 |
| SDDConfig   | *SDDIntegration  | nil when standalone                |

### SDDIntegration Struct

| Field       | Type                | Values                                  |
|-------------|---------------------|-----------------------------------------|
| Mode        | SDDIntegrationMode  | standalone \| new-phase \| phase-support |
| TargetPhase | string              | e.g. "design"                           |
| PhaseName   | string              | kebab-case new phase name               |

### AgentBuilderState Struct

| Field            | Type               | Description                     |
|------------------|--------------------|---------------------------------|
| Engine           | model.AgentID      | Selected generation engine      |
| AvailableEngines | []model.AgentID    | Detected engines                |
| PromptText       | string             | User's natural language input   |
| SDDMode          | SDDIntegrationMode | Chosen SDD mode                 |
| SDDTargetPhase   | string             | Phase chosen in phase picker    |
| Generating       | bool               | True during goroutine execution |
| GenerationErr    | error              | Last generation error           |
| Generated        | *GeneratedAgent    | Parsed generation result        |
| PreviewScroll    | int                | Scroll offset in preview        |
| Installing       | bool               | True during installation        |
| InstallResult    | *InstallResult     | Per-agent install outcomes      |

### Custom Registry JSON Schema

```
{
  "version": 1,          // integer — schema version for forward compat
  "agents": [
    {
      "name":               string,   // kebab-case
      "title":              string,   // human-readable
      "description":        string,
      "created_at":         string,   // RFC3339
      "generation_engine":  string,   // AgentID value
      "sdd_integration": {            // null if standalone
        "mode":         string,       // standalone|new-phase|phase-support
        "target_phase": string
      },
      "installed_agents": [string]    // AgentID values
    }
  ]
}
```

---

## Screen–Constant Mapping

| Constant                      | Route                  | Back                   |
|-------------------------------|------------------------|------------------------|
| ScreenAgentBuilderEngine      | step 1                 | ScreenWelcome          |
| ScreenAgentBuilderPrompt      | step 2                 | ScreenAgentBuilderEngine |
| ScreenAgentBuilderSDD         | step 3                 | ScreenAgentBuilderPrompt |
| ScreenAgentBuilderSDDPhase    | step 3b                | ScreenAgentBuilderSDD  |
| ScreenAgentBuilderGenerating  | step 4                 | ScreenAgentBuilderPrompt |
| ScreenAgentBuilderPreview     | step 5                 | ScreenAgentBuilderPrompt |
| ScreenAgentBuilderInstalling  | step 6                 | ScreenAgentBuilderPreview |
| ScreenAgentBuilderComplete    | step 7                 | ScreenWelcome          |
