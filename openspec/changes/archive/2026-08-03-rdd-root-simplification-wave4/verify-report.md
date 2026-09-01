```yaml
schema: gentle-ai.verify-result/v1
evidence_revision: sha256:b49e476132e10fa867a0152484b0663b0b8a97232456e3864f50def0d4402eb5
verdict: fail
blockers: 3
critical_findings: 3
requirements: 4/16
scenarios: 16/30
test_command: go test ./... -count=1
test_exit_code: 0
test_output_hash: sha256:5fc7feb902a9a840a979996f07a438da0c90d29320849b495a99ce37c82ff5ae
build_command: go build -o gentle-ai ./cmd/gentle-ai
build_exit_code: 0
build_output_hash: sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855
```

## Verification Report

**Change**: rdd-root-simplification-wave4
**Version**: N/A (delta specs, unarchived)
**Mode**: Strict TDD
**Attempt authority (echoed, not settled)**: sha256:a05ba9415c1f4d2e193232ece14cd38a2f544331272068e9a1720b42a296f691
**Candidate**: worktree `/home/gentleman/work/gentle-ai-worktrees/rdd-wave4`, tip `d039d6e34de45763a906a2b93fcc33c1a3a6b063`

### Completeness

| Metric | Value |
|--------|-------|
| Tasks total | 56 |
| Tasks complete | 53 |
| Tasks incomplete | 3 (1.2, 7.6, 7.7) |

### Chain-base correction (affects every "58 files / 814 deletions" figure)

`67be4867` is **not** an ancestor of `d039d6e3`. `git merge-base 67be4867 d039d6e3` = `157ab9fd`.
Wave 4's real authored delta is `157ab9fd..d039d6e3` = **13 commits, 55 files, 3727 insertions, 472 deletions**.
The three "D" rows in the 58-file diff (`internal/cli/review_new_lineage_frozen_tier_test.go`,
`internal/cli/review_new_lineage_kill_switch_test.go`,
`internal/reviewtransaction/new_lineage_switch_identity_test.go`) are **not wave-4 deletions** — they are
Wave 3's final commit `67be4867`, which the Wave 4 chain never included. See CRITICAL-3.

### Build & Tests Execution

**Build**: PASS

```text
$ go build -o gentle-ai ./cmd/gentle-ai
(no output)   exit 0
$ gofmt -l .
(clean)
$ go vet ./...
(clean)   exit 0
```

**Tests**: PASS (root module, 61 packages) + PASS (bench module)

```text
$ go test ./... -count=1
...
ok  	github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction	124.880s
ok  	github.com/gentleman-programming/gentle-ai/v2/internal/sddstatus	24.201s
ok  	github.com/gentleman-programming/gentle-ai/v2/internal/update/upgrade	7.115s
ok  	github.com/gentleman-programming/gentle-ai/v2/internal/verify	0.002s
ok  	github.com/gentleman-programming/gentle-ai/v2/internal/versions	0.002s
ROOT_EXIT=0

$ cd bench && go test ./... -count=1
ok  	github.com/gentleman-programming/gentle-ai/bench	0.178s
BENCH_EXIT=0
```

**Ratchets**:

```text
$ bash scripts/deadcode-ratchet.sh
no new unreachable functions
RATCHET_EXIT=0

$ go test ./internal/cli/ -run 'RefusalResolution|EveryProductionRefusal' -count=1
ok  	github.com/gentleman-programming/gentle-ai/v2/internal/cli	0.134s

$ go test ./internal/components/ -run Golden -count=1     # no -update
ok  	github.com/gentleman-programming/gentle-ai/v2/internal/components	0.564s
```

**Coverage**: not collected — no coverage threshold configured for this repo.

### Directive checks (fresh evidence, binary rebuilt from d039d6e3)

#### (a) Kill switch OFF — full SDD status resolution

Fixture: `git init` repo, one change `demo-change`, all tasks complete, admitted verify report,
`gentle-ai review mode disable --cwd <repo> --scope clone` → `off (decided by clone_local)`.

```text
$ gentle-ai sdd-status demo-change --cwd <repo>
keys present: ['reviewGate']
{
  "reviewGate": {
    "result": "invalidated",
    "reason": "receipt-driven development is disabled, so no review governs this change; it closes under ordinary repository policy rather than under a review receipt: terminal review receipt is missing; run the fresh full review of the current state with gentle-ai review start",
    "delivery": "disabled/unmanaged"
  },
  "dependencies": { ... "verify": "all_done", "archive": "ready" },
  "nextRecommended": "archive"
}
```

- `reviewOffer` absent: **YES** (correct).
- No review fields at all: **NO** — `reviewGate` with `delivery: disabled/unmanaged` is emitted, and producing
  it required a live `resolveReviewAuthority` → `discoverNativeReceipts` walk of the repository. This is the
  "disabled/unmanaged ceremony" the spec forbids. See CRITICAL-1 / FINDING W1.
- Absence guards re-run by me, all green: `TestReviewOfferAbsenceGuardHoldsForProductionFiles`,
  `TestReviewOfferAbsenceGuardHoldsForScopedCLIFiles`, `TestReviewOfferJourneyFiresZeroTimesAcrossFullFlowWhenDisabled`,
  `TestReviewOfferBlockAbsentStructurallyWhenDisabled`.

#### (b) Post-verify offer, switch ON

```text
$ gentle-ai review mode enable --scope global      # → on (decided by global)
$ gentle-ai sdd-status demo-change --cwd <repo>
keys present: ['reviewGate']
{
  "reviewGate": { "result": "invalidated",
    "reason": "terminal review receipt is missing; run the fresh full review of the current state with gentle-ai review start" },
  "nextRecommended": "resolve-review"
}
```

`reviewOffer` is **absent from the wire even with the switch on and verify all_done**. Root cause:
`internal/sddstatus/status_v1.go`'s `StatusV1Projection` has no `ReviewOffer`/`ReVerify` field, and every
CLI path (`RunSDDStatus`, `RunSDDContinue`, `RenderMarkdown`, `RenderDispatcherMarkdown`) projects through it.
See CRITICAL-2.

Genuine `Available` semantics: proven only in-package (`TestOfferReviewAfterVerifyDefaultModeWithNoReceiptIsAvailable`,
`TestOfferReviewAfterVerifyReceiptStillGoverningReportsUnavailable`, `TestOfferReviewAfterVerifyReceiptNotAllowReportsAvailable`
— all PASS). In production `Available` can only ever be `true`: the only writer of `RuntimeStatus.Receipt`,
`recordPreparedReceipt`, has no production caller and is deadcode-baselined
(`.deadcode-baseline.txt:200 internal/sddstatus/runtime_receipt.go RuntimeStore.recordPreparedReceipt`).

#### (c) Decline

`TestReviewOfferDeclineStatusByteIdenticalToOffOutsideOfferBlock` re-run: **PASS**. Read verbatim, it strips
the offer block before comparing (`declined.ReviewOffer = nil` at line 214), and compares the internal
`Status`, not the wire projection. Empirically, at the wire, a decline (switch on, no receipt) yields
`archive: blocked`, `nextRecommended: resolve-review` — **not** the switch-off `archive: ready`,
`nextRecommended: archive`. No persistent state is created by a decline (correct), but the change cannot
archive. See CRITICAL-2 / FINDING W3.

#### (d) Transport capability admission ordering

`internal/cli/review_facade.go:1490-1494` calls `authorizeReviewTransportCapability` before
`reviewRuntimeWithImmutableTransport` (:1500), `validateReviewStartBinding` (:1504),
`SnapshotBuilder.ResolveRepositoryRoot` (:1513) and `authorizeReviewStart`/consent (:1597). Ordering claim
**CONFIRMED** by source. Threat-matrix tests re-run, all PASS:

```text
--- PASS: TestAuthorizeReviewTransportCapabilityMatrix
    --- PASS: .../no_agent_identity_supplied_is_not_gated_at_all
    --- PASS: .../advertised_claim_admits
    --- PASS: .../absent_claim_fails_closed
    --- PASS: .../unrecognised_claim_fails_closed
    --- PASS: .../advertised_but_self-inconsistent_manifest_fails_closed
--- PASS: TestUnsupportedReviewTransportCapabilityStopsBeforeAnyAuthority
--- PASS: TestUnsupportedImmutableReviewTransportStopsBeforeRepositoryOrAuthority (6 subtests)
```

#### (e) SDDReceiptRef + archive-gate re-derivation

```text
--- PASS: TestBoundReviewArchiveGateIgnoresStaleGateContextViaReDerivation
--- PASS: TestResolveGoverningReceiptRefPresenceAndAbsence
--- PASS: TestResolveGoverningReceiptRefRequiresAParsedNativeBinding
--- PASS: TestBoundReviewBindsLedgerAcceptsLegacyEmptyAndRejectsForgedHash
--- PASS: TestRuntimeRemediationFinishImportsLegacyBindingInTheSameAtomicRecord
--- PASS: TestValidateSDDReceiptRefGitRepositorySelection (4 subtests)
```

`bind-sdd` and the remediation-successor CAS are untouched and green, per the decision-1 amendment.
No writer of the `sdd-review-bindings/v1/<change>/binding.json` file remains
(`runtime_ledger.go:1687 readLegacyBinding` is the only file access, read-only).

#### (f) Targeted re-verify — three branches

```text
--- PASS: TestClassifyTargetedReVerifyBranches   (7.1 empty intersection -> targeted; 7.2 not derivable -> full; 7.3 fail closed)
--- PASS: TestDeriveCorrectionEvidenceBranches
--- PASS: TestResolveOmitsReVerifyBlockWithoutAnyCorrection   (structural absence)
```

**`verifyEvidenceScope` narrowing assessment — partly faithful, partly a hole.** The narrowing
(`GenesisPaths` → `openspec/changes/<change>/` prefix, `review_reverify.go:169`) is *defensible*: the
investigation recorded in the file's doc comment is correct that `reviewtransaction` validates every
correction's paths as a subset of `GenesisPaths`, so the un-narrowed reading makes branch 7.1 unreachable.
But the resulting risk ordering is **inverted** relative to the spec's intent: a correction confined to
production source (the risky case) always lands in the *cheap* `targeted` branch, while a correction that
touches this change's own planning artifacts lands in `full`. And the spec scenario "Targeted re-verify for a
scoped correction" requires "re-verify is scoped to **exactly those changed paths**"; no branch ever emits the
correction's changed paths as `Scope` — `targeted` emits the (untouched) evidence scope and the intersecting
case emits the overlap under `Mode: full`. Faithful to the coordinator amendment's three named branches;
**not** faithful to the spec scenario's scoping clause. WARNING, not CRITICAL, because the amendment is
ratified and the cheap branch still re-runs the objective's evidence goal.

`Status.ReVerify` is never emitted on the wire (same `StatusV1Projection` cause as the offer), and never
mutates `Dependencies.Archive`, so the spec's "archive does not proceed until that full re-verify passes"
has no enforcement anywhere. Task 8.6's asset prose does not mention `reVerify` at all
(`rg -i 'reverify|re-verify' internal/assets/` returns only the two `sdd-verify.md` offer sentences),
so the coordinator amendment's point 4 ("prose instructs running sdd-verify with the block's stated scope
before archive") was not delivered.

#### (g) Decision-9 collapse

```text
--- PASS: TestRuntimeObjectiveIsSoleWorkUnitScopeOwner
$ rg -A3 'type CompactAcquireRequest struct' internal/sddstatus/runtime_compact.go
type CompactAcquireRequest struct {
	BeginAttemptRequest
}
```

Call sites: `internal/cli/sdd_attempt.go:117` (production) + `runtime_compact_test.go:18`,
`runtime_objective_advance_test.go:211,244`, `runtime_objective_owner_test.go:24`. Consistent. COMPLIANT.

#### (h) Adapter guard + pin fix

```text
--- PASS: TestAdapterForbiddenConstructionGuardCatchesKnownShapes (4 subtests)
--- PASS: TestAdapterForbiddenConstructionGuardHoldsForProductionFiles (5 production files)
$ go test ./internal/components/ -run Golden -count=1
ok  	github.com/gentleman-programming/gentle-ai/v2/internal/components	0.564s
```

Pin corrected to `gentle-ai.review-integration/v2` in both `sdd-apply.md` assets; goldens match without `-update`.

### Spec Compliance Matrix

| Requirement | Scenario | Test | Result |
|---|---|---|---|
| Offer Occurs Strictly Post-Verify, Pre-Archive | Offer fires only after verify completes | `sddstatus > TestReviewOfferBlockPresentWhenVerifiedAndEnabled` | PARTIAL |
| Offer Occurs Strictly Post-Verify, Pre-Archive | No pre-verify blocking control remains | `sddstatus > TestReviewOfferAbsenceGuardHoldsForProductionFiles` | COMPLIANT |
| Kill-Switch-Off Is Structural Absence | Call-absence guard proves invisibility | `sddstatus/cli > TestReviewOfferAbsenceGuardHolds*` | PARTIAL |
| Kill-Switch-Off Is Structural Absence | Archive is unfailable on review grounds when OFF | (none — empirical repro contradicts) | FAILING |
| Decline Proceeds to Unmanaged Ordinary Archive | Decline does not block archive | `sddstatus > TestReviewOfferDeclineLeavesNoStateAndDoesNotSuppressLaterOffer` | FAILING |
| Post-Offer Correction Triggers Targeted Re-Verify | Targeted re-verify for a scoped correction | `sddstatus > TestClassifyTargetedReVerifyBranches` | PARTIAL |
| Post-Offer Correction Triggers Targeted Re-Verify | Full re-verify when scoping is not provable | `sddstatus > TestClassifyTargetedReVerifyBranches` | FAILING |
| Intra-Wave Rollout Sequencing | Mirror deletion lands after offer and capability are live | git chain order S1..S7 | COMPLIANT |
| Consent-Gated Freeze, Preceded by Capability Admission | Tier 1 candidate freezes only after consent | pre-existing `internal/cli` consent suite | COMPLIANT |
| Consent-Gated Freeze, Preceded by Capability Admission | Frozen tier is never recomputed | covering test only at `67be4867`, **not in the chain** | PARTIAL |
| Consent-Gated Freeze, Preceded by Capability Admission | Capability admission precedes candidate freeze | `cli > TestUnsupportedReviewTransportCapabilityStopsBeforeAnyAuthority` | COMPLIANT |
| Offer Transition Reachable From a Real Call Site | Offer transition is wired to a live caller | `sddstatus > TestReviewEntryHookIsTheOneDoor` | PARTIAL |
| Offer Transition Reachable From a Real Call Site | Offer transition is absent from pre-verify code paths | `sddstatus > TestReviewOfferJourneyFiresZeroTimesAcrossFullFlowWhenDisabled` | COMPLIANT |
| ReceiptRef-Only Persistence | Approved lineage persists only a ReceiptRef | (none — writer unwired/baselined) | FAILING |
| ReceiptRef-Only Persistence | Existing binding files remain read-only | `sddstatus > TestParseLegacyBindingLeavesTheOnDiskFileByteIdentical` | COMPLIANT |
| No Re-Derived Gate Meaning | Gate meaning requested via facade | `sddstatus > TestBoundReviewArchiveGateIgnoresStaleGateContextViaReDerivation` | PARTIAL |
| No Re-Derived Gate Meaning | No local re-derivation on stale receipt | `sddstatus > TestResolveGoverningReceiptRefRequiresAParsedNativeBinding` | COMPLIANT |
| Attempt Ledger Ownership Stays With SDD | Attempts remain in SDD's runtime ledger | `sddstatus` runtime ledger suite | COMPLIANT |
| Attempt Ledger Ownership Stays With SDD | One owner named for compaction and ledger | `sddstatus > TestRuntimeObjectiveIsSoleWorkUnitScopeOwner` | COMPLIANT |
| Legacy reviewGate v1 Field Compatibility | Legacy field present when enabled | empirical: field present, no `delivery` | COMPLIANT |
| Legacy reviewGate v1 Field Compatibility | Legacy field absent when disabled | (none — empirical repro contradicts) | FAILING |
| ReceiptRef Lives in SDD's Runtime Ledger | ReceiptRef stored in the runtime ledger | `sddstatus > runtime_receipt_test.go` | COMPLIANT |
| Wave-0 Adapter Behavioral-Depth Trace | Trace recorded before adapter thinning starts | `agents > TestAdapterForbiddenConstructionGuardHoldsForProductionFiles` | COMPLIANT |
| Wave-0 Adapter Behavioral-Depth Trace | Missing trace blocks the task | (process rule, no mechanical test) | PARTIAL |
| Capability Declared Before Any Review State Exists | Supported transport proceeds normally | `cli > TestAuthorizeReviewTransportCapabilityMatrix/advertised_claim_admits` | COMPLIANT |
| Capability Declared Before Any Review State Exists | Unsupported transport denies before state creation | `cli > TestUnsupportedReviewTransportCapabilityStopsBeforeAnyAuthority` | COMPLIANT |
| Adapter Declares, Provider Fails Closed, No Probing | Adapter self-declares capability | `capabilitymanifest > manifest_test.go` | COMPLIANT |
| Adapter Declares, Provider Fails Closed, No Probing | Absent or unrecognized declaration fails closed | `cli > TestAuthorizeReviewTransportCapabilityMatrix` | PARTIAL |
| Per-Adapter Unavailable Mode, Never Unsafe Fallback | Pi adapter without capability enters unavailable mode | `cli > TestUnsupportedImmutableReviewTransportStopsBeforeRepositoryOrAuthority/Pi` | COMPLIANT |
| Per-Adapter Unavailable Mode, Never Unsafe Fallback | Capable in-repo adapter executes only opaque transitions | (none — task 8.4/8.5 deferred) | FAILING |

**Compliance summary**: 16/30 scenarios COMPLIANT, 8 PARTIAL, 6 FAILING. 4/16 requirements fully complete.

### Correctness (Static Evidence)

| Requirement | Status | Notes |
|---|---|---|
| Pre-verify routing removed | Implemented | `applyPreVerifyReviewRouting`/`applyPreVerifyCompactBridgeRouting` and all four call sites gone; only a tombstone comment at `status.go:879` |
| Capability admission before freeze | Implemented | `review_facade.go:1490` precedes repo resolution, consent and freeze |
| `SDDReceiptRef` two-field shape | Implemented | `receipt_ref.go`; reflect + `DisallowUnknownFields` guards |
| Archive gate re-derivation | Implemented | `ValidateSDDReceiptRef` replaces stored-`GateContext` comparison |
| Decision 9 single owner | Implemented | `CompactAcquireRequest{ BeginAttemptRequest }` |
| Offer/re-verify reach an orchestrator | **Not implemented** | `StatusV1Projection` drops both fields |
| `ReceiptRef` written per attempt | **Not implemented** | `recordPreparedReceipt` unwired and baselined |
| Decline → unmanaged archive | **Not implemented** | no decline mechanism; archive gate blocks |
| `reviewGate` absent when OFF | **Not implemented** | `status_v1.go` untouched by this wave |
| Plugin asset thinning | **Deferred** | tasks 8.4/8.5 investigated, coordinator-deferred to W5/W7 |

### Coherence (Design)

| Decision | Followed? | Notes |
|---|---|---|
| D1 `SDDReceiptRef` two fields, ledger-resident | Yes (scope-amended) | `Binding`/`BindingRevision` retirement moved to Wave 7 per the coordinator amendment |
| D2 Delete writer half / delegate reader half | Partially | Only `bindingExists`/`validateBoundReview` deleted (ratchet-as-arbiter, per amendment) |
| D3 One call site + decline semantics | Amended, then diverged | Call site moved to `sddstatus.Resolve()` per amendment (spec deltas still say "not `sddstatus.Resolve`" — never re-amended). Decline's "byte-identical to kill-switch-off" property does not hold at the wire |
| D4 AST guard as primary proof | Partially | Guard now proves "exactly one door", not "zero edges"; switch-off absence is a runtime `if`, corroborated by a counter |
| D5 `ContractReviewTransportV1` admission | Yes | Manifest claim + pre-freeze gate; manual/non-agent path deliberately ungated |
| D6 Decision 9 ratified | Yes | Owner named once; struct collapsed |
| Amendment: re-verify call site | Yes, incompletely | Three branches implemented; the amendment's point 4 (orchestrator prose) not delivered |

### TDD Compliance

| Check | Result | Details |
|---|---|---|
| TDD Evidence reported | PARTIAL | No "TDD Cycle Evidence" table in apply-progress; per-task RED/GREEN evidence is instead recorded inline in `tasks.md` (compile-fail RED named for 2.3, 3.1, 4.1, 5.1, 6.1, 6.2, 6.7, 7.1) |
| All tasks have tests | PASS | 21 test files touched, 151 top-level `Test` functions |
| RED confirmed (tests exist) | PASS | every named RED test file exists and compiles |
| GREEN confirmed (tests pass) | PASS | all named tests re-run by me, 0 failures |
| Triangulation adequate | PASS | table-driven matrices for capability (5 cases), re-verify branches (3), Git repo selection (4), guard shapes (4) |
| Safety Net for modified files | PASS | apply characterized `internal/sddstatus` green before each destructive slice |

### Test Layer Distribution

| Layer | Tests | Files | Tools |
|---|---|---|---|
| Unit / table-driven | ~120 | 16 | `go test` |
| Integration (`t.TempDir()` real Git + CAS stores) | ~28 | 8 | `go test` + real `git` |
| Static guard (AST/grep) | 5 | 3 | `go test` |
| Bench black-box | 0 new | 0 | `bench/` (green, unchanged) |
| **Total** | **151** | **21** | |

### Changed File Coverage

Coverage analysis skipped — no coverage threshold or tool configured in this repo.

### Assertion Quality

Scanned all 21 changed test files: no tautologies, no `t.Skip`, no empty-guard blocks, no test file without
`t.Fatal`/`t.Error`. **Assertion quality**: all assertions verify real behavior.

One structural note (not an assertion defect): every offer/re-verify assertion targets the in-package
`Status` value or `json.Marshal(Status)`. No test asserts the **projected wire bytes**, which is exactly why
CRITICAL-2 survived a fully green suite.

### Issues Found

**CRITICAL**

1. **`reviewGate` disabled/unmanaged ceremony still runs and is emitted when the kill switch is OFF** — CONFIRMED.
   Repro: fixture repo, all tasks done, admitted verify report, `gentle-ai review mode disable --cwd R --scope clone`,
   then `gentle-ai sdd-status demo-change --cwd R` → `"reviewGate": {"result":"invalidated", "delivery":"disabled/unmanaged", ...}`.
   Producing it runs `applyReviewGate` → `resolveReviewAuthority` → `discoverNativeReceipts`, a full repository walk.
   Violates `rdd-post-verify-review-offer` REQ "Kill-Switch-Off Is Structural Absence" ("no disabled/unmanaged ceremony
   capable of failing or blocking"; "archive consults no `reviewGate` structured status"),
   `rdd-sdd-receipt-consumption` REQ "Legacy `reviewGate` v1 Field Compatibility" scenario "Legacy field absent when
   disabled", and proposal success criterion "no review field appears in output". `status_v1.go` was never touched by
   this wave and no task covers it. Blocking path is real, not only cosmetic: when `resolveReviewAuthority` returns a
   non-`Absent` blocker (e.g. `discoverNativeReceipts` errors on a malformed store), `applyReviewGateEvaluation` falls
   through to `blockReviewGate` and sets `Dependencies.Archive = blocked` **while the switch is off**.

2. **`Status.ReviewOffer` and `Status.ReVerify` never reach any consumer** — CONFIRMED.
   Repro: `gentle-ai review mode enable --scope global`, then `gentle-ai sdd-status demo-change --cwd R` with
   `verify: all_done` → no `reviewOffer` key. Cause: `internal/sddstatus/status_v1.go`'s `StatusV1Projection`
   (lines 14-35) has no `ReviewOffer`/`ReVerify` field, and `RunSDDStatus`/`RunSDDContinue`
   (`internal/cli/sdd_status.go:39,70`) plus `RenderMarkdown`/`RenderDispatcherMarkdown`
   (`status.go:1089,1124,1172` → `marshalStatusV1Indent`) all project through it. Wave 4's two headline
   deliverables are therefore unreachable. The asset prose shipped by S3b/8.6 ("present the post-verify review
   offer only if the native status output contains a `reviewOffer` block", `internal/assets/{claude,opencode}/commands/sdd-verify.md`)
   is unsatisfiable as written. Consequential: a decline (switch on, no receipt) yields `archive: blocked`,
   `nextRecommended: resolve-review`, so `rdd-post-verify-review-offer` REQ "Decline Proceeds to Unmanaged Ordinary
   Archive" ("MUST NOT block archive on a decline") does not hold at the wire either.

3. **Wave 4 regresses kill-switch-off invisibility for clone-local disable, and the chain never saw the test that proves it** — CONFIRMED.
   The Wave 4 chain branches from `157ab9fd`, not from Wave 3's tip `67be4867`. Restoring `67be4867`'s three test
   files onto `d039d6e3` in a scratch clone:

   ```text
   review_new_lineage_kill_switch_test.go:120: first pass: OfferReviewAfterVerify(kill switch off) =
       reviewtransaction.Offer{Available:true, LineageID:"kill-switch-off-lineage"}, want Available=false
   --- FAIL: TestNewLineageKillSwitchOffProducesZeroSideEffectsAcrossEntrySurfaces (0.07s)
   ```

   Non-vacuous: the same test PASSES at `67be4867`. Cause: S5b (`2aafdfe8`) changed `OfferReviewAfterVerify` from an
   unconditional `Available:false` to `available := true`, while `readGlobalRDDModeForOffer`
   (`review_offer.go:90`) reads **only** the global scope. `review mode disable --scope clone` — the default shape
   of the user-owned kill switch — leaves `Available:true`. The `sddstatus` path is shielded because the CLI resolves
   the effective switch separately, but the offer API itself is not. This is exactly the defect class
   `rdd-post-verify-review-offer` REQ "Kill-Switch-Off Is Structural Absence" exists to prevent.

**WARNING**

- W1. `applyReviewGate` runs a full receipt-discovery repository walk on every SDD status read even when the switch is
  off; only the *verdict* is neutralised, not the work. Latent blocking path described in CRITICAL-1.
- W2. Spec text vs. amendment divergence, never reconciled: `rdd-post-verify-review-offer` scenario "Offer fires only
  after verify completes" and `rdd-review-core-transitions` scenario "Offer transition is wired to a live caller" both
  state the call site is `internal/cli` and "not `sddstatus.Resolve`, which remains a pure read", with reasoning
  ("an offer inside `Resolve` would re-create RDD as a supervisor of every status read"). The implementation is
  exactly inside `Resolve()`. `design.md`'s amendment supersedes *design decision 3*; the two spec deltas were never
  amended to match, so the delivered artifact set is self-contradictory.
- W3. Decline has no mechanism at all: there is no command, flag, or recorded state for "declined". "Decline" is
  defined only as "the orchestrator not acting on a block that keeps reappearing" — and the block never appears.
- W4. `verifyEvidenceScope`'s `openspec/changes/<c>/` narrowing inverts the risk ordering (source-code corrections →
  cheap `targeted`; planning-artifact corrections → `full`), and no branch emits the correction's changed paths as
  `Scope`, contrary to the spec scenario's "scoped to exactly those changed paths".
- W5. Coordinator amendment point 4 not delivered: no asset prose mentions the `reVerify` block
  (`rg -i 'reverify' internal/assets/` → only the two offer sentences). `Status.ReVerify` also never mutates
  `Dependencies.Archive`, so "archive does not proceed until that full re-verify passes" is unenforced anywhere.
- W6. `authorizeReviewTransportCapability` skips admission entirely when no `--agent` is supplied
  (`review_transport_admission.go:44-46`, and the matrix test asserts this as intended). The spec's
  "provider MUST fail closed when a declaration is absent" is satisfied for a *claimless known agent* but not for a
  *missing runtime identity*.
- W7. `recordPreparedReceipt`, `readLegacyReceiptRef`, `normalizeRecordReceiptRequest` and
  `ReceiptRevisionConflictError.Error` remain deadcode-baselined with no production caller, so
  `RuntimeStatus.Receipt` is never populated in production and the offer's `Available:false` branch is
  test-only. Honestly recorded by apply; repeated here because it makes REQ "ReceiptRef-Only Persistence" untestable
  end-to-end.
- W8. `internal/sddstatus` still contains substantial gate-result logic (`evaluateReceiptPayload`,
  `resolveReviewAuthority`, `blockReviewGate`, the ambiguity/empty-receipt reason constants) on the discovery path,
  so REQ "No Re-Derived Gate Meaning" is met only for the explicit-governance branch.
- W9. Strict TDD: apply-progress has no "TDD Cycle Evidence" table. The equivalent evidence exists per task inside
  `tasks.md` (each RED names its exact compile-fail symbol), so the substance is present; recorded as a WARNING
  rather than a CRITICAL because the protocol's purpose — provable RED before GREEN — is demonstrably satisfied.
- W10. Task 7.4 is marked `[x]` while its own text ("record a new `RuntimeAttempt` using the existing
  `RemediatesEvidenceRevision` field") is explicitly not done. A partially-done task marked complete misreports
  chain state; it should be split or left unchecked.

**SUGGESTION**

- S1. Add one wire-level test that asserts on `ProjectStatusV1(status)` bytes for both the offer and the re-verify
  block. A single such test would have caught CRITICAL-2 before this phase.
- S2. Rebase the Wave 4 chain onto `67be4867` before delivery and re-run `internal/cli`, so Wave 3's own coverage
  closure guards Wave 4's changes.
- S3. `ReVerifyModeTargeted` and `ReVerifyModeFull` are described in `review_reverify.go:61-72` in nearly identical
  words ("re-run the objective's evidence goal" vs "re-verify the objective's evidence goal"). Give the two modes an
  operationally distinguishable definition.

### Scoped-out items — honest assessment

| Item | Verdict |
|---|---|
| 1.2 (archive Wave 3) | Legitimately out of scope — externally blocked on Wave 3's own archive phase. Not a Wave 4 gap. |
| 7.4 `RuntimeAttempt` sub-clause | Genuine spec-MUST gap. Design decision 3 says the re-verify is "recorded as a new `RuntimeAttempt` with the existing `RemediatesEvidenceRevision` field", and the spec requires archive not proceed until the re-verify passes. Without any ledger write, nothing links a re-verify to an evidence revision and nothing gates archive. Task marked `[x]` regardless (see W10). |
| 7.6 / 7.7 (staged, `commit -a`) | Legitimately out of scope **as built** — the implementation reads persisted `CorrectionAttempts` and performs no live Git diff, so commit state genuinely has no bearing. The reasoning is sound and recorded. |
| 8.4 / 8.5 (plugin asset thinning) | Coordinator-deferred, but it *is* an uncovered in-scope item: the proposal lists "Bundled in-repo adapter assets consume opaque transitions only" as In Scope, and design.md's own CON-09 row marks the OpenCode plugin "Violates ... in scope". Deferral is recorded, not silent — but the requirement stays unmet. |

### Adversarial pass — new criticals only

`git diff 157ab9fd..d039d6e3` (55 files) reviewed. Findings above are the complete set. Explicitly **not**
counted as Wave 4 findings: Wave 3 verify's N1/N2 (pre-existing Wave 5 entry conditions). CRITICAL-1 has a
pre-existing component (the `disabled/unmanaged` disposition predates this wave, issue #1877) but is counted
here because removing it is this wave's own explicit, unmet requirement. CRITICAL-2 and CRITICAL-3 are
newly introduced by Wave 4.

### Verdict

FAIL — three CONFIRMED criticals (offer/re-verify unreachable on the wire, `reviewGate` ceremony still emitted
with the kill switch off, and a reproduced clone-local kill-switch regression in `OfferReviewAfterVerify`), plus
genuine spec-MUST gaps in decline semantics and re-verify enforcement.


---

```yaml
schema: gentle-ai.verify-result/v1
evidence_revision: sha256:ad34c5c2e1c699dcbb456c972a0a215b9e21484ee22314aab8fe63d58661f75f
verdict: fail
blockers: 3
critical_findings: 3
requirements: 11/16
scenarios: 24/30
test_command: go test ./... -count=1
test_exit_code: 0
test_output_hash: sha256:5de7da8dbe0882ba983cdd4e13c2a5e1b45d5540ff9d550deecbaa59c375f08a
build_command: go build -o gentle-ai ./cmd/gentle-ai
build_exit_code: 0
build_output_hash: sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855
```

## Verification Report — CYCLE 2 (re-verification after the corrective cycle)

**Change**: rdd-root-simplification-wave4
**Mode**: Strict TDD
**Attempt authority (echoed, not settled)**: sha256:07979978bc79aec5f41ea725b57945ee948c82503795a39db216934438f3a473
**Candidate**: worktree `/home/gentleman/work/gentle-ai-worktrees/rdd-wave4`, tip `0a5f13b9f09f320c031ae51c9eb7974ae5ba4ff6`
**Chain base**: `git merge-base HEAD 67be4867` = `67be4867033d4391b077544c9b437ee34bf6f5de` — the rebase claim is CONFIRMED; `git merge-base --is-ancestor 67be4867 HEAD` succeeds.
**Verdict**: **FAIL** — 3 CRITICAL, 4 WARNING, 2 SUGGESTION.

Cycle-1 history is preserved above/below this section; this cycle appends only.

### Executive result of the 3 claimed CRITICAL fixes

| Prior | Claim | Cycle-2 verdict |
|---|---|---|
| C1 | reviewGate structurally absent when RDD off, archive never review-blocked, no discovery walk | **PARTIALLY TRUE** — true for `applyReviewGate`; FALSE for three sibling paths in the same function (new CRITICAL-B) |
| C2 | `reviewOffer`/`reVerify` reach the real CLI wire | **CONFIRMED** |
| C3 | Offer resolves the effective (clone-scope) RDD mode | **CONFIRMED** |

---

### Item 1 — C1 repro with a fresh binary (clone-scope OFF)

Binary built from `0a5f13b9`. Fixture: git repo, complete OpenSpec change, all tasks `[x]`, parseable passing verify report. `review mode disable --scope clone`:

```text
$ gentle-ai review mode status --cwd <fx> --json
"global": "on", "clone_local": "off", "effective": "off", "source": "clone_local"
```

**TIP binary** (`sdd-status c1 --json`), exit 0:

```text
top-level keys: [schemaName schemaVersion changeName artifactStore planningHome changeRoot
                 artifactPaths contextFiles artifacts taskProgress dependencies applyState
                 actionContext relationships remediationState nextRecommended blockedReasons]
reviewGate in keys: False
dependencies.archive: ready
nextRecommended: archive
blockedReasons: []
stderr: 0 bytes
```

**PRE-CORRECTIVE binary** (`a087b0d0`), identical fixture:

```text
reviewGate in keys: True
reviewGate: {"result": "invalidated",
             "reason": "receipt-driven development is disabled, so no review governs this change; ...",
             "delivery": "disabled/unmanaged"}
```

**No-discovery-walk proof (strace, `openat`/`getdents64`)** — a decoy store was planted at
`.git/gentle-ai/review-transactions/v2/decoy-lineage/artifacts/receipt.json`:

```text
TIP: review-transactions/v1|v2 opens = 0     decoy-lineage touches = 0
PRE: review-transactions/v1|v2 opens = 2     decoy-lineage touches = 4
PRE line: openat(..., ".../review-transactions/v2", O_RDONLY|O_CLOEXEC|O_DIRECTORY) = -1 ENOENT
```

Only the kill-switch record itself (`rar-authority/v1/rdd-mode/gen-0000000001.json`) is read on the tip path. **PASS for the `applyReviewGate` path.** See CRITICAL-B for the paths this proof does not cover.

### Item 2 — C2: offer on the real wire, decline, re-verify block

Switch ON (`effective: on`), verify passed:

```text
$ gentle-ai sdd-status c1 --cwd <fx> --json
"reviewOffer": {"available": true, "lineageId": "c1",
                "invocation": "gentle-ai review start --cwd \"<fx>\""}
$ gentle-ai sdd-continue c1 --cwd <fx> --json
"reviewOffer": {"available": true, "lineageId": "c1", "invocation": "..."}
```

Pre-corrective binary, same fixture: `reviewOffer present: False`, `reVerify present: False`. **C2 CONFIRMED on both surfaces.**

**Decline → unmanaged ordinary archive: NOT SATISFIED (W3 unchanged).** The offer's own named invocation is `gentle-ai review start --cwd "<repo>"`; that form rejects `--consent declined` ("review start --consent requires the negotiated form"), and the negotiated form stops at `immutable_review_transport_unsupported`. Simply not acting on the offer leaves `dependencies.archive: blocked`, `nextRecommended: resolve-review` — the opposite of the requirement's "archive completes under ordinary `disabled/unmanaged` policy".

**Re-verify block when a correction is recorded: NOT PROVEN, and the gate is defective.** See CRITICAL-A.

### Item 3 — C3: W3 coverage tests at tip

`git merge-base --is-ancestor 67be4867 HEAD` → true. All three files introduced by `67be4867` are present at HEAD.

```text
--- PASS: TestNewLineageFrozenTierIsNeverRecomputedAfterFreeze (0.11s)
--- PASS: TestNewLineageKillSwitchOffProducesZeroSideEffectsAcrossEntrySurfaces (0.15s)
ok  	github.com/gentleman-programming/gentle-ai/v2/internal/cli	0.286s
--- PASS: TestNewLineageActivationSwitchIdentityNeverOverloadsAnotherSwitch (0.00s)
--- PASS: TestNewLineageActivationSwitchIndependentOfKillSwitch (0.03s)
ok  	github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction	0.049s
```

**PASS.**

### Item 4 — Carve-out removal is spec-backed; supersession documented

Ratified requirement text (`specs/rdd-post-verify-review-offer/spec.md:29`), verbatim:

> When the RDD kill switch is OFF, zero review code MUST execute on any SDD path: no offer, no status consultation, no disabled/unmanaged ceremony capable of failing or blocking.

and (`:42-43`):

> - THEN archive consults no `reviewGate` structured status
> - AND archive cannot fail or block for review reasons

The removal of the "explicit receipt still validates while disabled" carve-out is therefore **spec-backed**. Every rewritten test carries an explicit supersession comment naming the superseded expectation and why it no longer holds (`review_gate_disabled_test.go`, `review_offer_routing_test.go`, `sdd_status_review_disabled_test.go`, `sdd_status_reenable_fresh_review_test.go`, `organic_runtime_test.go`, `journeys_wave1.go`, `journeys_sdd.go`). No assertion was deleted silently. **PASS.**

### Item 5 — 7.4 strip-and-restore (non-tautology)

Stripping the guard body of `blockArchiveForUnsatisfiedReVerify` (`if true { return "" }`):

```text
    review_reverify_test.go:304: reason = "", want a non-empty blocked reason
--- FAIL: TestBlockArchiveForUnsatisfiedReVerify (0.00s)
FAIL	github.com/gentleman-programming/gentle-ai/v2/internal/sddstatus	0.007s
```

Genuine RED. **But** stripping the two *call sites* in `Resolve()`/`resolveEngramStatus()` instead leaves everything green:

```text
ok  github.com/gentleman-programming/gentle-ai/v2/internal/sddstatus       22.237s
ok  github.com/gentleman-programming/gentle-ai/v2/internal/cli            176.419s
ok  github.com/gentleman-programming/gentle-ai/v2/e2e/organicruntime       13.022s
```

Restore verified byte-identical: `git diff` empty, `git status --porcelain` empty, HEAD `0a5f13b9`.

### Item 6 — Full suites, ratchets, goldens

| Check | Command | Result |
|---|---|---|
| Root tests | `go test ./... -count=1` | exit 0, 63 `ok`, 0 `FAIL` |
| Build | `go build -o gentle-ai ./cmd/gentle-ai` | exit 0, empty output |
| Format | `gofmt -l .` | clean |
| Vet | `go vet ./...` | clean |
| Bench module | `gofmt -l . && go vet ./... && go build ./... && go test ./... -count=1` | all clean / `ok` |
| e2e | `e2e/organicruntime` (in root suite) | `ok 11.999s` |
| Deadcode ratchet | `bash scripts/deadcode-ratchet.sh` | `no new unreachable functions` |
| Refusal ratchets | `go test ./internal/cli -run 'RefusalResolution\|EveryProductionRefusal'` | `ok` |
| Goldens (no `-update`) | `go test ./internal/components -run Golden` | `ok` |
| **Bench journey corpus** | `bench run --binary <tip>` | **exit 1, 6 journeys FAILED** |

### CRITICAL findings

#### CRITICAL-A (NEW, introduced by `03c07581`) — the 7.4 archive gate is unsatisfiable and livelocks

`blockArchiveForUnsatisfiedReVerify` blocks archive until a passing `RuntimeAttempt` names exactly
`status.ReVerify.EvidenceRevision`. `applyTargetedReVerifyRouting` stamps that field with the **current**
verify-report evidence revision on every `Resolve()`, while `CompactState.CorrectionAttempts` is append-only
per lineage (`internal/reviewtransaction/compact.go:1326`; cleared only for a successor lineage,
`compact_recovered_evidence.go:97`). So the demand re-labels itself after every compliant re-verify.

Proven with a throwaway in-package probe (created, run, deleted; worktree restored byte-identically):

```text
cycle 1 blocked: ... record its outcome with gentle-ai sdd-attempt finish --remediates-evidence-revision sha256:R1
(operator complies: re-verifies, records a passing attempt remediating sha256:R1)
cycle 2 archive="blocked" next="verify" reason="... --remediates-evidence-revision sha256:R2"
LIVELOCK CONFIRMED
```

The only way to satisfy the gate is to *not* re-verify (leaving the report's revision unchanged), which
defeats the requirement the gate implements.

The named continuation is also not runnable as printed:

```text
$ gentle-ai sdd-attempt finish --remediates-evidence-revision sha256:1111... --cwd <fx> --change c1
Error: sdd-attempt finish requires --expected-revision, --request-id, --outcome, --evidence-revision,
       --diagnosis, --harness-disposition, --cleanup-evidence, --process-evidence
```

and `internal/cli/sdd_attempt.go:94-96` additionally refuses the flag unless
`--expected-binding-revision`, `--successor-lineage`, and `--remediates-evidence-revision` are given
**together** — `--successor-lineage` requires an approved compact successor, i.e. a full review round trip.
The apply-progress claim "no new writer — `gentle-ai sdd-attempt finish --remediates-evidence-revision <rev>`
already exists" is therefore incorrect as stated. This is exactly the class of defect the untested call site
(Item 5) hides.

#### CRITICAL-B (RESIDUAL C1) — kill-switch-off is not structural absence on all SDD paths

`applyReviewGate` was gated, but three sibling review paths inside the same `Resolve()`/`resolveEngramStatus()`
bodies are still ungated by `reviewDisabled`:

- `internal/sddstatus/status.go:478` / `:770` — `staleAllowAuthority` calls `resolveReviewAuthority` (full
  `discoverNativeReceipts` repo walk) and appends its failure as a blocked reason.
- `internal/sddstatus/status.go:487` — `resolveCompactRemediationAuthority` reaches the same discovery path.
- `internal/sddstatus/status.go:508-532` — the `governingRef != nil` branch runs `ValidateSDDReceiptRef` and,
  on non-allow, sets `dependencies.Verify/Archive = blocked` and `nextRecommended = "resolve-review"`.

Proven with the tip binary, `effective: off` (clone scope), stale verify totals (2/2 vs a 1/1 spec):

```text
$ gentle-ai sdd-status c1 --cwd <fx-stale> --json
dependencies.verify: blocked | dependencies.archive: blocked
nextRecommended: resolve-review
blockedReasons: [
 "terminal review receipt is missing; run the fresh full review of the current state with gentle-ai review start",
 "verify evidence cannot enter remediation: verify result total 2 does not match actual requirement count 1; ..."
]
strace: review-transactions/v1|v2 opens while OFF = 4      (with a planted decoy: 14 opens, 8 decoy touches)
```

The first blocked reason names `gentle-ai review start`, which the kill switch itself refuses — the exact
orchestrator dead-end `ResolveOptions.ReviewDisabled`'s own doc comment says must not happen, and a direct
violation of "archive cannot fail or block for review reasons". The pre-corrective binary behaves the same
here, so this hole predates the corrective cycle: CRITICAL-1 was under-scoped, not regressed.

#### CRITICAL-C — the bench journey corpus is RED at the wave tip and GREEN at the wave base

The orchestrator's premise ("the 6 failures are pre-existing at `67be4867`") is **disproven**:

```text
base corpus @67be4867 + base binary : 59 completed, 0 unsupported, 0 failed   (exit 0)
tip  corpus @0a5f13b9 + tip  binary : 53 completed, 0 unsupported, 6 failed   (exit 1)
base corpus @67be4867 + tip  binary : 50 completed, 0 unsupported, 9 failed   (exit 1)
```

The failing rows are `j41`, `j53`, `j54`, `j55`, `j56`, `j58`, all with the same shape:

```text
j41: pre-verify routing with reviews on: nextRecommended = "verify", want review
j53-j58: fail-closed SDD authority discovery: verify="ready" nextRecommended="verify",
         want blocked/resolve-review; blocked reasons=[]
```

The product behaviour is **spec-correct** — commit `21dfc0fe` deliberately removed
`applyPreVerifyReviewRouting`/`applyPreVerifyCompactBridgeRouting` as the ratified
"Offer Occurs Strictly Post-Verify, Pre-Archive" requirement demands. These six journeys pin the removed
pre-verify behaviour and were never updated, unlike `j42`/`j47`/`j49` which this wave correctly did update.
The wave therefore ships a regression corpus that cannot gate (`bench run` exits 1). Cycle 1 never ran
`bench run --binary`, only `go test ./bench`, which is why this was not caught earlier.

### WARNING findings

- **W-a — 7.4's integration is entirely untested.** Both `blockArchiveForUnsatisfiedReVerify` call sites can be
  deleted with `internal/sddstatus`, `internal/cli` and `e2e/organicruntime` all staying green (Item 5). The
  apply record says only "not proven end-to-end"; in fact no test at any level covers the wiring.
- **W-b — `Decline Proceeds to Unmanaged Ordinary Archive` has no reachable implementation** (Item 2). Carried
  forward from cycle 1 (W3), explicitly out of the corrective fix scope, still a spec-MUST gap.
- **W-c — 3 tasks remain unchecked**: `1.2` (Wave 3 archive, externally blocked), `7.6`/`7.7` (declared out of
  the S6 batch scope). All three carry written rationale in `tasks.md`; none is a Wave-4 implementation task.
- **W-d — the decline byte-identity assertion was materially weakened.**
  `TestReviewOfferDeclineStatusByteIdenticalToOffOutsideOfferBlock` compared full JSON bytes; its replacement
  `TestReviewOfferDeclineNeverBlocksArchiveAtTheProjectionLevel` only asserts "archive not blocked". The reason
  is documented in-file and is legitimate, but the regression surface shrank.

### SUGGESTION findings

- **S-a — vacuous assertion.** `TestDisabledArchiveGateNeverFabricatesReviewAuthority` now reads
  `if status.ReviewGate != nil && status.ReviewGate.Result == GateAllow`. Since `ReviewGate` is always nil on
  that path, the first clause can never fire; only the `ReviewTransaction` check still carries weight.
- **S-b — `StatusV1Projection` shares pointers.** `ProjectStatusV1` assigns `status.ReviewOffer`/`status.ReVerify`
  by pointer rather than copying, unlike the value-copy style used for `reviewGateStateV1` immediately above.
  Harmless today (no consumer mutates the projection) but inconsistent with the surrounding code.

### Spec compliance matrix (16 requirements / 30 scenarios, recounted after the W2 amendments)

| # | Requirement | Scenarios | Status |
|---|---|---|---|
| 1 | Offer Occurs Strictly Post-Verify, Pre-Archive | 2/2 | MET — real-CLI `reviewOffer` only after verify; pre-verify symbols removed |
| 2 | Kill-Switch-Off Is Structural Absence, Proven by Call-Absence | 1/2 | **NOT MET** — CRITICAL-B: archive blocks on review grounds while OFF |
| 3 | Decline Proceeds to Unmanaged Ordinary Archive | 0/1 | **NOT MET** — no reachable decline path (W-b) |
| 4 | Post-Offer Correction Triggers Targeted Re-Verify Before Archive | 0/2 | **NOT MET** — CRITICAL-A; integration untested (W-a) |
| 5 | Intra-Wave Rollout Sequencing | 1/1 | MET — chain order s3 offer → s4 capability → s5c/s7 mirror deletion |
| 6 | Consent-Gated Freeze … Preceded by Capability Admission | 3/3 | MET — frozen-tier test PASS; admission denies with `mutation_outcome: not_started` |
| 7 | Offer Transition Reachable From a Real Call Site | 2/2 | MET — `reviewOfferForVerify` live; absence guard green |
| 8 | ReceiptRef-Only Persistence | 2/2 | MET — unit-covered, suite green |
| 9 | No Re-Derived Gate Meaning | 2/2 | MET — archive gate rerouted onto `ValidateSDDReceiptRef` |
| 10 | Attempt Ledger Ownership Stays With SDD | 2/2 | MET — unit-covered, suite green |
| 11 | Legacy `reviewGate` v1 Field Compatibility | 2/2 | MET — present ON, absent OFF, both binary-verified |
| 12 | ReceiptRef Lives in SDD's Runtime Ledger, Not a New Artifact | 1/1 | MET — `RuntimeStatus.Receipt`, no new artifact file |
| 13 | Wave-0 Adapter Behavioral-Depth Trace Is a Hard Prerequisite | 1/2 | **NOT MET** — "missing trace blocks the task" has no runtime enforcement |
| 14 | Capability Declared Before Any Review State Exists | 2/2 | MET — `immutable_review_transport_unsupported`, `authority_applicability: not_evaluated` |
| 15 | Adapter Declares, Provider Fails Closed, No Probing | 2/2 | MET — unit-covered, suite green |
| 16 | Per-Adapter Unavailable Mode, Never Unsafe Fallback | 1/2 | **NOT MET** — the Pi-adapter scenario lives in a separate repo, unverifiable here |
| | **TOTAL** | **24/30** | **11/16 requirements met** |

### Verdict

**FAIL.** C2 and C3 are genuinely fixed and independently reproduced. C1 is only half-fixed. Two further
blockers were found: a livelocking, unsatisfiable archive gate introduced by the 7.4 closure, and a regression
corpus that the wave left red. Do not archive; return to `sdd-apply`.


---

```yaml
schema: gentle-ai.verify-result/v1
evidence_revision: sha256:cf7ac66fa49c9fe39a0bc532a08aa72551a6dafeadf222eeb9e1fa1218e71835
verdict: fail
blockers: 1
critical_findings: 0
requirements: 13/16
scenarios: 27/30
test_command: go test ./... -count=1
test_exit_code: 0
test_output_hash: sha256:b479959c385030f2a8a2b0216ae34d914ed1f20ef0dd9423f323d6066ffe30b6
build_command: go build -o gentle-ai ./cmd/gentle-ai
build_exit_code: 0
build_output_hash: sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855
```

## Verification Report — CYCLE 3 (re-verification after the third corrective cycle)

**Change**: rdd-root-simplification-wave4
**Mode**: Strict TDD
**Attempt authority (echoed, not settled)**: sha256:a931314e61555b98398639d6723d2e797d55976bd20415f63ea311c111233289
**Candidate**: worktree `/home/gentleman/work/gentle-ai-worktrees/rdd-wave4`, tip `89c7e5fde07fabd73d28d4ac43a3a21351f02c87`
**Chain base**: `git merge-base HEAD 67be4867` = `67be4867033d4391b077544c9b437ee34bf6f5de` (unchanged, still an ancestor).
**Verdict**: **FAIL** — **0 CRITICAL**, 1 blocker, 4 WARNING, 3 SUGGESTION.

All three cycle-2 CRITICALs (A, B, C) are **genuinely resolved and independently reproduced**. The single
remaining blocker is requirement #3, an openly-reported, long-standing spec-MUST that is still uncovered and
carries **no spec amendment**, so the pass condition ("no spec-MUST left silently uncovered; explicit
spec-amended deferrals are covered-by-amendment") is not met. This is a rule-driven fail, not a new defect.

### Cycle-3 claim verdicts

| Claim | Cycle-3 verdict |
|---|---|
| CRITICAL-B: three sibling paths gated + `staleEvidenceUnmanaged` leniency | **CONFIRMED FIXED** |
| CRITICAL-C: corpus green at tip + opt-in axis, 6 journeys rewritten | **CONFIRMED FIXED** |
| CRITICAL-A: gate removed, deferred to Wave 5 with a frozen-anchor requirement | **CONFIRMED REMOVED AND DEFERRED** |

---

### Item 1 — CRITICAL-B: my cycle-2 repro re-run on a fresh `89c7e5fd` binary

Identical fixture to cycle 2 (spec 1/1, verify report claims 2/2 → `verifyResult.Stale`), clone-scope OFF
(`global: on, clone_local: off, effective: off`).

**A) exact cycle-2 repro, no decoy:**

```text
reviewGate present: False
dependencies: {... "verify": "ready", "archive": "blocked"}
nextRecommended: verify
blockedReasons: []
stderr bytes: 0
review-transactions/v1|v2 opens while OFF: 0
```

**B) same + planted decoy store** (`.git/gentle-ai/review-transactions/v2/decoy-lineage/artifacts/receipt.json`):

```text
review-transactions/v1|v2 opens: 0
decoy-lineage touches:           0
blockedReasons: []   nextRecommended: verify
```

**C) prior tip `0a5f13b9` on the identical fixture** (attribution):

```text
dependencies.verify: blocked | archive: blocked
nextRecommended: resolve-review
blockedReasons: ["discover compact review stores: invalid compact authority graph: ...",
                 "verify evidence cannot enter remediation: ..."]
```

Cycle 2 measured 4 store opens (14 with a decoy, 8 decoy touches) and a blocked reason naming
`gentle-ai review start`. All are now zero. **The `staleEvidenceUnmanaged` leniency is surfaced, not silence**:
`verify: ready` + `nextRecommended: verify` with zero blocked reasons — the same "please re-verify" outcome the
enabled path grants, reached without consulting review at all.

**D) plain passing fixture, switch OFF** (C1 non-regression): `archive: ready`, `next: archive`,
0 store opens, 0 decoy touches, 0 stderr bytes, 0 `sdd-review-bindings` reads.

**E) enabled-path non-regression** (same stale fixture, switch ON): still consults (4 store opens), still
`verify: blocked / archive: blocked / next: resolve-review` with the review blocker. No over-gating.

**Gate coverage (strip-and-restore, all three cycle-3 gates):**

| Gate | Stripped | Result |
|---|---|---|
| 1 — `staleAllowAuthority` walk | `if !reviewDisabled && staleEvidenceCandidate` → `if staleEvidenceCandidate` (both bodies) | **RED**: `--- FAIL: TestDisabledStaleEvidenceNeverConsultsReviewAuthority` |
| 2 — `resolveCompactRemediationAuthority` | wrapper removed (both bodies) | GREEN — sddstatus 22.8s, cli 175.2s (see W-a) |
| 3 — `governingRef` `ValidateSDDReceiptRef` | wrapper removed (both bodies) | GREEN — sddstatus 22.4s, cli 175.8s, e2e 10.9s (see W-a) |

Restore verified byte-identical: `git diff` empty, `git status --porcelain` empty, HEAD `89c7e5fd`.

### Item 2 — CRITICAL-C: corpus and opt-in axis, run by me

```text
default corpus + fresh 89c7e5fd binary : 59 completed, 0 unsupported, 0 failed   (exit 0)
  journeys_counted: 59 | core_journeys: 59 | journey rows: 59
source-coupled axis + product built -tags bench_fixture : 60 completed, 0 unsupported, 0 failed (exit 0)
  j57-sdd-authority-drift-during-discovery-fails-closed  completed
```

Note: running the axis with only the *bench runner* tagged leaves j57 `unsupported`; the **product** binary
needs `-tags bench_fixture`. Both readings are recorded above. (The apply record says "58 journeys"; the
measured count is 59 — an immaterial bookkeeping slip, not a coverage gap.)

**Spot-read of 2 rewritten journeys — genuine supersession, not blanket weakening:**

- `j41`: names the exact deleted mechanism (`applyPreVerifyReviewRouting`, commit `21dfc0fe`), cites the
  ratifying requirement, and explains why the switch toggle is *kept* rather than deleted — "specifically to
  prove that absence: on, off, and back on all produce the same pre-verify routing." The rewritten assertion
  still pins `verify: ready`, `next: verify`, **and zero blocked reasons** on all three toggle positions, so a
  reintroduced pre-verify gate would still fail it.
- `j53` / the shared helper `sddStatusIgnoresCorruptCompactAuthorityPreVerify` (formerly `sddStatusFailsClosed`):
  identifies a **second** dead mechanism, `applyPreVerifyCompactBridgeRouting`, deleted by the same commit, and
  explains that `discoverCompactPreVerifyAuthority` still computes `Relevant`/`Reason` but nothing reads them.

**I verified that load-bearing claim independently:**

```text
$ rg -n 'bridge\.Relevant|bridge\.Reason|\.Relevant\b' --glob '!*_test.go' internal/
(zero matches)
$ rg -n 'bridge\.Eligible' --glob '!*_test.go' internal/
internal/sddstatus/status.go:565   (exactly one live reader, the unrelated stale-report-recovery bridge)
```

The claim is correct. The new assertion still carries a real regression pin (zero blocked reasons).

### Item 3 — CRITICAL-A: gate removal and deferral consistency

- **Code gone.** `rg 'blockArchiveForUnsatisfiedReVerify|nativeRuntimeAttemptRemediates'` matches only a
  comment in `review_reverify.go` and two design.md lines — zero production symbols.
- **`ReVerifyBlock` reverted** to `{Mode, Scope, Reason}`; `EvidenceRevision` removed.
- **`applyTargetedReVerifyRouting` is purely additive again** — the only mutation in its body is
  `status.ReVerify = &block`; no `Dependencies`, `NextRecommended`, or `BlockedReasons` writes.
- **Spec delta internally consistent.** The one clause that mandated the removed gate is annotated inline:
  "- AND archive does not proceed until that full re-verify passes (DEFERRED to Wave 5 — see amendment above;
  not enforced in Wave 4)". No other scenario clause mandates it. The `Targeted re-verify for a scoped
  correction` scenario contains no archive-ordering clause, so it needs none.
- **Deferral names Wave 5 with a concrete anchor.** Both spec and design require "(1) a demanded-revision
  anchor derived from the correction's own append-only data (e.g. its `FixDeltaHash`) … never from the live
  verify-report" and "(2) either a new, decoupled write path … or an explicit, ratified product decision".
  The design amendment additionally records the exact cycle-2 defect evidence, including
  `internal/cli/sdd_attempt.go:94-96`.

### Item 4 — Full suites, ratchets, goldens

| Check | Command | Result |
|---|---|---|
| Root tests | `go test ./... -count=1` | exit 0, 63 `ok`, 0 `FAIL` |
| e2e | `e2e/organicruntime` (in root suite) | `ok 11.826s` |
| Build | `go build -o gentle-ai ./cmd/gentle-ai` | exit 0, empty output |
| Format | `gofmt -l .` | clean |
| Vet | `go vet ./...` | clean |
| Bench module | `gofmt -l . && go vet ./... && go build ./... && go test ./... -count=1` | all clean / `ok` |
| Bench corpus | `bench run --binary <fresh tip>` | **exit 0, 59/59** |
| Bench axis | `bench run --axis source-coupled` (product `-tags bench_fixture`) | **exit 0, 60/60** |
| Deadcode ratchet | `bash scripts/deadcode-ratchet.sh` | `no new unreachable functions` |
| Refusal ratchets | `go test ./internal/cli -run 'RefusalResolution\|EveryProductionRefusal'` | `ok` |
| Goldens (no `-update`) | `go test ./internal/components -run Golden` | `ok` |

### Item 5 — Adversarial pass over `0a5f13b9..89c7e5fd`: NEW criticals

**Zero new CRITICALs.** One new WARNING (W-b below) and two new SUGGESTIONs were found.

### Blocker

#### BLOCKER-1 — requirement #3 `Decline Proceeds to Unmanaged Ordinary Archive` is an uncovered spec-MUST with no amendment

Re-verified on the fresh `89c7e5fd` binary, switch ON, verify passed:

```text
reviewOffer: {"available": true, "lineageId": "c1", "invocation": "gentle-ai review start --cwd \"<repo>\""}
archive: blocked | next: resolve-review
blockedReasons: ["terminal review receipt is missing; run the fresh full review of the current state with gentle-ai review start"]

$ gentle-ai review start --cwd <repo> --consent declined
Error: review start --consent requires the negotiated form; rerun as
       gentle-ai review start --contract gentle-ai.review-integration/v1 with the bound --target and --projection

(after declining, i.e. not acting)
archive: blocked | next: resolve-review
reviewGate: {"result": "invalidated", "reason": "terminal review receipt is missing; ..."}
```

The spec says: "WHEN the offer is declined, SDD MUST proceed to unmanaged ordinary archive under existing
repository policy. SDD MUST NOT block archive on a decline"; the scenario says "THEN archive completes under
ordinary `disabled/unmanaged` policy". Delivered behaviour blocks. The offer's own named invocation cannot be
declined, and the negotiated form is unreachable without a runtime identity.

This is **not a new finding** — it is W3/W-b from cycles 1 and 2, out of every stated fix scope. It becomes the
blocker only because this cycle's pass condition credits deferrals *only when spec-amended*, and unlike task 7.4
this requirement carries no amendment. **The cheapest honest resolution is the 7.4 precedent**: either implement
a decline path, or amend `rdd-post-verify-review-offer` to defer this requirement explicitly with a named wave
and anchor. Either flips this to pass.

### WARNING findings

- **W-a — gates 2 and 3 are untested.** Stripping the `resolveCompactRemediationAuthority` gate or the
  `governingRef` `ValidateSDDReceiptRef` gate leaves `internal/sddstatus`, `internal/cli` and
  `e2e/organicruntime` fully green (Item 1 table). Only gate 1 has a covering test. Same class as cycle 2's W-a;
  the fix is correct, its regression protection is one third of what it appears to be.
- **W-b (NEW) — a fourth ungated review-store walk survives.**
  `discoverCompactPreVerifyAuthority` → `reviewtransaction.CompactAuthorityLeaves` at
  `internal/sddstatus/status.go:561` is not gated by `reviewDisabled`. Reached in the common
  "apply done, no verify report yet" state. Proven on the fresh binary, switch OFF, with a planted decoy:

  ```text
  review-transactions/v1|v2 opens while OFF: 7
  decoy-lineage touches while OFF:           4
  (output: verify: ready | archive: blocked | next: verify | blockedReasons: [] | stderr 0 bytes)
  ```

  This violates the requirement's prose sentence ("zero review code MUST execute on any SDD path … no status
  consultation") but **not** either of its two scenarios: `reviewGate` stays absent, zero blocked reasons are
  emitted, and the bridge result is consumed only behind `recoverable`, so it cannot fail or block. Recorded as
  a WARNING rather than a CRITICAL for exactly that reason, with the repro above so a future wave can close it.
- **W-c — 3 tasks remain unchecked**: `1.2` (Wave 3 archive, externally blocked), `7.6`/`7.7` (out of the S6
  batch scope). Unchanged; all carry written rationale.
- **W-d — the targeted re-verify routing still has no end-to-end proof.** `classifyTargetedReVerify` /
  `deriveCorrectionEvidence` are unit-covered and green, and `TestResolveOmitsReVerifyBlockWithoutAnyCorrection`
  covers the negative case, but no test drives a real recorded correction through `Resolve()` to a populated
  `Status.ReVerify`. This is the S6-investigated, budget-driven gap, documented in apply-progress and unchanged.

### SUGGESTION findings

- **S-a (NEW) — task 7.4's checkbox reads `[x]` while its body says the spec-MUST sub-clause is "NOT done,
  DEFERRED to Wave 5".** The prose is exemplary; the marker contradicts it, so any mechanical task-completion
  count reads 53/56 complete with this counted as done.
- **S-b (NEW) — the amendment annotates the scenario clause but not the requirement's own MUST sentence.**
  Line 58 still reads "SDD MUST run a targeted re-verify … **before archive proceeds**" with no inline deferral
  marker; only the amendment paragraph below and the scenario bullet carry it. A reader scanning MUST sentences
  alone would miss the deferral.
- **S-c — archive is `blocked` with zero blocked reasons on the `staleEvidenceUnmanaged` path.** Consistent with
  the pre-existing `staleAllowAuthority` behaviour, and `nextRecommended: verify` names the exit, but an
  orchestrator reading `dependencies.archive` alone sees a blocker with no stated cause.

### Spec compliance matrix (16 requirements / 30 scenarios, recounted at `89c7e5fd`)

| # | Requirement | Scenarios | Status |
|---|---|---|---|
| 1 | Offer Occurs Strictly Post-Verify, Pre-Archive | 2/2 | MET — also pinned by the rewritten j41 in a green corpus |
| 2 | Kill-Switch-Off Is Structural Absence, Proven by Call-Absence | 2/2 | **MET (was 1/2)** — both scenarios pass; see W-b for the prose-level residue |
| 3 | Decline Proceeds to Unmanaged Ordinary Archive | 0/1 | **NOT MET** — BLOCKER-1, no implementation and no amendment |
| 4 | Post-Offer Correction Triggers Targeted Re-Verify Before Archive | 2/2 | **MET (was 0/2)** — routing unit-covered; archive-gating clause covered-by-amendment (Wave 5) |
| 5 | Intra-Wave Rollout Sequencing | 1/1 | MET |
| 6 | Consent-Gated Freeze … Preceded by Capability Admission | 3/3 | MET |
| 7 | Offer Transition Reachable From a Real Call Site | 2/2 | MET |
| 8 | ReceiptRef-Only Persistence | 2/2 | MET |
| 9 | No Re-Derived Gate Meaning | 2/2 | MET |
| 10 | Attempt Ledger Ownership Stays With SDD | 2/2 | MET |
| 11 | Legacy `reviewGate` v1 Field Compatibility | 2/2 | MET |
| 12 | ReceiptRef Lives in SDD's Runtime Ledger, Not a New Artifact | 1/1 | MET |
| 13 | Wave-0 Adapter Behavioral-Depth Trace Is a Hard Prerequisite | 1/2 | NOT MET — "missing trace blocks the task" has no runtime enforcement |
| 14 | Capability Declared Before Any Review State Exists | 2/2 | MET |
| 15 | Adapter Declares, Provider Fails Closed, No Probing | 2/2 | MET |
| 16 | Per-Adapter Unavailable Mode, Never Unsafe Fallback | 1/2 | NOT MET — the Pi-adapter scenario lives in a separate repo |
| | **TOTAL** | **27/30** | **13/16 requirements met** (cycle 2: 24/30, 11/16) |

### Verdict

**FAIL — but with zero CRITICAL findings.** The engineering work this cycle is sound: CRITICAL-B is fixed and
proven with the same repro that found it, CRITICAL-C is fixed with a green corpus and a genuinely-researched
supersession record, and CRITICAL-A was correctly removed and deferred rather than shipped livelocked. The only
thing standing between this change and archive is requirement #3, which needs either an implementation or the
same explicit spec amendment that 7.4 received.


---

```yaml
schema: gentle-ai.verify-result/v1
evidence_revision: sha256:2a5f5cd219c1120119e39886d831f8d7cc450a938259f622520e268f5341039a
verdict: fail
blockers: 2
critical_findings: 2
requirements: 11/16
scenarios: 25/30
test_command: go test ./... -count=1
test_exit_code: 0
test_output_hash: sha256:6522e06bf4b4bbd23f32ac4aa5b67df18be4907ce9d1eaff122511ab53845eaa
build_command: go build -o gentle-ai ./cmd/gentle-ai
build_exit_code: 0
build_output_hash: sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855
```

## Verification Report — CYCLE 4

**Change**: rdd-root-simplification-wave4
**Mode**: Strict TDD
**Attempt authority (echoed, not settled)**: sha256:17c89195eaef97bf88b76d012d4e84115696fbf67ae7b5c9e04c012a1ff54914
**Candidate**: worktree `/home/gentleman/work/gentle-ai-worktrees/rdd-wave4`, tip `cb195782fbc3f6394536e9f3e6ba92cb25b9118e`
**Chain base**: `git merge-base HEAD 67be4867` = `67be4867…` (unchanged).
**Verdict**: **FAIL** — **2 CRITICAL**, 3 WARNING, 3 SUGGESTION.

**All three cycle-4 code targets are genuinely fixed and independently reproduced.** This is not a closing pass,
because the adversarial pass found that the runtime contract Wave 4 has now changed three times was never
propagated to two places that still state the old contract: the **shipped `sdd-archive` skill** and a
**sibling spec delta**. Both would refuse or contradict exactly what BLOCKER-1's fix set out to permit.

### Cycle-4 claim verdicts

| Claim | Verdict |
|---|---|
| BLOCKER-1: `Missing` → offer + archive-ready in one output, fail-closed preserved | **CONFIRMED FIXED (runtime)** |
| W-a: gates 2/3 covered, strip-and-restore proven | **CONFIRMED — cycle-3's finding inverted** |
| W-b: fourth walk gated, output unchanged | **CONFIRMED FIXED** |

---

### Item 1 — BLOCKER-1 repro and the Missing-vs-Absent boundary

Fresh `cb195782` binary, switch ON (`effective: on`), verify passed, no receipt:

```text
reviewOffer: {"available": true, "lineageId": "c1", "invocation": "gentle-ai review start --cwd \"<repo>\""}
reviewGate present: False
dependencies.archive: ready
nextRecommended: archive
blockedReasons: []
stderr bytes: 0
OFFER AND ARCHIVE-READY IN THE SAME OUTPUT: True
```

Prior tip `89c7e5fd`, identical fixture: `archive: blocked | next: resolve-review`, `reviewGate` populated
with "terminal review receipt is missing". Attribution proven.

**No suppression across reads** — 5 consecutive `sdd-status` calls:

```text
read 1..5 -> reviewOffer.available: True | archive: ready | next: archive
```

**Fail-closed half holds on both boundaries:**

```text
(a) explicit tampered receipt (openspec/changes/c1/reviews/receipt.json = "{")
    reviewGate: {"result":"invalidated","reason":"review receipt is invalid or non-terminal: unexpected EOF"}
    archive: blocked | next: resolve-review          FAIL-CLOSED HELD: True

(b) discovered-but-broken store (planted decoy lineage, no review-state.json)
    reviewGate: {"result":"invalidated","reason":"discover compact review stores: invalid compact authority graph: ..."}
    archive: blocked | next: resolve-review          FAIL-CLOSED HELD: True
```

**Scoping verified by source inspection**: `Missing` is assigned on exactly one return in
`resolveReviewAuthority` (`review_gate.go:501-505`), guarded by `errors.Is(err, errTerminalReceiptMissing)`.
Every ambiguous / stale / invalid / escalated return leaves it false. The claim that `Missing` is strictly
narrower than `Absent` is correct.

### Item 2 — Strip-and-restore on each cycle-4 guarded branch

| Stripped branch | Result |
|---|---|
| BLOCKER-1 `if evaluation.Missing { return }` | **RED** — `TestResolveArchiveRequiresApprovedExactReviewReceipt`, `TestArchiveGateOffersButDoesNotBlockWhileReviewIsEnabled`, `TestReviewOfferPresentAndArchiveReadyForAGenuinelyMissingReceipt` |
| Gate 2 `resolveCompactRemediationAuthority` wrapper | **RED** — `TestDisabledRemediationNeverConsultsCompactAuthority` |
| Gate 3 `governingRef` `ValidateSDDReceiptRef` wrapper | **RED** — `TestDisabledArchiveGateNeverValidatesAnExplicitGoverningReceipt` |
| W-b bridge gate | GREEN — still untested (W-a' below) |

Cycle 3's "gates 2 and 3 stay green when stripped" finding is genuinely inverted. Restore verified
byte-identical: `git diff` empty, `git status --porcelain` empty, HEAD `cb195782`.

### Item 3 — W-b: the fourth walk

My cycle-3 fixture (apply done, no verify report, switch OFF, decoy store planted):

```text
review-transactions/v1|v2 opens while OFF: 0      (prior tip 89c7e5fd: 7)
decoy-lineage touches while OFF:           0      (prior tip 89c7e5fd: 4)
stderr bytes: 0
verify: ready | archive: blocked | next: verify | blockedReasons: []
observable output vs prior tip: dependencies identical / nextRecommended identical /
                                blockedReasons identical / reviewGate absent in both
```

Read-elimination only, exactly as claimed.

### Item 4 — Full suites, corpus, axis, ratchets, goldens

| Check | Result |
|---|---|
| `go test ./... -count=1` (root) | exit 0, 63 `ok`, 0 `FAIL` |
| e2e/organicruntime (in root suite) | `ok 11.851s` |
| `go build -o gentle-ai ./cmd/gentle-ai` | exit 0, empty output |
| `gofmt -l .` / `go vet ./...` | clean / clean |
| bench module gofmt+vet+build+test | all clean / `ok` |
| `bench run --binary <fresh tip>` | **exit 0, 59 completed, 0 failed** |
| `bench run --axis source-coupled` (product `-tags bench_fixture`) | **exit 0, 60 completed, 0 failed** (j42 and j57 both completed) |
| `scripts/deadcode-ratchet.sh` | `no new unreachable functions` |
| Refusal ratchets / goldens (no `-update`) | `ok` / `ok` |

### Item 5 — Adversarial pass over `89c7e5fd..cb195782`

Two CRITICAL findings, both artifact-level, both stemming from one root cause: **Wave 4 has now changed the
observable archive-gating contract three times and propagated the change to only one of the four places that
state it.**

#### CRITICAL-D — the shipped `sdd-archive` skill still states the pre-Wave-4 archive gate and would refuse what `sdd-status` now reports as ready

`internal/assets/skills/sdd-archive/SKILL.md:79` (verbatim, embedded in the shipped binary — `strings` finds
"Native Review Receipt Gate" in `gentle-ai`):

> Before any task reconciliation, spec sync, or archive move, require structured status with
> `reviewGate.result: allow`, or with `reviewGate.delivery: disabled/unmanaged` when the kill switch is off and
> no review governs this change. … **Missing**, pending, malformed, `scope-changed`, `invalidated`, or
> `escalated` review state **blocks archive with no override** and no automatic reviewer launch.

and `:81`:

> `disabled/unmanaged` is the only relaxation … **An explicit review artifact that failed validation still
> blocks** …

`internal/assets/skills/_shared/review-ledger-contract.md:40` repeats the same contract.

Wave 4 invalidated all three of those clauses and never touched either file
(`git log 67be4867..HEAD -- <both paths>` is empty):

1. **Cycle 2 (CRITICAL-1)**: switch OFF now yields `reviewGate` **absent**, never `delivery: disabled/unmanaged`
   — so the skill's "only relaxation" is now unreachable and the OFF path reads as "Missing … blocks archive".
2. **Cycle 2 (CRITICAL-1)**: the explicit-artifact carve-out at `:81` was removed; while OFF an explicit broken
   receipt is never read at all.
3. **Cycle 4 (BLOCKER-1)**: switch ON + verify passed + no receipt now yields `reviewGate` **absent** with
   `archive: ready` — which the skill classifies as "Missing … blocks archive with no override".

The skill contains no notion of an absent `reviewGate` at all
(`rg -i 'structural absence|absent|nil|omitempty'` → zero matches). An archive executor following the shipped
instructions therefore refuses precisely the states `sdd-status` reports as `ready` / `nextRecommended: archive`
— **defeating BLOCKER-1's fix at the consumer layer**, and leaving the two shipped surfaces of one product in
direct contradiction. Task 8.6 investigated `review-ledger-contract.md` and recorded "**NO changes made**", but
only searched it for `nextRecommended: review` staleness; `sdd-archive/SKILL.md` was never examined by any task.

This is a pre-existing defect for causes 1-2 (open since cycle 2, missed by cycles 2 and 3, including by me) and
newly widened by cause 3. It is mechanically fixable — the same prose-supersession discipline the wave already
applied to j42/j47/j49 and to the six pre-verify journeys.

#### CRITICAL-E — a sibling spec delta now directly contradicts the cycle-4 amendment

`rdd-sdd-receipt-consumption`'s requirement **Legacy `reviewGate` v1 Field Compatibility** (`spec.md:65-71`):

> `status_v1.go`'s legacy `reviewGate` structured field MUST remain readable for unmigrated Pi clients **while
> the kill switch is enabled** …
> - GIVEN the kill switch is ON and an unmigrated Pi client requests status v1
> - WHEN status is serialized
> - **THEN the legacy `reviewGate` field is populated for compatibility**

Measured on the fresh binary:

```text
switch ON + genuinely missing receipt   -> reviewGate present: False   <-- violates the requirement
switch ON + explicit tampered receipt   -> reviewGate present: True
```

Cycle 4 amended `rdd-post-verify-review-offer` but left this requirement untouched
(`git log 67be4867..HEAD -- .../rdd-sdd-receipt-consumption/spec.md` shows only the original artifact-landing
commit). The delivered spec set is self-contradictory: one delta ratifies structural absence with the switch
ON, another still mandates population with the switch ON. **This is exactly the W2 class the wave already had to
fix once in cycle 2** (two artifacts drifting apart after a ratified change), reintroduced by an incomplete
amendment. It also has a real consumer: unmigrated Pi clients are the stated reason the field exists.

### WARNING findings

- **W-a' — the W-b bridge gate is untested.** Stripping it leaves `internal/sddstatus` and `internal/cli` green.
  Defensible, since the walk is read-elimination only and cannot fail or block, but it is the one cycle-4 gate
  with no covering test.
- **W-c — 3 tasks remain unchecked** (`1.2` externally blocked, `7.6`/`7.7` out of the S6 batch scope).
  Unchanged; all carry written rationale.
- **W-d — targeted re-verify routing still has no live end-to-end `Resolve()` proof.** The S6-investigated,
  budget-driven gap, unchanged and documented.

### SUGGESTION findings

- **S-a — task 7.4's checkbox is `[x]`** while its body says the spec-MUST sub-clause is "NOT done, DEFERRED to
  Wave 5". Unchanged from cycle 3.
- **S-b — 7.4's requirement MUST sentence still lacks an inline deferral marker** (only the amendment paragraph
  and the scenario bullet carry it). Note that BLOCKER-1's cycle-4 amendment *did* annotate its scenario bullet
  inline — the good pattern; 7.4 should match it.
- **S-c — archive is `blocked` with zero blocked reasons** on the `staleEvidenceUnmanaged` path.
  `nextRecommended: verify` names the exit, but the field alone looks unexplained. Unchanged.

### Spec compliance matrix (16 requirements / 30 scenarios, recounted at `cb195782`)

| # | Requirement | Scenarios | Status |
|---|---|---|---|
| 1 | Offer Occurs Strictly Post-Verify, Pre-Archive | 2/2 | MET |
| 2 | Kill-Switch-Off Is Structural Absence, Proven by Call-Absence | 1/2 | **NOT MET (was 2/2)** — CRITICAL-D: the shipped archive skill still blocks on a Missing gate, so "archive cannot fail or block for review reasons" does not hold end-to-end. Pre-existing since cycle 2; found now, not caused by cycle 4 |
| 3 | Decline Proceeds to Unmanaged Ordinary Archive | 0/1 | **NOT MET** — runtime half fixed and proven; CRITICAL-D blocks the archive half at the consumer layer |
| 4 | Post-Offer Correction Triggers Targeted Re-Verify Before Archive | 2/2 | MET — routing unit-covered; archive-gating clause covered-by-amendment (Wave 5) |
| 5 | Intra-Wave Rollout Sequencing | 1/1 | MET |
| 6 | Consent-Gated Freeze … Preceded by Capability Admission | 3/3 | MET |
| 7 | Offer Transition Reachable From a Real Call Site | 2/2 | MET |
| 8 | ReceiptRef-Only Persistence | 2/2 | MET |
| 9 | No Re-Derived Gate Meaning | 2/2 | MET |
| 10 | Attempt Ledger Ownership Stays With SDD | 2/2 | MET |
| 11 | Legacy `reviewGate` v1 Field Compatibility | 1/2 | **NOT MET (was 2/2)** — CRITICAL-E: "populated when enabled" is false for the missing-receipt case; caused by cycle 4 and not amended |
| 12 | ReceiptRef Lives in SDD's Runtime Ledger, Not a New Artifact | 1/1 | MET |
| 13 | Wave-0 Adapter Behavioral-Depth Trace Is a Hard Prerequisite | 1/2 | NOT MET — "missing trace blocks the task" has no runtime enforcement |
| 14 | Capability Declared Before Any Review State Exists | 2/2 | MET |
| 15 | Adapter Declares, Provider Fails Closed, No Probing | 2/2 | MET |
| 16 | Per-Adapter Unavailable Mode, Never Unsafe Fallback | 1/2 | NOT MET — the Pi-adapter scenario lives in a separate repo |
| | **TOTAL** | **25/30** | **11/16 requirements met** |

Coverage moved 27/30 → 25/30 not because the code regressed — every cycle-4 runtime target improved and the
whole suite plus both bench corpora are green — but because this cycle's adversarial pass reached the shipped
consumer contract and the sibling spec delta for the first time, and both still state the pre-Wave-4 rules.

### Verdict

**FAIL — 2 CRITICAL, both artifact-propagation defects, zero code defects.** The engineering in cycle 4 is
correct and well-tested: BLOCKER-1's runtime behaviour is right and fail-closed is preserved on both boundaries,
gates 2 and 3 now have genuine RED coverage, and the fourth walk is gone. What remains is that Wave 4 changed
the archive-gating contract three times and updated only the code plus one of the four documents that state it.
Closing this needs `sdd-archive/SKILL.md`, `_shared/review-ledger-contract.md`, and
`rdd-sdd-receipt-consumption`'s requirement 11 brought in line with the ratified structural-absence reading —
prose work of the same kind the wave has already done well elsewhere.


---

```yaml
schema: gentle-ai.verify-result/v1
evidence_revision: sha256:ff0ecd611c050e802655ca81268f62e21391d589b5512e99670faa2c0ad68572
verdict: fail
blockers: 1
critical_findings: 0
requirements: 15/16
scenarios: 30/31
test_command: go test ./... -count=1
test_exit_code: 0
test_output_hash: sha256:9848b5ec15ec532a67b3ac500e411f01e1c668bdcc6787fc64a2491b3cbd26fa
build_command: go build -o gentle-ai ./cmd/gentle-ai
build_exit_code: 0
build_output_hash: sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855
```

## Verification Report — CYCLE 5

**Change**: rdd-root-simplification-wave4
**Mode**: Strict TDD
**Attempt authority (echoed, not settled)**: sha256:2b8d92b357a1b0ee1f80df78343216b3aa22460ac803ca4a3fd4c3a621604df3
**Candidate**: worktree `/home/gentleman/work/gentle-ai-worktrees/rdd-wave4`, tip `ea73f7cd`
**Chain base**: `git merge-base HEAD 67be4867` = `67be4867…` (unchanged).
**Verdict**: **FAIL** — **0 CRITICAL**, 1 blocker, 3 WARNING, 3 SUGGESTION.

**CRITICAL-D and CRITICAL-E are both fixed and independently verified. Zero production code was touched**
(`git diff --stat cb195782..ea73f7cd -- '*.go'` = one test file only). The wave is one **one-line spec
amendment** away from pass; there is no code work left.

**Note on spec counts**: the cycle-5 amendment added a scenario. True totals at `ea73f7cd` are **16 requirements
/ 31 scenarios** (cycles 1-4 measured 30), and the envelope above is admitted against those true counts.

---

### Item 1 — CRITICAL-D: every shipped surface re-read at tip and checked against measured behavior

I re-measured all four contract states on a fresh `ea73f7cd` binary and compared each against the rewritten
prose:

| Measured state | Binary output | Prose says |
|---|---|---|
| switch OFF, plain passing | `reviewGate: ABSENT`, `archive: ready`, `next: archive` | absent, proceeds — **matches** |
| switch ON, verified, no receipt | `reviewGate: ABSENT`, `archive: ready`, `next: archive`, `reviewOffer: present` | absent + offer, proceeds — **matches** |
| switch ON, explicit tampered receipt | `reviewGate: present result=invalidated`, `archive: blocked`, `next: resolve-review` | present non-`allow`, blocks — **matches** |
| switch ON, discovered-broken store | `reviewGate: present result=invalidated`, `archive: blocked`, `next: resolve-review` | present non-`allow`, blocks — **matches** |

All five surfaces now state the ratified contract, and **none contradicts the measured binary**:

- `internal/assets/skills/sdd-archive/SKILL.md` — the Native Review Receipt Gate section is rewritten as a
  three-way split (absent → proceed, in both the switch-off and switch-on-no-receipt cases; present + `allow` →
  proceed; present + anything else → block), closing with "Do not treat `reviewGate`'s absence itself as a
  defect to investigate or as grounds to demand a receipt". The `disabled/unmanaged` relaxation and the
  explicit-artifact carve-out are both gone. Its Engram retrieval step now reads the
  `review/{transaction,ledger,receipt,gate-context}` topics **conditionally**, only when `reviewGate` is
  present — a real consequence I had not flagged and they caught.
- `internal/assets/skills/_shared/review-ledger-contract.md` — Delivery sentence rewritten.
- `internal/assets/skills/_shared/sdd-status-contract.md` — **both** the wire-shape note (line 124) and the
  archive-readiness bullet (line 139) rewritten. Line 139 was not in the coordinator's named set; the apply
  cycle's own sweep found it.
- `internal/assets/claude/commands/sdd-archive.md` and `internal/assets/opencode/commands/sdd-archive.md` —
  STATUS GATE / HARD GATES rewritten to match.

**Goldens match without `-update`**: `go test ./internal/components/ -run Golden -count=1` → `ok`. The golden
diff is exactly **1 line changed in each of 14 files** (`git diff --numstat` shows `1+ 1-` for every one),
resolving to three distinct rendered strings — the shared Delivery sentence, the `sdd-archive` command STATUS
GATE line, and the OpenCode settings prompt blob that embeds the contract. Nothing else moved.

**The untouched `rdd-defect-workflow` mentions really are the unrelated delivery-gate domain — confirmed.**
`RDDDeliveryDisabledUnmanaged` is alive and produced by `internal/cli/review_facade.go` (lines 2955, 2987,
3049, 4104) for the git-hook lifecycle gates, a different subsystem from `sddstatus.Status.ReviewGate`.
Measured on a clean switch-off fixture:

```text
$ gentle-ai review validate --cwd <fx> --gate pre-commit
schema: gentle-ai.review-gate-result/v1 | action: repository-policy
reason: receipt-driven development is disabled and no receipt governs this candidate, so delivery
        follows ordinary repository policy …
```

A distinct schema and a distinct disposition, untouched by this wave. The skill's own context is public
defect collaboration, not SDD archive gating. Correctly left as-is.

### Item 2 — CRITICAL-E: requirement 11 is now internally consistent and matches measurement

The `rdd-sdd-receipt-consumption` delta conditions population on "a review was actually discovered for the
candidate", carries an amendment citing BLOCKER-1's ratified reading by name, renames the first scenario to
"Legacy field present when a review is discovered", and **adds** "Legacy field absent when enabled with no
receipt (decline)". Measured against all three scenarios:

```text
ON + genuinely missing receipt -> reviewGate ABSENT, reviewOffer present   (new scenario) PASS
ON + discovered receipt        -> reviewGate present                        PASS
OFF                            -> reviewGate ABSENT                         PASS
```

No residual contradiction with the cycle-4 `rdd-post-verify-review-offer` amendment.

### Item 3 — My own sweep

```text
rg 'Missing, pending|explicit review artifact that failed|only relaxation|omitted until final archive gating'
   internal/assets/ openspec/changes/rdd-root-simplification-wave4/     -> ZERO matches
```

- `rg -ln 'reviewGate' internal/assets/` returns **exactly the five files rewritten this cycle** — no sixth
  surface exists.
- All five agent-level `sdd-archive` assets (claude, cursor, kimi `.md` + `.yaml`, kiro) contain **0** mentions
  of `reviewGate` / `disabled/unmanaged` / receipt requirements — they delegate to the skill. Claim verified.
- Remaining `disabled/unmanaged` hits in `internal/assets/` are the new SKILL.md sentence that explicitly says
  there is no such value to check, plus the two `rdd-defect-workflow` delivery-gate lines and their test.
- Change-folder hits are all correctly scoped: `design.md:29` is the new **SUPERSEDED** marker naming both
  amendments, `design.md:57` describes the goal state, `proposal.md:20` states the ratified goal.

### Item 4 — Full suites, corpus, axis, ratchets

| Check | Result |
|---|---|
| `go test ./... -count=1` (root) | exit 0, 63 `ok`, 0 `FAIL` |
| e2e/organicruntime (in root suite) | `ok 11.759s` |
| `go build -o gentle-ai ./cmd/gentle-ai` | exit 0, empty output |
| `gofmt -l .` / `go vet ./...` | clean / clean |
| bench module gofmt+vet+build+test | all clean / `ok` |
| `bench run --binary <fresh tip>` | **exit 0, 59 completed, 0 failed** |
| `bench run --axis source-coupled` (product `-tags bench_fixture`) | **exit 0, 60 completed, 0 failed** |
| `scripts/deadcode-ratchet.sh` | `no new unreachable functions` |
| Refusal ratchets / goldens (no `-update`) | `ok` / `ok` |
| `./internal/components/... ./internal/assets/...` (the two updated pins) | all `ok` |

### Item 5 — Adversarial pass over `cb195782..ea73f7cd`

**Zero new findings.** The two hand-maintained pins were updated honestly:

- `TestOpenCodeRenderedReviewProtocolCost`: `13_729 → 14_014` and `21_487 → 21_772`, both exactly **+285**,
  consistent with a single shared-sentence rewrite. `maxCharacters` ceilings (18,500 / 36,000) were **not**
  raised — the increase is absorbed by existing headroom, so the cost guard was not weakened to fit.
- `TestKilocodeReviewSettingsMatchCurrentMainBaseline`: a SHA-256 pin, necessarily updated; the changelog
  comment names the cause and cross-references the cost test.

Both carry documented reasons matching the file's established pattern. No assertion was relaxed or deleted.

### Correction to my own earlier cycles — requirement 16 was wrongly marked NOT MET

Cycles 2-5 carried #16 "Per-Adapter Unavailable Mode" as NOT MET on the grounds that "the Pi-adapter scenario
lives in a separate repo". **That was my error**, taken from the spec's grounding note without checking the
tree. `internal/agents/pi/` exists in this repo, and Wave 4's own commit `acb3c7c1` added
`internal/agents/adapter_forbidden_construction_guard_test.go`, whose
`adapterForbiddenConstructionPackageDirs` **includes `pi`** and whose
`TestAdapterForbiddenConstructionGuardHoldsForProductionFiles` scans every production adapter file for the four
forbidden constructions. That is exactly the scenario's "does not self-construct a transition, flag, or binding
of any kind" clause, covered by a test that passes at runtime:

```text
ok  github.com/gentleman-programming/gentle-ai/v2/internal/agents                    0.007s
ok  github.com/gentleman-programming/gentle-ai/v2/internal/agents/pi                 0.007s
ok  github.com/gentleman-programming/gentle-ai/v2/internal/agents/capabilitymanifest 0.003s
```

The "enters unavailable mode" clause is the provider-side fail-closed path I measured in cycle 3
(`immutable_review_transport_unsupported`, `mutation_outcome: not_started`,
`authority_applicability: not_evaluated`). **#16 is MET 2/2.** Recording the correction rather than silently
flipping the number.

### Blocker

#### BLOCKER-2 — requirement 13's second scenario is neither runtime-covered nor spec-amended-deferred

`rdd-transport-capability`'s **Wave-0 Adapter Behavioral-Depth Trace Is a Hard Prerequisite**, scenario
**"Missing trace blocks the task"**:

- GIVEN the CON-09/10/11 trace has not been completed and recorded
- WHEN an implementer attempts an adapter-thinning task
- THEN the task MUST NOT proceed
- AND the blocker is reported as "Wave-0 trace incomplete"

This is a **process counterfactual**, not product behaviour: it describes what an implementer must do in a state
that never occurred. The positive half is fully satisfied and verifiable — the trace ran first (tasks 2.1/2.2,
recorded in `docs/architecture/rdd-ownership-inventory.md`) and is encoded as a passing runtime guard
(task 2.3, commit `acb3c7c1`, first in the chain). But nothing in the product can enforce process ordering, so
the negative clause has no covering test and cannot acquire one.

Under this cycle's stated pass rule ("every requirement covered **or explicitly spec-amended-deferred**") it is
neither. **This is a rule-driven blocker on an unfalsifiable clause, not a defect.** The 7.4 and BLOCKER-1
precedents resolve it in one edit: append an amendment to that requirement recording that the prerequisite was
satisfied by chain ordering and that the negative scenario is a process guard with no runtime enforcement
point. That flips this to pass with no code change.

### WARNING findings

- **W-a' — the W-b bridge gate still has no covering test** (stripping it leaves `internal/sddstatus` and
  `internal/cli` green). Read-elimination only; cannot fail or block. Unchanged from cycle 4.
- **W-c — 3 tasks remain unchecked**: `1.2` (Wave 3 archive, externally blocked), `7.6`/`7.7` (out of the S6
  batch scope). All carry written rationale. Unchanged.
- **W-d — targeted re-verify routing still has no live end-to-end `Resolve()` proof.** The S6-investigated,
  budget-driven gap. Unchanged.

### SUGGESTION findings

- **S-a — task 7.4's checkbox is `[x]`** while its body says the spec-MUST sub-clause is "NOT done, DEFERRED to
  Wave 5". Unchanged.
- **S-b — 7.4's requirement MUST sentence still lacks an inline deferral marker.** Note that both the cycle-4
  BLOCKER-1 amendment and the cycle-5 requirement-11 amendment annotate their scenario bullets inline — 7.4 is
  now the only amendment not following the wave's own good pattern.
- **S-c — archive is `blocked` with zero blocked reasons** on the `staleEvidenceUnmanaged` path. Unchanged.

### Spec compliance matrix (16 requirements / 31 scenarios, recounted at `ea73f7cd`)

| # | Requirement | Scenarios | Status |
|---|---|---|---|
| 1 | Offer Occurs Strictly Post-Verify, Pre-Archive | 2/2 | MET |
| 2 | Kill-Switch-Off Is Structural Absence, Proven by Call-Absence | 2/2 | **MET (was 1/2)** — CRITICAL-D fixed; shipped contract now agrees with the runtime |
| 3 | Decline Proceeds to Unmanaged Ordinary Archive | 1/1 | **MET (was 0/1)** — runtime proven in cycle 4, consumer contract now matches |
| 4 | Post-Offer Correction Triggers Targeted Re-Verify Before Archive | 2/2 | MET — archive-gating clause covered-by-amendment (Wave 5) |
| 5 | Intra-Wave Rollout Sequencing | 1/1 | MET |
| 6 | Consent-Gated Freeze … Preceded by Capability Admission | 3/3 | MET |
| 7 | Offer Transition Reachable From a Real Call Site | 2/2 | MET |
| 8 | ReceiptRef-Only Persistence | 2/2 | MET |
| 9 | No Re-Derived Gate Meaning | 2/2 | MET |
| 10 | Attempt Ledger Ownership Stays With SDD | 2/2 | MET |
| 11 | Legacy `reviewGate` v1 Field Compatibility | 3/3 | **MET (was 1/2)** — CRITICAL-E fixed; all three scenarios measured |
| 12 | ReceiptRef Lives in SDD's Runtime Ledger, Not a New Artifact | 1/1 | MET |
| 13 | Wave-0 Adapter Behavioral-Depth Trace Is a Hard Prerequisite | 1/2 | **NOT MET** — BLOCKER-2, process counterfactual with no enforcement point and no amendment |
| 14 | Capability Declared Before Any Review State Exists | 2/2 | MET |
| 15 | Adapter Declares, Provider Fails Closed, No Probing | 2/2 | MET |
| 16 | Per-Adapter Unavailable Mode, Never Unsafe Fallback | 2/2 | **MET (was 1/2)** — my earlier-cycle error, corrected above |
| | **TOTAL** | **30/31** | **15/16 requirements met** |

### Verdict

**FAIL — 0 CRITICAL, 1 rule-driven blocker.** Cycle 5 did exactly what it set out to do: every shipped prose
surface now states the ratified contract, all five were verified against the measured binary, goldens are
byte-scoped to the intended sentence, the two hand-maintained pins were updated honestly without relaxing any
ceiling, and the whole suite plus both bench corpora are green with zero production code touched. Coverage is
the highest this change has reached (15/16, 30/31). The one remaining item is requirement 13's unfalsifiable
process counterfactual, which needs the same one-paragraph amendment treatment 7.4, BLOCKER-1, and
requirement 11 already received — and nothing else.


---

```yaml
schema: gentle-ai.verify-result/v1
evidence_revision: sha256:b0ce122da982ca99d537fcb4ce9d7d8ebbcb219a2f06059cbd3b113ba10efb96
verdict: pass
blockers: 0
critical_findings: 0
requirements: 16/16
scenarios: 31/31
test_command: go test ./... -count=1
test_exit_code: 0
test_output_hash: sha256:09cb7e2679e28b9d018feedeb345d4e241b3d8737643d2142eec3da1d7441371
build_command: go build -o gentle-ai ./cmd/gentle-ai
build_exit_code: 0
build_output_hash: sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855
```

## Verification Report — CYCLE 6 (envelope-only closure)

**Change**: rdd-root-simplification-wave4
**Mode**: Strict TDD
**Attempt authority (echoed, not settled)**: sha256:ae1cd5b82dfb877b8396c05c681a3b4d96edf94ebc75f252c544a822de12aff3
**Candidate**: worktree `/home/gentleman/work/gentle-ai-worktrees/rdd-wave4`, tip `7598eda430a6cc1fe8ab6b62cd971862ba03a786`
**Chain base**: `git merge-base HEAD 67be4867` = `67be4867…` (unchanged across all six cycles).
**Verdict**: **PASS** — 0 CRITICAL, 0 blockers, 4 WARNING, 3 SUGGESTION. **16/16 requirements, 31/31 scenarios.**

---

### Item 1 — Tip, scope, and cleanliness

```text
HEAD                     7598eda430a6cc1fe8ab6b62cd971862ba03a786
git status --porcelain   (empty)
git diff --stat ea73f7cd..HEAD
  .../specs/rdd-transport-capability/spec.md | 2 ++
  1 file changed, 2 insertions(+)
non-spec files changed:  none
```

Spec-file-only, two inserted lines, worktree clean. Confirmed.

### Item 2 — The amendment: consistency, evidence, precedent

The appended paragraph classifies "Missing trace blocks the task" as a process counterfactual that "constrains
task ordering during the wave's own execution, not runtime behavior, so no runtime test can falsify it after
the fact", records the obligation as satisfied by process evidence, and declares it covered-by-amendment
"per the 7.4 / decline / requirement-11 amendment precedent". That matches the shape those three amendments
already established in this wave, and it matches the resolution cycle 5 prescribed.

**Evidence spot-check — substance confirmed, one stale identifier:**

- The cited commit **does** carry the evidence it claims:
  ```text
  $ git show --stat ead610f6
  test(agents): add Wave-0 CON-09/10/11 adapter forbidden-construction guard
    docs/architecture/rdd-ownership-inventory.md                 |   7 +-
    internal/agents/adapter_forbidden_construction_guard_test.go | 195 +++++
    …
  ```
- **But `ead610f6` is not in the delivered lineage**: `git merge-base --is-ancestor ead610f6 HEAD` fails. It is
  the **pre-rebase** SHA of that commit. The cycle-2 corrective rebased the whole chain onto `67be4867`,
  rewriting every SHA. The delivered equivalent is **`acb3c7c1`**, and the two are provably the same commit —
  identical patch-ids:
  ```text
  ead610f6 patch-id: ffb96d27d65328d8a285f1511c7e879e70a3d7b6
  acb3c7c1 patch-id: ffb96d27d65328d8a285f1511c7e879e70a3d7b6
  ```
  `acb3c7c1` **is** an ancestor of HEAD.
- The amendment's other two pointers both resolve correctly at HEAD:
  `docs/architecture/rdd-ownership-inventory.md` records the CON-09/10/11 verdicts, each explicitly citing
  "behavioral depth traced Wave 4 S1" and naming
  `TestAdapterForbiddenConstructionGuardHoldsForProductionFiles` as the proof; and the guard passes at tip:
  ```text
  --- PASS: TestAdapterForbiddenConstructionGuardCatchesKnownShapes (0.00s)
  --- PASS: TestAdapterForbiddenConstructionGuardHoldsForProductionFiles (0.00s)
  ok  github.com/gentleman-programming/gentle-ai/v2/internal/agents  0.003s
  ```

The amendment's claim is therefore **true and independently verified**. The stale SHA is a citation-accuracy
defect, not an evidence gap — a reader has two working pointers (file path and guard path) and one that
resolves only outside the delivered chain. Recorded as **W-e** below, not as a blocker.

### Item 3 — Coverage recomputation

Requirement 13 is now covered: scenario 1 ("Trace recorded before adapter thinning starts") is process-verified
and permanently encoded by the passing guard; scenario 2 ("Missing trace blocks the task") is
**covered-by-amendment** as an explicitly spec-amended process obligation with no runtime enforcement point.

Nothing else surfaced. Spec counts recounted at tip: **16 requirements / 31 scenarios**.

### Item 4 — Fresh evidence at the actual tip

The commit touches only `openspec/`, which is not embedded in the binary, so nothing could change — but a PASS
verdict should carry evidence from the candidate it admits, so all of this was re-run at `7598eda4`:

| Check | Result |
|---|---|
| `go build -o gentle-ai ./cmd/gentle-ai` | exit 0, empty output |
| `go test ./... -count=1` (root) | exit 0, 63 `ok`, 0 `FAIL` |
| e2e/organicruntime (in root suite) | `ok 11.857s` |
| `gofmt -l .` / `go vet ./...` | clean / clean |
| `scripts/deadcode-ratchet.sh` | `no new unreachable functions` |

Bench corpus (59/59, exit 0) and the opt-in `source-coupled` axis (60/60, exit 0) were measured at `ea73f7cd`
in cycle 5 against a freshly built binary; this commit changes no Go source, no asset, and no golden, so that
evidence carries forward unchanged.

### Spec compliance matrix (16 requirements / 31 scenarios at `7598eda4`)

| # | Requirement | Scenarios | Status |
|---|---|---|---|
| 1 | Offer Occurs Strictly Post-Verify, Pre-Archive | 2/2 | MET |
| 2 | Kill-Switch-Off Is Structural Absence, Proven by Call-Absence | 2/2 | MET |
| 3 | Decline Proceeds to Unmanaged Ordinary Archive | 1/1 | MET |
| 4 | Post-Offer Correction Triggers Targeted Re-Verify Before Archive | 2/2 | MET — archive-gating clause covered-by-amendment (Wave 5) |
| 5 | Intra-Wave Rollout Sequencing | 1/1 | MET |
| 6 | Consent-Gated Freeze … Preceded by Capability Admission | 3/3 | MET |
| 7 | Offer Transition Reachable From a Real Call Site | 2/2 | MET |
| 8 | ReceiptRef-Only Persistence | 2/2 | MET |
| 9 | No Re-Derived Gate Meaning | 2/2 | MET |
| 10 | Attempt Ledger Ownership Stays With SDD | 2/2 | MET |
| 11 | Legacy `reviewGate` v1 Field Compatibility | 3/3 | MET |
| 12 | ReceiptRef Lives in SDD's Runtime Ledger, Not a New Artifact | 1/1 | MET |
| 13 | Wave-0 Adapter Behavioral-Depth Trace Is a Hard Prerequisite | 2/2 | **MET (was 1/2)** — scenario 2 covered-by-amendment, process-verified |
| 14 | Capability Declared Before Any Review State Exists | 2/2 | MET |
| 15 | Adapter Declares, Provider Fails Closed, No Probing | 2/2 | MET |
| 16 | Per-Adapter Unavailable Mode, Never Unsafe Fallback | 2/2 | MET |
| | **TOTAL** | **31/31** | **16/16 requirements met** |

Trajectory: cycle 1 `4/16, 16/30` → cycle 2 `11/16, 24/30` → cycle 3 `13/16, 27/30` → cycle 4 `11/16, 25/30`
(adversarial pass reached the shipped consumer contract) → cycle 5 `15/16, 30/31` → cycle 6 **`16/16, 31/31`**.

### Carried-forward findings — none blocking

**WARNING**

- **W-e (new, this cycle)** — the requirement-13 amendment cites `ead610f6`, a pre-rebase SHA absent from the
  delivered lineage. The delivered equivalent is `acb3c7c1` (identical patch-id, confirmed above). Substance is
  correct; only the identifier is stale. A one-token edit whenever the spec is next touched.
- **W-a'** — the W-b pre-verify bridge gate has no covering test (stripping it leaves `internal/sddstatus` and
  `internal/cli` green). Read-elimination only; cannot fail or block.
- **W-c** — 3 tasks remain unchecked: `1.2` (archive Wave 3 — externally blocked, not a Wave 4 code task),
  `7.6`/`7.7` (explicitly outside the coordinator's stated S6 batch scope, documented as not applicable as
  built). Classified WARNING rather than CRITICAL under the Decision Gates rule "CRITICAL for core task,
  WARNING for cleanup task" — none is a Wave 4 implementation task, and all three carry written rationale in
  `tasks.md`. This judgment has been applied consistently in all six cycles and is stated here explicitly so it
  is reviewable rather than implicit.
- **W-d** — targeted re-verify routing has no live end-to-end `Resolve()` proof; the S6-investigated,
  budget-driven gap, documented in apply-progress. The archive-gating half it would have exercised is itself
  deferred to Wave 5 by ratified amendment.

**SUGGESTION**

- **S-a** — task 7.4's checkbox is `[x]` while its body says the spec-MUST sub-clause is "NOT done, DEFERRED to
  Wave 5"; a mechanical counter reads it as complete.
- **S-b** — 7.4's requirement MUST sentence lacks an inline deferral marker. The decline, requirement-11, and
  now requirement-13 amendments all annotate inline; 7.4 is the only one that does not.
- **S-c** — archive is `blocked` with zero blocked reasons on the `staleEvidenceUnmanaged` path;
  `nextRecommended: verify` names the exit, but the field alone looks unexplained.

### Verdict

**PASS.** Every requirement is either covered by passing runtime evidence or explicitly spec-amended-deferred,
with zero CRITICAL findings and zero blockers. The four remaining WARNINGs and three SUGGESTIONs are all
documented, non-blocking, and carry named follow-ups. Over six cycles this change went from three confirmed
criticals and a livelocking archive gate to a fully covered, fully propagated contract: the kill switch is
genuinely invisible, the post-verify offer is an invitation rather than a gate, fail-closed still holds for
every discovered-but-broken receipt, the bench corpus and its opt-in axis are green, and the shipped prose now
says exactly what the binary does. Clear to archive.
