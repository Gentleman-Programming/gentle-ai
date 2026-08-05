# RDD Ownership Inventory — Wave 0

**Snapshot:** `ece470dacd0041f394e7f6f3877a6a9fcb3482af` (`origin/main`, `fix(sdd): prefer approved authority over stale lineage (#2131)`).

This inventory is **not live authority**. Every row was derived by read-only CodeGraph-first source tracing against the pinned snapshot above. A later commit can change any cited `path:line`; this document is a Wave 0 instrument, not a runtime source of truth. Re-deriving it at a new snapshot means re-running the enumeration in `docs/architecture/rdd-root-simplification-design.md` §"Enumerate from authoritative enumeration points, never from prose", not editing rows by hand.

## Method

Each family below is enumerated from the same authoritative source-of-record the amended design names (`docs/architecture/rdd-root-simplification-design.md`, decision "Enumerate from authoritative enumeration points, never from prose"), using `codegraph_explore` and read-only `git show <sha>:<path>` against the pinned snapshot — never from prose or memory. A row that cannot be anchored to at least one `path:line@ece470da` or contract file/schema ID is dropped and logged under **Inventory gaps** instead of being guessed.

Two ownership columns are recorded per the design's "Inventory records observed ownership and target ownership separately" decision:

| Observed current owners | Row status |
|---|---|
| exactly 1 | clean |
| 0 | finding: `unowned` |
| more than 1 | finding: `split-ownership` |
| target not derivable from the design's Ownership boundaries table | finding: `undesignated-target` |

Target owner is drawn from the design's closed set: `ReviewCore`, `AuthorityStore`, `CandidateResolver`, `ReviewAdapter`, `DeliveryGate`, `SDD`. Target disposition reuses the design's control-reduction verbs: `KEEP`, `MERGE`, `DERIVE`, `DOWNGRADE`, `REMOVE`, `FAIL-CLOSED ONLY`.

**Row schema:** `ID | Surface | Kind | Current owner(s) | Target owner | Consumers | Evidence | Target disposition`

## Lifecycle transitions (`TRN`)

| ID | Surface | Kind | Current owner(s) | Target owner | Consumers | Evidence | Target disposition |
|---|---|---|---|---|---|---|---|
| TRN-01 | Legacy transaction state vocabulary (13 values: `unreviewed`…`invalidated`) | State enum | `internal/reviewtransaction` (1) | `ReviewCore` | All CLI review commands, `internal/sddstatus` | `internal/reviewtransaction/transaction.go:36-50` (const block); full compat switch at `:1101-1103` | REMOVE — superseded by the target five-state model (`reviewing/correcting/validating/approved/escalated`) |
| TRN-02 | `next_transition` producer — decides the one executable transition returned to callers | Decision function | `internal/cli` (`newReviewNextTransition`) **and** `internal/reviewtransaction` (state mutation sites, e.g. `transaction.State = StateReviewing`) (2) | `ReviewCore` | `internal/app` dispatch, adapters (OpenCode/Pi/Claude) | `internal/cli/review_next_transition.go:123`; `internal/reviewtransaction/transaction.go:288,302,608,656,669,688,870,880,958,960` (independent state mutation sites) | MERGE — one native `ReviewCore` returns the only executable transition |
| TRN-03 | Public operation dispatch forms (`review`, `review-start`, `review-resume`, `review-step`, `review-bundle-export`, `review-bundle-import`, `review-validate`) | CLI command surface | `internal/app` (1) | `ReviewCore` | End users, adapters, CI hooks | `internal/app/app.go:100,107,109,111,113,115,117`; cross-checked `internal/app/help_test.go:14` | MERGE — historical action/command strings collapse into one opaque provider-issued transition reference |

## Persisted artifacts (`ART`)

| ID | Surface | Kind | Current owner(s) | Target owner | Consumers | Evidence | Target disposition |
|---|---|---|---|---|---|---|---|
| ART-01 | Mutable CAS authority record | Active artifact | `internal/reviewtransaction` (1) | `AuthorityStore` | `ReviewCore`, all five gates | `internal/reviewtransaction/store.go`, `internal/reviewtransaction/compact_store.go` | KEEP — one of the two active artifacts in the target model |
| ART-02 | Immutable terminal receipt | Active artifact | `internal/reviewtransaction` (1) | `AuthorityStore` | All five gates, SDD | `internal/reviewtransaction/receipt.go`, `internal/reviewtransaction/rar_native_receipt.go` | KEEP — the other of the two active artifacts |
| ART-03 | Batch-reconcile journal | Historical artifact | `internal/reviewtransaction` (1) | `AuthorityStore` | Recovery/repair paths | `internal/reviewtransaction/compact_batch_reconcile_journal.go` | REMOVE — public journal persistence retired; immutable external evidence references only |
| ART-04 | Finalize-attempt journal | Historical artifact | `internal/reviewtransaction` (1) | `AuthorityStore` | Finalize/correction flow | `internal/reviewtransaction/finalize_attempt_journal.go` | REMOVE — same rationale as ART-03 |
| ART-05 | Bundle export/import artifact | Historical artifact | `internal/reviewtransaction` (1) | `AuthorityStore` | `review-bundle-export`/`review-bundle-import` CLI forms | `internal/reviewtransaction/bundle.go`; dispatch at `internal/app/app.go:113,115` | REMOVE — per Deletion plan: bundle import/export retires once independent-clone re-review is documented |
| ART-06 | Legacy fix-scope quarantine | Recovery residue | `internal/reviewtransaction` (1) | `ReviewCore` | Recovery/repair paths | `internal/reviewtransaction/legacy_fix_scope_quarantine.go` | REMOVE — superseded by the wave-2 classified disposition-plan facade |
| ART-07 | Legacy quarantine (general) | Recovery residue | `internal/reviewtransaction` (1) | `ReviewCore` | Recovery/repair paths | `internal/reviewtransaction/legacy_quarantine.go` | REMOVE — same rationale as ART-06 |
| ART-08 | Compat golden fixture (`v1.49.0-ordinary-4r` receipt artifact) | Test/golden fixture, not live authority | test-only (1) | — (see Findings: `undesignated-target`) | Compatibility/regression tests only | `internal/reviewtransaction/testdata/v1.49.0-ordinary-4r/artifacts/receipt.json` | N/A — the design's closed ownership set has no "compatibility golden" bucket |

## Contract surfaces (`CTR`)

| ID | Surface | Kind | Current owner(s) | Target owner | Consumers | Evidence | Target disposition |
|---|---|---|---|---|---|---|---|
| CTR-01 | `review-integration/v1` schemas | Contract schema dir | contract authors + `internal/cli` constant authors (2) | — (see Findings) | All v1-speaking adapters | `contracts/review-integration/v1/schemas/**` | DERIVE — generated projections from one contract model |
| CTR-02 | `review-integration/v1` fixtures | Contract fixture dir | contract authors + `internal/cli` constant authors (2) | — (see Findings) | Conformance tests | `contracts/review-integration/v1/fixtures/**` | DERIVE |
| CTR-03 | `review-integration/v2` schemas | Contract schema dir | contract authors + `internal/cli` constant authors (2) | — (see Findings) | All v2-speaking adapters | `contracts/review-integration/v2/schemas/**` | DERIVE |
| CTR-04 | `review-integration/v2` fixtures | Contract fixture dir | contract authors + `internal/cli` constant authors (2) | — (see Findings) | Conformance tests | `contracts/review-integration/v2/fixtures/**` | DERIVE |
| CTR-05 | Status/projection schema-ID constants (v1–v4) | Go constant set | `internal/cli` (1) | — (see Findings) | Status-contract producers/consumers | `internal/cli/review_status_contract.go:15-24` | DERIVE |
| CTR-06 | Operation/failure schema-ID constants (v1/v2) | Go constant set | `internal/cli` (1) | — (see Findings) | Operation-contract producers/consumers | `internal/cli/review_operation_contract.go:19-26` | DERIVE |

## Delivery gates and consumers (`CON`)

| ID | Surface | Kind | Current owner(s) | Target owner | Consumers | Evidence | Target disposition |
|---|---|---|---|---|---|---|---|
| CON-01 | `post-apply` gate | Delivery gate | `internal/reviewtransaction` (1) | `DeliveryGate` | Post-apply lifecycle hook | `internal/reviewtransaction/receipt.go:133`; ordering at `internal/cli/review_operation_contract.go:1457-1460` | KEEP |
| CON-02 | `pre-commit` gate | Delivery gate | `internal/reviewtransaction` (1) | `DeliveryGate` | Pre-commit hook | `internal/reviewtransaction/receipt.go:134`; ordering as above | KEEP |
| CON-03 | `pre-push` gate | Delivery gate | `internal/reviewtransaction` (1) | `DeliveryGate` | Pre-push hook | `internal/reviewtransaction/receipt.go:135`; ordering as above | KEEP |
| CON-04 | `pre-pr` gate | Delivery gate | `internal/reviewtransaction` (1) | `DeliveryGate` | Pre-PR hook | `internal/reviewtransaction/receipt.go:136`; gate-specific base-relationship logic at `:289,304,316,348,355`; ordering as above | KEEP |
| CON-05 | `release` gate | Delivery gate | `internal/reviewtransaction` (1) | `DeliveryGate` | Release hook | `internal/reviewtransaction/receipt.go:137`; gate-specific logic at `:304,307,316`; ordering as above | KEEP |
| CON-06 | SDD review-gate bridge | Consumer | `internal/sddstatus` (1) | `SDD` | `sdd-status`, `sdd-continue` | `internal/sddstatus/review_gate.go` | MERGE — fold into "request receipt validation when needed"; do not independently reinterpret review state |
| CON-07 | SDD review-binding record (`gentle-ai.sdd-review-binding/v1`) | Consumer / persisted mirror | `internal/sddstatus` (1) | `SDD` | `sdd-status` | `internal/sddstatus/review_binding.go` | REMOVE — per Deletion plan: "SDD review-binding and remediation mirrors" retire once SDD consumes `ReceiptRef` only |
| CON-08 | SDD runtime attempt ledger (change-level `Complete`) + runtime-compact objective (work-unit-scoped) | Consumer / persisted artifact | `internal/sddstatus/runtime_ledger.go` **and** `internal/sddstatus/runtime_compact.go` (2) | SDD — `RuntimeObjective`/`BeginAttemptRequest` (`internal/sddstatus/runtime_ledger.go`) unconditionally; `runtime_compact.go` is a projection only | `sdd-continue`, `sdd-status`, apply/verify phases | `internal/sddstatus/runtime_ledger.go`, `internal/sddstatus/runtime_compact.go`; dated evidence — issue #2133 (change-level `Complete` contradicting work-unit-scoped objectives), fixed by PR #2151 merged 2026-08-02 (postdates the `ece470da` pin; recorded as dated evidence, not part of the pinned snapshot state); Wave 4 S2 collapsed `CompactAcquireRequest`'s independent `WorkUnit`/`EvidenceGoal`/`MaxAttempts`/`MaxChangedLines` fields into an embedded `BeginAttemptRequest`, proven by `TestRuntimeObjectiveIsSoleWorkUnitScopeOwner` | Decision 9 — **MAINTAINER-CONFIRMED (2026-08-02, ratified)**: attempts stay in SDD; `RuntimeObjective` is the single work-unit-scope owner, unconditionally |
| CON-09 | OpenCode adapter dispatch + reviewer-result plugin | Consumer / adapter | `internal/agents/opencode` + `internal/assets/opencode/plugins` (1 in-repo dispatch surface; behavioral depth traced Wave 4 S1) | `ReviewAdapter` | OpenCode host runtime | `internal/agents/opencode/adapter.go` (clean: 0 review references, proven by `TestAdapterForbiddenConstructionGuardHoldsForProductionFiles`), `internal/assets/opencode/plugins/review-result-artifacts.ts` (**violates**: declares its own `ReviewBinding` type, composes `admissionRecoveryKey` from lineage/target/revision/context/lens/order/subject_hash, holds a session-scoped recovery budget `claimAdmissionRecovery`/`MAX_ADMISSION_RECOVERIES_PER_SESSION`) | KEEP the Go adapter (already thin, guard-proven); the bundled plugin asset's consumer-side recovery state is tracked for removal by Wave 4 S3/S7 |
| CON-10 | Pi adapter dispatch | Consumer / adapter | `internal/agents/pi` (1 in-repo dispatch surface; behavioral depth traced Wave 4 S1) | `ReviewAdapter` | Pi host runtime | `internal/agents/pi/adapter.go` (clean: 0 review references, proven by `TestAdapterForbiddenConstructionGuardHoldsForProductionFiles`); gentle-pi host repo is out of this repository and gated by its declared capability only | KEEP |
| CON-11 | Claude adapter dispatch | Consumer / adapter | `internal/agents/claude` (1 in-repo dispatch surface; behavioral depth traced Wave 4 S1) | `ReviewAdapter` | Claude host runtime | `internal/agents/claude/adapter.go` (clean: 0 review references, proven by `TestAdapterForbiddenConstructionGuardHoldsForProductionFiles`); `internal/assets/claude/commands/sdd-apply.md` (**violates**: hardcoded contract `gentle-ai.review-integration/v1` pin, fixed Wave 4 S1 to `/v2`) | KEEP the Go adapter (already thin, guard-proven); the pin mismatch is fixed in this same slice |
| CON-12 | External OpenCode/Pi/Claude host-runtime execution of the `GENTLE_AI_REVIEW_BINDING` protocol | Consumer | 0 observable in-repo | `ReviewAdapter` | End users running each host product | evidence: out-of-repo | N/A — cannot assign a control-reduction verb to a surface this repository does not contain |

## Findings

Unowned, split-ownership, and undesignated-target rows, left unresolved per the design's decision to keep findings mechanical rather than editorial:

- **TRN-02** — `split-ownership`. `internal/cli/review_next_transition.go` and `internal/reviewtransaction/transaction.go` both independently derive "what happens next" — exactly the design's named root cause ("Split transition ownership").
- **TRN-03** — documentation-drift evidence (not a row-status category, recorded per task instruction). `internal/app/app.go:111` dispatches `case "review-step"`, but `internal/app/help_test.go:14`'s `commands` list omits `"review-step"`. Not fixed in this pass — `help_test.go` is out of docs-only scope.
- **ART-08** — `undesignated-target`. The compat golden fixture has no cell in the design's Ownership boundaries table; it is test-only, not governed authority.
- **CTR-01 through CTR-06** — `split-ownership` (contract authors and hand-written Go constant authors independently maintain the same schema truth — the design's "Replicated contract truth" root cause) **and** `undesignated-target` (none of `ReviewCore`/`AuthorityStore`/`CandidateResolver`/`ReviewAdapter`/`DeliveryGate`/`SDD` is named as owning contract/schema generation in the Ownership boundaries table).
- **CON-08** — `split-ownership`, dated. Observed live during this wave: issue #2133 (change-level `Complete` contradicting work-unit-scoped objectives), fixed by PR #2151 merged 2026-08-02. This is the exact finding class — split ownership between a durable ledger and a bounded per-objective record — that this inventory exists to surface, occurring in real time rather than only historically.
- **CON-12** — `unowned` (0 in-repo owners) and flagged in Inventory gaps below.

## Inventory gaps

- **CON-12 / out-of-repo host runtimes.** The actual execution of reviewer subprocesses inside the OpenCode, Pi, and Claude host applications is outside this repository. Only the in-repo dispatch surfaces (CON-09, CON-10, CON-11) were enumerated with evidence; the host-runtime behavior itself is recorded as `evidence: out-of-repo` and is not traced further in Wave 0.
- **No dropped rows.** Every row above carries at least one `path:line@ece470da` or contract-directory evidence reference; none were dropped for lack of an anchor in this pass.

## SDD attempt-ledger ownership (Decision 9)

CON-08's target owner cell names `RuntimeObjective` unconditionally, per decision 9's maintainer-confirmed ratification (2026-08-02): the evidence condition (durable, cumulative, CAS-like properties in SDD's own store — `previous_revision` chaining, CAS `expected_revision`, `request_digest` replay identity) is already met, so the prior conditional "only if" wording no longer applies. `AuthorityStore` (native authority) does not own SDD's work-unit attempts.
