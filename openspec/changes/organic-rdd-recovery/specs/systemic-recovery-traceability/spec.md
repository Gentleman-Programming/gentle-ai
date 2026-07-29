# Systemic Recovery Traceability Specification

## Purpose
Prove that recovery preserves safety, credit, and releasable behavior while obsolete control-plane material is removed.

## Requirements

### Requirement: Complete recovery disposition ledger
The system MUST record one disposition of `KEEP`, `TRANSPLANT`, `REWRITE`, `DELETE`, `REGENERATE`, or `DEFER` for every snapshot item, branch commit, file, test, contract, and asset. Each row MUST identify owner, invariant, evidence, and contributor credit. `DELETE` MUST be authorized either by a destination owner and proof that a retained invariant was transplanted and tested, or by explicit proof that the obsolete item owns no retained invariant.

#### Scenario: Deletion is proposed
- GIVEN a recovery item is marked `DELETE`
- WHEN its ledger row is validated
- THEN it names destination proof or no-retained-invariant proof
- AND deletion is rejected if neither authorization exists

### Requirement: Authoritative history reconciliation
The recovery ledger MUST trace all 241 issues and 92 PRs, including the 74-PR collision component, 499 overlaps, and 16 cross-context decompositions. It MUST preserve contributor attribution for each transferred or retained contribution.

#### Scenario: Overlap is reconciled
- GIVEN an item appears in colliding PR contexts
- WHEN the ledger is finalized
- THEN the overlap relationship and chosen disposition are recorded
- AND original contributor credit remains queryable

### Requirement: Verifiable publication classification
Every path considered for early deletion MUST record presence evidence against `origin/main`, local `main`, and the latest published tag using exact Git object queries. The ledger MUST record the checked refs and results. Early Wave-6 deviation MUST be `authorized` only for paths absent from all three; published paths MUST retain systemic migration order.

#### Scenario: Unpublished path requests early deletion
- GIVEN a path is absent from `origin/main`, local `main`, and the latest tag
- WHEN its destination/no-retained-invariant proof passes
- THEN the ledger records the authorized deviation and Git evidence
- AND any present result rejects early deletion

### Requirement: Invariant and proof coverage
The system MUST link every branch artifact to commits, files, tests, contracts, assets, and generic edge-case proof ledgers. No retained invariant MAY lack an owning context and exact proof reference.

#### Scenario: Edge case is retained
- GIVEN an edge-case invariant is kept or transplanted
- WHEN release evidence is assembled
- THEN its owner and test or justified evidence are linked
- AND an orphaned row fails validation

### Requirement: Release-grade evidence
The system MUST validate every retained or transplanted invariant touched by this change under its owning HCR, MMI, ACI, MCA, RAR, EPD, DSR, SDD, or PAD context. Exact-SHA proof MUST bind release evidence; cross-OS proof MUST apply to platform-sensitive invariants, and real-agent proof MUST apply to adapter-visible behavior. Backlog closure MUST NOT occur before released proof exists.

#### Scenario: Pre-release validation passes
- GIVEN all applicable invariant evidence is complete but release proof is absent
- WHEN closure is requested
- THEN closure is denied
- AND the evidence gap is reported
