---
name: rdd-defect-workflow
description: "Trigger: RDD, receipt-driven development, review authority, receipt/lineage, correction/recovery, delivery gate/kill switch, bounded review defects. Guide work."
license: Apache-2.0
metadata:
  author: gentleman-programming
  version: "1.0"
---

## Activation Contract

Load when the frontmatter trigger terms apply to a defect workflow.

This skill guides public collaboration. It does not grant issue approval, label, review, exception, or merge authority.

## Hard Rules

- Check the user-owned RDD kill switch first. When disabled, do not start receipt reviews or fabricate approval; follow ordinary policy and report `disabled/unmanaged`.
- Require an approved issue (`status:approved`) and clean current `main` reproduction before implementation. Audit existing PRs for supersession or conflict; stop or narrow stale claims.
- Group by causal authority invariant. Use one issue and one PR or explicit chain per independent invariant and rollback boundary. Split independent causes; never merge a superseded or conflicting authority line.
- Inventory every operator flow claimed by the issue or PR, including entry, mode, environment, expectation, and negative controls. Require one truthful black-box bench journey per CLI or lifecycle flow, or actual runtime E2E proof when the core bench cannot represent it. Synthetic proxy coverage never proves another runtime.
- Use CodeGraph-first impact mapping, a dedicated worktree, and behavior-first tests. Run source-mutating normalization before candidate freeze.
- Forecast authored changes before edits. The hard limit is 400 additions plus deletions; above it, STOP for a chain or explicit maintainer-approved exception.
- Only when RDD is enabled, bind receipts, lineage, correction, recovery, and delivery gates to the exact candidate. Keep bounded review defects in one correction transaction.
- Require independent read-only candidate validation before publication. Validation cannot edit source or authority; findings require a new candidate.
- Keep communication humane and evidence-based. Repository labels and workflow metadata are maintainer-owned, never evidence of contributor blame.

## Decision Gates

| Condition | Action |
| --- | --- |
| RDD disabled | Ordinary policy; `disabled/unmanaged`; no receipt or approval claim. |
| Issue gate or reproduction fails | Wait, stop, or narrow with evidence. |
| Invariant or rollback is independent | Separate issue and authoritative PR line. |
| Core bench fits / does not fit | Bench journey / actual runtime E2E; never proxy. |
| Forecast exceeds 400 lines | Chain or approved exception before edits. |

## Execution Steps

1. Check mode, approval, PR conflicts, and current-main reproduction.
2. Name invariant and rollback; isolate the worktree; CodeGraph-map code, tests, evidence, docs, distribution, and registration.
3. Inventory flows and controls; add failing tests and the smallest correction.
4. Normalize, enforce budget, run tests, and record each flow's exact candidate, command, scenario, and result.
5. Freeze, validate read-only, and give the verdict, evidence, and one humane next action.

## Output Contract

Return `rdd_mode`, `issue_pr`, `causal_invariant`, `operator_flows`, `journey_runtime_evidence`, `changed_line_budget`, `tests`, `rollback`, and `unresolved_authority_decisions`.

Identify approved and superseded/conflicting authority lines; every flow, negative control, and candidate-bound proof; additions plus deletions and chain/exception; test results; independent rollback; and unresolved maintainer decisions.

## References

No supporting files. Current repository policy remains authoritative.
