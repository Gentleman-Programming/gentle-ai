# Design: Recover SDD Verification from Compact Authority

## Approach

Add a read-only pre-verification bridge in `sdd-status`. It enumerates repository-derived compact stores, validates immutable active-change path binding, loads approved state and its equal receipt, executes the live post-apply gate, and rechecks authority before routing. It never creates a transaction mirror.

For an existing failed verify report, parse a strict leading envelope. Recovery is eligible only when missing review authority was the sole failure, neither declared command ran, both exits are `125`, and the newly eligible authority revision differs from the observed revision. During multi-lineage discovery, only that exact observed stale denied predecessor may be skipped; any unrelated malformed, denied, or competing live authority blocks.

## Decisions

| Decision | Rationale |
|---|---|
| Exact active-change OpenSpec prefix and canonical path equality | Prevent cross-change authority reuse and traversal aliases. |
| Receipt equality plus live post-apply gate | A revision identifier alone is not authority. |
| Verify-only routing with no mirror writes | Discovery cannot manufacture persisted review state or archive readiness. |
| Strict authority-only envelope | Human prose cannot reclassify substantive or command failures. |
| Skip only the report-observed stale denied revision | Supports real multi-lineage recovery without ignoring unrelated denial evidence. |

## Rollback

Revert `internal/sddstatus`, the three embedded asset changes, and this OpenSpec change. Existing reports and compact stores remain valid data.
