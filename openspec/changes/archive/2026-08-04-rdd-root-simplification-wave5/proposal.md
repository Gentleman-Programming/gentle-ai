# Proposal: RDD Root Simplification — Wave 5 (Gate Cutover)

## Intent

Wave 3 added `resolveGoverningAuthority` inside the single `runReviewFacadeValidate` funnel as one *additive* branch; Wave 4 moved consumers to opaque transitions and `ReceiptRef`. Delivery gates still run two truths:

1. **A gate mutates authority.** `InvalidateApprovedCompactAuthority` takes the writer lock, rewrites state to `invalidated`, and `os.Remove`s the receipt. A read-only check owns a write path, so a delivery question can destroy delivery authority.
2. **Delivery authority is composable per gate.** `EvaluateCompactPrePRChain` composes a receipt graph only for pre-PR, and `ResolveCandidateDeclineForGate` authorizes delivery from a decline. Each is a gate-specific exception to "one immutable receipt authorizes delivery".

Cutover converts the additive branch into the only path for **all** lineages, legacy included. The design's `Receipt-only delivery gates` contract then holds literally: validate against the live boundary, or deny with a derived relation.

## Scope

### In scope

- All five gates (`post-apply`, `pre-commit`, `pre-push`, `pre-pr`, `release`) evaluate every lineage through the shared relation algebra plus read-only receipt validation. No per-gate and no per-lineage branch survives.
- **REMOVE gate invalidation writes.** A gate returns a derived mismatch; `invalidated` becomes fully derived. Existing persisted `invalidated` records stay readable.
- **REMOVE chain-specific delivery exceptions** — pre-PR receipt-graph composition at delivery time.
- **DOWNGRADE candidate-decline authorization** to ordinary unmanaged policy (design line 330 / control matrix `DOWNGRADE`).
- **Exit evidence**: gate-outcome boundary matrix, 5 gates × 7 algebra relations (`exact`, `compatible_base_advance`, `provable_contraction`, `changed`, `ambiguous`, `unknown`, `unrelated`), extending Wave 1's differential matrix from relations to gate verdicts.
- **Blocking budget**: every denial carries an executable next step — a typed transition or `stop` + `reason_code`.

### Out of scope

- Legacy state-machine deletion (W7; legacy records remain readable and validatable).
- Adapter changes (W4), descendant closure (W6), contract major bump.
- Any new repair verb or recovery lineage.

## Capabilities

### New

- `rdd-delivery-gate-cutover`: one read-only evaluation path for all five gates and all lineages; derived mismatch instead of invalidation writes; no composed or decline-sourced delivery authority; executable next step on every denial.

### Modified

- `rdd-review-core-transitions` (W3): `validate` becomes the single governing path for legacy lineages, not an additive new-lineage branch.
- `rdd-candidate-relation-algebra` (W1): gate boundary descriptors become first-class algebra inputs; gate verdict is a total function of relation × gate.
- `rdd-new-lineage-activation` (W3): Amendment C coexistence precedence generalizes into unconditional receipt precedence.

## Approach

Invert the gate's role from *actor* to *reporter*. `EvaluateCompactGate` keeps deriving the live boundary but its only outputs are a verdict and a derived relation; the mutation callers (`InvalidateApprovedCompactAuthority`, the receipt `os.Remove`) are deleted rather than guarded — a guarded write is still a write. `runReviewFacadeValidate` loses its three gate-shaped special cases (pre-PR chain composition, decline resolution, per-lineage discovery fork) and gains one ordered contract: capability → kill switch → governing authority → relation → verdict + next step. Legacy receipts are validated *through the algebra without rewriting them*.

## Affected areas

| Area | Impact | Description |
|---|---|---|
| `internal/reviewtransaction/compact_approved_invalidation.go` | Removed | Gate-triggered authority mutation and receipt deletion |
| `internal/reviewtransaction/compact_chain.go` | Removed | Pre-PR receipt-graph delivery composition |
| `internal/reviewtransaction/candidate_decline.go` | Removed | Decline-sourced delivery authorization → unmanaged policy |
| `internal/reviewtransaction/compact_gate.go` | Modified | Read-only verdict + derived relation only |
| `internal/reviewtransaction/gate.go` | Modified | `NativeGateEvaluation` carries relation + next step |
| `internal/cli/review_facade.go` | Modified | Single ordered funnel; special cases deleted |
| `internal/reviewtransaction/transaction.go` | Modified | `StateInvalidated` parse-only; no write path |
| `internal/reviewtransaction/testdata/` | New | Gate boundary matrix golden (35 cells) |

## Risks

| Risk | Likelihood | Mitigation |
|---|---|---|
| Deliveries that previously passed via chain composition or decline now deny | High | Intentional, but every denial must name a runnable next step; matrix proves the deny is derived, not accidental |
| Cutover loses the disabled-gate short-circuit, re-introducing #2222/#2239 | Med | Kill-switch check ordered *before* any authority read in the unified path; regression test per gate |
| Consumers depending on receipt-file deletion as the invalidation signal | Med | Receipt file now persists while validation denies; audit every reader of `ReceiptPath()` |
| In-flight legacy corrections at cutover | Med | Read-only compatibility: correction continues on its existing authority; only the *gate* changes |
| Matrix asserts implementation instead of contract | Med | Golden generated from the algebra, reviewed as a covering array like Wave 1's 40-row golden |

## Rollback

Gate-scoped and one-directional: **a gate may deny; it cannot revive legacy mutation.** Rollback restores the Wave 3/4 shape by re-adding the additive branch, never by re-enabling invalidation writes. No authority is rewritten and no receipt is invalidated by this wave, so rollback needs no data migration.

## Dependencies

- Wave 4 exit evidence (real runtime evidence per declared transport).
- Wave 3's `resolveGoverningAuthority` funnel and Amendment C precedence.
- Wave 1's relation algebra and differential-matrix generator.

## Success criteria

- [ ] No gate code path writes authority, removes a receipt, or acquires the writer lock (proven by call-absence, not by a passing green path).
- [ ] `invalidated` is derived at every gate; no new `invalidated` record is written via any gate or compact path. The legacy-v1 `review invalidate` operator branch retains its write until Wave 7 deletes it.
- [ ] The 35-cell gate boundary matrix executes with zero unexplained divergences.
- [ ] Pre-PR uses the same evaluation as the other four gates; no gate-specific composition exists.
- [ ] A declined candidate reaches ordinary unmanaged delivery with no receipt-like record.
- [ ] Every denial in the matrix carries an executable next step (typed transition or stop + reason).
- [ ] Legacy-lineage receipts validate through the algebra with byte-identical stored bytes before and after.
- [ ] #2222 and #2239 are closed as superseded, or their behavior is proven preserved by named tests.

## Unresolved decisions (recommended defaults)

| # | Question | Recommended default |
|---|---|---|
| 1 | How are legacy-lineage receipts validated? | Through the algebra with **no rewrite**: legacy bytes are read, projected into `CandidateIdentity`, and compared. No in-place translation, per the wave-table rule. |
| 2 | In-flight legacy corrections at cutover? | Correction lifecycle is untouched; only delivery evaluation cuts over. A correction started pre-cutover finalizes normally and its receipt is validated by the new path. |
| 3 | #2222/#2239 supersession conditions? | Supersede only when the unified path has a per-gate regression test proving each PR's exact behavior (disabled short-circuit; kill switch before pre-PR composition — the latter is trivially preserved since composition is deleted). Otherwise land them first. |
| 4 | #2126 (dual-row conflict, W1 + W5) | Its self-loop exclusion belongs to the algebra (W1); W5 only consumes it. No W5 work item. |
