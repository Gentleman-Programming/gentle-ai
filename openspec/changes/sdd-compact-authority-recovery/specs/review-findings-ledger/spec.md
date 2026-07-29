# Delta for review-findings-ledger

## MODIFIED Requirements

### Requirement: Compact authority pre-verification recovery

After apply and before final verification, `sdd-status` MAY discover one approved compact authority when no local legacy transaction mirror exists. The authority MUST have canonical paths bound exclusively to the active change, an equal terminal receipt, a stable revision, and a live post-apply `allow`. Discovery MUST write nothing and MUST NOT make archive ready.

A failed report MAY become verify-ready only when its strict envelope proves missing review authority was the sole blocker, no command or substantive verification failed, and a different valid authority is now live. Multi-lineage discovery MAY ignore only the exact stale denied revision observed by that report. All other invalid, denied, malformed, ambiguous, unchanged-authority, or substantive cases MUST remain blocked.

#### Scenario: Exactly one valid authority
- GIVEN one approved active-change compact authority returns live post-apply `allow`
- WHEN apply is complete and final verification has not passed
- THEN only verify becomes ready and no mirror is written

#### Scenario: Multi-lineage authority recovery
- GIVEN an authority-only report names a stale denied predecessor revision
- AND one newer valid authority is exactly bound and live
- WHEN readiness is resolved
- THEN the predecessor is preserved and only re-verification becomes ready

#### Scenario: Invalid or ambiguous authority
- GIVEN authority is absent, malformed, denied, cross-change, traversal-shaped, receipt-mismatched, changed during discovery, or multiply eligible
- WHEN readiness is resolved
- THEN verify and archive remain blocked

#### Scenario: Ineligible failed report
- GIVEN commands failed, substantive verification failed, the envelope is malformed, or authority is unchanged
- WHEN readiness is resolved
- THEN recovery is denied

#### Scenario: Authority-only verification envelope
- GIVEN authority preflight alone denies before commands execute
- WHEN `sdd-verify` records the failure
- THEN it emits the five recovery fields, exit `125` for both commands, and exact empty-output hashes
