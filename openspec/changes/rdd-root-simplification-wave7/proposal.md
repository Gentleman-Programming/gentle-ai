# Proposal: RDD Root Simplification — Wave 7 (Compatibility Retirement)

The LAST wave. Waves 0–6 built the new lineage beside the old one; Wave 7 deletes the old one so exactly one lifecycle remains. Also persisted to Engram `sdd/rdd-root-simplification-wave7/proposal`.

## Gate

**This wave cannot start until Waves 3, 4, 5 and 6 are all merged to `main`.** Verified at proposal time on `main`: Wave 1 and Wave 2 have landed (`shadow_*.go`, `authority_disposition_execute.go` present; both archived); Waves 3–6 have **not** (`GENTLE_AI_RDD_NEW_LINEAGE` appears in `openspec/changes/rdd-root-simplification-wave3/**` only, zero hits under `internal/`). Every inventory below is therefore *known candidates*, not the final list.

## Intent

Delete the legacy RDD machinery. Today two lifecycles coexist behind a switch: the legacy mutation state machine (13 persisted states, cause-specific repair verbs, v1 contract projections) and the new-lineage `start/finalize/validate` flow with two artifacts. Coexistence is the root cause the design named — every fix must be applied twice, and every consumer must know which owner is authoritative. Wave 7 removes the second owner. Success is a strongly net-negative deadcode ratchet with **zero** behavior change for the new lineage.

## Scope

### In scope

**Deletion inventory is a task-time discovery obligation.** `sdd-tasks` MUST re-derive the complete inventory against the post-Wave-6 tree (call graph + consumer evidence per target) before any deletion. Known candidates as of `main`:

| Candidate | Location | Known consumers |
|---|---|---|
| `ReconcileInvalidRecoveryEdge` (Wave 6 explicitly deferred this to W7) | `internal/reviewtransaction/compact_reconcile.go:233` | `internal/cli/review_reconcile.go`; bench `ds01`/`ds02`/`ds04` (`bench/axis_damaged_store.go`) |
| `ReconcileInvalidRecoveryEdges` | `internal/reviewtransaction/compact_batch_reconcile_journal.go:71` | `internal/cli/review_reconcile_batch.go` |
| Legacy public verbs | `internal/cli/review_facade.go:701–714` — `reconcile-authority`, `reconcile-authority-batch`, `quarantine-legacy`, `quarantine-legacy-fix-scope`, `repair-legacy-alias` | facade dispatch + per-verb handlers/tests |
| `GENTLE_AI_RDD_NEW_LINEAGE` switch + the legacy `start` branch it guards | Wave 3 (`review_facade.go` start branch) | switch removal makes the new lineage the only path |
| `GENTLE_AI_RDD_SHADOW` observer and the `ShadowRelation` alias | `internal/reviewtransaction/shadow_*.go`; alias in `candidate_relation.go` post-W3 rename | Wave 1 scaffolding; its exit evidence is already banked in the golden matrix |
| Duplicate contract projections | `contracts/review-integration/v1/**` (47 files) vs `v2/**` | adapters, schema guard tests |

Also in scope: the deletion proof the design requires for backlog rows classified `superseded-by-design` (`#1455`, `#1462`, `#1570`, PRs `#1549`, `#1550`).

### Out of scope

- Anything the release candidate needs before stable — a release-blocking defect routes to its own change, never into a deletion wave.
- Historical bytes: receipts, journals, bundles and quarantine residue are forensic evidence and are never deleted. The read-only/offline parser for legacy authority stays.
- Any new verb, route, contract version, or behavior change for the new lineage.
- The post-Wave-7 backlog closure audit (`docs/architecture/rdd-backlog-disposition.md` §Closure audit protocol) — a separate pass after W7 exit evidence.

## Capabilities

### New

- `rdd-legacy-retirement`: the deletion contract — consumer-first ordering, per-target consumer-inventory + deletion proof, forensic read-only preservation, and the no-behavior-change obligation.

### Modified

- `rdd-shadow-evaluation`: the runtime shadow-observation surface retires; the differential-matrix golden remains as historical Wave 1 exit evidence.
- Post-W6 discovery: the capabilities Waves 3–6 introduce (e.g. new-lineage activation, gate cutover, closure disposition) will each need a delta once those specs exist on `main`.

## Approach

**Consumer-first, never provider-first.** For each target: (1) prove the consumer inventory from the call graph, (2) migrate or retire every consumer, (3) delete the provider, (4) delete the now-unreachable tests *as part of the same slice, with the deletion proof naming what they covered*. A provider is never deleted while a live consumer exists — if a consumer cannot be retired, the target is deferred with a written reason rather than force-deleted.

The switch removal has its own ordering: **byte-equivalence exit evidence before removal.** Prove a `GENTLE_AI_RDD_NEW_LINEAGE=1` build and a switch-free build produce byte-identical goldens, envelopes and receipts across the full journey set; only then delete the switch and the legacy branch. Goldens stay byte-stable across the entire wave — a golden diff is a defect signal, not an update task.

Expected shape: deadcode ratchet strongly net-negative; net line count strongly negative. Slices are cut by **consumer cluster**, not by file count, so each PR remains a coherent, independently revertible retirement.

## Affected areas

| Area | Impact | Description |
|---|---|---|
| `internal/reviewtransaction/compact_reconcile.go`, `compact_batch_reconcile_journal.go` | Removed | Legacy reconcile providers |
| `internal/cli/review_facade.go` | Modified | Legacy verb dispatch removed; switch-free start |
| `internal/cli/review_reconcile*.go`, `review_legacy_*.go` | Removed | Legacy verb handlers |
| `internal/reviewtransaction/shadow_*.go` | Removed | Wave 1 scaffolding |
| `contracts/review-integration/v1/**` | Removed/frozen | Pending D3 |
| `bench/axis_damaged_store.go` | Modified | `ds01`/`ds02`/`ds04` retargeted or retired (D2) |

## Risks

| Risk | Likelihood | Mitigation |
|---|---|---|
| A released adapter (gentle-pi, pinned) still calls a deleted verb mid-upgrade | High | Declared support horizon + real adapter evidence before deletion; retire the route only after the pinned consumer moves |
| Switch removed before byte-equivalence proven → silent behavior change | Med | Byte-equivalence is a blocking exit gate, not a checklist item |
| Legacy *read* path deleted with the mutation path → historical authority unparseable | Med | Read-only parse of shipped legacy-v1 records is an explicit retained invariant with its own journey |
| Inventory derived from a stale pre-W6 tree targets the wrong symbols or double-counts W6's own deletions | High | Inventory re-derivation against post-W6 `main` is a hard task-time precondition |
| Tests deleted to make a deletion "pass", masking a live regression | Med | Every test deletion names the covered behavior and where it now lives (or that it is gone by design) |
| Large deletion diffs blow the 1000-line/PR budget | High | Slice by consumer cluster; pure-deletion slices still get their own PR |

## Open decisions for the maintainer (auto mode — recommended defaults stand unless corrected)

**D1 — Shadow alias: delete, or keep as an internal alias?** *Default: delete the alias and the whole shadow observer.* Evidence: `ShadowRelation` lives under `internal/reviewtransaction`, so a Go importer outside this module is impossible by language rule — gentle-pi cannot import it. Wave 1's exit evidence is already banked in `shadow-differential-matrix.golden`, which is retained. Task-time confirmation of zero in-module consumers is still required.

**D2 — Bench journey retirement policy.** *Default: retarget, do not delete.* `ds01`/`ds02`/`ds04` encode anomaly *shapes* (dangling edge, unclassified edge, pristine successor) that stay meaningful; only the assertions naming deleted verbs are removed. Deleting the journeys outright would silently drop damaged-store coverage.

**D3 — `contracts/review-integration/v1` retirement.** *Default: freeze read-only this wave, delete in a later dated pass.* The design requires a declared horizon plus proof that no supported consumer calls the retired contract; a pinned gentle-pi release is exactly such a consumer. Deleting v1 here would break users mid-upgrade.

**D4 — Verbs of ambiguous vintage** (`recover`, `invalidate`, `reopen-results`, `dispose-result`, `reclaim`, `abandon`). *Default: classify at task time against the post-W6 tree; retire only those with zero new-lineage role.* Not pre-judged here.

**D5 — Definition of "one lifecycle".** *Default: delete legacy **mutation**, retain legacy **read**.* Historical records must still parse; the forensic invariant outranks surface-count reduction.

## Rollback plan

Each slice is one consumer cluster and is independently revertible by `git revert` — deletions carry no persisted state, so reverting restores the surface without migration. The switch-removal slice is the ordering hinge: it is the last slice that can be reverted cheaply, so it lands after every consumer retirement. Historical bytes are never touched, so no rollback can lose authority or forensic evidence.

## Dependencies

- Waves 3, 4, 5 and 6 merged to `main` with exit evidence proven (hard gate).
- Wave 6's own deletion outcome (overlapping internal repair paths) landed, so W7 does not re-delete it.
- A declared, version-pinned compatibility horizon for adapters (design: *Compatibility horizon* decision).

## Success criteria

- [ ] Exactly one lifecycle: no `GENTLE_AI_RDD_NEW_LINEAGE` switch, no legacy start branch, no legacy mutation path.
- [ ] Every retired path has a measured consumer inventory, a migration boundary and a deletion proof (design *Acceptance criteria*).
- [ ] Byte-equivalence proven before switch removal; goldens byte-stable across the whole wave.
- [ ] Deadcode ratchet strongly net-negative; net line count strongly negative.
- [ ] Shipped legacy-v1 authority still parses read-only; quarantine residue and historical receipts byte-identical.
- [ ] `#1455`, `#1462`, `#1570` (and PRs `#1549`, `#1550`) have their `superseded-by-design` deletion proof recorded.
- [ ] Zero new verbs, zero new contract versions, zero behavior change for the new lineage.

## Proposal question round (auto mode)

The maintainer may answer, skip, or correct any of these; assumptions above stand otherwise.

1. D1 — is deleting the `ShadowRelation` alias outright acceptable, given no external Go importer is possible? *Assumption: yes.*
2. D2 — retarget `ds01`/`ds02`/`ds04` rather than delete them? *Assumption: retarget.*
3. D3 — freeze `contracts/review-integration/v1` read-only this wave instead of deleting it? *Assumption: freeze.*
4. D5 — is "delete legacy mutation, retain legacy read" the right reading of "one lifecycle"? *Assumption: yes.*
5. Is there a release candidate whose stable cut must precede this wave, so W7 never competes with a release fix? *Assumption: W7 waits.*
6. If a target's consumer cannot be retired within budget, is "defer with a written reason" acceptable, or must the wave block? *Assumption: defer with reason.*

## Session parameters

auto; auto-chain; feature-branch-chain on tracker `feature/rdd-root-simplification` (last wave, after Wave 6); `review_budget_lines` 1000/PR. Apply is gated on Waves 3–6 all landed on `main`.
