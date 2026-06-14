# Hermes Delegation Tuning Knobs

Configure these in `~/.hermes/config.yaml` under the `delegation` key, or pass per `delegate_task` call.

| Parameter | Default | Effect |
|-----------|---------|--------|
| `max_spawn_depth` | 2 | Maximum recursive delegation depth. Set to 1 to prevent workers from spawning their own workers. |
| `max_concurrent_children` | 4 | Maximum number of parallel workers active at once. |
| `max_iterations` | agent default | Iteration budget for each worker before it is forced to return. |
| `child_timeout_seconds` | agent default | Hard wall-clock timeout per worker. |
| `inherit_mcp_toolsets` | false | When true, workers inherit the parent's MCP toolsets automatically. When false (default), pass toolsets explicitly in the mission. |
| `subagent_auto_approve` | false | When true, workers auto-approve all tool calls. When false (default), workers prompt for approval on dangerous calls. |

## Mission Checklist

When `inherit_mcp_toolsets` is false (the default), every mission passed to `delegate_task` MUST explicitly list:

- Which toolsets the worker is allowed to use (file read, shell, browser, etc.)
- Which MCP servers the worker can call (engram, context7, etc.)
- Which `SKILL.md` paths the worker must load before starting work

Without this, workers start with no tools beyond their built-in defaults.
