# RDD Simplification Design Specification

## Purpose

Defines the normative target-architecture document for RDD Wave 0: its required location, evidence citations, preserved decisions, required amendments (A–E), and the migration chain-plan section.

## Requirements

### Requirement: Design Document Source and Evidence Paths

The design document MUST live at `docs/architecture/rdd-root-simplification-design.md`, copied from the sibling worktree design and amended per this spec, and MUST cite `docs/audits/2026-07-21-rdd-system-audit.md` (sha256 `4b41d15a…`) and `internal/reviewtransaction/prepr.go` (`deriveBaseAdvanceCompatibility`, line 73) as evidence sources, not an external path.

#### Scenario: Design cites in-repo evidence

- GIVEN the amended design document
- WHEN its evidence table is inspected
- THEN it references the in-repo audit path and the `prepr.go` proof, not an external path

### Requirement: Adopted Next-Step Decisions Preserved Verbatim

The design MUST state decisions 1–5 from the proposal's unresolved-decisions table as adopted, without re-derivation or re-litigation.

#### Scenario: Decision 1 — five-state model

- GIVEN the design's decision list
- WHEN decision 1 is inspected
- THEN it states the five-state model and two-active-artifact rule are adopted as specified

#### Scenario: Decision 2 — relation algebra and gates

- GIVEN the design's decision list
- WHEN decision 2 is inspected
- THEN it states the shared relation algebra and read-only gates are adopted, and gates never mutate authority

#### Scenario: Decision 3 — declined review / unsupported runtime

- GIVEN the design's decision list
- WHEN decision 3 is inspected
- THEN it states unmanaged ordinary delivery applies and the process fails before freeze

#### Scenario: Decision 4 — repair authorization and compatibility horizon

- GIVEN the design's decision list
- WHEN decision 4 is inspected
- THEN it states a maintainer-bound disposition plan and a short version-pinned read-only horizon

#### Scenario: Decision 5 — wave order

- GIVEN the design's decision list
- WHEN decision 5 is inspected
- THEN it states Wave 1 covers read-only equivalence, then #1892 leaf-only, with #2014 deferred

### Requirement: Amendment A — compatible_base_advance Normative Citation

The design's `compatible_base_advance` definition MUST cite the existing proof in `internal/reviewtransaction/prepr.go` (`deriveBaseAdvanceCompatibility`, line 73 — merge-base tree preservation, path digest identity, patch identity, path disjointness, conflict-free `merge-tree --write-tree`, issuer-bound CI attestation including the trust root, and base/HEAD non-advance revalidation) as normative semantics, instead of re-deriving them.

#### Scenario: compatible_base_advance references prepr.go

- GIVEN the design's `compatible_base_advance` section
- WHEN it is inspected
- THEN it cites `deriveBaseAdvanceCompatibility` at line 73 with all seven listed properties, and does not re-derive them independently

### Requirement: Amendment B / Decision 6 — provable_contraction Soundness Condition

The design's `provable_contraction` definition MUST validate only when admitted findings reference no excluded path; otherwise it MUST degrade to `changed`.

#### Scenario: Findings touch excluded path

- GIVEN a contraction candidate whose admitted findings reference an excluded path
- WHEN the design's soundness rule is applied
- THEN the state degrades to `changed` rather than validating as `provable_contraction`

### Requirement: Amendment C / Decision 7 — Legacy/New Lineage Precedence

The design's Wave 3 coexistence section MUST state that legacy readable authority never authorizes delivery of a candidate that has a new lineage.

#### Scenario: Legacy authority faces a new-lineage candidate

- GIVEN a candidate with a new lineage evaluated under legacy readable authority
- WHEN delivery authorization is attempted
- THEN legacy authority does not authorize delivery of that candidate

### Requirement: Amendment D / Decisions 8–9 — Unresolved Table Expansion

The design's unresolved-decisions table MUST add decision 8 (external evidence retention horizon: digests retained indefinitely in authority, raw payloads are provider diagnostics with a declared expiry) and decision 9 (SDD attempt-ledger ownership: attempts remain in SDD only if its store gains durable cumulative CAS-like properties, otherwise attempts move to native authority).

#### Scenario: Retention horizon entry present

- GIVEN the design's unresolved-decisions table
- WHEN decision 8 is inspected
- THEN it states digests are retained indefinitely and raw payloads carry a declared expiry

#### Scenario: Attempt-ledger ownership entry present

- GIVEN the design's unresolved-decisions table
- WHEN decision 9 is inspected
- THEN it states the SDD-vs-native-authority conditional exactly as adopted

### Requirement: Amendment E — Issue #1379 Coverage

The design MUST list issue #1379 (cross-lineage receipt contamination, audit-flagged potentially severe) in both its adversarial safety table and its coverage map.

#### Scenario: #1379 appears in both tables

- GIVEN the design's adversarial safety table and coverage map
- WHEN both are inspected
- THEN issue #1379 appears in each, flagged potentially severe

### Requirement: Migration Chain Plan Documented

The design MUST document the migration chain: tracker branch `feature/rdd-root-simplification` off `main@ece470dacd0041f394e7f6f3877a6a9fcb3482af`, one child PR per wave targeting the immediately previous slice, only the tracker merging to `main`, and an approximate 1000-changed-line budget per PR.

#### Scenario: Chain plan section present

- GIVEN the design document
- WHEN its chain-plan section is inspected
- THEN it states the tracker branch, per-wave PR targeting rule, and the merge-to-main rule

### Requirement: Change Scope Boundary

Wave 0 MUST NOT modify any file outside `docs/` and `openspec/`.

#### Scenario: Diff scope check

- GIVEN the complete Wave 0 changeset
- WHEN its changed-file list is inspected
- THEN every path is under `docs/` or `openspec/`
