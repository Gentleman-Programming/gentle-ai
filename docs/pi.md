# Pi integration

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
pi install npm:pi-mcp-adapter
npm exec --yes --package gentle-engram@latest -- pi-engram init
pi install npm:pi-web-access
pi install npm:pi-btw
```

| Component | Owner and purpose |
| --- | --- |
| `gentle-pi` (Gentle Shell) | Pi harness, ODD guidance, persona, models, skills, first-party clarification tool and delegation |
| `gentle-engram` | Pi session memory and Engram tools |
| `pi-mcp-adapter` | Pi MCP runtime, including Engram |
| `pi-engram init` | Initializes the Pi Engram MCP configuration |
| `pi-web-access`, `pi-btw` | Web access and companion workflow support |

Gentle AI no longer installs `npm:pi-subagents-j0k3r` or `npm:@juicesharp/rpiv-ask-user-question`: `gentle-pi` supplies their first-party replacements. Pi tool names are exclusive, so the latter package alongside `gentle-pi` can prevent Pi from loading. Existing entries are pruned from managed settings on the next install or sync. The retired `@juicesharp/rpiv-todo` entry is likewise removed; Gentle Todo ships with `gentle-pi`.

The installer preserves unrelated Pi settings and dependencies while provisioning Engram. It declares `npm:pi-mcp-adapter` in `.pi/agent/settings.json` and adds the `pi-mcp-adapter` dependency in `.pi/agent/npm/package.json`; `gentle-engram` owns the MCP schema initialized by `pi-engram init`. Set `PI_CODING_AGENT_DIR` before install or sync to redirect those agent-owned files, `mcp.json`, and `APPEND_SYSTEM.md` into an isolated Pi home instead of `~/.pi/agent`. The Pi package owns its commands and project-file layout; use its current package documentation for runtime-specific recovery, model overrides, and startup behavior. Starting Pi with `pi -ns` skips startup hooks and automatic refreshes.

## Optional CodeGraph

CodeGraph is an optional Gentle AI integration. When selected, Gentle AI merges its MCP entry without overwriting a conflicting user entry. Compatible Pi children receive tools or lazy-init guidance through managed overlays, not edits to package-owned child files. Guidance resolves a safe project root and initializes a missing index once; a stale index requires upstream recovery, not a claim that old graph results reflect current source. `gentle-ai sync` reconciles managed configuration, which is distinct from index freshness. Uninstall removes only manifest-owned entries and reports drifted child files instead of deleting them.

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
