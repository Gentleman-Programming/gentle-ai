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
