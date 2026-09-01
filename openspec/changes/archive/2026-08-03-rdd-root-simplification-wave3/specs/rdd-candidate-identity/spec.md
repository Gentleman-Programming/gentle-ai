# Delta for RDD Candidate Identity

**Pre-archive note**: `rdd-candidate-identity` exists only under `openspec/changes/rdd-root-simplification-wave1/specs/rdd-candidate-identity/spec.md` — Wave 1 has not archived to `openspec/specs/` yet. This delta is written against that Wave 1 filesystem copy, following the same pattern Wave 2 used for `rdd-authority-graph-classification`. If Wave 1 archives before this delta is applied, re-base this MODIFIED block against `openspec/specs/rdd-candidate-identity/spec.md` instead.

Wave 3 does not change how the resolver computes a `CandidateIdentity` from a selector. What changes is what happens to the result for a new lineage: it stops being an ephemeral computed observation and becomes persisted frozen authority inside the two-artifact store (`rdd-authority-store`).

## MODIFIED Requirements

### Requirement: Read-Only Resolution, Persisted as Frozen Authority When Consumed by a New Lineage

The resolver itself MUST NOT mutate authority state, write a persisted artifact, or introduce a new public operation or contract version — resolution stays pure. When `ReviewCore.start` consumes a resolver's output for a new lineage, that `CandidateIdentity` MUST be written into the authority store as frozen authority; subsequent `finalize`, `validate`, and gate decisions for that lineage MUST compare against the persisted identity, not by re-resolving.
(Previously: stated resolution has no side effect anywhere, with no notion of a downstream persisted consumer; Wave 3 introduces the first such consumer, `ReviewCore`, so the boundary between pure resolution and persisted freeze is now explicit.)

#### Scenario: Resolution has no side effect

- GIVEN any valid selector input
- WHEN the resolver executes
- THEN no file, authority record, or receipt is written
- AND repeated resolution of the same selector is idempotent

#### Scenario: New-lineage start persists the resolved identity as frozen authority

- GIVEN a new-lineage `start` with the activation switch on
- WHEN the resolver's output is accepted by `ReviewCore`
- THEN the `CandidateIdentity` is written into `review-state.json` as frozen authority
- AND later transitions in that lineage compare against the persisted identity rather than re-resolving the selector
