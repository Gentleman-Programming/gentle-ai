# RDD Receipt-Only Gates Specification

## Purpose

Define the single read-only evaluation path all five delivery gates (`post-apply`, `pre-commit`, `pre-push`, `pre-pr`, `release`) MUST use for every lineage, replacing gate-side authority mutation, composed delivery authority, and per-gate/per-lineage forks with a derived verdict over the shared relation algebra.

## Requirements

### Requirement: One Read-Only Path For All Gates And Lineages

Every gate MUST evaluate every lineage — new and legacy — through one ordered contract: capability admission, then kill switch, then governing authority discovery, then relation, then verdict plus next step. No gate MUST branch on gate identity or lineage kind beyond supplying its own boundary descriptor.

#### Scenario: Pre-PR uses the same evaluation as the other four gates

- GIVEN a pre-PR boundary and a post-apply boundary for an equivalent receipt
- WHEN each gate evaluates its candidate
- THEN both invoke the identical ordered evaluation, differing only in boundary descriptor

#### Scenario: Legacy lineage evaluated through the same path as a new lineage

- GIVEN a legacy-lineage candidate at any gate
- WHEN the gate evaluates it
- THEN it uses the same relation-algebra path a new-lineage candidate uses, not a separate discovery function

### Requirement: Kill Switch Consulted Before Governing Authority Is Read

The kill switch MUST be consulted before any governing-authority read. When off, the gate MUST report via ordinary unmanaged delivery without letting a missing, stale, ambiguous, or corrupted authority read alter its outcome. (Supersedes #2222/#2239.) (RATIFIED, maintainer, 2026-08-02: close #2222/#2239 as superseded once this ordering is proven by a named per-gate regression test landed in S2; do not merge #2222/#2239 first.)

#### Scenario: Kill switch off short-circuits before authority discovery

- GIVEN the kill switch is off
- WHEN a gate is invoked
- THEN it reports disabled-unmanaged delivery before attempting any receipt or authority read

#### Scenario: Corrupted authority is irrelevant while disabled

- GIVEN the kill switch is off and the authority store is ambiguous or damaged
- WHEN a gate is invoked
- THEN it still exits via ordinary unmanaged delivery, never surfacing the underlying authority error

### Requirement: Six Prohibited Gate Actions

A gate evaluation MUST NOT: (1) start review; (2) create correction authority; (3) consume a budget; (4) invalidate a receipt by mutating authority; (5) create a recovery lineage; (6) compose an unrelated receipt graph to invent delivery authority. Each prohibition holds for every gate and every lineage kind. (RATIFIED, maintainer, 2026-08-02: deliveries that previously passed via prohibited action 4 or 6 now deny hard, with an executable next step, and with no migration window.)

#### Scenario: Denial writes no invalidation record

- GIVEN a receipt whose live boundary has diverged from its persisted claim
- WHEN the gate evaluates it
- THEN it denies with a derived mismatch relation, and no writer lock is acquired and no state is rewritten

#### Scenario: Pre-PR denial does not compose a receipt graph

- GIVEN a pre-PR base-mismatch denial
- WHEN the gate reports its verdict
- THEN it does not construct or return a composed receipt graph as an alternate authorization

### Requirement: Every Denial Carries An Executable Next Step

Every denial MUST include either a typed transition the caller can invoke next, or a stop response with a `reason_code` — never a bare denial with no path forward.

#### Scenario: Base-mismatch denial names a next step

- GIVEN a `changed` relation denial at any gate
- WHEN the verdict is emitted
- THEN it includes a typed transition (e.g., start a new candidate) the caller can invoke

#### Scenario: Unknown relation stops with a reason code

- GIVEN an `unknown` relation
- WHEN the verdict is emitted
- THEN it stops and returns a `reason_code` rather than a silent or unexplained denial

### Requirement: Receipt File Persists On Invalidation; `invalidated` Is Fully Derived

The persisted receipt file MUST NOT be deleted as a side effect of gate evaluation. An `invalidated` verdict MUST be derived by comparing the live boundary against the persisted receipt at evaluation time, never recorded as new stored state. (AUDIT-GATED, maintainer, 2026-08-02: the maintainer does not know whether any consumer treats file-absence as its invalidation signal; this is resolved by audit, not assumption. The `ReceiptPath()` reader audit — already a slice-7 prerequisite — sweeps in-repo and bundled Pi assets for direct receipt-file existence reads; findings migrate to `review validate`, plus an rc release-notes line about receipt-file persistence under derived invalidation.)

#### Scenario: File-absence-as-signal regression is closed

- GIVEN a receipt that would have been `os.Remove`d under the pre-cutover invalidation writer
- WHEN the gate evaluates it post-cutover
- THEN the receipt file remains present on disk, the gate denies with a derived mismatch, and any reader MUST consult the derived verdict field rather than file presence

#### Scenario: Pre-cutover invalidated records stay readable

- GIVEN an `invalidated` record written before cutover
- WHEN it is read post-cutover
- THEN it parses and validates without rewrite

### Requirement: Legacy Receipts Validate Without Rewrite

Legacy-lineage receipts MUST validate through the algebra with byte-identical stored bytes before and after gate evaluation. No in-place translation MUST occur.

#### Scenario: Legacy bytes are unchanged after validation

- GIVEN a legacy receipt on disk
- WHEN a gate validates it
- THEN the stored bytes are byte-identical before and after, and the receipt was only projected into `CandidateIdentity` for comparison

### Requirement: Verdict Is A Total Function Of Relation × Gate

For every pairing of the 5 gates and the 7 algebra relations, a verdict MUST be defined and proven by the 35-cell gate boundary matrix; no cell may be left unhandled.

#### Scenario: Matrix executes with zero unexplained divergences

- GIVEN the 35-cell gate boundary matrix
- WHEN it runs against the cutover evaluation path
- THEN every cell resolves to an explained verdict, and pre-PR's `compatible_base_advance` AND `changed` cells (grounded at `compact_gate.go:97-100`, where `baseMatches` is forced `true` and admits a current-changes boundary proof) are pinned as named, explained divergences rather than a silent difference from the other four gates
