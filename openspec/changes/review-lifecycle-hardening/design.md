# Design: Review Lifecycle Hardening

## Technical Approach

Eighteen defects, seven mechanism groups, no new subsystem. Every fix is a typed contract at an existing seam: either an untyped error becomes a sentinel, a silent no-op becomes a typed refusal or a real transition, or an over-broad guard is narrowed and its real safety check moved to the layer that owns it. Delivery order `E → A → B → G → F → C → D`, one independently revertible commit per group, RED-first per issue.

**Two findings change the proposal's plan.** They are stated first because they are load-bearing.

### Finding 1 (BLOCKING) — `validateCompactRecoveryEdge` does NOT reject every incompatible predecessor kind

The proposal assumes relaxing `internal/cli/review_facade.go:669-671` is safe because the deeper check catches what the CLI gate catches. It does not. Evidence from `internal/reviewtransaction/compact_store.go:404-483`:

| Disposition branch | Target-kind constraint present? | Evidence |
|---|---|---|
| `RecoveryInvalidated` (`:453-456`) | **None** | asserts only `predecessor.State.State == StateInvalidated` |
| `RecoveryEscalated` (`:457-476`) | **None** | asserts escalated predecessor + `compactEscalatedRecoveryTargetChanged` (`:485-487`, trees/identity only) + authorization binding |
| `RecoveryScopeChanged` / `StateCorrectionRequired` (`:442-449`) | **None** | genesis-path expansion or contraction only |
| `RecoveryScopeChanged` / `StateApproved` (`:424-441`) | Partial, and bypassable | `compactRecoveryScopeChanged` (`:356-359`) passes `ExplicitScopeChange: true` into `classifyCompactTargetRelation`, whose kind gate at `compact_target_relation.go:52-55` is skipped whenever `substantiveScopeChange` holds |

`compactStartTargetKindsCompatible` (`compact_store.go:1013-1019`) is **never called** from `validateCompactRecoveryEdge`; it serves START equivalence (`:1004`) and the relation classifier only. Projection is retained (`:417-420`); Kind is not.

**Therefore the CLI gate is today the only thing blocking a kind-incompatible recovery of that class, and the design must add the missing check rather than keep the blanket rejection.** See Group C.

### Finding 2 (SPEC CORRECTION) — 1782 is a pre-PR gate defect, not a recovery-edge defect

The parallel spec phase filed 1782 under a new "Compact recovery edge admission" requirement, with the trigger written as "WHEN recovery evaluates whether a chain convergence exists". **That is wrong about both the trigger and the mechanism.** Decided from the code, not from the header name:

- The convergence predicate lives in `selectCompactPrePRChain` (`internal/reviewtransaction/compact_chain.go:383`), at `:452-458`. Call path: `EvaluateCompactPrePRChain` (`compact_chain.go:82`) / `deriveCompactPrePRChain` (`:114`) → `selectCompactPrePRChain` → the incoming-degree loop. Every entry is a **gate** evaluation of a pre-PR range.
- No recovery operation reaches it. `validateCompactRecoveryEdge` appears in `compact_chain.go` exactly once, at `:371` inside `compactDegenerateRecoveryMember`, where it *classifies an already-persisted chain member*. Data flows recovery-validation → chain-membership, never chain-selection → recovery-admission.
- The reproduction confirms the gate trigger: receipts X→A, A→B, B→C with the tracker branch at A; `review validate --gate pre-pr --base-ref origin/tracker` refuses. Selecting base A makes `path[0].receipt.BaseTree` the tree at A, and the historical X→A edge sets `incoming[treeA] == 1` against `wantIncoming == 0` (`:452-455`). Deleting the X→A lineage restores allow, which matches the predicate exactly.

**Requirement placement.** 1782 belongs to **`Deterministic lifecycle validation`** (`openspec/specs/review-findings-ledger/spec.md:143-145`) as a MODIFIED requirement — that is the pre-commit/pre-push/pre-PR/release gate contract, and the defective behavior is what a gate accepts as a valid receipt chain covering the selected base.

It does **not** belong to `Semantic chain completeness` (`spec.md:119-121`), despite the matching header word. That requirement governs *persisted successor* invariants within one lineage — allowed state pairs, monotonic counters, frozen-finding immutability, refuter batch consumption — i.e. `validateCompactState`. `selectCompactPrePRChain` validates no successor; it selects a path across receipts from different lineages. (`Semantic chain completeness` *is* the right home for Group D's `validateCompactState` change, 1813.)

1782 stays in the Group C commit for delivery, because it shares the recovery-graph fixture family and the same last-to-land risk order. Group membership is a delivery unit, not a requirement claim.

### Finding 3 — 1663 and 1788 share one fix site

Both land on `internal/cli/review_facade.go:1743-1760`. The `StateValidating` branch returns `plan, nil` with no transition when `plan.Evidence` is empty and the candidate is not a native low-risk one (`:1758-1759`) — a silent success. 1663 is the case where canonical captured evidence exists and is ignored; 1788 is the case where results were already consumed so the request is a genuine no-op. One contract, two named subtests.

## Pinned anchors (Duty 1)

| Issue | Exact anchor | Function | Mechanism |
|---|---|---|---|
| 1782 | `internal/reviewtransaction/compact_chain.go:452-458` | `selectCompactPrePRChain` (`:383`), reached from `EvaluateCompactPrePRChain` (`:82`) and `deriveCompactPrePRChain` (`:114`) — the **pre-PR gate** path, not recovery | `wantIncoming = 0` is demanded for the chain root `path[0].receipt.BaseTree`; a historical receipt whose `FinalCandidateTree` equals the publication base contributes `incoming[root] == 1` → `"compact receipt chain contains a convergence"` |
| 1663 | `internal/cli/review_facade.go:1758-1759` (primary), `:1490-1495` (evidence resolution) | facade finalize plan | `StateValidating` + empty evidence + not low-risk → silent `return plan, nil` |
| 1788 | `internal/cli/review_facade.go:1743-1760`; terminal guard at `:1394-1397` covers only Approved/Escalated | facade finalize plan | non-terminal lineage-only finalize after results were consumed falls through to the same silent no-op |
| 1800 | `internal/cli/review_next_transition.go:195-196` | `newReviewNextTransition` | `case StateEscalated: return reviewStopTransition("escalated_authority")` — a dead-end Stop, unlike `StateInvalidated` at `:166-167` which routes to `reviewRecoveryCollection` |

**Not pinned, deliberately:** 1771's selector-free (non-`--projection staged`) unborn-HEAD path. Exploration flagged it as unconfirmed and this pass did not confirm it. `internal/reviewtransaction/snapshot.go:702-799` (`resolveCurrentChangesBase`) is the file family; the exact non-staged branch must be pinned by the apply phase before its RED test. Do not invent a fix site.

## Group designs

### E — Limits (1778)

| | |
|---|---|
| Seam | `internal/cli/review_facade.go:255` (`reviewFacadeOperationTimeout = 25 * time.Second`) |
| Contract | Add `reviewFacadeStartOperationTimeout` (larger constant) plus a selector `reviewFacadeOperationDeadline(operation string) time.Duration` returning it for `review.start` only. The general 25 s var stays byte-identical and stays a `var`. |
| Why not a framework | `review_status_contract_test.go:848-861` and `review_final_verification_retry_test.go:261-264` mutate the general var directly; a config surface would break both and is out of scope. |
| Tests | Unit: selector table (start → large, every other operation → 25 s). E2E: `issue-1778`. |

### A — Error typing (published-delivery pre-push, 1699, 1666, 1807)

**Sentinel ladder audit** — every error currently reaching `authority_corrupted`, and its disposition:

| Producer | Path to terminal | Disposition |
|---|---|---|
| `review_facade.go:2049` `store.Load()` failure | direct `ReviewAuthorityCorrupted` | **stays corruption** |
| `:2056` receipt read failure | direct | **stays corruption** |
| `:2061` parse / derive / receipt inequality | direct | **stays corruption** |
| `:2092` `assessmentUnknown` ← `gate.go:1153` reviewed-delivery-base ambiguity | `:2161-2162` | **becomes typed** `*GateDeliveryBaseResolutionError`, routed into the existing `deliveryShape` bucket (`:2075-2090`) → existing terminal `ReviewReceiptScopeChanged` (`:2154-2159`). No new terminal code. Flows through `reviewReceiptDiscoveryIsUnmanagedWhileDisabled` (`:2771-2777`), which already admits `ReviewReceiptScopeChanged`. **This is the release blocker.** |
| `:2092` ← `gate.go:748` merge-base count | `:2161-2162` | **becomes typed**, same class, same `deliveryShape` route — the remedy is identical (`pass --base-ref <remote>/<branch>`) |
| `:2092` ← `gate.go:752` push remote not configured | `:2161-2162` | **typed sentinel, terminal unchanged.** An unconfigured remote is environment fault, not a receipt that stopped governing; reclassifying it to scope-changed would report `disabled/unmanaged` for a broken setup. Type it, surface the cause in `ReviewReceiptDiscoveryError.Detail`, keep `authority_corrupted`. |
| `:2161` `scopeWithoutContext` | direct | **stays corruption** |
| `:2168` partial `targetResolution` | direct | **stays corruption** (mixed outcomes are ambiguity) |

| Issue | Seam | Contract |
|---|---|---|
| 1699 | `internal/reviewtransaction/artifact_admission.go:216` | The predicate compares canonicalized `verifiedIDs` against the **raw** `request.CandidateCausalFindingIDs`. Split it: set mismatch (`!equalStrings(verifiedIDs, wantCandidateCausalIDs)`) keeps the existing `ArtifactAdmissionOutOfScope` message byte-identical; a canonicalization *error* becomes `ArtifactAdmissionIncomplete` naming the offending id; non-canonical order alone is accepted, since `admission.CandidateCausalFindingIDs = verifiedIDs` (`:220`) already persists the canonical form. |
| 1666 / 1807 | `internal/cli/review_operation_contract.go:228-235` | `Code`, `Message`, `MutationOutcome` stay byte-identical for the default envelope. Add (a) an additive `Cause string` JSON field populated from `runErr.Error()` whenever `runErr != nil`, and (b) typed branches ahead of the default for causes that provably did not mutate — `reviewPreflightError` → `phase: preflight`, `MutationOutcome: not_started`, `RetrySafe: true`. |

Tests: unit table on `discoverCompactFacadeGateReview` classification per producer; unit table on `newReviewIntegrationFailure`. E2E `issue-1801`, `issue-1699`, `issue-1666`, `issue-1807`.

### B — Executable contract (1745, 1775, 1663, 1788, 1800)

| Issue | Seam | Contract |
|---|---|---|
| 1745 | `internal/cli/review_next_transition.go:133`, `:157` | Two defects on one line: the emitted name is `captured_results` (underscore) while the flag is `--captured-results`, and the boolean is emitted as a detached value. Add `Token string` to `ReviewTransitionArgument`, carrying the exact literal argv token (`--captured-results=true`). Populate it on `Execute.Arguments` **only** — never on `Preconditions`, which are assertions, not argv. `Name`/`Value` stay byte-identical, so existing consumers do not move. |
| 1775 | `internal/cli/review_schema.go:12-17`, `:21` | Add the `gentle-ai.review-verification-evidence/v1` entry (key `verification-evidence`) and name it in the usage error. The schema shape must be derived from `readCapturedFinalEvidence` / `review capture-evidence`, not invented. |
| 1663 | `review_facade.go:1490-1495` + `:1758` | When `StateValidating` and no evidence was supplied and canonical captured evidence exists for the exact `(store.Dir, state, revision)`, consume it on the identical bytes path `--captured-evidence` uses. |
| 1788 | `review_facade.go:1758-1759` | Replace the silent `return plan, nil` with a typed `ErrReviewFinalizeNoTransition` naming the exact escape command verbatim: `gentle-ai review capture-evidence` then `gentle-ai review finalize --lineage <id> --captured-evidence`. |
| 1800 | `review_next_transition.go:195-196` | Route `StateEscalated` to `reviewRecoveryCollection` with disposition forced to `RecoveryEscalated` (the default at `:371-373` is `RecoveryInvalidated` and would be wrong). Because `validateCompactRecoveryEdge` requires `compactEscalatedRecoveryTargetChanged`, Stop with a new reason code naming the changed-target requirement when `status.TargetIdentity == reviewAuthorityTargetIdentity(status)` — regardless of whether a Selector is present, since the existing `recovery_scope_unchanged` guard (`:376-378`) fires only when one is. |

### G — Target shapes (1812, 1771, 1641)

| Issue | Seam | Contract |
|---|---|---|
| 1812 | `internal/cli/review_facade.go:938-940` (insert between the overlay guard and the dirty-tracked check at `:941`) | Explicit typed refusal for `--projection staged` + `--base-ref` without `--workspace-overlay`. Name the proven escape verbatim: `gentle-ai review start --base-ref <ref> --committed-only`. Do NOT implement index freezing. |
| 1771 | unpinned — see above | Route selector-free status on unborn HEAD through the same empty-tree projection the staged path already uses (`snapshot.go:798-799`). |
| 1641 | unborn-HEAD refusal family | Message names the escape verbatim: commit an authorized empty root, then run committed base-diff review. Do NOT implement empty-base publication. |

### F — Platform fail-safes (1781, 1804) — unverifiable on Linux

| Issue | Seam | Contract |
|---|---|---|
| 1781 | `internal/reviewtransaction/secure_open_unix.go:45-63` | Anchor the walk at the already-resolved Git common-dir fd instead of `unix.Open("/")`. Signature becomes `secureOpenLockParent(root, path string)`; open `root` with `secureDirectoryOpenFlags()` (no `O_NOFOLLOW` on the anchor, matching today's treatment of `/`), then walk only components relative to `root` with `O_NOFOLLOW`. **Fail-safe:** when `path` is not under `root`, fall back to today's root-anchored walk verbatim, so no working configuration changes. Separately, report a pre-lock failure as `not_started` at the caller. |
| 1804 | `internal/reviewtransaction/publish_immutable_darwin.go:11-13` | On `ENOTSUP`/`EINVAL`/`ENOSYS` from `RenameatxNp` only, fall back to exclusive create (`O_CREATE\|O_EXCL`) at the destination, copy the source bytes, fsync, remove the source. Exclusive create is the atomicity primitive available on every filesystem; the fallback is non-atomic-rename but never silently replaces. Every other errno propagates unchanged. |

**Unit coverage** requires injectable syscalls: extract package-level `var renameNoReplace = unix.RenameatxNp` and a root-resolution seam, then table-test the ENOTSUP branch, the non-ENOTSUP passthrough, the under-root walk, and the not-under-root fallback.

**The community must verify, because CI cannot:** (a) `review start`/`finalize` under a macOS managed configuration profile that restricts `/` traversal, and (b) the same flows with the Git common directory hosted on an ExFAT volume.

### C — Recovery graph (1744, 1816, 1782)

| Issue | Seam | Contract |
|---|---|---|
| 1744 | `internal/cli/review_facade.go:669-671` **and** `internal/reviewtransaction/compact_store.go:404` | **Split the CLI predicate.** Keep the flag-coherence clause `*committedOnly != (base != "")` — it is pure argv shape, not an authority boundary, and dropping it would build a `TargetBaseDiff` with an empty `BaseRef` at `:688-689`. Remove **only** the predecessor-kind coupling clause `baseDiff != *committedOnly`. **Then add the missing check** to `validateCompactRecoveryEdge`, evaluated for every disposition before the switch: `compactRecoveryTargetKindAdmissible` admits identical kinds, `compactStartTargetKindsCompatible` (current-changes ↔ base-diff, which is exactly 1744's scenario), `compactReleaseScopeRecovery`, or `compactApprovedStagedScopeRecoveryShape`; anything else returns a new `errCompactRecoveryTargetKindIncompatible` sentinel. |
| 1816 | `internal/reviewtransaction/compact_store.go:169-176` | Relax the `HEAD^2` requirement to "HEAD has at least one parent" so squash and rebase deliveries qualify; refuse a root commit with a typed message. This is safe because the guard was never the authority gate — `compactReleaseScopeRecovery` (`:393-401`) independently proves current-changes→base-diff kind, matching projection, identical candidate tree, and a strict genesis-path superset, and it is unchanged. |
| 1782 | `internal/reviewtransaction/compact_chain.go:452-458`, in `selectCompactPrePRChain` — **gate seam, see Finding 2** | Exempt the chain root `path[0].receipt.BaseTree` from the incoming-degree check. An edge arriving at the publication base is historical prologue outside the selected chain; genuine ambiguity is already rejected by the outgoing-degree fork check (`:444-451`) and the single-viable-candidate guard (`:430-432`). Requirement home is `Deterministic lifecycle validation`, not recovery-edge admission. |

**Apply-phase verification for 1782:** `compact_chain_test.go:421` runs an `addCompactChainConvergence` mutation that must stay RED. If that fixture injects its convergence at the chain root, re-pin it to a mid-chain node and add the root-prologue case as a new subtest. Confirm before editing production code.

Note that the `compactRecoveryTargetKindAdmissible` addition is a **fail-closed tightening** for `RecoveryInvalidated`, `RecoveryEscalated`, and correction-required scope-changed, which have no kind check today. Every existing recovery test must fall inside the admitted set; a full `internal/reviewtransaction` sweep is a gate for this commit.

### D — Store robustness (1813)

The poisoning entry point is not `DiscoverCompactStores` (`compact_store.go:662-697`), which never loads state and therefore never fails. It is each *consumer* that loads every enumerated store and treats one failure as fatal:

| Consumer | Fatal load |
|---|---|
| `internal/reviewtransaction/target_status_projection.go:56-59` | selector-free branch (`lineageID == ""`) |
| `internal/reviewtransaction/compact_store.go:558-560` (`CompactAuthorityLeaves`) | every store |
| `internal/cli/review_facade.go:2047-2049` | every store, → `ReviewAuthorityCorrupted` |

**Contract:**

1. `internal/reviewtransaction/compact.go:410-411` returns a typed `*CompactSemanticStateError{LineageID, State, Problem}` instead of an anonymous `errors.New`. `Load()` wraps it.
2. A single predicate `compactLineageQuarantinable(err error) (*CompactSemanticStateError, bool)` is true only when **all three** hold: (a) `errors.As` matches `*CompactSemanticStateError` — never a checksum, IO, or parse failure; (b) the state is terminal-for-lineage, i.e. `{StateApproved, StateEscalated, StateInvalidated}` — note this set differs from `facadeTerminalState` (`review_facade.go:1798-1800`), which excludes `StateInvalidated`, and 1813's shape *is* Invalidated; (c) the failure is the semantic-validation class, not a structural one.
3. The predicate applies **only on enumeration branches**: the three consumers above, and only where no explicit lineage selector was supplied. It is **never** applied on `target_status_projection.go:66-96` (explicit selector) or to the lineage being operated on. A genuinely corrupt active lineage therefore still fails closed, and so does an explicitly named quarantined one.
4. **Diagnostic, never silent:** `InventoryAuthority` (`internal/reviewtransaction/status.go:101-161`) already walks the same `v2` directories independently, so the quarantine surfaces as a structured `AuthorityInventoryDiagnostic{Path: <lineage dir>, Problem: "quarantined-terminal-lineage: ..."}` with a stable prefix. It must **not** flip `report.Complete` / `report.Authoritative` — a quarantined terminal lineage is excluded, not damage to live authority — and a regression test must prove that structural corruption still does flip them.

## Data flow

Group C bundles two **independent** flows. Keeping them visually separate is the point of Finding 2.

### 1782 — pre-PR gate chain selection (no recovery involved)

    review validate --gate pre-pr --base-ref origin/tracker
      ▼
    EvaluateCompactPrePRChain / deriveCompactPrePRChain
      ▼
    selectCompactPrePRChain
      ├─ cycle guard              (:393-395)   unchanged
      ├─ single-candidate guard   (:430-432)   unchanged  ← real ambiguity gate
      ├─ fork guard (outgoing)    (:444-451)   unchanged  ← real ambiguity gate
      └─ convergence (incoming)   (:452-458)   CHANGED: root exempt

### 1744 / 1816 — recovery admission

    review recover (argv)
      │  keep: flag-shape coherence  (--base-ref ⇔ --committed-only)
      │  drop: predecessor-kind coupling
      ▼
    SnapshotBuilder.Build / BuildReleaseScopeSnapshot
      │  1816: ≥1 parent, not ≥2
      ▼
    validateCompactRecoveryEdge
      │  NEW compactRecoveryTargetKindAdmissible  ← the moved fail-closed boundary
      ▼  per-disposition rules (unchanged)
    CompactRecord

## File changes

| File | Action | Description |
|---|---|---|
| `internal/cli/review_facade.go` | Modify | E deadline selector (`:255`); A delivery-base typing route (`:2069-2093`); B finalize evidence + typed refusal (`:1490-1495`, `:1743-1760`); G 1812 refusal (`:938-940`); C gate split (`:669-671`) |
| `internal/cli/review_next_transition.go` | Modify | B `Token` field (`:133`, `:157`); B escalate→recover routing (`:195-196`) |
| `internal/cli/review_schema.go` | Modify | B verification-evidence schema + usage string |
| `internal/cli/review_operation_contract.go` | Modify | A additive `Cause` + preflight branch (`:228-235`) |
| `internal/reviewtransaction/gate.go` | Modify | A sentinels at `:748`, `:752`, `:1153` |
| `internal/reviewtransaction/artifact_admission.go` | Modify | A 1699 canonicalization split (`:216`) |
| `internal/reviewtransaction/compact_store.go` | Modify | C kind admissibility (`:404`); C 1816 parent relaxation (`:169-176`); D quarantine at `CompactAuthorityLeaves` (`:558-560`) |
| `internal/reviewtransaction/compact_chain.go` | Modify | C 1782 root-incoming exemption (`:452-458`) |
| `internal/reviewtransaction/compact.go` | Modify | D typed `*CompactSemanticStateError` (`:410-411`) |
| `internal/reviewtransaction/target_status_projection.go` | Modify | D quarantine on the selector-free branch only (`:56-59`) |
| `internal/reviewtransaction/status.go` | Modify | D structured quarantine diagnostic |
| `internal/reviewtransaction/secure_open_unix.go` | Modify | F 1781 common-dir anchor + fallback (`:45-63`) |
| `internal/reviewtransaction/publish_immutable_darwin.go` | Modify | F 1804 ENOTSUP fallback (`:11-13`) |
| `internal/reviewtransaction/snapshot.go` | Modify | G 1771 / 1641 unborn-HEAD routing and message (anchor TBD) |
| `e2e/organicruntime/organic_lifecycle_hardening_test.go` | Create | 7 group journeys, 19 named subtests; keeps `organic_runtime_test.go` reviewable |

## Testing strategy

Every issue is RED-first. Unit proof lives next to the seam; each issue additionally carries one named E2E subtest whose name contains its issue number.

| Group journey (top-level `Test…`) | Subtests |
|---|---|
| `TestOrganicReviewStartDeadlineHeadroom` | `issue-1778` |
| `TestOrganicReviewLifecycleErrorTyping` | `issue-1801-published-delivery`, `issue-1699`, `issue-1666`, `issue-1807` |
| `TestOrganicReviewExecutableTransitionContract` | `issue-1745`, `issue-1775`, `issue-1663`, `issue-1788`, `issue-1800` |
| `TestOrganicReviewTargetShapeRefusals` | `issue-1812`, `issue-1771`, `issue-1641` |
| `TestOrganicReviewPlatformFailSafes` | `issue-1781`, `issue-1804` (both `t.Skip` off-platform with a recorded reason; unit tests carry the proof) |
| `TestOrganicReviewRecoveryGraph` | `issue-1744`, `issue-1816`, and `issue-1782` — the last is a **gate** scenario, not a recovery one: build receipts X→A, A→B, B→C with the tracker branch at A, then assert `review validate --gate pre-pr --base-ref origin/tracker` returns allow instead of `"compact receipt chain contains a convergence"`; control case deletes the X→A lineage and must return the same allow |
| `TestOrganicReviewStoreRobustness` | `issue-1813` |

Each journey builds its fixture once with the existing harness and reuses it across its subtests: `harness.writeFiles`, `harness.startReview`, `harness.runActor`, `harness.approveReview`, `harness.finalize`, `harness.gate` / `harness.gateAllowFailure`, `harness.gentle` / `harness.gentleAllowFailure`. Every journey ends with `harness.assertNoSDDArtifacts()` and `harness.assertSingleReviewLineage()`; byte-identity claims use `harness.lineageDigest(lineage)` captured before and after.

| Layer | What | Approach |
|---|---|---|
| Unit | Group A ladder, 1699 admission, 1745 tokens, 1775 schema key, C kind admissibility, D quarantine predicate | Table-driven, `t.TempDir()`, `t.Run(tt.name, …)` |
| Unit | Group F syscalls | Injected `renameNoReplace` / root-resolution seam; ENOTSUP and passthrough cases |
| Integration | `internal/cli/review_disabled_delivery_test.go` template | Gate-level non-regression for A and C |
| E2E | 19 named subtests above | `e2e/organicruntime`, real CLI invocation |

### Regression guards (must stay byte-identical)

| Guard | Where proven |
|---|---|
| Pre-push before push | `e2e/organicruntime` journey + `review_disabled_delivery_test.go` pattern |
| `--committed-only` review start and gate | Group C journey, asserted before the gate split lands |
| Governing-receipt allow | `TestReviewValidateReportsDisabledUnmanagedDelivery…` sibling asserting a governing receipt is **not** unmanaged (`review_disabled_delivery_test.go:115`) |
| `disabled/unmanaged` covered kinds | `reviewReceiptDiscoveryIsUnmanagedWhileDisabled` unit table — Missing, Unrelated, ScopeChanged true; Ambiguous, Corrupted false |
| Corrupt / ambiguous authority still fails closed | `review_disabled_delivery_test.go:439`, `:487`; plus a new D case proving structural corruption flips `report.Complete` |
| Existing 25 s facade deadline | `review_status_contract_test.go:848-861`, `review_final_verification_retry_test.go:261-264` unchanged |
| Chain convergence still rejected mid-chain | `compact_chain_test.go:421` mutation stays RED |

## Threat matrix

| Boundary | Applicability | Design response | Planned RED tests |
|---|---|---|---|
| Documentation-like paths | **N/A** — no change to path classification or execution decisions | — | — |
| Git repository selection | **Applicable** — 1781 re-anchors the lock walk at the Git common dir | Anchor at the resolved common-dir fd; fall back to the current root-anchored walk when the target is not under it | Unit: under-root walk, not-under-root fallback, symlinked component refused |
| Commit state | **Applicable** — 1816 (parent count), 1812/1771/1641 (staged, unborn, empty index) | 1816 accepts ≥1 parent and refuses a root commit; 1812/1641 emit typed refusals naming the escape verbatim | E2E `issue-1816`, `issue-1812`, `issue-1771`, `issue-1641`; unit for the root-commit refusal |
| Push state | **Applicable** — `gate.go:748` merge-base ambiguity, `:752` unconfigured remote, `:1153` delivery base | Typed sentinels; only the delivery-base class routes to `receipt_scope_changed`, the unconfigured remote keeps `authority_corrupted` with the cause in `Detail` | Unit ladder table per producer; E2E `issue-1801-published-delivery` |
| PR commands | **Applicable** — 1745 changes the argv an agent composes | Additive `Token` on `Execute.Arguments` only; `Preconditions` never carry argv | Unit: token shape for every execute transition; E2E `issue-1745` runs the emitted token literally |

## Decisions

| # | Decision | Rejected alternative | Rationale |
|---|---|---|---|
| 1 | Split the CLI recover predicate; move the kind check into `validateCompactRecoveryEdge` | Delete the whole CLI guard and trust the deeper check | Verified it has no general kind check (Finding 1). Deleting alone would silently admit currently-blocked recoveries. |
| 2 | Keep the argv-coherence clause in the CLI | Move that to `validateCompactRecoveryEdge` too | It is argv shape, not authority; dropping it builds a `TargetBaseDiff` with an empty `BaseRef` at `:688-689`. |
| 3 | 1816 relaxes to ≥1 parent | Keep the merge-commit requirement; or add a `--force` escape | `compactReleaseScopeRecovery` is the real gate and is unchanged, so the parent count adds no safety and only excludes squash/rebase deliveries. |
| 4 | 1782 exempts the chain root from the incoming check | Exclude historical receipts from the adjacency entirely | Exclusion would change chain selection for every case; the exemption is one predicate and is already backstopped by the fork and single-candidate guards. |
| 4b | 1782's requirement home is `Deterministic lifecycle validation` (`spec.md:143-145`) | The spec phase's new "Compact recovery edge admission" requirement; or `Semantic chain completeness` (`spec.md:119-121`) | Decided from the code (Finding 2). No recovery operation reaches the predicate. `Semantic chain completeness` governs persisted-successor invariants in one lineage; `selectCompactPrePRChain` selects a path across receipts from different lineages and validates no successor. Delta spec must move the scenario. |
| 5 | Published-delivery reclassifies to the **existing** `receipt_scope_changed` | New `delivery_base_ambiguous` terminal kind | Maintainer decision. A new terminal would need adding to `reviewReceiptDiscoveryIsUnmanagedWhileDisabled` and to every consumer; reusing the existing kind inherits the disabled/unmanaged behavior for free. |
| 6 | Unconfigured push remote stays `authority_corrupted` | Route it to `deliveryShape` with the others | It is environment fault, not a receipt that stopped governing; reporting `disabled/unmanaged` for a broken setup would be a false benign classification. |
| 7 | 1666/1807 add an additive `Cause` field | Rewrite `Code`/`Message` from `runErr` | Rewriting moves bytes on currently-passing paths. Additive JSON plus typed branches for provably-non-mutating causes gets the diagnosis with zero regression surface. |
| 8 | 1745 adds `Token` alongside `Name`/`Value` | Teach the flag parser detached booleans; or change `Name` to the hyphenated flag | Maintainer constraint. Changing `Name` moves bytes for every existing consumer; teaching the parser is a framework change. |
| 9 | D quarantines only on selector-free enumeration | A general "skip unloadable stores" escape | The narrow boundary is what keeps a genuinely corrupt *active* lineage failing closed, and keeps an explicitly named quarantined lineage failing closed too. |
| 10 | D terminal set includes `StateInvalidated` | Reuse `facadeTerminalState` | `facadeTerminalState` is Approved\|Escalated; 1813's actual shape is Invalidated-retaining-lens-results. Reuse would not fix the reported defect. |
| 11 | D diagnostic does not flip `Complete`/`Authoritative` | Mark the entry `AuthorityStatusInvalid` | That re-poisons `reviewAuthorityCorruptionConfinedToLegacyEntries` (`review_facade.go:2181-2183`) and reproduces 1813 through a different door. |
| 12 | 1778 is a START-specific constant plus a selector | Configurable timeout | Maintainer constraint; two existing tests mutate the general var directly. |
| 13 | 1663 and 1788 share one commit and one seam | Separate commits per issue | They are the same silent no-op; splitting would leave one half of the branch untested against the other. |
| 14 | Group F ships fail-safe fallbacks | Ship the direct fix and rely on community verification alone | Neither defect is reproducible in CI, so the fallback path must be the one that cannot regress a working configuration. |

## Migration / rollout

No migration. No schema version bump: `Token` and `Cause` are additive optional JSON fields, and `AuthorityInventoryDiagnostic` already exists. Rollback is `git revert` of the single group commit; the kill switch (`gentle-ai review mode disable`) remains the runtime escape throughout.

## Open questions

- [ ] 1771's selector-free unborn-HEAD branch is not line-pinned. The apply phase must pin it in `snapshot.go` before its RED test.
- [ ] 1775's evidence schema shape must be derived from `readCapturedFinalEvidence` / `review capture-evidence`, not authored.
- [ ] `compact_chain_test.go:421` `addCompactChainConvergence` — confirm the injected convergence is mid-chain, not at the root, before touching `compact_chain.go:452-458`.
- [ ] Adding `compactRecoveryTargetKindAdmissible` tightens three dispositions that have no kind check today. A full `internal/reviewtransaction` sweep is a gate for the Group C commit.
- [ ] **Orchestrator action:** the delta spec must move 1782's scenario out of "Compact recovery edge admission" into a MODIFIED `Deterministic lifecycle validation` (`openspec/specs/review-findings-ledger/spec.md:143-145`), and restate its trigger as a pre-PR gate evaluation over a receipt graph whose historical predecessor edge arrives at the selected publication base. Group D (1813) is the scenario that genuinely belongs to `Semantic chain completeness` (`spec.md:119-121`).
