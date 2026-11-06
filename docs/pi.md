# Pi integration

> [!NOTE]
> These docs track `main`, which may include unreleased changes. For the latest release, see the [v4.0.0 docs](https://github.com/Gentleman-Programming/gentle-ai/tree/v4.0.0/docs).

← [Back to README](../README.md)

Gentle AI configures Pi support, but the separate Gentle Shell (`gentle-pi`) package owns Pi's runtime prompts, persona, model assignments, delegation, and ODD behavior. Installing or syncing Gentle AI alone does not establish behavior parity with that package. [ODD](usage.md#organic-driven-development-odd) is the development workflow.

## Start

Install Pi separately and make sure `pi` is available on `PATH`, then run:

```bash
gentle-ai install --agent pi
pi
```

Gentle AI provisions the Pi companion package stack and Engram integration; it does not install Pi itself. The installer does not write Pi's system prompt: Gentle Shell owns that surface. Existing older Gentle AI managed prompt blocks are removed on install or sync without replacing unrelated user content. Pi-only installation leaves persona and model selection to Gentle Shell.

Gentle AI runs these setup steps:

```bash
pi install npm:gentle-pi
pi install npm:gentle-engram
pi install npm:pi-web-access
pi install npm:pi-btw
```

| Component | Owner and purpose |
| --- | --- |
| `gentle-pi` (Gentle Shell) | Pi harness, ODD guidance, persona, models, skills, first-party clarification tool and delegation |
| `gentle-engram` | Pi session memory and Engram tools |
| `pi-web-access`, `pi-btw` | Web access and companion workflow support |

Pi's package manager is the sole owner of Engram registration; Gentle AI does not run `pi-engram init`, which would add a second, version-pinned declaration. Install and sync repair unambiguous existing Engram duplicates: an identical declaration is kept once, and a bare string is removed when one distinct pinned or object declaration exists. Before package installation, Gentle AI normalizes compatible Engram duplicates and installs the retained source, so Pi does not overwrite an existing pin with the bare source. The retained declaration's version and object options are preserved. Conflicting declarations or malformed settings abort CLI and TUI installation at read-only admission, before runtime construction, commands, or configuration changes. Resolve the reported error before installing. Sync leaves conflicting declarations unchanged for manual resolution. Other packages are not deduplicated.

Gentle AI no longer installs `npm:pi-subagents-j0k3r` or `npm:@juicesharp/rpiv-ask-user-question`: `gentle-pi` supplies their first-party replacements. Pi tool names are exclusive, so the latter package alongside `gentle-pi` can prevent Pi from loading. Existing entries are pruned from managed settings on the next install or sync. The retired `@juicesharp/rpiv-todo` entry is likewise removed; Gentle Todo ships with `gentle-pi`.

Engram on Pi uses the native tools `gentle-engram` registers, not MCP, so Gentle AI never adds an `engram` server to `.pi/agent/mcp.json` and post-sync verification does not require that file; an `engram` entry you added yourself is left alone. Pi's built-in MCP support (Pi >= 0.99.0) runs other MCP servers, such as CodeGraph, from `mcp.json`. Gentle AI no longer installs `pi-mcp-adapter`: an installed extension that registers `/mcp` replaces Pi's built-in MCP support, so install and sync remove a previously installed `npm:pi-mcp-adapter` entry from `.pi/agent/settings.json` and its dependency from `.pi/agent/npm/package.json`, preserving unrelated Pi settings and dependencies. Install and sync also migrate servers from a legacy `.pi/agent/mcp-adapter.json` (the file `pi-mcp-adapter` 3.x read) into `mcp.json`, creating it only when there is a server to copy and never overwriting entries already in `mcp.json`. `mcp-adapter.json` is left in place, and a malformed `mcp-adapter.json`, or a malformed `mcp.json` that servers must be copied into, stops the run instead of being overwritten. Uninstall still lists `pi remove npm:pi-mcp-adapter` for older installs. Set `PI_CODING_AGENT_DIR` before install or sync to redirect those agent-owned files, `mcp.json`, and `APPEND_SYSTEM.md` into an isolated Pi home instead of `~/.pi/agent`. The Pi package owns its commands and project-file layout; use its current package documentation for runtime-specific recovery, model overrides, and startup behavior. Starting Pi with `pi -ns` skips startup hooks and automatic refreshes.

When Pi is selected for installation, Gentle AI snapshots its global settings regardless of the selected components or install scope. Rollback permits restoring snapshotted files inside its resolved agent directory (`PI_CODING_AGENT_DIR`, when set), even outside the home or workspace. This does not authorize restoring sibling paths or files outside the existing restore guards. An unselected Pi directory adds neither a settings snapshot target nor rollback authority.

## MCP disabled after migration

`sync` preserves `extensions: ["-builtin:mcp"]`: the adapter's onboarding marker does not distinguish a setting it created from an intentional user choice. When `pi-mcp-adapter` is absent and `mcp.json` contains servers, `sync` warns and `gentle-ai doctor` reports `pi:mcp` as a warning instead of silently treating those servers as available. These diagnostics use `PI_CODING_AGENT_DIR` when configured.

If you want those servers enabled, remove `-builtin:mcp` from `extensions` in the `settings.json` path shown by the warning, preserving other entries. Restart Pi, then run `gentle-ai doctor`. If MCP is intentionally disabled, leave the setting in place; neither command re-enables it automatically. Engram's native Pi tools are independent of this MCP setting.

An adapter package with `extensions: []` is inactive for this diagnostic, even if `npm/package.json` still lists its dependency; the warning identifies it as inactive rather than absent. Configuration that cannot be read, including a dangling symlink, produces an inspection warning. Genuinely absent configuration files remain optional.

## Optional CodeGraph

CodeGraph is an optional Gentle AI integration. When selected, Gentle AI merges its MCP entry without overwriting a conflicting user entry. Compatible Pi children receive tools or lazy-init guidance through managed overlays, not edits to package-owned child files. Guidance resolves a safe project root and initializes a missing index once; a stale index requires upstream recovery, not a claim that old graph results reflect current source. `gentle-ai sync` reconciles managed configuration, which is distinct from index freshness. Sync never recreates a child file that was deleted; it drops that file from its ownership record. Gentle AI also stops managing the retired SDD agents (`sdd-*.md`) in the Pi agent home: when one still holds exactly Gentle AI's recorded overlay, sync restores the bytes it had before the overlay so the Pi package can retire it, and leaves any edited copy untouched. Uninstall removes only manifest-owned entries and reports drifted child files instead of deleting them.

Index freshness depends on the intelligence surface:

- **MCP:** auto-sync requires a running daemon with an active file watcher. If the watcher is disabled or stale files do not refresh, run `codegraph sync -q <project-root>`.
- **CLI:** intelligence commands read the existing index without auto-sync. Run `codegraph sync -q <project-root>` before intelligence reads, including after edits. If sync fails, use filesystem tools and explain the failure instead of treating stale graph results as current.

The presence of `.codegraph/` alone does not guarantee freshness.

When the direct CodeGraph MCP capability is verified but Pi adapter activation health cannot be machine-verified, Community Tools reports Pi as `pending`, not `missing` or `configured`. Pending agents are counted separately from missing wiring. This validated pending state preserves the verified MCP capability in the reconciliation result and satisfies installation reconciliation. Rerunning setup can take the already-reconciled path while preserving the pending health guidance; its summary says configuration is reconciled and Pi activation health remains pending, not that every agent is configured. Missing configuration, failed capability probes, and invalid child guidance still report `missing` and do not satisfy reconciliation.

## Review and checks

Strict TDD follows the resolved configuration and exact test runner: observe RED, GREEN and REFACTOR when enabled; otherwise run applicable functional checks. RDD is separate and controlled by the user's `gentle-ai review mode status`, `gentle-ai review mode enable`, and `gentle-ai review mode disable` choices. Candidate consent and native authority do not authorize commits or releases. The review execution contract is provided to Pi through the provider bundle and mirrored by Gentle Shell, not by writing a Gentle AI system prompt block. See [Review](review-integration.md).

## Gentle Shell and its own home

[Gentle Shell](https://www.npmjs.com/package/gentle-pi) is a standalone launcher for Pi. By default it uses its own isolated Pi agent home, `~/.gentle-shell/agent`; `gentle-shell --link` uses `~/.pi/agent` live. On first run and whenever its pinned Gentle AI version changes, Gentle Shell provisions the isolated home with its pinned `gentle-ai install --agent pi --scope global`. `gentle-shell setup` reruns provisioning. Credentials are not copied between homes; a newly provisioned home needs its own `/login`.

`PI_CODING_AGENT_DIR` redirects agent-owned install/sync files; it does not move Pi's `~/.pi` config root. Persona selection, background-subagent policy, uninstall targets, skill-registry scanning and Pi config detection may still resolve against `~/.pi` in an isolated Gentle Shell home. Preview changes with `gentle-ai sync --dry-run`, then run `gentle-ai sync`. Uninstall backs up managed configuration and preserves unrelated user data; it does not uninstall Pi.

## Next steps

- [Supported Agents](agents.md) lists the integrations.
- [Engram Commands](engram.md) describes persistent memory.
- [Usage](usage.md) covers the CLI and TUI.

← [Back to README](../README.md)
