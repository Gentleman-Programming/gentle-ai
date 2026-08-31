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

> [!IMPORTANT]
> **Receipt-Driven Development (RDD) is opt-in and provides bounded review evidence.** `v2.2.0` was the historical release where that path became supported after RDD began in `v1.47.0`: small work stays direct, broader implementation is delegated, SDD stays optional, and once RDD is enabled every route can converge on structural proof and an informational bounded-review outcome. Ordinary repository policy owns delivery. RDD is off until you enable it with `gentle-ai review mode enable --scope global`.
>
> The current stable release is [`v2.3.0`](https://github.com/Gentleman-Programming/gentle-ai/releases/tag/v2.3.0). `@latest` is the stable channel:
>
> ```bash
> go install github.com/gentleman-programming/gentle-ai/v2/cmd/gentle-ai@latest
> ```
>
> To opt into the current prerelease, [`v2.4.0-rc.1`](https://github.com/Gentleman-Programming/gentle-ai/releases/tag/v2.4.0-rc.1), install its exact tag:
>
> ```bash
> go install github.com/gentleman-programming/gentle-ai/v2/cmd/gentle-ai@v2.4.0-rc.1
> ```
>
> Use `@main` only for unreleased development changes. See the [full RDD version policy](docs/quickstart.md#version-policy).
>
> Note the `/v2` suffix: Go requires it for major version 2 and above. Releases before `v2.0.0` use the unsuffixed import path.

## What It Does

Gentle-AI is NOT an AI agent installer. It adapts the agent runtime(s) already on your machine; it never installs one for you. If a selected agent isn't detected, Gentle-AI refuses and names the exact command you'd run yourself instead. It is an **ecosystem configurator** that equips the AI coding agent(s) you already use with persistent memory, Spec-Driven Development (SDD), curated skills, MCP servers, model routing, a teaching-oriented persona, and bounded native review.

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

### Organic Routing and Review Boundaries

Every configured agent receives the same outcome-first routing, even when the optional SDD component is not selected. Ask for the outcome; the agent uses exactly one implementation route and reviews the candidate only after implementation.

| Situation | Expected behavior |
| --- | --- |
| Understanding needs 1-3 files, or one mechanical file change is already understood | Keep the bounded action direct and inline. |
| Understanding needs 4+ files, reading prepares a write, broad research is needed, or a writer changes 2+ non-trivial files | Delegate the narrow exploration or one focused writer without creating SDD state. |
| Durable proposal, spec, design, and task artifacts would materially reduce substantial ambiguity | Offer optional SDD; select it only after an explicit request or an accepted proposal. |
| A candidate is ready for review | Freeze the exact bytes and derive review effort from evidence, never size alone. Interactive starts ask once per clone before reviewer work; non-interactive tier-1/tier-2 starts proceed without prompting and report how to disable review mode. |
| Commit, push, PR, or release | Follow ordinary repository policy. Review outcomes are informational and never authorize, block, or govern delivery. |
| Scope changes or an operation is interrupted | Use provider-owned status, recovery, and reconciliation; do not infer authority or replay safety from narration. |

Implementation routing does not decide review strength, and per-action test, build, install, or review workers do not change the selected route. Native commands own repository identity, candidate scope, lifecycle transitions, receipts, and safe continuations. See [Organic Implementation Routing](docs/trigger-rules.md), the [Organic RDD architecture](docs/architecture/organic-rdd.md), and the [review authority threat model](docs/review-authority-threat-model.md).

---

## Quick Start

### Install (recommended)

> [!NOTE]
> `gentle-ai install` requires Node.js 18+ and npm on every platform (it warns if either is missing). See [Prerequisites](docs/quickstart.md#prerequisites) for your distro's install hint.

**macOS / Linux**

```bash
curl -fsSL https://raw.githubusercontent.com/Gentleman-Programming/gentle-ai/main/scripts/install.sh | bash
```

**Windows (PowerShell)**

```powershell
go install github.com/gentleman-programming/gentle-ai/v2/cmd/gentle-ai@latest
```

> [!WARNING]
> Windows source builds and CI/runtime tests remain supported, but official Windows binary distribution and Scoop are temporarily unavailable. Windows installation and upgrades require Go 1.25.10+ and fail closed to source-install guidance; they never download an unsigned Gentle AI executable or execute a remote update script.

> [!IMPORTANT]
> After replacing or upgrading the `gentle-ai` binary, run `gentle-ai sync` to refresh its managed assets. See the [sync and upgrade reference](docs/usage.md#sync).

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
brew tap gentleman-programming/tap
brew trust --formula gentleman-programming/tap/gentle-ai  # one-time, if Homebrew requires trust
brew install gentle-ai
```

To install several tools from this tap, you can instead run `brew trust gentleman-programming/tap`. This broader option trusts all current and future formulas, casks, and external commands published in the tap.

**Go install: stable channel (any platform with Go 1.25.10+)**

```bash
go install github.com/gentleman-programming/gentle-ai/v2/cmd/gentle-ai@latest
```

Note the `/v2` in the module path: Go requires it for major version 2 and
above. Releases before `v2.0.0` use the unsuffixed path.

**Scoop (Windows)** — temporarily unavailable while official Windows binary distribution is held for public-trust Authenticode signing. Use the Windows `go install` command above.

By default, `gentle-ai install` writes agent-scoped files to each selected agent's global config directory. To keep the Gentleman stack isolated to one project, run:

```bash
gentle-ai install --scope=workspace
```

Workspace scope applies to selected agents for agent-scoped files such as system prompts, skills, SDD agents, and persona files. Global-only integrations remain global by design.

**Unreleased development channel (`main`)** — use only to test changes that are not part of a release. The managed beta installer tracks `main`; it requires Go 1.25.10+:

```bash
# macOS / Linux
curl -fsSL https://raw.githubusercontent.com/Gentleman-Programming/gentle-ai/main/scripts/install.sh | bash -s -- --channel beta

# Windows (PowerShell)
$env:GENTLE_AI_CHANNEL="beta"; go install github.com/gentleman-programming/gentle-ai/v2/cmd/gentle-ai@main
```

To update a beta installation later, preserve the beta channel:

```bash
# macOS / Linux
GENTLE_AI_CHANNEL=beta gentle-ai upgrade

# Windows (PowerShell)
$env:GENTLE_AI_CHANNEL="beta"; gentle-ai upgrade
```

> [!NOTE]
> **Beta upgrades & Go proxy caching**: On macOS, Linux, and Windows with Go on `PATH`, `gentle-ai upgrade` advances the `gentle-ai` binary from `main` and refreshes managed tools (Engram, plugins, etc.). If you re-run an installer instead, pass beta explicitly because both installers default to stable:
>
> ```bash
> # macOS / Linux
> curl -fsSL https://raw.githubusercontent.com/Gentleman-Programming/gentle-ai/main/scripts/install.sh | bash -s -- --channel beta
>
> # Windows (PowerShell)
> $env:GENTLE_AI_CHANNEL="beta"; irm https://raw.githubusercontent.com/Gentleman-Programming/gentle-ai/main/scripts/install.ps1 | iex
> ```
>
> If a manual `go install ...@main` does not pick up recent commits because `proxy.golang.org` is stale, bypass it with `GOPROXY=direct go install github.com/gentleman-programming/gentle-ai/v2/cmd/gentle-ai@main` (PowerShell: `$env:GOPROXY="direct"; go install github.com/gentleman-programming/gentle-ai/v2/cmd/gentle-ai@main`).

### RDD version policy

Receipt-Driven Development (RDD) started in `gentle-ai` `v1.47.0` on 2026-07-10, with the first bounded native review transactions, and became the supported stable path in `v2.2.0`. Those are historical milestones; the negotiated public review contract was published in `v2.1.6`.

The current stable release is [`v2.3.0`](https://github.com/Gentleman-Programming/gentle-ai/releases/tag/v2.3.0). The current prerelease is [`v2.4.0-rc.1`](https://github.com/Gentleman-Programming/gentle-ai/releases/tag/v2.4.0-rc.1). `main` is unreleased development.

**Stable channel (`@latest`, currently `v2.3.0`)**

```bash
go install github.com/gentleman-programming/gentle-ai/v2/cmd/gentle-ai@latest
gentle-ai version
```

**Prerelease channel (`v2.4.0-rc.1`)**

```bash
go install github.com/gentleman-programming/gentle-ai/v2/cmd/gentle-ai@v2.4.0-rc.1
gentle-ai version
```

**Unreleased development (`main`)**

```bash
go install github.com/gentleman-programming/gentle-ai/v2/cmd/gentle-ai@main
gentle-ai version
```

The managed installer tracks the channel's latest version and does not accept an arbitrary release pin. Use `go install` when reproducibility requires an exact version.

</details>

---

## Core Workflow

1. **Install and configure.** Run the installer, select the agents and components you want, then open your agent in a project.
2. **Use the smallest implementation route.** Keep bounded work direct, delegate actions that need fresh context, and use SDD only after an explicit request or an accepted proposal. SDD artifacts can live in **Engram** for cross-session memory, **OpenSpec** for versioned files, or **hybrid** for both.
3. **Build with discipline.** `/sdd-init` detects project testing capabilities; when Strict TDD is active, SDD apply works test-first. SDD verify audits RED/GREEN evidence and runs verification. Agents that support delegation use focused subagents instead of one growing conversation.
4. **Review one candidate.** After implementation, bounded native review freezes the candidate and reports an informational outcome. Commit, push, PR, and release remain separate decisions under ordinary repository policy; review does not authorize, block, or govern them.

> **Trust what the system can derive, not agent narration.** [Chapter 21 — Verifiable Trust](https://the-amazing-gentleman-programming-book.vercel.app/en/book/Chapter21_Verifiable-Trust) explains the mental model: agents assess the candidate; native review records bounded evidence while ordinary repository policy owns delivery.

1. **Upgrade, then sync.** Refresh the binary and the managed agent assets together:

   ```bash
   gentle-ai upgrade
   gentle-ai sync
   ```

### The flow at a glance

Once you enable it, both implementation routes can converge on RDD: a bounded native review freezes the candidate and reports an informational outcome — review is never reopened for unchanged content. RDD is opt-in, and ordinary repository policy owns delivery whether it is on or off.

**Organic route (no SDD)** — the agent picks the smallest useful route and RDD enters at the end, over the frozen candidate:

```mermaid
flowchart TD
    A["User requests a change<br/>(Claude Code · OpenCode · Codex...)"] --> B{"Implementation<br/>route"}
    B -->|"decide/verify<br/>1–3 files"| C["Direct inline"]
    B -->|"4+ file exploration<br/>or 2+ non-trivial writes"| D["Delegated direct<br/>(one bounded worker)"]
    C --> E["Implementation + tests"]
    D --> E
    E --> F{"RDD enabled?<br/>(user-owned, opt-in)"}
    F -->|"off (default)"| Z["Ordinary delivery<br/>reports disabled/unmanaged"]
    F -->|"on (explicitly enabled)"| G["review status --next-transition<br/>(provider-owned negotiated route)"]
    G --> H{"Risk frozen<br/>at START"}
    H -->|"low"| I["Structural readback<br/>0 lenses · silent"]
    H -->|"standard"| J["1 focus lens<br/>+ consent"]
    H -->|"high"| K["Canonical 4R + consent + forecast<br/>Risk · Readability · Reliability · Resilience"]
    J --> L["Reviewers inspect the immutable candidate<br/>(review inspect-candidate)"]
    K --> L
    L --> M{"Severe candidate-caused<br/>findings?"}
    I --> N["Review outcome: approved<br/>(informational)"]
    M -->|"no"| N
    M -->|"yes"| O["One bounded correction<br/>(frozen budget)"]
    O --> P["Fix validator<br/>(read-only, immutable trees)"]
    P -->|"passes"| N
    P -->|"fails with evidence"| Q["Escalated"]
    P -->|"no access to the diff"| R["Inconclusive: attempt not<br/>consumed, capture again"]
    R --> P
    Q --> S["review recover<br/>(authorized successor)"]
    N --> T["Ordinary repository policy"]
    T --> U["Commit → Push → PR"]
    Z --> U

    style N fill:#2D4F67,color:#fff
    style Q fill:#B8860B,color:#fff
    style U fill:#2D4F67,color:#fff
```

**SDD route** — durable planning artifacts first, then apply, independent verify, and an optional RDD review offer; archive and delivery follow ordinary repository policy:

```mermaid
flowchart TD
    A["User: sdd-new / sdd-explore<br/>(or sdd-ff to fast-forward planning)"] --> B["Explore<br/>investigate codebase and approaches"]
    B --> C["Propose<br/>intent · scope · approach"]
    C --> D{"User approves<br/>the proposal?"}
    D -->|"no"| B
    D -->|"yes"| E["Spec<br/>requirements + scenarios"]
    E --> F["Design<br/>architecture decisions"]
    F --> G["Tasks<br/>ordered deliverable checklist"]
    G --> H["Apply<br/>sub-agent implements against specs<br/>(sdd-attempt acquire/settle · CAS · budgets)"]
    H --> Q["Verify<br/>independent verification against<br/>spec · design · tasks"]
        Q -->|"passes"| I["Optional RDD review offer"]
        Q -->|"fails"| H

    subgraph RDD["RDD — same machine as the organic route"]
        I --> J{"Risk"}
        J -->|"low"| K["Structural readback"]
        J -->|"standard / high"| L["1 lens or 4R + consent"]
        L --> M{"Severe findings?"}
        M -->|"yes"| N["One bounded correction<br/>+ fix validator"]
        M -->|"no"| O["Review outcome: approved<br/>(informational)"]
        K --> O
        N -->|"validates"| O
        N -->|"fails"| P["Escalated → recover"]
    end

    O --> R["Archive<br/>merge delta-specs · close the cycle"]
    Q -->|"fails"| H
    R --> S["Ordinary repository policy"]
    S --> T["Commit → Push → PR"]

    style O fill:#2D4F67,color:#fff
    style P fill:#B8860B,color:#fff
    style T fill:#2D4F67,color:#fff
```

Size, file count, or perceived risk never select SDD on their own — only an explicit request or an accepted proposal does. Either way, one candidate gets one review, one possible correction, and one receipt.

### Control receipt-driven development

Review mode is user-owned and available independently of the review lifecycle. **Receipt-driven development is opt-in: it is off until you turn it on.**

```bash
gentle-ai review mode status --cwd .
gentle-ai review mode enable --scope global --cwd .
gentle-ai review mode disable --cwd .
```

`status` is read-only. With no source expressing an opinion the effective mode is `off`, reported as decided by `default`; only an explicit global enable turns review on. Any global or clone-local disabled source wins; a clone can opt out with `--scope clone` but cannot force review on, so `--scope global` is the only way in. Enabling applies only to future candidates, while declining a one-candidate review prompt does not change the mode. When review is off, native review does not run. Review outcomes are informational in every mode, and ordinary repository policy decides delivery without fabricated approval.

Historical note: `v2.2.2` introduced the native `disabled/unmanaged` disposition. Current SDD status does not use that disposition: with review disabled, it skips review authority, emits no `reviewGate`, and pre-verify continues without routing to a review that cannot start. Archive and delivery proceed under ordinary repository policy; any present review outcome is informational.

### Release verification

Official macOS and Linux release archives require an authenticated `checksums.txt`. The built-in upgrader verifies its Minisign signature, its exact `Gentleman-Programming/gentle-ai` + release-tag binding, and the selected archive checksum **before** replacing the installed binary. Release archives are capped at **128 MiB**, including chunked or unknown-length responses. Missing, oversized, malformed, untrusted, or placeholder key material fails closed without changing the installed binary.

To verify a release manually, obtain the production public-key payload and fingerprint from a maintainer-controlled channel, then download `checksums.txt` and `checksums.txt.minisig` from the same release:

```bash
minisign -VQm checksums.txt -x checksums.txt.minisig -P "$GENTLE_AI_MINISIGN_PUBLIC_KEY"
# Expected output: repo=Gentleman-Programming/gentle-ai;tag=vX.Y.Z
sha256sum --check --strict --ignore-missing checksums.txt
```

Do not bootstrap trust from a public key downloaded only beside the artifacts it verifies. See [Release signing and key rotation](docs/release-signing.md) for the first-signed-release procedure, exact CI injection points, and rotation runbook.

Windows archives and Scoop publication remain omitted until publicly trusted RSA Authenticode signing is provisioned (prefer managed OIDC with Azure Artifact Signing), both amd64 and arm64 executables are signed before archive and checksum generation, and release verification fails if either executable is unsigned.

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
| Choose direct, delegated, or optional SDD routing | [Organic Implementation Routing](docs/trigger-rules.md) |
| Plan substantial work with SDD | [Intended Usage](docs/intended-usage.md) and [OpenSpec Config](docs/openspec-config.md) |
| Configure a supported agent | [Agents](docs/agents.md) for the feature matrix and per-agent notes |
| Use the Pi package harness | [Pi Agent](docs/pi.md) for packages, Pi-native commands, models, and troubleshooting |
| Configure OpenCode phase models | [OpenCode SDD Profiles](docs/opencode-profiles.md) |
| Review or deliver a change safely | [Review Integration Contract](docs/review-integration.md) for provider consumers; [Review Authority Threat Model](docs/review-authority-threat-model.md) for technical boundaries; [Chapter 21 — Verifiable Trust](https://the-amazing-gentleman-programming-book.vercel.app/en/book/Chapter21_Verifiable-Trust) for the mental model |
| Find or share persistent context | [Engram Commands](docs/engram.md) |
| Refresh or troubleshoot an installation | [Usage](docs/usage.md), [Backup & Rollback](docs/rollback.md), and [Platforms](docs/platforms.md) |
| Extend or contribute to Gentle AI | [Codebase Guide](docs/CODEBASE-GUIDE.md), [Components, Skills & Presets](docs/components.md), [Skill Registry](docs/skill-registry.md), and [Architecture & Development](docs/architecture.md) |
| Understand how agent behavior is tested | [Testing Agents Deterministically](docs/testing-agents-deterministically.md) for the real-agent E2E and its model fixture |

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
- **Starting work?** Read [Organic Implementation Routing](docs/trigger-rules.md) to understand direct, delegated, and optional SDD behavior.
- **Reviewing a focused change?** Start with the [Organic RDD architecture](docs/architecture/organic-rdd.md) and [review authority threat model](docs/review-authority-threat-model.md).
- **Maintaining Gentle AI?** Use the [Codebase Guide](docs/CODEBASE-GUIDE.md) to find package ownership and review boundaries.
- **Using Pi?** Read [Pi Agent](docs/pi.md) for the `gentle-pi` harness, Pi commands, persona, and model assignments.
- **Ready to contribute?** Start at the [Community Roadmap](docs/community-roadmap.md) — everything labelled [`up-for-grabs`](https://github.com/Gentleman-Programming/gentle-ai/issues?q=is%3Aissue+is%3Aopen+label%3Aup-for-grabs) is scoped, approved and unclaimed. Then read [CONTRIBUTING.md](CONTRIBUTING.md).

---

<div align="center">
<a href="LICENSE"><img src="https://img.shields.io/badge/License-MIT-blue.svg" alt="License: MIT"></a>
</div>

> **Trademark notice:** The Gentle AI names and logos are trademarks of Alan Buscaglia. The MIT License applies to the code; it does not permit implying endorsement or official affiliation. See [TRADEMARKS.md](TRADEMARKS.md).
