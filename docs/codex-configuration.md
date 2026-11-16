# Codex multi-agent configuration

Gentle AI configures Codex collaboration through the current `[agents]` table.
It no longer writes the deprecated `[features].multi_agent` switch.

## Current behavior

```toml
[agents]
enabled = true
max_threads = 4
max_depth = 2
```

Fresh installs enable agents. An explicit `agents.enabled` value is always
preserved, including `false`.

During sync, Gentle AI removes the deprecated `features.multi_agent` key in
the format earlier Gentle AI versions generated. A
legacy `multi_agent = false` value migrates to `agents.enabled = false` only
when the current key is absent. Unrelated feature, profile, MCP, and permission
settings remain untouched. Thread and depth limits retain Gentle AI's existing
managed defaults.

## V2 caveat

Gentle AI does not add, remove, or change `features.multi_agent_v2`. Codex can
expose collaboration tools through an explicit V2 setting even when
`agents.enabled = false`, so the live `spawn_agent`, `wait_agent`, and
`list_agents` tool surface is the authoritative capability check.
