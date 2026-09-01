# RDD Post-Verify Review Offer Specification

## Purpose

Define the sequence SDD MUST follow per maintainer directive (Engram decision #10123, 2026-08-02): apply -> verify -> offer RDD review -> (optional correction -> targeted re-verify) -> archive. RDD is a service SDD invokes at exactly one point, never a supervisor of the SDD cycle. These are hard MUSTs, not defaults.

## Requirements

### Requirement: Offer Occurs Strictly Post-Verify, Pre-Archive

SDD MUST offer RDD review only after verify completes and before archive begins. SDD MUST NOT consult, block on, or offer RDD review before or during apply. The pre-apply status control (`applyPreVerifyReviewRouting`, `applyPreVerifyCompactBridgeRouting`) MUST be removed, not reordered. (Issue #1209)

#### Scenario: Offer fires only after verify completes

- GIVEN apply and verify have both completed for a change
- WHEN SDD reaches its post-verify, pre-archive status resolution — `internal/sddstatus`'s `Resolve()`/`resolveEngramStatus()`, through `applyReviewOfferRouting` and `review_door.go`'s `reviewOfferForVerify` (call-site amendment, 2026-08-03: the originally-named `internal/cli` verify-success exit was found genuinely underspecified — repo/context-free by `RunSDDVerifyValidate`'s own doc comment — and the routing surface that already owns integration was chosen instead; `RunSDDVerifyValidate` itself stays context-free)
- THEN it calls `OfferReviewAfterVerify` as the sole review entry point
- AND no code path offers or consults RDD before verify completes

#### Scenario: No pre-verify blocking control remains

- GIVEN a change mid-apply, verify not yet run
- WHEN SDD status is queried
- THEN SDD does not block verify on any review transaction
- AND `applyPreVerifyReviewRouting` is absent from the call graph, not merely bypassed

### Requirement: Kill-Switch-Off Is Structural Absence, Proven by Call-Absence

When the RDD kill switch is OFF, zero review code MUST execute on any SDD path: no offer, no status consultation, no disabled/unmanaged ceremony capable of failing or blocking. This MUST be proven by a call-absence assertion (static call-graph/AST guard demonstrating no edge from any SDD apply/verify/archive path into `ReviewCore`/offer symbols). A passing unit test of the disabled branch alone is explicitly NOT acceptable evidence for this requirement. (Issue #1227)

#### Scenario: Call-absence guard proves invisibility

- GIVEN the kill switch is OFF
- WHEN the call-absence guard runs (CodeGraph/AST-based, not a runtime test)
- THEN it asserts zero call edges from `internal/sddstatus` apply/verify/archive paths into any `ReviewCore` or offer-transition symbol
- AND a green "disabled path" behavioral test alone MUST NOT be accepted as satisfying this scenario

#### Scenario: Archive is unfailable on review grounds when OFF

- GIVEN the kill switch is OFF
- WHEN archive runs
- THEN archive consults no `reviewGate` structured status
- AND archive cannot fail or block for review reasons

### Requirement: Decline Proceeds to Unmanaged Ordinary Archive

WHEN the offer is declined, SDD MUST proceed to unmanaged ordinary archive under existing repository policy. SDD MUST NOT block archive on a decline and MUST NOT create a receipt-like record for a declined offer.

**Amendment (corrective verify cycle 4, 2026-08-03): the offer is an invitation, never a gate — implementation and the "disabled/unmanaged policy" phrase clarified.** This is the maintainer-ratified reading (cited by the coordinator's cycle-4 fix scope) and closes what was an openly-reported, uncovered spec-MUST since cycle 1 (W3), carried forward through cycles 2-3 as W-b and flagged blocking in cycle 3's own pass/fail budget policy. Decline is realized as the absence of action, not a verb: there is no `--consent declined` form of `review start` and no persisted decline state (consent stays scoped to one candidate and is never recorded) — with the switch ON, verify passed, and no review ever started for the candidate (`reviewtransaction.discoverNativeReceipts` reports its terminal inventory genuinely empty, not merely ambiguous or stale), `dependencies.archive` stays `ready` and `reviewGate` is structurally absent (nil, `omitempty`) in the SAME status output that carries the present `reviewOffer` block. A later status read of the same still-unarchived candidate gets an identical, unsuppressed offer — nothing about the decline is remembered.

This scoping does not make any discovered review result an archive gate. A receipt that was created and is ambiguous, stale, or otherwise invalid remains visible through populated informational `reviewGate` context so maintainers can repair the review lifecycle, but it MUST NOT change archive readiness, add an archive blocker, or route `nextRecommended` to review remediation. Review integrity is never laundered into approval; it is reported independently while archive continues under ordinary repository policy.

"Archive completes under ordinary `disabled/unmanaged` policy" in the scenario below is satisfied by CRITICAL-1's already-ratified structural-absence shape (no populated disposition marker at all, not even `delivery: disabled/unmanaged`) rather than by emitting that literal disposition string on the decline path: emitting a marker here would be the same "disabled/unmanaged ceremony" CRITICAL-1 removed from the kill-switch-off path, applied to decline instead of to the switch. Structural absence is the more consistent reading of "zero review code MUST execute" once no receipt exists at all.

#### Scenario: Decline does not block archive

- GIVEN the post-verify offer is presented and declined
- WHEN SDD proceeds to archive
- THEN archive completes under ordinary `disabled/unmanaged` policy (realized as `reviewGate` structural absence, per the amendment above — not a populated disposition marker)
- AND no receipt or receipt-like artifact is created for the declined offer

### Requirement: Post-Offer Correction Triggers Targeted Re-Verify Before Archive

A bounded correction issued after the offer MUST invalidate prior verify evidence for the paths it touches. SDD MUST run a targeted re-verify scoped to the correction's changed paths before archive proceeds; SDD MUST fall back to a full re-verify only when path-level scoping cannot be proven.

**Amendment (corrective verify cycle 3, 2026-08-03): archive-gating enforcement deferred to Wave 5.** Wave 4 delivers the routing/classification half of this requirement in full: `Status.ReVerify` (Mode/Scope/Reason) is computed and reaches the wire whenever a post-verify correction is recorded, using the three-branch taxonomy the scenarios below describe. It does **not** yet enforce "archive does not proceed until that re-verify passes" — a second corrective cycle attempted that enforcement and found it unsatisfiable and livelocking: the demanded evidence revision was re-derived from the live verify-report on every status read, so a compliant re-verify re-labeled the demand instead of clearing it, and the only existing write path capable of recording satisfaction (`gentle-ai sdd-attempt finish --remediates-evidence-revision <rev>`) requires `--expected-binding-revision` and `--successor-lineage` together — a full review round trip, which defeats the "run a cheap targeted re-verify" scenario entirely. A compliant Wave 5 implementation needs: (1) a demanded-revision anchor derived from the correction's own append-only data (e.g. its `FixDeltaHash`), not the live verify-report, so satisfaction is stable once granted; and (2) either a new, decoupled write path for recording "this re-verify demand was satisfied" that does not require an approved review successor, or an explicit product decision that satisfying a targeted re-verify legitimately requires one. Until Wave 5 lands this, the scenario below's "archive does not proceed" clause is aspirational, not enforced, and archive is never blocked by an outstanding `Status.ReVerify` block.

#### Scenario: Targeted re-verify for a scoped correction

- GIVEN a post-verify correction that touches a known, provable subset of paths
- WHEN SDD re-runs verify before archive
- THEN re-verify is scoped to exactly those changed paths
- AND unrelated prior verify evidence remains valid

#### Scenario: Full re-verify when scoping is not provable

- GIVEN a post-verify correction whose changed-path set cannot be reliably derived
- WHEN SDD re-runs verify before archive
- THEN SDD runs a full re-verify covering the objective's entire evidence goal
- AND archive does not proceed until that full re-verify passes (DEFERRED to Wave 5 — see amendment above; not enforced in Wave 4)

### Requirement: Intra-Wave Rollout Sequencing

Wave 4 delivery MUST land in this order: (1) the offer call site wired first, (2) transport capability admission wired second, (3) review-binding mirror deletion last, since binding removal is destructive and irreversible.

#### Scenario: Mirror deletion lands after offer and capability are live

- GIVEN Wave 4 implementation in progress
- WHEN the offer call site and capability admission are both live and verified
- THEN only then does the review-binding mirror deletion PR land
- AND no PR deletes the mirror before the offer call site exists
