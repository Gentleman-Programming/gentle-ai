# RDD Legacy Retirement Specification

## Purpose

Define the deletion contract for legacy RDD machinery: consumer-first
ordering, per-target retirement, contract v1 freeze, and the forensic
legacy-read invariant. Grounded at post-W6 chain tip `40176a8f` (pending
Wave 6 verify); re-derive the exact target inventory at task time.

## Requirements

### Requirement: Consumer-First Ordering With Deadcode Ratchet as Exit Evidence

For each target, every live consumer MUST be migrated or retired before its
provider is deleted; a provider MUST NOT be deleted while a live consumer
exists — a remaining consumer defers the target with a written reason
instead. Each slice's exit evidence MUST report a strongly net-negative
deadcode ratchet delta plus a deletion proof per removed test naming what
it covered and where equivalent coverage now lives.

#### Scenario: Provider deletion waits, then reports ratchet and proof

- GIVEN a target with one remaining live consumer
- WHEN the deletion slice is proposed
- THEN the consumer retires first, the provider deletes, and exit evidence
  reports a net-negative ratchet delta with a deletion proof per test

### Requirement: Legacy Public Verb and Reconcile Provider Retirement

(Recommended default — Proposal D5.) The five legacy public verbs
(`reconcile-authority`, `reconcile-authority-batch`, `quarantine-legacy`,
`quarantine-legacy-fix-scope`, `repair-legacy-alias`) and their facade
dispatch handlers, and both reconcile providers
(`ReconcileInvalidRecoveryEdge`, `ReconcileInvalidRecoveryEdges`) with their
CLI consumers and the batch reconcile journal, MUST each delete only as one
consumer cluster, after their own consumers retire.

#### Scenario: A retired cluster leaves nothing dispatchable

- GIVEN a verb or reconcile-provider cluster's retirement slice has landed
- WHEN any of its names is invoked
- THEN nothing dispatches it, and no partial half of the cluster remains

### Requirement: Contract v1 Freeze, Not Deletion

(Recommended default — Proposal D3.) `contracts/review-integration/v1/**`
MUST be frozen read-only this wave, not deleted, until a declared support
horizon passes with proof no supported consumer (including a pinned
adapter release) still calls it.

#### Scenario: v1 stays readable but unmodified this wave

- GIVEN the contract-freeze requirement
- WHEN Wave 7 completes
- THEN v1 still exists, byte-unchanged, unconsumed by new-lineage behavior

### Requirement: Legacy Read Retention (Forensic Parse Invariant)

(Recommended default — Proposal D5.) Deleting legacy mutation MUST NOT
delete the ability to read legacy history. The read-only/offline legacy
parser MUST remain, and historical bytes (receipts, journals, bundles,
quarantine residue) MUST stay parseable and untouched, with no mutation
call reachable from the read path.

#### Scenario: Legacy mutation is gone but legacy records still parse

- GIVEN legacy mutation has been fully retired
- WHEN a shipped legacy-v1 record is read through the retained parser
- THEN it parses successfully, its bytes are unchanged, and no write or
  repair call is reachable from that path
