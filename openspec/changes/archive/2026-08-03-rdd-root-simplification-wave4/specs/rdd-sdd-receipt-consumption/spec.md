# RDD SDD Receipt Consumption Specification

## Purpose

Define how SDD consumes RDD review outcomes as a pure consumer: it persists nothing but a terminal `ReceiptRef` plus its own work-unit attempts, never re-derives gate meaning, and never mirrors review-authority state (`gentle-ai.sdd-review-binding/v1` retires as a write target).

## Requirements

### Requirement: ReceiptRef-Only Persistence

SDD MUST persist, per lineage attempt, only a terminal `ReceiptRef` as its review-state footprint. SDD MUST NOT write new `gentle-ai.sdd-review-binding/v1` records. (Issue #1013)

#### Scenario: Approved lineage persists only a ReceiptRef

- GIVEN a lineage that finalized with an approved receipt
- WHEN SDD records the outcome
- THEN SDD's runtime ledger holds a `ReceiptRef` for that attempt
- AND no `gentle-ai.sdd-review-binding/v1` file is created or updated

#### Scenario: Existing binding files remain read-only

- GIVEN a pre-Wave-4 `gentle-ai.sdd-review-binding/v1` file from an in-flight change
- WHEN SDD loads status for that change
- THEN SDD parses the file read-only for compatibility
- AND SDD never writes to that file again

### Requirement: No Re-Derived Gate Meaning

SDD MUST NOT compute review-gate meaning (allow/deny/escalated/disabled) from local heuristics. SDD MUST request validity through one native validation entry point exposed by the RDD facade. (Issue #1204)

#### Scenario: Gate meaning requested via facade

- GIVEN an archive attempt that needs review disposition
- WHEN SDD evaluates whether delivery is governed
- THEN SDD calls the single native validation entry point with the `ReceiptRef`
- AND SDD does not re-implement gate-result logic in `internal/sddstatus`

#### Scenario: No local re-derivation on stale receipt

- GIVEN a `ReceiptRef` that no longer resolves to a valid receipt
- WHEN SDD queries validity
- THEN SDD surfaces the facade's typed answer verbatim
- AND SDD does not substitute a locally computed gate result

### Requirement: Attempt Ledger Ownership Stays With SDD (Maintainer-Confirmed, 2026-08-02)

Decision 9 is RATIFIED (maintainer-confirmed, 2026-08-02): SDD retains ownership of its own work-unit attempts in `runtime_ledger.go`, because `previous_revision` chaining, CAS `expected_revision`, and `request_digest` replay identity already satisfy durable cumulative-record properties; RDD owns only the receipt. `RuntimeObjective` MUST be the single named work-unit owner across `runtime_ledger.go` and `runtime_compact.go`, closing CON-08's split-ownership gap; `CompactAcquireRequest`'s work-unit fields MUST collapse into `BeginAttemptRequest`.

#### Scenario: Attempts remain in SDD's runtime ledger

- GIVEN Wave 4 lands with decision 9 ratified
- WHEN a work-unit attempt completes
- THEN it is appended to `runtime_ledger.go`'s CAS chain
- AND RDD's authority store holds no duplicate attempt record

#### Scenario: One owner named for compaction and ledger

- GIVEN `runtime_ledger.go` and `runtime_compact.go` both touch work-unit scope
- WHEN ownership is documented
- THEN exactly one named component/owner is recorded for both files
- AND no second, competing ownership claim exists

### Requirement: Legacy `reviewGate` v1 Field Compatibility

`status_v1.go`'s legacy `reviewGate` structured field MUST remain readable for unmigrated Pi clients when a review was actually discovered for the candidate, and MUST be structurally absent from the wire (not merely `null` — the key itself omitted) in every other case, including when the kill switch is ON with no receipt at all. Removal is deferred to Wave 7.

**Amendment (corrective verify cycle 5, 2026-08-03, CRITICAL-E): "kill switch ON" alone no longer implies population.** Cycle 4's own amendment to `rdd-post-verify-review-offer`'s "Decline Proceeds to Unmanaged Ordinary Archive" requirement (BLOCKER-1) ratified that the switch being ON, verify having passed, and no receipt existing is decline-by-absence-of-action: `reviewGate` stays structurally absent in that state, in the SAME status output that carries the present `reviewOffer` invitation. The original text of this requirement predates that amendment and still said the field is populated whenever the switch is ON, which the measured behavior contradicts (switch ON + genuinely missing receipt -> `reviewGate` absent). Same amendment rationale as BLOCKER-1: the offer is an invitation, never a gate, so "the switch is on" is not by itself a reason to fabricate or expect a populated disposition — only an actually DISCOVERED review artifact (governing or broken) populates the field. This keeps `rdd-sdd-receipt-consumption` and `rdd-post-verify-review-offer` consistent with each other, closing the same W2-class drift the wave already had to fix once in cycle 2.

#### Scenario: Legacy field present when a review is discovered

- GIVEN an unmigrated Pi client requests status v1 and a review was actually discovered for the candidate (a governing receipt that validates, or one that was discovered and failed validation)
- WHEN status is serialized
- THEN the legacy `reviewGate` field is populated for compatibility, carrying that receipt's own result

#### Scenario: Legacy field absent when disabled

- GIVEN the kill switch is OFF
- WHEN status v1 is serialized
- THEN the legacy `reviewGate` field is omitted from the response

#### Scenario: Legacy field absent when enabled with no receipt (decline)

- GIVEN the kill switch is ON, verify has passed, and no receipt exists for the candidate
- WHEN status v1 is serialized
- THEN the legacy `reviewGate` field is omitted from the response, in the same output that carries a populated `reviewOffer`

### Requirement: ReceiptRef Lives in SDD's Runtime Ledger, Not a New Artifact

The `ReceiptRef` MUST be stored inside SDD's existing runtime ledger record for the attempt. SDD MUST NOT introduce a new standalone OpenSpec artifact file to hold it, since a dedicated file recreates the mirror this wave removes.

#### Scenario: ReceiptRef stored in the runtime ledger

- GIVEN a finalized review outcome
- WHEN SDD records it
- THEN the `ReceiptRef` is a field on the existing runtime ledger attempt record
- AND no new `openspec/.../receipt-ref.*` file type is introduced
