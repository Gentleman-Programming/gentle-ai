# Tasks: Optional CodeGraph Integration for Pi

## Review Workload Forecast

| Field | Value |
|---|---|
| Estimated additions + deletions | 1,250–1,550 across 12–15 files |
| Estimated changed files | 12–15 |
| 800-line budget risk | High (450–750 lines over) |
| Chained PRs recommended | Yes |
| Suggested split | Candidate units below; not authorized under single-pr |
| Delivery strategy | single-pr |
| Chain strategy | pending |

Decision needed before apply: Yes
Chained PRs recommended: Yes
Chain strategy: pending
400-line budget risk: High

The cached strategy remains one PR. Before apply, a maintainer must approve a `size:exception` or explicitly change the delivery strategy; do not split automatically.

### Suggested Work Units

| Unit | Goal | Likely PR | Focused test command | Runtime harness | Rollback boundary |
|---|---|---|---|---|---|
| 1 | Paths, discovery, transaction | Candidate 1 | `go test ./internal/agents/pi ./internal/components/communitytool` | Fixture Pi home | Adapter/reconciler/tests |
| 2 | Install/sync and recovery | Candidate 2 | `go test ./internal/cli ./internal/components/communitytool` | Temp-home sync drift | CLI/lifecycle tests |
| 3 | Removal and docs | Candidate 3 | `go test ./internal/components/uninstall ./internal/cli` | Uninstall preserves user entry | Uninstall/tests/docs |

## Phase 1: Pi Contracts and Selection Boundary

- [x] 1.1 RED: Add `internal/agents/pi/adapter_test.go` fixtures for MCP/child precedence, shadowing, unreadable children, and `PI_CODING_AGENT_DIR` resolution.
- [x] 1.2 GREEN: Extend `internal/agents/pi/adapter.go` with deterministic Pi MCP/child path discovery; do not read or modify gentle-pi assets.
- [x] 1.3 RED: Prove in `pi_codegraph_test.go` Pi-unselected selection creates no CLI/MCP/overlay/guidance/index artifact or ownership record.
- [x] 1.4 GREEN: Add Pi gating and ownership manifest in `pi_codegraph.go`; retain gentle-pi agnosticism.

## Phase 2: Provisioning and Direct Verification

- [x] 2.1 RED: Test canonical MCP merge/adoption, `misconfigured` conflict, child classifications, and marker-only verification failure.
- [x] 2.2 GREEN: Implement atomic MCP merge, owned overlays/bounded blocks, capability probing, and direct tool/guidance verification in `pi_codegraph.go`.
- [x] 2.3 RED: Test relative/absolute workspace and `git -C` roots; reject home, filesystem root, temp, and failed init before fallback.
- [x] 2.4 GREEN: Add Pi lazy-init guidance: one root-scoped init, CodeGraph use on success, reported fallback; route status through `tool.go` and `codegraph_guidance.go`.

## Phase 3: Lifecycle Reconciliation and Recovery

- [x] 3.1 RED: Test provision/reconcile byte-idempotence, sync-restored artifacts, and every-child re-verification.
- [x] 3.2 GREEN: Wire reconciler last after Pi/component refresh in `run.go` and `sync.go`; add dynamic Pi targets to snapshots.
- [x] 3.3 RED: Test update/verify failure restores owned bytes, deletes new overlays, and reports no verification success.
- [x] 3.4 GREEN: Implement before-image rollback and commit manifest only after verification in `pi_codegraph.go`.

## Phase 4: Ownership-Safe Removal and Evidence

- [x] 4.1 RED: Add `service_test.go` cases preserving user MCP keys/blocks, gentle-pi files, drifted hashes, and repeat uninstall.
- [x] 4.2 GREEN: Add manifest-scoped Pi cleanup to `service.go`; restore bounded backups and report drift as manual action.
- [x] 4.3 Document selection, ownership, classifications, sync recovery, and manual drift in `docs/pi.md`.
- [x] 4.4 Run `gofmt`, `go test ./...`, and `go vet ./...`; verification covers every scenario, including gentle-pi non-modification.

> **Verification bookkeeping correction (2026-07-10):** This artifact has 16 numbered tasks (1.1–4.4), not 12 or 14. The checked boxes record task completion; `apply-progress.md` preserves the corresponding per-task evidence and remediation RED/GREEN evidence.
