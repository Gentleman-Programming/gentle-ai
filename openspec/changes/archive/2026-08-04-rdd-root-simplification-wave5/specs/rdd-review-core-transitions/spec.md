# Delta for RDD Review Core Transitions

> **Re-base caveat**: this delta is layered on `openspec/changes/rdd-root-simplification-wave4/specs/rdd-review-core-transitions/spec.md`, itself copied from Wave 3's not-yet-archived spec. If either Wave archives before Wave 5, re-diff this delta against the archived top-level spec before Wave 5 archives; the requirement below is authoritative as of this writing.

## ADDED Requirements

### Requirement: `validate` Is The Single Governing Path For Legacy Lineages

`validate` — the read-only evaluation transition — MUST become the sole governing path invoked by all five gates for legacy lineages, replacing bespoke per-gate or per-lineage discovery functions.

#### Scenario: Legacy lineage invokes the same transition as a new lineage

- GIVEN a legacy-lineage candidate and a new-lineage candidate at the same gate
- WHEN each is evaluated
- THEN both invoke `validate`; no gate calls a lineage-specific discovery function that bypasses it

#### Scenario: No bespoke discovery fork remains

- GIVEN the cutover has landed
- WHEN a gate's code path is inspected
- THEN it contains no per-lineage-kind discovery branch outside `validate`
