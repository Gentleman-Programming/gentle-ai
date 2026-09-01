# Design: RDD Root Simplification — Wave 5 (Gate Cutover)

## Evidence correction (read before the decisions)

The proposal claims "a gate mutates authority". Verified against `d591f4cf`, that is **imprecise and the design corrects it**: `EvaluateCompactGate` (`compact_gate.go:138`) already calls `evaluateCompactGate(..., authorityLockHeld=false)` and writes nothing, and `compact_approved_invalidation.go:62-64` states the invariant explicitly ("Gate reads remain pure; this explicit transition is the sole mutation boundary"). The real coupling is that an **explicit operator verb** — `review invalidate`, `review_facade.go:1371` → `InvalidateApprovedCompactAuthority` — takes the writer lock, re-derives a gate denial as its *justification*, rewrites state to `invalidated`, and `os.Remove`s the receipt. Wave 5 therefore removes a **gate-derived write verb**, not a hidden write inside a gate. Non-test callers: exactly one (`review_facade.go:1371`); one test caller in `internal/sddstatus/runtime_ledger_self_remediation_test.go:123`.

**Routing gate (hard block).** `resolveGoverningAuthority`, `CandidateIdentity` promotion, `ReceiptRef`, and capability admission do not exist at `d591f4cf` — they are Wave 3/4 deliverables. Wave 5 is HARD-GATED on Waves 3 and 4 landing; no Wave 5 slice may start before both merge.

**Amendment (Slice 2 implementation, documented per the PR0 precedent):** the "two late kill-switch reads (`:2905`, `:2967`)" and "Removes `:2905` and `:2967`" references below describe the funnel's shape at `d591f4cf`, before Wave 3's Amendment C landed. By Slice 2's implementation the funnel actually had **three** `reviewDeliveryDisposition` call sites, not two: Amendment C's own `resolveGoverningAuthority` discoveryErr branch (a new site Wave 3 introduced), the contested compact+legacy branch, and the `disabledDiscovery` branch. Decision 4's own rationale column already anticipated this ("Three reads of one switch is the #2222/#2239 bug"); only the exit-count prose and line numbers were stale. All three collapsed into the single early consultation Decision 4 specifies; substance unchanged.

## Technical Approach

Invert the gate from actor to reporter and collapse the funnel. `runReviewFacadeValidate` (`review_facade.go:2822`) today has four exits — compact receipt (`:2921`), pre-PR chain composition (`:2924`, `:2933`), decline authorization (`:2941`), legacy subprocess re-entry (`:3000-3023`) — plus two late kill-switch reads (`:2905`, `:2967`). Wave 5 replaces all of it with one ordered contract:

    capability → kill switch → governing authority → relation → verdict + next step

Legacy lineages traverse the same contract by **read-only projection** into `CandidateIdentity`; their stored bytes are never rewritten. This makes the root design's `Receipt-only delivery gates` contract (design.md:332) literally true for all five gates and every lineage kind.

## Architecture Decisions

| # | Choice | Rejected alternative | Rationale |
|---|---|---|---|
| 1 | **Cutover mechanics.** Wave 3's `resolveGoverningAuthority(new, legacy)` keeps its 2×2 matrix, but the cell "new absent, legacy present ⇒ legacy path byte-identical" changes to: project the legacy `ValidatedChain` head transaction + `facadeArtifacts.receipt` through a new read-only `projectLegacyAuthority(chain, artifacts) (CandidateIdentity, ReceiptRef, error)` and evaluate via `relateCandidates`. Delete `runFacadeLegacyValidateNegotiated` re-entry from the funnel. Zero historical bytes rewritten. (`resolveGoverningAuthority`, `CandidateIdentity`, `ReceiptRef` are **forward references** — Wave 3/4 deliverables absent at `d591f4cf`; this decision cannot execute before both land.) | Translate legacy records into v3 on first gate touch (in-place migration) | The wave-table rule forbids translating existing authority in place; migration destroys the "byte-identical stored bytes before/after" exit criterion and makes rollback destructive. Projection is a pure function of bytes already on disk. |
| 2 | **Invalidation derivation.** DELETE `InvalidateApprovedCompactAuthority`, `CompactApprovedInvalidationRequest`, `invalidateApproved`, `compactInvalidationTarget*`, `compactInvalidationDenialBound`, and the `review invalidate` compact branch. `invalidated` becomes a **derived verdict**: `relation ∈ {changed, unrelated} ⇒ GateInvalidated`. `StateInvalidated` + `InvalidationEvidence` stay **parse-only** in `transaction.go` so historical records still render. The receipt file is never removed. **Scope**: no new `invalidated` record is written via any gate or compact path; the legacy-v1 `review invalidate` operator branch retains its write until Wave 7 deletes it. | Keep the verb behind a guard flag; or keep only the `os.Remove` | A guarded write is still a write and still needs the writer lock in a read path. Removing the file without the state (or vice versa) leaves a skew that every reader must classify as corruption. |
| 3 | **`NativeGateEvaluation` evolution — additive.** Add two fields to `gate.go:109`: `Relation CandidateRelation` and `Next *GateNextStep{Transition string; ReasonCode string}`. `Result`, `Reason`, `Context`, `Cause`, `Contended` unchanged. All `NativeGateEvaluation` composite literals are keyed — verified zero unkeyed across non-test (47 sites) and test code — so the additive field compiles untouched; `internal/sddstatus` reads only `Result`/`Reason`/`Context`. | Replace `Result` with the relation (shape change) | A shape change touches 14 sites plus persisted `GateContext` comparison in `sddstatus.boundGateContextMatches` and widens the mixed-binary compatibility surface the `compact_chain.go:259-267` comment already warns about. |
| 4 | **Kill-switch ordering.** ONE `reviewDeliveryDisposition(ctx, root, false)` call immediately after flag/contract resolution and **before** `discoverCompactFacadeGateReview` or any authority read. Disabled ⇒ `emitDisabledUnmanagedDelivery` with `reason_code: reviews_disabled` and no discovery kind. Removes `:2905` and `:2967`. | Keep the two late reads and add an early third | Three reads of one switch is the #2222/#2239 bug with more places to drift. Early-and-only is what makes "reviews off ⇒ RDD does not exist" provable by call-absence. |
| 5 | **Characterization first (slice 1).** Before any removal, pin the legacy funnel's (`runFacadeLegacyValidateNegotiated`) observable contract and the invalidation verb's behavior with Wave-1's golden covering-array pattern (`-update`, deterministic rows). `EvaluateCompactPrePRChain`/`compactPrePRChainProof` already has covering tests — slice 1 adds composition-removal DELTA rows on top of those existing rows, not fresh characterization from scratch. Removal slices delete code **and** the rows whose behavior is intentionally gone, keeping surviving rows. `ValidatedChain` itself is NOT deleted here — legacy deletion is Wave 7; only its *funnel* role changes. | Delete first, observe divergence in review | `EvaluateCompactPrePRChain` already has 1101 lines / 25 test funcs in `compact_chain_test.go`; the genuine zero-coverage gap is the legacy funnel (`runFacadeLegacyValidateNegotiated`, no test references). Characterization slice = legacy funnel first-class characterization + composition-removal deltas layered onto the existing `compact_chain_test.go` rows; deleting the unpinned funnel would make every later divergence unattributable. |
| 6 | **Decline removal.** DELETE `ResolveCandidateDeclineForGate`, the funnel branch `:2941-2945`, `emitCandidateDeclinedUnmanagedDelivery`, and the writer `RecordCandidateDecline` (its only non-test caller is the consent decline at `review_facade.go:1606`). Decline ⇒ ordinary unmanaged delivery, output byte-identical to kill-switch-off. `parseCandidateDeclineAuthorization` survives **read-only** so `review status` can still describe pre-existing records; Wave 7 deletes the files. | Keep writing inert decline records as audit | Consistent with Wave 4 decision 4 ("decline = unmanaged proceed: nothing recorded"). An inert authority-shaped file is exactly the mirror pattern Wave 4 removed. |
| 7 | **Boundary matrix fixtures.** Extend Wave 1's generator (`shadow_matrix_test.go` corpus → `evaluateShadowMatrixCase` → golden + `shadowMatrixExitBarBlockers`) into `testdata/gate-boundary-matrix.golden`: 5 gates × 7 relations = 35 rows of `{gate, relation, verdict, next_step, explained, reason}`, generated from the algebra, reviewed as a covering array, unexplained divergence = exit-bar blocker. **Pre-PR divergence is pinned, not hidden**: `compact_gate.go:91-102` forces `baseMatches = true` and admits a current-changes boundary proof, so pre-PR's `compatible_base_advance` and `changed` cells legitimately differ from the other four gates and MUST carry `explained: true` with the boundary-proof reason. | 35 handwritten table tests | Handwritten cells assert the implementation back at itself and rot silently; a generated covering array with an explicit explained-divergence channel is the Wave-1 pattern that already caught a real algebra gap. |
| 8 | **Slicing.** Seven chained slices ≤1000 authored lines, destructive slices last (see below). | One "cutover" PR | Every removal slice depends on evidence a prior slice produced. |

## Data Flow

**Forward references** (not present at `d591f4cf`; Wave 3/4 deliverables — Wave 5 is hard-gated on both landing first): `capability admission` (W4), `resolveGoverningAuthority` (W3), `CandidateIdentity`, `ReceiptRef`.

    runReviewFacadeValidate
      ├─ capability admission (W4)
      ├─ KILL SWITCH  ── disabled ─→ disabled/unmanaged, reason_code=reviews_disabled  [no authority read]
      ├─ resolveGoverningAuthority (W3)
      │     ├─ v3 record present ─→ CandidateIdentity
      │     └─ v3 absent, legacy present ─→ projectLegacyAuthority (READ-ONLY) ─→ CandidateIdentity
      ├─ relateCandidates ─→ relation ∈ {exact, compatible_base_advance, provable_contraction,
      │                                   changed, ambiguous, unknown, unrelated}
      └─ verdict(gate, relation) ─→ NativeGateEvaluation{Result, Relation, Next}
                                     denial ⇒ Next = typed transition | stop + reason_code

    DELETED edges: EvaluateCompactPrePRChain · ResolveCandidateDeclineForGate ·
                   InvalidateApprovedCompactAuthority · legacy subprocess re-entry

## File Changes

| File | Action | Description |
|---|---|---|
| `internal/reviewtransaction/compact_approved_invalidation.go` | Delete | Gate-derived write verb; `invalidated` becomes derived |
| `internal/reviewtransaction/compact_chain.go` | Delete | Pre-PR receipt composition (`compactPrePRChainProof` and helpers) |
| `internal/reviewtransaction/candidate_decline.go` | Modify (mostly delete) | Resolver + writer removed; parser kept read-only |
| `internal/reviewtransaction/gate.go` | Modify | Additive `Relation` + `Next` on `NativeGateEvaluation` |
| `internal/reviewtransaction/compact_gate.go` | Modify | Read-only verdict + relation; pre-PR boundary rule stays, now declared as an explained matrix divergence |
| `internal/reviewtransaction/transaction.go` | Modify | `StateInvalidated` / `InvalidationEvidence` parse-only |
| `internal/reviewtransaction/legacy_projection.go` | Create | `projectLegacyAuthority` read-only chain→identity projection |
| `internal/cli/review_facade.go` | Modify | One ordered funnel; `review invalidate` compact branch removed |
| `internal/sddstatus/runtime_ledger_self_remediation_test.go` | Modify | Drops the invalidation-verb caller |
| `internal/reviewtransaction/testdata/gate-boundary-matrix.golden` | Create | 35-cell covering array |

## Interfaces / Contracts

```go
type GateNextStep struct { Transition string `json:"transition,omitempty"`; ReasonCode string `json:"reason_code"` }
// additive on NativeGateEvaluation: Relation CandidateRelation; Next *GateNextStep
func projectLegacyAuthority(chain ValidatedChain, artifacts facadeArtifacts) (CandidateIdentity, ReceiptRef, error) // read-only
func gateVerdict(gate GateKind, relation CandidateRelation) (GateResult, GateNextStep) // total function
```

**Amendment (Slice 3 implementation, documented per the PR0/Slice 2 precedent):** `gateVerdict`'s shipped signature is `gateVerdict(gate GateKind, relation CandidateRelation, context GateContext) (GateResult, GateNextStep)` — a disclosed, additive extension of the two-argument sketch above. The literal two-argument signature cannot express a per-gate boundary precondition at all, and the absorbed N2 debt (task 4.7, closed this slice) specifically requires `gateVerdict` to consult `BaseRelationshipValid`/`Release` — fields that live on `GateContext`, not on `gate`/`relation` alone. `Result`/`Next` stay the same two return values; only one additional input parameter was added, and only to carry information the function already needed to be total and correct.

## Testing Strategy

| Layer | What to Test | Approach |
|---|---|---|
| Characterization | Legacy funnel, pre-PR composition, invalidation verb — before removal | Wave-1 golden corpus with `-update`; deleted rows removed with their code |
| Unit | `gateVerdict` totality (35 cells), `projectLegacyAuthority` purity, derived `invalidated` | Table-driven; AST/guard test proving no gate path calls `acquireStoreLock`, `writeAtomic`, or `os.Remove` |
| Golden | 35-cell gate boundary matrix; disabled-gate output per gate | `testdata/gate-boundary-matrix.golden`, exit-bar blocker on unexplained divergence |
| Regression | #2222 (disabled short-circuit) and #2239 (kill switch before composition) per gate | Named per-gate tests; #2239 trivially preserved once composition is deleted |
| Byte-identity | Legacy receipt bytes unchanged across a full validate at all five gates | Hash `review-state.json` + `review-receipt.json` before/after |
| Bench | Declined candidate reaches ordinary unmanaged delivery; every denial names a runnable next step | Black-box journeys per `rdd-defect-workflow` |
| Regression | In-flight correction opened pre-cutover finalizes under the prior lifecycle; its receipt then validates via the new read-only path | Correction opened on pre-cutover code, finalized post-cutover, receipt validated through the cutover path |

## Threat Matrix

| Boundary | Applicability | Design response | Planned RED tests |
|---|---|---|---|
| Routing | **Applicable** — the funnel's exits collapse from four to one | Single ordered contract; total `gateVerdict`; no per-gate branch | Each gate × each relation reaches exactly one exit |
| Shell / subprocess | **Applicable** — gate target derivation runs `git`; legacy re-entry subprocess is removed | Reuse existing isolated-git recipe; projection adds no new process | Infrastructure fault ⇒ fail closed, never a derived allow |
| Git repository selection | **Applicable** — legacy + v3 roots under the common dir | Common-dir authority + identity lease unchanged | Relative `--cwd`, linked worktree, symlinked common dir, foreign repo |
| Commit state | **Applicable** — post-apply/pre-commit projections | Empty index / unborn HEAD ⇒ `unknown` + stop | staged, `commit -a`, empty index, unborn HEAD |
| Push state | **Applicable** — pre-push/pre-PR boundary without composition | Single receipt only; boundary proof stays inside `compact_gate.go` | first push, diverged base, multi-commit delivery |
| VCS/PR automation | N/A — gates read publication boundaries; no PR command is composed | — | — |
| Executable-file classification | N/A — reused unchanged from `risk.go` | — | — |
| Process integration (adapters) | N/A — Wave 4 owns adapters | — | — |

## Migration / Rollout

No data migration; nothing is translated in place. Three **named output changes** ship with goldens:

1. Receipt files persist through a derived invalidation. Consumers that read *file absence* as the invalidation signal MUST read the gate verdict instead — audit every `ReceiptPath()` reader before slice 7.
2. Disabled-gate output carries `reason_code: reviews_disabled` with no discovery kind, because the switch is now consulted before discovery runs. This is an intentional loss of discovery detail while reviews are off.
3. Deliveries that previously passed via pre-PR composition or a decline now deny — with a runnable next step.

4. A correction opened before cutover finalizes under the pre-cutover correction lifecycle; its receipt then validates through the new read-only path once complete (`rdd-new-lineage-activation/spec.md:55-58`).

Rollback is gate-scoped and one-directional: a gate MAY deny, it CANNOT revive legacy mutation. Restore the Wave 3/4 shape by re-adding the additive branch, never by re-enabling invalidation writes.

## PR Slicing Preview (for sdd-tasks)

Feature-branch chain after Wave 4; ≤1000 authored lines/slice; deadcode ratchet per slice; **cross-slice fixes ride slice 1**.

| Slice | Work unit | Forecast |
|---|---|---|
| S1 | Characterization corpus + gate-boundary matrix harness (zero behavior change) | ~650 |
| S2 | Kill switch consulted once, before any authority read + per-gate disabled goldens | ~350 |
| S3 | `NativeGateEvaluation` additive `Relation`/`Next`; every denial carries an executable next step | ~700 |
| S4 | `projectLegacyAuthority`; legacy lineages evaluate through the algebra; byte-identity proof | ~900 |
| S5 | Pre-PR composition deletion; pre-PR cells become explained divergences | ~800 |
| S6 | Decline downgrade to ordinary unmanaged; read-only parser retained | ~500 |
| S7 | Invalidation verb deletion, `StateInvalidated` parse-only (LANDS LAST — only destructive authority step) | ~600 |

**Rejected**: S4+S5 combined (mixes projection evidence with composition-removal evidence, over budget); S7 earlier (destroys the write path before its derived replacement is proven).

## Open Questions

- [ ] Slice 7 removes the only `os.Remove(ReceiptPath())` caller. RATIFIED (maintainer, 2026-08-02): resolved by audit, not assumption — the `ReceiptPath()` reader audit sweeps in-repo and bundled Pi assets before slice 7; findings migrate to `review validate`, plus an rc release-notes line about receipt-file persistence under derived invalidation.

`review invalidate` keeps a legacy-v1 branch after the compact branch is deleted (see Decision 2 scope line): the verb stays for legacy only until Wave 7 deletes it — this is a stated scope, not an open question.

- [ ] #2222/#2239 supersede only once S2 lands each PR's exact behavior as a named per-gate test; otherwise land those PRs first. RATIFIED (maintainer, 2026-08-02): close as superseded once this ordering is proven by the named per-gate regression test.
