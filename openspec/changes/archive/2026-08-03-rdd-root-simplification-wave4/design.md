# Design: RDD Root Simplification — Wave 4 (Thin Consumers)

## Technical Approach

Invert the dependency. SDD stops holding review truth and stops being supervised by it: it keeps one terminal pointer (`ReceiptRef`), asks the provider once for validity, and calls RDD at exactly one point — after verify. Every re-derivation in `internal/sddstatus` (gate meaning, binding mirror, pre-verify routing) is deleted rather than synchronised. The kill switch off is proven by absence of calls, not by a green disabled path. Transport capability becomes an admission precondition of candidate freeze, so a runtime limitation can never become lifecycle state.

## Wave-0 Prerequisite: Adapter Behavioral-Depth Trace (CON-09/10/11)

**Method** (reproducible, not a one-time read): per adapter row — (1) `codegraph_explore` the adapter package for its blast radius; (2) grep the adapter's Go package **and its bundled asset tree** for the four forbidden constructions the `ReviewAdapter` boundary names: CLI flag literals, revision/`expected-revision` values, target/lineage identities, binding JSON; (3) record the verdict in `docs/architecture/rdd-ownership-inventory.md` — amend the CON-09/10/11 evidence columns and delete the "behavioral depth" gap bullet (line 87), which is the file that carries the gap. The grep set ships as a guard test so the trace cannot silently rot.

**Trace already performed by this design** (tasks start from evidence, not zero):

| Row | Go adapter | Real dispatch surface | Verdict |
|---|---|---|---|
| CON-09 OpenCode | `internal/agents/opencode/adapter.go` — zero review references | `internal/assets/opencode/plugins/review-result-artifacts.ts` | **Violates**: declares its own `ReviewBinding` type, composes `admissionRecoveryKey` from lineage/target/revision/context/lens/order/subject_hash, and holds a session-scoped recovery budget (`claimAdmissionRecovery`, `MAX_ADMISSION_RECOVERIES_PER_SESSION`). Consumer-side recovery state — in scope. |
| CON-10 Pi | `internal/agents/pi/adapter.go` — zero review references | gentle-pi host repo | **Out of repo**; in-repo surface is clean. Gated by its declared capability only. |
| CON-11 Claude | `internal/agents/claude/adapter.go` — zero review references | `internal/assets/claude/commands/sdd-apply.md:51` | **Violates**: hardcodes contract `gentle-ai.review-integration/v1` and keys routing off `nextRecommended: review` — the pre-verify control this wave removes. |

Evidence: `rg "review|Review" internal/agents` returns exactly one hit, in `capabilitymanifest/manifest.go` (unrelated). The Go adapters are already thin; the payload they install is not.

*Rejected*: line-by-line manual read of each adapter — not re-runnable, and it produces prose instead of a guard.

## Architecture Decisions

| # | Choice | Rejected | Rationale |
|---|---|---|---|
| 1 | `ReceiptRef{Lineage, ReceiptHash}` — exactly two fields — persisted in the SDD runtime ledger as `RuntimeStatus.Receipt *ReceiptRef`, appended by a new `runtimeReceiptEvent` (operation `receipt`), replacing `BindingRevision`/`Binding *ReviewBinding`. Validity is one call: `reviewtransaction.ValidateReceiptRef(ctx, repo, ref)` | A new OpenSpec artifact file (`changes/<c>/reviews/receipt-ref.json`); keeping `AuthorityRevision`/`GateContext` on the ref | Two fields answer "what do I ask about" and "which bytes did I see"; any third field is a value SDD would have to keep in sync — the mirror. An OpenSpec file is a second store *and* human-editable, so it is forgeable. `GateContext` on the ref **is** the re-derivation (CON-06) |
| 2 | Delete the writer half: `BindApprovedReview`, `prepareApproved*Binding`, `bindingBytes`/`bindingDigest`/`bindingPath`, `bindingExists`, `validateBoundReview`, `verifyBindingLedger`, `runtimeSelfSuccessorAvailable`, `RuntimeStrandedSuccessor`. Delegate the reader half: `resolveReviewAuthority` + `resolveCompactRemediationAuthority` collapse into one provider call whose `result`+`reason` `sddstatus` stores verbatim. Migration: `parseBinding` survives as read-only `parseLegacyBinding`; an existing `gentle-ai.sdd-review-binding/v1` file projects **in memory** to a `ReceiptRef` and is never rewritten and never deleted by gentle-ai | A one-shot migration that writes the ref then unlinks `binding.json` | Read-only compat matches the freeze policy and keeps rollback available; deletion is Wave 7's destructive step, and doing it here removes the rollback path inside the wave that needs it most |
| 3 | **SUPERSEDED, see the two amendments below** ("decision 3 call site" and "corrective verify cycle 4: BLOCKER-1"). Original text, kept for the decision's own history: One call site: `internal/cli`'s SDD verify success exit — after verify-report admission, before archive eligibility. **Not** in `sddstatus.Resolve` (status stays a pure read). Decline = unmanaged proceed: nothing recorded, archive under ordinary policy, `reviewGate.delivery: disabled/unmanaged` — byte-identical to the kill-switch-off path. **Both claims are now wrong**: the call site moved INTO `sddstatus.Resolve()` (decision-3 amendment), and decline is realized as `reviewGate` structural absence, never a populated `delivery: disabled/unmanaged` value (BLOCKER-1 amendment) — emitting that literal string would be the same ceremony CRITICAL-1 already removed from the kill-switch-off path. Targeted re-verify scope = correction changed paths ∩ verify evidence scope; empty intersection ⇒ re-run the objective's evidence goal; changed-path set not reliably derivable ⇒ **FULL re-verify** of the objective's evidence goal (a distinct branch from empty intersection, and from empty-index/unborn-HEAD ⇒ fail closed). "Recorded as a new `RuntimeAttempt`..." — also superseded, see the "re-verify archive-gating deferred to Wave 5" amendment | Offer inside status resolution; a persisted "declined" record | An offer inside `Resolve` makes RDD a supervisor again through the back door — every status read would run it. A declined record is a mirror: it turns a human "no" into lifecycle state, which is the defect class this wave closes. Reusing the consent decline semantics keeps one meaning of "no". Reconciled 2026-08-02: `rdd-post-verify-review-offer` and `rdd-review-core-transitions` spec deltas amended from "SDD status path" to this exact call site, using this same rationale, so spec and design agree |
| 4 | Primary proof = AST/call-graph guard asserting **zero** call edges into offer/`ReviewCore` symbols from any SDD apply/verify/archive path, across **both** `internal/sddstatus` (its one door: `var reviewEntryHook = func() {}` in `review_door.go`, precedent `finalGateAuthorizationHook`/`artifactPreimagesReadHook` in `gate.go`) **and** `internal/cli` (a new, explicit door: `var offerEntryHook = func() {}` in `review_offer_door.go`, scoped to the verify-success-exit path only — the other ~30 non-test `internal/cli` files that import `reviewtransaction` for explicit `review` subcommands are out of this guard's scope, since they are user-invoked, not automatic apply/verify/archive paths). Corroborating = call-absence counter over an OFF-mode bench journey apply→verify→archive, asserting zero on the executed path | Bench counter as primary proof; AST guard scoped to `internal/sddstatus` alone | Static primacy is the spec's own requirement: "a passing unit test of the disabled branch alone is explicitly NOT acceptable evidence" (`rdd-post-verify-review-offer` spec). A counter only proves behavior on the one executed path; a call-graph guard proves absence across every path. Since decision 3 moved the call site to `internal/cli`, the guard must cover both packages' doors — `internal/sddstatus` alone no longer bounds the full reachable surface |
| 5 | Extend `internal/agents/capabilitymanifest`: add `ContractReviewTransportV1` to `ContractClaims`, reusing the existing `dormant\|advertised` exposure vocabulary, canonical registry, `Validate()`, and digest. Adapter declares; provider never probes. Checked in `review start` **before** risk/tier/lens/budget/consent/freeze. Absent or unrecognised ⇒ fail closed using the plugin's existing `unsupported-capability` outcome, with zero authority artifacts created | A new capability schema/file; provider-side probing of the host runtime | The manifest already is the canonical provider-neutral capability surface with validation and drift detection — a second one splits ownership again. Probing a host runtime is CON-12, explicitly out of repo. This is not hypothetical: Pi declares only `AutoInstall\|SystemPrompt\|MCP` today (no `FileSubAgents`, no `Skills`), so its lens transport is genuinely unavailable |
| 6 | Decision 9 — **MAINTAINER-CONFIRMED (2026-08-02, ratified)**: attempts stay in SDD. One owner = `RuntimeObjective` (`runtime_ledger.go`) owns work-unit scope. Concretely, `CompactAcquireRequest`'s `WorkUnit`/`EvidenceGoal`/`MaxAttempts`/`MaxChangedLines` stop being a parallel struct and become the objective-owned `BeginAttemptRequest` fields structurally (`runtime_compact.go` already funnels through `normalizeBeginAttemptRequest`); the advance-vs-reset distinction (#2133/#2151) is defined once at the objective; `runtime_compact.go` becomes projection only. `docs/architecture/rdd-ownership-inventory.md:93`'s conditional CON-08 owner row is scheduled for rewrite in slice S2 (see PR Slicing Preview) | Move attempts to `AuthorityStore` | Wave 0's condition ("only if SDD's own store gains durable, cumulative CAS-like properties") is already met: `previous_revision` chaining, CAS `expected_revision`, `request_digest` replay identity. Moving would recreate the mirror pointing the other way. Two request structs over one concept is exactly how #2133/#2151 drifted. No longer tagged pending-confirmation — ratified per maintainer decision |

## Data Flow

    apply ──→ verify ──→ [offer point: internal/cli verify success exit] ──→ archive
                              │
       kill switch OFF ───────┤ (checked FIRST, before any repo read)
                              │        └─→ Offer{Available:false} — reviewEntryHook never fires
                              ├─ decline ─→ unmanaged proceed (no record) ─→ archive
                              └─ accept  ─→ review start
                                              └─ capability admission (BEFORE risk/tier/lens/budget/consent/freeze)
                                                   ├─ unavailable ─→ deny, zero artifacts
                                                   └─ advertised  ─→ freeze → … → receipt
                                                        └─→ correction ─→ targeted re-verify (correction ∩ evidence paths)
                                                             └─→ ReceiptRef appended to SDD runtime ledger ─→ archive

    validity:  sddstatus ──one call──→ reviewtransaction.ValidateReceiptRef ──→ {result, reason}
               (sddstatus derives nothing; RDD never calls SDD)

## File Changes

| File | Action | Description |
|---|---|---|
| `internal/sddstatus/review_binding.go` | Delete | Writer half removed; `parseBinding`→`parseLegacyBinding` moves to a read-only projection file |
| `internal/sddstatus/legacy_binding_read.go` | Create | Read-only v1 parse → in-memory `ReceiptRef`; never writes, never unlinks |
| `internal/sddstatus/review_gate.go` | Modify | `applyReviewGateEvaluation` keeps only the provider verdict; the disabled/unmanaged branch becomes structurally unreachable when the switch is off (no call happens at all) |
| `internal/sddstatus/status.go` | Modify | Remove `applyPreVerifyReviewRouting` (:829) and `applyPreVerifyCompactBridgeRouting` (:857) and all four call sites (:487, :490, :770, :772) |
| `internal/sddstatus/runtime_ledger.go`, `runtime_compact.go` | Modify | `ReceiptRef` record + work-unit single owner (decision 6) |
| `internal/sddstatus/review_door.go` | Create | The one `reviewtransaction` door + `reviewEntryHook` |
| `internal/reviewtransaction/review_offer.go` | Modify | Wired; `.deadcode-baseline.txt` entry removed |
| `internal/reviewtransaction/receipt_ref.go` | Create | `ReceiptRef` + `ValidateReceiptRef` |
| `internal/agents/capabilitymanifest/manifest.go` | Modify | `ContractReviewTransportV1` claim |
| `internal/cli/review_facade.go` | Modify | Capability admission before freeze; offer call site |
| `internal/cli/review_offer_door.go` | Create | The one `internal/cli` door for offer/`ReviewCore` symbols on the verify-success-exit path (`offerEntryHook`); the ~30 other non-test `internal/cli` files importing `reviewtransaction` for explicit `review` subcommands stay out of this guard's scope |
| `internal/assets/claude/commands/sdd-apply.md`, `opencode/commands/sdd-apply.md`, `skills/_shared/review-ledger-contract.md`, `testdata/golden/*` | Modify | Post-apply routing text → post-verify offer; contract pin corrected |
| `internal/assets/opencode/plugins/review-result-artifacts.ts` | Modify | Consume opaque transitions only; recovery keying/budget returns to the provider |
| `docs/architecture/rdd-ownership-inventory.md` | Modify | CON-08 owner named; CON-09/10/11 traced; gap bullet deleted |

## Interfaces / Contracts

```go
type ReceiptRef struct { Lineage string `json:"lineage"`; ReceiptHash string `json:"receipt_hash"` }
func ValidateReceiptRef(ctx context.Context, repo string, ref ReceiptRef) (GateResult, string, error)
func OfferReviewAfterVerify(ctx context.Context, repo string, request OfferRequest) (Offer, error) // Wave 3 shape, now wired
const ContractReviewTransportV1 ContractID = "gentle-ai.review-transport/v1"
```

## Testing Strategy

| Layer | What to Test | Approach |
|---|---|---|
| Unit | `ReceiptRef` two-field shape and rejection of extra fields; legacy binding projects read-only and is never rewritten; work-unit owner uniqueness | Table-driven; `t.TempDir()` real ledgers; golden byte comparison of the untouched `binding.json` |
| Guard | Zero call edges into offer/`ReviewCore` symbols from any SDD apply/verify/archive path, across both `internal/sddstatus` (one door: `review_door.go`) and `internal/cli` (one door: `review_offer_door.go`); adapter forbidden-construction grep set | AST/call-graph guard modelled on `shadow_readonly_guard_test.go` — **primary** evidence |
| Absence | Kill switch OFF ⇒ `reviewEntryHook` and `offerEntryHook` fire zero times across apply→verify→archive | Counter hook + OFF bench journey — **corroborating** evidence only |
| Integration | Offer accept / decline / correction → targeted re-verify → archive; decline output byte-identical to OFF | `t.TempDir()` repos; golden status JSON |
| Capability | Unsupported transport denies before any authority/tier/lens/budget/collection artifact exists | Store inspection asserting an empty authority root |
| Bench | Black-box journeys for accept, decline, OFF, unsupported transport | `bench/` journeys |

## Threat Matrix

| Boundary | Applicability | Design response | Planned RED tests |
|---|---|---|---|
| Documentation-like paths | N/A: classification reused unchanged from `risk.go`; no new file-execution boundary | — | — |
| Git repository selection | Applicable: `ReceiptRef` lives in the Git common-dir runtime store; the offer resolves a repo | Common-dir authority via the existing `SnapshotBuilder.ResolveRepositoryRoot`; OFF path returns before any repository read | Relative `--cwd`, linked worktree, symlinked common dir, foreign-repo refusal |
| Commit state | Applicable: targeted re-verify derives paths from the correction | Empty intersection ⇒ objective re-run; unprovable/non-derivable path scoping ⇒ full re-verify of the objective's evidence goal (never silently skipped, never conflated with empty intersection); empty index / unborn HEAD ⇒ fail closed — three distinct branches | staged, `commit -a`, empty index, unborn HEAD, non-derivable path scoping |
| Push state | N/A: the five delivery gates are unchanged in this wave (cutover is Wave 5) | — | — |
| PR commands | N/A: no PR automation composed; adapters execute provider-issued transitions only | — | — |
| Process integration (adapters) | Applicable: adapters/plugins dispatch review operations | Capability declared, never probed; unavailable ⇒ typed `unsupported-capability`, no self-constructed flag, revision, target, or binding | Absent claim, unrecognised claim, advertised-but-failing transport |

## Migration / Rollout

No data migration. Existing `gentle-ai.sdd-review-binding/v1` files stay on disk, parse read-only, project to a `ReceiptRef` in memory, and are never rewritten — Wave 7 deletes them. Rollback is per-slice and non-destructive: revert the offer call site and restore the pre-verify routing (slice 3); an adapter without the capability claim degrades to unavailable mode, never to a self-constructed transition. No authority is rewritten and no receipt is invalidated.

## PR Slicing Preview (for sdd-tasks)

Chained on the Wave 3 branch (`auto-chain`, feature-branch chain); ≤1000 authored lines/slice; deadcode ratchet checked per slice. Cross-slice fixes ride **slice 1** (session lesson).

| Slice | Work unit | Forecast |
|---|---|---|
| S1 | Adapter behavioral-depth trace, inventory amendment, forbidden-construction guard test | ~300 |
| S2 | Decision 9 (ratified): one work-unit owner across `runtime_ledger.go`/`runtime_compact.go` + tests; rewrite CON-08 owner row at `docs/architecture/rdd-ownership-inventory.md:93` to name `RuntimeObjective` unconditionally (drop the conditional "only if" wording) | ~350 |
| S3 | Pre-verify routing removal, post-verify offer call site, decline semantics, OFF call-absence proof (offer wired **first**, per spec's intra-wave sequencing) | ~850 |
| S4 | Transport capability claim, pre-freeze admission, per-adapter unavailable mode (**second**) | ~650 |
| S5 | `ReceiptRef` record, legacy read-only projection, single native validation entry point | ~900 |
| S6 | Targeted re-verify from correction paths | ~500 |
| S7 | Mirror writer deletion, adapter/asset/golden updates (**lands last**) | ~750 |

**Rejected**: capability (S4) before offer (S3) — violates the spec's mandated intra-wave order (offer first, capability second, mirror last); combining offer (S3) with `ReceiptRef` storage (S5) — mixes sequence evidence with storage-shape evidence in one review surface and exceeds the budget; S7 earlier (mirror deletion is the only destructive step and must land after the replacement is proven).

## Open Questions

- [ ] Wave 3 is **not yet on `main`** — `internal/reviewtransaction/review_offer.go`, `review_core.go`, and `authority_store.go` do not exist at `d591f4cf`. Wave 4 cannot start until Wave 3's branch lands; confirm the chain base before `sdd-tasks` forecasts.
- [x] The OpenCode plugin's session-scoped admission-recovery budget (CON-09) is consumer-owned recovery state. **Resolved (coordinator ruling, 2026-08-03, task 8.5)**: the provider-side replacement is DEFERRED, not built this wave — either to Wave 5's entry (the gates-cutover work owns the admission flow) or to Wave 7 (if the plugin's whole legacy consumption retires there instead). Recorded explicitly rather than silently dropped; see the Wave 7 deletion-inventory Engram note for the exact loss being deferred.
- [x] `internal/assets/*/commands/sdd-apply.md` pins contract `gentle-ai.review-integration/v1` while the orchestrator contract is `/v2`. Resolved: corrected in S1 (cross-slice fix, CI-exact-head rule), not S7.
- [ ] Whether `SDDReceiptRef`/the compact receipt schema should carry correction-path data, so targeted re-verify's branch 7.2 ("not reliably derivable") stops being the general case, is left open for Wave 5 or Wave 7 — explicitly out of scope for this amendment (see "Amendment (coordinator-resolved): targeted re-verify call site").

## Amendment (orchestrator-resolved): decision 3 call site (2026-08-03)

S3's `sdd-apply` batch found decision 3's originally-named call site —
`internal/cli`'s SDD verify success exit, concretely `RunSDDVerifyValidate`
(`sdd_verify_validate.go`) — genuinely underspecified: that command is
deliberately repo/context-free by its own doc comment ("validates a
complete report without touching an artifact store") and carries no
`--cwd`/`--change`/`--lineage` `OfferReviewAfterVerify` needs. Escalated as
a blocking design question rather than resolved by inventing an unreviewed
CLI flag contract. The orchestrator resolved it within the wave's ratified
directive (post-verify offer; structural absence when the kill switch is
off):

**The offer call site is NOT `RunSDDVerifyValidate`; it stays deliberately
repo-context-free — no flags were added to it.** The offer instead routes
through the native SDD status/dispatcher surface, which already carries
repo and change context on every call:

1. `internal/sddstatus`'s `Resolve()` (and `resolveEngramStatus()`
   symmetrically) emits a `reviewOffer` block in `Status` in the exact
   post-verify-passed, not-yet-archived window (`Dependencies.Verify ==
   DependencyAllDone`) — if and only if the kill switch is on — resolved
   through the same door (`review_door.go`'s `reviewEntryHook`, now wired to
   a real call, `reviewOfferForVerify`) S3 already added. The block carries
   what an orchestrator needs to present the offer: whether
   `OfferReviewAfterVerify`'s own composed decision is available yet
   (conservatively `false` until S4/S5 compose the receipt/lens/tier
   evidence it needs — matching `review_offer.go`'s own documented Wave 3
   shape), the lineage identifier, and the exact `gentle-ai review start`
   invocation to run. This is `OfferReviewAfterVerify`'s first real caller.
2. Switch off is structural absence at BOTH the Go-value level (`Status.
   ReviewOffer` stays `nil`) and the serialized-output level (`omitempty`
   keeps the `reviewOffer` key off the wire entirely) — proven by a JSON
   marshal assertion, not only the AST guard. `applyReviewOfferRouting`
   returns before calling `reviewOfferForVerify`/`reviewEntryHook` at all
   when the switch is off, so the door's own call counter proves zero calls
   across a full simulated apply → verify → archive-pending flow.
3. Decline (task 4.5) is consistent with the consent contract: scoped to
   one candidate, persists no decision, and does not suppress the offer for
   a later read of the same still-unarchived candidate. Archive proceeds
   under whatever the pre-existing (unrelated to this amendment) post-verify
   archive gate already decides — the offer never blocks it and never
   changes its verdict. No new state, no new persistence: a decline is
   simply the orchestrator not acting on a block that keeps reappearing.
4. The OFF-mode absence counter (task 4.7) is realized as an in-process Go
   test rather than a `bench/` black-box journey: `reviewEntryHook` is a
   Go-internal instrumentation point a separate bench subprocess cannot
   observe. Same zero-cost-by-default proof shape as Wave 1's
   `shadowObserverCallCountForTest`, applied to this door.
5. Asset prose is in scope as an addition to the File Changes table above:
   `internal/assets/{claude,opencode}/commands/sdd-verify.md`'s pre-verify
   gate language ("Continue only when the native bounded transaction is
   `ready_final_verification` or `final_verifying`") is deleted and replaced
   with post-verify offer prose keyed off the `reviewOffer` block's
   presence/absence in native status output.

This amendment supersedes decision 3's "one call site: `internal/cli`'s SDD
verify success exit" line; the call site is `internal/sddstatus`'s
`Resolve()`/`resolveEngramStatus()` instead, `sddstatus.Resolve` is no
longer "a pure read" in the narrow sense decision 3 originally stated (it
now makes one conditional call through its own door), and `internal/cli`'s
`review_offer_door.go`/`offerEntryHook` (created in S3's first increment)
remains a dormant, correctly-guarded boundary — not this wave's active call
site, but still valid coverage for the `internal/cli` SDD surface's
continued absence of direct offer/`ReviewCore` references.

## Amendment (coordinator-resolved): decision 1 scope — Binding/BindingRevision retirement moves to Wave 7 (2026-08-03)

S5c's `sdd-apply` batch traced every non-test caller of
`RuntimeStatus.Binding`/`BindingRevision` (not just the symbol names decision
1 and decision 2 already list) and found the literal "replace
`BindingRevision`/`Binding *ReviewBinding`" line in decision 1 is not
compile-safe as an atomic field removal, for two reasons neither decision 1
nor decision 2 accounted for:

1. `internal/cli/review_facade.go` calls `sddstatus.BindApprovedReview` as the
   live `gentle-ai review bind-sdd` command — a production surface, not
   scheduled for retirement by any Wave 4 task — which reads
   `RuntimeStatus.Binding` through `bindPreparedReview`'s return value.
2. `runtime_ledger.go`'s `Finish()` remediation-successor CAS and
   `runtime_compact.go`'s `Settle()` form one coupled subsystem that both
   reads `RuntimeStatus.Binding` (`validateRuntimeBoundCandidate`,
   `runtimeSelfSuccessorAvailable`, `runtimeStrandedSuccessor`) **and**
   writes a new one in the same compare-and-swap
   (`ExpectedBindingRevision`/`SuccessorLineageID` as CAS inputs to
   `applyRuntimeBindingEvent`). `SDDReceiptRef`'s deliberately two-field
   shape (decision 1) has no analogue for "the successor lineage a
   remediation attempt just bound" — inventing one here would be a
   unilateral design decision, not an implementation detail.

**Resolution**: receipt-based evaluation governs the archive-gate path in
Wave 4. `RuntimeStatus.Binding`/`BindingRevision` remain the carrier for (a)
the live `review bind-sdd` command and (b) the remediation-successor CAS
subsystem (`runtime_ledger.go`'s `Finish()` + `runtime_compact.go`'s
`Settle()`), whose `ExpectedBindingRevision`/`SuccessorLineageID` inputs have
no `SDDReceiptRef` analogue by design. Their retirement belongs to Wave 7's
consumer-first legacy deletion (its proposal's D4 already classifies
ambiguous-vintage verbs, `bind-sdd` among them); Wave 4 records them in the
Wave 7 deletion inventory instead of removing them.

Concretely, this re-scopes decision 1/2/task 6.6-6.7/task 8.1 for Wave 4:

- **6.6/6.7 (S5c', this batch)**: reroute only the archive-gate evaluation
  path — `status.go`'s two `bindingPresent` branches plus
  `applyReviewGateEvaluation` — onto `RuntimeStatus.Receipt`/
  `ValidateSDDReceiptRef` (re-derivation, not a stored `GateContext`
  comparison). `RuntimeStatus.Binding`/`BindingRevision`, `BindApprovedReview`
  /`bind-sdd`, and the remediation-successor CAS subsystem stay untouched and
  keep their existing tests.
- **8.1 (S7)**: the deadcode ratchet is the arbiter — delete only the
  `review_binding.go`/`runtime_ledger.go` entries S5c' actually orphans (no
  caller left anywhere in the archive-gate path); anything `bindPreparedReview`
  /`BindApprovedReview`/the remediation CAS still needs is kept and recorded,
  one line each, in the Wave 7 deletion inventory
  (Engram topic `sdd/rdd-root-simplification-wave7/deletion-inventory-w4-contributions`)
  instead of deleted here.

## Amendment (coordinator-resolved): targeted re-verify call site (2026-08-03)

S6's `sdd-apply` batch found tasks 7.1-7.4's call site genuinely
underspecified in the same way decision 3's originally-named call site was:
design.md's own "correction changed paths ∩ verify evidence scope" language
names no owner, and no existing code bridges a review's own bounded
correction to SDD's runtime-attempt tracking. Escalated rather than invented.
The coordinator resolved it consistently with the decision-3 amendment's own
precedent:

**Targeted re-verify is a routing decision owned by `internal/sddstatus`'s
`Resolve()` (and `resolveEngramStatus()` symmetrically) — the routing
surface owns integration, the orchestrator consumes it,
`RunSDDVerifyValidate` stays context-free**, exactly mirroring decision 3's
"the offer routes through the native SDD status/dispatcher surface instead"
resolution.

1. When the change's governing receipt (S5c''s `resolveGoverningReceiptRef`
   path) records an applied correction, `Resolve()` computes
   correction-changed-paths against the runtime objective's verify evidence
   scope and emits a re-verify routing block —
   `Status.ReVerify *ReVerifyBlock{Mode: "targeted"|"full", Scope []string,
   Reason string}` — following the same `omitempty`/structural-absence
   pattern `Status.ReviewOffer` already established (S3b).
2. Branch semantics exactly per tasks 7.1-7.3, the three distinct cases the
   Threat Matrix's "Commit state" row already names and Wave 4's own design
   text forbids conflating:
   - Empty intersection (correction paths ∩ verify evidence scope is empty)
     ⇒ `Mode: "targeted"`, re-run only the objective's evidence goal.
   - Changed-path set not reliably derivable — including a governing receipt
     that carries no correction-path data at all, which is the common case
     until a future wave extends the receipt schema (see Open Questions
     below) ⇒ `Mode: "full"`, re-verify the objective's entire evidence goal.
     Never conflated with the empty-intersection branch: the reason string
     names which of the two applies.
   - Empty index / unborn HEAD (the commit state itself cannot be read) ⇒
     fail closed — no routing block emitted, the pre-existing native runtime
     error routing governs instead (mirrors `applyNativeRuntimeErrorRouting`'s
     existing fail-closed shape for corrupt/unreadable authority).
3. No correction recorded ⇒ no block emitted at all — structural absence,
   the same guard pattern the offer block (`Status.ReviewOffer`) already
   proved (Go-value nil, `omitempty` keeps the key off the wire).
4. The orchestrator-facing prose (task 8.6) instructs running `sdd-verify`
   with the block's stated scope before archive, mirroring how the offer
   block's `Invocation` field already names the exact follow-up command.

**Explicitly out of scope for this amendment**: extending `SDDReceiptRef` or
the compact receipt schema to carry correction-path data. If the governing
receipt genuinely cannot carry correction paths today — which S6's own
investigation expects to be the general case — that is exactly what branch
7.2 ("not reliably derivable") is for, not a reason to widen the receipt
shape mid-wave. Carrying correction-path data on the receipt (or elsewhere)
is noted here as an open question for Wave 5 or Wave 7 to pick up, not
resolved now.

This amendment supersedes the Threat Matrix's "Commit state" row only to the
extent of naming the owning package (`internal/sddstatus`, not a bare
"targeted re-verify" with no home); the three-branch semantics themselves
are unchanged from the original design text.

## Amendment (corrective verify cycle 3): re-verify archive-gating deferred to Wave 5 (2026-08-03)

A second corrective cycle (commit `03c07581`) implemented task 7.4's
remaining sub-clause — "record a new `RuntimeAttempt` using the existing
`RemediatesEvidenceRevision` field" and "archive does not proceed until that
re-verify passes" — by adding `ReVerifyBlock.EvidenceRevision` (stamped from
the live verify-report's evidence revision on every `Resolve()`) and
`blockArchiveForUnsatisfiedReVerify` (blocking `Dependencies.Archive` until a
`RuntimeAttempt` named that exact revision).

**Found defective (cycle 3, CRITICAL-A) and reverted:**

1. **Livelock.** `CompactState.CorrectionAttempts` is append-only, but the
   demanded `EvidenceRevision` was re-derived from the CURRENT verify-report
   on every status read, not frozen at correction-record time. A compliant
   operator who re-verified and recorded a passing attempt naming the
   demanded revision would see the demand simply re-label itself to the
   verify-report's new revision on the next status read — the gate could
   never be satisfied by doing the thing it demanded. Proven with a
   throwaway in-package probe (created, run, deleted; worktree restored
   byte-identically): cycle 1 blocked naming `sha256:R1`; after a compliant
   re-verify recording `--remediates-evidence-revision sha256:R1`, cycle 2
   blocked again naming `sha256:R2`.
2. **Unrunnable continuation.** The blocked reason named
   `gentle-ai sdd-attempt finish --remediates-evidence-revision <rev>`, but
   `sdd-attempt finish` refuses that flag unless `--expected-binding-revision`
   and `--successor-lineage` are ALSO given, together
   (`internal/cli/sdd_attempt.go:94-96`) — and `--successor-lineage` requires
   an approved compact review successor, i.e. a full review round trip. The
   cycle-2 apply-progress claim "no new writer needed — the existing
   `sdd-attempt finish --remediates-evidence-revision <rev>` already exists"
   was incorrect as stated: that exact invocation does not exist standalone.

**Resolution**: `blockArchiveForUnsatisfiedReVerify` and its call sites were
removed. `internal/sddstatus/review_reverify.go` is restored to S6's
original shape — `applyTargetedReVerifyRouting` is purely additive again,
only ever setting `Status.ReVerify`, never `Dependencies` or
`NextRecommended`. `ReVerifyBlock.EvidenceRevision` was removed with it (it
existed only to feed the now-removed gate).

**What a compliant Wave 5 (or later) implementation needs**, recorded here
so the next attempt does not repeat the same defects:

1. **A frozen demand anchor.** The demanded revision must be derived from
   the correction's own append-only data at record time — e.g. the
   correction attempt's `FixDeltaHash` (already computed, already
   content-addressed and immutable once recorded) — never from the live
   verify-report, which changes precisely when the demand is satisfied.
2. **A genuinely runnable satisfaction path.** Either extend
   `sdd-attempt finish`/`internal/sddstatus/runtime_ledger.go`'s `Finish()`
   validation to accept `--remediates-evidence-revision` decoupled from the
   binding/successor-lineage triple (a new, narrower write shape, with its
   own CAS-safety review — `runtime_ledger.go`'s `Finish()` is a
   security-sensitive path and any new shape needs the same rigor the
   existing one has), or make an explicit, ratified product decision that a
   targeted re-verify legitimately requires a full review round trip (in
   which case the spec's "cheap targeted re-verify" framing itself needs
   revisiting, not just the implementation).
3. Both of the above are materially larger than a single corrective-cycle
   slice; this is why they are deferred rather than rushed, per the
   coordinator's own standing guidance that a deferred-but-honest spec beats
   a livelocking gate.
