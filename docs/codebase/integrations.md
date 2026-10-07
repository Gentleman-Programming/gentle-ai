# Integrations

> [!NOTE]
> These docs track `main`, which may include unreleased changes. For the latest release, see the [v4.0.0 docs](https://github.com/Gentleman-Programming/gentle-ai/tree/v4.0.0/docs).

[Back to Codebase Guide](../CODEBASE-GUIDE.md)

Gentle-AI integration code should stay thin: adapters describe where and how an agent accepts configuration; components decide what managed content to inject.

## Agent integration map

| Integration area | Source owner | Purpose |
|---|---|---|
| Agent IDs and config roots | `internal/model/types.go`, `internal/catalog/agents.go` | Declare supported agent names and roots. |
| Adapter strategies | `internal/agents/<agent>/` | Return path, MCP strategy, prompt strategy, and capabilities. |
| ODD guidance and review assets | `internal/assets/<agent>/`, `internal/components/agentguidance/`, `internal/components/reviewassets/` | Deliver shared ODD guidance and managed reviewer contracts/agents through agent-specific adapters. |
| Engram MCP | `internal/components/engram/` | Add external Engram MCP server entries. |
| Context7 MCP | `internal/components/mcp/` | Add documentation MCP server entries. |
| Skills | `internal/components/skills/`, `internal/assets/skills/` | Copy curated skill files. |
| Skill registry | `internal/skillregistry/`, `internal/app/` | Refresh or list `.atl/skill-registry.md` entries. |
| Community tools | `internal/components/communitytool/` | Orchestrate community tool installation plus managed guidance/config/MCP reconciliation; do not own external runtime implementation. |
| OpenCode plugins | `internal/components/opencodeplugin/` | Register external OpenCode plugin package names; Gentle Logo also writes a managed local TUI plugin file and registers its path. |

## Setup boundaries

| Boundary | Rule |
|---|---|
| Binary installation | Install only external tools this component owns, such as Engram or GGA. |
| Agent discovery | Detect config roots or binaries through system/adapters; do not hard-code in UI screens. |
| MCP wiring | Use adapter MCP strategy instead of custom JSON writes in feature code. |
| Prompt injection | Use component/filemerge helpers to preserve user content when strategy requires it. |
| Community tool orchestration | Keep install commands, generated guidance/config, and MCP reconciliation thin and traceable to the selected external tool. |
| Plugin registration | Add external package names or managed local plugin paths; let OpenCode load them at runtime. |

## Community tools vs OpenCode plugins

Community tools and OpenCode plugins are different integration paths:

| Path | Gentle-AI owns | Runtime owner |
|---|---|---|
| `internal/components/communitytool/` | Installation orchestration plus managed guidance/config/MCP reconciliation, such as CodeGraph setup and guidance. | The external tool runtime. |
| `internal/components/opencodeplugin/` | Gentle Logo local TUI plugin file and registration; uninstall-only cleanup of legacy external plugin registrations. | OpenCode and the managed local plugin file. |

## Thin plugin principle

Gentle-AI no longer installs or upgrades external OpenCode community plugins (`sub-agent-statusline`, `sdd-engram-plugin`). They are unmaintained and do not work on OpenCode V2. `gentle-ai uninstall opencode-plugin <id>` still removes registrations left by older installations.

For Gentle Logo, Gentle-AI writes the managed local TUI plugin file under `~/.config/opencode/tui-plugins/` and registers that path in `~/.config/opencode/tui.json`. OpenCode owns runtime loading.

## Contributor checklist

- [ ] Add or update an adapter before adding special cases to components.
- [ ] Keep component behavior reusable across agents.
- [ ] Add golden tests when generated config changes.
- [ ] Update [Agents](../agents.md) for user-visible agent capabilities.
- [ ] Keep optional community integrations thin and reversible.
- [ ] Do not mix community tool guidance with OpenCode plugin package registration.

## Navigation

Previous: [Dashboard](dashboard.md) | Next: [Maintainer playbook](maintainer-playbook.md)
