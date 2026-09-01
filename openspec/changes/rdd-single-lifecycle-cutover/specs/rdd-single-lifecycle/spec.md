# Delta for RDD Single Lifecycle

Moved here from `rdd-root-simplification-wave7`'s delta, byte-identical to
the requirement and scenario text Wave 7 carried (verify finding C1 /
blocker B1) — Wave 7 deferred switch removal and never delivered this
requirement, so it does not belong in Wave 7's own delta. See this change's
`proposal.md` for the full re-entry brief this requirement's delivery
depends on.

## ADDED Requirements

### Requirement: Exactly One Lifecycle After Removal

After removal, no `GENTLE_AI_RDD_NEW_LINEAGE` reference, legacy start
branch, or legacy mutation path MUST remain reachable.

#### Scenario: Every start request takes the v3 path

- GIVEN the switch-removal slice has landed
- WHEN any `start` is requested, or the codebase is searched for the switch
- THEN it always proceeds through v3, and zero switch references remain
  outside historical/archived change specs

## MODIFIED Requirements

### Requirement: Byte-Equivalence Exit Evidence Precedes Switch Removal

Before the switch and its legacy start branch are deleted, the wave MUST
prove a `GENTLE_AI_RDD_NEW_LINEAGE=1` build and a switch-free build produce
byte-identical goldens, envelopes, and receipts, via same-fixture on/off
double-evaluation across the full journey set. A golden diff during this
proof MUST be treated as a defect signal, never a golden-update task.

This requirement itself is unchanged from Wave 7's spec (see that change's
own copy for the full text and Wave 7's own partial, scoped evidence). Only
its full-journey-set scenario moves here, byte-identical (verify finding
SL-1) — it can only be run by whichever change actually performs the
switch-free build, which Wave 7's deferral means is this change, not Wave
7 itself.

#### Scenario: Double-evaluation proves equivalence before deletion

- GIVEN the same fixture run once with the switch on and once switch-free
- WHEN goldens, envelopes, and receipts are compared
- THEN they are byte-identical, and only then does removal proceed
