# Proposal: feat(pi): resolve and invoke installed model-routing contract (#3530)

## Intent

Resolve and invoke installed `gentle-pi.model-routing/v1` (`gentle-pi-models`) as library unblocking #3522. `internal/agents/pi` today only installs Pi packages and verifies `package.json` bin via `ResolvePackageBin`; no resolver/client exists.

## Scope

### In Scope
- Resolver `ResolveModelRoutingExecutable(ctx,cwd,agentDir,target)`: PATH-first, then `npm:`/`git:`/`local:` roots via `docs/packages.md`, then `ResolvePackageBin`, then `capabilities` probe
- Precedence: project `settings.json` > user; `PI_CODING_AGENT_DIR` > `~/.pi/agent`; explicit `AgentDir`
- Manifest bin: reuse `ResolvePackageBin` (64 KiB, duplicate-key rejection, `absent-bin`/`unsafe-bin`/`missing-bin-target`/`non-regular`/`non-executable`, symlink containment)
- Client `ModelRoutingClient{Capabilities,Inspect,Validate,Apply}`: one JSON request (`contract: "gentle-pi.model-routing/v1"`), bounded response, exit-class → typed errors
- Invariants: read-only, no `pi install`/npm/network/writes/fallback edit; distinct errors (`missing`/`malformed`/`timeout`/`invalid-json`/`unsupported-contract`/`probe-failed`); cancellation terminates child; injectable seams

### Out of Scope
- TUI (`internal/tui/screens` — PR2 #3522)
- JSON editing, install/fetch, credentials
- Windows exec-bit enforcement
- Publishing `gentle-pi-models` binary

## Capabilities

### New Capabilities
- `pi-model-routing`: Resolve installed `gentle-pi-models` and invoke `gentle-pi.model-routing/v1` (`capabilities`/`inspect`/`validate`/`apply`) with bounded probe and typed errors.

### Modified Capabilities
- None

## Approach

Approach 1 — extend `internal/agents/pi` with injectable resolver + client. `model_routing_resolver.go` (`TargetProject`/`Global`, `resolveOnPATH`, `selectedPackageSource`, `packageRootForSource`, probe) and `model_routing_client.go` (`Client{Bin,Timeout,Runner}`, `do()` write-one/read-bounded). Reuses `CodeGraphPaths`/`AgentConfigPath` and `pi_codegraph.go` seam.

Rejected: `pi/modelrouting` subpackage (duplicates helpers, exceeds budget); generic `internal/pi` client (YAGNI before `#383`/`#384` freeze).

Delivery: single PR `<=400` A+D (goldens excluded), table-driven tests for PATH/layout/precedence/override/probe/timeout/protocol/no-network/no-write.

## Affected Areas

| Area | Impact | Description |
|------|--------|-------------|
| `internal/agents/pi/model_routing_resolver.go` | New | Resolver: PATH + package-root + probe |
| `internal/agents/pi/model_routing_client.go` | New | Client for `gentle-pi.model-routing/v1` |
| `internal/agents/pi/model_routing.go` | Modified | Keep as leaf bin verifier |
| `internal/agents/pi/adapter.go` | Modified | Read-only `gentle-pi` settings helper |
| `internal/agents/pi/model_routing_test.go` | Modified | Resolver/client matrix |

## Risks

| Risk | Likelihood | Mitigation |
|------|------------|------------|
| Upstream not frozen (`gentle-pi#381`/`#382`/`#383`/`#384`, `docs/packages.md` 404) | High | Isolate `packageRootForSource` + contract; preserve unknown fields |
| 400-line budget vs test matrix | Med | Shared helpers; defer TUI to #3522 |
| TOCTOU resolve→exec | Low | Re-stat before spawn / fail-closed |
| `PI_CODING_AGENT_DIR` collision | Low | Share `AgentConfigPath`/`CodeGraphPaths` |
| Regression `ProvisionEngramMCP` | Low | Resolver never calls `mergePiSettingsFile` |

## Rollback Plan

Revert commit; no persisted state. #3522 not started until this merges. If Pi contract shifts, patch `packageRootForSource` + exit map only.

## Dependencies

- `gentle-pi` `docs/packages.md` via `#382` (roots + `PI_CODING_AGENT_DIR`)
- `gentle-pi.model-routing/v1` schema/exits via `#383`/`#384`
- Base `v2.5.0-rc.2`

## Success Criteria

- [ ] Resolver returns probed `gentle-pi-models` with PATH-first, project>user, all 3 package kinds
- [ ] Client validates `gentle-pi.model-routing/v1`, maps exits to typed errors, respects timeout/cancellation
- [ ] `go test ./internal/agents/pi` green; no-network and no-write assertions pass
- [ ] No regression in `Adapter`/`opencode`/`cli`; PR `<=400` A+D
- [ ] Unblocks #3522 without TUI/install scope creep
