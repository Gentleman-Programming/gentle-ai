# Supported Agents

← [Back to README](../README.md)

Gentle AI configures agents you already have; it does not install an AI agent for you. ODD is the development workflow. Capabilities and delegation primitives depend on the client. [Usage](usage.md) covers installation and sync; [Pi](pi.md) explains the separately owned Pi integration.

| Agent | ID | Integration notes |
| --- | --- | --- |
| <a id="claude-code"></a>Claude Code | `claude-code` | Task subagents, MCP, skills, output styles, skill-registry and review hooks |
| <a id="opencode"></a>OpenCode | `opencode` | Native task subagents, skills, MCP, managed review plugins and optional background policy |
| <a id="kilo-code"></a>Kilo Code | `kilocode` | OpenCode-compatible configuration, skills and MCP |
| <a id="gemini-cli"></a>Gemini CLI | `gemini-cli` | Experimental native agents, skills and MCP |
| <a id="cursor"></a>Cursor | `cursor` | Native subagents, rules, skills and MCP |
| <a id="vs-code-copilot"></a>VS Code Copilot | `vscode-copilot` | `runSubagent`, user instructions, skills and MCP |
| <a id="codex"></a>Codex | `codex` | Native multi-agent when available, solo fallback, skills and MCP |
| <a id="windsurf"></a>Windsurf | `windsurf` | Native skills, global rules and MCP |
| <a id="antigravity"></a>Antigravity | `antigravity` | Native skills, MCP and built-in Mission Control tools |
| <a id="kimi-code"></a>Kimi Code | `kimi` | Native custom agents, skills and MCP |
| <a id="qwen-code"></a>Qwen Code | `qwen-code` | Native subagents, skills and MCP |
| <a id="kiro-ide"></a>Kiro IDE | `kiro-ide` | Native agents, steering, skills and MCP |
| <a id="openclaw"></a>OpenClaw | `openclaw` | Workspace instructions, skills and global MCP config |
| <a id="trae"></a>Trae | `trae-ide` | User rules, skills and MCP |
| <a id="pi"></a>Pi | `pi` | Package-owned runtime through Gentle Shell; see [Pi](pi.md) |
| <a id="hermes"></a>Hermes | `hermes` | Ephemeral `delegate_task` workers, skills and MCP |

## Agent guidance and ownership

Gentle AI installs or refreshes managed guidance, skills, and supported review agents for selected clients. It does not own Pi's runtime prompts or child-agent lifecycle: the separately installed Gentle Shell (`gentle-pi`) package does. Installing a single agent merges it into the recorded installed-agent selection; sync normally refreshes only that selection. `gentle-ai install --scope=workspace` places supported agent-scoped files in the project instead of the global agent configuration, while global-only settings remain global.

Delegated work stays focused: the parent supplies task context and exact relevant skill paths, then checks the outcome. A client without native delegation can perform smaller work inline; native availability does not grant permission for extra writes. Hermes workers require explicit mission context, tools and skills rather than assuming inheritance. Codex multi-agent execution depends on native tools and configuration. Review agents are separate from ordinary ODD implementation agents; RDD runs only under the user-owned switch and candidate consent when applicable.

## Client-specific details

- **Claude Code:** native Task subagents, skill-registry startup hook, and managed review/telemetry hooks. Review authority remains native, not a hook-generated approval.
- **OpenCode:** model discovery uses the active project's providers and tools. Optional background jobs are process-local and non-durable; do not use them for dependent work or parallel writers in one worktree. Managed launchers respect explicit background-off preferences; other launch paths use foreground fallback. See [OpenCode background subagents](opencode-profiles.md).
- **Kilo Code:** uses an OpenCode-compatible adapter; do not assume OpenCode runtime-specific guarantees automatically apply to Kilo.
- **Kimi Code:** `KIMI.md` includes its persona/output-style modules; no Claude-style `settings.json` output-style mechanism is assumed.
- **OpenClaw:** reads the active workspace from its configuration and writes managed `AGENTS.md`/`SOUL.md` there. MCP entries remain in global OpenClaw configuration.
- **Hermes:** detected from its configuration directory; installation of the client itself is manual. Existing top-level configuration is preserved when MCP entries are merged.
- **Pi:** the installer provisions companion packages, but Gentle Shell owns runtime prompts, model assignments, persona, and delegation. See [Pi integration](pi.md).

Model assignment is client-specific and applies to supported generic and review roles, not a formal development-phase matrix. Use the TUI **Configure Models** screen to inspect available roles, including Judgment Day review roles. Strict TDD is an independently configured ODD mode, not inferred from the presence of tests. Run `gentle-ai doctor` for read-only installation diagnostics and `gentle-ai sync --dry-run` to preview managed updates. Uninstall previews and backs up managed configuration; it must preserve unrelated user files. [Full CLI guidance](usage.md#cli-commands).
