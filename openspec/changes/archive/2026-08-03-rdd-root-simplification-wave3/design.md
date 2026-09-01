# Design: RDD Root Simplification — Wave 3 (New-Lineage Facade)

## Technical Approach

Add one native owner beside the existing machinery. `ReviewCore` (start/finalize/validate) becomes the only transition selector for lineages it creates; `AuthorityStore` persists exactly `review-state.json` + `review-receipt.json` under a new `v3/` root. Wave 1's resolver/algebra is *moved* out of shadow gating so one implementation serves both the shadow observer and the live core. Consent, tier, lens selection, and correction budget are called from existing code, never re-derived. Legacy stays byte-identical behind a start-only activation switch.

## Architecture Decisions

| # | Choice | Rejected | Rationale |
|---|---|---|---|
| 1 | `internal/reviewtransaction/review_core.go` (same package). One entry `func (ReviewCore) Next(ctx, authority, CoreRequest) (CoreTransition, error)`; `CoreTransition.Kind ∈ {continue, collect, approve, escalate, repair, stop}` per the design's Transition-result vocabulary | New `internal/reviewcore` package; methods on `CompactState`; per-gate transition builders | A new package forces exporting `shadowRelate`, `classifyCompactPathSetRelation`, `deriveBaseAdvanceCompatibility`, `SnapshotBuilder.build`, `runGitIsolated` — or re-deriving them, which *is* the root cause. Per-gate builders reproduce `review_next_transition.go`'s split |
| 2 | Promote by rename: `shadow_relation.go`→`candidate_relation.go`, `shadow_identity.go`→`candidate_identity.go`; `shadowRelate`→`relateCandidates`, `ShadowRelation`→`CandidateRelation` (+ `type ShadowRelation = CandidateRelation` alias so `shadow_observer.go` compiles unchanged). `shadow_observer.go`/`shadow_authority_health.go` keep the prefix and `GENTLE_AI_RDD_SHADOW`. The `shadow_*.go` glob in `shadow_readonly_guard_test.go` therefore stops covering promoted files; add `candidate_readonly_guard_test.go` reusing `scanShadowReadOnlyTree` verbatim over `candidate_*.go` | Call-through wrappers; moving to a new package; deleting the guard | Two names for one function is the duplication being removed. Relation/identity must still never mutate — only `ReviewCore`/`AuthorityStore` may — so the guard narrows rather than disappears. This is why the shadow spec's disable switch must scope to *observation*: `shadowObservationEnabled()` gates only `ObserveShadowRelation` |
| 3 | `<git-common-dir>/gentle-ai/review-transactions/v3/<lineage>/{review-state.json,review-receipt.json}`, lock `v3/LOCK`, root from the existing `reviewAuthorityRoot`. Reuse `acquireStoreLock` + the `.atomic-`/rename publish idiom; CAS on `revision` = canonical-bytes digest (`CompactRevisionForState` idiom). `replay_identity` = sha256(domain, lineage, state, candidate identity, request digest): equal ⇒ return stored transition, consume nothing; different in `correcting` ⇒ refuse (budget spent). Derived observations computed at read by `DeriveObservation(authority, live)`, never a persisted field. `v3/LOCK` is store-scoped (one lock guarding the whole `v3/` root, not one per lineage), and `.atomic-*` staging files are transient pre-rename artifacts that never survive past a publish, so "exactly two artifacts per lineage" stays provable by directory inspection | Reusing `v2/` with a `lineage_kind` field; a fresh lock/CAS implementation | A shared directory makes "new lineages write no legacy artifact" unprovable by inspection and blocks Wave 7 deletion. New locking would be new unproven concurrency surface |
| 4 | Lineage kind fixed once at start (implied by `v3/`, asserted by `schema: gentle-ai.review-authority/v3`). One `resolveGoverningAuthority(new, legacy)` in the single shared `runReviewFacadeValidate` path — all five gates traverse it, so Amendment C is one additive branch, not five | Per-gate copies; a flag threaded through `EvaluateCompactGate` | Five branches is the split-ownership pattern again |
| 5 | `GENTLE_AI_RDD_NEW_LINEAGE`, default OFF, read **only** at `review start` (review_facade.go:1618, where `NewCompactState` sits today). Finalize/validate/gates route on discovered lineage kind and never read it. Three switches exist across this wave with three distinct, never-overloaded meanings: `GENTLE_AI_RDD_NEW_LINEAGE` (start-only *activation* of the v3 write path), `GENTLE_AI_RDD_SHADOW` (read-only *observation* gating `ObserveShadowRelation`, decision 2), and the user-owned RDD kill switch (`gentle-ai review mode enable/disable`, delivery-gate scope). None of the three substitutes for another | Gating every operation; a persisted repo setting | Gating finalize strands an in-flight lineage on rollback. A persisted setting outlives the code a structural revert removes |
| 6 | Reuse, do not reimplement: consent `authorizeReviewStart` + `newReviewIntegrationConsentResult` (v2 envelope unchanged); tier `SnapshotBuilder.AssessSnapshotRisk`→`ClassifyRisk`; lenses `facadeSelectedLenses`→`SelectReviewLenses`; budget `CorrectionBudget` (risk.go:227) with `MaxCorrectionChangedLines`; base advance `deriveBaseAdvanceCompatibility` via the promoted `shadowDeriveBaseAdvance`. Only the state machine and store are new. Per the reused consent gate's own contract (spec `rdd-review-core-transitions` :22), tier 0 proceeds without a consent question — freeze happens immediately after tier/lens/budget assignment; only tier 1|4 require the `gentle-ai.review-integration.consent/v2` envelope granted before freeze | Re-deriving tier/budget for the new record | Divergence in the same wave that replaces the state machine is unattributable |
| 7 | Reason regression: table-driven `TestNewLineageReasonTaxonomyCoversLegacyRefusals` keyed on the typed constants, no default fallback — `receipt_missing`/`receipt_unrelated`→`unrelated`·stop; `receipt_scope_changed`→`changed` (or `provable_contraction` when admitted findings stay in scope); `receipt_ambiguous`→`ambiguous`·stop; `authority_corrupted`→health `blocked`·escalate/repair; `target_unresolvable`→`unknown`·stop; denial `base-mismatch`→`changed`/`compatible_base_advance` | Prose mapping in docs | Five states can only be accepted if every user-visible refusal survives |
| 8 | Ship `OfferReviewAfterVerify(ctx, repo, OfferRequest) (Offer, error)` in `review_offer.go` unwired; kill-switch OFF returns `Offer{Available:false}, nil` **before** any repository read. Record the exact symbol in `.deadcode-baseline.txt` with the reason in the commit | Wiring a temporary SDD call site; deferring the API to Wave 4 | A call site is Wave 4's sequence change a wave early. Tests do not satisfy the ratchet (it analyses from `./cmd/gentle-ai`), so an honest baselined entry is the cost of shipping the shape |

## Data Flow

    start: snapshot freeze → AssessSnapshotRisk (:1564) → SelectReviewLenses → authorizeReviewStart(consent) (:1572) → authority freeze (:1618)
             └─ switch OFF ─→ NewCompactState → StartCompactAuthority        (legacy, unchanged)
             └─ switch ON  ─→ ReviewCore.Next → AuthorityStore.Create(v3)    (reviewing)
    finalize: admitted candidate_causal? → correcting → one bounded correction → validating → approved (receipt)
    validate: DiscoverNewLineage(v3) ─→ resolveGoverningAuthority ─→ ReviewCore.Validate (relateCandidates)
                                    └─ absent ─→ discoverCompactFacadeGateReview (unchanged)

**Amendment C matrix** (`resolveGoverningAuthority`): lineage kind is established *solely* by v3/ record presence under the resolved common dir — there is no independent "candidate marker" of lineage kind separate from the record itself. Consequently {new absent} means "legacy candidate" BY DEFINITION: the spec's literal "new lineage, no v3 record" scenario is unreachable by construction under normal operation. The full 2×2 matrix over {new: related | unrelated | absent} × {legacy: present | absent}: new related (anything but `unrelated`) ⇒ new receipt governs, legacy ignored, regardless of legacy presence. New present but `unrelated`, legacy present ⇒ **deny** (a new-lineage candidate is never authorized by legacy). New absent, legacy present ⇒ legacy path, byte-identical (today's behavior, unchanged). New absent, legacy absent ⇒ legacy's `receipt_missing` path, byte-identical. **Discovery-integrity case (not a matrix cell — a corruption path)**: when a candidate carries a new-lineage MARKER (e.g. a `v3/<lineage>` directory reference or discovery hint) but the v3 record itself is absent or unreadable — deleted, quarantined, corrupted, or under a foreign common dir — `resolveGoverningAuthority` MUST deny and route to `authority_corrupted` ⇒ escalate/repair (decision 7). It MUST NEVER fall through to legacy authorization in this case, even when a legacy receipt is present and otherwise valid.

## File Changes

| File | Action | Description |
|---|---|---|
| `internal/reviewtransaction/review_core.go`, `review_core_transition.go` | Create | Transition owner, causal admission, one-correction accounting |
| `internal/reviewtransaction/authority_store.go` | Create | v3 two-artifact CAS store, five states, replay identity, receipt publication |
| `internal/reviewtransaction/review_offer.go` | Create | Wave-4 offer API, unwired |
| `shadow_relation.go`→`candidate_relation.go`, `shadow_identity.go`→`candidate_identity.go` | Rename | Promotion; observer/health keep the shadow gate |
| `shadow_readonly_guard_test.go` + new `candidate_readonly_guard_test.go` | Modify/Create | Guard follows the promoted files |
| `internal/cli/review_facade.go` | Modify | Start branch (:1618), governing-authority branch (:2872) |
| `internal/cli/review_next_transition.go` | Modify | New-lineage transitions delegate to `CoreTransition` |
| `.deadcode-baseline.txt` | Modify | One entry for the unwired offer API |
| `bench/` + tests | Create | Tier journeys, precedence matrix, switch-off equivalence |

## Interfaces / Contracts

```go
type CoreTransition struct { Kind CoreTransitionKind; ReasonCode string; Collect []CoreInput; Receipt *ReceiptRef }
func (core ReviewCore) Next(ctx context.Context, authority NewLineageAuthority, request CoreRequest) (CoreTransition, error)
func (store AuthorityStore) Mutate(ctx context.Context, expectedRevision string, apply func(*NewLineageAuthority) error) (string, error)
func DeriveObservation(authority NewLineageAuthority, live CandidateIdentity) DerivedObservation // never persisted
```

## Testing Strategy

| Layer | What | Approach |
|---|---|---|
| Unit | Five-state transitions, replay vs. correction per state, derived-observation purity, Amendment C matrix | Table-driven; AST guard proves no `DeriveObservation` result is persisted; RED: new-lineage marker present, v3 record removed, legacy receipt present ⇒ deny |
| Integration | v3 store CAS/lock/crash/replay, two-artifact inspection (zero legacy files), receipt immutability | `t.TempDir()` real repos; existing `maintenance_lock_test.go`/phase-hook idioms |
| Equivalence | Switch OFF ⇒ byte-identical gate stdout at all five gates | Golden JSON per gate, regenerated only via `-update` |
| Bench | tier 0/1/4: candidate→consent→review→correction→receipt→five gates | Black-box journeys |

## Threat Matrix

| Boundary | Applicability | Design response | Planned RED tests |
|---|---|---|---|
| Documentation-like paths | N/A: classification reused unchanged from `risk.go`; no new file-execution boundary | — | — |
| Git repository selection | Applicable: v3 root via `reviewAuthorityRoot`/`OpenRepositoryIdentityLease` | Common-dir authority; lease revalidated around every mutation | Relative `--cwd`, linked worktree, symlinked common dir, foreign-repo refusal |
| Commit state | Applicable: post-apply/pre-commit new-lineage validation | Empty index / unborn HEAD ⇒ `unknown`, fail closed (`relateCandidates` order) | staged, `commit -a`, empty index, unborn HEAD |
| Push state | Applicable: pre-push/pre-pr boundary | Reuse `prePushTargetForRequest`/`prePRBoundaryForRequest`; `PrePRBoundaryEmptyRemoteBootstrap` ⇒ `unknown` | tracking branch, first push, explicit refspec |
| PR commands | N/A: no PR automation composed; gates read boundaries only | — | — |

## Migration / Rollout

No data migration; nothing is translated in place. Switch OFF ⇒ every new start takes the legacy path and no `v3/` entry exists. Already-created new lineages stay readable **and** finalizable (decision 5). Structural revert deletes `review_core*.go`/`authority_store.go` and the two facade branches; the promoted files return behind the shadow gate by re-adding the prefix.

## PR Slicing Preview (for sdd-tasks)

Chained on `feature/rdd-root-simplification` after Wave 2's exit evidence; ≤1000 authored lines/slice; deadcode ratchet per slice.

| Slice | Work unit | Forecast |
|---|---|---|
| S1 | Promotion rename, alias, guard split + tests | ~350 |
| S2 | v3 `AuthorityStore`: two artifacts, CAS, lock, replay identity, receipt + tests | ~800 |
| S3 | `ReviewCore` start/finalize/validate, consent/tier/budget reuse, start branch | ~900 |
| S4 | Governing-authority branch, Amendment C matrix, switch-off equivalence goldens | ~700 |
| S5 | Reason-taxonomy regression, unwired offer API, bench journeys | ~600 |

**Rejected**: S2+S3 combined — merges store mutation evidence with transition evidence in one review surface and exceeds the slice budget.

## Open Questions

- [x] **Decided (S5, task 6.3)**: admitted-finding path references for `provable_contraction` belong in `review-state.json`, not the receipt. Rationale: `review-state.json` is the CAS-mutable artifact (spec `rdd-authority-store`, "Two-Artifact Model") every in-flight `validate` call reads before a terminal state exists, while the receipt is issued exactly once at finalize and is immutable thereafter (spec, "Receipt Immutability After Issuance") — a contraction can be evaluated repeatedly across many `validate` calls before any receipt exists at all, so the field the gate needs at that time cannot live somewhere finalize alone populates. This mirrors `NewLineageAuthority.AdmittedFindingIDs` (`authority_store.go`, S2), which already persists admitted-finding *identifiers* in `review-state.json` for the identical reason; the literal path-reference variant this open question named is not implemented in S5 (no `validate` call site in this wave constructs `CoreValidateEvidence.AdmittedFindingPaths`/`AdmittedPathsKnown` from persisted authority yet — `governingAuthorityLiveEvidence`, S4/S5, always passes the zero value, degrading every contraction to `changed` per Amendment B's own no-input-degradation rule), so this decision fixes WHERE the field belongs without yet wiring it — recorded, not silently resolved by omission.
- [x] **Decided (S5, task 6.4)**: shipping `OfferReviewAfterVerify` (`internal/reviewtransaction/review_offer.go`) unwired, with one documented `.deadcode-baseline.txt` entry, is acceptable and preferred over deferring the API shape to Wave 4. Rationale: unchanged from design decision 8's own — a temporary Wave-4 call site would be Wave 4's sequence change happening a wave early, which is a worse coupling than an honest, reasoned baseline entry (`.deadcode-baseline.txt`, two lines: `OfferReviewAfterVerify`, `readGlobalRDDModeForOffer`) that the ratchet (`scripts/deadcode-ratchet.sh`) can see and any future change can audit. The one behavior this wave's shape is required to prove — the kill switch, when off, returns `Offer{Available:false}` before any repository read — is regression-tested (`review_offer_test.go`), so the shape is not merely aspirational prose.
