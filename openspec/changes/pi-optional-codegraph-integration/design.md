# Design: Optional CodeGraph Integration for Pi

## Verified Runtime Evidence

Observed read-only on 2026-07-10; these contracts become fixtures.

| Evidence | Observed contract |
|---|---|
| `pi --version`; `pi install/list --help`; `pi list` | Pi `0.80.6`; packages live in user/project `settings.json`; installed `gentle-pi@0.15.0`, `pi-mcp-adapter@2.11.0`, and `pi-subagents-j0k3r@1.1.3`. |
| Pi MCP adapter README/source | Config precedence is `~/.config/mcp/mcp.json`, `$PI_CODING_AGENT_DIR/mcp.json`, `.mcp.json`, `.pi/mcp.json`. Schema is `mcpServers.<name>.command/args`; proxy tool is `mcp`. |
| Pi subagent README/source | Effective definitions are resolved from `$PI_CODING_AGENT_DIR/{agents,subagents}/*.md` then `.pi/{agents,subagents}/*.md`; later/project definitions override normalized names. Frontmatter `tools` is passed to `createAgentSession`; lean sessions retain allowlisted extension tools. |
| `codegraph --version`; `install/serve/init --help` | CodeGraph `1.2.0`; MCP command is `codegraph serve --mcp`; `init [path]` indexes by default. `codegraph install --print-config pi` returns `Unknown target "pi"` (known targets exclude Pi). |
| Installed CodeGraph MCP schema | Default `tools/list` exposes only read-only `codegraph_explore`, requiring `query`; optional fields are `maxFiles` and `projectPath`. |
| Pi adapter runtime status probe | Pi `0.80.6` accepts `pi --mcp-config <path> --mode json --no-session --offline --print "/mcp status"`, but has no dedicated machine-readable MCP status API: a valid local config emitted only a JSON `{"type":"session",...}` event, and a nonexistent config exited `0` with no output. Therefore exit `0`, session-only output, empty output, and plaintext `codegraph: connected`-style output are not adapter health evidence. The probe fails closed; direct CodeGraph MCP `tools/list` schema verification remains distinct capability evidence. |

## Technical Approach

Add a Pi-specific reconciler owning a minimal Pi MCP key, child overlays/marker blocks, and `~/.gentle-ai/pi-codegraph.json`. It never changes `gentle-pi`; install and sync invoke one transaction after refreshes.

## Architecture Decisions

| Decision | Alternatives / tradeoff | Rationale |
|---|---|---|
| Merge only `mcpServers.codegraph = {command:"codegraph", args:["serve","--mcp"]}` into Pi's global override. | Upstream installer cannot target Pi; a new adapter duplicates MCP. | Exact intersection of observed CodeGraph and Pi schemas. Existing unowned equivalent entries are preserved/adopted; conflicts are `misconfigured`, never overwritten. |
| Discover effective children using the observed four-directory precedence. | Scanning only gentle-pi assets misses user/project children. | Matches runtime identity; unreadable/shadowed candidates are reported `unavailable`. |
| A child is `compatible` only with parseable explicit `tools`, existing `bash`, detectable MCP proxy, and injectable `mcp`. | Adding `bash` broadens privilege; direct tools need warm cache. | Least privilege. Other readable children are `guidance-only`. |
| Never edit package-owned gentle-pi files. Create owned same-name overlays in `$PI_CODING_AGENT_DIR/subagents`; bounded-edit user/project effective files. | Editing gentle-pi files invalidates its hash ownership manifest. | Preserves upstream ownership and survives refresh. Tool and body blocks use ignored comments `gentle-ai:pi-codegraph-tool/guidance`; the manifest records source/target hashes and ownership mode. |
| Uninstall removes only owned MCP key/overlays/blocks/manifest. | `codegraph uninstall` would affect unrelated agents. | Preserve external MCP entries, user content, CLI, and project `.codegraph/` data. Hash drift becomes a manual action, not deletion. |

## Data Flow and Contracts

```text
Pi install/refresh -> discover + capability probe -> plan/snapshot -> MCP merge
  -> child overlay/block reconcile -> verify MCP schema + every effective child -> commit manifest
gentle-ai sync components ----------------------------------------------------^
```

Lifecycle methods return per-child `{name, source, target, classification, tools, guidance, reason}`. Verification requires canonical MCP transport, read-only `tools/list` with the observed schema, guidance in every readable effective child, and `bash` plus `mcp` for compatible children; `APPEND_SYSTEM.md` is never evidence.

Guidance resolves root with argv-based `git rev-parse --show-toplevel` or a validated workspace, rejects home/root/temp, checks `<root>/.codegraph`, runs `gentle-ai codegraph init --cwd <root>` once when absent, then calls `mcp`/`codegraph_explore`; fallback requires a reported failure. The CLI command canonicalizes the root and rejects symlink escapes and unrecognized projects before invoking CodeGraph.

Apply uses atomic writes plus an in-memory before-image journal. Any apply/verify failure restores changed files (including deleting newly created overlays) before pipeline rollback. Repeated runs compare canonical bytes/hashes and are no-ops.

## File Changes

| File | Action | Purpose |
|---|---|---|
| `internal/components/communitytool/pi_codegraph.go` | Create | Contracts, discovery, transaction, verification, ownership manifest. |
| `internal/components/communitytool/tool.go`, `codegraph_guidance.go` | Modify | Route selected/configured Pi through real capability status. |
| `internal/agents/pi/adapter.go` | Modify | Resolve `PI_CODING_AGENT_DIR` and expose child/MCP paths. |
| `internal/cli/run.go`, `sync.go` | Modify | Include dynamic backup targets; reconcile last, after Pi/component refresh. |
| `internal/components/uninstall/service.go` | Modify | Add owned Pi integration cleanup. |
| Corresponding `*_test.go`, `docs/pi.md` | Modify | RED contracts and optional ownership documentation. |

## Testing Strategy

Unit fixtures pin observed schemas, precedence, classifications, markers, drift, idempotence, and uninstall preservation. Integration tests cover selected/unselected install, MCP handshake, every-child verification, post-sync restoration, mid-plan rollback, and parent-marker false positives. No live network tests.

## Threat Matrix

| Boundary | Applicability | Safe/failure behavior and planned RED test |
|---|---|---|
| Documentation-like paths | N/A — no executable-file classification. | None. |
| Git repository selection | Applicable | Argv-only root resolution accepts relative/absolute workspaces and `git -C`; rejects home/root/temp and init failure before fallback. RED cases cover each selector/refusal. |
| Commit state | N/A — no commits. | None. |
| Push state | N/A — no pushes. | None. |
| PR commands | N/A — no PR automation. | None. |

## Migration / Rollout

No gentle-pi migration or default. Existing Pi parent-only markers become `misconfigured` until selected reconciliation succeeds. Rollback is uninstall plus transaction restore.

## Open Questions

None.
