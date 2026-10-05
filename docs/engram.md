# Engram™ Command Reference

> [!NOTE]
> These docs track `main`, which may include unreleased changes. For the latest release, see the [v4.0.0 docs](https://github.com/Gentleman-Programming/gentle-ai/tree/v4.0.0/docs).

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

## Cloud Sync (Optional)

Engram Cloud is optional replication for people who want project memories to follow them across machines they own. Local SQLite memory remains the default and authoritative source. Gentle AI ships the Engram client, but the cloud runtime and server lifecycle are owned by Engram upstream.

Use this only when you already have an Engram Cloud server URL and token.

### Quick path

```bash
# Persist the cloud server URL in ~/.engram/cloud.json
engram cloud config --server https://your-cloud-server.example

# Keep the token out of repos and docs; provide it through your user environment
export ENGRAM_CLOUD_TOKEN=<your-token>

# Confirm the local client can reach the configured server
engram cloud status

# Enroll one project explicitly, then run the first cloud sync
engram cloud enroll <project-name>
engram sync --cloud --project <project-name>
```

On each additional machine, configure the same server and token, enroll the project, and import existing cloud memories:

```bash
engram cloud enroll <project-name>
engram sync --cloud --import --project <project-name>
```

To let Engram's own runtime attempt background cloud sync, set autosync in the environment used to launch that runtime:

```bash
export ENGRAM_CLOUD_AUTOSYNC=1
```

`ENGRAM_CLOUD_SERVER` can also provide the server URL at runtime, but `engram cloud config --server ...` is easier to inspect and repeat.

### Environment carriers

| Setup | Environment carrier |
|---|---|
| macOS GUI sessions | `launchctl setenv ENGRAM_CLOUD_TOKEN <token>` and `launchctl setenv ENGRAM_CLOUD_AUTOSYNC 1` |
| Linux systemd user sessions | `systemctl --user import-environment ENGRAM_CLOUD_TOKEN ENGRAM_CLOUD_AUTOSYNC` after exporting them in the current shell |
| Shell-only use | `export ENGRAM_CLOUD_TOKEN=<token>` and `export ENGRAM_CLOUD_AUTOSYNC=1` in your shell profile |

The macOS and Linux manager commands update the current launchd or systemd user-manager environment. Reapply them after that manager restarts or after reboot, unless you configure a persistent service environment for the Engram process.

Treat `ENGRAM_CLOUD_TOKEN` like any other credential: do not commit it, paste it into issue reports, or put it in project-local scripts. Environment variables are inherited by child processes, so use the narrowest carrier that fits how you launch Engram.

### Verify and repair

```bash
# Readiness for the configured cloud connection
engram cloud status

# Inspect cloud-sync state for one project
engram sync --cloud --status --project <project-name>

# Diagnose and repair upgrade issues for one project
engram cloud upgrade doctor --project <project-name>
engram cloud upgrade repair --project <project-name> --dry-run
engram cloud upgrade repair --project <project-name> --apply
```

### What Gentle AI does not manage

- It does not provision or operate an Engram Cloud server.
- It does not make cloud sync mandatory; local memory is still the default.
- It does not replace git-based team sharing via `engram sync` and `.engram/`.

Full upstream docs: [Engram Cloud](https://github.com/Gentleman-Programming/engram/blob/main/docs/engram-cloud/README.md) and [Engram cloud CLI reference](https://github.com/Gentleman-Programming/engram/blob/main/DOCS.md#cloud-cli-opt-in).

---

## MCP Tools Reference

These are the tools the AI agent uses behind the scenes. You never call them directly, but understanding them helps you know what your agent is doing.

### Core Tools

| Tool | What it does |
|------|--------------|
| `mem_save` | Saves a decision, bug fix, discovery, or convention to memory. Engram v1.15.3+ captures the user prompt best-effort by default when prompt context was already fed for the same project/session |
| `mem_search` | Searches memory by keywords -- returns matching observations |
| `mem_context` | Gets recent session history (called at session start) |
| `mem_session_summary` | Saves an end-of-session summary so the next session has context |
| `mem_get_observation` | Retrieves full untruncated content of a specific observation by ID |
| `mem_save_prompt` | Saves the user's prompt and feeds session activity so a later `mem_save` can capture/dedupe it |

`mem_save` accepts optional `capture_prompt`. Leave it unset for normal human/proactive saves. Use `capture_prompt: false` only for automated artifacts such as testing-capabilities caches, onboarding/state artifacts, or skill-registry output. If the MCP server has no prompt context, `mem_save` still succeeds and does not invent prompt text.

Agents or plugin hooks that can observe the user's prompt should call `mem_save_prompt` before any derived `mem_save` calls so Engram can attach and dedupe the real prompt context.

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
