# Review Integration Contract

← [Back to README](../README.md)

Gentle AI exposes two negotiated review contracts. `gentle-ai.review-integration/v1` preserves the published Base64 candidate-diff transport byte for byte. `gentle-ai.review-integration/v2` is the native-Git contract: it carries immutable base/candidate tree IDs and an ordered changed-path manifest, never an inline patch. Its current minor is v2.1, which selects the immutable review runtime explicitly. Both let a consumer reconstruct one target after restart, drive explicit review operations, and validate the resulting receipt without reading provider-private authority files.

## Advisory Model Review

`gentle-ai review advisory` is the provider-owned prompt/text seam for Claude Code, OpenCode, and Codex. Codex was activated once its organic positive and negative runtime proof passed (`TestRealCodexReviewerOrdinarySessionAdmitsRawOutput` and its fail-closed companions, `e2e/organicruntime`): a real `codex exec` process, launched by `CodexAdapter` in an empty scratch directory it creates and deletes itself, received only the canonical prompt and its raw output reached native admission and a terminal receipt. Any runtime name outside the advertised set (`claude-code`, `codex`, `opencode`) still gets `PromptFor`'s typed unavailable error.

CI coverage of these two organic proofs is not symmetric today: `organic-runtime-e2e` installs a pinned OpenCode binary and actually re-executes `TestRealOpenCodeReviewerOrdinarySessionInjectsFrozenContextAndAdmitsRawOutput` on every run, while the equivalent Codex job step only verifies `TestRealCodexReviewerOrdinarySessionAdmitsRawOutput` is registered (`go test -list`) -- it does not install a codex binary or an authenticated account, so that test skips in CI and its activation proof is currently maintainer-executed locally instead. Continuous CI execution of the real Codex proof is tracked in [#2662](https://github.com/Gentleman-Programming/gentle-ai/issues/2662).

Its binding form is closed and identical to `review lens-context`: `gentle-ai review advisory prompt --repository-context <handle> --lens <lens> --runtime <runtime>` and `gentle-ai review advisory validate --repository-context <handle> --lens <lens> --input <result.json>`. The repository context and lens are the only caller-supplied tokens; lineage, target, revision, order, the signed artifact subject, and every path's evidence are all derived from frozen native authority, exactly as `review lens-context` derives them. There is no caller-authored request file.

It exports one bounded canonical prompt -- rendered with the same lens mandate (title and inspection focus) `review lens-context` renders from `reviewtransaction.LensMandate` -- and validates one strict native reviewer-result shape against the signed artifact subject and its ordered frozen path manifest, including a required self-reported lens binding. The transport validation records its raw hash and preserves findings, including `BLOCKER`/`CRITICAL`, evidence class, and causal disposition. A model never directly emits PASS, a receipt, a gate result, or delivery authorization.

Native Go then performs the existing admission and RDD pipeline unchanged: candidate-causality checks, deterministic/inferential handling, refutation, bounded correction, verification, terminal receipt issuance, and delivery gates decide the consequence of an admitted finding.

`advisory` is a model-transport trust label, not a non-blocking severity or delivery disposition.

OpenCode advisory review runs from the normal session. It does not require a restart, a special terminal, or global `OPENCODE_DISABLE_PROJECT_CONFIG` / `OPENCODE_DISABLE_EXTERNAL_SKILLS` variables. Runtime adapters only transport the prompt and return text; Go owns evidence bounds, schema, and validation.

## Negotiate the provider first

Resolve the exact `gentle-ai` executable that will perform review operations, then query it outside a repository:

```bash
gentle-ai review capabilities \
  --contract gentle-ai.review-integration/v1
```

The response identifies the protocol major, package and build identity, executable SHA-256, operations, five gates, projections, schemas, mandatory and optional features, and compatibility window. The executable digest is self-reported evidence; compare it with the published release manifest before trusting the binary.

New integrations SHOULD negotiate `gentle-ai.review-integration/v2`. The current response is capabilities v2.1 and requires `--agent claude-code` for negotiated STATUS and START. Capabilities v2.0, status v3, and consent v2 remain historical byte-pinned artifacts; they are not rewritten or emitted as the current response. Existing v1 consumers remain valid and MUST continue validating the published v1 schemas and fixture bytes; they do not gain tree-only fields additively.

Protocol v1.5 advertises `gentle-ai.review-integration.capabilities/v1.5` and adds `outcome_bound_verification_evidence` without changing the preserved v1.4 identity or its `one_shot_final_verification_retry` feature. `review capture-evidence` requires one closed `--outcome` (`passed`, `verification_failed`, or `procedural_tooling_failed`) and persists `gentle-ai.review-verification-evidence/v2` beside immutable candidate-addressed raw bytes. The record binds lineage, authority revision, target tree, canonical paths and ledger IDs, raw SHA-256 and size, outcome, and its own canonical digest. FINALIZE derives captured approval or escalation from that record; a caller-supplied `--failed` may agree with it but cannot override it.

Protocol v1.4 advertises `gentle-ai.review-integration.capabilities/v1.4` and adds `one_shot_final_verification_retry`, operation `review.retry_final_verification`, and incident schema `gentle-ai.review-final-verification-incident/v1`. This is a dedicated provider-owned retry for one exact completed failed final-verification tooling incident; it does not relax generic recovery.

Protocol v1.3 introduced `provider_artifact_admission`, `validating_result_reopen`, `recovered_correction_evidence`, and `classified_authority_repair`. START v2 supplies one provider-owned `ArtifactSubject` per selected lens. Result artifact v2 and status v2 expose the admitted subject hash and completed admission decision. Status v2 also requires a bounded `gentle-ai.review-authority-repair-assessment/v1`; `review.repair` publishes the matching strict preflight and execution contract. The durable admitted-result envelope preserves raw and canonical payload identities, the result identity, and repository-verified candidate-causal finding IDs. Exact accounting-only recovery may reuse that evidence only when the corrected predecessor bytes are the successor's exact initial target. Protocol v1.0 through v1.4 capability schemas and fixtures remain packaged unchanged. Consumers must reject an unknown schema/minor identity they do not support; v1.5 consumers validate the v1.5 schema before relying on the current features.

The v1.2 feature set includes `native_frozen_candidate_context`, `opaque_repository_context`, and `provider_targeted_validation_request`. Its published reviewer transport contains the canonical candidate diff and ordered changed-path manifest. Opaque repository context lets an external actor return results without receiving or rediscovering a repository path. Provider-targeted validation supplies the exact corrected candidate and frozen finding IDs to validate. Contract v2 replaces only that reviewer transport with immutable tree IDs; it does not rewrite v1.

The v1.1 artifact remains the compatibility record for `base_ref_workspace_overlay`, `bounded_process_waits`, `exact_gate_receipt_discovery`, `native_low_risk_verification`, `native_next_transition`, `risk_reasons`, and `scope_change_diagnostics`. The overlay feature requires immutable snapshots and restart-safe projection.

Consumers MUST reject an incompatible protocol major, an unsupported mandatory feature, an unknown mandatory enum, or a schema identity mismatch. Unknown optional fields may be ignored only under the advertised additive-minor policy. Existing unnegotiated CLI responses remain separate compatibility surfaces and do not gain negotiated fields silently.

Pass the same contract explicitly to negotiated repository operations:

```bash
gentle-ai review start --contract gentle-ai.review-integration/v2 --agent claude-code --cwd .
gentle-ai review status --contract gentle-ai.review-integration/v2 --agent claude-code --cwd .
gentle-ai review finalize --contract gentle-ai.review-integration/v2 --cwd . --lineage <lineage> ...
gentle-ai review validate --contract gentle-ai.review-integration/v2 --cwd . --gate pre-commit
gentle-ai review bind-sdd --contract gentle-ai.review-integration/v2 --cwd . --change <change> --lineage <lineage> --expected-binding-revision=<revision>
```

### Zero-help lifecycle bootstrap

When capabilities advertise `native_next_transition`, the parent orchestrator starts lifecycle routing exactly once with:

```bash
gentle-ai review status --cwd <repo> --contract gentle-ai.review-integration/v2 --agent claude-code --next-transition
```

Append a target selector only when its type is already known: `--projection staged`, `--base-ref <ref>`, `--workspace-overlay --base-ref <ref>`, or `--workspace-overlay --base-tree <tree>`. If the feature is unavailable, query exactly once `gentle-ai review capabilities --contract gentle-ai.review-integration/v2` and stop with `unsupported-capability`; do not explore commands or consult help. After bootstrap, only the parent executes the exact native `next_transition`. Reviewers, validators, executors, and refuters receive role inputs and return artifacts; they never invoke review lifecycle commands.

### Per-candidate consent relay

A session that can relay a blocking question to a human declares it on negotiated START with `--consent relay`. When the frozen candidate's tier would ask the per-candidate consent question (medium or high risk), START responds with the consent envelope matching the negotiated contract: `consent/v1` for integration v1 or current `consent/v3` for integration v2.1. The v2.1 envelope requires `agent: claude-code` and carries why input is required, the complete choice set, and one runnable follow-up invocation per choice scoped to the exact `--target` identity. Nothing is persisted while the question is outstanding, and no console notice is printed. Pass `--locale en` or `--locale es` to localize every human envelope field; omitting it preserves the established English projection. Answer tokens, commands, target IDs, projections, and invocations remain machine-stable.

The orchestrator relays the complete envelope losslessly and answers with exactly one named invocation. `--consent granted` revalidates the exact target before creating compact review authority; it is replay-safe and a rerun resumes that authority. `--consent declined` reports the typed declined START outcome (`consent: declined_this_candidate`) without creating a review lineage or receipt, then atomically records a canonical native candidate-decline authorization in the Git common directory. That authorization permits only exact `pre-commit`, `pre-push`, and `pre-pr` delivery to proceed under ordinary repository policy, reported as `candidate_declined/unmanaged`; it is never an approval and never permits release. Content, path, mode, base, untracked, publication-range, or advertised-head drift rejects it. Replay recovers lost output only for the same canonical decline, corrupted or ambiguous decline records fail closed, and a later candidate asks again. A decline is deliberately not the kill switch; the permanent disable remains `gentle-ai review mode disable`, documented in the envelope's `off_path` and never offered as a choice. Without the declaration, START behavior is unchanged: low risk asks nothing, a resolved question asks nothing, and a headless undeclared session keeps the skip-and-notice fallback.

## Keep provider and consumer ownership separate

| Gentle AI provider owns | Consumer owns |
| --- | --- |
| Git-derived immutable snapshot identity and projection | User interaction and explicit maintainer confirmation |
| Deterministic risk reasons, tier, lenses, and correction budget | Reviewer, validator, and correction actor execution |
| Compact-v2 authority transitions, opaque repository context, lock, and expected-revision CAS | Process invocation, cancellation, and transport diagnostics |
| Artifact subjects, admission decisions, and repository-derived causal checks | Echoing the exact subject and reporting structured candidate inspection |
| Corrected-candidate identity and targeted-validation request | Running the requested validator and returning its typed result |
| Receipt derivation and exact receipt publication replay | Rendering native outcomes without weakening them |
| Target applicability, replayability, and gate evaluation | Rechecking command intent immediately before execution |
| Approved-receipt binding for SDD | Derived worktree and temporary-view lifecycle |

Consumers MUST NOT reconstruct receipts, derive canonical hashes, inspect the Git common-dir authority store, select an ambiguous lineage automatically, or infer that a transport interruption did not mutate state. Gentle AI does not choose models, run arbitrary user commands, or replace a consumer's command-safety policy.

## Drive the bounded operation set

| Operation | Mutation boundary | Contract behavior |
| --- | --- | --- |
| `review.capabilities` | None | Reports the deterministic repository-independent provider surface. |
| `review.start` | Compact authority | Freezes one target, tier, lens set, and correction budget; negotiated selected-lens responses also return context derived from that exact authority. It never starts because a gate was invoked. |
| `review.status` | Provider-private derived context only | Reconstructs target-scoped applicability, projection, lifecycle, and one next action without mutating authority. |
| `review.repair` | Audited whole-lineage quarantine | Preflights the complete authority inventory and executes only the unique provider-classified historical alias repair with exact revision CAS and maintainer authorization. |
| `review.retry_final_verification` | One provider-derived compact successor | Re-enters only `validating` after an exact completed failed final-verification procedural/tooling incident. It copies frozen authority and accounting, clears active evidence, and creates no review or correction budget. |
| `review.finalize` | Compact authority and derived receipt | Accepts selected lens results and bounded correction evidence, performs deterministic native verification for an eligible low-risk zero-lens target, or performs an exact receipt-publication replay. |
| `review.validate` | None | Revalidates one existing content-bound receipt at a named lifecycle gate. |
| `review.bind_sdd` | SDD binding artifact | Binds only an approved receipt to an SDD change. |

`review.start` is the only ordinary entry point that creates a review budget. Finalize continues that frozen lifecycle. The dedicated final-verification retry creates a successor lineage but copies every frozen budget and accounting field without adding a reviewer, correction, SDD, or other budget. Status, validation, and gates are read-only.

`gentle-ai review capture-result` is an additive headless command, not a negotiated repository operation. It accepts no `--contract`; the provider-issued subject hash selects the transport version. Capture emits a manifest with capability `review.native_result_artifact` and schema `gentle-ai.review-result-artifact/v2`; the manifest binds `subject_hash` and `admission_decision: completed`, and exactly one provider-owned `path` or opaque `reference` locates the durable admitted-result envelope (`review-admitted-result/v1` for a v1 subject, `review-admitted-result/v2` for a v2 subject). A negotiated capture transition carries `--repository-context <opaque-handle>` plus `--expected-revision <revision>`, so consumers can invoke capture from an unrelated working directory without learning the repository path. Explicit `--cwd` remains the capture compatibility path-manifest mode and cannot be combined with a repository-context handle.

Add `--preflight --input <file>` to validate the same native admission without persistence. A successful input preflight emits `gentle-ai.review-capture-result-dry-run/v1` with `validation: accepted` and the existing lineage, lens, order, and subject binding. It does not emit an artifact path, reference, raw-input hash, or canonical-result hash. `--preflight` without `--input` retains the binding-only response.

### Choose the target explicitly

| Invocation | Frozen boundary |
| --- | --- |
| `review start` | `HEAD` to the synthetic staged/unstaged workspace tree. |
| `review start --base-ref <ref> --committed-only` | `<ref>` to `HEAD`; workspace changes are excluded. |
| `review start --base-ref <ref> --workspace-overlay` | `<ref>` to the synthetic workspace tree, including branch commits and staged and unstaged bytes. |

Review scope is declared, never inferred from the worktree. A file Git does not track enters the candidate only once it is in the index, which `git add` is the only way to put it there; an untracked, unignored file that merely exists in the workspace is out of scope and its bytes never reach a reviewer (issue #2394). `intended_untracked` therefore stays empty for every target START freezes, and remains only as an explicit provider input.

Overlay mode requires workspace projection and cannot be combined with `--committed-only`. START returns `target_mode` and `target_identity` only for this mode. Under contract v2, selected-lens START responses also return `base_tree` and `candidate_tree` as reviewer context for every target kind. Restarted consumers select an overlay target with `review status --base-tree <START base_tree> --workspace-overlay`; `--base-ref` remains available for a fresh symbolic selection, but cannot be combined with `--base-tree`. Snapshot construction uses a temporary index and does not mutate the real index or worktree.

### Use frozen reviewer context

Under integration v2, negotiated START with selected lenses returns `base_tree`, `candidate_tree`, `changed_path_manifest`, and one `artifact_subjects` entry per lens. The provider derives them from the selected authority's persisted initial snapshot, not the current index, worktree, or a correction snapshot. Each subject self-hashes the exact lineage, authority revision, target, both tree IDs, manifest digest, lens, selected order, and optional correction target.

After a restart, v2 `review status --next-transition` returns that same context inside every missing `review.capture-result` input. No v2 START, status, task, environment, or plugin payload contains the full patch or a Base64 copy. Contract v1 deliberately retains its published Base64 candidate-diff field.

Reviewers inspect the frozen candidate through read-only native Git commands against the exact `base_tree` and `candidate_tree` from their collection input. They treat the ordered `changed_path_manifest` as the complete frozen scope, begin with compact discovery, and then open only the paths their lens needs:

```bash
env -i PATH="$PATH" LC_ALL=C GIT_CONFIG_NOSYSTEM=1 GIT_CONFIG_GLOBAL=/dev/null GIT_ATTR_NOSYSTEM=1 \
  git --no-replace-objects --no-pager -c color.ui=false -c core.attributesFile=/dev/null -c diff.external= \
  diff --name-status --text --no-ext-diff --no-textconv --no-renames --ignore-submodules=none \
  <base_tree> <candidate_tree> --

env -i PATH="$PATH" LC_ALL=C GIT_CONFIG_NOSYSTEM=1 GIT_CONFIG_GLOBAL=/dev/null GIT_ATTR_NOSYSTEM=1 \
  git --no-replace-objects --no-pager -c color.ui=false -c core.attributesFile=/dev/null -c diff.external= \
  diff --patch --text --full-index --no-color --no-renames --no-ext-diff --no-textconv \
  --diff-algorithm=myers --no-indent-heuristic --unified=3 --ignore-submodules=none \
  <base_tree> <candidate_tree> -- ':(literal)<path>'
```

Reviewers may also use the corresponding allowlisted `--numstat`, selective `--stat`, and exact `cat-file -p '<tree>:<path>'` forms. Several related literal pathspecs may share one selective command. Reviewers never pass `--binary` and never render the entire candidate patch automatically. They run only these clean-environment commands in the session working directory. The frozen trees resolve through the repository object store shared by every worktree of the same repository. The guarantee is immutable candidate content addressed by the frozen trees, not byte-identical rendered patch transport across Git versions. Reviewers never inspect the live worktree, index, `HEAD`, or an unbound revision; a runtime without the enforced Git command boundary reports incomplete inspection instead of substituting live files.

Manifest entries stay in persisted path order and expose:

| Field | Meaning |
| --- | --- |
| `path` | Repository-relative logical path; absolute repository paths are never emitted. |
| `status` | Stable `A`, `D`, `M`, or `T` tree-diff status. |
| `old_mode` / `new_mode` | Six-digit Git modes, including zero modes for additions/deletions and symlink or gitlink modes where supported. |
| `deleted` / `type_changed` / `mode_only` | Explicit state that consumers do not need to infer from patch prose. |
| `intended_untracked` | Whether the frozen snapshot bound the path as intended-untracked provenance. START never sets it: since #2394 declared scope arrives through the index instead. |

Under v2, selected lenses require both valid tree IDs and the manifest. An empty candidate has equal valid tree IDs with `changed_path_manifest: []`; missing context remains invalid. V1 continues to require its candidate diff and manifest. Unnegotiated START retains its legacy response shape and emits no candidate contents.

Reviewer results must echo the exact `subject_hash` and report structured `inspection: {status: "completed", paths: [...]}` for the complete frozen manifest, including root-level paths. Finding IDs use the ASCII form `R[1-4]-[A-Za-z0-9][A-Za-z0-9._-]*`. Proof and evidence recognize `path:positive-line` only for canonical paths present in the immutable base/candidate tree union: bare root references contain a dot, while quoted references support extensionless, Unicode, and space-containing paths. Digests, timestamps, status labels, URLs, and arbitrary colon-delimited prose are not path references. The provider accepts transport prose around exactly one complete JSON object, but rejects zero, multiple, or unterminated objects. Missing inspection, access-denied or unavailable-inspection evidence, paths or locations outside the frozen candidate, repeated finding IDs, wrong lens prefixes, binding mismatch, and unsupported causal metadata are classified and rejected before publication. Severe findings must retain a supported `evidence_class` and `causal_disposition`; `introduced`, `behavior-activated`, and `worsened` claims are admitted only when repository-derived changed-line evidence supports the claimed location. Reviewer results may omit the top-level `lens`; when present, it must match the selected-lens position returned by START.

Claude Code, OpenCode, and Codex provide the enforced fresh reviewer-execution boundary through the one shared advisory contract: Claude reviewers are tool-free fresh agents with prompt-carried native evidence, OpenCode provider injection replaces the reviewer task with bound native evidence from an ordinary already-running session (no restart, child process, special user-visible session, or `OPENCODE_DISABLE_PROJECT_CONFIG` / `OPENCODE_DISABLE_EXTERNAL_SKILLS` variable), and Codex's shared advisory `CodexAdapter` launches a brand-new `codex exec` process in an empty scratch directory it creates and deletes itself, receiving only the canonical `review advisory prompt --runtime codex` output. Kilo receives `immutable_review_transport_unsupported` before repository, target, authority, collection, or process work; that typed result names the clone-scoped review-mode exit and the supported runtimes. Normal SDD and the manual compatibility path remain available; no provider or model is substituted.

Durable controllers capture each result with exact lineage, target, lens, selected order, authority revision, and provider-issued repository context. Current captures emit pathless manifests with opaque references; the provider can discover every canonical result with `--captured-results`, or controllers can write each emitted manifest to its own file and pass those files to FINALIZE in selected-lens order with repeatable `--result-artifact-file <path>` flags. A `--result-artifact-file -` occurrence reads exactly one manifest from stdin; because FINALIZE has one shared stdin, `-` may appear only once across reviewer results, artifact manifests, validation, refuter outcomes, and evidence.

Windows PowerShell 5.1 should use file transport because native argument reconstruction does not preserve dynamic inline JSON reliably. Write BOM-less UTF-8 so the strict JSON decoder receives the manifest bytes directly:

```powershell
$manifest = & gentle-ai review capture-result --repository-context $repositoryContext --expected-revision $revision --lineage $lineage --target $target --lens $lens --order $order --input $resultPath
$manifestPath = Join-Path $env:TEMP "gentle-ai-review-manifest.json"
$manifestText = [string]::Join([Environment]::NewLine, [string[]]$manifest)
[System.IO.File]::WriteAllText($manifestPath, $manifestText, (New-Object System.Text.UTF8Encoding($false)))
& gentle-ai review finalize --cwd $repo --lineage $lineage --result-artifact-file $manifestPath
```

Repeat `--result-artifact-file` once per selected lens. Each file contains one canonical manifest. For current captures, Gentle AI preserves the opaque reference and resolves it from private provider storage; for compatibility path manifests, it preserves path bytes. Both forms retain strict schema, lineage, target, lens, selected-order, subject, admission, ownership, lowercase SHA-256, file-identity, payload, and hash checks. The provider reopens the durable admitted envelope and re-runs admission before FINALIZE; a manifest is never authority by itself. File transport does not normalize manifest JSON or paths. Repository-context and artifact references are opaque capabilities, not serialized repository paths: the provider revalidates repository identity and Git-directory containment, and rejects them when lineage, target, revision, selected lens/order, or live authority no longer match.

The POSIX inline form remains fully compatible:

```bash
gentle-ai review finalize --cwd "$repo" --lineage "$lineage" \
  --result-artifact "$manifest_json"
```

Inline `--result-artifact`, file/stdin `--result-artifact-file`, legacy `--result`, and `--captured-results` are mutually exclusive reviewer-result sources. Legacy four-field captures use explicit `--cwd`; legacy `--result` files and path manifests remain compatible but are not a durable cross-agent handoff.

Proof and evidence strings accept ordinary technical notation, including `HEAD^{tree}`, `{}`, `<A>`, and `=>`. Blank values and exact non-evidence sentinels such as `n/a`, `none`, `todo`, `tbd`, `pass`, `passed`, `success`, and `placeholder` remain invalid.

Every public zero-lens result encodes `selected_lenses: []`, never `null`. Historical compact-v2 state and receipts that contain `null` remain readable: the provider verifies their original checksum before normalizing the value in memory and does not rewrite authority. Ordinary non-operational Markdown and static documentation assets may be low risk. `AGENTS.md`, `SKILL.md`, prompt/agent/workflow/runtime/OpenSpec paths, MDX, source or configuration files, binaries, symlinks, gitlinks, executable files, and mode-only changes are not eligible for native low-risk verification.

An exact no-input FINALIZE is eligible only when the frozen authority is low risk, selected no lenses, has no findings or correction state, and still resolves to the same Git snapshot and repository-derived risk assessment. The provider then hashes domain-separated native structural evidence into the normal compact state and receipt. External evidence remains accepted for backward compatibility. Medium/high-risk, corrected, SDD, and release flows still require their existing external evidence.

### Validate exactly five gates

| Gate | Required boundary |
| --- | --- |
| `post-apply` | Revalidate the implemented candidate against the terminal receipt. |
| `pre-commit` | Revalidate the intended staged candidate before commit. |
| `pre-push` | Revalidate the committed candidate before publication. |
| `pre-pr` | Revalidate the candidate, selected remote base, and compatible-base evidence before opening or updating a PR. |
| `release` | Revalidate the immutable release tree, configuration, generated manifest, provenance, publication boundary, and evidence freshness. |

There is no `archive` gate. An advisory preflight is not delivery authorization; the native live gate result is authoritative.

### Historical v2.2.0 compatibility boundary: unmanaged delivery and re-enabling

This is a historical compatibility boundary introduced in `v2.2.0`, not the current release version. Work delivered while the kill switch is off and no exact governing receipt applies is recorded as unmanaged, and it stays recorded as unmanaged: native lifecycle gates report `disabled/unmanaged` at exit 0, the change closes under ordinary repository policy, and nothing — not a stale receipt, not a later empty-candidate approval — may ever make that window read as reviewed. SDD status differs: it structurally omits `reviewGate` while the kill switch is off, and also when it is on but no review was discovered for the candidate; archive proceeds under ordinary policy in both cases. A present `reviewGate.result: allow` is required only for discovered review activity. Re-enabling re-validates the current state through one full fresh review, exactly as if receipt-driven development had never run: every downstream stop over unreviewed content names `gentle-ai review start` (with `--base-ref <commit>` when the delivered work is already committed), and completing that review is what unblocks the stop. The fresh review subsumes the unmanaged history; durable retroactive reconciliation — per-delivery dispositions, retroactive receipts, or any ledger that blesses past unmanaged deliveries — is deliberately not part of that historical release boundary.

### Follow applicability and action, not inventory

| Applicability | Meaning |
| --- | --- |
| `current_target` | Exactly one validated authority applies to the requested Git target. |
| `unrelated` | No authority applies, even when unrelated historical authority exists. |
| `ambiguous` | More than one authority applies or a required lineage selector is missing. |
| `corrupted` | Authority required for classification cannot be validated safely. |

STATUS derives one immutable live Git snapshot, then discovers the authority inventory and compact recovery graph once. Each selected compact candidate uses a bounded double-collect: it reads state, receipt, and finalize journal; rechecks state revision and snapshot identities; then rereads receipt and journal in the same order. Projection accepts only matching artifact existence, raw identity, and canonical content across both observations. Concurrent publication is retried at most three times; continuing churn is an operational concurrency error, not semantic corruption. Legacy approved receipts are inspected only after pure target matching selects that lineage, so unrelated receiptless v1 history remains readable. Git subprocess count therefore does not grow with terminal history; authority inventory and comparison remain linear CPU/filesystem work.

Persisted `intended_untracked` membership is immutable historical proof, not a request to rebuild an old workspace against current tracking state. When the live base tree equals a receipt-bound final candidate tree, the reviewed bytes and modes have been committed exactly. A clean target or disjoint next slice is `unrelated`; a contraction or overlap with frozen genesis scope remains scope-changed evidence. A path that became tracked or later disappeared does not make healthy authority `corrupted`, and STATUS never repairs, archives, invalidates, or rewrites that authority.

The provider returns one historical action from `start`, `finalize`, `validate`, `recover`, `retry_final_verification`, `maintainer_action`, `select_lineage`, `repair_authority`, `reconcile_finalize`, or `stop`. A consumer that negotiates `native_next_transition` requests `--next-transition` with STATUS or FINALIZE and MUST route only from its single `next_transition`. `execute` contains one native operation, every exact argument, immutable lineage/revision/target binding, and path-free native artifact identities; execute those values unchanged. `collect` names the exact missing input, schema, capture operation, and content-bound arguments. `stop` has exactly one reason code and no executable or collect routing data; `corrected_candidate_unavailable` additionally retains the read-only provider correction request needed to edit safely. Existing v1 consumers that do not request the flag retain their historical strict payload; `eligibility` remains a compatibility detail and is never a routing authority. Missing worktrees, refs, targets, lineage, revisions, ambiguous/corrupt authority, and unverifiable materiality never yield an executable partial operation. Applicable non-terminal legacy-v1 authority always stops. A consumer MUST NOT infer an authorization, command binding, template, recovery disposition, or target selector from prose, state, eligibility, or a statusline.

Native v2 STATUS with `--next-transition` also returns a descriptive one-step `forecast`. Its sole step mirrors the current `next_transition` kind and reason code, with a human description; it is never a route or a promise of a tail. `partial` means the head is non-stop and the consumer MUST re-query after executing or collecting it. `terminal` means the current head is `stop`. Consumers route and execute only from `next_transition`; a localized human relay preserves the forecast horizon and every step field in order.

For a fresh target, STATUS emits a complete START call with `contract`, frozen `target`, and explicit `projection`, plus an explicit `lineage` when one was requested. Base-diff START uses the resolved tree as `base-ref` with `committed-only=true`; workspace overlay uses the resolved tree as `base-ref` with `workspace-overlay=true`. START rejects partial or repeated negotiated bindings and revalidates the frozen snapshot immediately before new authority publication. The START surface does not accept or emit `base-tree`.

For compact-v2 current STATUS, `target_identity` identifies the live selected Git target. When the frozen authority `CurrentSnapshot` differs, STATUS also emits `authority_target_identity`; absence means the authority target equals the live target. A corrected candidate awaiting a dedicated final-verification retry keeps the corrected live identity in `target_identity` and the frozen validating identity in `authority_target_identity`. Retry authorization, retry execution, and successor final-evidence capture bind the authority identity; consumers MUST NOT substitute the live identity when the two differ.

An explicit compact-v2 reviewing lineage with pending reviewer input resumes its frozen collection after live drift; it never replays live target or untracked selectors. Selectorless STATUS remains live and collects new intended-untracked selection.

For `repair_authority`, the provider scans every compact-v2 and legacy-v1 lineage within fixed lineage, event, operation, event-size, and total-byte limits. Exactly one legacy lineage whose only anomaly is an approved historical `review/complete-fix` or `review/validate-fix` alias yields `repair.status: eligible`. Unknown or mixed corruption, multiple candidates, same-lineage v1/v2 collision, invalidated authority, active maintenance ownership, concurrent change, or a limit hit yields a non-executable stop with no candidate. This filesystem classification adds no Git subprocess after STATUS has resolved the repository root.

Run `gentle-ai review repair --preflight --cwd <repo>` before collecting maintainer intent. The path-free response supplies only provider-owned class, lineage, expected revision, cause, disposition, opaque repository binding, and authorization schema. The maintainer then provides `actor`, `reason`, and the exact nine-line LF-only authorization. An authorized STATUS transition marks `maintainer-authorization` as `provided` instead of echoing the completed authorization; the controller must reuse the exact secret it already holds. No response emits a repository path, quarantine record, or ready-made authorization string. The direct `repair-legacy-alias` verb remains compatibility-only.

STATUS keeps repair classification scoped to the selected native target. It publishes an eligible repair only when that target is `corrupted` with action `repair_authority`; a healthy explicit lineage retains the bounded unsupported sentinel and never absorbs an unrelated global repair candidate.

Classified execution persists a route-specific assessment digest, authorization-free request digest, and opaque record identity around the prepare/rename/commit boundary. A timeout joins the executing repair worker before reporting its mutation truth. Durable prepared or renamed progress retains `mutation_outcome: unknown` but permits only the bound `exact_replay_safe` repair request; committed progress reports `mutation_outcome: committed`. Exact retry accepts only one strict classified record, revalidates the complete live inventory and physical residue under exclusive maintenance ownership, and never adopts a compatibility-command record.

Before any correction edit, `correction_plan_required` carries a strict `gentle-ai.review-correction-plan-request/v1` object. Its provider-derived hash binds the current lineage, authority revision, frozen target, correction budget, canonical `fix_finding_ids`, and each accepted finding's location, claim, proof references, classification evidence, evidence class, and causal disposition. The same request is re-derived at the post-forecast revision and retained on `corrected_candidate_unavailable`; reading it does not consume an attempt or correction budget. Consumers plan and edit only from this accepted set, never from raw reviewer output.

### Already-edited pre-plan correction

At `correction_plan_required`, do not submit a retrospective forecast. Read the lineage row from `gentle-ai review status --cwd <repo>`, then run `review abandon` with `operator_disposition`, a maintainer actor, and this exact LF-only V2 binding (no trailing newline):

```text
gentle-ai.review-abandon-authorization/v2
lineage=<entries[].lineage_id>
revision=<entries[].revision>
snapshot_identity=<entries[].snapshot_identity>
reason=operator_disposition
captured_lens_results=<entries[].discarded_work.captured_lens_results comma-joined in the listed order>
findings_present=<entries[].discarded_work.findings_present>
evidence_records_present=<entries[].discarded_work.evidence_records_present>
actor=<actor>
```

Copy every array item verbatim in its listed order, then join them with a single `,` and no added whitespace. The native inventory uses lexicographic captured reviewer-result artifact filename order; do not re-sort or derive a different lens order.

Run `gentle-ai review abandon --cwd <repo> --lineage <lineage> --expected-revision <revision> --reason operator_disposition --actor <actor> --maintainer-authorization <binding>`.

The audit record quarantines authority and provenance; it creates no receipt and preserves candidate bytes. With RDD enabled, delivery stays denied until a fresh review. This applies only before a forecast or validator result, not post-forecast failed-validator or broader-scope cases.

After a correction forecast and an actual candidate change, STATUS first collects candidate-bound repository/full-suite verification evidence. A `verification_failed` record leaves the correction transaction open: no attempt, changed-line charge, or budget is consumed, and a changed candidate receives a distinct immutable evidence directory without replacing the failed bytes. A `procedural_tooling_failed` record executes a terminal escalation before any retry eligibility is considered. Only `passed` repository evidence unlocks `targeted_validation` with a strict `gentle-ai.review-targeted-validation-request/v1` object.

Execute the targeted request unchanged. Its provider-derived hash binds the lineage, expected authority revision, original target, exact frozen finding IDs, projection, corrected candidate tree and identity, and the exact canonical correction-path subset plus its digest. FINALIZE accepts the correction through one atomic state transition only when the targeted validation and passed repository record bind the same authority revision, candidate identity, paths, and ledger IDs. If the candidate did not materially change, no targeted-validation request is issued and routing stops with `corrected_candidate_unavailable`; consumers must not invent a validator request or another correction forecast. An ordinary lineage admits exactly one changed-target correction attempt, even when its measured delta is zero. It never admits a zero-edit correction or second fix transition.

For `targeted_validation_required`, copy the input's `repository-context`, `lineage`, `expected-revision`, `target`, `purpose=targeted-validation`, and `request-hash` exactly into `review inspect-candidate`. Then select an advertised operation: global `name-status` and `numstat` reject `--path-index`; path operations `stat` and `patch` require only `--path-index`; `object` requires `--path-index` and `--side base|candidate`. That command reads the immutable corrected tree named by the passed evidence, not the live worktree. Do not pass `--lens` or `--order`: those bind only the original reviewing-lens inspector and are rejected for targeted validation.
When the action is `recover`, negotiated status also returns the exact generic recovery disposition: `scope_changed`, `escalated`, or `invalidated`. A materially changed escalated candidate exposes only generic `review.recover`. An unchanged escalated candidate normally exposes only `stop`; it exposes `retry_final_verification` with disposition `final_verification_retry` only when native state, receipt, journal, failed evidence, ancestry, leaf, and live-current-snapshot proof all establish the dedicated boundary below. The disposition identifies the accepted provider class but never authorizes either operation. A consumer MUST NOT substitute a different disposition or route the dedicated class through generic `review recover`.

One recovery-only target expands an approved base-diff receipt into the exact staged index: request STATUS with the predecessor lineage, its original `--base-ref`, `--projection staged`, and `--workspace-overlay`. Native routing emits those same three selectors for `review recover` only when HEAD still equals the reviewed candidate, the index retains every reviewed path and adds at least one path, and the canonical predecessor receipt is present. The authorization binds the distinct successor lineage and the staged overlay identity, which already commits the base tree, index tree, projection, paths, and their digests. Unstaged and undeclared untracked bytes are excluded. The successor starts a fresh review with newly derived risk, lenses, changed-line count, and budget; it inherits no approval or evidence. Direct staged-overlay START, unchanged or disjoint scope, a removed reviewed path, selector drift, stale authority, or index drift stops without mutation.

### Continue after a stop reason code

`stop` carries exactly one reason code and no executable or collect route (see above), so a consumer that does not already know a code's continuation cannot safely proceed from prose alone. The read-only `correction_request` on `corrected_candidate_unavailable` defines what may be edited but does not authorize or name a command. The table below names every reason code `newReviewNextTransition` and its helpers in `internal/cli/review_next_transition.go` can emit, the exact continuation for consumers that hold `--cwd` access, and `terminal` where no flag-driven continuation exists. `terminal` never means "contact support with no further detail"; each terminal row states the concrete precondition that would unblock it.

| Reason code | Continuation |
| --- | --- |
| `captured_artifacts_unverifiable` | Terminal — a previously captured reviewer artifact failed local verification (tampering or hash mismatch). Requires a maintainer to inspect the review authority store before any further `review.capture-result` is trusted. |
| `captured_result_selection_unavailable` | Terminal — internal invariant violation: every selected lens already reports a captured artifact, yet the caller was routed here because the count was still short. File a defect; there is no caller-side retry. |
| `captured_verification_evidence_invalid` | Terminal — the captured verification record, immutable raw payload, or their content binding failed integrity validation. Requires a maintainer to inspect the authority artifacts before that evidence is trusted. |
| `corrected_candidate_unavailable` | Two distinct situations share this code; pick the one that is true. When the review found real defects: change the candidate content so it differs from the frozen original, then re-run `gentle-ai review status --cwd <repo> --contract gentle-ai.review-integration/v2 --agent claude-code --next-transition` (or `gentle-ai review finalize --lineage <id>`) to receive the `targeted_validation` collection — see "After a correction forecast and an actual candidate change" above. When the reviewers were given the wrong input and their findings describe content that was never the candidate: a maintainer quarantines those admitted results and reopens their lenses over the same frozen candidate with `gentle-ai review reopen-results --prepare --cwd <repo> --lineage <id> --expected-revision <revision> --target <target> --reason <reason> --actor <actor> --quarantine-lens <lens>` (repeat per affected lens), then applies the emitted authorization; the overridden result bytes are preserved in quarantine and named in the audit record. |
| `correction_repository_verification_failed` | Change the correction candidate within the same open frozen budget, then re-run `gentle-ai review status --cwd <repo> --contract gentle-ai.review-integration/v2 --agent claude-code --next-transition`. The failed candidate's evidence remains immutable under its own identity; no correction attempt or changed-line accounting was consumed. |
| `corrupted_or_unverifiable_authority` | Terminal — `gentle-ai review repair --preflight --cwd <repo>` classified this authority as `unsupported`, `ambiguous`, `conflicting`, or `truncated` rather than `eligible`. Requires a maintainer to inspect the review authority store directly; automated repair cannot proceed. |
| `final_verification_retry_unavailable` | Terminal — internal invariant violation: routed to final-verification-retry collection without a retry-eligible disposition. File a defect; there is no caller-side retry. |
| `manual_intervention_required` | Terminal — the authority state is not one of this negotiated protocol's known states. Requires maintainer review of the lineage. |
| `missing_authority_binding` | Terminal — internal invariant violation: applicability was `current_target` but no authority binding resolved. File a defect; there is no caller-side retry. |
| `native_stop_required` | Terminal — the authority state (for example an escalated lineage not yet eligible for recovery) accepts no automated action from this negotiation. Requires maintainer review of the lineage before any further command. |
| `original_finalize_request_required` | Re-run `gentle-ai review finalize --lineage <id>` with the exact original content-bound payload (results/evidence). A different payload is a typed reconciliation failure, not a retry — see "Re-run a non-terminal FINALIZE" above. |
| `recovery_scope_unchanged` | Change the candidate so its target identity differs from the current authority's, then retry the same selector-scoped `review.recover` once the identities differ. |
| `rdd_disabled` | Run the exact source-scoped `gentle-ai review mode enable` command rendered with this STATUS result, then re-run its exact repository-bound STATUS command. |
| `staged_delivery_candidate_required` | Stage every reviewed path exactly as it was reviewed, then re-run `gentle-ai review status --cwd <repo> --contract gentle-ai.review-integration/v2 --agent claude-code --lineage <id> --projection staged --gate pre-commit --next-transition`. STATUS returns `review.validate` only when that staged candidate exactly matches the approved receipt. |
| `staged_workspace_overlay_recovery_unavailable` | Terminal for a fresh target — staged projection combined with `--workspace-overlay` is recovery-only. Pass `--lineage <id>` to recover an existing lineage, or drop `--workspace-overlay` and run `gentle-ai review start --projection staged` to start fresh. |
| `unchanged_or_unverified_authority` | Terminal — the single correction attempt for this lineage is already consumed without a verified candidate change. `gentle-ai review start` on this exact unchanged candidate only resumes this same lineage, not a fresh one. Change the candidate content first, then run `gentle-ai review start` to begin a genuinely new lineage. |

### Retry one failed final verification

`gentle-ai review retry-final-verification` is provider-only and one-shot across the entire ancestry. Eligibility requires one exact compact-v2 leaf at the supplied revision in `escalated`, its matching receipt, exactly one completed receipt-published FINALIZE attempt whose last transition is `review/complete-verification`, a `procedural_tooling_failed` record whose digest and raw bytes match both journal and terminal bindings, and an unchanged exact live `CurrentSnapshot`. A genuine `verification_failed` outcome is permanently ineligible. Reviewer-result, correction, scoped-validator, SDD, ambiguous, superseded, already-retried, or target-drift states are also ineligible.

The incident file must be the compact canonical JSON object plus one LF for `gentle-ai.review-final-verification-incident/v1`. Its only class is `procedural_tooling_failure`; it binds predecessor lineage, terminal and validating revisions, current target identity, failed-evidence hash, and FINALIZE request digest. Inspect its closed shape with `gentle-ai review schema final-verification-incident`; the native parser additionally enforces canonical bytes.

The maintainer authorization is exact LF-only text in this order: `gentle-ai.review-final-verification-retry-authorization/v1`, predecessor lineage and revision, successor lineage, validating revision, target identity, failed-evidence hash, FINALIZE request digest, incident class and digest, actor, and reason. Public STATUS emits only path-free provider inputs and collects this authorization externally; it never emits the completed authorization or the failed-evidence path.

Creation is revision-CAS guarded under the repository-wide compact lock. The successor is generation `+1` in `validating`; every frozen target, policy, risk, lens, finding, classification, outcome, follow-up, correction attempt, cumulative-line, and budget field is copied exactly, while the active raw hash, record digest, outcome, target, and authority-revision bindings are cleared and recovery proof is added. An exact replay converges on the same successor. Any different replay, collision, stale revision, evidence mismatch, live drift, a different existing successor, or prior ancestry retry returns `final_verification_retry_denied` with `mutation_outcome: not_started` and no authority mutation. Capture new final evidence against the successor's `CurrentSnapshot`, then use normal FINALIZE. Success approves; another failure escalates permanently.

When an incomplete FINALIZE journal applies, negotiated status instead returns `action: reconcile_finalize`, `replayability: status_required`, `reconciliation.required: true`, and `next_transition.kind: stop`. Re-run a non-terminal FINALIZE only with the original content-bound payload; a different payload is a typed reconciliation failure, not a retry. If authority is already terminal and only receipt publication remains, `next_transition.execute` carries the exact explicit lineage and no mutation inputs.

Unqualified gate discovery compares every valid terminal receipt with the live immutable target before selecting authority. Zero exact matches returns `receipt_missing` or `receipt_unrelated`; exactly one scope-changed predecessor returns `receipt_scope_changed` with its complete recovery context. Multiple exact or viable scope-changed predecessors return `receipt_ambiguous` without choosing a predecessor or inventing singular recovery context. The failure requires only `lineage_id`, directs the caller to target-scoped `review.status`, and status returns the canonical sorted candidate lineage IDs for explicit selection. An explicit lineage remains a direct fail-closed lookup and derives its own scope diagnostics. Truly unrelated historical receipts never create false ambiguity.

Persistent compact `LOCK` JSON is advisory diagnostics, not current-holder proof. Status opens and probes the existing inode non-blockingly without creating, truncating, unlinking, or replacing it: live contention is `owned`, a released lock is `released`, and malformed metadata or probe failure is `ambiguous`. START waits at most two seconds for that lock and returns a typed non-retryable timeout or cancellation without claiming a persisted PID or hostname is the holder.

### Preserve the uniform failure envelope

Every failed negotiated operation emits the failure envelope matching its contract: `failure/v1` for integration v1 and `failure/v2` for integration v2, and still exits nonzero. Capabilities defaults to v1; repository operations use the selected envelope when `--contract` is present. Unnegotiated command errors retain their compatibility behavior.

| Field | Runtime meaning |
| --- | --- |
| `operation`, `phase`, `code`, `message` | Stable operation identity, failure boundary, machine code, and bounded package-controlled message. |
| `mutation_outcome` | Exactly `not_started`, `unknown`, or `committed`; uncertainty is never weakened to a no-mutation claim. |
| `authority_applicability` | `current_target`, `unrelated`, `ambiguous`, `corrupted`, or `not_evaluated`. |
| `retry_safe`, `replayability` | Independent retry and replay safety. Unknown mutation requires status; exact replay requires the declared identity. |
| `lineage_id`, `request_digest` | Present only when the provider has safe canonical replay evidence. |
| `required_inputs`, `next_action` | The bounded input names and one safe follow-up action. |
| `context` | Optional strict diagnostics. Scope change includes expected and actual tree/path evidence, canonical differing paths/count/digest, predecessor identity, and explicit `review.recover` inputs. Binding CAS conflicts expose the caller's expected binding revision and the current native revision; either value may be empty for the initial bind. |

Messages never contain authority or receipt paths, locks, tokens, raw provider stderr, completed repair authorizations, or canonical store bytes. Invalid or unsupported explicit contracts fail before mutation through the same envelope. A negotiated gate denial is a failure envelope, not a successful operation result; gate evaluation remains read-only. Malformed state, checksum, graph, or receipt evidence is semantic `corrupted` authority. Git command, timeout, process-control, cancellation/deadline, maintenance-lock timeout or cancellation, and non-missing filesystem failures instead propagate as operational errors and never become a successful `corrupted` status result. A valid terminal `invalidated` state remains complete, authoritative, and auditable, but delivery and ordinary status routing still refuse it.

Negotiated operations have a 25-second aggregate budget. Local Git children have a 120-second budget, remote `ls-remote` children have a 180-second budget, and every child uses a one-second wait delay after cancellation. The per-command child budgets are hang guards, not latency assertions: they sit far above scheduler-starvation dilation on loaded runners, a cut child reports the exact command and its observed elapsed time, and inside a negotiated operation the 25-second aggregate budget still fires first. `operation_timeout`, `git_command_timeout`, and `git_command_failed` are typed, non-amplifying failures with `retry_safe: false`. Process-control failures — a Git child that could not be started or whose process tree could not be brought under control (for example Windows job-object or resume failures) — classify as `git_command_failed` and carry the underlying cause in `message`. Read-only and proven pre-transition Git failures report `not_started`. Negotiated START renders and validates a new target's context before creating authority. If context rendering instead fails after START selected an existing durable authority, the failure reports `phase: native_committed`, `mutation_outcome: unknown`, the exact lineage input, and `next_action: review.status`; it never falsely reports `not_started` or recommends replay. Once FINALIZE has committed any native transition, a later Git or process failure follows the same unknown/status rule. Deterministic lock, receipt-discovery, and scope-change failures never recommend automatic retry.

## Reconcile interruptions before replay

| Replayability | Consumer behavior |
| --- | --- |
| `not_replayable` | Do not repeat the mutation from transport evidence alone. |
| `exact_replay_safe` | Replay only the provider-declared canonical request with every required input unchanged. |
| `status_required` | Run target-scoped status before deciding whether any replay is safe. |
| `manual_action_required` | Stop and obtain the named maintainer action or repair prerequisite. |

Reviewer-input schema and semantic preflight rejection happens before journal creation or authority mutation, so the caller may correct the input and retry the same explicit lineage. That retry is neither a correction attempt nor a journal replay. Once preflight succeeds, the provider atomically writes a separate `finalize-attempt-journal.json` before FINALIZE mutates compact authority. It binds lineage, the expected entry revision, a canonical request digest, candidate and payload digests (reviewer results, correction forecast, validation, refuter, evidence, and failed flag), and each committed transition. The journal never stores caller paths and does not alter historical `review-state.json` compatibility. Every journal replacement is reread as strict exact content after rename; an incomplete entry accepts only its exact matching request and is reconciled against current authority. Any mismatch fails with the typed replay-mismatch contract instead of becoming a generic retry.

Finalize commits terminal compact authority before publishing its derived receipt. If receipt publication is interrupted after that commit, the failure envelope reports `mutation_outcome: committed`, `exact_replay_safe`, the lineage, and the canonical request digest. That declaration permits the exact explicit-lineage finalize replay with no new review inputs; target status independently reports the same publication-pending condition after restart. The replay derives the same receipt bytes and does not mutate authority or open another budget. If a different or non-regular receipt already occupies the immutable path, replay cannot succeed: negotiated failure reports `receipt_publication_conflict`, `manual_action_required`, and `explicit-maintainer-action` instead.

Terminal compact receipts are published with a synced temporary file and a platform-native atomic no-clobber operation: an exact existing byte sequence is an idempotent success, while different or non-regular existing content is rejected without replacement. On filesystems that support directory synchronization, the parent directory is synced after publication. Windows may reject directory-handle synchronization; Gentle AI still provides atomic visibility and conflict rejection there, but does not claim power-loss durability for the directory entry.

SDD review bindings are records in the repository-common native SDD runtime CAS chain, not mutable `binding.json` authority. A repository with the old compatibility file imports it exactly once in the first native binding record and never writes it back or consults it after import. Binding replacement compares `expected_binding_revision` only with the effective binding revision; an authority revision or runtime-ledger HEAD is the wrong token and returns `binding_revision_conflict` before publication with `context.binding_revision.expected` and `.current`. Immutable records are published no-clobber, then one atomically replaced `HEAD` selects the chain. A post-HEAD directory-sync failure reports `binding_publication_pending` with `exact_replay_safe`; replay `review.bind_sdd` with the same change, lineage, and expected binding revision.

An ambiguous or lost transport result is never proof of `not_started`. Reconcile it with `review.status`; do not launch another reviewer, correction, or lineage while the outcome is unknown.

For `gate_scope_changed` or `receipt_scope_changed`, use the strict `context.scope_change` evidence to present the exact drift. Recovery remains explicit: pass `predecessor_lineage_id`, `expected_predecessor_revision`, a distinct `successor_lineage_id`, `disposition`, `reason`, and `actor` to `review.recover`. Diagnostics never create a successor, allocate another budget, or mutate the predecessor.

When a release merge retains an approved `current-changes` candidate but expands its path scope, add `--release-scope` to that explicit `scope_changed` recovery. The provider derives an immutable `HEAD^1..HEAD` base-diff; it rejects caller-selected base flags, candidate-tree changes, projection changes, omitted predecessor paths, and non-expanding scopes. The fresh successor must complete its newly derived review tier before the release gate can allow publication.

Malformed reviewer JSON, missing required reviewer arrays, canonicalization failures, and selected-lens mismatches are deterministic preflight failures. Negotiated finalize reports `invalid_request`, `mutation_outcome: not_started`, `retry_safe: true`, `replayability: not_replayable`, and `next_action: correct_request`, while preserving a valid requested lineage for target-scoped recovery. Correct the payload before retrying; do not run authority repair.

### Reopen unusable validating results without another budget

`gentle-ai review reopen-results` is a bounded maintenance operation for an uncorrected validating or correction-required authority. Native detection quarantines a historical reviewer artifact that was unadmitted or whose preserved evidence says candidate inspection was unavailable; a maintainer may additionally name admitted results with `--quarantine-lens <lens>` when the reviewers' input, not the candidate, was wrong — the emitted authorization binds those named lenses verbatim, the overridden bytes are preserved in quarantine, and the audit record lists every authorized lens. It never starts a lineage or recalculates target, tier, lenses, changed-line count, or correction budget, and a completed correction attempt closes the door for good.

First run `--prepare` with the exact lineage, authority revision, target, reason, and actor. Native classification re-decodes and re-admits the exact bytes in every frozen lens slot, then compares their canonical and stored hashes: only a current provider-admitted match is retained; historical, unadmitted, inspection-unavailable, or tampered slots are quarantined. The returned plan contains the exact maintainer authorization. Re-run without `--prepare` and pass that authorization unchanged. Under the store lock, Gentle AI rechecks the same slot classification, archives quarantined bytes and digest sidecars before replacement, records the transition, and moves the same lineage from `validating` to `reviewing`. Retry with the exact request is convergent. A receipt, stale revision, changed artifact, corrected authority, unknown artifact failure, or a plan with no unusable slot fails closed.

An escalated predecessor may transfer review and correction evidence to a fresh successor only for the `recovered_correction_evidence` class. The predecessor must contain one otherwise successful correction that exceeded its frozen budget only under stored historical line accounting. Native Git evidence must prove a smaller positive correction within both forecast and budget; the predecessor initial target to successor target must classify as `changed-scope` with the same genesis paths; and the corrected predecessor candidate tree must equal the successor initial tree exactly. Policy, risk, lenses, projection, intended-untracked set, path scope, receipt, predecessor revision, review evidence hash, correction attempt, and targeted-validation request all remain bound. The successor starts directly in `validating` and still requires successor-bound final verification. Any changed bytes or mismatched scope start a fresh reviewing successor instead; evidence is never partially imported.

## Preserve compatibility without reopening legacy mutation

Compact-v2 is the sole ordinary mutable authority. Legacy-v1 is in an active, release-based compatibility window with these guarantees:

- Valid applicable historical receipts remain readable and evaluable at supported gates.
- Ordinary legacy mutation through START, finalize, BIND-SDD, invalidation, and direct append—including the `review-step` compatibility route—returns the typed `LegacyReadOnlyError`, preserves `errors.Is(ErrLegacyReadOnly)`, and exposes stable code `legacy_v1_read_only` without changing authority bytes across retries or restarts.
- Negotiated wrappers preserve that typed cause as `legacy_v1_read_only` with `mutation_outcome: not_started`, retry and replay disabled, `next_action: stop`, and a package-controlled message that contains no provider paths or raw diagnostics.
- Applicable non-terminal legacy status returns the deterministic read-only action `stop`; applicable approved legacy receipts remain evaluable at supported gates.
- Applicable approved legacy status validates the canonical published v1 receipt and reports its SHA-256 identity as `present`. Legacy-v1 never reports `publication_pending`; a missing, corrupt, or wrong legacy receipt fails closed as corrupted authority without compact exact-replay semantics.
- Frozen tier, authored-line count, and correction budget are compact-v2 fields. Historical `ordinary_4r` legacy status omits `frozen` rather than inventing values; compact current targets still require the complete frozen object.
- Unrelated valid legacy history does not block a current compact target.
- An explicit valid compact lineage remains `current_target` when unrelated malformed legacy history exists. Unscoped inventory still fails closed and reports the malformed history; the provider does not quarantine or repair it automatically.
- Same-lineage mixed v1/v2 authority and unclassifiable corruption fail closed.
- The public classified repair may quarantine exactly one proven historical alias lineage; it never appends to, rewrites, migrates, or validates legacy history.
- Explicit maintenance transport import/export may preserve historical compatibility.
- Removal is not scheduled and requires at least one compatibility release plus separate reachability evidence.

The provider does not auto-upgrade, migrate, rewrite, quarantine, or delete legacy authority. A later deletion is a separate compatibility decision, not part of protocol v1 negotiation.

## Respect compatibility and non-goals

Protocol v1 supports `workspace` and `staged` projections and preserves existing compact authority and receipt schemas. Published archives contain the versioned JSON Schemas and conformance fixtures under `contracts/review-integration/v1/` and `contracts/review-integration/v2/`; consumers should validate against those packaged bytes rather than copying private Go structs.

This contract does not implement Gentle Pi, select a model or provider, transmit repository data, add remote telemetry, claim Windows runtime durability, define an archive coordinator, defend against a malicious actor with local filesystem access, or authorize a command merely because review passed.

## Consume the contract from Gentle Pi

Gentle Pi should remain a thin consumer:

1. Resolve and independently verify the exact Gentle AI executable.
2. Negotiate capabilities before repository work and cache them only for that executable identity.
3. Use negotiated status to reconstruct the provider-selected projection after restart.
4. Execute reviewers and validators, then pass their typed results to finalize without constructing authority bytes.
5. Preserve native actions, gate results, replayability, and mutation outcomes without semantic remapping.
6. Reconcile uncertain mutations through status before an exact replay.
7. Keep command interception, worktrees, user confirmation, and final intent rederivation on the Pi side.

Pi adoption, fallback retirement, package pinning, and Pi release sequencing are separate consumer work. They do not change Gentle AI's provider authority or release ownership.

## Inspect packaged contract artifacts

Each release archive contains:

- `contracts/review-integration/v1/schemas/` — 24 strict JSON Schemas, including preserved capability protocols v1.0–v1.4, current v1.5, versioned START/status/result-artifact contracts, outcome-bound verification evidence, final-verification incident, classified repair, provider subject/admission, correction planning, and targeted validation.
- `contracts/review-integration/v1/fixtures/` — 27 deterministic conformance fixtures, including all six capability minors, preserved v1 plus current v2 START/status examples, outcome-bound verification evidence, the final-verification incident and retry projection, classified repair preflight, and typed failure envelopes.
- `contracts/review-integration/v2/schemas/` — 15 strict JSON Schemas, including the capture-result dry-run response contract.
- `contracts/review-integration/v2/fixtures/` — 9 deterministic conformance fixtures, including the capture-result dry-run response.
- `docs/review-integration.md` — this ownership and consumption guide.

Repository maintainers can verify source inventory or a complete GoReleaser snapshot:

```bash
scripts/test-review-contract-package.sh
scripts/test-review-contract-package.sh dist
```

The archive assertion compares every packaged contract file with the repository source by SHA-256 and verifies each platform archive against `checksums.txt`.

### Next steps

- Read the [review authority threat model](review-authority-threat-model.md) before integrating delivery authorization.
- Query `review capabilities` from the exact executable you intend to run.
- Validate the packaged fixtures before implementing or updating a consumer.

← [Back to README](../README.md)
