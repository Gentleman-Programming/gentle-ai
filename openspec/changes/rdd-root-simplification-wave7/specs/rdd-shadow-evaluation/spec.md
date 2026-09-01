# Delta for RDD Shadow Evaluation

## REMOVED Requirements

### Requirement: Advisory-Only, Never Blocking

(Reason: Proposal D1 — the runtime shadow observer retired outright (WU4,
Wave 7 S2a). The `ShadowRelation` type alias itself (candidate_relation.go)
is DEFERRED, not retired, as of this wave's close-out: it lives under
`internal/reviewtransaction` so no external Go importer is possible, but
`shadow_relation_test.go` still spells out the `ShadowRelation*` names
across several tests, and the rename pass to `CandidateRelation*` was
disclosed at WU4 time as a small, independent follow-up not gating any
later work unit. Confirmed zero PRODUCTION consumers remain — only its own
test file and one unexported helper, `shadowRelationHasNoLiveCounterpart`,
still spell it out.)
(Migration: rename `ShadowRelation`/`ShadowRelation*` constants to
`CandidateRelation`/`CandidateRelation*` in a follow-up slice; purely
mechanical, no behavior change. The names are live in 6 production files,
not 2 — `candidate_relation.go`, `new_lineage_discovery.go:124`,
`gate.go:1730-1748`, `compact_gate.go:161,163`, `review_core.go:236-246`,
and `internal/cli/review_governing_authority.go:69` — plus
`shadow_relation_test.go`. Whoever picks this up should size it against
all 6 production files, not just the type-alias site and its test.)

### Requirement: Disable Switch Is the Observer's Rollback Boundary

(Reason: the observer it gates no longer exists.)
(Migration: None.)

### Requirement: Zero Live-Lifecycle Behavior Change

(Reason: no shadow code path remains to prove neutral against a live
decision; v3 behavior is now governed by `rdd-single-lifecycle`.)
(Migration: None.)

### Requirement: No Persisted Divergence Artifact (Assumption, pending maintainer confirmation)

(Reason: no divergence-recording code remains.)
(Migration: None.)

### Requirement: Off by Default in Live Paths (Assumption, pending maintainer confirmation)

(Reason: the observer is deleted, not merely defaulted off.)
(Migration: None.)

### Requirement: Unexplained Divergence Blocks Wave 2 (Assumption, pending maintainer confirmation)

(Reason: Wave 2 has long since landed; this was Wave 1's own boundary.)
(Migration: None.)

## ADDED Requirements

### Requirement: Historical Differential-Matrix Evidence Retained

(Recommended default — Proposal D1.) `shadow-differential-matrix.golden`
MUST remain byte-preserved as historical exit evidence after the observer
is deleted. It MUST NOT be regenerated, extended, or treated as
live-code-backed after this wave.

#### Scenario: Golden survives observer deletion unchanged

- GIVEN the shadow observer has been deleted (the `ShadowRelation` type
  alias itself was retained, deferred per the note above — not deleted)
- WHEN the repository is inspected
- THEN the golden still exists, byte-identical, and no code regenerates it
