# Tasks: Simplify SDD runtime interpretation

Planning record. The current human decision retains seven internal functional units (AI 3, Pi 4), with 400 authored additions plus deletions as a target, not a hard cap. Justified cohesive overruns are allowed. Exactly one final PR per repository is intended, both `size:exception`; no PR per unit or PR chain. No commits, publishing, PR/label changes, merge, release, or immediate delivery are authorized. Parent-owned native reconciliation/reset remains pending before execution; this policy change neither resets budgets nor changes history or consent.

## Review Workload Forecast

| Field | Value |
|-------|-------|
| Estimated changed lines | AI unit 1: 711–881 from the original implementation baseline; other units provisional below; final PR totals not yet measured |
| 400-line target risk | High; justified overruns allowed |
| Chained PRs recommended | No |
| Suggested internal sequence | AI units 1 → 2 → 3; Pi units 1 → 2 → 3 → 4, subject to task dependencies |
| Delivery strategy | exception-ok |
| Chain strategy | size-exception |

Decision needed before apply: Native reconciliation/reset remains pending with the parent; delivery policy is resolved.
Chained PRs recommended: No
Chain strategy: size-exception
400-line target risk: High

**Current explicit size decision:** `size-exception` is the supported strategy value for two final size-exception PRs, not a PR chain. Each internal unit keeps implementation, tests, fixture migrations, shipped guidance, strict TDD, and independent verification together. Advance only after that unit passes independent verification; never defer a broken slice's repair to a later unit.

**AI unit 1 reforecast:** the local estimate records 711–881 authored A+D against the original implementation baseline and 350–560 next-edit A+D, both BEFORE this planning correction. The proposed 950 ceiling was conditional and was not ratified; it is not a new hard gate or native limit. Cohesive behavior, both fixture migrations, and missing acceptance proofs justify the overrun—not compressed prose or dropped assertions. Count this three-file planning correction separately against its immediately preceding bytes; do not charge it to the old 25–40 documentation allowance. Reforecast subsequent work from the corrected candidate before execution. Final PR diffs must include complete planning documents wherever present; implementation-only subtotals are not final PR totals.

## Delivery topology and budget

Create no branches or PRs now. Retain only the proposed final branch names `feat/sdd-runtime-simplification-ai` and `feat/sdd-runtime-simplification-pi`, each for its repository's single eventual PR to `main`. There are no child branches, tracker bases, or predecessor PR bases. Internal units are verification/rollback boundaries, not delivery slices. Final acceptance requires the cross-repository tuple below; it authorizes no merge.

| Repository / internal unit | Focus | Provisional authored A+D |
|---|---|---:|
| AI unit 1 (1.1–1.4) | pure status, explicit continue, current-marker grant, fixture migrations and proofs | 711–881 before this correction |
| AI unit 2 (AI part of 3.1) | preserved admission/recovery contract | 60–85 |
| AI unit 3 (4.1–4.2) | proven runtime/replay/remediation plumbing only | 250–330 |
| Pi unit 1 (2.1–2.3) | v2 consumer, distinct handlers and host scope proof | 300–390 |
| Pi unit 2 (Pi part of 3.1, 3.2) | selected research grants and bounded persistence handoff | 280–390 |
| Pi unit 3 (4.3) | typed remediation and acquire/settle transport | 330–400 |
| Pi unit 4 (5.1) | installed uptake and cross-repository boundary proof | 210–290 |

All forecasts require recounting, including tests and guidance. Older per-task allocations below are provisional, not caps; AI unit 1's revised whole-unit forecast supersedes their sum. Neither repository has a measured final PR total yet. Include full planning-document content in each final PR where its diff contains it, not merely this correction's delta.

**Required final tuple:** one AI build identity, one Pi build identity, installed asset manifest/content, and `gentle-ai.sdd-status` v2 contract floor. The tuple must prove native producer → Pi consumer → managed child; process success, child self-report, or a decoder-only test is not acceptance.

## Execution rules for every work unit

- Keep the named test and shipped guidance/assets in the same work unit as its behavior. Run preservation characterization before defect RED; then record RED → GREEN → TRIANGULATE → REFACTOR evidence.
- An implementer records commands, artifacts, and rollback evidence. A different verifier reruns the stated command, inspects the resulting artifact/record and diff, and supplies acceptance evidence. Exit status alone is insufficient.
- Retire live local dispatch, exact-three assumptions, native-Engram bans, and legacy guidance across the named managed/source assets only after their callers are upgraded. Retained historical records remain readable; do not introduce dual-live schema readers or a compatibility layer.
- Do not add an engine, ledger, framework, generator, protocol/command, automatic reset, source reuse, upstream dependency, or speculative test outside the preserved guarantees. A proposed source edit below is conditional on its RED proof.

## 1. AI: pure status, authorized preparation, and binding

### 1.1 Preserve the existing v2 status contract before defect tests
- [x] **Start → end:** Start from the current AI v2/status behavior; add only preservation characterizations ending with an explicit baseline for seven dependencies, four instruction groups, and opaque present-consent output. Runtime settlement/replay preservation is owned solely by 4.1 and is not counted here.
  - **Paths:** `internal/sddstatus/status_v2_clean_break_test.go`.
  - **Depends on:** maintainer implementation authorization; no prior unit. **Forecast:** provisional 40–50 lines in AI unit 1.
  - **Focused command (future):** `go test ./internal/sddstatus -count=1 -json`.
  - **Managed/native boundary:** N/A—native preservation unit; managed uptake is proven in 5.1. **Independent proof:** verifier checks the JSON test record includes each intended preservation test and confirms no legacy live input is normalized. **Rollback:** remove only the added characterization cases.

### 1.2 RED: specify pure status, explicit continue preparation, and stale-marker refusal
- [x] **Start → end:** Start with status able to initialize/overwrite marker state and grant not comparing current persisted identity; end with failing tests that require read-only status snapshots, authorized continue preparation, no-replace/readback convergence, and A→recreated-B stale grant refusal.
  - **Paths:** proposed `internal/cli/sdd_status_continue_consent_test.go`, proposed `internal/cli/sdd_attempt_marker_binding_test.go`, `internal/sddstatus/status_v2_clean_break_test.go`. Historical genuine RED names: `TestSDDStatusDoesNotPrepareConsentMarker` and `TestSDDAttemptRefusesRecreatedMarker`. `TestSDDContinuePlanningMarkerScopePreparesWhileApplyStaysBlocked` and `TestSDDContinueRefusesWithoutMarkerScope` already passed historically and are preservation coverage, not defect RED. Missing proof cases still require their own truthful classification.
  - **Depends on:** 1.1. **Forecast:** provisional 80–100 lines within AI unit 1.
  - **Focused command (future):** `go test ./internal/cli ./internal/sddstatus -count=1 -json` must distinguish the two historical defect failures from preservation passes and new proof gaps; verifier checks actual `run` and assertion outcomes rather than a filename-shaped filter.
  - **Managed/native boundary:** native CLI status/continue boundary: repeated status has byte-identical marker/authority snapshots; explicit preparation re-renders v2 while source edit roots remain ungranted and apply is blocked. The host's human marker-scope precheck is separately proven in 2.2–2.3/5.1, not inferred from a writable native fixture. **Independent proof:** verifier observes recorded RED failure and confirms it did not create an attempt, grant, or artifact rewrite. **Rollback:** delete only proposed RED cases.

### 1.3 GREEN: move preparation to existing continue and bind grant to the persisted marker
- [x] **Start → end:** Start from 1.2's failing cases; end with `RunSDDStatus`/`Resolve` pure, only existing `RunSDDContinue` preparing an active selected OpenSpec marker when the current human scope includes the planning-directory marker path (even if source edit roots remain ungranted and apply stays blocked), and grant/replay refusing absent/mismatched/recreated identity without minting or authority append.
  - **Paths:** `internal/cli/sdd_status.go`, `internal/sddstatus/status.go`, `internal/sddstatus/edit_authority_consent.go`, `internal/cli/sdd_attempt.go`, `internal/sddstatus/runtime_ledger.go`; retain 1.2 tests. Migrate both loops in `e2e/organicruntime/sdd_consent_wiring_e2e_test.go` in this same unit using explicit built-binary continue then read-only status re-entry. Preserve all original assertions: initial no-marker/no-authority checks, missing-root ordering, exact grant/decline invocations, same-parent readiness, foreign-common-directory blocking, and single-repository byte identity. Keep `consentStatus` read-only; do not preseed invented tokens.
  - **Depends on:** 1.2. **Forecast:** 150–190 lines including test completion and `internal/assets/opencode/commands/sdd-status.md`, `internal/assets/opencode/commands/sdd-continue.md`, `internal/assets/skills/_shared/sdd-status-contract.md` guidance updates in this behavior slice.
  - **Focused command (future):** same unfiltered 1.2 package command, then `go test ./internal/cli ./internal/sddstatus -count=1`; then `go test ./... -count=1` and `go vet ./...`; independent verification must inspect both migrated fixture loops and all missing proofs. Historical focused passes alone do not accept this task.
  - **Managed/native boundary:** native explicit preparation returns bound consent while source roots remain ungranted and apply blocked; compare repeated absent/present marker, artifact, authority, and allowed-root snapshots. Ambiguous/unsafe selection, out-of-root context, and unwritable-at-entry directories publish nothing. Detected concurrent directory replacement during preparation emits no usable consent; this is not universal zero-write atomicity across a pathname race. True human read-only/excluded-marker suppression is host-owned in Pi tasks 2.2–2.3/5.1, not a native permission primitive. A directory at the marker filename proves filesystem failure, not human-scope denial or unwritable-directory behavior. **Independent proof:** separate verifier checks concurrent winner readback, malformed/readback failure, and stale A against absent and persisted distinct B with A's prior grant retained; refusal preserves marker/authority/attempt state. Shared publisher fallback may expose partial bytes: retain this limitation/proof gap, with fail-closed readback required; no universal atomic-visibility claim or shared publisher redesign is authorized. **Rollback:** revert this unit's behavior, paired assets, tests, and both organic fixture migrations together; never delete markers, history, or budgets.

### 1.4 TRIANGULATE/REFACTOR: consolidate resolver facts without new status protocol
- [x] **Start → end:** Start with passing primary status/continue tests; add malformed marker, publication failure, valid grant replay, Engram-only no-marker, and nullable discovery triangulation; end with shared internal missing-root facts rather than diagnostic parsing and no new public field/token/reason enum.
  - **Paths:** `internal/sddstatus/status.go`, `internal/sddstatus/edit_authority_consent.go`, proposed `internal/cli/sdd_status_continue_consent_test.go`, proposed `internal/cli/sdd_attempt_marker_binding_test.go`.
  - **Depends on:** 1.3. **Forecast:** old 25–35 allocation superseded by the whole-unit reforecast; missing negative proofs remain required.
  - **Focused command (future):** the unfiltered 1.2 package command with `-count=1 -json`; verifier confirms the intended named cases ran.
  - **Managed/native boundary:** N/A—negative native boundary already exercised in 1.3; this is internal permutation coverage. **Independent proof:** verifier checks no `ensureChangeInstanceMarker` remains reachable from `Resolve`, and that output schema/version is unchanged. **Rollback:** revert only triangulation/refactor hunks, preserving 1.3 behavior.

## 2. Pi: governed v2 status and separate continuation

### 2.1 Preserve supported consumer behavior before decoder defects
- [ ] **Start → end:** Start from accepted current supported-action behavior; characterize supported status rendering and refusal-before-execution without preserving the exact-three defect; end with a baseline that remains valid after v2 expansion.
  - **Paths:** proposed `tests/native-review-cli-sdd-status-v2.test.ts`, proposed `tests/sdd-status-continue.test.ts`.
  - **Depends on:** independently verified AI unit 1 contract fixture/CLI availability; Pi implementation authorization. **Forecast:** provisional 55–75 lines within Pi unit 1.
  - **Focused command (future):** `node --experimental-strip-types --test tests/native-review-cli-sdd-status-v2.test.ts tests/sdd-status-continue.test.ts`.
  - **Managed/native boundary:** N/A—consumer preservation unit; live child proof is 5.1. **Independent proof:** verifier confirms the cases consume producer-emitted v2 data, not an invented legacy alias. **Rollback:** remove only the proposed baseline cases.

### 2.2 RED: require the complete producer contract and no status-to-continue fallback
- [ ] **Start → end:** Start with the baseline; end with failing cases for all emitted action tokens, seven dependencies, optional four instruction groups, nullable discovery, wrong identity, malformed/unknown action, misleading prose, and separate status/continue calls.
  - **Paths:** proposed `tests/native-review-cli-sdd-status-v2.test.ts`, proposed `tests/sdd-status-continue.test.ts`, `tests/gentle-agents.test.ts`.
  - **Depends on:** 2.1 and AI unit 1's producer fixture. **Forecast:** provisional 110–145 lines within Pi unit 1.
  - **Focused command (future):** same as 2.1 plus `node --experimental-strip-types --test tests/gentle-agents.test.ts`; record RED failures first.
  - **Managed/native boundary:** host handler boundary proves `handleSddStatusCommand` cannot mutate and only expressly authorized continuation calls the mutating adapter. Read-only human scope and scope excluding the marker suppress that call; marker-only planning authorization may call it without granting source roots. **Independent proof:** verifier checks no malformed/prose route reaches a phase launch. **Rollback:** delete only RED cases.

### 2.3 GREEN/TRIANGULATE/REFACTOR: cut live reads to native v2 and keep handlers distinct
- [ ] **Start → end:** Start from 2.2 RED; end with native v2 decode/rendering and `sddStatus` read-only, a narrow authorized `sddContinue` adapter call, and no live local readiness reconstruction, `resolve-via-engram`, prefixed-token inference, automatic fallback, or `instructions` alias.
  - **Paths:** `lib/native-review-cli.ts`, `lib/sdd-status.ts`, `lib/sdd-preflight.ts`, `extensions/gentle-ai.ts`, `assets/agents/sdd-apply.md`; retain 2.1–2.2 tests.
  - **Depends on:** 2.2; must be synchronized with independently verified AI unit 1 before tuple acceptance. **Forecast:** provisional 135–170 lines including asset guidance within Pi unit 1.
  - **Focused command (future):** 2.2 commands, then package-declared `pnpm test`.
  - **Managed/native boundary:** status and selected startup call only native status; authorized continue alone invokes native continue and displays preparation text without executing it. **Independent proof:** independent verifier runs handler tests against the AI unit 1 binary/fixture and inspects invocation logs for no fallback/mutation. **Rollback:** revert Pi consumer, handler, asset, and tests together to the prior proven tuple; do not retain a dual-live reader.

## 3. Research capability and store-path continuation

### 3.1 Preserve native and managed admission/recovery facts before defects
- [ ] **Start → end:** Start with closed native declaration/admission and the currently supported managed route; end with separate preservation cases for exact class grants, matching/one-sided/missing-intent recovery, strict stale/divergent refusal, confirmed proposal handoff that does not interview, and automatic unresolved choices emitted once as a lossless grouped prompt—without encoding a whole-phase tool denial as desired behavior.
  - **Paths:** `internal/agents/researchcapability/contract.go`, `internal/components/sdd/research_state_contract_test.go`, `tests/sdd-research-capabilities.test.ts`, `tests/sdd-research-live.test.ts`. Proposed unique preservation names in `research_state_contract_test.go`: `TestConfirmedProposalHandoffDoesNotInterview` and `TestAutomaticUnresolvedChoicesEmitOneGroupedPrompt`.
  - **Depends on:** independently verified AI unit 1 only for shared status scope facts. **Forecast:** provisional 60–85 AI lines and 45–65 Pi lines within their respective internal unit 2.
  - **Focused commands (future):** `go test ./internal/agents/researchcapability ./internal/components/sdd -count=1 -json`; `node --experimental-strip-types --test tests/sdd-research-capabilities.test.ts tests/sdd-research-live.test.ts`. Verifier checks JSON `run/pass` records for the two proposed names and `TestHybridRestartRequiresByteEqualStateWithoutStorePreference`.
  - **Managed/native boundary:** N/A—preservation characterization; selected child transport is proven in 3.2 and 5.1. **Independent proof:** verifier confirms no host-inventory engine or additional capability class was added. **Rollback:** remove only characterization cases.

### 3.2 RED/GREEN/TRIANGULATE/REFACTOR: provision only selected admitted routes while retaining authorized persistence
- [ ] **Start → end:** Start with failing cases for documentation/open-web exact selected grants, inactive/missing extension tools, separately authorized read/write/Engram persistence, narrowed OpenSpec path, wrong worktree, and denial persistence; also require missing-tool denial → corrected capability/artifact facts → continuation with the **same** bounded selected-store path/scope, never a broadened or replacement scope. End with existing launch data carrying selected classes/per-class grants and actual extension selection, with child-local inventory recheck.
  - **Paths:** `lib/sdd-research-capabilities.ts`, `extensions/gentle-agents.ts`, `lib/agents-runner.ts`, `assets/agents/sdd-research.md`; retain 3.1 tests.
  - **Depends on:** 3.1 and independently verified Pi unit 1. **Forecast:** provisional 235–325 lines within Pi unit 2.
  - **Focused command (future):** 3.1 Pi command must first demonstrate RED; after GREEN run package-declared `pnpm test`. Verifier checks the named continuation case retained the identical bounded scope across denial and correction.
  - **Managed/native boundary:** fixed extension selection: selected research tools work; each absent/inactive route blocks research/proposal while the child persists denial through already-authorized selected-store tools. **Independent proof:** separate verifier inspects actual child allowlist plus loaded extension selection and both OpenSpec/Engram locator readbacks; no self-report accepted. **Rollback:** revert selected-class plumbing, research asset, and tests together; retain intent/evidence and never broaden roots or switch stores.

## 4. Typed remediation and truthful finite settlement

### 4.1 Preserve existing native settlement and replay guarantees before defect RED
- [x] **Start → end:** Start from existing runtime tests; end with distinct characterization cases for finite failed/interrupted/no-diff settlement, replay receipt idempotency, caps, failed-evidence binding, and fresh-selection strictness, without asserting the reported successor-acquire connection already passes.
  - **Paths:** `internal/sddstatus/runtime_compact_test.go`, `internal/sddstatus/runtime_truthful_remediation_settle_test.go`, `internal/sddstatus/verification_test.go`.
  - **Depends on:** 1.1 and independent acceptance of preceding AI units; AI unit 3. **Forecast:** provisional 30–45 lines; this is the sole AI unit 3 preservation allocation.
  - **Focused command (future):** `go test ./internal/sddstatus -count=1 -json`; verifier checks the intended preservation tests actually ran, including existing `TestCompactAcquire*`, `TestCompactSettle*`, `TestRuntimeFinishRecordsTruthful*`, and verification-package cases, rather than filtering on filenames.
  - **Managed/native boundary:** native acquire/settle preservation boundary. **Independent proof:** verifier confirms the baseline does not add a verification-success prerequisite for failed/interrupted settlement. **Rollback:** remove only characterization cases.

### 4.2 RED/GREEN/TRIANGULATE/REFACTOR: make only demonstrated native recovery plumbing changes
- [x] **Start → end:** Start with RED cases from the original failed baseline for successor-acquire replay, later-drift strictness, lost reply, and uncertain pre-settlement effect; end with the smallest proven connection through existing predicates, or a documented no-source-change result if the RED premise is invalidated by current behavior.
  - **Paths:** `internal/sddstatus/runtime_compact.go`, `internal/sddstatus/runtime_ledger.go`, `internal/sddstatus/verification.go`; retain 4.1 tests. Proposed unique RED names: `TestRuntimeSuccessorAcquireReconcilesHistoricalUntracked`, `TestRuntimeReplayRetainsLaterDrift`, and `TestRuntimeUncertainEffectRefusesUnsafeReplay`.
  - **Depends on:** 4.1; AI unit 3 blocks final tuple proof. **Forecast:** provisional 220–285 lines; AI unit 3 total 250–330.
  - **Focused command (future):** the unfiltered 4.1 package command, recording RED → GREEN → triangulation → refactor separately and checking the proposed test `run` records.
  - **Managed/native boundary:** native acquire/settle boundary with retained token, settle ID, real artifact/invocation evidence, record count, and lifetime charges. **Independent proof:** verifier replays exact committed settle payload without actor rerun and verifies unknown effects remain unknown. **Rollback:** revert only proven plumbing and paired cases; preserve immutable records, CAS, consent, refund caps, and failed evidence.

### 4.3 RED/GREEN/TRIANGULATE/REFACTOR: give the managed child typed remediation and existing compact bracket transport
- [ ] **Start → end:** Start with RED tests requiring `remediate` to refuse if unsupported, carry `failedEvidenceRevision` unchanged when supported, acquire once, and settle pass/fail/interruption through existing compact JSON; end with narrow adapter methods and runner finalization using retained session/task history, not a second ledger.
  - **Paths:** `lib/native-review-cli.ts`, `extensions/gentle-agents.ts`, `lib/agents-runner.ts`, `lib/sdd-preflight.ts:25–64,760–769` (`ASSET_OWNER_BY_KEY`), proposed `assets/agents/sdd-remediate.md`; tests `tests/gentle-agents.test.ts`, proposed `tests/sdd-managed-runtime-settlement.test.ts`.
  - **Depends on:** independently verified 4.2 and preceding Pi units, including Pi unit 1. **Forecast:** 330–400 provisional lines within Pi unit 3; explain and reforecast cohesive overruns of the 400-line target.
  - **Registration/install proof:** RED requires the asset-owner registry/install path to reject the absent remediation asset; GREEN registers it through the existing `ASSET_OWNER_BY_KEY`/installation mechanism and proves installed content is the selected actor before launch. 5.1 only re-proves final uptake; it is not first registration.
  - **Focused command (future):** `node --experimental-strip-types --test tests/gentle-agents.test.ts tests/sdd-managed-runtime-settlement.test.ts`, then `pnpm test`.
  - **Managed/native boundary:** acquired child spawn failure settles interrupted with process/cleanup facts; absent registry/install ownership refuses before launch; lost settle reply uses same token/settle ID/payload and never reruns actor or double-charges. **Independent proof:** verifier checks installed owner/manifest content, native record count/charges/evidence binding, and confirms failed/interrupted settlements do not require successful independent verification. **Rollback:** revert adapter, runner, preflight registration/install entry, typed asset, and tests as one behavior unit; retain existing attempt records and refuse unsupported consumers.

## 5. Installed managed uptake and cross-repository acceptance

### 5.1 RED/GREEN/TRIANGULATE/REFACTOR: prove the real producer → Pi → managed-child tuple
- [ ] **Start → end:** Start with a failing controlled boundary case using the built AI binary after its internal units pass independent verification, fixed Pi extension selection, and installed assets; end with a managed child that receives native v2 action/context, selected research/persistence tools, and typed remediation support, or fails safely before work when an asset/provisioning capability is deliberately absent.
  - **Paths:** `extensions/gentle-ai.ts`, `assets/agents/sdd-apply.md`, `assets/agents/sdd-research.md`, proposed `assets/agents/sdd-remediate.md`, `lib/agents-runner.ts`; proposed `tests/sdd-native-managed-uptake.test.ts`. The remediation actor registration/install prerequisite is already owned and proven by 4.3.
  - **Depends on:** independently verified AI units 1–3 and Pi units 1–3 (including 4.3); Pi unit 4 carries only final wiring/test adjustments necessary for this proof. **Forecast:** provisional 210–290 lines; final PR accounting remains separate.
  - **Focused command (future):** package-declared `pnpm run test:dev-binary` and `node --experimental-strip-types --test tests/sdd-native-managed-uptake.test.ts`; record exact binary paths/build identities and installed asset manifest/content.
  - **Managed/native boundary:** scope-authorized real producer → Pi → managed child proves host read-only/excluded-marker scope suppresses the mutating adapter, and exercises supported action, denial/partial research persistence, stale/wrong-worktree refusal, remediation binding, and incomplete-asset preflight refusal. **Independent proof:** an integrator not involved in either implementation independently rebuilds/runs the boundary, inspects native artifacts/ledger and asset manifest, and verifies no acceptance from child report or exit 0. **Rollback:** hold/revert the complete producer-consumer-asset tuple only to a prior tuple proven able to read retained records; otherwise refuse and keep read-only diagnosis.

## 6. Independent final verification and review gate

### 6.1 Verify both repositories without claiming implementation acceptance from a command alone
- [ ] **Start → end:** Start after all internal work units have independent acceptance and their RED/GREEN/TRIANGULATE/REFACTOR records and the 5.1 tuple passes; end with independently recorded full-suite results, diff/asset/record inspection, spec traceability, and a maintainer review package—never a delivery action.
  - **Paths:** review `internal/cli/`, `internal/sddstatus/`, `internal/agents/researchcapability/`, `internal/components/sdd/`, `internal/assets/`, Pi `lib/`, `extensions/`, `assets/agents/`, `tests/`; no new source asset is created by this task.
  - **Depends on:** 1.1–5.1, including 4.3. **Forecast:** no planned authored source lines; evidence records belong to the existing authorized verification process, not a new framework.
  - **Focused commands (future):** AI `go test ./... -count=1` and `go vet ./...`; Pi package-declared `pnpm test`, `pnpm run check:runtime-modules`, and `pnpm run test:dev-binary`.
  - **Managed/native boundary:** rerun 5.1 with independently built identities and inspect resulting records/artifacts. **Independent proof:** verifier maps results to the matrix below, verifies retired dispatch strings are absent from upgraded live callers/assets/help/refusals/tests, and reports failures as failures rather than accepting exit status. **Rollback:** no code rollback from this review-only task; use the relevant unit rollback boundary if verification fails.

## Spec coverage matrix

| Spec ID | Owning task |
|---|---|
| NR-01 | 1.2–1.4 |
| NR-02 | 2.2–2.3 |
| NR-03 | 2.3 |
| NR-04 | 2.2 |
| NR-05 | 3.2 |
| NR-06 | 3.2 |
| NR-07 | 4.2 |
| NR-08 | 4.2 |
| NR-09 | 4.3 |
| NR-10 | 4.3 |
| NR-10a | 4.3 |
| NR-11 | 4.1–4.2 |
| NR-12 | 4.2 |
| NR-12a | 4.2 |
| NR-13 | 6.1 |
| NR-14 | 5.1 |
| NR-15 | 5.1 |
| OA-01 | 3.1 |
| OA-02 | 3.1 |
| OA-03 | 2.3 |
| OA-04 | 1.2–1.3 native preparation; 2.2–2.3/5.1 host scope suppression |
| RS-01 | 3.1 |
| RS-02 | 3.1 |
| RS-03 | 3.2 |
| RS-04 | 3.2 |
| RS-05 | 3.1 |
| RS-06 | 3.1 |
| RS-07 | 3.1 |
| RS-08 | 3.1 |
| RS-09 | 3.2 |
| RS-10 | 3.2 |

## Out of scope and next gate

No bench corpus/driven scenario is planned: the required proofs are native CLI, managed-child, asset/provisioning, and runtime-ledger boundaries; no bench asset is identified by the accepted design. Do not add a benchmark merely to create coverage.

Current human policy supersedes the former hard-400, integrator-only exception, feature-branch-chain, and pre-implementation design-review stops. Keep seven cohesive internal units, each strict-TDD implementation → independent verification → only then advancement. Exactly two final PRs are intended, one per repository, both size exceptions; no delivery action is authorized now. Parent-owned native reconciliation/reset is still required before execution; finite budgets, history, and consent remain authoritative. Record partial progress honestly; no policy change converts failed evidence into acceptance.
