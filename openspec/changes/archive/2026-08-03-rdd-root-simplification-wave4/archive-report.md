# Archive Report: RDD Root Simplification — Wave 4 (Thin Consumers)

**Change**: `rdd-root-simplification-wave4`
**Archived**: 2026-08-03
**Archive location**: `openspec/changes/archive/2026-08-03-rdd-root-simplification-wave4/`
**Store**: hybrid (filesystem + Engram)

## What Shipped

Wave 4 inverted the SDD↔RDD dependency: SDD now calls RDD at exactly one point (post-verify offer)
and RDD never supervises the SDD cycle. Delivered on the `feature/rdd-root-simplification` tracker
branch as a feature-branch PR chain (PR0 + S1..S7): **PRs #2328–#2339**, preceded by sync PR **#2327**
and closed with boundary-merge PR **#2351**, landing on `main` at commit **943fdea3**.

Headline outcomes (see verify-report cycle 6 for full evidence):

- SDD persists only a terminal `SDDReceiptRef` per attempt plus its own work-unit attempts; the
  `gentle-ai.sdd-review-binding/v1` writer half is retired (two genuinely-orphaned symbols —
  `bindingExists`, `validateBoundReview` — deleted; the rest kept for Wave 7, see Deferrals below).
  No re-derived gate meaning remains in `internal/sddstatus`'s explicit-governance branch; validity is
  one call to `reviewtransaction.ValidateSDDReceiptRef`.
- The post-verify offer (`OfferReviewAfterVerify`) is wired to a live, non-test caller
  (`internal/sddstatus`'s `Resolve()`/`resolveEngramStatus()`, via `applyReviewOfferRouting` and
  `review_door.go`'s `reviewOfferForVerify`) and reaches the real CLI wire.
- Kill switch OFF is proven structural absence by call-absence guard (AST/call-graph, corroborated by
  a zero-count runtime counter across a full apply→verify→archive flow), not by a passing disabled-path
  test alone.
- Decline is realized as absence of action (no `--consent declined` verb, no persisted state):
  switch ON + verify passed + no receipt at all → `dependencies.archive` stays `ready` and `reviewGate`
  is structurally absent in the same output that carries the present `reviewOffer`.
- Transport capability (`ContractReviewTransportV1`) is declared by the adapter and admitted before any
  authority/tier/lens/budget/collection state exists; unsupported transport denies with zero recoverable
  remnant. Pi (out-of-repo) enters unavailable mode; OpenCode/Claude execute opaque transitions only.
- Decision 9 (attempt-ledger ownership) ratified: `RuntimeObjective` is the sole work-unit scope owner;
  `CompactAcquireRequest` collapsed into `BeginAttemptRequest`.
- Targeted re-verify's three-branch classification (`Status.ReVerify{Mode, Scope, Reason}`) is
  implemented and reaches the wire; its archive-gating enforcement half is explicitly deferred to
  Wave 5 (see Deferrals below).

Final evidence at the merged tip: `go build` exit 0; `go test ./... -count=1` exit 0, 63 packages `ok`,
0 `FAIL`; `gofmt -l .` / `go vet ./...` clean; bench corpus 59/59 and the opt-in `source-coupled` axis
60/60, both exit 0; `scripts/deadcode-ratchet.sh` reports no new unreachable functions.

## Verification Summary — Six Cycles

The change went through six `sdd-verify` cycles before reaching PASS. Trajectory:
cycle 1 `4/16 req, 16/30 scen` (FAIL, 3 CRITICAL) → cycle 2 `11/16, 24/30` (FAIL, 3 CRITICAL) →
cycle 3 `13/16, 27/30` (FAIL, 0 CRITICAL, 1 blocker) → cycle 4 `11/16, 25/30` (FAIL, 2 CRITICAL —
adversarial pass first reached the shipped consumer contract) → cycle 5 `15/16, 30/31` (FAIL, 0 CRITICAL,
1 blocker) → cycle 6 `16/16, 31/31` (**PASS**, 0 CRITICAL, 0 blockers).

### The eight CRITICALs found and closed, in order

1. **CRITICAL-1 (cycle 1)** — `reviewGate` disabled/unmanaged ceremony still ran and was emitted when the
   kill switch was OFF, including a real `discoverNativeReceipts` repository walk and a live path that
   could set `Dependencies.Archive = blocked` while OFF. **Closed**: `applyReviewGate` gated on
   `reviewDisabled`, verdict neutralised structurally, no ceremony marker emitted.
2. **CRITICAL-2 (cycle 1)** — `Status.ReviewOffer`/`Status.ReVerify` never reached any consumer:
   `StatusV1Projection` had no field for either, so every CLI path (`sdd-status`, `sdd-continue`,
   markdown renderers) dropped them silently. **Closed**: both fields added to the v1 projection and
   proven present on the real CLI wire.
3. **CRITICAL-3 (cycle 1)** — the Wave 4 chain branched from `157ab9fd`, not Wave 3's tip `67be4867`,
   regressing clone-local kill-switch invisibility in `OfferReviewAfterVerify` (global-scope-only read).
   **Closed**: chain rebased onto `67be4867`; `OfferReviewAfterVerify` reads the effective (clone-scope)
   mode.
4. **CRITICAL-A (cycle 2, introduced by the cycle-1 corrective fix)** — the task-7.4 archive gate
   (`blockArchiveForUnsatisfiedReVerify`) livelocked: the demanded evidence revision was re-derived from
   the live verify-report on every status read, so a compliant re-verify re-labeled the demand instead of
   clearing it, and the named continuation command was not runnable standalone. **Closed** by removal in
   cycle 3, with the enforcement half honestly deferred to Wave 5 (see Deferrals).
5. **CRITICAL-B (cycle 2, residual C1)** — three sibling review-discovery paths inside the same
   `Resolve()`/`resolveEngramStatus()` bodies (`staleAllowAuthority`, `resolveCompactRemediationAuthority`,
   the `governingRef` branch) were still ungated by `reviewDisabled`, including one that could block
   verify/archive naming `gentle-ai review start` while the switch itself refuses that command.
   **Closed** in cycle 3, proven by strace zero-open evidence and RED strip-and-restore on all three gates.
6. **CRITICAL-C (cycle 2)** — the bench journey corpus was RED at the wave tip (6 failures) despite being
   GREEN at the wave base; six journeys (`j41`, `j53`–`j56`, `j58`) pinned the deliberately-removed
   pre-verify routing and were never updated. **Closed** in cycle 3: journeys rewritten with documented
   supersession comments, corpus 59/59 and the opt-in source-coupled axis 60/60.
7. **CRITICAL-D (cycle 4)** — the shipped `sdd-archive` skill and `_shared/review-ledger-contract.md`
   still stated the pre-Wave-4 archive-gate contract (`reviewGate.result: allow` or
   `disabled/unmanaged`), which would have refused exactly the states BLOCKER-1's runtime fix reported as
   `archive: ready`. **Closed** in cycle 5: five shipped surfaces (the skill, the shared contract,
   `sdd-status-contract.md`, and both `sdd-archive.md` command assets) rewritten to the structural-absence
   reading, goldens regenerated byte-scoped to the intended sentences.
8. **CRITICAL-E (cycle 4)** — `rdd-sdd-receipt-consumption`'s Legacy `reviewGate` v1 Field Compatibility
   requirement directly contradicted the cycle-4 BLOCKER-1 amendment (still said "populated while the
   kill switch is enabled" when measured behavior showed absent-with-switch-ON-and-no-receipt). **Closed**
   in cycle 5: requirement conditioned on "a review was actually discovered," a new scenario added, all
   three scenarios measured PASS.

Two further rule-driven blockers (not code defects) were resolved by explicit spec amendment rather than
implementation, following the wave's own established precedent (7.4, BLOCKER-1, requirement 11):

- **BLOCKER-1 (cycle 3)** — requirement "Decline Proceeds to Unmanaged Ordinary Archive" was an openly-
  reported, uncovered spec-MUST with no amendment. Runtime behavior for the underlying case (switch ON,
  verify passed, no receipt) was already correct by cycle 4; the sibling `reviewGate`-consumer-layer
  defect that made it look unmet is CRITICAL-D above.
- **BLOCKER-2 (cycle 5)** — requirement 13's "Missing trace blocks the task" scenario is a process
  counterfactual (constrains task ordering during the wave's own execution, not runtime behavior) with no
  possible runtime enforcement point. Closed in cycle 6 by an explicit amendment recording it as
  process-verified and covered-by-amendment, per the 7.4/decline/requirement-11 precedent. The amendment
  cited a pre-rebase commit SHA (`ead610f6`); cycle 6 confirmed the substance independently and identified
  the delivered equivalent (`acb3c7c1`, identical patch-id) — recorded as non-blocking WARNING W-e.

### Final envelope (cycle 6, PASS)

```yaml
schema: gentle-ai.verify-result/v1
evidence_revision: sha256:b0ce122da982ca99d537fcb4ce9d7d8ebbcb219a2f06059cbd3b113ba10efb96
verdict: pass
blockers: 0
critical_findings: 0
requirements: 16/16
scenarios: 31/31
test_command: go test ./... -count=1
test_exit_code: 0
build_command: go build -o gentle-ai ./cmd/gentle-ai
build_exit_code: 0
```

Candidate tip: `7598eda430a6cc1fe8ab6b62cd971862ba03a786`; merge-base with Wave 3's `67be4867` unchanged
across all six cycles. Non-blocking carry-forward at PASS: 4 WARNING (W-e citation-accuracy, W-a' the
read-elimination W-b bridge gate has no covering test, W-c the three legitimately-out-of-scope unchecked
tasks below, W-d targeted re-verify has no live end-to-end `Resolve()` proof) and 3 SUGGESTION (7.4's
checkbox/body mismatch, 7.4's MUST sentence lacks an inline deferral marker, `staleEvidenceUnmanaged`
archive-blocked-with-no-reasons UX).

## Task Completion Gate — Explicit Reconciliation Note

Three implementation tasks remain unchecked in the archived `tasks.md`: **1.2** (archive Wave 3 —
externally blocked on Wave 3's own archive phase, not a Wave 4 code task), **7.6**/**7.7** (RED/GREEN for
staged/`commit -a` commit-state branches — explicitly outside the coordinator's stated "S6 (7.1-7.3)"
batch scope; the shipped implementation reads persisted `CorrectionAttempts` data and performs no live
Git diff, so those commit states have no bearing on the code path as built).

All three carry written rationale in `tasks.md` and are classified WARNING (not CRITICAL) in every one of
the six verify cycles under the Decision Gates rule "CRITICAL for core task, WARNING for cleanup task" —
none is a Wave-4 implementation task in the wave's own scope, and cycle 6's PASS verdict states "Clear to
archive" with this judgment applied consistently and explicitly. Per the launch instruction's explicit
final-state direction to record these as deferrals rather than treat them as blocking stale checkboxes,
archive proceeds; this note is the required exceptional-reconciliation record per the sdd-archive skill's
Task Completion Gate.

## Deferrals

### Carried to Wave 5

- **Task 7.4's archive-gating sub-clause** ("record a new `RuntimeAttempt` … archive does not proceed
  until that re-verify passes"). A second corrective cycle attempted this and produced CRITICAL-A's
  livelock (demanded revision re-derived from the live verify-report, so it re-labeled itself after every
  compliant re-verify; the only write path capable of recording satisfaction required a full review round
  trip). Removed and honestly deferred rather than shipped livelocked or rushed. A compliant Wave 5
  implementation needs (1) a frozen demand anchor derived from the correction's own append-only data (e.g.
  `FixDeltaHash`), never the live verify-report, and (2) either a new decoupled write path in
  `runtime_ledger.go`'s `Finish()` CAS validation (its own security review needed) or an explicit ratified
  decision that targeted re-verify legitimately requires a full review round trip.
- **N1's lens-result ingestion dependency** — a pre-existing Wave 3 verify entry condition (Wave 3's N1/N2
  findings), explicitly not counted as a Wave 4 finding in every adversarial pass of this change, carried
  forward as a Wave 5 entry condition.
- Whether `SDDReceiptRef`/the compact receipt schema should carry correction-path data (so targeted
  re-verify's "not reliably derivable" branch stops being the general case) — open, noted in design.md,
  explicitly out of scope for the targeted-re-verify-call-site amendment.

### Carried to Wave 7

- **Binding/BindingRevision retirement** (design.md's decision-1 scope amendment, 2026-08-03).
  `RuntimeStatus.Binding`/`BindingRevision`, `BindApprovedReview`/`review bind-sdd`, and the entire
  remediation-successor CAS subsystem (`runtime_ledger.go`'s `Finish()`, `runtime_compact.go`'s `Settle()`,
  `validateRuntimeBoundCandidate`, `runtimeSelfSuccessorAvailable`, `runtimeStrandedSuccessor`) were found
  not compile-safe to remove atomically within Wave 4: `review_facade.go` calls `BindApprovedReview` as
  the live `review bind-sdd` command, and the remediation-successor CAS both reads and writes
  `RuntimeStatus.Binding` with no `SDDReceiptRef` analogue for its `ExpectedBindingRevision`/
  `SuccessorLineageID` inputs. Only the two genuinely-orphaned symbols the deadcode ratchet named after the
  archive-gate rewire (`bindingExists`, `validateBoundReview`) were deleted this wave; everything else is
  recorded one line each in the Wave 7 deletion inventory
  (Engram `sdd/rdd-root-simplification-wave7/deletion-inventory-w4-contributions`).
- **Task 8.5 — relaunch-bound-loss provider-side replacement** (coordinator ruling, 2026-08-03).
  `internal/assets/opencode/plugins/review-result-artifacts.ts`'s session-scoped admission-recovery
  bookkeeping (`admissionRecoveryKey`, `claimAdmissionRecovery`, `MAX_ADMISSION_RECOVERIES_PER_SESSION`) is
  the only implementation of a relaunch-bound safety property and was investigated but not built this wave
  — deferred to either Wave 5's entry (gates-cutover owns the admission flow) or Wave 7 (if the plugin's
  whole legacy consumption retires there instead).
- Legacy `reviewGate` v1 field removal for unmigrated Pi clients (kept this wave for compatibility, per
  `rdd-sdd-receipt-consumption`'s "ReceiptRef Lives in SDD's Runtime Ledger" requirement).

## Spec-Merge Inventory

| Domain | Action | Requirements | Scenarios | Notes |
|---|---|---|---|---|
| `rdd-sdd-receipt-consumption` | Created (new capability) | 5 | 11 | Copied verbatim (no prior main spec existed) to `openspec/specs/rdd-sdd-receipt-consumption/spec.md`, including the cycle-5 requirement-11 amendment |
| `rdd-post-verify-review-offer` | Created (new capability) | 5 | 9 | Copied verbatim to `openspec/specs/rdd-post-verify-review-offer/spec.md`, including the cycle-3/cycle-4 amendments (re-verify archive-gating deferral, decline-as-invitation-not-gate) |
| `rdd-transport-capability` | Created (new capability) | 4 | 8 | Copied verbatim to `openspec/specs/rdd-transport-capability/spec.md`, including the cycle-5 requirement-9 process-counterfactual amendment |
| `rdd-review-core-transitions` | Modified (existing Wave 3 capability) | +1 added, 1 modified | +2 added, +1 modified-in-place | `openspec/specs/rdd-review-core-transitions/spec.md`: "Consent-Gated Freeze With Immutable Tier, Lenses, and Budget" renamed to "…, Preceded by Capability Admission" with a new "Capability admission precedes candidate freeze" scenario appended; new requirement "Offer Transition Reachable From a Real Call Site" (2 scenarios) appended at the end. All five pre-existing Wave 3 requirements ("Sole Transition Owner for New Lineages", "Candidate-Causal Admission Only", "One Bounded Correction, Exact Replay Exempt", "Terminal Receipt Issuance Exactly Once", and the modified freeze requirement itself) preserved unmodified otherwise |

Total: 16 requirements / 31 scenarios landed across four capability domains, matching the verify-report's
final spec compliance matrix exactly.

## Observation IDs (Traceability)

| Artifact | Engram ID | Topic key |
|---|---|---|
| Proposal | #10135 | `sdd/rdd-root-simplification-wave4/proposal` |
| Spec | #10140 | `sdd/rdd-root-simplification-wave4/spec` |
| Design | #10141 | `sdd/rdd-root-simplification-wave4/design` |
| Tasks | #10147 | `sdd/rdd-root-simplification-wave4/tasks` |
| Verify report | #10173 | `sdd/rdd-root-simplification-wave4/verify-report` |
| Wave 7 deletion inventory (referenced) | #10166 | `sdd/rdd-root-simplification-wave7/deletion-inventory-w4-contributions` |

## Archive Contents

- `proposal.md` ✅ (119 lines)
- `design.md` ✅ (367 lines, including all three coordinator/orchestrator amendments)
- `tasks.md` ✅ (132 lines, 53/56 tasks complete, 3 legitimately deferred and documented)
- `verify-report.md` ✅ (1682 lines, all six cycle histories, byte-complete)
- `specs/rdd-post-verify-review-offer/spec.md` ✅ (92 lines)
- `specs/rdd-review-core-transitions/spec.md` ✅ (50 lines, delta)
- `specs/rdd-sdd-receipt-consumption/spec.md` ✅ (97 lines)
- `specs/rdd-transport-capability/spec.md` ✅ (81 lines)

## Source of Truth Updated

- `openspec/specs/rdd-sdd-receipt-consumption/spec.md` (new)
- `openspec/specs/rdd-post-verify-review-offer/spec.md` (new)
- `openspec/specs/rdd-transport-capability/spec.md` (new)
- `openspec/specs/rdd-review-core-transitions/spec.md` (merged)

## SDD Cycle Complete

Wave 4 has been fully planned, implemented, verified across six cycles, and archived. Wave 5 (gate
cutover) and Wave 7 (legacy deletion, including Binding/bind-sdd retirement) inherit the deferrals
recorded above.
