# Runtime simplification: native action, truthful status, and managed uptake

## Decision summary

Plan a deletion-first, two-repository migration whose canonical planning record lives in **Gentle AI**. The target is one native-admitted action for each execution step, with the runtime ledger owning admission, snapshot/action binding, settlement, and recovery selection. Gentle Pi becomes a strict managed consumer of that contract; it must not keep a competing phase dispatcher or silently reinterpret remediation.

This is planning only. No source changes, runtime reproduction, issue closure, approval label, commit, PR, or release are authorized.

## Current mechanism map

| Area | Implemented mechanism | Planning implication |
|---|---|---|
| Native status producer (AI) | `RunSDDStatus` resolves then projects `ProjectStatusV2`. The v2 projection exposes seven dependencies and four instruction groups: apply, verify, remediate, archive. | Make this one externally consumable status/action contract, then remove duplicate consumer assumptions. |
| Native status consumer (Pi) | Pi validates only `apply`, `verify`, `archive`, and requires *exactly* three dependency and instruction keys. | This is a static main-to-main contract incompatibility, not packaged-runtime E2E evidence. Pi must accept the governed contract before AI relies on the expanded surface. |
| Status routing (AI) | Declared artifact store is resolved, but status still invokes review-mode resolution before returning declared-Engram status. Shipped OpenCode command assets state that an Engram store must not use the native dispatcher. | Separate read-only status from execution/delivery/review preflight; align shipped instructions with the actual declared-store-capable resolver. |
| Runtime authority (AI) | Immutable records in the Git common directory plus CAS/replay produce `RuntimeStatus`; status is a projection. The runtime has candidate/worktree/evidence/consent controls, bounded attempts/lines, and typed terminal outcomes. | Preserve this authority. Do not create a second state machine or make status a ledger. |
| Recovery (AI) | Recovery currently has distinct reset/rescope/supersede/handoff/repair predicates. A historical intended-untracked selection is reconciled when replayed into a present candidate capture; fresh selections remain strict. | Preserve the predicates, provenance, consent, and no-budget-laundering invariant—not a permanent public verb taxonomy. A native-selected successor/recovery design may consolidate routes only when it preserves those invariants and freshness. |
| Remediation (AI/Pi) | AI has chain-bound native remediation instructions. Pi deliberately declines native remediation until it has a typed remediation executor. | Keep remediation semantically distinct. Never alias `remediate` to ordinary `apply`. |
| Consumer provisioning (Pi) | SDD child capability provisioning is a separate managed-uptake dependency (#593 depends on unmerged #605); asset ownership (#608) can prevent an otherwise correct contract from shipping. Background policy discrepancy #877 is separate. | Treat Pi adoption as explicit work with its own tests and asset ownership, not an automatic consequence of AI changes. |

Primary code anchors are collected in [evidence.md](evidence.md). The parent audit reports that the AI source examined at `350f3155` is unchanged for these mechanisms in this detached `3050dc4cd071b64cbd58e450ffaf6abf8ba46fc3` checkout except for an unrelated telemetry datasource file.

## Keep, delete, and derive

| Category | Item | Decision | Reason |
|---|---|---|---|
| Keep | Immutable common-directory ledger, CAS, one active attempt, candidate/worktree/evidence/consent bindings | Keep | Canonical execution authority; preserves bounded execution and auditability. |
| Keep | Finite passed/failed/interrupted settlement and capped productive/harness-invalidated refunds | Keep | Honest terminal accounting; zero diff is not free execution. |
| Keep | Recovery invariants: distinct authorized conditions, provenance, consent, candidate freshness, and no budget laundering | Keep | The current reset/rescope/supersede predicates are evidence for these invariants. Later design MAY consolidate public routes behind native-selected recovery only if it preserves them; it must not substitute or silently weaken an authority decision. |
| Keep | Chain-derived remediation obligation and independent verification before archive | Keep | Failed evidence must bind to a real subject and correction evidence. |
| Delete | Consumer-owned phase vocabulary/arity assumptions and route copies | Delete after governed consumer uptake | They drift from the producer and already reject the current producer shape. |
| Delete | Shipped rule forbidding native dispatch for declared Engram | Delete/replace | It contradicts current AI declared-store resolution. |
| Delete | Read-only status dependency on irrelevant execution/delivery/review preflight | Delete | Inspection must not require permission to execute or deliver. |
| Derive | Consumer next action/instructions | Derive from native v2 contract | A consumer displays/executes only the native-selected action; it does not infer from prose. |
| Derive | Runtime readiness and recovery instruction | Derive from shared native predicate/ledger state | Prevent a second recovery router or mismatched status claim. |
| Do not add | Universal signed-context framework, compatibility readers, waiver paths, parallel state machine, unaudited reset | Excluded | Broad durable vocabulary and fallback readers enlarge the problem and erode authority. |

## Ownership and source of truth

| Repository | Owns | Future scope boundary |
|---|---|---|
| Gentle AI | Runtime ledger and admission; status schema/projection; read-only status preflight behavior; native recovery/remediation semantics; canonical planning artifacts for this change | Define and test the contract before asking Pi to consume it. AI remains the only canonical design/spec/task artifact location. |
| Gentle Pi | Package-local native status decoder/adapter; typed action execution; agent/asset provisioning and consumer tests | Consume the versioned AI contract exactly. Do not duplicate ledger policy, create a fallback reader, or change AI planning artifacts. |
| Integration | Feature-branch chain per repository, one integrator per repo; child targets immediate predecessor; tracker targets `main` only after integration | No PR creation is currently authorized. The selected delivery topology is recorded for later work, not activated now. |

## Ranked migration approach

1. **Contract inventory and deletion boundary (lowest risk).** Enumerate every producer, decoder, phase asset, dispatcher prompt, help/error string, and test that names phase keys or native-dispatch restrictions. Establish a single versioned contract shape and a consumer migration matrix. This prevents changing the ledger while duplicate routes remain.
2. **Make status genuinely read-only in AI.** Move only execution/delivery/review checks that are unnecessary for resolving a status out of the status path. Status may report actionable blocks that are intrinsic to the selected change, but MUST NOT require launch authorization merely to inspect it. Add focused regression tests for declared OpenSpec and declared Engram resolution.
3. **Normalize native action selection and contract projection in AI.** Keep the ledger’s existing authority; expose a bounded native-selected action/result shape sufficient for a consumer to act without reconstructing state. Define remediation as its own typed action or explicitly unavailable action—not apply-shaped prose.
4. **Update Pi decoder and typed executor together.** Replace exact-three validation with governed schema validation that accepts the canonical phase set and rejects unknown/invalid values. Add typed remediation handling only once its execution semantics are supported; until then report a clear unsupported-native-action result rather than substituting apply.
5. **Managed rollout and removals.** Ship Pi asset/provisioning ownership prerequisites, migrate consumers, then remove old dispatcher-only route wording and duplicate contracts. Gate removal on cross-repo contract tests and a packaged/native boundary test selected during design.

This order deliberately changes consumer interpretation before relying on a producer expansion, and removes duplicate routes before touching ledger mechanics.

## Scope and non-goals

**In scope**

- A narrow AI-native status/action contract that preserves ledger authority.
- Read-only status that does not hard-gate on irrelevant execution/delivery/review preflight.
- Pi’s managed decoding/execution uptake, including explicit remediation capability behavior.
- Evidence binding to the actual candidate/action/settlement subject and finite outcomes.
- Issue-cluster dispositions and future acceptance scenarios.

**Out of scope**

- Replacing the immutable ledger, CAS, candidate capture, worktree binding, consent, or bounded budgets.
- A universal signed-context or broad identity framework (#2238 is not approved for adoption).
- Compatibility readers, dual-format readers, silent action aliases, waivers, or any recovery redesign that loses audited provenance, consent, freshness, or no-budget-laundering safeguards.
- Treating RDD as delivery or archive authority. RDD remains advisory/evidence-related only.
- Closing reports based on this plan; reports that lack current reproduction remain open or “improved, not closed.”
- Pi source mutation, package publishing, and all delivery operations.

## Contract rollout risks and mitigations

| Risk | Why it matters | Planned control |
|---|---|---|
| Producer/consumer shape drift | Pi’s exact-three check already rejects AI’s seven dependencies/four instructions. | Contract fixture shared by value, schema-version compatibility tests in both repos, and consumer acceptance before producer reliance. |
| Read-only status still blocks | A refactor can accidentally retain review-disabled or delivery gates through a helper. | Tests distinguish status inspection from runtime acquire/settle and delivery/archive authorization. |
| Remediation is downgraded to apply | A generic executor could execute the wrong action and lose failed-evidence binding. | Discriminated typed action; reject unsupported remediation explicitly; no fallback alias. |
| Freshness becomes permanent identity cache | Avoiding stale selection errors must not accept unrelated later candidate state. | Reuse the existing replay-only reconciliation / fresh-capture distinction; test tracked-after-begin and subsequent drift separately. |
| Legacy/compatibility pressure | Dual readers and shims preserve dead routes indefinitely. | Version the replacement contract and fail closed with an actionable upgrade/rerun path; retain immutable history for forensics only. |
| Pi uptake is blocked despite code readiness | #605 provisioning and #608 asset ownership affect whether children actually receive the contract/capability. | Make prerequisites explicit in Pi tasks and test the packaged/asset surface, not just TypeScript decoder units. |
| Review overload | The likely two-repo work and regression corpus exceed a single 400-line review budget. | Use the already selected feature-branch chain later; one deliverable work unit per child, immediate-predecessor bases, one integrator per repo. No size exception is assumed. |

## Resolved constraints and remaining design work

The following are resolved by current-session human instruction and existing publication policy; they are not open product questionnaires:

- **C1 — target:** one native-admitted action per step is confirmed. The exact wire shape and version are delegated technical design; no particular JSON schema was selected here.
- **C2 — remediation:** Pi MUST NOT fall back to apply. Until a bounded typed executor exists it MUST report an explicit unsupported capability without inventing an unverified runnable upgrade/native command. End-to-end typed remediation is a target-plan work unit, not an indefinite deferral.
- **C3 — rollout:** derive the compatible release/capability floor and coordinated rollout from provider, consumer, and installed-asset proofs. No release number is selected or authorized in planning.
- **C4 — issue policy:** do not speculate about closure; no issue closure is authorized. Reports require a named future test or current-main verification before any later evidence-based disposition.

**Remaining technical design work:** define the narrow versioned native action/result representation, retain display instructions as non-authoritative guidance, derive the tested compatibility floor, and map current recovery predicates to a simpler native-selected successor interface without reducing authority. No new product choice is identified by this exploration.

## Optional research recommendation

No external research is a proposal gate: the existing bounded audit and source evidence are sufficient to draft the proposal. A future optional validation task MAY refresh public #4484 and Pi #605/#608/#877 state, merge metadata, and diff/test/asset evidence; it must not be presented as already verified or as a mandatory new research lane.

## Planning forecast

The full proposal/specs/design/tasks set and later two-repository implementation are likely to exceed 400 changed lines in aggregate. That is a forecast, not a size exception. Later delivery must pause under `ask-on-risk` and use the confirmed feature-branch-chain strategy if the reviewed slice exceeds the budget.
