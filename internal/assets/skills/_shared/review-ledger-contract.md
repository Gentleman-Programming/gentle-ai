# Native Bounded Review Orchestration

Parent orchestrator and native CLI only. Never pass this contract to a reviewer, refuter, judge, correction actor, or validator. Those roles receive only scope, candidate-causal admission, severity, evidence requirements, and output shape.

## Route

Call `gentle-ai review start` once. The native facade discovers the repository root and untracked scope, derives the immutable target, selects zero lenses for low risk, one focus lens for standard risk, or canonical 4R for high risk, and freezes the original line count, tier, and correction budget `min(200, ceil(original_changed_lines / 2))`. Goldens stay in snapshot identity but not that count. Correction and compatible base advance never recalculate risk or open review.

Run each selected lens once in the foreground. Prefix its task prompt with `GENTLE_AI_REVIEW_BINDING {"lineage":"<lineage>","target":"<target_identity>","lens":"<lens>","order":<selected_order>}` from START. Capture its JSON with `gentle-ai review capture-result --cwd <repo> --lineage <lineage> --target <target_identity> --lens <lens> --order <selected_order> --input <file-or-stdin>`; OpenCode's managed hook does this automatically. Pass every emitted manifest to FINALIZE in selected-lens order as repeated `--result-artifact '<manifest-json>'` arguments, never raw `--result` files. Native Go validates, canonicalizes, persists, hashes, reopens, and binds results; models never construct canonical bytes or hashes. Freeze merged findings and classify every severe finding. Only `introduced`, `behavior-activated`, or `worsened` with changed-hunk, candidate-created-path, differential-test, or before/after proof may block. Route `pre-existing` and `base-only` to follow-ups; `unknown` escalates. WARNING/SUGGESTION remain `info`. Deterministic blockers need no refuter; all inferential blockers share one read-only refuter batch. Judgment Day uses two independent judges instead.

Before each lens, append the exact immutable candidate diff and changed-path manifest from START; if unavailable, stop.

Ordinary review permits one correction transaction. When finalize reports correction required, rerun it with a positive `--correction-lines` forecast before editing. After the bounded edit, run one read-only scoped fix validator and pass its targeted result with `--validation <file>` plus final test/verification evidence with `--evidence <file>`. The facade maps correction only to corroborated frozen IDs and genesis paths, rejects over-budget repository evidence, and creates or discovers the terminal receipt. Later observations are follow-ups, not another correction. Judgment Day alone keeps its existing two-round rule. SDD then runs one independent requirements/runtime verification. Failure escalates and never starts another reviewer, refuter, correction, or validator.

<!-- authority-first-terminal-procedure:start -->
### Authority-First Terminal Procedure

Use only the compact facade; it appends and reads back native authority before materializing existing compatibility artifacts.

| Order | Operation | Required result | Terminal mirrors |
|---|---|---|---|
| 01 | `gentle-ai review start` | target, tier, lenses, and budget bound | blocked |
| 02 | `gentle-ai review finalize` | results, evidence, native transitions, and receipt bound | blocked |
| 03 | `gentle-ai review validate --gate <gate> --cwd <repo>` | authority, receipt, and live Git checked | blocked |
| 04 | `reconcile-terminal-mirrors` | existing mirrors reconciled | allowed |

After ambiguous output, rerun the same facade operation; native discovery resumes committed authority without another budget. Malformed or ambiguous lineage remains invalid.
<!-- authority-first-terminal-procedure:end -->

## Delivery

Repository Git common-dir CAS remains authoritative. Existing transaction, policy, ledger, receipt, bundle, and gate-context schemas, prerequisites, and compatibility behavior remain unchanged in this work unit. Reconcile mirrors only after native allow. Supported lifecycle CLI gates are `post-apply`, `pre-commit`, `pre-push`, `pre-pr`, and `release`; they discover and validate the same receipt and never launch reviewers or create a budget. Archive still requires structured status with `reviewGate.result: allow` and its approved receipt. Model/provider/profile selection remains user-owned.

Before commit, stage all reviewed paths without content/mode changes, then validate pre-commit. Frozen intended-untracked paths must remain all untracked or all move to an index whose complete tree and paths match the receipt.
