# Review Authority Threat Model

> [!NOTE]
> These docs track `main`, which may include unreleased changes. For the latest release, see the [v4.0.0 docs](https://github.com/Gentleman-Programming/gentle-ai/tree/v4.0.0/docs).

## Outcome

The compact review store protects valid review authority from accidental corruption and concurrent writers. It does not claim to authenticate state against a malicious local actor with the same user and filesystem access: without an external trust anchor, that actor can rewrite the state, receipt, Git repository, or binary.

Review authority governs review lifecycle continuation and recovery only. Receipts, candidate identity, gate/context results, and shadow output are review-context evidence; ordinary repository policy governs delivery.

## Scope

| Scenario | In scope | Required outcome |
|---|---|---|
| Truncated, malformed, or semantically invalid state | Yes | Validation fails closed; existing authority remains unchanged. |
| Interrupted replacement | Yes | Atomic replacement and filesystem synchronization preserve either the old or new valid record where practical. |
| Concurrent or stale writer | Yes | A lock plus expected revision rejects stale transitions; an exact retry is idempotent. |
| STATUS overlaps terminal receipt publication | Yes | A bounded double-collect rechecks state revision and snapshot identities, then requires matching receipt and journal existence, raw identity, and canonical content around that state observation; continuing churn returns a concurrency error. |
| Repository changes after review | Yes | Review-context evaluation re-derives evidence from live Git and reports incompatible scope or identity changes for review repair; ordinary delivery remains independently governed. |
| Historical intended path becomes tracked or disappears | Yes | Read-only status treats frozen membership as receipt-bound history; healthy authority and receipt bytes remain unchanged. |
| Terminal escalation needs another review | Yes | Generic `review recover` requires its existing scope/state predicates. Escalation remains receiptless and recoverable, while predecessor authority bytes remain immutable. |
| Malicious same-user local actor | No | No authenticity or tamper-resistance claim is made. |

## Retained Controls

- Strict schema and semantic validation before accepting or replacing authority.
- Legal-transition validation against the currently locked state and repository-derived evidence.
- Atomic file replacement, with file and directory synchronization where practical.
- A writer lock and expected revision for concurrent-writer detection.
- Native authority mutations first take a shared advisory maintenance lock at `<git-common-dir>/gentle-ai/REVIEW-MAINTENANCE.lock`, outside the replaceable `review-transactions` authority subtree, then their lineage or v2 lock; release is reversed. Approved maintenance tools take the same lock exclusively with a bounded context. The lock coordinates cooperative Gentle AI™ participants only and is not a defense against a malicious same-user actor.
- Exact retry recognition for idempotent operations.
- Live-Git review-context re-derivation rather than trusting persisted mirrors; its result governs review continuation or recovery only, never delivery or archive.
- Request-scoped status projection: one validated live snapshot is compared in memory with once-loaded authority, graph, and receipt evidence, so terminal history cannot amplify Git subprocesses.
- Checksums only where useful for detecting accidental corruption; they are not authentication.

## Candidate Projections

`gentle-ai review start` defaults to the legacy `workspace` projection. Use `gentle-ai review start --projection staged` to freeze exactly the Git index: unstaged and untracked worktree content is excluded, and the live index and worktree are not modified while deriving evidence. The selected projection is emitted in START JSON and retained by compact authority, receipts, corrections, recovery, and transport. Recovery inherits the predecessor projection when `--projection` is omitted; an explicit cross-projection successor is accepted only for escalated recovery with the canonical authorization bound to that successor's exact target identity.

After committing a staged candidate, use `gentle-ai review start --projection staged --base-ref <ref>` (and `--committed-only` when tracked workspace changes exist) to record immutable base-to-HEAD review provenance. It accepts no intended-untracked paths, remains domain-separated in the v2 identity, and can reuse only the matching staged authority; an otherwise identical workspace authority remains separate.

A staged review freezes the exact index; a divergent worktree does not broaden what was reviewed. `post-apply`, `pre-commit`, `pre-push`, `pre-pr`, and release remain ordinary repository delivery operations: review evidence is never a delivery requirement, and those operations neither create review authority nor reopen a completed review.

Frozen intended-untracked membership records what entered the reviewed candidate tree. It is never replayed as a live filesystem precondition during historical STATUS projection. If current `HEAD^{tree}` equals the stored final candidate tree, Git tree identity shows the reviewed bytes and modes match that historical snapshot; later tracking or deletion is normal repository evolution. Clean or disjoint follow-up work is unrelated, while overlap or contraction remains scope-changed. Neither case authorizes mutation of historical review state. Only malformed state, checksum, graph, or receipt evidence is corruption; operational Git and filesystem failures remain errors.

## Review Input Schemas

Run `gentle-ai review schema reviewer`, `gentle-ai review schema refuter`, or `gentle-ai review schema validator` to print a versioned JSON Schema. Each admitted capture is bound to its immutable candidate, revision, and selected role. Unknown fields and semantic violations are rejected before authority changes.

An ordinary bounded lineage permits exactly one changed-target correction attempt. The attempt and changed-line charge are appended only when one candidate-bound bundle contains passed repository/full-suite evidence and matching targeted validation for the same authority revision, candidate identity, paths, and frozen finding IDs. A repository verification failure before that atomic boundary leaves the transaction open and preserves failed candidate evidence under its own identity. Once accepted, even a zero-line measured delta exhausts ordinary correction; a later change requires an authorized successor. Historical multi-attempt records remain readable, but cannot append another attempt.

Synthetic Git trees used by persisted snapshots remain subject to repository object pruning. Durable retention needs a separate Git-lifecycle design; this correction does not create hidden refs or weaken missing-object validation.

## Deleted Controls

- Mandatory full transaction snapshots for every transition.
- Replay validation and snapshot-event accumulation on the compact path.
- A local hash chain presented as protection from the out-of-scope actor.
- Mandatory bundles, policy mirrors, ledger mirrors, evidence mirrors, fix-delta mirrors, or gate-context mirrors for ordinary review.

Legacy v1 authority is no longer supported: status, inspection, abandon, lineage-name occupancy and import collision checks do not read `review-transactions/v1`, and nothing deletes or rewrites what is already there. A clone that still carries a v1 directory keeps working as if it were absent; `gentle-ai review store-reset` is the human tool that removes it.

## Retired Legacy v1 Repair

Legacy v1 authority is unsupported, so its repair and quarantine providers are deleted: the classified `legacy_v1_historical_alias` repair class, `review repair-legacy-alias`, `review quarantine-legacy` and `review quarantine-legacy-fix-scope`. `gentle-ai review repair --preflight` assesses compact-v2 authority only and never offers a classified candidate; `review repair` executes only a leaf authority disposition plan.

A quarantine record those providers already committed (`legacy_alias_repair`, `malformed_legacy_freeze`, `legacy_fix_scope_quarantine`) is closed history. It stays decodable and byte-preserved as opaque JSON beside the other reclaim records; nothing re-derives, replays or interprets its proof, and no command acts on it.

## Rejected Validator Terminal Disposition

A canonical rejected targeted-validator verdict is valid authority, not corruption. An unchanged candidate still stops, and ordinary abandonment and recovery restrictions remain in force. An isolated terminal with a consumed correction and no recovery dependencies can be released only by an explicit maintainer decision.

1. Run `gentle-ai review repair --preflight --contract gentle-ai.review-integration/v2 --cwd <repo>`. In `disposition_selectors`, find the exact successor-only entry for the rejected lineage. Repeat preflight with `--successor-lineage <successor_lineage_id> --successor-revision <successor_expected_revision>` from that entry.
2. For this eligible exact selection, `disposition_provider_inputs` gives `repository_binding`, `plan_digest`, and `authority_inventory_revision`. If `repository_binding` is absent, this recipe does not apply. Do not use `assessment.authorization_schema`: it describes legacy classified repair, not leaf disposition.
3. If you authorize release, choose a single-line actor and reason. Build the following eight lines separated by LF, with no trailing newline. Replace the first three placeholders with the selected preflight values and the last two with your actor and reason, trimming any surrounding whitespace from actor and reason.

   ```text
   gentle-ai.review-disposition-authorization/v1
   schema=gentle-ai.review-authority-disposition-plan/v1
   repository=<repository_binding>
   class=rejected_targeted_validator_terminal
   plan_digest=<plan_digest>
   inventory_revision=<authority_inventory_revision>
   actor=<actor>
   reason=<reason>
   ```

4. Repeat the exact selected command without `--preflight`. Add `--plan-digest <plan_digest> --inventory-revision <authority_inventory_revision> --actor <actor> --reason <reason> --authorization <binding>`. Pass the full binding as one argument. Execution rederives eligibility under the maintenance lock, validates authorization and revisions, and retains byte-identical rejected authority with an audit proof. Exact replay converges on that proof.

The class is `rejected_targeted_validator_terminal`. Disposition releases only the active governing claim: it does not approve the candidate, replace the historical verdict, restore a correction attempt, disable RDD, or automatically launch another review. It is not delivery permission. Missing canonical evidence, stale or forged bindings, non-rejected terminals, incomplete inspection, and retained recovery dependencies fail closed. Unrelated historical and content-mismatch repair behavior is unchanged.

## Recovery And Rollback

- Use `gentle-ai review recover` with an explicit predecessor lineage and revision, a distinct successor lineage, a disposition, reason, and actor. `--projection workspace|staged` selects the successor target; omission preserves the predecessor projection for compatibility. Approved predecessors require a proven scope change; invalidated predecessors remain terminal. Only escalated recovery may cross projections, and it requires the exact six-line maintainer authorization binding built from the selected projection's target identity. Authorization diagnostics identify the projection and target identity without exposing repository paths.
- Generic recovery never restores burned approval authority. Escalated review remains receiptless and may recover only through the existing scope/state predicates and a distinct successor lineage.
- `review recover --release-scope` is the only recovery target-kind expansion with dedicated constraints: an approved `current-changes` predecessor may become a repository-derived first-parent `base-diff` only when the candidate tree and projection are unchanged and every predecessor path remains covered by a strictly larger release scope.
- Recovery runs under the shared v2 store lock, records predecessor and operator provenance in the successor, and starts a new generation and correction budget without rewriting or deleting history.
- Discovery authorizes only review continuation or recovery from a unique valid unsuperseded leaf. Forks, dangling or mismatched predecessor links, cycles, and unrelated leaves fail closed inside the review lifecycle. Explicitly selecting a superseded lineage permits historical inspection but not review continuation or delivery authorization.
- Recovery imports an explicitly exported compact authority record and binds it to the live delivered tree and original base-to-final path scope; it does not require or reconstruct obsolete intermediate trees or event history.
- Invalid temporary or imported data never replaces the current valid authority.
- WU6 can be rolled back by removing the compact store and routing new facade writes back to the preserved v1 implementation. Existing compact records must be exported or intentionally discarded before that rollback; v1 lineages are unaffected.

## Further Reading

[Chapter 21 — Verifiable Trust](https://the-amazing-gentleman-programming-book.vercel.app/en/book/Chapter21_Verifiable-Trust) complements these technical boundaries with the mental model: trust native review authority only for review-lifecycle facts, not an agent's narration. Ordinary repository policy owns delivery; this threat model and current CLI documentation define v2.1.4 behavior.
