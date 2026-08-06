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

## Cloud Sync (Optional)

Sync your engram memories across machines through a self-hosted cloud server, so the same observations are available wherever your agent runs.

Cloud sync runs **in-process** inside the `engram` binary -- any subcommand that holds the store open (e.g. `serve`, `mcp`, `protocol-mode`). It activates when three environment variables are set before engram starts. Without them, engram behaves exactly as it does today: local-only, with git-based team sharing via `engram sync`.

### Required environment variables

| Variable | Purpose |
|----------|---------|
| `ENGRAM_CLOUD_AUTOSYNC="1"` | Turns on the background sync loop (must be exactly the string `1` -- not `true`, not `yes`, not `1` as integer) |
| `ENGRAM_CLOUD_TOKEN` | Bearer token. Source of truth is `~/.engram/cloud.json` (loaded first); this env var overrides the file when set. Used directly when neither is present only as a fallback. |
| `ENGRAM_CLOUD_SERVER` | URL of the cloud server you self-host. Persist with `engram cloud config --server <url>` (writes `~/.engram/cloud.json`); this env var overrides the file when set. |

All three must be present **before the engram processes start**. Setting them after engram is already running has no effect on the current processes; restart is required.

### Setup (any platform)

```bash
# 1. Persist the server URL to cloud.json (skippable if you prefer env-only)
engram cloud config --server https://your-server

# 2. Export the two runtime variables
export ENGRAM_CLOUD_AUTOSYNC="1"
export ENGRAM_CLOUD_TOKEN=***

# 3. Restart engram so the new environment is picked up
pkill -f "engram mcp"
pkill -f "engram serve"

# 4. Verify
engram cloud status
```

For shell rc files (~/.zshrc, ~/.bashrc), the variables load on every new shell but not in already-running services. For long-running services that don't inherit your shell environment:

```bash
# macOS (launchd user services)
launchctl setenv ENGRAM_CLOUD_AUTOSYNC "1"
launchctl setenv ENGRAM_CLOUD_TOKEN YOUR_TOKEN_HERE
launchctl setenv ENGRAM_CLOUD_SERVER https://your-server

# Linux (systemd user services)
systemctl --user import-environment ENGRAM_CLOUD_AUTOSYNC ENGRAM_CLOUD_TOKEN ENGRAM_CLOUD_SERVER
```

### Running as a service (systemd, launchd, Task Scheduler)

The upstream engram docs publish ready-to-copy templates for the three supported service supervisors. If you want `engram serve` to survive reboots and `brew upgrade`, follow the upstream setup for your platform -- the templates set the cloud env vars in the service definition so autosync stays alive across restarts:

- [systemd (Linux user service)](https://github.com/Gentleman-Programming/engram/blob/main/DOCS.md#using-systemd-linux)
- [launchd (macOS LaunchAgent plist)](https://github.com/Gentleman-Programming/engram/blob/main/DOCS.md#using-launchd-macos)
- [Windows Task Scheduler](https://github.com/Gentleman-Programming/engram/blob/main/DOCS.md#using-windows-task-scheduler)

On Windows, Task Scheduler does **not** inherit session environment variables. `ENGRAM_CLOUD_TOKEN`, `ENGRAM_CLOUD_SERVER`, and `ENGRAM_CLOUD_AUTOSYNC` must be set as persistent user or system environment variables (Control Panel -> System -> Advanced -> Environment Variables), not via `$env:` in a PowerShell session or `export` in a shell profile. The token can also be stored in `~/.engram/cloud.json` so the scheduled task picks it up without needing session env vars at all.

### Verify it's working

```bash
engram cloud status
```

`engram cloud status` is a multi-line report. When everything is set you see:

```
Cloud status: configured (target=cloud)
Server: https://your-server
Auth status: ready (token provided via runtime cloud config)
Sync readiness: ready for explicit --project sync (project must be enrolled)
Local daemon: running on port 7437
```

The `Sync readiness: ready` line confirms the three variables are set, the server URL is reachable, and a token is available. If anything is missing or the server is unreachable, the command warns without blocking local-only operation.

### Security

- **Never commit the token** -- keep it in your shell rc file, a user-level env store, or a secrets manager. Not in `.env` files that get committed, not in version-controlled config, not in plist or unit files checked into git.
- **Token is per-agent** -- generate a separate token per machine or per service. Rotate by exporting a new value and restarting the engram processes, or by replacing the token inside `~/.engram/cloud.json`.
- **Server URL is yours** -- `ENGRAM_CLOUD_SERVER` points at a server you control. The official binary does not phone home to any default endpoint; without the three variables set, no network traffic leaves your machine.
- **`cloud.json` stores the server URL and can store the token** -- the token field in `~/.engram/cloud.json` is read first by `resolveCloudRuntimeConfig`, with `ENGRAM_CLOUD_TOKEN` overriding it when set. This is what makes Windows Task Scheduler work (the env var is often absent there -- see issues #343 and #421 upstream).

### What this does and does not replace

- **Adds** background sync of observations across machines pointing at the same cloud server.
- **Composes with** git-based team sharing (`engram sync` to `.engram/` then commit) -- they solve different problems. Cloud sync keeps your personal machine states aligned; git sharing keeps team knowledge versioned and reviewable.
- **Does not replace** local storage. If the cloud server is unreachable, engram keeps working locally and syncs when connectivity returns.
- **Does not include** scripts, install-time hooks, or `brew tap` automation. Those belong in engram's own `scripts/` directory if maintainers ever want them -- for now, the env vars and `engram cloud config` are the entire activation surface.

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

`mem_save` accepts optional `capture_prompt`. Leave it unset for normal human/proactive saves. Use `capture_prompt: false` only for automated artifacts such as SDD proposal/spec/design/tasks/apply/verify/archive/init reports, testing-capabilities caches, onboarding/state artifacts, or skill-registry output. If the MCP server has no prompt context, `mem_save` still succeeds and does not invent prompt text.

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
