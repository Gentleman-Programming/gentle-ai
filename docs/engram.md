# Engram Command Reference

<- [Back to README](../README.md)

---

Engram works automatically. Your AI agent saves decisions, discoveries, and context to persistent memory without you doing anything. You do not need to memorize commands or manage memory manually.

This page exists for when you want to inspect, share, or fix your memories by hand.

---

## Day-to-Day Commands

These are the only commands most people ever need.

```bash
# Browse memories visually -- search, filter, drill into observations
engram tui

# Search from the terminal without opening the TUI
engram search "auth refactor"

# Export project memories to .engram/ so you can commit them to git
engram sync
```

`engram tui` is the fastest way to see what your agent has been saving. Start there.

---

## Project Management

Engram groups memories by project name, auto-detected from your git remote since v1.11.0. Sometimes projects end up with duplicate names (e.g., "my-app" vs "My-App" vs "my-app-frontend"). These commands fix that.

```bash
# List all projects with observation counts
engram projects list

# Interactively merge duplicate project names into one
engram projects consolidate
```

`projects list` shows every project engram knows about and how many observations each has. If you see the same project under multiple names, run `projects consolidate` to merge them.

The MCP equivalent is `mem_merge_projects`, which the AI agent can call directly when it detects name drift.

---

## Team Sharing

Engram memories live locally by default. To share them with your team via git:

```bash
# After a work session -- export memories to .engram/ in your repo
engram sync

# On another machine -- import memories after cloning
engram sync --import
```

Add `.engram/` to your repo and commit it. When a teammate clones and runs `engram sync --import`, they get the full project context. This is especially useful for onboarding -- new contributors start with the accumulated knowledge of the team.

---

## MCP Tools Reference

These are the tools the AI agent uses behind the scenes. You never call them directly, but understanding them helps you know what your agent is doing.

### Core Tools

| Tool | What it does |
|------|--------------|
| `mem_save` | Saves a decision, bug fix, discovery, or convention to memory |
| `mem_search` | Searches memory by keywords -- returns matching observations |
| `mem_context` | Gets recent session history (called at session start) |
| `mem_session_summary` | Saves an end-of-session summary so the next session has context |
| `mem_get_observation` | Retrieves full untruncated content of a specific observation by ID |
| `mem_save_prompt` | Saves the user's prompt for additional context |

### Advanced Tools

<details>
<summary>Click to expand -- rarely needed, but available</summary>

| Tool | What it does |
|------|--------------|
| `mem_update` | Updates an existing observation by ID |
| `mem_suggest_topic_key` | Suggests a stable topic key for evolving topics |
| `mem_session_start` / `mem_session_end` | Session lifecycle management |
| `mem_stats` | Memory statistics (observation count, project breakdown) |
| `mem_delete` | Deletes an observation by ID |
| `mem_timeline` | Chronological view of observations |
| `mem_capture_passive` | Extracts learnings from conversation passively |
| `mem_merge_projects` | Merges project name variants (CLI equivalent: `engram projects consolidate`) |

</details>

---

## How Project Detection Works

Since v1.11.0, engram reads the git remote URL at startup, normalizes it to lowercase, and uses that as the project name. If it finds similar existing project names, it warns you. This prevents the most common issue -- the same project accumulating memories under slightly different names.

If you're working outside a git repo, engram falls back to the directory name.

---

## Full Documentation

For the complete source, configuration options, and contribution guide: [github.com/Gentleman-Programming/engram](https://github.com/Gentleman-Programming/engram)
