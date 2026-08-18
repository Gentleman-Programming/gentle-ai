<div align="center">

<img width="3276" height="1280" alt="Gentle-AI neon rose banner" src="docs/assets/brand/gentle-ai-banner.png" />

<h1>Gentle-AI</h1>

<p><strong>Configure the AI coding agents you already use with shared memory, skills, workflows, and review controls.</strong></p>

<p>
<a href="https://github.com/Gentleman-Programming/gentle-ai/releases"><img src="https://img.shields.io/github/v/release/Gentleman-Programming/gentle-ai" alt="Release"></a>
<a href="LICENSE"><img src="https://img.shields.io/badge/License-MIT-blue.svg" alt="License: MIT"></a>
<img src="https://img.shields.io/badge/Go-1.25.10+-00ADD8?logo=go&logoColor=white" alt="Go 1.25.10+">
<img src="https://img.shields.io/badge/platform-macOS%20%7C%20Linux%20%7C%20Windows-lightgrey" alt="Platform">
</p>

</div>

---

Gentle-AI is a Go CLI/TUI that configures supported AI coding agents. It installs and synchronizes managed prompts, skills, MCP integrations, persistent-memory wiring, optional Spec-Driven Development (SDD), model profiles, and native review controls without replacing the agent runtime itself.

> [!IMPORTANT]
> Install the AI agent runtimes you want to use before running Gentle-AI. If a selected runtime is unavailable, configuration stops and reports the required installation command. When Pi is selected, make sure `pi` is available on `PATH`; Gentle-AI then installs its package-managed support stack.

## Supported agent integrations

Gentle-AI currently has adapters for:

Claude Code, OpenCode, Kilo Code, Gemini CLI, Cursor, VS Code Copilot, Codex, Windsurf, Antigravity, Kimi Code, Qwen Code, Kiro IDE, OpenClaw, Trae, Pi, and Hermes.

Capabilities differ by runtime. See [Supported Agents](docs/agents.md) for detection rules, delegation models, config paths, and feature support. For Pi's package-managed workflow, see [Pi Agent](docs/pi.md).

> This project supersedes [Agent Teams Lite](https://github.com/Gentleman-Programming/agent-teams-lite), which is archived.

## Install

The stable channel is the default. Use `@latest` for the latest stable Gentle-AI release; use `@main` only when testing unreleased development changes.

### macOS or Linux

```bash
curl -fsSL https://raw.githubusercontent.com/Gentleman-Programming/gentle-ai/main/scripts/install.sh | bash
```

The installer supports macOS and Linux on amd64 and arm64. It prefers Homebrew when available and otherwise uses a verified release archive. See [Quickstart](docs/quickstart.md) for prerequisites and distro-specific guidance.

### Windows

Windows is supported through the Go toolchain:

```powershell
go install github.com/gentleman-programming/gentle-ai/v2/cmd/gentle-ai@latest
```

For current Windows distribution and upgrade options, see [Supported Platforms](docs/platforms.md).

### Other installation options

<details>
<summary>Homebrew and source installation</summary>

**Homebrew**

```bash
brew tap Gentleman-Programming/homebrew-tap
brew trust --formula gentleman-programming/tap/gentle-ai
brew install gentle-ai
```

**Go 1.25.10+**

```bash
go install github.com/gentleman-programming/gentle-ai/v2/cmd/gentle-ai@latest
```

The `/v2` suffix is required by the Go module path.

By default, `gentle-ai install` writes agent-scoped files to each selected agent's global config directory. To keep the Gentleman stack isolated to one project, run:

```bash
gentle-ai install --scope=workspace
```

Workspace scope applies to selected agents for agent-scoped files such as system prompts, skills, SDD agents, and persona files. Global-only integrations remain global by design.

</details>

## Quick Start

1. Launch the interactive installer:

   ```bash
   gentle-ai
   ```

2. Select the detected agents and the components you want Gentle-AI to manage.
3. Open one of those agents in your project and describe the outcome you want. Normal use does not require memorizing SDD phases or manually editing generated configuration.
4. If anything looks wrong, run the read-only health check:

   ```bash
   gentle-ai doctor
   ```

For a non-interactive preview, use a dry run before applying configuration:

```bash
gentle-ai install --dry-run --agent claude-code --preset full-gentleman
```

After replacing or upgrading the `gentle-ai` binary, refresh the managed assets selected in `~/.gentle-ai/state.json`:

```bash
gentle-ai sync --dry-run
gentle-ai sync
```

See [Usage](docs/usage.md) for the complete CLI/TUI reference and [Quickstart](docs/quickstart.md) for first-install verification.

## Core workflows

### Use the smallest implementation route

Configured agents receive outcome-first routing guidance whether or not SDD is installed:

- Keep bounded, well-understood work direct.
- Delegate focused exploration or implementation when fresh context is useful.
- Use SDD only when durable proposal, specification, design, and task artifacts reduce substantial ambiguity, or when you explicitly request it.

Read [Intended Usage](docs/intended-usage.md) for the product mental model and [Organic Implementation Routing](docs/trigger-rules.md) for the routing rules.

### Engram (Persistent Memory)

When selected, Engram gives agents cross-session project memory through MCP, while the skill registry makes installed and project-local skills discoverable to orchestrators. Startup hooks refresh the registry where the runtime supports them; `/sdd-init` also initializes project context when SDD needs it.

- [Engram Commands](docs/engram.md)
- [Skill Registry](docs/skill-registry.md)
- [Components, Skills, and Presets](docs/components.md)

### Route models by phase

OpenCode and Kilo Code support generated multi-mode overlays, Kiro supports native per-agent model assignments, and Pi owns model routing through its package-managed assets. Other integrations use their active model or runtime-specific delegation behavior.

See [OpenCode SDD Profiles](docs/opencode-profiles.md) and the [agent support matrix](docs/agents.md).

## Safety model

Gentle-AI separates agent advice from native authority:

- **Previewable changes.** Install and sync support `--dry-run`.
- **Scoped ownership.** Sync uses the agents recorded in Gentle-AI state unless you pass explicit `--agent` values.
- **Recoverable configuration.** Install, sync, upgrade, and managed uninstall snapshot affected managed configuration; restore follows the ownership boundaries documented in [Backup and Rollback](docs/rollback.md).
- **Verified releases.** macOS and Linux release archives are checksum-bound. Release-archive upgrades also verify the signed checksum manifest and release identity before replacement. See [Release Signing](docs/release-signing.md).

### Control receipt-driven development

Review mode is user-owned and available independently of the review lifecycle. **Receipt-driven development is opt-in: it is off until you turn it on.** When active, native review freezes a candidate, bounds reviewer work, issues a content-bound receipt, and revalidates that receipt at delivery gates. Agent narration is not delivery authorization.

```bash
gentle-ai review mode status --cwd .
gentle-ai review mode enable --scope global --cwd .
gentle-ai review mode disable --scope clone --cwd .
```

The global enable is the only way to opt in. A clone may disable review locally but cannot require it for the user, and any disabled source wins. Enabling applies to future candidates only.

For integration details, read the [Review Integration Contract](docs/review-integration.md). For security boundaries and failure behavior, read the [Review Authority Threat Model](docs/review-authority-threat-model.md). [Chapter 21 — Verifiable Trust](https://the-amazing-gentleman-programming-book.vercel.app/en/book/Chapter21_Verifiable-Trust) explains the broader mental model.

## Documentation

| Goal | Start here |
| --- | --- |
| Install and verify Gentle-AI | [Quickstart](docs/quickstart.md) |
| Understand intended day-to-day use | [Intended Usage](docs/intended-usage.md) |
| Check OS, architecture, and distribution support | [Supported Platforms](docs/platforms.md) |
| Compare agent capabilities and config paths | [Supported Agents](docs/agents.md) |
| Learn CLI/TUI commands, sync, upgrade, and uninstall | [Usage](docs/usage.md) |
| Configure OpenCode phase models | [OpenCode SDD Profiles](docs/opencode-profiles.md) |
| Integrate with native review | [Review Integration Contract](docs/review-integration.md) |
| Understand review security boundaries | [Review Authority Threat Model](docs/review-authority-threat-model.md) |
| Recover or remove managed configuration | [Backup and Rollback](docs/rollback.md) |
| Maintain or extend the repository | [Codebase Guide](docs/CODEBASE-GUIDE.md) and [Architecture and Development](docs/architecture.md) |

## Community Highlights

This project gets better when the community builds on top of it.

### Community Integrations

- [sub-agent-statusline](https://github.com/Joaquinvesapa/sub-agent-statusline) — optional OpenCode TUI plugin that shows sub-agent activity, status, elapsed time, and token/context usage when OpenCode exposes it.
- [sdd-engram-plugin](https://github.com/j0k3r-dev-rgl/sdd-engram-plugin) — optional OpenCode TUI plugin to manage SDD profiles and browse Engram memories directly from OpenCode, with runtime profile activation and no restart required.

### Contributors

This project exists because of the community. See [CONTRIBUTORS.md](CONTRIBUTORS.md) for the full list.

<a href="https://github.com/Gentleman-Programming/gentle-ai/graphs/contributors">
  <img src="https://contrib.rocks/image?repo=Gentleman-Programming/gentle-ai" alt="Gentle-AI contributors" />
</a>

## Contributing and support

Contributions follow an issue-first workflow. Start with the [Community Roadmap](docs/community-roadmap.md), then read [CONTRIBUTING.md](CONTRIBUTING.md) and the [AI-Assisted Contribution Policy](AI_POLICY.md) before opening a pull request.

For bugs and feature requests, use the repository's [GitHub issues](https://github.com/Gentleman-Programming/gentle-ai/issues). For release history, use [GitHub Releases](https://github.com/Gentleman-Programming/gentle-ai/releases).

---

<div align="center">
<a href="LICENSE"><img src="https://img.shields.io/badge/License-MIT-blue.svg" alt="License: MIT"></a>
</div>
