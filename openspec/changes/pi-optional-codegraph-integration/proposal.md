# Proposal: Optional CodeGraph Integration for Pi

## Intent

Deliver an optional Pi CodeGraph capability owned by Gentle AI. A parent `APPEND_SYSTEM.md` marker does not prove child tools, guidance, or index availability.

## Scope

### In Scope
- Own optional selection, installation, and verified Pi extension/MCP configuration in Gentle AI.
- Discover Pi children, inject tools into compatible children, and inject lazy-init guidance into every child.
- Auto-initialize CodeGraph at the project root when an index is missing.
- Reconcile after gentle-pi sync, verify real capability per child, and cleanly uninstall only owned artifacts.
- Start design by observing and recording the Pi/CodeGraph extension and MCP schema, commands, and ownership boundaries—never guess them.

### Out of Scope
- Any gentle-pi dependency, prompt, tool, configuration, or behavior change; it remains completely CodeGraph-agnostic.
- Installing, configuring, or requiring CodeGraph for normal Pi installs without CodeGraph selected.
- Treating a parent `APPEND_SYSTEM.md` marker as proof of child capability.
- Supporting unrelated Pi extensions or changing CodeGraph upstream.

## Capabilities

### New Capabilities
- `pi-codegraph-integration`: Gentle-AI-owned optional Pi provisioning, child reconciliation, lazy initialization, verification, sync, and uninstall.

### Modified Capabilities
None.

## Approach

Capture evidence from installed Pi and CodeGraph before defining the schema and ownership contract. Add an idempotent gentle-ai provisioner/reconciler after gentle-pi install and sync. Use bounded edits or owned files, discover children from verified runtime locations, and classify them as compatible, guidance-only, unavailable, or misconfigured. Verify each child’s effective tools and guidance directly.

## Affected Areas

| Area | Impact | Description |
|------|--------|-------------|
| `internal/components/communitytool/` | Modified | Provisioning, guidance, status, ownership |
| `internal/agents/pi/` | Modified | Pi paths and configuration |
| `internal/cli/run.go`, `internal/cli/sync.go` | Modified | Ordering and reconciliation |
| `internal/components/uninstall/` | Modified | Managed cleanup and rollback |
| `docs/pi.md` | Modified | Optional ownership contract |

## Risks

| Risk | Likelihood | Mitigation |
|------|------------|------------|
| Upstream schema differs | High | Evidence record and contract fixtures |
| Sync overwrites child integration | High | Reconcile after gentle-pi refresh |
| Cleanup removes user configuration | Medium | Ownership manifest and bounded edits |
| Work exceeds 800 review lines | High | Tasks phase forecasts review slices |

## Rollback Plan

Disable reconciliation, remove owned artifacts, and restore backed-up managed configuration without touching gentle-pi or user entries.

## Dependencies

- Current Pi, CodeGraph, and MCP adapter behavior available for observation.

## Success Criteria

- [ ] Selected installs configure and verify every discovered child according to real capability.
- [ ] Missing indexes initialize automatically; sync restores drift idempotently.
- [ ] Unselected Pi remains unchanged, and uninstall removes only Gentle-AI-owned integration.
- [ ] gentle-pi contains no CodeGraph-specific change.
