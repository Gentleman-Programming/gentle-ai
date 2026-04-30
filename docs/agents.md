# Supported Agents

← [Back to README](../README.md)

---

## Agent Matrix

| Agent           | ID               | Skills       | MCP | Delegation                   | Output Styles | Slash Commands | Config Path                         |
| --------------- | ---------------- | ------------ | --- | ---------------------------- | ------------- | -------------- | ----------------------------------- |
| Claude Code     | `claude-code`    | Yes          | Yes | Full (Task tool)             | Yes           | No             | `~/.claude`                         |
| OpenCode        | `opencode`       | Yes          | Yes | Full (multi-mode overlay)    | No            | Yes            | `~/.config/opencode`                |
| Gemini CLI      | `gemini-cli`     | Yes          | Yes | Full (experimental)          | No            | No             | `~/.gemini`                         |
| Cursor          | `cursor`         | Yes          | Yes | Full (native subagents)      | No            | No             | `~/.cursor`                         |
| VS Code Copilot | `vscode-copilot` | Yes          | Yes | Full (runSubagent)           | No            | No             | `~/.copilot` + VS Code User profile |
| Codex           | `codex`          | Yes          | Yes | Solo-agent                   | No            | No             | `~/.codex`                          |
| Windsurf        | `windsurf`       | Yes (native) | Yes | Solo-agent                   | No            | No             | `~/.codeium/windsurf`               |
| Antigravity     | `antigravity`    | Yes (native) | Yes | Solo-agent + Mission Control | No            | No             | `~/.gemini/antigravity`             |
| Kimi            | `kimi`           | Yes          | Yes | Full (native custom agents)  | No            | No             | `~/.kimi`                           |
| Qwen Code       | `qwen-code`      | Yes          | Yes | Full (native sub-agents)     | No            | Yes            | `~/.qwen`                           |
| Kiro IDE        | `kiro-ide`       | Yes          | Yes | Full (native subagents)      | No            | No             | `~/.kiro`                           |

All agents receive the **full SDD orchestrator** injected into their system prompt, plus skill files written to their skills directory. The agent handles SDD automatically when the task is large enough, or when the user explicitly asks for it — no manual setup required.

---

## Delegation Models

| Model                 | How It Works                                                                                                                         | Agents                                                           |
| --------------------- | ------------------------------------------------------------------------------------------------------------------------------------ | ---------------------------------------------------------------- |
| **Full (sub-agents)** | Each SDD phase runs in an isolated context window via native sub-agent delegation. The orchestrator coordinates; sub-agents execute. | Claude Code, OpenCode, Gemini CLI, Cursor, VS Code Copilot, Kimi, Kiro IDE, Qwen Code |
| **Solo-agent**        | All SDD phases run inline in the same conversation. The orchestrator IS the executor. Engram provides cross-phase persistence.       | Codex, Windsurf, Antigravity                                     |

### Cursor Native Subagents

Cursor uses its built-in `.cursor/agents/` system. `gentle-ai` writes 10 agent files to `~/.cursor/agents/sdd-{phase}.md` — one per SDD phase. Cursor's Agent auto-delegates to the correct subagent based on the `description` field in each file's YAML frontmatter.

- `sdd-explore` and `sdd-verify` run with `readonly: true`
- Each subagent gets its own context window (fresh context, no pollution)
- The orchestrator resolves compact rules from the skill registry and passes them in the invocation message

### Windsurf Cascade

Windsurf runs as a solo-agent (no custom sub-agents). The orchestrator leverages Windsurf-native features:

- **Plan Mode** — creates persistent plan documents that can be @mentioned across sessions; ideal for spec and design artifacts on large changes
- **Code Mode** — default agentic execution mode
- **Native Workflows** — `sdd-new` is available as a `.windsurf/workflows/sdd-new.md` workflow
- **Size Classification** — the orchestrator routes tasks through Small/Medium/Large decision paths

### Antigravity + Mission Control

Antigravity is an agent-first platform with built-in sub-agents (Browser, Terminal) managed by Mission Control. However, custom sub-agent creation is not yet available. SDD phases run inline, with Mission Control handling automatic delegation to built-in sub-agents when specialized tooling is needed (e.g., Browser for research during `sdd-explore`).

### Kiro Native Subagents

Kiro uses native custom agents in `~/.kiro/agents/`. `gentle-ai` writes 10 phase agents (`sdd-init` through `sdd-onboard`) and resolves the `model:` field during injection from Claude alias assignments (`opus|sonnet|haiku`) to Kiro-native model IDs.

- Frontmatter includes `includeMcpJson: true` for all phase agents
- Phase-specific tools are preserved (`sdd-explore` and `sdd-verify` use read/shell/context7 as required)
- Orchestrator remains in steering (`~/.kiro/steering/gentle-ai.md`) and delegates execution to native subagents

---

## SDD Mode Support

| Feature | Claude Code | OpenCode | Gemini CLI | Cursor | VS Code Copilot | Codex | Windsurf | Antigravity | Kiro IDE | Qwen Code |
|---------|:-----------:|:--------:|:----------:|:------:|:---------------:|:-----:|:--------:|:-----------:|:--------:|:---------:|
| SDD orchestrator | Yes | Yes | Yes | Yes | Yes | Yes | Yes | Yes | Yes | Yes |
| Single-mode SDD | Yes | Yes | Yes | Yes | Yes | Yes | Yes | Yes | Yes | Yes |
| Multi-mode SDD | — | Yes | — | — | — | — | — | — | Yes* | — |

**Multi-mode** (assigning different AI models to each SDD phase) is natively supported by **OpenCode** (via its provider system) and **Kiro IDE** (via native subagent `model:` frontmatter — each phase agent runs with its own model ID). All other agents run in **single-mode** — the orchestrator manages everything using whatever model the agent is already running.

> \* **Kiro multi-mode** assigns models per phase through `KiroModelAssignments` (configured via *Configure Models → Configure Kiro models* in the TUI). The selected alias (`opus|sonnet|haiku`) is resolved to a Kiro-native model ID and stamped into each `~/.kiro/agents/sdd-{phase}.md` at sync time.

---

## Agent Notes

### Claude Code

- Sub-agents via the native Task tool with isolated context windows
- MCP servers configured as plugins in `~/.claude/mcp/`
- Output styles in `~/.claude/output-styles/`
- System prompt via markdown sections in `~/.claude/CLAUDE.md`

### OpenCode

- Full multi-agent overlay with 12 named agents in `opencode.json`
- Slash commands for SDD phases (`/sdd-new`, `/sdd-explore`, etc.)
- Background-agents plugin for parallel execution
- Multi-mode prerequisite: connect your AI providers first, then run `opencode models --refresh`

### Gemini CLI

- Sub-agents are experimental: require `experimental.enableAgents: true` in `settings.json`
- Custom sub-agents defined as markdown files in `~/.gemini/agents/`

### Cursor
- Native subagents via `~/.cursor/agents/sdd-{phase}.md` (10 files installed by gentle-ai)
- Skills at `~/.cursor/skills/`
- System prompt in `~/.cursor/rules/gentle-ai.mdc`
- MCP config in `~/.cursor/mcp.json`

### VS Code Copilot

- Uses the `runSubagent` tool with support for parallel execution
- Skills at `~/.copilot/skills/`
- System prompt at `Code/User/prompts/gentle-ai.instructions.md`
- MCP config at `Code/User/mcp.json`

### Codex

- CLI-native agent with TOML config at `~/.codex/config.toml`
- Skills at `~/.codex/skills/`
- System prompt at `~/.codex/agents.md`
- Engram instruction files at `~/.codex/engram-instructions.md`

### Windsurf

- Skills at `~/.codeium/windsurf/skills/` (native Windsurf feature)
- MCP config at `~/.codeium/windsurf/mcp_config.json`
- Global rules at `~/.codeium/windsurf/memories/global_rules.md`
- Workflows at `.windsurf/workflows/` (workspace-scoped)

### Antigravity

- Skills at `~/.gemini/antigravity/skills/` (native Antigravity feature)
- MCP config at `~/.gemini/antigravity/mcp_config.json`
- System prompt appended to `~/.gemini/GEMINI.md` (shared with Gemini CLI — collision check warns if both are installed)
- Mission Control handles built-in sub-agent delegation (Browser, Terminal) automatically
- Settings managed via the IDE's Agent settings UI, not via `settings.json`

### Kimi

- Installation requires the `uv` Python package manager (`uv tool install kimi-cli`).
- Root custom agent at `~/.kimi/agents/gentleman.yaml` with `system_prompt_path: ../KIMI.md`
- `KIMI.md` is a thin Jinja template that includes modular prompt files:
  `persona.md`, `output-style.md`, `engram-protocol.md`, `sdd-orchestrator.md`
- Built-in Kimi variables are preserved in `KIMI.md`: `${KIMI_AGENTS_MD}` and `${KIMI_SKILLS}`

### Kiro IDE

- **Detection**: gentle-ai detects Kiro from its config root (`~/.kiro`) during install/TUI discovery — `~/.kiro` must exist (created on first Kiro launch). `kiro` on `PATH` is also checked for sync/upgrade flows but is not required for install auto-detection
- **Steering file** (all platforms): `~/.kiro/steering/gentle-ai.md` with frontmatter `inclusion: always`
- Native subagents at `~/.kiro/agents/sdd-{phase}.md` (10 files)
- Skills (all platforms) at `~/.kiro/skills/`
- **MCP config at a separate root** — always `~/.kiro/settings/mcp.json` (macOS/Linux) or `%USERPROFILE%\.kiro\settings\mcp.json` (Windows), regardless of GlobalConfigDir
- Native Kiro specs workflow: `.kiro/specs/<feature>/requirements.md`, `design.md`, `tasks.md` — with approval gates before apply and archive phases
- Manual install only — download from [kiro.dev/downloads](https://kiro.dev/downloads)
- See [docs/kiro.md](kiro.md) for full path reference and SDD behavior details

### Qwen Code
- **Detection**: gentle-ai detects Qwen Code from its config root (`~/.qwen`) and checks for `qwen` binary on `PATH`
- **Config root**: `~/.qwen/` (cross-platform)
- **System prompt**: `~/.qwen/QWEN.md` (managed via `StrategyFileReplace`)
- **Skills**: `~/.qwen/skills/`
- **MCP config**: `~/.qwen/settings.json` (managed via `StrategyMergeIntoSettings` with `mcpServers` key)
- **Slash commands**: `~/.qwen/commands/*.md` — supports custom namespaced slash commands (e.g., `commands/sdd/init.md` → `/sdd:init`)
- **Permissions**: `auto_edit` mode — auto-approves file edits, manual approval for shell commands
- **Install**: via npm — `npm install -g @qwen-code/qwen-code@latest`
- **Engram slug**: `"qwen-code"` for `engram setup` integration
- **SDD orchestrator**: `internal/assets/qwen/sdd-orchestrator.md` with Qwen-specific path references
