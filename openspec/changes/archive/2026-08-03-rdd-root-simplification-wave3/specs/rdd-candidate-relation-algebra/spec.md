# Delta for RDD Candidate Relation Algebra

**Pre-archive note**: `rdd-candidate-relation-algebra` exists only under `openspec/changes/rdd-root-simplification-wave1/specs/rdd-candidate-relation-algebra/spec.md` — Wave 1 has not archived to `openspec/specs/` yet. This delta is written against that Wave 1 filesystem copy, following the same pattern Wave 2 used for `rdd-authority-graph-classification`. If Wave 1 archives before this delta is applied, re-base this MODIFIED block against `openspec/specs/rdd-candidate-relation-algebra/spec.md` instead.

Wave 3 does not change the algebra's seven-value output or Amendments A/B. What changes is consumption: the same relation function is lifted out of shadow-only gating and becomes the deciding authority at new-lineage `start`/`finalize`/`validate` and all five gates, gated by the activation switch — while remaining purely observational at every legacy call site.

## MODIFIED Requirements

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
