<div align="center">

<img width="3276" height="1280" alt="Gentle-AI neon rose banner" src="docs/assets/brand/gentle-ai-banner.png" />

<h1>Gentle-AI</h1>

<p><strong>Gentle-AI — Ecosystem, Frameworks, Workflows for AI coding agents.</strong></p>

<p>
<a href="https://github.com/Gentleman-Programming/gentle-ai/releases"><img src="https://img.shields.io/github/v/release/Gentleman-Programming/gentle-ai" alt="Release"></a>
<a href="LICENSE"><img src="https://img.shields.io/badge/License-MIT-blue.svg" alt="License: MIT"></a>
<img src="https://img.shields.io/badge/Go-1.25.10+-00ADD8?logo=go&logoColor=white" alt="Go 1.25.10+">
<img src="https://img.shields.io/badge/platform-macOS%20%7C%20Linux%20%7C%20Windows-lightgrey" alt="Platform">
</p>

</div>

---

## What It Does

Gentle-AI is NOT an AI agent installer. Most agents are easy to install. It is an **ecosystem configurator** that equips the AI coding agent(s) you already use with persistent memory, Spec-Driven Development (SDD), curated skills, MCP servers, model routing, a teaching-oriented persona, and bounded native review.

**Before**: "I installed Claude Code / OpenCode / Cursor, but it's just a chatbot that writes code."

**After**: Your agent now has memory, skills, workflow, MCP tools, and a persona that actually teaches you.

### Supported Agent Integrations

| Agent               |         Delegation Model         | Key Feature                                                     |
| ------------------- | :------------------------------: | --------------------------------------------------------------- |
| **Claude Code**     |         Full (Task tool)         | Sub-agents, output styles                                       |
| **OpenCode**        |    Full (multi-mode overlay)     | Per-phase model routing                                         |
| **Kilo Code**       |    Full (multi-mode overlay)     | OpenCode-compatible config in `~/.config/kilo`                  |
| **Gemini CLI**      |       Full (experimental)        | Custom agents in `~/.gemini/agents/`                            |
| **Cursor**          |     Full (native subagents)      | 10 SDD agents in `~/.cursor/agents/`                            |
| **VS Code Copilot** |        Full (runSubagent)        | Parallel execution                                              |
| **Codex**           |            Solo-agent            | CLI-native, TOML config                                         |
| **Windsurf**        |            Solo-agent            | Plan Mode, Code Mode, native workflows                          |
| **Antigravity**     |   Solo-agent + Mission Control   | Built-in Browser/Terminal sub-agents                            |
| **Kimi Code**       |   Full (native custom agents)    | Modular prompt templates in `~/.kimi`                           |
| **Kiro IDE**        |     Full (native subagents)      | Native `~/.kiro/agents/` + steering orchestration               |
| **Qwen Code**       |     Full (native sub-agents)     | Slash commands, `~/.qwen/commands/`, `auto_edit` mode           |
| **OpenClaw**        |            Solo-agent            | Workspace-first `AGENTS.md` / `SOUL.md` with global MCP config  |
| **Trae**            |            Solo-agent            | Desktop app by ByteDance; `~/.trae/skills/` + OS-specific rules |
| **Pi**              | Full (package-managed subagents) | First-class `gentle-pi` harness with Pi-native persona/models, SDD, and Engram memory |
| **Hermes**          |         Detect-only              | YAML MCP config, SOUL.md persona; install manually first        |

> **Pi is package-managed, not just configured.** Selecting Pi installs the first-class [`gentle-pi`](docs/pi.md) harness, which owns Pi-native persona and model controls, SDD assets, chains, and memory wiring.

> **Note**: This project supersedes [Agent Teams Lite](https://github.com/Gentleman-Programming/agent-teams-lite) (now archived). Everything ATL provided is included here with better installation, automatic updates, and persistent memory.

### Delegation and Review Boundaries

Gentle-AI's workflow guidance keeps the parent/orchestrator thread thin. Once a task stops being small, delegation or an explicit SDD phase boundary is expected rather than optional.

| Trigger | Expected behavior |
| --- | --- |
| Reading 4+ files to understand a flow | Delegate exploration or run an exploration phase. |
| Touching 2+ non-trivial files | Use one focused writer and validate the result. |
| Implementation ready for review | Start one bounded native review that freezes the candidate and creates a content-bound receipt. |
| Commit, push, or PR | Validate that **same** receipt against the live Git candidate; never silently reopen review or create a new budget. |
| Release | Validate native authority/receipt, or use protected-main only with an exact tag/current `origin/main` SHA, exact-SHA CI, remote-head recheck, and no fresh risk. |
| Wrong cwd, worktree/git accident, merge recovery, or confusing test/env issue | Stop, preserve the review scope, and investigate or validate the existing receipt before proceeding. |
| Long monolithic session with accumulating complexity | Pause and delegate, re-plan, or justify why not. |

The workflow guidance directs agents; the native review commands bind receipts and lifecycle gates to the Git candidate they inspect. They protect against accidental scope or identity drift, not a malicious local actor. See the [review authority threat model](docs/review-authority-threat-model.md) for boundaries and assumptions.

---

## Quick Start

### Install (recommended)

**macOS / Linux**

```bash
curl -fsSL https://raw.githubusercontent.com/Gentleman-Programming/gentle-ai/main/scripts/install.sh | bash
```

**Windows (PowerShell)**

```powershell
irm https://raw.githubusercontent.com/Gentleman-Programming/gentle-ai/main/scripts/install.ps1 | iex
```

The managed installer works with Gentle AI's built-in updater; on Windows it installs to `%LOCALAPPDATA%\gentle-ai\bin`.

### Configure project context

Once your agents are configured, open your AI agent in a project and run these two commands to register the project context:

| Command                            | What it does                                                                | When to re-run                                                                 |
| ---------------------------------- | --------------------------------------------------------------------------- | ------------------------------------------------------------------------------ |
| `/sdd-init`                        | Detects stack, testing capabilities, activates Strict TDD Mode if available | When your project adds/removes test frameworks, or first time in a new project |
| `gentle-ai skill-registry refresh` | Scans installed skills and project conventions, builds the registry         | After installing/removing skills, or first time in a new project               |

These are **not required** for basic usage. The SDD orchestrator runs `/sdd-init` automatically if it detects no context. Startup hooks normally keep the skill registry fresh for agents that support hooks, including Codex, Claude Code, OpenCode, and Pi through `gentle-pi`. If you start Pi with `pi -ns`, startup skill loading/hooks are skipped, so run the registry refresh manually when you need updated project rules.

Run `gentle-ai doctor` at any time for a read-only health check of your ecosystem (tool binaries, `state.json`, Engram reachability, disk space).

<details>
<summary><strong>Alternative install and scope options</strong></summary>

**Homebrew (macOS / Linux)**

```bash
brew tap Gentleman-Programming/homebrew-tap
brew trust --formula gentleman-programming/tap/gentle-ai  # one-time, for Homebrew tap trust
brew install gentle-ai
```

**Go install (any platform with Go 1.25.10+)**

```bash
go install github.com/gentleman-programming/gentle-ai/cmd/gentle-ai@latest
```

**Scoop (Windows)** — this is a manual-update path; update it with `scoop update gentle-ai`.

```powershell
scoop bucket add gentleman https://github.com/Gentleman-Programming/scoop-bucket
scoop install gentle-ai
```

By default, `gentle-ai install` writes agent-scoped files to each selected agent's global config directory. To keep the Gentleman stack isolated to one project, run:

```bash
gentle-ai install --scope=workspace
```

Workspace scope applies to selected agents for agent-scoped files such as system prompts, skills, SDD agents, and persona files. Global-only integrations remain global by design.

**Beta channel** — use only to test unreleased `main` builds. It requires Go 1.25.10+:

```bash
# macOS / Linux
curl -fsSL https://raw.githubusercontent.com/Gentleman-Programming/gentle-ai/main/scripts/install.sh | bash -s -- --channel beta

# Windows (PowerShell)
$env:GENTLE_AI_CHANNEL="beta"; irm https://raw.githubusercontent.com/Gentleman-Programming/gentle-ai/main/scripts/install.ps1 | iex
```

</details>

---

## Core Workflow

1. **Install and configure.** Run the installer, select the agents and components you want, then open your agent in a project.
2. **Plan when it helps.** SDD is optional for substantial work. Its artifacts can live in **Engram** for cross-session memory, **OpenSpec** for versioned files, or **hybrid** for both.
3. **Build with discipline.** `/sdd-init` detects project testing capabilities; when Strict TDD is active, SDD apply works test-first. SDD verify audits RED/GREEN evidence and runs verification. Agents that support delegation use focused subagents instead of one growing conversation.
4. **Review one candidate.** After implementation, bounded native review freezes the candidate and issues one content-bound receipt. Commit, push, and PR validate that same receipt. Releases validate native authority and its receipt, unless the protected-main fast path has the exact tag/current `origin/main` SHA, exact-SHA successful CI, a remote-head recheck, and no fresh risk.

> **Trust what the system can derive, not agent narration.** [Chapter 21 — Verifiable Trust](https://the-amazing-gentleman-programming-book.vercel.app/en/book/Chapter21_Verifiable-Trust) explains the mental model: agents assess the candidate; native authority and delivery gates independently derive what may be trusted.

5. **Upgrade, then sync.** Refresh the binary and the managed agent assets together:

   ```bash
   gentle-ai upgrade
   gentle-ai sync
   ```

### Review a focused staged candidate

For a monorepo or shared worktree, explicitly review exactly what is in the Git index:

```bash
git add apps/my-service
git diff --cached
gentle-ai review start --projection staged
```

The staged projection freezes the **complete existing index**, including all previously staged paths. It starts review but does not itself issue an approved receipt; unstaged and untracked worktree content is excluded. The default `workspace` projection remains the complete workspace review, and an existing authority is never auto-converted between projections. See the [review authority threat model](docs/review-authority-threat-model.md) for delivery and base-ref details.

### Backups

Every install, sync, and upgrade automatically snapshots your config files. Backups are **compressed** (tar.gz), **deduplicated** (identical configs are not re-backed up), and **auto-pruned** (keeps the 5 most recent). Pin important backups via the TUI (`p` key) to protect them from pruning.

See [Backup & Rollback Guide](docs/rollback.md) for details.

---

## Key Features You Should Know About

### OpenCode SDD Profiles

Assign different AI models to different SDD phases -- a powerful model for design, a fast one for implementation, a cheap one for exploration. OpenCode uses **`gentle-orchestrator`** as the base SDD conductor, and generated named profiles still appear as `sdd-orchestrator-{name}` entries.

```bash
# Via CLI
gentle-ai sync --profile cheap:openrouter/qwen/qwen3-30b-a3b:free
gentle-ai sync --profile-phase cheap:sdd-design:anthropic/claude-sonnet-4-20250514

# Or via TUI: gentle-ai → "OpenCode SDD Profiles" → Create
```

After creating a profile, open OpenCode and press **Tab** to switch between `gentle-orchestrator` (default) and your custom profiles.

| What you need         | Use this                                                        |
| --------------------- | --------------------------------------------------------------- |
| Default SDD conductor | `gentle-orchestrator`                                           |
| Legacy configs        | `sdd-orchestrator` is migrated to `gentle-orchestrator` on sync |
| Named model profiles  | `sdd-orchestrator-cheap`, `sdd-orchestrator-premium`, etc.      |

**Full guide**: [OpenCode SDD Profiles](docs/opencode-profiles.md)

### Engram (Persistent Memory)

Your AI agent automatically remembers decisions, bugs, and context across sessions. You don't need to do anything -- but when you do:

```bash
engram projects list          # See all projects with memory counts
engram projects consolidate   # Fix name drift ("my-app" vs "My-App")
engram search "auth bug"      # Find a past decision from the terminal
engram tui                    # Visual memory browser
```

**Full reference**: [Engram Commands](docs/engram.md)

---

## Documentation

| Your task | Start here |
| --- | --- |
| Understand the Gentle-AI mental model | [Intended Usage](docs/intended-usage.md) |
| Plan substantial work with SDD | [Intended Usage](docs/intended-usage.md) and [OpenSpec Config](docs/openspec-config.md) |
| Configure a supported agent | [Agents](docs/agents.md) for the feature matrix and per-agent notes |
| Use the Pi package harness | [Pi Agent](docs/pi.md) for packages, Pi-native commands, models, and troubleshooting |
| Configure OpenCode phase models | [OpenCode SDD Profiles](docs/opencode-profiles.md) |
| Review or deliver a change safely | [Review Integration Contract](docs/review-integration.md) for provider consumers; [Review Authority Threat Model](docs/review-authority-threat-model.md) for technical boundaries; [Chapter 21 — Verifiable Trust](https://the-amazing-gentleman-programming-book.vercel.app/en/book/Chapter21_Verifiable-Trust) for the mental model |
| Find or share persistent context | [Engram Commands](docs/engram.md) |
| Refresh or troubleshoot an installation | [Usage](docs/usage.md), [Backup & Rollback](docs/rollback.md), and [Platforms](docs/platforms.md) |
| Extend or contribute to Gentle AI | [Components, Skills & Presets](docs/components.md), [Skill Registry](docs/skill-registry.md), and [Architecture & Development](docs/architecture.md) |

---

## Community Highlights

This project gets better when the community builds on top of it.

### Community Integrations

- [sub-agent-statusline](https://github.com/Joaquinvesapa/sub-agent-statusline) — optional OpenCode TUI plugin that shows sub-agent activity, status, elapsed time, and token/context usage when OpenCode exposes it.
- [sdd-engram-plugin](https://github.com/j0k3r-dev-rgl/sdd-engram-plugin) — optional OpenCode TUI plugin to manage SDD profiles and browse Engram memories directly from OpenCode, with runtime profile activation and no restart required.

When you select OpenCode in the installer, Gentle-AI asks whether to register each community plugin and offers a browser shortcut to review the repository first. Gentle-AI only ensures `~/.config/opencode/tui.json` exists and adds the plugin package names to its `plugin` array; OpenCode installs/loads those packages the next time it starts. Once OpenCode has materialized a plugin under `~/.config/opencode/node_modules/`, `gentle-ai update` can compare its local `package.json` version with the plugin's GitHub releases.

### Contributors

This project exists because of the community. See [CONTRIBUTORS.md](CONTRIBUTORS.md) for the full list.

<a href="https://github.com/Gentleman-Programming/gentle-ai/graphs/contributors">
  <img src="https://contrib.rocks/image?repo=Gentleman-Programming/gentle-ai" />
</a>

---

## Next Steps

- **Just installed?** Read [Intended Usage](docs/intended-usage.md) for the mental model, then run `gentle-ai doctor` if anything looks wrong.
- **Planning substantial work?** Learn the SDD and OpenSpec conventions in [Intended Usage](docs/intended-usage.md) and [OpenSpec Config](docs/openspec-config.md).
- **Reviewing a focused change?** Start with the [review authority threat model](docs/review-authority-threat-model.md), including staged-index boundaries.
- **Using Pi?** Read [Pi Agent](docs/pi.md) for the `gentle-pi` harness, Pi commands, persona, and model assignments.
- **Ready to contribute?** Check [CONTRIBUTING.md](CONTRIBUTING.md) and the [open issues](https://github.com/Gentleman-Programming/gentle-ai/issues?q=is%3Aissue+is%3Aopen+label%3A%22status%3Aapproved%22).

---

<div align="center">
<a href="LICENSE"><img src="https://img.shields.io/badge/License-MIT-blue.svg" alt="License: MIT"></a>
</div>
