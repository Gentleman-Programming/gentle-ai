# Supported Agents

← [Back to README](../README.md)

---

| Agent | ID | Skills | MCP | Sub-agents | Output Styles | Slash Commands | Config Path |
|-------|-----|--------|-----|------------|---------------|----------------|-------------|
| Claude Code | `claude-code` | Yes | Yes | Yes | Yes | No | `~/.claude` |
| OpenCode | `opencode` | Yes | Yes | Yes | No | Yes | `~/.config/opencode` |
| Gemini CLI | `gemini-cli` | Yes | Yes | Yes (experimental) | No | No | `~/.gemini` |
| Cursor | `cursor` | Yes | Yes | Yes | No | No | `~/.cursor` |
| VS Code Copilot | `vscode-copilot` | Yes | Yes | Yes | No | No | `~/.copilot` + VS Code User profile |
| Antigravity | `antigravity` | Yes | Yes | Yes | No | No | `~/.gemini/antigravity` |

All agents receive the **full SDD orchestrator** (agent-teams-lite) injected into their system prompt, plus skill files written to their skills directory. Every agent supports sub-agent delegation natively, enabling the full SDD orchestration workflow with parallel sub-agents.

## Notes

- **Gemini CLI** sub-agents are experimental and require `experimental.enableAgents: true` in `settings.json`. Custom sub-agents are defined as markdown files in `~/.gemini/agents/`.
- **Cursor** supports async sub-agents (v2.5+) that can run in background and spawn nested sub-agent trees.
- **VS Code Copilot** uses the `runSubagent` tool with support for parallel execution and custom agent definitions.
- **Output Styles** are currently a Claude Code exclusive feature (`~/.claude/output-styles/`).
- **Slash Commands** are currently supported by OpenCode only.
- **VS Code Copilot** stores skills under `~/.copilot/skills/` (global), system prompt under `Code/User/prompts/gentle-ai.instructions.md`, and MCP config under `Code/User/mcp.json`.
- **Antigravity** stores skills under `~/.gemini/antigravity/skills/`, MCP config under `~/.gemini/antigravity/mcp_config.json`. The global rules path is pending confirmation (`~/.gemini/GEMINI.md` is the documented path but collides with Gemini CLI when both are installed — see [#69](https://github.com/Gentleman-Programming/gentle-ai/issues/69)). Settings are managed via the IDE's Agent settings UI (Artifact Review Policy / Terminal Command Auto Execution) — not via `settings.json`.
- **Antigravity sub-agent support**: Antigravity currently executes all tasks as a single agent. The SDD orchestrator is adapted accordingly (sequential execution, no delegation). Native sub-agent invocation from within the agent is on Antigravity's roadmap (similar to Cursor's model) but not yet available. Tracked in [#69](https://github.com/Gentleman-Programming/gentle-ai/issues/69).
