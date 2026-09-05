# Gentle-AI™ Codebase Guide

This guide helps maintainers find the right code path before changing Gentle-AI. It is an index, not a full API reference.

## Who this is for

| Reader | Use this guide to |
|---|---|
| New maintainer | Build the project mental model before touching files. |
| Contributor | Find the package that owns a behavior. |
| Reviewer | Check whether a PR changes the right layer. |
| Release helper | Verify sync, install, update, and docs boundaries. |

## 90-second mental model

Gentle-AI is a Go CLI/TUI that configures AI coding agents. It installs and syncs managed assets such as SDD prompts, skills, MCP entries, permissions, personas, GGA support, Engram™ wiring, skill registries, and community tool/plugin helpers.

```text
User
  |
  v
gentle-ai CLI / Bubbletea TUI
  |
  +--> detection + flag normalization
  +--> planner resolves components and dependencies
  +--> pipeline applies adapter/component steps with backups
  +--> verification reports readiness
  |
  v
Agent config roots (~/.claude, ~/.config/opencode, ~/.cursor, ...)
  |
  v
External tools and agents: Engram, Context7, GGA, supported AI CLIs/IDEs
```

Golden rule: **agent-specific paths belong in adapters; reusable behavior belongs in components or shared orchestration packages.**

## Guide pages

| Page | Job |
|---|---|
| [Mental model](codebase/mental-model.md) | Understand what the project is, is not, and must preserve. |
| [Repository map](codebase/repository-map.md) | Find package ownership and placement rules. |
| [Memory core](codebase/memory-core.md) | Understand the Engram boundary and what this repo wires vs owns. |
| [Interfaces](codebase/interfaces.md) | Compare CLI, MCP, local HTTP API boundary, and TUI surfaces. |
| [Sync and cloud](codebase/sync-and-cloud.md) | Separate Gentle-AI config sync from Engram memory/cloud sync. |
| [Dashboard](codebase/dashboard.md) | Know what dashboard code is absent from this repo and how to avoid inventing it. |
| [Integrations](codebase/integrations.md) | Change agent adapters, plugins, and setup boundaries safely. |
| [Maintainer playbook](codebase/maintainer-playbook.md) | Use checklists by change type and PR review guardrails. |
| [Reference map](codebase/reference-map.md) | Trace docs and source files to responsibilities. |

## Recommended reading path

1. Start with [Mental model](codebase/mental-model.md).
2. Use [Repository map](codebase/repository-map.md) before editing code.
3. Read [Interfaces](codebase/interfaces.md) for user-facing changes.
4. Read the specialized page for your area: memory, sync, dashboard, or integrations.
5. Finish with [Maintainer playbook](codebase/maintainer-playbook.md) before opening or reviewing a PR.

## Existing references

| Page | Job |
|---|---|
| [Architecture & Development](architecture.md) | Short codebase layout and test commands. |
| [Usage](usage.md) | User-facing CLI/TUI behavior. |
| [Components](components.md) | Component, preset, and managed asset overview. |
| [Engram Commands](engram.md) | Engram user commands and MCP tool overview. |
| [OpenCode SDD Profiles](opencode-profiles.md) | Profile sync details. |
| [Skill Registry](skill-registry.md) | Skill indexing and refresh behavior. |
| [Agents](agents.md) | Supported agent matrix and config paths. |
| [Rollback](rollback.md) | Backup, restore, and uninstall safety model. |
| [Platforms](platforms.md) | Platform support and path notes. |
| [Intended Usage](intended-usage.md) | Product scope and expected workflow boundaries. |

## Next step

Read [Mental model](codebase/mental-model.md) next.
