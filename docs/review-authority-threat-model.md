# Review Authority Threat Model

## Outcome

The compact review store protects valid authority from accidental corruption and concurrent writers. It does not claim to authenticate state against a malicious local actor with the same user and filesystem access: without an external trust anchor, that actor can rewrite the state, receipt, Git repository, or binary.

## Scope

| Scenario | In scope | Required outcome |
|---|---|---|
| Truncated, malformed, or semantically invalid state | Yes | Validation fails closed; existing authority remains unchanged. |
| Interrupted replacement | Yes | Atomic replacement and filesystem synchronization preserve either the old or new valid record where practical. |
| Concurrent or stale writer | Yes | A lock plus expected revision rejects stale transitions; an exact retry is idempotent. |
| Repository changes after review | Yes | Gates re-derive evidence from live Git and reject incompatible scope or identity changes. |
| Terminal authority needs another review | Yes | `review recover` creates a distinct audited successor; predecessor state and receipt bytes remain immutable. |
| Malicious same-user local actor | No | No authenticity or tamper-resistance claim is made. |

## Retained Controls

- Strict schema and semantic validation before accepting or replacing authority.
- Legal-transition validation against the currently locked state and repository-derived evidence.
- Atomic file replacement, with file and directory synchronization where practical.
- A writer lock and expected revision for concurrent-writer detection.
- Exact retry recognition for idempotent operations.
- Live-Git gate re-derivation rather than trusting persisted mirrors.
- Checksums only where useful for detecting accidental corruption; they are not authentication.

## Review Input Schemas

Run `gentle-ai review schema reviewer`, `gentle-ai review schema refuter`, or `gentle-ai review schema validator` to print the versioned JSON Schema and one accepted example for the existing strict facade input. Final verification evidence remains arbitrary non-empty bytes and therefore has no invented JSON contract. Unknown JSON fields and semantic violations remain rejected before authority changes.

Failed targeted validation stays in the same ordinary bounded lineage. The initial lenses and frozen finding IDs execute once, while each correction attempt records its snapshot, validation checks, and changed-line charge. Cumulative changed lines cannot exceed the original correction budget or expand immutable genesis paths.
Three failed compact correction attempts exhaust the lineage even when their measured delta is zero; this bounds persisted retry history without rerunning review lenses or resetting the budget.

Synthetic Git trees used by persisted snapshots remain subject to repository object pruning. Durable retention needs a separate Git-lifecycle design; this correction does not create hidden refs or weaken missing-object validation.

## Deleted Controls

- Mandatory full transaction snapshots for every transition.
- Replay validation and snapshot-event accumulation on the compact path.
- A local hash chain presented as protection from the out-of-scope actor.
- Mandatory bundles, policy mirrors, ledger mirrors, evidence mirrors, fix-delta mirrors, or gate-context mirrors for ordinary review.

Legacy v1 chains and bundles remain readable for compatibility, but their history cannot be appended, rewritten, repaired, or migrated in place.

## Recovery And Rollback

- Use `gentle-ai review recover` with an explicit predecessor lineage and revision, a distinct successor lineage, a disposition, reason, and actor. Approved predecessors require a proven scope change; invalidated predecessors remain terminal; escalated predecessors additionally require explicit maintainer authorization.
- Recovery runs under the shared v2 store lock, records predecessor and operator provenance in the successor, and starts a new generation and correction budget without rewriting or deleting history.
- Discovery authorizes only a unique valid unsuperseded leaf. Forks, dangling or mismatched predecessor links, cycles, and unrelated leaves fail closed. Explicitly selecting a superseded lineage permits historical inspection but not delivery authorization.
- Recovery imports an explicitly exported compact authority record and binds it to the live delivered tree and original base-to-final path scope; it does not require or reconstruct obsolete intermediate trees or event history.
- Invalid temporary or imported data never replaces the current valid authority.
- Legacy v1 transport remains available only for legacy lineages.
- WU6 can be rolled back by removing the compact store and routing new facade writes back to the preserved v1 implementation. Existing compact records must be exported or intentionally discarded before that rollback; v1 lineages are unaffected.
