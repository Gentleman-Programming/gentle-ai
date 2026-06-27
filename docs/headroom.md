# Headroom — Context Compression MCP Server

← [Back to README](../README.md) · [Components & Presets](components.md)

---

Headroom is an MCP server that compresses verbose content — tool outputs, LLM responses, file contents — to help you stay within context window limits without losing information you might need later.

## The Problem

AI coding agents produce enormous tool outputs: directory listings, file reads, git diffs, search results, linter output, test logs. Each one eats into your context window. When the window fills, the agent starts forgetting earlier instructions, decisions, or code context.

Engram solves the **cross-session** memory problem. Headroom solves the **within-session** context pressure problem. They are complementary.

## How It Works

Headroom exposes three MCP tools that the agent can call on demand:

| Tool | What it does |
|------|-------------|
| `headroom_compress` | Compresses a text block and returns a short reference handle |
| `headroom_retrieve` | Decompresses a reference handle back to full text |
| `headroom_stats` | Reports compression ratios and total savings for the session |

The agent decides when to compress — typically when it notices the context is getting full or when a tool output is unusually large. Compressed content is stored locally; nothing leaves your machine.

## Installation

Headroom is installed automatically by `gentle-ai` when you select it as a component:

### Via TUI

```
gentle-ai install
```

Select `Headroom` in the components screen (included by default in the `full-gentleman` and `ecosystem-only` presets).

### Via CLI

```bash
gentle-ai install --component headroom
```

The installer will:
1. Check if `headroom` is already on your PATH
2. If not, find `pip` or `pip3` and run `pip install "headroom-ai[all]"`
3. Inject MCP configuration into all your configured agents

### Manual Installation

If you prefer to install Headroom yourself:

```bash
pip install "headroom-ai[all]"
```

Then run `gentle-ai install --component headroom` to configure your agents, or `gentle-ai sync` if Headroom is already in your selection.

## What Gets Configured

For each agent you have installed, gentle-ai writes the correct MCP config:

| Agent | Config File | Format |
|-------|-------------|--------|
| Claude Code | `~/.claude/mcp/headroom.json` | Separate file |
| OpenCode / Kilo Code | `opencode.json` `mcp.headroom` | Merge overlay |
| Cursor | `~/.cursor/mcp.json` `mcpServers.headroom` | Merge |
| VS Code Copilot | `Code/User/mcp.json` `servers.headroom` | Merge |
| Windsurf | `~/.codeium/windsurf/mcp_config.json` | Merge |
| Codex | `~/.codex/config.toml` `[mcp_servers.headroom]` | TOML block |
| Gemini CLI | `~/.gemini/settings.json` `mcpServers.headroom` | Merge |
| Antigravity | `~/.gemini/antigravity/mcp_config.json` | Merge |
| Kimi Code | `~/.kimi/settings.json` | Merge |
| Qwen Code | `~/.qwen/settings.json` | Merge |
| Kiro IDE | `~/.kiro/settings/mcp.json` | Merge |
| OpenClaw | `~/.openclaw/openclaw.json` | Merge |
| Trae | App support dir `mcp.json` | Merge |
| Pi | `.pi/settings.json` | Merge |
| Hermes | `~/.hermes/config.yaml` | YAML block |

All entries point to a local `headroom` process with `args: ["mcp", "serve"]`.

## Uninstalling

```bash
gentle-ai uninstall --component headroom
```

This removes all MCP config entries from your agents. To also remove the pip package:

```bash
pip uninstall headroom-ai
```

## Usage Tips

- Headroom is most useful during long coding sessions where the agent performs many read/search/write operations
- The agent calls `headroom_compress` automatically when it detects large outputs — you don't need to interact with it directly
- Check savings with `headroom_stats` if you're curious how much context you've recovered
- If the agent seems to forget earlier context, try asking: "check your context usage with headroom_stats and compress anything you don't need right now"

## Comparison

| | Engram | Headroom |
|--|--------|----------|
| What | Cross-session persistent memory | Within-session context compression |
| When | Between coding sessions | During a long session |
| How | FTS5 search + save/recall | Compress/decompress large text blocks |
| Analogy | Your project notebook | Vacuum packing bulky items in your current workspace |

## References

- [Headroom GitHub](https://github.com/headroomlabs-ai/headroom)
- [Headroom PyPI](https://pypi.org/project/headroom-ai/)
- [Components & Presets Reference](components.md)
