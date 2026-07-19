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

## Candidate Projections

`gentle-ai review start` defaults to the legacy `workspace` projection. Use `gentle-ai review start --projection staged` to freeze exactly the Git index: unstaged and untracked worktree content is excluded, and the live index and worktree are not modified while deriving evidence. The selected projection is emitted in START JSON and retained by compact authority, receipts, corrections, recovery, and transport. Recovery inherits the predecessor projection when `--projection` is omitted; an explicit cross-projection successor is accepted only for escalated recovery with the canonical authorization bound to that successor's exact target identity.

After committing a staged candidate, use `gentle-ai review start --projection staged --base-ref <ref>` (and `--committed-only` when tracked workspace changes exist) to record immutable base-to-HEAD delivery provenance. It accepts no intended-untracked paths, remains domain-separated in the v2 identity, and can reuse only the matching staged authority; an otherwise identical workspace authority remains separate.

For a staged authority, `post-apply` and `pre-commit` re-derive the exact index; a divergent worktree alone does not broaden or invalidate that candidate. `pre-push` and `pre-pr` continue to validate the delivered commit/base-diff tree against the same staged receipt.

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

- Use `gentle-ai review recover` with an explicit predecessor lineage and revision, a distinct successor lineage, a disposition, reason, and actor. `--projection workspace|staged` selects the successor target; omission preserves the predecessor projection for compatibility. Approved predecessors require a proven scope change; invalidated predecessors remain terminal. Only escalated recovery may cross projections, and it requires the exact six-line maintainer authorization binding built from the selected projection's target identity. Authorization diagnostics identify the projection and target identity without exposing repository paths.
- `review recover --release-scope` is the only recovery target-kind expansion with dedicated constraints: an approved `current-changes` predecessor may become a repository-derived first-parent `base-diff` only when the candidate tree and projection are unchanged and every predecessor path remains covered by a strictly larger release scope.
- Recovery runs under the shared v2 store lock, records predecessor and operator provenance in the successor, and starts a new generation and correction budget without rewriting or deleting history.
- Discovery authorizes only a unique valid unsuperseded leaf. Forks, dangling or mismatched predecessor links, cycles, and unrelated leaves fail closed. Explicitly selecting a superseded lineage permits historical inspection but not delivery authorization.
- Recovery imports an explicitly exported compact authority record and binds it to the live delivered tree and original base-to-final path scope; it does not require or reconstruct obsolete intermediate trees or event history.
- Invalid temporary or imported data never replaces the current valid authority.
- Legacy v1 transport remains available only for legacy lineages.
- WU6 can be rolled back by removing the compact store and routing new facade writes back to the preserved v1 implementation. Existing compact records must be exported or intentionally discarded before that rollback; v1 lineages are unaffected.

## Further Reading

[Chapter 21 — Verifiable Trust](https://the-amazing-gentleman-programming-book.vercel.app/en/book/Chapter21_Verifiable-Trust) complements these technical boundaries with the mental model: trust what native authority and delivery gates can deterministically derive, not an agent's narration. Its examples were written around v2.1.2, so use it for the durable principle; this threat model and current CLI documentation define v2.1.4 behavior.
