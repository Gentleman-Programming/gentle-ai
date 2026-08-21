# Delta for review-findings-ledger

## ADDED Requirements

### Requirement: Non-Admitted Reviewer Attempt Persistence and Retry Accounting

The store MUST persist unachievable reviewer attempts under `<storeDir>/reviewer-attempts/%02d-%s-%02d.json` bound to `(LineageID, TargetIdentity, AuthorityRevision, Lens, SelectedOrder, SubjectHash)`. Attempt records MUST NOT occupy, mutate, or satisfy the completed reviewer result slot (`<storeDir>/reviewer-results/%02d-%s.json`). The system MUST track per-slot attempts against a bounded limit (`maxReviewerAttemptsPerSlot = 3`).

#### Scenario: Attempt record saved without occupying completed slot
- GIVEN a provider emits an unachievable attempt envelope for lens `0`
- WHEN attempt capture executes
- THEN the record is persisted under `reviewer-attempts/`
- AND the completed result slot remains vacant

#### Scenario: Readback discovers attempt records for retry accounting
- GIVEN multiple recorded attempts for a lens slot
- WHEN reviewer slot attempts are discovered
- THEN all validated attempt records are enumerated for retry tracking
- AND completed result readback reports the slot as vacant
