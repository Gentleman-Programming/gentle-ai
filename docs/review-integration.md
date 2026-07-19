# Review Integration Contract

← [Back to README](../README.md)

`gentle-ai.review-integration/v1` is the versioned provider contract for consumers that coordinate Gentle AI's native bounded review lifecycle. It lets a consumer negotiate capabilities, reconstruct one target after restart, drive explicit review operations, and validate the resulting receipt without reading provider-private authority files.

## Negotiate the provider first

Resolve the exact `gentle-ai` executable that will perform review operations, then query it outside a repository:

```bash
gentle-ai review capabilities \
  --contract gentle-ai.review-integration/v1
```

The response identifies the protocol major, package and build identity, executable SHA-256, operations, five gates, projections, schemas, mandatory and optional features, and compatibility window. The executable digest is self-reported evidence; compare it with the published release manifest before trusting the binary.

Protocol v1.1 advertises the distinct `gentle-ai.review-integration.capabilities/v1.1` schema and additive optional features for `base_ref_workspace_overlay`, `bounded_process_waits`, `exact_gate_receipt_discovery`, `native_low_risk_verification`, `risk_reasons`, and `scope_change_diagnostics`. Protocol v1.0 artifacts remain available under `gentle-ai.review-integration.capabilities/v1`; old consumers must reject the unknown v1.1 identity, and new consumers must require protocol 1.1 and validate its schema before using overlay. The overlay feature requires immutable snapshots and restart-safe projection.

Consumers MUST reject an incompatible protocol major, an unsupported mandatory feature, an unknown mandatory enum, or a schema identity mismatch. Unknown optional fields may be ignored only under the advertised additive-minor policy. Existing unnegotiated CLI responses remain separate compatibility surfaces and do not gain negotiated fields silently.

Pass the same contract explicitly to negotiated repository operations:

```bash
gentle-ai review start --contract gentle-ai.review-integration/v1 --cwd .
gentle-ai review status --contract gentle-ai.review-integration/v1 --cwd .
gentle-ai review finalize --contract gentle-ai.review-integration/v1 --cwd . --lineage <lineage> ...
gentle-ai review validate --contract gentle-ai.review-integration/v1 --cwd . --gate pre-commit
gentle-ai review bind-sdd --contract gentle-ai.review-integration/v1 --cwd . --change <change> --lineage <lineage> --expected-binding-revision=<revision>
```

## Keep provider and consumer ownership separate

| Gentle AI provider owns | Consumer owns |
| --- | --- |
| Git-derived immutable snapshot identity and projection | User interaction and explicit maintainer confirmation |
| Deterministic risk reasons, tier, lenses, and correction budget | Reviewer, validator, and correction actor execution |
| Compact-v2 authority transitions, lock, and expected-revision CAS | Process invocation, cancellation, and transport diagnostics |
| Receipt derivation and exact receipt publication replay | Rendering native outcomes without weakening them |
| Target applicability, replayability, and gate evaluation | Rechecking command intent immediately before execution |
| Approved-receipt binding for SDD | Derived worktree and temporary-view lifecycle |

Consumers MUST NOT reconstruct receipts, derive canonical hashes, inspect the Git common-dir authority store, select an ambiguous lineage automatically, or infer that a transport interruption did not mutate state. Gentle AI does not choose models, run arbitrary user commands, or replace a consumer's command-safety policy.

## Drive the bounded operation set

| Operation | Mutation boundary | Contract behavior |
| --- | --- | --- |
| `review.capabilities` | None | Reports the deterministic repository-independent provider surface. |
| `review.start` | Compact authority | Freezes one target, tier, lens set, and correction budget; it never starts because a gate was invoked. |
| `review.status` | None | Reconstructs target-scoped applicability, projection, lifecycle, and one next action. |
| `review.finalize` | Compact authority and derived receipt | Accepts selected lens results and bounded correction evidence, performs deterministic native verification for an eligible low-risk zero-lens target, or performs an exact receipt-publication replay. |
| `review.validate` | None | Revalidates one existing content-bound receipt at a named lifecycle gate. |
| `review.bind_sdd` | SDD binding artifact | Binds only an approved receipt to an SDD change. |

`review.start` is the only ordinary entry point that creates a review budget. Finalize continues that frozen lifecycle. Status, validation, and gates are read-only and never allocate a reviewer, actor, lineage, or correction budget.

`gentle-ai review capture-result` is an additive headless command, not a negotiated `review-integration/v1` repository operation. It accepts no `--contract` and emits a manifest with capability `review.native_result_artifact` and schema `gentle-ai.review-result-artifact/v1`; that native artifact schema remains advertised for consumer validation.

### Choose the target explicitly

| Invocation | Frozen boundary |
| --- | --- |
| `review start` | `HEAD` to the synthetic staged/unstaged/intended-untracked workspace tree. |
| `review start --base-ref <ref> --committed-only` | `<ref>` to `HEAD`; workspace changes are excluded. |
| `review start --base-ref <ref> --workspace-overlay` | `<ref>` to the synthetic workspace tree, including branch commits and staged, unstaged, and intended-untracked bytes. |

Overlay mode requires workspace projection and cannot be combined with `--committed-only`. START returns `target_mode`, `target_identity`, `base_tree`, and `candidate_tree` only for this mode. Restarted consumers select the frozen target with `review status --base-tree <START base_tree> --workspace-overlay`; `--base-ref` remains available for a fresh symbolic selection, but cannot be combined with `--base-tree`. Existing workspace-only and committed-only payloads remain unchanged. Snapshot construction uses a temporary index and does not mutate the real index or worktree.

Reviewer results may omit the top-level `lens`; when present, it must match the selected-lens position returned by start. Both the short names (`risk`, `resilience`, `readability`, `reliability`) and the negotiated facade names (`review-risk`, `review-resilience`, `review-readability`, `review-reliability`) map to the same native lenses. A mismatch is rejected before authority mutation instead of being overwritten.

Durable controllers capture each result with exact lineage, target, lens, and selected order, then pass the emitted manifests to FINALIZE as ordered `--result-artifact` values. Legacy `--result` files remain compatible but cannot be mixed with artifact manifests and are not a durable cross-agent handoff.

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

### Follow applicability and action, not inventory

| Applicability | Meaning |
| --- | --- |
| `current_target` | Exactly one validated authority applies to the requested Git target. |
| `unrelated` | No authority applies, even when unrelated historical authority exists. |
| `ambiguous` | More than one authority applies or a required lineage selector is missing. |
| `corrupted` | Authority required for classification cannot be validated safely. |

The provider returns one action from `start`, `finalize`, `validate`, `recover`, `maintainer_action`, `select_lineage`, `repair_authority`, `reconcile_finalize`, or `stop`. Applicable non-terminal legacy-v1 authority always returns `stop`; it never recommends finalize or another mutation. A consumer MUST stop on ambiguity, corruption, invalidation, escalation, or `stop` and present the provider's required action. It MUST NOT silently choose, reset, quarantine, or create a lineage.

When an incomplete FINALIZE journal applies, negotiated status instead returns `action: reconcile_finalize`, `replayability: status_required`, and `reconciliation.required: true`. Re-run a non-terminal FINALIZE only with the original content-bound payload; a different payload is a typed reconciliation failure, not a retry. If authority is already terminal and only receipt publication remains, replay with the exact explicit lineage and no mutation inputs.

Unqualified gate discovery compares every valid terminal receipt with the live immutable target before selecting authority. Zero exact matches returns `receipt_missing` or `receipt_unrelated`; exactly one scope-changed predecessor returns `receipt_scope_changed` with its complete recovery context. Multiple exact or viable scope-changed predecessors return `receipt_ambiguous` without choosing a predecessor or inventing singular recovery context. The failure requires only `lineage_id`, directs the caller to target-scoped `review.status`, and status returns the canonical sorted candidate lineage IDs for explicit selection. An explicit lineage remains a direct fail-closed lookup and derives its own scope diagnostics. Truly unrelated historical receipts never create false ambiguity.

Persistent compact `LOCK` JSON is advisory diagnostics, not current-holder proof. Status opens and probes the existing inode non-blockingly without creating, truncating, unlinking, or replacing it: live contention is `owned`, a released lock is `released`, and malformed metadata or probe failure is `ambiguous`. START waits at most two seconds for that lock and returns a typed non-retryable timeout or cancellation without claiming a persisted PID or hostname is the holder.

### Preserve the uniform failure envelope

Every failed operation explicitly negotiated through `gentle-ai.review-integration/v1` emits `gentle-ai.review-integration.failure/v1` and still exits nonzero. Capabilities uses this envelope by default; repository operations use it when `--contract` is present. Unnegotiated command errors retain their compatibility behavior.

| Field | Runtime meaning |
| --- | --- |
| `operation`, `phase`, `code`, `message` | Stable operation identity, failure boundary, machine code, and bounded package-controlled message. |
| `mutation_outcome` | Exactly `not_started`, `unknown`, or `committed`; uncertainty is never weakened to a no-mutation claim. |
| `authority_applicability` | `current_target`, `unrelated`, `ambiguous`, `corrupted`, or `not_evaluated`. |
| `retry_safe`, `replayability` | Independent retry and replay safety. Unknown mutation requires status; exact replay requires the declared identity. |
| `lineage_id`, `request_digest` | Present only when the provider has safe canonical replay evidence. |
| `required_inputs`, `next_action` | The bounded input names and one safe follow-up action. |
| `context` | Optional strict gate-denial evidence. Scope change includes denial stage/code, expected and actual tree/path evidence, canonical differing paths/count/digest, predecessor identity, and explicit `review.recover` inputs. |

Messages never contain authority or receipt paths, locks, tokens, raw provider stderr, or canonical store bytes. Invalid or unsupported explicit contracts fail before mutation through the same envelope. A negotiated gate denial is a failure envelope, not a successful operation result; gate evaluation remains read-only.

Negotiated operations have a 25-second aggregate budget. Local Git children have a 15-second budget, remote `ls-remote` children have a 20-second budget, and every child uses a one-second wait delay after cancellation. `operation_timeout`, `git_command_timeout`, and `git_command_failed` are typed, non-amplifying failures with `retry_safe: false`. Process-control failures — a Git child that could not be started or whose process tree could not be brought under control (for example Windows job-object or resume failures) — classify as `git_command_failed` and carry the underlying cause in `message`. Read-only and proven pre-transition Git failures report `not_started`. Once FINALIZE has committed any native transition, a later Git or process failure reports `mutation_outcome: unknown`, `authority_applicability: current_target`, and `next_action: review.status`; it never weakens committed progress to `not_started` or recommends replay. Deterministic lock, receipt-discovery, and scope-change failures never recommend automatic retry.

## Reconcile interruptions before replay

| Replayability | Consumer behavior |
| --- | --- |
| `not_replayable` | Do not repeat the mutation from transport evidence alone. |
| `exact_replay_safe` | Replay only the provider-declared canonical request with every required input unchanged. |
| `status_required` | Run target-scoped status before deciding whether any replay is safe. |
| `manual_action_required` | Stop and obtain the named maintainer action or repair prerequisite. |

Before FINALIZE mutates compact authority, the provider atomically writes a separate `finalize-attempt-journal.json`. It binds lineage, the expected entry revision, a canonical request digest, candidate and payload digests (reviewer results, correction forecast, validation, refuter, evidence, and failed flag), and each committed transition. The journal never stores caller paths and does not alter historical `review-state.json` compatibility. Every journal replacement is reread as strict exact content after rename; an incomplete entry accepts only its matching request and is reconciled against current authority rather than generically replayed.

Finalize commits terminal compact authority before publishing its derived receipt. If receipt publication is interrupted after that commit, the failure envelope reports `mutation_outcome: committed`, `exact_replay_safe`, the lineage, and the canonical request digest. That declaration permits the exact explicit-lineage finalize replay with no new review inputs; target status independently reports the same publication-pending condition after restart. The replay derives the same receipt bytes and does not mutate authority or open another budget. If a different or non-regular receipt already occupies the immutable path, replay cannot succeed: negotiated failure reports `receipt_publication_conflict`, `manual_action_required`, and `explicit-maintainer-action` instead.

Terminal compact receipts are published with a synced temporary file and a platform-native atomic no-clobber operation: an exact existing byte sequence is an idempotent success, while different or non-regular existing content is rejected without replacement. On filesystems that support directory synchronization, the parent directory is synced after publication. Windows may reject directory-handle synchronization; Gentle AI still provides atomic visibility and conflict rejection there, but does not claim power-loss durability for the directory entry. SDD review-binding publication follows the same post-rename directory-sync rule. A post-rename sync failure reports `binding_publication_pending` with `exact_replay_safe`; replay `review.bind_sdd` with the same change, lineage, and expected binding revision.

An ambiguous or lost transport result is never proof of `not_started`. Reconcile it with `review.status`; do not launch another reviewer, correction, or lineage while the outcome is unknown.

For `gate_scope_changed` or `receipt_scope_changed`, use the strict `context.scope_change` evidence to present the exact drift. Recovery remains explicit: pass `predecessor_lineage_id`, `expected_predecessor_revision`, a distinct `successor_lineage_id`, `disposition`, `reason`, and `actor` to `review.recover`. Diagnostics never create a successor, allocate another budget, or mutate the predecessor.

When a release merge retains an approved `current-changes` candidate but expands its path scope, add `--release-scope` to that explicit `scope_changed` recovery. The provider derives an immutable `HEAD^1..HEAD` base-diff; it rejects caller-selected base flags, candidate-tree changes, projection changes, omitted predecessor paths, and non-expanding scopes. The fresh successor must complete its newly derived review tier before the release gate can allow publication.

Malformed reviewer JSON, missing required reviewer arrays, canonicalization failures, and selected-lens mismatches are deterministic preflight failures. Negotiated finalize reports `invalid_request`, `mutation_outcome: not_started`, `retry_safe: true`, `replayability: not_replayable`, and `next_action: correct_request`, while preserving a valid requested lineage for target-scoped recovery. Correct the payload before retrying; do not run authority repair.

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
- Explicit maintenance transport import/export may preserve historical compatibility.
- Removal is not scheduled and requires at least one compatibility release plus separate reachability evidence.

The provider does not auto-upgrade, migrate, rewrite, quarantine, or delete legacy authority. A later deletion is a separate compatibility decision, not part of protocol v1 negotiation.

## Respect compatibility and non-goals

Protocol v1 supports `workspace` and `staged` projections and preserves existing compact authority and receipt schemas. Published archives contain the versioned JSON Schemas and conformance fixtures under `contracts/review-integration/v1/`; consumers should validate against those packaged bytes rather than copying private Go structs.

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

- `contracts/review-integration/v1/schemas/` — the original six strict JSON Schemas plus the result-artifact schema.
- `contracts/review-integration/v1/fixtures/` — eight deterministic conformance fixtures, including all four target-applicability states.
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
