# Native Bounded Review Orchestration

Parent orchestrator and native CLI only. Never pass this contract to a reviewer, refuter, judge, correction actor, or validator. Those roles receive only scope, candidate-causal admission, severity, evidence requirements, and output shape.

## Route

Begin every generated negotiated lifecycle route with `gentle-ai review status --cwd <repo> --contract gentle-ai.review-integration/v1 --next-transition`. Read only the returned `next_transition`: route only from the returned `next_transition`, never from status prose, lifecycle state, or eligibility. For `execute`, invoke its exact operation and ordered argument tokens unchanged. For `collect`, satisfy only its named inputs with their exact capture operations and arguments, then query STATUS again. For `stop`, stop and surface its `reason_code` without running a lifecycle operation. Never hardcode or substitute START: invoke `review.start` only when the returned `execute.operation` names it. Direct `gentle-ai review start` remains compatibility-supported for explicit/manual non-negotiated callers. The native facade discovers repository scope, derives the immutable target, selects zero lenses for low risk, one focus lens for standard risk, or canonical 4R for high risk, and freezes the original line count, tier, and correction budget `min(200, ceil(original_changed_lines / 2))`. Goldens stay in snapshot identity but not that count. Correction and compatible base advance never recalculate risk or open review.

If the exact provider-returned START answers with the typed `gentle-ai.review-integration.consent/v1` envelope, treat it as a Lossless Blocking Prompt under the orchestrator contract: relay its complete choice envelope — headline, reason, risk evidence, both choices, and the documented off path — then run exactly the one named follow-up invocation for the human's answer, never answering on their behalf. Do not append `--consent relay` or any other argument to a returned transition. A decline is scoped to that one candidate and is not the kill switch.

A canonical four-lens selection is long work: before the first lens runs, give the one cost/side-effect forecast — four reviewer model runs over the frozen candidate, the frozen correction budget, and the at-most-one bounded correction it implies — once per candidate, never per lens.

Run each exact `review.capture-result` collection input once in the foreground. Begin its reviewer task prompt with the exact literal prefix `GENTLE_AI_REVIEW_BINDING `, including the trailing space and never `=`, immediately followed by one-line bound JSON assembled only from that input's arguments and `artifact_subject`: `lineage`, `target`, `lens`, `order`, `revision` from `expected-revision`, `repository_context` from `repository-context`, and `subject_hash` from `artifact_subject.subject_hash`; omit only fields the provider omitted. The prefix and JSON are the first bytes of the prompt. Return one JSON object echoing `subject_hash`; require `inspection.status: "completed"`, all manifest paths in order as `inspection.paths`, `findings`/`evidence`, and severe `evidence_class`/`causal_disposition`; access failure is not completion. `gentle-ai review capture-result` follows the native transition; handles are cwd-independent and legacy bindings need `--cwd`. Pass manifests in lens order with repeated `--result-artifact-file <path>` arguments, BOM-less UTF-8 on Windows PowerShell 5.1. The POSIX inline `--result-artifact '<manifest-json>'` form remains compatible; so does provider-owned `--captured-results`; never pass raw `--result`. Native Go validates, canonicalizes, persists, hashes, reopens, and binds results; models never construct canonical bytes or hashes. Freeze merged findings. Only `introduced`, `behavior-activated`, or `worsened` with changed-hunk, candidate-created-path, differential-test, or before/after proof may block. Route `pre-existing` and `base-only` to follow-ups; `unknown` escalates. WARNING/SUGGESTION remain `info`. Deterministic blockers need no refuter; inferential blockers share one read-only refuter batch. Judgment Day uses two independent judges.

Reviewer input transport is provider-owned. Never hand a reviewer input through `/tmp`, another external file, a repository scratch file, or any path reference. Never supply `GENTLE_AI_FROZEN_CANDIDATE_CONTEXT`; rely on native/plugin injection. The OpenCode review-result plugin appends the artifact subject, exact candidate diff, and changed-path manifest only after native preflight succeeds. If injection is unavailable, stop without launching the reviewer.

Ordinary review permits one correction transaction. When `next_transition.collect` requests `correction_lines`, provide a positive forecast before editing and continue only through the next provider-returned transition. After the bounded edit, run one read-only scoped fix validator only when the exact collection input requests it, then return its targeted result and final test/verification evidence through the exact named capture operations and arguments. The facade maps correction only to corroborated frozen IDs and genesis paths, rejects over-budget repository evidence, and creates or discovers the terminal receipt. Later observations are follow-ups, not another correction. Judgment Day alone keeps its existing two-round rule. SDD then runs one independent requirements/runtime verification. Failure escalates and never starts another reviewer, refuter, correction, or validator.

<!-- authority-first-terminal-procedure:start -->
### Authority-First Terminal Procedure

Use only the compact facade; it appends and reads back native authority before materializing existing compatibility artifacts.

| Order | Operation | Required result | Terminal mirrors |
|---|---|---|---|
| 01 | `gentle-ai review status --cwd <repo> --contract gentle-ai.review-integration/v1 --next-transition` | one provider-owned `next_transition` returned | blocked |
| 02 | `provider-returned transition` | exact `execute` operation/arguments or `collect` inputs completed; `stop` halts | blocked |
| 03 | repeat 01–02 | exact returned `review.validate` allows the terminal gate | blocked |
| 04 | `reconcile-terminal-mirrors` | existing mirrors reconciled | allowed |

After ambiguous output, query STATUS again; native discovery reports the committed authority and its next transition without another budget. Malformed or ambiguous lineage remains invalid.
<!-- authority-first-terminal-procedure:end -->

## Delivery

Repository Git common-dir CAS remains authoritative. Existing transaction, policy, ledger, receipt, bundle, and gate-context schemas, prerequisites, and compatibility behavior remain unchanged in this work unit. Reconcile mirrors only after native allow. Supported lifecycle CLI gates are `post-apply`, `pre-commit`, `pre-push`, `pre-pr`, and `release`; they discover and validate the same receipt and never launch reviewers or create a budget. Archive still requires structured status with `reviewGate.result: allow` and its approved receipt. Model/provider/profile selection remains user-owned.

Before commit, stage all reviewed paths without content/mode changes, then validate pre-commit. Frozen intended-untracked paths must remain all untracked or all move to an index whose complete tree and paths match the receipt.
