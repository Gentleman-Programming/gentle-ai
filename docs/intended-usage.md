# Intended Usage

<- [Back to README](../README.md)

---

This page explains how gentle-ai is meant to be used. Not the flags, not the architecture -- just the mental model. If you read one page besides the README, make it this one.

---

## After Installing -- You're Done

Once you run `gentle-ai` and select your agent(s), components, and preset, everything is configured. There is nothing else to do. No commands to memorize, no workflows to learn, no config files to edit.

Open your AI agent and start working. That's it.

---

## Engram (Memory) -- Automatic, But You CAN Use It

Engram is persistent memory for your AI agent. It saves decisions, discoveries, bug fixes, and context across sessions -- automatically. The agent manages all of it via MCP tools (`mem_save`, `mem_search`, etc.).

**Day-to-day: you don't need to do anything.** The agent handles memory automatically.

**But engram has useful tools when you need them:**

| Command | When to use |
|---------|-------------|
| `engram tui` | Browse your memories visually -- search, filter, drill into observations |
| `engram sync` | Export project memories to `.engram/` for git tracking. Run after significant work sessions |
| `engram sync --import` | Import memories on another machine after cloning a repo with `.engram/` |
| `engram projects list` | See all projects with observation counts |
| `engram projects consolidate` | Fix project name drift (e.g., "my-app" vs "My-App" vs "my-app-frontend") |
| `engram search <query>` | Quick memory search from the terminal |

Since v1.11.0, engram auto-detects the project name from git remote at startup, normalizes to lowercase, and warns if it finds similar existing project names. This prevents the name drift issue where the same project ends up with multiple name variants.

For full documentation: [github.com/Gentleman-Programming/engram](https://github.com/Gentleman-Programming/engram)

---

## SDD (Spec-Driven Development) -- It Happens Organically

SDD is a structured planning workflow for substantial features. It has phases (explore, propose, spec, design, implement, verify), but you do NOT need to learn any of them.

Here's how it actually works:

- **Small request?** The agent just does it. No ceremony.
- **Substantial feature?** The agent will suggest using SDD to plan it properly -- exploring the codebase, proposing an approach, designing the architecture, then implementing step by step.
- **Want SDD explicitly?** Just say "use sdd" or "hazlo con sdd" and the agent starts the workflow.

The agent handles all the phases internally. You just review and approve at key decision points.

If you want the project-level OpenSpec config convention SDD phases use for conventions, strict TDD, and testing metadata, see [OpenSpec Config for SDD](openspec-config.md).

---

## Multi-mode SDD (OpenCode SDD Profiles)

Multi-mode lets you assign different AI models to different SDD phases -- for example, a powerful model for design and a faster one for implementation. This is an OpenCode-exclusive feature, managed through **SDD Profiles**.

For **all other agents** (Claude Code, Cursor, Gemini CLI, VS Code Copilot), SDD runs in single-mode automatically. One model handles everything, and that works perfectly fine.

If you want multi-mode in OpenCode:

1. Connect your AI providers in OpenCode first
2. Create a profile via gentle-ai TUI ("OpenCode SDD Profiles") or CLI (`--profile` flag)
3. The base/default SDD conductor is `gentle-orchestrator`
4. Named profiles generate `sdd-orchestrator-{name}` + suffixed sub-agents, each assigned to your chosen model
5. In OpenCode, press **Tab** to switch between `gentle-orchestrator` and custom profiles

You can create multiple profiles (e.g., "cheap" for experimentation, "premium" for production) and switch between them freely.

If you prefer a **runtime profile manager** that keeps profiles outside `opencode.json`, gentle-ai now supports that too. During sync, OpenCode can auto-detect external profile files under `~/.config/opencode/profiles/*.json` and switch to a safer compatibility path that preserves the active `gentle-orchestrator` prompt instead of overwriting it.

**Full step-by-step guide**: [OpenCode SDD Profiles](opencode-profiles.md)

---

## Sub-Agents -- Smarter Than You Think

When the orchestrator delegates work to a sub-agent (say, `sdd-explore` to investigate a codebase), that sub-agent is not a dumb executor running a single script. It's a full agent with its own session, tools, and context.

What makes them "super sub-agents":

1. **They discover skills on their own.** Each sub-agent's first action is to search for the skill registry -- via engram memory or the local `.atl/skill-registry.md` file. If it finds relevant skills (React patterns, Go testing, Angular architecture, etc.), it loads and follows them. The orchestrator doesn't need to spoon-feed skill paths.

2. **They adapt to your project.** A `sdd-apply` sub-agent working on a React project will load React 19 patterns. The same sub-agent working on a Go project will load Go testing conventions. The skills it loads depend on what the registry says is relevant, not a hardcoded list.

3. **They persist their work.** Every sub-agent saves its artifacts to engram before returning. The next sub-agent in the pipeline can pick up exactly where the previous one left off, even across sessions.

This pattern works today on:

| Agent | How sub-agents run |
|-------|-------------------|
| **OpenCode** | Native sub-agent system — each phase is a dedicated agent with its own model, tools, and permissions defined in `opencode.json` |
| **Claude Code** | Via the Agent tool — the orchestrator spawns sub-agents that self-discover skills from the registry |
| **Others** | SDD runs inline (single session) — the model follows the orchestrator instructions without spawning separate agents |

You don't need to configure any of this. The installer sets it up, and the orchestrator manages delegation automatically.

---

## Skills -- Two Layers

gentle-ai installs **SDD skills** and **foundation skills** (workflow, testing patterns) directly into your agent's skills directory. These are embedded in the binary and always up to date.

For **coding skills** (React 19, Angular, TypeScript, Tailwind, Zod, Playwright, etc.), the community maintains a separate repository: [Gentleman-Programming/Gentleman-Skills](https://github.com/Gentleman-Programming/Gentleman-Skills). You install those manually by cloning the repo and copying the skills you want:

```bash
git clone https://github.com/Gentleman-Programming/Gentleman-Skills.git
cp -r Gentleman-Skills/curated/react-19 ~/.claude/skills/
cp -r Gentleman-Skills/curated/typescript ~/.claude/skills/
# ... or copy the entire curated/ directory
```

Once installed, your agent detects what you're working on and loads the relevant skills automatically. You don't need to activate or invoke them.

**The skill registry.** The skill registry is a catalog of all available skills that the orchestrator reads once per session to know what's available and where. It needs to run **inside each project** you work on, because it also scans for project-level conventions (like `CLAUDE.md`, `agents.md`, `.cursorrules`, etc.).

How it works:

1. **Run `/skill-registry` inside your project** -- it scans all your installed skills (user-level and project-level), reads their frontmatter, and builds a registry at `.atl/skill-registry.md`. If engram is available, it also saves the registry to memory for cross-session access.
2. **The orchestrator uses it automatically** -- once the registry exists, the orchestrator reads it at session start and passes pre-resolved skill paths to sub-agents. You don't interact with the registry after that.
3. **Re-run it when things change** -- any time you add, remove, or modify a skill, run `/skill-registry` again so the orchestrator picks up the changes.

There's also an automated side: `sdd-init` runs the same registry logic internally, so if you use SDD in a new project, the registry gets built as part of that flow.

**Pro tip**: If you find yourself updating skills often, you can create a skill (using `/skill-creator`) that automatically triggers a registry update after skill changes -- that way you never have to think about it.

---

## The Golden Rule

Gentle AI is an ecosystem **configurator**. It sets up your AI agent with memory, skills, workflows, and a persona -- then gets out of the way.

The less you think about gentle-ai after installing, the better it's working.

---

## Quick Reference

| Do | Don't |
|----|-------|
| Run the installer, pick your agents and preset | Manually edit the generated config files |
| Just start coding with your AI agent | Memorize SDD phases or commands |
| Let the agent suggest SDD when a task is big enough | Force SDD on every small task |
| Trust that engram is saving context for you | Dig into engram's storage unless you need `engram sync` or `engram tui` |
| Run `/skill-registry` after installing or changing skills | Forget to update the registry after adding new skills |
| Say "use sdd" if you know you want structured planning | Worry about which SDD phase comes next |
| Re-run the installer to update or change your setup | Manually patch skill files or persona instructions |
