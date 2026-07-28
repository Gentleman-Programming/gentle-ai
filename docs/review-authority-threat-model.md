# Review Authority Threat Model

## Outcome

The compact review store protects valid authority from accidental corruption and concurrent writers. It does not claim to authenticate state against a malicious local actor with the same user and filesystem access: without an external trust anchor, that actor can rewrite the state, receipt, Git repository, or binary.

## Scope

| Scenario | In scope | Required outcome |
|---|---|---|
| Truncated, malformed, or semantically invalid state | Yes | Validation fails closed; existing authority remains unchanged. |
| Interrupted replacement | Yes | Atomic replacement and filesystem synchronization preserve either the old or new valid record where practical. |
| Concurrent or stale writer | Yes | A lock plus expected revision rejects stale transitions; an exact retry is idempotent. |
| STATUS overlaps terminal receipt publication | Yes | A bounded double-collect rechecks state revision and snapshot identities, then requires matching receipt and journal existence, raw identity, and canonical content around that state observation; continuing churn returns a concurrency error. |
| Repository changes after review | Yes | Gates re-derive evidence from live Git and reject incompatible scope or identity changes. |
| Historical intended path becomes tracked or disappears | Yes | Read-only status treats frozen membership as receipt-bound history; healthy authority and receipt bytes remain unchanged. |
| Terminal authority needs another review | Yes | Generic `review recover` requires its existing scope/state predicates. Only the dedicated one-shot final-verification boundary may create an unchanged-target validating successor, and predecessor state, receipt, journal, and evidence bytes remain immutable. |
| Malicious same-user local actor | No | No authenticity or tamper-resistance claim is made. |

## Retained Controls

- Strict schema and semantic validation before accepting or replacing authority.
- Legal-transition validation against the currently locked state and repository-derived evidence.
- Atomic file replacement, with file and directory synchronization where practical.
- A writer lock and expected revision for concurrent-writer detection.
- Native authority mutations first take a shared advisory maintenance lock at `<git-common-dir>/gentle-ai/REVIEW-MAINTENANCE.lock`, outside the replaceable `review-transactions` authority subtree, then their lineage or v2 lock; release is reversed. Approved maintenance tools take the same lock exclusively with a bounded context. The lock coordinates cooperative Gentle AI participants only and is not a defense against a malicious same-user actor.
- Exact retry recognition for idempotent operations.
- Live-Git gate re-derivation rather than trusting persisted mirrors.
- Request-scoped status projection: one validated live snapshot is compared in memory with once-loaded authority, graph, and receipt evidence, so terminal history cannot amplify Git subprocesses.
- Checksums only where useful for detecting accidental corruption; they are not authentication.

## Candidate Projections

`gentle-ai review start` defaults to the legacy `workspace` projection. Use `gentle-ai review start --projection staged` to freeze exactly the Git index: unstaged and untracked worktree content is excluded, and the live index and worktree are not modified while deriving evidence. The selected projection is emitted in START JSON and retained by compact authority, receipts, corrections, recovery, and transport. Recovery inherits the predecessor projection when `--projection` is omitted; an explicit cross-projection successor is accepted only for escalated recovery with the canonical authorization bound to that successor's exact target identity.

After committing a staged candidate, use `gentle-ai review start --projection staged --base-ref <ref>` (and `--committed-only` when tracked workspace changes exist) to record immutable base-to-HEAD delivery provenance. It accepts no intended-untracked paths, remains domain-separated in the v2 identity, and can reuse only the matching staged authority; an otherwise identical workspace authority remains separate.

For a staged authority, `post-apply` and `pre-commit` re-derive the exact index; a divergent worktree alone does not broaden or invalidate that candidate. `pre-push` and `pre-pr` continue to validate the delivered commit/base-diff tree against the same staged receipt.

Frozen intended-untracked membership proves what entered the reviewed candidate tree. It is never replayed as a live filesystem precondition during historical STATUS projection. If current `HEAD^{tree}` equals the stored final candidate tree, Git tree identity proves the reviewed bytes and modes were delivered; later tracking or deletion is normal repository evolution. Clean or disjoint follow-up work is unrelated, while overlap or contraction remains scope-changed. Neither case authorizes mutation of the historical state or receipt. Only malformed state, checksum, graph, or receipt evidence is corruption; operational Git and filesystem failures remain errors.

## Review Input Schemas

Run `gentle-ai review schema reviewer`, `gentle-ai review schema refuter`, `gentle-ai review schema validator`, or `gentle-ai review schema final-verification-incident` to print a versioned JSON Schema. Final verification evidence remains arbitrary non-empty bytes and therefore has no invented JSON contract. The incident schema is different: its native parser accepts only compact canonical JSON plus one LF, rejects unknown fields, and admits only `procedural_tooling_failure`. Unknown JSON fields and semantic violations remain rejected before authority changes.

An ordinary bounded lineage permits exactly one changed-target correction attempt. The initial lenses and frozen finding IDs execute once, while that correction records its snapshot, validation checks, and changed-line charge without expanding immutable genesis paths or the frozen budget. Consuming the attempt exhausts ordinary correction even when its measured delta is zero; a later change requires an authorized successor rather than another fix transition. Historical multi-attempt records remain readable, but cannot append another attempt.

Synthetic Git trees used by persisted snapshots remain subject to repository object pruning. Durable retention needs a separate Git-lifecycle design; this correction does not create hidden refs or weaken missing-object validation.

## Deleted Controls

- Mandatory full transaction snapshots for every transition.
- Replay validation and snapshot-event accumulation on the compact path.
- A local hash chain presented as protection from the out-of-scope actor.
- Mandatory bundles, policy mirrors, ledger mirrors, evidence mirrors, fix-delta mirrors, or gate-context mirrors for ordinary review.

Legacy v1 chains and bundles remain readable for compatibility, but their history cannot be appended, rewritten, or migrated in place. The sole repair exception is an audited whole-lineage quarantine described below; it never makes the historical bytes valid.

## Historical Alias Quarantine

Use `gentle-ai review repair --preflight` for the public path. It scans the complete compact-v2 and legacy-v1 authority inventory without mutation and exposes an executable candidate only when exactly one lineage fails solely because of an approved historical `review/validate-fix` or `review/complete-fix` alias. The assessment is bounded, sorted, path-free, and byte-stable. Unknown or mixed corruption, multiple candidates, v1/v2 collision, invalidated authority, active maintenance ownership, concurrent change, or truncated proof returns a stop with no candidate.

The preflight returns provider-owned class `legacy_v1_historical_alias`, lineage, exact current revision, cause `unsupported_historical_v1_operation_alias`, disposition `quarantine-approved-historical-alias`, opaque repository binding, and authorization schema. The maintainer supplies actor, reason, and an exact LF-only nine-line `gentle-ai.review-repair-authorization/v1` binding in this order: schema, `repository`, `class`, `lineage`, `revision`, `cause`, `disposition`, `actor`, `reason`. Public status and repair results never echo that completed authorization, private paths, or quarantine records.

Execution reruns the same full assessment, acquires the common maintenance lock before the lineage lock, and requires the provider inputs and exact current revision to match. It moves the complete immutable lineage into audited quarantine and reads the record back. A concurrent change fails exact CAS; an exact committed retry returns the same path-free execution proof without rewriting quarantine.

`gentle-ai review repair-legacy-alias` remains a compatibility command for established automation. It is the cause-specific inner operation for a legacy-v1 lineage whose replay fails solely with `unsupported historical v1 operation alias`. It accepts only the same approved historical aliases, replays every other event normally, and requires the exact current `HEAD` revision, diagnostic `unsupported historical v1 operation alias`, and disposition `quarantine-approved-historical-alias`.

The maintainer authorization is an exact LF-only eight-line binding with schema `gentle-ai.review-legacy-alias-repair-authorization/v1`, followed by `repository`, `lineage`, `revision`, `diagnostic`, `disposition`, `actor`, and `reason`. The operation refuses unknown aliases, semantic corruption, mixed v1/v2 lineage identity, malformed authorization, stale revisions, and active or shared-maintenance-lock contention.

On success it moves the complete immutable lineage into `review-transactions/quarantine/`, writes a prepared then committed audit record with the re-derived chain identity and alias-event revisions, and reads the inventory back. It does not delete or rewrite authority. The status becomes complete/authoritative only if the remaining inventory is independently clean; unrelated invalid, incomplete, compact-v2, or graph authority continues to fail closed.

## Historical Complete-Fix Scope Quarantine

`gentle-ai review quarantine-legacy-fix-scope` is a separate narrow maintenance operation for immutable legacy-v1 `ordinary_4r` history whose sole semantic anomaly is a historical `review/complete-fix` expanding correction paths beyond frozen genesis scope. It is a quarantine, never validation, rewrite, migration, or repair.

It accepts exactly two literal anomaly sets: `complete_fix_scope_expansion`, or `complete_fix_scope_expansion,validate_fix_alias`. The combined set additionally binds the exact alias event revision and literal `review/validate-fix` operation. Reordered, missing, extra, or unrelated anomalies fail closed.

The only accepted disposition is `quarantine-historical-complete-fix-scope-expansion`. The exact LF-only eleven-line authorization order is `gentle-ai.review-legacy-fix-scope-quarantine-authorization/v2`, `repository`, `lineage`, `revision`, `diagnostic`, `disposition`, `anomaly_set`, `validate_fix_alias_revision`, `validate_fix_alias_operation`, `actor`, `reason`. The command recomputes the complete chain, fix delta, frozen and expanded paths, source lineage, and HEAD under the exclusive maintenance lock before atomically publishing an audited quarantine record. It rejects symlinks, unexpected components, and non-chain event residue at `gentle-ai`, `review-transactions`, `v1`, `quarantine`, lineage, and residue components.

When the source is already absent, replay takes the same exclusive lock, rechecks both source and same-lineage compact-v2 authority, strictly decodes the unique committed audit JSON, re-derives the full physical residue proof, and reads the inventory back. A prepared record found while that lock is held is stale or orphaned, never a successful replay; it cannot hide malformed or conflicting committed data. Fabricated, partial, stale-only, duplicate, or path-mismatched audit records are refused.

## Recovery And Rollback

- Use `gentle-ai review recover` with an explicit predecessor lineage and revision, a distinct successor lineage, a disposition, reason, and actor. `--projection workspace|staged` selects the successor target; omission preserves the predecessor projection for compatibility. Approved predecessors require a proven scope change; invalidated predecessors remain terminal. Only escalated recovery may cross projections, and it requires the exact six-line maintainer authorization binding built from the selected projection's target identity. Authorization diagnostics identify the projection and target identity without exposing repository paths.
- Generic recovery never accepts disposition `final_verification_retry`. `gentle-ai review retry-final-verification` is the sole provider for that edge and only after state, matching receipt, completed receipt-published `review/complete-verification` journal transition, safe failed-evidence bytes, canonical incident, exact live `CurrentSnapshot`, leaf, ancestry, and revision-CAS proof all agree.
- The retry successor copies frozen authority and accounting exactly, increments generation, enters `validating`, clears only the active evidence hash, and records the source attempt and incident in recovery provenance. It creates no reviewer, correction, SDD, or other budget. Exact replay is idempotent; a different replay, collision, target drift, evidence mismatch, existing successor, or any prior retry in ancestry fails before mutation. If retried final verification fails again, the lineage escalates permanently because the ancestry slot is consumed.
- `review recover --release-scope` is the only recovery target-kind expansion with dedicated constraints: an approved `current-changes` predecessor may become a repository-derived first-parent `base-diff` only when the candidate tree and projection are unchanged and every predecessor path remains covered by a strictly larger release scope.
- Recovery runs under the shared v2 store lock, records predecessor and operator provenance in the successor, and starts a new generation and correction budget without rewriting or deleting history.
- Discovery authorizes only a unique valid unsuperseded leaf. Forks, dangling or mismatched predecessor links, cycles, and unrelated leaves fail closed. Explicitly selecting a superseded lineage permits historical inspection but not delivery authorization.
- Recovery imports an explicitly exported compact authority record and binds it to the live delivered tree and original base-to-final path scope; it does not require or reconstruct obsolete intermediate trees or event history.
- Invalid temporary or imported data never replaces the current valid authority.
- Legacy v1 transport remains available only for legacy lineages.
- WU6 can be rolled back by removing the compact store and routing new facade writes back to the preserved v1 implementation. Existing compact records must be exported or intentionally discarded before that rollback; v1 lineages are unaffected.

## Further Reading

[Chapter 21 — Verifiable Trust](https://the-amazing-gentleman-programming-book.vercel.app/en/book/Chapter21_Verifiable-Trust) complements these technical boundaries with the mental model: trust what native authority and delivery gates can deterministically derive, not an agent's narration. Its examples were written around v2.1.2, so use it for the durable principle; this threat model and current CLI documentation define v2.1.4 behavior.
