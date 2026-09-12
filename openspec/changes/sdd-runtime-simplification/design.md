# Design: one native SDD decision, existing execution mechanisms

## Decision and scope

Fix the existing producer, consumer, and shipped guidance together. Keep status v2,
`nextRecommended`, compact acquire/settle, and the immutable common-directory ledger.
Pi transports the native-selected action; it does not calculate the next phase.
Research tool admission and artifact-write authorization remain separate responsibilities.
No engine, ledger, action-envelope version, generator, compatibility framework, or source reuse.

Planning basis: accepted proposal and NR/OA/RS delta specs, preproposal admission r8,
and `evidence.md`. AI baseline supplied as `3050dc4c`; read-only Pi checkout supplied
as `564ae19b`. Shell/CodeGraph tools were unavailable; bounded exact-source reads were
used instead. Revisions were not independently checked, and no tests ran in this phase.
Official installed Pi README, complete `docs/extensions.md`, and the permission-gate
example were read for tool inventory/allowlists and existing event behavior; no new SDK,
RPC protocol, extension infrastructure, or TUI component is proposed.

## Current approval and delivery policy

The current human decision supersedes earlier hard-400, integrator-only exception,
feature-branch-chain, and pre-implementation design-review stops. Keep seven internal
functional units (AI 3, Pi 4): strict-TDD implementation, independent verification, then
advancement for each unit. Source, fixture migrations, tests, and shipped guidance remain
cohesive. The 400 authored A+D figure is a target; justified overruns are allowed.
Exactly one final PR in AI and one in Pi are intended, both `size:exception`, with full
planning documents counted wherever included in those diffs. Delivery strategy is
`exception-ok`; the supported `Chain strategy: size-exception` value means no PR chain.
No immediate delivery, branch creation, commits, PR/label changes, merge, or release is
authorized. Native finite budgets, history, and consent remain unchanged; parent-owned
reconciliation/reset is pending before execution, not automatic because policy changed.
Historical planning provenance above remains historical, not new implementation evidence.

## Ownership and deletion plan

Paths prefixed **Pi** are relative to `/home/gentleman/work/gentle-pi-worktrees/main-live`.
Other source paths are relative to this planning repository. Rows name existing files;
new tests belong beside the named behavior, not in a new testing framework.

| Owner / existing files and symbols | Necessary change | Delete or preserve |
|---|---|---|
| AI `internal/cli/sdd_status.go:21–81`: `RunSDDStatus`, `RunSDDContinue`; `internal/sddstatus/status.go`: `Resolve`, `resolveByPreferenceOrder`, `applyReviewOfferRouting` | Keep `Resolve`/status pure; explicitly authorized continue prepares consent before rendering. Review-mode lookup failure omits only the advisory offer. | Remove review/preflight prerequisites for inspection; preserve execution authorization and genuine resolution errors. |
| AI `internal/sddstatus/status.go:630–717`; `internal/sddstatus/edit_authority_consent.go`: `readChangeInstanceMarker`, `ensureChangeInstanceMarker`, `newEditAuthorityConsent`; `internal/cli/sdd_attempt.go:258–277` | Move existing random-marker preparation into the existing continue entry point; use no-replace publication and readback. Grant must compare the supplied identity with the current persisted marker, never initialize it. | Remove status-time minting and overwrite races; preserve existing markers, consent envelope, instance binding, CAS and records. |
| AI `internal/sddstatus/status_v2.go`: `ProjectStatusV2`, `statusV2NextRecommended` | Keep the wire shape; extend producer contract tests to cover every emitted action and optional section. | Preserve seven dependencies, four `phaseInstructions` groups, bounded action vocabulary; no new envelope. |
| Pi `lib/native-review-cli.ts`: `NativeSddStatusV2`, `decodeNativeSddStatusV2`, `sddStatus`; `extensions/gentle-ai.ts:7646–7687`: `handleSddStatusCommand`, `handleSddContinueCommand` | Decode actual v2 fields; keep `sddStatus` read-only. Add a narrow `sddContinue` adapter call only for authorized continuation, using the same v2 decoder and mutating process classification. | Delete exact-three/`instructions` assumptions and review-negotiation gating of inspection; never use continue as status fallback. |
| Pi `lib/sdd-status.ts`: `resolveSddStatus`, `planningRecommendation`, `renderNativeSddPhasePrompt`, dispatcher/status renderers; `extensions/gentle-ai.ts`: selected startup and status/continue handlers | Route live lifecycle reads through native v2; render its fields without casting a native document to the local status type. | Remove live local dependency reconstruction, `resolve-via-engram`, prefixed-token inference, and automatic local fallback. Retain unrelated presentation helpers only where still used. |
| Pi `extensions/gentle-agents.ts`: `parseSddChange`, `buildRequest`; `lib/agents-runner.ts`: `SddChangeSelection`, `TaskRequest`, `childArguments` | Carry selected planning/execution action, bounded scope, and remediation binding through the existing launch path. | Replace duplicated phase acceptance lists with one consumer vocabulary; never use that vocabulary as a phase graph. |
| Pi `lib/sdd-research-capabilities.ts`: `resolveResearchCapabilities`, `researchAgent`; `extensions/gentle-agents.ts`: child restriction hook | Restrict research routes to selected admitted classes; retain separately authorized local/persistence tools and recheck actual child inventory. | Preserve host intersection; delete whole-phase denial caused solely by a missing research route. |
| AI `internal/agents/researchcapability/contract.go`: `ForAgent`, `Admit` | Preserve closed declaration/admission policy and verify agreement with host fixtures. | No Go host-inventory engine or new research command. |
| AI `internal/sddstatus/runtime_compact.go`, `runtime_ledger.go`, `verification.go` | Reuse readiness, replay, evidence derivation, settlement and remediation predicates; change only demonstrated missing plumbing. | Preserve authority operations, lifetime accounting, refund caps, failed-evidence linkage, and write/replay agreement. |
| AI `internal/assets/opencode/commands/sdd-status.md`, `sdd-continue.md`, `internal/assets/skills/_shared/sdd-status-contract.md`; Pi `assets/agents/sdd-apply.md`, `sdd-research.md`, `lib/sdd-preflight.ts` | Align shipped text and managed ownership with the corrected consumer. Add the distinct remediation agent asset using existing SDD asset installation. | Remove native-Engram bans, manual native-shaped status fabrication, prose dependency graph, and status-preflight gate. Preserve planning artifacts and quality guidance. |

No package deletion is prescribed before caller verification. During tasks, inventory
retired strings across installed-source assets, help, refusals, and tests; do not leave
an executable-looking continuation pointing at a deleted route. Canonical native v2
already omits `sdd-sync`; this design does not add a sync-removal product decision.
Preserve explicitly requested sync, its artifacts, and durability; do not invent a
native sync recommendation or include a sync migration in this change.

## Contract and read-only status

1. Keep `schemaName: gentle-ai.sdd-status`, `schemaVersion: 2` and the existing
   `--contract gentle-ai.sdd-status/v2` floor. Do not accept a live legacy schema.
2. Dependencies are exactly `proposal/specs/design/tasks/apply/verify/archive`.
   Optional `phaseInstructions` contains exactly `apply/verify/remediate/archive`.
   Pi requests instructions and requires them on that execution handoff; ordinary
   read-only decoding accepts their documented absence. Never accept `instructions` as an alias.
3. Bound `nextRecommended` to the producer's existing twelve tokens, including
   `archived`, `sdd-new`, `select-change`, and `resolve-blockers`. These four do not
   execute a phase. Planning tokens need no invented instruction-group keys.
4. Validate fields consumed for routing and writes: store, nullable selection/locators,
   action mode, canonical workspace, allowed roots, and remediation revision. Discovery
   may have null selection; selected child startup must match its requested identity.
   Unknown/malformed actions refuse before execution, even if accompanying prose says “ready”.
5. Consumers display dependencies and reasons but do not recompute readiness. A native
   `verify` selected to refresh evidence remains executable despite diagnostic reasons.
   Status itself launches nothing, including when it recommends a planning phase.
6. `actionContext` describes existing authority, not new consent. Effective writes are
   the intersection of its roots, the selected phase's artifact/source targets, and
   the current human authorization. Carry narrower path grants in existing launch data;
   a root-wide native context must not erase a design-file-only human scope.

### Consent preparation belongs to explicit continuation

**Decision:** assign preparation to existing `RunSDDContinue`, not grant. Source
`internal/cli/sdd_status.go:51–81` currently calls the same `Resolve` as status and has
no separate initialization boundary. This is a proposed behavior change to explicit
continuation intent, not a claim that a safe initializer already exists. The parent's
focused CodeGraph readback found only `resolveByPreferenceOrder` calling the current
initializer. Nothing in the inspected CLI requires continue to remain a pure status alias.

- **Pure output:** remove `ensureChangeInstanceMarker` from `Resolve`. With missing edit
  roots and no marker, omit optional `consent`; retain existing dependency/action blocking
  and append a preparation explanation to `blockedReasons`, naming the existing invocation
  `gentle-ai sdd-continue <selected-change> --cwd <canonical-workspace>` with proper quoting.
  Say it requires authorized change-directory writes and grants no roots. No new reason
  enum, action token, public field, grant invocation, or transient usable identity.
  With a valid present marker, render the existing bound envelope normally.
- **Preparation:** add a small package helper called only by `RunSDDContinue` between
  initial resolution and output. Carry missing-root facts internally from the resolver,
  not by parsing diagnostics. For a selected active OpenSpec-backed change needing consent,
  verify its current directory/workspace and already-authorized planning-directory marker
  writes, then persist or reuse the existing random marker. Check resolved planning-home
  containment; do not require the still-missing source edit roots to be granted first. Re-resolve and render through `ProjectStatusV2`/
  `RenderDispatcherMarkdown`; no grant, attempt, artifact rewrite, or automatic phase launch.
  Engram-only status never acquires an invented filesystem marker.
- **Authorization boundary:** the CLI invocation expresses explicit continuation intent,
  not a read-only query. Hosts must check current narrower human scope includes marker
  preparation before calling it; native `actionContext` alone cannot grant that scope.
  Planning-directory write scope and source edit authority are distinct: a narrow grant
  covering the marker permits preparation even while source roots remain unauthorized and
  apply stays blocked. Only read-only scope or scope excluding the marker forbids preparation;
  native unsafe selection, out-of-planning-root context, or unwritable directory refuses before publication. `--json` changes
  formatting, not authorization. There is no new approval flag or permission protocol.
- **Publication:** replace the initializer's current `os.WriteFile` overwrite with the
  existing no-replace publication convention (`reviewtransaction.PublishFileNoReplace`).
  Publish complete random-token bytes, then read the persisted winner; concurrent losers
  reuse it, never their unpersisted token. Preserve present markers byte-for-byte; malformed
  or unreadable markers refuse rather than regenerate. Confirm the selected directory has
  not been replaced during preparation before emitting consent. Publication/readback failure
  emits no usable consent; retry reads the marker rather than minting over an uncertain write.
- **Grant remains a consumer:** `sdd_attempt.go:258–277` passes the supplied token to
  `ForInstance`; `runtime_ledger.go:665–678` validates opaque text, not the current marker.
  Therefore the CLI's existing grant path must read and compare the selected persisted
  marker before grant/replay. Absent or mismatched identity refuses without creating a
  marker or appending authority. Keep ledger CAS/idempotency and instance-filtered projection.
  A grant for A cannot initialize recreated B from A's supplied token. This narrow binding
  check is necessary for the stale-invocation negative test, not a new grant mechanism.

**Boundary traceability clarification:** native tasks 1.2–1.4 prove explicit preparation,
planning-versus-source authority, selection/containment, and filesystem failures. True
human read-only or excluded-marker scope suppresses the mutating adapter at the host,
owned by Pi tasks 2.2–2.3/5.1. A directory at the marker filename is not permission proof.
Ambiguous/unsafe selection, out-of-root context, and unwritable-at-entry directories
publish nothing. Concurrent replacement detected during preparation requires no usable
consent, not universal zero-write atomicity across a pathname race. Shared publisher
fallback can expose partial destination bytes: this remains a limitation/proof gap;
no-replace alone proves neither universal atomic visibility nor all-filesystem convergence.
Keep fail-closed readback and winner-convergence proofs; no shared publisher redesign or
new permission primitive is authorized. Delta-spec quality, scope, and consent guarantees
remain unchanged.

**Pi call sites:** `handleSddStatusCommand` and `resolveSelectedNativeSddChangeStartup`
use only `sddStatus`; startup/status rendering never prepares consent. Only
`handleSddContinueCommand` (or the existing expressly authorized continuation path)
uses the new adapter method after scope checks. Both handlers currently call
`resolveControllerSddStatus` (`extensions/gentle-ai.ts:7646–7687`); replace those calls
with their distinct native operations, not one fallback chain. Display preparation
instructions losslessly but never execute them merely because status returned them.

This resolves the former owner gap at design level. Preparation concurrency, stale
binding, and host scope require the focused proofs below before implementation acceptance;
no source/runtime success or implementation authorization is asserted here.

## Handoff sequence

```text
Parent -> native status: selected change + canonical cwd, read-only v2
Native -> parent/Pi: one nextRecommended + store/locators/actionContext
Authorized explicit continue -> native continue: prepare missing instance, then return bound consent
Parent -> human: existing consent, if required; grant only on explicit answer, then reread status
Parent -> existing managed launch: selected actor + narrower authorization + intent
Child -> native status / local inventory: confirm current identity and actual tools
Runtime owner -> acquire: existing work unit, budgets, request ID, remediation revision
Native -> runtime owner: proceed(token) | blocked | complete
Child -> authorized tools: perform selected work; persist actual evidence
Runtime owner -> settle: same token, distinct settle ID, truthful terminal facts
Parent -> native status: fresh projection; independent verify if selected; archive only eligible
```

Planning does not acquire merely because apply dependencies are blocked. Existing
runtime-bearing work remains bracketed. When the parent already acquired, the child
continues with its token, not a second blind acquire. Use existing runner finalization
and retained task/session results for transport recovery; they are not workflow authority.

For managed runtime-bearing children, `extensions/gentle-agents.ts` owns the bracket:
add narrow acquire/settle methods to the existing `NativeReviewCli` adapter using its
exec-file transport and existing compact JSON states. Retain operation IDs, token and
exact settle inputs in existing task/session history, not a second attempt ledger.
Reuse `AgentRunner`'s `onFinish` for failed/interrupted completion; only an admitted token
can settle. A spawn failure after acquire settles interrupted with observed process/
cleanup facts. A completed actor supplies evidence, not automatic successful acceptance.
New adapter methods are necessary because the inspected Pi adapter has status but no
compact-attempt methods; a new agent prompt alone cannot own interruption settlement.

## Research routes and persistence

`ForAgent`/`Admit` own the native closed maximum (`gentle-ai.sdd-research-capability/v1`).
Pi owns registered ∩ active ∩ approved ∩ explicit-restriction ∩ selected-class reachability.
Thread selected classes and their admitted per-class grants into `researchAgent` and
`buildRequest`; a plain union of all available classes is not the selected admission.
Do not move host discovery into Go or infer a grant from a generic gateway.

- **Pi-specific grants:** documentation requires exactly `fetch_content`; open-web
  requires exactly `web_search`, `source_check`, `fetch_content`, `get_search_content`.
  Other supported runtimes retain their own exact `ForAgent` declarations, not Pi names.
- Preserve separately authorized `read/grep/find/edit/write` and exact Engram read/save
  tools from the agent definition. They are not research grants. Parent messaging remains
  existing transport, not a research route; align its effective child allowlist too.
- `childArguments` already emits `--tools`; that allowlist does not load an extension.
  Carry the parent's actual resolved extension selection through existing runner launch
  inputs, then verify child-local registration. Do not enable undisclosed extensions,
  auto-install providers, grant project trust, or substitute another fetch tool.
- Keep the selected store and existing change-local research/preproposal locators.
  OpenSpec reads/writes only OpenSpec; Engram uses its exact project/topic locators;
  hybrid retains positive revision and byte-equal readback of both stores; none cannot
  set readiness. Source data never changes scope or tool authorization.
- Missing selected tools stop collection and proposal, **not already-authorized diagnostic
  persistence**. Retain intent, observed grants, denial/partial evidence, and failed calls.
  If persistence itself fails, return the retained evidence and write failure without
  claiming durability or silently switching stores.
- Re-enter with fresh inventory and recovered artifact facts after correction. Do not
  cache a previous denial as permanent authority. Hybrid divergence remains blocked until
  existing retained-intent recovery writes and reads back a new matching revision.

Why more than a wording fix: `researchAgent` currently considers both classes, while
`childArguments` cannot make an absent extension's tools callable. Conversely, filtering
the whole child to research grants would remove the writes needed to record denial.
The necessary additions are launch-data plumbing and selected filtering, not a platform.

## Remediation and real evidence

Add one typed `remediate` actor in the existing managed-agent mechanism, not an apply
alias: proposed new Pi asset `assets/agents/sdd-remediate.md`, registered in existing
`ASSET_OWNER_BY_KEY`. Extend existing selection validation and asset ownership. Carry
`remediationState.failedEvidenceRevision` unchanged to acquire and settle. Unsupported
consumers refuse explicitly; no guessed upgrade command or unverified native shortcut.

Keep `Acquire`'s existing chain and candidate admission. An unchanged failed baseline
without its existing audited evidence-only authorization remains unsatisfiable; the
actor must not edit before admission to trick that check. Native recovery predicates
and explicit maintainer decisions supply the existing correction/retry route. Pi does
not pick reset/rescope/supersede. This route must be exercised from the original failed
baseline, not tested only after a fixture has conveniently changed the candidate.

For a passing correction, use existing `--remediation-evidence` and
`DeriveRemediationEvidenceRevision` (`verification.go:718–760`) to derive identity from
concrete focused-test, harness or justified N/A, and rollback evidence. Then native
`nativeRuntimeCompletesRemediation` selects fresh independent verification before
successful acceptance/archive. For failure, supply its actual evidence revision and
required diagnosis/process/cleanup facts; for interruption omit evidence revision.
Neither path requires successful independent verification or a passing evidence envelope.

| Actual subject | Existing evidence path; no fabricated source edit |
|---|---|
| Source candidate | Existing snapshot builder captures tracked/index changes and declared untracked selection; ledger binds candidate/worktree and measured lines. |
| OpenSpec or Engram artifact | Retain exact artifact locator, store/project, revision and content digest in persisted phase evidence; bind its actual evidence revision in settlement. A Git baseline is execution context, not proof that external artifact bytes changed. |
| Invocation/environment | Retain command, cwd, result/output digest, diagnosis, process and cleanup observations in phase evidence and existing settlement fields. A process exit alone is not verified acceptance. |

No new subject union or receipt store is needed for these scenarios: locator/content and
invocation facts fit existing evidence artifacts, and `EvidenceRevision` binds them.
No-diff does not imply no work or a refund. Evidence-only successful remediation still
requires existing audited authorization; artifact or environment labels are not waivers.

## Finite failure and recovery matrix

| Boundary / outcome | Required behavior |
|---|---|
| Invalid status, wrong worktree, absent executor | Refuse before work; no substitute phase or new attempt. |
| Missing research tool | Persist authorized denial/partial record; research/proposal blocked; no fallback route. |
| Out-of-scope write or divergent/stale recovery | Refuse that write; retain evidence/intent; no broadened roots or preferred hybrid copy. |
| Admitted failed/interrupted/no-diff execution | Settle once with available truthful required facts; failure/interruption does not discharge failed evidence. Preserve finite attempt charges and legitimate capped refunds. |
| Lost reply after committed settle | Reuse exact settle ID/token/payload through `Settle` request-receipt replay; no actor rerun, new ordinal, or duplicate charge. Returned projection may reflect a later ledger head. |
| Effect possible before settlement | Reconcile retained tool/process/artifact observations; unknown remains unknown. Do not fabricate success or claim a committed record; unsafe re-execution refuses. |
| Required settlement facts unavailable | Retain token and uncertainty, refuse further execution until existing reconciliation/settlement is possible; no automatic retry/reset or invented cleanup proof. |
| Historical intended-untracked path landed | Apply `runtimeReplayedIntendedUntracked` only at historical-to-current capture boundaries; keep fresh selection and later drift strict. |
| Corrected blocking fact | Reload native facts and existing readiness/recovery predicates; do not retain a consumer blocker cache or rewrite ledger history. |

Preserve `Finish`/`applyRuntimeFinishEvent` outcome parity, `runtimeReadiness`,
`runtimeAttemptRefundsBudget`, and receipt-before-mutation replay. For the reported
successor-acquire case, trace CLI preflight → `runtimeRescopeSuccessorRequest` → capture;
add reconciliation only where inherited history actually reaches a fresh capture.
A new recovery taxonomy or ledger rewrite would not improve that missing connection.

## Acceptance and boundary verification

| Spec IDs | Planned proof (new RED where defective; preservation where already correct) |
|---|---|
| NR-01; OA-04 | Pure OpenSpec/Engram status without session/launch/review authorization; no marker/authority/artifact mutation. Exercise the focused status/continue/first-consent cases below, including old invocation refusal after recreation. |
| NR-02–04; OA-03 | Feed actual `ProjectStatusV2` output to Pi, not hand-authored three-key mocks: all actions, optional instructions, nullable discovery, wrong identity, missing/extra keys, malformed action, misleading prose. Assert selected actor or refusal and no local resolver fallback. |
| NR-05–06; RS-01–04,09–10 | Real managed child with fixed extension selection: exact selected grants, missing each tool, SDK-only/inactive tools, narrowed write/read scope, wrong worktree; denial still persists through authorized OpenSpec/Engram tools. |
| NR-07–08 | Corrected inventory/artifact facts continue; stale hybrid facts refuse. Historical untracked→tracked successor-acquire and fresh tracked selection have opposite outcomes; subsequent content drift remains visible. |
| NR-09–10a | Original failed baseline → native admitted correction/recovery → distinct actor → bound settlement → independent verifier; stale/discharged revision and unsupported actor refuse. Failed/interrupted remediation closes without a verifier-success prerequisite. |
| NR-11–12a | Real artifact and invocation evidence with zero source diff; failure/interruption accounting, capped refunds, lost committed reply, uncertain pre-settlement effect, missing worktree, and exact conflicting replay. Assert record count and lifetime charges. |
| NR-13 | RED/GREEN/TRIANGULATE/REFACTOR evidence plus independent verifier; implementer completion, runner completion, or exit zero alone never accepts/archive-enables. |
| NR-14–15 | Installed assets + package-local native binary + actual managed child prove status uptake, callable research/persistence tools and remediation support. Deliberately old/missing assets must fail before work, not only in a decoder unit test. |
| OA-01–02; RS-05–08 | Preserve confirmed no-interview proposal, grouped unresolved-choice prompt, matching hybrid recovery, one-sided write recovery, and missing-intent refusal. |

Focused consent proofs (future RED tests at the existing native CLI and Pi handler boundaries):
1. Repeated status, marker absent/present: byte-identical filesystem/authority snapshots;
   absent marker yields preparation diagnostic and no consent/grant token, present marker
   yields the existing bound envelope without mutation.
2. Authorized continue persists one random marker before emitting bound consent; repeated
   and concurrent preparation converge on the same bytes; no grant or attempt is recorded.
3. Planning-directory-only scope permits marker preparation and bound consent emission while
   missing source roots keep apply blocked. Read-only scope or scope excluding the marker calls
   no mutating adapter; ambiguous selection, mismatched workspace, out-of-planning-root context
   and unwritable directory publish nothing.
   Pure status and selected child startup never invoke continue, including binary failure.
4. Obtain consent for A, delete/recreate the path as B, run A's old grant invocation:
   refusal, no B marker initialization, no authority append or old-root projection.
   After B's authorized preparation, A still refuses; B has a distinct random identity.
5. Present malformed marker, publication failure, and replacement during preparation:
   no overwrite or usable consent; exact valid grant replay preserves existing CAS/accounting.

Extend existing `status_v2_clean_break_test.go`, `runtime_truthful_remediation_settle_test.go`,
`runtime_compact_test.go`, `verification_test.go`, `internal/components/sdd/research_state_contract_test.go`,
and Pi `tests/sdd-research-capabilities.test.ts`, `tests/gentle-agents.test.ts`,
`tests/sdd-research-live.test.ts` with the mapped cases. Producer-emitted fixtures are
contract data, not copied implementation or a code generator. Unit mocks alone are insufficient.
Later verification runs focused tests, `go test ./...`, `go vet ./...`, and the Pi repository's
actual declared test commands; record commands/results and independently inspect artifacts.

### Threat applicability (carry applicable rows into tasks)

| Boundary | Applicability / cases | Safe behavior and RED boundary |
|---|---|---|
| Documentation-like paths | N/A: no executable-file classifier change | Never introduce an extension-based evidence exemption. |
| Git repository selection | Applicable: `git -C`, relative paths, absolute paths | Canonical selected workspace wins; mismatched child/capture refuses. Test each selector at native CLI and runner handoff. |
| Commit state | Applicable: staged, `commit -a`, empty index | Historical landing reconciles without erasing later drift; fresh invalid selections refuse. Test each state across successor acquire and settle. |
| Push state | N/A: no push automation or destination change | Delivery remains unauthorized here. |
| PR commands | N/A: no PR command composition | No implementation task for this boundary. |

## Minimal cutover, rollback, and remaining proofs

1. Pair pure status with authorized-continue preparation, no-replace marker reuse, current-marker
   grant validation, and their focused proofs in one cohesive behavior slice. Update shipped
   status/continue guidance and Pi call-site separation with uptake; never ship a preparation
   diagnostic without its supported continuation. Plan remaining v2 and managed execution
   work as cohesive behavior units with tests; independent verification precedes advancement
   to each next unit under the current approval above.
2. Prove a tuple of **actual AI build identity + Pi build identity + installed asset
   manifest/content + existing contract floor**. Existing `NATIVE_CLI_CONTRACTS` review
   rows do not prove SDD compatibility. Do not invent release numbers or assume Pi
   #605/#608 shipped; test `childArguments` and `installPackageAssets` uptake directly.
3. Cut over live status, selected startup, and status/continue rendering together to v2.
   No live local/native dual-schema routing. Historical ledger records and persisted
   preference normalization remain readable; they do not authorize legacy live dispatch.
4. Roll back consumer, producer, and managed assets only to a tuple proven able to read
   retained records. Never delete history, reset budgets, rewrite markers, or regain consent
   through rollback. If no safe tuple exists, refuse execution and retain read-only diagnosis.

No remaining initialization-owner design gap: the proposed owner is existing explicit
`RunSDDContinue`, with the separation and binding changes above. Verification still must
prove those changes, the reported successor-acquire case, deployed extension-selection/
asset uptake, and real-subject acceptance boundaries. These are proofs-to-run, not results.
Missing tests authorize no new mechanism or another issue audit.

Tasks forecast authored additions plus deletions against the 400-line target, including
cohesive tests, migrations, and guidance; justified overruns are permitted, not a new hard
cap. Measure each eventual final PR including full planning documents wherever present.
Use the current two-final-PR `exception-ok` policy above, with no child PRs or chain bases.
Only the planning correction is authorized now; parent-owned native reconciliation/reset
and the next bounded launch remain separate from this document. No delivery is authorized.
