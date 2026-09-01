# Delta for RDD Candidate Relation Algebra

## ADDED Requirements

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
