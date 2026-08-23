# Review Integration Contract

← [Back to README](../README.md)

`gentle-ai.review-integration/v2` coordinates one immutable review transaction at a time. Go owns the candidate snapshot, review admission, correction boundary, terminal burn, and all provider-facing bindings. Claude Code, OpenCode, Codex, and Pi transport provider-issued work; no runtime adapter decides review or delivery.

## RDD starts off

RDD is opt-in. Until a user enables it with `gentle-ai review mode enable --scope global`, review does not govern the candidate and delivery follows ordinary repository policy. Disabling returns to that state. Enabling revalidates the current candidate; it never resumes stale authority.

## Quick path

1. Preflight only the current worktree with selectorless negotiated STATUS.
2. Execute its exact START, then retain the returned lineage, revision, and target tokens.
3. Use those exact tokens for every STATUS and capture call until native approval burns the transaction.
4. Follow ordinary repository policy for commit, push, PR, release, and archive.

```bash
gentle-ai review status \
  --cwd <repo> \
  --contract gentle-ai.review-integration/v2 \
  --agent claude-code \
  --next-transition
```

## Cross-repository root

A session in repository A may review a nested target in unrelated repository B only after the user explicitly authorizes B. Native Go resolves the requested path to B's canonical worktree root; adapters carry opaque provider output and never parse authorization or roots.

| Rule | Contract |
| --- | --- |
| Lifecycle root | After B is selected, the host keeps canonical B from STATUS through consent, collection, correction, targeted validation, and burn. A is never a fallback. |
| Commands | Run provider-issued tokens unchanged. If a command omits `--cwd`, run it with process cwd B. |
| Opaque capture | `repository_context` can materialize or capture from another process cwd, but remains B-bound. |
| Isolation | Equal lineage text in A and B names independent transactions. Approval burns B only; A remains untouched. |
| Delivery | Ordinary repository policy and any explicit delivery authorization name B. Approval never authorizes delivery. |

This lifecycle is available only to Claude Code, Codex, OpenCode, and Pi. Unsupported runtimes fail before repository or authority mutation.

## Atomic lifecycle

### 1. Selectorless STATUS preflights only

Selectorless STATUS evaluates only the current worktree candidate and renders one exact START invocation. It does **not** discover ambient authority, resume another worktree, recover history, or select a stale lineage. The parent runs only the returned `next_transition` and its ordered tokens.

### 2. START freezes an independent transaction

START freezes the candidate in one compact transaction, explicitly bound to its lineage, worktree, and target. It selects risk and lenses natively. Capture the returned lineage, revision, and target tokens.

An exact replay of an active START can return `replayed`. A genuinely new START is independent. Do not reuse a burned lineage.

### 3. Bound calls drive the transaction

Every later STATUS and bound capture call for this transaction carries the exact captured lineage, revision, and target tokens. The parent routes only from that transaction's returned `next_transition`:

| Transition | Parent action |
| --- | --- |
| `execute` | Run the exact operation and ordered arguments unchanged. |
| `collect` | Provide only its named input through its exact capture operation, then query bound STATUS again. |
| `stop` | Run no lifecycle operation. Do not infer a recovery from prose. |

A forecast is descriptive, not a route. Relay every forecast step and horizon losslessly, but execute only `next_transition`.

### 4. Approval burns authority

Native Go owns frozen lenses, provider context and admission, refutation, one bounded correction, repository evidence, and targeted validation. On success it reads back terminal approval, then burns the exact lineage and its artifacts before returning `approved`.

No terminal receipt, tombstone, witness, mirror, or delivery authority survives the burn. Other lineages and worktrees are unaffected. After a malformed, incomplete, or unavailable capture, retain the exact lineage, revision, and target binding, query bound STATUS once, and follow only the reoffered capture route.

## Reviewer transport

The provider contract is shared by Claude Code, OpenCode, Codex, and Pi. Go derives frozen trees, manifest, subject hash, role, binding, schema, evidence limits, and admission. Adapters transport opaque provider output and never parse bindings, manufacture a verdict, or mutate review authority.

Each provider-issued capture input is one slot. Its reviewer prompt starts with `GENTLE_AI_REVIEW_BINDING ` followed by one-line binding JSON. A result echoes the exact `subject_hash`, reports completed inspection of the full manifest, and supplies structured findings/evidence. On malformed, incomplete, or unavailable inspection, query bound STATUS again; relaunch only when it reoffers the exact same slot.

Reviewers inspect only provider-bound immutable trees. They never inspect the live worktree, index, `HEAD`, or another revision, and candidate bytes must not move through `/tmp`, a repository scratch file, or `GENTLE_AI_FROZEN_CANDIDATE_CONTEXT`.

## Corrections and consent

Native Go alone selects lenses, classifies candidate causality, performs refutation, derives repository evidence, and permits at most one bounded correction. A validator that cannot inspect its immutable trees has no verdict; report that block rather than submitting a failed validation.

Medium and high-risk START may return the typed `gentle-ai.review-integration.consent/v3` envelope. Relay the complete choice envelope losslessly, preserve machine tokens and invocations exactly, and run only the invocation selected by the human. Global RDD mode permits review; it never grants per-candidate consent. A decline is not the kill switch.

## Delivery remains human-owned

`gentle-ai review validate` and named gates (`post-apply`, `pre-commit`, `pre-push`, `pre-pr`, and `release`) are compatibility/informational commands. They never discover authority or decide delivery:

| Mode | Informational result |
| --- | --- |
| RDD enabled | `invalidated/unmanaged` |
| RDD disabled | `disabled/unmanaged` |

They never allow, approve, block, commit, push, or open a pull request. Delivery follows ordinary repository policy.

Terminal review state is informational and never authorizes a commit. Commit, push, PR, release, and archive follow ordinary repository policy and require their own explicit authorization. For a selected repository B, any authorized delivery action runs in B only.

## Compatibility

The v1 contract and historical artifacts remain published compatibility surfaces. They may be inspected with explicit, manual compatibility operations, but they are not an ordinary v2 lifecycle, cannot resurrect a burned transaction, and do not authorize delivery.

### Continue after a stop reason code

A `stop` carries one reason code and no executable transition. The table below is the complete continuation inventory for the atomic target-root lifecycle. `Terminal` means no in-lineage continuation exists; it never authorizes delivery. For every clone-scoped exit, gates remain unmanaged and ordinary repository policy decides delivery.

| Reason code | Continuation |
| --- | --- |
| `captured_artifacts_unverifiable` | Terminal — a captured reviewer artifact failed local verification. Ask a maintainer to inspect the B authority, or run `gentle-ai review mode disable --scope clone --cwd <repo>`. |
| `captured_result_selection_unavailable` | Terminal — an internal result-selection invariant failed. Ask a maintainer to inspect the lineage, or run `gentle-ai review mode disable --scope clone --cwd <repo>`. |
| `corrected_candidate_unavailable` | Change the correction candidate in B, then re-query `gentle-ai review status --cwd <repo> --contract gentle-ai.review-integration/v2 --agent {{GENTLE_AI_RUNTIME_AGENT_ID}} --next-transition` with the captured lineage and target. Do not reuse the pre-correction target. |
| `empty_base_diff_bootstrap_required` | Terminal — the committed base has no reviewable paths. Use the separately authorized empty-root bootstrap for a new target, or run `gentle-ai review mode disable --scope clone --cwd <repo>`. |
| `lens_context_budget_exceeded` | Terminal — immutable reviewer context cannot be truncated. Reduce the B candidate scope and start a new transaction, or run `gentle-ai review mode disable --scope clone --cwd <repo>`. |
| `corrupted_or_unverifiable_authority` | Terminal — the authority is unreadable or unsupported. Ask a maintainer to inspect it, or run `gentle-ai review mode disable --scope clone --cwd <repo>`. |
| `manual_intervention_required` | Terminal — the authority state is outside the negotiated lifecycle. Ask a maintainer to inspect it, or run `gentle-ai review mode disable --scope clone --cwd <repo>`. |
| `missing_authority_binding` | Terminal — a current target had no authority binding. File a bounded defect with the lineage, or run `gentle-ai review mode disable --scope clone --cwd <repo>`. |
| `native_stop_required` | Terminal — the lineage is escalated but has no native continuation. Ask a maintainer to inspect it, or run `gentle-ai review mode disable --scope clone --cwd <repo>`. |
| `recovery_scope_unchanged` | Change B so its target identity differs, then retry the exact returned `gentle-ai review recover` invocation. |
| `staged_workspace_overlay_recovery_unavailable` | Terminal — pass `--lineage <id>` to recover an existing lineage, or drop `--workspace-overlay` and start a fresh target; otherwise run `gentle-ai review mode disable --scope clone --cwd <repo>`. |
| `rdd_disabled` | Run the exact source-scoped `gentle-ai review mode enable` command rendered by STATUS, then re-run its exact repository-bound STATUS command. |

## Published v1 compatibility reference

The published v1 directory contains 22 strict JSON Schemas and 24 deterministic conformance fixtures. These are read-only compatibility inventory, not durable v2 receipt, gate-allow, or mirror state.

- `legacy_v1_read_only` failures retain `mutation_outcome` values `not_started`, `unknown`, and `committed`; Legacy-v1 never reports `publication_pending`, with retry and replay disabled where its historical operation requires a new compact lineage.
- Historical `ordinary_4r` legacy status omits `frozen`. START, BIND-SDD, invalidation, and direct append are compatibility names only and never re-enter the atomic lifecycle.
- Published vocabulary remains readable: `native_frozen_candidate_context`, `base_tree`, `candidate_tree`, `changed_path_manifest`, `opaque_repository_context`, `provider_targeted_validation_request`, `provider_artifact_admission`, `validating_result_reopen`, and `recovered_correction_evidence`.
- Artifact compatibility names remain `artifact_subjects`, `subject_hash`, and `admission_decision: completed`; low-risk compatibility names remain `native_low_risk_verification`, `selected_lenses: []`, and `receipt_scope_changed`. They describe historical data only and do not restore a receipt after an atomic approval burn.
- Operational bounds remain a 25-second aggregate budget, 120-second budget, 180-second budget, and one-second wait delay. Persistent compact `LOCK` JSON is advisory diagnostics. `context.scope_change` and `review.recover` are explicit compatibility/recovery vocabulary, not ambient lifecycle discovery.

## Checklist

- [ ] Selectorless STATUS was used only to preflight the current worktree candidate.
- [ ] START's lineage, revision, and target tokens were retained and replayed unchanged.
- [ ] Reviewers and validators used only provider-issued immutable context.
- [ ] `approved` was reported only after native burn completed.
- [ ] Commit, push, PR, release, and archive followed ordinary repository policy.
- [ ] Each delivery action has explicit authorization.
