```yaml
schema: gentle-ai.verify-result/v1
evidence_revision: sha256:ccbcc9cc91e283864c4537f52873698bd1f61131f782bb6b1bd30b17edd5dd2b
verdict: fail
blockers: 0
critical_findings: 0
requirements: 9/10
scenarios: 44/45
test_command: go test ./internal/cli ./internal/reviewtransaction/... -count=1
test_exit_code: 0
test_output_hash: sha256:2b2498c47ad624803adae888fd985866152963cdaba1465a6569a8ad4a9aeec2
build_command: GOOS=darwin go build ./...
build_exit_code: 0
build_output_hash: sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855
```

## Verification Report

**Change**: review-lifecycle-hardening
**Version**: N/A (delta spec, amended twice post-design)
**Mode**: Strict TDD

### Completeness

| Metric | Value |
|--------|-------|
| Tasks total | 63 (7 phases + R.1-R.6 + C.15) |
| Tasks complete | 63 |
| Tasks incomplete | 0 |
| Tasks executed but NOT recorded | 1 (A.9) |

### Build & Tests Execution

All five declared commands were executed by the verifier, not trusted from the apply report.

| Command | Exit | Output digest | Runtime |
|---|---|---|---|
| `go run ./internal/gofmtcheck` | 0 | empty output | - |
| `go vet ./...` | 0 | empty output | - |
| `go test ./internal/cli ./internal/reviewtransaction/... -count=1` | 0 | `sha256:2b2498c4...` | cli 82.680s, reviewtransaction 101.867s |
| `go test ./e2e/organicruntime -count=1 -timeout=15m` (run with `-v`) | 0 | `sha256:1128b40f...` | 79.010s |
| `GOOS=darwin go build ./...` | 0 | empty output | - |

**Tests**: 49 PASS blocks in the e2e suite, 0 failures, 2 deliberate platform skips. Both unit packages `ok`.

**Coverage**: not collected - no coverage threshold is configured for this repository. Not a failure.

### Per-Issue E2E Traceability (spec hard requirement)

Enumerated independently from `e2e/organicruntime/organic_lifecycle_hardening_test.go`; every subtest was matched to a runtime result line, not to the apply report.

| Issue | Subtest | Runtime result |
|---|---|---|
| 1778 | `TestOrganicReviewStartDeadlineHeadroom/issue-1778` | PASS (69.48s) |
| published-delivery | `TestOrganicReviewLifecycleErrorTyping/issue-1801-published-delivery` (+2 children) | PASS |
| 1699 | `.../issue-1699` | PASS |
| 1699 (A.9) | `.../issue-1699-id-less-candidate-causal-finding` | PASS |
| 1666 | `.../issue-1666` | PASS |
| 1807 | `.../issue-1807` | PASS |
| 1832 | `.../issue-1832` | PASS |
| 1745 | `TestOrganicReviewExecutableTransitionContract/issue-1745` | PASS |
| 1775 | `.../issue-1775` | PASS |
| 1663 | `.../issue-1663-and-1788/issue-1663` | PASS |
| 1788 | `.../issue-1663-and-1788/issue-1788` | PASS |
| 1800 | `.../issue-1800` | PASS |
| 1812 | `TestOrganicReviewTargetShapeRefusals/issue-1812` | PASS |
| 1771 | `.../issue-1771` | PASS |
| 1641 | `.../issue-1641` | PASS |
| 1781 | `TestOrganicReviewPlatformFailSafes/issue-1781` | SKIP (darwin-only, named reason) |
| 1804 | `.../issue-1804` | SKIP (darwin-only, named reason) |
| 1744 | `TestOrganicReviewRecoveryGraph/issue-1744` | PASS |
| 1816 | `.../issue-1816` | PASS |
| 1782 | `.../issue-1782` | PASS |
| 1813 | `TestOrganicReviewStoreRobustness/issue-1813` | PASS |

**No defect is missing a named, numbered scenario.** 20 distinct issue-numbered subtests exist for 18 numbered defects plus the unnumbered published-delivery blocker plus the community-reported 1832. No group-level journey was accepted as a substitute for a per-issue scenario.

### Spec Compliance Matrix

| Requirement | Scenarios | Result |
|---|---|---|
| Compact recovery edge admission (ADDED) | 6 | 6 COMPLIANT |
| Start-scoped operation deadline (ADDED) | 1 | 1 COMPLIANT |
| Per-issue E2E traceability and typed escape naming (ADDED) | 2 | 1 COMPLIANT, 1 PARTIAL (see W4) |
| Deterministic lifecycle validation (MODIFIED) | 12 | 12 COMPLIANT |
| One ordinary correction transaction (MODIFIED) | 2 | 2 COMPLIANT |
| Native machine-readable entry points (MODIFIED) | 7 | 7 COMPLIANT |
| Exact persistence references (MODIFIED) | 4 | 4 COMPLIANT |
| Crash-safe bounded writer ownership (MODIFIED) | 4 | 3 COMPLIANT, 1 PARTIAL (ENOTSUP, see W1) |
| Complete current-changes snapshot (MODIFIED) | 5 | 5 COMPLIANT |
| Terminal receipt (MODIFIED) | 2 | 2 COMPLIANT |

**Compliance summary**: 44/45 scenarios have a covering test that passed at runtime on this platform. The one PARTIAL is the `ENOTSUP` rename fallback, whose covering tests exist and type-check but cannot execute on Linux.

### Safety-Critical Claims - Independently Verified Against Code

**1. Group C / 1744 - CLI pre-gate relaxation.** Verified at `internal/cli/review_facade.go:697-700`. The surviving predicate is exactly `(*committedOnly != (base != "")) && !explicitOverlayBase`; the predecessor-kind coupling clause `baseDiff != *committedOnly` is gone. `baseDiff` is defined at `:681` as `predecessorRecord.State.InitialSnapshot.Kind == TargetBaseDiff`, confirming the removed clause tied the requested flag shape to the predecessor's kind - precisely the defect. `compactRecoveryTargetKindAdmissible` was correctly NOT added (superseded by maintainer ruling).

All five amended-spec protections are enforced in code and covered by negative tests:

| Protection | Enforcement site | Negative test |
|---|---|---|
| Identity-bound maintainer authorization | `compact_store.go:477` via `compactRecoveryAuthorizationBinding(predecessorLineage, predecessorRevision, successor.InitialSnapshot.Identity, actor, reason)` - binds all three required elements | `TestReviewRecoverEscalatedRequiresExactSuccessorBoundAuthorization` |
| Invalidated requires invalidated predecessor | `compact_store.go:456-459` | `TestReviewRecoverInvalidatedRequiresInvalidatedPredecessor` |
| Base-diff predecessor pinned to frozen base tree | `review_facade.go:737-740` - fires when `baseDiff` (predecessor is base-diff) and rejects `snapshot.BaseTree != predecessor.InitialSnapshot.BaseTree` | `TestReviewRecoverBaseDiffPredecessorStillBindsFrozenBaseTree` |
| Argv coherence | `review_facade.go:697-700` (retained verbatim) | `TestReviewRecoverArgvCoherenceStillRejectsMismatchedBaseRefFlags` |
| Successor never inherits approval | `compact_store.go:415-417` generation must be `predecessor+1`; successor built through `NewCompactState`, a fresh non-terminal state; `:741-743` rejects an unchanged identity | covered by the 1744 journey plus escalated/invalidated negatives |

Also confirmed: substantive scope change still required for approved scope-changed recovery (`compact_store.go:442-444`), and escalated recovery still requires `compactEscalatedRecoveryTargetChanged` (`:474-476`).

**2. Group D / 1813 - quarantine narrowness.** `compactLineageQuarantinable` (`compact.go:56-67`) returns true only when `errors.As` matches `*CompactSemanticStateError` AND the state is in `{Approved, Escalated, Invalidated}`. The type has a single construction site (wrapping `State.Validate()`), so checksum/IO/parse failures can never satisfy it. Non-terminal states fall to `default: return nil, false`.

- Explicitly-selected lineages still fail closed: `target_status_projection.go:79-91` returns `loadErr` unconditionally in the `else if store, ok := storeByLineage[lineageID]` branch, and the unfiltered `storeByLineage` map is deliberately preserved so a caller naming the quarantined lineage still resolves it and fails.
- `report.Complete` / `Authoritative` are NOT flipped: `status.go:198-206` diverts the diagnostic into `result.diagnostics` and `continue`s without appending the entry or touching `result.complete`. The aggregate loop at `status.go:144-148` therefore never sees the quarantined lineage, and `report.Authoritative = report.Complete` (`:150`) is unaffected.
- Four production call sites, all enumeration-only: `compact_store.go:570` (`CompactAuthorityLeaves`), `:771` (`StartCompactAuthority`, the deviation found during apply), `target_status_projection.go:68` (selector-free branch only), `status.go:264` (diagnostic).

**3. Group A + C.15 - corrupted/ambiguous still fails closed while disabled.** Verified the ordering in `discoverCompactFacadeGateReview`:
- `store.Load()` failure returns `ReviewAuthorityCorrupted` at `review_facade.go:2139` **before** any target resolution is attempted, so genuine corruption can never be reclassified as target-unresolvable.
- Receipt read/parse/equality failures return corrupted at `:2146` and `:2151`.
- Mixed outcomes stay corrupted: `:2275-2277` returns `ReviewAuthorityCorrupted` whenever target-resolution failures are partial; `:2269-2271` does the same for `scopeWithoutContext`/`assessmentUnknown`; `:2240-2254` returns `ReviewReceiptAmbiguous` for multi-class candidates.
- Only the total case `len(targetResolution) == terminalCount` (`:2272`) surfaces the raw typed error. I checked the index-out-of-range edge: `terminalCount == 0` returns `ReviewReceiptMissing` at `:2236-2238`, so `targetResolution[0]` is unreachable when empty. No panic.
- `reviewReceiptDiscoveryIsUnmanagedWhileDisabled` (`:2882-2889`) admits only `{Missing, Unrelated, ScopeChanged, TargetUnresolvable}`. Ambiguous and Corrupted are excluded.
- The enabled-mode denial is unchanged: `:2028-2033` is reached by fallthrough with Stage `target-resolution`, Code `target_resolution_failed`, pinned by `TestReviewValidateDeniesNoUpstreamTargetResolutionWhileEnabled`.

**4. Group E / 1778 - START deadline.** `reviewFacadeStartOperationTimeout` is a `const` (120s) at `review_facade.go:272`. The shared `reviewFacadeOperationTimeout` remains a `var` at 25s, byte-identical, and the three pre-existing test files that mutate it directly still compile and pass. The selector has exactly one production call site (`:323`) and returns the shared value for every non-`review.start` operation. The table test pins status/finalize/validate/unknown to the shared value and asserts the start constant is strictly greater.

The `issue-1778` E2E is the strongest non-tautological test in this change: it builds a 5000-file candidate, asserts the call **succeeded**, and asserts `elapsed > 25s` - so it fails if the fix is reverted (START would be cut off) *and* fails if the fixture stops exercising the path (elapsed drops under 25s). Measured 69.48s.

### Regression Contract

| Spec regression scenario | Covering test | Real or tautological? |
|---|---|---|
| Pre-push before push still allows | `TestReviewValidateReportsDisabledUnmanagedDeliveryWithoutReceipt`, `TestReviewValidateWithoutReceiptStillDeniesWhileReviewIsEnabled` | Real - asserts allow plus the enabled-mode counterpart |
| `--committed-only` unaffected | `TestReviewRecoverRetainsCommittedOnlyBaseDiffAndIgnoresWorkspace` (pre-existing, unmodified), `TestReviewRecoverArgvCoherenceStillRejectsMismatchedBaseRefFlags` | Real - unmodified pre-existing test plus new negative |
| Governing receipt still allows | `TestReviewValidateKeepsGoverningReceiptAuthoritativeWhileDisabled` | Real |
| Disabled/unmanaged codes byte-identical | 4 tests incl. `...WithPriorReceipt`, `...OverDeliveredWorkspaceReceiptAtPrePush`, `TestReviewValidateDeniesDeliveredWorkspaceReceiptPrePushAsScopeMismatchWhileEnabled` | Real - field-level pinning via `wantEnabledReviewGateFields` |
| Corrupted/ambiguous still fails closed | `TestReviewValidateKeepsFailingClosedOnCorruptedAuthorityWhileDisabled{,AtPrePush,NoUpstream}`, `TestCompactQuarantineNeverAppliesToNonTerminalOrStructuralCorruption`, `TestInventoryAuthorityReportsActiveMalformedAndMixedCollisionWithoutMutation` | Real - truncated `review-state.json` fixtures, distinct failure classes |
| Real chain convergence still rejected | `TestCompactPrePRChainRejectsForkConvergenceAndCycle/convergence` | Real - verified independently that `addCompactChainConvergence` (`compact_chain_test.go:950-956`) injects at `fixture.commits[0]`, i.e. mid-chain, never at `fixture.base`, so the root exemption cannot mask it |

Additionally confirmed the 1782 exemption is scoped correctly: at `compact_chain.go:446-462`, the fork guard (`len(adjacency[node]) != wantOutgoing`) runs **before** the `continue`, so the chain root is still fork-checked. Only the incoming-degree check is skipped.

### Scope Discipline

- No new frameworks or subsystems. `Token` and `Cause` are additive optional JSON fields; `AuthorityInventoryDiagnostic` already existed. No schema version bump.
- 1778 shipped as a constant plus a selector, not a configurable timeout framework (design decision 12 honored).
- 1745 shipped as an additive `Token` field, not a flag-parser change (decision 8 honored).
- 1812 refuses instead of implementing index freezing; 1641 refuses instead of implementing empty-base publication. Both convert broken silence into typed refusals, as the proposal required.
- Typed refusals naming concrete commands verbatim: 1812 names `gentle-ai review start --projection staged` and `gentle-ai review start --base-ref <ref> --committed-only` (`review_facade.go:975-977`); 1788 names `gentle-ai review capture-evidence` then `gentle-ai review finalize --lineage <id> --captured-evidence` with the real lineage interpolated (`:1834-1839`).

### Accepted Deviations - Verified Against the Amended Spec

| Deviation | Verdict |
|---|---|
| Push-target failures typed as target-resolution rather than scope-changed | MATCHES the amended spec scenario "Untyped push-target ambiguity types as target-resolution failure". Supersedes both the stale task 2.2 wording and design ladder row `:752`. Verified the only `GateTargetResolutionError` construction site is `gate.go:555`. |
| 1812's refusal naming BOTH escapes | MATCHES. The spec requires "names the plain staged-projection escape verbatim"; that escape is present verbatim, and the second is additive. |
| 1641's anchor moved to `compact_gate.go:195-197` | Message reads exactly "commit an authorized empty root, then run committed base-diff review", matching the Terminal-receipt requirement verbatim. See W4 for a spec-internal tension. |
| C.15 routing inside the existing `errors.As` branch | Verified purely additive; the enabled-path code is unchanged and reached by fallthrough. |

### Untouched Work

All five protected files carry only the separately-approved #1825/#1827 deltas, not this change's edits. Verified by content, not by status:

| File | Content of the working-tree delta | Verdict |
|---|---|---|
| `internal/cli/review_mode.go` | consent-tier evidence phrasing, explicitly annotated `(issue #1827)` | untouched by this change |
| `internal/cli/review_start_evidence_test.go` | adds `TestReviewConsentRiskEvidence*` (consent tiers) | untouched by this change |
| `internal/cli/run.go`, `internal/cli/run_component_paths_test.go` | component-path work (#1825) | untouched by this change |
| `internal/components/agentguidance/inject.go` | #1825 | untouched by this change |

Corroborating signal: all five have mtimes of 2026-07-25 21:17-21:19, preceding this change's later apply work (`review_facade.go` 07-26 01:32, hardening E2E 01:29).

### TDD Compliance

| Check | Result | Details |
|-------|--------|---------|
| TDD Evidence reported | PASS | apply-progress and tasks.md record RED-first per issue, with `git stash` revert-and-rerun proof cited for C.15, 6.6, 6.9, 6.13, 7.1 |
| All tasks have tests | PASS | every phase task names its unit and/or E2E test; all named tests were located in the tree |
| RED confirmed (tests exist) | PASS | 5 new test files plus 15 modified, all present |
| GREEN confirmed (tests pass) | PASS | all located tests executed and passed, except the 4 darwin-gated ones (W1) |
| Triangulation adequate | PASS | Group C carries 4 negative tests beside 1 positive; Group D table-tests nil/generic/wrapped-non-semantic; Group F covers ENOTSUP, never-replaces, and other-errno passthrough |
| Safety Net for modified files | PASS | apply reports zero edits to pre-existing tests except one `reflect.DeepEqual` expectation update (task 3.13); full-suite sweeps gated phases 6 and 7 |

**Assertion quality**: no tautologies, ghost loops, or assertion-free tests found in the new or modified test files. Spot-audited `compact_release_scope_parent_test.go` in full: it pre-validates its own fixture shape (`len(parents) != 2` / `!= 1`) before asserting, so it cannot pass vacuously, and asserts concrete tree values rather than mere non-nil. The `issue-1778` E2E asserts a lower time bound, which is a genuine regression detector.

### Test Layer Distribution

| Layer | Files | Notes |
|-------|-------|-------|
| Unit | 4 new + 15 modified | `internal/cli`, `internal/reviewtransaction` |
| E2E | 1 new (`organic_lifecycle_hardening_test.go`, 54 KB) | real CLI invocation via the organic harness |

### Quality Metrics

**Formatter**: PASS (`go run ./internal/gofmtcheck`, exit 0, empty output)
**Vet**: PASS (`go vet ./...`, exit 0, empty output)
**Cross-compile**: PASS (`GOOS=darwin go build ./...`, exit 0; `GOOS=darwin go vet ./internal/reviewtransaction/` also exit 0, so the darwin-only test file type-checks)

### Issues Found

**CRITICAL**: None.

**WARNING**:

- **W1 - 1804's unit proof does not run on Linux, and the code says it does.** `internal/reviewtransaction/publish_immutable_darwin_test.go` carries `//go:build darwin`, so all four `TestPublishNoReplaceDarwin*` tests are excluded on Linux. I confirmed this by running the filter directly: only `TestSecureOpenLockParentAnchor` executed. The `issue-1804` E2E skip message nonetheless states "See TestPublishNoReplaceDarwinENOTSUPFallsBackToCopy for the mocked-syscall unit proof **that runs here**", and the file-level comment claims the mocked-syscall coverage "does run on every platform including Linux CI". Both statements are false for 1804 on Linux. Net effect: the ENOTSUP fallback has **zero executed proof** on this platform - E2E skipped and unit tests build-excluded. It does type-check under `GOOS=darwin`. By contrast 1781 is genuinely proven on Linux: `TestSecureOpenLockParentAnchor` is `//go:build unix` and passed, and `TestNegotiatedStoreLockPreAcquisitionFailureIsNotStarted` lives in `internal/cli/review_failure_contract_test.go` with no build tag and passed. Not a blocker per the accepted Group F fail-safe posture, but the two comments overstate coverage and should be corrected before the claim reaches a reader.
- **W2 - `tasks.md` references a "Deviations section below" that does not exist.** Tasks 2.2, 4.1, 4.8, and 4.9 all defer to it. The deviations are described inline and in apply-progress, so no information is lost, but four dangling cross-references remain in the artifact.
- **W3 - Task A.9 is not recorded in `tasks.md`.** The second, deeper 1699 fix shipped and is proven by `issue-1699-id-less-candidate-causal-finding` (PASS), but no A.9/2.9 entry exists anywhere in the tasks artifact - unlike C.15, which received its own "Added after community report" section. Tasks do not fully match code state.
- **W4 - Spec-internal contradiction on 1641's escape naming.** The "Per-issue E2E traceability and typed escape naming" requirement mandates that every typed refusal "names the concrete escape command verbatim ... never only a prose description the caller must translate into a command". 1641's message is "commit an authorized empty root, then run committed base-diff review" - a prose description, and one the Terminal-receipt requirement itself dictates word-for-word. The implementation satisfies the Terminal-receipt requirement exactly and cannot satisfy both as written. 1812 and 1788 do name real commands. Either 1641's message should name commands (for example `git commit --allow-empty` followed by `gentle-ai review start --base-ref <root> --committed-only`) or the cross-cutting requirement should carve 1641 out.
- **W5 - Minor spec contradiction on scenario counting.** The traceability requirement body says each issue must have "at least one" named scenario, while its scenario says "exactly one". 1699 has two subtests (the second is A.9's). The requirement is satisfied; the scenario as literally worded is not.
- **W6 - `review_facade.go:345` reports the wrong deadline for START.** The aggregate timeout cause is built as `&GitCommandTimeoutError{Timeout: reviewFacadeOperationTimeout, ...}`, which is the 25s shared value even when the operation is `review.start` under the 120s constant. A START timeout would therefore report a deadline it did not use - a misleading diagnostic, which is the exact defect class this change set out to eliminate.

**SUGGESTION**:

- **S1** - `status.go:198-206` skips `result.locks` along with the quarantined entry, so a stale or ambiguous lock held by a quarantined terminal lineage becomes invisible to `review status`. Low impact (the lineage is terminal, and reclaim targets an explicit lineage ID), but the lock evidence is arguably still worth surfacing beside the diagnostic.
- **S2** - The 1782 root exemption is unconditional, so two genuinely distinct historical chains that both end at the publication base would both be ignored rather than flagged. This is backstopped by the fork guard (still applied to the root) and accepted under design decision 4; worth recording as a known residual rather than an oversight.
- **S3** - `issue-1778` consumes 69.48s of the suite's 79.01s total. Correct and deliberately self-guarding, but it dominates e2e wall time and is worth a `testing.Short()` guard if the suite is ever put on a fast path.

### Verdict

**FAIL - ATTESTATION INCOMPLETE (no defect found, 0 blockers, 0 critical findings)**

This verdict records a coverage-attestation gap, not a broken implementation. Every safety-critical claim in this change was verified against the code and holds. The recovery-graph relaxation removed only the predecessor-kind coupling and left all five amended-spec protections enforced and negatively tested; the 1813 quarantine is genuinely narrow and cannot reach non-terminal, structural, or explicitly-selected lineages; corrupted and ambiguous authority still fails closed ahead of every reclassification introduced here; and the START deadline is a constant that leaves the shared timeout byte-identical. All 18 numbered defects plus the published-delivery blocker and the community-reported 1832 carry named, numbered E2E scenarios. All five declared commands pass with exit 0.

The envelope reports `fail` for one reason only: 1 of 45 scenarios - "ENOTSUP rename falls back to exclusive-create and rename (1804)" - has **no covering test that executed at runtime on this platform**, so the "Crash-safe bounded writer ownership" requirement cannot be counted complete. Its four covering tests are real and type-check under `GOOS=darwin`, but `publish_immutable_darwin_test.go` carries `//go:build darwin` and is excluded on Linux, while the `issue-1804` E2E skips on Linux. The verify contract admits a passing verdict only on fully complete evidence, and asserting 45/45 here would be the same overstatement this report flags as W1.

This is explicitly **not** a rejection of Group F's accepted fail-safe posture. 1781 is genuinely proven on Linux (`TestSecureOpenLockParentAnchor` and `TestNegotiatedStoreLockPreAcquisitionFailureIsNotStarted` both executed and passed). Only 1804 lacks executable proof here.

**Remediation to reach a clean pass - neither requires a code fix:**

1. Execute `go test ./internal/reviewtransaction -run TestPublishNoReplaceDarwin -count=1` on a darwin runner (or add a darwin CI job) and rerun verification. That alone moves the count to 45/45 and 10/10.
2. Correct the two inaccurate coverage comments identified in W1, which currently claim the 1804 unit proof "runs here" and executes "on every platform including Linux CI".

**What the community must verify (CI cannot):** `review start`/`finalize` under a macOS managed configuration profile restricting `/` traversal (1781), and the same flows with the Git common directory hosted on an ExFAT volume, exercising the real ENOTSUP copy fallback (1804).

The remaining warnings (W2-W6) are documentation-accuracy, artifact-traceability, and diagnostic-precision defects. None is behavioral and none blocks archive on its own.
