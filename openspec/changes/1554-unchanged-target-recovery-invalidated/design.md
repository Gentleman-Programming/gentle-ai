# Design: Relax CLI unchanged-target recovery gate for `--disposition invalidated`

## Technical Approach

`RunReviewRecover` runs a CLI-level pre-persist gate that rejects any base-diff /
workspace-overlay recovery whose freshly built successor snapshot has the same
`Identity` as the predecessor's initial snapshot, returning `"recovery scope has not
changed"`. It fires before `RecoverCompactAuthority` → `validateCompactRecoveryEdge`,
so it preempts the disposition-aware package validation. For `invalidated`, the
package `RecoveryInvalidated` case requires only a `StateInvalidated` predecessor and
imposes NO changed-target requirement — the CLI gate is the sole (over-broad) blocker.

Fix: add one conjunct to the gate condition so it is skipped when
`RecoveryDisposition(*disposition) == RecoveryInvalidated`, delegating that case to the
package validation as the single authority. Every other disposition keeps the gate.
This maps directly to the proposal's Approach and `review-recovery-gating` capability.

### Line-reference correction (load-bearing)

The proposal cites `review_facade.go:518-519`. After the clean rebase onto
`origin/main`, the unchanged-target gate is at **lines 553-555**; line 519 is a
DIFFERENT check (`"base-diff recovery requires matching --base-ref and --committed-only"`)
and MUST NOT be touched. Anchor the edit on the semantic string, not the number:

```go
// CURRENT (review_facade.go ~553-555)
if !*releaseScope && (baseDiff || overlay) && snapshot.Identity == predecessorRecord.State.InitialSnapshot.Identity {
    return errors.New("recovery scope has not changed")
}
// AFTER
if !*releaseScope && reviewtransaction.RecoveryDisposition(*disposition) != reviewtransaction.RecoveryInvalidated &&
    (baseDiff || overlay) && snapshot.Identity == predecessorRecord.State.InitialSnapshot.Identity {
    return errors.New("recovery scope has not changed")
}
```

The `RecoveryDisposition(*disposition)` conversion mirrors the existing precedent at
line 512 (`--release-scope requires --disposition scope_changed`). The base-tree guard
one line above (`"recovery base-ref does not match predecessor base"`) stays intact.

## Architecture Decisions

| Decision | Alternatives rejected | Rationale |
|---|---|---|
| Add a single `!= RecoveryInvalidated` conjunct to the existing gate | Remove the gate; move it after `RecoverCompactAuthority` | Smallest surgical change; keeps every other disposition byte-for-byte unchanged; no reordering risk. |
| Key strictly on `RecoveryInvalidated` only | Also exclude `scope_changed` | Verified: `compactRecoveryScopeChanged` compares different snapshots/fields (`CurrentSnapshot` vs `InitialSnapshot` on `CandidateTree`/`PathsDigest`/`BaseTree`) than the CLI gate (`snapshot.Identity` vs `InitialSnapshot.Identity`); relaxing it is not a provable no-op and "scope has not changed" is semantically correct for a scope-change disposition. |
| Leave `escalated` blocked by BOTH gates | Relax escalated too | `errCompactRecoveryTargetUnchanged` is a deliberate #1419/#1429 security invariant; out of scope, MUST NOT weaken. |
| Keep base-tree guard untouched | — | Orthogonal safety check; still required for all dispositions. |

## Data Flow

    review recover --disposition D
        │
        ├─ base-tree guard ─────────────► reject on base mismatch (ALL D)
        │
        ├─ unchanged-target gate
        │     D == invalidated ? SKIP ────┐
        │     else (scope_changed,        │
        │           escalated) ──► reject │ "recovery scope has not changed"
        │                                 ▼
        └─ RecoverCompactAuthority → validateCompactRecoveryEdge
              RecoveryInvalidated: requires StateInvalidated predecessor (sole authority)
              RecoveryEscalated:   requires changed target (#1419 invariant, unreachable here)

## File Changes

| File | Action | Description |
|------|--------|-------------|
| `internal/cli/review_facade.go` (~553-555) | Modify | Add `&& RecoveryDisposition(*disposition) != RecoveryInvalidated` conjunct to the unchanged-target gate. Single condition, single line logically. |
| `internal/cli/review_recovery_unchanged_target_test.go` (new sibling) | Create | RED-first regression suite (below). |

## Interfaces / Contracts

No interface, enum, schema, or signature changes. `RecoveryDisposition`,
`gentle-ai.review-recovery-authorization/v1`, and `ReconcileInvalidRecoveryEdge` are
untouched. Behavior change is confined to the `invalidated` branch of one CLI gate.

## Testing Strategy

Reuse existing helpers: `initReviewCLIRepo`, `approveDiscoveryMarkdown`,
`RunReviewInvalidate`, `RunReviewRecover`, base-diff review via `--committed-only`/`--base-ref`.

| Layer | What to Test | Approach |
|-------|-------------|----------|
| Unit/CLI (RED→GREEN) | Unchanged-target `invalidated` recovery on a base-diff/overlay `StateInvalidated` predecessor is ADMITTED (delegated to package validation; `RunReviewRecover` returns nil, successor persisted). | Currently fails with `"recovery scope has not changed"`; passes after the edit. |
| CLI regression (must stay RED-blocked) | Unchanged-target `escalated` recovery on the same predecessor is STILL rejected. | Assert error non-nil AND no successor persisted. Escalated is blocked by the CLI gate first (`"recovery scope has not changed"`); the deeper `errCompactRecoveryTargetUnchanged` invariant is independently locked by `TestEscalatedRecoveryRequiresChangedTarget`. |
| CLI regression (must stay RED-blocked) | Unchanged-target `scope_changed` recovery on the same predecessor is STILL rejected with `"recovery scope has not changed"`. | Assert exact error string unchanged. |
| CLI guard-preserved | Base-tree mismatch on an `invalidated` recovery is STILL rejected (`"recovery base-ref does not match predecessor base"`). | Proves only the identity check was relaxed, not the base guard. |
| CLI no-regression (happy path) | Changed-target `invalidated` recovery (identity differs) still succeeds. | Confirms the added conjunct did not alter the already-working path. |

The two "must stay RED-blocked" tests are the highest-risk assertions: they prove the
change is scoped to `invalidated` and does not leak into `escalated`/`scope_changed`.

## Threat Matrix

Generic routing/shell/subprocess/PR-automation/file-classification matrix: **N/A** — no
shell command, subprocess, PR automation, or executable-file classification is added or
altered; snapshot building and repo selection are unchanged. Focused authority matrix:

| Boundary | Adversarial case | Applicability | Design response | Planned RED test |
|---|---|---|---|---|
| Disposition confusion | Attacker submits `escalated`/`scope_changed` hoping the relaxation leaks | Applicable | Conjunct keyed exactly to `RecoveryInvalidated`; all others keep the gate | escalated + scope_changed still-blocked tests |
| Escalated invariant bypass (#1419/#1429) | Unchanged-target `escalated` self-service retry | Applicable | Untouched; blocked by CLI gate and `errCompactRecoveryTargetUnchanged` | escalated still-blocked test |
| Base-tree substitution | Recovery against a different base | Applicable | Base-tree guard left intact above the edited line | base-mismatch guard test |

## Migration / Rollout

No migration required. Single-file, single-condition change; revert the conjunct and drop
the new test to roll back. No data/schema/persisted-edge impact.

## Open Questions

- [ ] None blocking. (Parallel `sdd-spec` artifact `sdd/1554-unchanged-target-recovery/spec` not yet persisted at design time; reconcile if the spec later disagrees on the scope_changed/escalated boundary — this design treats both as behaviorally unchanged.)
