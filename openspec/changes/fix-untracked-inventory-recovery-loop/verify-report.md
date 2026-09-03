```yaml
schema: gentle-ai.verify-result/v1
evidence_revision: sha256:b81dec95b7c5d635b633b06456c7faee0ce0f37246d0fe5c952b9f9b3bbde633
verdict: pass
blockers: 0
critical_findings: 0
requirements: 8/8
scenarios: 13/13
test_command: go test ./internal/cli/... -run 'TestIntendedUntracked|TestReviewCapabilities|TestReviewProviderArtifact|TestExplicitFrozenReviewingStatus|TestRunSDDAttempt'
test_exit_code: 0
test_output_hash: sha256:7206f43ab708e5008fa4ea1e86c121032dd50543ddcbc9e760ac3a4a44c23c38
build_command: go build ./...
build_exit_code: 0
build_output_hash: sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855
```

## Verification Report

**Change**: fix-untracked-inventory-recovery-loop
**Version**: N/A (delta specs, no prior spec version)
**Mode**: Strict TDD

### Completeness
| Metric | Value |
|--------|-------|
| Tasks total | 34 |
| Tasks complete | 34 |
| Tasks incomplete | 0 |

### Build & Tests Execution
**Build**: ✅ Passed
```text
$ go build ./...
(clean, exit 0)
```

**Tests**: ✅ 100% passed (focused suite: all subtests across TestIntendedUntracked*, TestReviewCapabilities*, TestReviewProviderArtifact*, TestExplicitFrozenReviewingStatus*, TestRunSDDAttempt* — 64.9s)
```text
$ go test ./internal/cli/... -run 'TestIntendedUntracked|TestReviewCapabilities|TestReviewProviderArtifact|TestExplicitFrozenReviewingStatus|TestRunSDDAttempt'
ok  	github.com/gentleman-programming/gentle-ai/v2/internal/cli	64.935s
```
Additional commands run and verified independently in this verify session:
- `go vet ./...` → exit 0, clean.
- `go run ./internal/gofmtcheck` → exit 0, clean.
- `cd bench && go build . && go vet ./... && go test ./...` → exit 0, `ok github.com/gentleman-programming/gentle-ai/bench (cached)`.

Known pre-existing failures NOT attributable to this change (per orchestrator baseline, independently accepted): `internal/reviewtransaction` (pre-existing store-lock failures, confirmed identical on a pristine worktree; package does not import `internal/cli`), `internal/update` (bash 3.2 environmental, unrelated syntax gap). Neither package was touched by this change and neither appears in the required focused test command above.

**Coverage**: Not run in this verify session (focused-suite policy per orchestrator guidance; coverage was not a requested verification dimension) → ➖ Not available

### Spec Compliance Matrix — `review-findings-ledger`

| Requirement | Scenario | Test | Result |
|-------------|----------|------|--------|
| Unconditional Eligible-Untracked-Inventory Publication | Field published before declaration | `review_intended_untracked_test.go > TestEligibleUntrackedInventoryPublishedUnconditionally/undeclared` | ✅ COMPLIANT |
| Unconditional Eligible-Untracked-Inventory Publication | Field published after declaration | `review_intended_untracked_test.go > TestEligibleUntrackedInventoryPublishedUnconditionally/declared_select,declared_exclude` | ✅ COMPLIANT |
| Unconditional Eligible-Untracked-Inventory Publication | Field published while RDD is disabled | `review_intended_untracked_test.go > TestEligibleUntrackedInventoryPublishedUnconditionally/rdd_disabled` | ✅ COMPLIANT |
| Unconditional Eligible-Untracked-Inventory Publication | Field published under a current compact reviewing authority | `review_frozen_status_test.go > TestExplicitFrozenReviewingStatusUsesFrozenUntrackedScope` (extended) | ✅ COMPLIANT |
| Staged Projection Omits The Field | Key is structurally absent for staged | `review_intended_untracked_test.go > TestStagedProjectionOmitsEligibleUntrackedInventoryKey` (asserts `map[string]json.RawMessage` key absence, not `== ""`) | ✅ COMPLIANT |
| Schema v7 Advertisement | Capabilities list both v6 and v7 | `review_capabilities_test.go > TestReviewCapabilitiesV25AdvertisementIsCurrent` | ✅ COMPLIANT |
| Recovery Route Produces A Usable Value | Refusal, recovery, and retry close the loop | `review_intended_untracked_test.go > TestIntendedUntrackedRefusalsNameARunnableStatusInvocation` (strengthened in place) | ✅ COMPLIANT |
| Publication Does Not Weaken Declaration Safety | Undeclared untracked file is still refused | `review_intended_untracked_test.go > TestIntendedUntrackedRefusalsNameARunnableStatusInvocation` (`assertNoUntrackedSelectionAuthority`) + `sdd_attempt_untracked_test.go > TestRunSDDAttemptAcquireRefusesUndeclaredEligibleUntrackedScopeBeforeToken` (unchanged code path, still green) | ✅ COMPLIANT |

**Compliance summary**: 8/8 scenarios compliant (review-findings-ledger)

### Spec Compliance Matrix — `rdd-sdd-receipt-consumption`

| Requirement | Scenario | Test | Result |
|-------------|----------|------|--------|
| Ledger-Owned Untracked-Freshness Refusal For Finish And Settle | Finish refusal carries the ledger's message | `sdd_attempt_untracked_test.go > TestRunSDDAttemptFinishAndSettleSurfaceLedgerUntrackedFreshnessMessage/finish` | ✅ COMPLIANT |
| Ledger-Owned Untracked-Freshness Refusal For Finish And Settle | Settle refusal carries the ledger's message | `sdd_attempt_untracked_test.go > TestRunSDDAttemptFinishAndSettleSurfaceLedgerUntrackedFreshnessMessage/settle` | ✅ COMPLIANT |
| Begin And Acquire Keep Their Own Preflight | Begin preflight is unchanged | `sdd_attempt_untracked_test.go > TestRunSDDAttemptFinishAndSettleNegativeControlsStillRefuse/begin_still_refuses_a_stale_digest_at_the_CLI` + source diff confirms `sdd_attempt.go:105-126` byte-identical | ✅ COMPLIANT |
| Begin And Acquire Keep Their Own Preflight | Acquire preflight is unchanged | `sdd_attempt_untracked_test.go > TestRunSDDAttemptAcquireRefusesUndeclaredEligibleUntrackedScopeBeforeToken`, `TestRunSDDAttemptAcquireSelectsInventoryValidatedPaths` (unchanged code path, still green) | ✅ COMPLIANT |
| Refusal Ownership Change Does Not Weaken The Check | Genuine mismatch still refuses after the CLI precondition is removed | `sdd_attempt_untracked_test.go > TestRunSDDAttemptFinishAndSettleSurfaceLedgerUntrackedFreshnessMessage` (stale-digest subcase asserts continued refusal through the ledger) | ✅ COMPLIANT |

**Compliance summary**: 5/5 scenarios compliant (rdd-sdd-receipt-consumption)

**Combined compliance summary**: 13/13 scenarios compliant across both delta specs.

### Correctness (Static Evidence)

| Requirement | Status | Notes |
|------------|--------|-------|
| Digest assignment ordering (design decision 2) | ✅ Implemented | `review_facade.go:921` — `result.EligibleUntrackedInventory = intendedScope.Digest` assigned once, immediately after `result.intendedUntracked = intendedScope` (line 910), before the compact-reviewing replacement (928-935), `newReviewNextTransition` (~1176), and the `rdd_disabled` guard. Confirmed by `git diff`: this is a pure insertion, no existing line touched. |
| #1972 fail-closed intact | ✅ Implemented | `review_facade.go:928-935` (compact-reviewing replacement) is byte-identical to pre-change code per `git diff` — only the new line 921 sits ahead of it. `review_next_transition.go:855-858`'s `if scope.Declared && scope.Digest == "" { return nil, false }` is untouched (zero diff in that file). |
| begin/acquire preflight retained | ✅ Implemented | `sdd_attempt.go` diff shows the begin/acquire block (`intendedUntrackedScopeForTarget` call, still workspace-reading via `SnapshotBuilder.IntendedUntrackedInventory`/`ValidateIntendedUntrackedSelection`) is untouched; only the `finish`/`settle` block was rewritten. |
| Freshness check not weakened | ✅ Implemented | `internal/sddstatus/runtime_ledger.go` has zero diff (git status confirms package not modified). `settlementUntrackedSelection`'s `request.ExpectedUntrackedInventory != digest` comparison (line 1036) is the same code that ran before this change; it is now the *only* untracked-freshness check finish/settle reach, since the duplicate CLI-side workspace read was deleted. |
| Unconditional publication (ordering, not conditionals) | ✅ Implemented | The digest assignment at line 921 has no surrounding `if`. Live verification (orchestrator baseline, re-derivable from code): staged skips the `else` branch that computes `intendedScope` via `reviewIntendedUntrackedScopeForTarget` (lines 805-813), leaving `Digest` at its zero value, so `omitempty` drops the key — confirmed structurally by `TestStagedProjectionOmitsEligibleUntrackedInventoryKey`. |
| Staged absence mechanism | ✅ Implemented | `review_status_contract.go:84` — `EligibleUntrackedInventory string \`json:"eligible_untracked_inventory,omitempty"\`` combined with the zero-value `Digest` on the staged code path (never routed through `intendedUntrackedScopeForTarget`). |
| Schema v7 additive-optional | ✅ Implemented | `status-v7.schema.json` `$ref`s v5/v6 for every existing property, adds `eligible_untracked_inventory` as optional (not in `required`), and `next_transition` accepts either v5's or v6's shape with `submission` optional. |
| Capabilities v2.5 advertises v6 and v7 both | ✅ Implemented | `capabilities-v2.5.schema.json`'s 22-entry enum includes both `gentle-ai.review-integration.status/v6` and `.../v7`; `review_capabilities.go` appends `ReviewIntegrationStatusSchemaV6` (unconditionally, pre-existing) and `ReviewIntegrationStatusSchemaV7` (new) to `result.Schemas`. |
| `intendedUntrackedDeclarationShape` extraction, no duplication | ✅ Implemented | `review_intended_untracked.go` diff shows the mode/shape checks (lines ~91-109 pre-change) extracted into one pure function, called from both `intendedUntrackedScopeForTarget` (begin/acquire path) and `sdd_attempt.go`'s finish/settle path. |
| Task 3.1 grep re-verified independently | ✅ Confirmed | `grep -rn '"gentle-ai.review-integration.status/v5"' internal/ cmd/ --include="*.go" \| grep -v "_test.go"` returns only the constant declaration itself — no hard-coded consumer comparison exists. |

### Coherence (Design)

| Decision | Followed? | Notes |
|----------|-----------|-------|
| 1. Field home on `ReviewTargetStatusResult`, not shared `projection.schema.json` | ✅ Yes | `review_status_contract.go:84`, placed after `Projection` as specified. |
| 2. Assignment ordering bypasses suppressors | ✅ Yes | Verified via `git diff` — single insertion, no branch. |
| 3. Keep both the top-level field and the collect argument | ✅ Yes | `validateIntendedUntrackedSelectionTransition` still requires 6 arguments including `expected_untracked_inventory`; schema `arguments` `minItems/maxItems: 6` retained in v7. |
| 4. Schema v7 is `$ref`-based, not a copy, v5→v6 precedent followed | ✅ Yes | `status-v7.schema.json` is 66 lines, entirely `$ref`-composed. |
| 5. Capabilities v2.5 advertises v7 unconditionally | ✅ Yes | `review_facade.go:1183-1184`'s old conditional V6-bump deleted; `review_capabilities.go` v2.5 branch appends v7 unconditionally for contract v2. |
| 6. No new Go-side digest shape validation (schema stays sole authority) | ✅ Yes | `grep -n "sha256:\[0-9a-f\]" internal/cli/review_status_contract.go` (per apply-progress) found nothing added to `Validate()`; the only regex-based digest check (`reviewCapabilitySHA256Pattern`) lives in the test file, not production code. |
| 7. Fix B deletion boundary — only the two workspace-reading operations | ✅ Yes | `sdd_attempt.go` diff confirms exactly `builder.IntendedUntrackedInventory`/`ValidateIntendedUntrackedSelection` (via `intendedUntrackedScopeForTarget`) removed from finish/settle; flag-shape validation preserved via the extracted helper. |

### Strict TDD Compliance

| Check | Result | Details |
|-------|--------|---------|
| TDD Evidence reported | ✅ | Full "TDD Cycle Evidence" tables present in apply-progress.md for WU1 (implicit, RED/GREEN narrative per task), WU2 (explicit table for R1-R4), and WU3 (explicit table for R5) |
| All tasks have tests | ✅ | 34/34 tasks; every GREEN task traces to a RED test confirmed failing first (compile-level RED for Go, per the documented methodology) |
| RED confirmed (tests exist) | ✅ | All named test functions independently confirmed present in the codebase via direct file reads (`TestRunSDDAttemptFinishAndSettleSurfaceLedgerUntrackedFreshnessMessage`, `TestEligibleUntrackedInventoryPublishedUnconditionally`, `TestStagedProjectionOmitsEligibleUntrackedInventoryKey`, `TestIntendedUntrackedRefusalsNameARunnableStatusInvocation`, `TestReviewCapabilitiesV25AdvertisementIsCurrent`, `TestReviewProviderArtifactStatusV7ContractsArePinned`) |
| GREEN confirmed (tests pass) | ✅ | All re-run in this verify session; 100% pass (see Build & Tests Execution) |
| Triangulation adequate | ✅ | R2's presence table covers 4 distinct scenarios + 1 more via the extended frozen-status test = 5 total scenarios for one behavior; R7's negative-control table covers 4 shape-violation cases × finish/settle + 1 begin case |
| Safety Net for modified files | ✅ | `review_facade.go`, `sdd_attempt.go`, `review_intended_untracked.go`, `review_status_contract.go`, `review_capabilities.go` are all pre-existing files with large pre-existing test suites (`TestIntendedUntracked*`, `TestReviewCapabilities*`, `TestRunSDDAttempt*`) all re-run green in this session |

**TDD Compliance**: 6/6 checks passed

---

### Test Layer Distribution

| Layer | Tests | Files | Tools |
|-------|-------|-------|-------|
| Unit/Integration (CLI-boundary, real git repos via `t.TempDir()`) | ~15 new/modified test functions across WU1-WU3 | 7 Go test files | stdlib `testing`, real `git` subprocess |
| E2E (driven bench journey against a real built binary) | 1 (`j4040-untracked-inventory-recovery-loop`) | `bench/journeys_4040_untracked_inventory.go` | `gentle-ai-bench` harness driving `/tmp/gentle-ai` |
| **Total** | **~16** | **8** | |

---

### Assertion Quality

No tautologies, no ghost loops, no assertion-free tests found in the reviewed test additions. All new/strengthened tests assert concrete values: regex-matched digests fed back into a real retry, structural key-absence via `map[string]json.RawMessage`, exact error-message substring content, and exact struct-field equality against an independently computed digest.

**Assertion quality**: ✅ All assertions verify real behavior

---

### Quality Metrics
**Linter**: ➖ Not available (no linter configured in cached capabilities beyond `go vet`)
**Type Checker**: ✅ No errors (`go vet ./...`, `go build ./...` both exit 0)
**gofmt**: ✅ No errors (`go run ./internal/gofmtcheck` exit 0)

### Apply-Time Deviations Assessed

1. **Renamed `TestReviewCapabilitiesV24AdvertisementIsCurrent` → `...V25...` in place, rather than adding a parallel test.** Assessed as **acceptable, not a defect**. Verified: no fixture file (e.g. `capabilities-v2.4.fixture.json`) exists for a frozen v2.4 snapshot — this test asserts *live* emitted output. Once the emitter is bumped to v2.5 (which design decision 5 mandates), an unmodified `...V24...` test would assert `Schema == ".../v2.4"` against output that is now `.../v2.5`, i.e. a permanently red test — not a legitimate state to leave in the suite. This matches the codebase's own established pattern (WU2's `review_transition_schema_drift_test.go` and `review_intended_untracked_test.go` update `V5`→`V7`/`V6`→`V7` assertions in place for the identical reason). No coverage is lost: the renamed test still asserts the full v2.5 shape (`Schema`, `Protocol{2,5}`, `slices.Contains(..., V7)`, absence of stale versions, schema validation).
2. **Declared the new bench journey with `Review: reviewUntouched` instead of the task's literal `Review: reviewOptedIn`.** Assessed as **acceptable, not a defect**. `reviewOptedIn` would have the bench runner enable RDD globally before the journey's first step — which would defeat the exact scenario under test (design decision 2's claim that the digest survives the `rdd_disabled` guard specifically). Independently confirmed: the journey's own assertion reads a STATUS response while RDD remains disabled and validates `schema == status/v7` and a well-formed top-level digest at that point — this could not be exercised under `reviewOptedIn`. The choice is also consistent with an existing sibling journey (`j41-kill-switch-versus-sdd-pre-verify`, also `reviewUntouched`) whose subject is likewise the review switch itself.

### Issues Found

**CRITICAL**: None

**WARNING**: None

**SUGGESTION**:
- Coverage percentage was not measured in this verify session (no coverage tool was run); this is informational only per Strict-TDD-verify rules (coverage/quality metrics are never blocking).
- The `internal/reviewtransaction` pre-existing test failures (independently reproducible on a pristine worktree, per the orchestrator's baseline) were not re-verified inside this verify session due to their ~655s runtime exceeding a reasonable per-command budget; this verify session accepted the orchestrator-supplied and apply-progress-documented evidence that they are pre-existing/environmental and unrelated to this change (confirmed via `go list -deps` showing no import of `internal/cli`, independently re-derivable but not re-run here).

### Verdict
**PASS**

All 34 tasks are complete and independently confirmed against the actual code (not apply-progress claims alone). Both delta specs' 8 requirements / 13 scenarios trace to real, currently-passing tests. The five orchestrator-flagged risk areas (staged absence, #1972 fail-closed, begin/acquire preflight, freshness check strength, unconditional publication ordering) were each independently re-derived from `git diff` and source reads, not accepted on report claims, and all hold. The two apply-time deviations are justified engineering judgment calls, not defects. All four required verification commands (focused test suite, `go vet`, `gofmtcheck`, bench module gate) pass cleanly in this session. The size:exception for the ~730-line changeset is a previously recorded, accepted decision and is not re-flagged here.

---

## Post-Verify Amendment (commit `e54fade8`)

The PASS verdict above was reached against the tree at commit `149b1189`. One later commit
amended the change, and this section records it so the report is not read as covering work
it never saw.

**Scope of the amendment**: `bench/journeys_4040_untracked_inventory.go` only. No
production code, no contract schema, and no delta-spec requirement changed, so the 8
requirements / 13 scenarios traced above are unaffected and the PASS verdict stands for
them.

**What it does**: the journey now drives #4040's two refusals before reading the recovery
route, instead of reading the recovered digest directly. Full detail, including the
verified distinction between the two refusal sites, is in `apply-progress.md` under
"Post-Verify Amendment".

**What it changes about this report's own findings**: "Apply-Time Deviations Assessed" item
2 (the `reviewUntouched` declaration) is unaffected and still stands. The WU3 deviation
recorded in `apply-progress.md` as "No hard refusal-message assertion" is now REVERSED, and
the verification that deviation deferred has been performed against a built binary.

**Re-run evidence for the amended tree**:

| Gate | Result |
|------|--------|
| `cd bench && go vet ./... && go test ./...` | clean, `ok` |
| `gentle-ai-bench run --binary <built>` (full corpus) | `62 completed, 0 unsupported, 0 failed` |
| `gentle-ai-bench run --binary <built> --only j4040-untracked-inventory-recovery-loop` | `1 completed, 0 unsupported, 0 failed` |
| `go run ./internal/gofmtcheck` | clean |
| `go vet ./...` | clean |

The full-corpus run is new evidence this report did not previously carry: the verify session
above accepted a `--only` run, which cannot demonstrate that an amended journey leaves the
rest of the corpus undisturbed.

**Unchanged caveat**: `go test ./...` was still not observed green end to end locally, for
the three pre-existing reasons documented above. CI remains the authority.

---

## Merge-Time Amendment (merged as `1f9d7941`)

The change shipped to `main` through PR #4066. Two further commits landed on the branch
after the amendment above, and this section records them so the archived report matches
what actually merged rather than what this session last verified.

**1. Upstream integration (`6e321836`, authored here).** `main` advanced by 11 commits
while the PR was open, mostly the review stop-hook work (#4067, #4071). Two conflicts, both
adjacent insertions resolved by keeping both sides:

| File | Resolution |
|------|------------|
| `bench/journeys.go` | Register both `stopHookJourneys()` and `untrackedInventoryRecoveryLoopJourneys()` |
| `bench/review_declarations.go` | Declare review modes for both `j125-claude-code-stop-hook-reminds-once-per-candidate` and `j4040-untracked-inventory-recovery-loop` |

No semantic overlap with this change. `internal/cli/review_facade.go` auto-merged because
upstream added the `stop-hook` case to `runReviewCommand` while this change only touches
`runReviewStatus`. The capabilities v2.5 / status v7 bump is independent of upstream's
`review-provider-contract` 1.1.0 to 1.2.0 bump; they are different contracts.

**2. Maintainer amendment (`67193488`, `test(bench): cover #4040 deletion recovery`).** The
maintainer rewrote `bench/journeys_4040_untracked_inventory.go` (+57/-42) before merging.
This supersedes the journey described in the Post-Verify Amendment above.

The verified journey drove the freshness refusal with `untrackedRecoveryLoopStaleDigest`, a
well-formed all-zero digest that is deliberately wrong. The merged journey removes that
constant and produces real drift instead: it reads the recovery route while the candidate
file exists, deletes the file, and then asserts that STATUS republishes a canonical digest
distinct from the previous one, and that the now-stale digest's refusal discloses the
canonical empty inventory that can still close the active attempt.

**What this changes about this report's findings**: the 8 requirements / 13 scenarios traced
above are unaffected. No production code, contract schema, or delta-spec requirement changed
in either commit. The merged journey is strictly stronger evidence for the same
requirements: it exercises the deletion direction of inventory drift, which no reporter on
issue #4040 had covered and which the verified journey could not reach with a synthetic
digest. The `reviewUntouched` declaration assessed in "Apply-Time Deviations Assessed" item
2 is unchanged in the merged tree.

**Evidence for the merged tree**: the re-run table in the amendment above covers the
verified journey, not the merged one. The merged journey's execution proof is CI on PR
#4066 and the maintainer's own validation; this session did not re-run the driven harness
against `67193488`. Locally confirmed against the merged tree at `1f9d7941`: `go build ./...`
clean, and from `bench/`, `go vet ./...` and `go test ./...` clean (`ok`, 6.5s). Note that
`bench/` is a separate Go module, so `go test ./bench/...` from the repository root fails
with a module-prefix error and is not the gate; it must be run from inside `bench/`.

**Delivery**: PR #4066 merged 2026-09-03 as merge commit `1f9d7941`. Issue #4040 closed as
completed. The fix is on `main` and is not yet carried by any published release tag, so
occurrences reported against `2.5.0` stable remain reproducible until a release ships.
