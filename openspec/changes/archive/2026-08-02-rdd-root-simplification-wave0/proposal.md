# Proposal: RDD Root Simplification — Wave 0 (Freeze Expansion)

## Intent

RDD's trust kernel is sound, but lifecycle ownership is split across native authority, adapters, SDD, and five gates. Each local fix adds a route, reason code, artifact, or verb, so the same mechanism recurs elsewhere. Wave 0 stops that expansion by landing the approved architecture and naming one owner per surface — before any code moves.

## Scope

### In Scope

| # | Deliverable | Artifact |
|---|---|---|
| 1 | Land the RDD root simplification design, amended per maintainer review (A–E below) | `docs/architecture/rdd-root-simplification-design.md` |
| 2 | Ownership inventory: every lifecycle transition, persisted artifact, contract surface, and consumer mapped to exactly one owner | `docs/architecture/rdd-ownership-inventory.md` |
| 3 | Freeze-expansion policy: old-facade recovery/transport work accepted only for proven security defects; acceptance criteria documented | `docs/architecture/rdd-freeze-expansion-policy.md` |
| 4 | Backlog disposition baseline: open review/SDD issues and PRs classified `superseded-by-design` / `absorbed-into-wave-N` / `still-valid-fix-now` / `orthogonal` | `docs/architecture/rdd-backlog-disposition.md` |
| 5 | Migration chain plan (tracker branch + one PR per wave) documented in the design | design doc section |

### Out of Scope

- Any code under `internal/reviewtransaction`, `internal/cli/review*`, `internal/sddstatus`.
- New lifecycle behavior, contract version changes, CI gate enforcement of the freeze policy.
- Waves 1–7 (each becomes a separate SDD change on the same chain).
- Closing or relabeling issues/PRs — Wave 0 only produces the classification instrument.

## Design amendments required (maintainer review, accepted)

| ID | Amendment |
|---|---|
| A | `compatible_base_advance` cites the existing proof in `internal/reviewtransaction/prepr.go` (`deriveBaseAdvanceCompatibility`, line 73: merge-base tree preservation, path digest identity, patch identity, path disjointness, conflict-free `merge-tree --write-tree`, issuer-bound CI attestation, trust root) as normative semantics instead of re-deriving them. |
| B | `provable_contraction` gains a soundness condition: contraction validates only when admitted findings reference no excluded path; otherwise degrade to `changed`. |
| C | Wave 3 coexistence precedence: legacy readable authority never authorizes delivery of a candidate that has a new lineage. |
| D | Two decisions added to the unresolved table: external evidence retention horizon, and SDD attempt-ledger ownership. |
| E | #1379 (cross-lineage receipt contamination, audit-flagged potentially severe) appears in the adversarial safety table and coverage map. |

Evidence table also re-points to the in-repo audit `docs/audits/2026-07-21-rdd-system-audit.md` (sha256 `4b41d15a…`) instead of an external path.

## Capabilities

### New Capabilities

- `rdd-simplification-design`: the normative target architecture document and its amendment rules.
- `rdd-ownership-inventory`: one-owner mapping for transitions, artifacts, contracts, consumers.
- `rdd-freeze-expansion-policy`: acceptance criteria for old-facade work during migration.
- `rdd-backlog-disposition`: classification vocabulary and baseline for review/SDD backlog.

### Modified Capabilities

- None.

## Approach

1. Copy the design from the sibling worktree into `docs/architecture/`, applying amendments A–E and the evidence-path correction.
2. Derive the ownership inventory from the design's ownership-boundary and control-reduction tables, using CodeGraph to enumerate real transitions, artifacts, and consumers at snapshot `ece470da`. Every row names exactly one owner; unowned or multi-owner rows are recorded as findings, not resolved here.
3. Write the freeze policy as documentation only: scope, "proven security defect" criteria, evidence required, and the escalation path. No CI enforcement in this change.
4. Seed the backlog baseline from the design's issue/PR coverage map plus the audit's issue map; classify only what the two documents already support, and mark the rest `unclassified — needs triage`.
5. Document the chain: tracker `feature/rdd-root-simplification` off `main@ece470da`; one child PR per wave targeting the previous slice; only the tracker merges to `main`; ~1000 changed lines per PR.

## Affected Areas

| Area | Impact | Description |
|---|---|---|
| `docs/architecture/rdd-root-simplification-design.md` | New | Amended design |
| `docs/architecture/rdd-ownership-inventory.md` | New | Owner map |
| `docs/architecture/rdd-freeze-expansion-policy.md` | New | Migration policy |
| `docs/architecture/rdd-backlog-disposition.md` | New | Backlog baseline |
| `internal/**` | None | Explicit non-goal |

## Unresolved decisions (adopted by default, pending maintainer amendment)

| # | Decision | Default adopted for the spec phase |
|---|---|---|
| 1 | Five-state model + two-active-artifact rule | Adopt as specified |
| 2 | Shared relation algebra + read-only gates | Adopt; gates never mutate authority |
| 3 | Declined review / unsupported runtime | Unmanaged ordinary delivery; fail before freeze |
| 4 | Repair authorization + compatibility horizon | Maintainer-bound disposition plan; short version-pinned read-only horizon |
| 5 | Wave order | Wave 1 read-only equivalence, then #1892 leaf-only; #2014 deferred |
| 6 | `provable_contraction` soundness | Degrade to `changed` when admitted findings touch excluded paths |
| 7 | Legacy/new precedence | Legacy authority cannot authorize a new-lineage candidate |
| 8 | External evidence retention horizon | Retain digests indefinitely in authority; raw payloads are provider diagnostics with a declared expiry |
| 9 | SDD attempt-ledger ownership | Keep attempts in SDD, but only if its store gains durable cumulative CAS-like properties; otherwise move attempts to native authority |

Decisions 6–9 are new relative to the design; 1–5 are its "Immediate next step" list. Any maintainer amendment supersedes the default before `sdd-spec` encodes it.

## Risks

| Risk | Likelihood | Mitigation |
|---|---|---|
| Ownership inventory drifts from code after `ece470da` | High | Pin the inventory to the snapshot SHA and state it is not live authority |
| Backlog baseline misclassifies an open defect as superseded | Med | Classification is advisory; closing requires separate reproduction evidence |
| Freeze policy blocks a genuine security fix | Med | Security-defect exemption is explicit and needs no waiver |
| Adopted defaults 6–9 are later reversed | Med | Marked adopted-pending-amendment; spec phase re-reads this table |
| Docs-only wave reads as ceremony without value | Low | Wave 0 exit evidence is the inventory itself, per the design's wave table |

## Rollback Plan

Revert the Wave 0 commits on `feature/rdd-root-simplification`. Deliverables are new documentation files with no runtime, CI, or contract dependency, so deletion is complete rollback. The tracker branch may be abandoned without touching `main`.

## Dependencies

- Design snapshot `ece470dacd0041f394e7f6f3877a6a9fcb3482af` (current `main`).
- In-repo audit `docs/audits/2026-07-21-rdd-system-audit.md`.
- GitHub issue/PR state for the backlog baseline (read-only, snapshot-dated).

## Success Criteria

- [ ] Amended design lives in `docs/architecture/` and cites the in-repo audit path and `prepr.go` proof.
- [ ] Amendments A–E are all present and individually verifiable in the design.
- [ ] Every RDD lifecycle transition, persisted artifact, contract surface, and consumer (adapters, SDD, five gates) appears exactly once in the inventory with one named owner.
- [ ] Freeze policy states scope, security-defect criteria, required evidence, and escalation path.
- [ ] Backlog baseline classifies every issue/PR named in the design's coverage map and the audit's issue map, or marks it `unclassified — needs triage`.
- [ ] Chain plan documents tracker branch, per-wave PR targets, and the merge rule.
- [ ] No file outside `docs/` and `openspec/` is modified.

## Proposal question round (pending — execution mode `auto`)

These product questions were not asked interactively. Answering any of them before `sdd-spec` will change the encoded defaults:

1. Is the freeze-expansion policy binding on external contributors, or maintainer-internal guidance only? (Assumed: guidance, no CI enforcement.)
2. Should the backlog baseline propose issue closures, or only classify? (Assumed: classify only; closure is the end-of-migration audit.)
3. Does "proven security defect" require a reproduction on current `main`, or does a credible report suffice? (Assumed: reproduction required, per the RDD defect workflow.)
4. For decision 9, is growing SDD's store into a CAS-like ledger acceptable, or should attempts return to native authority as the audit recommended? (Assumed: conditional — SDD keeps attempts only with durable cumulative guarantees.)
5. What retention horizon is acceptable for raw reviewer/validator payloads (decision 8)? (Assumed: declared expiry, digests retained.)
