# contracts/review-integration/v1 — Read-Only Freeze

**Status: FROZEN, read-only, as of Wave 7 (RDD Root Simplification —
Compatibility Retirement), work unit WU3 (S9a).**

This directory (27 fixtures + 24 schemas, 51 files) is frozen, not deleted,
per proposal decision D3 and design decision (Legacy read retention, D5) —
recorded in:

- `openspec/changes/rdd-root-simplification-wave7/proposal.md`, "D3 —
  `contracts/review-integration/v1` retirement"
- `openspec/changes/rdd-root-simplification-wave7/design.md`, decision 5
  (Legacy read retention)
- `openspec/changes/rdd-root-simplification-wave7/specs/rdd-legacy-retirement/spec.md`,
  "Requirement: Contract v1 Freeze, Not Deletion"

## What "frozen" means

- Every file under this directory MUST stay byte-unchanged for the
  remainder of this wave (S1 through S9b/WU20).
- No new-lineage (v3) behavior consumes this contract; v3 uses
  `contracts/review-integration/v2/**` exclusively.
- This directory is NOT deleted by any Wave 7 work unit. Deletion requires a
  SEPARATE, later, dated change once the support horizon below is proven
  satisfied — never bundled into this deletion wave.

## Why frozen rather than deleted now (D3's rejected alternative)

A pinned adapter release (e.g. a version-pinned gentle-pi) may still call
v1 mid-upgrade. Deleting v1 in this wave would break such a consumer
without warning. The wave's own explicit scope boundary (proposal "Out of
scope") is: no new verb, no new contract version, no behavior change for
the new lineage — freezing v1 keeps that boundary intact while still
letting Wave 7 delete the legacy MUTATION lifecycle that used to write
alongside it.

## Support horizon (open question, tracked not resolved here)

The design's own "Open Questions" section records this as unresolved at
Wave 7 time: *"D3: the declared support horizon for a pinned gentle-pi
consuming contracts/review-integration/v1."* This freeze marker does not
resolve that horizon — it only records that until it is declared and
proven satisfied by a later, separate change, this directory stays exactly
as it is.

## Exit evidence this freeze must show at Wave 7 close-out (WU20)

- [ ] Every file's content hash unchanged from this freeze commit forward
      through the wave's last work unit (verifiable via `git diff
      --stat` against this commit for the `contracts/review-integration/v1/`
      path across every subsequent Wave 7 commit).
- [ ] No file under `internal/` added during Wave 7 imports or reads from
      this directory for new-lineage (v3) behavior.
