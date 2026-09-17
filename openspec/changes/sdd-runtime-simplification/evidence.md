# Durable evidence — runtime simplification

## Evidence posture

This file preserves the bounded parent audit as durable planning evidence for [Gentle AI #4484](https://github.com/Gentleman-Programming/gentle-ai/issues/4484), the canonical tracker. The tracker is `enhancement` / `status:needs-review`; it is **not** approved. The high-level direction in this change is user-confirmed session input, not an implementation approval.

**Audit basis:** parent completed a public-GitHub and source audit before this exploration: 2,591 AI issues (692 open), 522 Pi issues (174 open), 618 broad candidate threads captured, 38 threads deeply read, and 8 PR bodies read. The 136 AI / 32 Pi PR-index candidates and 132 AI / 14 Pi selected-open issues are different counting units, not a discrepancy. Source audit used AI main `350f3155` (reported unchanged for these mechanisms in detached AI `3050dc4cd071b64cbd58e450ffaf6abf8ba46fc3`, except unrelated telemetry datasource work) and Pi main `564ae19b70bdf32f8f2e9865ed068742a1908834`.

**Limits:** this phase did not reproduce a packaged runtime, execute tests, inspect a Pi graph, verify current remote issue state, or validate any in-flight PR diff/test. It performed bounded source reads only. A report is not a reproduction; an issue title is never evidence. The parent’s temporary public snapshot was an input, but this file—not that temporary location—is the durable record.

## Direct source observations

| Observation | Evidence class | Exact public source |
|---|---|---|
| AI CLI resolves status, explicitly calls `ProjectStatusV2`, then encodes that projection for JSON output. | Direct source read | [AI `internal/cli/sdd_status.go` L20–45](https://github.com/Gentleman-Programming/gentle-ai/blob/3050dc4cd071b64cbd58e450ffaf6abf8ba46fc3/internal/cli/sdd_status.go#L20-L45) |
| AI v2 declares dependencies `proposal`, `specs`, `design`, `tasks`, `apply`, `verify`, `archive`; instruction groups are `apply`, `verify`, `remediate`, `archive`; the projector copies those fields. | Direct source read; static contract | [AI `status_v2.go` L13–160](https://github.com/Gentleman-Programming/gentle-ai/blob/3050dc4cd071b64cbd58e450ffaf6abf8ba46fc3/internal/sddstatus/status_v2.go#L13-L160) |
| AI’s status resolver reads declared store but invokes review-mode resolution before declared-Engram resolution. | Direct source read | [AI `status.go` L475–533](https://github.com/Gentleman-Programming/gentle-ai/blob/3050dc4cd071b64cbd58e450ffaf6abf8ba46fc3/internal/sddstatus/status.go#L475-L533) |
| Shipped OpenCode `sdd-status` asset forbids native dispatcher for Engram; `sdd-continue` contains the same route. | Direct source read; shipped asset | [AI `sdd-status.md` L20](https://github.com/Gentleman-Programming/gentle-ai/blob/3050dc4cd071b64cbd58e450ffaf6abf8ba46fc3/internal/assets/opencode/commands/sdd-status.md#L20), [AI `sdd-continue.md` L13](https://github.com/Gentleman-Programming/gentle-ai/blob/3050dc4cd071b64cbd58e450ffaf6abf8ba46fc3/internal/assets/opencode/commands/sdd-continue.md#L13) |
| AI native status instructions prescribe acquire/settle and chain-bound remediation; reset and rescope are distinct recovery routes. | Direct source read | [AI `status.go` L1974–2055](https://github.com/Gentleman-Programming/gentle-ai/blob/3050dc4cd071b64cbd58e450ffaf6abf8ba46fc3/internal/sddstatus/status.go#L1974-L2055) |
| Runtime status is a projection containing objective/attempts/next action and recovery projections; immutable record operations include supersede. | Direct source read | [AI `runtime_ledger.go` L26–56](https://github.com/Gentleman-Programming/gentle-ai/blob/3050dc4cd071b64cbd58e450ffaf6abf8ba46fc3/internal/sddstatus/runtime_ledger.go#L26-L56), [L436–493](https://github.com/Gentleman-Programming/gentle-ai/blob/3050dc4cd071b64cbd58e450ffaf6abf8ba46fc3/internal/sddstatus/runtime_ledger.go#L436-L493) |
| A historical intended-untracked selection is reconciled against the current index before it is replayed **into a present candidate capture**; fresh caller-supplied selections remain strict. Relevant callers cover settlement, handoff, reset/readiness probes, reset, and rescope. | Direct source read | [AI helper `runtime_ledger.go` L3607–3625](https://github.com/Gentleman-Programming/gentle-ai/blob/3050dc4cd071b64cbd58e450ffaf6abf8ba46fc3/internal/sddstatus/runtime_ledger.go#L3607-L3625); callers: [settlement L1221–1230](https://github.com/Gentleman-Programming/gentle-ai/blob/3050dc4cd071b64cbd58e450ffaf6abf8ba46fc3/internal/sddstatus/runtime_ledger.go#L1221-L1230), [handoff L1451–1458](https://github.com/Gentleman-Programming/gentle-ai/blob/3050dc4cd071b64cbd58e450ffaf6abf8ba46fc3/internal/sddstatus/runtime_ledger.go#L1451-L1458), [readiness/reset probes L1712–1744](https://github.com/Gentleman-Programming/gentle-ai/blob/3050dc4cd071b64cbd58e450ffaf6abf8ba46fc3/internal/sddstatus/runtime_ledger.go#L1712-L1744), [reset L1828–1834](https://github.com/Gentleman-Programming/gentle-ai/blob/3050dc4cd071b64cbd58e450ffaf6abf8ba46fc3/internal/sddstatus/runtime_ledger.go#L1828-L1834), [rescope/supersede L1949–1955](https://github.com/Gentleman-Programming/gentle-ai/blob/3050dc4cd071b64cbd58e450ffaf6abf8ba46fc3/internal/sddstatus/runtime_ledger.go#L1949-L1955) |
| Pi accepts only `apply`, `verify`, `archive` and requires exactly three dependency/instruction keys. | Direct source read; static incompatibility | [Pi `lib/native-review-cli.ts` L1379–1398](https://github.com/Gentleman-Programming/gentle-pi/blob/564ae19b70bdf32f8f2e9865ed068742a1908834/lib/native-review-cli.ts#L1379-L1398) |

**Conclusion supported by source:** current AI main and Pi main have a static native-status contract mismatch. This is **not** evidence that a released/packaged Pi runtime has executed and failed against this AI binary.

## Runtime authority findings retained

- The runtime ledger is the canonical immutable common-directory CAS authority. `RuntimeStatus`/SDD `Status` are projections, not independent stores.
- Preserve candidate, worktree, evidence, consent, and CAS bindings; preserve bounded execution. A zero-diff settlement is not free execution.
- The current runtime public surface has 11 verbs and 9 persisted operations, including `supersede`; this is an audited current inventory, not a required permanent public taxonomy. Plans must not use pre-supersede anchors or imply that rescope/reset are the only recovery predicates.
- Shared readiness and capped productive/harness-invalidated budget refunds already exist. Do not introduce parallel readiness/accounting.
- The current burn removes a **non-authoritative effect-marker path**; do not claim that all filesystem auxiliaries are gone.
- Native remediation exists in AI. Pi’s refusal pending a typed remediation executor is intentional; `remediate` must never be silently routed as ordinary `apply`.

## Issue and PR disposition matrix

| Evidence item | Classification | Durable disposition for this change | Basis / limit |
|---|---|---|---|
| AI #4484 | Canonical tracker | Improved by this proposed design; not approved, not closed | User named it canonical; current label is needs-review. |
| AI #4470 | Reported stale predecessor intended-untracked → tracked → successor acquire refusal | Improved/not closed | Fresh stable2.7 report; not reproduced locally. The helper protects historical selections when replayed into present captures at its callers, but does not prove that every successor-acquire path or the reported class is fixed; retain a named regression scenario. |
| AI #4416 | Resolver/count mismatch report | Improved/not closed | Report, no current reproduction in this phase. |
| AI #4413 | Legacy replay report | Improved/not closed | Report; parent found no current reproduction. |
| AI #4436 | Excluded-scope accounting report | Improved/not closed | Report; preserve accounting until a reproduction identifies a remaining root. |
| AI #4415 | Remediation unsatisfiable failure | Resolved separately; not a general settlement rule | Closed by [AI #4454 commit `e200f7d04af3cdc29c2ca39e80b6d460252ddc69`](https://github.com/Gentleman-Programming/gentle-ai/commit/e200f7d04af3cdc29c2ca39e80b6d460252ddc69): fail-fast before Begin/token. It does not prove general external-artifact settlement. |
| AI #4075 | Prose recursion report | Unconfirmed / not a closure basis | Parent audit did not confirm it. |
| AI #3244 | Dual-root request | New capability, excluded | Capability request, not automatically a defect. |
| AI #4133, #3803, #1742 | Evidence provenance concerns | Design input, not waivers | Require real-subject evidence design; do not create exemptions. |
| AI #3464, #4392, #2988, #2878, #3832 | In-flight related work: shared identity, retry, remediation evidence, slice proofs, applicability | Dependencies/context only | Parent read metadata; no PR diff/test auto-adoption or closure. |
| AI #2238 | Broad signed-context design | New capability, excluded | Not approved; narrow reuse of existing resolver/identity handling is preferred. |
| Pi #593 | Child Engram capability provisioning | Related managed-uptake blocker | Public issue body reports creation `2026-09-04`; depends on #605. It is distinct from status schema but can prevent delivery. |
| Pi #605 | Parent extension selection preservation | In-flight prerequisite | Description/events only; no diff/tests validated. Do not assume shipped. |
| Pi #608 | Asset ownership | Managed-uptake prerequisite | Can prevent consumer adoption; no diff/tests validated. |
| Pi #877 | Background policy discrepancy | Related but separate | Do not merge into #593/#605/#608 scope without evidence. |
| Pi #427, #632 | Related Pi work | Context only | Descriptions/events only; no diff/tests validated. |
| RDD #3587 → merged PR #3627 | Historical evidence pattern | Retain narrow provenance lesson | Parent verified GitHub merge metadata and ancestry at [`56d50aa391231d2ceb7abebd4442a5760b996870`](https://github.com/Gentleman-Programming/gentle-ai/commit/56d50aa391231d2ceb7abebd4442a5760b996870). RDD does not become delivery/archive authority. |
| RDD #3797 → merged PR #3851 | Historical evidence pattern | Retain narrow provenance lesson | Parent verified merge metadata and ancestry at [`14c74229fe1a832c5a709436b7d8c91fdeed721c`](https://github.com/Gentleman-Programming/gentle-ai/commit/14c74229fe1a832c5a709436b7d8c91fdeed721c). |

## Future implementation acceptance scenarios

These are reproducible acceptance targets for later strict-TDD work, not results claimed now.

1. **Status is inspectable without launch authority.**
   - Given a selected, declared OpenSpec or Engram change and an execution/delivery/review preflight condition that would refuse launch,
   - When native `sdd-status --json --instructions` resolves the change,
   - Then it MUST return status and intrinsic blocked reasons/action context without requiring permission to acquire, deliver, review, or archive.

2. **Native contract has one governed shape.**
   - Given AI emits the supported v2 status/action contract,
   - When Pi decodes it,
   - Then Pi MUST accept every documented dependency/instruction/action field, reject unknown or malformed values, and MUST NOT require a hard-coded three-key object.

3. **Consumer does not infer the route from prose.**
   - Given a native status recommendation and presentation instructions,
   - When Pi decides what to execute or display as next,
   - Then it MUST use the typed native-selected action/state and MUST NOT parse prose or recompute ledger readiness.

4. **Remediation remains distinct.**
   - Given native status selects remediation bound to failed evidence,
   - When Pi lacks a typed remediation executor,
   - Then it MUST return an explicit unsupported-capability result and MUST NOT execute apply; it MUST NOT invent a runnable upgrade/native continuation unless that continuation is verified during implementation.
   - Given Pi supports typed remediation,
   - When it executes remediation,
   - Then it MUST preserve the failed evidence revision binding and require fresh independent verification before archive.

5. **Settlement remains finite and honest.**
   - Given an admitted attempt,
   - When it passes, fails, or is interrupted,
   - Then it MUST settle once with the applicable real evidence/diagnosis/process/cleanup facts; a zero-diff outcome MUST NOT be treated as automatically free execution, while existing legitimate capped productive/harness-invalidated refunds remain preserved.

6. **Freshness is bounded, not permanently cached.**
   - Given intended-untracked paths recorded for a prior attempt become tracked by normal landing,
   - When a replay-only status/recovery capture occurs,
   - Then the runtime MUST demonstrate the intended reconciliation for that caller without falsely refusing it; this scenario is required to determine whether the reported #4470 successor-acquire gap is covered, not evidence that the whole class is already fixed.
   - When a fresh caller-supplied selection or later candidate drift is evaluated,
   - Then it MUST remain strictly validated and MUST NOT inherit a permanent identity exemption.

7. **Recovery is native-selected and auditable.**
   - Given terminal failure/interruption, candidate drift, or changed successor scope,
   - When status/recovery is requested,
   - Then the native runtime MUST select/refuse an authorized successor or recovery outcome according to preserved provenance, consent, freshness, and no-budget-laundering predicates. Implementation MAY consolidate public route names only if it preserves those predicates; Pi MUST NOT substitute an authority decision or create an unaudited reset.

8. **Managed Pi uptake is real.**
   - Given a child SDD session with extension selection pinned,
   - When a declared memory/native capability is needed,
   - Then packaged Pi assets/provisioning MUST either supply it or fail before phase execution with an actionable capability mismatch. Unit decoder tests alone are insufficient.

## Future verification evidence required

Later implementation must record exact commands/results, but this exploration makes no execution claim. Minimum evidence should include:

- AI focused red/green tests for each scenario and repository-wide `go test ./...` plus `go vet ./...` per `openspec/config.yaml`.
- Pi decoder/executor tests plus an asset/provisioning or packaged/native boundary scenario proving the selected contract reaches a child session.
- A cross-repository fixture pinned to the contract schema/version and phase/action set.
- A fresh reproduction or current-main re-verification before any report is closed; each closure must name the proving test and commit.
- Grep inventory of retired native-dispatch and phase-route strings across shipped assets, help/refusals, tests, and consumer code before deletion.

## Follow-up evidence gaps

Before proposal, optionally refresh only: full #4484 public evidence; Pi #605/#608/#877 current state, last events, merged commit metadata, and their diff/test/asset ownership proof. This bounded refresh resolves rollout facts without restarting the backlog audit.
