# RDD Review-Context Validation Specification

## Purpose

Define the single read-only review-context evaluation path used by the five integration hooks (`post-apply`, `pre-commit`, `pre-push`, `pre-pr`, and `release`) for every lineage. The path replaces gate-side authority mutation, composed authority, and per-hook/per-lineage forks with a derived review-integrity result over the shared relation algebra. Its result is review-only evidence: ordinary repository and SDD policy alone decide commit, push, PR, release, and archive readiness.

## Requirements

### Requirement: One Read-Only Path For All Review-Context Hooks And Lineages

Every review-context hook MUST evaluate every lineage — new and legacy — through one ordered contract: capability admission, kill-switch check, review-authority discovery when enabled, relation, then review result plus repair guidance. No hook MUST branch on hook identity or lineage kind beyond supplying its own boundary descriptor. The returned review result MUST NOT authorize, deny, block, or otherwise route delivery.

#### Scenario: Pre-PR uses the same review-context evaluation as the other hooks

- GIVEN a pre-PR boundary and a post-apply boundary for an equivalent receipt
- WHEN each hook evaluates review context for its candidate
- THEN both invoke the identical ordered evaluation, differing only in boundary descriptor
- AND both results remain informational to delivery

#### Scenario: Legacy lineage uses the same path as a new lineage

- GIVEN a legacy-lineage candidate at any review-context hook
- WHEN the hook evaluates it
- THEN it uses the same relation-algebra path as a new-lineage candidate, not a separate discovery function
- AND the result changes no ordinary delivery decision

### Requirement: Kill Switch Is Consulted Before Review Authority Is Read

The kill switch MUST be consulted before any review-authority read. When off, the hook MUST report no active review context without letting a missing, stale, ambiguous, or corrupted authority read alter that status. Ordinary repository policy continues to own delivery. (Supersedes #2222/#2239.)

#### Scenario: Kill switch off short-circuits before authority discovery

- GIVEN the kill switch is off
- WHEN a review-context hook is invoked
- THEN it reports no active review context before attempting any receipt or authority read
- AND it does not add a delivery requirement

#### Scenario: Corrupted authority is irrelevant while disabled

- GIVEN the kill switch is off and the authority store is ambiguous or damaged
- WHEN a review-context hook is invoked
- THEN it returns without surfacing the underlying authority error as a delivery condition
- AND ordinary repository policy remains unchanged

### Requirement: Six Prohibited Review-Validation Actions

A review-context evaluation MUST NOT: (1) start review; (2) create correction authority; (3) consume a budget; (4) invalidate a receipt by mutating authority; (5) create a recovery lineage; or (6) compose an unrelated receipt graph to invent review authority. Each prohibition holds for every hook and lineage kind. Review integrity may fail closed inside the review lifecycle, but no prohibited action or result may become a delivery gate.

#### Scenario: Diverged receipt writes no invalidation record

- GIVEN a receipt whose live boundary has diverged from its persisted claim
- WHEN review context is evaluated
- THEN it reports a derived mismatch for lifecycle repair, with no writer lock acquired and no state rewritten
- AND the mismatch does not block archive or delivery

#### Scenario: Pre-PR mismatch does not compose a receipt graph

- GIVEN a pre-PR base mismatch
- WHEN the review-context hook reports its result
- THEN it does not construct or return a composed receipt graph as an alternate authority
- AND it leaves delivery to ordinary repository policy

### Requirement: Every Review-Integrity Result Carries Repair Guidance

Every non-exact review-integrity result MUST include either a typed transition for explicit review work or a stop response with a `reason_code`; it MUST never be a bare result. Repair guidance is limited to the review lifecycle and does not deny delivery.

#### Scenario: Base mismatch names a review next step

- GIVEN a `changed` relation at any review-context hook
- WHEN the result is emitted
- THEN it includes a typed transition such as starting a new review candidate
- AND the transition is optional review work, not a delivery prerequisite

#### Scenario: Unknown relation stops with a reason code

- GIVEN an `unknown` relation
- WHEN the result is emitted
- THEN it stops and returns a `reason_code` rather than a silent or unexplained review result
- AND ordinary delivery policy is unaffected

### Requirement: Receipt File Persists On Derived Review Mismatch

The persisted receipt file MUST NOT be deleted as a side effect of review-context evaluation. An `invalidated` result MUST be derived by comparing the live boundary against the persisted receipt at evaluation time, never recorded as new stored state. This preserves repairable review evidence without treating file presence or the result as delivery authority.

#### Scenario: File-absence-as-signal regression is closed

- GIVEN a receipt that would have been `os.Remove`d under the pre-cutover invalidation writer
- WHEN review context is evaluated post-cutover
- THEN the receipt file remains present on disk and the hook reports its derived mismatch
- AND any reader uses that result only for review-lifecycle visibility and repair

#### Scenario: Pre-cutover invalidated records stay readable

- GIVEN an `invalidated` record written before cutover
- WHEN it is read post-cutover
- THEN it parses and remains available as historical review evidence without rewrite

### Requirement: Legacy Receipts Are Read Without Rewrite

Legacy-lineage receipts MUST be read through the algebra with byte-identical stored bytes before and after review-context evaluation. No in-place translation MUST occur.

#### Scenario: Legacy bytes are unchanged after evaluation

- GIVEN a legacy receipt on disk
- WHEN a review-context hook evaluates it
- THEN the stored bytes are byte-identical before and after
- AND the receipt is projected into `CandidateIdentity` only for review comparison

### Requirement: Review Context Is A Total Function Of Relation × Hook

For every pairing of the five hooks and the seven algebra relations, a review-context result MUST be defined and proven by the 35-cell boundary matrix; no cell may be left unhandled. Every result is evidence for review lifecycle status or repair only and never a delivery authorization or denial.

#### Scenario: Matrix executes with zero unexplained divergences

- GIVEN the 35-cell hook boundary matrix
- WHEN it runs against the shared review-context evaluation path
- THEN every cell resolves to an explained result, and pre-PR's `compatible_base_advance` and `changed` cells are pinned as named, explained divergences rather than a silent difference from the other hooks
- AND no matrix result changes commit, push, PR, release, or archive readiness
