# Login-free Claude subagent tool proof

`TestClaudeMarkdownSubagentsEnforceEmptyTools` exercises Markdown subagents through the real Claude Code `Agent` tool against a scripted local Messages API. It needs Claude Code **2.1.289**, but no account, OAuth login, real API key, or model inference.

## Run

From the repository root, with `claude` on `PATH`:

```bash
GENTLE_AI_CLAUDE_SUBAGENT_E2E=1 go test ./e2e/organicruntime \
  -run '^TestClaudeMarkdownSubagentsEnforceEmptyTools$' -count=1 -v -timeout=6m
```

The test is opt-in and skips under `go test -short`. A missing or different Claude version fails an explicitly enabled run rather than silently proving another client version.

## What it checks

- The public installer produces the five reviewer definitions in a temporary project.
- Scripted responses invoke each reviewer through `Agent`, not `--agent`.
- Empty tool lists either prevent launch with a zero-tools error or expose no tools and reject a forced `Read` request.
- Explicit `Read` and omitted-tool controls must actually read a sentinel file, proving the subagent route is exercised.
- The sentinel file remains unchanged.

The parent process can use only `Agent` and `Read`. It uses a fresh configuration directory, synthetic API key, loopback endpoint, disabled hooks/plugins, and no configured MCP servers. No reviewer permissions are changed.

## Fork versus named reviewer

`TestClaudeForkAndNamedReviewerToolBoundary` compares the special `fork` Agent type with the installed `review-risk` Markdown definition on the same pinned client:

```bash
GENTLE_AI_CLAUDE_SUBAGENT_E2E=1 go test ./e2e/organicruntime \
  -run '^TestClaudeForkAndNamedReviewerToolBoundary$' -count=1 -v -timeout=3m
```

- A fork must inherit the parent-only context marker and `Read`, emit a client `Read` event linked to the parent Agent invocation, and return the actual sentinel without error.
- The named reviewer must fail closed without inheriting that marker or reading the sentinel; both cases leave the file unchanged.
- Fork mode is enabled with foreground scheduling so the parent awaits completion. This does not establish background-fork behavior.

The comparison fixture preserves the full inherited history while distinguishing the current child task and its own tool result from inherited parent-result placeholders. HTTP regressions cover those two routing boundaries and the named-reviewer parent-error control. The original seven-case proof and reviewer permissions remain unchanged.

## Limits

This is deterministic client-runtime evidence, not model-behavior evidence or a reproduction of the interactive agent-list display. It is **not** a network-namespace proof. Docker is not required for this local probe; the separate `Dockerfile.claude-network-none` lane verifies its own native-adapter transport boundary and pinned version.
