# Apply Progress: Recover SDD Verification from Compact Authority

**Mode**: Strict TDD
**Delivery**: Single PR `size:exception` (800 authored-line ceiling)

## Completed Tasks

- [x] 1.1 Add RED cases for absent, exact, invalid, denied, foreign, traversal, and ambiguous authority.
- [x] 1.2 Add multi-lineage coverage for an observed stale predecessor and a newer live authority.
- [x] 2.1 Implement canonical active-change path binding and stable compact authority discovery.
- [x] 2.2 Route exactly one valid authority to verify only without synthesizing mirrors.
- [x] 3.1 Parse strict authority-only failed-report evidence and require changed authority.
- [x] 3.2 Keep substantive, command-failed, malformed, unchanged, and unrelated denied lineages blocked.
- [x] 4.1 Update embedded `sdd-verify` instructions, report format, and asset contract tests.
- [x] 5.1 Run focused tests, full tests, vet, formatting, and diff checks.

## TDD Cycle Evidence

| Task | Test File | Layer | Safety Net | RED | GREEN | TRIANGULATE | REFACTOR |
|---|---|---|---|---|---|---|---|
| 1.1 | `internal/sddstatus/bounded_review_test.go` | Integration | `go test ./internal/sddstatus ./internal/assets` — PASS before this remediation | Existing RED cases cover absent/exact/foreign/traversal/ambiguous authority | Focused compact-discovery cases pass | Exact authority plus fail-closed variants | `skipsObservedStalePredecessor` keeps the exceptional rule explicit; focused tests stay green |
| 1.2 | `internal/sddstatus/bounded_review_test.go` | Integration | Same passing baseline | Added `TestObservedRevisionSkipsOnlyScopeChangedPredecessor` before production code; `go test ... -run '^TestObservedRevisionSkipsOnlyScopeChangedPredecessor$'` failed to build: undefined helper | Same command passed after the minimal helper | Matching scope-changed allows; matching invalidated, escalated, allow, and mismatched revisions deny | Extracted the narrow predicate, then reran focused recovery tests green |
| 2.1 | `internal/sddstatus/bounded_review_test.go` | Integration | Same passing baseline | Existing RED fixtures exercise canonical active-change binding and unstable/invalid authority paths | Focused compact-discovery cases pass | Active, foreign, traversal, receipt-mismatch, denied, and ambiguous stores | No further refactor needed beyond the explicit stale-predecessor predicate |
| 2.2 | `internal/sddstatus/bounded_review_test.go` | Integration | Same passing baseline | Existing RED fixture requires verify-only routing and no synthesized mirror | `TestResolveBridgesExactlyOnePathBoundCompactAuthorityToVerifyOnly` passes | Valid bridge versus invalid/ambiguous bridge routes | No behavior-changing refactor needed |
| 3.1 | `internal/sddstatus/bounded_review_test.go` | Unit | Same passing baseline | Existing strict-envelope cases reject non-125 and non-empty-output evidence | `TestAuthorityOnlyFailedReportRequiresStructuredFailClosedEvidence` passes | Valid changed authority versus unchanged authority | No behavior-changing refactor needed |
| 3.2 | `internal/sddstatus/bounded_review_test.go` | Integration | Same passing baseline | Existing denial cases plus the new observed-revision RED case define fail-closed routing | Recovery and rejection focused tests pass | Command failure, malformed evidence, unchanged authority, invalidated, escalated, and unrelated revisions remain denied | Narrow predicate removes the overbroad revision-only bypass |
| 4.1 | `internal/assets/assets_test.go` | Unit | Same passing baseline | Existing asset contract test requires the strict authority-preflight envelope | `TestSDDVerifyAuthorityPreflightDenialEnvelopeContract` passes | Instructions and report-format assets assert the same fields and paired `125`/empty-output contract | No refactor needed; assets remain contract-aligned |
| 5.1 | `internal/sddstatus/bounded_review_test.go`, `internal/assets/assets_test.go` | Integration | Same passing baseline | The remediation test was RED before its implementation helper existed | Focused tests pass after GREEN and refactor | Focused recovery, full suite, vet, format, and diff hygiene provide independent checks | Focused suite rerun after the helper extraction |

## Work Unit Evidence

| Evidence | Result |
|---|---|
| Focused test command | `go test ./internal/sddstatus -run '^(TestObservedRevisionSkipsOnlyScopeChangedPredecessor|TestResolveRecoversWithObservedStaleAndNewLiveCompactLineages|TestResolveRejectsReceiptMismatchAndNonAllowCompactBridge)$'` — PASS (0.226s) |
| Runtime harness | Temporary Git repositories and compact stores in `bounded_review_test.go`; the multi-lineage resolver runs against real discovered stores — PASS |
| Rollback boundary | Revert `internal/sddstatus/review_gate.go`, `internal/sddstatus/bounded_review_test.go`, and this apply-progress update; no authority store, receipt, or verify history is mutated |

## Verification

- The historical `verify-report.md` remains a **FAIL** record until a separate re-verification writes fresh evidence.
- Final commands for this remediation are recorded after they run; their output is not retroactively substituted into the historical verify report.
- E2E is N/A: this changes internal status routing and its runtime boundary is the temporary Git-store harness above.
