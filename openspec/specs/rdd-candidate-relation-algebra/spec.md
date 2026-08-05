# RDD Candidate Relation Algebra Specification

## Purpose

Define the one read-only relation function that compares a frozen `CandidateIdentity` with a live candidate and returns exactly one of the seven target-architecture relations. This spec encodes Amendment A (delegation, not reimplementation) and Amendment B (contraction soundness) as hard requirements, plus the characterization-test precondition Wave 0 flagged before this seam becomes normative.

## Requirements

### Requirement: Seven-Value Relation Output

The relation function MUST return exactly one of `exact`, `compatible_base_advance`, `provable_contraction`, `changed`, `unrelated`, `ambiguous`, or `unknown` for any comparison, and MUST NOT introduce an eighth value.

#### Scenario: Identical candidate and policy resolve to exact

- GIVEN a frozen candidate and a live candidate with identical `CandidateIdentity` and policy hash
- WHEN the relation function evaluates them
- THEN it returns `exact`

#### Scenario: No governing lineage resolves to unrelated

- GIVEN live content with no prior candidate authority
- WHEN the relation function evaluates it
- THEN it returns `unrelated`

### Requirement: `compatible_base_advance` Delegation (Amendment A)

The relation function MUST determine `compatible_base_advance` by delegating to `deriveBaseAdvanceCompatibility` (`internal/reviewtransaction/prepr.go:73`) as the normative semantics for all seven conditions (merge-base tree preservation, path digest identity, patch identity, path disjointness, conflict-free merge, issuer-bound CI attestation, base/HEAD non-advance revalidation). It MUST NOT reimplement any of the seven conditions independently in the shadow package.

#### Scenario: All seven conditions hold

- GIVEN a base advance where `deriveBaseAdvanceCompatibility` returns compatible
- WHEN the relation function evaluates the candidate pair
- THEN it returns `compatible_base_advance`
- AND the decision is attributable to the delegated call, not a parallel shadow-side computation

#### Scenario: Any condition fails

- GIVEN a base advance where `deriveBaseAdvanceCompatibility` returns not-compatible for any one of the seven conditions
- WHEN the relation function evaluates the candidate pair
- THEN it does not return `compatible_base_advance`
- AND no shadow-local override of the delegated result exists

### Requirement: `provable_contraction` Soundness Degradation (Amendment B)

The relation function MUST degrade `provable_contraction` to `changed` when any admitted finding references a path outside the delivered candidate's live scope.

#### Scenario: Contraction with no excluded-path findings

- GIVEN a live candidate that is a deterministic subset of the reviewed content, and every admitted finding references only included paths
- WHEN the relation function evaluates it
- THEN it returns `provable_contraction`

#### Scenario: Contraction with an excluded-path finding degrades

- GIVEN a live candidate that is a deterministic subset of the reviewed content, and at least one admitted finding references an excluded path
- WHEN the relation function evaluates it
- THEN it returns `changed`, not `provable_contraction`

### Requirement: Characterization Tests Precede Delegation-Seam Changes

`deriveBaseAdvanceCompatibility` MUST have direct, covering characterization tests committed before any refactor of its delegation seam, and before this relation function is treated as normative for `compatible_base_advance`.

#### Scenario: Characterization coverage exists before delegation is exercised

- GIVEN Wave 0 verify SUGGESTION-5 identified 4 callers and no direct covering tests
- WHEN the shadow relation function is wired to delegate to `deriveBaseAdvanceCompatibility`
- THEN direct characterization tests for all seven conditions already exist and pass
- AND no seam refactor of `deriveBaseAdvanceCompatibility` occurs before those tests exist

### Requirement: Read-Only at Legacy Call Sites, Deciding Authority at New-Lineage Call Sites

The relation function MUST NOT mutate authority state, consume a correction budget, or alter any live decision at any legacy call site it observes. At a new-lineage call site gated by the activation switch (`rdd-new-lineage-activation`), the identical function MAY be consumed by `ReviewCore` as the deciding input for `start`, `finalize`, `validate`, and gate authorization — no separate or re-derived implementation is permitted for that purpose.
(Previously: stated the function is read-only at every call site with no live-lifecycle exception; Wave 3 introduces the first live consumer, `ReviewCore`, so the boundary between legacy observation and new-lineage decision is now explicit.)

#### Scenario: Shadow evaluation changes nothing observable

- GIVEN the relation function runs alongside a live gate decision at a legacy call site
- WHEN it computes a relation
- THEN the live gate's outcome is unchanged and byte-identical to the shadow-off baseline

#### Scenario: New-lineage call site consumes the same function as deciding authority

- GIVEN a new-lineage `start` with the activation switch on
- WHEN the relation function evaluates the live candidate against the frozen `CandidateIdentity`
- THEN its output determines the live transition decision
- AND `ReviewCore` invokes the same function used by the shadow harness at legacy sites, not a re-derived copy

### Requirement: `ambiguous` and `unknown` Have No Fabricated Live Counterpart

When the relation function returns `ambiguous` or `unknown` for a fixture with no corresponding live decision, that row MUST be marked "no live decision" and MUST NOT be reported as agreement with a live outcome.

#### Scenario: Ambiguous fixture row is marked, not fabricated

- GIVEN a differential-matrix fixture where the shadow relation is `ambiguous` and no live decision exists to compare against
- WHEN the matrix row is recorded
- THEN it is labeled "no live decision"
- AND it is never recorded as agreement

### Requirement: Gate Boundary Descriptor Is A First-Class Algebra Input

The relation function MUST accept an explicit per-gate boundary descriptor (current candidate, staged candidate, delivery range/remote boundary, candidate/base relationship, or release target/publication boundary) as an input parameter. It MUST NOT infer the boundary implicitly from call-site state.

#### Scenario: Distinct boundary descriptors produce comparable relations

- GIVEN pre-PR's candidate/base descriptor and post-apply's current-candidate descriptor for equivalent underlying content
- WHEN the relation function evaluates each
- THEN both produce relation outputs comparable within the same 35-cell gate boundary matrix

### Requirement: Verdict Is A Total Function Of Relation × Gate

For every pairing among the 5 gates and the 7 relations, a verdict MUST be defined; no pairing may be left unhandled. (Assumption, pending maintainer confirmation, Wave-1-owned: #2126's self-loop exclusion belongs to the Wave 1 algebra; this wave only consumes it — no algebra change is required here beyond the boundary-descriptor and total-function additions above.)

#### Scenario: 35-cell matrix has zero unexplained divergences

- GIVEN the 5-gate x 7-relation boundary matrix
- WHEN it is generated from the algebra
- THEN every cell resolves to a defined verdict, and pre-PR's `compatible_base_advance` AND `changed` cells (grounded at `compact_gate.go:97-100`, where `baseMatches` is forced `true` and admits a current-changes boundary proof) are pinned as named, explained divergences from the other four gates
