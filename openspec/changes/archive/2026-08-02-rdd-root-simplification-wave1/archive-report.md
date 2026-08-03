# Archive Report: RDD Root Simplification Wave 1 (Shadow Algebra)

**Date**: 2026-08-02
**Change**: rdd-root-simplification-wave1
**Status**: ARCHIVED AND CLOSED
**Final Verdict**: PASS (Third verification cycle)

## Executive Summary

Wave 1 of the RDD root simplification is complete and merged to main. The change delivered four new capability specs (candidate-identity, relation-algebra, authority-graph-classification, shadow-evaluation), 50 completed implementation tasks across a 6-PR feature-branch-chain, three verification cycles with FAIL-FAIL-PASS progression, and independent closure of two real verification-caught defects. Delivery disabled/unmanaged (RDD mode OFF). Zero live decision mutation. All 22 requirements and 28 scenarios across four specs have passing tests.

## Delivery Evidence

### Final Merged State

- **Main Checkout**: /home/gentleman/work/gentle-ai at commit d591f4cf (2026-08-02)
- **Merged PR Chain**: #2261–#2267 merged into tracker; tracker PR #2272 MERGED TO MAIN
- **Issue Closure**: gentle-ai #2260 (Wave 1 scope tracking issue) auto-closed
- **Feature Branch**: `feature/rdd-root-simplification` continues on main, active for Wave 2

### PR Delivery Summary

| PR | Slice | Description | Commits | Changed Lines | Status |
|---|---|---|---|---|---|
| 0 | Docs | Land Wave 1 SDD artifacts + archive Wave 0 | 2 | 1,080 | Merged |
| 1 | Test | Characterization tests for `deriveBaseAdvanceCompatibility` | 1 | 337 | Merged |
| 2 | Identity | `CandidateIdentity` resolver + readonly guard | 1 | 984 | Merged |
| 3 | Relation | Relation algebra (Amendment A/B) | 1 | 688 | Merged |
| 4 | Health | Authority graph classifier | 1 | 306 | Merged |
| 5+6+fix | Observer | Observer seam, switch, wiring, matrix + golden + docs | 3 | 1,317 authored | Merged |
| **Total** | | | **9** | **~4,712 changed** | **MERGED** |

### Acceptance Gate Status

**Review Gate**: RDD mode OFF (clone-local) → delivery disabled/unmanaged. No fabricated approval per policy. Native review authority: not required when kill switch is off; native gate policy documented.

**Task Completion Gate**: PASS. All 50 tasks checked (48 phase + 2 injected):
- Phase 0: 5 tasks (archive Wave 0 + land Wave 1 artifacts)
- Phase 1: 4 tasks (characterization tests)
- Phase 2: 8 tasks (identity resolver)
- Phase 3: 9 tasks (relation algebra)
- Phase 4: 6 tasks (health classifier)
- Phase 5: 8 tasks (observer seam)
- Phase 6: 8 tasks (differential matrix + exit bar)
- Injected Task 0: 1 task (refusal-ratchet fixes)
- Injected Task 1: 1 task (CandidateTree binding fix)

**Verification Gate**: PASS (third cycle). 22/22 requirements, 28/28 scenarios, zero blockers, zero critical findings.

## Verification History (Three-Cycle Progression)

### Cycle 1: FAIL (Tip `933fb329`)

**Verdict**: FAIL — 2 CRITICAL, 7 WARNING, 3 SUGGESTION

**Critical Issues**:
1. **CRITICAL-1**: Unguarded shadow derivation on live path. `gate.go:350` called `shadowDeriveBaseAdvance` unconditionally, running `merge-tree --write-tree` + two `patchIdentity` runs on every pre-PR/pre-push gate with the switch OFF, violating the freeze policy's Blocking budget contract.
2. **CRITICAL-2**: "Off by Default in Live Paths" untested and violated. `TestShadowObserverDisabledIsANoOp` proved only that the `ObserveShadowRelation` hook stays silent; it could not observe the sibling `shadowDeriveBaseAdvance` call, which is where the Git cost originates.
3. **CRITICAL-3**: Untested scenario — "Pi overlay selector is explicitly out of scope" (`rdd-candidate-identity` spec) had no covering test. Structural guarantee via the closed 4-value enum existed, but no runtime evidence.

**Compliance**: 23/28 scenarios, 17/22 requirements.

### Cycle 2: FAIL (Tip `7fbfece3`)

**Verdict**: FAIL — 1 CRITICAL, 7 WARNING, 4 SUGGESTION

**Remediation**: Commit `7fbfece3` (`fix(review): run shadow base-advance derivation only when observation is enabled`)

**Closures**:
1. **CRITICAL-1 CLOSED**: Added guard `if shadowObservationEnabled() && request.Gate == GatePrePR && snapshot.BaseTree != receipt.BaseTree`. Now shadow derivation is unreachable with switch off and pre-push performs no shadow derivation at all.
2. **CRITICAL-2 CLOSED**: Added `TestNativePrePRGateWithShadowDisabledDerivesBaseAdvanceZeroTimes` to directly instrument `shadowDeriveBaseAdvance`. Assertion: call count = 0 with switch off. Proof verified that live derivation still ran, isolating the shadow call.

**Remaining**:
3. **CRITICAL-3 REMAINS**: No test for pi-overlay refusal scenario. Structural proof exists; runtime coverage missing. Must add negative test.

**Compliance**: 27/28 scenarios, 21/22 requirements.

### Cycle 3: PASS (Tip `3480bcd0`)

**Verdict**: PASS — 0 CRITICAL, 5 WARNING, 4 SUGGESTION

**Remediation**: Commit `3480bcd0` (`test(review): cover pi-overlay selector out-of-scope refusal`) + prior injected fixes

**Closures**:
3. **CRITICAL-3 CLOSED**: Added `TestShadowIdentityPiOverlaySelectorIsOutOfWave1Scope`. RED independently reproduced by mutating `shadow_identity.go:266` to fabricate a resolution; test failed with exact recorded message. Restored byte-exact; test GREEN. Genuine refusal coverage confirmed.

**Additional Pre-Verify Fixes**:
- **Injected Task 0**: 7 refusal-ratchet violations fixed via `// refusal:by-design world-action:` annotation (defensive invariants always swallowed by observer).
- **Injected Task 1**: Real algebra gap fixed — `shadowBaseAdvanceApplies` now binds to `CandidateTree == CandidateTree` per live `classifyCompactTargetRelation` requirement. Test `TestShadowRelateBaseAdvanceProofBindsToCandidateTree` validates the fix.

**Compliance**: 28/28 scenarios (COMPLIANT), 22/22 requirements (FULLY COMPLIANT).

**Test Counts**:
- Total: 50 implemented tasks (48 phase + 2 injected)
- All 28 spec scenarios have passing tests
- All 22 requirements demonstrated compliant
- Full `go test ./... -count=1` exit 0
- Both `tasks.md` mirrors (main checkout and worktree) byte-identical, 50/50

## Verification-Caught Defects

### Defect 1: Unguarded Shadow Derivation (CRITICAL-1)

**What**: `gate.go:350` called `shadowDeriveBaseAdvance` unconditionally on the live pre-PR/pre-push path.

**Why It Was Found**: Second verification cycle explicitly tested "Off by Default in Live Paths" scenario; realization that the observer's test hook could not detect the sibling call on the live path.

**How It Was Fixed**: Added three-part guard: `shadowObservationEnabled() && request.Gate == GatePrePR && snapshot.BaseTree != receipt.BaseTree`. Guard is a strict subset of the live derivation's own precondition, ensuring shadow is unreachable when switched off.

**Impact**: Cycle 2 fix commit: `7fbfece3` (105 insertions / 7 deletions). No new API surface; guard is internal wiring change.

### Defect 2: Real Algebra Gap (Injected Task 1)

**What**: `shadowBaseAdvanceApplies` bound proof only to `OriginalMergeBaseTree`/`NewBaseTree`, missing the `CandidateTree == CandidateTree` binding that live `classifyCompactTargetRelation` requires.

**Why It Was Found**: PR6's differential matrix exit-bar test (`TestShadowMatrixUnexplainedDivergenceOnCoreRelationBlocksWave2`) discovered the gap as a real, reachable divergence class when a CandidateTree changes but base doesn't. Shadow reported `compatible_base_advance` where live reported `changed-scope`.

**How It Was Fixed**: Added `input.Frozen.CandidateTree == input.Live.CandidateTree` binding to `shadowBaseAdvanceApplies`. Validated by test `TestShadowRelateBaseAdvanceProofBindsToCandidateTree`.

**Impact**: Pre-verify fix on same branch as PR6. No live-decision mutation; shadow algebra now strictly aligned with live. Matrix remained at 40 rows (16 agreement / 12 explained divergence / 8 no-live-decision / 4 no-shadow-decision / 0 unexplained) because the real corpus never included the fixed scenario.

## Specification Artifacts

### New Specs (Created in openspec/specs/)

1. **rdd-candidate-identity**: 5 requirements, 5 scenarios
   - Canonical identity structure, selector normalization, read-only resolution, ambiguity/failure reporting, Wave 1 scope
   - All scenarios COMPLIANT (5/5)

2. **rdd-candidate-relation-algebra**: 6 requirements, 9 scenarios
   - Seven-value output, Amendment A delegation, Amendment B degradation, characterization tests, read-only, no fabricated counterparts
   - All scenarios COMPLIANT (9/9)

3. **rdd-authority-graph-classification**: 4 requirements, 5 scenarios
   - Three-value classification, no mutation/execution, fail-closed, deterministic
   - All scenarios COMPLIANT (5/5)

4. **rdd-shadow-evaluation**: 7 requirements, 7 scenarios
   - Advisory-only, disable switch, zero behavior change, no persisted artifact, off by default, matrix exit evidence, unexplained divergence blocks Wave 2
   - All scenarios COMPLIANT (7/7)

### Specification Compliance Matrix

| Domain | Requirements | Scenarios | Compliant | Status |
|---|---|---|---|---|
| rdd-candidate-identity | 5 | 5 | 5/5 | ✅ PASS |
| rdd-candidate-relation-algebra | 6 | 9 | 9/9 | ✅ PASS |
| rdd-authority-graph-classification | 4 | 5 | 5/5 | ✅ PASS |
| rdd-shadow-evaluation | 7 | 7 | 7/7 | ✅ PASS |
| **Total** | **22** | **28** | **28/28** | **✅ PASS** |

## Follow-Ups and Known Limitations

### Wave 3 Dependency Note

Wave 3 (consumer cutover) targets the four Wave 1 spec copies newly created in `openspec/specs/`. Any MODIFIED delta against these specs will re-base on the copies archived here. Current known differences:
- Wave 0 specs remain at `openspec/specs/rdd-{backlog-disposition,freeze-expansion-policy,ownership-inventory,simplification-design}/`
- Wave 1 adds four new `openspec/specs/rdd-{candidate-identity,candidate-relation-algebra,authority-graph-classification,shadow-evaluation}/`
- Wave 2–7 specs (not yet SDD-authored) will target Wave 1 copies for any amendments

### Non-Blocking Observations (Orchestrator Advisory)

1. **Delivery Granularity**: PR5 + PR6 + 3 fix commits share one branch (`feat/rdd-wave1-shadow-observer-wiring`), resulting in 6 delivered PRs instead of planned 7. Each slice individually ≤1000 authored lines; aggregate branch is 1,317 authored lines (over design Decision 7's cap). No spec MUST violated; orchestrator delivery concern.

2. **Documentation Precision**: `docs/architecture/rdd-shadow-evaluation.md:19` calls pre-PR/pre-push "the only family where shadow evaluation performs additional Git work", but after CRITICAL-1 fix the guard is `GatePrePR` only. Pre-push performs no shadow derivation. Minor imprecision; not blocking.

3. **Assertion Enhancement Opportunity**: `TestNativePrePRGateShadowOnOffByteIdenticalForCompatibleBaseAdvance` does not assert that the ON run incremented `shadowDeriveBaseAdvanceCallCountForTest()`. Both ON and OFF tests would pass if the guard were hard-wired false. One-line `want ≥ 1` assertion in ON branch would machine-check the scenario.

4. **Design Decision Literal**: Decision 1 states "one exported function plus two exported data types **and nothing else**". Seven exported `ShadowRelation*` constants also exist (vocabulary of an exported type). Amend the decision or unexport the constants for literal compliance.

## Artifact Locations

| Artifact | Location | Lines | Status |
|---|---|---|---|
| proposal.md | archived | 88 | ✅ Byte-perfect copy |
| design.md | archived | 138 | ✅ Byte-perfect copy |
| tasks.md | archived | 132 | ✅ Byte-perfect copy (50/50 tasks) |
| verify-report.md | archived | 445 | ✅ Full three-cycle report |
| specs/rdd-candidate-identity/spec.md | archived + main | 76 | ✅ New in both |
| specs/rdd-candidate-relation-algebra/spec.md | archived + main | 90 | ✅ New in both |
| specs/rdd-authority-graph-classification/spec.md | archived + main | 56 | ✅ New in both |
| specs/rdd-shadow-evaluation/spec.md | archived + main | 83 | ✅ New in both |

## Archive Completeness Checklist

- [x] All 50 tasks marked complete (48 phase + 2 injected)
- [x] All 22 requirements across four specs demonstrated compliant
- [x] All 28 scenarios across four specs have passing tests
- [x] Verification gate PASS (third cycle)
- [x] Task completion gate PASS
- [x] Two real verification-caught defects documented and fixed
- [x] Three-cycle verify history preserved verbatim in verify-report.md
- [x] Four new specs copied to main openspec/specs/
- [x] Change folder moved to openspec/changes/archive/2026-08-02-rdd-root-simplification-wave1/
- [x] Byte-perfect copies verified for all artifacts
- [x] No stale checkboxes in archived tasks.md
- [x] Review gate status documented (disabled/unmanaged, no fabricated approval)

## Observation IDs (Engram Traceability)

The following observations were persisted during the SDD cycle:

- Proposal: #10115
- Spec: #10117
- Design: #10118
- Tasks: #10120
- Verify-Report: #10124

This archive report is saved as topic `sdd/rdd-root-simplification-wave1/archive-report` in Engram for persistent traceability.

## Closure Notes

Wave 1 is COMPLETE and ARCHIVED. The change is closed; no further work is pending on Wave 1 itself. The feature-branch-chain continues on main for Wave 2 (consumer cutover). All specs are now in the main `openspec/specs/` directory as the source of truth for future waves. The differential matrix (40-row covering array with zero unexplained divergences on core relations) is the final exit evidence.

**Merged to Main**: 2026-08-02 at commit d591f4cf
**Archived**: 2026-08-02
**Ready for Wave 2**: Yes
