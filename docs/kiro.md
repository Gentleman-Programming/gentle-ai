# Kiro IDE

> [!NOTE]
> These docs track `main`, which may include unreleased changes. For the latest release, see the [v4.0.0 docs](https://github.com/Gentleman-Programming/gentle-ai/tree/v4.0.0/docs).

← [Back to README](../README.md)

---

This document explains how gentle-ai integrates with **Kiro IDE** and what is installed in your local Kiro configuration.

## Overview

gentle-ai supports Kiro as a **native-subagent** platform (`kiro-ide`).

When configured, gentle-ai installs:

| Artifact | Path |
|----------|------|
| Steering file | `~/.kiro/steering/gentle-ai.md` |
| Native Judgment Day agents | `~/.kiro/agents/jd-fix-agent.md`, `jd-judge-a.md`, `jd-judge-b.md` *(3 files)* |
| Skills directory | `~/.kiro/skills/` |
| MCP config | `~/.kiro/settings/mcp.json` *(separate root — see note below)* |

> **Auto-install not supported.** Kiro must be installed manually before running gentle-ai.
> Download from: [kiro.dev/downloads](https://kiro.dev/downloads)

---

## Detection

gentle-ai uses **two signals** to detect Kiro:

1. **`~/.kiro` directory presence** — used by `system.ScanConfigs` for the install/TUI auto-detection flow. If `~/.kiro` exists on disk, Kiro is shown as detected in the installer, regardless of whether the binary is on `PATH`.
2. **`kiro` binary on `PATH`** — used by `adapter.Detect()` for the sync/upgrade flow and to confirm the IDE is actually runnable.

In practice: **the installer detects Kiro from `~/.kiro`**, not from `PATH`. If you have Kiro installed but `~/.kiro` hasn't been created yet (e.g., before first launch), run Kiro once to initialize its config dir, then re-run `gentle-ai install`.

---

## ODD Execution Model

> **Since v4.0.0:** SDD (Spec-Driven Development) is retired in favor of [ODD](usage.md#organic-driven-development-odd). gentle-ai no longer installs `sdd-*` Kiro agents and no longer routes work through `.kiro/specs/`. A global `gentle-ai install` or `gentle-ai sync` removes the `sdd-*` agent files earlier releases installed in `~/.kiro/agents/` when their content matches what a release wrote (model choices and Gentle AI managed blocks may differ). A file with any other change is kept and listed under manual actions: move or delete it yourself. Removed files are part of the sync backup, so `gentle-ai restore` brings them back.

Kiro runs with **native sub-agent delegation** via `~/.kiro/agents/`.

The ODD orchestrator stays in the steering file. It keeps work inline by default and delegates to Kiro's native subagents only for a named reason: an exploration map, parallel writers on disjoint edit surfaces, the context backstop, or independent verification of a high-risk change. Engram™ provides cross-session persistence when available.

The `jd-*` agents run the [Judgment Day](components.md#skills) adversarial review: two blind judges and one fix agent.

---

## Steering Files

**Steering files** at `.kiro/steering/*.md` provide persistent workspace context across sessions — treat them like always-on system context for your project conventions, architecture decisions, and team rules.

---

## Steering File Format

The steering file written by gentle-ai uses the following frontmatter:

```yaml
---
inclusion: always
---
```

`inclusion: always` ensures Kiro loads this context in every conversation automatically, regardless of workspace or file type.

## Native Agent Frontmatter

Kiro Judgment Day agents are generated with YAML frontmatter including:

- `name`
- `description`
- `tools`
- `model`
- `includeMcpJson: true`

The `model` value is injected during sync from Kiro model assignments, keyed by agent name or `default`, and resolved to a Kiro-native model ID:

| Alias | Kiro model ID |
|-------|---------------|
| `auto` | `auto` |
| `opus` | `claude-opus-5.5` |
| `sonnet` | `claude-sonnet-5.5` |
| `haiku` | `claude-haiku-4.5` |
| `luna` | `gpt-5.6-luna` |
| `terra` | `gpt-5.6-terra` |
| `sol` | `gpt-5.6-sol` |
| `minimax` | `minimax-m2.5` |
| `glm` | `glm-5` |
| `deepseek` | `deepseek-3.2` |
| `qwen` | `qwen3-coder-next` |

The model presets never assign GPT-5.6 Sol or Terra; both stay available in the custom picker.

| Preset | Judgment Day judges A / B | Fix agent | Default |
|--------|---------------------------|-----------|---------|
| `balanced` | Sonnet 5.5 / Luna | Sonnet 5.5 | Auto |
| `performance` | Opus 5.5 / Opus 5.5 | Sonnet 5.5 | Sonnet 5.5 |
| `economy` | Luna / Luna | Luna | Luna |
| `open-weight` | GLM-5 / Qwen3 Coder Next | Qwen3 Coder Next | Qwen3 Coder Next |

---

## Config Paths by Platform

### macOS

| Artifact | Path |
|----------|------|
| Global config dir | `~/Library/Application Support/Kiro/User` |
| Steering file | `~/.kiro/steering/gentle-ai.md` |
| Skills dir | `~/.kiro/skills/` |
| Settings path | `~/Library/Application Support/Kiro/User/settings.json` |
| MCP config | `~/.kiro/settings/mcp.json` |

### Windows

| Artifact | Path |
|----------|------|
| Global config dir | `%APPDATA%\kiro\User` |
| Steering file | `%USERPROFILE%\.kiro\steering\gentle-ai.md` |
| Skills dir | `%USERPROFILE%\.kiro\skills\` |
| Settings path | `%APPDATA%\kiro\User\settings.json` |
| MCP config | `%USERPROFILE%\.kiro\settings\mcp.json` |

### Linux (XDG)

| Artifact | Path |
|----------|------|
| Global config dir | `$XDG_CONFIG_HOME/kiro/user` *(fallback: `~/.config/kiro/user`)* |
| Steering file | `~/.kiro/steering/gentle-ai.md` |
| Skills dir | `~/.kiro/skills/` |
| Settings path | `$XDG_CONFIG_HOME/kiro/user/settings.json` |
| MCP config | `~/.kiro/settings/mcp.json` |

---

## ⚠️ Split-Root Layout

Kiro uses a **split-root layout** — gentle-ai managed files and IDE settings live in different directories:

- **Steering, skills, and native agents** → `~/.kiro/` (or `%USERPROFILE%\.kiro\` on Windows)
  - `~/.kiro/steering/gentle-ai.md` — orchestrator persona
  - `~/.kiro/skills/` — gentle-ai skill files
  - `~/.kiro/agents/` — Judgment Day subagents
- **IDE settings** → platform-native Kiro User dir (`settings.json` only)
  - macOS: `~/Library/Application Support/Kiro/User/settings.json`
  - Windows: `%APPDATA%\kiro\User\settings.json`
  - Linux: `$XDG_CONFIG_HOME/kiro/user/settings.json`
- **MCP config** → always `~/.kiro/settings/mcp.json` (or `%USERPROFILE%\.kiro\settings\mcp.json` on Windows)

If MCP tools are not loading, check `~/.kiro/settings/mcp.json`.  
If Kiro app settings are not applying, check the platform-native User dir (`settings.json`).  
If gentle-ai skills or steering are missing, check `~/.kiro/skills/` and `~/.kiro/steering/`.

---

## Capability Snapshot

| Capability | Status |
|------------|--------|
| Skills | ✅ Yes |
| System prompt | ✅ Yes |
| MCP | ✅ Yes |
| Output styles | ❌ No |
| Slash commands | ❌ No |
| Delegation model | Full (native subagents) |
| Auto-install | ❌ No — manual install required |
