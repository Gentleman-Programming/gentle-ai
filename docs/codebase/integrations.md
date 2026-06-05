# Integrations

[Back to Codebase Guide](../CODEBASE-GUIDE.md)

Gentle-AI integration code should stay thin: adapters describe where and how an agent accepts configuration; components decide what managed content to inject.

## Agent integration map

| Integration area | Source owner | Purpose |
|---|---|---|
| Agent IDs and config roots | `internal/model/types.go`, `internal/catalog/agents.go` | Declare supported agent names and roots. |
| Adapter strategies | `internal/agents/<agent>/` | Return path, MCP strategy, prompt strategy, and capabilities. |
| SDD assets | `internal/assets/<agent>/`, `internal/components/sdd/` | Install orchestrators, sub-agent prompts, and commands. |
| Engram MCP | `internal/components/engram/` | Add external Engram MCP server entries. |
| Context7 MCP | `internal/components/mcp/` | Add documentation MCP server entries. |
| Skills | `internal/components/skills/`, `internal/assets/skills/` | Copy curated skill files. |
| Community plugins | `internal/components/opencodeplugin/` | Register OpenCode plugin package names only. |

## Setup boundaries

| Boundary | Rule |
|---|---|
| Binary installation | Install only external tools this component owns, such as Engram or GGA. |
| Agent discovery | Detect config roots or binaries through system/adapters; do not hard-code in UI screens. |
| MCP wiring | Use adapter MCP strategy instead of custom JSON writes in feature code. |
| Prompt injection | Use component/filemerge helpers to preserve user content when strategy requires it. |
| Plugin registration | Add plugin package names; let OpenCode install/load plugin packages. |

## Thin plugin principle

OpenCode community plugins are optional integrations. Gentle-AI only ensures `~/.config/opencode/tui.json` exists and contains the plugin package name. It does not vendor, execute, or own plugin runtime code.

```text
TUI selection
  -> opencodeplugin.Install
  -> ensure ~/.config/opencode/tui.json
  -> append package name to plugin array
  -> OpenCode owns runtime loading later
```

## Contributor checklist

- [ ] Add or update an adapter before adding special cases to components.
- [ ] Keep component behavior reusable across agents.
- [ ] Add golden tests when generated config changes.
- [ ] Update [Agents](../agents.md) for user-visible agent capabilities.
- [ ] Keep optional plugin code thin and reversible.

## Navigation

Previous: [Dashboard](dashboard.md) | Next: [Maintainer playbook](maintainer-playbook.md)
