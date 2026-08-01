# Proposal: Restore Organic Post-Candidate RDD

## Intent

Keep Gentle AI an ecosystem configurator: agents implement organically; RDD begins after a candidate exists. Remove the unreleased control plane without losing invariants, contributions, or safe shutdown.

## Scope

### In Scope

- Install direct, delegated, and optional-SDD implementation routing for every configured agent, independently of optional SDD component selection.
- Implement proportional post-candidate RDD: trivial/docs-only uses no AI review; standard uses one consolidated review; genuinely high-risk uses focused 4R; ordinary review permits at most one scoped correction.
- Reuse one content-bound local receipt across flexible delivery gates.
- Provide an explicit user kill switch: reject new RDD starts, retain read-only status/replay/validation, and never restore retired behavior.
- Classify every snapshot and branch artifact before obsolete callers are removed.
- Reconcile 241 issues and 92 PRs, including 74 colliding PRs, 499 overlaps, and 16 cross-context decompositions with contributor credit.
- Preserve retained invariants from the nine systemic contexts with applicable exact-SHA, cross-OS, and real-agent proof.

### Out of Scope

- Remote/model runtime, daemon, enterprise control plane, or universal pre-intake WorkRun.
- Changing explicit Judgment Day's existing rounds; that requires a separate product change.
- Forced GitHub delivery or pre-release backlog closure.

## Capabilities

### New Capabilities

- `systemic-recovery-traceability`: Proof ledgers for preservation and safe removal.

### Modified Capabilities

- `organic-agent-trigger-rules`: Remove WorkRun negotiation while retaining organic routing and exposing safe disablement.
- `review-findings-ledger`: Make ordinary review proportional, post-candidate, locally bound, and limited to one correction.
- `sdd-orchestrator-assets`: Project corrected product guidance consistently across adapters.

## Approach

Freeze and classify the branch. Transplant safeguards before deleting remote-only callers. Regenerate derived assets, then prove kill-switch behavior, organic routing, proportional review, receipt reuse, and delivery. This uses SDD only.

## Authority Precedence

The systemic audit owns history, nine-context invariants, edge cases, security, and kill switches. SDD may order recovery only within recorded owner-approved deviations. Silence never permits a new decision; cross-domain topics follow systemic authority. The owner authorizes early removal only for per-path proven-unpublished items; published paths retain systemic migration order.

## Affected Areas

`internal/{agents/capabilitymanifest,app,assets,cli,components/agentguidance,components/sdd,deliveryadmission,doctor,recoverytrace,reviewtransaction,sddstatus,state,workprovider,workrun}`, `contracts/`, `e2e/`, `testdata/golden/`, `docs/audits/data/`, tests, and audits.

## Risks

| Risk | Mitigation |
|---|---|
| Orphaned invariant or credit | Exact disposition/proof ledgers before deletion |
| Cross-platform or derived drift | Applicable platform/agent proof and canonical regeneration |

## Rollback Plan

Use the product kill switch for safe disablement and append-only corrective commits or `git revert` for source rollback. Never force-push.

## Success Criteria

- [ ] No retained invariant, contributor, or branch artifact is orphaned.
- [ ] Kill switch, organic routing, proportional ordinary RDD, receipt reuse, and flexible delivery pass applicable proof.
- [ ] Backlog closure waits for released evidence; this change is validated through SDD only.
