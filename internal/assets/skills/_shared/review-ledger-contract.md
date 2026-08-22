# Native Compact Review Orchestration

The parent orchestrator coordinates one native transaction; reviewers, refuters, correction actors, and validators receive only their provider-issued role input. Prompt prose never creates authority or decides delivery.

## Atomic lifecycle

1. **Preflight only.** Selectorless STATUS only preflights the current worktree candidate and returns one exact START invocation. It never discovers, resumes, recovers, or evaluates ambient authority from another lineage or worktree: `gentle-ai review status --cwd <repo> --contract gentle-ai.review-integration/v2 --agent {{GENTLE_AI_RUNTIME_AGENT_ID}} --next-transition`.

2. **Freeze once.** Invoke only the returned START operation and its ordered tokens unchanged. START freezes one compact atomic transaction with an explicit lineage, worktree, and target binding. It ignores every other lineage and worktree. Capture the returned lineage, revision, and target tokens. An exact replay of an active START may return `replayed`; a genuinely new START is independent.

3. **Stay bound.** Every later STATUS, collection, and FINALIZE call for that transaction passes the exact captured lineage, revision, and target tokens. Route only from that transaction's returned `next_transition`; never infer a command from prose, ambient state, a gate, or a stale reply. Do not start another lineage, reuse a burned lineage, or perform ambient recovery. For `execute`, run the exact operation and ordered arguments. For `collect`, satisfy only the named inputs and their exact capture operations, then ask STATUS again with the same binding. For `stop`, run no lifecycle operation.

4. **Terminate atomically.** Native Go owns frozen lenses, provider context and admission, refutation, one bounded correction, repository evidence, and targeted validation. It reads back successful approval, then burns that exact authority and its artifacts before returning `approved`. No terminal receipt, tombstone, witness, mirror, or delivery authority survives. Unrelated transactions survive unchanged.

Clean FINALIZE success stops with no terminal STATUS. After any non-clean FINALIZE result, malformed or no output, transport loss, or post-mutation processing failure, issue exactly one retained target-bound read-only STATUS before replay. STATUS is read-only; FINALIZE alone owns mutation and burn. If that STATUS returns `original_finalize_request_required`, replay only the exact original FINALIZE request it identifies. Never blindly replay or invent a new binding.

When v2 returns `forecast`, relay it losslessly in the user's language: preserve every step's order and fields (`step`, `kind`, `reason_code`, `description`) and the horizon. Forecast is informational; route only from `next_transition`.

### Cross-repository lifecycle root

A session in repository A may review an explicitly selected nested target in unrelated repository B only after explicit user authorization. Native Go resolves the requested path to the canonical B worktree root; adapters never parse authorization or roots.

- After B is selected, retain canonical B as the lifecycle working root from selectorless STATUS through consent, collection, correction, validation, FINALIZE, and burn. Do not fall back to A.
- Run provider-issued command tokens exactly. Never append, remove, or rebuild provider-issued command tokens. When a command omits `--cwd`, run it with process cwd B.
- Opaque `repository_context` can capture or materialize from any process cwd, but the host still retains B for lifecycle continuity. Go owns repository binding; adapters never parse authorization or roots.
- The same lineage text in A and B is independent. Approval burns B only; A remains untouched.

This lifecycle is rendered for exactly Claude Code, Codex, OpenCode, and Pi. Unsupported runtimes remain unavailable before repository or authority mutation.

## Capture and correction

For each returned `review.capture-result` input, run the exact capture operation once. The reviewer prompt begins with the exact literal prefix `GENTLE_AI_REVIEW_BINDING ` (trailing space, never `=`), followed by one-line JSON assembled only from that input: `lineage`, `target`, `lens`, `order`, `revision` from `expected-revision`, `repository_context`, and `subject_hash` from `artifact_subject.subject_hash`; omit only provider-omitted fields. Return one JSON object that echoes `subject_hash`, reports completed inspection of every manifest path in order, and contains findings/evidence with severe evidence class and causality. Access failure is incomplete inspection, never completion.

After an empty, malformed, schema-invalid, access/provider-failed, or incomplete capture, query the same exact-lineage STATUS. Relaunch only if its fresh `next_transition` reoffers the same bound slot. Never infer a retry from transcript text. Use repeated `--result-artifact-file <path>` in lens order; BOM-less UTF-8 is required on Windows PowerShell 5.1. POSIX inline `--result-artifact '<manifest-json>'` and provider-owned `--captured-results` remain compatibility forms.

Only candidate-caused severe findings block. Pre-existing/base-only findings are follow-ups; unknown causality escalates. A deterministic blocker needs no refuter; inferential blockers share one read-only refuter batch. A four-lens review is long work: before its first lens, give one forecast covering four reviewer runs, the frozen correction budget, and the at-most-one bounded correction.

A correction is native-scoped. Before editing, give the provider-requested positive forecast. Native Go maps edits only to corroborated frozen findings, owns repository evidence and the targeted validator, and permits at most one bounded correction. A validator that cannot inspect the immutable trees produced no verdict: surface one blocked human decision and submit nothing. Do not route it to a refuter or another actor without read-only immutable-tree access. Independent requirements/runtime verification never starts another reviewer, refuter, correction, or validator.

## Consent and immutable inspection

If exact provider-returned START returns the typed `gentle-ai.review-integration.consent/v3` envelope, relay it as a Lossless Blocking Prompt. Global RDD enabled permits review; it never grants consent for this candidate. For medium/high candidates, faithfully translate the headline, reason, `value`, risk evidence, choice labels, every choice `effect`, and the off-path note while preserving original groups/order, selection mode, allowed-answer domain, answer tokens, commands, target IDs, and invocations. Project `value` as benefits and every `effect` as consequences. Do not translate machine answer tokens (`granted`, `declined`). Run exactly the invocation selected by the human; a decline is candidate-scoped and is not the kill switch.

Claude Code, OpenCode, Codex, and Pi use the shared Go provider contract. Go owns frozen evidence, binding, schema, byte bounds, validation, admission, and capture; runtime adapters transport opaque provider output. Claude uses a tool-free fresh reviewer; OpenCode relays one host Task through its live Go transport; Codex uses its provider-bound subprocess; Pi uses its gentle-pi-owned relay. Compiled capability is authoritative before repository, target, authority, collection, or process work.

Reviewers inspect only the provider-bound immutable trees. Never hand candidate bytes through `/tmp`, an external file, a repository scratch file, or `GENTLE_AI_FROZEN_CANDIDATE_CONTEXT`. Use the provider-issued inspection path, never the live worktree, index, `HEAD`, or an unbound revision. Never pass `--binary`, change checkout, or substitute live files.

<!-- authority-first-terminal-procedure:start -->
### Authority-First Terminal Procedure

| Order | Operation | Required result |
| --- | --- | --- |
| 01 | canonical initial STATUS above | exactly one current-worktree START preflight; no authority discovery |
| 02 | exact returned START | one compact lineage/worktree/target binding; retain lineage, revision, and target |
| 03 | exact-lineage STATUS, collect, and FINALIZE | only returned transaction actions; no ambient resume, reuse, or delivery gate |
| 04 | successful FINALIZE | native readback, exact authority/artifact burn, then `approved` |
| 05 | terminal lifecycle stop | ordinary repository policy owns any later delivery decision |

<!-- authority-first-terminal-procedure:end -->

### Continue after a stop reason code

A `stop` ends its transition, never approves delivery. Complete atomic inventory: B stays target root; gates stay unmanaged. `D` means `gentle-ai review mode disable --scope clone --cwd <B>`; B ordinary policy decides delivery. `S` means re-query the exact captured target-root STATUS command with lineage and target.

| Reason codes | Continuation |
| --- | --- |
| `captured_verification_evidence_invalid`, `captured_artifacts_unverifiable` | Terminal — maintainer inspects B authority, or `D`. |
| `captured_result_selection_unavailable`, `final_verification_retry_unavailable` | Terminal — maintainer inspects lineage, or `D`. |
| `missing_authority_binding` | Terminal — file a bounded defect with lineage, or `D`. |
| `corrupted_or_unverifiable_authority`, `manual_intervention_required`, `native_stop_required` | Terminal — maintainer inspects authority/lineage, or `D`. |
| `empty_base_diff_bootstrap_required` | Terminal — authorized empty-root bootstrap for a new target, or `D`. |
| `lens_context_budget_exceeded` | Terminal — reduce B scope and start a new transaction, or `D`. |
| `staged_workspace_overlay_recovery_unavailable` | Terminal — pass `--lineage <id>` to recover, or drop `--workspace-overlay` and start fresh; otherwise `D`. |
| `unchanged_or_unverified_authority` | Terminal — unchanged `review start` resumes its lineage; change B and start new, or `D`. |
| `corrected_candidate_unavailable`, `correction_repository_verification_failed` | Change B correction candidate, then `S`; do not reuse the pre-correction target and collect new evidence after verification failure. |
| `original_finalize_request_required` | Re-run `gentle-ai review finalize --lineage <id>` with the exact original content-bound payload. |
| `recovery_scope_unchanged` | Change B target identity, then retry the exact returned `gentle-ai review recover`. |
| `rdd_disabled` | Enable `gentle-ai review mode enable --scope clone --cwd <B>`, then `S`. |

## Delivery follows ordinary repository policy

Shipped `review validate` and gate commands are compatibility/informational only. They never discover authority or decide delivery: enabled gates return `invalidated/unmanaged`; disabled gates return `disabled/unmanaged`. They never allow, approve, block, commit, push, open a PR, or govern release.

After terminal `approved` burn, the review lifecycle stops. Commit, push, PR, and release remain separate human decisions under ordinary repository policy. A review outcome is informational and never authorizes delivery, including when the selected repository is B.

Historical compatibility commands may read older artifacts manually, but they are never the ordinary lifecycle and never restore delivery authority.
