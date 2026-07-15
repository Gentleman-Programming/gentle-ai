# Proposal: Stabilize Review Lifecycle

## Intent

Keep an approved correction usable through review/SDD delivery without false denial, dead ends, or repeated budgets. Evidence: final-scope mismatch (#1226, #1285), archive contract (#1296), volatile transitions (#1269, #1243, #1283, #1267, #1257, #1294), and weak drift diagnostics (#1248, #1234).

## Scope

### In Scope

- Bind each `TargetFixDiff` receipt to one final correction scope and its evidence.
- Keep lifecycle gates read-only: no start, finalize, recover, lineage, or budget allocation.
- Support verify → archive → stage → commit → push → pre-PR; archive survives OpenSpec relocation through the compact binding.
- Persist minimum invalidation, validator-escalation, verify-remediation, and restart transitions for the same lineage.
- Emit exact scope-change diagnostics and maintainer action; never create a lineage automatically.
- Prove one realistic end-to-end flow with one lineage and budget.

### Non-Goals

- A general v3 authority resolver/registry or repository identity subsystem.
- Authority unification across all legacy, Engram, or worktree sources beyond the compact binding required here.
- Legacy-hook adoption or suppression; executable/lifecycle risk-accounting redesign.
- Telemetry or broad prompt/provider cleanup unrelated to this flow.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `review-findings-ledger`: final correction identity, pure gates, durable same-lineage transitions, scope diagnostics, and one-lineage proof.
- `sdd-orchestrator-assets`: supported archive/delivery flow and explicit maintainer action for scope change.

## Approach

Extend only existing compact receipt/binding and lifecycle APIs, using typed gate results, CAS-protected transitions, and one Git-backed fixture. Target one coherent PR below 3000 authored changed lines; if forecasting cannot meet it, report that before apply.

## Migration and Rollback

Version changed receipt/binding fields without reinterpreting legacy bytes. Preserve chains and projections. Rollback reverts readers, gates, transitions, and guidance together without deleting evidence.

## Affected Areas

| Area | Impact |
|---|---|
| `internal/reviewtransaction/` | Receipt identity, pure gates, minimal durable transitions. |
| `internal/sddstatus/`, `internal/cli/` | Archive relocation, flow validation, exact diagnostics. |
| `internal/assets/skills/_shared/` | Supported lifecycle guidance only. |
| Lifecycle integration tests | One real Git-backed lineage/budget proof. |

## Risks

- Compatibility and archive races require schema/restart fixtures.
- Existing specs/design remain broader and must be narrowed before implementation.
- The slice may exceed 3000 lines; task forecasting decides honestly.

## Success Criteria

- [ ] Final receipt and correction evidence agree at every gate.
- [ ] Validation performs zero review/lifecycle-allocation operations.
- [ ] Archive survives relocation; later gates reuse the lineage/budget.
- [ ] Required transitions and restart cannot dead-end the lineage.
- [ ] Scope drift returns exact diagnostics plus maintainer action and creates no lineage.
- [ ] One realistic end-to-end fixture proves the complete flow.
