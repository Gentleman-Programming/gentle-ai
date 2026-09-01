# RDD Single Lifecycle Specification

## Purpose

Define the exit gate for removing `GENTLE_AI_RDD_NEW_LINEAGE` so exactly one
review lifecycle (v3) remains. Grounded at post-W6 chain tip `40176a8f`
(pending Wave 6 verify); re-validate line references at task time.

## Requirements

### Requirement: Byte-Equivalence Exit Evidence Precedes Switch Removal

Before the switch and its legacy start branch are deleted, the wave MUST
prove a `GENTLE_AI_RDD_NEW_LINEAGE=1` build and a switch-free build produce
byte-identical goldens, envelopes, and receipts, via same-fixture on/off
double-evaluation across the full journey set. A golden diff during this
proof MUST be treated as a defect signal, never a golden-update task.

Wave 7 obeyed this gate correctly (it never deleted the switch without
evidence, and stopped when the evidence it could gather was incomplete —
see the Amendment below) and recorded real, byte-identical-confirmed
evidence for the scoped surface it could reach without a switch-free build
(unnegotiated start, status, finalize, all 5 gates — tasks.md task 2.1).
The scenario proving the full-journey-set claim end to end, negotiated
form included, requires the switch-free build only an actual removal
attempt produces (verify finding SL-1). That scenario has moved,
byte-identical, to `rdd-single-lifecycle-cutover`'s `MODIFIED Requirements`
delta for this same requirement — it belongs there because only the
change that performs removal can run it, not because this requirement's
standing obligation changes.

### Requirement: Preconditions W-9, W-10, W-11 Close Before Removal

(Recommended default — Wave 5 verify #10186 cycle 3.) The switch-removal
slice MUST NOT start until W-9 (unknown `causal_disposition` on a severe
finding escalates on v3, matching v2), W-10 (v3 capture refuses a severe
finding missing `evidence_class`/`causal_disposition`, mirroring v2), and
W-11 (WARNING-severity candidate-causal findings stay non-blocking on v3,
matching v2) are all closed.

#### Scenario: Any open precondition blocks the slice

- GIVEN W-9, W-10, or W-11 is still open
- WHEN the switch-removal slice is proposed
- THEN it does not start; it starts only once all three are closed

## Requirement Physically Moved to the Successor Change (verify B1, closing the C1 gap)

This change (Wave 7) does not deliver "Exactly One Lifecycle After
Removal" — the WU18 attempt was deferred (see the Amendment below), so the
switch and legacy start branch still ship. Carrying that requirement in
this delta, even relabeled, still counted it in Wave 7's own requirement
inventory as an unmet requirement of THIS change (verify blocker B1:
relabeling in place fixed the claim but not the arithmetic — 15 headings,
14 delivered). It has therefore been physically moved, byte-identical and
unweakened, into `openspec/changes/rdd-single-lifecycle-cutover/specs/rdd-single-lifecycle/spec.md`
(`### Requirement: Exactly One Lifecycle After Removal`, same body, same
scenario) — see that change's `proposal.md` for the precise re-entry
brief. Ownership is now structural, not annotated: the requirement simply
does not appear here.

Wave 7's own delivered requirement is the re-entry gate immediately below
("Switch Removal Is Blocked On v3 Negotiated Repository Context") — that
one Wave 7 both owns and satisfies (the gap it names is real, disclosed,
and currently open).

**Corrected counts for this change's delta** (three capability specs
combined: `rdd-legacy-retirement`, `rdd-shadow-evaluation`,
`rdd-single-lifecycle`), after this move and the follow-up SL-1 scenario
move immediately above: **14/14 requirements** delivered (unchanged by the
scenario move — the "Byte-Equivalence Exit Evidence Precedes Switch
Removal" requirement heading stays in this delta; only its
full-journey-set scenario relocated). Scenarios: **8/8** — the
"Double-evaluation proves equivalence before deletion" scenario also moved
to `rdd-single-lifecycle-cutover` (as a `MODIFIED Requirements` addition to
the same requirement, since only the change that performs removal can run
it), so every scenario this change still names, it delivers. Prior counts, for
the record: requirements 14/15 (relabel-in-place, verify blocker B1) →
14/14 (B1's physical move, unchanged since); scenarios 8/10 → 8/9 (B1's
scenario left with its requirement) → 8/8 (this SL-1 scenario move).

## Amendment (Wave 7 S7, WU18 attempt — deferred, not landed)

WU18 attempted the switch removal this spec describes. The production
change (deleting `GENTLE_AI_RDD_NEW_LINEAGE` and the legacy start branch)
was completed and passed the byte-equivalence exit evidence above with zero
golden drift. W-9/W-10/W-11 were re-confirmed green immediately before
attempting removal.

Executing the removal then surfaced a capability gap this spec's exit
criteria did not anticipate: v3's negotiated START (`--contract` form) has
never supported `repository_context` (the opaque, path-free handle a
`capture-result` call can carry across a process cwd boundary). This gap
predates Wave 7 — `runReviewFacadeStartNewLineage` has always been called
unconditionally, ignoring `--contract`, even while the switch was on since
Wave 3 — so it was previously reachable only at the narrow, rarely-exercised
switch-ON-AND-negotiated intersection. Removing the switch makes v3
unconditional, so every negotiated START now takes that path: the gap
becomes universal rather than a corner case.

Building genuine v3 `repository_context` support is real, standalone
engineering, not a small patch: v3's `NewLineageAuthority` stores
`CandidateIdentity` (`RepositoryID`/`BaseTree`/`CandidateTree`/
`ChangedPathsModesDigest`/`PolicyHash`), a structurally different hash from
`Snapshot.Identity` (the single combined hash `repository_context`'s own
validator, `validateLiveReviewRepositoryContext`, compares against). Closing
the gap needs either a `NewLineageAuthority` schema change or re-deriving
`Snapshot.Identity` from Git at validation time — both real design
decisions, and `validateLiveReviewRepositoryContext` is a security-sensitive
binding validator: a wrong implementation could silently accept a
mismatched context and misbind a reviewer's result to the wrong candidate.
Extending it under the time pressure of a switch-removal PR is the wrong
trade.

**Decision**: switch removal is DEFERRED, not abandoned. Removing a switch
over a known capability gap is exactly what this spec's byte-equivalence
exit criteria exist to prevent (a diff or gap discovered during the
removal attempt is a defect signal, never a reason to relabel the gap as
acceptable and ship anyway). Keeping the switch also means v3 stays
opt-in through the upcoming release candidate, which is the safer posture
for community testing of the new lifecycle.

The genuinely additive work the WU18 attempt produced was kept and shipped
independently of the switch (Wave 7 S7a / WU18a): start-time collision
guards for both legacy authority kinds on the switch-ON v3 path (a real,
independent gap — a switch-ON `review start` previously performed no
collision check against either kind of existing legacy authority at all),
and negotiated-form support for v3 START's frozen candidate context (though
not `repository_context`, the specific gap above).

### Requirement: Switch Removal Is Blocked On v3 Negotiated Repository Context

The switch-removal slice this spec describes MUST NOT be attempted again
until v3's negotiated START (`runReviewFacadeStartNewLineage`'s negotiated
path) can populate `repository_context` for a newly created v3 lineage,
validated by a `validateLiveReviewRepositoryContext`-equivalent check that
is genuinely safe for v3 authority (never a bypass or a weakened check).

#### Scenario: Negotiated v3 START gains repository_context support

- GIVEN v3's negotiated START populates `repository_context` with the same
  security guarantees the compact-v2 path already has
- WHEN the switch-removal slice is re-attempted
- THEN the byte-equivalence exit evidence above is re-proven end to end
  (including the negotiated form this time), and only then does removal
  proceed

#### Scenario: The gap remains open

- GIVEN v3 negotiated START still cannot safely populate `repository_context`
- WHEN switch removal is proposed
- THEN it does not proceed; the switch and the legacy start branch stay in
  place
