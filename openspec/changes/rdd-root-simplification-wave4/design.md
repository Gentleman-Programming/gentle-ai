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
| 3 | One call site: `internal/cli`'s SDD verify success exit — after verify-report admission, before archive eligibility. **Not** in `sddstatus.Resolve` (status stays a pure read). Decline = unmanaged proceed: nothing recorded, archive under ordinary policy, `reviewGate.delivery: disabled/unmanaged` — byte-identical to the kill-switch-off path. Targeted re-verify scope = correction changed paths ∩ verify evidence scope; empty intersection ⇒ re-run the objective's evidence goal; changed-path set not reliably derivable ⇒ **FULL re-verify** of the objective's evidence goal (a distinct branch from empty intersection, and from empty-index/unborn-HEAD ⇒ fail closed). Recorded as a new `RuntimeAttempt` with the existing `RemediatesEvidenceRevision` field | Offer inside status resolution; a persisted "declined" record | An offer inside `Resolve` makes RDD a supervisor again through the back door — every status read would run it. A declined record is a mirror: it turns a human "no" into lifecycle state, which is the defect class this wave closes. Reusing the consent decline semantics keeps one meaning of "no". Reconciled 2026-08-02: `rdd-post-verify-review-offer` and `rdd-review-core-transitions` spec deltas amended from "SDD status path" to this exact call site, using this same rationale, so spec and design agree |
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
- [ ] The OpenCode plugin's session-scoped admission-recovery budget (CON-09) is consumer-owned recovery state. Confirm the provider absorbs it in this wave rather than Wave 5, since removing it without a provider-side replacement loses a real relaunch bound.
- [ ] `internal/assets/*/commands/sdd-apply.md` pins contract `gentle-ai.review-integration/v1` while the orchestrator contract is `/v2`. Confirm correcting the pin belongs in S7 rather than as an independent defect fix.
