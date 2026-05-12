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

| Command                       | When to use                                                                                 |
| ----------------------------- | ------------------------------------------------------------------------------------------- |
| `engram tui`                  | Browse your memories visually -- search, filter, drill into observations                    |
| `engram sync`                 | Export project memories to `.engram/` for git tracking. Run after significant work sessions |
| `engram sync --import`        | Import memories on another machine after cloning a repo with `.engram/`                     |
| `engram projects list`        | See all projects with observation counts                                                    |
| `engram projects consolidate` | Fix project name drift (e.g., "my-app" vs "My-App" vs "my-app-frontend")                    |
| `engram search <query>`       | Quick memory search from the terminal                                                       |

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

1. **The orchestrator keeps them focused.** The parent/orchestrator resolves the skill registry once, injects the relevant compact rules into each sub-agent prompt, and gives the child one concrete role. Sub-agents do not rediscover project skills or spawn more sub-agents during normal runtime.

2. **They adapt to your project.** A `sdd-apply` sub-agent working on a React project receives React patterns. The same sub-agent working on a Go project receives Go testing conventions. The rules depend on the registry and task context, not a hardcoded list.

3. **They persist their work.** Every sub-agent saves its artifacts to engram before returning. The next sub-agent in the pipeline can pick up exactly where the previous one left off, even across sessions.

This pattern works today on:

| Agent           | How sub-agents run                                                                                                              |
| --------------- | ------------------------------------------------------------------------------------------------------------------------------- |
| **OpenCode**    | Native sub-agent system — each phase is a dedicated agent with its own model, tools, and permissions defined in `opencode.json` |
| **Claude Code** | Via the Agent tool — the orchestrator spawns focused sub-agents and injects resolved project rules                              |
| **Others**      | SDD runs inline (single session) — the model follows the orchestrator instructions without spawning separate agents             |

You don't need to configure any of this. The installer sets it up, and the orchestrator manages delegation automatically.

### Delegation Stop Rules

The orchestrator must stop acting as a monolithic executor when complexity appears:

- **4-file rule**: reading 4+ files to understand a flow means delegate exploration or run an exploration phase.
- **Multi-file write rule**: touching 2+ non-trivial files means use one writer or require fresh review before completion.
- **PR rule**: before commit, push, or PR after code changes, run fresh review unless the diff is trivial docs/text.
- **Incident rule**: after wrong cwd, worktree/git accident, merge recovery, confusing test command, or environment workaround, run a fresh audit before continuing.
- **Long-session rule**: after roughly 20 tool calls, 5 exploratory reads, or 2 non-mechanical edits with growing complexity, pause and delegate, re-plan, or justify why not.
- **Fresh review rule**: use fresh context for adversarial review of diffs, conflicts, PR readiness, and incidents when the agent platform supports it.

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

1. **The registry refreshes at startup where the agent supports hooks.** Pi runs the `gentle-pi` session hook. Claude Code and OpenCode run `gentle-ai skill-registry refresh --quiet` from their installed startup/plugin hooks.
2. **The refresh is cached.** Gentle-AI fingerprints discovered `SKILL.md` files using schema version, path, mtime, and size. If `.atl/.skill-registry.cache.json` matches and `.atl/skill-registry.md` exists, startup is a cheap cache-hit.
3. **The orchestrator uses it automatically** -- once the registry exists, the orchestrator reads it at session start and passes pre-resolved compact rule text to sub-agents. You don't interact with the registry after that.
4. **Manual fallback stays available** -- run `gentle-ai skill-registry refresh --force` from a project if you want to regenerate immediately.

There's also an automated side: `sdd-init` runs the same registry logic internally, so if you use SDD in a new project, the registry gets built as part of that flow.

**Pro tip**: On Claude Code, OpenCode, and Pi you normally do not need to remember this. The startup hook refreshes the registry and the cache prevents unnecessary work.

---

## The Golden Rule

Gentle AI is an ecosystem **configurator**. It sets up your AI agent with memory, skills, workflows, and a persona -- then gets out of the way.

The less you think about gentle-ai after installing, the better it's working.

---

## Quick Reference

| Do                                                         | Don't                                                                             |
| ---------------------------------------------------------- | --------------------------------------------------------------------------------- |
| Run the installer, pick your agents and preset             | Manually edit the generated config files                                          |
| Just start coding with your AI agent                       | Memorize SDD phases or commands                                                   |
| Let the agent suggest SDD when a task is big enough        | Force SDD on every small task                                                     |
| Trust that engram is saving context for you                | Dig into engram's storage unless you need `engram sync` or `engram tui`           |
| Let startup hooks refresh the skill registry automatically | Manually rescan skills unless you need `gentle-ai skill-registry refresh --force` |
| Say "use sdd" if you know you want structured planning     | Worry about which SDD phase comes next                                            |
| Re-run the installer to update or change your setup        | Manually patch skill files or persona instructions                                |
