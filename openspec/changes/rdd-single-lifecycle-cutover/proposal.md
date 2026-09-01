# Proposal: RDD Single Lifecycle Cutover (successor to Wave 7)

Successor to `rdd-root-simplification-wave7`. Wave 7 deliberately deferred
the `GENTLE_AI_RDD_NEW_LINEAGE` switch removal (WU18) rather than remove it
over a known, disclosed capability gap — see
`openspec/changes/rdd-root-simplification-wave7/specs/rdd-single-lifecycle/spec.md`'s
Amendment for the full finding. This change exists to carry the one
requirement Wave 7 could not deliver, so Wave 7's own spec claims only what
it shipped (verify finding C1 / blocker B1, `sdd/rdd-root-simplification-wave7/verify-report`,
Engram #10215).

## Intent

Land the switch removal Wave 7 attempted and reverted: delete
`GENTLE_AI_RDD_NEW_LINEAGE` and the legacy `review start` branch it guards,
so exactly one review lifecycle (v3) remains reachable. This is a single,
precisely-scoped requirement — everything else Wave 7's `rdd-single-lifecycle`
capability defines (byte-equivalence exit evidence, W-9/W-10/W-11
preconditions, the negotiated-repository_context re-entry gate) already
shipped in Wave 7 and stays unchanged; this change only adds the one
requirement that gate protects.

## The re-entry brief (why removal was blocked, and what closes it)

Wave 7's WU18 attempt found that v3's negotiated START (`--contract` form,
`runReviewFacadeStartNewLineage`'s negotiated path) has never supported
`repository_context` — the opaque, path-free handle a `capture-result` call
carries across a process cwd boundary. This gap predates Wave 7 and was
previously reachable only at the narrow switch-ON-AND-negotiated
intersection; removing the switch makes v3 unconditional, so every
negotiated START would take that path and the gap becomes universal rather
than a corner case. Extending `validateLiveReviewRepositoryContext` — a
security-sensitive binding validator — under the time pressure of a
switch-removal PR was judged the wrong trade (non-negotiable: never remove
a switch over a known capability gap).

This change MUST, in order:

1. Give v3's negotiated START genuine `repository_context` support, by
   either:
   - extending `NewLineageAuthority`'s schema with a value comparable to
     `Snapshot.Identity` (the single combined hash the existing
     `validateLiveReviewRepositoryContext` compares against — structurally
     different from `NewLineageAuthority`'s current `CandidateIdentity`
     shape: `RepositoryID`/`BaseTree`/`CandidateTree`/
     `ChangedPathsModesDigest`/`PolicyHash`), or
   - safely re-deriving `Snapshot.Identity` from Git at validation time.
2. Add a v3-aware branch to `validateLiveReviewRepositoryContext` (or an
   equivalent validator) with the identical security guarantees the
   compact-v2 path already has — never a bypass, never a weakened check. A
   wrong implementation here could silently accept a mismatched context and
   misbind a reviewer's result to the wrong candidate.
3. Re-prove byte-equivalence exit evidence end to end across the full
   journey set, including the negotiated form this time — Wave 7's Commit A
   recording covered the unnegotiated form plus status/finalize/the 5
   gates, but explicitly deferred the negotiated form and `capture-result`
   (see Wave 7 apply-progress and tasks.md task 2.1); this change must
   close that gap before it re-attempts removal, not after.
4. Only then delete `GENTLE_AI_RDD_NEW_LINEAGE` and the legacy start
   branch.

If any of these steps surfaces a further capability gap, the same
non-negotiable applies: stop before removal and report, rather than ship
over a known gap.

## Scope

### In scope

- v3 negotiated START `repository_context` support (schema or re-derivation
  design decision, plus the validator branch).
- Full-journey-set byte-equivalence proof, negotiated form included.
- The switch + legacy start branch deletion itself, once the above are
  proven safe.

### Out of scope

- Any other legacy-retirement surface — Wave 7 already retired the 5
  legacy public verbs, both reconcile providers, and classified the 6
  remaining D4 verbs (`invalidate`/`abandon`/`recover`/`reclaim`/
  `dispose-result`/`reopen-results`) as live compact-v2 mutation surface,
  independent of this change.
- Re-litigating Wave 7's deferral decision — it is settled; this change's
  job is to close the gap that caused it, not to argue the switch should
  have been removed anyway.

## Capabilities

### Modified

- `rdd-single-lifecycle`: adds the "Exactly One Lifecycle After Removal"
  requirement (moved here byte-identical from Wave 7's delta, ownership
  only — text unweakened) now that this change is the one delivering it.
  Also carries a `MODIFIED Requirements` entry for "Byte-Equivalence Exit
  Evidence Precedes Switch Removal": the requirement itself stays defined
  in Wave 7's spec unchanged, but its full-journey-set scenario (the one
  requiring a switch-free build, negotiated form included) moves here
  byte-identical — only this change can run it (verify finding SL-1).

## Grounding

Chain from `feat/rdd-wave7-verify-remediation` (Wave 7, worktree
`/home/gentleman/work/gentle-ai-worktrees/rdd-wave7`), tip at proposal
time: to be re-validated at this change's own task time, per this
project's standing "re-validate line references at task time" convention.
